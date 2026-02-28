package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// MessageSenderFunc is a function that proactively sends a text message through
// the agent's configured channel (e.g. Telegram).
// Provided by the channel layer (BotPool) and injected per-agent at registry build time.
// Sends to all authorised users of the agent's first active Telegram channel.
type MessageSenderFunc func(ctx context.Context, text string) error

// WithMessageSender registers the send_message tool into the registry.
// If sender is nil, the tool is not registered (graceful degradation).
//
// Design: this is the key enabler of the cron "delivery=none + conditional push" pattern.
// Cron jobs with delivery=none run silently in isolated sessions; the agent calls
// send_message only when it judges the content to be significant enough to notify the user.
func (r *Registry) WithMessageSender(sender MessageSenderFunc) *Registry {
	if sender == nil {
		return r
	}

	r.register(llm.ToolDef{
		Name:        "send_message",
		Description: "向用户主动发送一条消息（通过当前智能成员的 Telegram 频道）。适用于后台任务、定时监控、突发预警等需要主动推送的场景。只在有真正值得通知的内容时才调用，避免刷屏。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"text": {
					"type": "string",
					"description": "要发送的消息内容，支持 Markdown 格式"
				}
			},
			"required": ["text"]
		}`),
	}, func(ctx context.Context, input json.RawMessage) (string, error) {
		var params struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return "", fmt.Errorf("send_message: invalid params: %w", err)
		}
		if params.Text == "" {
			return "", fmt.Errorf("send_message: text is required")
		}
		if err := sender(ctx, params.Text); err != nil {
			return "", fmt.Errorf("send_message: %w", err)
		}
		return "消息已发送", nil
	})

	return r
}
