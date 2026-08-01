package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestCORSDefaultsToSameOriginAndAllowsExactConfiguredOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := config.GatewayConfig{
		PublicURL: "https://panel.example.com",
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"https://remote.example.com"},
		},
	}
	router := gin.New()
	router.Use(corsMiddleware(gateway))
	router.OPTIONS("/api/config", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	tests := []struct {
		name       string
		origin     string
		host       string
		wantStatus int
		wantOrigin string
	}{
		{"same origin", "http://panel.local", "panel.local", http.StatusNoContent, "http://panel.local"},
		{"public URL", "https://panel.example.com", "panel.local", http.StatusNoContent, "https://panel.example.com"},
		{"configured remote UI", "https://remote.example.com", "panel.local", http.StatusNoContent, "https://remote.example.com"},
		{"unconfigured cross origin", "https://evil.example.com", "panel.local", http.StatusForbidden, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
			req.Host = tt.host
			req.Header.Set("Origin", tt.origin)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", res.Code, tt.wantStatus, res.Body.String())
			}
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Fatalf("allow origin = %q, want %q", got, tt.wantOrigin)
			}
			if res.Header().Get("Access-Control-Allow-Origin") == "*" {
				t.Fatal("wildcard origin must never be emitted")
			}
		})
	}
}

func TestConfigPatchRecursivelyPreservesGatewayFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Gateway = config.GatewayConfig{
		Port:      8080,
		Bind:      "lan",
		PublicURL: "https://panel.example.com",
		CORS:      config.CORSConfig{AllowedOrigins: []string{"https://ui.example.com"}},
	}
	path := filepath.Join(t.TempDir(), "config.json")
	handler := &configHandler{cfg: cfg, configPath: path}
	router := gin.New()
	router.PATCH("/config", handler.Patch)

	req := httptest.NewRequest(http.MethodPatch, "/config", strings.NewReader(`{"gateway":{"port":9090}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	if cfg.Gateway.Port != 9090 || cfg.Gateway.Bind != "lan" || cfg.Gateway.PublicURL != "https://panel.example.com" {
		t.Fatalf("gateway fields were not recursively merged: %+v", cfg.Gateway)
	}
	if got := cfg.Gateway.CORS.AllowedOrigins; len(got) != 1 || got[0] != "https://ui.example.com" {
		t.Fatalf("nested CORS config was lost: %#v", got)
	}
}

func TestConfigGetMasksAllRegistrySecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Providers = []config.ProviderEntry{{ID: "p", APIKey: "provider-secret-value"}}
	cfg.Models = []config.ModelEntry{{ID: "m", APIKey: "model-secret-value"}}
	cfg.Channels = []config.ChannelEntry{{
		ID: "c",
		Config: map[string]string{
			"botToken":  "channel-token-value",
			"appSecret": "channel-secret-value",
			"password":  "channel-password-value",
		},
	}}
	cfg.Tools = []config.ToolEntry{{ID: "t", APIKey: "tool-secret-value"}}
	handler := &configHandler{cfg: cfg}
	router := gin.New()
	router.GET("/config", handler.Get)

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/config", nil))
	for _, secret := range []string{
		cfg.Auth.Token,
		"provider-secret-value",
		"model-secret-value",
		"channel-token-value",
		"channel-secret-value",
		"channel-password-value",
		"tool-secret-value",
	} {
		if strings.Contains(res.Body.String(), secret) {
			t.Fatalf("response leaked secret %q: %s", secret, res.Body.String())
		}
	}
}

func TestAgentCreateRejectsUnknownModelAndPersistsToolPolicyAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	mgr := agent.NewManager(root)
	cfg := config.Default()
	cfg.Models = []config.ModelEntry{{
		ID: "known", Provider: "openai", Model: "gpt-test", Name: "Known",
	}}
	handler := &agentHandler{cfg: cfg, manager: mgr}
	router := gin.New()
	router.POST("/agents", handler.Create)

	for _, body := range []string{
		`{"id":"bad-id","name":"Bad","modelId":"missing"}`,
		`{"id":"bad-model","name":"Bad","model":"openai/missing"}`,
	} {
		unknown := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(unknown, req)
		if unknown.Code != http.StatusBadRequest {
			t.Fatalf("unknown model body %s: status = %d, response=%s", body, unknown.Code, unknown.Body.String())
		}
	}
	for _, id := range []string{"bad-id", "bad-model"} {
		if _, err := os.Stat(filepath.Join(root, id)); !os.IsNotExist(err) {
			t.Fatalf("unknown model must not create agent %q, stat err=%v", id, err)
		}
	}

	created := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(
		`{"id":"good","name":"Good","model":"openai/gpt-test","toolPolicy":{"profile":"minimal"}}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(created, req)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", created.Code, created.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "good", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal(stored["toolPolicy"], &policy); err != nil || policy.Profile != "minimal" {
		t.Fatalf("stored toolPolicy = %s, err=%v", stored["toolPolicy"], err)
	}
}

func TestModelCreateValidatesRequiredFieldsAndProviderReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Providers = []config.ProviderEntry{{ID: "anthropic-main", Provider: "anthropic"}}
	handler := &modelHandler{cfg: cfg, configPath: filepath.Join(t.TempDir(), "config.json")}
	router := gin.New()
	router.POST("/models", handler.Create)

	for _, body := range []string{
		`{"name":"M","provider":"anthropic","model":"claude"}`,
		`{"id":"m","provider":"anthropic","model":"claude"}`,
		`{"id":"m","name":"M","model":"claude"}`,
		`{"id":"m","name":"M","provider":"anthropic"}`,
		`{"id":"m","name":"M","provider":"anthropic","model":"claude","providerId":"missing"}`,
		`{"id":"m","name":"M","provider":"openai","model":"gpt","providerId":"anthropic-main"}`,
	} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/models", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status=%d response=%s", body, res.Code, res.Body.String())
		}
	}
	if len(cfg.Models) != 0 {
		t.Fatalf("invalid models mutated config: %#v", cfg.Models)
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/models", strings.NewReader(
		`{"id":"valid","name":"Valid","provider":"anthropic","model":"claude","providerId":"anthropic-main"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("valid model status=%d response=%s", res.Code, res.Body.String())
	}
}

func TestGlobalChannelTestRejectsTypesWithoutRealProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Channels = []config.ChannelEntry{{ID: "web", Type: "web", Status: "untested"}}
	handler := &channelHandler{cfg: cfg, configPath: filepath.Join(t.TempDir(), "config.json")}
	router := gin.New()
	router.POST("/channels/:id/test", handler.Test)

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/channels/web/test", nil))
	if res.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, body=%s", res.Code, res.Body.String())
	}
	if cfg.Channels[0].Status != "error" || !strings.Contains(res.Body.String(), "no real connectivity probe") {
		t.Fatalf("unexpected test result: status=%q body=%s", cfg.Channels[0].Status, res.Body.String())
	}
}
