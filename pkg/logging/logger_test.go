package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestParseLevel — accepts common spellings + falls back to info.
func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"garbage": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestFromContext_NilSafe — nil ctx must not panic.
func TestFromContext_NilSafe(t *testing.T) {
	//lint:ignore SA1012 deliberate nil to verify nil-safety guarantee
	if l := FromContext(nil); l == nil {
		t.Fatal("FromContext(nil) returned nil logger")
	}
}

// TestFromContext_AttachesAttrs — context values flow into structured fields
// in the JSON output.
func TestFromContext_AttachesAttrs(t *testing.T) {
	var buf bytes.Buffer
	prev := defaultLogger
	t.Cleanup(func() {
		defaultMu.Lock()
		defaultLogger = prev
		defaultMu.Unlock()
	})
	defaultMu.Lock()
	defaultLogger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defaultMu.Unlock()

	ctx := WithTraceID(context.Background(), "abc123")
	ctx = WithAgent(ctx, "alice")
	ctx = WithSession(ctx, "ses-1")
	FromContext(ctx).Info("hello", "k", "v")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	for _, want := range []string{"trace_id", "agent_id", "session_id"} {
		if _, ok := entry[want]; !ok {
			t.Errorf("missing %s in %v", want, entry)
		}
	}
}

// TestNewTraceID_LooksRandom — generated IDs are non-empty and not all the
// same, sufficient evidence that crypto/rand path works.
func TestNewTraceID_LooksRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		id := NewTraceID()
		if id == "" {
			t.Fatal("empty id")
		}
		seen[id] = true
	}
	if len(seen) < 8 {
		t.Fatalf("only %d unique ids in 16 — randomness suspect", len(seen))
	}
}

// TestTraceMiddleware_GeneratesAndEchoes — when no inbound header, generate
// one and echo; when present, reuse.
func TestTraceMiddleware_GeneratesAndEchoes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceMiddleware())
	r.GET("/x", func(c *gin.Context) {
		// echo the trace_id from ctx into the response body so we can assert
		// it survived the middleware.
		c.String(200, TraceID(c.Request.Context()))
	})

	// 1) no inbound — generates one
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	r.ServeHTTP(w, req)
	echoed := w.Body.String()
	if echoed == "" {
		t.Fatalf("middleware did not generate trace_id")
	}
	if w.Header().Get(HeaderTraceID) != echoed {
		t.Fatalf("response header %q != body %q", w.Header().Get(HeaderTraceID), echoed)
	}

	// 2) inbound header — reuses
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/x", nil)
	req2.Header.Set(HeaderTraceID, "external-abc")
	r.ServeHTTP(w2, req2)
	if w2.Body.String() != "external-abc" {
		t.Fatalf("expected external-abc, got %q", w2.Body.String())
	}
	if !strings.EqualFold(w2.Header().Get(HeaderTraceID), "external-abc") {
		t.Fatalf("response should echo external trace_id")
	}
}
