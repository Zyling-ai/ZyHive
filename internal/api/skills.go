// Skill registry handlers.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

type skillHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/skills
func (h *skillHandler) List(c *gin.Context) {
	skills := h.cfg.Skills
	if skills == nil {
		skills = []config.SkillEntry{}
	}
	c.JSON(http.StatusOK, skills)
}

// Install POST /api/skills/install
func (h *skillHandler) Install(c *gin.Context) {
	var entry config.SkillEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if entry.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	for _, s := range h.cfg.Skills {
		if s.ID == entry.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "skill id already exists"})
			return
		}
	}
	if entry.Version == "" {
		entry.Version = "1.0.0"
	}
	entry.Enabled = true
	h.cfg.Skills = append(h.cfg.Skills, entry)
	h.save(c)
	c.JSON(http.StatusCreated, entry)
}

// Delete DELETE /api/skills/:id
func (h *skillHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	for i := range h.cfg.Skills {
		if h.cfg.Skills[i].ID == id {
			h.cfg.Skills = append(h.cfg.Skills[:i], h.cfg.Skills[i+1:]...)
			h.save(c)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
}

func (h *skillHandler) save(c *gin.Context) {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	if err := config.Save(path, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
	}
}
