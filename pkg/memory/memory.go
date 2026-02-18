// Package memory handles hierarchical agent memory: core/, projects/, daily/, topics/.
// Reference: openclaw/src/agents/memory-search.ts
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileNode represents a file or directory in the memory tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`               // relative to memory/
	IsDir    bool       `json:"isDir"`
	Size     int64      `json:"size,omitempty"`
	ModTime  int64      `json:"modTime,omitempty"`   // unix ms
	Children []FileNode `json:"children,omitempty"`
}

// MemoryTree manages hierarchical memory for one agent workspace.
type MemoryTree struct {
	WorkspaceDir string
}

// NewMemoryTree creates a MemoryTree for the given workspace directory.
func NewMemoryTree(workspaceDir string) *MemoryTree {
	return &MemoryTree{WorkspaceDir: workspaceDir}
}

// memDir returns the absolute path to the memory/ directory.
func (m *MemoryTree) memDir() string {
	return filepath.Join(m.WorkspaceDir, "memory")
}

// Init creates the full memory directory structure and default files for a new agent.
func (m *MemoryTree) Init(agentName string) error {
	dirs := []string{
		"memory/core",
		"memory/projects",
		"memory/daily",
		"memory/topics",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(m.WorkspaceDir, d), 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// INDEX.md — lightweight, injected into every system prompt
	indexContent := fmt.Sprintf("# Memory Index\n\n- Agent: %s\n- Created: %s\n\n## Quick Reference\n\n(Add frequently needed facts here — this file is loaded into every conversation.)\n", agentName, time.Now().Format("2006-01-02"))
	if err := m.writeIfNotExists("memory/INDEX.md", indexContent); err != nil {
		return err
	}

	// Core files
	personality := fmt.Sprintf("# Personality & Preferences\n\nAgent: %s\n\n(Long-term personality traits, habits, communication style.)\n", agentName)
	if err := m.writeIfNotExists("memory/core/personality.md", personality); err != nil {
		return err
	}

	knowledge := "# Domain Knowledge\n\n(Accumulated domain knowledge, technical notes, learned facts.)\n"
	if err := m.writeIfNotExists("memory/core/knowledge.md", knowledge); err != nil {
		return err
	}

	relationships := "# Relationships & People\n\n(User profiles, team members, contacts, interaction preferences.)\n"
	if err := m.writeIfNotExists("memory/core/relationships.md", relationships); err != nil {
		return err
	}

	// .gitkeep for empty dirs
	for _, d := range []string{"memory/projects", "memory/topics"} {
		gk := filepath.Join(m.WorkspaceDir, d, ".gitkeep")
		if _, err := os.Stat(gk); os.IsNotExist(err) {
			_ = os.WriteFile(gk, []byte(""), 0644)
		}
	}

	return nil
}

// writeIfNotExists writes a file only if it doesn't already exist.
func (m *MemoryTree) writeIfNotExists(relPath, content string) error {
	abs := filepath.Join(m.WorkspaceDir, relPath)
	if _, err := os.Stat(abs); err == nil {
		return nil // already exists
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0644)
}

// GetIndex reads memory/INDEX.md — used in system prompt injection.
func (m *MemoryTree) GetIndex() (string, error) {
	data, err := os.ReadFile(filepath.Join(m.memDir(), "INDEX.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// UpdateIndex writes memory/INDEX.md.
func (m *MemoryTree) UpdateIndex(content string) error {
	return os.WriteFile(filepath.Join(m.memDir(), "INDEX.md"), []byte(content), 0644)
}

// GetFile reads any file under memory/ by relative path.
func (m *MemoryTree) GetFile(relPath string) (string, error) {
	absPath, err := m.safePath(relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes any file under memory/ by relative path, creating directories as needed.
func (m *MemoryTree) WriteFile(relPath, content string) error {
	absPath, err := m.safePath(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0644)
}

// AppendToFile appends content to a memory file with a separator.
func (m *MemoryTree) AppendToFile(relPath, content string) error {
	absPath, err := m.safePath(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n---\n" + content + "\n")
	return err
}

// WriteDailyLog writes/appends to memory/daily/YYYY/MM/DD.md using Asia/Shanghai time.
func (m *MemoryTree) WriteDailyLog(content string) error {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	relPath := fmt.Sprintf("daily/%s/%s/%s.md", now.Format("2006"), now.Format("01"), now.Format("02"))
	return m.AppendToFile(relPath, fmt.Sprintf("## %s\n\n%s", now.Format("15:04:05"), content))
}

// ListTree returns the full memory tree structure recursively.
func (m *MemoryTree) ListTree() ([]FileNode, error) {
	root := m.memDir()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []FileNode{}, nil
	}
	return m.walkDir(root, "")
}

func (m *MemoryTree) walkDir(absDir, relPrefix string) ([]FileNode, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	var nodes []FileNode
	for _, e := range entries {
		name := e.Name()
		if name == ".gitkeep" {
			continue
		}
		rel := name
		if relPrefix != "" {
			rel = relPrefix + "/" + name
		}
		node := FileNode{
			Name:  name,
			Path:  rel,
			IsDir: e.IsDir(),
		}
		if fi, err := e.Info(); err == nil {
			node.Size = fi.Size()
			node.ModTime = fi.ModTime().UnixMilli()
		}
		if e.IsDir() {
			children, err := m.walkDir(filepath.Join(absDir, name), rel)
			if err == nil {
				node.Children = children
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// safePath validates and resolves a relative path within memory/.
func (m *MemoryTree) safePath(relPath string) (string, error) {
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid path: %s", relPath)
	}
	abs := filepath.Join(m.memDir(), cleaned)
	if !strings.HasPrefix(abs, m.memDir()) {
		return "", fmt.Errorf("path escapes memory directory: %s", relPath)
	}
	return abs, nil
}

// MigrateFromFlatMemory migrates a flat MEMORY.md to the new tree structure.
// Returns true if migration was performed.
func MigrateFromFlatMemory(workspaceDir string) (bool, error) {
	memDir := filepath.Join(workspaceDir, "memory")
	indexPath := filepath.Join(memDir, "INDEX.md")

	// Check if already migrated (INDEX.md exists)
	if _, err := os.Stat(indexPath); err == nil {
		return false, nil
	}

	// Check if flat MEMORY.md exists
	flatPath := filepath.Join(workspaceDir, "MEMORY.md")
	flatData, err := os.ReadFile(flatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Initialize tree structure
	mt := NewMemoryTree(workspaceDir)
	if err := mt.Init("agent"); err != nil {
		return false, fmt.Errorf("init memory tree: %w", err)
	}

	// Move MEMORY.md content to core/personality.md
	content := string(flatData)
	if strings.TrimSpace(content) != "" && strings.TrimSpace(content) != "# MEMORY.md" {
		if err := mt.WriteFile("core/personality.md", content); err != nil {
			return false, err
		}
	}

	// Update INDEX.md with migration note
	indexContent := "# Memory Index\n\n> Migrated from MEMORY.md on " + time.Now().Format("2006-01-02") + "\n\nPrevious memory content moved to `core/personality.md`.\n"
	if err := mt.UpdateIndex(indexContent); err != nil {
		return false, err
	}

	return true, nil
}
