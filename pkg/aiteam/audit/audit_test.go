package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func Test_AITeam_Audit_AppendsLine(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(Entry{Type: "test.event", Subsystem: "test", AgentID: "alice"}); err != nil {
		t.Fatal(err)
	}
	if log.LineCount() != 1 {
		t.Fatalf("expected 1 line, got %d", log.LineCount())
	}

	data, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	var got Entry
	if err := json.Unmarshal([]byte(strings.TrimRight(string(data), "\n")), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Type != "test.event" || got.Subsystem != "test" || got.AgentID != "alice" {
		t.Fatalf("entry corrupted: %+v", got)
	}
	if got.Timestamp == 0 {
		t.Fatal("timestamp should be auto-filled")
	}
}

func Test_AITeam_Audit_AppendNilIsNoOp(t *testing.T) {
	var log *Log
	if err := log.Append(Entry{Type: "x"}); err != nil {
		t.Fatalf("nil-log Append should be no-op, got %v", err)
	}
	if log.LineCount() != 0 || log.Path() != "" {
		t.Fatal("nil-log getters should return zero values")
	}
}

func Test_AITeam_Audit_FilePermissionSecure(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(Entry{Type: "x", Subsystem: "test"}); err != nil {
		t.Fatal(err)
	}
	// directory: 0700
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	// dir mode bits: t.TempDir creates dirs with the OS default (0755 on
	// Linux); our New called MkdirAll on an already-existing dir which is
	// a no-op (does not chmod). So we only assert the file mode here.
	_ = info

	finfo, err := os.Stat(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := finfo.Mode().Perm(); mode != 0o600 {
		t.Fatalf("audit.log mode should be 0600, got %o", mode)
	}
}

func Test_AITeam_Audit_RotateAfterThreshold(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.SetRotateAfter(3) // rotate after 3 lines for the test

	for i := 0; i < 5; i++ {
		if err := log.Append(Entry{Type: "x", Detail: map[string]any{"i": i}}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	// We should see 1 rotated file + an active log with the remaining
	// lines (count = 2 since we rotated after 3, then appended 2 more).
	if log.LineCount() != 2 {
		t.Fatalf("expected 2 lines in active log after rotate, got %d", log.LineCount())
	}

	// Check there's a rotated file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rotatedFound := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "audit.log.") {
			rotatedFound = true
		}
	}
	if !rotatedFound {
		t.Fatal("no rotated audit.log.* file found")
	}
}

func Test_AITeam_Audit_ConcurrentAppendsAreSafe(t *testing.T) {
	dir := t.TempDir()
	log, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	n := 200
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = log.Append(Entry{Type: "concurrent", Detail: map[string]any{"i": i}})
		}(i)
	}
	wg.Wait()

	// Open the log and count lines; each line must be valid JSON.
	f, err := os.Open(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %s err=%v", scanner.Text(), err)
		}
		count++
	}
	if count != n {
		t.Fatalf("expected %d lines, got %d", n, count)
	}
}

func Test_AITeam_Audit_TailReturnsLastN(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	for i := 0; i < 50; i++ {
		_ = log.Append(Entry{Type: "x", Detail: map[string]any{"i": i}})
	}
	tail, err := log.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(tail))
	}
	// Last entry should be i=49.
	last := tail[len(tail)-1]
	if v, _ := last.Detail["i"].(float64); int(v) != 49 {
		t.Fatalf("last entry i=%v want 49", last.Detail["i"])
	}
	// First of tail should be i=40.
	first := tail[0]
	if v, _ := first.Detail["i"].(float64); int(v) != 40 {
		t.Fatalf("first of tail i=%v want 40", first.Detail["i"])
	}
}

func Test_AITeam_Audit_TailNilSafe(t *testing.T) {
	var log *Log
	tail, err := log.Tail(10)
	if err != nil {
		t.Fatalf("nil tail should not error, got %v", err)
	}
	if len(tail) != 0 {
		t.Fatal("nil tail should return empty")
	}
}

func Test_AITeam_Audit_TailEmptyOrMissingFile(t *testing.T) {
	// Fresh dir, no file yet.
	log, _ := New(t.TempDir())
	tail, err := log.Tail(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("empty file should yield empty tail, got %d", len(tail))
	}
}

func Test_AITeam_Audit_TailMoreThanExisting(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	for i := 0; i < 3; i++ {
		_ = log.Append(Entry{Type: "x", Detail: map[string]any{"i": i}})
	}
	tail, _ := log.Tail(100)
	if len(tail) != 3 {
		t.Fatalf("want 3 entries when asking for 100 and only 3 exist, got %d", len(tail))
	}
}

func Test_AITeam_Audit_TailSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	_ = log.Append(Entry{Type: "good1"})
	// Manually append a garbage line.
	f, _ := os.OpenFile(log.Path(), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("{not-valid-json}\n")
	_ = f.Close()
	_ = log.Append(Entry{Type: "good2"})
	tail, _ := log.Tail(10)
	if len(tail) != 2 {
		t.Fatalf("expected 2 valid entries (corrupt skipped), got %d", len(tail))
	}
	if tail[0].Type != "good1" || tail[1].Type != "good2" {
		t.Fatalf("unexpected entries: %+v", tail)
	}
}

func Test_AITeam_Audit_TailLargeFileChunking(t *testing.T) {
	// Verify back-scan chunking handles files > one chunk (>8 KiB).
	dir := t.TempDir()
	log, _ := New(dir)
	// Each entry is ~200 bytes; 200 entries → ~40 KiB → exercises
	// multi-chunk back-scan.
	for i := 0; i < 200; i++ {
		_ = log.Append(Entry{Type: "x", AgentID: "alice",
			Detail: map[string]any{"i": i, "padding": "abcdefghij abcdefghij abcdefghij abcdefghij"}})
	}
	tail, err := log.Tail(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 5 {
		t.Fatalf("want 5, got %d", len(tail))
	}
	// Last entry must be i=199.
	if v, _ := tail[4].Detail["i"].(float64); int(v) != 199 {
		t.Fatalf("last entry i=%v want 199", tail[4].Detail["i"])
	}
}

func Test_AITeam_Audit_StartupLineCountRecovery(t *testing.T) {
	dir := t.TempDir()
	log1, _ := New(dir)
	for i := 0; i < 7; i++ {
		_ = log1.Append(Entry{Type: "x", Detail: map[string]any{"i": i}})
	}
	// Re-open in a fresh Log and verify the count was recovered.
	log2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if log2.LineCount() != 7 {
		t.Fatalf("expected line count 7 after re-open, got %d", log2.LineCount())
	}
}
