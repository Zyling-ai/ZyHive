// Package channel — Feishu (飞书/Lark) Bot integration via WebSocket Long Connection.
// - App Access Token auto-refresh every 90 minutes
// - WebSocket Long Connection (no public HTTPS needed)
// - Streaming reply: send first then PATCH to update
// - Group chats: respond only when @mentioned
// - Pairing mode when no allowFrom is configured
package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/network"
	"github.com/gorilla/websocket"
)

// ── Feishu API types ──────────────────────────────────────────────────────

type feishuTokenResp struct {
	Code            int    `json:"code"`
	Msg             string `json:"msg"`
	AppAccessToken  string `json:"app_access_token"`
	Expire          int    `json:"expire"`
}

type feishuWsEndpointResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		URL string `json:"url"`
	} `json:"data"`
}

// feishuWsFrame is the envelope for all WebSocket messages.
type feishuWsFrame struct {
	Type    int             `json:"type"`    // 0=ping, 1=data
	Headers map[string]string `json:"headers,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// feishuEvent is the top-level event from Feishu.
type feishuEvent struct {
	Schema string          `json:"schema"`
	Header feishuEventHeader `json:"header"`
	Event  json.RawMessage `json:"event"`
}

type feishuEventHeader struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	AppID     string `json:"app_id"`
}

// feishuMessageEvent is the im.message.receive_v1 event payload.
type feishuMessageEvent struct {
	Sender  feishuSender  `json:"sender"`
	Message feishuMessage `json:"message"`
}

type feishuSender struct {
	SenderID struct {
		OpenID string `json:"open_id"`
		UserID string `json:"user_id"`
	} `json:"sender_id"`
	SenderType string `json:"sender_type"`
}

type feishuMessage struct {
	MessageID   string            `json:"message_id"`
	ChatID      string            `json:"chat_id"`
	ChatType    string            `json:"chat_type"` // "p2p" | "group"
	MessageType string            `json:"message_type"`
	Content     string            `json:"content"` // JSON string
	Mentions    []feishuMention   `json:"mentions"`
}

type feishuMention struct {
	Key string `json:"key"`
	ID  struct {
		OpenID string `json:"open_id"`
	} `json:"id"`
}

// ── FeishuBot ─────────────────────────────────────────────────────────────

type FeishuBot struct {
	appID       string
	appSecret   string
	agentID     string
	agentDir    string
	channelID   string
	domain      string // "open.feishu.cn" or "open.larksuite.com"
	getAllowFrom func() []string // open_id list; empty = pairing mode

	streamFunc   StreamFunc
	pendingStore *PendingStoreStr
	client       *http.Client

	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time

	seenMu     sync.Mutex
	seenEvents map[string]time.Time // event_id dedup

	botOpenID string // bot's own open_id (fetched on start)

	onConnected  func(name string)
	// panelBaseURL is the ZyHive panel URL shown in pairing messages (e.g. "https://hive.example.com")
	panelBaseURL string

	// chatMu serializes processing per chatID to avoid concurrent LLM calls for the same chat
	chatMu sync.Map // chatID → *sync.Mutex
}

// NewFeishuBotWithStream creates a FeishuBot.
func NewFeishuBotWithStream(appID, appSecret, agentID, agentDir, channelID string, getAllowFrom func() []string, sf StreamFunc, pending *PendingStoreStr) *FeishuBot {
	return &FeishuBot{
		appID:        appID,
		appSecret:    appSecret,
		agentID:      agentID,
		agentDir:     agentDir,
		channelID:    channelID,
		domain:       "open.feishu.cn",
		getAllowFrom:  getAllowFrom,
		streamFunc:   sf,
		pendingStore: pending,
		client:       &http.Client{Timeout: 15 * time.Second},
		seenEvents:   make(map[string]time.Time),
	}
}

// SetOnConnected sets a callback fired once the bot connects (gets its open_id).
func (b *FeishuBot) SetOnConnected(fn func(name string)) {
	b.onConnected = fn
}

// SetPanelBaseURL sets the ZyHive panel URL shown in pairing messages.
func (b *FeishuBot) SetPanelBaseURL(url string) {
	b.panelBaseURL = url
}

// Start runs the WebSocket loop, reconnecting on error.
func (b *FeishuBot) Start(ctx context.Context) {
	log.Printf("[feishu] starting agent=%s", b.agentID)
	// Load previously seen event IDs to avoid reprocessing on restart
	b.loadSeenEvents()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := b.runOnce(ctx); err != nil {
			log.Printf("[feishu] connection error: %v — reconnecting in 5s", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (b *FeishuBot) runOnce(ctx context.Context) error {
	token, err := b.refreshToken()
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	// Fetch bot self info (open_id)
	if b.botOpenID == "" {
		if oid, err := b.fetchBotOpenID(token); err == nil {
			b.botOpenID = oid
			log.Printf("[feishu] bot open_id=%s", oid)
			if b.onConnected != nil {
				b.onConnected(oid)
			}
		}
	}

	// Get WebSocket endpoint
	wsURL, err := b.getWsEndpoint(token)
	if err != nil {
		return fmt.Errorf("get ws endpoint: %w", err)
	}

	// Connect
	dialer := websocket.DefaultDialer
	// Feishu WS does not accept Authorization header; token is in the URL
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close()
	log.Printf("[feishu] WebSocket connected agent=%s", b.agentID)

	// Refresh token 90 minutes (token expires in 2h)
	tokenTicker := time.NewTicker(90 * time.Minute)
	defer tokenTicker.Stop()

	// Ping every 30s
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[feishu] ws read error: %v", err)
				return
			}
			b.handleWsMessage(ctx, conn, msg)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return nil
		case <-done:
			return fmt.Errorf("connection closed")
		case <-pingTicker.C:
			frame := feishuWsFrame{Type: 0} // ping
			data, _ := json.Marshal(frame)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return fmt.Errorf("ping write: %w", err)
			}
		case <-tokenTicker.C:
			if t, err := b.refreshToken(); err == nil {
				token = t
			}
		}
	}
}

func (b *FeishuBot) handleWsMessage(ctx context.Context, conn *websocket.Conn, raw []byte) {
	// Feishu WS uses protobuf-encoded frames (pbbp2.Frame), not JSON.
	frame, err := parseFeishuFrame(raw)
	if err != nil {
		// Fallback: try legacy JSON format (should not happen with msg-frontier)
		var jframe feishuWsFrame
		if jerr := json.Unmarshal(raw, &jframe); jerr == nil && jframe.Type == 0 {
			conn.WriteMessage(websocket.BinaryMessage, raw)
		}
		return
	}

	switch frame.Method {
	case feishuFrameMethodPing:
		// Respond with pong
		pong := encodeFeishuPong(frame.SeqID)
		conn.WriteMessage(websocket.BinaryMessage, pong)
		return

	case feishuFrameMethodPong:
		// Nothing to do
		return

	case feishuFrameMethodControl:
		// Handshake / config update — log but ignore for now
		log.Printf("[feishu] control frame: payload_type=%s", frame.PayloadType)
		return

	case feishuFrameMethodData:
		// Event data — decode JSON payload
		if len(frame.Payload) == 0 {
			// Send ACK even for empty payload
			conn.WriteMessage(websocket.BinaryMessage, encodeFeishuAck(frame, `{"code":200}`))
			return
		}
		var ev feishuEvent
		if err := json.Unmarshal(frame.Payload, &ev); err != nil {
			log.Printf("[feishu] event json error: %v payload=%s", err, string(frame.Payload[:min(200, len(frame.Payload))]))
			// Send ACK even on parse error so Feishu stops retrying
			conn.WriteMessage(websocket.BinaryMessage, encodeFeishuAck(frame, `{"code":500}`))
			return
		}

		// CRITICAL: Send ACK immediately so Feishu marks event as delivered and stops retrying.
		// Without this, Feishu re-pushes all unacknowledged events on every reconnect.
		conn.WriteMessage(websocket.BinaryMessage, encodeFeishuAck(frame, `{"code":200}`))

		// Dedup by event_id (defense in depth — ACK is the primary dedup mechanism)
		if ev.Header.EventID != "" {
			b.seenMu.Lock()
			if _, seen := b.seenEvents[ev.Header.EventID]; seen {
				b.seenMu.Unlock()
				return
			}
			b.seenEvents[ev.Header.EventID] = time.Now()
			if len(b.seenEvents) > 1000 {
				cutoff := time.Now().Add(-2 * time.Hour)
				for k, t := range b.seenEvents {
					if t.Before(cutoff) {
						delete(b.seenEvents, k)
					}
				}
			}
			b.seenMu.Unlock()
			go b.persistSeenEvents()
		}
		if ev.Header.EventType == "im.message.receive_v1" {
			var msgEvent feishuMessageEvent
			if err := json.Unmarshal(ev.Event, &msgEvent); err == nil {
				go b.handleMessageEvent(ctx, &msgEvent)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (b *FeishuBot) handleMessageEvent(ctx context.Context, ev *feishuMessageEvent) {
	msg := &ev.Message
	senderOpenID := ev.Sender.SenderID.OpenID

	// Accept text / image / post (rich text with images). Everything else
	// (audio / file / sticker / ...) is still ignored — vision models don't
	// consume those anyway.
	if msg.MessageType != "text" && msg.MessageType != "image" && msg.MessageType != "post" {
		return
	}

	// Serialize per chatID: only one LLM call at a time per chat
	muVal, _ := b.chatMu.LoadOrStore(msg.ChatID, &sync.Mutex{})
	mu := muVal.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Parse content based on message_type:
	//   text  → {"text":"hello"}
	//   image → {"image_key":"img_v3_..."}
	//   post  → {"title":"t","content":[[{tag:"text",text:"..."}, {tag:"img",image_key:"..."}, ...], ...]}
	var text string
	var imageKeys []string
	switch msg.MessageType {
	case "text":
		var c struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(msg.Content), &c); err != nil {
			return
		}
		text = strings.TrimSpace(c.Text)
	case "image":
		var c struct {
			ImageKey string `json:"image_key"`
		}
		if err := json.Unmarshal([]byte(msg.Content), &c); err != nil {
			return
		}
		if c.ImageKey != "" {
			imageKeys = append(imageKeys, c.ImageKey)
		}
		text = "[图片]"
	case "post":
		// Feishu post content is a 2D array of fragments; extract text + image_keys.
		var c struct {
			Title   string                     `json:"title"`
			Content [][]map[string]interface{} `json:"content"`
		}
		if err := json.Unmarshal([]byte(msg.Content), &c); err != nil {
			return
		}
		var textParts []string
		if t := strings.TrimSpace(c.Title); t != "" {
			textParts = append(textParts, t)
		}
		for _, line := range c.Content {
			for _, frag := range line {
				tag, _ := frag["tag"].(string)
				switch tag {
				case "text", "a", "md":
					if s, ok := frag["text"].(string); ok && strings.TrimSpace(s) != "" {
						textParts = append(textParts, s)
					}
				case "img":
					if k, ok := frag["image_key"].(string); ok && k != "" {
						imageKeys = append(imageKeys, k)
					}
				}
			}
		}
		text = strings.TrimSpace(strings.Join(textParts, "\n"))
		if text == "" && len(imageKeys) > 0 {
			text = "[图片]"
		}
	}

	// Group chat handling
	isGroup := msg.ChatType == "group"
	if isGroup {
		// Check if this bot is @mentioned
		mentioned := false
		for _, m := range msg.Mentions {
			if m.ID.OpenID == b.botOpenID {
				mentioned = true
				break
			}
		}

		if mentioned {
			// Strip all @mention tokens from text
			for _, m := range msg.Mentions {
				text = strings.ReplaceAll(text, m.Key, "")
			}
			text = strings.TrimSpace(text)

			// Handle bot commands first (e.g. /listen all, /status, /help)
			// Commands are only triggered when @this bot — ensures multi-bot safety
			if b.handleGroupCommand(msg.ChatID, text) {
				return
			}
			// Not a command — fall through to normal LLM processing
		} else {
			// Not mentioned — check per-chat listenAll config
			chatCfg := b.loadChatConfig(msg.ChatID)
			if !chatCfg.ListenAll {
				return // default: only respond when @mentioned
			}
			// listenAll mode: respond to all messages (no @mention needed)
			// Don't strip mentions since we weren't mentioned
		}
	}

	if text == "" {
		return
	}

	// Access control
	currentAllowFrom := b.getAllowFrom()
	if len(currentAllowFrom) == 0 {
		// Pairing mode — guide user to authorize via panel
		log.Printf("[feishu] pairing mode — user open_id=%s", senderOpenID)
		if b.pendingStore != nil {
			b.pendingStore.Add(senderOpenID, senderOpenID)
		}
		authURL := b.panelBaseURL
		if authURL == "" {
			authURL = "ZyHive 管理面板"
		} else {
			authURL = authURL + "/#/agents/" + b.agentID + "/channels"
		}
		reply := fmt.Sprintf("👋 您好！请前往管理面板授权后即可开始对话：\n\n%s\n\n授权完成后直接发消息即可。", authURL)
		_, _ = b.sendText(msg.ChatID, reply)
		return
	}

	allowed := false
	for _, id := range currentAllowFrom {
		if id == senderOpenID {
			allowed = true
			break
		}
	}
	if !allowed {
		// Unauthorized — add to pending and guide to authorize
		log.Printf("[feishu] unauthorized user open_id=%s", senderOpenID)
		if b.pendingStore != nil {
			b.pendingStore.Add(senderOpenID, senderOpenID)
		}
		authURL := b.panelBaseURL
		if authURL == "" {
			authURL = "ZyHive 管理面板"
		} else {
			authURL = authURL + "/#/agents/" + b.agentID + "/channels"
		}
		reply := fmt.Sprintf("👋 您好！您尚未获得访问授权，请联系管理员在以下地址为您开通：\n\n%s\n\n授权完成后直接发消息即可。", authURL)
		_, _ = b.sendText(msg.ChatID, reply)
		return
	}

	// Authorized user — remove from pending if present
	if b.pendingStore != nil {
		b.pendingStore.Remove(senderOpenID)
	}

	log.Printf("[feishu] message from open_id=%s chat=%s text=%q", senderOpenID, msg.ChatID, truncateStr(text, 60))

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Use chatID as sessionID so conversations persist per Feishu chat
	// Prefix with "feishu-" to namespace from other channel sessions
	feishuSessionID := "feishu-" + msg.ChatID

	// Build the message text with sender attribution for group chats
	// For group chats: prepend sender name so AI knows who is speaking
	// For DMs: just use the original text
	finalText := text
	if isGroup {
		// Try to get sender's display name (cached)
		senderName := b.getSenderName(senderOpenID)
		if senderName != "" {
			finalText = fmt.Sprintf("[%s]: %s", senderName, text)
		} else {
			finalText = fmt.Sprintf("[%s]: %s", senderOpenID[:min(8, len(senderOpenID))], text)
		}
	}

	// Inject sender identity as extra system context (NOT in the user message — invisible to users)
	extraCtx := fmt.Sprintf("当前飞书用户信息：open_id=%s，chat_id=%s，chat_type=%s",
		senderOpenID, msg.ChatID, msg.ChatType)

	// ── Network (contact book) — resolve feishu sender and append Layer-2 summary.
	if b.agentDir != "" && senderOpenID != "" {
		wsDir := filepath.Join(b.agentDir, "workspace")
		store := network.NewStore(wsDir)
		// Bug 2 fix: getSenderName 在刚加好友 / 群聊成员列表未拉取时会返回 "",
		// 直接落盘会导致 UI 显示空白. 走 FallbackDisplayName 链.
		displayName := network.FallbackDisplayName(senderOpenID, b.getSenderName(senderOpenID))
		if _, nerr := store.Resolve(network.SourceFeishu, senderOpenID, displayName); nerr != nil {
			log.Printf("[feishu] network.Resolve warning: %v", nerr)
		} else if summary := store.Summary(network.MakeID(network.SourceFeishu, senderOpenID)); summary != "" {
			extraCtx = extraCtx + "\n\n" + summary
		}
	}

	// Download any attached images and hand them to the model as MediaInput.
	// Cap at 5 images per message to protect token budget / vision limits.
	var media []MediaInput
	if len(imageKeys) > 0 {
		const maxImgs = 5
		if len(imageKeys) > maxImgs {
			log.Printf("[feishu] message %s has %d images, keeping first %d",
				msg.MessageID, len(imageKeys), maxImgs)
			imageKeys = imageKeys[:maxImgs]
		}
		for _, key := range imageKeys {
			data, ct, derr := b.downloadMessageResource(msg.MessageID, key, "image")
			if derr != nil {
				log.Printf("[feishu] download image_key=%s: %v", key, derr)
				continue
			}
			media = append(media, MediaInput{
				FileName:    key + extFromContentType(ct),
				ContentType: ct,
				Data:        data,
			})
		}
		if len(media) > 0 {
			log.Printf("[feishu] attached %d images to agent turn", len(media))
		}
	}

	events, err := b.streamFunc(runCtx, b.agentID, finalText, feishuSessionID, media, nil, extraCtx)
	if err != nil {
		_, _ = b.sendText(msg.ChatID, "⚠️ 出错了："+err.Error())
		return
	}

	// Immediately send a "typing" placeholder card so the user sees a response is coming
	var accumulated strings.Builder
	sentMsgID, _ := b.sendCard(msg.ChatID, "⌛ 正在思考...")
	lastPatched := ""

	throttle := time.NewTicker(1200 * time.Millisecond)
	defer throttle.Stop()

	patchCard := func(text string) {
		if text == "" || text == lastPatched || sentMsgID == "" {
			return
		}
		lastPatched = text
		_ = b.patchCard(sentMsgID, text)
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
			patchCard(accumulated.String())
		}
	}
done:
	// Final update with complete text
	replyText := strings.TrimSpace(accumulated.String())
	if replyText == "" {
		replyText = "(no response)"
	}
	if sentMsgID == "" {
		// Placeholder failed — send directly
		if _, err := b.sendCard(msg.ChatID, replyText); err != nil {
			log.Printf("[feishu] sendCard error: %v", err)
		}
	} else {
		patchCard(replyText)
	}
}

// ProactiveSend sends a message to the first chat we've interacted with (for cron notifications).
// For Feishu, this requires a known chat_id; we skip if unknown.
func (b *FeishuBot) ProactiveSend(text string) error {
	// Feishu proactive send requires a known chat_id — not supported without prior interaction.
	log.Printf("[feishu] ProactiveSend not supported without a target chat_id: %q", truncateStr(text, 40))
	return nil
}

// ── Feishu REST API helpers ───────────────────────────────────────────────

func (b *FeishuBot) apiBase() string {
	return "https://" + b.domain + "/open-apis"
}

func (b *FeishuBot) refreshToken() (string, error) {
	b.tokenMu.Lock()
	defer b.tokenMu.Unlock()

	if time.Now().Before(b.tokenExpiry) && b.accessToken != "" {
		return b.accessToken, nil
	}

	payload := map[string]string{
		"app_id":     b.appID,
		"app_secret": b.appSecret,
	}
	data, _ := json.Marshal(payload)
	resp, err := b.client.Post(
		b.apiBase()+"/auth/v3/app_access_token/internal",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result feishuTokenResp
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse token resp: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu token error %d: %s", result.Code, result.Msg)
	}

	b.accessToken = result.AppAccessToken
	expire := result.Expire
	if expire <= 0 {
		expire = 7200
	}
	b.tokenExpiry = time.Now().Add(time.Duration(expire-300) * time.Second)
	log.Printf("[feishu] app_access_token refreshed, expires in %ds", expire)
	return b.accessToken, nil
}

func (b *FeishuBot) getWsEndpoint(_ string) (string, error) {
	// Feishu WS long connection: POST /callback/ws/endpoint with AppID + AppSecret
	// This returns a wss://msg-frontier.feishu.cn/ws/v2?... URL.
	// The SDK does NOT use app_access_token for this call.
	reqBody, _ := json.Marshal(map[string]string{
		"AppID":     b.appID,
		"AppSecret": b.appSecret,
	})
	req, _ := http.NewRequest("POST", "https://"+b.domain+"/callback/ws/endpoint", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("locale", "zh")
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result feishuWsEndpointResp
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse ws endpoint: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu ws endpoint error %d: %s", result.Code, result.Msg)
	}
	return result.Data.URL, nil
}

func (b *FeishuBot) fetchBotOpenID(token string) (string, error) {
	req, _ := http.NewRequest("GET", b.apiBase()+"/bot/v3/info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("bot info error %d: %s", result.Code, result.Msg)
	}
	return result.Bot.OpenID, nil
}

// downloadMessageResource fetches an image/file attached to a Feishu message
// via GET /im/v1/messages/:message_id/resources/:file_key?type=image.
//
// Returns (raw bytes, content-type, error).
// Caller is responsible for bounding the total set of images sent to the LLM
// (vision providers typically cap around 5 images / 20MB).
func (b *FeishuBot) downloadMessageResource(messageID, fileKey, resourceType string) ([]byte, string, error) {
	token, err := b.refreshToken()
	if err != nil {
		return nil, "", err
	}
	if resourceType == "" {
		resourceType = "image"
	}
	url := fmt.Sprintf("%s/im/v1/messages/%s/resources/%s?type=%s",
		b.apiBase(), messageID, fileKey, resourceType)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch feishu resource: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// Feishu returns JSON error on 4xx/5xx — read a small window for diagnostics.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("feishu resource HTTP %d: %s", resp.StatusCode, string(body))
	}
	// Cap at 10MB to protect memory + match most vision model limits.
	const maxBytes = 10 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read feishu resource: %w", err)
	}
	if len(data) > maxBytes {
		return nil, "", fmt.Errorf("feishu image exceeds %d bytes (skipped)", maxBytes)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		// Feishu sometimes returns application/octet-stream for images.
		// Sniff from magic bytes; default to jpeg which vision providers accept.
		ct = sniffImageContentType(data)
	}
	return data, ct, nil
}

// extFromContentType returns a file extension (including leading dot) for
// common image content-types. Empty string when unknown.
func extFromContentType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ""
}

// sniffImageContentType inspects the first few bytes of an image payload and
// returns a normalized content-type. Falls back to image/jpeg.
func sniffImageContentType(data []byte) string {
	if len(data) >= 4 {
		switch {
		case data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
			return "image/jpeg"
		case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4e && data[3] == 0x47:
			return "image/png"
		case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46:
			return "image/gif"
		case len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 &&
			data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50:
			return "image/webp"
		}
	}
	return "image/jpeg"
}

// sendText sends a text message to a chat, returns message_id.
func (b *FeishuBot) sendText(chatID, text string) (string, error) {
	token, err := b.refreshToken()
	if err != nil {
		return "", err
	}

	// Truncate to 4000 chars (Feishu limit ~4096)
	runes := []rune(text)
	if len(runes) > 4000 {
		text = string(runes[:4000]) + "..."
	}

	contentJSON, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]string{
		"receive_id":  chatID,
		"msg_type":    "text",
		"content":     string(contentJSON),
	}
	data, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST",
		b.apiBase()+"/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("send message error %d: %s", result.Code, result.Msg)
	}
	return result.Data.MessageID, nil
}

// patchText updates an existing message content.
func (b *FeishuBot) patchText(messageID, text string) error {
	token, err := b.refreshToken()
	if err != nil {
		return err
	}

	runes := []rune(text)
	if len(runes) > 4000 {
		text = string(runes[:4000]) + "..."
	}

	contentJSON, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]string{
		"msg_type": "text",
		"content":  string(contentJSON),
	}
	data, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH",
		b.apiBase()+"/im/v1/messages/"+messageID,
		bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// TestFeishuBot verifies App ID + Secret by getting an access token and bot info.
// Returns the bot app_name on success.
func TestFeishuBot(appID, appSecret string) (string, error) {
	domain := "open.feishu.cn"
	payload := map[string]string{"app_id": appID, "app_secret": appSecret}
	data, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		"https://"+domain+"/open-apis/auth/v3/app_access_token/internal",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result feishuTokenResp
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu: %s (code %d)", result.Msg, result.Code)
	}

	// Get bot name
	req2, _ := http.NewRequest("GET", "https://"+domain+"/open-apis/bot/v3/info", nil)
	req2.Header.Set("Authorization", "Bearer "+result.AppAccessToken)
	resp2, err := client.Do(req2)
	if err != nil {
		return "feishu", nil
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))
	var botInfo struct {
		Bot struct{ AppName string `json:"app_name"` } `json:"bot"`
	}
	json.Unmarshal(body2, &botInfo)
	if botInfo.Bot.AppName != "" {
		return botInfo.Bot.AppName, nil
	}
	return "feishu", nil
}

// SendFeishuApprovedNotice sends an approval notification to a Feishu user via DM.
// Used by the API when an admin approves a pending user.
func SendFeishuApprovedNotice(appID, appSecret, openID string) error {
	// Get app_access_token
	type tokenResp struct {
		Code           int    `json:"code"`
		AppAccessToken string `json:"app_access_token"`
	}
	payload := fmt.Sprintf(`{"app_id":%q,"app_secret":%q}`, appID, appSecret)
	resp, err := http.Post(
		"https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal",
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return err
	}
	if tr.Code != 0 || tr.AppAccessToken == "" {
		return fmt.Errorf("token error code=%d", tr.Code)
	}

	// Send message to user's open_id
	msgBody := `{"receive_id":"` + openID + `","msg_type":"text","content":"{\"text\":\"✅ 授权成功！您已获得访问权限，现在可以直接发消息开始对话了。\"}"}`
	req, _ := http.NewRequest("POST",
		"https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id",
		strings.NewReader(msgBody))
	req.Header.Set("Authorization", "Bearer "+tr.AppAccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	r2, err := client.Do(req)
	if err != nil {
		return err
	}
	defer r2.Body.Close()
	return nil
}

// sendCard sends a markdown card message and returns the message_id.
func (b *FeishuBot) sendCard(chatID, text string) (string, error) {
	token, err := b.refreshToken()
	if err != nil {
		return "", err
	}

	// Build an interactive card with a single markdown element
	card := map[string]interface{}{
		"schema": "2.0",
		"body": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{
					"tag":     "markdown",
					"content": text,
				},
			},
		},
		"config": map[string]interface{}{
			"update_multi": true,
		},
	}
	cardJSON, _ := json.Marshal(card)

	payload := map[string]interface{}{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}
	data, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST",
		b.apiBase()+"/im/v1/messages?receive_id_type=chat_id",
		bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		// Fallback to plain text
		return b.sendText(chatID, text)
	}
	return result.Data.MessageID, nil
}

// patchCard updates an existing card message with new markdown content.
func (b *FeishuBot) patchCard(messageID, text string) error {
	token, err := b.refreshToken()
	if err != nil {
		return err
	}

	runes := []rune(text)
	if len(runes) > 4000 {
		text = string(runes[:4000]) + "..."
	}

	card := map[string]interface{}{
		"schema": "2.0",
		"body": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{
					"tag":     "markdown",
					"content": text,
				},
			},
		},
		"config": map[string]interface{}{
			"update_multi": true,
		},
	}
	cardJSON, _ := json.Marshal(card)

	payload := map[string]interface{}{
		"content": string(cardJSON),
	}
	data, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH",
		b.apiBase()+"/im/v1/messages/"+messageID,
		bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ── Per-chat configuration ─────────────────────────────────────────────────
// Stored in {agentDir}/feishu-chat-config/{chatID}.json

type feishuChatConfig struct {
	ListenAll bool `json:"listenAll"` // true = respond to all group messages; false = @mention only (default)
}

func (b *FeishuBot) chatConfigPath(chatID string) string {
	dir := filepath.Join(b.agentDir, "feishu-chat-config")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, strings.ReplaceAll(chatID, "/", "_")+".json")
}

func (b *FeishuBot) loadChatConfig(chatID string) feishuChatConfig {
	data, err := os.ReadFile(b.chatConfigPath(chatID))
	if err != nil {
		return feishuChatConfig{}
	}
	var cfg feishuChatConfig
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (b *FeishuBot) saveChatConfig(chatID string, cfg feishuChatConfig) {
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(b.chatConfigPath(chatID), data, 0600)
}

// handleGroupCommand checks if the @mentioned text is a bot command and handles it.
// Returns true if the text was a command (caller should not process it further).
func (b *FeishuBot) handleGroupCommand(chatID, text string) bool {
	cmd := strings.ToLower(strings.TrimSpace(text))

	switch {
	case cmd == "/listen all" || cmd == "/监听全部" || cmd == "/全部消息":
		cfg := feishuChatConfig{ListenAll: true}
		b.saveChatConfig(chatID, cfg)
		_, _ = b.sendText(chatID, "✅ 已切换为**全部消息模式**：我将回复群里的所有消息。\n\n发送 `/listen mention` 可切换回仅响应 @ 模式。")
		return true

	case cmd == "/listen mention" || cmd == "/监听@" || cmd == "/仅@":
		cfg := feishuChatConfig{ListenAll: false}
		b.saveChatConfig(chatID, cfg)
		_, _ = b.sendText(chatID, "✅ 已切换为 **@提及模式**：我只响应 @ 我的消息。\n\n发送 `/listen all` 可切换为响应全部消息。")
		return true

	case cmd == "/status" || cmd == "/状态":
		chatCfg := b.loadChatConfig(chatID)
		mode := "仅响应 @ 提及"
		if chatCfg.ListenAll {
			mode = "响应所有消息"
		}
		_, _ = b.sendText(chatID, fmt.Sprintf("ℹ️ 当前模式：**%s**\n\n可用命令：\n• `/listen all` — 响应全部消息\n• `/listen mention` — 仅响应 @\n• `/status` — 查看当前状态", mode))
		return true

	case cmd == "/help" || cmd == "/帮助":
		_, _ = b.sendText(chatID, "📖 群聊命令（需 @ 我）：\n\n• `/listen all` — 响应群里所有消息\n• `/listen mention` — 仅响应 @ 我的消息（默认）\n• `/status` — 查看当前配置\n• `/help` — 显示帮助")
		return true
	}
	return false
}

// ── Persistent event dedup ────────────────────────────────────────────────
// Saves seen event IDs to disk so they survive restarts.

func (b *FeishuBot) seenEventPath() string {
	dir := filepath.Join(b.agentDir, "feishu-seen-events")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, b.channelID+".json")
}

func (b *FeishuBot) loadSeenEvents() {
	data, err := os.ReadFile(b.seenEventPath())
	if err != nil {
		return
	}
	var m map[string]int64
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	b.seenMu.Lock()
	defer b.seenMu.Unlock()
	cutoff := time.Now().Add(-30 * time.Minute).UnixMilli()
	for id, ts := range m {
		if ts > cutoff {
			b.seenEvents[id] = time.UnixMilli(ts)
		}
	}
}

func (b *FeishuBot) persistSeenEvents() {
	b.seenMu.Lock()
	m := make(map[string]int64, len(b.seenEvents))
	for id, t := range b.seenEvents {
		m[id] = t.UnixMilli()
	}
	b.seenMu.Unlock()
	data, _ := json.Marshal(m)
	_ = os.WriteFile(b.seenEventPath(), data, 0600)
}

// ── Sender name cache ─────────────────────────────────────────────────────

var (
	senderNameCache   sync.Map // openID → name
	senderNameFetched sync.Map // openID → bool (fetch attempted)
)

// getSenderName returns the display name for a Feishu user, fetching if needed.
func (b *FeishuBot) getSenderName(openID string) string {
	if v, ok := senderNameCache.Load(openID); ok {
		return v.(string)
	}
	// Only fetch once per openID per process lifetime
	if _, attempted := senderNameFetched.LoadOrStore(openID, true); attempted {
		return ""
	}
	go func() {
		token, err := b.refreshToken()
		if err != nil {
			return
		}
		req, _ := http.NewRequest("GET",
			b.apiBase()+"/contact/v3/users/"+openID+"?user_id_type=open_id", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := b.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var result struct {
			Code int `json:"code"`
			Data struct {
				User struct {
					Name        string `json:"name"`
					DisplayName string `json:"display_name"`
					Nickname    string `json:"nickname"`
					EnName      string `json:"en_name"`
				} `json:"user"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil || result.Code != 0 {
			return
		}
		u := result.Data.User
		name := u.Name
		if name == "" {
			name = u.DisplayName
		}
		if name == "" {
			name = u.Nickname
		}
		if name == "" {
			name = u.EnName
		}
		if name != "" {
			senderNameCache.Store(openID, name)
		}
	}()
	return ""
}
