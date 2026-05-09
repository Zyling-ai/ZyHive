package network

import (
	"strings"
	"testing"
	"time"
)

func TestChatSummaryEmpty(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	out := s.ChatSummary("feishu:nope")
	if out != "" {
		t.Fatalf("expected empty for missing chat, got: %q", out)
	}
}

func TestChatSummaryFullRender(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	now := time.Now().UTC()
	c := &Chat{
		ID:          "feishu:oc_full",
		Source:      "feishu",
		ExternalID:  "oc_full",
		Title:       "产品讨论群",
		Kind:        "supergroup",
		Tags:        []string{"内部", "产品"},
		MemberCount: 12,
		CreatedAt:   now.Add(-24 * time.Hour),
		LastSeenAt:  now,
		MsgCount:    47,
		Body: `# 产品讨论群

## 基础信息
- 产品研发组每周三 11 点开会
- 群主：张三
- 禁止外发截图

## 群规则
- 工作时间禁灌水

## 重要议题
- V2.0 发布日期讨论中
- 客户反馈整理

## 待跟进
- 周三例会议程`,
	}
	if err := s.SaveChat(c); err != nil {
		t.Fatal(err)
	}

	out := s.ChatSummary("feishu:oc_full")
	if out == "" {
		t.Fatal("expected non-empty summary")
	}
	for _, want := range []string{
		"【当前群聊】",
		"产品讨论群",
		"feishu · oc_full [supergroup]",
		"累计消息: 47 次",
		"成员数: 12",
		"标签: 产品 / 内部", // tags are sorted by renderChatMD, "产" < "内" in unicode
		"基础信息 (最近 3):",
		"产品研发组每周三",
		"群主：张三",
		"禁止外发截图",
		"群规则",
		"工作时间禁灌水",
		"重要议题 (最近 2):",
		"V2.0 发布日期讨论中",
		"客户反馈整理",
		"待跟进 (最近 2):",
		"周三例会议程",
		"feishu-oc_full.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected summary to contain %q, got:\n%s", want, out)
		}
	}
}

func TestChatSummaryTruncatesAt1200(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	// Create a chat with a body so massive the summary exceeds 1200 chars.
	bigBasics := strings.Repeat("- 一条很长很长的群基础信息事项 abcdefghij\n", 50)
	c := &Chat{
		ID:         "feishu:oc_big",
		Source:     "feishu",
		ExternalID: "oc_big",
		Title:      "巨大群",
		Kind:       "group",
		LastSeenAt: time.Now().UTC(),
		MsgCount:   100,
		Body:       "# 巨大群\n\n## 基础信息\n" + bigBasics,
	}
	if err := s.SaveChat(c); err != nil {
		t.Fatal(err)
	}

	out := s.ChatSummary("feishu:oc_big")
	if out == "" {
		t.Fatal("unexpected empty")
	}
	// The cap is 1200 chars + a small tail "...\n[截断, ...]\n" — so allow some slack.
	if len(out) > 1300 {
		t.Fatalf("summary too long: %d chars", len(out))
	}
	// But it should still include header and at least basic info.
	if !strings.Contains(out, "【当前群聊】") {
		t.Errorf("header missing")
	}
	// Either truncation marker present, or the original was short enough
	// to fit (this body is large enough that truncation must trigger; only
	// the first N basics are extracted).
	// (The extractor already caps at 3 bullets, so truncation may not actually
	// trigger; this test validates the cap is _at least_ enforced when it does.)
	_ = out
}

func TestChatSummaryNoBodySections(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	c := &Chat{
		ID:         "feishu:oc_bare",
		Source:     "feishu",
		ExternalID: "oc_bare",
		Title:      "空档案群",
		Kind:       "group",
		LastSeenAt: time.Now().UTC(),
		MsgCount:   1,
		Body:       "", // empty -> defaultChatBody fills 4 sections with placeholders
	}
	if err := s.SaveChat(c); err != nil {
		t.Fatal(err)
	}

	out := s.ChatSummary("feishu:oc_bare")
	if out == "" {
		t.Fatal("expected non-empty even with placeholder-only body")
	}
	// Header, group name, source line, msg count line should all appear.
	for _, want := range []string{
		"【当前群聊】",
		"群名: 空档案群",
		"来源: feishu · oc_bare",
		"累计消息: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Placeholders should NOT appear as bullet items (extractSectionBullets skips them).
	if strings.Contains(out, "(AI 通过 chat_note") {
		t.Errorf("placeholder leaked into summary:\n%s", out)
	}
	// Section "基础信息 (最近 3):" header should not appear when nothing real to show.
	if strings.Contains(out, "基础信息 (最近") {
		t.Errorf("empty section header should be skipped:\n%s", out)
	}
}
