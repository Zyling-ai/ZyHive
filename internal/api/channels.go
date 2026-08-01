// Channel registry CRUD handlers.
package api

import (
	"context"
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
	channels := h.cfg.Channels
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
	for _, ch := range h.cfg.Channels {
		if ch.ID == entry.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "channel id already exists"})
			return
		}
	}
	if entry.Status == "" {
		entry.Status = "untested"
	}
	h.cfg.Channels = append(h.cfg.Channels, entry)
	h.save(c)
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
	for i := range h.cfg.Channels {
		if h.cfg.Channels[i].ID == id {
			ch := &h.cfg.Channels[i]
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
			h.save(c)
			c.JSON(http.StatusOK, ch)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
}

// Delete DELETE /api/channels/:id
func (h *channelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	for i := range h.cfg.Channels {
		if h.cfg.Channels[i].ID == id {
			h.cfg.Channels = append(h.cfg.Channels[:i], h.cfg.Channels[i+1:]...)
			h.save(c)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
}

// Test POST /api/channels/:id/test
func (h *channelHandler) Test(c *gin.Context) {
	id := c.Param("id")
	var ch *config.ChannelEntry
	for i := range h.cfg.Channels {
		if h.cfg.Channels[i].ID == id {
			ch = &h.cfg.Channels[i]
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
			ch.Status = "error"
			h.save(c)
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "telegram botToken is required"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
		defer cancel()
		name, err = channel.TestTelegramBot(ctx, token)
	case "feishu":
		appID, appSecret := ch.Config["appId"], ch.Config["appSecret"]
		if appID == "" || appSecret == "" || ismasked(appSecret) {
			ch.Status = "error"
			h.save(c)
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "feishu appId and appSecret are required"})
			return
		}
		name, err = channel.TestFeishuBot(appID, appSecret)
	default:
		ch.Status = "error"
		h.save(c)
		c.JSON(http.StatusNotImplemented, gin.H{
			"valid": false,
			"error": "channel type " + ch.Type + " has no real connectivity probe",
		})
		return
	}
	if err != nil {
		ch.Status = "error"
		h.save(c)
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": err.Error()})
		return
	}
	ch.Status = "ok"
	h.save(c)
	c.JSON(http.StatusOK, gin.H{"valid": true, "name": name})
}

func (h *channelHandler) save(c *gin.Context) {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	if err := config.Save(path, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
	}
}
