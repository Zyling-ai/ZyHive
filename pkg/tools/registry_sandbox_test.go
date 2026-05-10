package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// helper: invoke the registry's bash handler via the same code path the
// runner uses (registry.Execute → handler).
func runExec(t *testing.T, r *Registry, command string) (string, error) {
	t.Helper()
	input, _ := json.Marshal(map[string]any{"command": command, "timeout": 5})
	return r.Execute(context.Background(), "exec", input)
}

func Test_AITeam_Registry_SandboxFlagOff_LegacyPath(t *testing.T) {
	// Make sure no other test has left the env set.
	t.Setenv(flags.EnvSandbox, "")
	if flags.SandboxEnabled() {
		t.Fatal("setup: sandbox flag should be off")
	}
	r := New("", "", "test")
	out, err := runExec(t, r, "echo legacy-path-marker")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "legacy-path-marker") {
		t.Fatalf("unexpected output: %q", out)
	}
	// Legacy path injects "cd <workspaceDir> && ..." prefix. Since this
	// test uses an empty workspaceDir, the prefix is absent. That alone
	// proves we ran the legacy fork (sandbox path uses WorkDir field
	// instead of inline cd).
}

func Test_AITeam_Registry_SandboxFlagOn_RoutesThroughSandbox(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("sandbox enforced only on linux/darwin")
	}
	t.Setenv(flags.EnvSandbox, "1")
	if !flags.SandboxEnabled() {
		t.Fatal("setup: sandbox flag should be on")
	}
	r := New("", "", "test")
	// Verify the sandbox is being used by checking $AITEAM_SANDBOX —
	// only the sandbox path sets this env var.
	out, err := runExec(t, r, `echo "marker=$AITEAM_SANDBOX"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "marker=1") {
		t.Fatalf("expected AITEAM_SANDBOX=1 in env, got: %q", out)
	}
}

func Test_AITeam_Registry_SandboxFlagOn_TmpHomeIsolated(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("sandbox enforced only on linux/darwin")
	}
	t.Setenv(flags.EnvSandbox, "1")
	r := New("", "", "test")
	out, err := runExec(t, r, `echo "$HOME"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "aiteam-exec-") {
		t.Fatalf("expected sandboxed HOME (aiteam-exec-*), got %q", out)
	}
}

func Test_AITeam_Registry_SandboxFlagOn_TimeoutFormatted(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("sandbox enforced only on linux/darwin")
	}
	t.Setenv(flags.EnvSandbox, "1")
	r := New("", "", "test")
	// timeout=1 → wall clock 1 second. sleep 30 should be killed.
	input, _ := json.Marshal(map[string]any{"command": "sleep 30", "timeout": 1})
	out, err := r.Execute(context.Background(), "exec", input)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "timed out") {
		t.Fatalf("expected 'timed out' in output, got %q", out)
	}
}
