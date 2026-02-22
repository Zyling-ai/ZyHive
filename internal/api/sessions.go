// Global sessions handler — aggregates conversation sessions across all agents.
package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
)

type globalSessionsHandler struct {
	cfg     *config.Config
	manager *agent.Manager
}

// SessionSummary extends SessionIndexEntry with agent display info.
type SessionSummary struct {
	session.SessionIndexEntry
	AgentName string `json:"agentName"`
	AgentID   string `json:"agentId"`
}

// ParsedMessage is a cleaned-up message for the UI.
type ParsedMessage struct {
	Role      string                   `json:"role"`                // "user" | "assistant" | "compaction"
	Text      string                   `json:"text"`                // plain text extracted from content
	Timestamp int64                    `json:"timestamp"`
	IsCompact bool                     `json:"isCompact,omitempty"` // true for compaction summary entries
	ToolCalls []session.ToolCallRecord `json:"toolCalls,omitempty"` // tool timeline (display only)
}

// List GET /api/sessions?agentId=&limit=50&q=
// Returns sessions across all agents (or filtered by agentId).
func (h *globalSessionsHandler) List(c *gin.Context) {
	filterAgent := c.Query("agentId")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 100
	}

	var all []SessionSummary

	agents := h.manager.List()
	for _, ag := range agents {
		if filterAgent != "" && ag.ID != filterAgent {
			continue
		}
		store := session.NewStore(ag.SessionDir)
		sessions, err := store.ListSessions()
		if err != nil {
			continue
		}
		for _, s := range sessions {
			all = append(all, SessionSummary{
				SessionIndexEntry: s,
				AgentName:         ag.Name,
				AgentID:           ag.ID,
			})
		}
	}

	// Sort by lastAt descending
	sort.Slice(all, func(i, j int) bool {
		return all[i].LastAt > all[j].LastAt
	})

	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []SessionSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": all, "total": len(all)})
}

// Get GET /api/sessions/:agentId/:sid
// Returns session metadata + parsed message list (for conversation viewer).
func (h *globalSessionsHandler) Get(c *gin.Context) {
	agentID := c.Param("agentId")
	sid := c.Param("sid")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	store := session.NewStore(ag.SessionDir)
	meta, exists := store.GetMeta(sid)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Parse JSONL for messages
	entries, err := store.ReadAll(sid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	messages := parseMessagesFromJSONL(entries)

	c.JSON(http.StatusOK, gin.H{
		"session":  meta,
		"messages": messages,
		"agent": gin.H{
			"id":   ag.ID,
			"name": ag.Name,
		},
	})
}

// Delete DELETE /api/sessions/:agentId/:sid
func (h *globalSessionsHandler) Delete(c *gin.Context) {
	agentID := c.Param("agentId")
	sid := c.Param("sid")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	store := session.NewStore(ag.SessionDir)
	if err := store.DeleteSession(sid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Patch PATCH /api/sessions/:agentId/:sid
// Update session title or other metadata.
func (h *globalSessionsHandler) Patch(c *gin.Context) {
	agentID := c.Param("agentId")
	sid := c.Param("sid")

	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := session.NewStore(ag.SessionDir)
	if err := store.UpdateTitle(sid, body.Title); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	meta, _ := store.GetMeta(sid)
	c.JSON(http.StatusOK, meta)
}

// parseMessagesFromJSONL converts raw JSONL lines into ParsedMessage slice.
func parseMessagesFromJSONL(lines []json.RawMessage) []ParsedMessage {
	var result []ParsedMessage

	for _, line := range lines {
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &base); err != nil {
			continue
		}

		switch base.Type {
		case "message":
			var entry struct {
				Message   struct {
					Role      string                   `json:"role"`
					Content   json.RawMessage          `json:"content"`
					ToolCalls []session.ToolCallRecord `json:"toolCalls,omitempty"`
				} `json:"message"`
				Timestamp int64 `json:"timestamp"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if entry.Message.Role != "user" && entry.Message.Role != "assistant" {
				continue
			}
			// Skip intermediate tool-only messages (tool_use / tool_result exchanges
			// saved in the agentic loop). They have no display text and the final
			// assistant message already carries the ToolCalls display records.
			if isToolOnlyContent(entry.Message.Content) {
				continue
			}
			text := extractText(entry.Message.Content)
			if text == "" && len(entry.Message.ToolCalls) == 0 {
				continue // nothing to show
			}
			result = append(result, ParsedMessage{
				Role:      entry.Message.Role,
				Text:      text,
				Timestamp: entry.Timestamp,
				ToolCalls: entry.Message.ToolCalls,
			})

		case "compaction":
			var entry struct {
				Summary   string `json:"summary"`
				Timestamp int64  `json:"timestamp"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			result = append(result, ParsedMessage{
				Role:      "compaction",
				Text:      entry.Summary,
				Timestamp: entry.Timestamp,
				IsCompact: true,
			})
		}
	}
	return result
}

// extractText pulls plain text from a message content field.
// Content can be a string or a content-block array.
func extractText(content json.RawMessage) string {
	// Try plain string
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	// Try content block array: [{type:"text", text:"..."}]
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		if len(parts) > 0 {
			return joinStrings(parts, "\n")
		}
	}
	return string(content)
}

// isToolOnlyContent returns true when ALL content blocks are tool_use or tool_result
// (i.e. no display text). These are intermediate agentic-loop messages and should
// be hidden in the UI; the final assistant message carries the ToolCalls display records.
func isToolOnlyContent(content json.RawMessage) bool {
	if len(content) == 0 || content[0] != '[' {
		return false // plain string → has text
	}
	var blocks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil || len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != "tool_use" && b.Type != "tool_result" {
			return false
		}
	}
	return true
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
