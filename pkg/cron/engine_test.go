package cron

import (
	"testing"
	"time"
)

// TestEngine_LastTickAtZeroBeforeStart — the heartbeat is only meaningful
// after Start/Load; before then LastTickAt() must return the zero value so
// /readyz can distinguish "never started" from "started but stalled".
func TestEngine_LastTickAtZeroBeforeStart(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	if got := e.LastTickAt(); !got.IsZero() {
		t.Fatalf("LastTickAt before Start should be zero, got %v", got)
	}
}

// TestEngine_LastTickAtAfterStart — after Start the heartbeat goroutine sets
// lastTickAt immediately (before the first ticker fire), so the value must
// be very recent.
func TestEngine_LastTickAtAfterStart(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	e.Start()
	t.Cleanup(func() { e.Stop() })

	got := e.LastTickAt()
	if got.IsZero() {
		t.Fatalf("LastTickAt after Start should be non-zero")
	}
	if time.Since(got) > 5*time.Second {
		t.Fatalf("LastTickAt after Start should be recent, got %v ago", time.Since(got))
	}
}

// TestEngine_StartHeartbeatIdempotent — Load() then Start() (or any other
// double-call) must not spawn duplicate heartbeat goroutines, otherwise we
// leak on every restart.
func TestEngine_StartHeartbeatIdempotent(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	if err := e.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(func() { e.Stop() })

	first := e.heartbeatStop
	e.Start() // should be a no-op for the heartbeat
	if e.heartbeatStop != first {
		t.Fatalf("calling Start after Load swapped heartbeatStop channel; goroutine leaked")
	}
}
