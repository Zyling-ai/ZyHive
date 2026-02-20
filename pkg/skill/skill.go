// Package skill manages per-agent skill metadata and content.
// Skills are stored at: {workspaceDir}/skills/{skillId}/
//   skill.json  — metadata (id, name, version, icon, category, description, enabled, installedAt, source)
//   SKILL.md    — system prompt content injected by runner
package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Meta holds the metadata for a single skill (matches skill.json schema).
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	InstalledAt string `json:"installedAt"`
	Source      string `json:"source"` // "local" | "clawhub" | "url"
}

// skillDir returns the directory path for a specific skill.
func skillDir(workspaceDir, skillID string) string {
	return filepath.Join(workspaceDir, "skills", skillID)
}

// ScanSkills reads all skill.json files under workspaceDir/skills/ and returns their metadata.
func ScanSkills(workspaceDir string) ([]Meta, error) {
	dir := filepath.Join(workspaceDir, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Meta{}, nil
		}
		return nil, err
	}

	var metas []Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := ReadSkill(workspaceDir, e.Name())
		if err != nil {
			continue
		}
		metas = append(metas, *m)
	}
	if metas == nil {
		metas = []Meta{}
	}
	return metas, nil
}

// ReadSkill reads a single skill's metadata from skill.json.
func ReadSkill(workspaceDir, skillID string) (*Meta, error) {
	jsonPath := filepath.Join(skillDir(workspaceDir, skillID), "skill.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WriteSkill writes skill.json for a skill (creates directories if needed).
func WriteSkill(workspaceDir string, meta Meta) error {
	dir := skillDir(workspaceDir, meta.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "skill.json"), data, 0644)
}

// RemoveSkill deletes the skill directory (and all its files).
func RemoveSkill(workspaceDir, skillID string) error {
	return os.RemoveAll(skillDir(workspaceDir, skillID))
}

// SkillPrompt reads SKILL.md for a skill and returns its content.
// Returns "" if the file does not exist.
func SkillPrompt(workspaceDir, skillID string) string {
	mdPath := filepath.Join(skillDir(workspaceDir, skillID), "SKILL.md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return ""
	}
	return string(data)
}
