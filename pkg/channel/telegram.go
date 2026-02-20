// Package channel — Telegram Bot integration with streaming draft and group support.
// Design mirrors OpenClaw's telegram implementation:
//   - sendChatAction "typing" kept alive during generation
//   - Streaming: first chunk → sendMessage; subsequent chunks → editMessageText (throttled 1s)
//   - Group chats: respond only when @mentioned or replied-to
//   - Pairing mode when no allowFrom is configured
//   - Full media type support: photo, video, audio, voice, sticker, document, animation
//   - Forum thread support, callback query, channel post, ACK reactions
//   - Media group buffering (500ms), Markdown→HTML conversion
package channel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
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

// MediaInput represents a downloaded media file to pass to the LLM.
type MediaInput struct {
	Data        []byte
	ContentType string // "image/jpeg", "image/png", "image/webp", "application/pdf"
	FileName    string
}

// RunnerFunc executes an agent turn and returns the full text response.
type RunnerFunc func(ctx context.Context, agentID, message string) (string, error)

// StreamFunc executes an agent turn and returns a live StreamEvent channel.
type StreamFunc func(ctx context.Context, agentID, message string, media []MediaInput) (<-chan StreamEvent, error)

// ── Telegram API types ────────────────────────────────────────────────────

type TelegramUpdate struct {
	UpdateID      int64                `json:"update_id"`
	Message       *TelegramMessage     `json:"message"`
	ChannelPost   *TelegramMessage     `json:"channel_post"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query"`
}

type TelegramMessage struct {
	MessageID       int64               `json:"message_id"`
	From            TelegramUser        `json:"from"`
	Chat            TelegramChat        `json:"chat"`
	Text            string              `json:"text"`
	Caption         string              `json:"caption,omitempty"`
	MediaGroupID    string              `json:"media_group_id,omitempty"`
	MessageThreadID int64               `json:"message_thread_id,omitempty"`
	Date            int64               `json:"date"`
	ReplyToMessage  *TelegramMessage    `json:"reply_to_message,omitempty"`
	Entities        []TelegramEntity    `json:"entities,omitempty"`
	Photo           []TelegramPhotoSize `json:"photo,omitempty"`
	Video           *TelegramFile       `json:"video,omitempty"`
	Document        *TelegramFile       `json:"document,omitempty"`
	Audio           *TelegramFile       `json:"audio,omitempty"`
	Voice           *TelegramFile       `json:"voice,omitempty"`
	VideoNote       *TelegramFile       `json:"video_note,omitempty"`
	Sticker         *TelegramSticker    `json:"sticker,omitempty"`
	Animation       *TelegramFile       `json:"animation,omitempty"`
	// Forward context (old API)
	ForwardFrom     *TelegramUser       `json:"forward_from,omitempty"`
	ForwardFromChat *TelegramForwardChat `json:"forward_from_chat,omitempty"`
	ForwardDate     int64               `json:"forward_date,omitempty"`
	// Forward origin (Bot API 7.0+)
	ForwardOrigin   *TelegramForwardOrigin `json:"forward_origin,omitempty"`
}

// TelegramForwardChat represents the chat a message was forwarded from.
type TelegramForwardChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// TelegramForwardOrigin represents the new-style forward_origin (Bot API 7.0+).
type TelegramForwardOrigin struct {
	Type        string       `json:"type"` // "user" | "hidden_user" | "chat" | "channel"
	SenderUser  *TelegramUser `json:"sender_user,omitempty"`
	SenderUserName string    `json:"sender_user_name,omitempty"`
	Chat        *TelegramForwardChat `json:"chat,omitempty"`
	Date        int64        `json:"date,omitempty"`
}

type TelegramPhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size,omitempty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type TelegramFile struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
	FileName string `json:"file_name,omitempty"`
	FileSize int    `json:"file_size,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type TelegramSticker struct {
	FileID     string `json:"file_id"`
	IsAnimated bool   `json:"is_animated"`
	IsVideo    bool   `json:"is_video"`
	Emoji      string `json:"emoji,omitempty"`
	SetName    string `json:"set_name,omitempty"`
}

type TelegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	IsBot     bool   `json:"is_bot"`
}

type TelegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // "private" | "group" | "supergroup" | "channel"
	Title string `json:"title,omitempty"`
}

type TelegramEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data"`
}

// ── Media group buffer ────────────────────────────────────────────────────

type mediaGroupEntry struct {
	msgs   []*TelegramMessage
	timer  *time.Timer
	cancel func()
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

	// media group buffering
	mediaGroups   map[string]*mediaGroupEntry
	mediaGroupsMu sync.Mutex
}

// NewTelegramBot creates a Telegram bot that supports streaming and group chats.
func NewTelegramBot(token, agentID string, allowFrom []int64, runner RunnerFunc, pending *PendingStore) *TelegramBot {
	// Wrap the sync runner in a StreamFunc for backward compat when no stream func is set
	sf := func(ctx context.Context, agentID, message string, media []MediaInput) (<-chan StreamEvent, error) {
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
		mediaGroups:  make(map[string]*mediaGroupEntry),
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
		mediaGroups:  make(map[string]*mediaGroupEntry),
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
	url := fmt.Sprintf(
		`https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30&allowed_updates=["message","callback_query","channel_post"]`,
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
	// Handle callback queries
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}

	// Handle channel posts (no access control)
	if update.ChannelPost != nil {
		msg := update.ChannelPost
		rawText := msg.Text
		if rawText == "" {
			rawText = msg.Caption
		}
		text := b.enrichWithContext(msg, rawText)
		hasPostMedia := len(msg.Photo) > 0 || msg.Video != nil || msg.Document != nil
		if text == "" && !hasPostMedia {
			return
		}
		log.Printf("[telegram] Channel post: chat=%d text=%q", msg.Chat.ID, truncate(text, 60))
		go b.generateAndSendWithMedia(ctx, msg, text, 0)
		return
	}

	msg := update.Message
	if msg == nil {
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

	// ── Media group buffering ─────────────────────────────────────────────
	if msg.MediaGroupID != "" && len(msg.Photo) > 0 {
		b.bufferMediaGroup(ctx, msg)
		return
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

	// Allowed user: clean pending entry and send 👀 reaction
	if b.pendingStore != nil {
		b.pendingStore.Remove(senderID)
	}
	_ = b.sendReaction(msg.Chat.ID, msg.MessageID, "👀")

	// Determine text from message or caption
	rawText := msg.Text
	if rawText == "" {
		rawText = msg.Caption
	}

	// Strip bot @mention from message text
	text := b.cleanMessageText(rawText)
	if text == "" && isStart {
		text = "你好"
	}

	// Enrich with forward context (prepend as metadata)
	text = b.enrichWithContext(msg, text)

	// For pure media messages with no text, we still need to process them
	hasMedia := len(msg.Photo) > 0 || msg.Video != nil || msg.Audio != nil ||
		msg.Voice != nil || msg.VideoNote != nil || msg.Document != nil ||
		msg.Sticker != nil || msg.Animation != nil
	if text == "" && !hasMedia {
		return
	}

	replyToMsgID := int64(0)
	if isGroup {
		replyToMsgID = msg.MessageID // reply in thread
	}

	log.Printf("[telegram] Processing: chat=%d user=%s text=%q", msg.Chat.ID, msg.From.Username, truncate(text, 60))

	go b.generateAndSendWithMedia(ctx, msg, text, replyToMsgID)
}

// handleCallbackQuery answers and processes an inline button callback.
func (b *TelegramBot) handleCallbackQuery(ctx context.Context, cq *TelegramCallbackQuery) {
	// Answer immediately to remove loading spinner
	_, _ = b.apiPost("answerCallbackQuery", map[string]any{
		"callback_query_id": cq.ID,
	})

	if cq.Data == "" {
		return
	}

	// Process Data as a user message
	senderID := cq.From.ID
	var chatID int64
	var replyToMsgID int64
	if cq.Message != nil {
		chatID = cq.Message.Chat.ID
		replyToMsgID = cq.Message.MessageID
	}
	if chatID == 0 {
		return
	}

	// Access control
	if len(b.allowFrom) > 0 {
		allowed := false
		for _, id := range b.allowFrom {
			if id == senderID {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	log.Printf("[telegram] Callback query from user=%d data=%q", senderID, truncate(cq.Data, 60))

	// Create a synthetic message for generateAndSendWithMedia
	synth := &TelegramMessage{
		Chat:    TelegramChat{ID: chatID},
		Text:    cq.Data,
		From:    cq.From,
	}
	if cq.Message != nil {
		synth.MessageThreadID = cq.Message.MessageThreadID
	}
	go b.generateAndSendWithMedia(ctx, synth, cq.Data, replyToMsgID)
}

// bufferMediaGroup collects messages that belong to the same media group.
// When no new message arrives for 500ms, it dispatches all collected photos together.
func (b *TelegramBot) bufferMediaGroup(ctx context.Context, msg *TelegramMessage) {
	b.mediaGroupsMu.Lock()
	defer b.mediaGroupsMu.Unlock()

	groupID := msg.MediaGroupID
	entry, exists := b.mediaGroups[groupID]
	if exists {
		entry.msgs = append(entry.msgs, msg)
		// Reset the timer
		entry.timer.Reset(500 * time.Millisecond)
	} else {
		// Create new group entry
		groupCtx := ctx
		groupMsgs := []*TelegramMessage{msg}
		var t *time.Timer
		t = time.AfterFunc(500*time.Millisecond, func() {
			b.mediaGroupsMu.Lock()
			e, ok := b.mediaGroups[groupID]
			if !ok {
				b.mediaGroupsMu.Unlock()
				return
			}
			collected := e.msgs
			delete(b.mediaGroups, groupID)
			b.mediaGroupsMu.Unlock()

			if len(collected) == 0 {
				return
			}

			// Find caption from any message that has one
			caption := ""
			for _, m := range collected {
				if m.Caption != "" {
					caption = m.Caption
					break
				}
			}

			// Access control check using the first sender
			first := collected[0]
			senderID := first.From.ID
			isAllowed := len(b.allowFrom) == 0
			for _, id := range b.allowFrom {
				if id == senderID {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				return
			}

			// Download all images
			var allMedia []MediaInput
			for _, m := range collected {
				if len(m.Photo) == 0 {
					continue
				}
				// Pick highest resolution
				best := m.Photo[len(m.Photo)-1]
				data, ct, err := b.downloadFileByID(groupCtx, best.FileID)
				if err != nil {
					log.Printf("[telegram] mediaGroup download error: %v", err)
					continue
				}
				allMedia = append(allMedia, MediaInput{Data: data, ContentType: ct, FileName: "photo.jpg"})
			}

			replyToMsgID := int64(0)
			if first.Chat.Type == "group" || first.Chat.Type == "supergroup" {
				replyToMsgID = first.MessageID
			}

			_ = b.sendReaction(first.Chat.ID, first.MessageID, "👀")
			log.Printf("[telegram] MediaGroup dispatch: group=%s images=%d", groupID, len(allMedia))
			go b.generateAndSend(groupCtx, first, caption, replyToMsgID, allMedia)
		})

		b.mediaGroups[groupID] = &mediaGroupEntry{
			msgs:  groupMsgs,
			timer: t,
		}
	}
}

// generateAndSendWithMedia resolves media from a message and runs generateAndSend.
func (b *TelegramBot) generateAndSendWithMedia(ctx context.Context, msg *TelegramMessage, text string, replyToMsgID int64) {
	media, extraText, err := b.resolveMedia(ctx, msg)
	if err != nil {
		log.Printf("[telegram] resolveMedia error: %v", err)
	}
	fullText := text
	if extraText != "" {
		if fullText != "" {
			fullText = fullText + "\n" + extraText
		} else {
			fullText = extraText
		}
	}
	b.generateAndSend(ctx, msg, fullText, replyToMsgID, media)
}

// generateAndSend runs the agent and streams the response via send+edit draft pattern.
func (b *TelegramBot) generateAndSend(ctx context.Context, msg *TelegramMessage, message string, replyToMsgID int64, media []MediaInput) {
	chatID := msg.Chat.ID
	threadID := msg.MessageThreadID

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Start typing indicator (kept alive every 4s)
	typingCtx, stopTyping := context.WithCancel(runCtx)
	defer stopTyping()
	go b.keepTyping(typingCtx, chatID, threadID)

	events, err := b.streamFunc(runCtx, b.agentID, message, media)
	if err != nil {
		stopTyping()
		_, _ = b.sendPlain(chatID, "⚠️ 出错了："+err.Error(), replyToMsgID, threadID)
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
			// First send — try HTML first
			id, err := b.sendHTML2(chatID, markdownToHTML(text), replyToMsgID, threadID)
			if err != nil {
				// Fallback to plain
				id, err = b.sendPlain(chatID, text, replyToMsgID, threadID)
				if err != nil {
					log.Printf("[telegram] sendMessage error: %v", err)
					return
				}
			}
			sentMsgID = id
			return
		}
		// Edit existing message — try HTML first
		if err := b.editMessageHTML(chatID, sentMsgID, markdownToHTML(text), threadID); err != nil {
			// Fallback to plain edit
			if err2 := b.editMessage(chatID, sentMsgID, text, threadID); err2 != nil {
				log.Printf("[telegram] editMessage warning: %v", err2)
			}
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
		_, _ = b.sendPlain(chatID, "(no response)", replyToMsgID, threadID)
	}
}

// keepTyping sends "typing" chat action every 4 seconds until ctx is cancelled.
func (b *TelegramBot) keepTyping(ctx context.Context, chatID int64, threadID int64) {
	_ = b.sendChatAction(chatID, "typing", threadID)
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = b.sendChatAction(chatID, "typing", threadID)
		}
	}
}

// isAddressedToBot returns true if the group message targets this bot.
func (b *TelegramBot) isAddressedToBot(msg *TelegramMessage) bool {
	// Check @mention in entities
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	if b.botUsername != "" {
		mention := "@" + b.botUsername
		for _, entity := range msg.Entities {
			if entity.Type == "mention" {
				runes := []rune(text)
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

// enrichWithContext adds forward/reply context to the user's message text,
// mirroring OpenClaw's forwardPrefix + replySuffix pattern.
func (b *TelegramBot) enrichWithContext(msg *TelegramMessage, text string) string {
	var parts []string

	// ── Forward context ───────────────────────────────────────────────────
	forwardSender := b.resolveForwardSender(msg)
	if forwardSender != "" {
		fwdBody := text
		if fwdBody == "" {
			fwdBody = "(无文字内容)"
		}
		parts = append(parts, fmt.Sprintf("[转发自: %s]\n%s", forwardSender, fwdBody))
	} else if text != "" {
		parts = append(parts, text)
	}

	// ── Reply context ─────────────────────────────────────────────────────
	if msg.ReplyToMessage != nil {
		replyMsg := msg.ReplyToMessage
		replySender := replyMsg.From.FirstName
		if replyMsg.From.Username != "" {
			replySender += " (@" + replyMsg.From.Username + ")"
		}
		replyBody := replyMsg.Text
		if replyBody == "" {
			replyBody = replyMsg.Caption
		}
		if replyBody == "" {
			replyBody = "(非文字消息)"
		}
		parts = append(parts, fmt.Sprintf("\n[回复 %s (id:%d)]\n%s\n[/回复]",
			replySender, replyMsg.MessageID, replyBody))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// resolveForwardSender extracts the forward sender name from a message.
// Returns empty string if the message is not a forward.
func (b *TelegramBot) resolveForwardSender(msg *TelegramMessage) string {
	// New-style forward_origin (Bot API 7.0+)
	if msg.ForwardOrigin != nil {
		switch msg.ForwardOrigin.Type {
		case "user":
			if msg.ForwardOrigin.SenderUser != nil {
				name := strings.TrimSpace(msg.ForwardOrigin.SenderUser.FirstName)
				if msg.ForwardOrigin.SenderUser.Username != "" {
					name += " (@" + msg.ForwardOrigin.SenderUser.Username + ")"
				}
				return name
			}
		case "hidden_user":
			if msg.ForwardOrigin.SenderUserName != "" {
				return msg.ForwardOrigin.SenderUserName
			}
			return "匿名用户"
		case "chat", "channel":
			if msg.ForwardOrigin.Chat != nil {
				name := msg.ForwardOrigin.Chat.Title
				if msg.ForwardOrigin.Chat.Username != "" {
					name += " (@" + msg.ForwardOrigin.Chat.Username + ")"
				}
				return name
			}
		}
	}
	// Old-style forward_from
	if msg.ForwardFrom != nil {
		name := strings.TrimSpace(msg.ForwardFrom.FirstName)
		if msg.ForwardFrom.Username != "" {
			name += " (@" + msg.ForwardFrom.Username + ")"
		}
		return name
	}
	if msg.ForwardFromChat != nil {
		name := msg.ForwardFromChat.Title
		if msg.ForwardFromChat.Username != "" {
			name += " (@" + msg.ForwardFromChat.Username + ")"
		}
		return name
	}
	return ""
}

// ── Media resolution ──────────────────────────────────────────────────────

// resolveMedia downloads relevant media from a message, returning:
//   - media: list of MediaInput (images/PDFs to pass to LLM)
//   - extraText: placeholder text for non-downloadable media
func (b *TelegramBot) resolveMedia(ctx context.Context, msg *TelegramMessage) ([]MediaInput, string, error) {
	var media []MediaInput
	var extras []string

	// Photo: download highest resolution
	if len(msg.Photo) > 0 {
		best := msg.Photo[len(msg.Photo)-1]
		data, ct, err := b.downloadFileByID(ctx, best.FileID)
		if err != nil {
			log.Printf("[telegram] photo download error: %v", err)
			extras = append(extras, "[📷 图片]")
		} else {
			media = append(media, MediaInput{Data: data, ContentType: ct, FileName: "photo.jpg"})
		}
	}

	// Video
	if msg.Video != nil {
		extras = append(extras, "[📹 视频消息]")
	}

	// Audio
	if msg.Audio != nil {
		extras = append(extras, "[🎵 音频消息]")
	}

	// Voice
	if msg.Voice != nil {
		extras = append(extras, "[🎤 语音消息]")
	}

	// VideoNote
	if msg.VideoNote != nil {
		extras = append(extras, "[🎥 视频笔记]")
	}

	// Document
	if msg.Document != nil {
		doc := msg.Document
		if doc.MimeType == "application/pdf" {
			data, ct, err := b.downloadFileByID(ctx, doc.FileID)
			if err != nil {
				log.Printf("[telegram] pdf download error: %v", err)
				extras = append(extras, "[📎 文件: "+doc.FileName+"]")
			} else {
				media = append(media, MediaInput{Data: data, ContentType: ct, FileName: doc.FileName})
			}
		} else {
			name := doc.FileName
			if name == "" {
				name = "文件"
			}
			extras = append(extras, "[📎 文件: "+name+"]")
		}
	}

	// Sticker
	if msg.Sticker != nil {
		s := msg.Sticker
		if s.IsAnimated || s.IsVideo {
			emoji := s.Emoji
			if emoji == "" {
				emoji = "贴纸"
			}
			extras = append(extras, "[贴纸: "+emoji+"]")
		} else {
			// Static WEBP sticker — download as image
			data, ct, err := b.downloadFileByID(ctx, s.FileID)
			if err != nil {
				log.Printf("[telegram] sticker download error: %v", err)
				emoji := s.Emoji
				if emoji == "" {
					emoji = "贴纸"
				}
				extras = append(extras, "[贴纸: "+emoji+"]")
			} else {
				media = append(media, MediaInput{Data: data, ContentType: ct, FileName: "sticker.webp"})
			}
		}
	}

	// Animation / GIF
	if msg.Animation != nil {
		extras = append(extras, "[🎞 动图]")
	}

	extraText := strings.Join(extras, " ")
	return media, extraText, nil
}

// downloadFileByID uses getFile to get the file path, then downloads it.
func (b *TelegramBot) downloadFileByID(ctx context.Context, fileID string) ([]byte, string, error) {
	filePath, err := b.getFilePath(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	return b.downloadTelegramFile(ctx, filePath)
}

// getFilePath calls Telegram getFile to resolve a file_id to a file path.
func (b *TelegramBot) getFilePath(ctx context.Context, fileID string) (string, error) {
	body, err := b.apiPost("getFile", map[string]any{"file_id": fileID})
	if err != nil {
		return "", fmt.Errorf("getFile request: %w", err)
	}
	var result struct {
		OK     bool   `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
			FileSize int    `json:"file_size"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("getFile parse: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("getFile: %s", result.Description)
	}
	// 20MB limit
	const maxSize = 20 * 1024 * 1024
	if result.Result.FileSize > maxSize {
		return "", fmt.Errorf("file too large: %d bytes (max 20MB)", result.Result.FileSize)
	}
	return result.Result.FilePath, nil
}

// downloadTelegramFile fetches a file from Telegram servers.
// Returns: raw bytes, content-type, error.
func (b *TelegramBot) downloadTelegramFile(ctx context.Context, filePath string) ([]byte, string, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, filePath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	const maxDownload = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		// Guess from extension
		lower := strings.ToLower(filePath)
		switch {
		case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg"):
			ct = "image/jpeg"
		case strings.HasSuffix(lower, ".png"):
			ct = "image/png"
		case strings.HasSuffix(lower, ".webp"):
			ct = "image/webp"
		case strings.HasSuffix(lower, ".pdf"):
			ct = "application/pdf"
		case strings.HasSuffix(lower, ".gif"):
			ct = "image/gif"
		default:
			ct = "application/octet-stream"
		}
	}

	return data, ct, nil
}

// ── Markdown → Telegram HTML ──────────────────────────────────────────────

var (
	reCodeBlock = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9]*)?\n(.*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic    = regexp.MustCompile(`\*([^*]+?)\*`)
	reUnder     = regexp.MustCompile(`__(.+?)__`)
	reStrike    = regexp.MustCompile(`~~(.+?)~~`)
)

// markdownToHTML converts markdown-style text to Telegram-compatible HTML.
func markdownToHTML(text string) string {
	// 1. Escape HTML special characters first
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// 2. Code blocks (must come before inline code)
	text = reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		inner := reCodeBlock.FindStringSubmatch(m)
		if len(inner) < 2 {
			return m
		}
		return "<pre><code>" + inner[1] + "</code></pre>"
	})

	// 3. Inline code
	text = reInlineCode.ReplaceAllString(text, "<code>$1</code>")

	// 4. Bold
	text = reBold.ReplaceAllString(text, "<b>$1</b>")

	// 5. Italic (after bold so ** is already consumed)
	text = reItalic.ReplaceAllString(text, "<i>$1</i>")

	// 6. Underline
	text = reUnder.ReplaceAllString(text, "<u>$1</u>")

	// 7. Strikethrough
	text = reStrike.ReplaceAllString(text, "<s>$1</s>")

	return text
}

// ── Reactions ─────────────────────────────────────────────────────────────

// sendReaction sends an emoji reaction to a message using setMessageReaction.
func (b *TelegramBot) sendReaction(chatID, messageID int64, emoji string) error {
	_, err := b.apiPost("setMessageReaction", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction": []map[string]string{
			{"type": "emoji", "emoji": emoji},
		},
		"is_big": false,
	})
	return err
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

func (b *TelegramBot) sendChatAction(chatID int64, action string, threadID int64) error {
	payload := map[string]any{
		"chat_id": chatID,
		"action":  action,
	}
	if threadID > 0 {
		payload["message_thread_id"] = threadID
	}
	_, err := b.apiPost("sendChatAction", payload)
	return err
}

// sendPlain sends a plain text message and returns the message ID.
func (b *TelegramBot) sendPlain(chatID int64, text string, replyToMsgID int64, threadID int64) (int64, error) {
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
	if threadID > 0 {
		payload["message_thread_id"] = threadID
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

// sendHTML sends a message with HTML parse mode and returns the message ID.
func (b *TelegramBot) sendHTML2(chatID int64, html string, replyToMsgID int64, threadID int64) (int64, error) {
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
	if threadID > 0 {
		payload["message_thread_id"] = threadID
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
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("sendMessage parse: %w", err)
	}
	if !result.OK {
		return 0, fmt.Errorf("sendMessage HTML: %s", result.Description)
	}
	return result.Result.MessageID, nil
}

// sendHTML sends a message with HTML parse mode (old signature, kept for pairing messages).
func (b *TelegramBot) sendHTML(chatID int64, html string, replyToMsgID int64, threadID int64) error {
	_, err := b.sendHTML2(chatID, html, replyToMsgID, threadID)
	return err
}

// editMessageHTML edits an existing message with HTML parse mode.
func (b *TelegramBot) editMessageHTML(chatID, messageID int64, text string, threadID int64) error {
	if len(text) > 4096 {
		text = text[:4096]
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if threadID > 0 {
		payload["message_thread_id"] = threadID
	}
	body, err := b.apiPost("editMessageText", payload)
	if err != nil {
		return err
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}
	if !result.OK {
		if strings.Contains(result.Description, "not modified") {
			return nil
		}
		return fmt.Errorf("editMessageText HTML: %s", result.Description)
	}
	return nil
}

// editMessage edits an existing message (plain text fallback).
func (b *TelegramBot) editMessage(chatID, messageID int64, text string, threadID int64) error {
	if len(text) > 4096 {
		text = text[:4096]
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if threadID > 0 {
		payload["message_thread_id"] = threadID
	}
	body, err := b.apiPost("editMessageText", payload)
	if err != nil {
		return err
	}
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
	_, err := b.sendPlain(chatID, text, 0, 0)
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

// mediaToDataURIs converts a []MediaInput to base64 data URI strings for the LLM.
func mediaToDataURIs(media []MediaInput) []string {
	if len(media) == 0 {
		return nil
	}
	uris := make([]string, 0, len(media))
	for _, m := range media {
		if len(m.Data) == 0 {
			continue
		}
		ct := m.ContentType
		if ct == "" {
			ct = "image/jpeg"
		}
		encoded := base64.StdEncoding.EncodeToString(m.Data)
		uris = append(uris, "data:"+ct+";base64,"+encoded)
	}
	return uris
}

// Ensure RunnerFunc is exported so other packages can reference it cleanly.
