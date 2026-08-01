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

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/gin-gonic/gin"
)

type configHandler struct {
	cfg             *config.Config
	configPath      string
	activeGateway   config.GatewayConfig
	activeAuthToken string
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
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	safe := *snapshot
	configuredToken := safe.Auth.Token
	safe.Auth.Token = "***"
	maskedProviders := make([]config.ProviderEntry, len(safe.Providers))
	copy(maskedProviders, safe.Providers)
	for i := range maskedProviders {
		maskedProviders[i].APIKey = maskKey(maskedProviders[i].APIKey)
	}
	safe.Providers = maskedProviders
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
			if isSecretField(k) {
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
	data, err := json.Marshal(safe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pendingFields := make([]string, 0, 3)
	if h.activeGateway.Port != 0 && safe.Gateway.Port != h.activeGateway.Port {
		pendingFields = append(pendingFields, "gateway.port")
	}
	if h.activeGateway.Bind != "" && safe.Gateway.Bind != h.activeGateway.Bind {
		pendingFields = append(pendingFields, "gateway.bind")
	}
	if h.activeAuthToken != "" && configuredToken != h.activeAuthToken {
		pendingFields = append(pendingFields, "auth.token")
	}
	response["runtime"] = gin.H{
		"restartRequired": len(pendingFields) > 0,
		"pendingFields":   pendingFields,
		"activePort":      h.activeGateway.Port,
	}
	c.JSON(http.StatusOK, response)
}

func isSecretField(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "token") ||
		strings.Contains(lower, "key") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password")
}

// Patch PATCH /api/config — merge-patch config fields.
func (h *configHandler) Patch(c *gin.Context) {
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	err := config.Transaction(path, h.cfg, func(candidate *config.Config) error {
		current, err := json.Marshal(candidate)
		if err != nil {
			return fmt.Errorf("encode current config: %w", err)
		}
		var currentMap map[string]any
		if err := json.Unmarshal(current, &currentMap); err != nil {
			return fmt.Errorf("encode current config: %w", err)
		}
		mergeJSONObject(currentMap, patch)
		merged, err := json.Marshal(currentMap)
		if err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		var updated config.Config
		if err := json.Unmarshal(merged, &updated); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if _, err := tools.DecodeToolPolicy(updated.ToolPolicyRaw); err != nil {
			return fmt.Errorf("invalid toolPolicy: %w", err)
		}
		if err := updated.Gateway.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		for _, provider := range updated.Providers {
			if err := llm.ValidateProviderBaseURL(c.Request.Context(), provider.Provider, provider.BaseURL); err != nil {
				return fmt.Errorf("invalid provider baseUrl: %w", err)
			}
		}
		for i := range updated.Models {
			_, baseURL := config.ResolveCredentials(&updated.Models[i], updated.Providers)
			if err := llm.ValidateProviderBaseURL(c.Request.Context(), updated.Models[i].Provider, baseURL); err != nil {
				return fmt.Errorf("invalid model baseUrl: %w", err)
			}
		}
		*candidate = updated
		return nil
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "replace file") || strings.Contains(err.Error(), "write temp") ||
			strings.Contains(err.Error(), "sync ") || strings.Contains(err.Error(), "permission denied") {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": "save config: " + err.Error()})
		return
	}
	h.Get(c)
}

func mergeJSONObject(dst, patch map[string]any) {
	for key, patchValue := range patch {
		patchObject, patchIsObject := patchValue.(map[string]any)
		currentObject, currentIsObject := dst[key].(map[string]any)
		if patchIsObject && currentIsObject {
			mergeJSONObject(currentObject, patchObject)
			continue
		}
		dst[key] = patchValue
	}
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

	client, clientErr := llm.NewProviderHTTPClient("anthropic", baseURL, 15*time.Second)
	if clientErr != nil {
		return false, fmt.Sprintf("request blocked: %v", clientErr)
	}
	resp, err := client.Do(req)
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

	client, _ := llm.NewProviderHTTPClient("openai", "", 15*time.Second)
	resp, err := client.Do(req)
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

	client, _ := llm.NewProviderHTTPClient("deepseek", "", 15*time.Second)
	resp, err := client.Do(req)
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
func testOpenAICompatKey(provider, key, baseURL string) (bool, string) {
	if baseURL == "" {
		return false, "未配置调用地址"
	}
	baseURL = llm.NormalizeProviderBaseURL(provider, baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	client, clientErr := llm.NewProviderHTTPClient(provider, baseURL, 15*time.Second)
	if clientErr != nil {
		return false, fmt.Sprintf("request blocked: %v", clientErr)
	}
	resp, err := client.Do(req)
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

// testMiniMaxKey validates a MiniMax API key via a minimal chat completion request.
// MiniMax 不支持 GET /v1/models，用 POST /v1/chat/completions + max_tokens=1 探测。
func testMiniMaxKey(key, baseURL string) (bool, string) {
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	body := []byte(`{"model":"abab5.5s-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, clientErr := llm.NewProviderHTTPClient("minimax", baseURL, 15*time.Second)
	if clientErr != nil {
		return false, fmt.Sprintf("连接被阻止: %v", clientErr)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("连接失败: %v", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401:
		return false, "API Key 无效（401 Unauthorized）"
	case 403:
		return false, "API Key 权限不足（403 Forbidden）"
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return true, "MiniMax 连接成功"
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, string(b))
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
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return ""
	}
}
