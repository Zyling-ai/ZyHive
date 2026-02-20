// Public chat handler — no admin auth required.
// Used by the web channel: each agent can expose a public chat URL.
//
// Routes (no auth middleware):
//   GET  /pub/chat/:agentId/info    — channel info (name, hasPassword, welcome)
//   POST /pub/chat/:agentId/stream  — SSE streaming chat
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type publicChatHandler struct {
	manager *agent.Manager
	pool    *agent.Pool
}

// findWebChannel returns the first enabled web channel for an agent, or nil.
func findWebChannel(ag *agent.Agent) *config.ChannelEntry {
	for i := range ag.Channels {
		if ag.Channels[i].Type == "web" && ag.Channels[i].Enabled {
			return &ag.Channels[i]
		}
	}
	return nil
}

// Info GET /pub/chat/:agentId/info
func (h *publicChatHandler) Info(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "web channel not enabled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"agentId":     ag.ID,
		"name":        ag.Name,
		"avatarColor": ag.AvatarColor,
		"hasPassword": ch.Config["password"] != "",
		"title":       ch.Config["title"],
		"welcomeMsg":  ch.Config["welcomeMsg"],
	})
}

// Stream POST /pub/chat/:agentId/stream — SSE streaming response
func (h *publicChatHandler) Stream(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "web channel not enabled"})
		return
	}

	// Password check
	if pwd := ch.Config["password"]; pwd != "" {
		if c.GetHeader("X-Chat-Password") != pwd {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
			return
		}
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message required"})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()

	events, err := h.pool.RunStream(ctx, agentID, req.Message)
	if err != nil {
		data, _ := json.Marshal(gin.H{"type": "error", "text": err.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
		return
	}

	for ev := range events {
		switch ev.Type {
		case "text_delta":
			data, _ := json.Marshal(gin.H{"type": "text_delta", "text": ev.Text})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			flusher.Flush()
		case "error":
			if ev.Error != nil {
				data, _ := json.Marshal(gin.H{"type": "error", "text": ev.Error.Error()})
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}

	// Done
	data, _ := json.Marshal(gin.H{"type": "done"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
}

// _ suppress unused import
var _ = agent.Agent{}
