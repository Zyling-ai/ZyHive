// Package budget provides per-agent and global daily USD budget tracking
// with hard-stop enforcement before a runner enters the LLM loop.
//
// Why:
//   - Once an agent has self_schedule + agent_spawn + multi-channel inbound,
//     a single misconfigured loop can burn dozens of USD before the user notices.
//   - pkg/usage already records every LLM call's cost; this package consumes
//     that signal in real time and acts on it.
//
// Design:
//   - In-memory only. The source of truth for historical usage is the JSONL
//     in pkg/usage; budget is a derivative view of "today" that resets on
//     date rollover (in the configured timezone, default Asia/Shanghai).
//   - Disabled by default. Operators opt in via zyhive.json.
//   - Soft warn (>= warnPct) and hard stop (>= 100%) are the only states the
//     callers see.
//   - Topup is per-day, additive credit; it lapses on the next day rollover.
package budget

import (
	"strconv"
	"sync"
	"time"
)

// Config controls budget behaviour. Wire from zyhive.json `budget` block.
//
// Defaults (when Enabled is true but other fields are 0/empty):
//   GlobalDailyUSD = 0   (no global cap)
//   DefaultAgentDailyUSD = 0  (no per-agent cap)
//   WarnAtPct = 80
//   TZ = "Asia/Shanghai"
type Config struct {
	Enabled              bool    `json:"enabled"`
	GlobalDailyUSD       float64 `json:"global_daily_usd"`
	DefaultAgentDailyUSD float64 `json:"default_agent_daily_usd"`
	WarnAtPct            int     `json:"warn_at_pct"`
	TZ                   string  `json:"tz"`
}

// Store maintains the running per-day usage map.
//
// Thread-safe. Charge() is on the hot path (every UsageRecord append) so all
// mutations use a single mutex with O(1) operations.
type Store struct {
	cfg Config
	tz  *time.Location

	mu      sync.Mutex
	dayKey  string // current day key, e.g. "2026-05-09" in cfg.TZ
	agents  map[string]float64 // dayKey-scoped: agent_id → USD used today
	global  float64            // dayKey-scoped: total USD used today
	limits  map[string]float64 // permanent: agent_id → daily USD limit override
	topups  map[string]float64 // dayKey-scoped: agent_id → emergency credit (or "" key for global)
}

// NewStore constructs a Store from cfg.
func NewStore(cfg Config) *Store {
	tz, err := time.LoadLocation(cfg.TZ)
	if err != nil || cfg.TZ == "" {
		tz, _ = time.LoadLocation("Asia/Shanghai")
		if tz == nil {
			tz = time.UTC
		}
	}
	if cfg.WarnAtPct <= 0 || cfg.WarnAtPct >= 100 {
		cfg.WarnAtPct = 80
	}
	s := &Store{
		cfg:    cfg,
		tz:     tz,
		agents: map[string]float64{},
		limits: map[string]float64{},
		topups: map[string]float64{},
	}
	s.dayKey = s.todayKey()
	return s
}

// Enabled reports whether enforcement is active.
func (s *Store) Enabled() bool { return s != nil && s.cfg.Enabled }

func (s *Store) todayKey() string {
	return time.Now().In(s.tz).Format("2006-01-02")
}

// rotateIfNeededLocked clears day-scoped state when the date has rolled.
// Caller must hold s.mu.
func (s *Store) rotateIfNeededLocked() {
	k := s.todayKey()
	if k == s.dayKey {
		return
	}
	s.dayKey = k
	s.agents = map[string]float64{}
	s.global = 0
	s.topups = map[string]float64{}
}

// SetLimit sets a per-agent daily USD limit. Pass 0 to remove (fall back to
// DefaultAgentDailyUSD).
func (s *Store) SetLimit(agentID string, dailyUSD float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if dailyUSD <= 0 {
		delete(s.limits, agentID)
		return
	}
	s.limits[agentID] = dailyUSD
}

// LimitFor returns the effective daily USD cap for an agent. Returns 0 when
// no cap is set (effectively unlimited at the agent layer; global cap may
// still apply).
func (s *Store) LimitFor(agentID string) float64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.limits[agentID]; ok {
		return v
	}
	return s.cfg.DefaultAgentDailyUSD
}

// Charge records that `costUSD` was spent by `agentID` and the global pool.
// Hooked from pkg/usage on every LLM call regardless of cfg.Enabled (so we
// can flip Enabled on at runtime and have accurate state immediately).
//
// Negative or zero values are silently ignored.
func (s *Store) Charge(agentID string, costUSD float64) {
	if s == nil || costUSD <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()
	s.agents[agentID] += costUSD
	s.global += costUSD
}

// Topup adds emergency credit for the current day. Pass agentID="" for a
// global topup. The credit lapses at next day rollover.
func (s *Store) Topup(agentID string, addUSD float64) {
	if s == nil || addUSD <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()
	s.topups[agentID] += addUSD
}

// Snapshot is the read-only state struct returned to API consumers and to
// the runner pre-flight check.
type Snapshot struct {
	Enabled        bool             `json:"enabled"`
	DayKey         string           `json:"day_key"`
	TZ             string           `json:"tz"`
	GlobalUsed     float64          `json:"global_used_usd"`
	GlobalLimit    float64          `json:"global_limit_usd"`
	GlobalTopup    float64          `json:"global_topup_usd"`
	GlobalRemaining float64         `json:"global_remaining_usd"` // -1 = unlimited
	WarnAtPct      int              `json:"warn_at_pct"`
	Agents         []AgentSnapshot  `json:"agents"`
}

// AgentSnapshot is the per-agent breakdown.
type AgentSnapshot struct {
	AgentID    string  `json:"agent_id"`
	Used       float64 `json:"used_usd"`
	Limit      float64 `json:"limit_usd"`     // 0 = no per-agent cap
	Topup      float64 `json:"topup_usd"`
	Remaining  float64 `json:"remaining_usd"` // -1 = unlimited at agent layer
	WarnLevel  string  `json:"warn_level"`     // "ok" | "warn" | "exceeded"
}

// SnapshotFor returns a snapshot. If agentIDs is non-empty, only those agents
// are included; otherwise all known (= those that have charged today or have
// a limit set) agents are returned.
func (s *Store) SnapshotFor(agentIDs []string) Snapshot {
	if s == nil {
		return Snapshot{Enabled: false}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()

	out := Snapshot{
		Enabled:     s.cfg.Enabled,
		DayKey:      s.dayKey,
		TZ:          s.tz.String(),
		GlobalUsed:  s.global,
		GlobalLimit: s.cfg.GlobalDailyUSD,
		GlobalTopup: s.topups[""],
		WarnAtPct:   s.cfg.WarnAtPct,
	}
	out.GlobalRemaining = remainingLocked(s.global, s.cfg.GlobalDailyUSD, s.topups[""])

	if len(agentIDs) == 0 {
		seen := map[string]struct{}{}
		for id := range s.agents {
			seen[id] = struct{}{}
		}
		for id := range s.limits {
			seen[id] = struct{}{}
		}
		agentIDs = make([]string, 0, len(seen))
		for id := range seen {
			agentIDs = append(agentIDs, id)
		}
	}

	for _, id := range agentIDs {
		used := s.agents[id]
		limit := s.limits[id]
		if limit == 0 {
			limit = s.cfg.DefaultAgentDailyUSD
		}
		topup := s.topups[id]
		rem := remainingLocked(used, limit, topup)
		warn := warnLevelLocked(used, limit, topup, s.cfg.WarnAtPct)
		out.Agents = append(out.Agents, AgentSnapshot{
			AgentID:   id,
			Used:      used,
			Limit:     limit,
			Topup:     topup,
			Remaining: rem,
			WarnLevel: warn,
		})
	}
	return out
}

// remainingLocked computes the remaining USD given used, limit, and topup.
// Returns -1 to signal "unlimited" when limit == 0.
func remainingLocked(used, limit, topup float64) float64 {
	if limit <= 0 {
		return -1
	}
	rem := limit + topup - used
	if rem < 0 {
		return 0
	}
	return rem
}

// warnLevelLocked classifies current state as "ok" | "warn" | "exceeded".
func warnLevelLocked(used, limit, topup float64, warnPct int) string {
	if limit <= 0 {
		return "ok"
	}
	effLimit := limit + topup
	if used >= effLimit {
		return "exceeded"
	}
	pct := used / effLimit * 100
	if pct >= float64(warnPct) {
		return "warn"
	}
	return "ok"
}

// ── Enforcement ─────────────────────────────────────────────────────────────

// CheckResult is what BeforeRun returns to the runner.
type CheckResult struct {
	Allowed       bool
	Scope         string  // "agent" | "global" when blocked
	Used          float64
	EffectiveCap  float64 // limit + topup
	Reason        string
	WarnInjection string  // when non-empty, append to system prompt as soft warning
}

// BeforeRun is called by the runner at turn entry. When Enabled is false this
// is always Allowed=true with no warning. When Enabled is true, returns:
//   - Allowed=false if global or agent budget is exhausted
//   - WarnInjection set when use is past WarnAtPct of any cap
func (s *Store) BeforeRun(agentID string) CheckResult {
	if s == nil || !s.cfg.Enabled {
		return CheckResult{Allowed: true}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()

	// 1. Global cap
	if s.cfg.GlobalDailyUSD > 0 {
		effGlobal := s.cfg.GlobalDailyUSD + s.topups[""]
		if s.global >= effGlobal {
			return CheckResult{
				Allowed:      false,
				Scope:        "global",
				Used:         s.global,
				EffectiveCap: effGlobal,
				Reason:       "global daily budget exhausted",
			}
		}
	}

	// 2. Per-agent cap
	limit := s.limits[agentID]
	if limit == 0 {
		limit = s.cfg.DefaultAgentDailyUSD
	}
	used := s.agents[agentID]
	if limit > 0 {
		effLimit := limit + s.topups[agentID]
		if used >= effLimit {
			return CheckResult{
				Allowed:      false,
				Scope:        "agent",
				Used:         used,
				EffectiveCap: effLimit,
				Reason:       "agent daily budget exhausted",
			}
		}
		// Warn injection
		pct := used / effLimit * 100
		if pct >= float64(s.cfg.WarnAtPct) {
			return CheckResult{
				Allowed:      true,
				Used:         used,
				EffectiveCap: effLimit,
				WarnInjection: warnInjectionText(used, effLimit, pct, s.cfg.WarnAtPct),
			}
		}
	}

	return CheckResult{Allowed: true, Used: used, EffectiveCap: limit + s.topups[agentID]}
}

func warnInjectionText(used, cap float64, pct float64, warnAt int) string {
	rem := cap - used
	if rem < 0 {
		rem = 0
	}
	return "## 预算提醒\n" +
		"今日预算已用 " + fmtUSD(used) + " / " + fmtUSD(cap) +
		" (" + fmtPct(pct) + "% , 阈值 " + fmtPctInt(warnAt) + "%)。" +
		"剩余 " + fmtUSD(rem) + "。请尽量给出精炼回答，避免不必要的工具循环。"
}

func fmtUSD(v float64) string {
	return "$" + strconv.FormatFloat(v, 'f', 4, 64)
}
func fmtPct(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func fmtPctInt(v int) string  { return strconv.Itoa(v) }
