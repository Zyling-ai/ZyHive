// Model registry CRUD handlers.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type modelHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/models
func (h *modelHandler) List(c *gin.Context) {
	models := h.cfg.Models
	if models == nil {
		models = []config.ModelEntry{}
	}
	// Mask keys in response
	result := make([]config.ModelEntry, len(models))
	copy(result, models)
	for i := range result {
		result[i].APIKey = maskKey(result[i].APIKey)
	}
	c.JSON(http.StatusOK, result)
}

// Create POST /api/models
func (h *modelHandler) Create(c *gin.Context) {
	var entry config.ModelEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if entry.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	// Check duplicate
	for _, m := range h.cfg.Models {
		if m.ID == entry.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "model id already exists"})
			return
		}
	}
	if entry.Status == "" {
		entry.Status = "untested"
	}
	h.cfg.Models = append(h.cfg.Models, entry)
	h.save(c)
	entry.APIKey = maskKey(entry.APIKey)
	c.JSON(http.StatusCreated, entry)
}

// Update PATCH /api/models/:id
func (h *modelHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var patch config.ModelEntry
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := range h.cfg.Models {
		if h.cfg.Models[i].ID == id {
			m := &h.cfg.Models[i]
			if patch.Name != "" {
				m.Name = patch.Name
			}
			if patch.Provider != "" {
				m.Provider = patch.Provider
			}
			if patch.Model != "" {
				m.Model = patch.Model
			}
			if patch.APIKey != "" && !ismasked(patch.APIKey) {
				m.APIKey = patch.APIKey
			}
			if patch.BaseURL != "" {
				m.BaseURL = patch.BaseURL
			}
			m.IsDefault = patch.IsDefault
			if patch.Status != "" {
				m.Status = patch.Status
			}
			h.save(c)
			result := *m
			result.APIKey = maskKey(result.APIKey)
			c.JSON(http.StatusOK, result)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
}

// Delete DELETE /api/models/:id
func (h *modelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	for i := range h.cfg.Models {
		if h.cfg.Models[i].ID == id {
			h.cfg.Models = append(h.cfg.Models[:i], h.cfg.Models[i+1:]...)
			h.save(c)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
}

// resolveKey returns the effective API key for a model:
// uses the stored key if non-empty, otherwise falls back to the env var for that provider.
func resolveKey(m *config.ModelEntry) string {
	if m.APIKey != "" && !ismasked(m.APIKey) {
		return m.APIKey
	}
	if envVar, ok := envVarForProvider[m.Provider]; ok {
		return os.Getenv(envVar)
	}
	return ""
}

// Test POST /api/models/:id/test
func (h *modelHandler) Test(c *gin.Context) {
	id := c.Param("id")
	m := h.cfg.FindModel(id)
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	key := resolveKey(m)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid": false,
			"error": fmt.Sprintf("未配置 API Key（也未找到 %s 环境变量）", envVarForProvider[m.Provider]),
		})
		return
	}
	var valid bool
	var errMsg string
	switch m.Provider {
	case "anthropic":
		valid, errMsg = testAnthropicKey(key)
	case "openai":
		valid, errMsg = testOpenAIKey(key)
	case "deepseek":
		valid, errMsg = testDeepSeekKey(key)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
		return
	}
	if valid {
		m.Status = "ok"
	} else {
		m.Status = "error"
	}
	h.save(c)
	result := gin.H{"valid": valid}
	if errMsg != "" {
		result["error"] = errMsg
	}
	c.JSON(http.StatusOK, result)
}

// EnvKeys GET /api/models/env-keys
// Returns API keys found in environment variables, masked for display.
func (h *modelHandler) EnvKeys(c *gin.Context) {
	type EnvKey struct {
		Provider string `json:"provider"`
		EnvVar   string `json:"envVar"`
		Masked   string `json:"masked"`
		BaseURL  string `json:"baseUrl,omitempty"`
	}

	checks := []struct {
		provider string
		envVar   string
		baseURL  string
	}{
		{"anthropic", "ANTHROPIC_API_KEY", "https://api.anthropic.com"},
		{"openai", "OPENAI_API_KEY", "https://api.openai.com"},
		{"deepseek", "DEEPSEEK_API_KEY", "https://api.deepseek.com"},
		{"openrouter", "OPENROUTER_API_KEY", "https://openrouter.ai/api"},
	}

	found := []EnvKey{}
	for _, ch := range checks {
		val := os.Getenv(ch.envVar)
		if val != "" {
			found = append(found, EnvKey{
				Provider: ch.provider,
				EnvVar:   ch.envVar,
				Masked:   maskKey(val),
				BaseURL:  ch.baseURL,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"envKeys": found})
}

// envVarForProvider returns the env var name for a given provider.
var envVarForProvider = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"deepseek":   "DEEPSEEK_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
}

// FetchModels GET /api/models/probe?baseUrl=...&apiKey=...&provider=...
// Proxies to {baseUrl}/v1/models and returns a unified model list.
// If apiKey is empty, falls back to environment variable for the given provider.
// OpenRouter public endpoint works without any apiKey.
func (h *modelHandler) FetchModels(c *gin.Context) {
	baseURL := strings.TrimRight(c.Query("baseUrl"), "/")
	apiKey := c.Query("apiKey")
	provider := c.Query("provider")

	// Fallback to env var if no key provided
	if apiKey == "" && provider != "" {
		if envVar, ok := envVarForProvider[provider]; ok {
			apiKey = os.Getenv(envVar)
		}
	}

	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "baseUrl is required"})
		return
	}

	target := baseURL + "/v1/models"
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", target, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url: " + err.Error()})
		return
	}

	// Provider-specific headers
	switch provider {
	case "anthropic":
		// Anthropic uses x-api-key + version header; Bearer is optional but also supported
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		// OpenAI-compatible: standard Bearer token only
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
	req.Header.Set("User-Agent", "ai-panel/0.4.0")

	// If still no key and not OpenRouter (which has a public endpoint), warn early
	if apiKey == "" && provider != "openrouter" && provider != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("未配置 API Key（也未找到 %s 环境变量），请填写后再获取", envVarForProvider[provider]),
		})
		return
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "request failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("provider returned %d: %s", resp.StatusCode, truncate(string(body), 300)),
		})
		return
	}

	// Parse standard OpenAI-compatible response: {"data": [{id, name/display_name, ...}]}
	var raw struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Object      string `json:"object"`
		} `json:"data"`
		// Some providers return a flat array instead
	}

	type ModelInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	if err := json.Unmarshal(body, &raw); err != nil || len(raw.Data) == 0 {
		// Fallback: maybe it's a flat array of strings or objects
		var flat []json.RawMessage
		if json.Unmarshal(body, &flat) == nil && len(flat) > 0 {
			models := make([]ModelInfo, 0, len(flat))
			for _, item := range flat {
				var obj struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}
				if json.Unmarshal(item, &obj) == nil && obj.ID != "" {
					if obj.Name == "" {
						obj.Name = obj.ID
					}
					models = append(models, ModelInfo{ID: obj.ID, Name: obj.Name})
				}
			}
			c.JSON(http.StatusOK, gin.H{"models": models, "count": len(models)})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "unexpected response format"})
		return
	}

	models := make([]ModelInfo, 0, len(raw.Data))
	for _, d := range raw.Data {
		name := d.Name
		if name == "" {
			name = d.DisplayName
		}
		if name == "" {
			name = d.ID
		}
		models = append(models, ModelInfo{ID: d.ID, Name: name})
	}

	c.JSON(http.StatusOK, gin.H{"models": models, "count": len(models)})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (h *modelHandler) save(c *gin.Context) {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	if err := config.Save(path, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
	}
}

func ismasked(s string) bool {
	return len(s) > 3 && s[len(s)-3:] == "***"
}
