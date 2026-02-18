// Package memory handles reading and writing agent memory files.
// Reference: openclaw/src/agents/memory-search.ts
package memory

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReadLongTerm reads MEMORY.md from the workspace.
func ReadLongTerm(workspaceDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "MEMORY.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MemoryFile represents a daily memory file.
type MemoryFile struct {
	Name    string    `json:"name"`
	Date    string    `json:"date"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// ListDailyFiles returns memory files sorted newest first.
func ListDailyFiles(workspaceDir string) ([]MemoryFile, error) {
	memDir := filepath.Join(workspaceDir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryFile{}, nil
		}
		return nil, err
	}

	var files []MemoryFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		// Extract date from filename like "2026-02-18.md" or just use name
		date := name
		if len(name) >= 10 {
			date = name[:10]
		}
		files = append(files, MemoryFile{
			Name:    name,
			Date:    date,
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}

	// Sort newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// ReadDailyFile reads a specific memory file.
func ReadDailyFile(workspaceDir, filename string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "memory", filename))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteDailyFile writes a daily memory file.
func WriteDailyFile(workspaceDir, filename, content string) error {
	memDir := filepath.Join(workspaceDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(memDir, filename), []byte(content), 0644)
}

// AppendToday appends content to today's memory file.
func AppendToday(workspaceDir, content string) error {
	memDir := filepath.Join(workspaceDir, "memory")
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return err
	}
	filename := time.Now().Format("2006-01-02") + ".md"
	path := filepath.Join(memDir, filename)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content + "\n")
	return err
}
