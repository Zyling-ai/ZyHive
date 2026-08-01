package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerRejectsUnsafeProjectIDs(t *testing.T) {
	manager := NewManager(t.TempDir())
	for _, id := range []string{"", ".", "..", "../outside", "a/b", `a\b`} {
		if _, err := manager.Create(CreateOpts{ID: id, Name: "unsafe"}); err == nil {
			t.Errorf("Create(%q) should fail", id)
		}
	}
}

func TestManagerCreatesPrivateProjectTree(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	project, err := manager.Create(CreateOpts{ID: "研发项目", Name: "研发项目"})
	if err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(project.FilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Errorf("project dir mode = %o, want 700", got)
	}
	for _, name := range []string{"meta.json", "README.md"} {
		info, statErr := os.Stat(filepath.Join(project.FilesDir, name))
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("%s mode = %o, want 600", name, got)
		}
	}
}

func TestManagerRemoveRejectsTamperedFilesDir(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	project, err := manager.Create(CreateOpts{ID: "safe", Name: "safe"})
	if err != nil {
		t.Fatal(err)
	}
	project.FilesDir = t.TempDir()
	if err := manager.Remove("safe"); err == nil {
		t.Fatal("Remove should reject a files directory outside the managed project directory")
	}
	if _, err := os.Stat(filepath.Join(root, "safe")); err != nil {
		t.Fatalf("managed project directory was removed: %v", err)
	}
}
