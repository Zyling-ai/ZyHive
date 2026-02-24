// Goals & Planning API handler — CRUD for goals, milestones, and periodic checks.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/goal"
)

type goalHandler struct {
	mgr *goal.Manager
}

// List GET /api/goals
// Optional ?agentId=xxx to filter by agent.
func (h *goalHandler) List(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	var goals []*goal.Goal
	if agentID := c.Query("agentId"); agentID != "" {
		goals = h.mgr.ListByAgent(agentID)
	} else {
		goals = h.mgr.List()
	}
	if goals == nil {
		goals = []*goal.Goal{}
	}
	c.JSON(http.StatusOK, goals)
}

// Create POST /api/goals
func (h *goalHandler) Create(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	var g goal.Goal
	if err := c.ShouldBindJSON(&g); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.Create(&g); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, g)
}

// Get GET /api/goals/:id
func (h *goalHandler) Get(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	id := c.Param("id")
	g, err := h.mgr.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, g)
}

// Update PATCH /api/goals/:id
func (h *goalHandler) Update(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	id := c.Param("id")
	var patch goal.Goal
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.Update(id, &patch); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete DELETE /api/goals/:id
func (h *goalHandler) Delete(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	id := c.Param("id")
	if err := h.mgr.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UpdateProgress PATCH /api/goals/:id/progress
// Body: {"progress": 80}
func (h *goalHandler) UpdateProgress(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	id := c.Param("id")
	var body struct {
		Progress int `json:"progress"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.UpdateProgress(id, body.Progress); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SetMilestoneDone PATCH /api/goals/:id/milestones/:mid
// Body: {"done": true}
func (h *goalHandler) SetMilestoneDone(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	goalID := c.Param("id")
	milestoneID := c.Param("mid")
	var body struct {
		Done bool `json:"done"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.SetMilestoneDone(goalID, milestoneID, body.Done); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Check plans ──────────────────────────────────────────────────────────────

// ListChecks GET /api/goals/:id/checks
func (h *goalHandler) ListChecks(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	goalID := c.Param("id")
	g, err := h.mgr.Get(goalID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	checks := g.Checks
	if checks == nil {
		checks = []goal.GoalCheck{}
	}
	c.JSON(http.StatusOK, checks)
}

// AddCheck POST /api/goals/:id/checks
func (h *goalHandler) AddCheck(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	goalID := c.Param("id")
	var check goal.GoalCheck
	if err := c.ShouldBindJSON(&check); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.AddCheck(goalID, &check); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, check)
}

// UpdateCheck PATCH /api/goals/:id/checks/:checkId
func (h *goalHandler) UpdateCheck(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	goalID := c.Param("id")
	checkID := c.Param("checkId")
	var patch goal.GoalCheck
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.mgr.UpdateCheck(goalID, checkID, &patch); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RemoveCheck DELETE /api/goals/:id/checks/:checkId
func (h *goalHandler) RemoveCheck(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	goalID := c.Param("id")
	checkID := c.Param("checkId")
	if err := h.mgr.RemoveCheck(goalID, checkID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RunCheckNow POST /api/goals/:id/checks/:checkId/run
func (h *goalHandler) RunCheckNow(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "goal manager not initialized"})
		return
	}
	goalID := c.Param("id")
	checkID := c.Param("checkId")
	if err := h.mgr.RunCheckNow(goalID, checkID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "check triggered"})
}

// ListCheckRecords GET /api/goals/:id/check-records
func (h *goalHandler) ListCheckRecords(c *gin.Context) {
	if h.mgr == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	goalID := c.Param("id")
	records, err := h.mgr.ListCheckRecords(goalID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}
