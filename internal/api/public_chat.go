// Public chat handler — no admin auth required.
// Routes (no auth middleware):
//
//	GET  /pub/chat/:agentId/:channelId/info      — channel info
//	GET  /pub/chat/:agentId/:channelId/history   — load session history
//	POST /pub/chat/:agentId/:channelId/stream    — send message (WorkerPool + SSE)
//	GET  /pub/chat/:agentId/:channelId/reconnect — SSE reconnect to existing session
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/convlog"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/network"
	"github.com/Zyling-ai/zyhive/pkg/runner"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/toolaudit"
	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/gin-gonic/gin"
)

var pubSSECounter atomic.Uint64

type publicChatHandler struct {
	manager    *agent.Manager
	pool       *agent.Pool
	workerPool *session.WorkerPool
	cfg        *config.Config
	limiter    *publicAccessLimiter
}

func findWebChannelByID(ag *agent.Agent, channelID string) *config.ChannelEntry {
	for i := range ag.Channels {
		if ag.Channels[i].ID == channelID && ag.Channels[i].Type == "web" && ag.Channels[i].Enabled {
			return &ag.Channels[i]
		}
	}
	return nil
}

func findFirstWebChannel(ag *agent.Agent) *config.ChannelEntry {
	for i := range ag.Channels {
		if ag.Channels[i].Type == "web" && ag.Channels[i].Enabled {
			return &ag.Channels[i]
		}
	}
	return nil
}

func infoResponse(ag *agent.Agent, ch *config.ChannelEntry) gin.H {
	return gin.H{
		"agentId":     ag.ID,
		"channelId":   ch.ID,
		"name":        ag.Name,
		"avatarColor": ag.AvatarColor,
		"hasPassword": ch.Config["password"] != "",
		"title":       ch.Config["title"],
		"welcomeMsg":  ch.Config["welcomeMsg"],
	}
}

func checkPassword(ch *config.ChannelEntry, provided string) bool {
	return ch.Config["password"] == "" || provided == ch.Config["password"]
}

// newPublicToolRegistry returns a deliberately empty registry. Public web
// visitors can chat with the model but cannot execute host, file, project,
// skill, messaging, or self-management tools.
func newPublicToolRegistry(workspaceDir, agentID, sessionID string) *tools.Registry {
	reg := tools.New(workspaceDir, filepath.Dir(workspaceDir), agentID)
	reg.WithSessionID(sessionID)
	reg.ApplyPolicy(tools.ToolPolicy{Deny: []string{"*"}})
	return reg
}

func buildWebSessionID(channelID, sessionToken string) string {
	t := sanitizeToken(sessionToken)
	if t == "" {
		return ""
	}
	return "web-" + channelID + "-" + t
}

func publicWorkerOwner(agentID, channelID string) string {
	return "public/" + agentID + "/" + channelID
}

// ─── Info ─────────────────────────────────────────────────────────────────────

func (h *publicChatHandler) Info(c *gin.Context) {
	ag, ch := h.resolveChannel(c, c.Param("agentId"), c.Param("channelId"))
	if ag == nil {
		return
	}
	if pw := c.GetHeader("X-Chat-Password"); pw != "" && !checkPassword(ch, pw) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	c.JSON(http.StatusOK, infoResponse(ag, ch))
}

// ─── History ─────────────────────────────────────────────────────────────────

func (h *publicChatHandler) History(c *gin.Context) {
	ag, ch := h.resolveChannel(c, c.Param("agentId"), c.Param("channelId"))
	if ag == nil {
		return
	}
	if !checkPassword(ch, c.GetHeader("X-Chat-Password")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}

	sid := buildWebSessionID(ch.ID, c.Query("sessionToken"))
	if sid == "" {
		c.JSON(http.StatusOK, gin.H{"messages": []any{}, "sessionId": ""})
		return
	}

	store := session.NewStore(ag.SessionDir)
	msgs, _, err := store.ReadHistory(sid)
	if err != nil || len(msgs) == 0 {
		c.JSON(http.StatusOK, gin.H{"messages": []any{}, "sessionId": sid})
		return
	}

	type outMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	out := make([]outMsg, 0, len(msgs))
	for _, m := range msgs {
		var text string
		if err2 := json.Unmarshal(m.Content, &text); err2 != nil {
			text = string(m.Content)
		}
		if text != "" {
			out = append(out, outMsg{Role: m.Role, Content: text})
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": out, "sessionId": sid})
}

// ─── Stream ───────────────────────────────────────────────────────────────────

func (h *publicChatHandler) Stream(c *gin.Context) {
	ag, ch := h.resolveChannel(c, c.Param("agentId"), c.Param("channelId"))
	if ag == nil {
		return
	}
	if !checkPassword(ch, c.GetHeader("X-Chat-Password")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}

	var req struct {
		Message      string `json:"message"`
		SessionToken string `json:"sessionToken"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message required"})
		return
	}
	if len(req.Message) > h.limiter.maxMessageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "message too large"})
		return
	}

	sid := buildWebSessionID(ch.ID, req.SessionToken)
	if sid == "" {
		sid = fmt.Sprintf("web-%s-%d", ch.ID, pubSSECounter.Add(1))
	}
	workerOwner := publicWorkerOwner(ag.ID, ch.ID)
	release, status, err := h.limiter.admit(h.limiter.sourceKey(c), workerOwner+"\x00"+sid, time.Now())
	if err != nil {
		c.Header("Retry-After", "60")
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	releaseSSE, err := h.limiter.acquireSSE()
	if err != nil {
		release()
		c.Header("Retry-After", "5")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// Conversation log (admin-visible)
	clChID := "web-" + ch.ID
	agentDir := filepath.Dir(ag.WorkspaceDir)
	cl := convlog.New(agentDir, clChID)
	_ = cl.Append(convlog.Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339), Role: "user",
		Content: req.Message, ChannelID: clChID, ChannelType: "web", Sender: req.SessionToken,
	})

	// Snapshot fields needed in the closure (avoid data races)
	agID, wsDir, sessDir := ag.ID, ag.WorkspaceDir, ag.SessionDir
	msgCopy, sidCopy := req.Message, sid
	visitorToken := req.SessionToken

	runFn := func(ctx context.Context, sessionID, _ string, bc *session.Broadcaster) error {
		defer release()
		runCtx, cancel := context.WithTimeout(ctx, h.limiter.runTimeout)
		defer cancel()
		err := h.runPublic(runCtx, agID, wsDir, sessDir, sessionID, msgCopy, bc, cl, clChID, visitorToken)
		if err == nil && runCtx.Err() != nil {
			return runCtx.Err()
		}
		return err
	}

	worker := h.workerPool.GetOrCreate(workerOwner, sid)
	if err := worker.Enqueue(session.RunRequest{
		AgentID: ag.ID, SessionID: sid, Message: req.Message, RunFn: runFn,
	}); err != nil {
		release()
		releaseSSE()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	h.pipeSSE(c, worker, sidCopy, releaseSSE)
}

// ─── Reconnect ───────────────────────────────────────────────────────────────

func (h *publicChatHandler) Reconnect(c *gin.Context) {
	ag, ch := h.resolveChannel(c, c.Param("agentId"), c.Param("channelId"))
	if ag == nil {
		return
	}
	if !checkPassword(ch, c.GetHeader("X-Chat-Password")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	sid := c.Query("sessionId")
	if sid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId required"})
		return
	}
	if !strings.HasPrefix(sid, "web-"+ch.ID+"-") {
		c.JSON(http.StatusForbidden, gin.H{"error": "session does not belong to this channel"})
		return
	}
	worker := h.workerPool.Get(publicWorkerOwner(ag.ID, ch.ID), sid)
	if worker == nil {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		data, _ := json.Marshal(map[string]any{"type": "idle"})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
		return
	}
	releaseSSE, err := h.limiter.acquireSSE()
	if err != nil {
		c.Header("Retry-After", "5")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	h.pipeSSE(c, worker, sid, releaseSSE)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (h *publicChatHandler) resolveChannel(c *gin.Context, agentID, channelID string) (*agent.Agent, *config.ChannelEntry) {
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "deleted": true})
		return nil, nil
	}
	ch := findWebChannelByID(ag, channelID)
	if ch == nil {
		c.JSON(http.StatusGone, gin.H{"error": "web channel has been closed", "deleted": true})
		return nil, nil
	}
	return ag, ch
}

// runPublic creates a runner and publishes events to the broadcaster.
//
// visitorToken (optional) uniquely identifies the anonymous web visitor —
// if present, a contact is auto-upserted to workspace/network/contacts/ and
// its Layer-2 summary is injected via runner.Config.ExtraContext.
func (h *publicChatHandler) runPublic(
	ctx context.Context,
	agentID, workspaceDir, sessionDir string,
	sessionID, message string,
	bc *session.Broadcaster,
	cl *convlog.ConvLog, clChannelID string,
	visitorToken string,
) error {
	ag, ok := h.manager.Get(agentID)
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	// Resolve model
	var me *config.ModelEntry
	if ag.ModelID != "" {
		me = h.cfg.FindModel(ag.ModelID)
	}
	if me == nil {
		me = h.cfg.DefaultModel()
	}
	if me == nil {
		return fmt.Errorf("no model configured")
	}
	apiKey := resolveKeyWithProviders(me, h.cfg.Providers)
	_, resolvedBaseURL := config.ResolveCredentials(me, h.cfg.Providers)
	if apiKey == "" && llm.RequiresAPIKey(me.Provider) {
		return fmt.Errorf("no API key for model %s", me.ProviderModel())
	}

	llmClient := llm.NewClient(me.Provider, resolvedBaseURL)
	store := session.NewStore(sessionDir)
	toolRegistry := newPublicToolRegistry(workspaceDir, agentID, sessionID)

	// ── Network: auto-upsert web visitor as contact + inject Layer-2 summary.
	var extraCtx string
	if visitorToken != "" {
		nstore := network.NewStore(workspaceDir)
		// Bug 2: 统一走 FallbackDisplayName — 对 Web 访客这里"Web 访客"作为
		// 首选名, token 前缀兜底（visitorToken 可能是长 sid）.
		name := network.FallbackDisplayName(visitorToken, "Web 访客")
		if _, nerr := nstore.Resolve(network.SourceWeb, visitorToken, name); nerr == nil {
			extraCtx = nstore.Summary(network.MakeID(network.SourceWeb, visitorToken))
		}
	}

	r := runner.New(runner.Config{
		AgentID:       agentID,
		WorkspaceDir:  workspaceDir,
		Model:         me.ProviderModel(),
		APIKey:        apiKey,
		SessionID:     sessionID,
		LLM:           llmClient,
		Tools:         toolRegistry,
		SupportsTools: false,
		Session:       store,
		ExtraContext:  extraCtx,
		ToolAudit:     toolaudit.New(filepath.Dir(workspaceDir)),
	})

	var fullResponse strings.Builder
	for ev := range r.Run(ctx, message) {
		switch ev.Type {
		case "text_delta":
			fullResponse.WriteString(ev.Text)
			data, _ := json.Marshal(map[string]any{"type": "text_delta", "text": ev.Text})
			bc.Publish(session.BroadcastEvent{Type: "text_delta", Data: data})
		case "tool_call":
			data := runEventToJSON(ev)
			bc.Publish(session.BroadcastEvent{Type: "tool_call", Data: data})
		case "tool_result":
			data := runEventToJSON(ev)
			bc.Publish(session.BroadcastEvent{Type: "tool_result", Data: data})
		case "error":
			if ev.Error != nil {
				data, _ := json.Marshal(map[string]any{"type": "error", "error": ev.Error.Error()})
				bc.Publish(session.BroadcastEvent{Type: "error", Data: data})
			}
		case "done":
			bc.Publish(session.BroadcastEvent{Type: "done", Data: runEventToJSON(ev)})
		}
	}

	// Log response
	if resp := fullResponse.String(); resp != "" {
		_ = cl.Append(convlog.Entry{
			Timestamp: time.Now().UTC().Format(time.RFC3339), Role: "assistant",
			Content: resp, ChannelID: clChannelID, ChannelType: "web",
		})
	}

	return nil
}

// pipeSSE streams broadcaster events to the client, injecting sessionId into done.
func (h *publicChatHandler) pipeSSE(
	c *gin.Context,
	worker *session.SessionWorker,
	sessionID string,
	release func(),
) {
	defer release()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	subKey := fmt.Sprintf("pub-%d", pubSSECounter.Add(1))
	ch, unsub := worker.Broadcaster.Subscribe(subKey)
	defer unsub()

	c.Stream(func(w io.Writer) bool {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			if ev.Type == "done" || ev.Type == "error" {
				data := publicTerminalData(ev, sessionID)
				fmt.Fprintf(w, "data: %s\n\n", data)
				return false
			}
			fmt.Fprintf(w, "data: %s\n\n", ev.Data)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func publicTerminalData(ev session.BroadcastEvent, sessionID string) []byte {
	payload := map[string]any{"type": ev.Type}
	_ = json.Unmarshal(ev.Data, &payload)
	payload["type"] = ev.Type
	payload["sessionId"] = sessionID
	data, _ := json.Marshal(payload)
	return data
}

// ─── Legacy ──────────────────────────────────────────────────────────────────

func (h *publicChatHandler) InfoLegacy(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "deleted": true})
		return
	}
	ch := findFirstWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusGone, gin.H{"error": "channel closed", "deleted": true})
		return
	}
	if pw := c.GetHeader("X-Chat-Password"); pw != "" && !checkPassword(ch, pw) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	c.JSON(http.StatusOK, infoResponse(ag, ch))
}

func (h *publicChatHandler) StreamLegacy(c *gin.Context) {
	agentID := c.Param("agentId")
	ag, ok := h.manager.Get(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ch := findFirstWebChannel(ag)
	if ch == nil {
		c.JSON(http.StatusGone, gin.H{"error": "channel closed"})
		return
	}
	// Inject channelId param and delegate to primary handler
	c.Params = append(c.Params, gin.Param{Key: "channelId", Value: ch.ID})
	h.Stream(c)
}

func sanitizeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

var _ = agent.Agent{}
