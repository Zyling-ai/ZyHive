// Chat handler — streaming SSE conversation endpoint.
// Reference: openclaw/src/gateway/server-chat.ts
//
// The Chat endpoint creates a runner.Runner, calls runner.Run(ctx, message),
// and streams RunEvents back as Server-Sent Events (SSE).
// SSE format: data: {"type":"text_delta","text":"..."}\n\n
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/runner"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/tools"
)

type chatHandler struct {
	cfg     *config.Config
	manager *agent.Manager
}

// Chat POST /api/agents/:id/chat (SSE streaming)
// Accepts: {"message": "user text here"}
// Streams back SSE events as the agent processes the message.
func (h *chatHandler) Chat(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var req struct {
		Message   string   `json:"message" binding:"required"`
		SessionID string   `json:"sessionId"` // resume existing session; empty = create new
		Context   string   `json:"context"`   // extra system context (page scenario, background)
		Scenario  string   `json:"scenario"`  // label e.g. "agent-creation", "general"
		Images    []string `json:"images"`    // base64 data URIs: "data:image/png;base64,..."
		History   []struct {
			Role    string `json:"role"`    // "user" | "assistant"
			Content string `json:"content"` // plain text (legacy fallback when no sessionId)
		} `json:"history"` // legacy: client-side history, used only when sessionId is empty
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve model from global registry
	var modelEntry *config.ModelEntry
	if ag.ModelID != "" {
		modelEntry = h.cfg.FindModel(ag.ModelID)
	}
	if modelEntry == nil && ag.Model != "" {
		// Legacy compat: try matching by provider/model string
		for i := range h.cfg.Models {
			if h.cfg.Models[i].ProviderModel() == ag.Model {
				modelEntry = &h.cfg.Models[i]
				break
			}
		}
	}
	if modelEntry == nil {
		modelEntry = h.cfg.DefaultModel()
	}
	if modelEntry == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no model configured"})
		return
	}
	apiKey := resolveKey(modelEntry) // uses stored key, falls back to env var
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("no API key configured (set %s env var or add key in model settings)", envVarForProvider[modelEntry.Provider])})
		return
	}
	model := modelEntry.ProviderModel()

	// Create runner dependencies
	llmClient := llm.NewAnthropicClient()
	toolRegistry := tools.New(ag.WorkspaceDir)
	store := session.NewStore(ag.SessionDir)

	// Resolve session: resume existing or create new
	sessionID, isNewSession, err := store.GetOrCreate(req.SessionID, ag.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session error: " + err.Error()})
		return
	}
	_ = isNewSession // could be used for logging

	// Legacy fallback: convert client-side history when no server-side session history
	var preHistory []llm.ChatMessage
	if req.SessionID == "" {
		for _, h := range req.History {
			if h.Role == "user" || h.Role == "assistant" {
				content, _ := json.Marshal(h.Content)
				preHistory = append(preHistory, llm.ChatMessage{Role: h.Role, Content: content})
			}
		}
	}

	r := runner.New(runner.Config{
		AgentID:          ag.ID,
		WorkspaceDir:     ag.WorkspaceDir,
		Model:            model,
		APIKey:           apiKey,
		SessionID:        sessionID,
		LLM:              llmClient,
		Tools:            toolRegistry,
		Session:          store,
		ExtraContext:     req.Context,
		Images:           req.Images,
		PreloadedHistory: preHistory,
	})

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Run the agent and stream events back as SSE
	ctx := c.Request.Context()
	events := r.Run(ctx, req.Message)

	c.Stream(func(w io.Writer) bool {
		ev, ok := <-events
		if !ok {
			return false
		}
		sseEvent := map[string]any{"type": ev.Type}
		switch ev.Type {
		case "text_delta":
			sseEvent["text"] = ev.Text
		case "thinking_delta":
			sseEvent["text"] = ev.Text
		case "tool_call":
			if ev.ToolCall != nil {
				sseEvent["tool_call"] = ev.ToolCall
			}
		case "tool_result":
			sseEvent["text"] = ev.Text
		case "error":
			sseEvent["error"] = fmt.Sprintf("%v", ev.Error)
		case "done":
			sseEvent["sessionId"] = ev.SessionID
			sseEvent["tokenEstimate"] = ev.TokenEstimate
		}
		data, _ := json.Marshal(sseEvent)
		fmt.Fprintf(w, "data: %s\n\n", data)
		return true
	})
}

// ListSessions GET /api/agents/:id/sessions
func (h *chatHandler) ListSessions(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	store := session.NewStore(ag.SessionDir)
	sessions, err := store.ListSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sessions)
}

// GetSession GET /api/agents/:id/sessions/:sid
func (h *chatHandler) GetSession(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	sid := c.Param("sid")
	store := session.NewStore(ag.SessionDir)
	entries, err := store.ReadAll(sid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}
