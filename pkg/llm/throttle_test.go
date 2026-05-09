package llm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFixedThrottle_NoLimitPassThrough — MaxInflight=0 disables gating.
func TestFixedThrottle_NoLimitPassThrough(t *testing.T) {
	th := NewFixedThrottle(0)
	for i := 0; i < 100; i++ {
		release, err := th.Acquire(context.Background(), "anthropic")
		if err != nil {
			t.Fatalf("Acquire #%d: %v", i, err)
		}
		release(nil)
	}
}

// TestFixedThrottle_GatesAtCap — concurrent Acquire respects the cap.
func TestFixedThrottle_GatesAtCap(t *testing.T) {
	th := NewFixedThrottle(2)

	var inflight int32
	var maxSeen int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			rel, err := th.Acquire(ctx, "anthropic")
			if err != nil {
				t.Errorf("Acquire: %v", err)
				return
			}
			cur := atomic.AddInt32(&inflight, 1)
			for {
				prev := atomic.LoadInt32(&maxSeen)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxSeen, prev, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&inflight, -1)
			rel(nil)
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&maxSeen); got > 2 {
		t.Fatalf("max inflight = %d, want <= 2", got)
	}
}

// TestFixedThrottle_CtxCancelDuringWait — context cancel unparks waiter.
func TestFixedThrottle_CtxCancelDuringWait(t *testing.T) {
	th := NewFixedThrottle(1)
	rel, err := th.Acquire(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		_, err := th.Acquire(ctx, "anthropic")
		doneCh <- err
	}()
	time.Sleep(20 * time.Millisecond) // ensure the goroutine is parked
	cancel()
	select {
	case err := <-doneCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("Acquire did not unblock after cancel")
	}
	rel(nil)
}

// TestAdaptiveThrottle_SuccessGrowsWindow — after GrowEvery successes the cap
// increases by 1 (up to Max).
func TestAdaptiveThrottle_SuccessGrowsWindow(t *testing.T) {
	th := NewAdaptiveThrottle(AdaptiveConfig{
		Min: 1, Max: 4, Init: 2, GrowEvery: 3,
	}, nil)

	for i := 0; i < 3; i++ {
		rel, err := th.Acquire(context.Background(), "any")
		if err != nil {
			t.Fatalf("Acquire #%d: %v", i, err)
		}
		rel(nil)
	}
	snap := mustSnapshot(t, th, "any")
	if snap.MaxInflight != 3 {
		t.Fatalf("MaxInflight = %d, want 3", snap.MaxInflight)
	}
}

// TestAdaptiveThrottle_TransientHalves — a transient error halves the cap
// (floored at Min) and resets the consec-OK counter.
func TestAdaptiveThrottle_TransientHalves(t *testing.T) {
	th := NewAdaptiveThrottle(AdaptiveConfig{
		Min: 1, Max: 16, Init: 8, GrowEvery: 10,
	}, nil)

	rel, _ := th.Acquire(context.Background(), "any")
	rel(errors.New("HTTP 503 service unavailable")) // transient

	snap := mustSnapshot(t, th, "any")
	if snap.MaxInflight != 4 {
		t.Fatalf("MaxInflight after 503 = %d, want 4", snap.MaxInflight)
	}
}

// TestAdaptiveThrottle_RetryAfterCooldown — Retry-After embedded in the err
// sets cooldown; subsequent Acquire waits.
func TestAdaptiveThrottle_RetryAfterCooldown(t *testing.T) {
	th := NewAdaptiveThrottle(AdaptiveConfig{
		Min: 1, Max: 8, Init: 4, GrowEvery: 10, MaxBackoff: 200 * time.Millisecond,
	}, nil)

	rel, _ := th.Acquire(context.Background(), "x")
	// 5 seconds in the err string but capped to MaxBackoff (200ms in this test)
	rel(errors.New("HTTP 429 too many requests retry-after: 5"))

	start := time.Now()
	rel2, err := th.Acquire(context.Background(), "x")
	if err != nil {
		t.Fatalf("Acquire after cooldown: %v", err)
	}
	elapsed := time.Since(start)
	rel2(nil)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("expected to wait for cooldown, only waited %v", elapsed)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("cooldown should have been ~200ms, waited %v", elapsed)
	}
}

// TestAdaptiveThrottle_NonTransientLeavesCap — auth-failure-style errors do
// not change the cap.
func TestAdaptiveThrottle_NonTransientLeavesCap(t *testing.T) {
	th := NewAdaptiveThrottle(AdaptiveConfig{
		Min: 1, Max: 8, Init: 4, GrowEvery: 10,
	}, nil)

	rel, _ := th.Acquire(context.Background(), "x")
	rel(errors.New("HTTP 401 unauthorized"))

	snap := mustSnapshot(t, th, "x")
	if snap.MaxInflight != 4 {
		t.Fatalf("MaxInflight after 401 = %d, want unchanged 4", snap.MaxInflight)
	}
}

// TestParseRetryAfterFromErr — happy path + edge cases.
func TestParseRetryAfterFromErr(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"HTTP 429 retry-after: 30", 30 * time.Second},
		{"http 503 - retry after 5s", 5 * time.Second},
		{"i/o timeout", 0},
		{"", 0},
		{"retry-after: 9999", 600 * time.Second}, // capped
	}
	for _, c := range cases {
		got := parseRetryAfterFromErr(errors.New(c.in))
		if got != c.want {
			t.Errorf("parseRetryAfterFromErr(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := parseRetryAfterFromErr(nil); got != 0 {
		t.Errorf("nil err should yield 0, got %v", got)
	}
}

// TestWithThrottle_NilPassThrough — nil throttle returns the inner client.
func TestWithThrottle_NilPassThrough(t *testing.T) {
	mock := &noopClient{}
	got := WithThrottle(mock, nil, "any")
	if got != mock {
		t.Fatalf("nil throttle should pass through unchanged")
	}
}

// TestWithThrottle_ReleasesOnStreamEnd — release fires when the inner stream
// channel closes; subsequent Acquire on a 1-cap throttle succeeds.
func TestWithThrottle_ReleasesOnStreamEnd(t *testing.T) {
	mock := &noopClient{}
	th := NewFixedThrottle(1)
	c := WithThrottle(mock, th, "p1")

	out, err := c.Stream(context.Background(), &ChatRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range out {
		// drain
	}
	// Now we should be able to acquire again immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rel, err := th.Acquire(ctx, "p1")
	if err != nil {
		t.Fatalf("second Acquire blocked despite release: %v", err)
	}
	rel(nil)
}

// ── helpers ────────────────────────────────────────────────────────────────

func mustSnapshot(t *testing.T, th *AdaptiveThrottle, providerID string) ThrottleStateSnapshot {
	t.Helper()
	for _, s := range th.Snapshot() {
		if s.ProviderID == providerID {
			return s
		}
	}
	t.Fatalf("no snapshot for %q (got %+v)", providerID, th.Snapshot())
	return ThrottleStateSnapshot{}
}

// noopClient is a minimal Client used to exercise the throttle wrapper path.
type noopClient struct{}

func (noopClient) Stream(_ context.Context, _ *ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: EventStop}
	close(ch)
	return ch, nil
}
