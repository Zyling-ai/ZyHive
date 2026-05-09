package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreResolveChatAndList(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	// First resolve creates chat.
	c1, err := s.ResolveChat("feishu", "oc_abc", "产品讨论群", "group")
	if err != nil {
		t.Fatalf("resolve chat: %v", err)
	}
	if c1.ID != "feishu:oc_abc" {
		t.Fatalf("expected id feishu:oc_abc, got %s", c1.ID)
	}
	if c1.MsgCount != 1 {
		t.Fatalf("expected MsgCount=1, got %d", c1.MsgCount)
	}
	if c1.Title != "产品讨论群" || c1.Kind != "group" {
		t.Fatalf("expected title/kind set, got %q/%q", c1.Title, c1.Kind)
	}

	// Second resolve bumps count.
	c2, err := s.ResolveChat("feishu", "oc_abc", "", "")
	if err != nil {
		t.Fatalf("resolve chat 2: %v", err)
	}
	if c2.MsgCount != 2 {
		t.Fatalf("expected MsgCount=2 after second resolve, got %d", c2.MsgCount)
	}
	// title/kind preserved (empty new value did not overwrite).
	if c2.Title != "产品讨论群" || c2.Kind != "group" {
		t.Fatalf("expected title/kind preserved, got %q/%q", c2.Title, c2.Kind)
	}

	// Different source — independent chat.
	if _, err := s.ResolveChat("telegram", "-1001234", "客户支持", "supergroup"); err != nil {
		t.Fatalf("resolve tg chat: %v", err)
	}

	list, err := s.ListChats()
	if err != nil {
		t.Fatalf("list chats: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(list))
	}

	// chats/ directory exists with both files.
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "feishu-oc_abc.md")); err != nil {
		t.Fatalf("chats file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "telegram--1001234.md")); err != nil {
		t.Fatalf("chats file missing: %v", err)
	}

	// INDEX.md contains chat section + chat names.
	idxBytes, err := os.ReadFile(filepath.Join(tmp, "network", "INDEX.md"))
	if err != nil {
		t.Fatalf("read INDEX.md: %v", err)
	}
	idx := string(idxBytes)
	if !strings.Contains(idx, "## 群聊 (2)") {
		t.Fatalf("INDEX.md missing chat section header:\n%s", idx)
	}
	if !strings.Contains(idx, "产品讨论群") || !strings.Contains(idx, "客户支持") {
		t.Fatalf("INDEX.md missing chat titles:\n%s", idx)
	}
}

func TestStoreResolveChatBackfillsEmptyFieldsButProtectsUserEdits(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	// Initial: empty title and kind.
	if _, err := s.ResolveChat("feishu", "oc_xyz", "", ""); err != nil {
		t.Fatal(err)
	}
	// Second: provides both — should back-fill.
	c, err := s.ResolveChat("feishu", "oc_xyz", "Backfilled Title", "group")
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Backfilled Title" {
		t.Fatalf("expected title backfilled, got %q", c.Title)
	}
	if c.Kind != "group" {
		t.Fatalf("expected kind backfilled, got %q", c.Kind)
	}

	// Third: tries to overwrite — must NOT clobber user edit.
	c, err = s.ResolveChat("feishu", "oc_xyz", "Different Title", "supergroup")
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Backfilled Title" {
		t.Fatalf("expected title preserved, got %q", c.Title)
	}
	if c.Kind != "group" {
		t.Fatalf("expected kind preserved, got %q", c.Kind)
	}
}

func TestStoreGetChatNotExist(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	c, err := s.GetChat("feishu:nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil for missing chat, got %+v", c)
	}
}

func TestStoreSaveChatAndGet(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	now := time.Now().UTC().Truncate(time.Second)
	in := &Chat{
		ID:          "feishu:oc_save",
		Source:      "feishu",
		ExternalID:  "oc_save",
		Title:       "保存测试群",
		Kind:        "group",
		Tags:        []string{"内部", "测试"},
		MemberCount: 7,
		CreatedAt:   now,
		LastSeenAt:  now,
		MsgCount:    3,
		Body:        "# 保存测试群\n\n## 基础信息\n- 群创建于 2026 年\n",
	}
	if err := s.SaveChat(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetChat("feishu:oc_save")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected chat, got nil")
	}
	if got.Title != in.Title || got.Kind != in.Kind {
		t.Fatalf("title/kind mismatch: %q/%q", got.Title, got.Kind)
	}
	if got.MsgCount != 3 || got.MemberCount != 7 {
		t.Fatalf("msgCount/memberCount mismatch: %d/%d", got.MsgCount, got.MemberCount)
	}
	if !strings.Contains(got.Body, "群创建于") {
		t.Fatalf("body lost: %q", got.Body)
	}
	// Tags sorted alphabetically by render.
	if len(got.Tags) != 2 {
		t.Fatalf("tags lost: %v", got.Tags)
	}
}

func TestStoreListChatsSortedByLastSeen(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	now := time.Now().UTC()
	older := &Chat{ID: "feishu:older", Source: "feishu", ExternalID: "older",
		Title: "old", LastSeenAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour), MsgCount: 1}
	newer := &Chat{ID: "feishu:newer", Source: "feishu", ExternalID: "newer",
		Title: "new", LastSeenAt: now, CreatedAt: now.Add(-time.Hour), MsgCount: 1}
	if err := s.SaveChat(older); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveChat(newer); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "feishu:newer" {
		t.Fatalf("sort order wrong; expected newer first, got %s", list[0].ID)
	}
}

func TestStoreDeleteChat(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	if _, err := s.ResolveChat("telegram", "-1001", "群1", "group"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveChat("telegram", "-1002", "群2", "group"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteChat("telegram:-1001"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "telegram--1001.md")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, err=%v", err)
	}
	list, err := s.ListChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 left, got %d", len(list))
	}
	// INDEX must reflect deletion.
	idxBytes, _ := os.ReadFile(filepath.Join(tmp, "network", "INDEX.md"))
	if !strings.Contains(string(idxBytes), "## 群聊 (1)") {
		t.Fatalf("INDEX not refreshed after delete:\n%s", string(idxBytes))
	}
}

func TestStoreTouchChat(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	if _, err := s.ResolveChat("feishu", "oc_t", "Touch 测试", "group"); err != nil {
		t.Fatal(err)
	}
	// Read back to capture the disk-truncated (RFC3339, second precision)
	// timestamp — the in-memory Chat has nanosecond precision which
	// would not survive a round-trip.
	pre, _ := s.GetChat("feishu:oc_t")
	origCreated := pre.CreatedAt
	origTitle := pre.Title
	origLastSeen := pre.LastSeenAt

	time.Sleep(1100 * time.Millisecond) // ensure RFC3339-second precision differs
	if err := s.TouchChat("feishu:oc_t"); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ := s.GetChat("feishu:oc_t")
	if got.MsgCount != 2 {
		t.Fatalf("expected MsgCount=2, got %d", got.MsgCount)
	}
	if !got.CreatedAt.Equal(origCreated) {
		t.Fatalf("CreatedAt should be unchanged: %v != %v", got.CreatedAt, origCreated)
	}
	if got.Title != origTitle {
		t.Fatalf("Title should be unchanged, got %q", got.Title)
	}
	if !got.LastSeenAt.After(origLastSeen) {
		t.Fatalf("LastSeenAt should be later, got %v <= %v", got.LastSeenAt, origLastSeen)
	}
}

func TestChatAndContactCoexist(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	if _, err := s.Resolve("feishu", "ou_alice", "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveChat("feishu", "oc_abc", "产品群", "group"); err != nil {
		t.Fatal(err)
	}
	// Both files exist in their own subdirs.
	if _, err := os.Stat(filepath.Join(tmp, "network", "contacts", "feishu-ou_alice.md")); err != nil {
		t.Fatalf("contact file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "feishu-oc_abc.md")); err != nil {
		t.Fatalf("chat file missing: %v", err)
	}
	// INDEX.md has both sections.
	idxBytes, _ := os.ReadFile(filepath.Join(tmp, "network", "INDEX.md"))
	idx := string(idxBytes)
	if !strings.Contains(idx, "## 真人联系人") {
		t.Fatalf("INDEX missing contact section:\n%s", idx)
	}
	if !strings.Contains(idx, "## 群聊") {
		t.Fatalf("INDEX missing chat section:\n%s", idx)
	}
	// INDEX.json has both fields.
	jsonBytes, _ := os.ReadFile(filepath.Join(tmp, "network", "INDEX.json"))
	var idxJSON Index
	if err := json.Unmarshal(jsonBytes, &idxJSON); err != nil {
		t.Fatalf("INDEX.json parse: %v", err)
	}
	if len(idxJSON.Contacts) != 1 {
		t.Fatalf("expected 1 contact in INDEX.json, got %d", len(idxJSON.Contacts))
	}
	if len(idxJSON.Chats) != 1 {
		t.Fatalf("expected 1 chat in INDEX.json, got %d", len(idxJSON.Chats))
	}
}

func TestParseChatMDRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	orig := &Chat{
		ID:          "feishu:oc_round",
		Source:      "feishu",
		ExternalID:  "oc_round",
		Title:       "Round-trip 群",
		Kind:        "supergroup",
		Tags:        []string{"内部", "测试"},
		MemberCount: 5,
		CreatedAt:   now,
		LastSeenAt:  now,
		MsgCount:    10,
		Body:        "# Round-trip 群\n\n## 基础信息\n- A 段\n\n## 群规则\n- B 段\n",
	}
	rendered := renderChatMD(orig)
	parsed := parseChatMD(rendered, "")

	if parsed.ID != orig.ID {
		t.Errorf("ID: %q != %q", parsed.ID, orig.ID)
	}
	if parsed.Source != orig.Source {
		t.Errorf("Source: %q != %q", parsed.Source, orig.Source)
	}
	if parsed.ExternalID != orig.ExternalID {
		t.Errorf("ExternalID: %q != %q", parsed.ExternalID, orig.ExternalID)
	}
	if parsed.Title != orig.Title {
		t.Errorf("Title: %q != %q", parsed.Title, orig.Title)
	}
	if parsed.Kind != orig.Kind {
		t.Errorf("Kind: %q != %q", parsed.Kind, orig.Kind)
	}
	if parsed.MemberCount != orig.MemberCount {
		t.Errorf("MemberCount: %d != %d", parsed.MemberCount, orig.MemberCount)
	}
	if parsed.MsgCount != orig.MsgCount {
		t.Errorf("MsgCount: %d != %d", parsed.MsgCount, orig.MsgCount)
	}
	if !parsed.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt: %v != %v", parsed.CreatedAt, orig.CreatedAt)
	}
	if !parsed.LastSeenAt.Equal(orig.LastSeenAt) {
		t.Errorf("LastSeenAt: %v != %v", parsed.LastSeenAt, orig.LastSeenAt)
	}
	if len(parsed.Tags) != len(orig.Tags) {
		t.Errorf("Tags lost: %v vs %v", parsed.Tags, orig.Tags)
	}
	if !strings.Contains(parsed.Body, "## 基础信息") || !strings.Contains(parsed.Body, "## 群规则") {
		t.Errorf("body sections lost:\n%s", parsed.Body)
	}
}

func TestParseChatMDLegacyNoFrontmatter(t *testing.T) {
	raw := "# 老格式群\n\n这是一段裸 markdown，没有 frontmatter。\n"
	c := parseChatMD(raw, "feishu:legacy")
	if c.ID != "feishu:legacy" {
		t.Fatalf("expected hint ID, got %q", c.ID)
	}
	if !strings.Contains(c.Body, "老格式群") {
		t.Fatalf("body not preserved: %q", c.Body)
	}
}

func TestRenderChatMDDefaultBody(t *testing.T) {
	c := &Chat{
		ID:         "feishu:oc_new",
		Source:     "feishu",
		ExternalID: "oc_new",
		Title:      "新群",
		Kind:       "group",
		CreatedAt:  time.Now().UTC(),
		LastSeenAt: time.Now().UTC(),
		MsgCount:   1,
	}
	rendered := renderChatMD(c)
	for _, want := range []string{
		"# 新群",
		"## 基础信息",
		"## 群规则",
		"## 重要议题",
		"## 待跟进",
		"(AI 通过 chat_note 工具追加此处)",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("default body missing %q in:\n%s", want, rendered)
		}
	}

	// Empty title also OK — uses fallback "群聊"
	c.Title = ""
	rendered2 := renderChatMD(c)
	if !strings.Contains(rendered2, "# 群聊") {
		t.Errorf("expected fallback header '群聊' in:\n%s", rendered2)
	}
}

func TestChatIDIsolation_DoesNotCollideWithContact(t *testing.T) {
	// Chat ID and Contact ID can be textually identical because they live in
	// different subdirs.
	tmp := t.TempDir()
	s := NewStore(tmp)
	if _, err := s.Resolve("feishu", "ou_same", "Person"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveChat("feishu", "ou_same", "Group same name", "group"); err != nil {
		t.Fatal(err)
	}
	contact, err := s.Get("feishu:ou_same")
	if err != nil || contact == nil {
		t.Fatalf("contact lost: err=%v contact=%v", err, contact)
	}
	chat, err := s.GetChat("feishu:ou_same")
	if err != nil || chat == nil {
		t.Fatalf("chat lost: err=%v chat=%v", err, chat)
	}
	if contact.DisplayName != "Person" {
		t.Errorf("contact DisplayName wrong: %q", contact.DisplayName)
	}
	if chat.Title != "Group same name" {
		t.Errorf("chat Title wrong: %q", chat.Title)
	}
}
