package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// LLMCallFromClient adapts a real *llm.Client into the LLMScorer.Call
// signature. It builds a one-turn ChatRequest, drains the stream, and
// returns the accumulated text. Errors are propagated; transient
// failures are NOT retried here (LLMScorer.Fallback already covers).
//
// model: required. The model identifier the Client implementation
//        understands (e.g. "claude-sonnet-4-20250514" for AnthropicClient).
// apiKey: required by every Provider client.
// maxTokens: soft cap on the response length. Default 1024 if 0.
// timeout: hard wall-clock cap on the LLM call. Default 30s if 0.
//
// The adapter is intentionally minimal: judge prompts are short and
// the expected JSON output is small, so we don't need streaming UX —
// we just accumulate text into a single string.
func LLMCallFromClient(client llm.Client, model, apiKey string, maxTokens int, timeout time.Duration) LLMCall {
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return func(systemPrompt, userPrompt string) (string, error) {
		if client == nil {
			return "", fmt.Errorf("llm_adapter: nil client")
		}
		userContent, _ := json.Marshal(userPrompt)
		req := &llm.ChatRequest{
			Model:     model,
			System:    systemPrompt,
			APIKey:    apiKey,
			MaxTokens: maxTokens,
			Messages: []llm.ChatMessage{
				{
					Role:    "user",
					Content: userContent,
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		events, err := client.Stream(ctx, req)
		if err != nil {
			return "", fmt.Errorf("llm_adapter: stream start: %w", err)
		}

		var sb strings.Builder
		for ev := range events {
			switch ev.Type {
			case llm.EventTextDelta:
				sb.WriteString(ev.Text)
			case llm.EventError:
				return sb.String(), fmt.Errorf("llm_adapter: stream error: %v", ev.Err)
			case llm.EventStop:
				// done; drain remaining and return below
			}
		}
		out := strings.TrimSpace(sb.String())
		if out == "" {
			return "", fmt.Errorf("llm_adapter: empty response")
		}
		return out, nil
	}
}
