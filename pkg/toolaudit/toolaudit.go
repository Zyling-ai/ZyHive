// Package toolaudit — per-agent, append-only JSONL log of every tool call,
// stored with full (un-truncated) input + result so users can drill into a
// tool card and see what the agent actually did.
//
// File layout, rooted at agents/{agentId}/tool-audit/:
//
//	tool-audit/
//	  2026-05-12.jsonl          ← one row per tool call, ts in filename
//	  2026-05-13.jsonl
//	  blobs/
//	    abc123_input.bin        ← overflow input (>InlineCapBytes)
//	    abc123_result.bin       ← overflow result
//
// Why two writes? Most tool calls fit inline; only large reads/exec outputs
// blob out. Keeps the JSONL trivially grepable for the 99% case.
//
// All operations are no-ops when called on a nil *Log so callers can hold
// an optional reference (e.g. unit tests skip audit by passing nil).
//
// Added 26.5.12v1 (F-03).

package toolaudit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// InlineCapBytes — anything bigger than this for either input or result is
// spilled to blobs/.
const InlineCapBytes = 200 * 1024 // 200 KiB

// Entry is the canonical row written to the JSONL file. Either inline fields
// (Input/Result) are set, or the Ref counterparts point to blobs/. They are
// mutually exclusive per field.
type Entry struct {
	Timestamp   int64           `json:"ts"`           // UnixMilli
	AgentID     string          `json:"agentId"`
	SessionID   string          `json:"sessionId,omitempty"`
	ToolCallID  string          `json:"toolCallId"`
	Name        string          `json:"name"`
	Input       json.RawMessage `json:"input,omitempty"`
	InputRef    string          `json:"inputRef,omitempty"`
	Result      string          `json:"result,omitempty"`
	ResultRef   string          `json:"resultRef,omitempty"`
	DurationMs  int             `json:"durationMs"`
	Error       string          `json:"error,omitempty"`
}

// Log is the per-agent handle. Construct with New; nil-safe.
type Log struct {
	agentDir string
	mu       sync.Mutex
}

// New returns a Log rooted at the given agent dir. Files are created lazily.
// Pass an empty agentDir to disable (returns a nil-equivalent Log that no-ops).
func New(agentDir string) *Log {
	if agentDir == "" {
		return nil
	}
	return &Log{agentDir: agentDir}
}

// Dir returns the absolute path of the tool-audit/ directory.
func (l *Log) Dir() string {
	if l == nil {
		return ""
	}
	return filepath.Join(l.agentDir, "tool-audit")
}

// blobsDir returns the path of tool-audit/blobs/.
func (l *Log) blobsDir() string {
	return filepath.Join(l.Dir(), "blobs")
}

// fileFor returns the JSONL path for a UTC date.
func (l *Log) fileFor(ts time.Time) string {
	return filepath.Join(l.Dir(), ts.UTC().Format("2006-01-02")+".jsonl")
}

// ensureDir creates tool-audit/ and tool-audit/blobs/ if missing.
func (l *Log) ensureDir() error {
	if err := os.MkdirAll(l.blobsDir(), 0o700); err != nil {
		return err
	}
	return nil
}

// Append writes an entry. Large input/result are spilled to blobs/.
func (l *Log) Append(e Entry) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureDir(); err != nil {
		return err
	}
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixMilli()
	}
	if e.ToolCallID == "" {
		return errors.New("toolaudit.Append: empty ToolCallID")
	}
	// Input overflow → blob.
	if len(e.Input) > InlineCapBytes {
		blobName := safeBlobName(e.ToolCallID) + "_input.bin"
		if err := os.WriteFile(filepath.Join(l.blobsDir(), blobName), e.Input, 0o600); err != nil {
			return err
		}
		e.InputRef = blobName
		e.Input = nil
	}
	// Result overflow → blob.
	if len(e.Result) > InlineCapBytes {
		blobName := safeBlobName(e.ToolCallID) + "_result.bin"
		if err := os.WriteFile(filepath.Join(l.blobsDir(), blobName), []byte(e.Result), 0o600); err != nil {
			return err
		}
		e.ResultRef = blobName
		e.Result = ""
	}
	// Append.
	path := l.fileFor(time.UnixMilli(e.Timestamp))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

// GetByID returns the entry with the matching ToolCallID, scanning the most
// recent N days (default 14). Heavy queries should re-implement; this is the
// "click drawer in chat → fetch full data" path.
func (l *Log) GetByID(toolCallID string) (*Entry, error) {
	if l == nil {
		return nil, nil
	}
	const lookbackDays = 14
	files, err := l.recentFiles(lookbackDays)
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		hit, err := l.scanFileForID(path, toolCallID)
		if err != nil {
			continue
		}
		if hit != nil {
			return l.materialize(hit), nil
		}
	}
	return nil, nil
}

// ListBySession returns the last `limit` entries for a session, scanning the
// most recent N days. Caller-supplied limit is capped at 500.
func (l *Log) ListBySession(sessionID string, limit int) ([]Entry, error) {
	if l == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	files, err := l.recentFiles(14)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, path := range files {
		entries, err := l.scanFile(path, func(e *Entry) bool {
			return e.SessionID == sessionID
		})
		if err != nil {
			continue
		}
		for _, e := range entries {
			out = append(out, *e)
		}
	}
	// Sort by ts desc, then trim.
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListAll returns the last `limit` entries across all sessions matching the
// optional filter. Used by the admin ToolAuditView.
type ListFilter struct {
	SessionID string
	ToolName  string
	DateFrom  time.Time // inclusive (UTC)
	DateTo    time.Time // inclusive (UTC); zero means "today"
}

// ListAll scans days in [DateFrom, DateTo] and returns up to `limit` entries
// matching the filter, newest first.
func (l *Log) ListAll(filter ListFilter, limit, offset int) ([]Entry, int, error) {
	if l == nil {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	now := time.Now().UTC()
	to := filter.DateTo
	if to.IsZero() {
		to = now
	}
	from := filter.DateFrom
	if from.IsZero() {
		from = to.AddDate(0, 0, -7)
	}
	// Iterate day-by-day from `to` backwards to `from`.
	var collected []Entry
	for d := startOfDay(to); !d.Before(startOfDay(from)); d = d.AddDate(0, 0, -1) {
		path := l.fileFor(d)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		matches, err := l.scanFile(path, func(e *Entry) bool {
			if filter.SessionID != "" && e.SessionID != filter.SessionID {
				return false
			}
			if filter.ToolName != "" && !strings.EqualFold(e.Name, filter.ToolName) {
				return false
			}
			return true
		})
		if err != nil {
			continue
		}
		for _, e := range matches {
			collected = append(collected, *e)
		}
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].Timestamp > collected[j].Timestamp })
	total := len(collected)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return collected[offset:end], total, nil
}

// materialize reads any *_input.bin / *_result.bin referenced by the entry
// and inlines the bytes back. Used by GetByID — for ListAll/ListBySession we
// keep the blob refs intact (caller can fetch on demand).
func (l *Log) materialize(e *Entry) *Entry {
	if e.InputRef != "" {
		raw, err := os.ReadFile(filepath.Join(l.blobsDir(), e.InputRef))
		if err == nil {
			e.Input = raw
			e.InputRef = ""
		}
	}
	if e.ResultRef != "" {
		raw, err := os.ReadFile(filepath.Join(l.blobsDir(), e.ResultRef))
		if err == nil {
			e.Result = string(raw)
			e.ResultRef = ""
		}
	}
	return e
}

// recentFiles returns paths of the last N daily files (newest first).
func (l *Log) recentFiles(days int) ([]string, error) {
	now := time.Now().UTC()
	out := make([]string, 0, days)
	for i := 0; i < days; i++ {
		path := l.fileFor(now.AddDate(0, 0, -i))
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out, nil
}

func (l *Log) scanFile(path string, keep func(*Entry) bool) ([]*Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024) // up to 8 MiB per line for safety
	var out []*Entry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if keep == nil || keep(&e) {
			cp := e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (l *Log) scanFileForID(path, id string) (*Entry, error) {
	entries, err := l.scanFile(path, func(e *Entry) bool { return e.ToolCallID == id })
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return entries[0], nil
}

// safeBlobName turns a ToolCallID into a filesystem-safe filename stem.
// Anthropic ToolCallIDs use `toolu_xxx` ASCII; we still sanitise for safety.
func safeBlobName(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("anon_%d", time.Now().UnixNano())
	}
	return b.String()
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
