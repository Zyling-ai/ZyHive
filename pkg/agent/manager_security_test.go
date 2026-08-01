package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerRejectsUnsafeAgentIDs(t *testing.T) {
	manager := NewManager(t.TempDir())
	for _, id := range []string{"", ".", "..", "../outside", "a/b", `a\b`} {
		if _, err := manager.Create(id, "unsafe", "model"); err == nil {
			t.Errorf("Create(%q) should fail", id)
		}
	}
}

func TestManagerCreatesPrivateAgentTree(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	agent, err := manager.Create("成员一", "成员一", "model")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "成员一"),
		agent.WorkspaceDir,
		agent.SessionDir,
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Errorf("%s mode = %o, want 700", path, got)
		}
	}
	configInfo, err := os.Stat(filepath.Join(root, "成员一", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := configInfo.Mode().Perm(); got != 0600 {
		t.Errorf("config mode = %o, want 600", got)
	}
}

func TestManagerRemoveRejectsTamperedWorkspacePath(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	agent, err := manager.Create("safe", "safe", "model")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	agent.WorkspaceDir = filepath.Join(outside, "workspace")
	if err := manager.Remove("safe"); err == nil {
		t.Fatal("Remove should reject a workspace path outside the managed agent directory")
	}
	if _, err := os.Stat(filepath.Join(root, "safe")); err != nil {
		t.Fatalf("managed agent directory was removed: %v", err)
	}
}
