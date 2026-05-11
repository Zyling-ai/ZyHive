package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Very long single-line entry (>1 MB) — bufio Scanner has 64 KB default
// buffer; we already set 1 MiB. Anything bigger should not crash.
func Test_AITeam_S8_Edge_VeryLongEntry(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	// 200 KB detail blob
	big := strings.Repeat("x", 200_000)
	if err := log.Append(Entry{Type: "big", Detail: map[string]any{"payload": big}}); err != nil {
		t.Fatal(err)
	}
	tail, err := log.Tail(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tail))
	}
	got, _ := tail[0].Detail["payload"].(string)
	if len(got) != 200_000 {
		t.Fatalf("payload truncated: got %d chars", len(got))
	}
}

// File with NUL bytes anywhere in the middle.
func Test_AITeam_S8_Edge_NULBytesInFile(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	_ = log.Append(Entry{Type: "good1"})
	f, _ := os.OpenFile(log.Path(), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.Write([]byte{0, 0, 0, '\n'})
	f.Close()
	_ = log.Append(Entry{Type: "good2"})
	// Tail should skip the bad line.
	tail, err := log.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	good := 0
	for _, e := range tail {
		if strings.HasPrefix(e.Type, "good") {
			good++
		}
	}
	if good != 2 {
		t.Fatalf("expected 2 good entries, got %d", good)
	}
}

// Tail with n=0 → no panic, empty result.
func Test_AITeam_S8_Edge_TailZero(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	_ = log.Append(Entry{Type: "x"})
	tail, err := log.Tail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("tail(0) should be empty, got %d", len(tail))
	}
}

// Tail with negative n → no panic, empty result.
func Test_AITeam_S8_Edge_TailNegative(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	_ = log.Append(Entry{Type: "x"})
	tail, err := log.Tail(-5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("tail(-5) should be empty, got %d", len(tail))
	}
}

// File with no final newline (write was interrupted at end).
func Test_AITeam_S8_Edge_NoFinalNewline(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	f, _ := os.OpenFile(filepath.Join(dir, "audit.log"), os.O_CREATE|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"type":"a"}` + "\n" + `{"type":"b"}` /* no trailing newline */)
	f.Close()
	tail, err := log.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 2 {
		t.Fatalf("file without trailing newline: want 2 entries, got %d", len(tail))
	}
}

// Empty lines in file.
func Test_AITeam_S8_Edge_EmptyLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	log, _ := New(dir)
	f, _ := os.OpenFile(filepath.Join(dir, "audit.log"), os.O_CREATE|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("\n\n" + `{"type":"a"}` + "\n\n\n" + `{"type":"b"}` + "\n\n")
	f.Close()
	tail, err := log.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 2 {
		t.Fatalf("expected 2 entries skipping empty lines, got %d", len(tail))
	}
}
