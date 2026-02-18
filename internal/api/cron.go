// Cron job handler — CRUD for scheduled jobs + run history.
// Reference: openclaw/src/gateway/server-cron.ts, src/cron/
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/cron"
)

type cronHandler struct {
	engine *cron.Engine
}

// List GET /api/cron
func (h *cronHandler) List(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	jobs := h.engine.ListJobs()
	if jobs == nil {
		jobs = []*cron.Job{}
	}
	c.JSON(http.StatusOK, jobs)
}

// Create POST /api/cron
func (h *cronHandler) Create(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cron engine not initialized"})
		return
	}
	var job cron.Job
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.engine.Add(&job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

// Update PATCH /api/cron/:jobId
func (h *cronHandler) Update(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cron engine not initialized"})
		return
	}
	jobID := c.Param("jobId")
	var patch cron.Job
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.engine.Update(jobID, &patch); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete DELETE /api/cron/:jobId
func (h *cronHandler) Delete(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cron engine not initialized"})
		return
	}
	jobID := c.Param("jobId")
	if err := h.engine.Remove(jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Run POST /api/cron/:jobId/run — trigger immediately
func (h *cronHandler) Run(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cron engine not initialized"})
		return
	}
	jobID := c.Param("jobId")
	if err := h.engine.RunNow(jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "job triggered"})
}

// Runs GET /api/cron/:jobId/runs
func (h *cronHandler) Runs(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	jobID := c.Param("jobId")
	runs, err := h.engine.ListRuns(jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, runs)
}
