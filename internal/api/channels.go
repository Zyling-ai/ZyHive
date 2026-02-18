// Channel registry CRUD handlers.
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
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
	// For now just mark as ok
	ch.Status = "ok"
	h.save(c)
	c.JSON(http.StatusOK, gin.H{"valid": true})
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
