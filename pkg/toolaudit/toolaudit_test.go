package toolaudit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNilLogIsNoOp(t *testing.T) {
	var l *Log
	if err := l.Append(Entry{ToolCallID: "x"}); err != nil {
		t.Errorf("nil Append should be no-op, got: %v", err)
	}
	got, err := l.GetByID("x")
	if err != nil || got != nil {
		t.Errorf("nil GetByID should return (nil,nil), got (%v,%v)", got, err)
	}
	list, err := l.ListBySession("s", 10)
	if err != nil || list != nil {
		t.Errorf("nil ListBySession should return (nil,nil), got (%v,%v)", list, err)
	}
}

func TestNewEmptyDirReturnsNil(t *testing.T) {
	if New("") != nil {
		t.Fatalf("New(\"\") should return nil")
	}
}

func TestAppendAndGetByIDInline(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	in := json.RawMessage(`{"path":"/tmp/x"}`)
	e := Entry{
		AgentID: "a1", SessionID: "s1", ToolCallID: "toolu_1",
		Name: "read", Input: in, Result: "hello world", DurationMs: 12,
	}
	if err := l.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := l.GetByID("toolu_1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatalf("expected entry, got nil")
	}
	if got.Name != "read" || got.Result != "hello world" {
		t.Errorf("entry fields wrong: %+v", got)
	}
	if string(got.Input) != string(in) {
		t.Errorf("input round-trip failed: %s", got.Input)
	}
	if got.InputRef != "" || got.ResultRef != "" {
		t.Errorf("inline entry should not have refs: %+v", got)
	}
}

func TestAppendOverflowsToBlobs(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	bigResult := strings.Repeat("x", InlineCapBytes+1)
	bigInput := json.RawMessage(`{"data":"` + strings.Repeat("a", InlineCapBytes) + `"}`)
	e := Entry{
		AgentID: "a1", SessionID: "s1", ToolCallID: "toolu_big",
		Name: "exec", Input: bigInput, Result: bigResult,
	}
	if err := l.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Verify the JSONL row has refs, not inline bytes.
	now := time.Now().UTC().Format("2006-01-02")
	raw, _ := os.ReadFile(filepath.Join(dir, "tool-audit", now+".jsonl"))
	if !strings.Contains(string(raw), `"inputRef"`) {
		t.Errorf("expected inputRef in JSONL, got: %s", raw)
	}
	if !strings.Contains(string(raw), `"resultRef"`) {
		t.Errorf("expected resultRef in JSONL")
	}
	if strings.Contains(string(raw), `"input":{"data"`) {
		t.Errorf("expected input to NOT be inlined")
	}
	// GetByID should rehydrate from blob.
	got, err := l.GetByID("toolu_big")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatalf("nil entry")
	}
	if got.Result != bigResult {
		t.Errorf("blob result not rehydrated, got len=%d want=%d", len(got.Result), len(bigResult))
	}
	if string(got.Input) != string(bigInput) {
		t.Errorf("blob input not rehydrated")
	}
}

func TestListBySession(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	for i := 0; i < 5; i++ {
		_ = l.Append(Entry{
			AgentID: "a", SessionID: "s1", ToolCallID: "id_" + idx(i),
			Name: "read", Result: "ok",
		})
	}
	for i := 0; i < 3; i++ {
		_ = l.Append(Entry{
			AgentID: "a", SessionID: "s2", ToolCallID: "other_" + idx(i),
			Name: "write", Result: "ok",
		})
	}
	got, err := l.ListBySession("s1", 100)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("expected 5 entries for s1, got %d", len(got))
	}
	got2, _ := l.ListBySession("s2", 100)
	if len(got2) != 3 {
		t.Errorf("expected 3 entries for s2, got %d", len(got2))
	}
	// Newest first
	got3, _ := l.ListBySession("s1", 100)
	for i := 1; i < len(got3); i++ {
		if got3[i].Timestamp > got3[i-1].Timestamp {
			t.Errorf("ListBySession not sorted desc")
			break
		}
	}
}

func TestListAllFilters(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	_ = l.Append(Entry{ToolCallID: "a1", Name: "read", SessionID: "S1", Result: "r"})
	_ = l.Append(Entry{ToolCallID: "a2", Name: "write", SessionID: "S1", Result: "r"})
	_ = l.Append(Entry{ToolCallID: "a3", Name: "read", SessionID: "S2", Result: "r"})

	all, total, _ := l.ListAll(ListFilter{}, 50, 0)
	if total != 3 || len(all) != 3 {
		t.Errorf("expected 3 total, got total=%d len=%d", total, len(all))
	}
	bySess, _, _ := l.ListAll(ListFilter{SessionID: "S1"}, 50, 0)
	if len(bySess) != 2 {
		t.Errorf("expected 2 for S1, got %d", len(bySess))
	}
	byTool, _, _ := l.ListAll(ListFilter{ToolName: "read"}, 50, 0)
	if len(byTool) != 2 {
		t.Errorf("expected 2 read, got %d", len(byTool))
	}
	// pagination
	page1, _, _ := l.ListAll(ListFilter{}, 2, 0)
	page2, _, _ := l.ListAll(ListFilter{}, 2, 2)
	if len(page1) != 2 || len(page2) != 1 {
		t.Errorf("pagination wrong: %d / %d", len(page1), len(page2))
	}
}

func TestGetByIDNotFound(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	got, err := l.GetByID("nope")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil entry, got %+v", got)
	}
}

func TestAppendRequiresToolCallID(t *testing.T) {
	l := New(t.TempDir())
	if err := l.Append(Entry{Name: "read"}); err == nil {
		t.Errorf("expected error on empty ToolCallID")
	}
}

func TestSafeBlobNameSanitisesWeirdInput(t *testing.T) {
	cases := []struct{ in string }{
		{"toolu_abc123"}, {"id/with/slash"}, {"with space"}, {"中文"}, {""},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		got := safeBlobName(c.in)
		if got == "" {
			t.Errorf("safeBlobName(%q) returned empty", c.in)
		}
		// Filesystem-safe: ASCII alnum + _ -
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
			if !ok {
				t.Errorf("safeBlobName(%q) returned %q with bad char %q", c.in, got, r)
			}
		}
		seen[got] = true
	}
}

// helper: zero-pad small int to keep timestamps in append order distinct.
func idx(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return ""
}
