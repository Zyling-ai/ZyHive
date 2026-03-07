// internal/api/usage.go — Usage statistics API handlers.
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/usage"
)

type usageHandler struct {
	store *usage.Store
}

func newUsageHandler(store *usage.Store) *usageHandler {
	return &usageHandler{store: store}
}

// parsetime reads a Unix-seconds query param; returns 0 on missing/invalid.
func parsetime(c *gin.Context, key string) int64 {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// GET /api/usage/summary?from=&to=&agentId=&provider=
func (h *usageHandler) Summary(c *gin.Context) {
	from := parsetime(c, "from")
	to := parsetime(c, "to")
	agentID := c.Query("agentId")
	provider := c.Query("provider")
	sum := h.store.Summarize(from, to, agentID, provider)
	c.JSON(http.StatusOK, sum)
}

// GET /api/usage/timeline?from=&to=&agentId=&provider=
func (h *usageHandler) Timeline(c *gin.Context) {
	from := parsetime(c, "from")
	to := parsetime(c, "to")
	agentID := c.Query("agentId")
	provider := c.Query("provider")
	pts := h.store.Timeline(from, to, agentID, provider)
	if pts == nil {
		pts = []usage.TimelinePoint{}
	}
	c.JSON(http.StatusOK, gin.H{"points": pts})
}

// GET /api/usage/records?from=&to=&agentId=&provider=&model=&page=&pageSize=
func (h *usageHandler) Records(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	params := usage.QueryParams{
		From:     parsetime(c, "from"),
		To:       parsetime(c, "to"),
		AgentID:  c.Query("agentId"),
		Provider: c.Query("provider"),
		Model:    c.Query("model"),
		Page:     page,
		PageSize: pageSize,
	}
	result := h.store.Query(params)
	c.JSON(http.StatusOK, result)
}
