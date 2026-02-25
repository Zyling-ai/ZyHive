// Package llm provides LLM client abstractions.
// Reference: pi-ai/dist/providers/anthropic.js, openai-responses.js
package llm

import (
	"context"
	"encoding/json"
	"strings"
)

// ---- Request types --------------------------------------------------------

// ChatRequest is the provider-agnostic request format.
type ChatRequest struct {
	Model     string         `json:"model"`
	System    string         `json:"system,omitempty"`
	Messages  []ChatMessage  `json:"messages"`
	Tools     []ToolDef      `json:"tools,omitempty"`
	MaxTokens int            `json:"max_tokens,omitempty"`
	APIKey    string         `json:"-"`
	// Anthropic-specific options
	CacheRetention string `json:"-"` // "none" | "short" | "long"
	// Extra beta headers
	BetaHeaders []string `json:"-"`
}

// ChatMessage is one turn in the conversation history.
type ChatMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"`
}

// ToolDef describes a tool the model can invoke.
// Reference: anthropic.js claudeCodeTools section
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolCall is a single tool invocation returned by the model.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult is the output of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// ---- Response / stream types ----------------------------------------------

// StreamEventType discriminates streaming events.
type StreamEventType string

const (
	EventStart         StreamEventType = "start"
	EventTextDelta     StreamEventType = "text_delta"
	EventThinkingDelta StreamEventType = "thinking_delta"
	EventToolCall      StreamEventType = "tool_call"
	EventToolDelta     StreamEventType = "tool_delta"
	EventUsage         StreamEventType = "usage"
	EventStop          StreamEventType = "stop"
	EventError         StreamEventType = "error"
)

// StreamEvent is one item emitted by the streaming LLM response.
type StreamEvent struct {
	Type StreamEventType `json:"type"`
	// text_delta
	Text string `json:"text,omitempty"`
	// tool_call (complete) / tool_delta (partial input JSON)
	ToolCall  *ToolCall `json:"tool_call,omitempty"`
	ToolDelta string    `json:"tool_delta,omitempty"`
	// usage (emitted at message_stop)
	Usage *Usage `json:"usage,omitempty"`
	// stop
	StopReason string `json:"stop_reason,omitempty"`
	// error
	Err error `json:"-"`
}

// Usage holds token counts for a single API call.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// ---- Client interface -----------------------------------------------------

// Client is the provider-agnostic LLM streaming interface.
type Client interface {
	// Stream sends a request and returns a channel of events.
	// The channel is closed when the response is complete or an error occurs.
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
}

// NewClient 根据 provider 返回对应的专用客户端。
// baseURL 为空时使用各 provider 默认地址。
func NewClient(provider, baseURL string) Client {
	switch strings.ToLower(provider) {
	case "anthropic":
		return NewAnthropicClient(baseURL)
	case "openai":
		return NewOpenAIClient(baseURL)
	case "deepseek":
		return NewDeepSeekClient(baseURL)
	case "moonshot", "kimi":
		return NewMoonshotClient(baseURL)
	case "zhipu", "glm":
		return NewZhipuClient(baseURL)
	case "minimax":
		return NewMinimaxClient(baseURL)
	case "qwen", "dashscope":
		return NewQwenClient(baseURL)
	case "openrouter":
		return NewOpenRouterClient(baseURL)
	default:
		// 自定义或未知 provider → 通用 OpenAI-compatible 客户端
		return NewCustomClient(baseURL)
	}
}
