// Package config handles loading and saving the aipanel.json configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config is the top-level configuration.
// Models/Channels/Tools/Skills are global registries; agents reference them by ID.
type Config struct {
	Gateway  GatewayConfig  `json:"gateway"`
	Agents   AgentsConfig   `json:"agents"`
	Models   []ModelEntry   `json:"models"`   // global model registry
	Channels []ChannelEntry `json:"channels"` // global channel registry
	Tools    []ToolEntry    `json:"tools"`    // global capability registry
	Skills   []SkillEntry   `json:"skills"`   // installed skills
	Auth     AuthConfig     `json:"auth"`
}

type GatewayConfig struct {
	Port      int    `json:"port"`
	Bind      string `json:"bind"`
	PublicURL string `json:"publicUrl,omitempty"` // e.g. "https://zyhive.example.com"
}

// BaseURL returns the canonical server base URL (no trailing slash).
// Uses PublicURL if configured, otherwise falls back to http://localhost:PORT.
func (g *GatewayConfig) BaseURL() string {
	if g.PublicURL != "" {
		return strings.TrimRight(g.PublicURL, "/")
	}
	port := g.Port
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

type AgentsConfig struct {
	Dir string `json:"dir"`
}

// ModelEntry — one configured LLM provider/model
type ModelEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"` // "anthropic" | "openai" | "deepseek" | "openrouter" | "custom"
	Model     string `json:"model"`    // "claude-sonnet-4-6"
	APIKey    string `json:"apiKey"`
	BaseURL   string `json:"baseUrl,omitempty"` // API base URL; empty = provider default
	IsDefault bool   `json:"isDefault"`
	Status    string `json:"status"` // "ok" | "error" | "untested"
}

// ChannelEntry — one messaging channel
type ChannelEntry struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "telegram" | "imessage" | "whatsapp"
	Config  map[string]string `json:"config"`
	Enabled bool              `json:"enabled"`
	Status  string            `json:"status"`
}

// ToolEntry — one capability/tool API key
type ToolEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // "brave_search" | "elevenlabs" | "custom"
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl,omitempty"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

// SkillEntry — an installed skill
type SkillEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
}

// AgentConfig is the on-disk config.json per agent. References global entries by ID.
type AgentConfig struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	ModelID     string         `json:"modelId"`
	Channels    []ChannelEntry `json:"channels,omitempty"`   // per-agent channel config (own bot tokens)
	ToolIDs     []string       `json:"toolIds,omitempty"`
	SkillIDs    []string       `json:"skillIds,omitempty"`
	AvatarColor string         `json:"avatarColor,omitempty"`
}

type AuthConfig struct {
	Mode  string `json:"mode"`
	Token string `json:"token"`
}

// --- Legacy compat types (for migration) ---

type legacyConfig struct {
	Gateway  GatewayConfig       `json:"gateway"`
	Agents   AgentsConfig        `json:"agents"`
	Models   json.RawMessage     `json:"models"`
	Channels json.RawMessage     `json:"channels"`
	Auth     AuthConfig          `json:"auth"`
}

type legacyModelsConfig struct {
	Primary   string            `json:"primary"`
	APIKeys   map[string]string `json:"apiKeys"`
	Fallbacks []string          `json:"fallbacks"`
}

type legacyChannelsConfig struct {
	Telegram *legacyTelegramConfig `json:"telegram,omitempty"`
}

type legacyTelegramConfig struct {
	Enabled      bool    `json:"enabled"`
	BotToken     string  `json:"botToken"`
	DefaultAgent string  `json:"defaultAgent,omitempty"`
	AllowedFrom  []int64 `json:"allowedFrom,omitempty"`
}

// Load reads aipanel.json from disk, auto-migrating legacy format.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Always read via legacyConfig first (uses json.RawMessage for models/channels
	// so it handles both old object-format and new array-format safely).
	var raw legacyConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Detect legacy format: if models is an object with "primary" field
	if raw.Models != nil {
		var lm legacyModelsConfig
		if json.Unmarshal(raw.Models, &lm) == nil && lm.Primary != "" {
			// Migrate legacy → new format and persist
			cfg := migrateFromLegacy(raw, lm)
			_ = Save(path, &cfg)
			return &cfg, nil
		}
	}

	// New format: unmarshal directly into Config
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func migrateFromLegacy(raw legacyConfig, lm legacyModelsConfig) Config {
	cfg := Config{
		Gateway: raw.Gateway,
		Agents:  raw.Agents,
		Auth:    raw.Auth,
		Models:  []ModelEntry{},
		Channels: []ChannelEntry{},
		Tools:   []ToolEntry{},
		Skills:  []SkillEntry{},
	}

	// Migrate models
	for provider, key := range lm.APIKeys {
		model := ""
		name := ""
		id := ""
		switch provider {
		case "anthropic":
			model = "claude-sonnet-4-6"
			name = "Claude Sonnet 4"
			id = "anthropic-sonnet-4"
		case "openai":
			model = "gpt-4o"
			name = "GPT-4o"
			id = "openai-gpt4o"
		case "deepseek":
			model = "deepseek-chat"
			name = "DeepSeek V3"
			id = "deepseek-v3"
		default:
			id = provider
			name = provider
			model = provider
		}
		entry := ModelEntry{
			ID:       id,
			Name:     name,
			Provider: provider,
			Model:    model,
			APIKey:   key,
			IsDefault: lm.Primary != "" && (provider+"/"+model == lm.Primary || (provider == "anthropic" && lm.Primary == "anthropic/claude-sonnet-4-6")),
			Status:   "untested",
		}
		cfg.Models = append(cfg.Models, entry)
	}

	// Migrate telegram channel
	if raw.Channels != nil {
		var lc legacyChannelsConfig
		if json.Unmarshal(raw.Channels, &lc) == nil && lc.Telegram != nil {
			t := lc.Telegram
			chConfig := map[string]string{
				"botToken": t.BotToken,
			}
			if t.DefaultAgent != "" {
				chConfig["defaultAgent"] = t.DefaultAgent
			}
			cfg.Channels = append(cfg.Channels, ChannelEntry{
				ID:      "telegram-main",
				Name:    "Telegram Bot",
				Type:    "telegram",
				Config:  chConfig,
				Enabled: t.Enabled,
				Status:  "untested",
			})
		}
	}

	return cfg
}

// Save writes config back to disk.
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Default returns sensible defaults for first run.
func Default() *Config {
	return &Config{
		Gateway:  GatewayConfig{Port: 8080, Bind: "lan"},
		Agents:   AgentsConfig{Dir: "./agents"},
		Models:   []ModelEntry{},
		Channels: []ChannelEntry{},
		Tools:    []ToolEntry{},
		Skills:   []SkillEntry{},
		Auth:     AuthConfig{Mode: "token", Token: "changeme"},
	}
}

// FindModel returns the model entry by ID.
func (c *Config) FindModel(id string) *ModelEntry {
	for i := range c.Models {
		if c.Models[i].ID == id {
			return &c.Models[i]
		}
	}
	return nil
}

// DefaultModel returns the first model marked as default, or the first model.
func (c *Config) DefaultModel() *ModelEntry {
	for i := range c.Models {
		if c.Models[i].IsDefault {
			return &c.Models[i]
		}
	}
	if len(c.Models) > 0 {
		return &c.Models[0]
	}
	return nil
}

// ModelProviderKey returns the provider and API key for the given model entry.
// This is used by the chat/runner system to construct the LLM client.
func (m *ModelEntry) ProviderModel() string {
	return m.Provider + "/" + m.Model
}
