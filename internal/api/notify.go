// notify.go — proactive notification endpoint.
//
// POST /api/agents/:id/notify
//   Runs the agent in the per-chat Telegram session and sends the response to
//   the specified chat. This is the primary mechanism for cron-triggered or
//   system-event-triggered outbound messages that maintain conversation context.
//
// Architecture:
//   1. Agent runner loads/saves the session keyed "telegram-{chatID}".
//   2. The user's prompt is recorded as a "user" turn.
//   3. The agent's response is recorded as an "assistant" turn.
//   4. The response is sent to Telegram (HTML-formatted, falls back to plain).
//
// This makes proactive notifications first-class citizens of the conversation:
// the user can reply in Telegram and the agent remembers the prior notification.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type notifyHandler struct {
	botCtrl BotControl
}

// NotifyRequest is the request body for POST /api/agents/:id/notify.
type NotifyRequest struct {
	// ChannelID is the agent channel config ID. Pass "" to use the first active Telegram bot.
	ChannelID string `json:"channelID"`
	// ChatID is the Telegram chat ID to send the notification to.
	ChatID int64 `json:"chatID"`
	// ThreadID is the message thread ID. Use 0 for non-threaded chats.
	ThreadID int64 `json:"threadID,omitempty"`
	// Prompt is the message injected as the "user" turn in the session.
	// The agent will respond to this and send its response to the chat.
	Prompt string `json:"prompt"`
}

// Notify POST /api/agents/:id/notify
func (h *notifyHandler) Notify(c *gin.Context) {
	agentID := c.Param("id")

	var req NotifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ChatID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chatID is required"})
		return
	}
	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}
	if h.botCtrl.Notify == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notify not configured"})
		return
	}

	if err := h.botCtrl.Notify(c.Request.Context(), agentID, req.ChannelID, req.ChatID, req.ThreadID, req.Prompt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
