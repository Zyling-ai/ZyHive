package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/session"
)

// fakeTitler is a minimal SessionTitleWriter for testing session_rename.
type fakeTitler struct {
	titles      map[string]string
	overridden  map[string]bool
	updateCalls int
}

func (f *fakeTitler) UpdateTitle(sessionID, title string) error {
	f.titles[sessionID] = title
	f.overridden[sessionID] = true
	f.updateCalls++
	return nil
}

func (f *fakeTitler) GetMeta(sessionID string) (session.SessionIndexEntry, bool) {
	t, ok := f.titles[sessionID]
	if !ok {
		return session.SessionIndexEntry{}, false
	}
	return session.SessionIndexEntry{
		ID:              sessionID,
		Title:           t,
		TitleOverridden: f.overridden[sessionID],
	}, true
}

func TestSessionRename_Basic(t *testing.T) {
	titler := &fakeTitler{
		titles:     map[string]string{"ses-1": "你好"},
		overridden: map[string]bool{"ses-1": false},
	}
	r := New(t.TempDir(), t.TempDir(), "agent-1")
	r.WithSessionTools(nil, nil, nil, titler)
	r.WithSessionID("ses-1")

	input, _ := json.Marshal(map[string]string{"title": "ZyStudio 战略规划"})
	out, err := r.handleSessionRename(context.Background(), input)
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if !strings.Contains(out, "已更新") {
		t.Fatalf("expected update confirmation, got: %s", out)
	}
	if titler.titles["ses-1"] != "ZyStudio 战略规划" {
		t.Fatalf("title not updated: %q", titler.titles["ses-1"])
	}
	if !titler.overridden["ses-1"] {
		t.Fatal("expected TitleOverridden=true after manual rename")
	}
}

func TestSessionRename_NoOp(t *testing.T) {
	titler := &fakeTitler{
		titles:     map[string]string{"ses-1": "相同标题"},
		overridden: map[string]bool{"ses-1": false},
	}
	r := New(t.TempDir(), t.TempDir(), "agent-1")
	r.WithSessionTools(nil, nil, nil, titler)
	r.WithSessionID("ses-1")

	input, _ := json.Marshal(map[string]string{"title": "相同标题"})
	out, err := r.handleSessionRename(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "未变") {
		t.Fatalf("expected no-op confirmation, got: %s", out)
	}
	if titler.updateCalls != 0 {
		t.Fatalf("expected no UpdateTitle call on no-op, got %d", titler.updateCalls)
	}
}

func TestSessionRename_EmptyTitle(t *testing.T) {
	titler := &fakeTitler{
		titles:     map[string]string{"ses-1": "x"},
		overridden: map[string]bool{"ses-1": false},
	}
	r := New(t.TempDir(), t.TempDir(), "agent-1")
	r.WithSessionTools(nil, nil, nil, titler)
	r.WithSessionID("ses-1")

	input, _ := json.Marshal(map[string]string{"title": "   "})
	_, err := r.handleSessionRename(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestSessionRename_NoSession(t *testing.T) {
	titler := &fakeTitler{titles: map[string]string{}, overridden: map[string]bool{}}
	r := New(t.TempDir(), t.TempDir(), "agent-1")
	r.WithSessionTools(nil, nil, nil, titler)
	// Intentionally NOT calling WithSessionID

	input, _ := json.Marshal(map[string]string{"title": "anything"})
	_, err := r.handleSessionRename(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when sessionID is empty")
	}
}

func TestSessionRename_TruncatesLongTitle(t *testing.T) {
	titler := &fakeTitler{
		titles:     map[string]string{"ses-1": "x"},
		overridden: map[string]bool{"ses-1": false},
	}
	r := New(t.TempDir(), t.TempDir(), "agent-1")
	r.WithSessionTools(nil, nil, nil, titler)
	r.WithSessionID("ses-1")

	// 40-char title should be capped at 30
	long := strings.Repeat("长", 40)
	input, _ := json.Marshal(map[string]string{"title": long})
	_, err := r.handleSessionRename(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	got := titler.titles["ses-1"]
	if runes := []rune(got); len(runes) != 30 {
		t.Fatalf("expected 30-rune cap, got %d runes: %q", len(runes), got)
	}
}
