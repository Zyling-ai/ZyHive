// Memory handler — hierarchical memory tree API.
package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
)

type memoryHandler struct {
	manager *agent.Manager
}

func (h *memoryHandler) getTree(ag *agent.Agent) *memory.MemoryTree {
	return memory.NewMemoryTree(ag.WorkspaceDir)
}

// Tree GET /api/agents/:id/memory/tree — returns full memory tree structure
func (h *memoryHandler) Tree(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	nodes, err := h.getTree(ag).ListTree()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

// ReadFile GET /api/agents/:id/memory/file/*path — read a specific memory file
func (h *memoryHandler) ReadFile(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	if relPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	content, err := h.getTree(ag).GetFile(relPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": relPath, "content": content, "size": len(content)})
}

// WriteFile PUT /api/agents/:id/memory/file/*path — write a memory file
func (h *memoryHandler) WriteFile(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	if relPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 5*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.getTree(ag).WriteFile(relPath, string(body)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": relPath, "size": len(body)})
}

// DailyLog POST /api/agents/:id/memory/daily — append to today's daily log
func (h *memoryHandler) DailyLog(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content := strings.TrimSpace(string(body))
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content required"})
		return
	}
	if err := h.getTree(ag).WriteDailyLog(content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
