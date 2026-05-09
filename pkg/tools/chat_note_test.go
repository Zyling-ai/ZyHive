package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/network"
)

// newTestRegistryWithChat builds a minimal Registry pointing at a workspace
// where one chat is pre-resolved. Returns the registry, workspace path, and
// the resolved chat ID for convenience.
func newTestRegistryWithChat(t *testing.T, source, externalID, title, kind string) (*Registry, string, string) {
	t.Helper()
	tmp := t.TempDir()
	store := network.NewStore(tmp)
	if _, err := store.ResolveChat(source, externalID, title, kind); err != nil {
		t.Fatalf("setup ResolveChat: %v", err)
	}
	r := &Registry{
		handlers:     map[string]Handler{},
		workspaceDir: tmp,
		agentDir:     filepath.Dir(tmp),
		agentID:      "test-agent",
	}
	return r, tmp, network.MakeID(source, externalID)
}

func TestChatNoteAppendsToExisting(t *testing.T) {
	r, tmp, chatID := newTestRegistryWithChat(t, "feishu", "oc_abc", "产品讨论群", "group")
	in := mustJSON(map[string]string{
		"chatId":  chatID,
		"section": "群规则",
		"text":    "工作时间禁灌水",
	})
	out, err := r.handleChatNote(context.Background(), in)
	if err != nil {
		t.Fatalf("chat_note: %v", err)
	}
	if !strings.Contains(out, "已在群") {
		t.Errorf("response missing success marker: %s", out)
	}

	// Verify on disk
	store := network.NewStore(tmp)
	chat, _ := store.GetChat(chatID)
	if chat == nil {
		t.Fatal("chat lost after note")
	}
	if !strings.Contains(chat.Body, "工作时间禁灌水") {
		t.Errorf("note not persisted to body:\n%s", chat.Body)
	}
}

func TestChatNoteCreatesMissingSection(t *testing.T) {
	r, tmp, chatID := newTestRegistryWithChat(t, "telegram", "-1001", "Group", "group")
	// Strip default body's "重要议题" section so handler must create it.
	store := network.NewStore(tmp)
	chat, _ := store.GetChat(chatID)
	chat.Body = "# Group\n\n## 群规则\n- 不闲聊\n"
	_ = store.SaveChat(chat)

	in := mustJSON(map[string]string{
		"chatId": chatID, "section": "重要议题", "text": "Q3 路线图复盘",
	})
	if _, err := r.handleChatNote(context.Background(), in); err != nil {
		t.Fatalf("chat_note: %v", err)
	}
	chat, _ = store.GetChat(chatID)
	if !strings.Contains(chat.Body, "## 重要议题") {
		t.Fatalf("section not created:\n%s", chat.Body)
	}
	if !strings.Contains(chat.Body, "- Q3 路线图复盘") {
		t.Fatalf("entry missing:\n%s", chat.Body)
	}
}

func TestChatNoteRejectsInvalidSection(t *testing.T) {
	r, _, chatID := newTestRegistryWithChat(t, "feishu", "oc_x", "X", "group")
	in := mustJSON(map[string]string{
		"chatId": chatID, "section": "随便", "text": "x",
	})
	_, err := r.handleChatNote(context.Background(), in)
	if err == nil {
		t.Fatal("expected error for invalid section")
	}
	if !strings.Contains(err.Error(), "section must be one of") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestChatNoteRejectsMissingChat(t *testing.T) {
	r, _, _ := newTestRegistryWithChat(t, "feishu", "oc_one", "Real", "group")
	in := mustJSON(map[string]string{
		"chatId": "feishu:oc_does_not_exist", "section": "群规则", "text": "x",
	})
	_, err := r.handleChatNote(context.Background(), in)
	if err == nil {
		t.Fatal("expected error for missing chat")
	}
	// Should include Did-you-mean hint pointing to the existing chat.
	if !strings.Contains(err.Error(), "Did you mean") {
		t.Errorf("expected Did-you-mean hint, got: %v", err)
	}
	if !strings.Contains(err.Error(), "feishu:oc_one") {
		t.Errorf("hint should point to existing chat, got: %v", err)
	}
}

func TestChatNoteRejectsRequiredFields(t *testing.T) {
	r, _, chatID := newTestRegistryWithChat(t, "feishu", "oc_y", "Y", "group")
	cases := []struct {
		name string
		in   map[string]string
	}{
		{"missing chatId", map[string]string{"section": "群规则", "text": "x"}},
		{"missing section", map[string]string{"chatId": chatID, "text": "x"}},
		{"missing text", map[string]string{"chatId": chatID, "section": "群规则"}},
		{"empty chatId", map[string]string{"chatId": "  ", "section": "群规则", "text": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.handleChatNote(context.Background(), mustJSON(tc.in))
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestChatNoteAuditLog(t *testing.T) {
	r, tmp, chatID := newTestRegistryWithChat(t, "feishu", "oc_audit", "Audit", "group")
	in := mustJSON(map[string]string{
		"chatId": chatID, "section": "群规则", "text": "审计日志测试",
	})
	if _, err := r.handleChatNote(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	logBytes, err := os.ReadFile(filepath.Join(tmp, "network", "changes.log"))
	if err != nil {
		t.Fatalf("read changes.log: %v", err)
	}
	logStr := string(logBytes)
	if !strings.Contains(logStr, "chat:feishu:oc_audit") {
		t.Errorf("audit log missing chat: prefix:\n%s", logStr)
	}
	if !strings.Contains(logStr, "群规则") {
		t.Errorf("audit log missing section:\n%s", logStr)
	}
}

func TestSuggestChatIDsRanking(t *testing.T) {
	tmp := t.TempDir()
	store := network.NewStore(tmp)
	if _, err := store.ResolveChat("feishu", "oc_alpha", "Alpha 群", "group"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveChat("feishu", "oc_beta", "Beta 群", "group"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveChat("telegram", "-1001", "TG group", "group"); err != nil {
		t.Fatal(err)
	}

	// Source-prefix match: feishu typo → both feishu chats first
	sugg := suggestChatIDs(store, "feishu:oc_", 3)
	if len(sugg) == 0 {
		t.Fatal("expected suggestions")
	}
	if !strings.HasPrefix(sugg[0], "feishu:") {
		t.Fatalf("top suggestion should be feishu:, got %q", sugg[0])
	}

	// Title substring match: "Beta" → telegram doesn't match, feishu beta does
	sugg = suggestChatIDs(store, "Beta", 3)
	if len(sugg) == 0 || sugg[0] != "feishu:oc_beta" {
		t.Errorf("expected feishu:oc_beta as top match for 'Beta', got %v", sugg)
	}

	// Empty store → nil
	tmp2 := t.TempDir()
	empty := network.NewStore(tmp2)
	if got := suggestChatIDs(empty, "anything", 3); got != nil {
		t.Fatalf("empty store should return nil, got %v", got)
	}
}

