package api

// Feishu card action callback handler.
// When a user clicks a button on an interactive card, Feishu POSTs to this endpoint.
// We inject the user's choice into the agent's session so the AI can respond to it.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/session"
)

// feishuCardCallbackHandler handles POST /feishu/card-callback
type feishuCardCallbackHandler struct {
	manager *agent.Manager
	pool    *agent.Pool
}

// FeishuCardCallbackRequest is the payload Feishu sends when a card button is clicked.
type FeishuCardCallbackRequest struct {
	Challenge   string `json:"challenge"`    // URL verification challenge
	Type        string `json:"type"`         // "url_verification" or "card.action.trigger"
	Action      struct {
		Value     map[string]string `json:"value"`      // button value map
		Tag       string            `json:"tag"`        // "button"
		OpenID    string            `json:"open_id"`    // who clicked
	} `json:"action"`
	OpenID      string `json:"open_id"`
	OperatorID  struct {
		OpenID string `json:"open_id"`
	} `json:"operator"`
	Token       string `json:"token"`
}

// ServeHTTP handles the Feishu card callback.
func (h *feishuCardCallbackHandler) Handle(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))

	var req FeishuCardCallbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
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

// verifyFeishuSign verifies the Feishu callback signature (optional but recommended).
func verifyFeishuSign(timestamp, nonce, body, secret, signature string) bool {
	if secret == "" {
		return true // skip verification if not configured
	}
	s := timestamp + nonce + secret + body
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h) == signature
}
