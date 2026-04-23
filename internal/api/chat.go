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
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/chatlog"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/project"
	"github.com/Zyling-ai/zyhive/pkg/runner"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/subagent"
	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/Zyling-ai/zyhive/pkg/usage"
)

var subCounter atomic.Uint64

type chatHandler struct {
	cfg         *config.Config
	manager     *agent.Manager
	projectMgr  *project.Manager
	subagentMgr *subagent.Manager
	workerPool  *session.WorkerPool
	usageStore  *usage.Store // 记录每轮 token 用量到 .usage/*.jsonl
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

	me, apiKey, model, err := h.resolveModel(ag)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	modelProvider := me.Provider
	_, resolvedBaseURL := config.ResolveCredentials(me, h.cfg.Providers)
	modelBaseURL := resolvedBaseURL

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
	modelSupportsTools := config.ModelSupportsTools(me)
	runFn := func(ctx context.Context, sid string, message string, bc *session.Broadcaster) error {
		return h.execRunner(ctx, agID, workspaceDir, sessionDir, model, apiKey,
			modelProvider, modelBaseURL,
			sid, message, extraContext, scenario, skillID, images, legacyHist, agEnv, bc,
			modelSupportsTools)
	}

	worker := h.workerPool.GetOrCreate(sessionID)

	// Clear stale replay buffer BEFORE subscribing via pipeSSE.
	// Without this, Subscribe() snapshots the previous generation's buffer
	// (which ends with "done") and replays the old response to the new request.
	// Calling StartGen() here is safe: the buffer is only used for reconnect replay;
	// live subscribers receive events via their own channels and are unaffected.
	worker.Broadcaster.StartGen()

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

	// Keepalive ticker: send SSE comment every 15s so intermediate proxies
	// (Clash, nginx, CF) don't drop the connection during long LLM processing.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", ev.Data)
			return ev.Type != "done" && ev.Type != "error"
		case <-ticker.C:
			// SSE comment — ignored by browser but resets any proxy idle timer
			fmt.Fprintf(w, ": keepalive\n\n")
			return true
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
	provider, baseURL,
	sessionID, message,
	extraContext, scenario, skillID string,
	images []string,
	legacyHistory []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	},
	agEnv map[string]string,
	bc *session.Broadcaster,
	supportsTools bool,
) error {
	llmClient := llm.NewClient(provider, baseURL)
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

	// Allow the agent to update its own env vars via self_set_env / self_delete_env.
	if scenario != "skill-studio" {
		agIDcopy := agentID
		toolRegistry.WithEnvUpdater(func(key, value string, remove bool) error {
			return h.manager.SetAgentEnvVar(agIDcopy, key, value, remove)
		})
	}

	// Web UI file sender: render files inline in the chat window.
	//   Images      → [media:path]   → AiChat.vue renders as <img>
	//   Other files → [file_card:URL|NAME|SIZE] → AiChat.vue renders as download card
	if scenario != "skill-studio" {
		baseURL := h.cfg.Gateway.BaseURL()
		authToken := h.cfg.Auth.Token
		webSender := func(filePath string) (string, error) {
			info, err := os.Stat(filePath)
			if err != nil {
				return "", fmt.Errorf("file not found: %v", err)
			}
			name := filepath.Base(filePath)
			ext := strings.ToLower(filepath.Ext(name))

			// Images: reuse the existing [media:path] rendering path in AiChat.vue
			imageExts := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}
			if imageExts[ext] {
				sizeKB := float64(info.Size()) / 1024
				return fmt.Sprintf("[media:%s] (%.1f KB)", filePath, sizeKB), nil
			}

			// Other files: file card marker rendered by AiChat.vue
			dlURL := baseURL + "/api/download?path=" + url.QueryEscape(filePath) +
				"&token=" + url.QueryEscape(authToken)
			sizeKB := float64(info.Size()) / 1024
			var sizeStr string
			if sizeKB < 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", sizeKB)
			} else {
				sizeStr = fmt.Sprintf("%.2f MB", sizeKB/1024)
			}
			return fmt.Sprintf("[file_card:%s|%s|%s]", dlURL, name, sizeStr), nil
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

	// 构造 UsageRecorder (chat API 之前遗漏此字段, 导致所有对话 output_tokens 记录为 0)
	var usageRec func(in, out int, provider, model, agentID, sessionID string)
	if h.usageStore != nil {
		us := h.usageStore
		usageRec = func(in, out int, providerIn, modelIn, agentIDIn, sessionIDIn string) {
			rec := usage.Record{
				ID:           usage.NewID(),
				AgentID:      agentIDIn,
				SessionID:    sessionIDIn,
				Provider:     providerIn,
				Model:        modelIn,
				InputTokens:  in,
				OutputTokens: out,
				Cost:         usage.EstimateCost(modelIn, in, out),
				CreatedAt:    time.Now().Unix(),
			}
			_ = us.Append(rec)
		}
	}
	// 构造能力上下文（工具体检 + WISHLIST）让 AI 感知真实能力边界
	ag, _ := h.manager.Get(agentID)
	capCtx := ""
	if ag != nil {
		capCtx = agent.BuildCapabilitiesContext(toolRegistry, ag, h.cfg, workspaceDir)
	}
	r := runner.New(runner.Config{
		AgentID:             agentID,
		WorkspaceDir:        workspaceDir,
		Model:               model,
		APIKey:              apiKey,
		Provider:            provider,
		SessionID:           sessionID,
		LLM:                 llmClient,
		Tools:               toolRegistry,
		SupportsTools:       supportsTools,
		Session:             store,
		ExtraContext:        extraContext,
		Images:              images,
		PreloadedHistory:    preHistory,
		ProjectContext:      runner.BuildProjectContext(h.projectMgr, agentID),
		AgentEnv:            agEnv,
		UsageRecorder:       usageRec,
		CapabilitiesContext: capCtx,
	})

	// Chatlog: write user message entry
	clMgr := chatlog.NewManager(workspaceDir)
	channelID := "web"
	if sessionID != "" {
		channelID = "web"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_ = clMgr.Append(chatlog.Entry{
		Ts:          now,
		SessionID:   sessionID,
		ChannelID:   channelID,
		ChannelType: "web",
		Role:        "user",
		Content:     message,
	})

	// Run and collect assistant response for chatlog
	var assistantText strings.Builder
	for ev := range r.Run(ctx, message) {
		bc.Publish(session.BroadcastEvent{
			Type: ev.Type,
			Data: runEventToJSON(ev),
		})
		if ev.Type == "text_delta" {
			assistantText.WriteString(ev.Text)
		}
	}

	// Chatlog: write assistant response entry
	if assistantText.Len() > 0 {
		_ = clMgr.Append(chatlog.Entry{
			Ts:          time.Now().UTC().Format(time.RFC3339),
			SessionID:   sessionID,
			ChannelID:   channelID,
			ChannelType: "web",
			Role:        "assistant",
			Content:     assistantText.String(),
		})
	}

	return nil
}

// runEventToJSON serialises a RunEvent to SSE-ready JSON bytes.
func runEventToJSON(ev runner.RunEvent) []byte {
	m := map[string]any{"type": ev.Type}
	switch ev.Type {
	case "text_delta", "thinking_delta":
		m["text"] = ev.Text
	case "tool_result":
		m["text"] = ev.Text
		// 并行 tool 场景下前端必须按此 ID 精准匹配, 不能靠 activeToolId 猜测
		if ev.ToolCallID != "" {
			m["tool_call_id"] = ev.ToolCallID
		}
	case "tool_call":
		if ev.ToolCall != nil {
			m["tool_call"] = ev.ToolCall
		}
	case "error":
		m["error"] = fmt.Sprintf("%v", ev.Error)
	case "usage":
		m["input_tokens"] = ev.InputTokens
		m["output_tokens"] = ev.OutputTokens
	case "done":
		m["sessionId"] = ev.SessionID
		m["tokenEstimate"] = ev.TokenEstimate
		// Only include token counts if both are non-zero (complete data)
		// Avoids overwriting a correct usage event with a partial done event
		if ev.InputTokens > 0 && ev.OutputTokens > 0 {
			m["input_tokens"] = ev.InputTokens
			m["output_tokens"] = ev.OutputTokens
		}
	case "compaction_start":
		m["tokens_before"] = ev.CompactionTokensBefore
	case "compaction_end":
		m["tokens_before"] = ev.CompactionTokensBefore
		m["tokens_after"] = ev.CompactionTokensAfter
		if ev.Text != "" {
			m["error"] = ev.Text
		}
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
	key := resolveKeyWithProviders(me, h.cfg.Providers)
	if key == "" {
		return nil, "", "", fmt.Errorf("no API key configured (set %s env var or add key in model settings)", envVarForProvider[me.Provider])
	}
	// 检查 provider 健康状态: 用了一个已测试为 error 的 provider, 提前给出明确提示
	// (不阻止调用——也许是临时故障, 但提示用户)
	if me.ProviderID != "" {
		for _, p := range h.cfg.Providers {
			if p.ID == me.ProviderID && p.Status == "error" {
				return nil, "", "", fmt.Errorf("当前模型 %q 绑定的 API Key（%s）上次测试失败（invalid/expired），请先去「模型配置」重新测试或更换 API Key", me.Name, p.Name)
			}
		}
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
		// Subagent sessions are stored in a "subagent/" subdirectory; fall back to it.
		subStore := session.NewStore(filepath.Join(ag.SessionDir, "subagent"))
		entries, err = subStore.ReadAll(sid)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
	}

	// Convert raw JSONL entries to UI-friendly {messages:[{role,text,toolCalls,isCompact}]} format.
	type UIMessage struct {
		Role      string                  `json:"role"`
		Text      string                  `json:"text"`
		ToolCalls []session.ToolCallRecord `json:"toolCalls,omitempty"`
		IsCompact bool                    `json:"isCompact,omitempty"`
	}
	type UISession struct {
		Messages []UIMessage `json:"messages"`
	}

	var msgs []UIMessage
	for _, raw := range entries {
		var base struct {
			Type string `json:"type"`
		}
		if err2 := json.Unmarshal(raw, &base); err2 != nil {
			continue
		}
		switch base.Type {
		case "message":
			var me session.MessageEntry
			if err2 := json.Unmarshal(raw, &me); err2 != nil {
				continue
			}
			// Extract displayable text from content (string or block array)
			text := extractTextFromContent(me.Message.Content)
			msgs = append(msgs, UIMessage{
				Role:      me.Message.Role,
				Text:      text,
				ToolCalls: me.Message.ToolCalls,
			})
		case "compaction":
			msgs = append(msgs, UIMessage{
				Role:      "system",
				Text:      "更早的内容已压缩",
				IsCompact: true,
			})
		}
	}
	if msgs == nil {
		msgs = []UIMessage{}
	}
	c.JSON(http.StatusOK, UISession{Messages: msgs})
}

// extractTextFromContent pulls displayable text from a message content field.
// Content can be a plain string or an array of content blocks.
func extractTextFromContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// Try plain string first
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// Try content block array
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
