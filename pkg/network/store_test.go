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
