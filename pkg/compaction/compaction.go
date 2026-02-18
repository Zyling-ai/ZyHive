// Package compaction handles context window compression.
// Reference: pi-coding-agent/dist/core/compaction/compaction.js
// Triggers when context usage exceeds ~80% of model's context window.
// Full implementation: Phase 1 (after runner is working)
package compaction

// ShouldCompact returns true when the context is getting too full.
// Reference: pi-coding-agent shouldCompact() function
func ShouldCompact(usedTokens, maxTokens int) bool {
	if maxTokens == 0 {
		return false
	}
	return float64(usedTokens)/float64(maxTokens) > 0.8
}

// TODO: implement Compact() — calls LLM to summarise history,
// writes a compaction entry to the session JSONL,
// and returns a trimmed history slice.
