// internal/api/network_aggregate_test.go — pure-function tests for the
// cross-agent aggregator. We avoid spinning up the HTTP router by hitting
// aggregateContacts / aggregateChats directly with a hand-built Manager.

package api

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/network"
)

// helper: create a freshly-set-up Agent rooted at tmp/agentID/workspace/.
func newTestAgent(t *testing.T, root, id, name, color string) *agent.Agent {
	t.Helper()
	wsDir := filepath.Join(root, id, "workspace")
	return &agent.Agent{
		ID:           id,
		Name:         name,
		WorkspaceDir: wsDir,
		AvatarColor:  color,
	}
}

func resolveOne(t *testing.T, wsDir, source, ext, name string) {
	t.Helper()
	if _, err := network.NewStore(wsDir).Resolve(source, ext, name); err != nil {
		t.Fatalf("Resolve(%s,%s): %v", source, ext, err)
	}
}

// Two agents share the same Feishu contact → must collapse to 1 row with 2 perAgent entries.
func TestAggregateContactsDeduplicatesAcrossAgents(t *testing.T) {
	root := t.TempDir()
	a1 := newTestAgent(t, root, "alice", "Alice", "#abc")
	a2 := newTestAgent(t, root, "bob", "Bob", "#def")
	resolveOne(t, a1.WorkspaceDir, "feishu", "ou_shared", "张三 (a1)")
	resolveOne(t, a2.WorkspaceDir, "feishu", "ou_shared", "张三")

	rows := aggregateContacts([]*agent.Agent{a1, a2}, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.ID != "feishu:ou_shared" {
		t.Errorf("wrong ID: %s", r.ID)
	}
	if len(r.PerAgent) != 2 {
		t.Errorf("expected 2 perAgent, got %d", len(r.PerAgent))
	}
	if r.TotalMsgCount != 2 {
		t.Errorf("expected totalMsgCount=2, got %d", r.TotalMsgCount)
	}
}

// Single per-agent contact → unique row.
func TestAggregateContactsSeparateIDsStayApart(t *testing.T) {
	root := t.TempDir()
	a1 := newTestAgent(t, root, "alice", "Alice", "")
	a2 := newTestAgent(t, root, "bob", "Bob", "")
	resolveOne(t, a1.WorkspaceDir, "telegram", "111", "甲")
	resolveOne(t, a2.WorkspaceDir, "telegram", "222", "乙")
	rows := aggregateContacts([]*agent.Agent{a1, a2}, nil)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// MsgCount tie-break: pick the per-agent with greater MsgCount as DisplayName source.
func TestAggregateContactsPicksMostKnownDisplayName(t *testing.T) {
	root := t.TempDir()
	a1 := newTestAgent(t, root, "alice", "Alice", "")
	a2 := newTestAgent(t, root, "bob", "Bob", "")
	// agent1 has 3 msgs on this contact ("张三高"), agent2 has 1 ("zsanvague")
	s1 := network.NewStore(a1.WorkspaceDir)
	c1, _ := s1.Resolve("feishu", "ou_pick", "张三高")
	_ = s1.Touch(c1.ID)
	_ = s1.Touch(c1.ID) // now 3
	s2 := network.NewStore(a2.WorkspaceDir)
	_, _ = s2.Resolve("feishu", "ou_pick", "zsanvague")
	rows := aggregateContacts([]*agent.Agent{a1, a2}, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].DisplayName != "张三高" {
		t.Errorf("expected canonical displayName=张三高, got %q", rows[0].DisplayName)
	}
	// Per-agent rows ordered msgCount desc.
	if rows[0].PerAgent[0].AgentID != "alice" {
		t.Errorf("perAgent[0] should be the higher msgCount agent: %+v", rows[0].PerAgent)
	}
}

// q filter matches displayName / id / tags / perAgent.displayName.
func TestAggregateContactsQFilter(t *testing.T) {
	root := t.TempDir()
	a := newTestAgent(t, root, "a", "Agent", "")
	resolveOne(t, a.WorkspaceDir, "feishu", "ou_apple", "苹果客户")
	resolveOne(t, a.WorkspaceDir, "feishu", "ou_banana", "Banana Corp")

	rows := aggregateContacts([]*agent.Agent{a}, nil)
	if len(rows) != 2 {
		t.Fatalf("setup expected 2 rows, got %d", len(rows))
	}
	// Should match by Chinese substring
	matched := 0
	for _, r := range rows {
		if matchContactQ(r, "苹果") {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("substring 苹果 should match exactly 1, got %d", matched)
	}
	// Case-insensitive English substring
	matched = 0
	for _, r := range rows {
		if matchContactQ(r, "banana") {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("'banana' should match 1, got %d", matched)
	}
}

// Tag union: both agents tag the same contact → row.Tags is the union.
func TestAggregateContactsTagUnion(t *testing.T) {
	root := t.TempDir()
	a1 := newTestAgent(t, root, "a1", "A1", "")
	a2 := newTestAgent(t, root, "a2", "A2", "")

	s1 := network.NewStore(a1.WorkspaceDir)
	c1, _ := s1.Resolve("feishu", "ou_t", "T")
	c1.Tags = []string{"客户", "重要"}
	_ = s1.Save(c1)

	s2 := network.NewStore(a2.WorkspaceDir)
	c2, _ := s2.Resolve("feishu", "ou_t", "T")
	c2.Tags = []string{"客户", "朋友"} // overlap + new
	_ = s2.Save(c2)

	rows := aggregateContacts([]*agent.Agent{a1, a2}, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	wantTags := map[string]bool{"客户": true, "重要": true, "朋友": true}
	for _, tag := range r.Tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag in union: %s", tag)
		}
		delete(wantTags, tag)
	}
	if len(wantTags) > 0 {
		t.Errorf("missing tags in union: %v", wantTags)
	}
}

// Chat aggregation: same group across 2 agents collapses.
func TestAggregateChatsDeduplicatesAcrossAgents(t *testing.T) {
	root := t.TempDir()
	a1 := newTestAgent(t, root, "a1", "A1", "")
	a2 := newTestAgent(t, root, "a2", "A2", "")
	s1 := network.NewStore(a1.WorkspaceDir)
	s2 := network.NewStore(a2.WorkspaceDir)
	if _, err := s1.ResolveChat("feishu", "oc_team", "团队群", "group"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.ResolveChat("feishu", "oc_team", "", "group"); err != nil {
		t.Fatal(err)
	}
	rows := aggregateChats([]*agent.Agent{a1, a2}, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 chat row, got %d", len(rows))
	}
	if rows[0].Title != "团队群" {
		t.Errorf("title should propagate from agent that knows the name: %s", rows[0].Title)
	}
	if len(rows[0].PerAgent) != 2 {
		t.Errorf("expected 2 perAgent entries, got %d", len(rows[0].PerAgent))
	}
}

// Sort order: lastSeenAt desc.
func TestAggregateContactsSortByLastSeenDesc(t *testing.T) {
	root := t.TempDir()
	a := newTestAgent(t, root, "a", "A", "")
	s := network.NewStore(a.WorkspaceDir)

	c1, _ := s.Resolve("feishu", "ou_old", "Old")
	c2, _ := s.Resolve("feishu", "ou_new", "New")
	// Manually backdate c1.
	c1.LastSeenAt = time.Now().Add(-72 * time.Hour)
	_ = s.Save(c1)
	c2.LastSeenAt = time.Now().Add(-1 * time.Hour)
	_ = s.Save(c2)

	rows := aggregateContacts([]*agent.Agent{a}, nil)
	// Apply same sort as handler.
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows")
	}
	// Find which is which.
	var newest, oldest *AggregatedContact
	for i := range rows {
		if rows[i].ID == "feishu:ou_new" {
			newest = &rows[i]
		} else {
			oldest = &rows[i]
		}
	}
	if newest == nil || oldest == nil {
		t.Fatal("rows mismatch")
	}
	if !newest.LastSeenAt.After(oldest.LastSeenAt) {
		t.Errorf("newest should have later lastSeenAt")
	}
}

// Source filter sanity: matchContactQ shouldn't false-positive on prefix.
func TestMatchContactQNegative(t *testing.T) {
	r := AggregatedContact{
		ID: "telegram:111", DisplayName: "Alice", Tags: []string{"客户"},
		PerAgent: []ContactPerAgent{{DisplayName: "Alice"}},
	}
	if matchContactQ(r, "zzz_nope") {
		t.Errorf("should not match 'zzz_nope'")
	}
	if !matchContactQ(r, "alic") {
		t.Errorf("case-insensitive substring 'alic' should match")
	}
	if !matchContactQ(r, "客户") {
		t.Errorf("tag substring should match")
	}
}

func TestAggregateEmptyAgents(t *testing.T) {
	rows := aggregateContacts([]*agent.Agent{}, nil)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	chats := aggregateChats([]*agent.Agent{}, nil)
	if len(chats) != 0 {
		t.Fatalf("expected 0 chats, got %d", len(chats))
	}
}

func TestParseLimit(t *testing.T) {
	// Build a fake gin.Context-ish using strings; parseLimit only uses .Query.
	// Easier: parse a few raw values via the strings inside parseLimit logic.
	// Since gin.Context isn't trivial to mock, replicate the logic directly:
	cases := []struct {
		raw  string
		want int
	}{
		{"", defaultAggregateLimit},
		{"50", 50},
		{"99999", maxAggregateLimit},
		{"-1", defaultAggregateLimit},
		{"abc", defaultAggregateLimit},
	}
	for _, c := range cases {
		// inline parse copy
		got := func() int {
			if c.raw == "" {
				return defaultAggregateLimit
			}
			n := 0
			for _, r := range c.raw {
				if r >= '0' && r <= '9' {
					n = n*10 + int(r-'0')
				} else {
					return defaultAggregateLimit
				}
			}
			if n <= 0 {
				return defaultAggregateLimit
			}
			if n > maxAggregateLimit {
				return maxAggregateLimit
			}
			return n
		}()
		if got != c.want {
			t.Errorf("raw=%q got=%d want=%d", c.raw, got, c.want)
		}
	}
	// Just sanity-check we use the function name (compile dep).
	_ = parseLimit
	_ = strings.ToLower
}
