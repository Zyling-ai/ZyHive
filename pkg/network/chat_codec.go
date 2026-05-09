// pkg/network/chat_codec.go — Chat 档案的 markdown frontmatter 编解码.
//
// 设计与 codec.go (Contact) 完全对称, 用同一套 frontmatter 风格, 字段集不同:
//
//   ---
//   id: feishu:oc_abc
//   source: feishu
//   externalId: oc_abc
//   title: "产品讨论群"
//   kind: group
//   tags:
//     - 内部
//     - 产品
//   memberCount: 12
//   createdAt: 2026-04-23T10:00:00Z
//   lastSeenAt: 2026-04-24T08:30:00Z
//   msgCount: 47
//   ---
//
//   # 产品讨论群
//
//   ## 基础信息
//   - 产品研发组每周三 11 点开会
//
//   ## 群规则
//   - 禁止讨论商业敏感
//
//   ## 重要议题
//   - V2.0 发布日期讨论中
//
//   ## 待跟进
//   - 客户反馈整理
package network

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// parseChatMD splits a markdown file into frontmatter + body and loads them
// into a Chat. If the file has no frontmatter, the whole content becomes Body
// and only ID from hint is used.
func parseChatMD(raw string, hintID string) *Chat {
	c := &Chat{ID: hintID}
	m := frontmatterRe.FindStringSubmatch(raw)
	if m == nil {
		c.Body = strings.TrimSpace(raw)
		return c
	}
	fmBlock := m[1]
	body := strings.TrimSpace(m[2])
	c.Body = body

	for _, line := range strings.Split(fmBlock, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Array continuation handled in second pass via collectArrayField.
		if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "- ") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		assignChatField(c, key, val)
	}
	c.Tags = collectArrayField(fmBlock, "tags")
	return c
}

func assignChatField(c *Chat, key, val string) {
	switch key {
	case "id":
		c.ID = val
	case "source":
		c.Source = val
	case "externalId":
		c.ExternalID = val
	case "title":
		c.Title = stripQuotes(val)
	case "kind":
		c.Kind = stripQuotes(val)
	case "memberCount":
		var n int
		_, _ = fmt.Sscanf(val, "%d", &n)
		c.MemberCount = n
	case "msgCount":
		var n int
		_, _ = fmt.Sscanf(val, "%d", &n)
		c.MsgCount = n
	case "createdAt":
		t, err := time.Parse(time.RFC3339, val)
		if err == nil {
			c.CreatedAt = t
		}
	case "lastSeenAt":
		t, err := time.Parse(time.RFC3339, val)
		if err == nil {
			c.LastSeenAt = t
		}
	}
}

// renderChatMD serializes a Chat to the frontmatter+body markdown.
func renderChatMD(c *Chat) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", c.ID))
	sb.WriteString(fmt.Sprintf("source: %s\n", c.Source))
	sb.WriteString(fmt.Sprintf("externalId: %s\n", c.ExternalID))
	sb.WriteString(fmt.Sprintf("title: %s\n", quoteIfNeeded(c.Title)))
	sb.WriteString(fmt.Sprintf("kind: %s\n", quoteIfNeeded(c.Kind)))
	if len(c.Tags) > 0 {
		sort.Strings(c.Tags)
		sb.WriteString("tags:\n")
		for _, t := range c.Tags {
			sb.WriteString("  - " + quoteIfNeeded(t) + "\n")
		}
	} else {
		sb.WriteString("tags: []\n")
	}
	if c.MemberCount > 0 {
		sb.WriteString(fmt.Sprintf("memberCount: %d\n", c.MemberCount))
	}
	if !c.CreatedAt.IsZero() {
		sb.WriteString("createdAt: " + c.CreatedAt.Format(time.RFC3339) + "\n")
	}
	if !c.LastSeenAt.IsZero() {
		sb.WriteString("lastSeenAt: " + c.LastSeenAt.Format(time.RFC3339) + "\n")
	}
	sb.WriteString(fmt.Sprintf("msgCount: %d\n", c.MsgCount))
	sb.WriteString("---\n\n")

	body := strings.TrimSpace(c.Body)
	if body == "" {
		body = defaultChatBody(c.Title)
	}
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

// defaultChatBody returns the initial markdown sections for a new chat.
func defaultChatBody(title string) string {
	if strings.TrimSpace(title) == "" {
		title = "群聊"
	}
	return fmt.Sprintf(`# %s

## 基础信息
- (AI 通过 chat_note 工具追加此处)

## 群规则
-

## 重要议题
-

## 待跟进
-
`, title)
}
