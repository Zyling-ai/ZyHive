package toolaudit

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAuditDurationReflectsRealTime — issue an entry with a measured
// duration and verify it round-trips through the JSONL.
func TestAuditDurationReflectsRealTime(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	// Simulate a slow tool: write entry with explicit DurationMs.
	start := time.Now()
	// Tiny sleep so the round-trip latency is measurable even on fast machines.
	time.Sleep(20 * time.Millisecond)
	durMs := int(time.Since(start).Milliseconds())

	if err := l.Append(Entry{
		ToolCallID: "slow_1",
		Name:       "exec",
		Result:     "hello",
		DurationMs: durMs,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := l.GetByID("slow_1")
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v / nil=%v", err, got == nil)
	}
	if got.DurationMs < 15 {
		t.Errorf("DurationMs lost in round-trip: got %d, want >=15", got.DurationMs)
	}
}

// TestAuditConcurrentAppendsKeepDurationConsistent — fire 30 concurrent
// appends, each with its own DurationMs, and verify each one's duration is
// preserved (no torn writes mixing fields across rows).
func TestAuditConcurrentAppendsKeepDurationConsistent(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	const N = 30
	expected := make(map[string]int, N)
	for i := 0; i < N; i++ {
		id := "tc_" + itoaPad(i)
		// Each entry gets a unique DurationMs so we can verify per-id later.
		expected[id] = i * 7
	}

	var wg sync.WaitGroup
	for id, d := range expected {
		wg.Add(1)
		go func(id string, d int) {
			defer wg.Done()
			_ = l.Append(Entry{
				ToolCallID: id,
				Name:       "exec",
				Result:     "r",
				DurationMs: d,
			})
		}(id, d)
	}
	wg.Wait()

	// Pull every row and verify duration matches.
	for id, want := range expected {
		got, err := l.GetByID(id)
		if err != nil {
			t.Errorf("GetByID(%s): %v", id, err)
			continue
		}
		if got == nil {
			t.Errorf("GetByID(%s): nil", id)
			continue
		}
		if got.DurationMs != want {
			t.Errorf("DurationMs mismatch for %s: got %d, want %d", id, got.DurationMs, want)
		}
	}
}

// TestAuditEntryMarshalsToValidJSON — every appended entry must produce a
// single valid JSON object per line. Catches accidental embedded newlines or
// invalid Unicode escaping.
func TestAuditEntryMarshalsToValidJSON(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	weirdInput := json.RawMessage(`{"path":"/tmp/a\nb","emoji":"🚀"}`)
	weirdResult := "first line\nsecond line\ttab\r\nwindows newline"

	if err := l.Append(Entry{
		ToolCallID: "w1",
		Name:       "read",
		Input:      weirdInput,
		Result:     weirdResult,
		DurationMs: 42,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := l.GetByID("w1")
	if err != nil || got == nil {
		t.Fatalf("retrieval failed: %v / nil=%v", err, got == nil)
	}
	if string(got.Input) != string(weirdInput) {
		t.Errorf("input not preserved: got %s", got.Input)
	}
	if got.Result != weirdResult {
		t.Errorf("result not preserved with newlines/emoji")
	}
	if got.DurationMs != 42 {
		t.Errorf("DurationMs lost")
	}
}

// itoaPad returns a fixed-width decimal so map keys collate nicely.
func itoaPad(n int) string {
	s := ""
	if n == 0 {
		s = "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

var _ = strings.Builder{} // keep import for future tests
