// Package channel — Telegram Bot integration with streaming draft and group support.
// Design mirrors OpenClaw's telegram implementation:
//   - sendChatAction "typing" kept alive during generation
//   - Streaming: first chunk → sendMessage; subsequent chunks → editMessageText (throttled 1s)
//   - Group chats: respond only when @mentioned or replied-to
//   - Pairing mode when no allowFrom is configured
package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── Event types ───────────────────────────────────────────────────────────

// StreamEvent is a simplified event emitted during streaming generation.
type StreamEvent struct {
	Type string // "text_delta" | "error" | "done"
	Text string
	Err  error
}

// RunnerFunc executes an agent turn and returns the full text response.
type RunnerFunc func(ctx context.Context, agentID, message string) (string, error)

// StreamFunc executes an agent turn and returns a live StreamEvent channel.
type StreamFunc func(ctx context.Context, agentID, message string) (<-chan StreamEvent, error)

// ── Telegram API types ────────────────────────────────────────────────────

type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

type TelegramMessage struct {
	MessageID      int64            `json:"message_id"`
	From           TelegramUser     `json:"from"`
	Chat           TelegramChat     `json:"chat"`
	Text           string           `json:"text"`
	Date           int64            `json:"date"`
	ReplyToMessage *TelegramMessage `json:"reply_to_message,omitempty"`
	Entities       []TelegramEntity `json:"entities,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	IsBot     bool   `json:"is_bot"`
}

type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private" | "group" | "supergroup" | "channel"
}

type TelegramEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// ── TelegramBot ───────────────────────────────────────────────────────────

type TelegramBot struct {
	token        string
	agentID      string
	allowFrom    []int64
	streamFunc   StreamFunc
	client       *http.Client
	offset       int64
	pendingStore *PendingStore
	// resolved on Start via getMe
	botID       int64
	botUsername string
	mu          sync.Mutex
}

// NewTelegramBot creates a Telegram bot that supports streaming and group chats.
func NewTelegramBot(token, agentID string, allowFrom []int64, runner RunnerFunc, pending *PendingStore) *TelegramBot {
	// Wrap the sync runner in a StreamFunc for backward compat when no stream func is set
	sf := func(ctx context.Context, agentID, message string) (<-chan StreamEvent, error) {
		ch := make(chan StreamEvent, 1)
		go func() {
			defer close(ch)
			text, err := runner(ctx, agentID, message)
			if err != nil {
				ch <- StreamEvent{Type: "error", Err: err}
				return
			}
			// Emit in chunks so the edit loop has something to work with
			chunk := 60
			for i := 0; i < len(text); i += chunk {
				end := i + chunk
				if end > len(text) {
					end = len(text)
				}
				ch <- StreamEvent{Type: "text_delta", Text: text[i:end]}
			}
			ch <- StreamEvent{Type: "done"}
		}()
		return ch, nil
	}
	return &TelegramBot{
		token:        token,
		agentID:      agentID,
		allowFrom:    allowFrom,
		streamFunc:   sf,
		client:       &http.Client{Timeout: 90 * time.Second},
		pendingStore: pending,
	}
}

// NewTelegramBotWithStream creates a bot that uses a real StreamFunc.
func NewTelegramBotWithStream(token, agentID string, allowFrom []int64, sf StreamFunc, pending *PendingStore) *TelegramBot {
	return &TelegramBot{
		token:        token,
		agentID:      agentID,
		allowFrom:    allowFrom,
		streamFunc:   sf,
		client:       &http.Client{Timeout: 90 * time.Second},
		pendingStore: pending,
	}
}

// Start runs the long-poll loop. Fetches bot info first, then polls.
func (b *TelegramBot) Start(ctx context.Context) {
	// Fetch bot identity
	if err := b.fetchBotInfo(ctx); err != nil {
		log.Printf("[telegram] getMe failed: %v", err)
	} else {
		log.Printf("[telegram] Bot started: @%s (id=%d, agent=%s)", b.botUsername, b.botID, b.agentID)
	}
	b.pollLoop(ctx)
}

func (b *TelegramBot) fetchBotInfo(ctx context.Context) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", b.token)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		OK     bool         `json:"ok"`
		Desc   string       `json:"description"`
		Result TelegramUser `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return fmt.Errorf("getMe: %s", result.Desc)
	}
	b.botID = result.Result.ID
	b.botUsername = result.Result.Username
	return nil
}

func (b *TelegramBot) pollLoop(ctx context.Context) {
	log.Printf("[telegram] Polling updates (agent=%s)", b.agentID)
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
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
		Desc   string           `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram api: %s", result.Desc)
	}
	return result.Result, nil
}

func (b *TelegramBot) handleUpdate(ctx context.Context, update TelegramUpdate) {
	msg := update.Message
	if msg == nil || msg.Text == "" {
		return
	}

	isGroup := msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
	isStart := msg.Text == "/start"
	senderID := msg.From.ID

	// ── Group chat filtering ──────────────────────────────────────────────
	// In group chats only respond when @mentioned or the message is a reply to the bot.
	if isGroup {
		if !b.isAddressedToBot(msg) {
			return
		}
	}

	// ── Access control ────────────────────────────────────────────────────
	if len(b.allowFrom) == 0 {
		// Pairing mode: tell user their ID
		log.Printf("[telegram] Pairing mode — user %d (%s) in chat %d", senderID, msg.From.Username, msg.Chat.ID)
		if b.pendingStore != nil {
			b.pendingStore.Add(senderID, msg.From.Username, msg.From.FirstName)
		}
		pairMsg := fmt.Sprintf(
			"👋 你好！此 Bot 尚未完成配对。\n\n请将以下信息发送给管理员，管理员将你加入白名单后即可开始使用：\n\n🔑 你的 Telegram ID：<code>%d</code>",
			senderID,
		)
		_ = b.sendHTML(msg.Chat.ID, pairMsg, 0, 0)
		return
	}

	allowed := false
	for _, id := range b.allowFrom {
		if id == senderID {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Printf("[telegram] Pending user %d (%s)", senderID, msg.From.Username)
		if b.pendingStore != nil {
			b.pendingStore.Add(senderID, msg.From.Username, msg.From.FirstName)
		}
		if isStart {
			_ = b.sendHTML(msg.Chat.ID, "👋 你好！你的申请已收到，等待管理员审核后即可使用。", 0, 0)
		}
		return
	}

	// Allowed user: clean pending entry
	if b.pendingStore != nil {
		b.pendingStore.Remove(senderID)
	}

	// Strip bot @mention from message text
	text := b.cleanMessageText(msg.Text)
	if text == "" && isStart {
		text = "你好"
	}
	if text == "" {
		return
	}

	replyToMsgID := int64(0)
	if isGroup {
		replyToMsgID = msg.MessageID // reply in thread
	}

	log.Printf("[telegram] Processing: chat=%d user=%s text=%q", msg.Chat.ID, msg.From.Username, truncate(text, 60))

	go b.generateAndSend(ctx, msg.Chat.ID, text, replyToMsgID)
}

// isAddressedToBot returns true if the group message targets this bot.
func (b *TelegramBot) isAddressedToBot(msg *TelegramMessage) bool {
	// Check @mention in entities
	if b.botUsername != "" {
		mention := "@" + b.botUsername
		for _, entity := range msg.Entities {
			if entity.Type == "mention" {
				runes := []rune(msg.Text)
				if entity.Offset+entity.Length <= len(runes) {
					entityText := string(runes[entity.Offset : entity.Offset+entity.Length])
					if strings.EqualFold(entityText, mention) {
						return true
					}
				}
			}
		}
	}
	// Check reply to bot's message
	if msg.ReplyToMessage != nil && b.botID != 0 {
		if msg.ReplyToMessage.From.ID == b.botID {
			return true
		}
	}
	return false
}

// cleanMessageText removes the bot @mention from message text.
func (b *TelegramBot) cleanMessageText(text string) string {
	if b.botUsername == "" {
		return strings.TrimSpace(text)
	}
	mention := "@" + b.botUsername
	cleaned := strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(cleaned)
}

// generateAndSend runs the agent and streams the response via send+edit draft pattern.
func (b *TelegramBot) generateAndSend(ctx context.Context, chatID int64, message string, replyToMsgID int64) {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Start typing indicator (kept alive every 4s)
	typingCtx, stopTyping := context.WithCancel(runCtx)
	defer stopTyping()
	go b.keepTyping(typingCtx, chatID)

	events, err := b.streamFunc(runCtx, b.agentID, message)
	if err != nil {
		stopTyping()
		_, _ = b.sendPlain(chatID, "⚠️ 出错了："+err.Error(), replyToMsgID)
		return
	}

	// Draft stream: accumulate text, send first chunk, then edit
	var accumulated strings.Builder
	var sentMsgID int64
	lastSent := ""
	throttle := time.NewTicker(1 * time.Second)
	defer throttle.Stop()

	sendOrEdit := func(text string, isFinal bool) {
		if text == "" || text == lastSent {
			return
		}
		lastSent = text
		if sentMsgID == 0 {
			// First send
			id, err := b.sendPlain(chatID, text, replyToMsgID)
			if err != nil {
				log.Printf("[telegram] sendMessage error: %v", err)
				return
			}
			sentMsgID = id
			return
		}
		// Edit existing message
		if err := b.editMessage(chatID, sentMsgID, text); err != nil {
			// Edit can fail if message is too old or identical — ignore
			log.Printf("[telegram] editMessage warning: %v", err)
		}
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				goto done
			}
			switch ev.Type {
			case "text_delta":
				accumulated.WriteString(ev.Text)
			case "error":
				if ev.Err != nil {
					accumulated.WriteString("\n⚠️ " + ev.Err.Error())
				}
			case "done":
				goto done
			}
		case <-throttle.C:
			sendOrEdit(accumulated.String(), false)
		}
	}

done:
	stopTyping()
	sendOrEdit(accumulated.String(), true)

	// If nothing was sent at all (edge case), send a fallback
	if sentMsgID == 0 && accumulated.Len() == 0 {
		_, _ = b.sendPlain(chatID, "(no response)", replyToMsgID)
	}
}

// keepTyping sends "typing" chat action every 4 seconds until ctx is cancelled.
func (b *TelegramBot) keepTyping(ctx context.Context, chatID int64) {
	_ = b.sendChatAction(chatID, "typing")
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = b.sendChatAction(chatID, "typing")
		}
	}
}

// ── API helpers ───────────────────────────────────────────────────────────

func (b *TelegramBot) apiPost(endpoint string, payload any) ([]byte, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, endpoint)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func (b *TelegramBot) sendChatAction(chatID int64, action string) error {
	_, err := b.apiPost("sendChatAction", map[string]any{
		"chat_id": chatID,
		"action":  action,
	})
	return err
}

// sendPlain sends a plain text message and returns the message ID.
func (b *TelegramBot) sendPlain(chatID int64, text string, replyToMsgID int64) (int64, error) {
	// Split if > 4096 chars
	if len(text) > 4096 {
		text = text[:4096]
	}
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if replyToMsgID > 0 {
		payload["reply_to_message_id"] = replyToMsgID
	}
	body, err := b.apiPost("sendMessage", payload)
	if err != nil {
		return 0, err
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return 0, fmt.Errorf("sendMessage: %s", result.Description)
	}
	return result.Result.MessageID, nil
}

// sendHTML sends a message with HTML parse mode.
func (b *TelegramBot) sendHTML(chatID int64, html string, replyToMsgID int64, _ int64) error {
	if len(html) > 4096 {
		html = html[:4096]
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       html,
		"parse_mode": "HTML",
	}
	if replyToMsgID > 0 {
		payload["reply_to_message_id"] = replyToMsgID
	}
	_, err := b.apiPost("sendMessage", payload)
	return err
}

// editMessage edits an existing message.
func (b *TelegramBot) editMessage(chatID, messageID int64, text string) error {
	if len(text) > 4096 {
		text = text[:4096]
	}
	body, err := b.apiPost("editMessageText", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	})
	if err != nil {
		return err
	}
	// "message is not modified" is not an error
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}
	if !result.OK && !strings.Contains(result.Description, "not modified") {
		return fmt.Errorf("editMessageText: %s", result.Description)
	}
	return nil
}

// SendMessage sends a plain text message to a chat (public API for external callers).
func (b *TelegramBot) SendMessage(chatID int64, text string) error {
	_, err := b.sendPlain(chatID, text, 0)
	return err
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
			Username  string `json:"username"`
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Ensure RunnerFunc is exported so other packages can reference it cleanly.
