// Channel registry CRUD handlers.
package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

type channelHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/channels
func (h *channelHandler) List(c *gin.Context) {
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	channels := snapshot.Channels
	if channels == nil {
		channels = []config.ChannelEntry{}
	}
	// Mask secrets
	result := make([]config.ChannelEntry, len(channels))
	copy(result, channels)
	for i := range result {
		mc := make(map[string]string)
		for k, v := range result[i].Config {
			if isSecretField(k) {
				mc[k] = maskKey(v)
			} else {
				mc[k] = v
			}
		}
		result[i].Config = mc
	}
	c.JSON(http.StatusOK, result)
}

// Create POST /api/channels
func (h *channelHandler) Create(c *gin.Context) {
	var entry config.ChannelEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if entry.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if entry.Status == "" {
		entry.Status = "untested"
	}
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for _, channelEntry := range candidate.Channels {
			if channelEntry.ID == entry.ID {
				return errChannelExists
			}
		}
		candidate.Channels = append(candidate.Channels, entry)
		return nil
	})
	if errors.Is(err, errChannelExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, entry)
}

// Update PATCH /api/channels/:id
func (h *channelHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var patch config.ChannelEntry
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var updated config.ChannelEntry
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Channels {
			if candidate.Channels[i].ID == id {
				ch := &candidate.Channels[i]
				if patch.Name != "" {
					ch.Name = patch.Name
				}
				if patch.Type != "" {
					ch.Type = patch.Type
				}
				if patch.Config != nil {
					for k, v := range patch.Config {
						if !ismasked(v) {
							ch.Config[k] = v
						}
					}
				}
				ch.Enabled = patch.Enabled
				if patch.Status != "" {
					ch.Status = patch.Status
				}
				updated = *ch
				return nil
			}
		}
		return errChannelNotFound
	})
	if errors.Is(err, errChannelNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// Delete DELETE /api/channels/:id
func (h *channelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Channels {
			if candidate.Channels[i].ID == id {
				candidate.Channels = append(candidate.Channels[:i], candidate.Channels[i+1:]...)
				return nil
			}
		}
		return errChannelNotFound
	})
	if errors.Is(err, errChannelNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/channels/:id/test
func (h *channelHandler) Test(c *gin.Context) {
	id := c.Param("id")
	snapshot, snapshotErr := config.Snapshot(h.cfg)
	if snapshotErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": snapshotErr.Error()})
		return
	}
	var ch *config.ChannelEntry
	for i := range snapshot.Channels {
		if snapshot.Channels[i].ID == id {
			ch = &snapshot.Channels[i]
			break
		}
	}
	if ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	var (
		name string
		err  error
	)
	switch ch.Type {
	case "telegram":
		token := ch.Config["botToken"]
		if token == "" || ismasked(token) {
			_ = h.updateStatus(id, "error")
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "telegram botToken is required"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
		defer cancel()
		name, err = channel.TestTelegramBot(ctx, token)
	case "feishu":
		appID, appSecret := ch.Config["appId"], ch.Config["appSecret"]
		if appID == "" || appSecret == "" || ismasked(appSecret) {
			_ = h.updateStatus(id, "error")
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "feishu appId and appSecret are required"})
			return
		}
		name, err = channel.TestFeishuBot(appID, appSecret)
	default:
		_ = h.updateStatus(id, "error")
		c.JSON(http.StatusNotImplemented, gin.H{
			"valid": false,
			"error": "channel type " + ch.Type + " has no real connectivity probe",
		})
		return
	}
	if err != nil {
		_ = h.updateStatus(id, "error")
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": err.Error()})
		return
	}
	if err := h.updateStatus(id, "ok"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "name": name})
}

func (h *channelHandler) path() string {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	return path
}

func (h *channelHandler) updateStatus(id, status string) error {
	return config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Channels {
			if candidate.Channels[i].ID == id {
				candidate.Channels[i].Status = status
				return nil
			}
		}
		return errChannelNotFound
	})
}

var (
	errChannelExists   = errors.New("channel id already exists")
	errChannelNotFound = errors.New("channel not found")
)
