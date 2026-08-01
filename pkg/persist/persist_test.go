package persist

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileIsAtomicAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "config.json")
	payloads := [][]byte{
		bytes.Repeat([]byte("a"), 32*1024),
		bytes.Repeat([]byte("b"), 32*1024),
		bytes.Repeat([]byte("c"), 32*1024),
	}
	var wait sync.WaitGroup
	for _, payload := range payloads {
		payload := payload
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := WriteFile(path, payload, 0600); err != nil {
				t.Errorf("WriteFile: %v", err)
			}
		}()
	}
	wait.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, payload := range payloads {
		if bytes.Equal(got, payload) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatal("final file contains a partial or interleaved write")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("mode = %o, want 600", mode)
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := AtomicWrite(path, []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "state.json" {
			t.Fatalf("unexpected file left behind: %s", entry.Name())
		}
	}
}
