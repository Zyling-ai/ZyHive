// pkg/agent/session_context.go — Build "current session" prompt block for
// runner.Config.CurrentSessionContext. Mirror of internal/api helper, lives
// in agent package so pool.go (out-of-process Telegram/Feishu/cron turns)
// can also inject session meta.
package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/session"
)

// BuildSessionContext formats a compact meta block about the given session
// for inclusion in the LLM system prompt. Returns "" when sessionID is empty
// or the session is not found in the store (both benign — nothing to inject).
//
// Target size: <350 chars.
func BuildSessionContext(store *session.Store, sessionID string) string {
	if store == nil || sessionID == "" {
		return ""
	}
	meta, ok := store.GetMeta(sessionID)
	if !ok {
		return ""
	}

	var b strings.Builder
	b.WriteString("## 当前会话\n")
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "(尚未设置)"
	}
	b.WriteString(fmt.Sprintf("- 当前标题: %s\n", title))
	b.WriteString(fmt.Sprintf("- Session ID: %s\n", meta.ID))
	b.WriteString(fmt.Sprintf("- 消息数: %d\n", meta.MessageCount))
	if meta.CreatedAt > 0 {
		b.WriteString(fmt.Sprintf("- 创建于: %s\n",
			time.UnixMilli(meta.CreatedAt).Format("2006-01-02 15:04")))
	}
	if meta.TitleOverridden {
		b.WriteString("- 标题状态: 已手动命名 (除非主题大变, 不要再改)\n")
	} else {
		b.WriteString("- 标题状态: 自动生成, 可在必要时优化\n")
	}

	if titleLooksWeak(title) && !meta.TitleOverridden {
		b.WriteString("\n💡 当前标题信息量不足, 若主题已清晰, 可调用 `session_rename` 设置 8-20 字新标题.\n")
	}
	return b.String()
}

// titleLooksWeak flags titles that likely benefit from a rename — greeting
// openers, placeholders, very short.
func titleLooksWeak(t string) bool {
	if t == "" || t == "(尚未设置)" {
		return true
	}
	t = strings.TrimSuffix(strings.TrimSpace(t), "…")
	if len([]rune(t)) < 6 {
		return true
	}
	weak := []string{"你好", "请问", "hello", "hi ", "ok", "好的", "开始", "测试", "闲聊"}
	lower := strings.ToLower(t)
	for _, w := range weak {
		if strings.HasPrefix(lower, w) {
			return true
		}
	}
	return false
}
