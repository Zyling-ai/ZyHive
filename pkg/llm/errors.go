// pkg/llm/errors.go — Error classification for LLM calls.
//
// The runner uses IsTransient() to decide whether a failed LLM call should be
// silently retried (jitter / network blip) or surfaced to the user immediately
// (auth failure, bad request, content policy).
//
// We intentionally stay conservative: when in doubt, return false so the user
// sees the real error rather than the agent silently retrying on something
// that will never succeed.
package llm

import (
	"context"
	"errors"
	"net"
	"strings"
)

// IsTransient reports whether err is a retry-worthy transport or rate-limit
// error from an LLM provider. Business-logic errors (400, 401, 403, 404,
// context length, content filter) return false.
//
// Detection is heuristic-based: we match on:
//  1. Stable HTTP status codes (429 / 500 / 502 / 503 / 504)
//  2. Go stdlib net error interfaces (net.Error.Timeout, *net.OpError etc.)
//  3. Common substrings across provider error messages
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Explicit cancellation should never retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// DeadlineExceeded counts as transient at the NETWORK level (request timeout)
		// but if the caller's context deadline fired, retrying same call will just
		// time out again. So we exclude it here and let the caller handle it.
		return errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "user")
	}

	// Go net error timeout (connection timeout, read timeout, etc.)
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// String-based matching (providers send varied error messages).
	msg := strings.ToLower(err.Error())

	// HTTP-layer transient statuses from provider-specific error strings.
	transientStatuses := []string{
		"429", "rate limit", "rate_limit", "too many requests",
		"502", "bad gateway",
		"503", "service unavailable", "temporarily unavailable",
		"504", "gateway timeout",
		"500", "internal server error",
	}
	for _, needle := range transientStatuses {
		if strings.Contains(msg, needle) {
			return true
		}
	}

	// TCP/TLS transient signatures.
	transientTransport := []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"no such host",    // DNS blip
		"i/o timeout",
		"eof",             // server dropped mid-stream
		"tls handshake",
		"stream error",    // HTTP/2 stream reset
	}
	for _, needle := range transientTransport {
		if strings.Contains(msg, needle) {
			return true
		}
	}

	return false
}

// IsAuthFailure reports whether err looks like a credential problem (401/403,
// "invalid api key" patterns). Useful to give user-facing hints like "check
// Provider config" instead of retrying forever.
func IsAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") {
		return true
	}
	if strings.Contains(msg, "403") || strings.Contains(msg, "forbidden") {
		return true
	}
	if strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "api_key") && strings.Contains(msg, "invalid") {
		return true
	}
	return false
}
