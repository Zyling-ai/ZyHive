package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreResolveAndList(t *testing.T) {
	tmp, err := os.MkdirTemp("", "network-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s := NewStore(tmp)

	// First resolve creates contact.
	c1, err := s.Resolve("feishu", "ou_abc", "张三")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if c1.ID != "feishu:ou_abc" {
		t.Fatalf("expected id feishu:ou_abc, got %s", c1.ID)
	}
	if c1.MsgCount != 1 {
		t.Fatalf("expected MsgCount=1, got %d", c1.MsgCount)
	}
	if !c1.Primary {
		t.Fatalf("expected primary=true for new contact")
	}

	// Second resolve bumps count.
	c2, err := s.Resolve("feishu", "ou_abc", "张三")
	if err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if c2.MsgCount != 2 {
		t.Fatalf("expected MsgCount=2 after second resolve, got %d", c2.MsgCount)
	}

	// Second contact from different source.
	_, err = s.Resolve("telegram", "123456", "Lilian")
	if err != nil {
		t.Fatalf("resolve tg: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(list))
	}

	// INDEX.md exists and contains both names.
	idx, err := os.ReadFile(filepath.Join(tmp, "network", "INDEX.md"))
	if err != nil {
		t.Fatalf("read INDEX.md: %v", err)
	}
	idxStr := string(idx)
	if !strings.Contains(idxStr, "张三") || !strings.Contains(idxStr, "Lilian") {
		t.Fatalf("INDEX.md missing names:\n%s", idxStr)
	}
}

func TestRoundtripMarkdown(t *testing.T) {
	tmp, err := os.MkdirTemp("", "network-roundtrip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s := NewStore(tmp)
	c, err := s.Resolve("feishu", "ou_xyz", "李四")
	if err != nil {
		t.Fatal(err)
	}
	c.Tags = []string{"客户", "合作伙伴"}
	c.Body = "# 李四\n\n## 事实\n- 公司 A 法务\n- 在深圳\n\n## 偏好（AI 观察）\n- 简短直给\n"
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(c.ID)
	if err != nil || got == nil {
		t.Fatalf("get back: err=%v nil=%v", err, got == nil)
	}
	if got.DisplayName != "李四" {
		t.Fatalf("lost displayName: %q", got.DisplayName)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "合作伙伴" && got.Tags[0] != "客户" {
		t.Fatalf("tags lost: %#v", got.Tags)
	}
	if !strings.Contains(got.Body, "公司 A 法务") {
		t.Fatalf("body lost facts:\n%s", got.Body)
	}
}

func TestSummaryExtraction(t *testing.T) {
	tmp, err := os.MkdirTemp("", "network-summary-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s := NewStore(tmp)
	c, err := s.Resolve("feishu", "ou_a", "王五")
	if err != nil {
		t.Fatal(err)
	}
	c.Tags = []string{"家人"}
	c.Body = "# 王五\n\n## 事实\n- 公司合伙人\n- 爱喝茶\n- 在上海\n- 周末打球\n\n## 偏好（AI 观察）\n- 直给不废话\n"
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	sum := s.Summary(c.ID)
	if !strings.Contains(sum, "王五") || !strings.Contains(sum, "公司合伙人") {
		t.Fatalf("summary missing content:\n%s", sum)
	}
	if !strings.Contains(sum, "家人") {
		t.Fatalf("summary missing tag:\n%s", sum)
	}
	// Should only include 3 facts (max).
	if strings.Count(sum, "周末打球") != 0 {
		t.Fatalf("summary should cap facts at 3:\n%s", sum)
	}
}

func TestResolveRoutesThroughAliases(t *testing.T) {
	// Bug 3 regression: after manual merge, a later inbound message from the
	// alias ID must bump the primary's MsgCount (not recreate an orphan).
	tmp, err := os.MkdirTemp("", "network-alias-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s := NewStore(tmp)

	// Seed: two contacts for the same real person, plus a merge.
	primary, err := s.Resolve("feishu", "ou_boss", "老板")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Resolve("telegram", "555", "Boss (TG)")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate MergeContact: record the alias on primary + delete the alias file.
	primary.Aliases = []string{"telegram:555"}
	primary.MsgCount += 1 // absorb alias msgCount
	if err := s.Save(primary); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("telegram:555"); err != nil {
		t.Fatal(err)
	}
	primaryBefore, _ := s.Get(primary.ID)
	if primaryBefore == nil {
		t.Fatal("primary missing after merge")
	}
	countBefore := primaryBefore.MsgCount

	// Alias user sends a new message → Resolve should route to primary.
	got, err := s.Resolve("telegram", "555", "Boss (TG)")
	if err != nil {
		t.Fatalf("resolve after merge: %v", err)
	}
	if got.ID != primary.ID {
		t.Fatalf("expected alias to route to primary %s, got %s", primary.ID, got.ID)
	}
	if got.MsgCount != countBefore+1 {
		t.Fatalf("expected primary MsgCount to increment, got %d (was %d)",
			got.MsgCount, countBefore)
	}
	// Critical: no orphan alias file should be recreated.
	orphan, err := s.Get("telegram:555")
	if err != nil {
		t.Fatalf("get orphan: %v", err)
	}
	if orphan != nil {
		t.Fatalf("alias file should not have been recreated, got:\n%+v", orphan)
	}
}

func TestFallbackDisplayName(t *testing.T) {
	// Bug 2 regression: displayName fallback chain never returns empty.
	cases := []struct {
		name       string
		externalID string
		candidates []string
		want       string
	}{
		{"first non-empty wins", "ou_abc", []string{"张三", "zhang3"}, "张三"},
		{"skip empty", "ou_abc", []string{"", "", "fallback"}, "fallback"},
		{"whitespace treated as empty", "ou_abc", []string{"  ", "real"}, "real"},
		{"all empty → short externalID prefix", "ou_abcdefghij", []string{"", ""}, "ou_abcde"},
		{"all empty short ID returned whole", "a1", []string{""}, "a1"},
		{"no candidates → short prefix", "longtoken12345", nil, "longtoke"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FallbackDisplayName(tc.externalID, tc.candidates...)
			if got != tc.want {
				t.Fatalf("FallbackDisplayName(%q, %v) = %q, want %q",
					tc.externalID, tc.candidates, got, tc.want)
			}
		})
	}
}

func TestSummaryIsOwnerSkips(t *testing.T) {
	// Bug 1 regression: IsOwner=true → Summary() should return empty to avoid
	// duplicating owner-profile.md injection.
	tmp, err := os.MkdirTemp("", "network-owner-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s := NewStore(tmp)
	c, err := s.Resolve("feishu", "ou_owner", "老板")
	if err != nil {
		t.Fatal(err)
	}
	c.IsOwner = true
	c.Body = "# 老板\n\n## 事实\n- 主人本人\n"
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	sum := s.Summary(c.ID)
	if sum != "" {
		t.Fatalf("IsOwner=true should produce empty summary, got:\n%s", sum)
	}

	// Sanity: after clearing IsOwner, Summary should produce content again.
	c.IsOwner = false
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	sum = s.Summary(c.ID)
	if !strings.Contains(sum, "老板") {
		t.Fatalf("after IsOwner=false, summary should contain name:\n%s", sum)
	}
}

func TestMigrateIfNeeded(t *testing.T) {
	tmp, err := os.MkdirTemp("", "network-mig-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Seed legacy files
	if err := os.MkdirAll(filepath.Join(tmp, "memory", "core"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "RELATIONS.md"), []byte("| to | type | strength | desc |\n| abao | 平级协作 | 常用 | 讨论 |\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "memory", "core", "user-profile.md"), []byte("# 我\n叫老张\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateIfNeeded(tmp); err != nil {
		t.Fatal(err)
	}

	// RELATIONS.md moved
	if _, err := os.Stat(filepath.Join(tmp, "network", "RELATIONS.md")); err != nil {
		t.Fatalf("expected network/RELATIONS.md: %v", err)
	}
	// user-profile renamed
	if _, err := os.Stat(filepath.Join(tmp, "memory", "core", "owner-profile.md")); err != nil {
		t.Fatalf("expected owner-profile.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "memory", "core", "user-profile.md")); !os.IsNotExist(err) {
		t.Fatalf("expected user-profile.md removed, err=%v", err)
	}

	// Idempotent: call again should not fail.
	if err := MigrateIfNeeded(tmp); err != nil {
		t.Fatalf("idempotent migration failed: %v", err)
	}
}
