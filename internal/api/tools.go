// Tool/capability registry CRUD handlers.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type toolHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/tools
func (h *toolHandler) List(c *gin.Context) {
	tools := h.cfg.Tools
	if tools == nil {
		tools = []config.ToolEntry{}
	}
	result := make([]config.ToolEntry, len(tools))
	copy(result, tools)
	for i := range result {
		result[i].APIKey = maskKey(result[i].APIKey)
	}
	c.JSON(http.StatusOK, result)
}

// Create POST /api/tools
func (h *toolHandler) Create(c *gin.Context) {
	var entry config.ToolEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if entry.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	for _, t := range h.cfg.Tools {
		if t.ID == entry.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "tool id already exists"})
			return
		}
	}
	if entry.Status == "" {
		entry.Status = "untested"
	}
	h.cfg.Tools = append(h.cfg.Tools, entry)
	h.save(c)
	entry.APIKey = maskKey(entry.APIKey)
	c.JSON(http.StatusCreated, entry)
}

// Update PATCH /api/tools/:id
func (h *toolHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var patch config.ToolEntry
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := range h.cfg.Tools {
		if h.cfg.Tools[i].ID == id {
			t := &h.cfg.Tools[i]
			if patch.Name != "" {
				t.Name = patch.Name
			}
			if patch.Type != "" {
				t.Type = patch.Type
			}
			if patch.APIKey != "" && !ismasked(patch.APIKey) {
				t.APIKey = patch.APIKey
			}
			if patch.BaseURL != "" {
				t.BaseURL = patch.BaseURL
			}
			t.Enabled = patch.Enabled
			if patch.Status != "" {
				t.Status = patch.Status
			}
			h.save(c)
			result := *t
			result.APIKey = maskKey(result.APIKey)
			c.JSON(http.StatusOK, result)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
}

// Delete DELETE /api/tools/:id
func (h *toolHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	for i := range h.cfg.Tools {
		if h.cfg.Tools[i].ID == id {
			h.cfg.Tools = append(h.cfg.Tools[:i], h.cfg.Tools[i+1:]...)
			h.save(c)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
}

// Test POST /api/tools/:id/test
func (h *toolHandler) Test(c *gin.Context) {
	id := c.Param("id")
	for i := range h.cfg.Tools {
		if h.cfg.Tools[i].ID == id {
			h.cfg.Tools[i].Status = "ok"
			h.save(c)
			c.JSON(http.StatusOK, gin.H{"valid": true})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
}

func (h *toolHandler) save(c *gin.Context) {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	if err := config.Save(path, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
	}
}
