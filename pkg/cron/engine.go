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
	"sync"
	"time"

	"github.com/google/uuid"
	cron "github.com/robfig/cron/v3"
)

// RunnerFunc executes an agent turn and returns the full text response.
type RunnerFunc func(ctx context.Context, agentID, message string) (string, error)

// ── Job types ─────────────────────────────────────────────────────────────

type Job struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Schedule    Schedule `json:"schedule"`
	Payload     Payload  `json:"payload"`
	Delivery    Delivery `json:"delivery"`
	AgentID     string   `json:"agentId"`
	CreatedAtMs int64    `json:"createdAtMs"`
	State       JobState `json:"state"`
}

type Schedule struct {
	Kind string `json:"kind"` // "cron" | "every" | "at"
	Expr string `json:"expr"` // cron expression
	TZ   string `json:"tz"`   // timezone, e.g. "Asia/Shanghai"
}

type Payload struct {
	Kind    string `json:"kind"`            // "agentTurn" | "systemEvent"
	Message string `json:"message"`         // the prompt to send to the agent
	Model   string `json:"model,omitempty"` // optional model override
}

type Delivery struct {
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
	Output    string `json:"output"` // truncated agent response
	Error     string `json:"error,omitempty"`
}

// ── Engine ────────────────────────────────────────────────────────────────

type Engine struct {
	cron     *cron.Cron
	jobs     map[string]*Job
	entryIDs map[string]cron.EntryID // jobID -> cron entry
	jobMu    sync.RWMutex
	dataDir  string
	runner   RunnerFunc
}

// NewEngine creates a new cron engine backed by the given data directory.
func NewEngine(dataDir string, runner RunnerFunc) *Engine {
	return &Engine{
		cron:     cron.New(cron.WithSeconds()),
		jobs:     make(map[string]*Job),
		entryIDs: make(map[string]cron.EntryID),
		dataDir:  dataDir,
		runner:   runner,
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
			return nil // no jobs yet
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

// Start starts the cron scheduler (call after Load if not already started).
func (e *Engine) Start() {
	e.cron.Start()
}

// Stop stops the cron scheduler gracefully.
func (e *Engine) Stop() context.Context {
	return e.cron.Stop()
}

// Add adds a new job, saves to disk, and schedules it if enabled.
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

	// Unschedule old
	e.unscheduleJobLocked(id)

	// Apply patch fields
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	existing.Enabled = patch.Enabled
	if patch.Schedule.Expr != "" {
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
	// Copy to avoid races
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

// ListRuns returns run records for a job.
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

	// Return last 50
	if len(records) > 50 {
		records = records[len(records)-50:]
	}
	return records, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────

func (e *Engine) scheduleJobLocked(job *Job) {
	spec := job.Schedule.Expr
	if job.Schedule.TZ != "" {
		spec = fmt.Sprintf("CRON_TZ=%s %s", job.Schedule.TZ, spec)
	}

	// robfig/cron/v3 with seconds support; if user gives 5-field, prepend "0 "
	j := job // capture for closure
	entryID, err := e.cron.AddFunc(spec, func() {
		e.executeJob(j)
	})
	if err != nil {
		// Try with "0 " prefix for standard 5-field cron
		entryID, err = e.cron.AddFunc("0 "+spec, func() {
			e.executeJob(j)
		})
		if err != nil {
			fmt.Printf("cron: failed to schedule job %s: %v\n", job.ID, err)
			return
		}
	}
	e.entryIDs[job.ID] = entryID

	// Update next run time
	entry := e.cron.Entry(entryID)
	if !entry.Next.IsZero() {
		job.State.NextRunAtMs = entry.Next.UnixMilli()
	}
}

func (e *Engine) unscheduleJobLocked(id string) {
	if entryID, ok := e.entryIDs[id]; ok {
		e.cron.Remove(entryID)
		delete(e.entryIDs, id)
	}
}

func (e *Engine) executeJob(job *Job) {
	startedAt := time.Now().UnixMilli()

	agentID := job.AgentID
	if agentID == "" && job.Payload.Kind == "agentTurn" {
		agentID = "main" // default agent
	}

	record := RunRecord{
		JobID:     job.ID,
		RunID:     "run-" + uuid.New().String()[:8],
		StartedAt: startedAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if e.runner != nil {
		output, err := e.runner(ctx, agentID, job.Payload.Message)
		if err != nil {
			record.Status = "error"
			record.Error = err.Error()
		} else {
			record.Status = "ok"
			// Truncate output
			if len(output) > 2000 {
				record.Output = output[:2000] + "..."
			} else {
				record.Output = output
			}
		}
	} else {
		record.Status = "error"
		record.Error = "no runner configured"
	}

	record.EndedAt = time.Now().UnixMilli()

	// Update job state
	e.jobMu.Lock()
	if j, ok := e.jobs[job.ID]; ok {
		j.State.LastRunAtMs = startedAt
		j.State.LastStatus = record.Status
		// Update next run
		if entryID, ok := e.entryIDs[job.ID]; ok {
			entry := e.cron.Entry(entryID)
			if !entry.Next.IsZero() {
				j.State.NextRunAtMs = entry.Next.UnixMilli()
			}
		}
		e.saveLocked()
	}
	e.jobMu.Unlock()

	// Append run record
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
