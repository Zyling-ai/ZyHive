package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCommandArgsConfigCompatibility(t *testing.T) {
	args, cfg, ok, err := backupCommandArgs([]string{"--config", "/tmp/current.json", "backup", "create", "--output", "x.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg != "/tmp/current.json" || len(args) != 3 || args[0] != "create" {
		t.Fatalf("unexpected parse result: args=%v config=%q ok=%v", args, cfg, ok)
	}
	args, cfg, ok, err = backupCommandArgs([]string{"backup", "restore", "--input", "x.tar.gz", "--config=/tmp/other.json"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || cfg != "/tmp/other.json" || len(args) != 3 || args[0] != "restore" {
		t.Fatalf("unexpected parse result: args=%v config=%q ok=%v", args, cfg, ok)
	}
}

func TestBackupCLICreateAndRestoreNoService(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	agents := filepath.Join(root, "agents")
	configPath := filepath.Join(root, "current.json")
	archive := filepath.Join(root, "backup.tar.gz")
	for _, dir := range []string{agents, filepath.Join(work, "projects"), filepath.Join(work, "cron")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	configJSON := `{"configVersion":3,"gateway":{"port":8080,"bind":"localhost"},"agents":{"dir":"` +
		filepath.ToSlash(agents) + `"},"models":[],"channels":[],"tools":[],"skills":[],"auth":{"token":"test"}}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(work, "projects", "data.txt")
	if err := os.WriteFile(dataPath, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runBackupCLI([]string{"create", "--output", archive, "--workdir", work}, configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runBackupCLI([]string{"restore", "--input", archive, "--yes", "--no-service", "--workdir", work}, configPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before" {
		t.Fatalf("restored data = %q, want before", got)
	}
}
