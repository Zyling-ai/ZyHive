// Package agent — Pool manages multiple concurrent agent runners.
package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
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

// Run executes a message against the specified agent and returns the full
// response text (collects all text_delta events).
func (p *Pool) Run(ctx context.Context, agentID, message string) (string, error) {
	ag, ok := p.manager.Get(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", agentID)
	}

	// Resolve API key
	model := ag.Model
	if model == "" {
		model = p.cfg.Models.Primary
	}
	provider := "anthropic"
	if parts := strings.SplitN(model, "/", 2); len(parts) == 2 {
		provider = parts[0]
	}
	apiKey := ""
	if p.cfg.Models.APIKeys != nil {
		apiKey = p.cfg.Models.APIKeys[provider]
	}
	if apiKey == "" {
		return "", fmt.Errorf("no API key configured for provider: %s", provider)
	}

	// Create a fresh runner for this invocation
	llmClient := llm.NewAnthropicClient()
	toolRegistry := tools.New()
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
