package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestFetchModelsBlocksCustomLoopbackEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	cfg := config.Default()
	handler := &modelHandler{cfg: cfg}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/models/probe?provider=custom&apiKey=test&baseUrl="+url.QueryEscape(server.URL),
		nil,
	)
	handler.FetchModels(ctx)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if requests.Load() != 0 {
		t.Fatal("blocked endpoint received a request")
	}
}

func TestFetchModelsAllowsExactOllamaLoopbackEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "llama3.2"}},
		})
	}))
	defer server.Close()

	cfg := config.Default()
	handler := &modelHandler{cfg: cfg}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/models/probe?provider=ollama&baseUrl="+url.QueryEscape(server.URL),
		nil,
	)
	handler.FetchModels(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("llama3.2")) {
		t.Fatalf("model response missing: %s", recorder.Body.String())
	}
}

func TestProviderTestNormalizesAndAllowsOllamaEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	valid, message := testOpenAICompatKey("ollama", "", server.URL)
	if !valid {
		t.Fatalf("ollama provider test failed: %s", message)
	}
	valid, message = testOpenAICompatKey("custom", "test", server.URL)
	if valid || !bytes.Contains([]byte(message), []byte("blocked")) {
		t.Fatalf("custom loopback should be blocked: valid=%v message=%s", valid, message)
	}
}

func TestProviderAndModelCreateRejectPrivateBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	configPath := filepath.Join(t.TempDir(), "aipanel.json")

	providerBody := []byte(`{
		"name":"blocked",
		"provider":"custom",
		"apiKey":"test",
		"baseUrl":"http://127.0.0.1:8080/v1"
	}`)
	providerRecorder := httptest.NewRecorder()
	providerCtx, _ := gin.CreateTestContext(providerRecorder)
	providerCtx.Request = httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewReader(providerBody))
	providerCtx.Request.Header.Set("Content-Type", "application/json")
	(&providerHandler{cfg: cfg, configPath: configPath}).Create(providerCtx)
	if providerRecorder.Code != http.StatusBadRequest {
		t.Fatalf("provider status=%d body=%s", providerRecorder.Code, providerRecorder.Body.String())
	}

	modelBody := []byte(`{
		"id":"blocked-model",
		"name":"blocked",
		"provider":"custom",
		"model":"test",
		"baseUrl":"http://169.254.169.254/latest"
	}`)
	modelRecorder := httptest.NewRecorder()
	modelCtx, _ := gin.CreateTestContext(modelRecorder)
	modelCtx.Request = httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewReader(modelBody))
	modelCtx.Request.Header.Set("Content-Type", "application/json")
	(&modelHandler{cfg: cfg, configPath: configPath}).Create(modelCtx)
	if modelRecorder.Code != http.StatusBadRequest {
		t.Fatalf("model status=%d body=%s", modelRecorder.Code, modelRecorder.Body.String())
	}
}
