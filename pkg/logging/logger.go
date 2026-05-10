// Package logging — structured (slog) logging facade for ZyHive.
//
// Goals:
//   - One initialization point (Init from main).
//   - Trace-id propagation through context for SSE / runner / tools / LLM.
//   - JSON output for production, text for local dev (env-controlled).
//   - log/slog (Go 1.21+ stdlib) as the underlying mechanism — no extra deps.
//
// Usage:
//
//	// At startup
//	logging.Init(os.Getenv("LOG_FORMAT"), os.Getenv("LOG_LEVEL"))
//	logging.Default().Info("server starting", "port", 8080)
//
//	// Inside a request handler
//	ctx = logging.WithAgent(ctx, agentID)
//	logging.FromContext(ctx).Info("turn start", "msg_len", n)
//
// Trace-id propagation: gin middleware (see middleware.go) attaches a
// trace_id to ctx; FromContext bakes it into the returned *slog.Logger as
// an "Attr". Downstream code does NOT need to remember to log it.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type ctxKey int

const (
	ctxKeyTraceID ctxKey = iota + 1
	ctxKeyAgentID
	ctxKeySessionID
)

var (
	defaultMu     sync.RWMutex
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))
)

// Init configures the package-level default logger. Idempotent.
//
//	format: "" | "text" | "json" — anything other than "json" means text.
//	level:  "" | "debug" | "info" | "warn" | "error" — empty means info.
func Init(format, level string) {
	lvl := parseLevel(level)
	h := buildHandler(os.Stderr, format, lvl)
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = slog.New(h)
	slog.SetDefault(defaultLogger)
}

// Default returns the package-level logger. Always non-nil.
func Default() *slog.Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// FromContext returns Default with any trace/agent/session attributes from
// ctx pre-attached. Always non-nil; safe for nil ctx.
func FromContext(ctx context.Context) *slog.Logger {
	l := Default()
	if ctx == nil {
		return l
	}
	if v := TraceID(ctx); v != "" {
		l = l.With("trace_id", v)
	}
	if v := AgentID(ctx); v != "" {
		l = l.With("agent_id", v)
	}
	if v := SessionID(ctx); v != "" {
		l = l.With("session_id", v)
	}
	return l
}

// WithTraceID returns a child context bound to trace_id.
func WithTraceID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyTraceID, id)
}

// WithAgent returns a child context bound to agent_id.
func WithAgent(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAgentID, id)
}

// WithSession returns a child context bound to session_id.
func WithSession(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeySessionID, id)
}

// TraceID extracts the trace_id from ctx, "" if absent.
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKeyTraceID).(string); ok {
		return v
	}
	return ""
}

// AgentID extracts the agent_id from ctx, "" if absent.
func AgentID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKeyAgentID).(string); ok {
		return v
	}
	return ""
}

// SessionID extracts the session_id from ctx, "" if absent.
func SessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKeySessionID).(string); ok {
		return v
	}
	return ""
}

// NewTraceID generates a random short trace identifier (16 hex = 8 bytes).
// Falls back to a deterministic-looking value when crypto/rand fails (the
// stdlib never has, but explicit fallback simplifies test reasoning).
func NewTraceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "trace-fallback"
	}
	return hex.EncodeToString(b[:])
}

// ── helpers ─────────────────────────────────────────────────────────────────

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	}
	return slog.LevelInfo
}

func buildHandler(w io.Writer, format string, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(format, "json") {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}
