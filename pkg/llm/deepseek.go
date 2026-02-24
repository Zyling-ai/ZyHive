// pkg/llm/deepseek.go — DeepSeek 客户端
// 特殊适配：deepseek-reasoner 模型的 delta.reasoning_content 字段
// 映射为 EventThinkingDelta（与 Anthropic extended-thinking 一致）。
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

const deepseekDefaultBase = "https://api.deepseek.com/v1"

// DeepSeekClient implements Client for the DeepSeek API.
type DeepSeekClient struct {
	openAIBase
}

func NewDeepSeekClient(baseURL string) *DeepSeekClient {
	if baseURL == "" {
		baseURL = deepseekDefaultBase
	}
	c := &DeepSeekClient{openAIBase: newOpenAIBase(baseURL, nil)}
	c.parseSSE = c.parseDeepSeekSSE
	return c
}

func (c *DeepSeekClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}

// parseDeepSeekSSE 在标准 OpenAI SSE 基础上额外处理 reasoning_content。
func (c *DeepSeekClient) parseDeepSeekSSE(ctx context.Context, body io.Reader, out chan<- StreamEvent) {
	type deepseekDelta struct {
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"` // deepseek-reasoner 专属
		ToolCalls        []struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	type deepseekChunk struct {
		Choices []struct {
			Delta        deepseekDelta `json:"delta"`
			FinishReason string        `json:"finish_reason"`
		} `json:"choices"`
	}

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

		var chunk deepseekChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// 推理内容（<think> 阶段）→ ThinkingDelta
		if delta.ReasoningContent != "" {
			out <- StreamEvent{Type: EventThinkingDelta, Text: delta.ReasoningContent}
		}
		// 正文
		if delta.Content != "" {
			out <- StreamEvent{Type: EventTextDelta, Text: delta.Content}
		}
		// 工具调用
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
