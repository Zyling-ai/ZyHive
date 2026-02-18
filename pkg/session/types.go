// Package session handles JSONL-based session storage.
// Reference: pi-coding-agent/dist/core/session-manager.js
// Session format is compatible with OpenClaw/pi-coding-agent v3.
package session

import "encoding/json"

const CurrentVersion = 3

// EntryType is the discriminator for session JSONL entries.
type EntryType string

const (
	EntryTypeSession    EntryType = "session"
	EntryTypeMessage    EntryType = "message"
	EntryTypeCompaction EntryType = "compaction"
)

// BaseEntry is the common fields for all JSONL entries.
type BaseEntry struct {
	Type     EntryType `json:"type"`
	ID       string    `json:"id,omitempty"`
	ParentID string    `json:"parentId,omitempty"`
}

// SessionHeader is the first entry in every session file.
type SessionHeader struct {
	BaseEntry
	Version   int    `json:"version"`
	AgentID   string `json:"agentId"`
	CreatedAt int64  `json:"createdAt"`
}

// MessageEntry wraps a user or assistant message.
type MessageEntry struct {
	BaseEntry
	Message   Message `json:"message"`
	Timestamp int64   `json:"timestamp"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"`
}

// ContentBlock is one element of a message's content array.
type ContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use" | "tool_result"
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ToolID string          `json:"id,omitempty"`
	Name   string          `json:"name,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// CompactionEntry records a context compression event.
type CompactionEntry struct {
	BaseEntry
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore,omitempty"`
	TokensAfter      int    `json:"tokensAfter,omitempty"`
	Timestamp        int64  `json:"timestamp"`
}

// SessionIndexEntry is one entry in the sessions.json index.
// Stored in sessions.json — lightweight metadata, no message bodies.
type SessionIndexEntry struct {
	ID            string `json:"id"`
	AgentID       string `json:"agentId"`
	FilePath      string `json:"filePath"`
	CreatedAt     int64  `json:"createdAt"`
	Title         string `json:"title,omitempty"`        // first user message (truncated)
	MessageCount  int    `json:"messageCount"`           // total user+assistant turns
	LastAt        int64  `json:"lastAt"`                 // last activity timestamp
	TokenEstimate int    `json:"tokenEstimate"`          // rough token count, triggers compaction
}
