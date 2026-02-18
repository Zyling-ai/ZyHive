// Package compaction handles context window compression.
// Reference: pi-coding-agent/dist/core/compaction/compaction.js
// Triggers when context usage exceeds ~75% of model's context window.
package compaction

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
)

const compactionPrompt = `Please provide a detailed summary of our conversation so far. Include: key decisions made, important information shared, tasks completed, current status, and what needs to happen next. Be comprehensive - this summary will replace the conversation history.`

// Compactor manages context window compression via LLM summarization.
type Compactor struct {
	llmClient llm.Client
	apiKey    string
	model     string
}

// NewCompactor creates a new Compactor.
func NewCompactor(client llm.Client, apiKey, model string) *Compactor {
	return &Compactor{
		llmClient: client,
		apiKey:    apiKey,
		model:     model,
	}
}

// ShouldCompact returns true when the context is getting too full.
// Reference: pi-coding-agent shouldCompact() function
func ShouldCompact(usedTokens, maxTokens int) bool {
	if maxTokens == 0 {
		return false
	}
	return float64(usedTokens)/float64(maxTokens) > 0.75
}

// Compact calls LLM to summarize the session history, writes a compaction
// entry to the session, and returns trimmed message history.
func (c *Compactor) Compact(ctx context.Context, history []llm.ChatMessage, store *session.Store, sessionID string) ([]llm.ChatMessage, error) {
	if len(history) == 0 {
		return history, nil
	}

	// Build a request asking the LLM to summarize the conversation
	summaryMessages := make([]llm.ChatMessage, 0, len(history)+1)
	summaryMessages = append(summaryMessages, history...)

	// Add the compaction request as a user message
	promptContent, _ := json.Marshal(compactionPrompt)
	summaryMessages = append(summaryMessages, llm.ChatMessage{
		Role:    "user",
		Content: promptContent,
	})

	req := &llm.ChatRequest{
		Model:     c.model,
		APIKey:    c.apiKey,
		Messages:  summaryMessages,
		MaxTokens: 4096,
	}

	events, err := c.llmClient.Stream(ctx, req)
	if err != nil {
		return history, fmt.Errorf("compaction llm request: %w", err)
	}

	// Collect the full summary text
	var summary string
	for ev := range events {
		if ev.Type == llm.EventTextDelta {
			summary += ev.Text
		}
		if ev.Type == llm.EventError && ev.Err != nil {
			return history, fmt.Errorf("compaction llm error: %w", ev.Err)
		}
	}

	if summary == "" {
		return history, fmt.Errorf("compaction produced empty summary")
	}

	// Estimate token counts (rough: 1 token ≈ 4 chars)
	tokensBefore := 0
	for _, m := range history {
		tokensBefore += len(m.Content) / 4
	}
	tokensAfter := len(summary) / 4

	// Write compaction entry to session JSONL
	compactionEntry := session.CompactionEntry{
		BaseEntry: session.BaseEntry{
			Type: session.EntryTypeCompaction,
			ID:   "compact-" + uuid.New().String()[:8],
		},
		Summary:      summary,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
	}

	if store != nil && sessionID != "" {
		if err := store.Append(sessionID, compactionEntry); err != nil {
			// Log but don't fail
			fmt.Printf("compaction: failed to write entry to session: %v\n", err)
		}
	}

	// Return new trimmed history: just the compaction summary as a user message
	summaryContent, _ := json.Marshal("[Previous conversation summary]\n\n" + summary)
	newHistory := []llm.ChatMessage{
		{
			Role:    "user",
			Content: summaryContent,
		},
	}

	return newHistory, nil
}
