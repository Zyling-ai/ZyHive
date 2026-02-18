// Store provides append-only JSONL session read/write.
// Reference: pi-coding-agent/dist/core/session-manager.js
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// SessionIndex maps session IDs to their file paths and metadata.
// Persisted as sessions.json in the sessions directory.
type SessionIndex struct {
	Sessions map[string]SessionIndexEntry `json:"sessions"`
}

// Store manages session files for one agent.
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore creates a Store backed by the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// GetOrCreate returns a session ID, creating a new session if sessionID is empty or not found.
// Returns the resolved sessionID and whether it was newly created.
func (s *Store) GetOrCreate(sessionID, agentID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return "", false, err
	}

	// If sessionID provided, check it exists
	if sessionID != "" {
		idx, err := s.loadIndex()
		if err == nil {
			if _, ok := idx.Sessions[sessionID]; ok {
				return sessionID, false, nil
			}
		}
	}

	// Create new session
	if sessionID == "" {
		sessionID = fmt.Sprintf("ses-%d", nowMs())
	}
	path := filepath.Join(s.dir, sessionID+".jsonl")

	header := SessionHeader{
		BaseEntry: BaseEntry{Type: EntryTypeSession},
		Version:   CurrentVersion,
		AgentID:   agentID,
		CreatedAt: nowMs(),
	}
	if err := appendEntry(path, header); err != nil {
		return "", false, err
	}

	idx, _ := s.loadIndex()
	idx.Sessions[sessionID] = SessionIndexEntry{
		ID:        sessionID,
		AgentID:   agentID,
		FilePath:  sessionID + ".jsonl",
		CreatedAt: nowMs(),
		LastAt:    nowMs(),
	}
	if err := s.saveIndex(idx); err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

// Create initialises a new session file and returns its path (legacy compat).
func (s *Store) Create(sessionID, agentID string) (string, error) {
	id, _, err := s.GetOrCreate(sessionID, agentID)
	return filepath.Join(s.dir, id+".jsonl"), err
}

// AppendMessage appends a user or assistant message and updates session metadata.
func (s *Store) AppendMessage(sessionID, role string, content json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, sessionID+".jsonl")
	entry := MessageEntry{
		BaseEntry: BaseEntry{Type: EntryTypeMessage},
		Message:   Message{Role: role, Content: content},
		Timestamp: nowMs(),
	}
	if err := appendEntry(path, entry); err != nil {
		return err
	}

	// Update metadata in index
	idx, err := s.loadIndex()
	if err != nil {
		return nil // best-effort
	}
	meta, ok := idx.Sessions[sessionID]
	if !ok {
		return nil
	}
	meta.MessageCount++
	meta.LastAt = nowMs()
	meta.TokenEstimate += estimateTokensRaw(content)

	// Auto-title from first user message
	if meta.Title == "" && role == "user" {
		meta.Title = extractTitle(content)
	}
	idx.Sessions[sessionID] = meta
	return s.saveIndex(idx)
}

// ReadHistory loads all conversation turns from a session, handling compaction entries.
// Returns messages in chronological order, suitable for LLM context.
// If a compaction entry is found, the summary is returned as a synthetic "system" entry
// and only messages after the compaction boundary are included.
func (s *Store) ReadHistory(sessionID string) ([]Message, string, error) {
	path := filepath.Join(s.dir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	defer f.Close()

	var messages []Message
	var compactionSummary string
	var afterCompaction bool

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var base BaseEntry
		if err := json.Unmarshal(line, &base); err != nil {
			continue
		}
		switch base.Type {
		case EntryTypeCompaction:
			// Found compaction — reset messages, store summary
			var ce CompactionEntry
			if err := json.Unmarshal(line, &ce); err == nil {
				compactionSummary = ce.Summary
				messages = nil // clear old messages
				afterCompaction = true
			}
		case EntryTypeMessage:
			if afterCompaction || compactionSummary == "" {
				var me MessageEntry
				if err := json.Unmarshal(line, &me); err == nil {
					if me.Message.Role == "user" || me.Message.Role == "assistant" {
						messages = append(messages, me.Message)
					}
				}
			}
		}
	}
	return messages, compactionSummary, scanner.Err()
}

// EstimateTokens returns a rough token estimate for a session (from the index).
func (s *Store) EstimateTokens(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return 0
	}
	return idx.Sessions[sessionID].TokenEstimate
}

// GetMeta returns the index entry for a session.
func (s *Store) GetMeta(sessionID string) (SessionIndexEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return SessionIndexEntry{}, false
	}
	entry, ok := idx.Sessions[sessionID]
	return entry, ok
}

// Append adds a raw entry to an existing session file (legacy compat).
func (s *Store) Append(sessionID string, entry any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, sessionID+".jsonl")
	return appendEntry(path, entry)
}

// ReadAll parses all raw JSON lines from a session file.
func (s *Store) ReadAll(sessionID string) ([]json.RawMessage, error) {
	path := filepath.Join(s.dir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		entries = append(entries, append([]byte{}, line...))
	}
	return entries, scanner.Err()
}

// DeleteSession removes a session file and its index entry.
func (s *Store) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove JSONL file
	path := filepath.Join(s.dir, sessionID+".jsonl")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove from index
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	delete(idx.Sessions, sessionID)
	return s.saveIndex(idx)
}

// UpdateTitle updates the title of a session in the index.
func (s *Store) UpdateTitle(sessionID, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	entry, ok := idx.Sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	entry.Title = title
	idx.Sessions[sessionID] = entry
	return s.saveIndex(idx)
}

// ListSessions returns all session entries from the index file.
func (s *Store) ListSessions() ([]SessionIndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	result := make([]SessionIndexEntry, 0, len(idx.Sessions))
	for _, entry := range idx.Sessions {
		result = append(result, entry)
	}
	return result, nil
}

// updateIndex adds or updates a session entry in sessions.json (internal, no lock).
func (s *Store) updateIndex(sessionID, agentID, filePath string) error {
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	idx.Sessions[sessionID] = SessionIndexEntry{
		ID:        sessionID,
		AgentID:   agentID,
		FilePath:  filePath,
		CreatedAt: nowMs(),
		LastAt:    nowMs(),
	}
	return s.saveIndex(idx)
}

// loadIndex reads sessions.json or returns an empty index.
func (s *Store) loadIndex() (*SessionIndex, error) {
	indexPath := filepath.Join(s.dir, "sessions.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionIndex{Sessions: make(map[string]SessionIndexEntry)}, nil
		}
		return nil, err
	}
	var idx SessionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return &SessionIndex{Sessions: make(map[string]SessionIndexEntry)}, nil
	}
	if idx.Sessions == nil {
		idx.Sessions = make(map[string]SessionIndexEntry)
	}
	return &idx, nil
}

// saveIndex writes sessions.json to disk.
func (s *Store) saveIndex(idx *SessionIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "sessions.json"), data, 0644)
}

// appendEntry marshals v as JSON and appends a newline-terminated line.
func appendEntry(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// estimateTokensRaw estimates token count for raw JSON content (~4 chars per token).
func estimateTokensRaw(content json.RawMessage) int {
	return len(content) / 4
}

// extractTitle returns the first 60 chars of a user message as a session title.
func extractTitle(content json.RawMessage) string {
	// Try plain string first
	var s string
	if json.Unmarshal(content, &s) == nil {
		return truncateRune(s, 60)
	}
	// Try content block array
	var blocks []ContentBlock
	if json.Unmarshal(content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return truncateRune(b.Text, 60)
			}
		}
	}
	return ""
}

func truncateRune(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

// nowMs returns current Unix timestamp in milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}
