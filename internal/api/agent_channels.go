// Per-agent channel configuration handlers.
// Each AI member manages its own messaging channels (e.g. its own Telegram Bot Token).
// Routes: GET/PUT /api/agents/:id/channels
package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/channel"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

func removeFile(path string) error { return os.Remove(path) }

type agentChannelHandler struct {
	manager    *agent.Manager
	runnerFunc channel.RunnerFunc
}

// GetChannels GET /api/agents/:id/channels
func (h *agentChannelHandler) GetChannels(c *gin.Context) {
	ag, ok := h.manager.Get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	channels := ag.Channels
	if channels == nil {
		channels = []config.ChannelEntry{}
	}
	// Mask secrets
	result := make([]config.ChannelEntry, len(channels))
	copy(result, channels)
	for i := range result {
		mc := make(map[string]string)
		for k, v := range result[i].Config {
			if strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "key") {
				mc[k] = maskKey(v)
			} else {
				mc[k] = v
			}
		}
		result[i].Config = mc
	}
	c.JSON(http.StatusOK, result)
}

// SetChannels PUT /api/agents/:id/channels
// Accepts the full list; saves to agent's config.json.
func (h *agentChannelHandler) SetChannels(c *gin.Context) {
	agentID := c.Param("id")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var incoming []config.ChannelEntry
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Preserve existing secrets: if the frontend sent a masked value, keep the real one
	existing := ag.Channels
	for i := range incoming {
		ch := &incoming[i]
		if ch.Config == nil {
			ch.Config = map[string]string{}
		}
		// Find matching existing channel by ID to restore masked keys
		for _, ex := range existing {
			if ex.ID == ch.ID {
				for k, v := range ch.Config {
					if ismasked(v) {
						if real, ok := ex.Config[k]; ok {
							ch.Config[k] = real
						}
					}
				}
				break
			}
		}
		// Auto-assign ID if empty
		if ch.ID == "" {
			ch.ID = ch.Type + "-" + agentID + "-" + timeStampShort()
		}
		if ch.Status == "" {
			ch.Status = "untested"
		}
	}

	// Find channels that were removed and clean up their pending stores
	incomingIDs := make(map[string]bool)
	for _, ch := range incoming {
		incomingIDs[ch.ID] = true
	}
	agentDir := filepath.Join(h.manager.AgentsDir(), agentID)
	pendingDir := filepath.Join(agentDir, "channels-pending")
	for _, ex := range existing {
		if !incomingIDs[ex.ID] {
			// Channel removed — delete its pending store
			pendingFile := filepath.Join(pendingDir, ex.ID+"-pending.json")
			_ = removeFile(pendingFile)
		}
	}

	if err := h.manager.UpdateChannels(agentID, incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(incoming)})
}

// TestChannel POST /api/agents/:id/channels/:chId/test
// For Telegram: calls getMe to verify the bot token.
func (h *agentChannelHandler) TestChannel(c *gin.Context) {
	agentID := c.Param("id")
	chID := c.Param("chId")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var ch *config.ChannelEntry
	for i := range ag.Channels {
		if ag.Channels[i].ID == chID {
			ch = &ag.Channels[i]
			break
		}
	}
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}

	switch ch.Type {
	case "telegram":
		token := ch.Config["botToken"]
		if token == "" || ismasked(token) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "botToken is required"})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		botName, err := channel.TestTelegramBot(ctx, token)
		if err != nil {
			ch.Status = "error"
			_ = h.manager.UpdateChannels(agentID, ag.Channels)
			c.JSON(http.StatusOK, gin.H{"valid": false, "error": err.Error()})
			return
		}
		ch.Status = "ok"
		if botName != "" {
			ch.Config["botName"] = botName
		}
		_ = h.manager.UpdateChannels(agentID, ag.Channels)
		c.JSON(http.StatusOK, gin.H{"valid": true, "botName": botName})

	default:
		// Generic: just mark ok
		ch.Status = "ok"
		_ = h.manager.UpdateChannels(agentID, ag.Channels)
		c.JSON(http.StatusOK, gin.H{"valid": true})
	}
}

func timeStampShort() string {
	return strings.ToLower(strings.ReplaceAll(
		time.Now().Format("0102150405"),
		":", "",
	))
}

// ── Pending users (users who messaged bot but not yet in allowlist) ────────

// pendingDir returns the channels-pending directory for an agent.
func pendingDir(ag *agent.Agent) string {
	// WorkspaceDir = {agentsDir}/{agentId}/workspace → parent = {agentsDir}/{agentId}
	return filepath.Join(filepath.Dir(ag.WorkspaceDir), "channels-pending")
}

// ListPending GET /api/agents/:id/channels/:chId/pending
func (h *agentChannelHandler) ListPending(c *gin.Context) {
	agentID := c.Param("id")
	chID := c.Param("chId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	ps := channel.NewPendingStore(pendingDir(ag), chID)
	c.JSON(http.StatusOK, ps.List())
}

// AllowPending POST /api/agents/:id/channels/:chId/pending/:userId/allow
// Adds the user to the channel's allowedFrom list and removes from pending.
func (h *agentChannelHandler) AllowPending(c *gin.Context) {
	agentID := c.Param("id")
	chID := c.Param("chId")
	userIDStr := c.Param("userId")

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userId"})
		return
	}

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Find the channel
	chIdx := -1
	for i, ch := range ag.Channels {
		if ch.ID == chID {
			chIdx = i
			break
		}
	}
	if chIdx < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}

	ch := &ag.Channels[chIdx]
	if ch.Config == nil {
		ch.Config = map[string]string{}
	}

	// Parse existing allowedFrom, add new ID (dedup)
	existing := ch.Config["allowedFrom"]
	ids := parseIDList(existing)
	ids = appendUnique(ids, fmt.Sprintf("%d", userID))
	ch.Config["allowedFrom"] = strings.Join(ids, ",")

	// Save channels
	if err := h.manager.UpdateChannels(agentID, ag.Channels); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Remove from pending store
	ps := channel.NewPendingStore(pendingDir(ag), chID)
	ps.Remove(userID)

	c.JSON(http.StatusOK, gin.H{"ok": true, "allowedFrom": ch.Config["allowedFrom"]})
}

// DismissPending DELETE /api/agents/:id/channels/:chId/pending/:userId
// Removes the user from the pending list without adding to allowlist.
func (h *agentChannelHandler) DismissPending(c *gin.Context) {
	agentID := c.Param("id")
	chID := c.Param("chId")
	userIDStr := c.Param("userId")

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userId"})
		return
	}

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	ps := channel.NewPendingStore(pendingDir(ag), chID)
	ps.Remove(userID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parseIDList splits a comma-separated ID string into a slice.
func parseIDList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// appendUnique adds id to list if not already present.
func appendUnique(list []string, id string) []string {
	for _, v := range list {
		if v == id {
			return list
		}
	}
	return append(list, id)
}
