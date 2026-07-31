package cron

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestEngine_LastTickAtZeroBeforeStart — the heartbeat is only meaningful
// after Start/Load; before then LastTickAt() must return the zero value so
// /readyz can distinguish "never started" from "started but stalled".
func TestEngine_LastTickAtZeroBeforeStart(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	if got := e.LastTickAt(); !got.IsZero() {
		t.Fatalf("LastTickAt before Start should be zero, got %v", got)
	}
}

// TestEngine_LastTickAtAfterStart — after Start the heartbeat goroutine sets
// lastTickAt immediately (before the first ticker fire), so the value must
// be very recent.
func TestEngine_LastTickAtAfterStart(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	e.Start()
	t.Cleanup(func() { e.Stop() })

	got := e.LastTickAt()
	if got.IsZero() {
		t.Fatalf("LastTickAt after Start should be non-zero")
	}
	if time.Since(got) > 5*time.Second {
		t.Fatalf("LastTickAt after Start should be recent, got %v ago", time.Since(got))
	}
}

// TestEngine_StartHeartbeatIdempotent — Load() then Start() (or any other
// double-call) must not spawn duplicate heartbeat goroutines, otherwise we
// leak on every restart.
func TestEngine_StartHeartbeatIdempotent(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	if err := e.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(func() { e.Stop() })

	first := e.heartbeatStop
	e.Start() // should be a no-op for the heartbeat
	if e.heartbeatStop != first {
		t.Fatalf("calling Start after Load swapped heartbeatStop channel; goroutine leaked")
	}
}

func TestPlanScheduleAppliesTimezone(t *testing.T) {
	plan, err := planSchedule(Schedule{
		Kind: "cron",
		Expr: "0 9 * * *",
		TZ:   "Asia/Shanghai",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := cronParser.Parse(plan.spec)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	if got := schedule.Next(after); !got.Equal(want) {
		t.Fatalf("next run = %v, want %v", got, want)
	}
}

func TestEngineRejectsInvalidScheduleAtomically(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	job := &Job{
		Name:     "invalid",
		Enabled:  true,
		Schedule: Schedule{Kind: "cron", Expr: "not a cron", TZ: "Asia/Shanghai"},
		Payload:  Payload{Kind: "agentTurn", Message: "hello"},
		Delivery: Delivery{Mode: "none"},
	}
	if err := e.Add(job); err == nil {
		t.Fatal("expected invalid schedule error")
	}
	if got := e.ListJobs(); len(got) != 0 {
		t.Fatalf("invalid job leaked into memory: %+v", got)
	}
	if len(e.entryIDs) != 0 {
		t.Fatalf("invalid job leaked scheduler entries: %+v", e.entryIDs)
	}
}

func TestEngineInvalidUpdateKeepsExistingSchedule(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	job := testJob(Schedule{Kind: "every", EveryMs: 60_000})
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}
	oldEntry := e.entryIDs[job.ID]
	err := e.Update(job.ID, &Job{
		Enabled:  true,
		Schedule: Schedule{Kind: "cron", Expr: "invalid", TZ: "UTC"},
	})
	if !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("Update error = %v, want ErrInvalidJob", err)
	}
	jobs := e.ListJobs()
	if len(jobs) != 1 || jobs[0].Schedule.Kind != "every" || jobs[0].Schedule.EveryMs != 60_000 {
		t.Fatalf("invalid update changed persisted job: %+v", jobs)
	}
	if e.entryIDs[job.ID] != oldEntry {
		t.Fatal("invalid update replaced the active scheduler entry")
	}
}

func TestEngineCanonicalizesRelativeOneShotBeforeSaving(t *testing.T) {
	e := NewEngine(t.TempDir(), nil, nil)
	job := testJob(Schedule{Kind: "at", Expr: "2h", TZ: "Asia/Shanghai"})
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, job.Schedule.Expr); err != nil {
		t.Fatalf("one-shot expression was not canonicalized: %q", job.Schedule.Expr)
	}
	if job.Schedule.TZ != "UTC" {
		t.Fatalf("canonical one-shot timezone = %q, want UTC", job.Schedule.TZ)
	}
}

func TestEngineKeepsCompletedOneShotAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	e := NewEngine(dataDir, nil, nil)
	job := testJob(Schedule{
		Kind: "at",
		Expr: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		TZ:   "UTC",
	})
	job.Enabled = false
	job.State.DisabledReason = "one-shot completed"
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}

	reloaded := NewEngine(dataDir, nil, nil)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { <-reloaded.Stop().Done() })
	jobs := reloaded.ListJobs()
	if len(jobs) != 1 || jobs[0].Enabled || jobs[0].State.DisabledReason != "one-shot completed" {
		t.Fatalf("completed one-shot changed after restart: %+v", jobs)
	}
}

func TestEngineAddRollsBackWhenPersistenceFails(t *testing.T) {
	parent := t.TempDir()
	notDir := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(notDir, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(notDir, nil, nil)
	job := testJob(Schedule{Kind: "every", EveryMs: 1000})
	if err := e.Add(job); err == nil {
		t.Fatal("expected persistence error")
	}
	if len(e.ListJobs()) != 0 || len(e.entryIDs) != 0 {
		t.Fatal("failed add must not leave memory or scheduler state")
	}
}

func TestEngineAtRunsOnceAndDisablesJob(t *testing.T) {
	var calls atomic.Int32
	fired := make(chan struct{}, 1)
	e := NewEngine(t.TempDir(), func(context.Context, string, string, string, string, string) (string, error) {
		calls.Add(1)
		fired <- struct{}{}
		return "done", nil
	}, nil)
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { <-e.Stop().Done() })

	job := testJob(Schedule{Kind: "at", Expr: "1s", TZ: "Asia/Shanghai"})
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("one-shot job did not fire")
	}
	waitFor(t, 2*time.Second, func() bool {
		jobs := e.ListJobs()
		return len(jobs) == 1 && !jobs[0].Enabled && jobs[0].State.DisabledReason == "one-shot completed"
	})
	time.Sleep(1200 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("one-shot job ran %d times", got)
	}
}

func TestEngineSkipsOverlappingRuns(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	e := NewEngine(t.TempDir(), func(ctx context.Context, _, _, _, _, _ string) (string, error) {
		started <- struct{}{}
		select {
		case <-release:
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}, nil)
	job := testJob(Schedule{Kind: "every", EveryMs: 60_000})
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}
	if err := e.RunNow(job.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first run did not start")
	}
	if err := e.RunNow(job.ID); !errors.Is(err, ErrJobAlreadyRunning) {
		t.Fatalf("second run error = %v, want ErrJobAlreadyRunning", err)
	}
	close(release)
	waitFor(t, 2*time.Second, func() bool {
		runs, err := e.ListRuns(job.ID)
		return err == nil && len(runs) == 2
	})
	runs, err := e.ListRuns(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]int{}
	for _, run := range runs {
		statuses[run.Status]++
	}
	if statuses["ok"] != 1 || statuses["skipped"] != 1 {
		t.Fatalf("unexpected run statuses: %+v", statuses)
	}
}

func TestRemoveForAgentChecksOwnershipAndCancelsRun(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	e := NewEngine(t.TempDir(), func(ctx context.Context, _, _, _, _, _ string) (string, error) {
		<-ctx.Done()
		cancelled <- struct{}{}
		return "", ctx.Err()
	}, nil)
	job := testJob(Schedule{Kind: "every", EveryMs: 60_000})
	job.AgentID = "agent-a"
	if err := e.Add(job); err != nil {
		t.Fatal(err)
	}
	if err := e.RunNow(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveForAgent(job.ID, "agent-b"); err == nil {
		t.Fatal("cross-agent removal should fail")
	}
	if err := e.RemoveForAgent(job.ID, "agent-a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("removing a running job did not cancel its context")
	}
}

func testJob(schedule Schedule) *Job {
	return &Job{
		Name:     "test",
		Enabled:  true,
		AgentID:  "agent-test",
		Schedule: schedule,
		Payload:  Payload{Kind: "agentTurn", Message: "hello"},
		Delivery: Delivery{Mode: "none"},
	}
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
