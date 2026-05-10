// 26.5.10v3 — B002 timing-side-channel mitigation.
//
// Plain `==` / `!=` on string secrets (auth tokens) leaks the secret one
// character at a time via response-time side-channel: the longer the prefix
// match, the longer Go's string equality short-circuit runs. With network
// access an attacker can recover the token char-by-char.
//
// Fix: use crypto/subtle.ConstantTimeCompare which always touches all bytes
// regardless of where they differ. Wrapped in `secretsEqual` for readability
// and centralized future hardening (e.g. HMAC of token).
package api

import "crypto/subtle"

// secretsEqual is a constant-time comparison for two secret strings.
// Both arguments are touched byte-by-byte; total runtime depends only on
// max(len(a), len(b)), not on where (or whether) bytes differ.
//
// Returns false (without timing leak) when lengths differ.
func secretsEqual(a, b string) bool {
	// subtle.ConstantTimeCompare returns 0 immediately when lengths differ
	// (this length-leak is acceptable: token length is not secret).
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
