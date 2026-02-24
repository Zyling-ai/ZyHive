// pkg/llm/minimax.go — MiniMax 客户端
// OpenAI-compatible，支持 abab6.5s-chat / MiniMax-Text-01 等模型。
// API Key 为 Bearer JWT（eyJ... 格式）。
package llm

import "context"

const minimaxDefaultBase = "https://api.minimax.chat/v1"

// MinimaxClient implements Client for the MiniMax API.
type MinimaxClient struct {
	openAIBase
}

func NewMinimaxClient(baseURL string) *MinimaxClient {
	if baseURL == "" {
		baseURL = minimaxDefaultBase
	}
	return &MinimaxClient{openAIBase: newOpenAIBase(baseURL, nil)}
}

func (c *MinimaxClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
