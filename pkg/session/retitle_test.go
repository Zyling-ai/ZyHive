package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSanitizeTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"闲聊", "闲聊"},
		{"\"讨论 API 限流\"", "讨论 API 限流"},
		{"「写周报」", "写周报"},
		{"标题：测试登录接口", "测试登录接口"},
		{"**架构讨论**", "架构讨论"},
		{"  多余空格  \n第二行", "多余空格"},
		// truncation: should cap at 30 runes
		{strings.Repeat("很长标题", 20), strings.Repeat("很长标题", 7) + "很长…"},
	}
	for _, c := range cases {
		got := sanitizeTitle(c.in)
		if got != c.want {
			t.Errorf("sanitizeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildRetitleInput(t *testing.T) {
	mk := func(role, text string) Message {
		b, _ := json.Marshal(text)
		return Message{Role: role, Content: b}
	}
	msgs := []Message{
		mk("user", "你好"),
		mk("assistant", "你好，我能帮你什么？"),
		mk("user", "帮我分析一下这个日志里有几次错误"),
		mk("assistant", "共检测到 3 次错误：A / B / C。"),
	}
	out := buildRetitleInput(msgs, 1000)
	if !strings.Contains(out, "你好") || !strings.Contains(out, "日志") {
		t.Fatalf("expected all messages, got:\n%s", out)
	}
	// Order preserved
	if strings.Index(out, "你好") > strings.Index(out, "日志") {
		t.Fatal("order not chronological")
	}
}

func TestBuildRetitleInput_TruncatesToLatest(t *testing.T) {
	mk := func(role, text string) Message {
		b, _ := json.Marshal(text)
		return Message{Role: role, Content: b}
	}
	msgs := []Message{
		mk("user", strings.Repeat("旧", 500)),
		mk("assistant", "A"),
		mk("user", "最新问题"),
	}
	out := buildRetitleInput(msgs, 100)
	// Latest content should be present
	if !strings.Contains(out, "最新问题") {
		t.Fatalf("latest message dropped: %q", out)
	}
}

func TestNeedsAutoRetitle_Milestones(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(tmp)
	sid, _, err := store.GetOrCreate("ses-test", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	// Just created → 0 msgs, not needed.
	if store.NeedsAutoRetitle(sid) {
		t.Fatal("should not need retitle with 0 msgs")
	}

	// Fake 5 user msgs to cross threshold 4.
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal("msg " + string(rune('A'+i)))
		if err := store.AppendMessage(sid, "user", body); err != nil {
			t.Fatal(err)
		}
	}
	if !store.NeedsAutoRetitle(sid) {
		t.Fatal("5 msgs should trigger threshold 4")
	}

	// Mark as retitled at 4.
	if err := store.UpdateAutoTitle(sid, "闲聊", 4); err != nil {
		t.Fatal(err)
	}
	if store.NeedsAutoRetitle(sid) {
		t.Fatal("after retitle at 4, shouldn't re-trigger until 12")
	}

	// Hit 13 msgs → cross threshold 12.
	for i := 0; i < 8; i++ {
		body, _ := json.Marshal("more")
		if err := store.AppendMessage(sid, "user", body); err != nil {
			t.Fatal(err)
		}
	}
	if !store.NeedsAutoRetitle(sid) {
		t.Fatal("13 msgs should trigger threshold 12")
	}

	// User override stops all future retitle.
	if err := store.UpdateTitle(sid, "用户自定义"); err != nil {
		t.Fatal(err)
	}
	if store.NeedsAutoRetitle(sid) {
		t.Fatal("user override should block auto retitle")
	}
}

func TestMaybeAutoRetitle_RunsSummarizer(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(tmp)
	sid, _, _ := store.GetOrCreate("ses-test", "agent-1")
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal("内容 " + string(rune('A'+i)))
		_ = store.AppendMessage(sid, "user", body)
	}

	called := make(chan bool, 1)
	summarizer := func(ctx context.Context, system, userMsg string) (string, error) {
		called <- true
		return "测试主题", nil
	}
	MaybeAutoRetitle(store, sid, summarizer)

	select {
	case <-called:
		// wait briefly for the async UpdateAutoTitle
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			meta, _ := store.GetMeta(sid)
			if meta.Title == "测试主题" {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("title not updated in time")
	case <-time.After(3 * time.Second):
		t.Fatal("summarizer not called within 3s")
	}
}
