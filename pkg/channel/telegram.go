// Package channel — Telegram Bot integration using long-polling.
// Reference: openclaw/src/telegram/bot.ts, fetch.ts, send.ts
package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// RunnerFunc executes an agent turn and returns the full text response.
type RunnerFunc func(ctx context.Context, agentID, message string) (string, error)

// ── Telegram types ────────────────────────────────────────────────────────

type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

type TelegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      TelegramUser `json:"from"`
	Chat      TelegramChat `json:"chat"`
	Text      string       `json:"text"`
	Date      int64        `json:"date"`
}

type TelegramUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// ── TelegramBot ───────────────────────────────────────────────────────────

type TelegramBot struct {
	token     string
	agentID   string   // default agent to route messages to
	allowFrom []int64  // allowed sender IDs (empty = allow all)
	runner    RunnerFunc
	client    *http.Client
	offset    int64
}

// NewTelegramBot creates a new Telegram bot with long polling.
func NewTelegramBot(token, agentID string, allowFrom []int64, runner RunnerFunc) *TelegramBot {
	return &TelegramBot{
		token:     token,
		agentID:   agentID,
		allowFrom: allowFrom,
		runner:    runner,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Start runs the long-poll loop. Blocks until ctx is cancelled.
func (b *TelegramBot) Start(ctx context.Context) {
	log.Printf("[telegram] Bot started, polling for updates (agent=%s)", b.agentID)
	for {
		select {
		case <-ctx.Done():
			log.Println("[telegram] Bot stopped")
			return
		default:
		}

		updates, err := b.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[telegram] getUpdates error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			b.handleUpdate(ctx, u)
		}
	}
}

func (b *TelegramBot) getUpdates(ctx context.Context) ([]TelegramUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30&allowed_updates=[\"message\"]",
		b.token, b.offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
		Desc   string           `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram api: %s", result.Desc)
	}
	return result.Result, nil
}

func (b *TelegramBot) handleUpdate(ctx context.Context, update TelegramUpdate) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	msg := update.Message
	senderID := msg.From.ID

	// Check allow list
	if len(b.allowFrom) > 0 {
		allowed := false
		for _, id := range b.allowFrom {
			if id == senderID {
				allowed = true
				break
			}
		}
		if !allowed {
			log.Printf("[telegram] Blocked message from unauthorized user %d (%s)", senderID, msg.From.Username)
			return
		}
	}

	log.Printf("[telegram] Message from %s (%d): %s", msg.From.Username, senderID, truncate(msg.Text, 80))

	// Run agent turn
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		output, err := b.runner(runCtx, b.agentID, msg.Text)
		if err != nil {
			log.Printf("[telegram] Runner error: %v", err)
			_ = b.SendMessage(msg.Chat.ID, "⚠️ Error: "+err.Error())
			return
		}

		if output == "" {
			output = "(no response)"
		}

		// Telegram has 4096 char limit; split if needed
		for len(output) > 0 {
			chunk := output
			if len(chunk) > 4000 {
				chunk = output[:4000]
				output = output[4000:]
			} else {
				output = ""
			}
			if err := b.SendMessage(msg.Chat.ID, chunk); err != nil {
				log.Printf("[telegram] SendMessage error: %v", err)
				return
			}
		}
	}()
}

// SendMessage sends a text message to a Telegram chat.
func (b *TelegramBot) SendMessage(chatID int64, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	payload, _ := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	resp, err := b.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram sendMessage: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
