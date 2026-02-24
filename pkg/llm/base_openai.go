// pkg/llm/base_openai.go — OpenAI-compatible 基础实现
// 各 provider 客户端通过组合（embedding）复用此基础层，
// 可按需覆盖 streamHook 处理各家差异（如 DeepSeek reasoning_content）。
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

// openAIBase 是所有 OpenAI-compatible provider 共享的底层实现。
// 各 provider 客户端嵌入此结构体并可覆盖 parseSSE。
type openAIBase struct {
	baseURL      string
	extraHeaders map[string]string
	httpClient   *http.Client
	// parseSSE 是 SSE 流解析钩子，各 provider 可自定义（如 DeepSeek reasoning）。
	// nil = 使用默认实现。
	parseSSE func(ctx context.Context, body io.Reader, out chan<- StreamEvent)
}

func newOpenAIBase(baseURL string, extra map[string]string) openAIBase {
	return openAIBase{
		baseURL:      strings.TrimRight(baseURL, "/"),
		extraHeaders: extra,
		httpClient:   &http.Client{},
	}
}

// stream 是实际发起请求并返回事件 channel 的方法。
func (b *openAIBase) stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	body, err := buildOpenAIRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		b.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	for k, v := range b.extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai api error: status %d: %s", resp.StatusCode, string(errBody))
	}

	events := make(chan StreamEvent, 32)
	parseFn := defaultParseOpenAISSE
	if b.parseSSE != nil {
		parseFn = b.parseSSE
	}
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseFn(ctx, resp.Body, events)
	}()
	return events, nil
}

// ── 请求构建（共享）────────────────────────────────────────────────────────────

func buildOpenAIRequestBody(req *ChatRequest) ([]byte, error) {
	// 去掉 provider 前缀：deepseek/deepseek-chat → deepseek-chat
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

// ── 默认 SSE 解析（共享）──────────────────────────────────────────────────────

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

type partialTool struct {
	id   string
	name string
	args string
}

func defaultParseOpenAISSE(ctx context.Context, body io.Reader, out chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 128*1024), 128*1024)
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
			flushTools(toolMap, out)
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

		if delta.Content != "" {
			out <- StreamEvent{Type: EventTextDelta, Text: delta.Content}
		}
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
		if chunk.Choices[0].FinishReason == "tool_calls" {
			flushTools(toolMap, out)
			toolMap = map[int]*partialTool{}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		out <- StreamEvent{Type: EventError, Err: err}
	}
}

func flushTools(toolMap map[int]*partialTool, out chan<- StreamEvent) {
	for _, pt := range toolMap {
		if pt.name == "" {
			continue
		}
		input := json.RawMessage("{}")
		if pt.args != "" {
			input = json.RawMessage(pt.args)
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
