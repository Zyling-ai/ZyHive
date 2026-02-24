// pkg/llm/openrouter.go — OpenRouter 客户端
// OpenAI-compatible 聚合路由，访问数百个模型。
// 需要额外 header：HTTP-Referer（标识来源）和 X-Title（应用名称）。
package llm

import "context"

const openrouterDefaultBase = "https://openrouter.ai/api/v1"

// OpenRouterClient implements Client for the OpenRouter API.
type OpenRouterClient struct {
	openAIBase
}

func NewOpenRouterClient(baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = openrouterDefaultBase
	}
	return &OpenRouterClient{
		openAIBase: newOpenAIBase(baseURL, map[string]string{
			"HTTP-Referer": "https://hive.zyling.ai",
			"X-Title":      "ZyHive",
		}),
	}
}

func (c *OpenRouterClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
