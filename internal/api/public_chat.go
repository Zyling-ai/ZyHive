// Public chat handler — no admin auth required.
// Used by the web channel: each agent can expose multiple public chat URLs.
//
// Routes (no auth middleware):
//   GET  /pub/chat/:agentId/:channelId/info    — channel info (name, hasPassword, welcome)
//   POST /pub/chat/:agentId/:channelId/stream  — SSE streaming chat
//
// Legacy (compat — redirects to first enabled web channel):
//   GET  /pub/chat/:agentId/info
//   POST /pub/chat/:agentId/stream
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

// findWebChannelByID returns the web channel with the given ID, or nil.
func findWebChannelByID(ag *agent.Agent, channelID string) *config.ChannelEntry {
	for i := range ag.Channels {
		if ag.Channels[i].ID == channelID && ag.Channels[i].Type == "web" && ag.Channels[i].Enabled {
			return &ag.Channels[i]
		}
	}
	return nil
}

// findFirstWebChannel returns the first enabled web channel, for legacy compat.
func findFirstWebChannel(ag *agent.Agent) *config.ChannelEntry {
	for i := range ag.Channels {
		if ag.Channels[i].Type == "web" && ag.Channels[i].Enabled {
			return &ag.Channels[i]
		}
	}
	return nil
}

// infoResponse builds the JSON response for /info.
func infoResponse(ag *agent.Agent, ch *config.ChannelEntry) gin.H {
	return gin.H{
		"agentId":     ag.ID,
		"channelId":   ch.ID,
		"name":        ag.Name,
		"avatarColor": ag.AvatarColor,
		"hasPassword": ch.Config["password"] != "",
		"title":       ch.Config["title"],
		"welcomeMsg":  ch.Config["welcomeMsg"],
	}
}

// checkPassword returns true if password is satisfied (channel has no password, or header matches).
func checkPassword(ch *config.ChannelEntry, provided string) bool {
	expected := ch.Config["password"]
	return expected == "" || provided == expected
}

// ─── Handlers with channelId ──────────────────────────────────────────────────

// Info GET /pub/chat/:agentId/:channelId/info
func (h *publicChatHandler) Info(c *gin.Context) {
	agentID := c.Param("agentId")
	channelID := c.Param("channelId")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findWebChannelByID(ag, channelID)
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "web channel not found or not enabled"})
		return
	}
	// Optional password validation (client submits to check before loading)
	if pw := c.GetHeader("X-Chat-Password"); pw != "" {
		if !checkPassword(ch, pw) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
			return
		}
	}
	c.JSON(http.StatusOK, infoResponse(ag, ch))
}

// Stream POST /pub/chat/:agentId/:channelId/stream — SSE streaming response
func (h *publicChatHandler) Stream(c *gin.Context) {
	agentID := c.Param("agentId")
	channelID := c.Param("channelId")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findWebChannelByID(ag, channelID)
	if ch == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "web channel not found or not enabled"})
		return
	}

	// Password check
	if !checkPassword(ch, c.GetHeader("X-Chat-Password")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}

	var req struct {
		Message     string `json:"message"`
		SessionToken string `json:"sessionToken"` // browser-generated visitor token
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

	// Build a unique session ID: per-channel + per-visitor (enables history + compaction)
	sessionID := ""
	if req.SessionToken != "" {
		// sanitize: keep only alphanumeric + hyphen/underscore, max 64 chars
		token := sanitizeToken(req.SessionToken)
		if token != "" {
			sessionID = "web-" + channelID + "-" + token
		}
	}

	// Conversation log (permanent, admin-visible only)
	clChannelID := "web-" + channelID
	agentDir := filepath.Dir(ag.WorkspaceDir)
	cl := convlog.New(agentDir, clChannelID)

	_ = cl.Append(convlog.Entry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Role:        "user",
		Content:     req.Message,
		ChannelID:   clChannelID,
		ChannelType: "web",
		Sender:      req.SessionToken,
	})

	events, err := h.pool.RunStream(ctx, agentID, req.Message, sessionID)
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

	if resp := fullResponse.String(); resp != "" {
		_ = cl.Append(convlog.Entry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Role:        "assistant",
			Content:     resp,
			ChannelID:   clChannelID,
			ChannelType: "web",
		})
	}

	data, _ := json.Marshal(gin.H{"type": "done"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
}

// ─── Legacy compat (first enabled web channel) ────────────────────────────────

// InfoLegacy GET /pub/chat/:agentId/info
func (h *publicChatHandler) InfoLegacy(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findFirstWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "web channel not enabled"})
		return
	}
	if pw := c.GetHeader("X-Chat-Password"); pw != "" {
		if !checkPassword(ch, pw) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
			return
		}
	}
	c.JSON(http.StatusOK, infoResponse(ag, ch))
}

// StreamLegacy POST /pub/chat/:agentId/stream
func (h *publicChatHandler) StreamLegacy(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findFirstWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "web channel not enabled"})
		return
	}
	if !checkPassword(ch, c.GetHeader("X-Chat-Password")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	var req struct {
		Message      string `json:"message"`
		SessionToken string `json:"sessionToken"`
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

	sessionID := ""
	if req.SessionToken != "" {
		if token := sanitizeToken(req.SessionToken); token != "" {
			sessionID = "web-" + ch.ID + "-" + token
		}
	}

	clChannelID := "web-" + ch.ID
	agentDir := filepath.Dir(ag.WorkspaceDir)
	cl := convlog.New(agentDir, clChannelID)
	_ = cl.Append(convlog.Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339), Role: "user",
		Content: req.Message, ChannelID: clChannelID, ChannelType: "web", Sender: req.SessionToken,
	})

	events, err := h.pool.RunStream(ctx, agentID, req.Message, sessionID)
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
	if resp := fullResponse.String(); resp != "" {
		_ = cl.Append(convlog.Entry{
			Timestamp: time.Now().UTC().Format(time.RFC3339), Role: "assistant",
			Content: resp, ChannelID: clChannelID, ChannelType: "web",
		})
	}
	data, _ := json.Marshal(gin.H{"type": "done"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	flusher.Flush()
}

// sanitizeToken keeps only alphanumeric + hyphen/underscore, caps at 64 chars.
func sanitizeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// _ suppress unused import
var _ = agent.Agent{}
