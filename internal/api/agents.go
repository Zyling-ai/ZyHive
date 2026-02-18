// Agent CRUD handlers.
// Reference: openclaw/src/gateway/server-chat.ts, agent-paths.ts
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type agentHandler struct {
	cfg     *config.Config
	manager *agent.Manager
}

// AgentInfo is the JSON shape returned to the frontend.
type AgentInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Model        string `json:"model"`
	Status       string `json:"status"` // "running" | "stopped" | "idle"
	WorkspaceDir string `json:"workspaceDir"`
}

func agentToInfo(a *agent.Agent) AgentInfo {
	return AgentInfo{
		ID:           a.ID,
		Name:         a.Name,
		Model:        a.Model,
		Status:       a.Status,
		WorkspaceDir: a.WorkspaceDir,
	}
}

// List GET /api/agents — returns all agents from the manager.
func (h *agentHandler) List(c *gin.Context) {
	agents := h.manager.List()
	result := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		result = append(result, agentToInfo(a))
	}
	c.JSON(http.StatusOK, result)
}

// Create POST /api/agents — creates a new agent with directory structure.
func (h *agentHandler) Create(c *gin.Context) {
	var req struct {
		ID    string `json:"id" binding:"required"`
		Name  string `json:"name" binding:"required"`
		Model string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Model == "" {
		req.Model = h.cfg.Models.Primary
	}
	a, err := h.manager.Create(req.ID, req.Name, req.Model)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, agentToInfo(a))
}

// Get GET /api/agents/:id — returns a single agent's details.
func (h *agentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	a, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, agentToInfo(a))
}

// Update PATCH /api/agents/:id
func (h *agentHandler) Update(c *gin.Context) {
	// TODO: update agent config.json and/or IDENTITY.md / SOUL.md
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Delete DELETE /api/agents/:id
func (h *agentHandler) Delete(c *gin.Context) {
	// TODO: stop agent if running, then remove directory
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Start POST /api/agents/:id/start
func (h *agentHandler) Start(c *gin.Context) {
	// TODO: launch agent runner goroutine
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Stop POST /api/agents/:id/stop
func (h *agentHandler) Stop(c *gin.Context) {
	// TODO: signal agent runner to stop
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}
