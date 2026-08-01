// Package project manages shared project workspaces accessible by all agents.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/safefs"
)

// Project represents a shared project (source code, docs, assets, etc.)
type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Editors: agent IDs that can write to this project.
	// Empty slice = all agents can write (default open).
	// Use ["__none__"] to make a project read-only for everyone.
	Editors   []string  `json:"editors,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	FilesDir  string    `json:"-"` // absolute path
}

// CanWrite returns true if agentID has write permission on this project.
// An empty Editors list means all agents can write.
func (p *Project) CanWrite(agentID string) bool {
	if len(p.Editors) == 0 {
		return true // open write by default
	}
	for _, id := range p.Editors {
		if id == agentID {
			return true
		}
	}
	return false
}

// projectMeta is the on-disk meta.json format.
type projectMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Editors     []string  `json:"editors,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Manager manages all projects under a root directory.
type Manager struct {
	rootDir  string
	projects map[string]*Project
	mu       sync.RWMutex
}

// NewManager creates a Manager rooted at rootDir.
func NewManager(rootDir string) *Manager {
	if abs, err := filepath.Abs(rootDir); err == nil {
		rootDir = abs
	}
	return &Manager{
		rootDir:  rootDir,
		projects: make(map[string]*Project),
	}
}

// LoadAll scans rootDir and loads all project meta.json files.
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.rootDir, 0700); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}
	if err := os.Chmod(m.rootDir, 0700); err != nil {
		return fmt.Errorf("secure projects dir: %w", err)
	}

	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return fmt.Errorf("read projects dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := safefs.ValidateResourceID(e.Name()); err != nil {
			continue
		}
		projectDir, err := safefs.ConfineResource(m.rootDir, e.Name())
		if err != nil {
			continue
		}
		metaPath := filepath.Join(projectDir, "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta projectMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.ID != e.Name() || safefs.ValidateResourceID(meta.ID) != nil {
			continue
		}
		if _, exists := m.projects[meta.ID]; exists {
			continue
		}
		if err := secureProjectTree(projectDir); err != nil {
			return fmt.Errorf("secure project %q: %w", meta.ID, err)
		}
		m.projects[meta.ID] = &Project{
			ID:          meta.ID,
			Name:        meta.Name,
			Description: meta.Description,
			Tags:        meta.Tags,
			Editors:     meta.Editors,
			CreatedAt:   meta.CreatedAt,
			UpdatedAt:   meta.UpdatedAt,
			FilesDir:    projectDir,
		}
	}
	return nil
}

// List returns all projects sorted by creation time (newest first).
func (m *Manager) List() []*Project {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Project, 0, len(m.projects))
	for _, p := range m.projects {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// Get returns the project with the given ID.
func (m *Manager) Get(id string) (*Project, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	return p, ok
}

// CreateOpts holds options for creating a project.
type CreateOpts struct {
	ID          string
	Name        string
	Description string
	Tags        []string
	Editors     []string // empty = all can write
}

// Create creates a new project.
func (m *Manager) Create(opts CreateOpts) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := safefs.ValidateResourceID(opts.ID); err != nil {
		return nil, fmt.Errorf("invalid project id %q: %w", opts.ID, err)
	}
	if _, exists := m.projects[opts.ID]; exists {
		return nil, fmt.Errorf("project %q already exists", opts.ID)
	}

	if err := os.MkdirAll(m.rootDir, 0700); err != nil {
		return nil, fmt.Errorf("create projects dir: %w", err)
	}
	projectDir, err := safefs.ConfineResource(m.rootDir, opts.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve project dir: %w", err)
	}
	if _, err := os.Lstat(projectDir); err == nil {
		return nil, fmt.Errorf("project %q already exists on disk", opts.ID)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect project dir: %w", err)
	}
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		return nil, fmt.Errorf("create project dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(projectDir)
		}
	}()

	now := time.Now()
	meta := projectMeta{
		ID:          opts.ID,
		Name:        opts.Name,
		Description: opts.Description,
		Tags:        opts.Tags,
		Editors:     opts.Editors,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(projectDir, "meta.json"), data, 0600); err != nil {
		return nil, fmt.Errorf("write meta.json: %w", err)
	}

	// Create a default README.md
	readme := fmt.Sprintf("# %s\n\n%s\n", opts.Name, opts.Description)
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte(readme), 0600); err != nil {
		return nil, fmt.Errorf("write README.md: %w", err)
	}

	p := &Project{
		ID:          opts.ID,
		Name:        opts.Name,
		Description: opts.Description,
		Tags:        opts.Tags,
		Editors:     opts.Editors,
		CreatedAt:   now,
		UpdatedAt:   now,
		FilesDir:    projectDir,
	}
	m.projects[opts.ID] = p
	cleanup = false
	return p, nil
}

// SetEditors updates the editor list for a project and persists to meta.json.
// Pass nil to allow all agents to write (open). Pass []string{} also means open.
// Pass []string{"__none__"} to make read-only for everyone.
func (m *Manager) SetEditors(projectID string, editors []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project %q not found", projectID)
	}
	candidate := *p
	candidate.Editors = editors
	candidate.UpdatedAt = time.Now()

	meta := projectMeta{
		ID: candidate.ID, Name: candidate.Name, Description: candidate.Description,
		Tags: candidate.Tags, Editors: candidate.Editors,
		CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
	}
	if err := writeProjectMeta(candidate.FilesDir, meta); err != nil {
		return err
	}
	*p = candidate
	return nil
}

// Update updates project metadata.
func (m *Manager) Update(id string, name, description string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}

	candidate := *p
	if name != "" {
		candidate.Name = name
	}
	candidate.Description = description
	if tags != nil {
		candidate.Tags = tags
	}
	candidate.UpdatedAt = time.Now()

	meta := projectMeta{
		ID: candidate.ID, Name: candidate.Name, Description: candidate.Description,
		Tags: candidate.Tags, Editors: candidate.Editors,
		CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
	}
	if err := writeProjectMeta(candidate.FilesDir, meta); err != nil {
		return err
	}
	*p = candidate
	return nil
}

// Remove deletes a project and all its files.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	projectDir, err := safefs.ConfineResource(m.rootDir, id)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}
	if filepath.Clean(p.FilesDir) != filepath.Clean(projectDir) {
		return fmt.Errorf("project %q files are outside its managed directory", id)
	}
	if err := os.RemoveAll(projectDir); err != nil {
		return err
	}
	delete(m.projects, id)
	return nil
}

func writeProjectMeta(projectDir string, meta projectMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(projectDir, "meta.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func secureProjectTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0700)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		mode := os.FileMode(0600)
		if info.Mode().Perm()&0100 != 0 {
			mode = 0700
		}
		return os.Chmod(path, mode)
	})
}
