// Package memory handles reading and writing agent memory files.
// Reference: openclaw/src/agents/memory-search.ts
// Full implementation: Phase 3
package memory

import (
	"os"
	"path/filepath"
)

// ReadLongTerm reads MEMORY.md from the workspace.
func ReadLongTerm(workspaceDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "MEMORY.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// TODO: implement daily memory log reading, search, and semantic retrieval.
