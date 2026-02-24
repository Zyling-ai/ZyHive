// pkg/llm/openai.go — OpenAI-compatible streaming client
// 适用于：openai / deepseek / 通义千问 / openrouter / custom 等兼容 OpenAI API 的服务
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

// 各 provider 的默认 base URL
var providerBaseURLs = map[string]string{
	"openai":     "https://api.openai.com/v1",
	"deepseek":   "https://api.deepseek.com/v1",
	"openrouter": "https://openrouter.ai/api/v1",
	"qwen":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
}

// OpenAIClient implements Client for the OpenAI-compatible Chat Completions API.
type OpenAIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOpenAIClient creates a client targeting the given baseURL.
// If baseURL is empty, falls back to the provider default or api.openai.com.
func NewOpenAIClient(provider, baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = providerBaseURLs[strings.ToLower(provider)]
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

// Stream sends a streaming chat/completions request and emits events.
func (c *OpenAIClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	body, err := buildOpenAIRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai api error: status %d: %s", resp.StatusCode, string(errBody))
	}

	events := make(chan StreamEvent, 32)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseOpenAISSE(ctx, resp.Body, events)
	}()

	return events, nil
}

// ── 请求构建 ─────────────────────────────────────────────────────────────────

func buildOpenAIRequest(req *ChatRequest) ([]byte, error) {
	// 只取 provider/model 里的 model 部分（去掉 "deepseek/" 前缀）
	model := req.Model
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}

	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.System,
		})
	}
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}

	// Tools（OpenAI 格式）
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			})
		}
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	return json.Marshal(payload)
}

// ── SSE 解析 ─────────────────────────────────────────────────────────────────

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func parseOpenAISSE(ctx context.Context, body io.Reader, out chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 128*1024), 128*1024)

	// 累积工具调用（按 index）
	type partialTool struct {
		id   string
		name string
		args string
	}
	toolMap := map[int]*partialTool{}

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
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// 发出所有未 flush 的工具调用
			for _, pt := range toolMap {
				if pt.name != "" {
					var input json.RawMessage
					if pt.args != "" {
						input = json.RawMessage(pt.args)
					} else {
						input = json.RawMessage("{}")
					}
					out <- StreamEvent{
						Type: EventToolCall,
						ToolCall: &ToolCall{
							ID:    pt.id,
							Name:  pt.name,
							Input: input,
						},
					}
				}
			}
			out <- StreamEvent{Type: EventStop}
			return
		}

		var chunk openAIChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// 文本 delta
		if delta.Content != "" {
			out <- StreamEvent{Type: EventTextDelta, Text: delta.Content}
		}

		// 工具调用 delta（流式累积）
		for _, tc := range delta.ToolCalls {
			pt, ok := toolMap[tc.Index]
			if !ok {
				pt = &partialTool{}
				toolMap[tc.Index] = pt
			}
			if tc.ID != "" {
				pt.id = tc.ID
			}
			if tc.Function.Name != "" {
				pt.name = tc.Function.Name
			}
			pt.args += tc.Function.Arguments
		}

		// finish_reason=tool_calls：工具调用完整，立即 flush
		if chunk.Choices[0].FinishReason == "tool_calls" {
			for _, pt := range toolMap {
				if pt.name != "" {
					var input json.RawMessage
					if pt.args != "" {
						input = json.RawMessage(pt.args)
					} else {
						input = json.RawMessage("{}")
					}
					out <- StreamEvent{
						Type: EventToolCall,
						ToolCall: &ToolCall{
							ID:    pt.id,
							Name:  pt.name,
							Input: input,
						},
					}
				}
			}
			toolMap = map[int]*partialTool{}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		out <- StreamEvent{Type: EventError, Err: err}
	}
}
