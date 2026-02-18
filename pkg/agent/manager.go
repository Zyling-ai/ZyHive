// Package agent manages AI agent lifecycle and workspace files.
// Reference: pi-coding-agent/dist/core/agent-session.js, openclaw/src/agents/
//
// Each agent has:
//   - config.json  — basic metadata (id, name, model)
//   - workspace/   — IDENTITY.md, SOUL.md, MEMORY.md, memory/
//   - sessions/    — sessions.json index + *.jsonl session files
package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
)

// Agent represents a single AI agent (employee) managed by the panel.
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Model        string   `json:"model"`          // legacy: "provider/model"
	ModelID      string   `json:"modelId"`         // references Config.Models[].ID
	ChannelIDs   []string `json:"channelIds,omitempty"`
	ToolIDs      []string `json:"toolIds,omitempty"`
	SkillIDs     []string `json:"skillIds,omitempty"`
	AvatarColor  string   `json:"avatarColor,omitempty"`
	WorkspaceDir string   `json:"workspaceDir"`
	SessionDir   string   `json:"sessionDir"`
	Status       string   `json:"status"` // "running" | "stopped" | "idle"
}

// agentConfig is the on-disk config.json format for each agent.
type agentConfig struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Model       string   `json:"model,omitempty"`   // legacy compat
	ModelID     string   `json:"modelId,omitempty"`
	ChannelIDs  []string `json:"channelIds,omitempty"`
	ToolIDs     []string `json:"toolIds,omitempty"`
	SkillIDs    []string `json:"skillIds,omitempty"`
	AvatarColor string   `json:"avatarColor,omitempty"`
}

// Manager manages all agents under a root directory.
// Directory structure:
//
//	{rootDir}/{agentID}/
//	    config.json
//	    workspace/   (IDENTITY.md, SOUL.md, MEMORY.md, memory/)
//	    sessions/    (sessions.json + *.jsonl)
type Manager struct {
	rootDir string
	agents  map[string]*Agent
	mu      sync.RWMutex
}

// NewManager creates a new Manager rooted at the given directory.
func NewManager(rootDir string) *Manager {
	return &Manager{
		rootDir: rootDir,
		agents:  make(map[string]*Agent),
	}
}

// LoadAll scans rootDir for agent subdirectories and loads each agent's config.json.
// This should be called once at startup.
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return fmt.Errorf("read agents dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentDir := filepath.Join(m.rootDir, e.Name())
		cfgPath := filepath.Join(agentDir, "config.json")

		data, err := os.ReadFile(cfgPath)
		if err != nil {
			// Skip directories without config.json
			continue
		}

		var cfg agentConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		wsDir := filepath.Join(agentDir, "workspace")
		m.agents[cfg.ID] = &Agent{
			ID:           cfg.ID,
			Name:         cfg.Name,
			Description:  cfg.Description,
			Model:        cfg.Model,
			ModelID:      cfg.ModelID,
			ChannelIDs:   cfg.ChannelIDs,
			ToolIDs:      cfg.ToolIDs,
			SkillIDs:     cfg.SkillIDs,
			AvatarColor:  cfg.AvatarColor,
			WorkspaceDir: wsDir,
			SessionDir:   filepath.Join(agentDir, "sessions"),
			Status:       "idle",
		}

		// Migrate flat MEMORY.md → hierarchical memory tree if needed
		if migrated, err := memory.MigrateFromFlatMemory(wsDir); err != nil {
			log.Printf("[manager] warning: memory migration failed for agent %s: %v", cfg.ID, err)
		} else if migrated {
			log.Printf("[manager] migrated agent %s from flat MEMORY.md to memory tree", cfg.ID)
		}
	}

	return nil
}

// Get returns the agent with the given ID, or false if not found.
func (m *Manager) Get(id string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	return a, ok
}

// List returns all loaded agents.
func (m *Manager) List() []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		result = append(result, a)
	}
	return result
}

// Create creates a new agent with the given id, name, and model.
// It creates the full directory structure:
//
//	{rootDir}/{id}/config.json
//	{rootDir}/{id}/workspace/  (with IDENTITY.md, SOUL.md, MEMORY.md, memory/)
//	{rootDir}/{id}/sessions/
// CreateOpts holds the options for creating a new agent.
type CreateOpts struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Model       string   `json:"model,omitempty"`   // legacy: "provider/model"
	ModelID     string   `json:"modelId,omitempty"`
	ChannelIDs  []string `json:"channelIds,omitempty"`
	ToolIDs     []string `json:"toolIds,omitempty"`
	SkillIDs    []string `json:"skillIds,omitempty"`
	AvatarColor string   `json:"avatarColor,omitempty"`
}

func (m *Manager) Create(id, name, model string) (*Agent, error) {
	return m.CreateWithOpts(CreateOpts{ID: id, Name: name, Model: model})
}

func (m *Manager) CreateWithOpts(opts CreateOpts) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[opts.ID]; exists {
		return nil, fmt.Errorf("agent %q already exists", opts.ID)
	}

	agentDir := filepath.Join(m.rootDir, opts.ID)
	workspaceDir := filepath.Join(agentDir, "workspace")
	sessionDir := filepath.Join(agentDir, "sessions")

	// Create directory structure
	for _, dir := range []string{workspaceDir, sessionDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Write config.json
	cfg := agentConfig{
		ID:          opts.ID,
		Name:        opts.Name,
		Description: opts.Description,
		Model:       opts.Model,
		ModelID:     opts.ModelID,
		ChannelIDs:  opts.ChannelIDs,
		ToolIDs:     opts.ToolIDs,
		SkillIDs:    opts.SkillIDs,
		AvatarColor: opts.AvatarColor,
	}
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(agentDir, "config.json"), cfgData, 0644); err != nil {
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	// Initialize workspace with identity files
	role := "AI Assistant"
	if err := InitWorkspace(workspaceDir, opts.Name, role); err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}

	a := &Agent{
		ID:           opts.ID,
		Name:         opts.Name,
		Description:  opts.Description,
		Model:        opts.Model,
		ModelID:      opts.ModelID,
		ChannelIDs:   opts.ChannelIDs,
		ToolIDs:      opts.ToolIDs,
		SkillIDs:     opts.SkillIDs,
		AvatarColor:  opts.AvatarColor,
		WorkspaceDir: workspaceDir,
		SessionDir:   sessionDir,
		Status:       "idle",
	}
	m.agents[opts.ID] = a

	return a, nil
}
