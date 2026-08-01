// Skill registry handlers.
package api

import (
	"errors"
	"net/http"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

type skillHandler struct {
	cfg        *config.Config
	configPath string
}

// List GET /api/skills
func (h *skillHandler) List(c *gin.Context) {
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	skills := snapshot.Skills
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
	if entry.Version == "" {
		entry.Version = "1.0.0"
	}
	entry.Enabled = true
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for _, skill := range candidate.Skills {
			if skill.ID == entry.ID {
				return errSkillExists
			}
		}
		candidate.Skills = append(candidate.Skills, entry)
		return nil
	})
	if errors.Is(err, errSkillExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, entry)
}

// Delete DELETE /api/skills/:id
func (h *skillHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.Skills {
			if candidate.Skills[i].ID == id {
				candidate.Skills = append(candidate.Skills[:i], candidate.Skills[i+1:]...)
				return nil
			}
		}
		return errSkillNotFound
	})
	if errors.Is(err, errSkillNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *skillHandler) path() string {
	path := h.configPath
	if path == "" {
		path = "aipanel.json"
	}
	return path
}

var (
	errSkillExists   = errors.New("skill id already exists")
	errSkillNotFound = errors.New("skill not found")
)
