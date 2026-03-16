package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RunEvent is a single streaming event from a task execution.
type RunEvent struct {
	Type  string // "text_delta" | "error" | "done"
	Text  string
	Error error
}

// RunFunc executes a task for the given agent and streams events.
// agentID, model (empty = default), sessionID, parentSessionID, taskPrompt.
type RunFunc func(ctx context.Context, agentID, model, sessionID, parentSessionID, task string) <-chan RunEvent

// NotifyFunc is called when a task completes. It can inject a system message
// back into the parent session / send a Telegram notification.
type NotifyFunc func(spawnedBy, spawnedBySession, taskID, label, output string, status TaskStatus)

// ContextReadFn reads the last N conversation turns from the given session.
type ContextReadFn func(sessionID string, lastN int) string

// RunFuncExt is an extended RunFunc that receives the full Task for richer dispatch.
// pool.go uses this to read SharedProjectID, register artifact callbacks, etc.
// If set, takes priority over the basic RunFunc.
type RunFuncExt func(ctx context.Context, task *Task, enrichedTask string) <-chan RunEvent

// Manager manages lifecycle of all background subagent tasks.
type Manager struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	cancels  map[string]context.CancelFunc
	run      RunFunc
	notify   NotifyFunc   // optional
	storeDir string       // for persistence (optional)

	// Dispatch panel broadcasting
	broadcastFn   BroadcastFn  // optional: publishes subagent events to parent session broadcaster
	agentInfoFn   AgentInfoFn  // optional: fetches agent name/avatarColor

	// contextReadFn allows Spawn to read parent session history for ContextSnapshot.
	contextReadFn ContextReadFn // optional
	// runExt is the extended run function; takes priority over run when set.
	runExt RunFuncExt // optional

	// In-memory event history per parent session (for page-reload recovery)
	eventsMu      sync.RWMutex
	eventHistory  map[string][]SubagentEvent // parentSessionID → []SubagentEvent
}

// New creates a new Manager.
// storeDir: if non-empty, tasks are persisted to this directory.
func New(runFunc RunFunc, storeDir string) *Manager {
	m := &Manager{
		tasks:        make(map[string]*Task),
		cancels:      make(map[string]context.CancelFunc),
		run:          runFunc,
		storeDir:     storeDir,
		eventHistory: make(map[string][]SubagentEvent),
	}
	if storeDir != "" {
		if err := os.MkdirAll(storeDir, 0755); err == nil {
			m.loadFromDisk()
		}
	}
	return m
}

// SetNotify registers a completion callback.
func (m *Manager) SetNotify(fn NotifyFunc) {
	m.notify = fn
}

// SetBroadcaster registers a function that publishes subagent lifecycle events
// to the parent session's broadcaster so the UI DispatchPanel gets live updates.
func (m *Manager) SetBroadcaster(fn BroadcastFn) {
	m.broadcastFn = fn
}

// SetAgentInfoFn registers a function that fetches an agent's display name and avatar color.
func (m *Manager) SetAgentInfoFn(fn AgentInfoFn) {
	m.agentInfoFn = fn
}

// SetContextReader registers a function that reads parent session history.
func (m *Manager) SetContextReader(fn ContextReadFn) {
	m.contextReadFn = fn
}

// SetRunFuncExt registers the extended run function.
// When set, it replaces the basic RunFunc for all task executions.
func (m *Manager) SetRunFuncExt(fn RunFuncExt) {
	m.runExt = fn
}

// UpdateArtifacts stores artifact results for a task and broadcasts to parent.
// Called by the report_result tool via a registered callback.
func (m *Manager) UpdateArtifacts(taskID string, artifacts []TaskArtifact) {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	task.Artifacts = artifacts
	parentSession := task.SpawnedBySession
	agentID := task.AgentID
	sessionID := task.SessionID
	m.mu.Unlock()
	m.persist(task)

	agentName := agentID
	avatarColor := "#6366f1"
	if m.agentInfoFn != nil {
		if n, c := m.agentInfoFn(agentID); n != "" {
			agentName, avatarColor = n, c
		}
	}
	m.publishEvent(parentSession, SubagentEvent{
		Type:              "artifacts",
		SubagentSessionID: sessionID,
		AgentID:           agentID,
		AgentName:         agentName,
		AvatarColor:       avatarColor,
		Artifacts:         artifacts,
		Timestamp:         time.Now().UnixMilli(),
	})
}

// enrichTask builds the full task string from SpawnOpts:
//   1. Task brief header (background, deliverable, priority) — if Brief is set
//   2. Parent session context snapshot — if ContextSnapshot is non-empty
//   3. Attachments as reference sections
//   4. The original task instruction
func enrichTask(opts SpawnOpts) string {
	var sb strings.Builder

	hasBrief := opts.Brief != nil && (opts.Brief.Background != "" || opts.Brief.Deliverable != "" || opts.Brief.Priority != "")
	hasCtx := opts.ContextSnapshot != ""
	hasAttach := len(opts.Attachments) > 0

	if hasBrief || hasCtx || hasAttach {
		sb.WriteString("# 任务简报\n\n")
	}

	if hasBrief {
		b := opts.Brief
		if b.Priority != "" && b.Priority != "normal" {
			label := map[string]string{"high": "🔴 紧急", "low": "🟢 低优先级"}[b.Priority]
			if label == "" {
				label = b.Priority
			}
			sb.WriteString("**优先级：** " + label + "\n\n")
		}
		if b.Background != "" {
			sb.WriteString("**背景说明：**\n" + b.Background + "\n\n")
		}
		if b.Deliverable != "" {
			sb.WriteString("**期望交付物：**\n" + b.Deliverable + "\n\n")
		}
	}

	if hasCtx {
		sb.WriteString("**派遣方对话背景（最近几轮）：**\n```\n")
		sb.WriteString(opts.ContextSnapshot)
		sb.WriteString("\n```\n\n")
	}

	if hasAttach {
		sb.WriteString("**参考资料：**\n\n")
		for i, a := range opts.Attachments {
			name := a.Name
			if name == "" {
				name = fmt.Sprintf("附件%d", i+1)
			}
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", name, a.Content))
		}
	}

	if hasBrief || hasCtx || hasAttach {
		sb.WriteString("---\n\n**任务指令：**\n")
	}
	sb.WriteString(opts.Task)
	return sb.String()
}

// ReadContext reads recent conversation turns from the given session.
// Returns empty string if no ContextReadFn is configured or session is not found.
func (m *Manager) ReadContext(sessionID string, lastN int) string {
	if m.contextReadFn == nil || sessionID == "" || lastN <= 0 {
		return ""
	}
	return m.contextReadFn(sessionID, lastN)
}

// ListEvents returns stored subagent events for the given parent session ID.
// Used by the frontend to restore dispatch panel state after a page reload.
func (m *Manager) ListEvents(parentSessionID string) []SubagentEvent {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	evs := m.eventHistory[parentSessionID]
	if evs == nil {
		return []SubagentEvent{}
	}
	result := make([]SubagentEvent, len(evs))
	copy(result, evs)
	return result
}

// publishEvent broadcasts an event and appends it to the in-memory history.
func (m *Manager) publishEvent(parentSessionID string, ev SubagentEvent) {
	if parentSessionID == "" {
		return
	}
	// Store in history
	m.eventsMu.Lock()
	m.eventHistory[parentSessionID] = append(m.eventHistory[parentSessionID], ev)
	m.eventsMu.Unlock()

	// Broadcast to live subscribers (SSE)
	if m.broadcastFn != nil {
		data, err := json.Marshal(ev)
		if err == nil {
			m.broadcastFn(parentSessionID, "subagent_"+ev.Type, data)
		}
	}
}

// Spawn creates and starts a new background task. Returns the task immediately.
func (m *Manager) Spawn(opts SpawnOpts) (*Task, error) {
	if opts.AgentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	if opts.Task == "" {
		return nil, fmt.Errorf("task description is required")
	}
	taskID := uuid.New().String()[:12] // short ID for readability
	sessionID := "subagent-" + taskID

	taskType := opts.TaskType
	if taskType == "" {
		taskType = TaskTypeTask
	}

	// Enrich task with brief, context snapshot, and attachments.
	enrichedTask := enrichTask(opts)

	// Populate brief display fields
	priority := ""
	background := ""
	deliverable := ""
	if opts.Brief != nil {
		priority = opts.Brief.Priority
		background = opts.Brief.Background
		deliverable = opts.Brief.Deliverable
	}

	// If isolated, discard parent context snapshot so the session starts clean.
	if opts.Isolated {
		opts.ContextSnapshot = ""
	}

	task := &Task{
		ID:               taskID,
		AgentID:          opts.AgentID,
		Label:            opts.Label,
		Description:      opts.Task, // store original instruction (not enriched) for display
		Priority:         priority,
		Background:       background,
		Deliverable:      deliverable,
		AttachmentCount:  len(opts.Attachments),
		HasContext:       opts.ContextSnapshot != "" && !opts.Isolated,
		SharedProjectID:  opts.SharedProjectID,
		Status:           TaskPending,
		SessionID:        sessionID,
		SpawnedBy:        opts.SpawnedBy,
		SpawnedBySession: opts.SpawnedBySession,
		Model:            opts.Model,
		TaskType:         taskType,
		Relation:         opts.Relation,
		TimeoutSec:       opts.TimeoutSec,
		MaxTurns:         opts.MaxTurns,
		Isolated:         opts.Isolated,
		CreatedAt:        time.Now().UnixMilli(),
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if opts.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(opts.TimeoutSec)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	m.mu.Lock()
	m.tasks[taskID] = task
	m.cancels[taskID] = cancel
	m.mu.Unlock()

	m.persist(task)

	// Broadcast spawn event to parent session
	agentName := task.AgentID
	avatarColor := "#6366f1"
	if m.agentInfoFn != nil {
		n, c := m.agentInfoFn(task.AgentID)
		if n != "" {
			agentName = n
		}
		if c != "" {
			avatarColor = c
		}
	}
	m.publishEvent(task.SpawnedBySession, SubagentEvent{
		Type:              "spawn",
		SubagentSessionID: sessionID,
		AgentID:           task.AgentID,
		AgentName:         agentName,
		AvatarColor:       avatarColor,
		Timestamp:         time.Now().UnixMilli(),
		Priority:          task.Priority,
		Deliverable:       task.Deliverable,
		AttachmentCount:   task.AttachmentCount,
		HasContext:        task.HasContext,
	})
	_ = task.SharedProjectID // used by RunFuncExt

	go m.runTask(ctx, task, enrichedTask)
	return task, nil
}

// Kill cancels a running task.
func (m *Manager) Kill(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	cancel := m.cancels[taskID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	if task.Status != TaskRunning && task.Status != TaskPending {
		return fmt.Errorf("task %q is not running (status: %s)", taskID, task.Status)
	}
	if cancel != nil {
		cancel()
	}
	m.mu.Lock()
	task.Status = TaskKilled
	task.EndedAt = time.Now().UnixMilli()
	m.mu.Unlock()
	m.persist(task)
	return nil
}

// Get returns a task by ID.
func (m *Manager) Get(taskID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid races
	cp := *t
	return &cp, true
}

// List returns all tasks, sorted by createdAt desc.
// If agentID is non-empty, filter to that agent's tasks.
func (m *Manager) List(agentID string) []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		if agentID != "" && t.AgentID != agentID {
			continue
		}
		cp := *t
		result = append(result, &cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})
	return result
}

// runTask executes the task in a goroutine.
// enrichedTask is the full task string (with brief, context, attachments) passed to the runner;
// task.Description holds the original raw instruction for display purposes.
func (m *Manager) runTask(ctx context.Context, task *Task, enrichedTask string) {
	defer func() {
		if r := recover(); r != nil {
			m.mu.Lock()
			task.Status = TaskError
			task.ErrorMsg = fmt.Sprintf("panic: %v", r)
			task.EndedAt = time.Now().UnixMilli()
			m.mu.Unlock()
			m.persist(task)
		}
	}()

	// Mark as running
	m.mu.Lock()
	task.Status = TaskRunning
	task.StartedAt = time.Now().UnixMilli()
	m.mu.Unlock()
	m.persist(task)

	log.Printf("[subagent] task %s started: agent=%s label=%q", task.ID, task.AgentID, task.Label)

	var events <-chan RunEvent
	if m.runExt != nil {
		events = m.runExt(ctx, task, enrichedTask)
	} else {
		events = m.run(ctx, task.AgentID, task.Model, task.SessionID, task.SpawnedBySession, enrichedTask)
	}

	var outputBuf string
	var taskErr error

	for ev := range events {
		switch ev.Type {
		case "text_delta":
			m.mu.Lock()
			task.Output += ev.Text
			outputBuf = task.Output
			m.mu.Unlock()
		case "error":
			taskErr = ev.Error
		}
	}

	m.mu.Lock()
	task.EndedAt = time.Now().UnixMilli()
	if task.Status == TaskKilled {
		// already marked killed
		m.mu.Unlock()
	} else if taskErr != nil {
		task.Status = TaskError
		task.ErrorMsg = taskErr.Error()
		m.mu.Unlock()
	} else {
		task.Status = TaskDone
		m.mu.Unlock()
	}

	m.persist(task)
	log.Printf("[subagent] task %s finished: status=%s duration=%s", task.ID, task.Status, task.Duration())

	// Broadcast completion event to parent session
	agentName := task.AgentID
	avatarColor := "#6366f1"
	if m.agentInfoFn != nil {
		n, c := m.agentInfoFn(task.AgentID)
		if n != "" { agentName = n }
		if c != "" { avatarColor = c }
	}
	evType := "done"
	if task.Status == TaskError || task.Status == TaskKilled {
		evType = "error"
	}
	m.publishEvent(task.SpawnedBySession, SubagentEvent{
		Type:              evType,
		SubagentSessionID: task.SessionID,
		AgentID:           task.AgentID,
		AgentName:         agentName,
		AvatarColor:       avatarColor,
		Timestamp:         time.Now().UnixMilli(),
	})

	// Notify parent
	if m.notify != nil && task.SpawnedBy != "" {
		m.notify(task.SpawnedBy, task.SpawnedBySession, task.ID, task.Label, outputBuf, task.Status)
	}
}

// ── Persistence ────────────────────────────────────────────────────────────────

func (m *Manager) persist(task *Task) {
	if m.storeDir == "" {
		return
	}
	m.mu.RLock()
	data, err := json.Marshal(task)
	m.mu.RUnlock()
	if err != nil {
		return
	}
	path := filepath.Join(m.storeDir, task.ID+".json")
	_ = os.WriteFile(path, data, 0644)
}

func (m *Manager) loadFromDisk() {
	entries, err := os.ReadDir(m.storeDir)
	if err != nil {
		return
	}
	loaded := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.storeDir, e.Name()))
		if err != nil {
			continue
		}
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		// Mark running tasks as killed (they didn't survive the restart)
		if t.Status == TaskRunning || t.Status == TaskPending {
			t.Status = TaskKilled
			t.ErrorMsg = "server restarted"
			t.EndedAt = time.Now().UnixMilli()
		}
		m.tasks[t.ID] = &t
		loaded++
	}
	if loaded > 0 {
		log.Printf("[subagent] loaded %d tasks from disk", loaded)
	}
}
