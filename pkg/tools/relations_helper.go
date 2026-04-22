// pkg/tools/relations_helper.go — 读取当前 agent 的 RELATIONS.md，
// 供 agent_spawn 等工具做权限检查。
package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// allowedPeersFromRelations 解析 RELATIONS.md, 返回当前 agent 可派遣的 agentID 集合.
// 优先读 network/RELATIONS.md（新位置），fallback 到根部旧位置。
// "可派遣" 定义: RELATIONS.md 里出现的任何 agentID 都算 (有任何关系 → 允许).
// 文件不存在 → 返回 nil (调用方应解释为"空关系, 仅允许派内置 agent").
func allowedPeersFromRelations(workspaceDir string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(workspaceDir, "network", "RELATIONS.md"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(workspaceDir, "RELATIONS.md"))
		if err != nil {
			return nil
		}
	}
	peers := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		var cols []string
		for _, p := range parts {
			cols = append(cols, strings.TrimSpace(p))
		}
		if len(cols) > 0 && cols[0] == "" {
			cols = cols[1:]
		}
		if len(cols) > 0 && cols[len(cols)-1] == "" {
			cols = cols[:len(cols)-1]
		}
		if len(cols) < 1 {
			continue
		}
		id := cols[0]
		if id == "" || strings.HasPrefix(id, "-") ||
			strings.Contains(id, "成员") || strings.Contains(id, "目标") ||
			strings.EqualFold(id, "ID") {
			continue
		}
		// Contact IDs contain ":" (e.g. "feishu:ou_abc") — skip, only agents
		// are dispatchable.
		if strings.Contains(id, ":") {
			continue
		}
		// New 6-col format has toKind in col[2]; skip non-agent rows.
		if len(cols) >= 6 {
			kind := strings.ToLower(cols[2])
			if kind != "" && kind != "agent" {
				continue
			}
		}
		peers[id] = true
	}
	return peers
}
