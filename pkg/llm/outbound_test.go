package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/netguard"
)

func TestCustomModelClientBlocksLoopbackBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("blocked custom endpoint received a request")
	}))
	defer server.Close()

	client := NewClient("custom", server.URL+"/v1")
	_, err := client.Stream(context.Background(), &ChatRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	})
	if !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("expected loopback block, got %v", err)
	}
}

func TestOllamaModelClientAllowsExactLoopbackBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewClient("ollama", server.URL+"/v1")
	events, err := client.Stream(context.Background(), &ChatRequest{
		Model:    "llama3.2",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for event := range events {
		text.WriteString(event.Text)
	}
	if text.String() != "ok" {
		t.Fatalf("unexpected stream output %q", text.String())
	}
}

func TestOllamaClientRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target received a request")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/private", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := NewProviderHTTPClient("ollama", source.URL+"/v1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, source.URL+"/redirect", nil)
	_, err = client.Do(req)
	if !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("expected redirect block, got %v", err)
	}
}

func TestEmbedderUsesProviderOutboundPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float32{1, 2, 3}}},
		})
	}))
	defer server.Close()

	blocked := NewEmbedder("custom", server.URL+"/v1", "embed")
	if _, err := blocked.Embed(context.Background(), "key", []string{"hello"}); !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("custom loopback embed should be blocked, got %v", err)
	}

	allowed := NewEmbedder("ollama", server.URL+"/v1", "embed")
	vectors, err := allowed.Embed(context.Background(), "", []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}
