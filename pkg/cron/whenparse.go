// pkg/cron/whenparse.go — Parses human-friendly "when" strings into absolute
// fire-times. Used by the self_schedule tool so AI agents can ask for
// "30 minutes from now" or "tomorrow 09:00" without juggling RFC 3339 by hand.
//
// Supported forms (case-insensitive, leading/trailing whitespace ignored):
//
//   30m / 2h / 1h30m         relative duration (Go time.ParseDuration)
//   today HH:MM              today at HH:MM in tz; errors if already past
//   tomorrow [HH:MM]         next day, default 09:00
//   next monday [HH:MM]      next occurrence of weekday, default 09:00
//   next mon                 short form
//   2026-05-10T09:00:00+08:00  explicit ISO-8601 / RFC 3339
//   2026-05-10 09:00         explicit "YYYY-MM-DD HH:MM" interpreted in tz
//
// Design notes:
//   - tz must be a non-nil *time.Location; callers usually pass the agent's
//     configured tz (default Asia/Shanghai).
//   - now is parameterized for testability — production callers pass time.Now().
//   - All parser paths return a fire-time strictly in the future; if the input
//     resolves to the past (e.g. "today 08:00" when it's 09:00) we return an
//     error so the caller can ask the AI to retry rather than silently snap
//     to tomorrow.
package cron

import (
	"fmt"
	"strings"
	"time"
)

// ParseWhen parses a human-friendly schedule string into an absolute UTC time.
//
// tz is used to anchor "today" / "tomorrow" / time-of-day fragments; the
// returned time is always converted to UTC for storage but its wall-clock
// reflects tz when formatted in tz.
func ParseWhen(input string, tz *time.Location, now time.Time) (time.Time, error) {
	if tz == nil {
		tz = time.UTC
	}
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return time.Time{}, fmt.Errorf("when 不能为空")
	}

	// 1) relative duration: "30m", "2h", "1h30m", "45s"
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("时间必须为正数：%q", input)
		}
		return now.Add(d).UTC(), nil
	}

	// 2) explicit ISO-8601 / RFC3339
	if t, err := time.Parse(time.RFC3339, strings.ToUpper(s)); err == nil {
		if !t.After(now) {
			return time.Time{}, fmt.Errorf("时间已过：%s", t.Format(time.RFC3339))
		}
		return t.UTC(), nil
	}

	// 3) "YYYY-MM-DD HH:MM" in tz
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, tz); err == nil {
		if !t.After(now) {
			return time.Time{}, fmt.Errorf("时间已过：%s", t.Format("2006-01-02 15:04 MST"))
		}
		return t.UTC(), nil
	}

	// 4) prefix-based: "today HH:MM" / "tomorrow [HH:MM]" / "next <weekday> [HH:MM]"
	if t, err := parseTodayTomorrowNext(s, tz, now); err == nil {
		if !t.After(now) {
			return time.Time{}, fmt.Errorf("时间已过：%s", t.In(tz).Format("2006-01-02 15:04 MST"))
		}
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("无法解析 when：%q（支持示例：30m / 2h / today 18:30 / tomorrow / tomorrow 09:00 / next monday / 2026-05-10T09:00:00+08:00）", input)
}

// parseTodayTomorrowNext handles the symbolic-prefix forms.
//
// The grammar is intentionally narrow to avoid ambiguity:
//
//   today  HH:MM
//   tomorrow [HH:MM]              default 09:00
//   next <weekday> [HH:MM]         default 09:00
func parseTodayTomorrowNext(s string, tz *time.Location, now time.Time) (time.Time, error) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return time.Time{}, fmt.Errorf("empty")
	}
	defaultHour, defaultMin := 9, 0

	switch parts[0] {
	case "today":
		if len(parts) != 2 {
			return time.Time{}, fmt.Errorf("today 需要 HH:MM")
		}
		hh, mm, err := parseHHMM(parts[1])
		if err != nil {
			return time.Time{}, err
		}
		nowTZ := now.In(tz)
		return time.Date(nowTZ.Year(), nowTZ.Month(), nowTZ.Day(), hh, mm, 0, 0, tz), nil

	case "tomorrow":
		hh, mm := defaultHour, defaultMin
		if len(parts) >= 2 {
			var err error
			hh, mm, err = parseHHMM(parts[1])
			if err != nil {
				return time.Time{}, err
			}
		}
		nowTZ := now.In(tz)
		t := time.Date(nowTZ.Year(), nowTZ.Month(), nowTZ.Day(), hh, mm, 0, 0, tz)
		return t.AddDate(0, 0, 1), nil

	case "next":
		if len(parts) < 2 {
			return time.Time{}, fmt.Errorf("next 需要后跟 weekday")
		}
		wd, ok := parseWeekday(parts[1])
		if !ok {
			return time.Time{}, fmt.Errorf("无法识别的 weekday：%q", parts[1])
		}
		hh, mm := defaultHour, defaultMin
		if len(parts) >= 3 {
			var err error
			hh, mm, err = parseHHMM(parts[2])
			if err != nil {
				return time.Time{}, err
			}
		}
		nowTZ := now.In(tz)
		// Days until target weekday: 1..7 (always advance, even if "next monday"
		// is called on a Monday — that means the FOLLOWING Monday).
		offset := int(wd-nowTZ.Weekday()+7) % 7
		if offset == 0 {
			offset = 7
		}
		t := time.Date(nowTZ.Year(), nowTZ.Month(), nowTZ.Day(), hh, mm, 0, 0, tz)
		return t.AddDate(0, 0, offset), nil
	}
	return time.Time{}, fmt.Errorf("unknown form: %q", parts[0])
}

// parseHHMM accepts "HH:MM" (24h, zero-padded or single-digit hours both ok).
func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("HH:MM 格式不正确：%q", s)
	}
	return t.Hour(), t.Minute(), nil
}

// parseWeekday accepts both full and abbreviated forms.
func parseWeekday(s string) (time.Weekday, bool) {
	switch s {
	case "monday", "mon":
		return time.Monday, true
	case "tuesday", "tue", "tues":
		return time.Tuesday, true
	case "wednesday", "wed":
		return time.Wednesday, true
	case "thursday", "thu", "thur", "thurs":
		return time.Thursday, true
	case "friday", "fri":
		return time.Friday, true
	case "saturday", "sat":
		return time.Saturday, true
	case "sunday", "sun":
		return time.Sunday, true
	}
	return 0, false
}
