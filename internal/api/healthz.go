// Package api — /healthz, /readyz and /api/status endpoints for observability.
package api

import (
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/session"
)

// startTime records when the process started (set at package init).
var startTime = time.Now()

// healthzHandler serves the public GET /healthz endpoint (no auth required).
// Returns a lightweight JSON snapshot of system health.
type healthzHandler struct {
	manager    *agent.Manager
	cronEngine *cron.Engine
	cfg        *config.Config
	workerPool *session.WorkerPool
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

	// ── Session worker pool ────────────────────────────────────────────────
	sessionsTotal, sessionsBusy := 0, 0
	if h.workerPool != nil {
		sessionsTotal, sessionsBusy = h.workerPool.ActiveCount()
	}

	// ── Cron heartbeat freshness ───────────────────────────────────────────
	cronLastTickAgo := int64(-1)
	if h.cronEngine != nil {
		t := h.cronEngine.LastTickAt()
		if !t.IsZero() {
			cronLastTickAgo = int64(time.Since(t).Seconds())
		}
	}

	// ── Provider ping snapshot (read-only, never triggers a probe) ─────────
	probes := llm.PingSnapshot()
	probesOK, probesFail := 0, 0
	for _, p := range probes {
		if p.OK {
			probesOK++
		} else {
			probesFail++
		}
	}

	resp := gin.H{
		"status":          "ok",
		"version":         AppVersion,
		"uptime_seconds":  uptimeSecs,
		"agents": gin.H{
			"total":  agentTotal,
			"active": agentActive,
		},
		"cron": gin.H{
			"total":              cronTotal,
			"disabled":           cronDisabled,
			"running":            cronRunning,
			"last_tick_ago_secs": cronLastTickAgo,
		},
		"sessions": gin.H{
			"total": sessionsTotal,
			"busy":  sessionsBusy,
		},
		"providers": gin.H{
			"probed_ok":   probesOK,
			"probed_fail": probesFail,
		},
		"telegram": gin.H{
			"connected":       telegramConnected,
			"last_message_at": lastMessageAt,
		},
		"memory_mb": memMB,
	}

	c.JSON(http.StatusOK, resp)
}

// ── /readyz ─────────────────────────────────────────────────────────────────
//
// readyzHandler answers the Kubernetes-style readiness question: is the system
// ready to serve real traffic? Distinct from /healthz (process-up "I'm alive")
// in that we deliberately fail with 503 when:
//   - cron engine has stopped ticking (no heartbeat in >3× heartbeat interval)
//   - session pool backlog is unhealthy (configurable cap; default very lax)
//   - any provider that has been probed shows failures while ZERO providers
//     are healthy (cold start with no probes is treated as "unknown -> ok")
//
// All thresholds are intentionally generous; we tune as we collect production
// signal. None of the checks call out (e.g. trigger Pings) — they read in-memory
// state populated by other code paths. Strict read-only.
type readyzHandler struct {
	cronEngine *cron.Engine
	workerPool *session.WorkerPool
}

// readyzCheck holds the result of one named subsystem check.
type readyzCheck struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// staleCronAfter — if the cron engine's heartbeat hasn't refreshed in this
// many seconds, /readyz treats it as failed. cron heartbeat fires every 10s
// (see pkg/cron.heartbeatInterval), so 60s = 6 missed beats.
const staleCronAfter = 60 * time.Second

// sessionsBacklogCap — total active workers above this triggers fail.
// Generous default. Operators can tune via future config; for now hard-coded.
const sessionsBacklogCap = 200

func (h *readyzHandler) Handle(c *gin.Context) {
	checks := map[string]readyzCheck{}

	// cron heartbeat
	if h.cronEngine == nil {
		// No engine wired — treat as ok (likely a unit-test or partial-init env).
		checks["cron"] = readyzCheck{OK: true, Detail: "engine not wired"}
	} else {
		t := h.cronEngine.LastTickAt()
		switch {
		case t.IsZero():
			checks["cron"] = readyzCheck{OK: false, Detail: "engine never started"}
		case time.Since(t) > staleCronAfter:
			ago := int64(time.Since(t).Seconds())
			checks["cron"] = readyzCheck{
				OK:     false,
				Detail: "no heartbeat in " + strconv.FormatInt(ago, 10) + "s",
			}
		default:
			checks["cron"] = readyzCheck{OK: true}
		}
	}

	// session pool backlog
	if h.workerPool == nil {
		checks["sessions"] = readyzCheck{OK: true, Detail: "pool not wired"}
	} else {
		total, busy := h.workerPool.ActiveCount()
		switch {
		case total > sessionsBacklogCap:
			checks["sessions"] = readyzCheck{
				OK:     false,
				Detail: "active workers " + strconv.Itoa(total) + " > cap " + strconv.Itoa(sessionsBacklogCap),
			}
		default:
			checks["sessions"] = readyzCheck{
				OK:     true,
				Detail: "active=" + strconv.Itoa(total) + " busy=" + strconv.Itoa(busy),
			}
		}
	}

	// provider probes — cold start (zero probes) is treated as "ok unknown".
	probes := llm.PingSnapshot()
	if len(probes) == 0 {
		checks["providers"] = readyzCheck{OK: true, Detail: "no probes yet (cold start)"}
	} else {
		okCount, failNotes := 0, []string{}
		for _, p := range probes {
			if p.OK {
				okCount++
			} else {
				note := p.Error
				if note == "" {
					note = "fail"
				}
				failNotes = append(failNotes, note)
			}
		}
		if okCount == 0 {
			checks["providers"] = readyzCheck{
				OK:     false,
				Detail: "all probed providers failing: " + strings.Join(failNotes, "; "),
			}
		} else {
			checks["providers"] = readyzCheck{
				OK:     true,
				Detail: strconv.Itoa(okCount) + "/" + strconv.Itoa(len(probes)) + " ok",
			}
		}
	}

	allOK := true
	for _, ck := range checks {
		if !ck.OK {
			allOK = false
			break
		}
	}
	status := http.StatusOK
	if !allOK {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"ready":   allOK,
		"checks":  checks,
		"version": AppVersion,
	})
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
