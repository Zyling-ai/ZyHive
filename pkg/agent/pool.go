// Package agent — Pool manages multiple concurrent agent runners.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
	"github.com/sunhuihui6688-star/ai-panel/pkg/runner"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/tools"
)

// Pool manages multiple concurrent agent runners (one per agent).
type Pool struct {
	manager *Manager
	cfg     *config.Config
	runners map[string]*runner.Runner
	mu      sync.Mutex
}

// NewPool creates a new multi-agent runner pool.
func NewPool(cfg *config.Config, mgr *Manager) *Pool {
	return &Pool{
		manager: mgr,
		cfg:     cfg,
		runners: make(map[string]*runner.Runner),
	}
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

	if err := memory.Consolidate(ctx, store, memTree, ag.Name, convCfg, callLLM); err != nil {
		return "", err
	}
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
	toolRegistry := tools.New(ag.WorkspaceDir)
	store := session.NewStore(ag.SessionDir)

	r := runner.New(runner.Config{
		AgentID:      ag.ID,
		WorkspaceDir: ag.WorkspaceDir,
		Model:        model,
		APIKey:       apiKey,
		LLM:          llmClient,
		Tools:        toolRegistry,
		Session:      store,
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
