// 26.5.10v2 — B001 path traversal regression at the tools layer.
//
// AI-callable tools (read/write/edit/grep/glob) historically resolved any
// AI-supplied path against r.workspaceDir but accepted absolute paths
// unchanged → AI / prompt injection could read /etc/passwd, write to
// /etc/cron.d, etc. After the safefs migration, all such paths must be
// either workspace-relative OR an absolute path that resolves under workspace.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRegistryAt(t *testing.T) (*Registry, string) {
	t.Helper()
	tmp := t.TempDir()
	r := New(tmp, filepath.Dir(tmp), "test-agent")
	return r, tmp
}

func TestB001Tools_ReadAbsoluteOutsideWorkspaceRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleReadWS(context.Background(),
		mustJSON(map[string]any{"file_path": "/etc/passwd"}))
	if err == nil {
		t.Fatal("read('/etc/passwd') accepted (B001 regression)")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Errorf("expected 'outside workspace' in error, got: %v", err)
	}
}

func TestB001Tools_ReadRelativeEscapeRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleReadWS(context.Background(),
		mustJSON(map[string]any{"file_path": "../../etc/passwd"}))
	if err == nil {
		t.Fatal("read('../../etc/passwd') accepted (B001 regression)")
	}
}

func TestB001Tools_ReadSiblingPrefixBypassRejected(t *testing.T) {
	// Mirror the sibling-prefix attack at the tools level.
	root := t.TempDir()
	alice := filepath.Join(root, "alice", "workspace")
	evil := filepath.Join(root, "alice-evil", "workspace")
	for _, d := range []string{alice, evil} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(evil, "secret.md"), []byte("EVIL"), 0644); err != nil {
		t.Fatal(err)
	}
	r := New(alice, root, "alice")
	_, err := r.handleReadWS(context.Background(),
		mustJSON(map[string]any{"file_path": "../alice-evil/workspace/secret.md"}))
	if err == nil {
		t.Fatal("sibling-prefix bypass accepted (B001 regression — CRITICAL)")
	}
}

func TestB001Tools_WriteAbsoluteRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleWriteWS(context.Background(),
		mustJSON(map[string]any{
			"file_path": "/tmp/zyhive-poison-do-not-create.txt",
			"content":   "x",
		}))
	if err == nil {
		// Defensive cleanup in case the test fails:
		_ = os.Remove("/tmp/zyhive-poison-do-not-create.txt")
		t.Fatal("write('/tmp/...') accepted (B001 regression)")
	}
}

func TestB001Tools_LegitimateRelativePathOK(t *testing.T) {
	r, ws := newRegistryAt(t)
	if err := os.WriteFile(filepath.Join(ws, "ok.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := r.handleReadWS(context.Background(),
		mustJSON(map[string]any{"file_path": "ok.md"}))
	if err != nil {
		t.Fatalf("legit read failed: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected file content in output, got: %q", out)
	}
}

func TestB001Tools_AbsolutePathInsideWorkspaceOK(t *testing.T) {
	// AI can still pass an absolute path IF it resolves inside workspace.
	// (Useful when AI gets a path from another tool's output.)
	r, ws := newRegistryAt(t)
	abs := filepath.Join(ws, "ok2.md")
	if err := os.WriteFile(abs, []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := r.handleReadWS(context.Background(),
		mustJSON(map[string]any{"file_path": abs}))
	if err != nil {
		t.Fatalf("abs-path-inside-workspace rejected: %v", err)
	}
	if !strings.Contains(out, "inside") {
		t.Errorf("expected content, got: %q", out)
	}
}

func TestB001Tools_GrepAbsoluteOutsideRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleGrepWS(context.Background(),
		mustJSON(map[string]any{
			"path":    "/etc",
			"pattern": "root",
		}))
	if err == nil {
		t.Fatal("grep on /etc accepted (B001 regression)")
	}
}

func TestB001Tools_GlobAbsoluteOutsideRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleGlobWS(context.Background(),
		mustJSON(map[string]any{
			"base_dir": "/etc",
			"pattern":  "*.conf",
		}))
	if err == nil {
		t.Fatal("glob on /etc accepted (B001 regression)")
	}
}

func TestB001Tools_NullByteRejected(t *testing.T) {
	r, _ := newRegistryAt(t)
	_, err := r.handleReadWS(context.Background(),
		json.RawMessage(`{"file_path":"foo\u0000bar"}`))
	if err == nil {
		t.Fatal("NUL byte in path accepted (defense in depth)")
	}
}
