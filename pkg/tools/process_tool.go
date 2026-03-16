package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// ── Background session store ──────────────────────────────────────────────────

type bgSession struct {
	id        string
	cmd       *exec.Cmd
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
	mu        sync.Mutex
	done      chan struct{}
	exitCode  int
	exitErr   string
	startedAt time.Time
}

var (
	bgStore   sync.Map // sessionID → *bgSession
	bgCounter int
	bgCountMu sync.Mutex
)

func newSessionID() string {
	bgCountMu.Lock()
	defer bgCountMu.Unlock()
	bgCounter++
	return fmt.Sprintf("bg-%d-%d", time.Now().Unix(), bgCounter)
}

// startBackground runs a shell command in background and returns sessionID.
func startBackground(command string) (string, error) {
	id := newSessionID()
	sess := &bgSession{
		id:        id,
		stdout:    &bytes.Buffer{},
		stderr:    &bytes.Buffer{},
		done:      make(chan struct{}),
		startedAt: time.Now(),
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = sess.stdout
	cmd.Stderr = sess.stderr
	sess.cmd = cmd

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start failed: %v", err)
	}

	bgStore.Store(id, sess)

	go func() {
		err := cmd.Wait()
		sess.mu.Lock()
		if err != nil {
			sess.exitCode = -1
			sess.exitErr = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				sess.exitCode = exitErr.ExitCode()
			}
		}
		sess.mu.Unlock()
		close(sess.done)
	}()

	return id, nil
}

// ── process tool ─────────────────────────────────────────────────────────────

var processToolDef = llm.ToolDef{
	Name:        "process",
	Description: "Manage background shell sessions started with bash (background=true). Actions: list, log, write, kill, poll.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"action":{"type":"string","enum":["list","log","write","kill","poll"],"description":"Action to perform"},
			"sessionId":{"type":"string","description":"Session ID (required for log/write/kill/poll)"},
			"data":{"type":"string","description":"Data to write to stdin (for write action)"},
			"offset":{"type":"number","description":"Line offset for log (0-indexed)"},
			"limit":{"type":"number","description":"Max lines to return for log (default 100)"},
			"timeout":{"type":"number","description":"For poll: wait up to N ms before returning"}
		},
		"required":["action"]
	}`),
}

func handleProcess(_ context.Context, input json.RawMessage) (string, error) {
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
		var sessions []string
		bgStore.Range(func(k, v any) bool {
			id := k.(string)
			sess := v.(*bgSession)
			status := "running"
			select {
			case <-sess.done:
				status = fmt.Sprintf("exited(%d)", sess.exitCode)
			default:
			}
			elapsed := time.Since(sess.startedAt).Round(time.Second)
			sessions = append(sessions, fmt.Sprintf("%s [%s, %s]", id, status, elapsed))
			return true
		})
		if len(sessions) == 0 {
			return "No background sessions.", nil
		}
		return strings.Join(sessions, "\n"), nil

	case "log":
		if p.SessionID == "" {
			return "", fmt.Errorf("process log: sessionId required")
		}
		v, ok := bgStore.Load(p.SessionID)
		if !ok {
			return "", fmt.Errorf("process log: session %q not found", p.SessionID)
		}
		sess := v.(*bgSession)
		sess.mu.Lock()
		combined := sess.stdout.String()
		if sess.stderr.Len() > 0 {
			combined += "\n[stderr]\n" + sess.stderr.String()
		}
		sess.mu.Unlock()

		lines := strings.Split(combined, "\n")
		limit := p.Limit
		if limit <= 0 {
			limit = 100
		}
		start := p.Offset
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
		v, ok := bgStore.Load(p.SessionID)
		if !ok {
			return "", fmt.Errorf("process write: session %q not found", p.SessionID)
		}
		sess := v.(*bgSession)
		if sess.cmd.Process == nil {
			return "", fmt.Errorf("process write: process not started")
		}
		// Write to stdin via /proc (Linux only fallback)
		stdinPipe, err := os.OpenFile(fmt.Sprintf("/proc/%d/fd/0", sess.cmd.Process.Pid), os.O_WRONLY, 0)
		if err != nil {
			return "", fmt.Errorf("process write: cannot write to stdin: %v", err)
		}
		defer stdinPipe.Close()
		_, err = io.WriteString(stdinPipe, p.Data)
		if err != nil {
			return "", fmt.Errorf("process write: %v", err)
		}
		return "written", nil

	case "kill":
		if p.SessionID == "" {
			return "", fmt.Errorf("process kill: sessionId required")
		}
		v, ok := bgStore.Load(p.SessionID)
		if !ok {
			return "", fmt.Errorf("process kill: session %q not found", p.SessionID)
		}
		sess := v.(*bgSession)
		if sess.cmd.Process != nil {
			if err := sess.cmd.Process.Kill(); err != nil {
				return "", fmt.Errorf("process kill: %v", err)
			}
		}
		bgStore.Delete(p.SessionID)
		return "killed", nil

	case "poll":
		if p.SessionID == "" {
			return "", fmt.Errorf("process poll: sessionId required")
		}
		v, ok := bgStore.Load(p.SessionID)
		if !ok {
			return "", fmt.Errorf("process poll: session %q not found", p.SessionID)
		}
		sess := v.(*bgSession)

		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 5000
		}

		select {
		case <-sess.done:
			sess.mu.Lock()
			out := sess.stdout.String()
			exitCode := sess.exitCode
			sess.mu.Unlock()
			return fmt.Sprintf("status: exited(%d)\noutput:\n%s", exitCode, out), nil
		case <-time.After(time.Duration(timeout) * time.Millisecond):
			sess.mu.Lock()
			out := sess.stdout.String()
			sess.mu.Unlock()
			return fmt.Sprintf("status: running\noutput so far:\n%s", out), nil
		}

	default:
		return "", fmt.Errorf("process: unknown action %q", p.Action)
	}
}
