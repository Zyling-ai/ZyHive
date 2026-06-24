// Package agentcli implements the agent-facing business CLI for ZyHive.
//
// Unlike cmd/aipanel/cli.go (the human ops panel: start/stop/nginx/ssl/backup),
// this package turns the full REST API into a machine-friendly command tree so
// that AI agents (internal members via exec, or external agents / scripts) can
// drive the whole system: manage members, cron, goals, memory, network, etc.
//
// Design: a thin HTTP client over the existing REST API. It reuses server-side
// auth / audit / runtime side effects (cron engine, bot pool, worker pool)
// rather than touching the data layer directly.
package agentcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/config"
)

// Client is a thin HTTP client for the ZyHive REST API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// APIError represents a non-2xx HTTP response, carrying a CLI exit code.
type APIError struct {
	StatusCode int
	Message    string // server-provided error message (best effort)
	Body       string // raw body (fallback)
	ExitCode   int
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Body
	}
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, msg)
}

// connError wraps a transport-level failure (server not running, DNS, etc.).
type connError struct{ err error }

func (e *connError) Error() string {
	return fmt.Sprintf("无法连接到 ZyHive 服务: %v\n  提示: 确认服务已启动 (zyhive status), 或用 --host / ZYHIVE_HOST 指定地址", e.err)
}
func (e *connError) Unwrap() error { return e.err }

// resolveClient builds a Client from the global options, applying the
// precedence: flag > env > config file > default.
func resolveClient(opts *globalOpts) (*Client, error) {
	baseURL := opts.Host
	token := opts.Token

	if baseURL == "" {
		baseURL = os.Getenv("ZYHIVE_HOST")
	}
	if token == "" {
		token = os.Getenv("ZYHIVE_TOKEN")
	}

	// Fall back to the local config file for anything still unset.
	if baseURL == "" || token == "" {
		cfgPath := opts.Config
		if cfgPath == "" {
			cfgPath = os.Getenv("AIPANEL_CONFIG")
		}
		if cfgPath == "" {
			cfgPath = findConfigPath()
		}
		if cfg, err := config.Load(cfgPath); err == nil {
			if baseURL == "" {
				baseURL = cfg.Gateway.BaseURL()
			}
			if token == "" {
				token = cfg.Auth.Token
			}
		}
	}

	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			// No global timeout: SSE chat streams stay open for minutes.
			// Per-request timeouts are applied via context where needed.
			Timeout: 0,
		},
	}, nil
}

// findConfigPath mirrors cmd/aipanel/cli.go's lookup order so that the business
// CLI resolves the same config the running service uses.
func findConfigPath() string {
	candidates := []string{
		os.Getenv("AIPANEL_CONFIG"),
		"/etc/zyhive/zyhive.json",
		"/usr/local/etc/zyhive/zyhive.json",
		os.ExpandEnv("$HOME/.config/zyhive/zyhive.json"),
		"aipanel.json",
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/etc/zyhive/zyhive.json"
}

// newRequest builds an authenticated request. body may be nil, []byte, or any
// JSON-marshalable value.
func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		switch b := body.(type) {
		case []byte:
			rdr = bytes.NewReader(b)
		case string:
			rdr = strings.NewReader(b)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			rdr = bytes.NewReader(data)
		}
	}
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "zyhive-cli")
	return req, nil
}

// do performs a request and returns (statusCode, body). Transport failures are
// wrapped in *connError (exit code 5). Non-2xx responses are returned to the
// caller as a successful call with the status + body so callers can decide.
func (c *Client) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, &connError{err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, data, nil
}

// Request performs a request and returns the body on 2xx, or an *APIError
// (with the right exit code) on a non-2xx status.
func (c *Client) Request(ctx context.Context, method, path string, body any) ([]byte, error) {
	status, data, err := c.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, &APIError{
			StatusCode: status,
			Message:    extractErrMsg(data),
			Body:       string(data),
			ExitCode:   exitCodeForStatus(status),
		}
	}
	return data, nil
}

// extractErrMsg pulls the {"error": "..."} field most ZyHive handlers return.
func extractErrMsg(data []byte) string {
	var m struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &m) == nil && m.Error != "" {
		return m.Error
	}
	return ""
}

// exitCodeForStatus maps an HTTP status to a CLI exit code (see output.go).
func exitCodeForStatus(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ExitAuth
	case http.StatusNotFound:
		return ExitNotFound
	default:
		return ExitError
	}
}

// SSEEvent is a decoded `data:` line from a Server-Sent Events stream.
type SSEEvent map[string]any

// StreamSSE POSTs a request and invokes onEvent for each decoded SSE `data:`
// line until the stream closes or onEvent returns an error.
func (c *Client) StreamSSE(ctx context.Context, method, path string, body any, onEvent func(SSEEvent) error) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return &connError{err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    extractErrMsg(data),
			Body:       string(data),
			ExitCode:   exitCodeForStatus(resp.StatusCode),
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE lines can be large (e.g. a big tool_result), so grow the buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // skip comments (": keepalive") and blank lines
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" {
			continue
		}
		var ev SSEEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue // tolerate malformed lines
		}
		if err := onEvent(ev); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		// A closed stream after `done` is normal; surface other read errors.
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return fmt.Errorf("SSE read timeout: %w", err)
		}
		return err
	}
	return nil
}

// shortTimeout returns a context with a sensible per-request deadline for
// non-streaming calls.
func shortTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 60*time.Second)
}
