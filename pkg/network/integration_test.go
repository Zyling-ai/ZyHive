package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChannelLikeUsage_FeishuGroup mirrors what pkg/channel/feishu.go does
// when a group message arrives: ResolveChat(group) + Resolve(sender) +
// build extraCtx by concatenating ChatSummary + Summary.
//
// Also catches regressions where one accidentally lands in the wrong
// physical directory (chats vs contacts).
func TestChannelLikeUsage_FeishuGroup(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(tmp)

	// Simulate 3 sequential group messages from the same sender.
	for i := 0; i < 3; i++ {
		// 1. Chat profile — title "" (Feishu doesn't give title in event).
		if _, err := store.ResolveChat(SourceFeishu, "oc_team", "", "group"); err != nil {
			t.Fatalf("iter %d ResolveChat: %v", i, err)
		}
		// 2. Sender contact.
		if _, err := store.Resolve(SourceFeishu, "ou_alice", "Alice"); err != nil {
			t.Fatalf("iter %d Resolve: %v", i, err)
		}
	}

	// Verify on disk: chat file in chats/, contact file in contacts/.
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "feishu-oc_team.md")); err != nil {
		t.Fatalf("chat file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "network", "contacts", "feishu-ou_alice.md")); err != nil {
		t.Fatalf("contact file missing: %v", err)
	}

	// Both should have MsgCount=3.
	chat, _ := store.GetChat("feishu:oc_team")
	if chat == nil || chat.MsgCount != 3 {
		t.Fatalf("chat MsgCount expected 3, got %v", chat)
	}
	contact, _ := store.Get("feishu:ou_alice")
	if contact == nil || contact.MsgCount != 3 {
		t.Fatalf("contact MsgCount expected 3, got %v", contact)
	}

	// Build extraCtx the same way feishu.go does.
	extraCtx := "当前飞书用户信息：open_id=ou_alice, chat_id=oc_team, chat_type=group"
	if cs := store.ChatSummary(MakeID(SourceFeishu, "oc_team")); cs != "" {
		extraCtx += "\n\n" + cs
	}
	if pcs := store.Summary(MakeID(SourceFeishu, "ou_alice")); pcs != "" {
		extraCtx += "\n\n" + pcs
	}

	// extraCtx must contain both chat and contact context markers.
	if !strings.Contains(extraCtx, "【当前群聊】") {
		t.Errorf("missing chat header in extraCtx:\n%s", extraCtx)
	}
	if !strings.Contains(extraCtx, "【当前对话对方】") {
		t.Errorf("missing contact header in extraCtx:\n%s", extraCtx)
	}
	// Source field correct in chat summary.
	if !strings.Contains(extraCtx, "来源: feishu · oc_team [group]") {
		t.Errorf("chat source line wrong:\n%s", extraCtx)
	}
}

// TestChannelLikeUsage_TelegramPrivate verifies private (non-group) telegram
// chats DO NOT create a chat profile — only the contact is resolved.
func TestChannelLikeUsage_TelegramPrivate(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(tmp)

	// Simulate private DM: only contact resolved, ResolveChat NOT called.
	if _, err := store.Resolve(SourceTelegram, "12345", "Bob"); err != nil {
		t.Fatal(err)
	}

	// Contact file present.
	if _, err := os.Stat(filepath.Join(tmp, "network", "contacts", "telegram-12345.md")); err != nil {
		t.Fatalf("contact file missing: %v", err)
	}
	// Chat file NOT created. Even chats/ dir might not exist yet.
	if _, err := os.Stat(filepath.Join(tmp, "network", "chats", "telegram-12345.md")); !os.IsNotExist(err) {
		t.Fatalf("expected no chat file for private DM, err=%v", err)
	}
	// ListChats returns empty.
	chats, err := store.ListChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 0 {
		t.Fatalf("expected 0 chats for private-only flow, got %d", len(chats))
	}
}

// TestChannelLikeUsage_TelegramGroupBackfillsTitle verifies that telegram (which
// DOES give msg.Chat.Title in the event) successfully back-fills the title on
// first sight.
func TestChannelLikeUsage_TelegramGroupBackfillsTitle(t *testing.T) {
	tmp := t.TempDir()
	store := NewStore(tmp)

	// 1st message: full title "Alpha 群"
	if _, err := store.ResolveChat(SourceTelegram, "-1001", "Alpha 群", "supergroup"); err != nil {
		t.Fatal(err)
	}
	c, _ := store.GetChat("telegram:-1001")
	if c.Title != "Alpha 群" || c.Kind != "supergroup" {
		t.Fatalf("expected title/kind set on first sight, got %q/%q", c.Title, c.Kind)
	}

	// User renames the group via UI — server doesn't know yet, sends "" again
	// (or maybe an outdated cached title). Should preserve user value.
	if _, err := store.ResolveChat(SourceTelegram, "-1001", "Different name", ""); err != nil {
		t.Fatal(err)
	}
	c, _ = store.GetChat("telegram:-1001")
	if c.Title != "Alpha 群" {
		t.Fatalf("expected title preserved, got %q", c.Title)
	}
}
