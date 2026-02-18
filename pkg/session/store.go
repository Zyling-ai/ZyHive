// Store provides append-only JSONL session read/write.
// Reference: pi-coding-agent/dist/core/session-manager.js
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionIndex maps session IDs to their file paths.
// Persisted as sessions.json in the sessions directory.
type SessionIndex struct {
	Sessions map[string]SessionIndexEntry `json:"sessions"`
}

// SessionIndexEntry is one entry in the sessions.json index.
type SessionIndexEntry struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	FilePath  string `json:"filePath"`
	CreatedAt int64  `json:"createdAt"`
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

// Create initialises a new session file and returns its path.
func (s *Store) Create(sessionID, agentID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(s.dir, sessionID+".jsonl")

	header := SessionHeader{
		BaseEntry: BaseEntry{Type: EntryTypeSession},
		Version:   CurrentVersion,
		AgentID:   agentID,
		CreatedAt: nowMs(),
	}
	if err := appendEntry(path, header); err != nil {
		return "", err
	}

	// Update sessions.json index
	if err := s.updateIndex(sessionID, agentID, path); err != nil {
		return "", fmt.Errorf("update session index: %w", err)
	}

	return path, nil
}

// Append adds a new entry to an existing session file.
func (s *Store) Append(sessionID string, entry any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, sessionID+".jsonl")
	return appendEntry(path, entry)
}

// ReadAll parses all entries from a session file.
func (s *Store) ReadAll(sessionID string) ([]json.RawMessage, error) {
	path := filepath.Join(s.dir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		entries = append(entries, append([]byte{}, line...))
	}
	return entries, scanner.Err()
}

// ListSessions returns all session entries from the index file.
func (s *Store) ListSessions() ([]SessionIndexEntry, error) {
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

// updateIndex adds or updates a session entry in sessions.json.
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

// nowMs returns current Unix timestamp in milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}
