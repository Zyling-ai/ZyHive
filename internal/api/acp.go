// Package api — ACP agents CRUD handler.
// ACP (Agent Control Protocol) agents are external coding-agent CLIs
// such as claude-code, codex, gemini-cli.
package api

import (
	"errors"
	"fmt"
	"net/http"
	osexec "os/exec"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

// Ensure acpHandler has the expected interface shape.
var _ = (*acpHandler)(nil)

type acpHandler struct {
	cfg     *config.Config
	pool    *agent.Pool
	cfgPath string // path to config file on disk
}

// List GET /api/acp
func (h *acpHandler) List(c *gin.Context) {
	snapshot, err := config.Snapshot(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if snapshot.ACPAgents == nil {
		c.JSON(http.StatusOK, []config.ACPAgentEntry{})
		return
	}
	c.JSON(http.StatusOK, snapshot.ACPAgents)
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

	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for _, existing := range candidate.ACPAgents {
			if existing.ID == entry.ID {
				return errACPExists
			}
		}
		candidate.ACPAgents = append(candidate.ACPAgents, entry)
		return nil
	})
	if errors.Is(err, errACPExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.syncPool()
	c.JSON(http.StatusOK, entry)
}

// Update PATCH /api/acp/:id
func (h *acpHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var patch config.ACPAgentEntry
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	patch.ID = id // protect ID
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.ACPAgents {
			if candidate.ACPAgents[i].ID == id {
				candidate.ACPAgents[i] = patch
				return nil
			}
		}
		return errACPNotFound
	})
	if errors.Is(err, errACPNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.syncPool()
	c.JSON(http.StatusOK, patch)
}

// Delete DELETE /api/acp/:id
func (h *acpHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		newList := make([]config.ACPAgentEntry, 0, len(candidate.ACPAgents))
		found := false
		for _, entry := range candidate.ACPAgents {
			if entry.ID == id {
				found = true
			} else {
				newList = append(newList, entry)
			}
		}
		if !found {
			return errACPNotFound
		}
		candidate.ACPAgents = newList
		return nil
	})
	if errors.Is(err, errACPNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.syncPool()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test POST /api/acp/:id/test — checks if the CLI binary exists in PATH.
func (h *acpHandler) Test(c *gin.Context) {
	id := c.Param("id")
	snapshot, snapshotErr := config.Snapshot(h.cfg)
	if snapshotErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": snapshotErr.Error()})
		return
	}
	var found *config.ACPAgentEntry
	for i := range snapshot.ACPAgents {
		if snapshot.ACPAgents[i].ID == id {
			found = &snapshot.ACPAgents[i]
			break
		}
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ACP agent not found"})
		return
	}

	path, err := osexec.LookPath(found.Binary)
	if err != nil {
		_ = h.updateStatus(id, "error")
		c.JSON(http.StatusOK, gin.H{"id": found.ID, "binary": found.Binary, "status": "error", "error": err.Error()})
		return
	}
	if err := h.updateStatus(id, "ok"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": found.ID, "binary": found.Binary, "path": path, "status": "ok"})
}

func (h *acpHandler) path() string {
	if h.cfgPath == "" {
		return "aipanel.json"
	}
	return h.cfgPath
}

func (h *acpHandler) syncPool() {
	snapshot, err := config.Snapshot(h.cfg)
	if err == nil && h.pool != nil {
		h.pool.SetACPAgents(snapshot.ACPAgents)
	}
}

func (h *acpHandler) updateStatus(id, status string) error {
	return config.Transaction(h.path(), h.cfg, func(candidate *config.Config) error {
		for i := range candidate.ACPAgents {
			if candidate.ACPAgents[i].ID == id {
				candidate.ACPAgents[i].Status = status
				return nil
			}
		}
		return errACPNotFound
	})
}

var (
	errACPExists   = errors.New("ACP agent id already exists")
	errACPNotFound = errors.New("ACP agent not found")
)
