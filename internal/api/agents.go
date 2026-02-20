// Agent CRUD handlers.
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
	pool    *agent.Pool
}

// AgentInfo is the JSON shape returned to the frontend.
type AgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Model        string   `json:"model"`
	ModelID      string   `json:"modelId,omitempty"`
	ToolIDs      []string `json:"toolIds,omitempty"`
	SkillIDs     []string `json:"skillIds,omitempty"`
	AvatarColor  string   `json:"avatarColor,omitempty"`
	Status       string   `json:"status"`
	WorkspaceDir string   `json:"workspaceDir"`
}

func agentToInfo(a *agent.Agent) AgentInfo {
	return AgentInfo{
		ID:           a.ID,
		Name:         a.Name,
		Description:  a.Description,
		Model:        a.Model,
		ModelID:      a.ModelID,
		ToolIDs:      a.ToolIDs,
		SkillIDs:     a.SkillIDs,
		AvatarColor:  a.AvatarColor,
		Status:       a.Status,
		WorkspaceDir: a.WorkspaceDir,
	}
}

// List GET /api/agents
func (h *agentHandler) List(c *gin.Context) {
	agents := h.manager.List()
	result := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		result = append(result, agentToInfo(a))
	}
	c.JSON(http.StatusOK, result)
}

// Create POST /api/agents — supports both legacy and new format
func (h *agentHandler) Create(c *gin.Context) {
	var req struct {
		ID          string   `json:"id" binding:"required"`
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Model       string   `json:"model"`
		ModelID     string   `json:"modelId"`
		ToolIDs     []string `json:"toolIds"`
		SkillIDs    []string `json:"skillIds"`
		AvatarColor string   `json:"avatarColor"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve model: prefer modelId, fall back to model string, then default
	model := req.Model
	modelID := req.ModelID
	if modelID != "" {
		if m := h.cfg.FindModel(modelID); m != nil {
			model = m.ProviderModel()
		}
	} else if model == "" {
		if m := h.cfg.DefaultModel(); m != nil {
			model = m.ProviderModel()
			modelID = m.ID
		}
	}

	a, err := h.manager.CreateWithOpts(agent.CreateOpts{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Model:       model,
		ModelID:     modelID,
		ToolIDs:     req.ToolIDs,
		SkillIDs:    req.SkillIDs,
		AvatarColor: req.AvatarColor,
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, agentToInfo(a))
}

// Get GET /api/agents/:id
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
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Delete DELETE /api/agents/:id
func (h *agentHandler) Delete(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Start POST /api/agents/:id/start
func (h *agentHandler) Start(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Stop POST /api/agents/:id/stop
func (h *agentHandler) Stop(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "TODO: Phase 2"})
}

// Message POST /api/agents/:id/message
// 让一个 Agent 同步发消息给另一个 Agent，等待响应后返回。
// Body:   { "message": "...", "fromAgentId": "..." }
// Return: { "response": "..." }
func (h *agentHandler) Message(c *gin.Context) {
	targetID := c.Param("id")

	var req struct {
		Message     string `json:"message" binding:"required"`
		FromAgentID string `json:"fromAgentId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify target agent exists
	if _, ok := h.manager.Get(targetID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "target agent not found"})
		return
	}

	if h.pool == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent pool not initialized"})
		return
	}

	// Optionally prepend sender context so the target agent knows who is talking
	message := req.Message
	if req.FromAgentID != "" {
		if from, ok := h.manager.Get(req.FromAgentID); ok {
			message = "[来自 Agent「" + from.Name + "」的消息]\n" + req.Message
		}
	}

	response, err := h.pool.Run(c.Request.Context(), targetID, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"response": response})
}
