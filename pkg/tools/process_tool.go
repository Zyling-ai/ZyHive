package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

const (
	maxBackgroundProcessesGlobal   = 32
	maxBackgroundProcessesPerOwner = 4
	maxBackgroundSessionsGlobal    = 128
	maxBackgroundSessionsPerOwner  = 16
	maxBackgroundOutputBytes       = 1024 * 1024
	defaultBackgroundTimeout       = 30 * time.Minute
	maxBackgroundTimeout           = 30 * time.Minute
	backgroundRetention            = 10 * time.Minute
)

type processOwner struct {
	AgentID   string
	SessionID string
}

func (r *Registry) processOwner() processOwner {
	return processOwner{AgentID: r.agentID, SessionID: r.sessionID}
}

type cappedBuffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxBackgroundOutputBytes - len(b.data)
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		b.data = append(b.data, p[:n]...)
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := string(b.data)
	if b.truncated {
		out += "\n[output truncated at 1 MiB]"
	}
	return out
}

type processSpec struct {
	Name            string
	Args            []string
	Dir             string
	Env             []string
	InitialStdin    string
	CloseStdinAfter bool
	Timeout         time.Duration
	Kind            string
}

type bgSession struct {
	id      string
	owner   processOwner
	kind    string
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	stdin   io.WriteCloser
	stdout  *cappedBuffer
	stderr  *cappedBuffer
	done    chan struct{}
	stateMu sync.Mutex
	stdinMu sync.Mutex

	status    string
	exitCode  int
	exitErr   string
	startedAt time.Time
	endedAt   time.Time
}

func (s *bgSession) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *bgSession) state() (string, int, string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.status, s.exitCode, s.exitErr
}

func (s *bgSession) output() string {
	stdout := s.stdout.String()
	stderr := s.stderr.String()
	if stderr == "" {
		return stdout
	}
	if stdout == "" {
		return "[stderr]\n" + stderr
	}
	return stdout + "\n[stderr]\n" + stderr
}

func (s *bgSession) write(data string) error {
	if s.isDone() {
		return fmt.Errorf("process already exited")
	}
	s.stdinMu.Lock()
	defer s.stdinMu.Unlock()
	if s.stdin == nil {
		return fmt.Errorf("process stdin is closed")
	}
	_, err := io.WriteString(s.stdin, data)
	return err
}

type processManager struct {
	mu       sync.Mutex
	sessions map[string]*bgSession
}

func newProcessManager() *processManager {
	return &processManager{sessions: make(map[string]*bgSession)}
}

var backgroundProcesses = newProcessManager()

// CloseBackgroundProcesses terminates every managed subprocess during server shutdown.
func CloseBackgroundProcesses() {
	backgroundProcesses.reset()
}

func normalizeProcessTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 || timeout > maxBackgroundTimeout {
		return defaultBackgroundTimeout
	}
	return timeout
}

func (m *processManager) start(owner processOwner, spec processSpec) (string, error) {
	if owner.AgentID == "" {
		return "", fmt.Errorf("background process requires an agent owner")
	}
	if spec.Name == "" {
		return "", fmt.Errorf("background process command is required")
	}
	spec.Timeout = normalizeProcessTimeout(spec.Timeout)

	m.mu.Lock()
	m.pruneCompletedLocked(owner)
	globalRunning := 0
	ownerRunning := 0
	for _, existing := range m.sessions {
		if existing.isDone() {
			continue
		}
		globalRunning++
		if existing.owner == owner {
			ownerRunning++
		}
	}
	if globalRunning >= maxBackgroundProcessesGlobal {
		m.mu.Unlock()
		return "", fmt.Errorf("global background process limit reached (%d)", maxBackgroundProcessesGlobal)
	}
	if ownerRunning >= maxBackgroundProcessesPerOwner {
		m.mu.Unlock()
		return "", fmt.Errorf("background process limit reached for this agent/session (%d)", maxBackgroundProcessesPerOwner)
	}

	id, err := newProcessID()
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), spec.Timeout)
	cmd := exec.Command(spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	prepareOwnedProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		m.mu.Unlock()
		return "", fmt.Errorf("create stdin pipe: %w", err)
	}
	session := &bgSession{
		id:        id,
		owner:     owner,
		kind:      spec.Kind,
		cmd:       cmd,
		cancel:    cancel,
		stdin:     stdin,
		stdout:    &cappedBuffer{},
		stderr:    &cappedBuffer{},
		done:      make(chan struct{}),
		status:    "running",
		exitCode:  -1,
		startedAt: time.Now(),
	}
	cmd.Stdout = session.stdout
	cmd.Stderr = session.stderr
	m.sessions[id] = session
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
		return "", fmt.Errorf("start background process: %w", err)
	}
	stopGroupWatcher := watchOwnedProcessGroup(ctx, cmd)
	go m.wait(session, ctx, spec, stopGroupWatcher)
	return id, nil
}

func (m *processManager) pruneCompletedLocked(owner processOwner) {
	type candidate struct {
		id        string
		startedAt time.Time
	}
	ownerTotal := 0
	allDone := make([]candidate, 0)
	ownerDone := make([]candidate, 0)
	for id, session := range m.sessions {
		if session.owner == owner {
			ownerTotal++
		}
		if !session.isDone() {
			continue
		}
		item := candidate{id: id, startedAt: session.startedAt}
		allDone = append(allDone, item)
		if session.owner == owner {
			ownerDone = append(ownerDone, item)
		}
	}
	sort.Slice(ownerDone, func(i, j int) bool {
		return ownerDone[i].startedAt.Before(ownerDone[j].startedAt)
	})
	for ownerTotal >= maxBackgroundSessionsPerOwner && len(ownerDone) > 0 {
		oldest := ownerDone[0]
		ownerDone = ownerDone[1:]
		if _, ok := m.sessions[oldest.id]; ok {
			delete(m.sessions, oldest.id)
			ownerTotal--
		}
	}
	sort.Slice(allDone, func(i, j int) bool {
		return allDone[i].startedAt.Before(allDone[j].startedAt)
	})
	for len(m.sessions) >= maxBackgroundSessionsGlobal && len(allDone) > 0 {
		oldest := allDone[0]
		allDone = allDone[1:]
		delete(m.sessions, oldest.id)
	}
}

func (m *processManager) wait(
	session *bgSession,
	ctx context.Context,
	spec processSpec,
	stopGroupWatcher func(),
) {
	defer session.cancel()
	defer stopGroupWatcher()
	if spec.InitialStdin != "" {
		_ = session.write(spec.InitialStdin)
	}
	if spec.CloseStdinAfter {
		session.stdinMu.Lock()
		_ = session.stdin.Close()
		session.stdin = nil
		session.stdinMu.Unlock()
	}
	err := session.cmd.Wait()
	session.stdinMu.Lock()
	if session.stdin != nil {
		_ = session.stdin.Close()
		session.stdin = nil
	}
	session.stdinMu.Unlock()

	status := "done"
	exitCode := 0
	exitErr := ""
	if err != nil {
		status = "error"
		exitErr = err.Error()
		if exit, ok := err.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
		}
	}
	switch ctx.Err() {
	case context.DeadlineExceeded:
		status = "timeout"
		exitErr = "maximum runtime exceeded"
	case context.Canceled:
		status = "killed"
		exitErr = "cancelled"
	}
	session.stateMu.Lock()
	session.status = status
	session.exitCode = exitCode
	session.exitErr = exitErr
	session.endedAt = time.Now()
	session.stateMu.Unlock()
	close(session.done)

	time.AfterFunc(backgroundRetention, func() {
		m.mu.Lock()
		if current, ok := m.sessions[session.id]; ok && current == session && session.isDone() {
			delete(m.sessions, session.id)
		}
		m.mu.Unlock()
	})
}

func (m *processManager) get(owner processOwner, id string) (*bgSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	return session, ok && session.owner == owner
}

func (m *processManager) list(owner processOwner) []*bgSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessions := make([]*bgSession, 0)
	for _, session := range m.sessions {
		if session.owner == owner {
			sessions = append(sessions, session)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].startedAt.Before(sessions[j].startedAt)
	})
	return sessions
}

func (m *processManager) kill(owner processOwner, id string) error {
	session, ok := m.get(owner, id)
	if !ok {
		return fmt.Errorf("process %q not found", id)
	}
	if session.isDone() {
		return nil
	}
	session.cancel()
	killOwnedProcessGroup(session.cmd)
	return nil
}

func (m *processManager) reset() {
	m.mu.Lock()
	sessions := make([]*bgSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*bgSession)
	m.mu.Unlock()
	for _, session := range sessions {
		if session.cancel != nil {
			session.cancel()
		}
		killOwnedProcessGroup(session.cmd)
		if session.done == nil {
			continue
		}
		select {
		case <-session.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func newProcessID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate process id: %w", err)
	}
	return fmt.Sprintf("proc-%x", raw[:]), nil
}

// ── process tool ─────────────────────────────────────────────────────────────

var processToolDef = llm.ToolDef{
	Name:        "process",
	Description: "Manage background Bash and ACP processes owned by the current agent session. Actions: list, log, write, kill, poll.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"action":{"type":"string","enum":["list","log","write","kill","poll"],"description":"Action to perform"},
			"sessionId":{"type":"string","description":"Process ID returned by bash/acp_spawn (required for log/write/kill/poll)"},
			"data":{"type":"string","description":"Data to write to stdin (for write action)"},
			"offset":{"type":"number","description":"Line offset for log (0-indexed)"},
			"limit":{"type":"number","description":"Max lines to return for log (default 100)"},
			"timeout":{"type":"number","description":"For poll: wait up to N ms before returning"}
		},
		"required":["action"]
	}`),
}

func (r *Registry) handleProcess(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action    string `json:"action"`
		SessionID string `json:"sessionId"`
		Data      string `json:"data"`
		Offset    int    `json:"offset"`
		Limit     int    `json:"limit"`
		Timeout   int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("process: invalid input: %v", err)
	}

	switch p.Action {
	case "list":
		owned := backgroundProcesses.list(r.processOwner())
		sessions := make([]string, 0, len(owned))
		for _, session := range owned {
			status, exitCode, _ := session.state()
			if status == "done" || status == "error" {
				status = fmt.Sprintf("%s(%d)", status, exitCode)
			}
			elapsed := time.Since(session.startedAt).Round(time.Second)
			sessions = append(sessions, fmt.Sprintf("%s [%s, %s, %s]", session.id, session.kind, status, elapsed))
		}
		if len(sessions) == 0 {
			return "No background sessions.", nil
		}
		return strings.Join(sessions, "\n"), nil

	case "log":
		if p.SessionID == "" {
			return "", fmt.Errorf("process log: sessionId required")
		}
		sess, ok := backgroundProcesses.get(r.processOwner(), p.SessionID)
		if !ok {
			return "", fmt.Errorf("process log: session %q not found", p.SessionID)
		}
		combined := sess.output()
		lines := strings.Split(combined, "\n")
		limit := p.Limit
		if limit <= 0 {
			limit = 100
		} else if limit > 1000 {
			limit = 1000
		}
		start := p.Offset
		if start < 0 {
			start = 0
		}
		if start >= len(lines) {
			start = len(lines)
		}
		end := start + limit
		if end > len(lines) {
			end = len(lines)
		}
		return strings.Join(lines[start:end], "\n"), nil

	case "write":
		if p.SessionID == "" {
			return "", fmt.Errorf("process write: sessionId required")
		}
		sess, ok := backgroundProcesses.get(r.processOwner(), p.SessionID)
		if !ok {
			return "", fmt.Errorf("process write: session %q not found", p.SessionID)
		}
		if err := sess.write(p.Data); err != nil {
			return "", fmt.Errorf("process write: %v", err)
		}
		return "written", nil

	case "kill":
		if p.SessionID == "" {
			return "", fmt.Errorf("process kill: sessionId required")
		}
		if err := backgroundProcesses.kill(r.processOwner(), p.SessionID); err != nil {
			return "", fmt.Errorf("process kill: %v", err)
		}
		return "killed", nil

	case "poll":
		if p.SessionID == "" {
			return "", fmt.Errorf("process poll: sessionId required")
		}
		sess, ok := backgroundProcesses.get(r.processOwner(), p.SessionID)
		if !ok {
			return "", fmt.Errorf("process poll: session %q not found", p.SessionID)
		}
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 5000
		} else if timeout > 30000 {
			timeout = 30000
		}

		select {
		case <-sess.done:
			status, exitCode, exitErr := sess.state()
			return fmt.Sprintf("status: %s(%d)\nerror: %s\noutput:\n%s", status, exitCode, exitErr, sess.output()), nil
		case <-time.After(time.Duration(timeout) * time.Millisecond):
			status, _, _ := sess.state()
			return fmt.Sprintf("status: %s\noutput so far:\n%s", status, sess.output()), nil
		}

	default:
		return "", fmt.Errorf("process: unknown action %q", p.Action)
	}
}
