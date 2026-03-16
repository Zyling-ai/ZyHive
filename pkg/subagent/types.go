// Package subagent implements background task execution and tracking for ZyHive agents.
// An agent can spawn another agent as a background "subagent" task, which runs
// asynchronously and auto-reports its result back to the requester.
package subagent

import (
	"fmt"
	"time"
)

// BroadcastFn is a function that publishes an event to a session's broadcaster.
// sessionID is the target session; eventType is the SSE event name; data is JSON payload.
type BroadcastFn func(sessionID string, eventType string, data []byte)

// AgentInfoFn fetches minimal agent info (name, avatarColor) by agentID.
// Returns empty strings if the agent is not found.
type AgentInfoFn func(agentID string) (name, avatarColor string)

// SubagentEvent is the unified SSE event format sent to the parent session's broadcaster.
type SubagentEvent struct {
	Type              string `json:"type"`              // "spawn"|"report"|"done"|"error"
	SubagentSessionID string `json:"subagentSessionId"`
	AgentID           string `json:"agentId"`
	AgentName         string `json:"agentName"`
	AvatarColor       string `json:"avatarColor"`
	Content           string `json:"content,omitempty"`
	Status            string `json:"status,omitempty"`
	Progress          int    `json:"progress,omitempty"`
	Timestamp         int64  `json:"timestamp"`

	// Brief metadata for DispatchPanel display (set on spawn events)
	Priority        string `json:"priority,omitempty"`
	Deliverable     string `json:"deliverable,omitempty"`
	AttachmentCount int    `json:"attachmentCount,omitempty"`
	HasContext      bool   `json:"hasContext,omitempty"`
	// Artifact events: set when executor calls report_result
	Artifacts []TaskArtifact `json:"artifacts,omitempty"`
}

// TaskStatus represents the lifecycle state of a subagent task.
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskError   TaskStatus = "error"
	TaskKilled  TaskStatus = "killed"
)

// Task is a background task executed by a subagent.
type Task struct {
	ID               string     `json:"id"`
	AgentID          string     `json:"agentId"`           // which agent runs this task
	Label            string     `json:"label,omitempty"`   // human-readable label
	Description      string     `json:"task"`              // the raw task instruction (for display)
	Status           TaskStatus `json:"status"`
	Output           string     `json:"output"`            // accumulated text output
	ErrorMsg         string     `json:"error,omitempty"`
	SessionID        string     `json:"sessionId"`         // isolated session key
	SpawnedBy        string     `json:"spawnedBy,omitempty"`        // parent agent ID
	SpawnedBySession string     `json:"spawnedBySession,omitempty"` // parent session ID
	Model            string     `json:"model,omitempty"`      // overridden model
	TaskType         TaskType   `json:"taskType,omitempty"`   // task | report | system
	Relation         string     `json:"relation,omitempty"`   // relation type at spawn time
	TimeoutSec       int        `json:"timeoutSec,omitempty"` // 0 = no limit
	MaxTurns         int        `json:"maxTurns,omitempty"`   // 0 = no limit
	Isolated         bool       `json:"isolated,omitempty"`   // clean session, no parent context

	// Brief metadata for display in DispatchPanel
	Background      string `json:"background,omitempty"`
	Deliverable     string `json:"deliverable,omitempty"`
	Priority        string `json:"priority,omitempty"`
	AttachmentCount int    `json:"attachmentCount,omitempty"`
	HasContext      bool   `json:"hasContext,omitempty"`
	SharedProjectID string `json:"sharedProjectId,omitempty"`

	// Structured output from executor (populated via report_result tool)
	Artifacts []TaskArtifact `json:"artifacts,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	StartedAt int64 `json:"startedAt,omitempty"`
	EndedAt   int64 `json:"endedAt,omitempty"`
}

// Duration returns a human-readable elapsed time string.
func (t *Task) Duration() string {
	if t.StartedAt == 0 {
		return "—"
	}
	end := t.EndedAt
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	d := time.Duration(end-t.StartedAt) * time.Millisecond
	if d < time.Second {
		return "< 1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// TaskType classifies the intent of a task.
type TaskType string

const (
	TaskTypeTask   TaskType = "task"   // superior → subordinate delegation
	TaskTypeReport TaskType = "report" // subordinate → superior report
	TaskTypeSystem TaskType = "system" // internal / cron-triggered
)

// Attachment is a piece of material attached to a task.
type Attachment struct {
	// Name is the display name shown in the briefing (e.g. filename or label).
	Name string
	// Content is the text content of the attachment (resolved before Spawn).
	Content string
}

// TaskArtifact is an output file produced by the executor agent.
// Written to a shared project and reported back via the report_result tool.
type TaskArtifact struct {
	Name      string `json:"name"`      // display name
	Path      string `json:"path"`      // project-relative file path
	ProjectID string `json:"projectId"` // which shared project it belongs to
	Type      string `json:"type"`      // "code" | "report" | "data" | "file"
	Size      int    `json:"size,omitempty"`
}

// TaskBrief enriches a task with structured metadata beyond the raw instruction.
// All fields are optional; non-empty fields are injected into the task briefing.
type TaskBrief struct {
	// Background explains why this task is needed / what the bigger context is.
	Background string
	// Deliverable describes what the output should look like.
	Deliverable string
	// Priority is "high" | "normal" | "low". Default is "normal".
	Priority string
}

// SpawnOpts configures a new subagent task.
type SpawnOpts struct {
	AgentID          string   // target agent
	Label            string   // optional human label
	Task             string   // the task prompt / instruction
	Model            string   // optional model override (e.g. "anthropic/claude-opus-4-5")
	SpawnedBy        string   // parent agent ID (for attribution)
	SpawnedBySession string   // parent session ID
	TaskType         TaskType // task | report | system
	Relation         string   // relation type at spawn time (e.g. "上下级")

	// TimeoutSec limits how long the subagent can run (0 = no limit).
	TimeoutSec int
	// MaxTurns limits the number of LLM turns (0 = no limit).
	MaxTurns int
	// Isolated: if true, the subagent starts with a clean session (no parent context snapshot).
	Isolated bool

	// Brief adds structured context beyond the raw task instruction.
	Brief *TaskBrief
	// Attachments are reference materials injected into the task briefing.
	Attachments []Attachment
	// ContextSnapshot is the recent conversation history from the parent session,
	// pre-resolved by the caller.
	ContextSnapshot string
	// SharedProjectID grants the spawned agent write access to this project.
	// The executor can use project_write to deposit output files there.
	SharedProjectID string
}
