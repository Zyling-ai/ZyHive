//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived process-group termination", pid)
}

func TestBackgroundTimeoutKillsChildProcessGroup(t *testing.T) {
	resetBackgroundProcesses(t)
	pidFile := t.TempDir() + "/child.pid"
	owner := processOwner{AgentID: "agent-a", SessionID: "session-1"}
	id := startTestProcess(
		t,
		owner,
		fmt.Sprintf("sleep 30 & echo $! > %q; wait", pidFile),
		250*time.Millisecond,
	)
	session, ok := backgroundProcesses.get(owner, id)
	if !ok {
		t.Fatal("process missing")
	}
	select {
	case <-session.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out process did not exit")
	}
	status, _, _ := session.state()
	if status != "timeout" {
		t.Fatalf("status=%s want timeout", status)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessExit(t, pid)
}

func TestForegroundTimeoutKillsChildProcessGroup(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	registry := New(t.TempDir(), t.TempDir(), "agent-a")
	input, err := json.Marshal(map[string]any{
		"command": fmt.Sprintf("sleep 30 & echo $! > %q; wait", pidFile),
		"timeout": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Execute(context.Background(), "exec", input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "timed out") {
		t.Fatalf("unexpected timeout result: %s", result)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessExit(t, pid)
}
