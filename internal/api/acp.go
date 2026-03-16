// Package api — ACP agents CRUD handler.
// ACP (Agent Control Protocol) agents are external coding-agent CLIs
// such as claude-code, codex, gemini-cli.
package api

import (
	"fmt"
	"net/http"
	osexec "os/exec"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
)

// Ensure acpHandler has the expected interface shape.
var _ = (*acpHandler)(nil)

type acpHandler struct {
	cfg  *config.Config
	pool *agent.Pool
	cfgPath string                  // path to config file on disk

}

// List GET /api/acp
func (h *acpHandler) List(c *gin.Context) {
	if h.cfg.ACPAgents == nil {
		c.JSON(http.StatusOK, []config.ACPAgentEntry{})
		return
	}
	c.JSON(http.StatusOK, h.cfg.ACPAgents)
}

// Create POST /api/acp
func (h *acpHandler) Create(c *gin.Context) {
	var entry config.ACPAgentEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if entry.Name == "" || entry.Binary == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and binary are required"})
		return
	}
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("acp-%d", time.Now().UnixNano()%1_000_000_000)
	}
	entry.Status = "untested"

	h.cfg.ACPAgents = append(h.cfg.ACPAgents, entry)
	if err := config.Save(h.cfgPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.pool.SetACPAgents(h.cfg.ACPAgents)
	c.JSON(http.StatusOK, entry)
}

// Update PATCH /api/acp/:id
func (h *acpHandler) Update(c *gin.Context) {
	id := c.Param("id")
	idx := -1
	for i, a := range h.cfg.ACPAgents {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}

	var patch config.ACPAgentEntry
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	patch.ID = id // protect ID
	h.cfg.ACPAgents[idx] = patch

	if err := config.Save(h.cfgPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.pool.SetACPAgents(h.cfg.ACPAgents)
	c.JSON(http.StatusOK, patch)
}

// Delete DELETE /api/acp/:id
func (h *acpHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	newList := make([]config.ACPAgentEntry, 0, len(h.cfg.ACPAgents))
	found := false
	for _, a := range h.cfg.ACPAgents {
		if a.ID == id {
			found = true
		} else {
			newList = append(newList, a)
		}
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}
	h.cfg.ACPAgents = newList

	if err := config.Save(h.cfgPath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.pool.SetACPAgents(h.cfg.ACPAgents)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/acp/:id/test — checks if the CLI binary exists in PATH.
func (h *acpHandler) Test(c *gin.Context) {
	id := c.Param("id")
	var found *config.ACPAgentEntry
	for i := range h.cfg.ACPAgents {
		if h.cfg.ACPAgents[i].ID == id {
			found = &h.cfg.ACPAgents[i]
			break
		}
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}

	path, err := osexec.LookPath(found.Binary)
	if err != nil {
		// Mark as error in memory (not persisted).
		found.Status = "error"
		c.JSON(http.StatusOK, gin.H{"id": found.ID, "binary": found.Binary, "status": "error", "error": err.Error()})
		return
	}
	found.Status = "ok"
	c.JSON(http.StatusOK, gin.H{"id": found.ID, "binary": found.Binary, "path": path, "status": "ok"})
}
