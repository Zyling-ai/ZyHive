// pkg/network/chat_store.go — Store 的 Chat (群档案) 操作.
//
// 与 contact 操作 (store.go) 的方法集对称: GetChat / SaveChat / DeleteChat /
// ListChats / TouchChat / ResolveChat. 共享同一个 *Store 实例 (因此共享 mu),
// 但物理落盘在 chats/ 子目录, 不污染 contacts/.
package network

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── Chat: read ─────────────────────────────────────────────────────────────

// GetChat reads one chat by ID. Returns (nil, nil) if the chat does not exist.
func (s *Store) GetChat(id string) (*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getChatUnlocked(id)
}

func (s *Store) getChatUnlocked(id string) (*Chat, error) {
	path := filepath.Join(s.chatsDir(), filenameForID(id))
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	c := parseChatMD(string(raw), id)
	if c.ID == "" {
		c.ID = id
	}
	return c, nil
}

// ── Chat: list ─────────────────────────────────────────────────────────────

// ListChats returns all chats (sorted by LastSeenAt desc).
func (s *Store) ListChats() ([]ChatSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listChatsUnlocked()
}

func (s *Store) listChatsUnlocked() ([]ChatSummary, error) {
	dir := s.chatsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]ChatSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		// Derive ID fallback from filename.
		fallbackID := strings.TrimSuffix(e.Name(), ".md")
		if i := strings.Index(fallbackID, "-"); i >= 0 {
			fallbackID = fallbackID[:i] + ":" + fallbackID[i+1:]
		}
		c := parseChatMD(string(data), fallbackID)
		if c.ID == "" {
			c.ID = fallbackID
		}
		out = append(out, c.Summary())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out, nil
}

// ── Chat: write ────────────────────────────────────────────────────────────

// SaveChat persists the chat, regenerates INDEX.*.
func (s *Store) SaveChat(c *Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveChatUnlocked(c)
}

func (s *Store) saveChatUnlocked(c *Chat) error {
	if err := s.ensureChatsDir(); err != nil {
		return err
	}
	path := filepath.Join(s.chatsDir(), filenameForID(c.ID))
	if err := os.WriteFile(path, []byte(renderChatMD(c)), 0644); err != nil {
		return err
	}
	return s.refreshIndexUnlocked()
}

// DeleteChat removes a chat file and refreshes the index.
func (s *Store) DeleteChat(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.chatsDir(), filenameForID(id))
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return s.refreshIndexUnlocked()
}

// TouchChat bumps LastSeenAt / MsgCount without changing anything else.
// Used for subsequent messages when the caller already has a resolved chat.
func (s *Store) TouchChat(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.getChatUnlocked(id)
	if err != nil || c == nil {
		return err
	}
	c.LastSeenAt = time.Now().UTC()
	c.MsgCount++
	return s.saveChatUnlocked(c)
}

// ── Chat: resolve (the canonical upsert called by every channel handler) ──

// ResolveChat is called by every inbound-message handler when the message
// arrives in a group/channel/multi-party context. Behavior:
//  1. If a chat file at {source}:{externalID} exists → bump LastSeenAt/MsgCount.
//     - title is back-filled only when current title is empty (protects user edits)
//     - kind  is back-filled only when current kind  is empty (same reason)
//  2. Else → create a new chat with default body and given title/kind.
//
// Channels MUST NOT call ResolveChat for 1-on-1 (private/p2p) messages — those
// are already covered by Resolve (contact). Group-only by design: a group
// profile only earns its own file when there are multiple senders.
func (s *Store) ResolveChat(source, externalID, title, kind string) (*Chat, error) {
	if source == "" || externalID == "" {
		return nil, fmt.Errorf("network.ResolveChat: source and externalID required")
	}
	id := MakeID(source, externalID)
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.getChatUnlocked(id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if existing != nil {
		existing.LastSeenAt = now
		existing.MsgCount++
		// Only back-fill if currently empty — never overwrite user edits.
		if title != "" && existing.Title == "" {
			existing.Title = title
		}
		if kind != "" && existing.Kind == "" {
			existing.Kind = kind
		}
		if err := s.saveChatUnlocked(existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	c := &Chat{
		ID:         id,
		Source:     source,
		ExternalID: externalID,
		Title:      title,
		Kind:       kind,
		CreatedAt:  now,
		LastSeenAt: now,
		MsgCount:   1,
	}
	if err := s.saveChatUnlocked(c); err != nil {
		return nil, err
	}
	return c, nil
}
