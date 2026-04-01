// Package agent — Pool manages multiple concurrent agent runners.
package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/browser"
	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/memory"
	"github.com/Zyling-ai/zyhive/pkg/project"
	"github.com/Zyling-ai/zyhive/pkg/runner"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/subagent"
	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/Zyling-ai/zyhive/pkg/usage"
)

// Pool manages multiple concurrent agent runners (one per agent).
type Pool struct {
	manager      *Manager
	cfg          *config.Config
	projectMgr   *project.Manager  // shared project workspace (may be nil)
	SubagentMgr  *subagent.Manager // background task manager (set after NewPool)
	workerPool   *session.WorkerPool // session worker pool for subagent broadcast (may be nil)
	browserMgr   *browser.Manager  // shared headless browser (lazy-init, may be nil if disabled)
	runners      map[string]*runner.Runner
	mu           sync.Mutex

	// messageSenderFn returns a MessageSenderFunc for the given agentID.
	// Used to inject the send_message tool so agents can proactively push notifications
	// (e.g. from isolated cron sessions with delivery=none).
	messageSenderFn func(agentID string) tools.MessageSenderFunc

	usageStore *usage.Store // records LLM API usage; nil = disabled

	cronEngine *cron.Engine // optional: enables cron_list/add/remove tools

	// Heartbeat management: one goroutine per agent with heartbeat enabled.
	hbMu      sync.Mutex
	hbCancels map[string]context.CancelFunc // agentID → cancel

	// ACP agents (external coding CLIs) — injected from config.
	acpAgents []config.ACPAgentEntry
}

// NewPool creates a new multi-agent runner pool.
func NewPool(cfg *config.Config, mgr *Manager) *Pool {
	// Browser dataDir: siblings of agents dir, e.g. ./agents/../.browser → ./.browser
	agentsDir := cfg.Agents.Dir
	if agentsDir == "" {
		agentsDir = "./agents"
	}
	return &Pool{
		manager:    mgr,
		cfg:        cfg,
		runners:    make(map[string]*runner.Runner),
		browserMgr: browser.NewManager(agentsDir), // lazy: browser + auto-download on first use
		hbCancels:  make(map[string]context.CancelFunc),
	}
}

// CloseBrowser shuts down the shared browser process (call on Pool shutdown).
func (p *Pool) CloseBrowser() {
	if p.browserMgr != nil {
		p.browserMgr.Close()
	}
}

// SetSubagentManager attaches the subagent manager to the pool.
func (p *Pool) SetSubagentManager(mgr *subagent.Manager) {
	p.SubagentMgr = mgr
	// Register the extended run function so pool can wire SharedProject + report_result.
	mgr.SetRunFuncExt(p.subagentRunFuncExt())
}

// SetWorkerPool attaches the session worker pool so subagent events can be broadcast
// to the parent session's SSE subscribers in real time.
func (p *Pool) SetWorkerPool(wp *session.WorkerPool) {
	p.workerPool = wp
	if p.SubagentMgr != nil && wp != nil {
		p.SubagentMgr.SetBroadcaster(func(sessionID, eventType string, data []byte) {
			w := wp.Get(sessionID)
			if w == nil {
				return
			}
			w.Broadcaster.Publish(session.BroadcastEvent{Type: eventType, Data: data})
		})
		// Wire ContextReadFn: searches all agent session stores for the given session ID.
		p.SubagentMgr.SetContextReader(func(sessionID string, lastN int) string {
			return p.readSessionContext(sessionID, lastN)
		})
	}
}

// SetCronEngine attaches the cron engine so agents can manage their own scheduled jobs.
func (p *Pool) SetCronEngine(e *cron.Engine) {
	p.cronEngine = e
}

// ── Heartbeat ────────────────────────────────────────────────────────────────

const defaultHeartbeatPrompt = "Read HEARTBEAT.md if it exists. Follow it strictly. Do not infer or repeat old tasks from prior chats. If nothing needs attention, reply HEARTBEAT_OK."

// StartHeartbeats reads each agent's heartbeat config and starts background goroutines.
// Call after all agents are loaded and the pool is fully wired.
func (p *Pool) StartHeartbeats() {
	for _, ag := range p.manager.List() {
		if ag.Heartbeat != nil && ag.Heartbeat.Enabled {
			p.startAgentHeartbeat(ag.ID, ag.Heartbeat)
		}
	}
}

// StopHeartbeats cancels all running heartbeat goroutines.
func (p *Pool) StopHeartbeats() {
	p.hbMu.Lock()
	defer p.hbMu.Unlock()
	for id, cancel := range p.hbCancels {
		cancel()
		delete(p.hbCancels, id)
	}
}

// RestartAgentHeartbeat stops any existing heartbeat for the agent and starts a new one
// based on the agent's current config. Call after updating an agent's heartbeat settings.
func (p *Pool) RestartAgentHeartbeat(agentID string) {
	p.hbMu.Lock()
	if cancel, ok := p.hbCancels[agentID]; ok {
		cancel()
		delete(p.hbCancels, agentID)
	}
	p.hbMu.Unlock()

	if ag, ok := p.manager.Get(agentID); ok {
		if ag.Heartbeat != nil && ag.Heartbeat.Enabled {
			p.startAgentHeartbeat(agentID, ag.Heartbeat)
		}
	}
}

func (p *Pool) startAgentHeartbeat(agentID string, cfg *config.HeartbeatConfig) {
	interval := time.Duration(cfg.IntervalMin) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	prompt := cfg.Prompt
	if prompt == "" {
		prompt = defaultHeartbeatPrompt
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.hbMu.Lock()
	p.hbCancels[agentID] = cancel
	p.hbMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Fire and forget — don't block the heartbeat goroutine.
				go func() {
					runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Minute)
					defer runCancel()
					if _, err := p.Run(runCtx, agentID, prompt); err != nil {
						log.Printf("[heartbeat] agent %s: %v", agentID, err)
					}
				}()
			}
		}
	}()
	log.Printf("[heartbeat] started for agent %s (interval=%s)", agentID, interval)
}

// ── ACP Agents ───────────────────────────────────────────────────────────────

// SetACPAgents injects the list of external coding-agent CLIs from global config.
func (p *Pool) SetACPAgents(entries []config.ACPAgentEntry) {
	p.acpAgents = entries
}

// GetACPAgents returns the configured ACP agents (read-only snapshot).
func (p *Pool) GetACPAgents() []config.ACPAgentEntry {
	return p.acpAgents
}

// readSessionContext reads the last N conversation turns from any agent's session store.
// It searches all known agents' session directories to find the matching session file.
func (p *Pool) readSessionContext(sessionID string, lastN int) string {
	agents := p.manager.List()
	for _, ag := range agents {
		store := session.NewStore(ag.SessionDir)
		msgs, _, err := store.ReadHistory(sessionID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		// Take the last lastN user+assistant pairs (each pair = 2 messages)
		start := 0
		if want := lastN * 2; len(msgs) > want {
			start = len(msgs) - want
		}
		msgs = msgs[start:]

		var sb strings.Builder
		for _, m := range msgs {
			role := "用户"
			if m.Role == "assistant" {
				role = "助手"
			}
			text := extractTextFromContent(m.Content)
			if text == "" {
				continue
			}
			if len(text) > 400 {
				text = text[:400] + "…"
			}
			sb.WriteString(role + ": " + text + "\n")
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

// extractTextFromContent extracts plain text from a message content (string or block array).
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try block array
	var blocks []session.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

// SetProjectManager attaches the shared project manager so agents can access projects via tools.
func (p *Pool) SetProjectManager(mgr *project.Manager) {
	p.projectMgr = mgr
}

// poolSessionAdapter aggregates session data across all agents for the sessions_* tools.
type poolSessionAdapter struct {
	pool *Pool
}

func (p *Pool) buildSessionAdapter() *poolSessionAdapter {
	return &poolSessionAdapter{pool: p}
}

// ListSessions aggregates sessions from all agents' session stores.
func (a *poolSessionAdapter) ListSessions(limit int) []tools.SessionSummary {
	if limit <= 0 {
		limit = 20
	}
	a.pool.manager.mu.RLock()
	agents := make([]*Agent, 0, len(a.pool.manager.agents))
	for _, ag := range a.pool.manager.agents {
		agents = append(agents, ag)
	}
	a.pool.manager.mu.RUnlock()

	var all []tools.SessionSummary
	for _, ag := range agents {
		store := session.NewStore(ag.SessionDir)
		sessions, err := store.ListSessions()
		if err != nil {
			continue
		}
		for _, s := range sessions {
			all = append(all, tools.SessionSummary{
				SessionKey:   s.ID,
				AgentID:      ag.ID,
				LastActiveMs: s.LastAt,
				LastMessage:  s.Title,
			})
		}
	}
	// Sort by LastActiveMs descending (newest first), take limit.
	sort.Slice(all, func(i, j int) bool {
		return all[i].LastActiveMs > all[j].LastActiveMs
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ReadHistory reads session history from the agent that owns the session.
func (a *poolSessionAdapter) ReadHistory(sessionKey string, limit int) ([]tools.SessionMessage, error) {
	a.pool.manager.mu.RLock()
	agents := make([]*Agent, 0, len(a.pool.manager.agents))
	for _, ag := range a.pool.manager.agents {
		agents = append(agents, ag)
	}
	a.pool.manager.mu.RUnlock()

	for _, ag := range agents {
		store := session.NewStore(ag.SessionDir)
		msgs, _, err := store.ReadHistory(sessionKey)
		if err == nil && msgs != nil {
			var result []tools.SessionMessage
			for _, m := range msgs {
				text := extractTextFromContent(m.Content)
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				result = append(result, tools.SessionMessage{
					Role:    m.Role,
					Content: text,
				})
			}
			if limit > 0 && len(result) > limit {
				result = result[len(result)-limit:]
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionKey)
}

// poolSessionSender sends a message to another agent using the pool.
type poolSessionSender struct {
	pool *Pool
}

func (s *poolSessionSender) SendToAgent(agentID, message string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := s.pool.Run(ctx, agentID, message)
	if err != nil {
		return "", fmt.Errorf("sessions_send to %s: %v", agentID, err)
	}
	return result, nil
}

// GetProjectMgr returns the project manager (may be nil).
func (p *Pool) GetProjectMgr() *project.Manager {
	return p.projectMgr
}

// SetMessageSenderFn wires the proactive message-sending capability into the pool.
// fn(agentID) returns a MessageSenderFunc that routes to the agent's active channel.
// Called from main.go after the bot pool is available.
func (p *Pool) SetMessageSenderFn(fn func(agentID string) tools.MessageSenderFunc) {
	p.messageSenderFn = fn
}

// SetUsageStore wires up the usage recorder. Call once from main after NewPool.
func (p *Pool) SetUsageStore(s *usage.Store) { p.usageStore = s }

// usageRecorder returns a recorder func for use in runner.Config.
func (p *Pool) usageRecorder() func(in, out int, provider, model, agentID string) {
	if p.usageStore == nil {
		return nil
	}
	store := p.usageStore
	return func(in, out int, provider, model, agentID string) {
		rec := usage.Record{
			ID:           usage.NewID(),
			AgentID:      agentID,
			Provider:     provider,
			Model:        model,
			InputTokens:  in,
			OutputTokens: out,
			Cost:         usage.EstimateCost(model, in, out),
			CreatedAt:    timeNow(),
		}
		_ = store.Append(rec)
	}
}

func timeNow() int64 { return time.Now().Unix() }

// configureToolRegistry applies all optional middlewares to a fresh tool registry.
// fileSender is optional; when non-nil, the send_file tool is registered.
func (p *Pool) configureToolRegistry(reg *tools.Registry, ag *Agent, fileSender channel.FileSenderFunc) {
	if p.projectMgr != nil {
		reg.WithProjectAccess(p.projectMgr)
	}
	if len(ag.Env) > 0 {
		reg.WithEnv(ag.Env)
	}
	if p.SubagentMgr != nil {
		reg.WithSubagentManager(p.SubagentMgr)
	}
	// Register agent_list so agents can discover peers.
	reg.WithAgentLister(func() []tools.AgentSummary {
		agents := p.manager.List()
		summaries := make([]tools.AgentSummary, 0, len(agents))
		for _, a := range agents {
			summaries = append(summaries, tools.AgentSummary{
				ID:          a.ID,
				Name:        a.Name,
				Description: a.Description,
			})
		}
		return summaries
	})
	if fileSender != nil {
		reg.WithFileSender(fileSender, p.cfg.Gateway.BaseURL(), p.cfg.Auth.Token)
	}
	// Allow the agent to update its own env vars via self_set_env / self_delete_env tools.
	agID := ag.ID
	reg.WithEnvUpdater(func(key, value string, remove bool) error {
		return p.manager.SetAgentEnvVar(agID, key, value, remove)
	})

	// Register memory_search tool (semantic when embedding provider available, BM25 fallback).
	memTree := memory.NewMemoryTree(ag.WorkspaceDir)
	embedder, embedAPIKey := p.resolveEmbedder()
	reg.WithMemorySearch(memTree, embedder, embedAPIKey)

	// Register browser automation tools (headless Chrome; lazy-starts on first use).
	if p.browserMgr != nil {
		reg.WithBrowser(p.browserMgr, ag.WorkspaceDir)
	}

	// Register send_message tool: lets agents proactively push notifications to users.
	// Particularly useful in isolated cron sessions (delivery=none) where the agent itself
	// decides whether to send based on content significance.
	if p.messageSenderFn != nil {
		reg.WithMessageSender(p.messageSenderFn(ag.ID))
	}

	// Register image vision tool — uses the agent's configured model for analysis.
	if modelEntry, err := p.resolveModel(ag); err == nil {
		resolvedAPIKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
		visionClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
		caller := tools.BuildVisionCaller(visionClient, modelEntry.Model, resolvedAPIKey)
		reg.WithVisionCaller(caller)
	}

	// Register web_search tool if Brave API key is configured.
	for _, tool := range p.cfg.Tools {
		if tool.Type == "brave_search" && tool.APIKey != "" {
			reg.WithWebSearch(tool.APIKey)
			break
		}
	}

	// Register cron_list/add/remove tools if cron engine is available.
	if p.cronEngine != nil {
		reg.WithCronEngine(p.cronEngine)
	}

	// Register sessions_list/history/send tools.
	// Uses a cross-agent adapter that aggregates all known agents' session stores.
	sessAdapter := p.buildSessionAdapter()
	sessionSender := &poolSessionSender{pool: p}
	reg.WithSessionTools(sessAdapter, sessAdapter, sessionSender)

	// Register ACP tools (acp_list + acp_spawn) if any ACP agents are configured.
	if len(p.acpAgents) > 0 {
		acpAgents := p.acpAgents // capture
		reg.WithACPAgents(func() []config.ACPAgentEntry { return acpAgents })
	}

	// Register Feishu tools if the agent has a Feishu channel configured.
	for _, ch := range ag.Channels {
		if ch.Type == "feishu" && ch.Enabled && ch.Config["appId"] != "" && ch.Config["appSecret"] != "" {
			reg.WithFeishu(ch.Config["appId"], ch.Config["appSecret"])
			break // only use the first Feishu channel
		}
	}

	// Apply tool permission policy (allow/deny/profile).
	// Parse raw JSON policies from config; nil-safe.
	var globalPolicy, agentPolicy *tools.ToolPolicy
	if len(p.cfg.ToolPolicyRaw) > 0 {
		var gp tools.ToolPolicy
		if err := json.Unmarshal(p.cfg.ToolPolicyRaw, &gp); err == nil {
			globalPolicy = &gp
		}
	}
	if len(ag.ToolPolicyRaw) > 0 {
		var ap tools.ToolPolicy
		if err := json.Unmarshal(ag.ToolPolicyRaw, &ap); err == nil {
			agentPolicy = &ap
		}
	}
	effectivePolicy := tools.MergePolicy(globalPolicy, agentPolicy)
	reg.ApplyPolicy(effectivePolicy)
}

// resolveEmbedder finds the first configured provider that supports the embeddings API.
// Returns (nil, "") when no suitable provider is found — memory_search degrades to BM25.
func (p *Pool) resolveEmbedder() (*llm.Embedder, string) {
	for _, prov := range p.cfg.Providers {
		// Skip providers that require an API key but have none configured.
		if prov.APIKey == "" && llm.RequiresAPIKey(prov.Provider) {
			continue
		}
		if !llm.SupportsEmbedding(prov.Provider) {
			continue
		}
		embedder := llm.NewEmbedder(prov.Provider, prov.BaseURL, prov.EmbedModel)
		if embedder != nil {
			return embedder, prov.APIKey
		}
	}
	return nil, ""
}

// buildProjectContext returns the shared project context string for system prompt injection.
func (p *Pool) buildProjectContext(agentID string) string {
	if p.projectMgr == nil {
		return ""
	}
	return runner.BuildProjectContext(p.projectMgr, agentID)
}


// resolveModel finds the model entry for an agent, falling back to default.
func (p *Pool) resolveModel(ag *Agent) (*config.ModelEntry, error) {
	// 系统 config agent 始终跟随当前默认模型，避免创建后模型不更新
	if ag.System && ag.ID == "__config__" {
		if m := p.cfg.DefaultModel(); m != nil {
			return m, nil
		}
		if len(p.cfg.Models) > 0 {
			return &p.cfg.Models[0], nil
		}
		return nil, fmt.Errorf("no model configured")
	}

	// Agent may store a modelId reference
	if ag.ModelID != "" {
		if m := p.cfg.FindModel(ag.ModelID); m != nil {
			return m, nil
		}
	}
	// Try to match by provider/model string (legacy compat)
	if ag.Model != "" {
		for i := range p.cfg.Models {
			pm := p.cfg.Models[i].ProviderModel()
			if pm == ag.Model || p.cfg.Models[i].Provider+"/"+p.cfg.Models[i].Model == ag.Model {
				return &p.cfg.Models[i], nil
			}
		}
	}
	// Fall back to default model
	if m := p.cfg.DefaultModel(); m != nil {
		return m, nil
	}
	return nil, fmt.Errorf("no model configured")
}

// ConsolidateMemory triggers memory consolidation for an agent (summarise + trim sessions).
func (p *Pool) ConsolidateMemory(ctx context.Context, agentID string) (string, error) {
	ag, ok := p.manager.Get(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", agentID)
	}
	modelEntry, err := p.resolveModel(ag)
	if err != nil {
		return "", err
	}
	apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
	if apiKey == "" {
		return "", fmt.Errorf("no API key for model: %s", modelEntry.ProviderModel())
	}

	memCfg, _ := memory.ReadMemConfig(ag.WorkspaceDir)
	convCfg := memory.ConsolidateConfig{
		KeepTurns: memCfg.KeepTurns,
		FocusHint: memCfg.FocusHint,
	}

	llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
	callLLM := func(ctx context.Context, system, user string) (string, error) {
		userJSON, _ := json.Marshal(user)
		req := &llm.ChatRequest{
			Model:  modelEntry.ProviderModel(),
			APIKey: apiKey,
			System: system,
			Messages: []llm.ChatMessage{
				{Role: "user", Content: userJSON},
			},
			MaxTokens: 2048,
		}
		ch, err := llmClient.Stream(ctx, req)
		if err != nil {
			return "", err
		}
		var resp strings.Builder
		for ev := range ch {
			if ev.Type == llm.EventTextDelta {
				resp.WriteString(ev.Text)
			}
			if ev.Type == llm.EventError && ev.Err != nil {
				return resp.String(), ev.Err
			}
		}
		return resp.String(), nil
	}

	store := session.NewStore(ag.SessionDir)
	memTree := memory.NewMemoryTree(ag.WorkspaceDir)

	nowMs := time.Now().UnixMilli()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	today := time.Now().In(loc).Format("2006-01-02")

	written, err := memory.Consolidate(ctx, store, memTree, ag.Name, convCfg, callLLM)
	if err != nil {
		log.Printf("[memory] consolidate agent=%s error: %v", agentID, err)
		_ = memory.AppendRunLog(ag.WorkspaceDir, memory.RunLogEntry{
			Timestamp: nowMs,
			Status:    "error",
			Message:   err.Error(),
		})
		return "", err
	}
	if !written {
		log.Printf("[memory] consolidate agent=%s: no new content", agentID)
		_ = memory.AppendRunLog(ag.WorkspaceDir, memory.RunLogEntry{
			Timestamp: nowMs,
			Status:    "ok",
			Message:   "无新增内容，跳过写入",
		})
		return "✅ 无新增内容", nil
	}
	log.Printf("[memory] consolidate agent=%s ok → daily/%s", agentID, today)
	_ = memory.AppendRunLog(ag.WorkspaceDir, memory.RunLogEntry{
		Timestamp: nowMs,
		Status:    "ok",
		Message:   fmt.Sprintf("已写入 memory/daily/%s.md", today),
	})
	return "✅ 记忆整理完成", nil
}

// Run executes a message against the specified agent and returns the full
// response text (collects all text_delta events).
func (p *Pool) Run(ctx context.Context, agentID, message string) (string, error) {
	// Special: memory consolidation trigger from cron
	if message == "__MEMORY_CONSOLIDATE__" {
		return p.ConsolidateMemory(ctx, agentID)
	}
	ag, ok := p.manager.Get(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", agentID)
	}

	modelEntry, err := p.resolveModel(ag)
	if err != nil {
		return "", err
	}

	model := modelEntry.ProviderModel()
	apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
	if apiKey == "" {
		return "", fmt.Errorf("no API key configured for model: %s", model)
	}

	// Create a fresh runner for this invocation
	llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	p.configureToolRegistry(toolRegistry, ag, nil)
	store := session.NewStore(ag.SessionDir)

	r := runner.New(runner.Config{
		AgentID:       ag.ID,
		WorkspaceDir:  ag.WorkspaceDir,
		Model:         model,
		APIKey:        apiKey,
		Provider:      modelEntry.Provider,
		LLM:           llmClient,
		Tools:         toolRegistry,
		SupportsTools: config.ModelSupportsTools(modelEntry),
		Session:       store,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:      ag.Env,
		UsageRecorder: p.usageRecorder(),
	})

	// Run and collect all text
	events := r.Run(ctx, message)
	var fullText strings.Builder
	for ev := range events {
		switch ev.Type {
		case "text_delta":
			fullText.WriteString(ev.Text)
		case "error":
			if ev.Error != nil {
				return fullText.String(), ev.Error
			}
		}
	}

	return fullText.String(), nil
}

// RunStreamEvents wraps RunStream output as channel.StreamEvent for the Telegram/web channel layer.
// This avoids the channel package importing the runner package directly.
// sessionID — if non-empty, history is loaded/saved under this key (enables per-chat persistent memory).
// media is an optional list of downloaded files (images/PDFs) to pass to the LLM as base64 data URIs.
// fileSender is optional; if non-nil, the agent's send_file tool is registered and can deliver files.
func (p *Pool) RunStreamEvents(ctx context.Context, agentID, message, sessionID string, media []channel.MediaInput, fileSender channel.FileSenderFunc) (<-chan channel.StreamEvent, error) {
	ag, ok := p.manager.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	modelEntry, err := p.resolveModel(ag)
	if err != nil {
		return nil, err
	}
	model := modelEntry.ProviderModel()
	apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for model: %s", model)
	}

	llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	p.configureToolRegistry(toolRegistry, ag, fileSender)
	store := session.NewStore(ag.SessionDir)

	// Convert MediaInput to base64 data URI strings for the runner.
	// Anthropic Vision only accepts: image/jpeg, image/png, image/gif, image/webp
	// (plus application/pdf for documents). Normalize and validate content types.
	var images []string
	for _, m := range media {
		if len(m.Data) == 0 {
			continue
		}
		ct := normalizeVisionContentType(m.ContentType, m.FileName)
		if ct == "" {
			log.Printf("[pool] skipping media %q: unsupported content type %q", m.FileName, m.ContentType)
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(m.Data)
		images = append(images, "data:"+ct+";base64,"+encoded)
	}

	// Ensure session exists in the index so messages are persisted and visible in the UI.
	// This is required for channel-originated sessions (Feishu, Telegram) whose IDs
	// are not auto-generated by the store (e.g. "feishu-oc_xxx" or "telegram-12345").
	if sessionID != "" {
		if _, _, err := store.GetOrCreate(sessionID, agentID); err != nil {
			log.Printf("[pool] warning: could not ensure session %q: %v", sessionID, err)
		}
	}

	r := runner.New(runner.Config{
		AgentID:        ag.ID,
		WorkspaceDir:   ag.WorkspaceDir,
		Model:          model,
		APIKey:         apiKey,
		Provider:       modelEntry.Provider,
		LLM:            llmClient,
		Tools:          toolRegistry,
		SupportsTools:  config.ModelSupportsTools(modelEntry),
		Session:        store,
		SessionID:      sessionID,
		Images:         images,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:       ag.Env,
		UsageRecorder:  p.usageRecorder(),
	})

	raw := r.Run(ctx, message)
	out := make(chan channel.StreamEvent, 32)
	go func() {
		defer close(out)
		for ev := range raw {
			switch ev.Type {
			case "text_delta":
				out <- channel.StreamEvent{Type: "text_delta", Text: ev.Text}
			case "error":
				if ev.Error != nil {
					out <- channel.StreamEvent{Type: "error", Err: ev.Error}
				}
			}
		}
		out <- channel.StreamEvent{Type: "done"}
	}()
	return out, nil
}

// RunStream executes a message against the specified agent and returns a live event channel.
// The caller must drain the channel. Used for SSE streaming (e.g. web channel).
// sessionID — if non-empty, history is loaded/saved under this key (enables per-visitor memory + compaction).
func (p *Pool) RunStream(ctx context.Context, agentID, message, sessionID string) (<-chan runner.RunEvent, error) {
	ag, ok := p.manager.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	modelEntry, err := p.resolveModel(ag)
	if err != nil {
		return nil, err
	}
	model := modelEntry.ProviderModel()
	apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for model: %s", model)
	}

	llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	p.configureToolRegistry(toolRegistry, ag, nil)
	store := session.NewStore(ag.SessionDir)

	r := runner.New(runner.Config{
		AgentID:        ag.ID,
		WorkspaceDir:   ag.WorkspaceDir,
		Model:          model,
		APIKey:         apiKey,
		Provider:       modelEntry.Provider,
		LLM:            llmClient,
		Tools:          toolRegistry,
		SupportsTools:  config.ModelSupportsTools(modelEntry),
		Session:        store,
		SessionID:      sessionID,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:       ag.Env,
		UsageRecorder:  p.usageRecorder(),
	})

	return r.Run(ctx, message), nil
}

// normalizeVisionContentType maps raw Content-Type values (from Telegram CDN or elsewhere)
// to the set accepted by Anthropic Vision: image/jpeg, image/png, image/gif, image/webp,
// or application/pdf. Returns "" for unsupported types.
func normalizeVisionContentType(ct, fileName string) string {
	// Strip parameters (e.g. "image/jpeg; charset=binary" → "image/jpeg")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	ct = strings.ToLower(strings.TrimSpace(ct))

	switch ct {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/gif":
		return "image/gif"
	case "image/webp":
		return "image/webp"
	case "application/pdf":
		return "application/pdf"
	}

	// Fall back to guessing from file extension
	lower := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	}

	// For unknown types from photo/sticker fields, default to jpeg
	if ct == "application/octet-stream" || ct == "" {
		if strings.HasSuffix(lower, ".photo") || lower == "photo.jpg" || lower == "sticker.webp" {
			if strings.HasSuffix(lower, ".webp") {
				return "image/webp"
			}
			return "image/jpeg"
		}
	}

	return "" // unsupported
}

// subagentRunFuncExt returns a RunFuncExt that uses the full Task for richer dispatch.
// It handles SharedProjectID access grants and report_result wiring.
func (p *Pool) subagentRunFuncExt() subagent.RunFuncExt {
	return func(ctx context.Context, task *subagent.Task, enrichedTask string) <-chan subagent.RunEvent {
		out := make(chan subagent.RunEvent, 32)
		go func() {
			defer close(out)

			ag, ok := p.manager.Get(task.AgentID)
			if !ok {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("agent %q not found", task.AgentID)}
				return
			}
			modelEntry, err := p.resolveModel(ag)
			if err != nil {
				out <- subagent.RunEvent{Type: "error", Error: err}
				return
			}
			resolvedModel := modelEntry.ProviderModel()
			if task.Model != "" {
				resolvedModel = task.Model
			}
			apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
			if apiKey == "" {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("no API key for model: %s", resolvedModel)}
				return
			}

			supportsTools := config.ModelSupportsTools(modelEntry)
			llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
			subSessionDir := filepath.Join(ag.SessionDir, "subagent")
			if err := os.MkdirAll(subSessionDir, 0755); err != nil {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("create subagent session dir: %w", err)}
				return
			}
			store := session.NewStore(subSessionDir)
			toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
			p.configureToolRegistry(toolRegistry, ag, nil)

			// ── SharedProjectID: grant write access ──────────────────────────────
			if task.SharedProjectID != "" && p.projectMgr != nil {
				if proj, ok := p.projectMgr.Get(task.SharedProjectID); ok {
					// Add agentID to editors; restore original editors after task completes.
					origEditors := append([]string(nil), proj.Editors...)
					newEditors := origEditors
					alreadyEditor := false
					for _, e := range origEditors {
						if e == ag.ID {
							alreadyEditor = true
							break
						}
					}
					if !alreadyEditor {
						newEditors = append(newEditors, ag.ID)
						_ = p.projectMgr.SetEditors(task.SharedProjectID, newEditors)
						defer func() {
							_ = p.projectMgr.SetEditors(task.SharedProjectID, origEditors)
						}()
					}
				}
			}

			// ── report_result tool: wire artifact callback ────────────────────────
			if p.SubagentMgr != nil {
				taskID := task.ID
				toolRegistry.WithTaskArtifactFn(func(artifacts []subagent.TaskArtifact) {
					p.SubagentMgr.UpdateArtifacts(taskID, artifacts)
				})
			}

			// ── report_to_parent broadcaster ──────────────────────────────────────
			if task.SpawnedBySession != "" && p.workerPool != nil {
				broadcastFn := func(sid, evType string, data []byte) {
					w := p.workerPool.Get(sid)
					if w == nil {
						return
					}
					w.Broadcaster.Publish(session.BroadcastEvent{Type: evType, Data: data})
				}
				toolRegistry.WithParentSession(task.SpawnedBySession, broadcastFn)
			}

			r := runner.New(runner.Config{
				AgentID:         ag.ID,
				WorkspaceDir:    ag.WorkspaceDir,
				Model:           resolvedModel,
				APIKey:          apiKey,
				Provider:        modelEntry.Provider,
				SessionID:       task.SessionID,
				ParentSessionID: task.SpawnedBySession,
				LLM:             llmClient,
				Tools:           toolRegistry,
				SupportsTools:   supportsTools,
				Session:         store,
				ProjectContext:  p.buildProjectContext(ag.ID),
				AgentEnv:        ag.Env,
				UsageRecorder:   p.usageRecorder(),
			})

			for ev := range r.Run(ctx, enrichedTask) {
				switch ev.Type {
				case "text_delta":
					out <- subagent.RunEvent{Type: "text_delta", Text: ev.Text}
				case "error":
					out <- subagent.RunEvent{Type: "error", Error: ev.Error}
				}
			}
		}()
		return out
	}
}

// SubagentRunFunc returns a RunFunc compatible with subagent.Manager.
// This lets the Pool serve as the execution backend for background tasks.
func (p *Pool) SubagentRunFunc() subagent.RunFunc {
	return func(ctx context.Context, agentID, model, sessionID, parentSessionID, task string) <-chan subagent.RunEvent {
		out := make(chan subagent.RunEvent, 32)

		go func() {
			defer close(out)

			ag, ok := p.manager.Get(agentID)
			if !ok {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("agent %q not found", agentID)}
				return
			}
			modelEntry, err := p.resolveModel(ag)
			if err != nil {
				out <- subagent.RunEvent{Type: "error", Error: err}
				return
			}
			resolvedModel := modelEntry.ProviderModel()
			if model != "" {
				resolvedModel = model
			}
			apiKey, resolvedBaseURL := config.ResolveCredentials(modelEntry, p.cfg.Providers)
			if apiKey == "" {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("no API key for model: %s", resolvedModel)}
				return
			}

			supportsToolsLegacy := config.ModelSupportsTools(modelEntry)
			llmClient := llm.NewClient(modelEntry.Provider, resolvedBaseURL)
			// Subagent gets its own isolated session store (separate dir)
			subSessionDir := filepath.Join(ag.SessionDir, "subagent")
			if err := os.MkdirAll(subSessionDir, 0755); err != nil {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("create subagent session dir: %w", err)}
				return
			}
			store := session.NewStore(subSessionDir)
			toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
			p.configureToolRegistry(toolRegistry, ag, nil)

			// Wire report_to_parent if this is a subagent with a known parent session
			if parentSessionID != "" && p.workerPool != nil {
				broadcastFn := func(sid, evType string, data []byte) {
					w := p.workerPool.Get(sid)
					if w == nil {
						return
					}
					w.Broadcaster.Publish(session.BroadcastEvent{Type: evType, Data: data})
				}
				toolRegistry.WithParentSession(parentSessionID, broadcastFn)
			}

			r := runner.New(runner.Config{
				AgentID:         ag.ID,
				WorkspaceDir:    ag.WorkspaceDir,
				Model:           resolvedModel,
				APIKey:          apiKey,
				Provider:        modelEntry.Provider,
				SessionID:       sessionID,
				ParentSessionID: parentSessionID,
				LLM:             llmClient,
				Tools:           toolRegistry,
				SupportsTools:   supportsToolsLegacy,
				Session:         store,
				ProjectContext:  p.buildProjectContext(ag.ID),
				AgentEnv:        ag.Env,
				UsageRecorder:   p.usageRecorder(),
			})

			for ev := range r.Run(ctx, task) {
				switch ev.Type {
				case "text_delta":
					out <- subagent.RunEvent{Type: "text_delta", Text: ev.Text}
				case "error":
					out <- subagent.RunEvent{Type: "error", Error: ev.Error}
				}
			}
		}()

		return out
	}
}
