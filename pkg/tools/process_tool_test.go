package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/config"
)

func resetBackgroundProcesses(t *testing.T) {
	t.Helper()
	backgroundProcesses.reset()
	t.Cleanup(backgroundProcesses.reset)
}

func testProcessRegistry(agentID, sessionID string) *Registry {
	return &Registry{agentID: agentID, sessionID: sessionID}
}

func processAction(t *testing.T, registry *Registry, action, id, data string) (string, error) {
	t.Helper()
	input, err := json.Marshal(map[string]any{
		"action":    action,
		"sessionId": id,
		"data":      data,
		"timeout":   3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry.handleProcess(context.Background(), input)
}

func startTestProcess(t *testing.T, owner processOwner, script string, timeout time.Duration) string {
	t.Helper()
	id, err := backgroundProcesses.start(owner, processSpec{
		Name:    "sh",
		Args:    []string{"-c", script},
		Timeout: timeout,
		Kind:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBackgroundProcessOwnerIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	resetBackgroundProcesses(t)
	owner := processOwner{AgentID: "agent-a", SessionID: "session-1"}
	id := startTestProcess(t, owner, "sleep 30", time.Minute)
	allowed := testProcessRegistry(owner.AgentID, owner.SessionID)

	for name, denied := range map[string]*Registry{
		"other agent":   testProcessRegistry("agent-b", owner.SessionID),
		"other session": testProcessRegistry(owner.AgentID, "session-2"),
	} {
		t.Run(name, func(t *testing.T) {
			list, err := processAction(t, denied, "list", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(list, id) {
				t.Fatal("foreign process leaked through list")
			}
			if _, err := processAction(t, denied, "log", id, ""); err == nil {
				t.Fatal("foreign owner read process output")
			}
			if _, err := processAction(t, denied, "kill", id, ""); err == nil {
				t.Fatal("foreign owner killed process")
			}
		})
	}
	list, err := processAction(t, allowed, "list", "", "")
	if err != nil || !strings.Contains(list, id) {
		t.Fatalf("owner could not list process: %q err=%v", list, err)
	}
	if _, err := processAction(t, allowed, "kill", id, ""); err != nil {
		t.Fatal(err)
	}
	session, _ := backgroundProcesses.get(owner, id)
	select {
	case <-session.done:
	case <-time.After(3 * time.Second):
		t.Fatal("killed process did not exit")
	}
	status, _, _ := session.state()
	if status != "killed" {
		t.Fatalf("status=%s want killed", status)
	}
}

func TestBackgroundProcessStdinAndPoll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	resetBackgroundProcesses(t)
	owner := processOwner{AgentID: "agent-a", SessionID: "session-1"}
	id := startTestProcess(t, owner, `read line; printf 'got:%s' "$line"`, time.Minute)
	registry := testProcessRegistry(owner.AgentID, owner.SessionID)
	if _, err := processAction(t, registry, "write", id, "hello\n"); err != nil {
		t.Fatal(err)
	}
	output, err := processAction(t, registry, "poll", id, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "status: done(0)") || !strings.Contains(output, "got:hello") {
		t.Fatalf("unexpected poll output: %s", output)
	}
}

func TestBackgroundProcessLimitsArePerOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	resetBackgroundProcesses(t)
	first := processOwner{AgentID: "agent-a", SessionID: "session-1"}
	for i := 0; i < maxBackgroundProcessesPerOwner; i++ {
		startTestProcess(t, first, "sleep 30", time.Minute)
	}
	if _, err := backgroundProcesses.start(first, processSpec{
		Name: "sh", Args: []string{"-c", "sleep 30"}, Timeout: time.Minute,
	}); err == nil || !strings.Contains(err.Error(), "limit reached for this agent/session") {
		t.Fatalf("expected owner limit, got %v", err)
	}
	second := processOwner{AgentID: "agent-a", SessionID: "session-2"}
	startTestProcess(t, second, "sleep 30", time.Minute)
}

func TestBackgroundProcessGlobalLimit(t *testing.T) {
	manager := newProcessManager()
	for i := 0; i < maxBackgroundProcessesGlobal; i++ {
		manager.sessions[fmt.Sprintf("existing-%d", i)] = &bgSession{
			owner: processOwner{AgentID: fmt.Sprintf("agent-%d", i), SessionID: "session"},
			done:  make(chan struct{}),
		}
	}
	_, err := manager.start(
		processOwner{AgentID: "overflow", SessionID: "session"},
		processSpec{Name: "unused"},
	)
	if err == nil || !strings.Contains(err.Error(), "global background process limit") {
		t.Fatalf("expected global limit, got %v", err)
	}
}

func TestCompletedBackgroundSessionsArePrunedPerOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	manager := newProcessManager()
	owner := processOwner{AgentID: "agent-a", SessionID: "session-1"}
	for i := 0; i < maxBackgroundSessionsPerOwner; i++ {
		done := make(chan struct{})
		close(done)
		manager.sessions[fmt.Sprintf("completed-%d", i)] = &bgSession{
			id:        fmt.Sprintf("completed-%d", i),
			owner:     owner,
			done:      done,
			startedAt: time.Unix(int64(i), 0),
		}
	}
	id, err := manager.start(owner, processSpec{
		Name: "sh", Args: []string{"-c", "true"}, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.reset)
	if len(manager.list(owner)) > maxBackgroundSessionsPerOwner {
		t.Fatalf("retained owner sessions exceeded %d", maxBackgroundSessionsPerOwner)
	}
	if _, ok := manager.get(owner, "completed-0"); ok {
		t.Fatal("oldest completed session was not pruned")
	}
	if _, ok := manager.get(owner, id); !ok {
		t.Fatal("new process missing after prune")
	}
}

func TestCappedBufferBoundsOutput(t *testing.T) {
	buffer := &cappedBuffer{}
	payload := strings.Repeat("x", maxBackgroundOutputBytes+1024)
	n, err := buffer.Write([]byte(payload))
	if err != nil || n != len(payload) {
		t.Fatalf("write n=%d err=%v", n, err)
	}
	output := buffer.String()
	if len(output) > maxBackgroundOutputBytes+64 {
		t.Fatalf("bounded output grew to %d bytes", len(output))
	}
	if !strings.Contains(output, "output truncated") {
		t.Fatal("truncation marker missing")
	}
}

func TestACPUsesOwnedProcessStoreAndConfinedWorkDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	resetBackgroundProcesses(t)
	t.Setenv("ZYHIVE_TEST_TOKEN", "must-not-leak")
	workspace := t.TempDir()
	registry := New(workspace, t.TempDir(), "agent-a")
	registry.WithSessionID("session-1")
	registry.WithACPAgents(func() []config.ACPAgentEntry {
		return []config.ACPAgentEntry{{
			ID:      "shell",
			Name:    "Shell ACP",
			Binary:  "sh",
			Args:    []string{"-c", `printf '%s:%s:%s' "$ACP_ALLOWED" "$ZYHIVE_TEST_TOKEN" "{{task}}"`},
			WorkDir: ".",
			Env:     []string{"ACP_ALLOWED=yes"},
		}}
	})
	result, err := registry.handleACPSpawn(context.Background(), json.RawMessage(`{
		"acpId":"shell",
		"task":"hello",
		"timeout":10
	}`))
	if err != nil {
		t.Fatal(err)
	}
	sessions := backgroundProcesses.list(registry.processOwner())
	if len(sessions) != 1 || sessions[0].kind != "acp:shell" {
		t.Fatalf("ACP process not registered with owner: %+v", sessions)
	}
	if !strings.Contains(result, sessions[0].id) {
		t.Fatalf("result missing process id: %s", result)
	}
	select {
	case <-sessions[0].done:
	case <-time.After(3 * time.Second):
		t.Fatal("ACP process did not finish")
	}
	if got := sessions[0].output(); got != "yes::hello" {
		t.Fatalf("ACP output=%q", got)
	}

	_, err = registry.handleACPSpawn(context.Background(), json.RawMessage(`{
		"acpId":"shell",
		"task":"blocked",
		"workDir":"../outside"
	}`))
	if err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatalf("outside ACP workDir was not blocked: %v", err)
	}
}
