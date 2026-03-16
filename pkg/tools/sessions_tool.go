package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/session"
)

// ── Session tool interfaces ────────────────────────────────────────────────────

// SessionSummary is the minimal info shown per session in sessions_list.
type SessionSummary struct {
	AgentID      string
	SessionKey   string
	LastActiveMs int64
	LastMessage  string // snippet of last message
}

// SessionMessage is a single turn returned by ReadHistory.
type SessionMessage struct {
	Role    string
	Content string
}

// SessionLister can list active sessions.
type SessionLister interface {
	ListSessions(limit int) []SessionSummary
}

// SessionHistoryReader can read the history of a specific session.
type SessionHistoryReader interface {
	ReadHistory(sessionKey string, limit int) ([]SessionMessage, error)
}

// SessionSender can send a message to another agent's session.
type SessionSender interface {
	SendToAgent(agentID, message string) (string, error)
}

// sessionToolSet groups the three optional session interfaces.
type sessionToolSet struct {
	lister SessionLister
	reader SessionHistoryReader
	sender SessionSender
}

// ── Store adapters ─────────────────────────────────────────────────────────────

// SessionStoreAdapter wraps a *session.Store to implement the session tool interfaces.
// Use NewSessionStoreAdapter to create one.
type SessionStoreAdapter struct {
	store *session.Store
}

// NewSessionStoreAdapter creates a SessionStoreAdapter around an existing session.Store.
func NewSessionStoreAdapter(store *session.Store) *SessionStoreAdapter {
	return &SessionStoreAdapter{store: store}
}

// ListSessions implements SessionLister using the store's index.
func (a *SessionStoreAdapter) ListSessions(limit int) []SessionSummary {
	entries, err := a.store.ListSessions()
	if err != nil || len(entries) == 0 {
		return nil
	}
	result := make([]SessionSummary, 0, len(entries))
	for _, e := range entries {
		sum := SessionSummary{
			AgentID:      e.AgentID,
			SessionKey:   e.ID,
			LastActiveMs: e.LastAt,
			LastMessage:  truncate(e.Title, 100),
		}
		result = append(result, sum)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// ReadHistory implements SessionHistoryReader using the store.
func (a *SessionStoreAdapter) ReadHistory(sessionKey string, limit int) ([]SessionMessage, error) {
	msgs, _, err := a.store.ReadHistory(sessionKey)
	if err != nil {
		return nil, err
	}
	result := make([]SessionMessage, 0, len(msgs))
	for _, m := range msgs {
		text := extractMessageText(m.Content)
		result = append(result, SessionMessage{Role: m.Role, Content: text})
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// extractMessageText extracts a plain text representation from a message content blob.
func extractMessageText(raw json.RawMessage) string {
	// Try plain string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// ── Tool definitions ──────────────────────────────────────────────────────────

var sessionsListDef = llm.ToolDef{
	Name:        "sessions_list",
	Description: "列出系统中的 Agent 会话（含 agentID、sessionKey、最后活跃时间、最后一条消息摘要）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"limit": {
				"type": "integer",
				"description": "返回条数上限（默认 20）"
			}
		}
	}`),
}

var sessionsHistoryDef = llm.ToolDef{
	Name:        "sessions_history",
	Description: "读取指定会话的对话历史记录（格式化输出 role: content，每条截断到 500 字符）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"sessionKey": {
				"type": "string",
				"description": "会话 ID（从 sessions_list 获取）"
			},
			"limit": {
				"type": "integer",
				"description": "返回消息条数上限（默认 20）"
			}
		},
		"required": ["sessionKey"]
	}`),
}

var sessionsSendDef = llm.ToolDef{
	Name:        "sessions_send",
	Description: "向另一个 Agent 的会话发送消息（用于跨 Agent 通信）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"agentId": {
				"type": "string",
				"description": "目标 Agent 的 ID"
			},
			"message": {
				"type": "string",
				"description": "要发送的消息内容"
			}
		},
		"required": ["agentId", "message"]
	}`),
}

// ── WithSessionTools ──────────────────────────────────────────────────────────

// WithSessionTools registers sessions_list, sessions_history, sessions_send tools.
// Any of lister/reader/sender can be nil; those tools will return "not configured" errors.
func (r *Registry) WithSessionTools(lister SessionLister, reader SessionHistoryReader, sender SessionSender) {
	r.sessionTools = &sessionToolSet{
		lister: lister,
		reader: reader,
		sender: sender,
	}

	r.register(sessionsListDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSessionsList(ctx, input)
	})
	r.register(sessionsHistoryDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSessionsHistory(ctx, input)
	})
	r.register(sessionsSendDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSessionsSend(ctx, input)
	})
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (r *Registry) handleSessionsList(_ context.Context, input json.RawMessage) (string, error) {
	if r.sessionTools == nil || r.sessionTools.lister == nil {
		return "", fmt.Errorf("session lister not configured")
	}
	var p struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(input, &p)
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}

	sessions := r.sessionTools.lister.ListSessions(limit)
	if len(sessions) == 0 {
		return "（暂无会话记录）", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个会话：\n\n", len(sessions)))
	for _, s := range sessions {
		sb.WriteString(fmt.Sprintf("• Agent: %s\n", s.AgentID))
		sb.WriteString(fmt.Sprintf("  Key: %s\n", s.SessionKey))
		if s.LastActiveMs > 0 {
			sb.WriteString(fmt.Sprintf("  最后活跃: %s\n", time.UnixMilli(s.LastActiveMs).Format("2006-01-02 15:04:05")))
		}
		if s.LastMessage != "" {
			sb.WriteString(fmt.Sprintf("  最后消息: %s\n", truncate(s.LastMessage, 100)))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (r *Registry) handleSessionsHistory(_ context.Context, input json.RawMessage) (string, error) {
	if r.sessionTools == nil || r.sessionTools.reader == nil {
		return "", fmt.Errorf("session history reader not configured")
	}
	var p struct {
		SessionKey string `json:"sessionKey"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.SessionKey == "" {
		return "", fmt.Errorf("sessionKey is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}

	msgs, err := r.sessionTools.reader.ReadHistory(p.SessionKey, limit)
	if err != nil {
		return "", fmt.Errorf("读取历史失败: %w", err)
	}
	if len(msgs) == 0 {
		return "（该会话暂无消息记录）", nil
	}

	const maxPerMsg = 500
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("会话 %s 的对话记录（共 %d 条）：\n\n", p.SessionKey, len(msgs)))
	for i, m := range msgs {
		content := m.Content
		if len([]rune(content)) > maxPerMsg {
			runes := []rune(content)
			content = string(runes[:maxPerMsg]) + "…"
		}
		sb.WriteString(fmt.Sprintf("[%d] %s: %s\n\n", i+1, m.Role, content))
	}
	return sb.String(), nil
}

func (r *Registry) handleSessionsSend(_ context.Context, input json.RawMessage) (string, error) {
	if r.sessionTools == nil || r.sessionTools.sender == nil {
		return "", fmt.Errorf("session sender not configured")
	}
	var p struct {
		AgentID string `json:"agentId"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.AgentID == "" {
		return "", fmt.Errorf("agentId is required")
	}
	if p.Message == "" {
		return "", fmt.Errorf("message is required")
	}

	result, err := r.sessionTools.sender.SendToAgent(p.AgentID, p.Message)
	if err != nil {
		return fmt.Sprintf("❌ 发送失败: %v", err), nil
	}
	if result == "" {
		result = "消息已发送"
	}
	return fmt.Sprintf("✅ 已向 Agent %s 发送消息\n%s", p.AgentID, result), nil
}
