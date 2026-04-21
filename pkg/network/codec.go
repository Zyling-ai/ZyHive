package network

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ── Markdown (frontmatter + body) codec ────────────────────────────────────
//
// We use a lightweight, hand-rolled YAML-ish frontmatter to avoid pulling in a
// YAML library. Only string / []string / bool / time.Time / int values — no
// nested objects. This is stable, human-readable, and safe to round-trip.

var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?(.*)$`)

// parseContactMD splits a markdown file into frontmatter + body and loads them
// into a Contact. If the file has no frontmatter, the whole content becomes Body
// and only ID/DisplayName from hint are used.
func parseContactMD(raw string, hintID string) *Contact {
	c := &Contact{ID: hintID, Primary: true}
	m := frontmatterRe.FindStringSubmatch(raw)
	if m == nil {
		c.Body = strings.TrimSpace(raw)
		return c
	}
	fmBlock := m[1]
	body := strings.TrimSpace(m[2])
	c.Body = body

	for _, line := range strings.Split(fmBlock, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Array continuation ("  - item")
		if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "- ") {
			// handled inside key parsing below (we buffer)
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		assignField(c, key, val)
	}
	// Second pass for arrays (tags / aliases) — need to collect multi-line blocks.
	c.Tags = collectArrayField(fmBlock, "tags")
	c.Aliases = collectArrayField(fmBlock, "aliases")
	return c
}

func assignField(c *Contact, key, val string) {
	switch key {
	case "id":
		c.ID = val
	case "source":
		c.Source = val
	case "externalId":
		c.ExternalID = val
	case "displayName":
		c.DisplayName = stripQuotes(val)
	case "primary":
		c.Primary = parseBool(val, true)
	case "isOwner":
		c.IsOwner = parseBool(val, false)
	case "msgCount":
		var n int
		_, _ = fmt.Sscanf(val, "%d", &n)
		c.MsgCount = n
	case "createdAt":
		t, err := time.Parse(time.RFC3339, val)
		if err == nil {
			c.CreatedAt = t
		}
	case "lastSeenAt":
		t, err := time.Parse(time.RFC3339, val)
		if err == nil {
			c.LastSeenAt = t
		}
	}
}

// collectArrayField extracts a YAML-style array: either inline "tags: [a, b]" or
// block style with "tags:" followed by "  - a" lines.
func collectArrayField(fmBlock, key string) []string {
	var out []string
	lines := strings.Split(fmBlock, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, key+":") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
		// Inline "[a, b, c]"
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			inner := strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]")
			for _, item := range strings.Split(inner, ",") {
				it := stripQuotes(strings.TrimSpace(item))
				if it != "" {
					out = append(out, it)
				}
			}
			return out
		}
		if rest != "" && !strings.HasPrefix(rest, "[") {
			// Inline scalar, treat as single-item list? Prefer empty.
			break
		}
		// Block style
		for j := i + 1; j < len(lines); j++ {
			l := strings.TrimRight(lines[j], "\r")
			if strings.HasPrefix(l, "  - ") {
				out = append(out, stripQuotes(strings.TrimSpace(strings.TrimPrefix(l, "  - "))))
				continue
			}
			if strings.HasPrefix(l, "- ") {
				out = append(out, stripQuotes(strings.TrimSpace(strings.TrimPrefix(l, "- "))))
				continue
			}
			break
		}
		return out
	}
	return out
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseBool(val string, def bool) bool {
	switch strings.ToLower(val) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	return def
}

// renderContactMD serializes a Contact to the frontmatter+body markdown.
func renderContactMD(c *Contact) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", c.ID))
	sb.WriteString(fmt.Sprintf("source: %s\n", c.Source))
	sb.WriteString(fmt.Sprintf("externalId: %s\n", c.ExternalID))
	sb.WriteString(fmt.Sprintf("displayName: %s\n", quoteIfNeeded(c.DisplayName)))
	if len(c.Tags) > 0 {
		sort.Strings(c.Tags)
		sb.WriteString("tags:\n")
		for _, t := range c.Tags {
			sb.WriteString("  - " + quoteIfNeeded(t) + "\n")
		}
	} else {
		sb.WriteString("tags: []\n")
	}
	if len(c.Aliases) > 0 {
		sb.WriteString("aliases:\n")
		for _, a := range c.Aliases {
			sb.WriteString("  - " + a + "\n")
		}
	} else {
		sb.WriteString("aliases: []\n")
	}
	sb.WriteString(fmt.Sprintf("primary: %v\n", c.Primary))
	if c.IsOwner {
		sb.WriteString("isOwner: true\n")
	}
	if !c.CreatedAt.IsZero() {
		sb.WriteString("createdAt: " + c.CreatedAt.Format(time.RFC3339) + "\n")
	}
	if !c.LastSeenAt.IsZero() {
		sb.WriteString("lastSeenAt: " + c.LastSeenAt.Format(time.RFC3339) + "\n")
	}
	sb.WriteString(fmt.Sprintf("msgCount: %d\n", c.MsgCount))
	sb.WriteString("---\n\n")

	body := strings.TrimSpace(c.Body)
	if body == "" {
		body = defaultContactBody(c.DisplayName)
	}
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

// quoteIfNeeded wraps a string in quotes if it contains special YAML-ish chars.
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#[]{},\"'\n") {
		// Escape existing quotes
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// defaultContactBody returns the initial markdown sections for a new contact.
func defaultContactBody(name string) string {
	if name == "" {
		name = "联系人"
	}
	return fmt.Sprintf(`# %s

## 事实
- (AI 通过 network_note 工具追加此处)

## 偏好（AI 观察）
-

## 最近话题
-

## 待跟进
-
`, name)
}
