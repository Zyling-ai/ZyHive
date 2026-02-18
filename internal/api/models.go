// Model registry CRUD handlers.
package api

import (
	"net/http"

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

// Test POST /api/models/:id/test
func (h *modelHandler) Test(c *gin.Context) {
	id := c.Param("id")
	m := h.cfg.FindModel(id)
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}
	var valid bool
	var errMsg string
	switch m.Provider {
	case "anthropic":
		valid, errMsg = testAnthropicKey(m.APIKey)
	case "openai":
		valid, errMsg = testOpenAIKey(m.APIKey)
	case "deepseek":
		valid, errMsg = testDeepSeekKey(m.APIKey)
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
