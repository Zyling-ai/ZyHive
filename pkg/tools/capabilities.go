// pkg/tools/capabilities.go — 把"工具体检 + 愿望清单"格式化为系统提示词片段，
// 让 AI 在对话开始时就感知自己真实的能力边界，不再靠训练记忆猜"我应该有什么"。
package tools

import (
	"fmt"
	"sort"
	"strings"
)

// AgentHealthCtx 是"健康检查"需要的外部配置快照。
// 由 internal/api 层构造并传入，避免 pkg/tools 反向依赖 config/agent 包。
type AgentHealthCtx struct {
	ModelProvider string          // "anthropic" / "openai" / ""；用于判断 image/vision 支持
	ChannelTypes  map[string]bool // 启用的 channel type: feishu / telegram / ...
	ToolAPIKeys   map[string]bool // 已配置 key 的 tool type: brave_search / elevenlabs / ...
	HasRelations  bool            // RELATIONS.md 是否有任何条目（决定是否提"派遣受限"）
}

// 与 internal/api/agent_ext.go::ToolHealth 用同一套判定规则（保持一致性）。
// 这里不做真实 HTTP 探测，只基于配置存在性检查，启动时调用成本低。

var groupOrder = []string{"fs", "runtime", "web", "browser", "agent", "sessions", "cron",
	"memory", "project", "self", "messaging", "feishu", "telegram", "ui", "misc"}

var groupLabel = map[string]string{
	"fs":        "📁 文件/命令",
	"runtime":   "⚡ 执行",
	"web":       "🌐 网页",
	"browser":   "🖥️ 浏览器",
	"agent":     "👥 派遣",
	"sessions":  "💬 会话",
	"cron":      "⏱️ 定时",
	"memory":    "🧠 记忆",
	"project":   "📂 项目",
	"self":      "🎛️ 自管理",
	"messaging": "📨 消息",
	"feishu":    "📱 飞书",
	"telegram":  "✈️ Telegram",
	"ui":        "🖼️ UI",
	"misc":      "🔧 其它",
}

// FormatCapabilitiesForPrompt 根据当前 registry + 外部配置，生成可以直接嵌入系统提示词的能力清单。
// 输出长度控制在 ~1KB 左右。
func FormatCapabilitiesForPrompt(r *Registry, ctx AgentHealthCtx) string {
	if r == nil {
		return ""
	}
	defs := r.Definitions()
	if len(defs) == 0 {
		return ""
	}

	// 按组织聚合 ready / blocked
	readyByGroup := make(map[string][]string)
	type blockedItem struct{ Name, Reason, Hint string }
	var blocked []blockedItem

	for _, d := range defs {
		g := toolGroupOf(d.Name)
		ok, reason, hint := checkToolReadiness(d.Name, ctx)
		if ok {
			readyByGroup[g] = append(readyByGroup[g], d.Name)
		} else {
			blocked = append(blocked, blockedItem{d.Name, reason, hint})
		}
	}

	var sb strings.Builder
	readyCount := 0
	for _, list := range readyByGroup {
		readyCount += len(list)
	}

	sb.WriteString(fmt.Sprintf("--- 你当前可用的工具（实时体检 · 共 %d 个可用，%d 个受阻）---\n", readyCount, len(blocked)))

	// 可用组：按固定顺序展示
	sb.WriteString("✅ 可用：\n")
	for _, g := range groupOrder {
		list := readyByGroup[g]
		if len(list) == 0 {
			continue
		}
		sort.Strings(list)
		if len(list) > 8 {
			sb.WriteString(fmt.Sprintf("  %s (%d): %s ...\n", groupLabel[g], len(list), strings.Join(list[:8], ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", groupLabel[g], strings.Join(list, ", ")))
		}
	}

	// 阻塞工具：每个独占一行，展示具体原因
	if len(blocked) > 0 {
		sb.WriteString("\n⚠️ 当前不可用（如用户要求相关功能，请诚实说明缺口）：\n")
		for _, b := range blocked {
			sb.WriteString(fmt.Sprintf("  • %s — %s", b.Name, b.Reason))
			if b.Hint != "" {
				sb.WriteString(fmt.Sprintf("（%s）", b.Hint))
			}
			sb.WriteString("\n")
		}
	}

	// 关键约束
	sb.WriteString("\n📋 关键约束：\n")
	if ctx.HasRelations {
		sb.WriteString("  • agent_spawn：只能派遣 RELATIONS.md 里有记录的成员；派遣不在关系里的会被系统拒绝\n")
	} else {
		sb.WriteString("  • agent_spawn：你目前没有任何关系，无法派遣其他用户成员。仅可派遣内置 type (general-purpose/explore/plan/verification/coordinator)\n")
	}
	sb.WriteString("  • 诚实原则：如果某个工具在上方'不可用'列表里，不要假装你会用它；请主动告知用户并用 wish_add 记录缺口\n")

	return sb.String()
}

// toolGroupOf 给一个工具名分组（和 policy.go/agent_ext.go 的规则保持一致）。
func toolGroupOf(name string) string {
	switch {
	case name == "read" || name == "write" || name == "edit" || name == "grep" || name == "glob":
		return "fs"
	case name == "exec" || name == "bash" || name == "process":
		return "runtime"
	case name == "web_fetch" || name == "web_search":
		return "web"
	case strings.HasPrefix(name, "browser_"):
		return "browser"
	case strings.HasPrefix(name, "agent_"):
		return "agent"
	case strings.HasPrefix(name, "sessions_"):
		return "sessions"
	case strings.HasPrefix(name, "cron_"):
		return "cron"
	case strings.HasPrefix(name, "memory_"):
		return "memory"
	case strings.HasPrefix(name, "project_"):
		return "project"
	case strings.HasPrefix(name, "self_") || name == "wish_add" || name == "wish_list":
		return "self"
	case strings.HasPrefix(name, "send_"):
		return "messaging"
	case strings.HasPrefix(name, "feishu_"):
		return "feishu"
	case strings.HasPrefix(name, "telegram_"):
		return "telegram"
	case name == "image" || name == "show_image" || name == "tts":
		return "ui"
	}
	return "misc"
}

// checkToolReadiness 返回 (ready, reason, hint)，与 internal/api/agent_ext.go 的逻辑对齐。
// 没有明确判定条件的 tool → 默认 ready。
func checkToolReadiness(name string, ctx AgentHealthCtx) (bool, string, string) {
	switch {
	case name == "web_search":
		if !ctx.ToolAPIKeys["brave_search"] {
			return false, "未配置 Brave Search API Key", "前往「密钥管理」添加 brave_search 类型的 key"
		}
	case name == "image":
		if ctx.ModelProvider != "" && ctx.ModelProvider != "anthropic" && ctx.ModelProvider != "openai" {
			return false, "当前绑定模型不支持视觉", "切换到 Claude / GPT-4o 等多模态模型"
		}
	case name == "send_message" || name == "send_file":
		if len(ctx.ChannelTypes) == 0 {
			return false, "未绑定任何消息渠道", "先在「渠道」tab 绑定飞书/Telegram 等"
		}
	case strings.HasPrefix(name, "feishu_"):
		if !ctx.ChannelTypes["feishu"] {
			return false, "未绑定飞书渠道", "前往「渠道」tab 添加飞书 Bot"
		}
	case strings.HasPrefix(name, "telegram_"):
		if !ctx.ChannelTypes["telegram"] {
			return false, "未绑定 Telegram 渠道", "添加 Telegram Bot Token"
		}
	}
	return true, "", ""
}

// FormatWishlistForPrompt 读取 workspace/WISHLIST.md 头部 N 条, 格式化为 prompt 片段.
func FormatWishlistForPrompt(workspaceDir string, maxItems int) string {
	if maxItems <= 0 {
		maxItems = 5
	}
	wishes, err := ReadWishlist(workspaceDir)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("--- 你之前记录的能力愿望（WISHLIST.md）---\n")
	if len(wishes) == 0 {
		sb.WriteString("（目前为空。首次发现能力缺口时请用 wish_add 记录，用户会在面板看到并决定是否为你启用）\n")
		return sb.String()
	}
	n := len(wishes)
	if n > maxItems {
		n = maxItems
	}
	for i := 0; i < n; i++ {
		w := wishes[i]
		pri := w.Priority
		if pri == "" {
			pri = "P1"
		}
		reason := w.Reason
		if len(reason) > 80 {
			reason = reason[:80] + "…"
		}
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s", i+1, pri, w.Title))
		if reason != "" {
			sb.WriteString(" — " + reason)
		}
		sb.WriteString("\n")
	}
	if len(wishes) > maxItems {
		sb.WriteString(fmt.Sprintf("  … (共 %d 条，完整列表用 wish_list 工具读取)\n", len(wishes)))
	}
	sb.WriteString("⚠️ 发现新能力缺口再用 wish_add；不要重复记录已在列表里的愿望。\n")
	return sb.String()
}
