// pkg/tools/network.go — 通讯录（network/）相关工具。
//
// 第一版只提供一个工具 `network_note`：把一条事实/偏好/待跟进追加到指定
// 联系人档案。完整档案浏览使用通用 `read` 工具，关系编辑使用 `edit`。
// 这是"提示词披露工程"的一部分：让 AI 只在真正发现新信息时才动
// 文件系统，而不是每次对话都重写档案。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/network"
)

var networkNoteDef = llm.ToolDef{
	Name: "network_note",
	Description: "往指定联系人档案的对应段落追加一条事实/偏好/待跟进。" +
		"当你在对话中发现关于对方的重要信息（姓名、职业、关系、偏好、待办等），请用此工具长期记录。" +
		"档案路径 network/contacts/<id>.md 会被你在下次对话时通过 INDEX 看到。" +
		"entityId 必须是标准形式 source:externalId（如 feishu:ou_abc, telegram:123456, web:sid-xxx）。" +
		"section 只允许: 事实 | 偏好 | 最近话题 | 待跟进。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"entityId": {"type": "string", "description": "联系人 ID，格式 source:externalId"},
			"section":  {"type": "string", "enum": ["事实", "偏好", "最近话题", "待跟进"], "description": "追加到哪个段"},
			"text":     {"type": "string", "description": "要记录的一条内容（一行一句，不要太长）"}
		},
		"required": ["entityId", "section", "text"]
	}`),
}

func (r *Registry) handleNetworkNote(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		EntityID string `json:"entityId"`
		Section  string `json:"section"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	req.EntityID = strings.TrimSpace(req.EntityID)
	req.Section = strings.TrimSpace(req.Section)
	req.Text = strings.TrimSpace(req.Text)
	if req.EntityID == "" {
		return "", fmt.Errorf("entityId is required")
	}
	if req.Section == "" {
		return "", fmt.Errorf("section is required")
	}
	if req.Text == "" {
		return "", fmt.Errorf("text is required")
	}
	allowedSections := map[string]bool{"事实": true, "偏好": true, "最近话题": true, "待跟进": true}
	if !allowedSections[req.Section] {
		return "", fmt.Errorf("section must be one of 事实/偏好/最近话题/待跟进, got %q", req.Section)
	}

	store := network.NewStore(r.workspaceDir)
	c, err := store.Get(req.EntityID)
	if err != nil {
		return "", fmt.Errorf("load contact: %w", err)
	}
	if c == nil {
		return "", fmt.Errorf("contact %q not found (请先让用户通过该来源发过一次消息, 或手动在通讯录创建)", req.EntityID)
	}

	updated, err := appendToSection(c.Body, req.Section, req.Text)
	if err != nil {
		return "", err
	}
	c.Body = updated
	if err := store.Save(c); err != nil {
		return "", fmt.Errorf("save contact: %w", err)
	}

	// Record a lightweight side-log for observability
	_ = appendNetworkChangeLog(r.workspaceDir, req.EntityID, req.Section, req.Text)

	return fmt.Sprintf("✅ 已在 %s 的「%s」段追加一条记录", displayNameOrID(c), req.Section), nil
}

func displayNameOrID(c *network.Contact) string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.ID
}

// appendToSection takes a contact body (markdown) and appends `- <text>` at the
// end of the section identified by `sectionName` (match on "## <name>" line,
// substring allowed). If the section does not exist, it is created at the end.
//
// Placeholder lines like "- (AI 通过 network_note 工具追加此处)" are stripped
// when the section becomes non-empty, keeping the file tidy.
func appendToSection(body, sectionName, text string) (string, error) {
	body = strings.TrimRight(body, "\n")
	lines := strings.Split(body, "\n")

	sectionStart := -1
	sectionEnd := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && strings.Contains(trimmed, sectionName) {
			sectionStart = i
			// Find end of this section (next "## " or EOF)
			sectionEnd = len(lines)
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}

	newEntry := fmt.Sprintf("- %s", text)

	if sectionStart < 0 {
		// Section not found — append a new one.
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += fmt.Sprintf("\n## %s\n%s\n", sectionName, newEntry)
		return body, nil
	}

	// Strip placeholder lines inside the found section.
	var kept []string
	for j := sectionStart; j < sectionEnd; j++ {
		l := lines[j]
		trimmed := strings.TrimSpace(l)
		// Placeholder looks like "- (AI ... )" or just "-" — keep header and non-placeholder items.
		if j == sectionStart {
			kept = append(kept, l) // header
			continue
		}
		if trimmed == "-" || (strings.HasPrefix(trimmed, "- (") && strings.HasSuffix(trimmed, ")")) {
			continue
		}
		kept = append(kept, l)
	}
	// Append new entry before the next section or EOF (i.e. at end of kept slice).
	// Make sure it doesn't duplicate a blank line at the end of the section.
	// Trim trailing blanks inside the section:
	for len(kept) > 1 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	kept = append(kept, newEntry)

	// Rebuild: lines[:sectionStart] + kept + lines[sectionEnd:]
	var out []string
	out = append(out, lines[:sectionStart]...)
	out = append(out, kept...)
	if sectionEnd < len(lines) {
		// Ensure blank line before next section header for readability.
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, lines[sectionEnd:]...)
	}
	result := strings.Join(out, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

// appendNetworkChangeLog writes a one-line audit record to network/changes.log
// so users/admins can see what the AI has been modifying.
func appendNetworkChangeLog(workspaceDir, entityID, section, text string) error {
	logPath := filepath.Join(workspaceDir, "network", "changes.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	line := fmt.Sprintf("%s  note  %s  %s  %s\n",
		time.Now().Format(time.RFC3339),
		entityID, section, strings.ReplaceAll(text, "\n", " "))
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
