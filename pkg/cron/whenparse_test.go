package cron

import (
	"strings"
	"testing"
	"time"
)

// TestParseWhen_Relative — relative durations resolve correctly.
func TestParseWhen_Relative(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, tz)

	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"2h", 2 * time.Hour},
		{"1h30m", 90 * time.Minute},
		{"45s", 45 * time.Second},
		{"  10m  ", 10 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseWhen(c.in, tz, now)
			if err != nil {
				t.Fatalf("ParseWhen(%q): %v", c.in, err)
			}
			if !got.Equal(now.Add(c.want).UTC()) {
				t.Fatalf("ParseWhen(%q) = %v, want %v", c.in, got, now.Add(c.want))
			}
		})
	}
}

// TestParseWhen_NegativeOrZero — refuses 0 / negative durations.
func TestParseWhen_NegativeOrZero(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, tz)

	for _, in := range []string{"0s", "-5m"} {
		if _, err := ParseWhen(in, tz, now); err == nil {
			t.Fatalf("ParseWhen(%q) should error", in)
		}
	}
}

// TestParseWhen_TodayHHMM — same-day with time later than now is fine.
func TestParseWhen_TodayHHMM(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 8, 0, 0, 0, tz)

	got, err := ParseWhen("today 18:30", tz, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 10, 18, 30, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestParseWhen_TodayPast — refuses a same-day time that's already passed.
func TestParseWhen_TodayPast(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 18, 0, 0, 0, tz)

	if _, err := ParseWhen("today 09:00", tz, now); err == nil {
		t.Fatalf("expected error for past 'today' time")
	}
}

// TestParseWhen_Tomorrow — default 09:00.
func TestParseWhen_Tomorrow(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 8, 0, 0, 0, tz)

	got, err := ParseWhen("tomorrow", tz, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 11, 9, 0, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestParseWhen_TomorrowWithTime — explicit time overrides default.
func TestParseWhen_TomorrowWithTime(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 8, 0, 0, 0, tz)

	got, err := ParseWhen("tomorrow 14:30", tz, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 11, 14, 30, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestParseWhen_NextWeekday — "next monday" from Monday means the FOLLOWING
// Monday (7 days forward), not "today" — safer default for AI scheduling.
func TestParseWhen_NextWeekday(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	monday := time.Date(2026, 5, 11, 8, 0, 0, 0, tz) // 2026-05-11 is a Monday

	got, err := ParseWhen("next monday", tz, monday)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 18, 9, 0, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// short form
	if _, err := ParseWhen("next mon 10:00", tz, monday); err != nil {
		t.Fatalf("'next mon 10:00' err: %v", err)
	}
}

// TestParseWhen_RFC3339 — explicit ISO-8601 with timezone.
func TestParseWhen_RFC3339(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 8, 0, 0, 0, tz)

	got, err := ParseWhen("2026-05-10T18:30:00+08:00", tz, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 10, 18, 30, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestParseWhen_RFC3339Past — refuses a past RFC3339.
func TestParseWhen_RFC3339Past(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 18, 0, 0, 0, tz)

	if _, err := ParseWhen("2026-05-10T09:00:00+08:00", tz, now); err == nil {
		t.Fatalf("expected past error")
	}
}

// TestParseWhen_LocalDateTime — "YYYY-MM-DD HH:MM" interpreted in tz.
func TestParseWhen_LocalDateTime(t *testing.T) {
	tz, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 5, 10, 8, 0, 0, 0, tz)

	got, err := ParseWhen("2026-05-10 18:30", tz, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 5, 10, 18, 30, 0, 0, tz).UTC()
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestParseWhen_Empty — explicit empty rejection.
func TestParseWhen_Empty(t *testing.T) {
	if _, err := ParseWhen("  ", time.UTC, time.Now()); err == nil {
		t.Fatalf("expected empty error")
	}
}

// TestParseWhen_Garbage — totally invalid input gives a helpful error message.
func TestParseWhen_Garbage(t *testing.T) {
	_, err := ParseWhen("当然现在", time.UTC, time.Now())
	if err == nil {
		t.Fatalf("expected error")
	}
	// Error message should contain examples to help the AI self-correct.
	if !strings.Contains(err.Error(), "30m") {
		t.Fatalf("error should include format examples, got: %v", err)
	}
}
