// pkg/tools/self_schedule.go — self_schedule tool: lets an agent set a one-shot
// reminder for itself in human-friendly language without juggling cron exprs.
//
// Design:
//   - A thin layer on top of pkg/cron's "kind=at" one-shot job; we don't fork
//     the storage path.
//   - Anti-abuse: per-agent cap on PENDING (= future, not-yet-fired) at-jobs
//     created by self_schedule. cron_add is unaffected.
//   - Default tz: Asia/Shanghai (matches the rest of the product). Agents that
//     want a different tz should use cron_add with explicit tz.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// defaultSelfScheduleMaxPerAgent is the upper bound on PENDING self_schedule
// jobs for one agent. Hardcoded for now; can be overridden per-call by tests
// via the package-level variable below. Future work: surface to zyhive.json.
const defaultSelfScheduleMaxPerAgent = 20

// selfScheduleMaxPerAgent is the runtime-tunable cap. Tests can override this
// to drive the limit-exceeded branch quickly.
var selfScheduleMaxPerAgent = defaultSelfScheduleMaxPerAgent

// defaultSelfScheduleTZ — the timezone used to anchor "today" / "tomorrow" /
// "next monday" relative inputs when the AI did not pass an explicit RFC 3339
// time. Asia/Shanghai matches the product's primary user base and existing
// system-prompt conventions (see pkg/runner/system_prompt.go).
var defaultSelfScheduleTZName = "Asia/Shanghai"

var selfScheduleDef = llm.ToolDef{
	Name: "self_schedule",
	Description: "设一个一次性提醒：到时给自己（当前 Agent）发一条 note。" +
		"用于'X 分钟后再做'/'明天早上做 Y'等场景。要做周期性任务请用 cron_add。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"when": {
				"type": "string",
				"description": "何时触发。支持：30m / 2h / 1h30m（相对当下）；today HH:MM / tomorrow [HH:MM] / next monday [HH:MM]（默认 09:00）；YYYY-MM-DD HH:MM（按 Asia/Shanghai 解释）；2026-05-10T09:00:00+08:00（完整 ISO-8601）。已过去的时间会被拒绝。"
			},
			"note": {
				"type": "string",
				"description": "到时自己会读到的提示词。建议含足够上下文，因为触发时是新 session。"
			}
		},
		"required": ["when", "note"]
	}`),
}

// WithSelfSchedule registers the self_schedule tool. Requires that
// WithCronEngine has already wired r.cronEngine; if it hasn't, this is a no-op.
//
// We keep this as a separate registration call (rather than folding into
// WithCronEngine) so consumers can opt in/out independently — useful if a
// future deployment wants to disable AI self-scheduling without disabling the
// cron_* management tools.
func (r *Registry) WithSelfSchedule() {
	if r.cronEngine == nil {
		return
	}
	r.register(selfScheduleDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSelfSchedule(ctx, input)
	})
}

func (r *Registry) handleSelfSchedule(_ context.Context, input json.RawMessage) (string, error) {
	if r.cronEngine == nil {
		return "", fmt.Errorf("cron engine not configured")
	}
	var p struct {
		When string `json:"when"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %v", err)
	}
	p.When = strings.TrimSpace(p.When)
	p.Note = strings.TrimSpace(p.Note)
	if p.When == "" {
		return "", fmt.Errorf("when 不能为空")
	}
	if p.Note == "" {
		return "", fmt.Errorf("note 不能为空")
	}

	// Resolve tz; fall back to UTC if the host system somehow lacks the
	// Asia/Shanghai zoneinfo (it shouldn't, since CGO_ENABLED=0 binaries embed
	// the standard tzdata).
	tz, err := time.LoadLocation(defaultSelfScheduleTZName)
	if err != nil {
		tz = time.UTC
	}

	fireAt, err := cron.ParseWhen(p.When, tz, time.Now())
	if err != nil {
		return "", err
	}

	// Anti-abuse: count this agent's existing PENDING self_schedule jobs.
	pending := countPendingSelfSchedule(r.cronEngine.ListJobsByAgent(r.agentID), time.Now())
	if pending >= selfScheduleMaxPerAgent {
		return "", fmt.Errorf("已有 %d 条未触发的 self_schedule 提醒（上限 %d），请先用 cron_remove 清理", pending, selfScheduleMaxPerAgent)
	}

	job := &cron.Job{
		Name:    fmt.Sprintf("🔔 %s", truncate(p.Note, 20)),
		Enabled: true,
		AgentID: r.agentID,
		Schedule: cron.Schedule{
			Kind: "at",
			Expr: fireAt.Format(time.RFC3339),
		},
		Payload: cron.Payload{
			Kind:    "agentTurn",
			Message: p.Note,
		},
		Delivery: cron.Delivery{Mode: "announce"},
		Remark:   "self_schedule", // marker so UI / countPendingSelfSchedule recognise origin
	}

	if err := r.cronEngine.Add(job); err != nil {
		return "", fmt.Errorf("创建提醒失败: %w", err)
	}

	until := time.Until(fireAt).Round(time.Second)
	return fmt.Sprintf(
		"✅ 已设定 1 次提醒\n  时间: %s\n  剩余: %s\n  Note: %s\n  任务 ID: %s",
		fireAt.In(tz).Format("2006-01-02 15:04 MST"),
		until,
		truncate(p.Note, 80),
		job.ID,
	), nil
}

// countPendingSelfSchedule counts kind=at jobs whose Remark marker is
// "self_schedule" and whose Expr resolves to a still-future time.
//
// We use Remark as a marker rather than a dedicated Job field to keep the
// patch surface tiny and stay 100% compatible with existing cron storage.
// If a future P1-XX adds a structured `Source` field, switch this to read it.
func countPendingSelfSchedule(jobs []*cron.Job, now time.Time) int {
	count := 0
	for _, j := range jobs {
		if j == nil || !j.Enabled {
			continue
		}
		if j.Remark != "self_schedule" {
			continue
		}
		if j.Schedule.Kind != "at" {
			continue
		}
		t, err := time.Parse(time.RFC3339, j.Schedule.Expr)
		if err != nil {
			continue
		}
		if t.After(now) {
			count++
		}
	}
	return count
}
