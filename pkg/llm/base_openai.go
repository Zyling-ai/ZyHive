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

	messages := make([]map[string]any, 0, len(req.Messages)+2)
	if req.System != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.System,
		})
	}
	for _, m := range req.Messages {
		converted := convertAnthropicMessageToOpenAI(m)
		messages = append(messages, converted...)
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

// ── Anthropic → OpenAI 格式消息转换 ────────────────────────────────────────
// Anthropic 格式的 Content 是 json.RawMessage，可能是：
//   - 字符串 "hello"
//   - 文本块数组 [{"type":"text","text":"..."}]
//   - 工具调用块（assistant）[{"type":"tool_use","id":"...","name":"...","input":{}}]
//   - 工具结果块（user）[{"type":"tool_result","tool_use_id":"...","content":"..."}]
// OpenAI 格式要求：
//   - tool_use → assistant 消息中的 tool_calls 数组
//   - tool_result → role:"tool" 独立消息（每个结果一条）

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// convertAnthropicMessageToOpenAI 将单条 Anthropic 格式消息转为 0~N 条 OpenAI 格式消息。
// 一条 Anthropic user 消息可能包含多个 tool_result，需拆分成多条 role:"tool" 消息。
func convertAnthropicMessageToOpenAI(m ChatMessage) []map[string]any {
	raw := m.Content

	// 尝试解析为数组
	var blocks []anthropicBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		// 不是数组（可能是字符串），直接使用原始内容
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			return []map[string]any{{"role": m.Role, "content": s}}
		}
		// 无法识别，原样发送
		return []map[string]any{{"role": m.Role, "content": raw}}
	}

	// 分析 blocks 类型
	var textParts []string
	var toolCalls []map[string]any
	var toolResults []map[string]any

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case "tool_use":
			// assistant tool call → OpenAI tool_calls 格式
			argsStr := "{}"
			if len(b.Input) > 0 {
				argsStr = string(b.Input)
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   b.ID,
				"type": "function",
				"function": map[string]any{
					"name":      b.Name,
					"arguments": argsStr,
				},
			})
		case "tool_result":
			// user tool result → OpenAI role:"tool" 消息
			var resultContent string
			// content 可能是字符串或 [{type:text,text:...}]
			if len(b.Content) > 0 {
				var s string
				if err := json.Unmarshal(b.Content, &s); err == nil {
					resultContent = s
				} else {
					var innerBlocks []anthropicBlock
					if err2 := json.Unmarshal(b.Content, &innerBlocks); err2 == nil {
						for _, ib := range innerBlocks {
							if ib.Type == "text" {
								resultContent += ib.Text
							}
						}
					} else {
						resultContent = string(b.Content)
					}
				}
			}
			toolResults = append(toolResults, map[string]any{
				"role":         "tool",
				"tool_call_id": b.ToolUseID,
				"content":      resultContent,
			})
		}
	}

	// 如果有 tool_result，拆成独立消息（忽略 role，直接用 "tool"）
	if len(toolResults) > 0 {
		return toolResults
	}

	// 如果有 tool_use，构建 assistant 消息（带 tool_calls）
	if len(toolCalls) > 0 {
		// 注意：content 必须用空字符串而非 null。
		// 部分 provider（如 MiniMax）不接受 content:null，会导致该消息被忽略，
		// 后续 role:"tool" 消息找不到前驱 tool_calls，触发 400 错误。
		content := strings.Join(textParts, "")
		return []map[string]any{{
			"role":       "assistant",
			"content":    content,
			"tool_calls": toolCalls,
		}}
	}

	// 纯文本
	content := strings.Join(textParts, "")
	if content == "" {
		// 空消息，原样发送（可能是 null 或空字符串）
		return []map[string]any{{"role": m.Role, "content": ""}}
	}
	return []map[string]any{{"role": m.Role, "content": content}}
}
