// Package agent — Pool manages multiple concurrent agent runners.
package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sunhuihui6688-star/ai-panel/pkg/channel"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
	"github.com/sunhuihui6688-star/ai-panel/pkg/runner"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/tools"
)

// Pool manages multiple concurrent agent runners (one per agent).
type Pool struct {
	manager    *Manager
	cfg        *config.Config
	projectMgr *project.Manager // shared project workspace (may be nil)
	runners    map[string]*runner.Runner
	mu         sync.Mutex
}

// NewPool creates a new multi-agent runner pool.
func NewPool(cfg *config.Config, mgr *Manager) *Pool {
	return &Pool{
		manager: mgr,
		cfg:     cfg,
		runners: make(map[string]*runner.Runner),
	}
}

// SetProjectManager attaches the shared project manager so agents can access projects via tools.
func (p *Pool) SetProjectManager(mgr *project.Manager) {
	p.projectMgr = mgr
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

	llmClient := llm.NewAnthropicClient()
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
	llmClient := llm.NewAnthropicClient()
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	if p.projectMgr != nil {
		toolRegistry.WithProjectAccess(p.projectMgr)
	}
	if len(ag.Env) > 0 {
		toolRegistry.WithEnv(ag.Env)
	}
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
// media is an optional list of downloaded files (images/PDFs) to pass to the LLM as base64 data URIs.
func (p *Pool) RunStreamEvents(ctx context.Context, agentID, message string, media []channel.MediaInput) (<-chan channel.StreamEvent, error) {
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

	llmClient := llm.NewAnthropicClient()
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	if p.projectMgr != nil {
		toolRegistry.WithProjectAccess(p.projectMgr)
	}
	if len(ag.Env) > 0 {
		toolRegistry.WithEnv(ag.Env)
	}
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

	llmClient := llm.NewAnthropicClient()
	toolRegistry := tools.New(ag.WorkspaceDir, filepath.Dir(ag.WorkspaceDir), ag.ID)
	if p.projectMgr != nil {
		toolRegistry.WithProjectAccess(p.projectMgr)
	}
	if len(ag.Env) > 0 {
		toolRegistry.WithEnv(ag.Env)
	}
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
