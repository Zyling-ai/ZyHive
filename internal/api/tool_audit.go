// internal/api/tool_audit.go — REST handlers for the tool-audit log.
//
// Endpoints (all under /api, all behind the standard auth middleware):
//
//	GET /api/agents/:id/tool-audit/:toolCallId
//	GET /api/agents/:id/sessions/:sid/tool-audit?limit=N
//	GET /api/tool-audit?agentId=&sessionId=&tool=&dateFrom=&dateTo=&limit=&offset=
//	GET /api/agents/:id/tool-audit/blobs/:name        (raw blob bytes)
//
// Added 26.5.12v1 (F-03).

package api

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/toolaudit"
	"github.com/gin-gonic/gin"
)

type toolAuditHandler struct {
	manager *agent.Manager
}

func (h *toolAuditHandler) logFor(c *gin.Context) (*toolaudit.Log, *agent.Agent, bool) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return nil, nil, false
	}
	agentDir := filepath.Dir(ag.WorkspaceDir)
	return toolaudit.New(agentDir), ag, true
}

// GET /api/agents/:id/tool-audit/:toolCallId
func (h *toolAuditHandler) GetEntry(c *gin.Context) {
	log, _, ok := h.logFor(c)
	if !ok {
		return
	}
	tcid := c.Param("toolCallId")
	if tcid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "toolCallId required"})
		return
	}
	e, err := log.GetByID(tcid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if e == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool call not in audit log (older than 14 days?)"})
		return
	}
	c.JSON(http.StatusOK, e)
}

// GET /api/agents/:id/sessions/:sid/tool-audit?limit=N
func (h *toolAuditHandler) ListBySession(c *gin.Context) {
	log, _, ok := h.logFor(c)
	if !ok {
		return
	}
	sid := c.Param("sid")
	if sid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id required"})
		return
	}
	limit := parseLimit(c, 50, 500)
	entries, err := log.ListBySession(sid, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "total": len(entries)})
}

// GET /api/agents/:id/tool-audit/blobs/:name
// Serves a single blob file (input or result) by its filename.
func (h *toolAuditHandler) GetBlob(c *gin.Context) {
	log, _, ok := h.logFor(c)
	if !ok {
		return
	}
	name := c.Param("name")
	// Defensive: prevent path traversal. Only allow simple basenames.
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blob name"})
		return
	}
	abs := filepath.Join(log.Dir(), "blobs", name)
	c.Header("Content-Type", "application/octet-stream")
	c.File(abs)
}

// ── Cross-agent admin endpoint (used by ToolAuditView) ────────────────────

type toolAuditAggregateHandler struct {
	manager *agent.Manager
}

// GET /api/tool-audit?agentId=&sessionId=&tool=&dateFrom=&dateTo=&limit=&offset=
//
// When agentId is omitted we scan all agents; otherwise we go straight to
// the named agent's log. dateFrom/dateTo are inclusive UTC YYYY-MM-DD.
func (h *toolAuditAggregateHandler) ListAll(c *gin.Context) {
	agentID := c.Query("agentId")
	filter := toolaudit.ListFilter{
		SessionID: c.Query("sessionId"),
		ToolName:  c.Query("tool"),
	}
	if v := c.Query("dateFrom"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			filter.DateFrom = t
		}
	}
	if v := c.Query("dateTo"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			filter.DateTo = t
		}
	}
	limit := parseLimit(c, 100, 500)
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Collect entries from one or all agents.
	type entryWithAgent struct {
		toolaudit.Entry
		AgentName string `json:"agentName"`
	}
	var (
		collected []entryWithAgent
		total     int
	)
	if agentID != "" {
		ag, ok := h.manager.Get(agentID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		log := toolaudit.New(filepath.Dir(ag.WorkspaceDir))
		entries, t, err := log.ListAll(filter, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		total = t
		for _, e := range entries {
			collected = append(collected, entryWithAgent{Entry: e, AgentName: ag.Name})
		}
	} else {
		// Pull from every agent. We bypass offset/limit per-agent and
		// re-sort+page on the merged set (entries are small; agent count
		// usually under 20).
		all := []entryWithAgent{}
		for _, ag := range h.manager.List() {
			log := toolaudit.New(filepath.Dir(ag.WorkspaceDir))
			entries, _, err := log.ListAll(filter, 500, 0)
			if err != nil || len(entries) == 0 {
				continue
			}
			for _, e := range entries {
				all = append(all, entryWithAgent{Entry: e, AgentName: ag.Name})
			}
		}
		// Sort desc by ts.
		sort.Slice(all, func(i, j int) bool { return all[i].Timestamp > all[j].Timestamp })
		total = len(all)
		if offset >= total {
			collected = nil
		} else {
			end := offset + limit
			if end > total {
				end = total
			}
			collected = all[offset:end]
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"entries": collected,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

