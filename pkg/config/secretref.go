// pkg/config/secretref.go — SecretRef: runtime secret resolution for config values.
//
// # Overview
//
// Instead of storing sensitive values (API keys, bot tokens, passwords) as
// plain strings in aipanel.json, you can reference them via SecretRef objects:
//
//	Environment variable:
//	  "apiKey": {"$env": "ANTHROPIC_API_KEY"}
//
//	File contents:
//	  "botToken": {"$file": "/run/secrets/telegram_token"}
//
// Plain string values are passed through unchanged (backward-compatible).
//
// # Usage
//
// Call ResolveSecretRefs(cfg) after loading a Config from disk.
// It walks all known string fields and resolves any SecretRef it finds.
//
// To resolve a single value (e.g. inside custom code):
//
//	plain, err := ResolveValue(`{"$env": "MY_VAR"}`)
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// secretRef is the wire format for a secret reference.
// Exactly one of Env or File should be non-empty.
type secretRef struct {
	Env  string `json:"$env,omitempty"`
	File string `json:"$file,omitempty"`
}

// ResolveValue parses a JSON string value that may contain a SecretRef.
//
// Behaviour:
//   - If value is a JSON object with "$env" key → read from environment variable.
//   - If value is a JSON object with "$file" key → read from file (trimmed).
//   - Otherwise → return value unchanged.
//
// Returns an error if the referenced env var is unset or the file cannot be read.
func ResolveValue(value string) (string, error) {
	v := strings.TrimSpace(value)
	if !strings.HasPrefix(v, "{") {
		return value, nil // plain string — pass through
	}

	var ref secretRef
	if err := json.Unmarshal([]byte(v), &ref); err != nil {
		return value, nil // not a valid JSON object — treat as plain string
	}

	switch {
	case ref.Env != "":
		env := os.Getenv(ref.Env)
		if env == "" {
			return "", fmt.Errorf("secretref: environment variable %q is not set", ref.Env)
		}
		return env, nil

	case ref.File != "":
		data, err := os.ReadFile(ref.File)
		if err != nil {
			return "", fmt.Errorf("secretref: cannot read file %q: %w", ref.File, err)
		}
		return strings.TrimRight(string(data), "\n\r"), nil

	default:
		return value, nil // unknown SecretRef format — pass through
	}
}

// ResolveSecretRefs resolves all SecretRef values embedded in cfg.
//
// It walks every known string-typed credential field across the Config
// and replaces SecretRef JSON objects with the resolved plaintext value.
// Fields without a SecretRef are left unchanged.
//
// Returns the first resolution error encountered, or nil on success.
func ResolveSecretRefs(cfg *Config) error {
	// ── ProviderEntry.APIKey ─────────────────────────────────────────────────
	for i := range cfg.Providers {
		v, err := ResolveValue(cfg.Providers[i].APIKey)
		if err != nil {
			return fmt.Errorf("providers[%s].apiKey: %w", cfg.Providers[i].ID, err)
		}
		cfg.Providers[i].APIKey = v
	}

	// ── ModelEntry.APIKey (legacy / per-model key) ───────────────────────────
	for i := range cfg.Models {
		v, err := ResolveValue(cfg.Models[i].APIKey)
		if err != nil {
			return fmt.Errorf("models[%s].apiKey: %w", cfg.Models[i].ID, err)
		}
		cfg.Models[i].APIKey = v
	}

	// ── ToolEntry.APIKey ─────────────────────────────────────────────────────
	for i := range cfg.Tools {
		v, err := ResolveValue(cfg.Tools[i].APIKey)
		if err != nil {
			return fmt.Errorf("tools[%s].apiKey: %w", cfg.Tools[i].ID, err)
		}
		cfg.Tools[i].APIKey = v
	}

	// ── ChannelEntry.Config (map[string]string) ──────────────────────────────
	for i := range cfg.Channels {
		for k, val := range cfg.Channels[i].Config {
			v, err := ResolveValue(val)
			if err != nil {
				return fmt.Errorf("channels[%s].config[%s]: %w", cfg.Channels[i].ID, k, err)
			}
			cfg.Channels[i].Config[k] = v
		}
	}

	// ── AuthConfig.Token ─────────────────────────────────────────────────────
	v, err := ResolveValue(cfg.Auth.Token)
	if err != nil {
		return fmt.Errorf("auth.token: %w", err)
	}
	cfg.Auth.Token = v

	return nil
}
