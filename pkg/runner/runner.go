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
	"sort"
	"strings"

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
	SessionID    string // persistent session ID; if set, history is loaded from/saved to JSONL
	LLM          llm.Client
	Tools        *tools.Registry
	Session      *session.Store
	// Optional: shared project list injected into the system prompt
	ProjectContext string
	// Optional: extra context injected before the user message (e.g. page context, scenario)
	ExtraContext string
	// Optional: base64 image data URIs attached to the user message
	Images []string
	// Optional: preloaded conversation history (from client-side state, used when SessionID is empty)
	PreloadedHistory []llm.ChatMessage
	// Optional: per-agent env vars — tells the agent which credentials/env vars are available
	AgentEnv map[string]string
}

// Runner drives a single agent's conversation lifecycle.
type Runner struct {
	cfg     Config
	history []llm.ChatMessage
}

// New creates a Runner for the given agent.
// If cfg.SessionID is set, history is loaded from the session store.
// Otherwise, cfg.PreloadedHistory is used (legacy client-side history).
func New(cfg Config) *Runner {
	r := &Runner{cfg: cfg}

	// Load server-side session history (preferred)
	if cfg.SessionID != "" && cfg.Session != nil {
		msgs, summary, err := cfg.Session.ReadHistory(cfg.SessionID)
		if err == nil && len(msgs) > 0 {
			if summary != "" {
				// Prepend compaction summary as a system-style assistant message
				summaryJSON, _ := json.Marshal("[Previous conversation summary]\n" + summary)
				r.history = append(r.history, llm.ChatMessage{Role: "user", Content: summaryJSON})
				ackJSON, _ := json.Marshal("Understood. I have the context from the previous conversation.")
				r.history = append(r.history, llm.ChatMessage{Role: "assistant", Content: ackJSON})
			}
			for _, m := range msgs {
				r.history = append(r.history, llm.ChatMessage{Role: m.Role, Content: m.Content})
			}
			// Sanitize: remove consecutive same-role messages to prevent Anthropic 400 errors.
			// This can happen when concurrent requests or errors leave orphaned user messages.
			r.history = sanitizeHistory(r.history)
			return r
		}
	}

	// Fallback: client-supplied history
	if len(cfg.PreloadedHistory) > 0 {
		r.history = append(r.history, cfg.PreloadedHistory...)
	}
	return r
}

// RunEvent is emitted to the caller during a conversation turn.
type RunEvent struct {
	Type          string // "text_delta" | "tool_call" | "tool_result" | "error" | "done"
	Text          string
	ToolCall      *llm.ToolCall
	Error         error
	// Done event extras
	SessionID     string
	TokenEstimate int
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

// sanitizeHistory fixes Anthropic-incompatible conversation history.
// Problems handled:
//  1. Consecutive same-role messages (user→user or assistant→assistant)
//  2. Orphaned tool_use in assistant messages (next user msg has no tool_result)
//  3. Orphaned tool_result in user messages (preceding assistant has no matching tool_use)
func sanitizeHistory(msgs []llm.ChatMessage) []llm.ChatMessage {
	if len(msgs) == 0 {
		return msgs
	}
	// Pass 1: deduplicate consecutive same-role messages
	deduped := make([]llm.ChatMessage, 0, len(msgs))
	for _, msg := range msgs {
		if len(deduped) == 0 {
			deduped = append(deduped, msg)
			continue
		}
		last := &deduped[len(deduped)-1]
		if msg.Role == last.Role {
			if msg.Role == "user" {
				*last = msg // keep latest user message
			}
			// For assistant: keep the first (skip duplicates)
		} else {
			deduped = append(deduped, msg)
		}
	}

	// Pass 1b: sanitise empty text blocks in any message content
	// (handles stale session data written before this fix was applied)
	for i, m := range deduped {
		if len(m.Content) > 0 && m.Content[0] == '[' {
			deduped[i].Content = sanitizeContentBlocksInHistory(m.Content)
		}
	}

	// Pass 2: fix orphaned tool_use / tool_result pairs
	result := make([]llm.ChatMessage, 0, len(deduped))
	for i, msg := range deduped {
		switch msg.Role {
		case "assistant":
			// Check if this assistant message has tool_use blocks
			toolIDs := extractToolUseIDs(msg.Content)
			if len(toolIDs) == 0 {
				result = append(result, msg)
				continue
			}
			// Check the NEXT message — it must be a user message with matching tool_result
			hasNextToolResult := false
			if i+1 < len(deduped) && deduped[i+1].Role == "user" {
				resultIDs := extractToolResultIDs(deduped[i+1].Content)
				hasNextToolResult = setsOverlap(toolIDs, resultIDs)
			}
			if hasNextToolResult {
				result = append(result, msg)
			} else {
				// Strip tool_use blocks, keep only text
				stripped := stripToolUseBlocks(msg.Content)
				if stripped != nil {
					result = append(result, llm.ChatMessage{Role: "assistant", Content: stripped})
				}
				// If stripped is nil (nothing left), skip this message entirely
			}
		case "user":
			// Check if this user message contains tool_result blocks
			resultIDs := extractToolResultIDs(msg.Content)
			if len(resultIDs) == 0 {
				result = append(result, msg)
				continue
			}
			// Verify the preceding assistant message has matching tool_use
			if len(result) > 0 && result[len(result)-1].Role == "assistant" {
				prevToolIDs := extractToolUseIDs(result[len(result)-1].Content)
				if setsOverlap(prevToolIDs, resultIDs) {
					result = append(result, msg)
					continue
				}
			}
			// Orphaned tool_result — strip tool_result blocks, keep plain text
			stripped := stripToolResultBlocks(msg.Content)
			if stripped != nil {
				result = append(result, llm.ChatMessage{Role: "user", Content: stripped})
			}
			// If stripped is nil, skip this message entirely
		default:
			result = append(result, msg)
		}
	}
	return result
}

// sanitizeContentBlocksInHistory replaces empty text blocks with a space,
// preventing Anthropic "text content blocks must be non-empty" errors
// when session history was written with empty text content.
func sanitizeContentBlocksInHistory(raw json.RawMessage) json.RawMessage {
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw
	}
	changed := false
	for i, b := range blocks {
		var probe struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &probe); err != nil {
			continue
		}
		if probe.Type == "text" && strings.TrimSpace(probe.Text) == "" {
			if nb, err := json.Marshal(map[string]any{"type": "text", "text": "."}); err == nil {
				blocks[i] = nb
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(blocks)
	if err != nil {
		return raw
	}
	return out
}

// extractToolUseIDs returns the set of tool_use IDs in a message content block.
func extractToolUseIDs(raw json.RawMessage) map[string]bool {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	ids := map[string]bool{}
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			ids[b.ID] = true
		}
	}
	return ids
}

// extractToolResultIDs returns the set of tool_use_ids referenced in tool_result blocks.
func extractToolResultIDs(raw json.RawMessage) map[string]bool {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	ids := map[string]bool{}
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			ids[b.ToolUseID] = true
		}
	}
	return ids
}

// setsOverlap returns true if both sets share at least one element.
func setsOverlap(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// stripToolUseBlocks removes tool_use blocks from content, keeping only text blocks.
// Returns nil if nothing remains.
func stripToolUseBlocks(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || raw[0] != '[' {
		return raw
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw
	}
	kept := blocks[:0]
	for _, b := range blocks {
		var probe struct{ Type string `json:"type"` }
		if json.Unmarshal(b, &probe) == nil && probe.Type != "tool_use" {
			kept = append(kept, b)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	out, _ := json.Marshal(kept)
	return out
}

// stripToolResultBlocks removes tool_result blocks, keeping plain text/image blocks.
// Returns nil if nothing remains.
func stripToolResultBlocks(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || raw[0] != '[' {
		// Plain string user message — keep as-is
		return raw
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return raw
	}
	kept := blocks[:0]
	for _, b := range blocks {
		var probe struct{ Type string `json:"type"` }
		if json.Unmarshal(b, &probe) == nil && probe.Type != "tool_result" {
			kept = append(kept, b)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	out, _ := json.Marshal(kept)
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
	// If history ends with a "user" message (orphaned from a failed turn),
	// replace it in-memory so we don't send consecutive user messages to the LLM.
	if len(r.history) > 0 && r.history[len(r.history)-1].Role == "user" {
		r.history[len(r.history)-1] = llm.ChatMessage{Role: "user", Content: userContent}
	} else {
		r.history = append(r.history, llm.ChatMessage{Role: "user", Content: userContent})
	}

	// Persist user message to session (server-side history)
	if r.cfg.SessionID != "" && r.cfg.Session != nil {
		_ = r.cfg.Session.AppendMessage(r.cfg.SessionID, "user", userContent)
	}

	// 2. Agentic loop — call LLM, handle tools, repeat
	const maxIter = 30
	for i := 0; i < maxIter; i++ {
		// Build system prompt from workspace identity files
		systemPrompt, _ := BuildSystemPrompt(r.cfg.WorkspaceDir)
		// Inject shared project workspace context
		if r.cfg.ProjectContext != "" {
			systemPrompt = systemPrompt + "\n\n" + r.cfg.ProjectContext
		}
		if r.cfg.ExtraContext != "" {
			systemPrompt = systemPrompt + "\n\n---\n" + r.cfg.ExtraContext
		}
		// Inject env vars hint so agent knows which credentials are configured
		if len(r.cfg.AgentEnv) > 0 {
			keys := make([]string, 0, len(r.cfg.AgentEnv))
			for k := range r.cfg.AgentEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			systemPrompt = systemPrompt + "\n\n## 可用环境变量\n" +
				"以下环境变量已配置，exec 工具运行时自动可用（无需手动导出）：\n" +
				"- " + strings.Join(keys, "\n- ") + "\n"
		}
		// Inject runtime metadata so the agent knows what model/context it's running in
		systemPrompt = systemPrompt + fmt.Sprintf(
			"\n\n## Runtime\nModel: %s | Agent: %s | Workspace: %s",
			r.cfg.Model, r.cfg.AgentID, r.cfg.WorkspaceDir,
		)

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
			// Only persist if we have actual content (avoid saving null/empty assistant turns
			// that would corrupt the session history and cause Anthropic 400 errors later).
			if r.cfg.SessionID != "" && r.cfg.Session != nil && strings.TrimSpace(assistantText) != "" {
				// When saving, strip tool_use blocks from the final assistant message.
				// If the final turn (unexpectedly) had both text and tool_use, saving the
				// tool_use without corresponding tool_result would corrupt the session.
				safeContent := stripToolUseBlocks(assistantContent)
				if safeContent == nil {
					safeContent = assistantContent
				}
				_ = r.cfg.Session.AppendMessage(r.cfg.SessionID, "assistant", safeContent)
			}
			tokenEstimate := 0
			if r.cfg.Session != nil {
				tokenEstimate = r.cfg.Session.EstimateTokens(r.cfg.SessionID)
			}
			out <- RunEvent{
				Type:          "done",
				SessionID:     r.cfg.SessionID,
				TokenEstimate: tokenEstimate,
			}
			// Trigger compaction asynchronously if token budget exceeded
			if r.cfg.SessionID != "" && r.cfg.Session != nil {
				session.CompactIfNeeded(r.cfg.Session, r.cfg.SessionID, r.makeSimpleLLMCaller())
			}
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

// makeSimpleLLMCaller returns a function suitable for compaction summarization.
// It calls the LLM non-streamingly and returns the full response text.
func (r *Runner) makeSimpleLLMCaller() func(ctx context.Context, system, userMsg string) (string, error) {
	return func(ctx context.Context, system, userMsg string) (string, error) {
		userContent, _ := json.Marshal(userMsg)
		req := &llm.ChatRequest{
			Model:  r.cfg.Model,
			APIKey: r.cfg.APIKey,
			System: system,
			Messages: []llm.ChatMessage{
				{Role: "user", Content: userContent},
			},
		}
		events, err := r.cfg.LLM.Stream(ctx, req)
		if err != nil {
			return "", err
		}
		var text string
		for ev := range events {
			if ev.Type == llm.EventTextDelta {
				text += ev.Text
			}
			if ev.Type == llm.EventError {
				return text, ev.Err
			}
		}
		return text, nil
	}
}

// buildAssistantContent constructs the assistant message content array.
// Guarantees at least one valid block (Anthropic rejects empty arrays and
// empty text blocks).
func buildAssistantContent(text string, toolCalls []llm.ToolCall) json.RawMessage {
	blocks := make([]map[string]any, 0, 1+len(toolCalls))
	if strings.TrimSpace(text) != "" {
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
	// Guard: Anthropic rejects empty content arrays and empty/whitespace text blocks
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": "."})
	}
	data, _ := json.Marshal(blocks)
	return data
}
