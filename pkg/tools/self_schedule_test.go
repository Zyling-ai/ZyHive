package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/cron"
)

// fakeCronEngine satisfies the tools.CronEngine interface for self_schedule
// tests without spinning up the real scheduler. It captures Add() calls so we
// can assert the resulting Job structure.
type fakeCronEngine struct {
	mu   sync.Mutex
	jobs map[string]*cron.Job
	seq  int
}

func newFakeCronEngine() *fakeCronEngine {
	return &fakeCronEngine{jobs: map[string]*cron.Job{}}
}

func (f *fakeCronEngine) Add(j *cron.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if j.ID == "" {
		f.seq++
		j.ID = fmt.Sprintf("job-test-%d", f.seq)
	}
	f.jobs[j.ID] = j
	return nil
}

func (f *fakeCronEngine) Remove(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.jobs, id)
	return nil
}

func (f *fakeCronEngine) ListJobs() []*cron.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*cron.Job, 0, len(f.jobs))
	for _, j := range f.jobs {
		out = append(out, j)
	}
	return out
}

func (f *fakeCronEngine) ListJobsByAgent(agentID string) []*cron.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*cron.Job
	for _, j := range f.jobs {
		if j.AgentID == agentID {
			out = append(out, j)
		}
	}
	return out
}

// newRegForSelfSchedule wires a Registry to a fake cron engine and registers
// self_schedule. Returns the registry so tests can invoke handlers.
func newRegForSelfSchedule(t *testing.T) (*Registry, *fakeCronEngine) {
	t.Helper()
	reg := New(t.TempDir(), t.TempDir(), "agent-test")
	fake := newFakeCronEngine()
	reg.WithCronEngine(fake)
	reg.WithSelfSchedule()
	return reg, fake
}

// TestSelfSchedule_Success — happy path: 30m relative, job is created with
// kind=at and Remark marker.
func TestSelfSchedule_Success(t *testing.T) {
	reg, fake := newRegForSelfSchedule(t)

	in, _ := json.Marshal(map[string]string{"when": "30m", "note": "喝水"})
	out, err := reg.handleSelfSchedule(context.Background(), in)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !strings.Contains(out, "✅ 已设定 1 次提醒") {
		t.Fatalf("unexpected output: %s", out)
	}

	jobs := fake.ListJobsByAgent("agent-test")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.Schedule.Kind != "at" {
		t.Fatalf("kind should be 'at', got %q", j.Schedule.Kind)
	}
	if j.Remark != "self_schedule" {
		t.Fatalf("Remark should be marker 'self_schedule', got %q", j.Remark)
	}
	if j.Payload.Message != "喝水" {
		t.Fatalf("Payload.Message = %q", j.Payload.Message)
	}
	if !strings.HasPrefix(j.Name, "🔔 ") {
		t.Fatalf("Name should start with bell emoji, got %q", j.Name)
	}
	// fireAt should be ~30m in the future.
	fireAt, perr := time.Parse(time.RFC3339, j.Schedule.Expr)
	if perr != nil {
		t.Fatalf("Expr is not RFC3339: %v", perr)
	}
	delta := time.Until(fireAt)
	if delta < 29*time.Minute || delta > 31*time.Minute {
		t.Fatalf("fire-at delta %v not ~30m", delta)
	}
}

// TestSelfSchedule_RejectsEmpty — empty when / note get explicit errors.
func TestSelfSchedule_RejectsEmpty(t *testing.T) {
	reg, _ := newRegForSelfSchedule(t)

	cases := []map[string]string{
		{"when": "", "note": "x"},
		{"when": "30m", "note": ""},
		{"when": "  ", "note": "x"},
	}
	for _, c := range cases {
		in, _ := json.Marshal(c)
		if _, err := reg.handleSelfSchedule(context.Background(), in); err == nil {
			t.Fatalf("expected error for input %+v", c)
		}
	}
}

// TestSelfSchedule_RejectsPast — past time produces a useful error.
func TestSelfSchedule_RejectsPast(t *testing.T) {
	reg, _ := newRegForSelfSchedule(t)

	in, _ := json.Marshal(map[string]string{
		"when": "2020-01-01T00:00:00+08:00",
		"note": "test",
	})
	_, err := reg.handleSelfSchedule(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error for past time")
	}
	if !strings.Contains(err.Error(), "已过") {
		t.Fatalf("error should mention past time, got: %v", err)
	}
}

// TestSelfSchedule_AntiAbuseLimit — when at the cap, refuses with helpful
// message pointing at cron_remove.
func TestSelfSchedule_AntiAbuseLimit(t *testing.T) {
	// Crank limit to 2 for fast test; restore after.
	old := selfScheduleMaxPerAgent
	selfScheduleMaxPerAgent = 2
	t.Cleanup(func() { selfScheduleMaxPerAgent = old })

	reg, fake := newRegForSelfSchedule(t)

	for i := 0; i < 2; i++ {
		in, _ := json.Marshal(map[string]string{"when": "30m", "note": "n"})
		if _, err := reg.handleSelfSchedule(context.Background(), in); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if got := len(fake.ListJobsByAgent("agent-test")); got != 2 {
		t.Fatalf("expected 2 seeded jobs, got %d", got)
	}

	in, _ := json.Marshal(map[string]string{"when": "30m", "note": "x"})
	_, err := reg.handleSelfSchedule(context.Background(), in)
	if err == nil {
		t.Fatalf("expected limit error")
	}
	if !strings.Contains(err.Error(), "cron_remove") {
		t.Fatalf("error should hint cron_remove, got: %v", err)
	}
}

// TestSelfSchedule_LimitOnlyCountsSelfSchedule — pre-existing cron_add jobs
// (non-self_schedule) must NOT count against the self_schedule cap.
func TestSelfSchedule_LimitOnlyCountsSelfSchedule(t *testing.T) {
	old := selfScheduleMaxPerAgent
	selfScheduleMaxPerAgent = 1
	t.Cleanup(func() { selfScheduleMaxPerAgent = old })

	reg, fake := newRegForSelfSchedule(t)

	// Seed an unrelated kind=at job (not from self_schedule) — should NOT count.
	fake.Add(&cron.Job{
		Name:     "from cron_add",
		Enabled:  true,
		AgentID:  "agent-test",
		Schedule: cron.Schedule{Kind: "at", Expr: time.Now().Add(time.Hour).Format(time.RFC3339)},
		Payload:  cron.Payload{Kind: "agentTurn", Message: "n"},
	})

	// First self_schedule should still succeed despite cap=1.
	in, _ := json.Marshal(map[string]string{"when": "30m", "note": "n"})
	if _, err := reg.handleSelfSchedule(context.Background(), in); err != nil {
		t.Fatalf("first self_schedule should succeed when only manual cron jobs exist, got: %v", err)
	}
}

// TestCountPendingSelfSchedule — pure unit test for the counter helper.
func TestCountPendingSelfSchedule(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	mk := func(remark, kind, expr string, enabled bool) *cron.Job {
		return &cron.Job{
			Remark: remark, Enabled: enabled,
			Schedule: cron.Schedule{Kind: kind, Expr: expr},
		}
	}
	jobs := []*cron.Job{
		mk("self_schedule", "at", now.Add(time.Hour).Format(time.RFC3339), true), // 计数
		mk("self_schedule", "at", now.Add(-time.Hour).Format(time.RFC3339), true), // 已过去, 不计数
		mk("self_schedule", "at", now.Add(time.Hour).Format(time.RFC3339), false), // 已禁用, 不计数
		mk("", "at", now.Add(time.Hour).Format(time.RFC3339), true),               // 非 self_schedule, 不计数
		mk("self_schedule", "cron", "0 9 * * 1", true),                            // 非 at, 不计数
		mk("self_schedule", "at", "garbage", true),                                // 解析失败, 不计数
		nil,                                                                       // nil-safety
	}
	if got := countPendingSelfSchedule(jobs, now); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}
