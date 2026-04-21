// pkg/agent/capabilities.go — 统一构造 "能力上下文" 字符串（工具体检 + 愿望清单）
// 供 internal/api/chat.go 和 pkg/agent/pool.go 共享调用。
//
// 设计目的：让 AI 在每次对话开始就准确知道
//   1. 当前有哪些工具可用（分组展示）
//   2. 哪些工具被阻塞 + 原因 + 解决方案
//   3. 已记录的愿望（避免重复 wish_add）
// 消除"训练记忆猜能力"导致的幻觉。
package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/tools"
)

// BuildCapabilitiesContext 构造可以直接塞进 runner.Config.CapabilitiesContext 的文本。
// 参数：
//   reg     — 当前 agent 已配置好的 tool registry（注入依赖 / policy / 动态 tool 全部已就位）
//   ag      — agent 元信息（取 channels / modelId）
//   cfg     — 全局 config（取 tools api keys / providers）
//   wsDir   — agent workspace 目录（读 WISHLIST.md / RELATIONS.md）
func BuildCapabilitiesContext(reg *tools.Registry, ag *Agent, cfg *config.Config, wsDir string) string {
	if reg == nil || cfg == nil {
		return ""
	}

	ctx := tools.AgentHealthCtx{
		ModelProvider: resolveModelProvider(ag, cfg),
		ChannelTypes:  collectChannelTypes(ag),
		ToolAPIKeys:   collectToolKeys(cfg),
		HasRelations:  hasAnyRelations(wsDir),
	}

	healthText := tools.FormatCapabilitiesForPrompt(reg, ctx)
	wishText := tools.FormatWishlistForPrompt(wsDir, 5)

	var sb strings.Builder
	if healthText != "" {
		sb.WriteString(healthText)
	}
	if wishText != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(wishText)
	}
	return sb.String()
}

// resolveModelProvider 从 agent 绑定的 modelId 找到 provider 字符串。
func resolveModelProvider(ag *Agent, cfg *config.Config) string {
	if ag == nil || ag.ModelID == "" {
		return ""
	}
	if m := cfg.FindModel(ag.ModelID); m != nil {
		return strings.ToLower(m.Provider)
	}
	return ""
}

func collectChannelTypes(ag *Agent) map[string]bool {
	out := make(map[string]bool)
	if ag == nil {
		return out
	}
	for _, ch := range ag.Channels {
		if ch.Enabled {
			out[strings.ToLower(ch.Type)] = true
		}
	}
	return out
}

func collectToolKeys(cfg *config.Config) map[string]bool {
	out := make(map[string]bool)
	for _, t := range cfg.Tools {
		if t.Enabled && strings.TrimSpace(t.APIKey) != "" {
			out[strings.ToLower(t.Type)] = true
		}
	}
	return out
}

// hasAnyRelations 检查 RELATIONS.md 是否存在有效条目（任何 ID）。
// 优先读 network/RELATIONS.md（新位置），fallback 到根部旧位置。
// 仅用于 capabilities prompt 里的 agent_spawn 约束提示，不需精确解析。
func hasAnyRelations(wsDir string) bool {
	data, err := os.ReadFile(filepath.Join(wsDir, "network", "RELATIONS.md"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(wsDir, "RELATIONS.md"))
		if err != nil {
			return false
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		parts := strings.Split(line, "|")
		// 第一个非空列作为 ID
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" || strings.HasPrefix(p, "-") || strings.Contains(p, "成员") || strings.EqualFold(p, "ID") {
				continue
			}
			return true
		}
	}
	return false
}
