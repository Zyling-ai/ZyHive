// Anthropic LLM client — streams messages via the Anthropic Messages API.
// Reference: pi-ai/dist/providers/anthropic.js (streamAnthropic function)
//
// Key implementation notes from the reference:
//  - Uses @anthropic-ai/sdk under the hood; we replicate the HTTP layer directly.
//  - Supports prompt caching via cache_control blocks (ephemeral, 5m or 1h TTL).
//  - Tool names are normalised to Claude Code canonical casing (see claudeCodeTools list).
//  - SSE events to handle: content_block_start, content_block_delta, message_delta, message_stop.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicAPIBase = "https://api.anthropic.com/v1"
const anthropicVersion = "2023-06-01"

// AnthropicClient implements Client for the Anthropic Messages API.
type AnthropicClient struct {
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic streaming client.
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{httpClient: &http.Client{}}
}

// Stream sends a streaming Messages API request and emits events.
// Reference: anthropic.js → streamAnthropic → client.messages.stream()
func (c *AnthropicClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	body, err := buildAnthropicRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		anthropicAPIBase+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	// Always include extended cache TTL beta header (ref: openclaw anthropic provider)
	httpReq.Header.Set("anthropic-beta", "extended-cache-ttl-2025-04-11")
	for _, h := range req.BetaHeaders {
		existing := httpReq.Header.Get("anthropic-beta")
		httpReq.Header.Set("anthropic-beta", existing+","+h)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("anthropic api error: status %d: %s", resp.StatusCode, string(errBody))
	}

	events := make(chan StreamEvent, 32)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseAnthropicSSE(ctx, resp.Body, events)
	}()

	return events, nil
}

// buildAnthropicRequest converts our generic ChatRequest to Anthropic JSON.
func buildAnthropicRequest(req *ChatRequest) ([]byte, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8096
	}

	payload := map[string]any{
		"model":      normaliseAnthropicModel(req.Model),
		"max_tokens": maxTokens,
		"stream":     true,
		"messages":   req.Messages,
	}
	if req.System != "" {
		payload["system"] = req.System
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}
	return json.Marshal(payload)
}

// normaliseAnthropicModel strips the "anthropic/" provider prefix.
func normaliseAnthropicModel(model string) string {
	return strings.TrimPrefix(model, "anthropic/")
}

// parseAnthropicSSE reads the SSE stream and sends events to the channel.
// Reference: anthropic.js → anthropicStream event handlers
//
// SSE event flow:
//   message_start           → (ignore, metadata)
//   content_block_start     → text block or tool_use block begins
//   content_block_delta     → text_delta or input_json_delta
//   content_block_stop      → block complete
//   message_delta           → stop_reason + usage
//   message_stop            → stream end
func parseAnthropicSSE(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)

	var (
		currentBlockType string // "text" | "tool_use"
		currentToolID    string
		currentToolName  string
		toolInputBuf     strings.Builder
	)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			return
		}

		var event struct {
			Type  string          `json:"type"`
			Index int             `json:"index"`
			Delta json.RawMessage `json:"delta"`
			Usage json.RawMessage `json:"usage"`
			// content_block_start
			ContentBlock struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Name  string `json:"name"`
			} `json:"content_block"`
			// error event
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			currentBlockType = event.ContentBlock.Type
			if currentBlockType == "tool_use" {
				currentToolID = event.ContentBlock.ID
				currentToolName = event.ContentBlock.Name
				toolInputBuf.Reset()
			}

		case "content_block_delta":
			var delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
			}
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			switch delta.Type {
			case "text_delta":
				events <- StreamEvent{Type: EventTextDelta, Text: delta.Text}
			case "thinking_delta":
				events <- StreamEvent{Type: EventThinkingDelta, Text: delta.Thinking}
			case "input_json_delta":
				toolInputBuf.WriteString(delta.PartialJSON)
				events <- StreamEvent{Type: EventToolDelta, ToolDelta: delta.PartialJSON}
			}

		case "content_block_stop":
			if currentBlockType == "tool_use" {
				events <- StreamEvent{
					Type: EventToolCall,
					ToolCall: &ToolCall{
						ID:    currentToolID,
						Name:  currentToolName,
						Input: json.RawMessage(toolInputBuf.String()),
					},
				}
			}
			currentBlockType = ""

		case "message_delta":
			var delta struct {
				StopReason string `json:"stop_reason"`
				Usage      struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			events <- StreamEvent{Type: EventStop, StopReason: delta.StopReason}

		case "message_stop":
			return

		case "error":
			msg := event.Error.Message
			if msg == "" {
				msg = event.Error.Type
			}
			if msg == "" {
				msg = "unknown Anthropic error"
			}
			events <- StreamEvent{Type: EventError, Err: fmt.Errorf("anthropic: %s", msg)}
			return
		}
	}
}
