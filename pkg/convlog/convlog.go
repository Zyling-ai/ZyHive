// Package convlog — permanent conversation audit log.
// Separate from agent session memory: agent cannot see this log.
// Each agent gets one JSONL file per channel: agents/{id}/convlogs/{channelId}.jsonl
package convlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Entry is a single message in the conversation log.
type Entry struct {
	Timestamp   string `json:"ts"`
	Role        string `json:"role"`             // "user" | "assistant"
	Content     string `json:"content"`
	ChannelID   string `json:"channelId"`
	ChannelType string `json:"channelType"`      // "telegram" | "web" | "api"
	Sender      string `json:"sender,omitempty"` // user ID or name
}

// ConvLog handles appending to and reading from a channel's conversation log.
type ConvLog struct {
	agentDir  string
	channelID string
}

// New creates a ConvLog for a given agent directory and channel ID.
func New(agentDir, channelID string) *ConvLog {
	return &ConvLog{agentDir: agentDir, channelID: channelID}
}

// path returns the JSONL file path: {agentDir}/convlogs/{channelID}.jsonl
func (cl *ConvLog) path() string {
	// sanitize channelID: replace path separators so it's safe as a filename
	safe := strings.NewReplacer("/", "-", "\\", "-").Replace(cl.channelID)
	return filepath.Join(cl.agentDir, "convlogs", safe+".jsonl")
}

// Append writes a new entry to the log (appends to JSONL file).
// Creates the convlogs/ directory and file if needed.
// Uses O_APPEND which provides atomic appends for small writes on most OSes.
func (cl *ConvLog) Append(entry Entry) error {
	p := cl.path()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// ChannelSummary holds summary info for one channel's conversation log.
type ChannelSummary struct {
	ChannelID    string `json:"channelId"`
	ChannelType  string `json:"channelType"`
	MessageCount int    `json:"messageCount"`
	LastAt       string `json:"lastAt"`
	FirstAt      string `json:"firstAt"`
}

// ListChannels scans all *.jsonl files in agentDir/convlogs/ and returns summaries.
func ListChannels(agentDir string) ([]ChannelSummary, error) {
	dir := filepath.Join(agentDir, "convlogs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ChannelSummary{}, nil
		}
		return nil, err
	}

	var summaries []ChannelSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		channelID := strings.TrimSuffix(e.Name(), ".jsonl")
		p := filepath.Join(dir, e.Name())
		msgs, _, err := readAllEntries(p)
		if err != nil {
			continue
		}
		sum := ChannelSummary{
			ChannelID:    channelID,
			MessageCount: len(msgs),
		}
		if len(msgs) > 0 {
			sum.FirstAt = msgs[0].Timestamp
			sum.LastAt = msgs[len(msgs)-1].Timestamp
			sum.ChannelType = msgs[0].ChannelType
		}
		summaries = append(summaries, sum)
	}
	if summaries == nil {
		summaries = []ChannelSummary{}
	}
	return summaries, nil
}

// ReadMessages reads all entries from a channel log with optional pagination.
// Returns: entries slice, total count, error.
// If limit <= 0, return all entries. offset is 0-based from the beginning.
func ReadMessages(agentDir, channelID string, limit, offset int) ([]Entry, int, error) {
	safe := strings.NewReplacer("/", "-", "\\", "-").Replace(channelID)
	p := filepath.Join(agentDir, "convlogs", safe+".jsonl")
	all, total, err := readAllEntries(p)
	if err != nil {
		return nil, 0, err
	}
	if offset >= total {
		return []Entry{}, total, nil
	}
	slice := all[offset:]
	if limit > 0 && limit < len(slice) {
		slice = slice[:limit]
	}
	return slice, total, nil
}

// readAllEntries reads all valid JSONL entries from a file.
func readAllEntries(p string) ([]Entry, int, error) {
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	// Allow large lines (up to 1MB for long messages)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, len(entries), scanner.Err()
}
