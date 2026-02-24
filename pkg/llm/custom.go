// pkg/llm/custom.go — 自定义 OpenAI-compatible 接口客户端
// 适用于任何自托管或第三方 OpenAI-compatible 服务。
package llm

import "context"

// CustomClient implements Client for any OpenAI-compatible custom endpoint.
type CustomClient struct {
	openAIBase
}

func NewCustomClient(baseURL string) *CustomClient {
	return &CustomClient{openAIBase: newOpenAIBase(baseURL, nil)}
}

func (c *CustomClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
