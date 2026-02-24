// pkg/llm/qwen.go — 通义千问 (阿里云 DashScope) 客户端
// 使用 DashScope OpenAI-compatible 模式，支持 qwen-turbo / qwen-plus / qwen-max 等。
// 注意：DashScope compatible 模式需要传递 `stream_options: {"include_usage": true}` 以获取 usage。
package llm

import (
	"context"
	"encoding/json"
	"strings"
)

const qwenDefaultBase = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// QwenClient implements Client for the 阿里云通义千问 API (DashScope compatible mode).
type QwenClient struct {
	openAIBase
}

func NewQwenClient(baseURL string) *QwenClient {
	if baseURL == "" {
		baseURL = qwenDefaultBase
	}
	c := &QwenClient{openAIBase: newOpenAIBase(baseURL, nil)}
	return c
}

func (c *QwenClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	// DashScope compatible 模式与标准 OpenAI 完全兼容，直接复用
	return c.stream(ctx, req)
}

// buildQwenRequest 在标准 OpenAI 请求基础上添加 DashScope 特有参数。
// 目前 DashScope compatible 模式完全兼容标准格式，无需额外处理。
func buildQwenRequest(req *ChatRequest) ([]byte, error) {
	payload, err := buildOpenAIRequestBody(req)
	if err != nil {
		return nil, err
	}

	// 解析后添加 stream_options（DashScope 推荐参数）
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return payload, nil // 解析失败则返回原始请求
	}

	// 确保模型 ID 无前缀（qwen/ → qwen-turbo）
	if model, ok := m["model"].(string); ok {
		if idx := strings.LastIndex(model, "/"); idx >= 0 {
			m["model"] = model[idx+1:]
		}
	}

	return json.Marshal(m)
}
