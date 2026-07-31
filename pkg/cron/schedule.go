package cron

import (
	"errors"
	"fmt"
	"strings"
	"time"

	cronlib "github.com/robfig/cron/v3"
)

type schedulePlan struct {
	spec string
	at   time.Time
}

type atSchedule struct {
	at time.Time
}

func (s atSchedule) Next(after time.Time) time.Time {
	if after.Before(s.at) {
		return s.at
	}
	return time.Time{}
}

var cronParser = cronlib.NewParser(
	cronlib.Second | cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow | cronlib.Descriptor,
)

func planSchedule(s Schedule, now time.Time) (schedulePlan, error) {
	switch s.Kind {
	case "every":
		if s.EveryMs < 1000 {
			return schedulePlan{}, fmt.Errorf("everyMs must be at least 1000")
		}
		const maxEveryMs = int64((365 * 24 * time.Hour) / time.Millisecond)
		if s.EveryMs > maxEveryMs {
			return schedulePlan{}, fmt.Errorf("everyMs must not exceed one year")
		}
		return schedulePlan{spec: fmt.Sprintf("@every %s", time.Duration(s.EveryMs)*time.Millisecond)}, nil
	case "cron", "":
		expr := strings.TrimSpace(s.Expr)
		if expr == "" {
			return schedulePlan{}, fmt.Errorf("cron expression is required")
		}
		tz := strings.TrimSpace(s.TZ)
		if tz == "" {
			tz = "UTC"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			return schedulePlan{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		spec := "CRON_TZ=" + tz + " " + expr
		if _, err := cronParser.Parse(spec); err != nil {
			spec = "CRON_TZ=" + tz + " 0 " + expr
			if _, retryErr := cronParser.Parse(spec); retryErr != nil {
				return schedulePlan{}, fmt.Errorf("invalid cron expression %q: %w", expr, retryErr)
			}
		}
		return schedulePlan{spec: spec}, nil
	case "at":
		tz := strings.TrimSpace(s.TZ)
		if tz == "" {
			tz = "UTC"
		}
		location, err := time.LoadLocation(tz)
		if err != nil {
			return schedulePlan{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		at, err := ParseWhen(s.Expr, location, now.In(location))
		if err != nil {
			return schedulePlan{}, fmt.Errorf("invalid one-shot time: %w", err)
		}
		return schedulePlan{at: at}, nil
	default:
		return schedulePlan{}, fmt.Errorf("unsupported schedule kind %q", s.Kind)
	}
}

func normalizeJob(job *Job, now time.Time) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	if strings.TrimSpace(job.Name) == "" {
		return fmt.Errorf("job name is required")
	}
	if strings.TrimSpace(job.Payload.Message) == "" {
		return fmt.Errorf("job payload message is required")
	}
	switch job.Payload.Kind {
	case "", "agentTurn", "systemEvent":
	default:
		return fmt.Errorf("unsupported payload kind %q", job.Payload.Kind)
	}
	switch job.Delivery.Mode {
	case "", "announce", "none":
	default:
		return fmt.Errorf("unsupported delivery mode %q", job.Delivery.Mode)
	}
	plan, err := planSchedule(job.Schedule, now)
	if err != nil && !job.Enabled && job.Schedule.Kind == "at" {
		if at, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(job.Schedule.Expr)); parseErr == nil {
			plan = schedulePlan{at: at.UTC()}
			err = nil
		}
	}
	if err != nil {
		return err
	}
	switch job.Schedule.Kind {
	case "at":
		job.Schedule.Expr = plan.at.UTC().Format(time.RFC3339)
		job.Schedule.TZ = "UTC"
	case "cron", "":
		job.Schedule.Kind = "cron"
		if strings.TrimSpace(job.Schedule.TZ) == "" {
			job.Schedule.TZ = "UTC"
		}
	case "every":
		job.Schedule.Expr = ""
		job.Schedule.TZ = ""
	}
	if job.Payload.Kind == "" {
		job.Payload.Kind = "agentTurn"
	}
	if job.Delivery.Mode == "" {
		job.Delivery.Mode = "announce"
	}
	return nil
}

// scheduleJobLocked validates and registers one enabled job.
func (e *Engine) scheduleJobLocked(job *Job) error {
	plan, err := planSchedule(job.Schedule, time.Now())
	if err != nil {
		return fmt.Errorf("schedule job %s: %w", job.ID, err)
	}
	callback := func() {
		if err := e.startJobByID(job.ID, true); err != nil && !errors.Is(err, ErrJobAlreadyRunning) {
			fmt.Printf("cron: failed to start job %s: %v\n", job.ID, err)
		}
	}
	var entryID cronlib.EntryID
	if !plan.at.IsZero() {
		entryID = e.cron.Schedule(atSchedule{at: plan.at}, cronlib.FuncJob(callback))
	} else {
		entryID, err = e.cron.AddFunc(plan.spec, callback)
		if err != nil {
			return fmt.Errorf("schedule job %s: %w", job.ID, err)
		}
	}
	e.entryIDs[job.ID] = entryID
	entry := e.cron.Entry(entryID)
	if !entry.Next.IsZero() {
		job.State.NextRunAtMs = entry.Next.UnixMilli()
	} else if !plan.at.IsZero() {
		job.State.NextRunAtMs = plan.at.UnixMilli()
	}
	return nil
}

func (e *Engine) unscheduleJobLocked(id string) {
	if entryID, ok := e.entryIDs[id]; ok {
		e.cron.Remove(entryID)
		delete(e.entryIDs, id)
	}
}
