// Package memory — agent memory consolidation.
// Reads all sessions, summarises via LLM, writes to MEMORY.md, trims sessions to last N turns.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
)

// ConsolidateConfig controls how memory consolidation works.
type ConsolidateConfig struct {
	KeepTurns int    `json:"keepTurns"` // Q&A pairs to keep per session after trim (default 3)
	FocusHint string `json:"focusHint"` // optional hint to LLM on what to record
}

// Consolidate reads all sessions for an agent, summarises the content via LLM,
// appends a structured entry to MEMORY.md, and trims each session to the last N turns.
func Consolidate(
	ctx context.Context,
	store *session.Store,
	memTree *MemoryTree,
	agentName string,
	cfg ConsolidateConfig,
	callLLM func(ctx context.Context, system, user string) (string, error),
) error {
	sessions, err := store.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Gather conversation content from all sessions
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent: %s\nTime: %s\n\n", agentName, time.Now().Format("2006-01-02 15:04")))

	validCount := 0
	for _, sess := range sessions {
		msgs, existingSummary, err := store.ReadHistory(sess.ID)
		if err != nil || (len(msgs) == 0 && existingSummary == "") {
			continue
		}
		title := sess.Title
		if title == "" {
			title = sess.ID
		}
		sb.WriteString(fmt.Sprintf("### 会话: %s\n", title))
		if existingSummary != "" {
			sb.WriteString(fmt.Sprintf("[已有摘要]: %s\n\n", existingSummary))
		}
		for _, m := range msgs {
			text := extractMsgText(m.Content)
			if text == "" {
				continue
			}
			if m.Role == "user" {
				sb.WriteString(fmt.Sprintf("用户: %s\n", text))
			} else {
				sb.WriteString(fmt.Sprintf("AI: %s\n\n", text))
			}
		}
		validCount++
	}

	if validCount == 0 {
		return nil // nothing to consolidate
	}

	// Build focus instruction
	focus := cfg.FocusHint
	if focus == "" {
		focus = "提炼关键信息、重要决策、任务进展和知识积累"
	}

	systemPrompt := fmt.Sprintf(`你是记忆整理助手。根据以下对话内容，%s。

输出格式（严格遵守）：
### 关键信息
- ...

### 重要决策
- ...

### 任务进展
- ...

### 知识积累
- ...

要求：条目简洁，每条不超过50字；忽略无意义的闲聊；只输出结构化内容，不要开头的说明语。`, focus)

	summary, err := callLLM(ctx, systemPrompt, sb.String())
	if err != nil {
		return fmt.Errorf("llm summarize: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Errorf("empty summary from LLM")
	}

	// Append to MEMORY.md
	today := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("\n---\n\n## %s 自动记忆整理\n\n%s\n", today, summary)

	if err := memTree.AppendToFile("MEMORY.md", entry); err != nil {
		// If MEMORY.md doesn't exist yet, create it
		if err2 := memTree.WriteFile("MEMORY.md", fmt.Sprintf("# MEMORY.md — 自动记忆\n%s", entry)); err2 != nil {
			return fmt.Errorf("write MEMORY.md: %w", err2)
		}
	}

	// Trim sessions: keep last keepTurns Q&A pairs
	keepMsgs := cfg.KeepTurns * 2
	if keepMsgs < 2 {
		keepMsgs = 6 // default: 3 turns
	}
	for _, sess := range sessions {
		_ = store.TrimToLastN(sess.ID, keepMsgs)
	}

	return nil
}

// extractMsgText pulls plain text from raw message content.
func extractMsgText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return string(content)
}
