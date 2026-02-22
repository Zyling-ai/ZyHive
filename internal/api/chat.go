// Chat handler — streaming SSE conversation endpoint.
//
// Architecture (post-worker refactor):
//
//   Browser → POST /api/agents/:id/chat
//              ├─ Resolves session, builds RunFn closure, enqueues into SessionWorker
//              └─ Subscribes this HTTP connection to the Broadcaster (SSE stream)
//
//   Runner executes in background goroutine — independent of HTTP connections.
//   Browser disconnect stops SSE but does NOT cancel the runner.
//
//   Browser reconnects → GET /api/agents/:id/chat/stream?sessionId=...
//              └─ Subscribes to Broadcaster; gets buffered events first, then live.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
	"github.com/sunhuihui6688-star/ai-panel/pkg/runner"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/subagent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/tools"
)

var subCounter atomic.Uint64

type chatHandler struct {
	cfg         *config.Config
	manager     *agent.Manager
	projectMgr  *project.Manager
	subagentMgr *subagent.Manager
	workerPool  *session.WorkerPool
}

// Chat POST /api/agents/:id/chat
// Enqueues the message into a background SessionWorker, then SSE-streams
// the broadcaster output. Disconnecting does NOT stop the runner.
func (h *chatHandler) Chat(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var body struct {
		Message   string   `json:"message" binding:"required"`
		SessionID string   `json:"sessionId"`
		Context   string   `json:"context"`
		Scenario  string   `json:"scenario"`
		SkillID   string   `json:"skillId"`
		Images    []string `json:"images"`
		History   []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, apiKey, model, err := h.resolveModel(ag)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := session.NewStore(ag.SessionDir)
	sessionID, _, err := store.GetOrCreate(body.SessionID, ag.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session error: " + err.Error()})
		return
	}

	// Snapshot legacy history (closure capture, no aliasing)
	legacyHist := make([]struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}, len(body.History))
	copy(legacyHist, body.History)

	// agCopy is a value copy; ag pointer is stable but we copy fields we need
	agID := ag.ID
	workspaceDir := ag.WorkspaceDir
	sessionDir := ag.SessionDir
	agEnv := ag.Env
	scenario := body.Scenario
	skillID := body.SkillID
	images := append([]string{}, body.Images...)
	extraContext := body.Context

	// RunFn is called by the worker goroutine with ctx=context.Background()
	runFn := func(ctx context.Context, sid string, message string, bc *session.Broadcaster) error {
		return h.execRunner(ctx, agID, workspaceDir, sessionDir, model, apiKey,
			sid, message, extraContext, scenario, skillID, images, legacyHist, agEnv, bc)
	}

	worker := h.workerPool.GetOrCreate(sessionID)
	if err := worker.Enqueue(session.RunRequest{
		AgentID:   ag.ID,
		SessionID: sessionID,
		Message:   body.Message,
		RunFn:     runFn,
	}); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	h.pipeSSE(c, worker)
}

// StreamSession GET /api/agents/:id/chat/stream?sessionId=...
// Reconnect: subscribe to an existing session's broadcaster.
func (h *chatHandler) StreamSession(c *gin.Context) {
	id := c.Param("id")
	if _, ok := h.manager.Get(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId required"})
		return
	}
	worker := h.workerPool.Get(sessionID)
	if worker == nil {
		// Worker gone — generation finished before reconnect; signal idle
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		data, _ := json.Marshal(map[string]any{"type": "idle"})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
		return
	}
	h.pipeSSE(c, worker)
}

// SessionStatus GET /api/agents/:id/chat/status?sessionId=...
func (h *chatHandler) SessionStatus(c *gin.Context) {
	sessionID := c.Query("sessionId")
	w := h.workerPool.Get(sessionID)
	if w == nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle", "hasWorker": false})
		return
	}
	status := "idle"
	if w.IsBusy() {
		status = "generating"
	}
	c.JSON(http.StatusOK, gin.H{
		"status":         status,
		"hasWorker":      true,
		"bufferedEvents": w.Broadcaster.BufferLen(),
	})
}

// pipeSSE subscribes to the worker's broadcaster and streams events via SSE.
// Browser disconnect stops the SSE pipe but does NOT cancel the runner.
func (h *chatHandler) pipeSSE(c *gin.Context, worker *session.SessionWorker) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	subKey := fmt.Sprintf("sse-%d", subCounter.Add(1))
	ch, unsub := worker.Broadcaster.Subscribe(subKey)
	defer unsub()

	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", ev.Data)
			return ev.Type != "done" && ev.Type != "error"
		case <-c.Request.Context().Done():
			return false // browser left; runner continues
		}
	})
}

// execRunner creates and runs a runner.Runner, publishing events to bc.
// Called exclusively from inside a SessionWorker goroutine with context.Background().
func (h *chatHandler) execRunner(
	ctx context.Context,
	agentID, workspaceDir, sessionDir,
	model, apiKey,
	sessionID, message,
	extraContext, scenario, skillID string,
	images []string,
	legacyHistory []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	},
	agEnv map[string]string,
	bc *session.Broadcaster,
) error {
	llmClient := llm.NewAnthropicClient()
	store := session.NewStore(sessionDir)

	var toolRegistry *tools.Registry
	if scenario == "skill-studio" && skillID != "" {
		toolRegistry = tools.NewSkillStudio(workspaceDir, filepath.Dir(workspaceDir), agentID, skillID)
	} else {
		toolRegistry = tools.New(workspaceDir, filepath.Dir(workspaceDir), agentID)
		if h.projectMgr != nil {
			toolRegistry.WithProjectAccess(h.projectMgr)
		}
	}
	if len(agEnv) > 0 {
		toolRegistry.WithEnv(agEnv)
	}
	if h.subagentMgr != nil {
		toolRegistry.WithSubagentManager(h.subagentMgr)
		toolRegistry.WithAgentLister(func() []tools.AgentSummary {
			list := h.manager.List()
			out := make([]tools.AgentSummary, 0, len(list))
			for _, a := range list {
				if !a.System {
					out = append(out, tools.AgentSummary{ID: a.ID, Name: a.Name, Description: a.Description})
				}
			}
			return out
		})
	}
	toolRegistry.WithSessionID(sessionID)

	// Web UI file sender: generate a download link so the user can click to download.
	// Unlike Telegram (which uploads the file), the web UI just needs a URL.
	if scenario != "skill-studio" {
		baseURL := h.cfg.Gateway.BaseURL()
		authToken := h.cfg.Auth.Token
		webSender := func(filePath string) (string, error) {
			info, err := os.Stat(filePath)
			if err != nil {
				return "", fmt.Errorf("file not found: %v", err)
			}
			dlURL := baseURL + "/api/download?path=" + url.QueryEscape(filePath) +
				"&token=" + url.QueryEscape(authToken)
			sizeMB := float64(info.Size()) / (1024 * 1024)
			name := filepath.Base(filePath)
			return fmt.Sprintf("📎 **%s** (%.2f MB)\n\n下载链接：%s", name, sizeMB, dlURL), nil
		}
		toolRegistry.WithFileSender(webSender, baseURL, authToken)
	}

	var preHistory []llm.ChatMessage
	if sessionID == "" {
		for _, m := range legacyHistory {
			if m.Role == "user" || m.Role == "assistant" {
				content, _ := json.Marshal(m.Content)
				preHistory = append(preHistory, llm.ChatMessage{Role: m.Role, Content: content})
			}
		}
	}

	r := runner.New(runner.Config{
		AgentID:          agentID,
		WorkspaceDir:     workspaceDir,
		Model:            model,
		APIKey:           apiKey,
		SessionID:        sessionID,
		LLM:              llmClient,
		Tools:            toolRegistry,
		Session:          store,
		ExtraContext:     extraContext,
		Images:           images,
		PreloadedHistory: preHistory,
		ProjectContext:   runner.BuildProjectContext(h.projectMgr, agentID),
		AgentEnv:         agEnv,
	})

	for ev := range r.Run(ctx, message) {
		bc.Publish(session.BroadcastEvent{
			Type: ev.Type,
			Data: runEventToJSON(ev),
		})
	}
	return nil
}

// runEventToJSON serialises a RunEvent to SSE-ready JSON bytes.
func runEventToJSON(ev runner.RunEvent) []byte {
	m := map[string]any{"type": ev.Type}
	switch ev.Type {
	case "text_delta", "thinking_delta", "tool_result":
		m["text"] = ev.Text
	case "tool_call":
		if ev.ToolCall != nil {
			m["tool_call"] = ev.ToolCall
		}
	case "error":
		m["error"] = fmt.Sprintf("%v", ev.Error)
	case "done":
		m["sessionId"] = ev.SessionID
		m["tokenEstimate"] = ev.TokenEstimate
	}
	data, _ := json.Marshal(m)
	return data
}

// resolveModel finds the model entry and API key for an agent.
func (h *chatHandler) resolveModel(ag *agent.Agent) (*config.ModelEntry, string, string, error) {
	var me *config.ModelEntry
	if ag.ModelID != "" {
		me = h.cfg.FindModel(ag.ModelID)
	}
	if me == nil && ag.Model != "" {
		for i := range h.cfg.Models {
			if h.cfg.Models[i].ProviderModel() == ag.Model {
				me = &h.cfg.Models[i]
				break
			}
		}
	}
	if me == nil {
		me = h.cfg.DefaultModel()
	}
	if me == nil {
		return nil, "", "", fmt.Errorf("no model configured")
	}
	key := resolveKey(me)
	if key == "" {
		return nil, "", "", fmt.Errorf("no API key configured (set %s env var or add key in model settings)", envVarForProvider[me.Provider])
	}
	return me, key, me.ProviderModel(), nil
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
