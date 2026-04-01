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

	// Infer source from sessionID prefix for channel-originated sessions
	source := "web"
	if strings.HasPrefix(sessionID, "feishu-") {
		source = "feishu"
	} else if strings.HasPrefix(sessionID, "telegram-") || strings.HasPrefix(sessionID, "tg-") {
		source = "telegram"
	} else if strings.HasPrefix(sessionID, "ses-") {
		source = "web"
	}

	idx, _ := s.loadIndex()
	idx.Sessions[sessionID] = SessionIndexEntry{
		ID:        sessionID,
		AgentID:   agentID,
		FilePath:  sessionID + ".jsonl",
		CreatedAt: nowMs(),
		LastAt:    nowMs(),
		Source:    source,
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
	return s.AppendMessageWithTools(sessionID, role, content, nil)
}

// AppendMessageWithTools appends a message and optionally attaches display-only tool call metadata.
// ToolCalls are NOT sent to the LLM — they are stored only for UI timeline reconstruction.
func (s *Store) AppendMessageWithTools(sessionID, role string, content json.RawMessage, toolCalls []ToolCallRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, sessionID+".jsonl")
	entry := MessageEntry{
		BaseEntry: BaseEntry{Type: EntryTypeMessage},
		Message:   Message{Role: role, Content: content, ToolCalls: toolCalls},
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
	// 修复孤立 tool_use：如果尾部 assistant 消息包含 tool_use 块，
	// 但后续消息中没有对应的 tool_result，则补一条 synthetic tool_result。
	messages = fixOrphanedToolUse(messages)

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

// TrimToLastN rewrites the session JSONL keeping only the last keepMsgs messages.
// keepMsgs = keepTurns * 2 (each turn = 1 user + 1 assistant message).
// Non-message entries (session header, compaction) are preserved.
func (s *Store) TrimToLastN(sessionID string, keepMsgs int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	var headerLines [][]byte
	var msgLines [][]byte

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var base BaseEntry
		if err2 := json.Unmarshal(line, &base); err2 != nil {
			continue
		}
		if base.Type == EntryTypeMessage {
			msgLines = append(msgLines, append([]byte{}, line...))
		} else {
			headerLines = append(headerLines, append([]byte{}, line...))
		}
	}
	f.Close()
	if err := scanner.Err(); err != nil {
		return err
	}

	// Keep only last keepMsgs messages
	if len(msgLines) > keepMsgs {
		msgLines = msgLines[len(msgLines)-keepMsgs:]
	}

	// Rewrite atomically via temp file
	tmp, err := os.CreateTemp(s.dir, ".trim-*")
	if err != nil {
		return err
	}
	for _, line := range headerLines {
		fmt.Fprintf(tmp, "%s\n", line)
	}
	for _, line := range msgLines {
		fmt.Fprintf(tmp, "%s\n", line)
	}
	tmp.Close()

	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}

	// Update index
	idx, err := s.loadIndex()
	if err == nil {
		if meta, ok := idx.Sessions[sessionID]; ok {
			var tokens int
			for _, line := range msgLines {
				tokens += len(line) / 4
			}
			meta.TokenEstimate = tokens
			meta.MessageCount = len(msgLines)
			idx.Sessions[sessionID] = meta
			_ = s.saveIndex(idx)
		}
	}
	return nil
}

// nowMs returns current Unix timestamp in milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// fixOrphanedToolUse 扫描尾部 assistant 消息，如果包含没有对应 tool_result 的
// tool_use block，则自动追加一条 synthetic tool_result 消息，避免 Anthropic API
// 因历史中存在孤立 tool_use 而返回 400 错误。
//
// 典型场景：runner 在执行工具调用中途崩溃，历史尾部残留 tool_use 但缺少 tool_result。
func fixOrphanedToolUse(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}
	// 从尾部向前找最后一条 assistant 消息
	lastAssistantIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			lastAssistantIdx = i
			break
		}
	}
	if lastAssistantIdx < 0 {
		return messages
	}

	assistantMsg := messages[lastAssistantIdx]
	toolUseIDs := extractToolUseIDsFromContent(assistantMsg.Content)
	if len(toolUseIDs) == 0 {
		return messages
	}

	// 收集后续消息中的 tool_result tool_use_id 集合
	resolvedIDs := make(map[string]bool)
	for i := lastAssistantIdx + 1; i < len(messages); i++ {
		for _, id := range extractToolResultIDsFromContent(messages[i].Content) {
			resolvedIDs[id] = true
		}
	}

	// 找出未被 resolve 的 tool_use_id
	var orphaned []string
	for _, id := range toolUseIDs {
		if !resolvedIDs[id] {
			orphaned = append(orphaned, id)
		}
	}
	if len(orphaned) == 0 {
		return messages
	}

	// 为每个孤立 tool_use_id 构建一条 synthetic tool_result
	syntheticBlocks := make([]json.RawMessage, 0, len(orphaned))
	for _, id := range orphaned {
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": id,
			"is_error":    true,
			"content":     "interrupted",
		}
		if b, err := json.Marshal(block); err == nil {
			syntheticBlocks = append(syntheticBlocks, b)
		}
	}
	if len(syntheticBlocks) == 0 {
		return messages
	}
	contentRaw, err := json.Marshal(syntheticBlocks)
	if err != nil {
		return messages
	}

	synthetic := Message{
		Role:    "user",
		Content: json.RawMessage(contentRaw),
	}
	return append(messages, synthetic)
}

// extractToolUseIDsFromContent 从 content 的 JSON 中提取所有 tool_use 块的 ID。
func extractToolUseIDsFromContent(content json.RawMessage) []string {
	if len(content) == 0 {
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ToolID != "" {
			ids = append(ids, b.ToolID)
		}
	}
	return ids
}

// extractToolResultIDsFromContent 从 content 的 JSON 中提取所有 tool_result 引用的 tool_use_id。
func extractToolResultIDsFromContent(content json.RawMessage) []string {
	if len(content) == 0 {
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			ids = append(ids, b.ToolUseID)
		}
	}
	return ids
}
