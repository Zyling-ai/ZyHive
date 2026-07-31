package api

// Feishu card action callback handler.
// When a user clicks a button on an interactive card, Feishu POSTs to this endpoint.
// We inject the user's choice into the agent's session so the AI can respond to it.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/gin-gonic/gin"
)

// feishuCardCallbackHandler handles POST /feishu/card-callback
type feishuCardCallbackHandler struct {
	manager    *agent.Manager
	pool       *agent.Pool
	replayMu   sync.Mutex
	seenNonces map[string]time.Time
}

// FeishuCardCallbackRequest is the payload Feishu sends when a card button is clicked.
type FeishuCardCallbackRequest struct {
	Challenge string `json:"challenge"` // URL verification challenge
	Type      string `json:"type"`      // "url_verification" or "card.action.trigger"
	Action    struct {
		Value  map[string]string `json:"value"`   // button value map
		Tag    string            `json:"tag"`     // "button"
		OpenID string            `json:"open_id"` // who clicked
	} `json:"action"`
	OpenID     string `json:"open_id"`
	OperatorID struct {
		OpenID string `json:"open_id"`
	} `json:"operator"`
	Token string `json:"token"`
}

// ServeHTTP handles the Feishu card callback.
func (h *feishuCardCallbackHandler) Handle(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))

	var req FeishuCardCallbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if !h.authorizeCallback(c, body, &req) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid callback signature"})
		return
	}

	// URL verification challenge (Feishu sends this when you first configure the callback URL)
	if req.Type == "url_verification" || req.Challenge != "" {
		c.JSON(http.StatusOK, gin.H{"challenge": req.Challenge})
		return
	}

	// Get the operator open_id
	operatorOpenID := req.OpenID
	if operatorOpenID == "" {
		operatorOpenID = req.Action.OpenID
	}
	if operatorOpenID == "" {
		operatorOpenID = req.OperatorID.OpenID
	}

	// Extract routing info from button value
	// Expected value keys: agent_id, session_id, action, label
	val := req.Action.Value
	if val == nil {
		c.JSON(http.StatusOK, gin.H{"toast": map[string]interface{}{"type": "info", "content": "已收到"}})
		return
	}

	agentID := val["agent_id"]
	sessionID := val["session_id"]
	actionKey := val["action"]
	actionLabel := val["label"]
	if actionLabel == "" {
		actionLabel = actionKey
	}

	log.Printf("[feishu-callback] open_id=%s agent=%s session=%s action=%s",
		operatorOpenID, agentID, sessionID, actionKey)

	// If we have enough context, inject the user's choice into the session
	if agentID != "" && sessionID != "" && actionKey != "" {
		go func() {
			h.injectCallback(agentID, sessionID, operatorOpenID, actionKey, actionLabel, val)
		}()
	}

	// Return a toast notification to the user who clicked
	toast := fmt.Sprintf("已记录：%s", actionLabel)
	c.JSON(http.StatusOK, gin.H{
		"toast": map[string]interface{}{
			"type":    "success",
			"content": toast,
		},
	})
}

// injectCallback injects a system message into the agent's session to notify the AI of the user's choice.
func (h *feishuCardCallbackHandler) injectCallback(agentID, sessionID, operatorOpenID, actionKey, actionLabel string, extraVal map[string]string) {
	ag, ok := h.manager.Get(agentID)
	if !ok {
		log.Printf("[feishu-callback] agent %q not found", agentID)
		return
	}

	store := session.NewStore(ag.SessionDir)

	// Build a system injection message so the AI knows what happened
	extraParts := []string{}
	for k, v := range extraVal {
		if k != "agent_id" && k != "session_id" && k != "action" && k != "label" {
			extraParts = append(extraParts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	extra := ""
	if len(extraParts) > 0 {
		extra = "\n附加信息：" + strings.Join(extraParts, "，")
	}

	// Inject as a "user" message so the AI responds to it naturally
	userMsg := fmt.Sprintf("[卡片操作] 用户（open_id=%s）点击了按钮：**%s**（action=%s）%s",
		operatorOpenID, actionLabel, actionKey, extra)

	content, _ := json.Marshal(userMsg)

	// Ensure session exists
	if _, _, err := store.GetOrCreate(sessionID, agentID); err != nil {
		log.Printf("[feishu-callback] ensure session error: %v", err)
		return
	}
	if err := store.AppendMessage(sessionID, "user", content); err != nil {
		log.Printf("[feishu-callback] append message error: %v", err)
		return
	}

	// Trigger the AI to respond — run a background query
	if h.pool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// The follow-up prompt is the injected message itself (already in history)
	// We send an empty trigger to make the AI pick it up
	triggerMsg := fmt.Sprintf("用户（open_id=%s）刚才点击了卡片按钮「%s」，请根据用户的选择继续处理。",
		operatorOpenID, actionLabel)

	events, err := h.pool.RunStreamEvents(ctx, agentID, triggerMsg, sessionID, nil, nil,
		fmt.Sprintf("飞书卡片回调：用户 open_id=%s 选择了 action=%s label=%s", operatorOpenID, actionKey, actionLabel))
	if err != nil {
		log.Printf("[feishu-callback] run error: %v", err)
		return
	}

	// Collect response (the AI will send the reply via its own stream → feishu bot)
	var sb strings.Builder
	for ev := range events {
		if ev.Type == "text_delta" {
			sb.WriteString(ev.Text)
		}
	}
	log.Printf("[feishu-callback] AI response (%d chars) for session %s", sb.Len(), sessionID)
}

type feishuCallbackCredential struct {
	encryptKey        string
	verificationToken string
}

func (h *feishuCardCallbackHandler) authorizeCallback(c *gin.Context, body []byte, req *FeishuCardCallbackRequest) bool {
	agentID := ""
	if req.Action.Value != nil {
		agentID = req.Action.Value["agent_id"]
	}
	credentials := h.callbackCredentials(agentID)
	if len(credentials) == 0 {
		return false
	}

	isChallenge := req.Type == "url_verification" || req.Challenge != ""
	if isChallenge && req.Token != "" {
		for _, credential := range credentials {
			if credential.verificationToken != "" && secretsEqual(req.Token, credential.verificationToken) {
				return true
			}
		}
	}

	timestamp := c.GetHeader("X-Lark-Request-Timestamp")
	nonce := c.GetHeader("X-Lark-Request-Nonce")
	signature := c.GetHeader("X-Lark-Signature")
	if !freshFeishuTimestamp(timestamp, time.Now()) || nonce == "" || signature == "" {
		return false
	}
	for _, credential := range credentials {
		if verifyFeishuSign(timestamp, nonce, string(body), credential.encryptKey, signature) {
			if isChallenge {
				return true
			}
			return h.acceptNonce(timestamp + "|" + nonce + "|" + signature)
		}
	}
	return false
}

func (h *feishuCardCallbackHandler) callbackCredentials(agentID string) []feishuCallbackCredential {
	if h.manager == nil {
		return nil
	}
	var agents []*agent.Agent
	if agentID != "" {
		if ag, ok := h.manager.Get(agentID); ok {
			agents = []*agent.Agent{ag}
		}
	} else {
		agents = h.manager.List()
	}

	var credentials []feishuCallbackCredential
	for _, ag := range agents {
		for _, channel := range ag.Channels {
			if !channel.Enabled || channel.Type != "feishu" {
				continue
			}
			credential := feishuCallbackCredential{
				encryptKey:        configValue(channel.Config, "encryptKey", "encrypt_key"),
				verificationToken: configValue(channel.Config, "verificationToken", "verification_token"),
			}
			if credential.encryptKey != "" || credential.verificationToken != "" {
				credentials = append(credentials, credential)
			}
		}
	}
	return credentials
}

func configValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := values[key]; value != "" {
			return value
		}
	}
	return ""
}

func freshFeishuTimestamp(raw string, now time.Time) bool {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Unix() - seconds
	if delta < 0 {
		delta = -delta
	}
	return delta <= int64((5 * time.Minute).Seconds())
}

func (h *feishuCardCallbackHandler) acceptNonce(key string) bool {
	h.replayMu.Lock()
	defer h.replayMu.Unlock()
	if h.seenNonces == nil {
		h.seenNonces = make(map[string]time.Time)
	}
	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)
	for nonce, seenAt := range h.seenNonces {
		if seenAt.Before(cutoff) {
			delete(h.seenNonces, nonce)
		}
	}
	if _, exists := h.seenNonces[key]; exists {
		return false
	}
	h.seenNonces[key] = now
	return true
}

// verifyFeishuSign verifies the Feishu callback signature.
func verifyFeishuSign(timestamp, nonce, body, secret, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	s := timestamp + nonce + secret + body
	h := sha256.Sum256([]byte(s))
	expected := fmt.Sprintf("%x", h)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
