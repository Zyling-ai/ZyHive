// Per-agent skill management REST handlers.
// Routes:
//   GET    /api/agents/:agentId/skills
//   POST   /api/agents/:agentId/skills
//   PATCH  /api/agents/:agentId/skills/:skillId
//   DELETE /api/agents/:agentId/skills/:skillId
package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/skill"
)

type agentSkillHandler struct {
	manager *agent.Manager
}

func newAgentSkillHandler(mgr *agent.Manager) *agentSkillHandler {
	return &agentSkillHandler{manager: mgr}
}

// List GET /api/agents/:id/skills
func (h *agentSkillHandler) List(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	metas, err := skill.ScanSkills(ag.WorkspaceDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metas)
}

// installSkillRequest is the POST body for installing a skill.
type installSkillRequest struct {
	Meta          skill.Meta `json:"meta"`
	PromptContent string     `json:"promptContent"`
}

// Create POST /api/agents/:id/skills
func (h *agentSkillHandler) Create(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var req installSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	meta := req.Meta
	if meta.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "meta.id is required"})
		return
	}
	if meta.Version == "" {
		meta.Version = "1.0.0"
	}
	if meta.Source == "" {
		meta.Source = "local"
	}
	meta.Enabled = true
	if meta.InstalledAt == "" {
		meta.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	}

	if err := skill.WriteSkill(ag.WorkspaceDir, meta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write skill.json: " + err.Error()})
		return
	}

	// Write SKILL.md
	skillMdPath := filepath.Join(ag.WorkspaceDir, "skills", meta.ID, "SKILL.md")
	promptContent := req.PromptContent
	if promptContent == "" {
		promptContent = fmt.Sprintf("# %s\n\n%s\n", meta.Name, meta.Description)
	}
	if err := os.WriteFile(skillMdPath, []byte(promptContent), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write SKILL.md: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, meta)
}

// updateSkillRequest is the PATCH body.
type updateSkillRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
	Icon        *string `json:"icon"`
	Category    *string `json:"category"`
}

// Update PATCH /api/agents/:id/skills/:skillId
func (h *agentSkillHandler) Update(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	skillID := c.Param("skillId")

	meta, err := skill.ReadSkill(ag.WorkspaceDir, skillID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	var req updateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		meta.Name = *req.Name
	}
	if req.Description != nil {
		meta.Description = *req.Description
	}
	if req.Enabled != nil {
		meta.Enabled = *req.Enabled
	}
	if req.Icon != nil {
		meta.Icon = *req.Icon
	}
	if req.Category != nil {
		meta.Category = *req.Category
	}

	if err := skill.WriteSkill(ag.WorkspaceDir, *meta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write skill.json: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, meta)
}

// Delete DELETE /api/agents/:id/skills/:skillId
func (h *agentSkillHandler) Delete(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	skillID := c.Param("skillId")

	if err := skill.RemoveSkill(ag.WorkspaceDir, skillID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
