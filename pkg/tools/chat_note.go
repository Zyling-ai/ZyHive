// pkg/tools/chat_note.go — 群档案 (chats/) 维护工具.
//
// 与 network.go 的 network_note 工具对称: 把群规则 / 重要议题 / 待跟进 等
// 关于群的发现追加到 workspace/network/chats/<id>.md 对应段落.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/network"
)

var chatNoteDef = llm.ToolDef{
	Name: "chat_note",
	Description: "往指定群聊档案的对应段落追加一条记录 (规则 / 议题 / 待跟进 等). " +
		"当你在群聊中发现群规则 / 重要议题 / 群组事实 / 待跟进事项时, 用此工具长期记录. " +
		"档案路径 network/chats/<id>.md, 你可以通过 INDEX 看到所有群档案. " +
		"chatId 必须是标准形式 source:externalChatId (如 feishu:oc_abc, telegram:-1001234). " +
		"section 只允许: 基础信息 | 群规则 | 重要议题 | 待跟进.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"chatId":  {"type": "string", "description": "群聊 ID, 格式 source:externalChatId"},
			"section": {"type": "string", "enum": ["基础信息", "群规则", "重要议题", "待跟进"], "description": "追加到哪个段"},
			"text":    {"type": "string", "description": "要记录的一条内容 (一行一句, 不要太长)"}
		},
		"required": ["chatId", "section", "text"]
	}`),
}

func (r *Registry) handleChatNote(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		ChatID  string `json:"chatId"`
		Section string `json:"section"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	req.ChatID = strings.TrimSpace(req.ChatID)
	req.Section = strings.TrimSpace(req.Section)
	req.Text = strings.TrimSpace(req.Text)
	if req.ChatID == "" {
		return "", fmt.Errorf("chatId is required")
	}
	if req.Section == "" {
		return "", fmt.Errorf("section is required")
	}
	if req.Text == "" {
		return "", fmt.Errorf("text is required")
	}
	allowedSections := map[string]bool{"基础信息": true, "群规则": true, "重要议题": true, "待跟进": true}
	if !allowedSections[req.Section] {
		return "", fmt.Errorf("section must be one of 基础信息/群规则/重要议题/待跟进, got %q", req.Section)
	}

	store := network.NewStore(r.workspaceDir)
	c, err := store.GetChat(req.ChatID)
	if err != nil {
		return "", fmt.Errorf("load chat: %w", err)
	}
	if c == nil {
		// Help the AI self-correct: suggest the 3 closest existing chat IDs
		// by prefix/substring match so it can retry with a valid chatId.
		suggestions := suggestChatIDs(store, req.ChatID, 3)
		hint := ""
		if len(suggestions) > 0 {
			hint = " · Did you mean: " + strings.Join(suggestions, " / ") + "?"
		} else {
			hint = " · 当前无群档案, 群档案在飞书/TG群消息进入时自动创建"
		}
		return "", fmt.Errorf("chat %q not found%s", req.ChatID, hint)
	}

	// Reuse appendToSection from network.go (package-level helper, sister tool).
	updated, err := appendToSection(c.Body, req.Section, req.Text)
	if err != nil {
		return "", err
	}
	c.Body = updated
	if err := store.SaveChat(c); err != nil {
		return "", fmt.Errorf("save chat: %w", err)
	}

	// Audit log — prefix the entityID with "chat:" to distinguish from
	// contact entries in the same changes.log file.
	_ = appendNetworkChangeLog(r.workspaceDir, "chat:"+req.ChatID, req.Section, req.Text)

	return fmt.Sprintf("✅ 已在群「%s」的「%s」段追加一条记录", chatDisplayNameOrID(c), req.Section), nil
}

func chatDisplayNameOrID(c *network.Chat) string {
	if strings.TrimSpace(c.Title) != "" {
		return c.Title
	}
	return c.ID
}

// suggestChatIDs returns up to `maxN` chat IDs from the store that are most
// similar to `query`. Mirrors suggestContactIDs but operates over chats.
//
// Ranking (same shape as contacts):
//  1. Source-prefix match (e.g. "feishu:xxx" → other "feishu:" chats first)
//  2. Substring match in full ID
//  3. Title substring match
//  4. Char overlap (rough fuzz)
func suggestChatIDs(store *network.Store, query string, maxN int) []string {
	list, err := store.ListChats()
	if err != nil || len(list) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	qSource := ""
	if i := strings.Index(q, ":"); i > 0 {
		qSource = q[:i]
	}

	ranked := make([]scoredID, 0, len(list))
	for _, c := range list {
		id := c.ID
		idLower := strings.ToLower(id)
		score := 0
		if qSource != "" && strings.HasPrefix(idLower, qSource+":") {
			score += 100
		}
		if strings.Contains(idLower, q) || strings.Contains(q, idLower) {
			score += 50
		}
		if c.Title != "" && strings.Contains(strings.ToLower(c.Title), q) {
			score += 40
		}
		overlap := 0
		qSet := map[rune]bool{}
		for _, r := range q {
			qSet[r] = true
		}
		for _, r := range idLower {
			if qSet[r] {
				overlap++
			}
		}
		score += overlap
		ranked = append(ranked, scoredID{id: id, score: score})
	}
	sortSuggestions(ranked) // reuse stable insertion sort from network.go
	out := make([]string, 0, maxN)
	for i := 0; i < len(ranked) && i < maxN; i++ {
		if ranked[i].score > 0 {
			out = append(out, ranked[i].id)
		}
	}
	return out
}
