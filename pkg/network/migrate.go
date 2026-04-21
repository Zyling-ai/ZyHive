package network

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrateIfNeeded performs idempotent migration for legacy workspaces.
//
// 1. workspace/RELATIONS.md → workspace/network/RELATIONS.md (move)
// 2. workspace/memory/core/user-profile.md → .../owner-profile.md (rename)
// 3. Ensure workspace/network/ exists.
//
// Safe to call on every agent startup.
func MigrateIfNeeded(workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}
	netDir := filepath.Join(workspaceDir, "network")
	if err := os.MkdirAll(filepath.Join(netDir, "contacts"), 0755); err != nil {
		return fmt.Errorf("network.MigrateIfNeeded: mkdir: %w", err)
	}

	// (1) RELATIONS.md move
	oldRel := filepath.Join(workspaceDir, "RELATIONS.md")
	newRel := filepath.Join(netDir, "RELATIONS.md")
	if _, err := os.Stat(newRel); os.IsNotExist(err) {
		if data, err := os.ReadFile(oldRel); err == nil {
			if err := os.WriteFile(newRel, data, 0644); err != nil {
				return fmt.Errorf("network.MigrateIfNeeded: write new RELATIONS: %w", err)
			}
			// Leave the old file as a breadcrumb pointer for a grace period —
			// safer than deleting. Replace content with a pointer stub.
			pointer := "> RELATIONS.md 已迁移到 network/RELATIONS.md\n"
			_ = os.WriteFile(oldRel, []byte(pointer), 0644)
		}
	}

	// (2) user-profile.md → owner-profile.md
	oldUP := filepath.Join(workspaceDir, "memory", "core", "user-profile.md")
	newUP := filepath.Join(workspaceDir, "memory", "core", "owner-profile.md")
	if _, err := os.Stat(newUP); os.IsNotExist(err) {
		if data, err := os.ReadFile(oldUP); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			if err := os.WriteFile(newUP, data, 0644); err != nil {
				return fmt.Errorf("network.MigrateIfNeeded: write owner-profile: %w", err)
			}
			// Remove the old file so system_prompt stops injecting it twice
			_ = os.Remove(oldUP)
		}
	}
	return nil
}
