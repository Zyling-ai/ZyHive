// Package agent manages AI agent lifecycle and workspace files.
// Reference: openclaw/src/agents/identity-file.ts, workspace.ts
package agent

import (
	"os"
	"path/filepath"

	"github.com/sunhuihui6688-star/ai-panel/pkg/memory"
)

// ReadIdentity reads IDENTITY.md from the agent's workspace.
func ReadIdentity(workspaceDir string) (string, error) {
	return readMD(workspaceDir, "IDENTITY.md")
}

// WriteIdentity writes IDENTITY.md to the agent's workspace.
func WriteIdentity(workspaceDir, content string) error {
	return writeMD(workspaceDir, "IDENTITY.md", content)
}

// ReadSoul reads SOUL.md from the agent's workspace.
func ReadSoul(workspaceDir string) (string, error) {
	return readMD(workspaceDir, "SOUL.md")
}

// WriteSoul writes SOUL.md to the agent's workspace.
func WriteSoul(workspaceDir, content string) error {
	return writeMD(workspaceDir, "SOUL.md", content)
}

// ReadMemory reads MEMORY.md from the agent's workspace (legacy, for backward compat).
func ReadMemory(workspaceDir string) (string, error) {
	return readMD(workspaceDir, "MEMORY.md")
}

// InitWorkspace creates the standard workspace structure for a new agent.
// Now uses hierarchical memory tree instead of flat MEMORY.md.
// Reference: openclaw/src/agents/workspace-templates.ts
func InitWorkspace(workspaceDir, agentName, role string) error {
	identity := "# IDENTITY.md\n\n- **Name:** " + agentName + "\n- **Role:** " + role + "\n"
	if err := writeMD(workspaceDir, "IDENTITY.md", identity); err != nil {
		return err
	}

	soul := "# SOUL.md\n\nBe helpful, direct, and efficient.\n"
	if err := writeMD(workspaceDir, "SOUL.md", soul); err != nil {
		return err
	}

	// Initialize hierarchical memory tree
	mt := memory.NewMemoryTree(workspaceDir)
	return mt.Init(agentName)
}

func readMD(dir, filename string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeMD(dir, filename, content string) error {
	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}
