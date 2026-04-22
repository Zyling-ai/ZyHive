package network

import (
	"fmt"
	"strings"
)

// Summary returns the "Layer 2" runtime summary injected into the system prompt
// when a specific contact is the current-conversation counterpart.
// Target size: ~300 chars (hard cap at 500).
//
// Format:
//
//	【当前对话对方】
//	- 姓名: 张三
//	- 来源: feishu · ou_abc (客户/家人)
//	- 累计对话: 47 次 / 最后 2026-04-20
//	- 事实 (最近 3):
//	  - 技术合伙人
//	  - 在北京
//	  - 偏好简答
//	[完整档案 read("network/contacts/feishu-ou_abc.md")]
func (s *Store) Summary(contactID string) string {
	c, err := s.Get(contactID)
	if err != nil || c == nil {
		return ""
	}
	// IsOwner=true 表示"这就是 agent 主人本人在该渠道的身份"。系统提示词
	// 已经通过 memory/core/owner-profile.md 注入过主人档案，此处再注入 contact
	// summary 会导致档案重复 + 描述冲突。直接返回空，让 owner-profile 接管。
	if c.IsOwner {
		return ""
	}
	return buildSummary(c)
}

func buildSummary(c *Contact) string {
	var sb strings.Builder
	sb.WriteString("【当前对话对方】\n")
	name := c.DisplayName
	if name == "" {
		name = c.ExternalID
	}
	sb.WriteString(fmt.Sprintf("- 姓名: %s\n", name))
	tagPart := ""
	if len(c.Tags) > 0 {
		tagPart = " · " + strings.Join(c.Tags, " / ")
	}
	sb.WriteString(fmt.Sprintf("- 来源: %s · %s%s\n", c.Source, c.ExternalID, tagPart))
	lastSeen := ""
	if !c.LastSeenAt.IsZero() {
		lastSeen = " / 最后 " + c.LastSeenAt.Format("2006-01-02")
	}
	sb.WriteString(fmt.Sprintf("- 累计对话: %d 次%s\n", c.MsgCount, lastSeen))

	facts := extractSectionBullets(c.Body, "事实", 3)
	if len(facts) > 0 {
		sb.WriteString("- 事实 (最近 3):\n")
		for _, f := range facts {
			sb.WriteString("  - " + f + "\n")
		}
	}
	prefs := extractSectionBullets(c.Body, "偏好", 2)
	if len(prefs) > 0 {
		sb.WriteString("- 偏好 (最近 2):\n")
		for _, f := range prefs {
			sb.WriteString("  - " + f + "\n")
		}
	}
	sb.WriteString(fmt.Sprintf("[完整档案 read(\"network/contacts/%s\")]\n", filenameForID(c.ID)))

	result := sb.String()
	// Hard cap 1200 chars.
	if len(result) > 1200 {
		result = result[:1200] + "...\n[截断，请 read 完整档案]\n"
	}
	return result
}

// extractSectionBullets reads a "## 事实" / "## 偏好..." section from a body
// and returns up to max non-placeholder bullet items.
func extractSectionBullets(body, section string, max int) []string {
	if body == "" {
		return nil
	}
	lines := strings.Split(body, "\n")
	var collecting bool
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match "## 事实" or "## 事实（AI 观察）" etc.
		if strings.HasPrefix(trimmed, "## ") {
			collecting = strings.Contains(trimmed, section)
			continue
		}
		if !collecting {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			// Skip placeholder like "(AI 通过 network_note ...)"
			if item == "" || strings.HasPrefix(item, "(") {
				continue
			}
			out = append(out, item)
			if len(out) >= max {
				break
			}
		}
	}
	return out
}
