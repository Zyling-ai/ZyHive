// Package api — 成员扩展端点：愿望清单 + 工具体检。
//
// GET /api/agents/:id/wishlist     读取 WISHLIST.md 返回结构化数组
// GET /api/agents/:id/tool-health  返回每个工具的 ready 状态 + 原因
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/tools"
)

type agentExtHandler struct {
	cfg     *config.Config
	manager *agent.Manager
}

// Wishlist returns the parsed WISHLIST.md entries for this agent.
// GET /api/agents/:id/wishlist
func (h *agentExtHandler) Wishlist(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	wishes, err := tools.ReadWishlist(ag.WorkspaceDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":  len(wishes),
		"wishes": wishes,
	})
}

// ToolHealthItem 是单个工具的体检结果。
type ToolHealthItem struct {
	Name   string `json:"name"`
	Group  string `json:"group,omitempty"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`   // ready=false 时的阻塞原因
	Hint   string `json:"hint,omitempty"`     // 解决提示
}

// ToolHealth 返回当前 agent 可用/受阻工具列表。
// GET /api/agents/:id/tool-health
func (h *agentExtHandler) ToolHealth(c *gin.Context) {
	id := c.Param("id")
	ag, ok := h.manager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// 构建一个临时 registry 查 tool 定义列表（不执行任何 tool）
	reg := tools.New(ag.WorkspaceDir, agentParentDir(ag), ag.ID)
	defs := reg.Definitions()

	// 当前 agent 绑定的 modelId / provider
	modelProvider := ""
	if ag.ModelID != "" {
		if me := h.cfg.FindModel(ag.ModelID); me != nil {
			modelProvider = me.Provider
		}
	}
	// agent 绑定的 channel type 集合
	channelTypes := make(map[string]bool)
	for _, ch := range ag.Channels {
		if ch.Enabled {
			channelTypes[strings.ToLower(ch.Type)] = true
		}
	}
	// 已配置的 tools（api keys）
	toolKeys := make(map[string]bool)
	for _, t := range h.cfg.Tools {
		if t.Enabled && strings.TrimSpace(t.APIKey) != "" {
			toolKeys[strings.ToLower(t.Type)] = true
		}
	}

	var ready, blocked int
	items := make([]ToolHealthItem, 0, len(defs))
	for _, d := range defs {
		item := ToolHealthItem{Name: d.Name, Group: groupOf(d.Name), Ready: true}
		// 按名字做 readiness 判断
		switch {
		case d.Name == "web_search":
			if !toolKeys["brave_search"] {
				item.Ready = false
				item.Reason = "未配置 Brave Search API Key"
				item.Hint = "前往「密钥管理」添加 brave_search 类型的 key"
			}
		case d.Name == "image" || d.Name == "show_image":
			// Vision 能力: 需要 anthropic / openai / 其他支持的模型
			if modelProvider != "anthropic" && modelProvider != "openai" && modelProvider != "" {
				item.Ready = false
				item.Reason = "当前绑定模型不支持视觉（需要 Claude 或 GPT-4o 等多模态模型）"
				item.Hint = "在「身份 & 灵魂」里切换到 Claude / GPT-4o"
			}
		case strings.HasPrefix(d.Name, "feishu_"):
			if !channelTypes["feishu"] {
				item.Ready = false
				item.Reason = "未配置飞书渠道"
				item.Hint = "前往「渠道」tab 绑定飞书 Bot"
			}
		case strings.HasPrefix(d.Name, "telegram_"):
			if !channelTypes["telegram"] {
				item.Ready = false
				item.Reason = "未配置 Telegram 渠道"
				item.Hint = "前往「渠道」tab 添加 Telegram Bot Token"
			}
		case d.Name == "send_message":
			if len(channelTypes) == 0 {
				item.Ready = false
				item.Reason = "未配置任何消息渠道"
				item.Hint = "需要先绑定飞书/Telegram 等渠道才能发送消息"
			}
		}

		if item.Ready {
			ready++
		} else {
			blocked++
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"agentId": id,
		"tools":   items,
		"summary": gin.H{
			"total":   len(items),
			"ready":   ready,
			"blocked": blocked,
		},
	})
}

// groupOf 把 tool name 映射回它所在的分组（近似实现，与 policy.go toolGroups 对齐）。
func groupOf(name string) string {
	switch {
	case stringsIn(name, "read", "write", "edit", "grep", "glob"):
		return "fs"
	case stringsIn(name, "exec", "bash", "process"):
		return "runtime"
	case stringsIn(name, "web_fetch", "web_search"):
		return "web"
	case strings.HasPrefix(name, "browser_"):
		return "browser"
	case strings.HasPrefix(name, "memory_"):
		return "memory"
	case strings.HasPrefix(name, "image") || name == "show_image":
		return "ui"
	case strings.HasPrefix(name, "agent_"):
		return "agent"
	case strings.HasPrefix(name, "sessions_"):
		return "sessions"
	case strings.HasPrefix(name, "cron_"):
		return "cron"
	case strings.HasPrefix(name, "send_"):
		return "messaging"
	case strings.HasPrefix(name, "self_") || name == "wish_add" || name == "wish_list":
		return "self"
	case strings.HasPrefix(name, "project_"):
		return "project"
	case strings.HasPrefix(name, "feishu_"):
		return "feishu"
	case strings.HasPrefix(name, "telegram_"):
		return "telegram"
	case strings.HasPrefix(name, "acp_"):
		return "acp"
	}
	return "misc"
}

func stringsIn(s string, list ...string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}

// agentParentDir 返回 agent workspace 的父目录（含 config.json）。
// 兼容 manager 未暴露 AgentDir 字段的情况。
func agentParentDir(ag *agent.Agent) string {
	// ag.WorkspaceDir 是 "<agentsDir>/<id>/workspace"
	// 父目录就是 "<agentsDir>/<id>"
	if ag.WorkspaceDir == "" {
		return ""
	}
	return stripLastSegment(ag.WorkspaceDir, "workspace")
}

func stripLastSegment(path, last string) string {
	if strings.HasSuffix(path, "/"+last) {
		return strings.TrimSuffix(path, "/"+last)
	}
	if strings.HasSuffix(path, "\\"+last) {
		return strings.TrimSuffix(path, "\\"+last)
	}
	return path
}