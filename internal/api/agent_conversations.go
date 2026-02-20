// Package api — Agent conversation log handlers.
// Provides read-only admin access to permanent channel conversation logs.
// Routes:
//   GET /api/agents/:agentId/conversations           — list channel summaries
//   GET /api/agents/:agentId/conversations/:channelId — paginated message list
package api

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/convlog"
)

type convHandler struct {
	manager   *agent.Manager
	agentsDir string
}

func newConvHandler(mgr *agent.Manager, agentsDir string) *convHandler {
	return &convHandler{manager: mgr, agentsDir: agentsDir}
}

// List GET /api/agents/:id/conversations
// Returns a list of channel summaries for the given agent.
func (h *convHandler) List(c *gin.Context) {
	agentID := c.Param("id")
	if _, ok := h.manager.Get(agentID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	agentDir := filepath.Join(h.agentsDir, agentID)
	summaries, err := convlog.ListChannels(agentDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summaries)
}

// Messages GET /api/agents/:id/conversations/:channelId
// Query params: limit (default 50), offset (default 0)
// Returns: { total: int, messages: []Entry }
func (h *convHandler) Messages(c *gin.Context) {
	agentID := c.Param("id")
	channelID := c.Param("channelId")

	if _, ok := h.manager.Get(agentID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v >= 0 {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	agentDir := filepath.Join(h.agentsDir, agentID)
	messages, total, err := convlog.ReadMessages(agentDir, channelID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"messages": messages,
	})
}
