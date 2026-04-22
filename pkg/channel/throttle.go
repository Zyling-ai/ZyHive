// pkg/channel/throttle.go — Pluggable send-throttle strategy for channel adapters.
//
// Why extract an interface when today we only have one implementation?
//   - Telegram / Feishu hard-code `time.NewTicker(1 * time.Second)` for stream
//     edit cadence. 1s matches small-group (<20 members) API limits.
//   - Large groups (>=20 members) need 3s+ per message. User-hosted ZyHive
//     can bind any bot to any group size → we don't control the chat.
//   - Today there's no adaptive code, BUT hard-coding on a private field
//     makes replacing it later a grep-and-replace across both channels.
//     A one-line interface switch is much cleaner when the day comes that
//     a user reports "my bot in the 80-person group drops messages".
//
// This file ships FixedThrottle (behavior identical to the original
// `time.NewTicker(1*time.Second)`). A future AdaptiveThrottle can then be
// swapped in without touching generateAndSend / feishu handlers.
package channel

import (
	"strings"
	"sync"
	"time"
)

// Throttle controls the minimum interval between outbound updates for a given
// chat ID.
//
//   - Wait(chatID) blocks until the next send for that chat is allowed and
//     returns how long it actually waited.
//   - OnResponse(chatID, err) lets adaptive implementations learn from the
//     provider's response (e.g. back off on 429). For FixedThrottle it's a no-op.
type Throttle interface {
	Wait(chatID int64) time.Duration
	OnResponse(chatID int64, err error)
}

// FixedThrottle applies a constant minimum interval per chat, independent of
// response codes. This matches the pre-26.4.23 behavior.
type FixedThrottle struct {
	interval time.Duration
	mu       sync.Mutex
	nextAt   map[int64]time.Time
}

// NewFixedThrottle returns a Throttle that enforces at least `interval` between
// consecutive updates to the same chat.
func NewFixedThrottle(interval time.Duration) *FixedThrottle {
	return &FixedThrottle{
		interval: interval,
		nextAt:   make(map[int64]time.Time),
	}
}

// Wait blocks until the per-chat cadence slot opens, then reserves the next
// slot. Returns the actual wait duration (0 for first call on a chat).
func (t *FixedThrottle) Wait(chatID int64) time.Duration {
	t.mu.Lock()
	now := time.Now()
	var wait time.Duration
	if next, ok := t.nextAt[chatID]; ok && next.After(now) {
		wait = next.Sub(now)
		t.nextAt[chatID] = next.Add(t.interval)
	} else {
		t.nextAt[chatID] = now.Add(t.interval)
	}
	t.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
	return wait
}

// OnResponse is a no-op for FixedThrottle. Adaptive implementations override
// this to inspect `err` for 429 / rate-limit signatures and adjust interval.
func (t *FixedThrottle) OnResponse(_ int64, _ error) {}

// IsRateLimitError is a small helper so future AdaptiveThrottle and existing
// callers agree on what counts as "too fast, back off".
// Currently unused; exported so tests and AdaptiveThrottle share the rule.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "flood_wait") // Telegram-specific
}
