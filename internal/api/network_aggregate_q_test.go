package api

import (
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/network"
)

// TestAggregateContactsQ_MultiByteSubstring — the q filter does case-folded
// substring match; ensure it correctly handles UTF-8 Chinese, emoji, and
// mixed-script names without breaking on the byte/rune boundary.
func TestAggregateContactsQ_MultiByteSubstring(t *testing.T) {
	root := t.TempDir()
	ag := &agent.Agent{ID: "a", Name: "A", WorkspaceDir: filepath.Join(root, "a", "workspace")}
	s := network.NewStore(ag.WorkspaceDir)
	// Seed contacts with varied names.
	names := map[string]string{
		"feishu:ou_zhang": "张三 (产品)",
		"feishu:ou_wang":  "王五👑 老板",
		"feishu:ou_zhou":  "Zhōu 周",
		"feishu:ou_li":    "Lily Lee",
	}
	for id, name := range names {
		_, ext, _ := splitID(id)
		if _, err := s.Resolve("feishu", ext, name); err != nil {
			t.Fatal(err)
		}
	}

	rows := aggregateContacts([]*agent.Agent{ag}, nil)
	if len(rows) != 4 {
		t.Fatalf("seed expected 4 rows, got %d", len(rows))
	}

	cases := []struct {
		q       string
		wantIDs []string
	}{
		{"张三", []string{"feishu:ou_zhang"}},
		{"老板", []string{"feishu:ou_wang"}},
		{"产品", []string{"feishu:ou_zhang"}},
		{"👑", []string{"feishu:ou_wang"}},
		{"周", []string{"feishu:ou_zhou"}},
		{"LILY", []string{"feishu:ou_li"}}, // case-insensitive
		{"li", []string{"feishu:ou_li"}},   // English substring
		{"NOPE", nil},
	}

	for _, c := range cases {
		// Verify the q is valid UTF-8 (sanity).
		if !utf8.ValidString(c.q) {
			t.Fatalf("bad utf-8 q: %q", c.q)
		}
		matched := []string{}
		for _, r := range rows {
			if matchContactQ(r, lowerString(c.q)) {
				matched = append(matched, r.ID)
			}
		}
		if !sameSet(matched, c.wantIDs) {
			t.Errorf("q=%q: got %v want %v", c.q, matched, c.wantIDs)
		}
	}
}

// TestAggregateContactsQ_ByteSafeOnInvalidUTF8 — feeding malformed UTF-8 to
// the filter must not panic; we accept whatever match (or none) the stdlib
// case-folder produces.
func TestAggregateContactsQ_DoesNotPanicOnGarbage(t *testing.T) {
	r := AggregatedContact{
		ID: "x:y", DisplayName: "test", Tags: []string{"foo"},
		PerAgent: []ContactPerAgent{{DisplayName: "test"}},
	}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("matchContactQ panicked: %v", rec)
		}
	}()
	_ = matchContactQ(r, string([]byte{0xff, 0xfe, 0xfd}))
	_ = matchContactQ(r, "")
}

// ── helpers ───────────────────────────────────────────────────────────────

func splitID(id string) (source, ext string, ok bool) {
	for i, r := range id {
		if r == ':' {
			return id[:i], id[i+1:], true
		}
	}
	return "", "", false
}

func lowerString(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		out = appendRune(out, r)
	}
	return string(out)
}

func appendRune(b []byte, r rune) []byte {
	const (
		surrogateMin = 0xD800
		surrogateMax = 0xDFFF
	)
	switch {
	case r < 0:
		return b
	case r < 0x80:
		return append(b, byte(r))
	case r < 0x800:
		return append(b, byte(0xC0|r>>6), byte(0x80|r&0x3F))
	case r >= surrogateMin && r <= surrogateMax:
		return b
	case r < 0x10000:
		return append(b, byte(0xE0|r>>12), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	default:
		return append(b, byte(0xF0|r>>18), byte(0x80|(r>>12)&0x3F), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int)
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		if m[x] == 0 {
			return false
		}
		m[x]--
	}
	return true
}
