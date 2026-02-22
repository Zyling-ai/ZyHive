// Package subagent implements background task execution and tracking for ZyHive agents.
// An agent can spawn another agent as a background "subagent" task, which runs
// asynchronously and auto-reports its result back to the requester.
package subagent

import (
	"fmt"
	"time"
)

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
	Description      string     `json:"task"`              // the task prompt
	Status           TaskStatus `json:"status"`
	Output           string     `json:"output"`            // accumulated text output
	ErrorMsg         string     `json:"error,omitempty"`
	SessionID        string     `json:"sessionId"`         // isolated session key
	SpawnedBy        string     `json:"spawnedBy,omitempty"`        // parent agent ID
	SpawnedBySession string     `json:"spawnedBySession,omitempty"` // parent session ID
	Model            string     `json:"model,omitempty"`   // overridden model
	TaskType         TaskType   `json:"taskType,omitempty"` // task | report | system
	Relation         string     `json:"relation,omitempty"` // relation type at spawn time
	CreatedAt        int64      `json:"createdAt"`         // unix ms
	StartedAt        int64      `json:"startedAt,omitempty"`
	EndedAt          int64      `json:"endedAt,omitempty"`
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

// SpawnOpts configures a new subagent task.
type SpawnOpts struct {
	AgentID          string   // target agent
	Label            string   // optional human label
	Task             string   // the task prompt
	Model            string   // optional model override
	SpawnedBy        string   // parent agent ID (for attribution)
	SpawnedBySession string   // parent session ID
	TaskType         TaskType // task | report | system
	Relation         string   // relation type at spawn time (e.g. "上下级")
}
