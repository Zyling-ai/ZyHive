package session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func waitForWorker(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for worker state")
}

func terminalEvents(b *Broadcaster) []BroadcastEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var events []BroadcastEvent
	for _, event := range b.buffer {
		if event.Type == "done" || event.Type == "error" {
			events = append(events, event)
		}
	}
	return events
}

func TestWorkerPoolIsolatesOwners(t *testing.T) {
	pool := NewWorkerPool()
	defer pool.StopAll()

	first := pool.GetOrCreate("agent-a", "same-session")
	if got := pool.GetOrCreate("agent-a", "same-session"); got != first {
		t.Fatal("same owner/session should reuse worker")
	}
	second := pool.GetOrCreate("agent-b", "same-session")
	if second == first {
		t.Fatal("different owners must not share worker")
	}
	if got := pool.Get("agent-a", "same-session"); got != first {
		t.Fatal("owner-scoped lookup returned wrong worker")
	}
	if got := pool.GetUnique("same-session"); got != nil {
		t.Fatal("ambiguous legacy lookup must fail closed")
	}

	unique := pool.GetOrCreate("agent-c", "unique-session")
	if got := pool.GetUnique("unique-session"); got != unique {
		t.Fatal("unique legacy lookup should find the only worker")
	}
}

func TestSessionWorkerRejectsConcurrentTurnWithoutClearingGeneration(t *testing.T) {
	pool := NewWorkerPool()
	defer pool.StopAll()
	worker := pool.GetOrCreate("agent", "session")
	started := make(chan struct{})
	releaseRun := make(chan struct{})

	err := worker.Enqueue(RunRequest{
		SessionID: "session",
		RunFn: func(_ context.Context, _ string, _ string, bc *Broadcaster) error {
			bc.Publish(BroadcastEvent{Type: "text_delta", Data: []byte(`{"type":"text_delta","text":"first"}`)})
			close(started)
			<-releaseRun
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if worker.Broadcaster.BufferLen() != 1 {
		t.Fatalf("expected first generation event, got %d", worker.Broadcaster.BufferLen())
	}

	err = worker.Enqueue(RunRequest{SessionID: "session", RunFn: func(context.Context, string, string, *Broadcaster) error {
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected busy rejection, got %v", err)
	}
	if worker.Broadcaster.BufferLen() != 1 {
		t.Fatal("rejected enqueue must not clear the active generation")
	}

	close(releaseRun)
	waitForWorker(t, worker.Broadcaster.IsDone)
	events := terminalEvents(worker.Broadcaster)
	if len(events) != 1 || events[0].Type != "done" {
		t.Fatalf("expected one done event, got %+v", events)
	}
}

func TestSessionWorkerPublishesExactlyOneTerminalEvent(t *testing.T) {
	tests := []struct {
		name     string
		run      RunFnType
		wantType string
		wantText string
	}{
		{
			name:     "success",
			run:      func(context.Context, string, string, *Broadcaster) error { return nil },
			wantType: "done",
		},
		{
			name:     "error",
			run:      func(context.Context, string, string, *Broadcaster) error { return errors.New("run failed") },
			wantType: "error",
			wantText: "run failed",
		},
		{
			name: "already published",
			run: func(_ context.Context, _ string, _ string, bc *Broadcaster) error {
				bc.Publish(BroadcastEvent{Type: "done", Data: []byte(`{"type":"done"}`)})
				return errors.New("late error")
			},
			wantType: "done",
		},
		{
			name: "panic",
			run: func(context.Context, string, string, *Broadcaster) error {
				panic("boom")
			},
			wantType: "error",
			wantText: "panic: boom",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := NewWorkerPool()
			defer pool.StopAll()
			worker := pool.GetOrCreate("agent", test.name)
			if err := worker.Enqueue(RunRequest{SessionID: test.name, RunFn: test.run}); err != nil {
				t.Fatal(err)
			}
			waitForWorker(t, worker.Broadcaster.IsDone)
			events := terminalEvents(worker.Broadcaster)
			if len(events) != 1 || events[0].Type != test.wantType {
				t.Fatalf("expected one %s event, got %+v", test.wantType, events)
			}
			if test.wantText != "" && !strings.Contains(string(events[0].Data), test.wantText) {
				t.Fatalf("terminal data %q does not contain %q", events[0].Data, test.wantText)
			}
			if test.wantType == "done" && test.name == "success" {
				var payload map[string]any
				if err := json.Unmarshal(events[0].Data, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["sessionId"] != test.name {
					t.Fatalf("sessionId=%v, want %s", payload["sessionId"], test.name)
				}
			}
		})
	}
}
