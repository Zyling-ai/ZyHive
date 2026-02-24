// pkg/llm/zhipu.go — 智谱 AI (GLM) 客户端
// OpenAI-compatible，支持 glm-4 / glm-4-flash / glm-4-air 等模型。
// API Key 格式为随机字符串，直接用于 Bearer 认证。
package llm

import "context"

const zhipuDefaultBase = "https://open.bigmodel.cn/api/paas/v4"

// ZhipuClient implements Client for the 智谱 AI (GLM) API.
type ZhipuClient struct {
	openAIBase
}

func NewZhipuClient(baseURL string) *ZhipuClient {
	if baseURL == "" {
		baseURL = zhipuDefaultBase
	}
	return &ZhipuClient{openAIBase: newOpenAIBase(baseURL, nil)}
}

func (c *ZhipuClient) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return c.stream(ctx, req)
}
