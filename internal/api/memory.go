// Memory handler — hierarchical memory tree API + memory consolidation config.
package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/cron"
	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
)

type memoryHandler struct {
	manager    *agent.Manager
	cronEngine *cron.Engine
	pool       *agent.Pool
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

// ── Memory Config API ────────────────────────────────────────────────────────

// GetConfig GET /api/agents/:id/memory/config — read memory consolidation config
func (h *memoryHandler) GetConfig(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	cfg, _ := memory.ReadMemConfig(ag.WorkspaceDir)
	c.JSON(http.StatusOK, cfg)
}

// SetConfig PUT /api/agents/:id/memory/config — save config + manage cron job
func (h *memoryHandler) SetConfig(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var incoming memory.MemConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Read existing config to preserve cronJobId
	existing, _ := memory.ReadMemConfig(ag.WorkspaceDir)

	// Remove old cron job if any
	if existing.CronJobID != "" && h.cronEngine != nil {
		_ = h.cronEngine.Remove(existing.CronJobID)
		existing.CronJobID = ""
	}

	// Merge fields
	existing.Enabled = incoming.Enabled
	existing.Schedule = incoming.Schedule
	existing.KeepTurns = incoming.KeepTurns
	existing.FocusHint = incoming.FocusHint

	// Create new cron job if enabling
	if incoming.Enabled && h.cronEngine != nil {
		job := &cron.Job{
			Name:    "memory-consolidate-" + ag.ID,
			Enabled: true,
			AgentID: ag.ID,
			Schedule: cron.Schedule{
				Kind: "cron",
				Expr: memory.ScheduleToCron(incoming.Schedule),
				TZ:   "Asia/Shanghai",
			},
			Payload: cron.Payload{
				Kind:    "agentTurn",
				Message: "__MEMORY_CONSOLIDATE__",
			},
			Delivery: cron.Delivery{Mode: "none"},
		}
		if err := h.cronEngine.Add(job); err == nil {
			existing.CronJobID = job.ID
		}
	}

	if err := memory.WriteMemConfig(ag.WorkspaceDir, existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

// ConsolidateNow POST /api/agents/:id/memory/consolidate — trigger immediately
func (h *memoryHandler) ConsolidateNow(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if h.pool == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pool not initialized"})
		return
	}
	go func() {
		_, _ = h.pool.ConsolidateMemory(context.Background(), ag.ID)
	}()
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "记忆整理已在后台启动"})
}

// RunLog GET /api/agents/:id/memory/run-log — read consolidation run history
func (h *memoryHandler) RunLog(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	entries, err := memory.ReadRunLog(ag.WorkspaceDir, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}
