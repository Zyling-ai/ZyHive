// internal/api/feishu_setup.go — REST endpoints driving the 4-step Feishu
// binding wizard (F1, 26.5.13v1).
//
//	POST /api/feishu/probe         body: {appId, appSecret}
//	POST /api/feishu/test-connect  body: {appId, appSecret}
//
// Both are unauthenticated only in the sense that they don't touch any agent
// state — they run a one-shot read-only check against Feishu's open platform
// using the operator-supplied credentials. The standard auth middleware still
// gates the endpoints themselves.

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/gin-gonic/gin"
)

type feishuSetupHandler struct{}

type probeRequest struct {
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
}

// Probe POST /api/feishu/probe
func (h *feishuSetupHandler) Probe(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 8*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}
	var req probeRequest
	if jerr := json.Unmarshal(raw, &req); jerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if req.AppID == "" || req.AppSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "appId and appSecret required"})
		return
	}

	// 12-second budget for the whole probe (probe.go uses 8s per request, but
	// runs multiple in sequence).
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()

	result, _ := channel.Probe(ctx, req.AppID, req.AppSecret)
	// Always return 200 with structured result; the wizard reads result.error.
	c.JSON(http.StatusOK, result)
}

// TestConnect POST /api/feishu/test-connect
//
// Lighter than Probe — only tries to fetch a token to confirm credentials
// still work, and reports the latency. Used by the wizard's "再次测试"
// button after the user has fixed something in the Feishu admin console.
func (h *feishuSetupHandler) TestConnect(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 8*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}
	var req probeRequest
	if jerr := json.Unmarshal(raw, &req); jerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if req.AppID == "" || req.AppSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "appId and appSecret required"})
		return
	}

	start := time.Now()
	// Reuse the bot-level test which is more thorough (gets bot info too).
	name, err := channel.TestFeishuBot(req.AppID, req.AppSecret)
	latency := time.Since(start)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"ok":        false,
			"error":     err.Error(),
			"latencyMs": latency.Milliseconds(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"botName":   name,
		"latencyMs": latency.Milliseconds(),
	})
}
