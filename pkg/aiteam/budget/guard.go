// Package budget (under pkg/aiteam/) implements the aiteam *hard* budget
// guard described in PR-003. It complements the ZyHive main-line
// pkg/budget P1-02 *soft* brake — they share the usage signal stream but
// solve different problems:
//
//   pkg/budget (P1-02)        — soft warn + simple hard stop, ephemeral.
//   pkg/aiteam/budget         — hard panic-stop state machine with
//                               cooldown, per-session ceiling, and
//                               persistent panic record.
//
// Activation: ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1 (zero impact when off).
//
// Storage: <dataDir>/aiteam/guard/state.json. The file is written on
// every state transition so a restart preserves panic / cooldown timers.
//
// Currency: every amount is `decimal.Decimal` in USDT (see PLAN § 2.7).
// Records coming from pkg/usage in USD float64 are converted with
// `decimal.NewFromFloat` on ingest; the 1:1 USD↔USDT peg assumption
// matches the wallet ledger we'll build in S5.
//
// Concurrency: a single sync.Mutex guards all state; Check + Charge are
// O(1) operations on small maps and run in the hot path.
package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// Limits is the read-only ceiling configuration. All zero values are
// treated as "unlimited at that scope".
type Limits struct {
	PerAgentDailyUSDT decimal.Decimal `json:"per_agent_daily_usdt"`
	GlobalDailyUSDT   decimal.Decimal `json:"global_daily_usdt"`
	PerSessionUSDT    decimal.Decimal `json:"per_session_usdt"`
	Cooldown          time.Duration   `json:"cooldown_ns"`
	TZ                string          `json:"tz"`
}

// AgentState is the per-agent ledger row in the in-memory map.
type AgentState struct {
	UsedDailyUSDT decimal.Decimal `json:"used_daily_usdt"`
	LimitUSDT     decimal.Decimal `json:"limit_usdt"` // zero → use Limits.PerAgentDailyUSDT
	Panicked      bool            `json:"panicked"`
	PanicAt       time.Time       `json:"panic_at"`
	PanicReason   string          `json:"panic_reason"`
	CooldownUntil time.Time       `json:"cooldown_until"`
}

// SessionState tracks per-session running cost — separate from per-day
// to defeat single-prompt runaway loops that don't violate a daily cap.
type SessionState struct {
	UsedUSDT decimal.Decimal `json:"used_usdt"`
	Panicked bool            `json:"panicked"`
}

// BalanceReader is the minimal interface Guard needs from a wallet
// implementation. *pkg/aiteam/wallet.Store satisfies it. Pluggable so
// guard tests don't need to import the wallet package directly.
type BalanceReader interface {
	Balance(agentID string) decimal.Decimal
}

// Guard is the panic-stop state machine. Safe for concurrent use.
type Guard struct {
	cfg        Limits
	tz         *time.Location
	persistDir string
	audit      *audit.Log

	mu       sync.Mutex
	dayKey   string
	agents   map[string]*AgentState
	sessions map[string]*SessionState
	globalUsed decimal.Decimal

	// wallet — optional BalanceReader (S6). When set, Check() additionally
	// panics on zero / negative balance. Wired via SetWallet from main.go
	// after both subsystems are constructed.
	wallet BalanceReader

	// notifyHook — optional P3-S1 callback fired when a panic is
	// triggered. Receives the agentID, the canonical reason
	// ("agent_daily" / "global_daily" / "session" / "zero_balance"),
	// and a human-readable message. nil = no notification.
	notifyHook func(agentID, reason, message string)
}

// New constructs a Guard. dir is <dataDir>/aiteam/guard; created if
// missing. logIn may be nil (no audit). Limits may be zero values.
// Returns Guard even when the underlying flag is off; callers should
// gate at the call site using flags.BudgetGuardEnabled().
func New(dir string, cfg Limits, logIn *audit.Log) (*Guard, error) {
	if dir == "" {
		return nil, fmt.Errorf("budget: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if cfg.TZ == "" {
		cfg.TZ = "Asia/Shanghai"
	}
	tz, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		tz = time.UTC
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = time.Hour
	}
	g := &Guard{
		cfg:        cfg,
		tz:         tz,
		persistDir: dir,
		audit:      logIn,
		agents:     map[string]*AgentState{},
		sessions:   map[string]*SessionState{},
	}
	g.dayKey = g.todayKey()
	_ = g.loadStateLocked()
	return g, nil
}

// todayKey returns the YYYY-MM-DD key in the configured tz. Uses the
// overridable Now() so tests can fast-forward the clock.
func (g *Guard) todayKey() string {
	return Now().In(g.tz).Format("2006-01-02")
}

// SetWallet wires an optional BalanceReader so the guard can panic on
// zero-balance in addition to the usage-cap checks. Idempotent. Pass
// nil to detach.
func (g *Guard) SetWallet(w BalanceReader) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.wallet = w
}

// SetNotifyHook wires an optional callback fired once per panic
// transition. Called WITHOUT holding g.mu so the hook is free to do
// network IO. Idempotent. Pass nil to detach.
//
// The hook receives:
//   * agentID — the agent that just panicked
//   * reason — canonical: "agent_daily" / "global_daily" / "session" / "zero_balance"
//   * message — pre-formatted human-readable line for operators
func (g *Guard) SetNotifyHook(hook func(agentID, reason, message string)) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.notifyHook = hook
}

// stateFile is the persistence path.
func (g *Guard) stateFile() string { return filepath.Join(g.persistDir, "state.json") }

// diskState mirrors what we persist to JSON.
type diskState struct {
	DayKey     string                   `json:"day_key"`
	Agents     map[string]*AgentState   `json:"agents"`
	Sessions   map[string]*SessionState `json:"sessions"`
	GlobalUsed decimal.Decimal          `json:"global_used_usdt"`
}

func (g *Guard) loadStateLocked() error {
	data, err := os.ReadFile(g.stateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var d diskState
	if uErr := json.Unmarshal(data, &d); uErr != nil {
		return uErr
	}
	g.dayKey = d.DayKey
	if d.Agents != nil {
		g.agents = d.Agents
	}
	if d.Sessions != nil {
		g.sessions = d.Sessions
	}
	g.globalUsed = d.GlobalUsed
	g.rotateIfNeededLocked() // may immediately clear stale day-scoped state
	return nil
}

func (g *Guard) saveStateLocked() {
	d := diskState{
		DayKey:     g.dayKey,
		Agents:     g.agents,
		Sessions:   g.sessions,
		GlobalUsed: g.globalUsed,
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return
	}
	tmp := g.stateFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, g.stateFile())
}

// rotateIfNeededLocked resets day-scoped state on date rollover. Also
// auto-clears panic state on cross-day per PLAN § 0 Q3 (recovery
// strategy B + C: cooldown unless next day).
func (g *Guard) rotateIfNeededLocked() {
	k := g.todayKey()
	if k == g.dayKey {
		return
	}
	g.dayKey = k
	g.agents = map[string]*AgentState{}
	g.sessions = map[string]*SessionState{}
	g.globalUsed = decimal.Zero
}

// Now is overridable for tests.
var Now = time.Now

// Charge increments the per-day / per-session / global counters by
// costUSDT. Negative or zero values are silently ignored. Hooked from
// pkg/usage.SetBudgetCharger (which currently delivers USD float64 →
// adapter converts to USDT 1:1 in main.go).
func (g *Guard) Charge(agentID, sessionID string, costUSDT decimal.Decimal) {
	if g == nil || costUSDT.LessThanOrEqual(decimal.Zero) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rotateIfNeededLocked()

	a := g.getAgentLocked(agentID)
	a.UsedDailyUSDT = a.UsedDailyUSDT.Add(costUSDT)
	g.globalUsed = g.globalUsed.Add(costUSDT)
	if sessionID != "" {
		s := g.getSessionLocked(sessionID)
		s.UsedUSDT = s.UsedUSDT.Add(costUSDT)
	}
	g.saveStateLocked()
}

func (g *Guard) getAgentLocked(id string) *AgentState {
	a, ok := g.agents[id]
	if !ok {
		a = &AgentState{}
		g.agents[id] = a
	}
	return a
}

func (g *Guard) getSessionLocked(id string) *SessionState {
	s, ok := g.sessions[id]
	if !ok {
		s = &SessionState{}
		g.sessions[id] = s
	}
	return s
}

// CheckResult is the verdict returned to runner / chat handler.
//
// Scope and PanicReason are filled when Allowed=false to help callers
// surface a useful error message to the LLM stream.
type CheckResult struct {
	Allowed       bool
	Scope         string          // "agent" | "global" | "session" | "panic"
	UsedUSDT      decimal.Decimal
	LimitUSDT     decimal.Decimal
	PanicReason   string
	CooldownUntil time.Time
}

// Check is the pre-LLM gate. Called every turn in the runner. Returns
// Allowed=false when:
//   1. agent is currently panicked + cooldown not yet elapsed
//   2. per-agent daily cap exhausted
//   3. global daily cap exhausted
//   4. per-session cap exhausted
//
// When the gate trips for reasons 2-4 the agent's panic flag is set
// and an audit row is written.
//
// When the guard flag is off (default), Check returns Allowed=true
// unconditionally — no audit, no state mutation.
func (g *Guard) Check(agentID, sessionID string) CheckResult {
	if g == nil || !flags.BudgetGuardEnabled() {
		return CheckResult{Allowed: true}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rotateIfNeededLocked()

	a := g.getAgentLocked(agentID)

	// 0. Zero-balance check (S6 — Guard × Wallet integration). When a
	// wallet reader is wired AND the agent's USDT balance is non-positive,
	// trigger panic with reason "zero_balance". Done BEFORE the existing-
	// panic check so a fresh over-spend immediately flips state instead
	// of waiting for the cooldown path to release first.
	if g.wallet != nil {
		bal := g.wallet.Balance(agentID)
		if !bal.IsPositive() {
			// Only trigger if not already panicked for the same reason
			// (avoid resetting CooldownUntil on every Check).
			if !a.Panicked || a.PanicReason != "zero_balance" {
				g.triggerPanicLocked(a, agentID, sessionID, "zero_balance", decimal.Zero)
			}
			return CheckResult{
				Allowed:       false,
				Scope:         "wallet",
				UsedUSDT:      bal,
				LimitUSDT:     decimal.Zero,
				PanicReason:   "zero_balance",
				CooldownUntil: a.CooldownUntil,
			}
		}
	}

	// 1. Existing panic — release only when cooldown elapsed.
	if a.Panicked {
		if Now().Before(a.CooldownUntil) {
			return CheckResult{
				Allowed:       false,
				Scope:         "panic",
				UsedUSDT:      a.UsedDailyUSDT,
				LimitUSDT:     g.effectiveAgentLimit(a),
				PanicReason:   a.PanicReason,
				CooldownUntil: a.CooldownUntil,
			}
		}
		// Cooldown elapsed → auto-clear.
		a.Panicked = false
		a.PanicReason = ""
		g.appendAuditLocked("guard.cooldown_elapsed", agentID, sessionID, map[string]any{
			"agent_used_usdt": a.UsedDailyUSDT.String(),
		})
		g.saveStateLocked()
	}

	// 2. Per-agent daily cap.
	if cap := g.effectiveAgentLimit(a); !cap.IsZero() && a.UsedDailyUSDT.GreaterThanOrEqual(cap) {
		g.triggerPanicLocked(a, agentID, sessionID, "agent_daily", cap)
		return CheckResult{
			Allowed:       false,
			Scope:         "agent",
			UsedUSDT:      a.UsedDailyUSDT,
			LimitUSDT:     cap,
			PanicReason:   "agent_daily",
			CooldownUntil: a.CooldownUntil,
		}
	}

	// 3. Global daily cap.
	if !g.cfg.GlobalDailyUSDT.IsZero() && g.globalUsed.GreaterThanOrEqual(g.cfg.GlobalDailyUSDT) {
		g.triggerPanicLocked(a, agentID, sessionID, "global_daily", g.cfg.GlobalDailyUSDT)
		return CheckResult{
			Allowed:       false,
			Scope:         "global",
			UsedUSDT:      g.globalUsed,
			LimitUSDT:     g.cfg.GlobalDailyUSDT,
			PanicReason:   "global_daily",
			CooldownUntil: a.CooldownUntil,
		}
	}

	// 4. Per-session cap.
	if sessionID != "" && !g.cfg.PerSessionUSDT.IsZero() {
		s := g.getSessionLocked(sessionID)
		if s.UsedUSDT.GreaterThanOrEqual(g.cfg.PerSessionUSDT) {
			g.triggerPanicLocked(a, agentID, sessionID, "session", g.cfg.PerSessionUSDT)
			s.Panicked = true
			g.saveStateLocked()
			return CheckResult{
				Allowed:       false,
				Scope:         "session",
				UsedUSDT:      s.UsedUSDT,
				LimitUSDT:     g.cfg.PerSessionUSDT,
				PanicReason:   "session",
				CooldownUntil: a.CooldownUntil,
			}
		}
	}

	return CheckResult{Allowed: true, UsedUSDT: a.UsedDailyUSDT, LimitUSDT: g.effectiveAgentLimit(a)}
}

func (g *Guard) effectiveAgentLimit(a *AgentState) decimal.Decimal {
	if !a.LimitUSDT.IsZero() {
		return a.LimitUSDT
	}
	return g.cfg.PerAgentDailyUSDT
}

func (g *Guard) triggerPanicLocked(a *AgentState, agentID, sessionID, reason string, cap decimal.Decimal) {
	now := Now()
	a.Panicked = true
	a.PanicAt = now
	a.PanicReason = reason
	a.CooldownUntil = now.Add(g.cfg.Cooldown)
	g.appendAuditLocked("guard.panic", agentID, sessionID, map[string]any{
		"reason":          reason,
		"used_usdt":       a.UsedDailyUSDT.String(),
		"cap_usdt":        cap.String(),
		"cooldown_until":  a.CooldownUntil.UnixMilli(),
	})
	g.saveStateLocked()

	// P3-S1: fire the notification hook OUTSIDE the mutex so the hook
	// is free to do network IO (Telegram / Feishu push). Snapshot the
	// hook + formatted message under the lock; release; then call.
	if g.notifyHook != nil {
		hook := g.notifyHook
		msg := formatPanicMessage(agentID, reason, a.UsedDailyUSDT, cap, a.CooldownUntil)
		// Defer-style release: we're inside Check/Charge which holds
		// g.mu. The hook should not be invoked while holding mu, so we
		// spawn it on a goroutine. Errors / panics in the hook do not
		// affect the guard's correctness.
		go func() {
			defer func() { _ = recover() }()
			hook(agentID, reason, msg)
		}()
	}
}

// formatPanicMessage builds the human-readable line passed to the
// notify hook. Kept stable so dashboards / pushed messages stay
// grep-able.
func formatPanicMessage(agentID, reason string, used, cap decimal.Decimal, cooldownUntil time.Time) string {
	reasonZH := map[string]string{
		"agent_daily":   "agent 日累计超限",
		"global_daily":  "全局日累计超限",
		"session":       "单 session 超限",
		"zero_balance":  "钱包余额耗尽",
	}[reason]
	if reasonZH == "" {
		reasonZH = reason
	}
	cooldownDesc := cooldownUntil.Format("2006-01-02 15:04")
	return fmt.Sprintf(
		"⚠️ aiteam 护栏触发熔断\nagent: %s\n原因: %s (%s)\n已用: %s USDT / 上限: %s USDT\n冷却到: %s",
		agentID, reasonZH, reason, used.String(), cap.String(), cooldownDesc,
	)
}

// appendAuditLocked writes to audit.log if configured; never blocks long
// because audit.Append fsyncs and we're under g.mu.
func (g *Guard) appendAuditLocked(t, agentID, sessionID string, detail map[string]any) {
	if g.audit == nil {
		return
	}
	_ = g.audit.Append(audit.Entry{
		Type:      t,
		Subsystem: "guard",
		AgentID:   agentID,
		SessionID: sessionID,
		Detail:    detail,
	})
}

// Release manually clears an agent's panic state. operator/reason are
// audit metadata.
func (g *Guard) Release(agentID, operator, reason string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.agents[agentID]
	if !ok || !a.Panicked {
		return false
	}
	a.Panicked = false
	a.PanicReason = ""
	a.CooldownUntil = time.Time{}
	g.appendAuditLocked("guard.release", agentID, "", map[string]any{
		"operator": operator,
		"reason":   reason,
	})
	g.saveStateLocked()
	return true
}

// SetAgentLimit overrides Limits.PerAgentDailyUSDT for a single agent.
// Zero clears the override.
//
// BUG-FIX P3-S8: previously accepted negative limits, which then made
// `used >= cap` always true (used ≥ 0 ≥ negative cap), permanently
// panic-stopping the agent. We now clamp negative input to zero
// (= no per-agent override; fall back to default). An audit row
// records the clamp so operators see the correction.
func (g *Guard) SetAgentLimit(agentID string, limitUSDT decimal.Decimal) {
	if g == nil {
		return
	}
	clamped := limitUSDT
	if limitUSDT.IsNegative() {
		clamped = decimal.Zero
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rotateIfNeededLocked()
	a := g.getAgentLocked(agentID)
	a.LimitUSDT = clamped
	g.appendAuditLocked("guard.limit_set", agentID, "", map[string]any{
		"limit_usdt":         clamped.String(),
		"original_input":     limitUSDT.String(),
		"clamped_to_zero":    !limitUSDT.Equal(clamped),
	})
	g.saveStateLocked()
}

// Snapshot is the read-only state for /api/aiteam/guard.
type Snapshot struct {
	Enabled    bool                      `json:"enabled"`
	DayKey     string                    `json:"day_key"`
	TZ         string                    `json:"tz"`
	GlobalUsed string                    `json:"global_used_usdt"`
	Limits     Limits                    `json:"limits"`
	Agents     map[string]AgentSnapshot  `json:"agents"`
	Sessions   map[string]SessionSnapshot `json:"sessions,omitempty"`
}

type AgentSnapshot struct {
	UsedDailyUSDT  string    `json:"used_daily_usdt"`
	EffectiveLimit string    `json:"effective_limit_usdt"`
	Panicked       bool      `json:"panicked"`
	PanicReason    string    `json:"panic_reason,omitempty"`
	CooldownUntil  time.Time `json:"cooldown_until,omitempty"`
}

type SessionSnapshot struct {
	UsedUSDT string `json:"used_usdt"`
	Panicked bool   `json:"panicked,omitempty"`
}

func (g *Guard) SnapshotJSON() Snapshot {
	if g == nil {
		return Snapshot{Enabled: false}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rotateIfNeededLocked()
	out := Snapshot{
		Enabled:    flags.BudgetGuardEnabled(),
		DayKey:     g.dayKey,
		TZ:         g.tz.String(),
		GlobalUsed: g.globalUsed.String(),
		Limits:     g.cfg,
		Agents:     map[string]AgentSnapshot{},
		Sessions:   map[string]SessionSnapshot{},
	}
	for id, a := range g.agents {
		out.Agents[id] = AgentSnapshot{
			UsedDailyUSDT:  a.UsedDailyUSDT.String(),
			EffectiveLimit: g.effectiveAgentLimit(a).String(),
			Panicked:       a.Panicked,
			PanicReason:    a.PanicReason,
			CooldownUntil:  a.CooldownUntil,
		}
	}
	for id, s := range g.sessions {
		out.Sessions[id] = SessionSnapshot{UsedUSDT: s.UsedUSDT.String(), Panicked: s.Panicked}
	}
	return out
}

// ChargerFromUSD returns a func(agentID, costUSD float64) suitable for
// passing to pkg/usage.SetBudgetCharger. It converts USD float64 to
// USDT decimal 1:1 (peg assumption). sessionID is unknown at the usage
// callback site so we Charge without it — per-session tracking has to
// be wired separately via Charge with full args from the runner if
// needed.
func (g *Guard) ChargerFromUSD() func(agentID string, costUSD float64) {
	if g == nil {
		return nil
	}
	return func(agentID string, costUSD float64) {
		g.Charge(agentID, "", decimal.NewFromFloat(costUSD))
	}
}
