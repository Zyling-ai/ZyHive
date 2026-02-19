// Package memory — per-agent memory config (memory-config.json) + run log.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemConfig defines how automatic memory consolidation works for an agent.
type MemConfig struct {
	Enabled   bool   `json:"enabled"`
	Schedule  string `json:"schedule"`  // "hourly" | "every6h" | "daily" | "weekly"
	KeepTurns int    `json:"keepTurns"` // Q&A pairs to keep per session after trim
	FocusHint string `json:"focusHint"` // optional hint for what to record
	CronJobID string `json:"cronJobId"` // registered cron job ID (set when enabled)
}

// DefaultMemConfig returns a MemConfig with sensible defaults.
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
func ScheduleToCron(schedule string) string {
	switch schedule {
	case "hourly":
		return "0 0 * * * *"
	case "every6h":
		return "0 0 */6 * * *"
	case "daily":
		return "0 0 2 * * *"
	case "weekly":
		return "0 0 2 * * 1"
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

// ── Run log ───────────────────────────────────────────────────────────────

// RunLogEntry records one consolidation attempt.
type RunLogEntry struct {
	Timestamp int64  `json:"timestamp"` // unix ms
	Status    string `json:"status"`    // "ok" | "error"
	Message   string `json:"message"`   // summary preview or error text
}

const runLogFilename = "memory-run-log.jsonl"

// AppendRunLog appends one entry to memory-run-log.jsonl in the workspace.
func AppendRunLog(workspaceDir string, entry RunLogEntry) error {
	logPath := filepath.Join(workspaceDir, runLogFilename)
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// ReadRunLog reads all entries from memory-run-log.jsonl, newest first.
// Returns at most n entries (pass 0 for all).
func ReadRunLog(workspaceDir string, n int) ([]RunLogEntry, error) {
	logPath := filepath.Join(workspaceDir, runLogFilename)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunLogEntry{}, nil
		}
		return nil, err
	}

	var entries []RunLogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e RunLogEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}

	// Reverse to get newest first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}
	return entries, nil
}
