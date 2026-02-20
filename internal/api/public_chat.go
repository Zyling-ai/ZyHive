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
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/convlog"
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
// Optional: if X-Chat-Password header is provided and the channel has a password,
// returns 401 if incorrect (used for client-side password validation).
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
	// If client is submitting a password for validation, check it
	if pw := c.GetHeader("X-Chat-Password"); pw != "" {
		if expected := ch.Config["password"]; expected != "" && pw != expected {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
			return
		}
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

	// Build conversation log for this web channel
	channelID := "web-" + agentID
	agentDir := filepath.Dir(ag.WorkspaceDir) // workspace is at {agentDir}/workspace
	cl := convlog.New(agentDir, channelID)

	// Log inbound user message
	_ = cl.Append(convlog.Entry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Role:        "user",
		Content:     req.Message,
		ChannelID:   channelID,
		ChannelType: "web",
		Sender:      "visitor",
	})

	events, err := h.pool.RunStream(ctx, agentID, req.Message)
	if err != nil {
		data, _ := json.Marshal(gin.H{"type": "error", "text": err.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
		return
	}

	var fullResponse strings.Builder
	for ev := range events {
		switch ev.Type {
		case "text_delta":
			fullResponse.WriteString(ev.Text)
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

	// Log assistant response
	if resp := fullResponse.String(); resp != "" {
		_ = cl.Append(convlog.Entry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Role:        "assistant",
			Content:     resp,
			ChannelID:   channelID,
			ChannelType: "web",
		})
	}

	// Done
	data, _ := json.Marshal(gin.H{"type": "done"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
}

// _ suppress unused import
var _ = agent.Agent{}
