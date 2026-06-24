package agentcli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// parseCSV splits a comma-separated flag into a string slice.
func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// bodyFromData returns raw JSON from --data when present, otherwise marshals
// the fallback body. It accepts --data - to read JSON from stdin.
func bodyFromData(data string, fallback map[string]any) (any, error) {
	if data == "" {
		if fallback == nil {
			fallback = map[string]any{}
		}
		return fallback, nil
	}
	raw, err := stdinIfDash(data)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, usageErr("--data 不能为空")
	}
	if !json.Valid([]byte(raw)) {
		return nil, usageErr("--data 不是合法 JSON")
	}
	return []byte(raw), nil
}

func addIf(body map[string]any, key string, val string) {
	if val != "" {
		body[key] = val
	}
}

func addIfSlice(body map[string]any, key string, vals []string) {
	if len(vals) > 0 {
		body[key] = vals
	}
}

func q(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		if v != "" {
			values.Set(k, v)
		}
	}
	if enc := values.Encode(); enc != "" {
		return "?" + enc
	}
	return ""
}

func idPath(base, id string) (string, error) {
	if id == "" {
		return "", usageErr("缺少资源 ID")
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), url.PathEscape(id)), nil
}

func slashPath(parts ...string) string {
	var cleaned []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return "/" + strings.Join(cleaned, "/")
}

func renderRows(c *ctx, v any, keys []string, headers []string, listKeys ...string) {
	rows := [][]string{}
	for _, item := range asSlice(v, listKeys...) {
		m := asMap(item)
		row := make([]string, 0, len(keys))
		for _, k := range keys {
			row = append(row, str(m, k))
		}
		rows = append(rows, row)
	}
	table(c.out, headers, rows)
}
