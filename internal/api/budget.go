// internal/api/budget.go — REST endpoints for the P1-02 budget brake.
//
// Routes (all under v1 = authenticated):
//   GET  /api/budget                 — read-only Snapshot
//   POST /api/budget/topup           — emergency credit for current day
//                                       body: {"agent_id":"<id|''>", "amount_usd": 1.0}
//   PATCH /api/budget/limits/:id     — set per-agent daily limit
//                                       body: {"daily_usd": 5.0}  (0 = remove override)
//
// All endpoints behave correctly even when the budget store is in disabled
// mode — Snapshot still returns current Used (it's tracked regardless of
// Enabled), Topup/SetLimit are idempotent no-ops on a nil store.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/budget"
)

type budgetHandler struct {
	store *budget.Store
}

func (h *budgetHandler) Get(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "agents": []any{}})
		return
	}
	snap := h.store.SnapshotFor(nil)
	c.JSON(http.StatusOK, snap)
}

func (h *budgetHandler) Topup(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "budget store not configured"})
		return
	}
	var body struct {
		AgentID   string  `json:"agent_id"`
		AmountUSD float64 `json:"amount_usd"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.AmountUSD <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount_usd must be > 0"})
		return
	}
	h.store.Topup(body.AgentID, body.AmountUSD)
	c.JSON(http.StatusOK, h.store.SnapshotFor([]string{body.AgentID}))
}

func (h *budgetHandler) SetLimit(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "budget store not configured"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent id required"})
		return
	}
	var body struct {
		DailyUSD float64 `json:"daily_usd"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.store.SetLimit(id, body.DailyUSD)
	c.JSON(http.StatusOK, h.store.SnapshotFor([]string{id}))
}
