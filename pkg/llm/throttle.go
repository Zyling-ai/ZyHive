// pkg/llm/throttle.go — Provider-aware concurrency limiter for LLM calls.
//
// Why:
//   - retry.go retries transient errors, but it doesn't reduce request rate.
//     Repeated 429s can hammer the provider and make the situation worse.
//   - Real providers expose Retry-After headers; we should honor them.
//
// Two implementations live here:
//
//   FixedThrottle    — bounded inflight per provider, no learning. Default.
//                     Behaviour-equivalent to "no throttle" when MaxInflight=0.
//
//   AdaptiveThrottle — AIMD-like: shrink window on transient/Retry-After,
//                     grow window after consecutive successes. Per-provider
//                     state. Off by default; opt-in via Config.
//
// Both implement the Throttle interface. Wrap a Client via WithThrottle to
// add gating; the wrapper chains underneath WithRetry so retries operate at
// the same throttle slot they were granted.
package llm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Interfaces ──────────────────────────────────────────────────────────────

// Throttle is the gate consulted before each LLM Stream attempt.
//
// Acquire blocks until the caller may proceed (or ctx expires). The returned
// release function MUST be invoked exactly once when the call ends, with the
// observed outcome:
//
//   release(err)       — pass the error from Stream(), or nil on success
//
// Implementations use the err to update internal state (e.g. shrink window
// on 429). Pass retryAfter=0 when not known; AdaptiveThrottle parses common
// HTTP "Retry-After" header values from error strings as a fallback.
type Throttle interface {
	Acquire(ctx context.Context, providerID string) (release func(err error), err error)
}

// ── FixedThrottle ──────────────────────────────────────────────────────────

// FixedThrottle limits inflight per provider with a fixed cap. When
// MaxInflight is 0 (zero value), the throttle is a no-op pass-through —
// matching today's "no throttle" behaviour exactly.
type FixedThrottle struct {
	MaxInflight int

	mu      sync.Mutex
	inUse   map[string]int
	waiters map[string][]chan struct{}
}

// NewFixedThrottle returns a FixedThrottle with the given cap. Pass 0 for
// "no throttle". Callers can also reach in and tweak MaxInflight at runtime;
// it's read under the mutex on every Acquire/release.
func NewFixedThrottle(maxInflight int) *FixedThrottle {
	return &FixedThrottle{
		MaxInflight: maxInflight,
		inUse:       map[string]int{},
		waiters:     map[string][]chan struct{}{},
	}
}

func (t *FixedThrottle) Acquire(ctx context.Context, providerID string) (func(error), error) {
	for {
		t.mu.Lock()
		cap := t.MaxInflight
		if cap <= 0 || t.inUse[providerID] < cap {
			t.inUse[providerID]++
			t.mu.Unlock()
			return func(error) { t.release(providerID) }, nil
		}
		// Park as a waiter.
		ch := make(chan struct{})
		t.waiters[providerID] = append(t.waiters[providerID], ch)
		t.mu.Unlock()

		select {
		case <-ch:
			// woken by release; loop and try again
		case <-ctx.Done():
			t.removeWaiter(providerID, ch)
			return nil, ctx.Err()
		}
	}
}

func (t *FixedThrottle) release(providerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inUse[providerID] > 0 {
		t.inUse[providerID]--
	}
	// Wake one waiter, if any.
	queue := t.waiters[providerID]
	for len(queue) > 0 {
		ch := queue[0]
		queue = queue[1:]
		t.waiters[providerID] = queue
		select {
		case ch <- struct{}{}:
			return
		default:
			// Receiver gone; drop and try next.
		}
	}
}

func (t *FixedThrottle) removeWaiter(providerID string, ch chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	queue := t.waiters[providerID]
	for i, w := range queue {
		if w == ch {
			t.waiters[providerID] = append(queue[:i], queue[i+1:]...)
			return
		}
	}
}

// ── AdaptiveThrottle ───────────────────────────────────────────────────────

// AdaptiveConfig configures one provider's adaptive window (AIMD).
type AdaptiveConfig struct {
	Min        int // lower bound for maxInflight; default 1
	Max        int // upper bound; default 8
	Init       int // starting maxInflight; default 4
	GrowEvery  int // grow window after this many consecutive successes; default 10
	MaxBackoff time.Duration // cap for cooldown after Retry-After; default 60s
}

func (c AdaptiveConfig) withDefaults() AdaptiveConfig {
	if c.Min < 1 {
		c.Min = 1
	}
	if c.Max < c.Min {
		c.Max = 8
	}
	if c.Init < c.Min {
		c.Init = c.Min
	}
	if c.Init > c.Max {
		c.Init = c.Max
	}
	if c.GrowEvery <= 0 {
		c.GrowEvery = 10
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 60 * time.Second
	}
	return c
}

// AdaptiveThrottle is a per-provider AIMD limiter:
//   - Success: count toward GrowEvery; growing window by +1 (capped at Max)
//   - Transient (429/503/etc): halve the window (down to Min)
//   - Retry-After observed: set cooldownTill = now + retryAfter (capped)
//   - Hard auth fail / non-transient: untouched
type AdaptiveThrottle struct {
	configs map[string]AdaptiveConfig
	def     AdaptiveConfig

	mu     sync.Mutex
	state  map[string]*provState
}

type provState struct {
	cap          int
	inflight     int
	cooldownTill time.Time
	consecOK     int
	cond         *sync.Cond // for waking waiters
}

// NewAdaptiveThrottle constructs an AdaptiveThrottle. perProvider maps
// providerID → AdaptiveConfig; def is used for any provider not in the map
// (typically via key "*" sentinel).
func NewAdaptiveThrottle(def AdaptiveConfig, perProvider map[string]AdaptiveConfig) *AdaptiveThrottle {
	cfgs := map[string]AdaptiveConfig{}
	for k, v := range perProvider {
		cfgs[k] = v.withDefaults()
	}
	return &AdaptiveThrottle{
		configs: cfgs,
		def:     def.withDefaults(),
		state:   map[string]*provState{},
	}
}

func (a *AdaptiveThrottle) configFor(providerID string) AdaptiveConfig {
	if c, ok := a.configs[providerID]; ok {
		return c
	}
	return a.def
}

// stateForLocked returns or creates the provider state. Caller must hold a.mu.
func (a *AdaptiveThrottle) stateForLocked(providerID string) *provState {
	st, ok := a.state[providerID]
	if !ok {
		cfg := a.configFor(providerID)
		st = &provState{cap: cfg.Init}
		st.cond = sync.NewCond(&a.mu)
		a.state[providerID] = st
	}
	return st
}

func (a *AdaptiveThrottle) Acquire(ctx context.Context, providerID string) (func(error), error) {
	a.mu.Lock()
	st := a.stateForLocked(providerID)

	// Wait for cooldown + capacity.
	for {
		if err := ctx.Err(); err != nil {
			a.mu.Unlock()
			return nil, err
		}
		now := time.Now()
		if now.Before(st.cooldownTill) {
			// Sleep with ctx awareness; release lock during sleep.
			delta := time.Until(st.cooldownTill)
			a.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delta):
			}
			a.mu.Lock()
			continue
		}
		if st.inflight < st.cap {
			st.inflight++
			a.mu.Unlock()
			return func(err error) { a.release(providerID, err) }, nil
		}
		// Capacity full — block on cond. Use a watchdog goroutine to wake
		// us if the ctx expires while parked, since sync.Cond can't select
		// on ctx natively.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				a.mu.Lock()
				st.cond.Broadcast()
				a.mu.Unlock()
			case <-done:
			}
		}()
		st.cond.Wait()
		close(done)
	}
}

// release returns one slot and updates the AIMD state based on err.
func (a *AdaptiveThrottle) release(providerID string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := a.state[providerID]
	if st == nil {
		return
	}
	cfg := a.configFor(providerID)
	if st.inflight > 0 {
		st.inflight--
	}
	switch {
	case err == nil:
		st.consecOK++
		if st.consecOK >= cfg.GrowEvery && st.cap < cfg.Max {
			st.cap++
			st.consecOK = 0
		}
	case IsTransient(err):
		st.consecOK = 0
		// Halve cap, floored at Min
		nc := st.cap / 2
		if nc < cfg.Min {
			nc = cfg.Min
		}
		st.cap = nc
		// Honour Retry-After if discoverable in the error string.
		if d := parseRetryAfterFromErr(err); d > 0 {
			if d > cfg.MaxBackoff {
				d = cfg.MaxBackoff
			}
			until := time.Now().Add(d)
			if until.After(st.cooldownTill) {
				st.cooldownTill = until
			}
		}
	default:
		// Non-transient (auth / context length): leave cap alone but reset OK
		// counter so we don't immediately grow on the next success.
		st.consecOK = 0
	}
	st.cond.Broadcast()
}

// parseRetryAfterFromErr is a best-effort scan for "retry-after: N" or
// "retry after Ns" patterns embedded in error messages. Returns 0 when
// nothing matches.
//
// Real HTTP-level Retry-After parsing happens at the provider layer (see
// pkg/llm/httpclient.go::parseRetryAfter for the header-form helper); this
// fallback covers the common case where the error has been wrapped into a
// string. Conservative: fail-closed to 0 so callers don't sleep forever on
// garbage input.
func parseRetryAfterFromErr(err error) time.Duration {
	if err == nil {
		return 0
	}
	s := strings.ToLower(err.Error())
	idx := strings.Index(s, "retry-after")
	if idx < 0 {
		idx = strings.Index(s, "retry after")
	}
	if idx < 0 {
		return 0
	}
	rest := s[idx:]
	// Find first run of digits.
	start, end := -1, -1
	for i, c := range rest {
		if c >= '0' && c <= '9' {
			if start < 0 {
				start = i
			}
			end = i + 1
		} else if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	n, perr := strconv.Atoi(rest[start:end])
	if perr != nil || n <= 0 {
		return 0
	}
	if n > 600 { // cap absurd values to 10 minutes
		n = 600
	}
	return time.Duration(n) * time.Second
}

// ── Snapshot for /api/llm/throttle ──────────────────────────────────────────

// ThrottleStateSnapshot is a sanitized read-only view of one provider's
// adaptive throttle state. Used by /api/llm/throttle endpoint (TODO).
type ThrottleStateSnapshot struct {
	ProviderID         string `json:"providerId"`
	MaxInflight        int    `json:"maxInflight"`
	Inflight           int    `json:"inflight"`
	CooldownRemainingS int64  `json:"cooldownRemainingS"`
	ConsecOK           int    `json:"consecOK"`
}

// Snapshot returns the per-provider state, intended for observability.
func (a *AdaptiveThrottle) Snapshot() []ThrottleStateSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	out := make([]ThrottleStateSnapshot, 0, len(a.state))
	for id, st := range a.state {
		rem := int64(0)
		if st.cooldownTill.After(now) {
			rem = int64(st.cooldownTill.Sub(now).Seconds())
		}
		out = append(out, ThrottleStateSnapshot{
			ProviderID:         id,
			MaxInflight:        st.cap,
			Inflight:           st.inflight,
			CooldownRemainingS: rem,
			ConsecOK:           st.consecOK,
		})
	}
	return out
}

// ── Wrapper Client ─────────────────────────────────────────────────────────

// throttledClient wraps an underlying Client and consults a Throttle before
// each Stream call. The release callback is invoked on stream completion or
// error; the throttle uses the err to update its window.
type throttledClient struct {
	inner    Client
	throttle Throttle
	provider string
}

// WithThrottle wraps client so that every Stream goes through `t` keyed by
// providerID. Returns inner unchanged if t is nil (no-op pass-through).
func WithThrottle(client Client, t Throttle, providerID string) Client {
	if t == nil {
		return client
	}
	return &throttledClient{inner: client, throttle: t, provider: providerID}
}

func (c *throttledClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	release, err := c.throttle.Acquire(ctx, c.provider)
	if err != nil {
		return nil, err
	}
	ch, err := c.inner.Stream(ctx, req)
	if err != nil {
		release(err)
		return nil, err
	}
	// Drain the inner channel and release on close. Forward all events.
	out := make(chan StreamEvent, 16)
	go func() {
		defer close(out)
		var lastErr error
		for ev := range ch {
			if ev.Type == EventError && ev.Err != nil {
				lastErr = ev.Err
			}
			out <- ev
		}
		release(lastErr)
	}()
	return out, nil
}

// Sentinel error so callers can distinguish "throttle gave up" from upstream
// transient errors. Currently only used in tests.
var ErrThrottleAbort = errors.New("throttle aborted")

// ── Process-wide throttle (opt-in) ─────────────────────────────────────────
//
// Most callers wrap a Client at construction; but our existing call graph
// has runner.New wrapping cfg.LLM with WithRetry inside the runner package.
// To avoid plumbing a Throttle through every constructor, we expose a
// process-global slot. main.go installs once at startup; runner.New checks
// and wraps if non-nil.
//
// This is intentionally a single global — multi-tenant deployments would
// need per-tenant throttles, but ZyHive is single-tenant today.

var (
	globalThrottleMu sync.Mutex
	globalThrottle   Throttle
)

// SetGlobalThrottle installs (or removes, with nil) a process-global throttle.
// Idempotent; calling twice replaces. Safe to call before or after runners
// are constructed; runner.New consults this every call.
func SetGlobalThrottle(t Throttle) {
	globalThrottleMu.Lock()
	defer globalThrottleMu.Unlock()
	globalThrottle = t
}

// GlobalThrottle returns the currently installed process-wide throttle, or
// nil. Read-only consumers (runner, /api/llm/throttle) call this on every
// request.
func GlobalThrottle() Throttle {
	globalThrottleMu.Lock()
	defer globalThrottleMu.Unlock()
	return globalThrottle
}
