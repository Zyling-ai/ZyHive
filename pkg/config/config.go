// Package config handles loading and saving the aipanel.json configuration file.
// Reference: openclaw/src/config/config.ts
package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Gateway  GatewayConfig  `json:"gateway"`
	Agents   AgentsConfig   `json:"agents"`
	Models   ModelsConfig   `json:"models"`
	Channels ChannelsConfig `json:"channels"`
	Auth     AuthConfig     `json:"auth"`
}

type GatewayConfig struct {
	Port int    `json:"port"`
	Bind string `json:"bind"` // "localhost" | "lan" | "0.0.0.0"
}

type AgentsConfig struct {
	Dir string `json:"dir"` // root directory for all agents
}

type ModelsConfig struct {
	Primary   string            `json:"primary"`   // e.g. "anthropic/claude-sonnet-4-6"
	APIKeys   map[string]string `json:"apiKeys"`   // provider -> key
	Fallbacks []string          `json:"fallbacks"` // fallback models
}

type ChannelsConfig struct {
	Telegram *TelegramConfig `json:"telegram,omitempty"`
}

type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"botToken"`
}

type AuthConfig struct {
	Mode  string `json:"mode"`  // "token"
	Token string `json:"token"` // panel login token
}

// Load reads aipanel.json from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
		Gateway: GatewayConfig{Port: 8080, Bind: "lan"},
		Agents:  AgentsConfig{Dir: "./agents"},
		Models:  ModelsConfig{Primary: "anthropic/claude-sonnet-4-6"},
		Auth:    AuthConfig{Mode: "token", Token: "changeme"},
	}
}
