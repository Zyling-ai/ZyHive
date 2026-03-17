// Package channel — Telegram Bot integration with streaming draft and group support.
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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/chatlog"
	"github.com/Zyling-ai/zyhive/pkg/convlog"
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

// FileSenderFunc sends a local file to the user (via the active channel).
// Returns a human-readable confirmation or an error.
type FileSenderFunc func(filePath string) (string, error)

// StreamFunc executes an agent turn and returns a live StreamEvent channel.
// sessionID is used for persistent per-chat history (e.g. "telegram-{chatID}").
// Pass "" to use an auto-generated ephemeral session.
// fileSender is optional (may be nil); if provided, the runner's send_file tool uses it.
type StreamFunc func(ctx context.Context, agentID, message, sessionID string, media []MediaInput, fileSender FileSenderFunc) (<-chan StreamEvent, error)

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
	agentDir     string // root dir for this agent (agents/{id}), used for conv logging
	channelID    string // channel config ID, used for approved user store
	getAllowFrom func() []int64 // dynamic allowFrom getter — hot-reloads on every message
	streamFunc   StreamFunc
	client       *http.Client
	offset       int64
	pendingStore *PendingStore
	// resolved on Start via getMe
	botID       int64
	botUsername string
	mu          sync.Mutex

	// Callback fired once after a successful getMe — used to update channel status in manager.
	onConnected func(botUsername string)

	// media group buffering
	mediaGroups   map[string]*mediaGroupEntry
	mediaGroupsMu sync.Mutex
}

// NewTelegramBot creates a Telegram bot that supports streaming and group chats.
func NewTelegramBot(token, agentID, agentDir string, allowFrom []int64, runner RunnerFunc, pending *PendingStore) *TelegramBot {
	// Wrap the sync runner in a StreamFunc for backward compat when no stream func is set
	sf := func(ctx context.Context, agentID, message, _ string, media []MediaInput, _ FileSenderFunc) (<-chan StreamEvent, error) {
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
	fixedList := allowFrom
	return &TelegramBot{
		token:        token,
		agentID:      agentID,
		agentDir:     agentDir,
		getAllowFrom:  func() []int64 { return fixedList },
		streamFunc:   sf,
		client:       &http.Client{Timeout: 90 * time.Second},
		pendingStore: pending,
		mediaGroups:  make(map[string]*mediaGroupEntry),
	}
}

// SetOnConnected sets a callback that fires once after a successful getMe.
// Used by callers (e.g. main.go) to update channel status in the manager.
func (b *TelegramBot) SetOnConnected(fn func(botUsername string)) {
	b.onConnected = fn
}

// NewTelegramBotWithStream creates a bot that uses a real StreamFunc.
// getAllowFrom is called on every message so the allowlist can be updated dynamically
// (e.g. after admin approves a pending user) without restarting the bot.
func NewTelegramBotWithStream(token, agentID, agentDir, channelID string, getAllowFrom func() []int64, sf StreamFunc, pending *PendingStore) *TelegramBot {
	return &TelegramBot{
		token:        token,
		agentID:      agentID,
		agentDir:     agentDir,
		channelID:    channelID,
		getAllowFrom:  getAllowFrom,
		streamFunc:   sf,
		client:       &http.Client{Timeout: 90 * time.Second},
		pendingStore: pending,
		mediaGroups:  make(map[string]*mediaGroupEntry),
	}
}

// SendApprovalWelcome sends a welcome message with inline buttons when a pending user is approved.
// Called by the AllowPending API endpoint after adding the user to the allowlist.
func SendApprovalWelcome(token string, chatID int64, agentName string) error {
	if agentName == "" {
		agentName = "AI 助手"
	}
	text := fmt.Sprintf(
		"✅ <b>你已通过审核，欢迎使用 %s！</b>\n\n"+
			"我是你的专属 AI 助手，可以帮你：\n"+
			"• 💬 回答问题、提供建议\n"+
			"• 📝 撰写内容、整理思路\n"+
			"• 🔍 搜索信息、分析数据\n"+
			"• 🌐 翻译文本、多语言沟通\n"+
			"• 🖼 识别图片、解读文件\n\n"+
			"直接发消息就能开始对话，试试说「你好」👋",
		agentName,
	)
	keyboard := map[string]any{
		"inline_keyboard": [][]map[string]any{
			{
				{"text": "💬 开始对话", "callback_data": "你好，介绍一下你自己吧！"},
				{"text": "🛠 我能做什么", "callback_data": "你都能帮我做什么？列举一下。"},
			},
			{
				{"text": "❓ 使用帮助", "callback_data": "怎么使用你？有什么技巧？"},
			},
		},
	}
	payload := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"parse_mode":   "HTML",
		"reply_markup": keyboard,
	}
	data, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// getConvLog returns a ConvLog for a given chatID, or nil if agentDir is unset.
func (b *TelegramBot) getConvLog(chatID int64) *convlog.ConvLog {
	if b.agentDir == "" {
		return nil
	}
	return convlog.New(b.agentDir, fmt.Sprintf("telegram-%d", chatID))
}

// Start runs the long-poll loop. Fetches bot info first, then polls.
func (b *TelegramBot) Start(ctx context.Context) {
	// Fetch bot identity
	if err := b.fetchBotInfo(ctx); err != nil {
		log.Printf("[telegram] getMe failed: %v", err)
	} else {
		log.Printf("[telegram] Bot started: @%s (id=%d, agent=%s)", b.botUsername, b.botID, b.agentID)
		// Notify caller (e.g. to update channel status → "ok" in the manager)
		if b.onConnected != nil {
			b.onConnected(b.botUsername)
		}
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
	// Debug: log raw update JSON for messages with ReplyToMessage or Photo
	for _, u := range result.Result {
		msg := u.Message
		if msg == nil {
			continue
		}
		if msg.ReplyToMessage != nil || len(msg.Photo) > 0 {
			raw, _ := json.Marshal(u)
			log.Printf("[telegram][debug] update json: %s", string(raw))
		}
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
	// Call getAllowFrom() once per message so admin approvals take effect immediately.
	currentAllowFrom := b.getAllowFrom()
	if len(currentAllowFrom) == 0 {
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
	for _, id := range currentAllowFrom {
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

	// Allowed user: clean pending entry, cache username, send 👀 reaction
	if b.pendingStore != nil {
		b.pendingStore.Remove(senderID)
	}
	// Cache username info in approved store so Web UI can display it
	if b.agentDir != "" {
		pendingDir := filepath.Join(b.agentDir, "channels-pending")
		as := NewApprovedStore(pendingDir, b.channelID)
		as.Upsert(PendingUser{
			ID:        senderID,
			Username:  msg.From.Username,
			FirstName: msg.From.FirstName,
			LastSeen:  msg.Date,
		})
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

	// Log inbound user message to permanent conversation log (admin-only, agent-blind)
	// Always log, even for media-only messages (use placeholder if text empty)
	{
		logContent := text
		if logContent == "" {
			// media-only message
			if len(msg.Photo) > 0 {
				logContent = "[📷 图片]"
			} else if msg.Voice != nil {
				logContent = "[🎤 语音]"
			} else if msg.Video != nil {
				logContent = "[📹 视频]"
			} else if msg.Document != nil {
				logContent = "[📎 文件]"
			} else if msg.Sticker != nil {
				logContent = "[贴纸 " + msg.Sticker.Emoji + "]"
			} else {
				logContent = "[媒体消息]"
			}
		}
		tsUser := time.Now().UTC().Format(time.RFC3339)
		tgChannelID := fmt.Sprintf("telegram-%d", msg.Chat.ID)
		tgSessionID := fmt.Sprintf("telegram-%d", msg.Chat.ID)
		senderStr := fmt.Sprintf("%s (%d)", msg.From.FirstName, msg.From.ID)
		if cl := b.getConvLog(msg.Chat.ID); cl != nil {
			_ = cl.Append(convlog.Entry{
				Timestamp:   tsUser,
				Role:        "user",
				Content:     logContent,
				ChannelID:   tgChannelID,
				ChannelType: "telegram",
				Sender:      senderStr,
			})
		}
		// Write to AI-visible chatlog
		if b.agentDir != "" {
			clMgr := chatlog.NewManager(b.agentDir + "/workspace")
			_ = clMgr.Append(chatlog.Entry{
				Ts:          tsUser,
				SessionID:   tgSessionID,
				ChannelID:   tgChannelID,
				ChannelType: "telegram",
				Role:        "user",
				Content:     logContent,
				Sender:      senderStr,
			})
		}
	}

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

	// Access control (dynamic — reads live allowlist)
	cbAllowFrom := b.getAllowFrom()
	if len(cbAllowFrom) > 0 {
		allowed := false
		for _, id := range cbAllowFrom {
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

			// Access control check using the first sender (dynamic — reads live allowlist)
			first := collected[0]
			senderID := first.From.ID
			mgAllowFrom := b.getAllowFrom()
			isAllowed := len(mgAllowFrom) == 0
			for _, id := range mgAllowFrom {
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

	// Per-chat session ID: gives the agent persistent memory per Telegram conversation.
	sessionID := fmt.Sprintf("telegram-%d", chatID)

	// File sender: AI can call send_file tool to deliver files to this chat.
	fileSender := FileSenderFunc(func(filePath string) (string, error) {
		return b.SendFileToChat(chatID, threadID, filePath)
	})

	events, err := b.streamFunc(runCtx, b.agentID, message, sessionID, media, fileSender)
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
	finalText := accumulated.String()
	sendOrEdit(finalText, true)

	// Log assistant response to permanent conversation log
	if finalText != "" {
		tsAssist := time.Now().UTC().Format(time.RFC3339)
		tgAssistChannelID := fmt.Sprintf("telegram-%d", chatID)
		tgAssistSessionID := fmt.Sprintf("telegram-%d", chatID)
		if cl := b.getConvLog(chatID); cl != nil {
			_ = cl.Append(convlog.Entry{
				Timestamp:   tsAssist,
				Role:        "assistant",
				Content:     finalText,
				ChannelID:   tgAssistChannelID,
				ChannelType: "telegram",
			})
		}
		// Write to AI-visible chatlog
		if b.agentDir != "" {
			clMgr := chatlog.NewManager(b.agentDir + "/workspace")
			_ = clMgr.Append(chatlog.Entry{
				Ts:          tsAssist,
				SessionID:   tgAssistSessionID,
				ChannelID:   tgAssistChannelID,
				ChannelType: "telegram",
				Role:        "assistant",
				Content:     finalText,
			})
		}
	}

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
// mirroring forwardPrefix + replySuffix pattern.
func (b *TelegramBot) enrichWithContext(msg *TelegramMessage, text string) string {
	var parts []string

	// ── Forward context ───────────────────────────────────────────────────
	forwardSender := b.resolveForwardSender(msg)
	if forwardSender != "" {
		fwdBody := text
		if fwdBody == "" {
			fwdBody = "(无文字内容)"
		}
		// Note: forwarded images are already in msg.Photo[], handled by resolveMedia
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
		// If the replied-to message has an image, note it (the image is downloaded separately)
		if replyBody == "" {
			if len(replyMsg.Photo) > 0 {
				replyBody = "[图片]"
			} else if replyMsg.Sticker != nil {
				replyBody = "[贴纸: " + replyMsg.Sticker.Emoji + "]"
			} else if replyMsg.Video != nil {
				replyBody = "[视频]"
			} else if replyMsg.Voice != nil {
				replyBody = "[语音]"
			} else if replyMsg.Document != nil {
				replyBody = "[文件: " + replyMsg.Document.FileName + "]"
			} else {
				replyBody = "(非文字消息)"
			}
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

