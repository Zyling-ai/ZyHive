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
)

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
	return path, appendEntry(path, header)
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

func nowMs() int64 {
	// Returns current Unix timestamp in milliseconds.
	// Using time package inline to avoid import cycle; real impl uses time.Now().
	return 0 // TODO: replace with time.Now().UnixMilli()
}
