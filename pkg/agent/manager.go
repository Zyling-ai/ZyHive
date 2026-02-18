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
	"os"
	"path/filepath"
	"sync"
)

// Agent represents a single AI agent (employee) managed by the panel.
type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	WorkspaceDir string `json:"workspaceDir"`
	SessionDir   string `json:"sessionDir"`
	Status       string `json:"status"` // "running" | "stopped" | "idle"
}

// agentConfig is the on-disk config.json format for each agent.
type agentConfig struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
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

		m.agents[cfg.ID] = &Agent{
			ID:           cfg.ID,
			Name:         cfg.Name,
			Model:        cfg.Model,
			WorkspaceDir: filepath.Join(agentDir, "workspace"),
			SessionDir:   filepath.Join(agentDir, "sessions"),
			Status:       "idle",
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
func (m *Manager) Create(id, name, model string) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[id]; exists {
		return nil, fmt.Errorf("agent %q already exists", id)
	}

	agentDir := filepath.Join(m.rootDir, id)
	workspaceDir := filepath.Join(agentDir, "workspace")
	sessionDir := filepath.Join(agentDir, "sessions")

	// Create directory structure
	for _, dir := range []string{workspaceDir, sessionDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	// Write config.json
	cfg := agentConfig{ID: id, Name: name, Model: model}
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(agentDir, "config.json"), cfgData, 0644); err != nil {
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	// Initialize workspace with identity files
	role := "AI Assistant"
	if err := InitWorkspace(workspaceDir, name, role); err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}

	agent := &Agent{
		ID:           id,
		Name:         name,
		Model:        model,
		WorkspaceDir: workspaceDir,
		SessionDir:   sessionDir,
		Status:       "idle",
	}
	m.agents[id] = agent

	return agent, nil
}
