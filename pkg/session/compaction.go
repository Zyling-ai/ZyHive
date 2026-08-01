// Package session — context compaction logic.
// When a session's token estimate exceeds the threshold (80k tokens),
// old messages are summarized via LLM and replaced with a CompactionEntry.
package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/chatlog"
	"github.com/Zyling-ai/zyhive/pkg/persist"
	"github.com/Zyling-ai/zyhive/pkg/safefs"
)

// CompactionThreshold is the token count that triggers compaction.
// Set conservatively so that after compaction the remaining context fits
// well within any proxy/CDN timeout budget (Cloudflare: ~100s idle).
const CompactionThreshold = 50_000

var ErrSessionChanged = errors.New("session changed during compaction")

// CompactionEventFunc is a lifecycle hook invoked before/after an async
// compaction so callers (runner → SSE) can inform the user that the long
// "thinking…" gap is due to context compression, not a stuck session.
//
// phase is one of "start", "end", "error".
// info carries phase-specific fields:
//
//	start: { "tokens": int }
//	end:   { "tokens_before": int, "tokens_after": int, "summary_chars": int }
//	error: { "error": string }
type CompactionEventFunc func(phase string, info map[string]any)

// CompactIfNeeded checks if a session needs compaction and runs it asynchronously.
// Safe to call from runner after a completed turn; fires and forgets.
// workspaceDir is used to update the chatlog summary after compaction.
// onEvent (optional, P0.6) is invoked with "start"/"end"/"error" phase hooks.
func CompactIfNeeded(store *Store, sessionID string,
	callLLM func(ctx context.Context, systemPrompt, userMsg string) (string, error),
	workspaceDir string,
	onEvent CompactionEventFunc) {
	tokens := store.EstimateTokens(sessionID)
	if tokens < CompactionThreshold {
		return
	}
	log.Printf("[compaction] session %s has ~%d tokens, triggering compaction", sessionID, tokens)
	go func() {
		if onEvent != nil {
			onEvent("start", map[string]any{"tokens": tokens})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := Compact(ctx, store, sessionID, callLLM, workspaceDir); err != nil {
			log.Printf("[compaction] failed for session %s: %v", sessionID, err)
			if onEvent != nil {
				onEvent("error", map[string]any{"error": err.Error()})
			}
		} else {
			log.Printf("[compaction] completed for session %s", sessionID)
			if onEvent != nil {
				tokensAfter := store.EstimateTokens(sessionID)
				onEvent("end", map[string]any{
					"tokens_before": tokens,
					"tokens_after":  tokensAfter,
				})
			}
		}
	}()
}

// Compact performs context compaction on a session:
//  1. Reads all messages from JSONL
//  2. Keeps the last KeepTurns turns unchanged
//  3. Summarizes everything before the boundary via LLM
//  4. Writes a CompactionEntry to JSONL
//  5. Updates tokenEstimate in sessions.json
func Compact(ctx context.Context, store *Store, sessionID string, callLLM func(ctx context.Context, systemPrompt, userMsg string) (string, error), workspaceDir string) error {
	const keepTurns = 20

	snapshot, err := store.compactionSnapshot(sessionID)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	msgs := snapshot.Messages
	if len(msgs) <= keepTurns {
		return nil // nothing to compact
	}

	// Split: old (to summarize) + recent (to keep)
	boundary := len(msgs) - keepTurns
	old := msgs[:boundary]
	// recent := msgs[boundary:] // kept as-is in JSONL (not re-written)

	// Build conversation text for summarization
	var sb strings.Builder
	if snapshot.PreviousSummary != "" {
		sb.WriteString("Earlier conversation summary:\n")
		sb.WriteString(snapshot.PreviousSummary)
		sb.WriteString("\n\n")
	}
	for _, m := range old {
		label := "User"
		if m.Role == "assistant" {
			label = "Assistant"
		}
		text := extractTextFromContent(m.Content)
		if text != "" {
			sb.WriteString(label)
			sb.WriteString(": ")
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
	}

	systemPrompt := `You are a conversation summarizer. 
Produce a concise summary (max 500 words) of the conversation below that captures:
- Key topics discussed
- Important decisions or conclusions  
- Code, data, or technical context that would be needed for continuation
- The user's main goals

Be factual and preserve technical details. Reply with just the summary, no preamble.`

	state, _ := store.loadCompactionState(sessionID)
	summary := ""
	if state.Status == "prepared" && state.Generation == snapshot.Generation && state.Summary != "" {
		summary = state.Summary
	} else {
		state = compactionState{
			Version:      1,
			Status:       "summarizing",
			Generation:   snapshot.Generation,
			TokensBefore: snapshot.TokensBefore,
			Boundary:     boundary,
			UpdatedAt:    nowMs(),
		}
		if err := store.saveCompactionState(sessionID, state); err != nil {
			return fmt.Errorf("save compaction intent: %w", err)
		}
		summary, err = callLLM(ctx, systemPrompt, sb.String())
		if err != nil {
			return fmt.Errorf("llm summarize: %w", err)
		}
		summary = strings.TrimSpace(summary)
		if summary == "" {
			return fmt.Errorf("empty summary from LLM")
		}
	}
	summary = strings.TrimSpace(summary)

	// Update token estimate to post-compaction size (~summary + recent turns)
	summaryTokens := len(summary) / 4
	var recentTokens int
	for _, m := range msgs[boundary:] {
		recentTokens += len(m.Content) / 4
	}
	newEstimate := summaryTokens + recentTokens + 500 // 500 overhead
	state = compactionState{
		Version:      1,
		Status:       "prepared",
		Generation:   snapshot.Generation,
		Summary:      summary,
		TokensBefore: snapshot.TokensBefore,
		TokensAfter:  newEstimate,
		Boundary:     boundary,
		UpdatedAt:    nowMs(),
	}
	if err := store.saveCompactionState(sessionID, state); err != nil {
		return fmt.Errorf("save prepared compaction: %w", err)
	}
	if err := store.commitCompaction(sessionID, state, keepTurns); err != nil {
		return fmt.Errorf("commit compaction: %w", err)
	}

	log.Printf("[compaction] session %s: %d → %d tokens, summary: %d chars",
		sessionID, snapshot.TokensBefore, newEstimate, len(summary))

	// Update chatlog summary so the AI-visible index reflects this session's topic
	if workspaceDir != "" {
		if err := chatlog.NewManager(workspaceDir).UpdateSummary(sessionID, summary); err != nil {
			log.Printf("[compaction] chatlog UpdateSummary failed for session %s: %v", sessionID, err)
		}
	}

	return nil
}

type sessionGeneration struct {
	Size        int64 `json:"size"`
	ModUnixNano int64 `json:"modUnixNano"`
}

type compactionSnapshot struct {
	Generation      sessionGeneration
	Messages        []Message
	PreviousSummary string
	TokensBefore    int
}

type compactionState struct {
	Version      int               `json:"version"`
	Status       string            `json:"status"`
	Generation   sessionGeneration `json:"generation"`
	Summary      string            `json:"summary,omitempty"`
	TokensBefore int               `json:"tokensBefore"`
	TokensAfter  int               `json:"tokensAfter,omitempty"`
	Boundary     int               `json:"boundary"`
	UpdatedAt    int64             `json:"updatedAt"`
}

func (s *Store) compactionStatePath(sessionID string) (string, error) {
	if err := safefs.ValidateResourceID(sessionID); err != nil {
		return "", err
	}
	return safefs.ConfineToBase(s.dir, sessionID+".compaction.json")
}

func (s *Store) loadCompactionState(sessionID string) (compactionState, error) {
	path, err := s.compactionStatePath(sessionID)
	if err != nil {
		return compactionState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return compactionState{}, err
	}
	var state compactionState
	if err := json.Unmarshal(data, &state); err != nil {
		return compactionState{}, err
	}
	return state, nil
}

func (s *Store) saveCompactionState(sessionID string, state compactionState) error {
	path, err := s.compactionStatePath(sessionID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return persist.WriteFile(path, data, 0600)
}

func (s *Store) compactionSnapshot(sessionID string) (compactionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockStore()
	if err != nil {
		return compactionSnapshot{}, err
	}
	defer unlock()

	path, err := s.sessionPath(sessionID)
	if err != nil {
		return compactionSnapshot{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return compactionSnapshot{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return compactionSnapshot{}, err
	}
	var snapshot compactionSnapshot
	snapshot.Generation = sessionGeneration{Size: info.Size(), ModUnixNano: info.ModTime().UnixNano()}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var base BaseEntry
		if json.Unmarshal(line, &base) != nil {
			continue
		}
		switch base.Type {
		case EntryTypeCompaction:
			var entry CompactionEntry
			if json.Unmarshal(line, &entry) == nil {
				snapshot.PreviousSummary = entry.Summary
				snapshot.Messages = nil
			}
		case EntryTypeMessage:
			var entry MessageEntry
			if json.Unmarshal(line, &entry) == nil &&
				(entry.Message.Role == "user" || entry.Message.Role == "assistant") {
				snapshot.Messages = append(snapshot.Messages, entry.Message)
			}
		}
	}
	closeErr := file.Close()
	if err := scanner.Err(); err != nil {
		return compactionSnapshot{}, err
	}
	if closeErr != nil {
		return compactionSnapshot{}, closeErr
	}
	idx, err := s.loadIndex()
	if err == nil {
		snapshot.TokensBefore = idx.Sessions[sessionID].TokenEstimate
	}
	return snapshot, nil
}

func (s *Store) commitCompaction(sessionID string, state compactionState, keepMessages int) error {
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
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	current := sessionGeneration{Size: info.Size(), ModUnixNano: info.ModTime().UnixNano()}
	if current != state.Generation {
		return ErrSessionChanged
	}
	header, messages, err := compactableLines(path)
	if err != nil {
		return err
	}
	if len(messages) <= keepMessages {
		return nil
	}
	messages = messages[len(messages)-keepMessages:]
	entry := CompactionEntry{
		BaseEntry:        BaseEntry{Type: EntryTypeCompaction},
		Summary:          state.Summary,
		FirstKeptEntryID: fmt.Sprintf("generation-%d", state.Generation.ModUnixNano),
		TokensBefore:     state.TokensBefore,
		TokensAfter:      state.TokensAfter,
		Timestamp:        nowMs(),
	}
	entryData, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	output.Write(header)
	output.WriteByte('\n')
	output.Write(entryData)
	output.WriteByte('\n')
	for _, line := range messages {
		output.Write(line)
		output.WriteByte('\n')
	}
	if err := persist.AtomicWrite(path, output.Bytes(), 0600); err != nil {
		return err
	}
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if meta, ok := idx.Sessions[sessionID]; ok {
		meta.TokenEstimate = state.TokensAfter
		meta.MessageCount = len(messages)
		idx.Sessions[sessionID] = meta
	}
	if err := s.saveIndex(idx); err != nil {
		return err
	}
	sidecar, err := s.compactionStatePath(sessionID)
	if err == nil {
		_ = os.Remove(sidecar)
	}
	return nil
}

func compactableLines(path string) ([]byte, [][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	var header []byte
	var messages [][]byte
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 8*1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var base BaseEntry
		if json.Unmarshal(line, &base) != nil {
			continue
		}
		switch base.Type {
		case EntryTypeSession:
			if header == nil {
				header = line
			}
		case EntryTypeCompaction:
			messages = nil
		case EntryTypeMessage:
			messages = append(messages, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, fmt.Errorf("session header missing")
	}
	return header, messages, nil
}

// extractTextFromContent pulls plain text from raw message content.
func extractTextFromContent(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return string(content)
}
