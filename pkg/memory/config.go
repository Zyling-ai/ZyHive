// Package memory — per-agent memory config (memory-config.json).
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MemConfig defines how automatic memory consolidation works for an agent.
type MemConfig struct {
	Enabled    bool   `json:"enabled"`
	Schedule   string `json:"schedule"`   // "hourly" | "every6h" | "daily" | "weekly"
	KeepTurns  int    `json:"keepTurns"`  // Q&A pairs to keep per session after trim
	FocusHint  string `json:"focusHint"`  // optional hint for what to record
	CronJobID  string `json:"cronJobId"`  // registered cron job ID (set when enabled)
}

// Default returns a MemConfig with sensible defaults.
func DefaultMemConfig() MemConfig {
	return MemConfig{
		Enabled:   false,
		Schedule:  "daily",
		KeepTurns: 3,
		FocusHint: "",
		CronJobID: "",
	}
}

// ScheduleToCron converts a schedule name to a cron expression (with seconds field).
// Uses UTC 02:00 for time-based schedules.
func ScheduleToCron(schedule string) string {
	switch schedule {
	case "hourly":
		return "0 0 * * * *" // every hour on the hour
	case "every6h":
		return "0 0 */6 * * *" // every 6 hours
	case "daily":
		return "0 0 2 * * *" // 02:00 daily
	case "weekly":
		return "0 0 2 * * 1" // 02:00 Monday
	default:
		return "0 0 2 * * *"
	}
}

// ReadMemConfig reads memory-config.json from the agent workspace.
func ReadMemConfig(workspaceDir string) (MemConfig, error) {
	path := filepath.Join(workspaceDir, "memory-config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultMemConfig(), nil
		}
		return DefaultMemConfig(), err
	}
	var cfg MemConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultMemConfig(), err
	}
	if cfg.KeepTurns <= 0 {
		cfg.KeepTurns = 3
	}
	if cfg.Schedule == "" {
		cfg.Schedule = "daily"
	}
	return cfg, nil
}

// WriteMemConfig persists memory-config.json in the agent workspace.
func WriteMemConfig(workspaceDir string, cfg MemConfig) error {
	path := filepath.Join(workspaceDir, "memory-config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
