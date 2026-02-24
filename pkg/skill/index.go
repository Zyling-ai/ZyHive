// Package skill — skill INDEX.md builder.
// RebuildIndex scans installed skills and generates a lightweight INDEX.md
// that the runner injects into the system prompt instead of full SKILL.md content.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RebuildIndex scans all installed skills in workspaceDir/skills/ and writes
// workspaceDir/skills/INDEX.md with a summary table.
// Called after self_install_skill and self_uninstall_skill tool executions.
func RebuildIndex(workspaceDir string) error {
	metas, err := ScanSkills(workspaceDir)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("## 已安装技能\n\n")
	sb.WriteString("| 技能 | 分类 | 描述 | 状态 |\n")
	sb.WriteString("|------|------|------|------|\n")
	for _, m := range metas {
		status := "✅ 已启用"
		if !m.Enabled {
			status = "❌ 已禁用"
		}
		category := m.Category
		if category == "" {
			category = "通用"
		}
		desc := m.Description
		runes := []rune(desc)
		if len(runes) > 40 {
			desc = string(runes[:40]) + "…"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			m.Name, category, desc, status))
	}

	indexPath := filepath.Join(workspaceDir, "skills", "INDEX.md")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(indexPath, []byte(sb.String()), 0644)
}
