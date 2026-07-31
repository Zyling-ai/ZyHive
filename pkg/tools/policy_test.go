package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hasTool(registry *Registry, name string) bool {
	for _, definition := range registry.Definitions() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func TestPolicyLayersCannotBroadenGlobalPolicy(t *testing.T) {
	registry := New(t.TempDir(), t.TempDir(), "agent-1")
	global := ToolPolicy{Profile: "minimal"}
	agent := ToolPolicy{Profile: "full", Allow: []string{"exec", "write"}}
	registry.ApplyPolicyLayers(&global, &agent)
	if hasTool(registry, "exec") || hasTool(registry, "write") {
		t.Fatal("agent policy broadened the global minimal profile")
	}
}

func TestPolicyGroupsCoverDynamicToolNames(t *testing.T) {
	ui := expandNames([]string{"group:ui"})
	for _, name := range []string{"browser_navigate", "browser_eval", "browser_new_tab"} {
		if !ui[name] {
			t.Errorf("group:ui missing %s", name)
		}
	}
	agent := expandNames([]string{"group:agent"})
	if !agent["report_result"] || !agent["report_to_parent"] {
		t.Fatal("group:agent missing subagent reporting tools")
	}
	runtime := expandNames([]string{"group:runtime"})
	if !runtime["process"] || !runtime["acp_list"] || !runtime["acp_spawn"] {
		t.Fatal("group:runtime missing managed process tools")
	}
}

func TestInvalidPolicyFailsClosed(t *testing.T) {
	cases := map[string]string{
		"unknown profile": `{"profile":"not-real"}`,
		"unknown field":   `{"denny":["exec"]}`,
		"unknown group":   `{"deny":["group:typo"]}`,
		"invalid JSON":    `{"deny":`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			registry := New(t.TempDir(), t.TempDir(), "agent-1")
			err := registry.ConfigureGovernance(
				json.RawMessage(raw),
				nil,
				nil,
				time.Second,
			)
			if err == nil {
				t.Fatal("invalid policy should return an error")
			}
			if len(registry.Definitions()) != 0 {
				t.Fatal("invalid policy must remove every tool")
			}
		})
	}
}

func TestApprovalRequiredWithoutBrokerFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := New(workspace, t.TempDir(), "agent-1")
	registry.WithSessionID("session-1")
	if err := registry.ConfigureGovernance(
		json.RawMessage(`{"ask":["read"]}`),
		nil,
		nil,
		time.Second,
	); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Execute(
		context.Background(),
		"read",
		json.RawMessage(`{"file_path":"note.txt"}`),
	)
	if !errors.Is(err, ErrApprovalUnavailable) {
		t.Fatalf("expected approval service failure, got %v", err)
	}
}

func TestLateToolRegistrationCannotBypassPolicy(t *testing.T) {
	registry := New(t.TempDir(), t.TempDir(), "agent-1")
	registry.ApplyPolicy(ToolPolicy{Deny: []string{"self_set_env"}})
	registry.WithEnvUpdater(func(string, string, bool) error { return nil })
	if hasTool(registry, "self_set_env") {
		t.Fatal("late WithEnvUpdater registration bypassed policy")
	}
}

func TestGlobalAndAgentAskPatternsAreCombined(t *testing.T) {
	registry := New(t.TempDir(), t.TempDir(), "agent-1")
	if err := registry.ConfigureGovernance(
		json.RawMessage(`{"ask":["read"]}`),
		json.RawMessage(`{"ask":["write"]}`),
		nil,
		time.Second,
	); err != nil {
		t.Fatal(err)
	}
	if !registry.askNames["read"] || !registry.askNames["write"] {
		t.Fatalf("combined ask set missing entries: %#v", registry.askNames)
	}
}
