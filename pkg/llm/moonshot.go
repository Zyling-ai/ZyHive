// pkg/llm/moonshot.go — Kimi (月之暗面) 客户端
// OpenAI-compatible，支持 moonshot-v1-8k / 32k / 128k 等模型。
package llm

import "context"

const moonshotDefaultBase = "https://api.moonshot.cn/v1"

// MoonshotClient implements Client for the Moonshot (Kimi) API.
type MoonshotClient struct {
	openAIBase
}

func NewMoonshotClient(baseURL string) *MoonshotClient {
	if baseURL == "" {
		baseURL = moonshotDefaultBase
	}
	return &MoonshotClient{openAIBase: newOpenAIBase(baseURL, nil)}
}

func (c *MoonshotClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
