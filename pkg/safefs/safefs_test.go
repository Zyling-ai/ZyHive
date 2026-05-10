package safefs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfineToBase_Basic(t *testing.T) {
	base := t.TempDir()
	got, err := ConfineToBase(base, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "foo")
	// EvalSymlinks may have normalized base (e.g. /tmp -> /private/tmp on macOS).
	wantResolved := mustEvalBase(t, base) + string(os.PathSeparator) + "foo"
	if got != want && got != wantResolved {
		t.Fatalf("got %q want %q or %q", got, want, wantResolved)
	}
}

func TestConfineToBase_Subpath(t *testing.T) {
	base := t.TempDir()
	got, err := ConfineToBase(base, "sub/dir/foo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("sub", "dir", "foo.txt")) {
		t.Fatalf("subpath not preserved: %s", got)
	}
}

func TestConfineToBase_RejectsRelativeEscape(t *testing.T) {
	base := t.TempDir()
	if _, err := ConfineToBase(base, "../escape"); !errors.Is(err, ErrEscape) {
		t.Fatalf("expected ErrEscape, got %v", err)
	}
	if _, err := ConfineToBase(base, "foo/../../escape"); !errors.Is(err, ErrEscape) {
		t.Fatalf("expected ErrEscape via interior .., got %v", err)
	}
}

// THIS is the regression test for B001.
// Sibling-prefix confusion: base=/tmp/x/alice, rel="../alice-evil/x" should
// NOT pass even though the simple HasPrefix check would say it does.
func TestConfineToBase_RejectsSiblingPrefixConfusion(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "alice")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "alice-evil"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfineToBase(base, "../alice-evil/secret"); !errors.Is(err, ErrEscape) {
		t.Fatalf("sibling-prefix bypass NOT blocked. got err=%v (this is B001)", err)
	}
}

func TestConfineToBase_RejectsAbsolutePath(t *testing.T) {
	base := t.TempDir()
	if _, err := ConfineToBase(base, "/etc/passwd"); !errors.Is(err, ErrAbsoluteRel) {
		t.Fatalf("expected ErrAbsoluteRel, got %v", err)
	}
}

func TestConfineToBase_RejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on Windows")
	}
	root := t.TempDir()
	base := filepath.Join(root, "ws")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside-ws")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a symlink inside ws pointing OUTSIDE ws.
	if err := os.Symlink(target, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	// Reading "link/file" must be blocked (symlink walks out of base).
	if _, err := ConfineToBase(base, "link/file"); !errors.Is(err, ErrEscape) {
		t.Fatalf("symlink escape NOT blocked. got err=%v", err)
	}
	// But "link" itself stat'd is also outside; ensure reject.
	if _, err := ConfineToBase(base, "link"); !errors.Is(err, ErrEscape) {
		t.Fatalf("symlink target not rejected for own resolve. got err=%v", err)
	}
}

func TestConfineToBase_AllowsExactBase(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"", ".", "./"} {
		got, err := ConfineToBase(base, rel)
		if err != nil {
			t.Fatalf("rel=%q: unexpected error: %v", rel, err)
		}
		expected := mustEvalBase(t, base)
		if got != expected {
			t.Fatalf("rel=%q: got %q want %q", rel, got, expected)
		}
	}
}

func TestConfineToBase_TolerantOfTrailingSlash(t *testing.T) {
	base := t.TempDir() + "/"
	if _, err := ConfineToBase(base, "foo"); err != nil {
		t.Fatalf("trailing slash rejected: %v", err)
	}
}

func TestConfineToBase_RejectsNullByte(t *testing.T) {
	base := t.TempDir()
	if _, err := ConfineToBase(base, "foo\x00bar"); !errors.Is(err, ErrNullByte) {
		t.Fatalf("expected ErrNullByte, got %v", err)
	}
}

func TestConfineToBase_RejectsEmptyBase(t *testing.T) {
	if _, err := ConfineToBase("", "foo"); err == nil {
		t.Fatal("expected error for empty base")
	}
}

func TestConfineToBase_NonExistentBaseStillResolves(t *testing.T) {
	// Base might not exist yet (e.g. agent workspace not created). Should
	// fall back to filepath.Abs without crash.
	got, err := ConfineToBase("/tmp/this-does-not-exist-for-sure-zzz/x", "leaf")
	if err != nil {
		t.Fatalf("non-existent base should still resolve: %v", err)
	}
	if !strings.HasSuffix(got, "leaf") {
		t.Fatalf("unexpected: %s", got)
	}
}

// Composite test: symlink INSIDE base → another file inside base is fine.
func TestConfineToBase_AllowsSymlinkStayingInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on Windows")
	}
	base := t.TempDir()
	target := filepath.Join(base, "real")
	if err := os.WriteFile(target, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := ConfineToBase(base, "link")
	if err != nil {
		t.Fatalf("inner symlink rejected: %v", err)
	}
	// Should resolve to the real file path, still inside base.
	resolvedBase := mustEvalBase(t, base)
	if !strings.HasPrefix(got, resolvedBase) {
		t.Fatalf("got %q outside base %q", got, resolvedBase)
	}
}

func mustEvalBase(t *testing.T, base string) string {
	t.Helper()
	abs, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return strings.TrimRight(filepath.Clean(abs), string(os.PathSeparator))
	}
	out := strings.TrimRight(filepath.Clean(resolved), string(os.PathSeparator))
	if out == "" {
		out = string(os.PathSeparator)
	}
	return out
}
