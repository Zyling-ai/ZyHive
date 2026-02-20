// Package memory — agent memory consolidation.
// Reads sessions, deduplicates against today's existing daily log, writes incremental update.
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
	KeepTurns int    `json:"keepTurns"` // Q&A pairs to keep per session after trim
	FocusHint string `json:"focusHint"` // optional hint to LLM on what to record
}

// Consolidate reads all sessions for an agent, writes an incremental daily memory entry,
// and trims sessions to the last N turns.
//
// Returns (written bool, err error):
//   written=true  → new content was appended to today's daily log
//   written=false → no new content (dedup), nothing written
//
// Dedup logic: reads today's existing daily log and passes it to the LLM as context.
// The LLM only outputs new information not already recorded — preventing duplicate entries.
func Consolidate(
	ctx context.Context,
	store *session.Store,
	memTree *MemoryTree,
	agentName string,
	cfg ConsolidateConfig,
	callLLM func(ctx context.Context, system, user string) (string, error),
) (written bool, err error) {
	// ── 1. Load location (Shanghai) ─────────────────────────────────────────
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	todayStr := now.Format("2006-01-02")
	dailyRelPath := fmt.Sprintf("daily/%s/%s/%s.md",
		now.Format("2006"), now.Format("01"), now.Format("02"))

	// ── 2. Gather conversation content from all sessions ─────────────────────
	sessions, err := store.ListSessions()
	if err != nil {
		return false, fmt.Errorf("list sessions: %w", err)
	}

	var convBuf strings.Builder
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
		convBuf.WriteString(fmt.Sprintf("### 会话：%s\n", title))
		if existingSummary != "" {
			convBuf.WriteString(fmt.Sprintf("[压缩摘要] %s\n\n", existingSummary))
		}
		for _, m := range msgs {
			text := extractMsgText(m.Content)
			if text == "" {
				continue
			}
			if m.Role == "user" {
				convBuf.WriteString(fmt.Sprintf("用户：%s\n", text))
			} else {
				convBuf.WriteString(fmt.Sprintf("AI：%s\n\n", text))
			}
		}
		validCount++
	}

	if validCount == 0 {
		return false, nil // no conversations
	}

	// ── 3. Read today's existing daily log (for dedup) ───────────────────────
	existingToday, _ := memTree.GetFile(dailyRelPath)
	existingToday = strings.TrimSpace(existingToday)

	// ── 4. Build focus instruction ───────────────────────────────────────────
	focus := cfg.FocusHint
	if focus == "" {
		focus = "关键信息、重要决策、任务进展、知识积累"
	}

	// ── 5. Call LLM (with dedup context if today has existing content) ───────
	var systemPrompt, userMsg string

	if existingToday != "" {
		// Incremental mode: only output NEW content not in existing records
		systemPrompt = fmt.Sprintf(`你是记忆整理助手。今天（%s）已有如下记忆记录，请对比新对话内容，只输出**尚未记录的新增信息**。

已有记录：
%s

要求：
- 对比已有记录，只输出真正新增的内容
- 如果对话内容与已有记录完全重复，输出：[无新增]
- 输出格式（只输出有内容的分类，没有则跳过）：

### 关键信息
- ...

### 重要决策
- ...

### 任务进展
- ...

### 知识积累
- ...

条目简洁，每条不超过60字，忽略无意义闲聊，不要开头说明语。`, todayStr, existingToday)
		userMsg = fmt.Sprintf("【新对话内容】\nAgent: %s\n时间: %s\n\n%s",
			agentName, now.Format("15:04"), convBuf.String())
	} else {
		// First consolidation of the day
		systemPrompt = fmt.Sprintf(`你是记忆整理助手。请整理以下对话内容，提炼：%s。

输出格式（只输出有内容的分类，没有则跳过）：

### 关键信息
- ...

### 重要决策
- ...

### 任务进展
- ...

### 知识积累
- ...

条目简洁，每条不超过60字，忽略无意义闲聊，不要开头说明语。`, focus)
		userMsg = fmt.Sprintf("Agent: %s\n时间: %s\n\n%s",
			agentName, now.Format("15:04"), convBuf.String())
	}

	summary, err := callLLM(ctx, systemPrompt, userMsg)
	if err != nil {
		return false, fmt.Errorf("llm summarize: %w", err)
	}
	summary = strings.TrimSpace(summary)

	// Check if LLM signals no new content
	if summary == "" || strings.Contains(summary, "[无新增]") {
		return false, nil // dedup: nothing new
	}

	// ── 6. Write incremental entry to today's daily log ──────────────────────
	timeStr := now.Format("15:04")
	entry := fmt.Sprintf("\n## %s 自动整理\n\n%s\n", timeStr, summary)

	if existingToday == "" {
		// First entry: create file with date header
		header := fmt.Sprintf("# %s 记忆日志\n\n> Agent: %s\n", todayStr, agentName)
		if err2 := memTree.WriteFile(dailyRelPath, header+entry); err2 != nil {
			return false, fmt.Errorf("write daily log: %w", err2)
		}
	} else {
		if err2 := memTree.AppendToFile(dailyRelPath, entry); err2 != nil {
			return false, fmt.Errorf("append daily log: %w", err2)
		}
	}

	// ── 7. Trim sessions to last N turns ────────────────────────────────────
	keepMsgs := cfg.KeepTurns * 2
	if keepMsgs < 2 {
		keepMsgs = 6 // default 3 turns
	}
	for _, sess := range sessions {
		_ = store.TrimToLastN(sess.ID, keepMsgs)
	}

	return true, nil
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
