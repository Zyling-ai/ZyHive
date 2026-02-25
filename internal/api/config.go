// Config handler — read/write aipanel.json via API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/config"
)

type configHandler struct {
	cfg        *config.Config
	configPath string
}

// maskKey shows first 8 chars + "***" for API keys.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "***"
}

// Get GET /api/config — return current config with masked keys.
func (h *configHandler) Get(c *gin.Context) {
	safe := *h.cfg
	safe.Auth.Token = "***"
	// Mask model API keys
	maskedModels := make([]config.ModelEntry, len(safe.Models))
	copy(maskedModels, safe.Models)
	for i := range maskedModels {
		maskedModels[i].APIKey = maskKey(maskedModels[i].APIKey)
	}
	safe.Models = maskedModels
	// Mask channel secrets
	maskedChannels := make([]config.ChannelEntry, len(safe.Channels))
	copy(maskedChannels, safe.Channels)
	for i := range maskedChannels {
		mc := make(map[string]string)
		for k, v := range maskedChannels[i].Config {
			if strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "key") {
				mc[k] = maskKey(v)
			} else {
				mc[k] = v
			}
		}
		maskedChannels[i].Config = mc
	}
	safe.Channels = maskedChannels
	// Mask tool API keys
	maskedTools := make([]config.ToolEntry, len(safe.Tools))
	copy(maskedTools, safe.Tools)
	for i := range maskedTools {
		maskedTools[i].APIKey = maskKey(maskedTools[i].APIKey)
	}
	safe.Tools = maskedTools
	c.JSON(http.StatusOK, safe)
}

// Patch PATCH /api/config — merge-patch config fields.
func (h *configHandler) Patch(c *gin.Context) {
	var patch map[string]json.RawMessage
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	current, _ := json.Marshal(h.cfg)
	var currentMap map[string]json.RawMessage
	json.Unmarshal(current, &currentMap)

	for k, v := range patch {
		currentMap[k] = v
	}

	merged, _ := json.Marshal(currentMap)
	var updated config.Config
	if err := json.Unmarshal(merged, &updated); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config: " + err.Error()})
		return
	}

	if _, hasAuth := patch["auth"]; !hasAuth {
		updated.Auth = h.cfg.Auth
	}

	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	if err := config.Save(path, &updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	*h.cfg = updated
	h.Get(c)
}

// TestKey POST /api/config/test-key — validate an API key.
func (h *configHandler) TestKey(c *gin.Context) {
	var req struct {
		Provider string `json:"provider" binding:"required"`
		Key      string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var valid bool
	var errMsg string

	switch strings.ToLower(req.Provider) {
	case "anthropic":
		valid, errMsg = testAnthropicKey(req.Key, "") // 无 model baseURL，用默认地址
	case "openai":
		valid, errMsg = testOpenAIKey(req.Key)
	case "deepseek":
		valid, errMsg = testDeepSeekKey(req.Key)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider: " + req.Provider})
		return
	}

	result := gin.H{"valid": valid}
	if errMsg != "" {
		result["error"] = errMsg
	}
	c.JSON(http.StatusOK, result)
}

func testAnthropicKey(key, baseURL string) (bool, string) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.Contains(baseURL, "/v1/") {
		baseURL += "/v1"
	}
	payload := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBody))
	if resp.StatusCode == 403 {
		msg = "403 地区限制（当前 IP 被 Anthropic 屏蔽），请配置转发地址或切换到其他模型"
	}
	return false, msg
}

func testOpenAIKey(key string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBody))
}

func testDeepSeekKey(key string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.deepseek.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBody))
}

// testOpenAICompatKey tests any OpenAI-compatible provider by calling /models.
func testOpenAICompatKey(key, baseURL string) (bool, string) {
	if baseURL == "" {
		return false, "未配置调用地址"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBody))
}

// defaultBaseURLForProvider returns the default API base URL for a known provider.
func defaultBaseURLForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "moonshot", "kimi":
		return "https://api.moonshot.cn/v1"
	case "zhipu", "glm":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "minimax":
		return "https://api.minimax.chat/v1"
	case "qwen", "dashscope":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}
