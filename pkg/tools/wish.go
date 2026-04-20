// pkg/tools/wish.go — WISHLIST tools.
//
// 让 AI 能主动表达"我想要的新能力"：
//   - wish_add({title, reason, priority?}) — 追加一条到 workspace/WISHLIST.md
//   - wish_list({limit?})                   — 读取并返回结构化 JSON
//
// 面板前端会在成员页展示 badge + 详情，用户可以据此决定是否启用对应能力。
//
// 设计哲学：
//   AI 不应该只是"执行指令的工具"，也应该有表达能力需求的通道。
//   这把"AI 自主驱动的能力扩展"变成平台的一等公民。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// WishEntry 是 WISHLIST.md 中一条愿望的结构化表示。
type WishEntry struct {
	Title     string `json:"title"`
	Reason    string `json:"reason"`
	Priority  string `json:"priority"`  // "P0" | "P1" | "P2" | "" (default)
	CreatedAt string `json:"createdAt"` // RFC3339
}

// wishlistFileName 在 workspace 下固定命名。
const wishlistFileName = "WISHLIST.md"

var wishAddDef = llm.ToolDef{
	Name: "wish_add",
	Description: "把一条能力愿望追加到 WISHLIST.md。当你发现某项能力缺失（如联网搜索、数据库、邮件、PDF解析等），或想要某个新工具/集成时调用此工具。" +
		"用户会在面板成员页看到你的愿望，并可能为你启用。每次只记录一条，描述要具体。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "愿望标题（简短），如 '联网搜索' '连接 MySQL' 'PDF 深度解析'"},
			"reason": {"type": "string", "description": "为什么想要？（一两句话说清楚场景与价值）"},
			"priority": {"type": "string", "enum": ["P0", "P1", "P2"], "description": "优先级: P0=极度渴望 P1=非常想要 P2=锦上添花（可选，默认 P1）"}
		},
		"required": ["title", "reason"]
	}`),
}

var wishListDef = llm.ToolDef{
	Name:        "wish_list",
	Description: "读取 WISHLIST.md，返回当前已记录的所有愿望（JSON 数组）。用于回顾已经记录过哪些需求，避免重复记录。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"limit": {"type": "integer", "description": "最多返回多少条（可选，默认全部）"}
		}
	}`),
}

func (r *Registry) handleWishAdd(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Title    string `json:"title"`
		Reason   string `json:"reason"`
		Priority string `json:"priority"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	if req.Reason == "" {
		return "", fmt.Errorf("reason is required")
	}
	if req.Priority == "" {
		req.Priority = "P1"
	}

	path := filepath.Join(r.workspaceDir, wishlistFileName)
	// 如果文件不存在，先写入头部
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := "# 我的能力愿望清单\n\n" +
			"> 这是我（AI 成员）希望拥有的新能力清单。用户会看到并决定是否为我启用。\n\n"
		if err := os.WriteFile(path, []byte(header), 0644); err != nil {
			return "", fmt.Errorf("create WISHLIST.md: %w", err)
		}
	}

	now := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("\n## %s · %s · %s\n- **理由**: %s\n",
		now, req.Title, req.Priority, req.Reason)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("open WISHLIST.md: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(entry); err != nil {
		return "", fmt.Errorf("append WISHLIST.md: %w", err)
	}
	return fmt.Sprintf("✅ 已记录愿望「%s」(%s) 到 %s", req.Title, req.Priority, wishlistFileName), nil
}

func (r *Registry) handleWishList(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(input, &req)

	wishes, err := ReadWishlist(r.workspaceDir)
	if err != nil {
		return "", err
	}
	if req.Limit > 0 && len(wishes) > req.Limit {
		wishes = wishes[:req.Limit]
	}
	out, err := json.MarshalIndent(map[string]any{
		"total":  len(wishes),
		"wishes": wishes,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ReadWishlist 解析 workspace 下的 WISHLIST.md 为结构化列表。
// 支持简单格式: "## <时间> · <标题> · <优先级>\n- **理由**: <reason>\n"
// 老格式兼容: "## <日期>\n- **<标题>**\n  - 理由: <reason>"
// 导出供 internal/api 层使用。
func ReadWishlist(workspaceDir string) ([]WishEntry, error) {
	path := filepath.Join(workspaceDir, wishlistFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []WishEntry{}, nil
		}
		return nil, err
	}
	content := string(data)

	var out []WishEntry
	// 简单 regex: 匹配 "## <header>" 到下一个 "## " 或文末
	re := regexp.MustCompile(`(?m)^## +([^\n]+)\n([\s\S]*?)(?:\n## |\z)`)
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		header := strings.TrimSpace(m[1])
		body := strings.TrimSpace(m[2])

		// 解析 header: "2026-04-20 10:32 · 联网搜索 · P0"  或 "2026-04-20"
		parts := strings.Split(header, "·")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		w := WishEntry{}
		if len(parts) >= 3 {
			w.CreatedAt = parts[0]
			w.Title = parts[1]
			w.Priority = parts[2]
		} else {
			w.CreatedAt = header
			w.Title = header
		}

		// body: 尝试提取 "- **理由**: ..."  或 "- 理由: ..."
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if idx := strings.Index(line, "理由"); idx != -1 {
				rest := line[idx+len("理由"):]
				rest = strings.TrimLeft(rest, "*:：- ")
				rest = strings.TrimSpace(rest)
				if rest != "" {
					w.Reason = rest
					break
				}
			}
		}
		// 如果 title 是日期而没解出真 title，尝试从 body 里第一个加粗的文本或第一行提取
		if w.Title == w.CreatedAt || w.Title == "" {
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "- ") {
					t := strings.TrimPrefix(line, "- ")
					t = strings.TrimPrefix(t, "**")
					t = strings.TrimSuffix(t, "**")
					// 只取粗体内部
					if end := strings.Index(t, "**"); end > 0 {
						t = t[:end]
					}
					t = strings.TrimSpace(t)
					if t != "" {
						w.Title = t
						break
					}
				}
			}
		}
		if w.Title != "" {
			out = append(out, w)
		}
	}
	return out, nil
}