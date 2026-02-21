package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/subagent"
)

type subagentHandler struct {
	mgr     *subagent.Manager
	agentMgr *agent.Manager
}

// List GET /api/tasks
func (h *subagentHandler) List(c *gin.Context) {
	agentID := c.Query("agentId")
	status := c.Query("status")
	sessionID := c.Query("sessionId")
	tasks := h.mgr.List(agentID)

	// Filter by sessionId if requested (for re-attaching after page reload)
	if sessionID != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if t.SpawnedBySession == sessionID {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Filter by status if requested
	if status != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if string(t.Status) == status {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}
	if tasks == nil {
		tasks = []*subagent.Task{}
	}
	c.JSON(http.StatusOK, tasks)
}

// Get GET /api/tasks/:id
func (h *subagentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	task, ok := h.mgr.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// Kill DELETE /api/tasks/:id
func (h *subagentHandler) Kill(c *gin.Context) {
	id := c.Param("id")
	if err := h.mgr.Kill(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Spawn POST /api/tasks
func (h *subagentHandler) Spawn(c *gin.Context) {
	var req struct {
		AgentID string `json:"agentId" binding:"required"`
		Task    string `json:"task" binding:"required"`
		Label   string `json:"label"`
		Model   string `json:"model"`
		SpawnedBy string `json:"spawnedBy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate agent exists
	if _, ok := h.agentMgr.Get(req.AgentID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found: " + req.AgentID})
		return
	}

	task, err := h.mgr.Spawn(subagent.SpawnOpts{
		AgentID:   req.AgentID,
		Label:     req.Label,
		Task:      req.Task,
		Model:     req.Model,
		SpawnedBy: req.SpawnedBy,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, task)
}
