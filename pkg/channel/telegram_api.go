package channel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

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

// SendFileToChat sends a local file to a Telegram chat using multipart form upload.
// It auto-selects the appropriate Telegram method based on file extension:
//   image/*  → sendPhoto (Telegram displays inline)
//   video/*  → sendVideo
//   audio/*  → sendAudio
//   other    → sendDocument (generic file delivery)
//
// Telegram hard-limits are 10 MB for photos and 50 MB for other types.
// Files exceeding the limit return an error so the caller can fall back to a download link.
func (b *TelegramBot) SendFileToChat(chatID, threadID int64, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	baseName := filepath.Base(filePath)

	// Choose Telegram method + form field name based on extension
	type sendConfig struct{ method, field string }
	var sc sendConfig
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		sc = sendConfig{"sendPhoto", "photo"}
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		sc = sendConfig{"sendVideo", "video"}
	case ".mp3", ".m4a", ".ogg", ".flac", ".wav", ".aac":
		sc = sendConfig{"sendAudio", "audio"}
	default:
		sc = sendConfig{"sendDocument", "document"}
	}

	// Build multipart/form-data body
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	_ = mw.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if threadID > 0 {
		_ = mw.WriteField("message_thread_id", fmt.Sprintf("%d", threadID))
	}

	fw, err := mw.CreateFormFile(sc.field, baseName)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err = fw.Write(data); err != nil {
		return "", fmt.Errorf("write file data: %w", err)
	}
	mw.Close()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, sc.method)
	resp, err := b.client.Post(url, mw.FormDataContentType(), &body)
	if err != nil {
		return "", fmt.Errorf("telegram upload: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("telegram %s failed: %s", sc.method, result.Description)
	}
	return fmt.Sprintf("✅ 已发送 %s (%.1f KB)", baseName, float64(len(data))/1024), nil
}

// Ensure RunnerFunc is exported so other packages can reference it cleanly.
