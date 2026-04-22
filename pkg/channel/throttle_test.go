package channel

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFixedThrottle_FirstCallReturnsImmediately(t *testing.T) {
	th := NewFixedThrottle(500 * time.Millisecond)
	start := time.Now()
	w := th.Wait(42)
	elapsed := time.Since(start)
	if w > 5*time.Millisecond {
		t.Errorf("first call should not wait, got %v", w)
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("first call elapsed too long: %v", elapsed)
	}
}

func TestFixedThrottle_SecondCallWaits(t *testing.T) {
	th := NewFixedThrottle(100 * time.Millisecond)
	th.Wait(42)
	start := time.Now()
	th.Wait(42)
	elapsed := time.Since(start)
	if elapsed < 90*time.Millisecond {
		t.Errorf("second call should wait ~100ms, got %v", elapsed)
	}
}

func TestFixedThrottle_DifferentChatsIndependent(t *testing.T) {
	th := NewFixedThrottle(100 * time.Millisecond)
	th.Wait(1)
	start := time.Now()
	th.Wait(2) // different chat — should not wait
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Errorf("different chat should not wait, got %v", elapsed)
	}
}

func TestFixedThrottle_ConcurrentSafe(t *testing.T) {
	th := NewFixedThrottle(5 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			th.Wait(id % 5)
		}(int64(i))
	}
	wg.Wait()
}

func TestFixedThrottle_OnResponseNoOp(t *testing.T) {
	// OnResponse on FixedThrottle is a no-op; calling it should not error or panic.
	th := NewFixedThrottle(10 * time.Millisecond)
	th.OnResponse(42, nil)
	th.OnResponse(42, errors.New("429 too many requests"))
	// Interval unchanged — verify Wait still returns ~10ms after a primer call.
	th.Wait(42)
	start := time.Now()
	th.Wait(42)
	elapsed := time.Since(start)
	if elapsed > 20*time.Millisecond {
		t.Errorf("interval should not adapt for FixedThrottle, got %v", elapsed)
	}
}

// Interface contract: ensure FixedThrottle satisfies Throttle.
var _ Throttle = (*FixedThrottle)(nil)
