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
	"strings"
	"sync"

	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
)

// SystemConfigAgentID is the reserved ID for the built-in configuration assistant.
// This agent cannot be deleted.
const SystemConfigAgentID = "__config__"

// Agent represents a single AI agent (employee) managed by the panel.
type Agent struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Description  string                `json:"description,omitempty"`
	Model        string                `json:"model"`          // legacy: "provider/model"
	ModelID      string                `json:"modelId"`         // references Config.Models[].ID
	Channels     []config.ChannelEntry `json:"channels,omitempty"`   // per-agent channels (own bots)
	ToolIDs      []string              `json:"toolIds,omitempty"`
	SkillIDs     []string              `json:"skillIds,omitempty"`
	AvatarColor  string                `json:"avatarColor,omitempty"`
	System       bool                  `json:"system,omitempty"` // built-in, cannot be deleted
	WorkspaceDir string                `json:"workspaceDir"`
	SessionDir   string                `json:"sessionDir"`
	Status       string                `json:"status"` // "running" | "stopped" | "idle"
}

// agentConfig is the on-disk config.json format for each agent.
type agentConfig struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Model       string                `json:"model,omitempty"`   // legacy compat
	ModelID     string                `json:"modelId,omitempty"`
	Channels    []config.ChannelEntry `json:"channels,omitempty"`   // per-agent channels
	ToolIDs     []string              `json:"toolIds,omitempty"`
	SkillIDs    []string              `json:"skillIds,omitempty"`
	AvatarColor string                `json:"avatarColor,omitempty"`
	System      bool                  `json:"system,omitempty"`
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
			Channels:     cfg.Channels,
			ToolIDs:      cfg.ToolIDs,
			SkillIDs:     cfg.SkillIDs,
			AvatarColor:  cfg.AvatarColor,
			System:       cfg.System,
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

// AgentsDir returns the root directory where all agent subdirectories live.
func (m *Manager) AgentsDir() string {
	return m.rootDir
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
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Model       string                `json:"model,omitempty"`   // legacy: "provider/model"
	ModelID     string                `json:"modelId,omitempty"`
	Channels    []config.ChannelEntry `json:"channels,omitempty"`   // per-agent channels
	ToolIDs     []string              `json:"toolIds,omitempty"`
	SkillIDs    []string              `json:"skillIds,omitempty"`
	AvatarColor string                `json:"avatarColor,omitempty"`
	System      bool                  `json:"system,omitempty"`
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
		Channels:    opts.Channels,
		ToolIDs:     opts.ToolIDs,
		SkillIDs:    opts.SkillIDs,
		AvatarColor: opts.AvatarColor,
		System:      opts.System,
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
		Channels:     opts.Channels,
		ToolIDs:      opts.ToolIDs,
		SkillIDs:     opts.SkillIDs,
		AvatarColor:  opts.AvatarColor,
		System:       opts.System,
		WorkspaceDir: workspaceDir,
		SessionDir:   sessionDir,
		Status:       "idle",
	}
	m.agents[opts.ID] = a

	return a, nil
}

// Remove unloads an agent from memory and deletes its directory on disk.
// The caller is responsible for stopping any running bots before calling this.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	// System agents (e.g. config assistant) cannot be deleted
	if ag.System {
		return fmt.Errorf("agent %q is a system agent and cannot be deleted", id)
	}

	// Delete the agent directory (workspace, sessions, convlogs, config)
	agentDir := filepath.Dir(ag.WorkspaceDir) // agents/{id}
	if err := os.RemoveAll(agentDir); err != nil {
		return fmt.Errorf("remove agent dir: %w", err)
	}

	delete(m.agents, id)
	return nil
}

// UpdateOpts holds fields that can be patched on an existing agent.
// Pointer fields: nil means "leave unchanged"; non-nil means "apply this value".
// Slice fields: nil means "leave unchanged"; non-nil (even empty) means "replace".
type UpdateOpts struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	ModelID     *string  `json:"modelId,omitempty"`
	Model       *string  `json:"model,omitempty"`
	AvatarColor *string  `json:"avatarColor,omitempty"`
	ToolIDs     []string `json:"toolIds"`
	SkillIDs    []string `json:"skillIds"`
}

// UpdateAgent patches an agent's config fields and persists to disk.
func (m *Manager) UpdateAgent(agentID string, opts UpdateOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag, ok := m.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}

	agentDir := filepath.Join(m.rootDir, agentID)
	cfgPath := filepath.Join(agentDir, "config.json")

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read config.json: %w", err)
	}
	var cfg agentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config.json: %w", err)
	}

	if opts.Name != nil {
		cfg.Name = *opts.Name
		ag.Name = *opts.Name
	}
	if opts.Description != nil {
		cfg.Description = *opts.Description
		ag.Description = *opts.Description
	}
	if opts.ModelID != nil {
		cfg.ModelID = *opts.ModelID
		ag.ModelID = *opts.ModelID
	}
	if opts.Model != nil {
		cfg.Model = *opts.Model
		ag.Model = *opts.Model
	}
	if opts.AvatarColor != nil {
		cfg.AvatarColor = *opts.AvatarColor
		ag.AvatarColor = *opts.AvatarColor
	}
	if opts.ToolIDs != nil {
		cfg.ToolIDs = opts.ToolIDs
		ag.ToolIDs = opts.ToolIDs
	}
	if opts.SkillIDs != nil {
		cfg.SkillIDs = opts.SkillIDs
		ag.SkillIDs = opts.SkillIDs
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config.json: %w", err)
	}
	return os.WriteFile(cfgPath, out, 0644)
}

// UpdateChannels replaces the channel config for an agent and persists it to disk.
func (m *Manager) UpdateChannels(agentID string, channels []config.ChannelEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag, ok := m.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}

	ag.Channels = channels

	// Read existing config.json, update channels, write back
	agentDir := filepath.Join(m.rootDir, agentID)
	cfgPath := filepath.Join(agentDir, "config.json")

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read config.json: %w", err)
	}
	var cfg agentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config.json: %w", err)
	}
	cfg.Channels = channels
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}

// UpdateChannelStatus sets the status (and optionally botName) for a specific channel.
// Called by the BotPool callback when a Telegram bot connects successfully.
func (m *Manager) UpdateChannelStatus(agentID, channelID, status, botName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag, ok := m.agents[agentID]
	if !ok {
		return
	}
	changed := false
	for i := range ag.Channels {
		if ag.Channels[i].ID == channelID {
			ag.Channels[i].Status = status
			if botName != "" {
				if ag.Channels[i].Config == nil {
					ag.Channels[i].Config = map[string]string{}
				}
				ag.Channels[i].Config["botName"] = botName
			}
			changed = true
			break
		}
	}
	if !changed {
		return
	}

	// Persist to disk
	cfgPath := filepath.Join(m.rootDir, agentID, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	var cfg agentConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	cfg.Channels = ag.Channels
	out, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(cfgPath, out, 0644)
}

// FindAgentByBotToken returns the agent and channel that use the given bot token,
// excluding excludeAgentID (so an agent can update its own token without false conflict).
// Returns nil if no other agent uses this token.
func (m *Manager) FindAgentByBotToken(token, excludeAgentID string) (*Agent, string) {
	if token == "" {
		return nil, ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ag := range m.agents {
		if ag.ID == excludeAgentID {
			continue
		}
		for _, ch := range ag.Channels {
			if ch.Type == "telegram" && ch.Config["botToken"] == token {
				return ag, ch.Name
			}
		}
	}
	return nil, ""
}

// GetAllowFrom returns the live allowedFrom list for a specific channel of an agent.
// This is called on every Telegram message by the bot, so admin approvals in the Web UI
// take effect immediately without restarting the bot process.
func (m *Manager) GetAllowFrom(agentID, channelID string) []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ag, ok := m.agents[agentID]
	if !ok {
		return nil
	}
	for _, ch := range ag.Channels {
		if ch.ID == channelID {
			raw := ch.Config["allowedFrom"]
			if raw == "" {
				return nil
			}
			var ids []int64
			for _, s := range splitTrim(raw) {
				if id, err := parseInt64(s); err == nil {
					ids = append(ids, id)
				}
			}
			return ids
		}
	}
	return nil
}

// EnsureSystemConfigAgent creates the built-in configuration assistant if it doesn't exist.
// It uses the first available model in cfg as the LLM backend.
func (m *Manager) EnsureSystemConfigAgent(cfg *config.Config) error {
	if _, exists := m.Get(SystemConfigAgentID); exists {
		return nil
	}

	// Resolve model: use first available model
	modelID := ""
	model := ""
	if len(cfg.Models) > 0 {
		modelID = cfg.Models[0].ID
		model = cfg.Models[0].ProviderModel()
	}

	a, err := m.CreateWithOpts(CreateOpts{
		ID:          SystemConfigAgentID,
		Name:        "配置助手",
		Description: "系统内置 AI 配置助手，帮助创建和配置其他 AI 成员",
		Model:       model,
		ModelID:     modelID,
		AvatarColor: "#6366f1",
		System:      true,
	})
	if err != nil {
		return fmt.Errorf("create system config agent: %w", err)
	}

	// Write SOUL.md for the config assistant
	soul := `# SOUL.md - 配置助手

我是 ZyHive 内置的配置助手，专门帮助用户设计和创建 AI 成员。

## 核心职责
- 根据用户描述，生成 AI 成员的 IDENTITY.md 和 SOUL.md
- 提供专业的角色设计建议
- 用 JSON 格式输出可一键应用的配置

## 行为准则
- 直接给出建议，不废话
- 生成的配置要实用、清晰
- 如果用户描述不清晰，先问清楚再生成
`
	soulPath := filepath.Join(a.WorkspaceDir, "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(soul), 0644); err != nil {
		log.Printf("[manager] warning: write config agent SOUL.md: %v", err)
	}

	log.Printf("[manager] created system config agent: %s", SystemConfigAgentID)
	return nil
}

func splitTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
