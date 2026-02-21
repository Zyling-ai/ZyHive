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
)

// Project represents a shared project (source code, docs, assets, etc.)
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FilesDir    string    `json:"-"` // absolute path
}

// projectMeta is the on-disk meta.json format.
type projectMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
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
	return &Manager{
		rootDir:  rootDir,
		projects: make(map[string]*Project),
	}
}

// LoadAll scans rootDir and loads all project meta.json files.
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}

	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return fmt.Errorf("read projects dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(m.rootDir, e.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta projectMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		m.projects[meta.ID] = &Project{
			ID:          meta.ID,
			Name:        meta.Name,
			Description: meta.Description,
			Tags:        meta.Tags,
			CreatedAt:   meta.CreatedAt,
			UpdatedAt:   meta.UpdatedAt,
			FilesDir:    filepath.Join(m.rootDir, e.Name()),
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
}

// Create creates a new project.
func (m *Manager) Create(opts CreateOpts) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.projects[opts.ID]; exists {
		return nil, fmt.Errorf("project %q already exists", opts.ID)
	}

	projectDir := filepath.Join(m.rootDir, opts.ID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return nil, fmt.Errorf("create project dir: %w", err)
	}

	now := time.Now()
	meta := projectMeta{
		ID:          opts.ID,
		Name:        opts.Name,
		Description: opts.Description,
		Tags:        opts.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(projectDir, "meta.json"), data, 0644); err != nil {
		return nil, fmt.Errorf("write meta.json: %w", err)
	}

	// Create a default README.md
	readme := fmt.Sprintf("# %s\n\n%s\n", opts.Name, opts.Description)
	_ = os.WriteFile(filepath.Join(projectDir, "README.md"), []byte(readme), 0644)

	p := &Project{
		ID:          opts.ID,
		Name:        opts.Name,
		Description: opts.Description,
		Tags:        opts.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
		FilesDir:    projectDir,
	}
	m.projects[opts.ID] = p
	return p, nil
}

// Update updates project metadata.
func (m *Manager) Update(id string, name, description string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}

	if name != "" {
		p.Name = name
	}
	p.Description = description
	if tags != nil {
		p.Tags = tags
	}
	p.UpdatedAt = time.Now()

	meta := projectMeta{
		ID: p.ID, Name: p.Name, Description: p.Description,
		Tags: p.Tags, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	return os.WriteFile(filepath.Join(p.FilesDir, "meta.json"), data, 0644)
}

// Remove deletes a project and all its files.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	if err := os.RemoveAll(p.FilesDir); err != nil {
		return err
	}
	delete(m.projects, id)
	return nil
}
