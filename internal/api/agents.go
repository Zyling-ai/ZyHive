// Agent CRUD handlers.
// Reference: openclaw/src/gateway/server-chat.ts, agent-paths.ts
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type agentHandler struct {
	cfg *config.Config
}

// AgentInfo is the JSON shape returned to the frontend.
type AgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model"`
	Status      string `json:"status"` // "running" | "stopped" | "idle"
	WorkspaceDir string `json:"workspaceDir"`
}

// List GET /api/agents
func (h *agentHandler) List(c *gin.Context) {
	// TODO: scan h.cfg.Agents.Dir for agent subdirectories,
	// load each agent's config.json and IDENTITY.md.
	c.JSON(http.StatusOK, []AgentInfo{})
}

// Create POST /api/agents
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
	// TODO: create agent directory structure:
	//   agents/{id}/workspace/  (IDENTITY.md, SOUL.md, MEMORY.md)
	//   agents/{id}/sessions/
	//   agents/{id}/config.json
	c.JSON(http.StatusCreated, AgentInfo{
		ID:     req.ID,
		Name:   req.Name,
		Model:  req.Model,
		Status: "stopped",
	})
}

// Get GET /api/agents/:id
func (h *agentHandler) Get(c *gin.Context) {
	// TODO: load agent details + running status
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO"})
}

// Update PATCH /api/agents/:id
func (h *agentHandler) Update(c *gin.Context) {
	// TODO: update agent config.json and/or IDENTITY.md / SOUL.md
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO"})
}

// Delete DELETE /api/agents/:id
func (h *agentHandler) Delete(c *gin.Context) {
	// TODO: stop agent if running, then remove directory
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO"})
}

// Start POST /api/agents/:id/start
func (h *agentHandler) Start(c *gin.Context) {
	// TODO: launch agent runner goroutine
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO"})
}

// Stop POST /api/agents/:id/stop
func (h *agentHandler) Stop(c *gin.Context) {
	// TODO: signal agent runner to stop
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO"})
}
