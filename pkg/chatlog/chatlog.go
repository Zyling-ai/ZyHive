// Package chatlog — AI-visible conversation history index system.
// Parallel to convlog (admin audit log). This log is injected into system prompts
// so the AI can look up past conversations.
//
// Directory layout under workspace/conversations/:
//   index.json       — complete index (all sessions/channels), concurrent-safe
//   INDEX.md         — lightweight summary injected into system prompt (latest 20)
//   {sessionId}__{channelId}.jsonl  — full per-session+channel messages (AI can read)
package chatlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one message record in the per-session JSONL file.
type Entry struct {
	Ts          string `json:"ts"`
	SessionID   string `json:"sessionId"`
	ChannelID   string `json:"channelId"`
	ChannelType string `json:"channelType"`
	Role        string `json:"role"`    // "user" | "assistant"
	Content     string `json:"content"`
	Sender      string `json:"sender,omitempty"` // user display name / ID
}

// IndexEntry holds summary metadata for one session+channel conversation.
type IndexEntry struct {
	SessionID    string `json:"sessionId"`
	ChannelID    string `json:"channelId"`
	ChannelType  string `json:"channelType"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	CreatedAt    string `json:"createdAt"`
	LastAt       string `json:"lastAt"`
	Summary      string `json:"summary,omitempty"`
	FilePath     string `json:"filePath"` // relative to workspaceDir
}

// diskIndex is the on-disk shape of index.json.
type diskIndex struct {
	Entries []IndexEntry `json:"entries"`
}

// Manager handles all chatlog operations for one workspace.
type Manager struct {
	workspaceDir string
	mu           sync.Mutex
}

// NewManager creates a Manager for the given workspace directory.
func NewManager(workspaceDir string) *Manager {
	return &Manager{workspaceDir: workspaceDir}
}

// conversationsDir returns workspace/conversations/.
func (m *Manager) conversationsDir() string {
	return filepath.Join(m.workspaceDir, "conversations")
}

func (m *Manager) indexPath() string {
	return filepath.Join(m.conversationsDir(), "index.json")
}

func (m *Manager) indexMDPath() string {
	return filepath.Join(m.conversationsDir(), "INDEX.md")
}

// entryFilePath returns the JSONL path for a session+channel pair.
// File naming: {sessionId}__{channelId}.jsonl (double underscore separator).
func (m *Manager) entryFilePath(sessionID, channelID string) string {
	safe := func(s string) string {
		return strings.NewReplacer("/", "-", "\\", "-", " ", "_").Replace(s)
	}
	name := safe(sessionID) + "__" + safe(channelID) + ".jsonl"
	return filepath.Join(m.conversationsDir(), name)
}

// Append writes an Entry to the per-session JSONL file, updates index.json,
// and regenerates INDEX.md. All writes are protected by mu.
func (m *Manager) Append(entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir := m.conversationsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("chatlog mkdir: %w", err)
	}

	// Set timestamp if not provided
	if entry.Ts == "" {
		entry.Ts = time.Now().UTC().Format(time.RFC3339)
	}

	// Append to JSONL file
	fp := m.entryFilePath(entry.SessionID, entry.ChannelID)
	f, err := os.OpenFile(fp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("chatlog open: %w", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		f.Close()
		return err
	}
	_, err = f.Write(append(data, '\n'))
	f.Close()
	if err != nil {
		return err
	}

	// Update index
	idx, _ := m.loadIndex()
	m.upsertIndexEntry(idx, entry, fp)
	if err := m.saveIndex(idx); err != nil {
		return err
	}
	return m.writeIndexMD(idx)
}

// UpdateSummary sets the summary for all IndexEntry records matching sessionID.
// Called after compaction completes.
func (m *Manager) UpdateSummary(sessionID, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx, err := m.loadIndex()
	if err != nil {
		return err
	}
	updated := false
	for i, e := range idx.Entries {
		if e.SessionID == sessionID {
			idx.Entries[i].Summary = summary
			updated = true
		}
	}
	if !updated {
		return nil
	}
	if err := m.saveIndex(idx); err != nil {
		return err
	}
	return m.writeIndexMD(idx)
}

// GetIndexMD returns the content of INDEX.md (empty string if not found).
func (m *Manager) GetIndexMD() string {
	data, err := os.ReadFile(m.indexMDPath())
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadMessages reads entries from a session+channel JSONL with optional pagination.
// Returns: entries, total count, error.
func (m *Manager) ReadMessages(sessionID, channelID string, limit, offset int) ([]Entry, int, error) {
	fp := m.entryFilePath(sessionID, channelID)
	f, err := os.Open(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err2 := json.Unmarshal([]byte(line), &e); err2 != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []Entry{}
	}
	total := len(entries)
	if offset >= total {
		return []Entry{}, total, nil
	}
	slice := entries[offset:]
	if limit > 0 && limit < len(slice) {
		slice = slice[:limit]
	}
	return slice, total, nil
}

// ── internal helpers ──────────────────────────────────────────────────────

// loadIndex loads index.json. Caller must hold mu.
func (m *Manager) loadIndex() (*diskIndex, error) {
	data, err := os.ReadFile(m.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &diskIndex{Entries: []IndexEntry{}}, nil
		}
		return nil, err
	}
	var idx diskIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return &diskIndex{Entries: []IndexEntry{}}, nil
	}
	if idx.Entries == nil {
		idx.Entries = []IndexEntry{}
	}
	return &idx, nil
}

// saveIndex writes index.json atomically via temp-file + rename. Caller must hold mu.
func (m *Manager) saveIndex(idx *diskIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, m.indexPath())
}

// upsertIndexEntry updates an existing IndexEntry or inserts a new one. Caller must hold mu.
func (m *Manager) upsertIndexEntry(idx *diskIndex, entry Entry, fp string) {
	for i, e := range idx.Entries {
		if e.SessionID == entry.SessionID && e.ChannelID == entry.ChannelID {
			idx.Entries[i].MessageCount++
			idx.Entries[i].LastAt = entry.Ts
			if idx.Entries[i].Title == "" && entry.Role == "user" {
				idx.Entries[i].Title = truncateTitle(entry.Content)
			}
			return
		}
	}
	// New entry
	title := ""
	if entry.Role == "user" {
		title = truncateTitle(entry.Content)
	}
	relPath := fp
	if rel, err := filepath.Rel(m.workspaceDir, fp); err == nil {
		relPath = rel
	}
	idx.Entries = append(idx.Entries, IndexEntry{
		SessionID:    entry.SessionID,
		ChannelID:    entry.ChannelID,
		ChannelType:  entry.ChannelType,
		Title:        title,
		MessageCount: 1,
		CreatedAt:    entry.Ts,
		LastAt:       entry.Ts,
		FilePath:     relPath,
	})
}

// writeIndexMD regenerates INDEX.md with the most recent 20 entries. Caller must hold mu.
func (m *Manager) writeIndexMD(idx *diskIndex) error {
	entries := make([]IndexEntry, len(idx.Entries))
	copy(entries, idx.Entries)

	// Sort by LastAt descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastAt > entries[j].LastAt
	})
	if len(entries) > 20 {
		entries = entries[:20]
	}

	var sb strings.Builder
	sb.WriteString("## 历史对话索引（最近20条）\n\n")
	sb.WriteString("| 时间 | 会话 | 渠道 | 标题 | 消息数 | 摘要 |\n")
	sb.WriteString("|------|------|------|------|--------|------|\n")
	for _, e := range entries {
		ts := formatTime(e.LastAt)
		summary := e.Summary
		if len([]rune(summary)) > 30 {
			runes := []rune(summary)
			summary = string(runes[:30]) + "…"
		}
		title := e.Title
		if len([]rune(title)) > 20 {
			runes := []rune(title)
			title = string(runes[:20]) + "…"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
			ts, e.SessionID, e.ChannelID, title, e.MessageCount, summary))
	}
	return os.WriteFile(m.indexMDPath(), []byte(sb.String()), 0644)
}

func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 20 {
		return string(runes[:20]) + "…"
	}
	return s
}

func formatTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04")
}
