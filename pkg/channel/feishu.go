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
	"strings"
	"sync"
	"time"

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
			return
		}
		var ev feishuEvent
		if err := json.Unmarshal(frame.Payload, &ev); err != nil {
			log.Printf("[feishu] event json error: %v payload=%s", err, string(frame.Payload[:min(200, len(frame.Payload))]))
			return
		}
		// Dedup by event_id
		if ev.Header.EventID != "" {
			b.seenMu.Lock()
			if _, seen := b.seenEvents[ev.Header.EventID]; seen {
				b.seenMu.Unlock()
				return
			}
			b.seenEvents[ev.Header.EventID] = time.Now()
			if len(b.seenEvents) > 500 {
				cutoff := time.Now().Add(-30 * time.Minute)
				for k, t := range b.seenEvents {
					if t.Before(cutoff) {
						delete(b.seenEvents, k)
					}
				}
			}
			b.seenMu.Unlock()
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

	// Only handle text messages for now
	if msg.MessageType != "text" {
		return
	}

	// Serialize per chatID: only one LLM call at a time per chat
	muVal, _ := b.chatMu.LoadOrStore(msg.ChatID, &sync.Mutex{})
	mu := muVal.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Parse content: {"text":"hello"}
	var contentObj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &contentObj); err != nil {
		return
	}
	text := strings.TrimSpace(contentObj.Text)

	// Group chat: only respond when @mentioned
	isGroup := msg.ChatType == "group"
	if isGroup {
		mentioned := false
		for _, m := range msg.Mentions {
			if m.ID.OpenID == b.botOpenID {
				mentioned = true
				break
			}
		}
		if !mentioned {
			return
		}
		// Strip @mention keys from text (e.g. "@_user_1 ")
		for _, m := range msg.Mentions {
			text = strings.ReplaceAll(text, m.Key, "")
		}
		text = strings.TrimSpace(text)
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
	events, err := b.streamFunc(runCtx, b.agentID, text, feishuSessionID, nil, nil)
	if err != nil {
		_, _ = b.sendText(msg.ChatID, "⚠️ 出错了："+err.Error())
		return
	}

	// Stream: send card first, then patch card content (Feishu only supports PATCH on cards)
	var accumulated strings.Builder
	var sentMsgID string
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
				// Send initial card on first content
				if sentMsgID == "" && accumulated.Len() > 0 {
					id, err := b.sendCard(msg.ChatID, accumulated.String())
					if err != nil {
						log.Printf("[feishu] sendCard error: %v", err)
					} else {
						sentMsgID = id
						lastPatched = accumulated.String()
					}
				}
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
	finalText := strings.TrimSpace(accumulated.String())
	if finalText == "" {
		finalText = "(no response)"
	}
	if sentMsgID == "" {
		// Never sent anything yet
		if _, err := b.sendCard(msg.ChatID, finalText); err != nil {
			log.Printf("[feishu] sendCard error: %v", err)
		}
	} else {
		patchCard(finalText)
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
