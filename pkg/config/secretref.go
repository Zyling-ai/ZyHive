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

// preserveSecretRefs copies unchanged credential references from the on-disk
// config into the serialized candidate. Runtime snapshots keep resolved values.
func preserveSecretRefs(path string, before, candidate *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var disk Config
	if json.Unmarshal(data, &disk) != nil {
		return
	}

	beforeProviders := providersByID(before.Providers)
	for i := range candidate.Providers {
		raw, ok := providerByID(disk.Providers, candidate.Providers[i].ID)
		old, existed := beforeProviders[candidate.Providers[i].ID]
		if ok && existed && candidate.Providers[i].APIKey == old.APIKey && isSecretRef(raw.APIKey) {
			candidate.Providers[i].APIKey = raw.APIKey
		}
	}
	beforeModels := modelsByID(before.Models)
	for i := range candidate.Models {
		raw, ok := modelByID(disk.Models, candidate.Models[i].ID)
		old, existed := beforeModels[candidate.Models[i].ID]
		if ok && existed && candidate.Models[i].APIKey == old.APIKey && isSecretRef(raw.APIKey) {
			candidate.Models[i].APIKey = raw.APIKey
		}
	}
	beforeTools := toolsByID(before.Tools)
	for i := range candidate.Tools {
		raw, ok := toolByID(disk.Tools, candidate.Tools[i].ID)
		old, existed := beforeTools[candidate.Tools[i].ID]
		if ok && existed && candidate.Tools[i].APIKey == old.APIKey && isSecretRef(raw.APIKey) {
			candidate.Tools[i].APIKey = raw.APIKey
		}
	}
	beforeChannels := channelsByID(before.Channels)
	for i := range candidate.Channels {
		raw, ok := channelByID(disk.Channels, candidate.Channels[i].ID)
		old, existed := beforeChannels[candidate.Channels[i].ID]
		if !ok || !existed {
			continue
		}
		for key, rawValue := range raw.Config {
			if isSecretRef(rawValue) && candidate.Channels[i].Config[key] == old.Config[key] {
				candidate.Channels[i].Config[key] = rawValue
			}
		}
	}
	if candidate.Auth.Token == before.Auth.Token && isSecretRef(disk.Auth.Token) {
		candidate.Auth.Token = disk.Auth.Token
	}
}

func isSecretRef(value string) bool {
	var ref secretRef
	if json.Unmarshal([]byte(strings.TrimSpace(value)), &ref) != nil {
		return false
	}
	return (ref.Env != "") != (ref.File != "")
}

func providersByID(values []ProviderEntry) map[string]ProviderEntry {
	out := make(map[string]ProviderEntry, len(values))
	for _, value := range values {
		out[value.ID] = value
	}
	return out
}

func providerByID(values []ProviderEntry, id string) (ProviderEntry, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ProviderEntry{}, false
}

func modelsByID(values []ModelEntry) map[string]ModelEntry {
	out := make(map[string]ModelEntry, len(values))
	for _, value := range values {
		out[value.ID] = value
	}
	return out
}

func modelByID(values []ModelEntry, id string) (ModelEntry, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ModelEntry{}, false
}

func toolsByID(values []ToolEntry) map[string]ToolEntry {
	out := make(map[string]ToolEntry, len(values))
	for _, value := range values {
		out[value.ID] = value
	}
	return out
}

func toolByID(values []ToolEntry, id string) (ToolEntry, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ToolEntry{}, false
}

func channelsByID(values []ChannelEntry) map[string]ChannelEntry {
	out := make(map[string]ChannelEntry, len(values))
	for _, value := range values {
		out[value.ID] = value
	}
	return out
}

func channelByID(values []ChannelEntry, id string) (ChannelEntry, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ChannelEntry{}, false
}
