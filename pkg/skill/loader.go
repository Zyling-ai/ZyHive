// Package skill loads and manages agent skills.
// Reference: openclaw/src/agents/skills.ts
//
// Go Skill format:
//   skills/{skill-name}/SKILL.md   — content injected into system prompt
//   skills/{skill-name}/skill.json — metadata (name, description, version)
//
// Full implementation: Phase 4
package skill

import (
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill.
type Skill struct {
	Name        string
	Description string
	Content     string // SKILL.md content
}

// LoadAll scans skillsDir and returns all available skills.
func LoadAll(skillsDir string) ([]Skill, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}
		skills = append(skills, Skill{
			Name:    e.Name(),
			Content: strings.TrimSpace(string(data)),
		})
	}
	return skills, nil
}
