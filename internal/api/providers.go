// internal/api/providers.go — Provider API key CRUD endpoints
// GET/POST/PUT/DELETE /api/providers
// POST /api/providers/:id/test
package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/gin-gonic/gin"
)

type providerHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/providers
func (h *providerHandler) List(c *gin.Context) {
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	providers := snapshot.Providers
	if providers == nil {
		providers = []config.ProviderEntry{}
	}
	// 返回时脱敏 apiKey
	type ProviderResp struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Provider   string `json:"provider"`
		APIKey     string `json:"apiKey"` // 脱敏
		BaseURL    string `json:"baseUrl"`
		Status     string `json:"status"`
		ModelCount int    `json:"modelCount"` // 引用此 provider 的模型数量
	}
	resp := make([]ProviderResp, 0, len(providers))
	for _, p := range providers {
		masked := maskKey(p.APIKey)
		cnt := 0
		for _, m := range snapshot.Models {
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
	body.APIKey = strings.TrimSpace(body.APIKey)
	body.Name = strings.TrimSpace(body.Name)
	if body.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
		return
	}
	if body.APIKey == "" && llm.RequiresAPIKey(body.Provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "apiKey is required"})
		return
	}
	if body.Name == "" {
		body.Name = providerDisplayName(body.Provider)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
	if err := llm.ValidateProviderBaseURL(c.Request.Context(), body.Provider, baseURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "baseUrl is blocked: " + err.Error()})
		return
	}

	entry := config.ProviderEntry{
		ID:       config.RandID(),
		Name:     body.Name,
		Provider: body.Provider,
		APIKey:   body.APIKey,
		BaseURL:  baseURL,
		Status:   "untested",
	}
	if err := config.Transaction(h.configPath, h.cfg, func(candidate *config.Config) error {
		candidate.Providers = append(candidate.Providers, entry)
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response := entry
	response.APIKey = maskKey(response.APIKey)
	c.JSON(http.StatusOK, gin.H{"provider": response})
}

// Update PUT /api/providers/:id
func (h *providerHandler) Update(c *gin.Context) {
	id := c.Param("id")
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var current *config.ProviderEntry
	for i := range snapshot.Providers {
		if snapshot.Providers[i].ID == id {
			current = &snapshot.Providers[i]
			break
		}
	}
	if current == nil {
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
	var validatedBaseURL *string
	if body.BaseURL != nil {
		baseURL := strings.TrimRight(strings.TrimSpace(*body.BaseURL), "/")
		if err := llm.ValidateProviderBaseURL(c.Request.Context(), current.Provider, baseURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "baseUrl is blocked: " + err.Error()})
			return
		}
		validatedBaseURL = &baseURL
	}
	var updated config.ProviderEntry
	err = config.Transaction(h.configPath, h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Providers {
			if candidate.Providers[i].ID != id {
				continue
			}
			p := &candidate.Providers[i]
			if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
				p.Name = strings.TrimSpace(*body.Name)
			}
			if body.APIKey != nil && strings.TrimSpace(*body.APIKey) != "" && !ismasked(*body.APIKey) {
				p.APIKey = strings.TrimSpace(*body.APIKey)
				p.Status = "untested"
			}
			if validatedBaseURL != nil {
				p.BaseURL = *validatedBaseURL
			}
			updated = *p
			return nil
		}
		return errProviderNotFound
	})
	if errors.Is(err, errProviderNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updated.APIKey = maskKey(updated.APIKey)
	c.JSON(http.StatusOK, gin.H{"provider": updated})
}

// Delete DELETE /api/providers/:id
func (h *providerHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.configPath, h.cfg, func(candidate *config.Config) error {
		for _, model := range candidate.Models {
			if model.ProviderID == id {
				return errProviderInUse
			}
		}
		found := false
		newList := make([]config.ProviderEntry, 0, len(candidate.Providers))
		for _, provider := range candidate.Providers {
			if provider.ID == id {
				found = true
				continue
			}
			newList = append(newList, provider)
		}
		if !found {
			return errProviderNotFound
		}
		candidate.Providers = newList
		return nil
	})
	if errors.Is(err, errProviderNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	if errors.Is(err, errProviderInUse) {
		c.JSON(http.StatusConflict, gin.H{
			"error": "该 API Key 仍被模型使用，请先删除或重新分配这些模型",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/providers/:id/test
func (h *providerHandler) Test(c *gin.Context) {
	id := c.Param("id")
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var p *config.ProviderEntry
	for i := range snapshot.Providers {
		if snapshot.Providers[i].ID == id {
			p = &snapshot.Providers[i]
			break
		}
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	apiKey := p.APIKey
	if apiKey == "" && llm.RequiresAPIKey(p.Provider) {
		if envVar, ok := envVarForProvider[p.Provider]; ok {
			apiKey = os.Getenv(envVar)
		}
	}
	if apiKey == "" && llm.RequiresAPIKey(p.Provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no API key configured"})
		return
	}

	baseURL := p.BaseURL
	// 用户未填 baseURL 时，补全已知厂商的默认地址（minimax/kimi/zhipu/qwen 等）
	if baseURL == "" {
		baseURL = defaultBaseURLForProvider(p.Provider)
	}

	// 使用轻量的 /v1/models 探测接口
	status := "ok"
	msg := "连接成功"

	var ok bool
	var msg2 string
	switch p.Provider {
	case "anthropic":
		ok, msg2 = testAnthropicKey(apiKey, baseURL)
	case "minimax":
		// MiniMax 不支持 GET /v1/models，改用 chat completion 轻量探测
		ok, msg2 = testMiniMaxKey(apiKey, baseURL)
	default:
		ok, msg2 = testOpenAICompatKey(p.Provider, apiKey, baseURL)
	}
	if !ok {
		status = "error"
		msg = msg2
	} else if msg2 != "" {
		msg = msg2
	}

	if err := config.Transaction(h.configPath, h.cfg, func(candidate *config.Config) error {
		found := false
		for i := range candidate.Providers {
			if candidate.Providers[i].ID == id {
				candidate.Providers[i].Status = status
				found = true
				break
			}
		}
		if !found {
			return errProviderNotFound
		}
		for i := range candidate.Models {
			if candidate.Models[i].ProviderID == id {
				candidate.Models[i].Status = status
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": status, "message": msg})
}

var (
	errProviderNotFound = errors.New("provider not found")
	errProviderInUse    = errors.New("provider is in use")
)

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
		"ollama":     "Ollama（本机）",
		"custom":     "自定义",
	}
	if n, ok := names[provider]; ok {
		return n
	}
	return provider
}
