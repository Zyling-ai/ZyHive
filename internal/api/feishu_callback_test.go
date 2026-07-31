package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/gin-gonic/gin"
)

func newFeishuCallbackTestManager(t *testing.T) *agent.Manager {
	t.Helper()
	root := t.TempDir()
	agentDir := filepath.Join(root, "alice")
	if err := os.MkdirAll(agentDir, 0700); err != nil {
		t.Fatal(err)
	}
	configJSON := `{
		"id":"alice",
		"name":"Alice",
		"channels":[{
			"id":"feishu-main",
			"name":"Feishu",
			"type":"feishu",
			"enabled":true,
			"config":{
				"encryptKey":"callback-secret",
				"verificationToken":"verify-token"
			}
		}]
	}`
	if err := os.WriteFile(filepath.Join(agentDir, "config.json"), []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	manager := agent.NewManager(root)
	if err := manager.LoadAll(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func signedFeishuContext(t *testing.T, body, timestamp, nonce, secret string) *gin.Context {
	t.Helper()
	hash := sha256.Sum256([]byte(timestamp + nonce + secret + body))
	request := httptest.NewRequest(http.MethodPost, "/feishu/card-callback", strings.NewReader(body))
	request.Header.Set("X-Lark-Request-Timestamp", timestamp)
	request.Header.Set("X-Lark-Request-Nonce", nonce)
	request.Header.Set("X-Lark-Signature", fmt.Sprintf("%x", hash))
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}

func TestFeishuCallbackAcceptsValidSignatureOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &feishuCardCallbackHandler{manager: newFeishuCallbackTestManager(t)}
	body := `{"type":"card.action.trigger","action":{"value":{"agent_id":"alice","session_id":"s1","action":"approve"}}}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	request := FeishuCardCallbackRequest{}
	request.Action.Value = map[string]string{"agent_id": "alice"}

	first := signedFeishuContext(t, body, timestamp, "nonce-1", "callback-secret")
	if !handler.authorizeCallback(first, []byte(body), &request) {
		t.Fatal("valid callback signature was rejected")
	}
	second := signedFeishuContext(t, body, timestamp, "nonce-1", "callback-secret")
	if handler.authorizeCallback(second, []byte(body), &request) {
		t.Fatal("replayed callback signature was accepted")
	}
}

func TestFeishuCallbackRejectsStaleTimestamp(t *testing.T) {
	handler := &feishuCardCallbackHandler{manager: newFeishuCallbackTestManager(t)}
	body := `{"type":"card.action.trigger","action":{"value":{"agent_id":"alice"}}}`
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	request := FeishuCardCallbackRequest{}
	request.Action.Value = map[string]string{"agent_id": "alice"}
	context := signedFeishuContext(t, body, timestamp, "nonce-stale", "callback-secret")
	if handler.authorizeCallback(context, []byte(body), &request) {
		t.Fatal("stale callback timestamp was accepted")
	}
}

func TestFeishuChallengeAcceptsConfiguredVerificationToken(t *testing.T) {
	handler := &feishuCardCallbackHandler{manager: newFeishuCallbackTestManager(t)}
	request := FeishuCardCallbackRequest{
		Type:      "url_verification",
		Challenge: "challenge",
		Token:     "verify-token",
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/feishu/card-callback", nil)
	if !handler.authorizeCallback(context, nil, &request) {
		t.Fatal("configured verification token was rejected")
	}
}

func TestFeishuCallbackHandlerRejectsUnsignedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &feishuCardCallbackHandler{manager: newFeishuCallbackTestManager(t)}
	router := gin.New()
	router.POST("/feishu/card-callback", handler.Handle)
	body := `{"type":"card.action.trigger","action":{"value":{"agent_id":"alice","session_id":"s1","action":"approve"}}}`
	request := httptest.NewRequest(http.MethodPost, "/feishu/card-callback", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned callback should be rejected, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
