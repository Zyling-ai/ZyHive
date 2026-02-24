// pkg/llm/openai.go — OpenAI 客户端
package llm

import "context"

const openAIDefaultBase = "https://api.openai.com/v1"

// OpenAIClient implements Client for the OpenAI Chat Completions API.
type OpenAIClient struct {
	openAIBase
}

func NewOpenAIClient(baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = openAIDefaultBase
	}
	return &OpenAIClient{openAIBase: newOpenAIBase(baseURL, nil)}
}

func (c *OpenAIClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
