// Integration tests for Phase 1 — config, agent manager, system prompt.
// These tests do NOT make real API calls; they verify local logic only.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/runner"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
)

// TestConfigLoad verifies that config loading and defaults work correctly.
func TestConfigLoad(t *testing.T) {
	// Test default config
	cfg := config.Default()
	if cfg.Gateway.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Gateway.Port)
	}
	if cfg.Gateway.Bind != "lan" {
		t.Errorf("expected default bind 'lan', got %q", cfg.Gateway.Bind)
	}
	if cfg.Agents.Dir != "./agents" {
		t.Errorf("expected default agents dir './agents', got %q", cfg.Agents.Dir)
	}
	if cfg.Models.Primary != "anthropic/claude-sonnet-4-6" {
		t.Errorf("expected default model, got %q", cfg.Models.Primary)
	}

	// Test save and load round-trip
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test-config.json")
	cfg.Auth.Token = "test-token-123"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Auth.Token != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got %q", loaded.Auth.Token)
	}
}

// TestAgentManager verifies that agent creation produces the correct directory structure.
func TestAgentManager(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := agent.NewManager(tmpDir)

	// LoadAll on empty dir should succeed
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll on empty dir: %v", err)
	}
	if len(mgr.List()) != 0 {
		t.Errorf("expected 0 agents, got %d", len(mgr.List()))
	}

	// Create an agent
	a, err := mgr.Create("test-agent", "Test Agent", "anthropic/claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID != "test-agent" {
		t.Errorf("expected ID 'test-agent', got %q", a.ID)
	}
	if a.Status != "idle" {
		t.Errorf("expected status 'idle', got %q", a.Status)
	}

	// Verify directory structure
	for _, path := range []string{
		filepath.Join(tmpDir, "test-agent", "config.json"),
		filepath.Join(tmpDir, "test-agent", "workspace", "IDENTITY.md"),
		filepath.Join(tmpDir, "test-agent", "workspace", "SOUL.md"),
		filepath.Join(tmpDir, "test-agent", "workspace", "MEMORY.md"),
		filepath.Join(tmpDir, "test-agent", "sessions"),
	} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected path to exist: %s", path)
		}
	}

	// Get should find the agent
	got, ok := mgr.Get("test-agent")
	if !ok {
		t.Fatal("Get returned false for existing agent")
	}
	if got.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %q", got.Name)
	}

	// Duplicate creation should fail
	_, err = mgr.Create("test-agent", "Dupe", "model")
	if err == nil {
		t.Error("expected error on duplicate agent creation")
	}

	// LoadAll should re-discover the agent
	mgr2 := agent.NewManager(tmpDir)
	if err := mgr2.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(mgr2.List()) != 1 {
		t.Errorf("expected 1 agent after LoadAll, got %d", len(mgr2.List()))
	}
}

// TestSystemPromptBuilder verifies that the system prompt includes identity files.
func TestSystemPromptBuilder(t *testing.T) {
	tmpDir := t.TempDir()

	// Write sample identity files
	os.WriteFile(filepath.Join(tmpDir, "IDENTITY.md"), []byte("# I am TestBot\nRole: tester"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("Be precise and thorough."), 0644)
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte("Remember: tests are important."), 0644)

	prompt, err := runner.BuildSystemPrompt(tmpDir)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}

	// Should contain date/time
	if !strings.Contains(prompt, "Current date and time:") {
		t.Error("prompt missing date/time header")
	}

	// Should contain identity content
	if !strings.Contains(prompt, "I am TestBot") {
		t.Error("prompt missing IDENTITY.md content")
	}
	if !strings.Contains(prompt, "Be precise") {
		t.Error("prompt missing SOUL.md content")
	}
	if !strings.Contains(prompt, "tests are important") {
		t.Error("prompt missing MEMORY.md content")
	}
}

// TestSystemPromptMissingFiles verifies graceful handling of missing files.
func TestSystemPromptMissingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	// No files — should still return a prompt with just the date
	prompt, err := runner.BuildSystemPrompt(tmpDir)
	if err != nil {
		t.Fatalf("BuildSystemPrompt with no files: %v", err)
	}
	if !strings.Contains(prompt, "Current date and time:") {
		t.Error("prompt missing date/time even with no identity files")
	}
}

// TestSessionStore verifies basic session JSONL operations.
func TestSessionStore(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	// Create a session
	path, err := store.Create("sess-001", "agent-1")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty session path")
	}

	// Append an entry
	entry := map[string]any{"type": "message", "text": "hello"}
	if err := store.Append("sess-001", entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Read all entries (header + 1 appended)
	entries, err := store.ReadAll("sess-001")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// List sessions from index
	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session in index, got %d", len(sessions))
	}
}
