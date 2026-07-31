package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/cron"
)

func TestCronToolsDoNotExposeOrRemoveOtherAgentsJobs(t *testing.T) {
	reg := New(t.TempDir(), t.TempDir(), "agent-a")
	fake := newFakeCronEngine()
	reg.WithCronEngine(fake)

	own := &cron.Job{
		ID:       "job-own",
		Name:     "own",
		AgentID:  "agent-a",
		Enabled:  true,
		Schedule: cron.Schedule{Kind: "every", EveryMs: 60_000},
		Payload:  cron.Payload{Kind: "agentTurn", Message: "own"},
		Delivery: cron.Delivery{Mode: "none"},
	}
	other := &cron.Job{
		ID:       "job-other",
		Name:     "other-secret",
		AgentID:  "agent-b",
		Enabled:  true,
		Schedule: cron.Schedule{Kind: "every", EveryMs: 60_000},
		Payload:  cron.Payload{Kind: "agentTurn", Message: "other"},
		Delivery: cron.Delivery{Mode: "none"},
	}
	if err := fake.Add(own); err != nil {
		t.Fatal(err)
	}
	if err := fake.Add(other); err != nil {
		t.Fatal(err)
	}

	listed, err := reg.handleCronList(context.Background(), json.RawMessage(`{"all":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "own") || strings.Contains(listed, "other-secret") {
		t.Fatalf("cron_list crossed agent boundary: %s", listed)
	}

	_, err = reg.handleCronRemove(context.Background(), json.RawMessage(`{"id":"job-other"}`))
	if err == nil {
		t.Fatal("cron_remove should reject another agent's job")
	}
	if len(fake.ListJobsByAgent("agent-b")) != 1 {
		t.Fatal("another agent's job was removed")
	}
}
