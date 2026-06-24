package agentcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Exit codes — a stable contract so agents/scripts can branch on failures.
const (
	ExitOK       = 0 // success
	ExitError    = 1 // generic error
	ExitUsage    = 2 // bad usage / arguments
	ExitAuth     = 3 // authentication failure (401/403)
	ExitNotFound = 4 // resource not found (404)
	ExitConn     = 5 // connection failure (server not running)
)

// ctx carries per-invocation state through command handlers.
type ctx struct {
	client *Client
	json   bool // --json: emit raw machine-readable JSON
	quiet  bool // --quiet: suppress non-essential human chatter
	yes    bool // --yes: skip confirmation prompts (required for writes when non-TTY)
	out    io.Writer
	err    io.Writer
}

// exitErr is an error carrying an explicit exit code.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }

// usageErr signals an argument/usage problem (exit code 2).
func usageErr(format string, a ...any) error {
	return &exitErr{code: ExitUsage, msg: fmt.Sprintf(format, a...)}
}

// codeOf extracts the exit code an error should map to.
func codeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		return ee.code
	}
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.ExitCode
	}
	var ce *connError
	if errors.As(err, &ce) {
		return ExitConn
	}
	return ExitError
}

// ── output helpers ────────────────────────────────────────────────────────

// emitJSON writes a value as indented JSON.
func (c *ctx) emitJSON(v any) error {
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// emitRawJSON pretty-prints raw JSON bytes (falls back to verbatim).
func (c *ctx) emitRawJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		_, werr := c.out.Write(append(raw, '\n'))
		return werr
	}
	buf.WriteByte('\n')
	_, err := c.out.Write(buf.Bytes())
	return err
}

// result renders an API response: raw JSON in --json mode, otherwise the
// human-friendly renderer. If raw is not valid JSON, it's printed verbatim.
func (c *ctx) result(raw []byte, human func(v any)) error {
	if c.json {
		return c.emitRawJSON(raw)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		_, werr := fmt.Fprintln(c.out, string(bytes.TrimSpace(raw)))
		return werr
	}
	if human == nil {
		return c.emitJSON(v)
	}
	human(v)
	return nil
}

// ok prints a success line in human mode; in --json mode emits {"ok":true,...}.
func (c *ctx) ok(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if c.json {
		_ = c.emitJSON(map[string]any{"ok": true, "message": msg})
		return
	}
	if !c.quiet {
		fmt.Fprintln(c.out, "OK "+msg)
	}
}

// printf writes human-mode text (suppressed in --json mode).
func (c *ctx) printf(format string, a ...any) {
	if c.json {
		return
	}
	fmt.Fprintf(c.out, format, a...)
}

// ── small renderers reused across commands ────────────────────────────────

// asMap coerces an arbitrary decoded JSON value to a map.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// asSlice returns a []any from either a bare array or {key: [...]}.
// It tries the given keys in order before giving up.
func asSlice(v any, keys ...string) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range keys {
			if s, ok := m[k].([]any); ok {
				return s
			}
		}
	}
	return nil
}

// str safely reads a string field.
func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// table prints rows aligned by the widest cell per column.
func table(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len([]rune(h))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := len([]rune(cell)); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}
	printRow := func(cells []string) {
		var sb strings.Builder
		for i, cell := range cells {
			if i > 0 {
				sb.WriteString("  ")
			}
			pad := widths[i] - len([]rune(cell))
			sb.WriteString(cell)
			if i < len(cells)-1 && pad > 0 {
				sb.WriteString(strings.Repeat(" ", pad))
			}
		}
		fmt.Fprintln(w, sb.String())
	}
	if len(headers) > 0 {
		printRow(headers)
	}
	for _, row := range rows {
		printRow(row)
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "(空)")
	}
}

// sortedKeys returns the map keys sorted, for stable output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fail writes an error to stderr (JSON-aware) and returns its exit code.
func fail(c *ctx, err error) int {
	code := codeOf(err)
	if c.json {
		_ = json.NewEncoder(c.err).Encode(map[string]any{
			"ok":    false,
			"error": err.Error(),
			"code":  code,
		})
	} else {
		fmt.Fprintln(c.err, "错误: "+err.Error())
	}
	return code
}

// stdinIfDash reads all of stdin when val == "-", else returns val unchanged.
func stdinIfDash(val string) (string, error) {
	if val != "-" {
		return val, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("读取 stdin 失败: %w", err)
	}
	return string(data), nil
}
