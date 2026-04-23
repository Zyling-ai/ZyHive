// pkg/session/retitle.go — Async LLM-based session title refresher.
//
// Triggered by the runner on every "done" event. Milestone-gated so we only
// actually call the LLM when message count crosses 4 / 12 / 30 / 80. User
// manual renames (TitleOverridden=true) are always preserved.
//
// The summarizer prompt is intentionally minimal: feed the last N turns
// (capped), ask for a 8-20 char Chinese title. Cheap model use recommended
// (the UsageRecorder tracks the cost — user decides).
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// TitleSummarizer is the function the runner supplies to let the session
// package call the same LLM without importing pkg/llm directly (avoiding
// circular imports).
//
// Contract:
//   - systemPrompt is the role instruction
//   - userMsg is the conversation excerpt to summarize
//   - returns the title text (no quotes, no "标题:" prefix), or error
type TitleSummarizer func(ctx context.Context, systemPrompt, userMsg string) (string, error)

// retitleSystemPrompt is kept short on purpose — the summarizer doesn't need
// context about ZyHive, just a crisp directive.
const retitleSystemPrompt = `你是会话标题摘要器。根据给出的对话内容，生成一个简洁准确的中文标题，概括本次对话的主题。

要求:
1. 8-20 个汉字（允许英文/数字，但总长度不超过 30 字符）
2. 只输出标题本身，不加引号、不加"标题:"前缀
3. 捕捉核心主题，不要写成"用户问XXX"或"AI回复XXX"这种元描述
4. 如对话仅是寒暄无实质内容，输出: 闲聊
5. 如对话是命令/工具调用结果，输出动作+对象，例如 "生成周报"、"测试登录接口"`

// maxConversationCharsForTitle caps the input we send to the summarizer —
// avoids runaway cost on long sessions. Later messages get priority.
const maxConversationCharsForTitle = 4000

// MaybeAutoRetitle checks if this session crossed a retitle threshold and
// calls the summarizer asynchronously. Safe fire-and-forget from runner.
// If summarizer is nil (no LLM configured) this is a no-op.
func MaybeAutoRetitle(store *Store, sessionID string, summarizer TitleSummarizer) {
	if store == nil || sessionID == "" || summarizer == nil {
		return
	}
	if !store.NeedsAutoRetitle(sessionID) {
		return
	}
	meta, ok := store.GetMeta(sessionID)
	if !ok {
		return
	}
	currentMsgCount := meta.MessageCount
	go func() {
		if err := runAutoRetitle(store, sessionID, currentMsgCount, summarizer); err != nil {
			log.Printf("[session-retitle] %s: %v", sessionID, err)
		}
	}()
}

func runAutoRetitle(store *Store, sessionID string, atMsgCount int, summarizer TitleSummarizer) error {
	msgs, _, err := store.ReadHistory(sessionID)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if len(msgs) < 2 {
		return nil
	}

	convo := buildRetitleInput(msgs, maxConversationCharsForTitle)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	title, err := summarizer(ctx, retitleSystemPrompt, convo)
	if err != nil {
		return fmt.Errorf("summarizer: %w", err)
	}
	title = sanitizeTitle(title)
	if title == "" {
		return nil
	}

	if err := store.UpdateAutoTitle(sessionID, title, atMsgCount); err != nil {
		return fmt.Errorf("update title: %w", err)
	}
	log.Printf("[session-retitle] %s -> %q (at %d msgs)", sessionID, title, atMsgCount)
	return nil
}

// buildRetitleInput concatenates the most-recent messages (truncated) into a
// form the summarizer can digest. Messages are interleaved with role labels.
// maxChars is a hard cap; we keep the TAIL (most recent) since topic drift
// lives in the latest turns. If even a single latest message exceeds the cap,
// we take the last `maxChars` bytes of it.
func buildRetitleInput(msgs []Message, maxChars int) string {
	if len(msgs) == 0 {
		return ""
	}
	// Extract role-prefixed text lines from newest → oldest.
	type line struct{ text string }
	lines := make([]line, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		text := extractTextForTitle(m.Content)
		if text == "" {
			continue
		}
		label := "用户"
		if m.Role == "assistant" {
			label = "AI"
		} else if m.Role != "user" {
			continue
		}
		lines = append(lines, line{text: label + ": " + text})
	}
	if len(lines) == 0 {
		return ""
	}

	// Walk newest-first; keep lines that still fit; stop when next line would overflow.
	kept := make([]string, 0, len(lines))
	remaining := maxChars
	for _, l := range lines {
		ln := len(l.text) + 1 // +1 for newline separator
		if ln <= remaining {
			kept = append(kept, l.text)
			remaining -= ln
			continue
		}
		// Doesn't fit; if we have nothing yet, keep the tail of this single
		// line so summarizer has something (rune-safe truncation).
		if len(kept) == 0 {
			runes := []rune(l.text)
			if len(runes) > maxChars {
				kept = append(kept, string(runes[len(runes)-maxChars:]))
			} else {
				kept = append(kept, l.text)
			}
		}
		break
	}

	// Reverse back to chronological and join.
	var b strings.Builder
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// extractTextForTitle pulls a readable string from a possibly-multimodal
// Message.Content payload. Handles: plain string, content block array.
func extractTextForTitle(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Plain string
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return truncateRune(s, 400)
	}
	// Content block array
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return truncateRune(b.Text, 400)
			}
		}
	}
	return ""
}

// sanitizeTitle cleans a raw LLM title output:
//   - strip surrounding quotes
//   - strip leading "标题:" / "Title:" / numbered list markers
//   - collapse whitespace
//   - truncate to 30 chars (match dashboards / sidebars)
func sanitizeTitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip markdown bold / quotes
	for _, p := range []string{"**", "__"} {
		s = strings.TrimPrefix(s, p)
		s = strings.TrimSuffix(s, p)
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		s = s[1 : len(s)-1]
	}
	if strings.HasPrefix(s, "「") && strings.HasSuffix(s, "」") {
		s = strings.TrimPrefix(s, "「")
		s = strings.TrimSuffix(s, "」")
	}
	// Strip common prefixes
	for _, p := range []string{"标题：", "标题:", "Title:", "title:"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	// Only take first line
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	return truncateRune(s, 30)
}
