// internal/api/providers.go — Provider API key CRUD endpoints
// GET/POST/PUT/DELETE /api/providers
// POST /api/providers/:id/test
package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/config"
)

type providerHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/providers
func (h *providerHandler) List(c *gin.Context) {
	providers := h.cfg.Providers
	if providers == nil {
		providers = []config.ProviderEntry{}
	}
	// 返回时脱敏 apiKey
	type ProviderResp struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`   // 脱敏
		BaseURL  string `json:"baseUrl"`
		Status   string `json:"status"`
		ModelCount int  `json:"modelCount"` // 引用此 provider 的模型数量
	}
	resp := make([]ProviderResp, 0, len(providers))
	for _, p := range providers {
		masked := maskKey(p.APIKey)
		cnt := 0
		for _, m := range h.cfg.Models {
			if m.ProviderID == p.ID {
				cnt++
			}
		}
		resp = append(resp, ProviderResp{
			ID: p.ID, Name: p.Name, Provider: p.Provider,
			APIKey: masked, BaseURL: p.BaseURL, Status: p.Status,
			ModelCount: cnt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"providers": resp})
}

// Create POST /api/providers
func (h *providerHandler) Create(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.Provider = strings.TrimSpace(body.Provider)
	body.APIKey   = strings.TrimSpace(body.APIKey)
	body.Name     = strings.TrimSpace(body.Name)
	if body.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
		return
	}
	if body.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "apiKey is required"})
		return
	}
	if body.Name == "" {
		body.Name = providerDisplayName(body.Provider)
	}

	entry := config.ProviderEntry{
		ID:       config.RandID(),
		Name:     body.Name,
		Provider: body.Provider,
		APIKey:   body.APIKey,
		BaseURL:  strings.TrimRight(body.BaseURL, "/"),
		Status:   "untested",
	}
	h.cfg.Providers = append(h.cfg.Providers, entry)
	if err := config.Save(h.configPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"provider": entry})
}

// Update PUT /api/providers/:id
func (h *providerHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var p *config.ProviderEntry
	for i := range h.cfg.Providers {
		if h.cfg.Providers[i].ID == id {
			p = &h.cfg.Providers[i]
			break
		}
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	var body struct {
		Name    *string `json:"name"`
		APIKey  *string `json:"apiKey"`
		BaseURL *string `json:"baseUrl"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.APIKey != nil && strings.TrimSpace(*body.APIKey) != "" && !ismasked(*body.APIKey) {
		p.APIKey = strings.TrimSpace(*body.APIKey)
		p.Status = "untested" // key 变了，需要重新测试
	}
	if body.BaseURL != nil {
		p.BaseURL = strings.TrimRight(strings.TrimSpace(*body.BaseURL), "/")
	}
	if err := config.Save(h.configPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"provider": p})
}

// Delete DELETE /api/providers/:id
func (h *providerHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	found := false
	newList := h.cfg.Providers[:0]
	for _, p := range h.cfg.Providers {
		if p.ID == id {
			found = true
			continue
		}
		newList = append(newList, p)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	// 检查是否有模型引用此 provider
	refCount := 0
	for _, m := range h.cfg.Models {
		if m.ProviderID == id {
			refCount++
		}
	}
	if refCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("该 API Key 被 %d 个模型使用，请先删除或重新分配这些模型", refCount),
		})
		return
	}
	h.cfg.Providers = newList
	if err := config.Save(h.configPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/providers/:id/test
func (h *providerHandler) Test(c *gin.Context) {
	id := c.Param("id")
	var p *config.ProviderEntry
	for i := range h.cfg.Providers {
		if h.cfg.Providers[i].ID == id {
			p = &h.cfg.Providers[i]
			break
		}
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	apiKey := p.APIKey
	if apiKey == "" {
		if envVar, ok := envVarForProvider[p.Provider]; ok {
			apiKey = os.Getenv(envVar)
		}
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no API key configured"})
		return
	}

	baseURL := p.BaseURL

	// 使用轻量的 /v1/models 探测接口
	status := "ok"
	msg := "连接成功"

	switch p.Provider {
	case "anthropic":
		ok, msg2 := testAnthropicKey(apiKey, baseURL)
		if !ok {
			status = "error"
			msg = msg2
		} else {
			msg = msg2
		}
	default:
		ok, msg2 := testOpenAICompatKey(apiKey, baseURL)
		if !ok {
			status = "error"
			msg = msg2
		} else {
			msg = msg2
		}
	}

	// 更新状态
	p.Status = status
	_ = config.Save(h.configPath, h.cfg)

	// 同步更新所有引用此 provider 的模型状态
	for i := range h.cfg.Models {
		if h.cfg.Models[i].ProviderID == id {
			h.cfg.Models[i].Status = status
		}
	}
	_ = config.Save(h.configPath, h.cfg)

	c.JSON(http.StatusOK, gin.H{"status": status, "message": msg})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func providerDisplayName(provider string) string {
	names := map[string]string{
		"anthropic":  "Anthropic",
		"openai":     "OpenAI",
		"deepseek":   "DeepSeek",
		"openrouter": "OpenRouter",
		"zhipu":      "智谱 AI",
		"kimi":       "月之暗面 (Kimi)",
		"minimax":    "MiniMax",
		"qwen":       "阿里通义千问",
		"custom":     "自定义",
	}
	if n, ok := names[provider]; ok {
		return n
	}
	return provider
}


