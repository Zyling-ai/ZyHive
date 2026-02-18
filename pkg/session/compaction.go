// Package session — context compaction logic.
// When a session's token estimate exceeds the threshold (80k tokens),
// old messages are summarized via LLM and replaced with a CompactionEntry.
// Reference: openclaw/src/hooks/bundled/session-memory/
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// CompactionThreshold is the token count that triggers compaction.
const CompactionThreshold = 80_000

// CompactIfNeeded checks if a session needs compaction and runs it asynchronously.
// Safe to call from runner after a completed turn; fires and forgets.
func CompactIfNeeded(store *Store, sessionID string, callLLM func(ctx context.Context, systemPrompt, userMsg string) (string, error)) {
	tokens := store.EstimateTokens(sessionID)
	if tokens < CompactionThreshold {
		return
	}
	log.Printf("[compaction] session %s has ~%d tokens, triggering compaction", sessionID, tokens)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := Compact(ctx, store, sessionID, callLLM); err != nil {
			log.Printf("[compaction] failed for session %s: %v", sessionID, err)
		} else {
			log.Printf("[compaction] completed for session %s", sessionID)
		}
	}()
}

// Compact performs context compaction on a session:
//  1. Reads all messages from JSONL
//  2. Keeps the last KeepTurns turns unchanged
//  3. Summarizes everything before the boundary via LLM
//  4. Writes a CompactionEntry to JSONL
//  5. Updates tokenEstimate in sessions.json
func Compact(ctx context.Context, store *Store, sessionID string, callLLM func(ctx context.Context, systemPrompt, userMsg string) (string, error)) error {
	const keepTurns = 20

	msgs, _, err := store.ReadHistory(sessionID)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if len(msgs) <= keepTurns {
		return nil // nothing to compact
	}

	// Split: old (to summarize) + recent (to keep)
	boundary := len(msgs) - keepTurns
	old := msgs[:boundary]
	// recent := msgs[boundary:] // kept as-is in JSONL (not re-written)

	// Build conversation text for summarization
	var sb strings.Builder
	for _, m := range old {
		label := "User"
		if m.Role == "assistant" {
			label = "Assistant"
		}
		text := extractTextFromContent(m.Content)
		if text != "" {
			sb.WriteString(label)
			sb.WriteString(": ")
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
	}

	systemPrompt := `You are a conversation summarizer. 
Produce a concise summary (max 500 words) of the conversation below that captures:
- Key topics discussed
- Important decisions or conclusions  
- Code, data, or technical context that would be needed for continuation
- The user's main goals

Be factual and preserve technical details. Reply with just the summary, no preamble.`

	summary, err := callLLM(ctx, systemPrompt, sb.String())
	if err != nil {
		return fmt.Errorf("llm summarize: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Errorf("empty summary from LLM")
	}

	// Write CompactionEntry to JSONL
	// The entry marks the boundary: history before this is replaced by summary
	compEntry := CompactionEntry{
		BaseEntry:        BaseEntry{Type: EntryTypeCompaction},
		Summary:          summary,
		FirstKeptEntryID: fmt.Sprintf("turn-%d", boundary),
		TokensBefore:     store.EstimateTokens(sessionID),
		Timestamp:        nowMs(),
	}
	if err := store.Append(sessionID, compEntry); err != nil {
		return fmt.Errorf("append compaction entry: %w", err)
	}

	// Re-append the recent messages after the compaction marker
	// (so ReadHistory picks them up correctly on next load)
	for _, m := range msgs[boundary:] {
		if err := store.AppendMessage(sessionID, m.Role, m.Content); err != nil {
			log.Printf("[compaction] failed to re-append message: %v", err)
		}
	}

	// Update token estimate to post-compaction size (~summary + recent turns)
	summaryTokens := len(summary) / 4
	var recentTokens int
	for _, m := range msgs[boundary:] {
		recentTokens += len(m.Content) / 4
	}
	newEstimate := summaryTokens + recentTokens + 500 // 500 overhead
	compEntry.TokensAfter = newEstimate

	// Update the index
	store.mu.Lock()
	idx, err2 := store.loadIndex()
	if err2 == nil {
		if meta, ok := idx.Sessions[sessionID]; ok {
			meta.TokenEstimate = newEstimate
			idx.Sessions[sessionID] = meta
			_ = store.saveIndex(idx)
		}
	}
	store.mu.Unlock()

	log.Printf("[compaction] session %s: %d → %d tokens, summary: %d chars",
		sessionID, compEntry.TokensBefore, newEstimate, len(summary))
	return nil
}

// extractTextFromContent pulls plain text from raw message content.
func extractTextFromContent(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return string(content)
}
