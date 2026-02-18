// Cron job handler — CRUD for scheduled jobs + run history.
// Reference: openclaw/src/gateway/server-cron.ts, src/cron/
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

// ── Types ─────────────────────────────────────────────────────────────────

// CronJob is one scheduled job (compatible with OpenClaw cron format).
type CronJob struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Enabled     bool         `json:"enabled"`
	Schedule    CronSchedule `json:"schedule"`
	Payload     CronPayload  `json:"payload"`
	Delivery    CronDelivery `json:"delivery"`
	CreatedAtMs int64        `json:"createdAtMs"`
}

type CronSchedule struct {
	Kind string `json:"kind"` // "cron"
	Expr string `json:"expr"` // e.g. "30 3 * * *"
	TZ   string `json:"tz"`   // e.g. "Asia/Shanghai"
}

type CronPayload struct {
	Kind    string `json:"kind"`    // "agentTurn"
	Message string `json:"message"` // the message to send to the agent
	AgentID string `json:"agentId,omitempty"`
}

type CronDelivery struct {
	Mode string `json:"mode"` // "announce"
}

// CronRunRecord stores one execution record.
type CronRunRecord struct {
	ID        string `json:"id"`
	JobID     string `json:"jobId"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt"`
	Status    string `json:"status"` // "success" | "error"
	Error     string `json:"error,omitempty"`
}

// ── Handler ───────────────────────────────────────────────────────────────

type cronHandler struct {
	cfg     *config.Config
	manager *agent.Manager
	mu      sync.Mutex
}

const cronDir = "cron"
const jobsFile = "jobs.json"

func (h *cronHandler) jobsPath() string {
	return filepath.Join(cronDir, jobsFile)
}

func (h *cronHandler) runsPath(jobID string) string {
	return filepath.Join(cronDir, "runs", jobID+".json")
}

func (h *cronHandler) loadJobs() ([]CronJob, error) {
	data, err := os.ReadFile(h.jobsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []CronJob{}, nil
		}
		return nil, err
	}
	var jobs []CronJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (h *cronHandler) saveJobs(jobs []CronJob) error {
	if err := os.MkdirAll(cronDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.jobsPath(), data, 0644)
}

func (h *cronHandler) loadRuns(jobID string) ([]CronRunRecord, error) {
	data, err := os.ReadFile(h.runsPath(jobID))
	if err != nil {
		if os.IsNotExist(err) {
			return []CronRunRecord{}, nil
		}
		return nil, err
	}
	var runs []CronRunRecord
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func (h *cronHandler) appendRun(jobID string, record CronRunRecord) error {
	runs, _ := h.loadRuns(jobID)
	runs = append(runs, record)
	// Keep only last 50
	if len(runs) > 50 {
		runs = runs[len(runs)-50:]
	}
	dir := filepath.Join(cronDir, "runs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(runs, "", "  ")
	return os.WriteFile(h.runsPath(jobID), data, 0644)
}

// List GET /api/cron
func (h *cronHandler) List(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	jobs, err := h.loadJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// Create POST /api/cron
func (h *cronHandler) Create(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var job CronJob
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%d", time.Now().UnixMilli())
	}
	if job.CreatedAtMs == 0 {
		job.CreatedAtMs = time.Now().UnixMilli()
	}

	jobs, err := h.loadJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	jobs = append(jobs, job)
	if err := h.saveJobs(jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

// Update PATCH /api/cron/:jobId
func (h *cronHandler) Update(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	jobID := c.Param("jobId")
	var patch CronJob
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobs, err := h.loadJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	found := false
	for i, j := range jobs {
		if j.ID == jobID {
			patch.ID = jobID
			if patch.CreatedAtMs == 0 {
				patch.CreatedAtMs = j.CreatedAtMs
			}
			jobs[i] = patch
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if err := h.saveJobs(jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, patch)
}

// Delete DELETE /api/cron/:jobId
func (h *cronHandler) Delete(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	jobID := c.Param("jobId")
	jobs, err := h.loadJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filtered := make([]CronJob, 0, len(jobs))
	found := false
	for _, j := range jobs {
		if j.ID == jobID {
			found = true
			continue
		}
		filtered = append(filtered, j)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if err := h.saveJobs(filtered); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Run POST /api/cron/:jobId/run — trigger immediately
func (h *cronHandler) Run(c *gin.Context) {
	h.mu.Lock()
	jobs, err := h.loadJobs()
	h.mu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jobID := c.Param("jobId")
	var job *CronJob
	for _, j := range jobs {
		if j.ID == jobID {
			jc := j
			job = &jc
			break
		}
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	startedAt := time.Now().UnixMilli()
	record := CronRunRecord{
		ID:        fmt.Sprintf("run-%d", startedAt),
		JobID:     jobID,
		StartedAt: startedAt,
		Status:    "success",
	}

	// For now, just record the run. Real execution would use runner.
	record.EndedAt = time.Now().UnixMilli()

	h.mu.Lock()
	h.appendRun(jobID, record)
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"ok": true, "run": record})
}

// Runs GET /api/cron/:jobId/runs
func (h *cronHandler) Runs(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	jobID := c.Param("jobId")
	runs, err := h.loadRuns(jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Return last 20
	if len(runs) > 20 {
		runs = runs[len(runs)-20:]
	}
	c.JSON(http.StatusOK, runs)
}
