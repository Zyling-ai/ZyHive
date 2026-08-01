package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func seedSession(t *testing.T, store *Store, sessionID string, count int) {
	t.Helper()
	if _, _, err := store.GetOrCreate(sessionID, "agent"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content, _ := json.Marshal(fmt.Sprintf("message-%02d", i))
		if err := store.AppendMessage(sessionID, role, content); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompactReplacesOneGenerationWithoutDuplicates(t *testing.T) {
	store := NewStore(t.TempDir())
	seedSession(t, store, "session-one", 30)
	var calls atomic.Int32
	err := Compact(context.Background(), store, "session-one",
		func(context.Context, string, string) (string, error) {
			calls.Add(1)
			return "summary-one", nil
		}, "")
	if err != nil {
		t.Fatal(err)
	}
	history, summary, err := store.ReadHistory("session-one")
	if err != nil {
		t.Fatal(err)
	}
	if summary != "summary-one" || len(history) != 20 {
		t.Fatalf("summary=%q messages=%d", summary, len(history))
	}
	entries, err := store.ReadAll("session-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 22 {
		t.Fatalf("entries=%d, want header + compaction + 20 messages", len(entries))
	}
	if err := Compact(context.Background(), store, "session-one",
		func(context.Context, string, string) (string, error) {
			calls.Add(1)
			return "unexpected", nil
		}, ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("LLM called %d times for one generation", calls.Load())
	}
}

func TestCompactRecoversPreparedSidecarWithoutRepeatingLLM(t *testing.T) {
	store := NewStore(t.TempDir())
	seedSession(t, store, "recover", 30)
	snapshot, err := store.compactionSnapshot("recover")
	if err != nil {
		t.Fatal(err)
	}
	state := compactionState{
		Version:      1,
		Status:       "prepared",
		Generation:   snapshot.Generation,
		Summary:      "prepared-summary",
		TokensBefore: snapshot.TokensBefore,
		TokensAfter:  600,
		Boundary:     10,
		UpdatedAt:    nowMs(),
	}
	if err := store.saveCompactionState("recover", state); err != nil {
		t.Fatal(err)
	}
	if err := Compact(context.Background(), store, "recover",
		func(context.Context, string, string) (string, error) {
			return "", errors.New("LLM must not be called")
		}, ""); err != nil {
		t.Fatal(err)
	}
	_, summary, err := store.ReadHistory("recover")
	if err != nil {
		t.Fatal(err)
	}
	if summary != "prepared-summary" {
		t.Fatalf("summary=%q", summary)
	}
}

func TestCompactRejectsChangedGenerationWithoutDataLoss(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	other := NewStore(dir)
	seedSession(t, store, "changing", 30)
	err := Compact(context.Background(), store, "changing",
		func(context.Context, string, string) (string, error) {
			content, _ := json.Marshal("concurrent-message")
			if appendErr := other.AppendMessage("changing", "user", content); appendErr != nil {
				t.Fatal(appendErr)
			}
			return "stale-summary", nil
		}, "")
	if !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("expected ErrSessionChanged, got %v", err)
	}
	history, _, readErr := store.ReadHistory("changing")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(history) != 31 {
		t.Fatalf("history=%d, want all 31 messages", len(history))
	}
}

func TestMultipleStoresShareOneSessionBoundary(t *testing.T) {
	dir := t.TempDir()
	first := NewStore(dir)
	second := NewStore(dir)
	seedSession(t, first, "shared", 0)
	var wait sync.WaitGroup
	for i := 0; i < 50; i++ {
		store := first
		if i%2 == 1 {
			store = second
		}
		wait.Add(1)
		go func(i int, store *Store) {
			defer wait.Done()
			content, _ := json.Marshal(fmt.Sprintf("message-%d", i))
			if err := store.AppendMessage("shared", "user", content); err != nil {
				t.Errorf("AppendMessage: %v", err)
			}
		}(i, store)
	}
	wait.Wait()
	history, _, err := first.ReadHistory("shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 50 {
		t.Fatalf("history=%d, want 50", len(history))
	}
	meta, ok := first.GetMeta("shared")
	if !ok || meta.MessageCount != 50 {
		t.Fatalf("meta=%+v ok=%v", meta, ok)
	}
}
