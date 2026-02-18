// Package runner implements the core agent conversation loop.
// Reference: pi-coding-agent/dist/core/agent-session.js (AgentSession._handleAgentEvent)
//            openclaw/src/agents/pi-embedded-runner/run/attempt.ts
//
// The main loop:
//   1. Build system prompt (identity + soul + workspace files + skills)
//   2. Append user message to history
//   3. Call LLM with history + tools
//   4. If response contains tool_use blocks → execute tools → append results → goto 3
//   5. If response is plain text → return to caller
package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/tools"
)

// Config holds all dependencies for a Runner instance.
type Config struct {
	AgentID      string
	WorkspaceDir string
	Model        string
	APIKey       string
	LLM          llm.Client
	Tools        *tools.Registry
	Session      *session.Store
	// Optional: extra context injected before the user message (e.g. page context, scenario)
	ExtraContext string
	// Optional: base64 image data URIs attached to the user message
	Images []string
	// Optional: preloaded conversation history (from client-side state)
	PreloadedHistory []llm.ChatMessage
}

// Runner drives a single agent's conversation lifecycle.
type Runner struct {
	cfg     Config
	history []llm.ChatMessage
}

// New creates a Runner for the given agent.
func New(cfg Config) *Runner {
	r := &Runner{cfg: cfg}
	if len(cfg.PreloadedHistory) > 0 {
		r.history = append(r.history, cfg.PreloadedHistory...)
	}
	return r
}

// RunEvent is emitted to the caller during a conversation turn.
type RunEvent struct {
	Type    string // "text_delta" | "tool_call" | "tool_result" | "error" | "done"
	Text    string
	ToolCall *llm.ToolCall
	Error   error
}

// Run processes one user message and streams events until the model stops.
// The caller receives events via the returned channel.
func (r *Runner) Run(ctx context.Context, userMsg string) <-chan RunEvent {
	out := make(chan RunEvent, 32)
	go func() {
		defer close(out)
		if err := r.run(ctx, userMsg, out); err != nil {
			out <- RunEvent{Type: "error", Error: err}
		}
	}()
	return out
}

func (r *Runner) run(ctx context.Context, userMsg string, out chan<- RunEvent) error {
	// 1. Append user message to history (with optional images)
	var userContent json.RawMessage
	if len(r.cfg.Images) > 0 {
		// Multimodal: build content array [image, ..., text]
		type imgSrc struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		}
		type imgBlock struct {
			Type   string `json:"type"`
			Source imgSrc `json:"source"`
		}
		type textBlock struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		parts := make([]any, 0, len(r.cfg.Images)+1)
		for _, img := range r.cfg.Images {
			// img is "data:image/png;base64,..." or just raw base64
			mediaType := "image/jpeg"
			data := img
			if idx := len("data:"); len(img) > idx {
				if img[:idx] == "data:" {
					semi := 0
					for i, c := range img[idx:] {
						if c == ';' { semi = idx + i; break }
					}
					if semi > 0 {
						mediaType = img[idx:semi]
						// skip "base64,"
						comma := semi
						for i, c := range img[semi:] {
							if c == ',' { comma = semi + i + 1; break }
						}
						data = img[comma:]
					}
				}
			}
			parts = append(parts, imgBlock{
				Type:   "image",
				Source: imgSrc{Type: "base64", MediaType: mediaType, Data: data},
			})
		}
		parts = append(parts, textBlock{Type: "text", Text: userMsg})
		userContent, _ = json.Marshal(parts)
	} else {
		userContent, _ = json.Marshal(userMsg)
	}
	r.history = append(r.history, llm.ChatMessage{
		Role:    "user",
		Content: userContent,
	})

	// 2. Agentic loop — call LLM, handle tools, repeat
	const maxIter = 10
	for i := 0; i < maxIter; i++ {
		// Build system prompt from workspace identity files
		systemPrompt, _ := BuildSystemPrompt(r.cfg.WorkspaceDir)
		if r.cfg.ExtraContext != "" {
			systemPrompt = systemPrompt + "\n\n---\n" + r.cfg.ExtraContext
		}

		req := &llm.ChatRequest{
			Model:    r.cfg.Model,
			APIKey:   r.cfg.APIKey,
			System:   systemPrompt,
			Messages: r.history,
			Tools:    r.cfg.Tools.Definitions(),
		}

		events, err := r.cfg.LLM.Stream(ctx, req)
		if err != nil {
			return fmt.Errorf("llm stream: %w", err)
		}

		var (
			assistantText  string
			toolCalls      []llm.ToolCall
			stopReason     string
		)

		for ev := range events {
			switch ev.Type {
			case llm.EventThinkingDelta:
				out <- RunEvent{Type: "thinking_delta", Text: ev.Text}
			case llm.EventTextDelta:
				assistantText += ev.Text
				out <- RunEvent{Type: "text_delta", Text: ev.Text}
			case llm.EventToolCall:
				if ev.ToolCall != nil {
					toolCalls = append(toolCalls, *ev.ToolCall)
					out <- RunEvent{Type: "tool_call", ToolCall: ev.ToolCall}
				}
			case llm.EventStop:
				stopReason = ev.StopReason
			case llm.EventError:
				return ev.Err
			}
		}

		// 3. Append assistant turn to history
		assistantContent := buildAssistantContent(assistantText, toolCalls)
		r.history = append(r.history, llm.ChatMessage{
			Role:    "assistant",
			Content: assistantContent,
		})

		// 4. If no tool calls or stop reason is "end_turn", we're done
		if stopReason == "end_turn" || len(toolCalls) == 0 {
			out <- RunEvent{Type: "done"}
			return nil
		}

		// 5. Execute tools and append results
		toolResults := r.executeTools(ctx, toolCalls, out)
		toolResultContent, _ := json.Marshal(toolResults)
		r.history = append(r.history, llm.ChatMessage{
			Role:    "user",
			Content: toolResultContent,
		})
	}

	return fmt.Errorf("exceeded max iterations (%d)", maxIter)
}

// executeTools runs all tool calls in parallel and returns results.
func (r *Runner) executeTools(ctx context.Context, calls []llm.ToolCall, out chan<- RunEvent) []map[string]any {
	var results []map[string]any
	for _, tc := range calls {
		result, err := r.cfg.Tools.Execute(ctx, tc.Name, tc.Input)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
		}
		out <- RunEvent{Type: "tool_result", Text: result}
		results = append(results, map[string]any{
			"type":        "tool_result",
			"tool_use_id": tc.ID,
			"content":     result,
		})
	}
	return results
}

// buildAssistantContent constructs the assistant message content array.
func buildAssistantContent(text string, toolCalls []llm.ToolCall) json.RawMessage {
	var blocks []map[string]any
	if text != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	for _, tc := range toolCalls {
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": tc.Input,
		})
	}
	data, _ := json.Marshal(blocks)
	return data
}
