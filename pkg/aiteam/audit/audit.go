// Package audit provides an append-only JSONL audit log shared by every
// aiteam (experimental) subsystem: wallet, payroll, judge, revenue,
// guard, promptdef, sandbox.
//
// The file is rotated by line count (default 50k lines) and lives at
// <dataDir>/aiteam/audit.log. Concurrent writes from multiple goroutines
// are serialised via a single sync.Mutex; the package supports a
// process-global default audit instance plus user-constructed ones for
// tests.
//
// Why a dedicated file (not slog text/json stream)?
//   * Single grep-able trail for "everything that touched aiteam money /
//     security policy". Useful for forensics.
//   * Distinct from the operator-facing journalctl log; we don't want
//     audit events to be lost in routine info spam.
//   * Append-only JSONL is trivial to ship to S3 / SIEM later.
//
// All operations are no-op when called on a nil *Log so call sites can
// hold an optional reference.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultRotateLines is the line count at which the audit log rotates
// (i.e. renames the file to audit.log.<timestamp> and starts a fresh one).
const DefaultRotateLines = 50_000

// Entry is the canonical shape of an audit row. Subsystems should
// populate Type with a stable verb such as "wallet.credit", "guard.panic",
// "judge.score", "promptdef.hit" so consumers can grep deterministically.
//
// Detail is a free-form map; keep it small (<1 KB per entry) to keep
// rotation predictable.
type Entry struct {
	Type      string         `json:"type"`
	Subsystem string         `json:"subsystem"`
	AgentID   string         `json:"agentId,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	Timestamp int64          `json:"ts"`           // UnixMilli
	Detail    map[string]any `json:"detail,omitempty"`
}

// Log is the append-only writer. It is safe for concurrent use.
type Log struct {
	path        string
	rotateAfter int
	mu          sync.Mutex
	lineCount   int
}

// New creates (or opens for append) an audit log at dir/audit.log.
// The directory is created with 0o700 (operator-only) if missing.
func New(dir string) (*Log, error) {
	if dir == "" {
		return nil, fmt.Errorf("audit: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("audit: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "audit.log")
	// Count existing lines so we know when to rotate. Cheap O(n) on
	// startup, then incremental.
	count, _ := countLines(path)
	return &Log{path: path, rotateAfter: DefaultRotateLines, lineCount: count}, nil
}

// SetRotateAfter overrides the rotate-after-N-lines threshold (mainly
// for tests).
func (l *Log) SetRotateAfter(n int) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotateAfter = n
}

// Append writes one entry. Errors are returned to the caller so
// security-critical paths can choose to fail loudly; aiteam subsystems
// typically log the error to slog and continue rather than crashing the
// whole agent on a disk hiccup.
//
// If l is nil the call is a no-op (returns nil).
func (l *Log) Append(e Entry) error {
	if l == nil {
		return nil
	}
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixMilli()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lineCount >= l.rotateAfter {
		if err := l.rotateLocked(); err != nil {
			return fmt.Errorf("audit: rotate: %w", err)
		}
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}
	l.lineCount++
	return nil
}

// Path returns the active audit log path (mainly for diagnostics).
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// LineCount returns the current line count (cheap; cached in-memory).
func (l *Log) LineCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lineCount
}

// Tail returns the last `n` entries from the audit log, oldest-of-tail
// first. Reads the file from the end using a chunked back-scan so it
// stays fast even for fully-rotated 50k-line files.
//
// Lines that fail JSON parse are silently skipped (resilient to
// partial-write corruption at the tail). When the file does not exist
// or n <= 0, returns an empty slice.
func (l *Log) Tail(n int) ([]Entry, error) {
	if l == nil || n <= 0 {
		return []Entry{}, nil
	}
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return []Entry{}, nil
	}

	// Read backwards in 8 KiB chunks until we collect n+1 newline
	// boundaries (n+1 because the trailing newline counts as the end of
	// the last entry). The captured `tail` buffer is then forward-split
	// and the last n lines parsed.
	const chunk = 8 * 1024
	var (
		buf      []byte
		pos      = size
		newlines = 0
	)
	for pos > 0 && newlines <= n {
		readSize := int64(chunk)
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		seg := make([]byte, readSize)
		if _, err := f.ReadAt(seg, pos); err != nil {
			return nil, err
		}
		buf = append(seg, buf...) // prepend
		for _, b := range seg {
			if b == '\n' {
				newlines++
			}
		}
	}

	// Split into lines; keep only the last n non-empty lines.
	lines := splitJSONL(buf)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]Entry, 0, len(lines))
	for _, line := range lines {
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip partial / corrupt lines
		}
		out = append(out, e)
	}
	return out, nil
}

// splitJSONL splits the buffer on '\n' and drops empty lines. Used by
// Tail; cheap O(n).
func splitJSONL(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// rotateLocked renames the active log to <path>.<ts> and resets the line
// count. Caller must hold l.mu.
func (l *Log) rotateLocked() error {
	stamp := time.Now().UTC().Format("20060102-150405")
	rotated := l.path + "." + stamp
	if err := os.Rename(l.path, rotated); err != nil && !os.IsNotExist(err) {
		return err
	}
	l.lineCount = 0
	return nil
}

// countLines is a quick line-counter used at startup.
func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count, nil
}
