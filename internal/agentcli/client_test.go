package agentcli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRequestAddsAuthAndParsesJSONError(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		http.Error(w, `{"error":"nope"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
	_, err := c.Request(context.Background(), "GET", "/api/test", nil)
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q, want Bearer tok", gotAuth)
	}
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.ExitCode != ExitAuth || apiErr.Message == "" {
		t.Fatalf("APIError = %+v, want auth exit with message", apiErr)
	}
}

func TestClientStreamSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"text_delta\",\"text\":\"hi\"}\n\n")
		fmt.Fprint(w, ": keepalive\n\n")
		fmt.Fprint(w, "data: {\"type\":\"done\",\"sessionId\":\"s1\"}\n\n")
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	var events []SSEEvent
	if err := c.StreamSSE(context.Background(), "POST", "/chat", map[string]any{"message": "x"}, func(ev SSEEvent) error {
		events = append(events, ev)
		return nil
	}); err != nil {
		t.Fatalf("StreamSSE: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2 (%v)", len(events), events)
	}
	if events[0]["text"] != "hi" || events[1]["sessionId"] != "s1" {
		t.Fatalf("events = %#v", events)
	}
}

func TestExtractGlobalsAndDispatchAPI(t *testing.T) {
	opts, rest := extractGlobals([]string{"api", "GET", "/api/ok", "--json", "--host", "http://x", "--token=tok"})
	if !opts.JSON || opts.Host != "http://x" || opts.Token != "tok" {
		t.Fatalf("opts = %+v", opts)
	}
	if len(rest) != 3 || rest[0] != "api" {
		t.Fatalf("rest = %#v", rest)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ok" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	code := Dispatch([]string{"api", "GET", "/api/ok", "--json", "--host", srv.URL, "--token", "tok"})
	if code != ExitOK {
		t.Fatalf("Dispatch exit = %d, want 0", code)
	}
}

func TestCodeOf(t *testing.T) {
	if codeOf(usageErr("bad")) != ExitUsage {
		t.Fatal("usage error should map to ExitUsage")
	}
	if codeOf(&APIError{StatusCode: 404, ExitCode: ExitNotFound}) != ExitNotFound {
		t.Fatal("api 404 should map to ExitNotFound")
	}
}
