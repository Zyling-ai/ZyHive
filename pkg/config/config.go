// Package config handles loading and saving the aipanel.json configuration file.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// CurrentConfigVersion is the latest config schema version.
// Bump this when the config format changes; add a migration in applyMigrations().
const CurrentConfigVersion = 3

// Config is the top-level configuration.
// Models/Channels/Tools/Skills are global registries; agents reference them by ID.
type Config struct {
	ConfigVersion int              `json:"configVersion,omitempty"` // schema version; 0 = pre-versioning
	Gateway   GatewayConfig    `json:"gateway"`
	Agents    AgentsConfig     `json:"agents"`
	Providers []ProviderEntry  `json:"providers,omitempty"` // API Key 注册表（每个厂商一条）
	Models    []ModelEntry     `json:"models"`              // global model registry
	Channels  []ChannelEntry   `json:"channels"`            // global channel registry
	Tools     []ToolEntry      `json:"tools"`               // global capability registry
	Skills    []SkillEntry     `json:"skills"`              // installed skills
	Auth      AuthConfig       `json:"auth"`
}

// ProviderEntry 代表一个大模型服务商的凭据配置。
// 一个厂商只需配置一次 APIKey，旗下所有模型共享使用。
type ProviderEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`              // 用户自定义名称，如"我的 DeepSeek"
	Provider string `json:"provider"`          // "anthropic" | "openai" | "deepseek" | ...
	APIKey   string `json:"apiKey"`
	BaseURL  string `json:"baseUrl,omitempty"` // 留空 = 使用 provider 默认地址
	Status   string `json:"status"`            // "ok" | "error" | "untested"
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
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`              // "anthropic" | "openai" | "deepseek" | "openrouter" | "custom"
	Model        string `json:"model"`                 // "claude-sonnet-4-6"
	ProviderID   string `json:"providerId,omitempty"`  // 引用 ProviderEntry.ID（优先使用 provider 的 apiKey）
	APIKey       string `json:"apiKey,omitempty"`      // 兼容旧配置；新建模型用 ProviderID
	BaseURL      string `json:"baseUrl,omitempty"`     // API base URL；空 = 用 provider/default
	IsDefault    bool   `json:"isDefault"`
	Status       string `json:"status"`                  // "ok" | "error" | "untested"
	SupportsTools *bool `json:"supportsTools,omitempty"` // nil=自动判断; true/false=手动指定
}

// ResolveCredentials 从模型或关联 provider 中取出 (apiKey, baseURL)。
// 优先级：model.ProviderID → model.APIKey（向后兼容）。
func ResolveCredentials(m *ModelEntry, providers []ProviderEntry) (apiKey, baseURL string) {
	if m.ProviderID != "" {
		for _, p := range providers {
			if p.ID == m.ProviderID {
				return p.APIKey, firstNonEmpty(m.BaseURL, p.BaseURL)
			}
		}
	}
	return m.APIKey, m.BaseURL
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// noToolPatterns 是已知不支持工具调用的模型名称关键词（子串匹配，忽略大小写）。
var noToolPatterns = []string{
	"reasoner",    // deepseek-reasoner
	"o1-mini",     // openai o1-mini
	"o1-preview",  // openai o1-preview
	"o1-2024",     // openai o1 系列
}

// ModelSupportsTools 判断某个 ModelEntry 是否支持工具调用。
// 优先使用手动配置，其次按模型名自动判断。
func ModelSupportsTools(m *ModelEntry) bool {
	if m.SupportsTools != nil {
		return *m.SupportsTools
	}
	name := strings.ToLower(m.Model)
	for _, p := range noToolPatterns {
		if strings.Contains(name, p) {
			return false
		}
	}
	return true
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

// ── Config Migration System ────────────────────────────────────────────────────
//
// How to add a new migration:
//   1. Bump CurrentConfigVersion (e.g. 1 → 2)
//   2. Add a case in applyMigrations() for the new version
//   3. Write migration logic; set cfg.ConfigVersion = <new version> at the end
//
// Migrations run at startup (Load) after binary update, and are safe to run repeatedly.

func applyMigrations(cfg *Config, path string) {
	if cfg.ConfigVersion >= CurrentConfigVersion {
		return
	}
	migrated := false

	// ── v0 → v1 ──────────────────────────────────────────────────────────────
	// Changes: ensure every ModelEntry/ChannelEntry/ToolEntry/SkillEntry has a
	// non-empty ID; fill in missing default values introduced in v0.9.x.
	if cfg.ConfigVersion < 1 {
		log.Printf("[config] migrating v%d → v1", cfg.ConfigVersion)

		// Ensure all ModelEntry IDs are non-empty
		for i := range cfg.Models {
			if cfg.Models[i].ID == "" {
				cfg.Models[i].ID = randID()
				log.Printf("[config]   auto-assigned model ID: %s (%s)", cfg.Models[i].ID, cfg.Models[i].Name)
			}
			// Ensure Status has a value
			if cfg.Models[i].Status == "" {
				cfg.Models[i].Status = "untested"
			}
		}

		// Ensure all ChannelEntry IDs are non-empty
		for i := range cfg.Channels {
			if cfg.Channels[i].ID == "" {
				cfg.Channels[i].ID = randID()
				log.Printf("[config]   auto-assigned channel ID: %s (%s)", cfg.Channels[i].ID, cfg.Channels[i].Name)
			}
			if cfg.Channels[i].Status == "" {
				cfg.Channels[i].Status = "untested"
			}
		}

		// Ensure all ToolEntry IDs are non-empty
		for i := range cfg.Tools {
			if cfg.Tools[i].ID == "" {
				cfg.Tools[i].ID = randID()
				log.Printf("[config]   auto-assigned tool ID: %s (%s)", cfg.Tools[i].ID, cfg.Tools[i].Name)
			}
		}

		// Ensure gateway.bind default
		if cfg.Gateway.Bind == "" {
			cfg.Gateway.Bind = "lan"
		}

		// Ensure auth.mode default
		if cfg.Auth.Mode == "" {
			cfg.Auth.Mode = "token"
		}

		cfg.ConfigVersion = 1
		migrated = true
	}

	// ── v1 → v2 ──────────────────────────────────────────────────────────────
	// Changes (v0.9.18+):
	//   - Auto-set supportsTools=false for models matching noToolPatterns
	//   - Ensure at least one model has isDefault=true (auto-pick first if none)
	//   - Normalize baseUrl: strip trailing /v1 duplicate if present
	if cfg.ConfigVersion < 2 {
		log.Printf("[config] migrating v1 → v2")

		// 标记不支持工具调用的模型
		for i := range cfg.Models {
			if cfg.Models[i].SupportsTools == nil {
				supports := ModelSupportsTools(&cfg.Models[i])
				if !supports {
					f := false
					cfg.Models[i].SupportsTools = &f
					log.Printf("[config]   marked supportsTools=false: %s", cfg.Models[i].Name)
				}
			}
		}

		// 确保至少有一个默认模型
		hasDefault := false
		for _, m := range cfg.Models {
			if m.IsDefault {
				hasDefault = true
				break
			}
		}
		if !hasDefault && len(cfg.Models) > 0 {
			// 优先选第一个 Anthropic 模型，否则第一个
			chosen := 0
			for i, m := range cfg.Models {
				if m.Provider == "anthropic" {
					chosen = i
					break
				}
			}
			cfg.Models[chosen].IsDefault = true
			log.Printf("[config]   auto-set default model: %s", cfg.Models[chosen].Name)
		}

		cfg.ConfigVersion = 2
		migrated = true
	}

	// ── v2 → v3 ──────────────────────────────────────────────────────────────
	// Changes (v0.9.21+):
	//   - Extract apiKey from ModelEntry into shared ProviderEntry
	//   - Set model.ProviderID; clear model.APIKey (key now lives on provider)
	if cfg.ConfigVersion < 3 {
		log.Printf("[config] migrating v2 → v3: extracting provider API keys")

		// 为每个唯一的 (provider, apiKey, baseURL) 组合创建 ProviderEntry
		type provKey struct{ provider, apiKey, baseURL string }
		providerMap := map[provKey]string{} // provKey → provider ID

		// 先对现有 ProviderEntry 建索引（防止重复创建）
		for _, p := range cfg.Providers {
			k := provKey{p.Provider, p.APIKey, p.BaseURL}
			if _, exists := providerMap[k]; !exists {
				providerMap[k] = p.ID
			}
		}

		for i := range cfg.Models {
			m := &cfg.Models[i]
			if m.APIKey == "" || m.ProviderID != "" {
				continue // 无 apiKey 或已有 ProviderID，跳过
			}
			k := provKey{m.Provider, m.APIKey, m.BaseURL}
			pid, exists := providerMap[k]
			if !exists {
				// 新建 ProviderEntry
				pid = randID()
				providerName := providerDisplayName(m.Provider)
				cfg.Providers = append(cfg.Providers, ProviderEntry{
					ID:      pid,
					Name:    providerName,
					Provider: m.Provider,
					APIKey:  m.APIKey,
					BaseURL: m.BaseURL,
					Status:  "untested",
				})
				providerMap[k] = pid
				log.Printf("[config]   created provider: %s (%s)", providerName, m.Provider)
			}
			m.ProviderID = pid
			m.APIKey = ""  // key 已迁移到 ProviderEntry
			log.Printf("[config]   model %q linked to provider %s", m.Name, pid)
		}

		cfg.ConfigVersion = 3
		migrated = true
	}

	// ── future migrations go here ─────────────────────────────────────────────
	// if cfg.ConfigVersion < 3 { ... cfg.ConfigVersion = 3; migrated = true }

	if migrated {
		log.Printf("[config] migration complete → v%d, saving", cfg.ConfigVersion)
		if err := Save(path, cfg); err != nil {
			log.Printf("[config] warning: failed to save migrated config: %v", err)
		}
	}
}

// randID generates a short random hex ID (8 bytes = 16 hex chars).
func randID() string { return RandID() }

// RandID generates a random hex ID (exported for use by other packages).
func RandID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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

	// Apply any pending schema migrations and persist if changed
	applyMigrations(&cfg, path)

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

// FindProvider returns the ProviderEntry for the given ID.
func (c *Config) FindProvider(id string) *ProviderEntry {
	for i := range c.Providers {
		if c.Providers[i].ID == id {
			return &c.Providers[i]
		}
	}
	return nil
}

// providerDisplayName returns a human-friendly name for a provider type.
func providerDisplayName(provider string) string {
	names := map[string]string{
		"anthropic":  "Anthropic",
		"openai":     "OpenAI",
		"deepseek":   "DeepSeek",
		"openrouter": "OpenRouter",
		"zhipu":      "智谱 AI",
		"kimi":       "月之暗面 (Kimi)",
		"minimax":    "MiniMax",
		"qwen":       "阿里通义千问",
		"custom":     "自定义",
	}
	if n, ok := names[provider]; ok {
		return n
	}
	return provider
}
