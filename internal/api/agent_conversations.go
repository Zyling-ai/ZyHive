// Package api — Agent conversation log handlers.
// Provides read-only admin access to permanent channel conversation logs.
// Routes:
//   GET /api/conversations                            — global: all agents, all channels (filterable)
//   GET /api/agents/:agentId/conversations           — list channel summaries for one agent
//   GET /api/agents/:agentId/conversations/:channelId — paginated message list
package api

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

// GlobalConvRow is a flattened conversation entry for the global view.
type GlobalConvRow struct {
	AgentID      string `json:"agentId"`
	AgentName    string `json:"agentName"`
	ChannelID    string `json:"channelId"`
	ChannelType  string `json:"channelType"`
	MessageCount int    `json:"messageCount"`
	LastAt       string `json:"lastAt"`
	FirstAt      string `json:"firstAt"`
}

// GlobalList GET /api/conversations
// Query params: agentId (optional), channelType (optional: "telegram"|"web"|...)
// Returns all channel conversation summaries across all agents, newest first.
func (h *convHandler) GlobalList(c *gin.Context) {
	filterAgent := c.Query("agentId")
	filterType := c.Query("channelType")

	agents := h.manager.List()
	var rows []GlobalConvRow

	for _, ag := range agents {
		if filterAgent != "" && ag.ID != filterAgent {
			continue
		}
		agentDir := filepath.Join(h.agentsDir, ag.ID)
		summaries, err := convlog.ListChannels(agentDir)
		if err != nil {
			continue
		}
		for _, s := range summaries {
			if filterType != "" && !strings.EqualFold(s.ChannelType, filterType) {
				continue
			}
			rows = append(rows, GlobalConvRow{
				AgentID:      ag.ID,
				AgentName:    ag.Name,
				ChannelID:    s.ChannelID,
				ChannelType:  s.ChannelType,
				MessageCount: s.MessageCount,
				LastAt:       s.LastAt,
				FirstAt:      s.FirstAt,
			})
		}
	}

	// Sort by lastAt desc
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].LastAt > rows[j].LastAt
	})

	if rows == nil {
		rows = []GlobalConvRow{}
	}
	c.JSON(http.StatusOK, rows)
}
