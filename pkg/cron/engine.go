// Package cron provides the scheduled job engine.
// Reference: openclaw/src/cron/service.ts, schedule.ts
package cron

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	cron "github.com/robfig/cron/v3"
)

// CronRunFunc executes an agent turn in an isolated session and returns the full text response.
// agentID, model (empty = default), jobID, runID, message.
// Each call MUST create a fresh session (sessionID = "cron-{jobID}-{runID}") so
// cron jobs never pollute the main conversation history.
type CronRunFunc func(ctx context.Context, agentID, model, jobID, runID, message string) (string, error)

// AnnounceFunc delivers the completed job output to the user (e.g. sends a Telegram message).
// Called only when delivery.mode == "announce" and output is not suppressed.
type AnnounceFunc func(agentID, jobName, output string)

// SilentToken — if the agent's output starts with (or equals) this token, the
// result is recorded but NOT announced. Agents use this to signal "nothing to report".
const SilentToken = "NO_ALERT"

// ── Job types ─────────────────────────────────────────────────────────────

type Job struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Remark      string   `json:"remark,omitempty"`
	Enabled     bool     `json:"enabled"`
	Schedule    Schedule `json:"schedule"`
	Payload     Payload  `json:"payload"`
	Delivery    Delivery `json:"delivery"`
	AgentID     string   `json:"agentId"`
	CreatedAtMs int64    `json:"createdAtMs"`
	State       JobState `json:"state"`
}

type Schedule struct {
	Kind    string `json:"kind"`              // "cron" | "every" | "at"
	Expr    string `json:"expr,omitempty"`    // cron expression (kind=cron/at)
	EveryMs int64  `json:"everyMs,omitempty"` // interval in ms (kind=every); e.g. 300000 = 5 min
	TZ      string `json:"tz,omitempty"`      // timezone, e.g. "Asia/Shanghai"
}

type Payload struct {
	Kind    string `json:"kind"`            // "agentTurn" | "systemEvent"
	Message string `json:"message"`         // the prompt to send to the agent
	Model   string `json:"model,omitempty"` // optional model override
}

type Delivery struct {
	// "announce" — send output to user via AnnounceFunc (unless agent outputs SilentToken)
	// "none"     — silently record; agent must call send_message tool to push notifications
	Mode string `json:"mode"` // "announce" | "none"
}

type JobState struct {
	NextRunAtMs int64  `json:"nextRunAtMs,omitempty"`
	LastRunAtMs int64  `json:"lastRunAtMs,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"` // "ok" | "error"
}

type RunRecord struct {
	JobID     string `json:"jobId"`
	RunID     string `json:"runId"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt"`
	Status    string `json:"status"` // "ok" | "error"
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
	Announced bool   `json:"announced,omitempty"` // true if delivered to user
}

// ── Engine ────────────────────────────────────────────────────────────────

type Engine struct {
	cron     *cron.Cron
	jobs     map[string]*Job
	entryIDs map[string]cron.EntryID
	jobMu    sync.RWMutex
	dataDir  string

	// runJob executes a job in an isolated session (each run gets a fresh context).
	runJob CronRunFunc

	// announce delivers output to the user when delivery.mode == "announce".
	announce AnnounceFunc
}

// NewEngine creates a new cron engine.
//   - runJob:   isolated session runner (see CronRunFunc)
//   - announce: output delivery callback; may be nil (disables announce mode)
func NewEngine(dataDir string, runJob CronRunFunc, announce AnnounceFunc) *Engine {
	return &Engine{
		cron:     cron.New(cron.WithSeconds()),
		jobs:     make(map[string]*Job),
		entryIDs: make(map[string]cron.EntryID),
		dataDir:  dataDir,
		runJob:   runJob,
		announce: announce,
	}
}

// Load reads jobs.json from disk and schedules all enabled jobs.
func (e *Engine) Load() error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(e.dataDir, "runs"), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(e.dataDir, "jobs.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("parse jobs.json: %w", err)
	}

	for _, j := range jobs {
		e.jobs[j.ID] = j
		if j.Enabled {
			e.scheduleJobLocked(j)
		}
	}

	e.cron.Start()
	return nil
}

func (e *Engine) Start() { e.cron.Start() }

func (e *Engine) Stop() context.Context { return e.cron.Stop() }

// Add adds a new job, persists to disk, and schedules it if enabled.
func (e *Engine) Add(job *Job) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	if job.ID == "" {
		job.ID = "job-" + uuid.New().String()[:8]
	}
	if job.CreatedAtMs == 0 {
		job.CreatedAtMs = time.Now().UnixMilli()
	}
	e.jobs[job.ID] = job
	if job.Enabled {
		e.scheduleJobLocked(job)
	}
	return e.saveLocked()
}

// Update patches a job, reschedules, and saves.
func (e *Engine) Update(id string, patch *Job) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	existing, ok := e.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	e.unscheduleJobLocked(id)

	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Remark != "" {
		existing.Remark = patch.Remark
	}
	existing.Enabled = patch.Enabled
	if patch.Schedule.Expr != "" || patch.Schedule.EveryMs > 0 {
		existing.Schedule = patch.Schedule
	}
	if patch.Payload.Message != "" {
		existing.Payload = patch.Payload
	}
	if patch.Delivery.Mode != "" {
		existing.Delivery = patch.Delivery
	}
	if patch.AgentID != "" {
		existing.AgentID = patch.AgentID
	}

	if existing.Enabled {
		e.scheduleJobLocked(existing)
	}
	return e.saveLocked()
}

// Remove deletes a job and unschedules it.
func (e *Engine) Remove(id string) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	if _, ok := e.jobs[id]; !ok {
		return fmt.Errorf("job %q not found", id)
	}
	e.unscheduleJobLocked(id)
	delete(e.jobs, id)
	return e.saveLocked()
}

// RunNow triggers a job immediately in a goroutine.
func (e *Engine) RunNow(id string) error {
	e.jobMu.RLock()
	job, ok := e.jobs[id]
	if !ok {
		e.jobMu.RUnlock()
		return fmt.Errorf("job %q not found", id)
	}
	j := *job
	e.jobMu.RUnlock()

	go e.executeJob(&j)
	return nil
}

// ListJobs returns all jobs.
func (e *Engine) ListJobs() []*Job {
	e.jobMu.RLock()
	defer e.jobMu.RUnlock()
	result := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		result = append(result, j)
	}
	return result
}

// ListJobsByAgent returns jobs for a specific agent ("*" = all).
func (e *Engine) ListJobsByAgent(agentID string) []*Job {
	e.jobMu.RLock()
	defer e.jobMu.RUnlock()
	result := make([]*Job, 0)
	for _, j := range e.jobs {
		if agentID == "*" || j.AgentID == agentID {
			result = append(result, j)
		}
	}
	return result
}

// ListRuns returns the last 50 run records for a job.
func (e *Engine) ListRuns(jobID string) ([]RunRecord, error) {
	path := filepath.Join(e.dataDir, "runs", jobID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunRecord{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []RunRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r RunRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		records = append(records, r)
	}
	if len(records) > 50 {
		records = records[len(records)-50:]
	}
	return records, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────

// scheduleJobLocked converts the Job's Schedule to a robfig/cron spec and registers it.
// Supports:
//   - kind=cron  → use Expr directly (5 or 6-field cron)
//   - kind=every → convert EveryMs to "@every Xs" / "@every Xm" / "@every Xh"
//   - kind=at    → use Expr directly (one-shot; robfig supports absolute timestamps via Expr)
func (e *Engine) scheduleJobLocked(job *Job) {
	spec := e.buildSpec(job.Schedule)
	if spec == "" {
		fmt.Printf("cron: job %s has no valid schedule, skipping\n", job.ID)
		return
	}

	j := job // capture for closure
	entryID, err := e.cron.AddFunc(spec, func() {
		e.executeJob(j)
	})
	if err != nil {
		// Retry with "0 " prefix for standard 5-field cron (no seconds column)
		entryID, err = e.cron.AddFunc("0 "+spec, func() {
			e.executeJob(j)
		})
		if err != nil {
			fmt.Printf("cron: failed to schedule job %s (%s): %v\n", job.ID, spec, err)
			return
		}
	}
	e.entryIDs[job.ID] = entryID

	entry := e.cron.Entry(entryID)
	if !entry.Next.IsZero() {
		job.State.NextRunAtMs = entry.Next.UnixMilli()
	}
}

// buildSpec converts a Schedule to a robfig/cron spec string.
func (e *Engine) buildSpec(s Schedule) string {
	switch s.Kind {
	case "every":
		if s.EveryMs <= 0 {
			return ""
		}
		d := time.Duration(s.EveryMs) * time.Millisecond
		// Use seconds-level precision; robfig/cron WithSeconds() supports @every with sub-minute
		secs := int(d.Seconds())
		if secs < 1 {
			secs = 1
		}
		return fmt.Sprintf("@every %ds", secs)
	case "cron", "at", "":
		return s.Expr
	default:
		return s.Expr
	}
}

func (e *Engine) unscheduleJobLocked(id string) {
	if entryID, ok := e.entryIDs[id]; ok {
		e.cron.Remove(entryID)
		delete(e.entryIDs, id)
	}
}

// executeJob runs a single job invocation in an isolated session.
func (e *Engine) executeJob(job *Job) {
	startedAt := time.Now().UnixMilli()

	agentID := job.AgentID
	if agentID == "" {
		agentID = "main"
	}

	runID := "run-" + uuid.New().String()[:8]
	record := RunRecord{
		JobID:     job.ID,
		RunID:     runID,
		StartedAt: startedAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var output string
	switch job.Payload.Kind {
	case "agentTurn", "":
		if e.runJob == nil {
			record.Status = "error"
			record.Error = "no runner configured"
			break
		}
		out, err := e.runJob(ctx, agentID, job.Payload.Model, job.ID, runID, job.Payload.Message)
		if err != nil {
			record.Status = "error"
			record.Error = err.Error()
		} else {
			record.Status = "ok"
			output = out
			if len(output) > 4000 {
				record.Output = output[:4000] + "…"
			} else {
				record.Output = output
			}
		}

	case "systemEvent":
		// systemEvent injects directly into the agent session without LLM — not isolated.
		// Kept for legacy/simple use cases; no announce.
		record.Status = "ok"
		record.Output = "(system event)"

	default:
		record.Status = "error"
		record.Error = fmt.Sprintf("unknown payload kind: %s", job.Payload.Kind)
	}

	record.EndedAt = time.Now().UnixMilli()

	// Delivery: announce unless suppressed
	if record.Status == "ok" && job.Delivery.Mode == "announce" && e.announce != nil {
		trimmed := strings.TrimSpace(output)
		if !strings.HasPrefix(trimmed, SilentToken) && trimmed != "" {
			e.announce(agentID, job.Name, trimmed)
			record.Announced = true
		}
	}

	// Update job state
	e.jobMu.Lock()
	if j, ok := e.jobs[job.ID]; ok {
		j.State.LastRunAtMs = startedAt
		j.State.LastStatus = record.Status
		if entryID, ok2 := e.entryIDs[job.ID]; ok2 {
			entry := e.cron.Entry(entryID)
			if !entry.Next.IsZero() {
				j.State.NextRunAtMs = entry.Next.UnixMilli()
			}
		}
		e.saveLocked()
	}
	e.jobMu.Unlock()

	e.appendRunRecord(record)
}

func (e *Engine) appendRunRecord(record RunRecord) {
	path := filepath.Join(e.dataDir, "runs", record.JobID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("cron: failed to write run record: %v\n", err)
		return
	}
	defer f.Close()
	data, _ := json.Marshal(record)
	fmt.Fprintf(f, "%s\n", data)
}

func (e *Engine) saveLocked() error {
	jobs := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		jobs = append(jobs, j)
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.dataDir, "jobs.json"), data, 0644)
}
