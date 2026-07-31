// Package cron provides the scheduled job engine.
package cron

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	NextRunAtMs    int64  `json:"nextRunAtMs,omitempty"`
	LastRunAtMs    int64  `json:"lastRunAtMs,omitempty"`
	LastStatus     string `json:"lastStatus,omitempty"`     // "ok" | "error"
	ErrorCount     int    `json:"errorCount,omitempty"`     // consecutive failure count
	DisabledReason string `json:"disabledReason,omitempty"` // set when auto-disabled
}

// maxConsecutiveErrors is the threshold after which a job is automatically disabled.
const maxConsecutiveErrors = 3

var (
	ErrInvalidJob        = errors.New("invalid cron job")
	ErrJobAlreadyRunning = errors.New("cron job is already running")
)

type RunRecord struct {
	JobID     string `json:"jobId"`
	RunID     string `json:"runId"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt"`
	Status    string `json:"status"` // "ok" | "error" | "skipped"
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

	// lastTickAtMs is updated by a lightweight heartbeat goroutine while the
	// engine is running, so /readyz can verify the scheduler is alive even
	// when no jobs happen to be triggering.
	lastTickAtMs  int64      // accessed via atomic load/store
	heartbeatMu   sync.Mutex // guards heartbeatStop
	heartbeatStop chan struct{}

	runMu      sync.Mutex
	activeRuns map[string]*activeRun
	runWG      sync.WaitGroup
	stopping   bool
	recordMu   sync.Mutex
}

type activeRun struct {
	runID  string
	cancel context.CancelFunc
}

// NewEngine creates a new cron engine.
//   - runJob:   isolated session runner (see CronRunFunc)
//   - announce: output delivery callback; may be nil (disables announce mode)
func NewEngine(dataDir string, runJob CronRunFunc, announce AnnounceFunc) *Engine {
	return &Engine{
		cron:       cron.New(cron.WithSeconds()),
		jobs:       make(map[string]*Job),
		entryIDs:   make(map[string]cron.EntryID),
		activeRuns: make(map[string]*activeRun),
		dataDir:    dataDir,
		runJob:     runJob,
		announce:   announce,
	}
}

// Load reads jobs.json from disk and schedules all enabled jobs.
func (e *Engine) Load() error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()
	for _, entryID := range e.entryIDs {
		e.cron.Remove(entryID)
	}
	e.jobs = make(map[string]*Job)
	e.entryIDs = make(map[string]cron.EntryID)

	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(e.dataDir, "runs"), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(e.dataDir, "jobs.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		var jobs []*Job
		if err := json.Unmarshal(data, &jobs); err != nil {
			return fmt.Errorf("parse jobs.json: %w", err)
		}
		changed := false
		for _, j := range jobs {
			if j == nil || !validJobID(j.ID) {
				changed = true
				continue
			}
			originalSchedule := j.Schedule
			if validationErr := normalizeJob(j, time.Now()); validationErr != nil {
				j.Enabled = false
				j.State.NextRunAtMs = 0
				j.State.DisabledReason = "invalid job: " + validationErr.Error()
				changed = true
			} else if j.Schedule != originalSchedule {
				changed = true
			}
			e.jobs[j.ID] = j
			if j.Enabled {
				if scheduleErr := e.scheduleJobLocked(j); scheduleErr != nil {
					j.Enabled = false
					j.State.NextRunAtMs = 0
					j.State.DisabledReason = "invalid schedule: " + scheduleErr.Error()
					changed = true
				}
			}
		}
		if changed {
			if err := e.saveLocked(); err != nil {
				return err
			}
		}
	}

	// Start the scheduler and heartbeat regardless of whether jobs.json existed,
	// so /readyz can verify the engine is alive even on a fresh install with
	// zero jobs configured.
	e.setStopping(false)
	e.cron.Start()
	e.startHeartbeat()
	return nil
}

func (e *Engine) Start() {
	e.setStopping(false)
	e.cron.Start()
	e.startHeartbeat()
}

func (e *Engine) Stop() context.Context {
	e.stopHeartbeat()
	e.runMu.Lock()
	e.stopping = true
	for _, run := range e.activeRuns {
		run.cancel()
	}
	e.runMu.Unlock()

	schedulerDone := e.cron.Stop()
	done, cancel := context.WithCancel(context.Background())
	go func() {
		<-schedulerDone.Done()
		e.runWG.Wait()
		cancel()
	}()
	return done
}

func (e *Engine) setStopping(stopping bool) {
	e.runMu.Lock()
	e.stopping = stopping
	e.runMu.Unlock()
}

// heartbeatInterval — how often the engine refreshes its liveness timestamp.
// /readyz treats lastTickAt older than ~3× this as "stale".
const heartbeatInterval = 10 * time.Second

// startHeartbeat launches a goroutine that periodically updates lastTickAtMs.
// Idempotent: calling twice (Load + Start) only spawns one goroutine.
func (e *Engine) startHeartbeat() {
	e.heartbeatMu.Lock()
	defer e.heartbeatMu.Unlock()
	if e.heartbeatStop != nil {
		return
	}
	stop := make(chan struct{})
	e.heartbeatStop = stop
	atomic.StoreInt64(&e.lastTickAtMs, time.Now().UnixMilli())
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				atomic.StoreInt64(&e.lastTickAtMs, time.Now().UnixMilli())
			}
		}
	}()
}

func (e *Engine) stopHeartbeat() {
	e.heartbeatMu.Lock()
	defer e.heartbeatMu.Unlock()
	if e.heartbeatStop != nil {
		close(e.heartbeatStop)
		e.heartbeatStop = nil
	}
}

// LastTickAt returns the engine's last heartbeat timestamp.
//
// Returns the zero time before the engine has been started. /readyz uses this
// to fail-fast when the scheduler goroutine has somehow stopped ticking.
func (e *Engine) LastTickAt() time.Time {
	ms := atomic.LoadInt64(&e.lastTickAtMs)
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// Add adds a new job, persists to disk, and schedules it if enabled.
func (e *Engine) Add(job *Job) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	candidate := cloneJob(job)
	if candidate.ID == "" {
		candidate.ID = "job-" + uuid.New().String()[:8]
	}
	if !validJobID(candidate.ID) {
		return fmt.Errorf("%w: invalid job id %q", ErrInvalidJob, candidate.ID)
	}
	if _, exists := e.jobs[candidate.ID]; exists {
		return fmt.Errorf("job %q already exists", candidate.ID)
	}
	if candidate.CreatedAtMs == 0 {
		candidate.CreatedAtMs = time.Now().UnixMilli()
	}
	if err := normalizeJob(candidate, time.Now()); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJob, err)
	}

	e.jobs[candidate.ID] = candidate
	if candidate.Enabled {
		if err := e.scheduleJobLocked(candidate); err != nil {
			delete(e.jobs, candidate.ID)
			return err
		}
	}
	if err := e.saveLocked(); err != nil {
		e.unscheduleJobLocked(candidate.ID)
		delete(e.jobs, candidate.ID)
		return err
	}
	*job = *cloneJob(candidate)
	return nil
}

// Update patches a job, reschedules, and saves.
func (e *Engine) Update(id string, patch *Job) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	existing, ok := e.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	candidate := cloneJob(existing)
	if patch.Name != "" {
		candidate.Name = patch.Name
	}
	if patch.Remark != "" {
		candidate.Remark = patch.Remark
	}
	candidate.Enabled = patch.Enabled
	if patch.Schedule.Expr != "" || patch.Schedule.EveryMs > 0 {
		candidate.Schedule = patch.Schedule
	}
	if patch.Payload.Message != "" {
		candidate.Payload = patch.Payload
	}
	if patch.Delivery.Mode != "" {
		candidate.Delivery = patch.Delivery
	}
	if patch.AgentID != "" {
		candidate.AgentID = patch.AgentID
	}
	if err := normalizeJob(candidate, time.Now()); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJob, err)
	}

	oldEntry, hadOldEntry := e.entryIDs[id]
	e.jobs[id] = candidate
	if candidate.Enabled {
		if err := e.scheduleJobLocked(candidate); err != nil {
			e.jobs[id] = existing
			if hadOldEntry {
				e.entryIDs[id] = oldEntry
			} else {
				delete(e.entryIDs, id)
			}
			return err
		}
	} else {
		delete(e.entryIDs, id)
	}
	newEntry, hasNewEntry := e.entryIDs[id]
	if err := e.saveLocked(); err != nil {
		if hasNewEntry && (!hadOldEntry || newEntry != oldEntry) {
			e.cron.Remove(newEntry)
		}
		e.jobs[id] = existing
		if hadOldEntry {
			e.entryIDs[id] = oldEntry
		} else {
			delete(e.entryIDs, id)
		}
		return err
	}
	if hadOldEntry && (!hasNewEntry || oldEntry != newEntry) {
		e.cron.Remove(oldEntry)
	}
	return nil
}

// Remove deletes a job and unschedules it.
func (e *Engine) Remove(id string) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()
	return e.removeLocked(id)
}

func (e *Engine) removeLocked(id string) error {
	job, ok := e.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}
	delete(e.jobs, id)
	if err := e.saveLocked(); err != nil {
		e.jobs[id] = job
		return err
	}
	e.unscheduleJobLocked(id)
	e.cancelRun(id)
	return nil
}

// RemoveForAgent removes a job only when it belongs to the given agent.
func (e *Engine) RemoveForAgent(id, agentID string) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()
	job, ok := e.jobs[id]
	if !ok || job.AgentID != agentID {
		return fmt.Errorf("job %q not found", id)
	}
	return e.removeLocked(id)
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

	return e.startJob(&j, false)
}

// ListJobs returns all jobs.
func (e *Engine) ListJobs() []*Job {
	e.jobMu.RLock()
	defer e.jobMu.RUnlock()
	result := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		result = append(result, cloneJob(j))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAtMs < result[j].CreatedAtMs })
	return result
}

// ListJobsByAgent returns jobs for a specific agent ("*" = all).
func (e *Engine) ListJobsByAgent(agentID string) []*Job {
	e.jobMu.RLock()
	defer e.jobMu.RUnlock()
	result := make([]*Job, 0)
	for _, j := range e.jobs {
		if agentID == "*" || j.AgentID == agentID {
			result = append(result, cloneJob(j))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAtMs < result[j].CreatedAtMs })
	return result
}

// EnableJob re-enables a job that was manually or automatically disabled,
// resets the consecutive error counter, and reschedules it.
func (e *Engine) EnableJob(id string) error {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	j, ok := e.jobs[id]
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}

	candidate := cloneJob(j)
	candidate.Enabled = true
	candidate.State.ErrorCount = 0
	candidate.State.DisabledReason = ""
	if err := normalizeJob(candidate, time.Now()); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJob, err)
	}
	oldEntry, hadOldEntry := e.entryIDs[id]
	e.jobs[id] = candidate
	if err := e.scheduleJobLocked(candidate); err != nil {
		e.jobs[id] = j
		if hadOldEntry {
			e.entryIDs[id] = oldEntry
		}
		return err
	}
	newEntry := e.entryIDs[id]
	if err := e.saveLocked(); err != nil {
		e.cron.Remove(newEntry)
		e.jobs[id] = j
		if hadOldEntry {
			e.entryIDs[id] = oldEntry
		} else {
			delete(e.entryIDs, id)
		}
		return err
	}
	if hadOldEntry && oldEntry != newEntry {
		e.cron.Remove(oldEntry)
	}
	fmt.Printf("cron: job %s (%s) manually re-enabled\n", j.ID, j.Name)
	return nil
}

// ListRuns returns the last 50 run records for a job.
func (e *Engine) ListRuns(jobID string) ([]RunRecord, error) {
	if !validJobID(jobID) {
		return nil, fmt.Errorf("invalid job id %q", jobID)
	}
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

func (e *Engine) startJobByID(id string, scheduled bool) error {
	e.jobMu.RLock()
	job, ok := e.jobs[id]
	if !ok || (scheduled && !job.Enabled) {
		e.jobMu.RUnlock()
		return nil
	}
	candidate := cloneJob(job)
	e.jobMu.RUnlock()
	return e.startJob(candidate, scheduled)
}

func (e *Engine) startJob(job *Job, scheduled bool) error {
	runID := "run-" + uuid.New().String()[:8]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	e.runMu.Lock()
	if e.stopping {
		e.runMu.Unlock()
		cancel()
		return fmt.Errorf("cron engine is stopping")
	}
	if active, exists := e.activeRuns[job.ID]; exists {
		e.runMu.Unlock()
		cancel()
		e.appendRunRecord(RunRecord{
			JobID:     job.ID,
			RunID:     runID,
			StartedAt: time.Now().UnixMilli(),
			EndedAt:   time.Now().UnixMilli(),
			Status:    "skipped",
			Error:     fmt.Sprintf("%v: active run %s", ErrJobAlreadyRunning, active.runID),
		})
		return ErrJobAlreadyRunning
	}
	e.activeRuns[job.ID] = &activeRun{runID: runID, cancel: cancel}
	e.runWG.Add(1)
	e.runMu.Unlock()

	go e.executeJob(job, runID, ctx, scheduled)
	return nil
}

func (e *Engine) finishRun(jobID, runID string) {
	e.runMu.Lock()
	if active, ok := e.activeRuns[jobID]; ok && active.runID == runID {
		active.cancel()
		delete(e.activeRuns, jobID)
	}
	e.runMu.Unlock()
	e.runWG.Done()
}

func (e *Engine) cancelRun(jobID string) {
	e.runMu.Lock()
	if active := e.activeRuns[jobID]; active != nil {
		active.cancel()
	}
	e.runMu.Unlock()
}

// executeJob runs a claimed job invocation in an isolated session.
func (e *Engine) executeJob(job *Job, runID string, ctx context.Context, scheduled bool) {
	defer e.finishRun(job.ID, runID)
	startedAt := time.Now().UnixMilli()

	agentID := job.AgentID
	if agentID == "" {
		agentID = "main"
	}

	record := RunRecord{
		JobID:     job.ID,
		RunID:     runID,
		StartedAt: startedAt,
	}

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

	// Update job state and handle error counting / auto-disable
	e.jobMu.Lock()
	if j, ok := e.jobs[job.ID]; ok {
		j.State.LastRunAtMs = startedAt
		j.State.LastStatus = record.Status

		if record.Status == "ok" {
			// Success: reset consecutive error count
			j.State.ErrorCount = 0
			j.State.DisabledReason = ""
		} else {
			// Failure: increment consecutive error count
			j.State.ErrorCount++
			fmt.Printf("cron: job %s (%s) failed (consecutive errors: %d)\n", j.ID, j.Name, j.State.ErrorCount)
			if j.State.ErrorCount >= maxConsecutiveErrors && j.Enabled {
				// Auto-disable the job to prevent infinite error loops
				j.Enabled = false
				j.State.DisabledReason = fmt.Sprintf("auto-disabled after %d consecutive failures (last error: %s)", j.State.ErrorCount, record.Error)
				e.unscheduleJobLocked(j.ID)
				fmt.Printf("cron: job %s (%s) AUTO-DISABLED — %s\n", j.ID, j.Name, j.State.DisabledReason)
			}
		}

		if scheduled && job.Schedule.Kind == "at" && j.Schedule == job.Schedule {
			j.Enabled = false
			j.State.NextRunAtMs = 0
			if record.Status == "ok" {
				j.State.DisabledReason = "one-shot completed"
			} else {
				j.State.DisabledReason = "one-shot completed with error: " + record.Error
			}
			e.unscheduleJobLocked(j.ID)
		} else if entryID, ok2 := e.entryIDs[job.ID]; ok2 {
			entry := e.cron.Entry(entryID)
			if !entry.Next.IsZero() {
				j.State.NextRunAtMs = entry.Next.UnixMilli()
			} else {
				j.State.NextRunAtMs = 0
			}
		}
		if err := e.saveLocked(); err != nil {
			fmt.Printf("cron: failed to save job state: %v\n", err)
		}
	}
	e.jobMu.Unlock()

	e.appendRunRecord(record)
}

func (e *Engine) appendRunRecord(record RunRecord) {
	e.recordMu.Lock()
	defer e.recordMu.Unlock()
	runsDir := filepath.Join(e.dataDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		fmt.Printf("cron: failed to create run directory: %v\n", err)
		return
	}
	path := filepath.Join(runsDir, record.JobID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("cron: failed to write run record: %v\n", err)
		return
	}
	defer f.Close()
	data, _ := json.Marshal(record)
	fmt.Fprintf(f, "%s\n", data)
}

var jobIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validJobID(id string) bool {
	return jobIDPattern.MatchString(id)
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func (e *Engine) saveLocked() error {
	jobs := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		jobs = append(jobs, cloneJob(j))
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAtMs < jobs[j].CreatedAtMs })
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(e.dataDir, ".jobs-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filepath.Join(e.dataDir, "jobs.json"))
}
