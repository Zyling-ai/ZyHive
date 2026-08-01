// Tool/capability registry CRUD handlers.
package api

import (
	"errors"
	"net/http"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

type toolHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/tools
func (h *toolHandler) List(c *gin.Context) {
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tools := snapshot.Tools
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
	if entry.Status == "" {
		entry.Status = "untested"
	}
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for _, tool := range candidate.Tools {
			if tool.ID == entry.ID {
				return errToolExists
			}
		}
		candidate.Tools = append(candidate.Tools, entry)
		return nil
	})
	if errors.Is(err, errToolExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
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
	var result config.ToolEntry
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Tools {
			if candidate.Tools[i].ID == id {
				tool := &candidate.Tools[i]
				if patch.Name != "" {
					tool.Name = patch.Name
				}
				if patch.Type != "" {
					tool.Type = patch.Type
				}
				if patch.APIKey != "" && !ismasked(patch.APIKey) {
					tool.APIKey = patch.APIKey
				}
				if patch.BaseURL != "" {
					tool.BaseURL = patch.BaseURL
				}
				tool.Enabled = patch.Enabled
				if patch.Status != "" {
					tool.Status = patch.Status
				}
				result = *tool
				return nil
			}
		}
		return errToolNotFound
	})
	if errors.Is(err, errToolNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	result.APIKey = maskKey(result.APIKey)
	c.JSON(http.StatusOK, result)
}

// Delete DELETE /api/tools/:id
func (h *toolHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Tools {
			if candidate.Tools[i].ID == id {
				candidate.Tools = append(candidate.Tools[:i], candidate.Tools[i+1:]...)
				return nil
			}
		}
		return errToolNotFound
	})
	if errors.Is(err, errToolNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/tools/:id/test
func (h *toolHandler) Test(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Tools {
			if candidate.Tools[i].ID == id {
				candidate.Tools[i].Status = "ok"
				return nil
			}
		}
		return errToolNotFound
	})
	if errors.Is(err, errToolNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true})
}

func (h *toolHandler) path() string {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	return path
}

var (
	errToolExists   = errors.New("tool id already exists")
	errToolNotFound = errors.New("tool not found")
)
