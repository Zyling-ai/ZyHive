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
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/browser"
	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/memory"
	"github.com/Zyling-ai/zyhive/pkg/project"
	"github.com/Zyling-ai/zyhive/pkg/runner"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/subagent"
	"github.com/Zyling-ai/zyhive/pkg/tools"
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
}

// NewPool creates a new multi-agent runner pool.
func NewPool(cfg *config.Config, mgr *Manager) *Pool {
	return &Pool{
		manager:    mgr,
		cfg:        cfg,
		runners:    make(map[string]*runner.Runner),
		browserMgr: browser.NewManager(), // lazy: browser starts only when first tool is called
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
	}
}

// SetProjectManager attaches the shared project manager so agents can access projects via tools.
func (p *Pool) SetProjectManager(mgr *project.Manager) {
	p.projectMgr = mgr
}

// GetProjectMgr returns the project manager (may be nil).
func (p *Pool) GetProjectMgr() *project.Manager {
	return p.projectMgr
}

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
}

// resolveEmbedder finds the first configured provider that supports the embeddings API.
// Returns (nil, "") when no suitable provider is found — memory_search degrades to BM25.
func (p *Pool) resolveEmbedder() (*llm.Embedder, string) {
	for _, prov := range p.cfg.Providers {
		if prov.APIKey == "" {
			continue
		}
		if !llm.SupportsEmbedding(prov.Provider) {
			continue
		}
		embedder := llm.NewEmbedder(prov.Provider, prov.BaseURL)
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
	apiKey := modelEntry.APIKey
	if apiKey == "" {
		return "", fmt.Errorf("no API key for model: %s", modelEntry.ProviderModel())
	}

	memCfg, _ := memory.ReadMemConfig(ag.WorkspaceDir)
	convCfg := memory.ConsolidateConfig{
		KeepTurns: memCfg.KeepTurns,
		FocusHint: memCfg.FocusHint,
	}

	llmClient := llm.NewClient(modelEntry.Provider, modelEntry.BaseURL)
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
	apiKey := modelEntry.APIKey
	if apiKey == "" {
		return "", fmt.Errorf("no API key configured for model: %s", model)
	}

	// Create a fresh runner for this invocation
	llmClient := llm.NewClient(modelEntry.Provider, modelEntry.BaseURL)
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	p.configureToolRegistry(toolRegistry, ag, nil)
	store := session.NewStore(ag.SessionDir)

	r := runner.New(runner.Config{
		AgentID:      ag.ID,
		WorkspaceDir: ag.WorkspaceDir,
		Model:        model,
		APIKey:       apiKey,
		LLM:          llmClient,
		Tools:        toolRegistry,
		Session:      store,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:     ag.Env,
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
	apiKey := modelEntry.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for model: %s", model)
	}

	llmClient := llm.NewClient(modelEntry.Provider, modelEntry.BaseURL)
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

	r := runner.New(runner.Config{
		AgentID:      ag.ID,
		WorkspaceDir: ag.WorkspaceDir,
		Model:        model,
		APIKey:       apiKey,
		LLM:          llmClient,
		Tools:          toolRegistry,
		Session:        store,
		SessionID:      sessionID,
		Images:         images,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:       ag.Env,
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
	apiKey := modelEntry.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for model: %s", model)
	}

	llmClient := llm.NewClient(modelEntry.Provider, modelEntry.BaseURL)
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	p.configureToolRegistry(toolRegistry, ag, nil)
	store := session.NewStore(ag.SessionDir)

	r := runner.New(runner.Config{
		AgentID:      ag.ID,
		WorkspaceDir: ag.WorkspaceDir,
		Model:        model,
		APIKey:       apiKey,
		LLM:          llmClient,
		Tools:          toolRegistry,
		Session:        store,
		SessionID:      sessionID,
		ProjectContext: p.buildProjectContext(ag.ID),
		AgentEnv:       ag.Env,
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
			apiKey := modelEntry.APIKey
			if apiKey == "" {
				out <- subagent.RunEvent{Type: "error", Error: fmt.Errorf("no API key for model: %s", resolvedModel)}
				return
			}

			llmClient := llm.NewClient(modelEntry.Provider, modelEntry.BaseURL)
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
				SessionID:       sessionID,
				ParentSessionID: parentSessionID,
				LLM:             llmClient,
				Tools:           toolRegistry,
				Session:         store,
				ProjectContext:  p.buildProjectContext(ag.ID),
				AgentEnv:        ag.Env,
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
