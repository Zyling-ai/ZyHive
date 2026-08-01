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

	"github.com/Zyling-ai/zyhive/pkg/persist"
	"github.com/Zyling-ai/zyhive/pkg/safefs"
)

// SessionIndex maps session IDs to their file paths and metadata.
// Persisted as sessions.json in the sessions directory.
type SessionIndex struct {
	Sessions map[string]SessionIndexEntry `json:"sessions"`
}

// Store manages session files for one agent.
type Store struct {
	dir string
	mu  *sync.Mutex
}

var sessionDirLocks sync.Map

// NewStore creates a Store backed by the given directory.
func NewStore(dir string) *Store {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = filepath.Clean(abs)
	}
	value, _ := sessionDirLocks.LoadOrStore(dir, &sync.Mutex{})
	return &Store{dir: dir, mu: value.(*sync.Mutex)}
}

func (s *Store) sessionPath(sessionID string) (string, error) {
	if err := safefs.ValidateResourceID(sessionID); err != nil {
		return "", fmt.Errorf("invalid session id %q: %w", sessionID, err)
	}
	return safefs.ConfineToBase(s.dir, sessionID+".jsonl")
}

func (s *Store) lockStore() (func() error, error) {
	return persist.LockFile(filepath.Join(s.dir, ".store-transaction"))
}

// GetOrCreate returns a session ID, creating a new session if sessionID is empty or not found.
// Returns the resolved sessionID and whether it was newly created.
func (s *Store) GetOrCreate(sessionID, agentID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return "", false, err
	}
	defer unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
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
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return "", false, err
	}

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
		Source:    sessionSource(sessionID),
	}
	if err := s.saveIndex(idx); err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

// Create initialises a new session file and returns its path (legacy compat).
func (s *Store) Create(sessionID, agentID string) (string, error) {
	id, _, err := s.GetOrCreate(sessionID, agentID)
	if err != nil {
		return "", err
	}
	return s.sessionPath(id)
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
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()

	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}
	entry := MessageEntry{
		BaseEntry: BaseEntry{Type: EntryTypeMessage},
		Message:   Message{Role: role, Content: content, ToolCalls: toolCalls},
		Timestamp: nowMs(),
	}
	// Read metadata before appending. Reconciliation treats JSONL as source of
	// truth, so loading after append would already count the new line once.
	idx, indexErr := s.loadIndex()
	var meta SessionIndexEntry
	var metaExists bool
	if indexErr == nil {
		meta, metaExists = idx.Sessions[sessionID]
	}
	if err := appendEntry(path, entry); err != nil {
		return err
	}

	// Update metadata in index
	if indexErr != nil {
		return nil // best-effort
	}
	if !metaExists {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return nil, "", err
	}
	defer unlock()
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return nil, "", err
	}
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
	unlock, err := s.lockStore()
	if err != nil {
		return 0
	}
	defer unlock()
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
	unlock, err := s.lockStore()
	if err != nil {
		return SessionIndexEntry{}, false
	}
	defer unlock()
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
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}
	return appendEntry(path, entry)
}

// ReadAll parses all raw JSON lines from a session file.
func (s *Store) ReadAll(sessionID string) ([]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return nil, err
	}
	defer unlock()
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return nil, err
	}
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
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()

	// Remove JSONL file
	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}
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
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	entry, ok := idx.Sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	entry.Title = title
	entry.TitleOverridden = true // user manual rename; auto-retitle won't override
	idx.Sessions[sessionID] = entry
	return s.saveIndex(idx)
}

// UpdateAutoTitle is called by the async LLM summarizer. Unlike UpdateTitle
// it does NOT mark TitleOverridden (future runs may further refine as the
// conversation grows). Caller passes the current messageCount so subsequent
// retitle calls can dedupe via TitledAtMsgCount.
func (s *Store) UpdateAutoTitle(sessionID, title string, atMsgCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	entry, ok := idx.Sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	// Respect user's manual rename.
	if entry.TitleOverridden {
		return nil
	}
	entry.Title = title
	entry.TitledAtMsgCount = atMsgCount
	idx.Sessions[sessionID] = entry
	return s.saveIndex(idx)
}

// Retitle thresholds (message count milestones): 4, 12, 30, 80.
// The first auto-retitle triggers at 4 msgs (2 turns) — enough signal to
// summarize past "你好" openers. Later thresholds catch topic drift.
var autoRetitleThresholds = []int{4, 12, 30, 80}

// NeedsAutoRetitle returns true if the session has crossed a threshold since
// the last retitle AND title is not user-overridden.
func (s *Store) NeedsAutoRetitle(sessionID string) bool {
	meta, ok := s.GetMeta(sessionID)
	if !ok {
		return false
	}
	if meta.TitleOverridden {
		return false
	}
	mc := meta.MessageCount
	lastAt := meta.TitledAtMsgCount
	for _, th := range autoRetitleThresholds {
		if mc >= th && lastAt < th {
			return true
		}
	}
	return false
}

// ListSessions returns all session entries from the index file.
func (s *Store) ListSessions() ([]SessionIndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return nil, err
	}
	defer unlock()
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
	idx := &SessionIndex{Sessions: make(map[string]SessionIndexEntry)}
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else if json.Unmarshal(data, idx) != nil || idx.Sessions == nil {
		idx.Sessions = make(map[string]SessionIndexEntry)
	}
	if err := s.reconcileIndex(idx); err != nil {
		return nil, err
	}
	return idx, nil
}

// reconcileIndex treats JSONL files as the source of truth and sessions.json
// as a rebuildable cache. It adds files missing after an interrupted index
// update and removes entries whose source file no longer exists.
func (s *Store) reconcileIndex(idx *SessionIndex) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	existing := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if safefs.ValidateResourceID(id) != nil {
			continue
		}
		existing[id] = true
		path := filepath.Join(s.dir, entry.Name())
		current, ok := idx.Sessions[id]
		info, statErr := entry.Info()
		if ok && (statErr != nil || info.ModTime().UnixMilli() <= current.LastAt) {
			continue
		}
		rebuilt, rebuildErr := rebuildSessionMeta(path, id)
		if rebuildErr == nil {
			if ok {
				rebuilt.Title = current.Title
				rebuilt.TitleOverridden = current.TitleOverridden
				rebuilt.TitledAtMsgCount = current.TitledAtMsgCount
				rebuilt.Active = current.Active
				if current.Source != "" {
					rebuilt.Source = current.Source
				}
			}
			idx.Sessions[id] = rebuilt
		}
	}
	for id := range idx.Sessions {
		if !existing[id] {
			delete(idx.Sessions, id)
		}
	}
	return nil
}

func rebuildSessionMeta(path, sessionID string) (SessionIndexEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionIndexEntry{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return SessionIndexEntry{}, err
	}
	meta := SessionIndexEntry{
		ID:        sessionID,
		FilePath:  sessionID + ".jsonl",
		CreatedAt: info.ModTime().UnixMilli(),
		LastAt:    info.ModTime().UnixMilli(),
		Source:    sessionSource(sessionID),
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var base BaseEntry
		if json.Unmarshal(line, &base) != nil {
			continue
		}
		switch base.Type {
		case EntryTypeSession:
			var header SessionHeader
			if json.Unmarshal(line, &header) == nil {
				meta.AgentID = header.AgentID
				meta.CreatedAt = header.CreatedAt
			}
		case EntryTypeCompaction:
			var compaction CompactionEntry
			if json.Unmarshal(line, &compaction) == nil {
				meta.MessageCount = 0
				meta.TokenEstimate = len(compaction.Summary)/4 + 500
			}
		case EntryTypeMessage:
			var message MessageEntry
			if json.Unmarshal(line, &message) != nil {
				continue
			}
			meta.MessageCount++
			meta.TokenEstimate += estimateTokensRaw(message.Message.Content)
			if message.Timestamp > meta.LastAt {
				meta.LastAt = message.Timestamp
			}
			if meta.Title == "" && message.Message.Role == "user" {
				meta.Title = extractTitle(message.Message.Content)
			}
		}
	}
	return meta, scanner.Err()
}

func sessionSource(sessionID string) string {
	switch {
	case strings.HasPrefix(sessionID, "feishu-"):
		return "feishu"
	case strings.HasPrefix(sessionID, "telegram-"), strings.HasPrefix(sessionID, "tg-"):
		return "telegram"
	default:
		return "web"
	}
}

// saveIndex writes sessions.json to disk.
func (s *Store) saveIndex(idx *SessionIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return persist.WriteFile(filepath.Join(s.dir, "sessions.json"), data, 0o600)
}

// appendEntry marshals v as JSON and appends a newline-terminated line.
func appendEntry(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	return persist.WithFileLock(path, func() error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	})
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
	unlock, err := s.lockStore()
	if err != nil {
		return err
	}
	defer unlock()

	path, err := s.sessionPath(sessionID)
	if err != nil {
		return err
	}
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
