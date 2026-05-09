// pkg/network/chat_summary.go — Layer-2 群档案摘要, 注入到 system prompt.
//
// 与 contact summary.go 对称: 当前对话发生在某个群里时, 把该群的精简档案
// 与该群发送者的 contact summary 一起注入到本轮 prompt. AI 看到的不再是
// 一个孤零零的 chat_id, 而是有上下文的"产品讨论群 (47 msg, 群规则:..., 重要议题:...)".
//
// 输出目标体积: ~300 chars, 硬 cap 1200.
package network

import (
	"fmt"
	"strings"
)

// ChatSummary returns the "Layer 2" runtime summary injected into the system
// prompt when the current conversation is happening inside a specific chat
// (group / channel / multi-party room).
//
// Returns "" when:
//   - chat is missing
//   - chat exists but has zero useful info
//
// Format mirror Contact Summary:
//
//	【当前群聊】
//	- 群名: 产品讨论群
//	- 来源: feishu · oc_abc [supergroup]
//	- 累计消息: 47 次 / 最后 2026-04-23
//	- 成员数: 12 (近似)
//	- 标签: 产品 / 内部
//	- 基础信息 (最近 3):
//	  - 产品研发组每周三 11 点开会
//	  - 群主：张三
//	- 重要议题 (最近 2):
//	  - V2.0 发布日期讨论中
//	[完整档案 read("network/chats/feishu-oc_abc.md")]
func (s *Store) ChatSummary(chatID string) string {
	c, err := s.GetChat(chatID)
	if err != nil || c == nil {
		return ""
	}
	return buildChatSummary(c)
}

func buildChatSummary(c *Chat) string {
	var sb strings.Builder
	sb.WriteString("【当前群聊】\n")

	name := strings.TrimSpace(c.Title)
	if name == "" {
		name = c.ExternalID
	}
	sb.WriteString(fmt.Sprintf("- 群名: %s\n", name))

	kindPart := ""
	if c.Kind != "" {
		kindPart = " [" + c.Kind + "]"
	}
	sb.WriteString(fmt.Sprintf("- 来源: %s · %s%s\n", c.Source, c.ExternalID, kindPart))

	lastSeen := ""
	if !c.LastSeenAt.IsZero() {
		lastSeen = " / 最后 " + c.LastSeenAt.Format("2006-01-02")
	}
	sb.WriteString(fmt.Sprintf("- 累计消息: %d 次%s\n", c.MsgCount, lastSeen))

	if c.MemberCount > 0 {
		sb.WriteString(fmt.Sprintf("- 成员数: %d (近似)\n", c.MemberCount))
	}
	if len(c.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("- 标签: %s\n", strings.Join(c.Tags, " / ")))
	}

	// Body sections (extract bullets, skip placeholders)
	basics := extractSectionBullets(c.Body, "基础信息", 3)
	if len(basics) > 0 {
		sb.WriteString("- 基础信息 (最近 3):\n")
		for _, f := range basics {
			sb.WriteString("  - " + f + "\n")
		}
	}
	rules := extractSectionBullets(c.Body, "群规则", 3)
	if len(rules) > 0 {
		sb.WriteString("- 群规则 (最近 3):\n")
		for _, f := range rules {
			sb.WriteString("  - " + f + "\n")
		}
	}
	topics := extractSectionBullets(c.Body, "重要议题", 2)
	if len(topics) > 0 {
		sb.WriteString("- 重要议题 (最近 2):\n")
		for _, f := range topics {
			sb.WriteString("  - " + f + "\n")
		}
	}
	todos := extractSectionBullets(c.Body, "待跟进", 2)
	if len(todos) > 0 {
		sb.WriteString("- 待跟进 (最近 2):\n")
		for _, f := range todos {
			sb.WriteString("  - " + f + "\n")
		}
	}

	sb.WriteString(fmt.Sprintf("[完整档案 read(\"network/chats/%s\")]\n", filenameForID(c.ID)))

	result := sb.String()
	// Hard cap 1200 chars.
	if len(result) > 1200 {
		result = result[:1200] + "...\n[截断, 请 read 完整档案]\n"
	}
	return result
}
