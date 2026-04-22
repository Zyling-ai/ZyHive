// pkg/llm/retry.go — Transient-error retry wrapper for Client.Stream.
//
// Wraps any llm.Client so that transient errors (detected by IsTransient) are
// retried with exponential backoff before being surfaced to the caller.
//
// Design constraints:
//   - Retries only happen when the stream errors BEFORE yielding any events.
//     If the LLM already streamed partial text then aborted, retrying would
//     double-send (confusing user + billing).
//   - Total wait time capped at ~8 seconds (0.5 + 2 + 5 = 7.5) so we don't
//     hang the HTTP handler forever.
//   - Each retry produces a ChatRequest-identical call; the provider's own
//     idempotency is assumed (generally true for stateless Chat endpoints).
package llm

import (
	"context"
	"log"
	"time"
)

// RetryClient wraps an underlying Client with automatic transient-error retry.
//
// Typical usage:
//
//	base := llm.NewClient(provider, baseURL)
//	client := llm.WithRetry(base)
//
// The returned client is still an llm.Client; callers don't need to change
// anything else.
type RetryClient struct {
	inner    Client
	attempts []time.Duration // backoff schedule
}

// DefaultRetrySchedule: 0.5s / 2s / 5s  — total ~7.5s worst case.
var DefaultRetrySchedule = []time.Duration{
	500 * time.Millisecond,
	2 * time.Second,
	5 * time.Second,
}

// WithRetry wraps a Client with the default retry schedule.
func WithRetry(inner Client) Client {
	return &RetryClient{inner: inner, attempts: DefaultRetrySchedule}
}

// WithRetrySchedule is for tests or specialized tuning.
func WithRetrySchedule(inner Client, schedule []time.Duration) Client {
	return &RetryClient{inner: inner, attempts: schedule}
}

// Stream implements Client. Retries only the initial-call error; once events
// start flowing, any subsequent error is surfaced immediately to avoid
// partial-response duplication.
func (r *RetryClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	attempt := 0
	for {
		// Respect context cancellation between attempts
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ch, err := r.inner.Stream(ctx, req)
		if err == nil {
			// On success: if this is a retry, wrap channel to detect mid-stream
			// errors — but we explicitly do NOT retry mid-stream.
			return ch, nil
		}
		if !IsTransient(err) {
			return nil, err
		}
		if attempt >= len(r.attempts) {
			return nil, err
		}
		backoff := r.attempts[attempt]
		log.Printf("[llm-retry] transient error (attempt %d/%d): %v — retrying in %v",
			attempt+1, len(r.attempts), err, backoff)
		attempt++
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
