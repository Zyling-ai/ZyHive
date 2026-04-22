package tools

import (
	"os"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/network"
)

func TestSuggestContactIDs(t *testing.T) {
	tmp, err := os.MkdirTemp("", "suggest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	store := network.NewStore(tmp)
	if _, err := store.Resolve("feishu", "ou_abc", "张三"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve("feishu", "ou_xyz", "李四"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve("telegram", "12345", "Boss"); err != nil {
		t.Fatal(err)
	}

	// Typo: AI mangled the ID. Should suggest feishu ones first due to source prefix.
	sugg := suggestContactIDs(store, "feishu:ou_", 3)
	if len(sugg) == 0 {
		t.Fatalf("expected suggestions, got none")
	}
	if !strings.HasPrefix(sugg[0], "feishu:") {
		t.Fatalf("top suggestion should be feishu: got %q", sugg[0])
	}

	// Unrelated query — still returns top matches (non-empty if store has any).
	sugg = suggestContactIDs(store, "zzz_nothing_matches", 3)
	// Allowed to be empty if score 0 across the board, just shouldn't panic.
	_ = sugg

	// Empty store → nil
	tmp2, _ := os.MkdirTemp("", "suggest-empty-*")
	defer os.RemoveAll(tmp2)
	empty := network.NewStore(tmp2)
	if got := suggestContactIDs(empty, "anything", 3); got != nil {
		t.Fatalf("empty store should return nil, got %v", got)
	}
}
