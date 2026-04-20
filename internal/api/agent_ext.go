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

	// 由于 pool 级动态注入（WithBrowser/WithFeishu/WithMemory 等）不走 tools.New，
	// 我们用一份全平台已知工具清单来做 readiness 检查。
	knownTools := []struct {
		Name, Group string
		// checker 返回 (ready, reason, hint)
		checker func() (bool, string, string)
	}{
		// 基础 always-ready
		{"read", "fs", nil}, {"write", "fs", nil}, {"edit", "fs", nil},
		{"grep", "fs", nil}, {"glob", "fs", nil},
		{"exec", "runtime", nil}, {"bash", "runtime", nil}, {"process", "runtime", nil},
		{"web_fetch", "web", nil},
		{"show_image", "ui", nil},
		{"self_list_skills", "self", nil},
		{"self_install_skill", "self", nil}, {"self_uninstall_skill", "self", nil},
		{"self_rename", "self", nil}, {"self_update_soul", "self", nil},
		{"self_set_env", "self", nil}, {"self_delete_env", "self", nil},
		{"wish_add", "self", nil}, {"wish_list", "self", nil},
		{"agent_list", "agent", nil}, {"agent_spawn", "agent", nil},
		{"agent_tasks", "agent", nil}, {"agent_kill", "agent", nil},
		{"agent_result", "agent", nil},
		{"sessions_list", "sessions", nil}, {"sessions_history", "sessions", nil},
		{"sessions_send", "sessions", nil}, {"sessions_spawn", "sessions", nil},
		{"cron_list", "cron", nil}, {"cron_add", "cron", nil}, {"cron_remove", "cron", nil},
		{"memory_search", "memory", nil},
		{"project_list", "project", nil}, {"project_read", "project", nil},
		{"project_write", "project", nil}, {"project_create", "project", nil},
		{"project_glob", "project", nil},
		// 浏览器工具（始终 ready，go-rod 自带）
		{"browser_navigate", "browser", nil}, {"browser_snapshot", "browser", nil},
		{"browser_screenshot", "browser", nil}, {"browser_click", "browser", nil},
		{"browser_type", "browser", nil}, {"browser_fill", "browser", nil},
		{"browser_press", "browser", nil}, {"browser_hover", "browser", nil},
		{"browser_scroll", "browser", nil}, {"browser_select", "browser", nil},
		{"browser_eval", "browser", nil}, {"browser_wait", "browser", nil},
		// web_search: 需要 brave api key
		{"web_search", "web", func() (bool, string, string) {
			if toolKeys["brave_search"] {
				return true, "", ""
			}
			return false, "未配置 Brave Search API Key", "前往「密钥管理」添加 brave_search 类型的 key"
		}},
		// image: 视觉能力依赖模型
		{"image", "ui", func() (bool, string, string) {
			if modelProvider == "anthropic" || modelProvider == "openai" || modelProvider == "" {
				return true, "", ""
			}
			return false, "当前绑定模型不支持视觉", "切换到 Claude / GPT-4o 等多模态模型"
		}},
		// send_message: 需要至少一个渠道
		{"send_message", "messaging", func() (bool, string, string) {
			if len(channelTypes) > 0 {
				return true, "", ""
			}
			return false, "未配置任何消息渠道", "先绑定飞书/Telegram 等渠道才能发送消息"
		}},
		{"send_file", "messaging", func() (bool, string, string) {
			if len(channelTypes) > 0 {
				return true, "", ""
			}
			return false, "未配置任何消息渠道", "同 send_message"
		}},
		// 飞书专属工具：依赖绑定飞书渠道
		{"feishu_send_message", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_send_rich_message", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_chat", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_bitable_app", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_bitable_table", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_list_bitable_records", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_bitable_record", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_get_user_info", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_calendar_event", "feishu", checkFeishuChannel(channelTypes)},
		{"feishu_create_task", "feishu", checkFeishuChannel(channelTypes)},
	}

	var ready, blocked int
	items := make([]ToolHealthItem, 0, len(knownTools))
	for _, kt := range knownTools {
		item := ToolHealthItem{Name: kt.Name, Group: kt.Group, Ready: true}
		if kt.checker != nil {
			ok, reason, hint := kt.checker()
			item.Ready = ok
			item.Reason = reason
			item.Hint = hint
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

// checkFeishuChannel 返回飞书工具的 readiness 检查闭包。
func checkFeishuChannel(channelTypes map[string]bool) func() (bool, string, string) {
	return func() (bool, string, string) {
		if channelTypes["feishu"] {
			return true, "", ""
		}
		return false, "未绑定飞书渠道", "前往「渠道」tab 添加飞书 Bot"
	}
}
