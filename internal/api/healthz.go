// Package api — /healthz and /api/status endpoints for observability.
package api

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/cron"
)

// startTime records when the process started (set at package init).
var startTime = time.Now()

// healthzHandler serves the public GET /healthz endpoint (no auth required).
// Returns a lightweight JSON snapshot of system health.
type healthzHandler struct {
	manager    *agent.Manager
	cronEngine *cron.Engine
	cfg        *config.Config
}

func (h *healthzHandler) Handle(c *gin.Context) {
	uptimeSecs := int64(time.Since(startTime).Seconds())

	// ── Agents ──────────────────────────────────────────────────────────────
	agentTotal := 0
	agentActive := 0
	if h.manager != nil {
		agents := h.manager.List()
		agentTotal = len(agents)
		for _, ag := range agents {
			if ag.Status == "running" {
				agentActive++
			}
		}
	}

	// ── Cron ────────────────────────────────────────────────────────────────
	cronTotal := 0
	cronDisabled := 0
	cronRunning := 0 // currently no per-job running state; placeholder
	if h.cronEngine != nil {
		jobs := h.cronEngine.ListJobs()
		cronTotal = len(jobs)
		for _, j := range jobs {
			if !j.Enabled {
				cronDisabled++
			}
		}
	}

	// ── Telegram ────────────────────────────────────────────────────────────
	telegramConnected := false
	var lastMessageAt *string
	if h.manager != nil {
		for _, ag := range h.manager.List() {
			for _, ch := range ag.Channels {
				if ch.Type == "telegram" && ch.Status == "connected" {
					telegramConnected = true
				}
			}
		}
	}

	// ── Memory ──────────────────────────────────────────────────────────────
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memMB := ms.Alloc / 1024 / 1024

	resp := gin.H{
		"status":          "ok",
		"version":         AppVersion,
		"uptime_seconds":  uptimeSecs,
		"agents": gin.H{
			"total":  agentTotal,
			"active": agentActive,
		},
		"cron": gin.H{
			"total":    cronTotal,
			"disabled": cronDisabled,
			"running":  cronRunning,
		},
		"telegram": gin.H{
			"connected":       telegramConnected,
			"last_message_at": lastMessageAt,
		},
		"memory_mb": memMB,
	}

	c.JSON(http.StatusOK, resp)
}

// statusHandler serves the authenticated GET /api/status endpoint.
// Returns detailed system status including per-agent info and cron job list.
type statusHandler struct {
	manager    *agent.Manager
	cronEngine *cron.Engine
}

func (h *statusHandler) Handle(c *gin.Context) {
	uptimeSecs := int64(time.Since(startTime).Seconds())

	// ── Agents ──────────────────────────────────────────────────────────────
	type agentDetail struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		Model    string `json:"model"`
		ModelID  string `json:"modelId,omitempty"`
	}

	var agentDetails []agentDetail
	if h.manager != nil {
		for _, ag := range h.manager.List() {
			agentDetails = append(agentDetails, agentDetail{
				ID:      ag.ID,
				Name:    ag.Name,
				Status:  ag.Status,
				Model:   ag.Model,
				ModelID: ag.ModelID,
			})
		}
	}
	if agentDetails == nil {
		agentDetails = []agentDetail{}
	}

	// ── Cron ────────────────────────────────────────────────────────────────
	type cronJobStatus struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Enabled        bool   `json:"enabled"`
		Schedule       string `json:"schedule"`
		LastStatus     string `json:"lastStatus,omitempty"`
		ErrorCount     int    `json:"errorCount,omitempty"`
		DisabledReason string `json:"disabledReason,omitempty"`
		NextRunAtMs    int64  `json:"nextRunAtMs,omitempty"`
		LastRunAtMs    int64  `json:"lastRunAtMs,omitempty"`
	}

	var cronJobs []cronJobStatus
	if h.cronEngine != nil {
		for _, j := range h.cronEngine.ListJobs() {
			schedule := j.Schedule.Expr
			if j.Schedule.Kind == "every" && j.Schedule.EveryMs > 0 {
				// "every" kind uses EveryMs, not Expr; display as human-readable duration
				d := time.Duration(j.Schedule.EveryMs) * time.Millisecond
				schedule = "@every " + d.String()
			}
			cronJobs = append(cronJobs, cronJobStatus{
				ID:             j.ID,
				Name:           j.Name,
				Enabled:        j.Enabled,
				Schedule:       schedule,
				LastStatus:     j.State.LastStatus,
				ErrorCount:     j.State.ErrorCount,
				DisabledReason: j.State.DisabledReason,
				NextRunAtMs:    j.State.NextRunAtMs,
				LastRunAtMs:    j.State.LastRunAtMs,
			})
		}
	}
	if cronJobs == nil {
		cronJobs = []cronJobStatus{}
	}

	// ── Memory ──────────────────────────────────────────────────────────────
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memMB := ms.Alloc / 1024 / 1024

	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"version":        AppVersion,
		"uptime_seconds": uptimeSecs,
		"memory_mb":      memMB,
		"agents":         agentDetails,
		"cron":           cronJobs,
		"goroutines":     runtime.NumGoroutine(),
	})
}
