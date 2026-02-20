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

// Ensure RunnerFunc is exported so other packages can reference it cleanly.

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
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// ── TelegramBot ───────────────────────────────────────────────────────────

type TelegramBot struct {
	token        string
	agentID      string   // default agent to route messages to
	allowFrom    []int64  // allowed sender IDs (empty = allow all)
	runner       RunnerFunc
	client       *http.Client
	offset       int64
	pendingStore *PendingStore // tracks users who messaged but aren't allowed yet
}

// NewTelegramBot creates a new Telegram bot with long polling.
func NewTelegramBot(token, agentID string, allowFrom []int64, runner RunnerFunc, pending *PendingStore) *TelegramBot {
	return &TelegramBot{
		token:        token,
		agentID:      agentID,
		allowFrom:    allowFrom,
		runner:       runner,
		client:       &http.Client{Timeout: 60 * time.Second},
		pendingStore: pending,
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
	isStart := msg.Text == "/start"

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
			log.Printf("[telegram] Pending user %d (%s): %s", senderID, msg.From.Username, truncate(msg.Text, 40))
			// Log to pending store so admin can approve from UI
			if b.pendingStore != nil {
				b.pendingStore.Add(senderID, msg.From.Username, msg.From.FirstName)
			}
			// Send a friendly reply if it's /start or first contact
			if isStart {
				_ = b.SendMessage(msg.Chat.ID, "👋 你好！你的请求已收到，等待管理员审核后即可使用。\n\nYour request has been received. Please wait for admin approval.")
			}
			return
		}
	}

	// For allowed users sending /start: record them too (so admin knows who's using the bot)
	if isStart && b.pendingStore != nil {
		// Remove from pending (they're already allowed) and just greet
		b.pendingStore.Remove(senderID)
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

// TestTelegramBot calls getMe to verify a bot token. Returns the bot username on success.
func TestTelegramBot(ctx context.Context, token string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		OK     bool   `json:"ok"`
		Desc   string `json:"description"`
		Result struct {
			Username string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("telegram api: %s", result.Desc)
	}
	name := result.Result.Username
	if name == "" {
		name = result.Result.FirstName
	}
	return name, nil
}
