package budget

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// fakeClock provides a deterministic time source for tests.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// installFakeClock swaps Now for the duration of the test. Returns a
// cleanup func that restores the real clock.
func installFakeClock(t *testing.T, start time.Time) *fakeClock {
	t.Helper()
	c := &fakeClock{now: start}
	old := Now
	Now = c.Now
	t.Cleanup(func() { Now = old })
	return c
}

func usdt(x string) decimal.Decimal {
	d, _ := decimal.NewFromString(x)
	return d
}

func newGuard(t *testing.T, lim Limits) *Guard {
	t.Helper()
	g, err := New(t.TempDir(), lim, nil)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	return g
}

func Test_AITeam_Guard_FlagOff_AlwaysAllowed(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	// Charge well past the limit; with the flag off, Check should still
	// return Allowed=true (because flag is the master switch).
	g.Charge("alice", "s1", usdt("5.00"))
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatalf("flag off must always allow; got %+v", res)
	}
}

func Test_AITeam_Guard_AgentDailyTriggers(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	g.Charge("alice", "s1", usdt("0.50"))
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("half-spent should still be allowed")
	}
	g.Charge("alice", "s1", usdt("0.60")) // pushes over 1.00
	res := g.Check("alice", "s1")
	if res.Allowed {
		t.Fatalf("over-cap should block; got %+v", res)
	}
	if res.Scope != "agent" || res.PanicReason != "agent_daily" {
		t.Fatalf("expected agent_daily panic; got %+v", res)
	}
	if !res.LimitUSDT.Equal(usdt("1.00")) {
		t.Fatalf("LimitUSDT mismatch: %s", res.LimitUSDT)
	}
}

func Test_AITeam_Guard_GlobalDailyTriggers(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{GlobalDailyUSDT: usdt("2.00")})
	g.Charge("a1", "s1", usdt("1.00"))
	g.Charge("a2", "s2", usdt("1.20"))
	res := g.Check("a3", "s3")
	if res.Allowed || res.Scope != "global" {
		t.Fatalf("global cap should block any agent; got %+v", res)
	}
}

func Test_AITeam_Guard_PerSessionTriggers(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerSessionUSDT: usdt("0.50")})
	g.Charge("alice", "loop-session", usdt("0.55"))
	res := g.Check("alice", "loop-session")
	if res.Allowed || res.Scope != "session" {
		t.Fatalf("session cap should block; got %+v", res)
	}
}

func Test_AITeam_Guard_PanicCooldownReleasesAfterTime(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	clk := installFakeClock(t, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC))
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00"), Cooldown: 30 * time.Minute})
	g.Charge("alice", "s1", usdt("1.50"))

	if g.Check("alice", "s1").Allowed {
		t.Fatal("should be blocked right after over-charge")
	}
	// 29 min in → still blocked
	clk.now = clk.now.Add(29 * time.Minute)
	if g.Check("alice", "s1").Allowed {
		t.Fatal("should still be blocked at 29min < 30min cooldown")
	}
	// 31 min → cooldown elapsed → auto-release
	clk.now = clk.now.Add(2 * time.Minute)
	// Note: with the existing daily charge still above the cap, the
	// next Check sees Panicked=false then re-triggers panic since
	// UsedDailyUSDT is still over the cap. That's intended — cooldown
	// only clears the panic flag; the over-cap will trip again.
	res := g.Check("alice", "s1")
	if res.Allowed {
		t.Fatal("over-cap should re-trigger after cooldown clears")
	}
}

func Test_AITeam_Guard_CrossDayResetsState(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	// Use Asia/Shanghai locally so the test time semantics match the
	// guard's default tz. Start at 23:30 CST so +60min crosses midnight.
	cst, _ := time.LoadLocation("Asia/Shanghai")
	clk := installFakeClock(t, time.Date(2026, 5, 10, 23, 30, 0, 0, cst))
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00"), Cooldown: 5 * time.Minute, TZ: "Asia/Shanghai"})
	g.Charge("alice", "s1", usdt("2.00"))
	if g.Check("alice", "s1").Allowed {
		t.Fatal("over-cap should block")
	}
	// Cross CST midnight by 60 minutes → next day key 2026-05-11 CST.
	clk.now = clk.now.Add(60 * time.Minute)
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatalf("cross-day must reset state; got %+v", res)
	}
}

func Test_AITeam_Guard_ManualReleaseClearsPanic(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	g.Charge("alice", "s1", usdt("2.00"))
	g.Check("alice", "s1") // triggers panic
	if !g.Release("alice", "human", "manual_review") {
		t.Fatal("release should report success")
	}
	// Bring usage just below the cap so the next Check isn't tripped by
	// the same daily over-charge.
	g.agents["alice"].UsedDailyUSDT = usdt("0.50")
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatalf("after release + sub-cap usage, should allow; got %+v", res)
	}
}

func Test_AITeam_Guard_NilCheckIsNoOp(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	var g *Guard
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("nil guard should allow (no-op)")
	}
	g.Charge("alice", "s1", usdt("1")) // must not panic
	if g.Release("alice", "x", "y") {
		t.Fatal("nil guard release should return false")
	}
}

func Test_AITeam_Guard_StatePersistsAcrossRestart(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	dir := t.TempDir()
	g1, _ := New(dir, Limits{PerAgentDailyUSDT: usdt("5.00")}, nil)
	g1.Charge("alice", "s1", usdt("3.50"))
	// Re-open and verify counters survived.
	g2, err := New(dir, Limits{PerAgentDailyUSDT: usdt("5.00")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap := g2.SnapshotJSON()
	a := snap.Agents["alice"]
	if a.UsedDailyUSDT != "3.5" {
		t.Fatalf("expected used 3.5 after restart, got %q", a.UsedDailyUSDT)
	}
}

func Test_AITeam_Guard_AuditLogsTransitions(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	dir := t.TempDir()
	log, _ := audit.New(dir)
	g, _ := New(dir, Limits{PerAgentDailyUSDT: usdt("1.00")}, log)
	g.Charge("alice", "s1", usdt("2.00"))
	g.Check("alice", "s1") // triggers panic
	g.Release("alice", "human", "manual_review")
	g.SetAgentLimit("alice", usdt("5.00"))
	count := log.LineCount()
	if count < 3 {
		t.Fatalf("expected at least 3 audit rows (panic + release + limit_set), got %d", count)
	}
}

func Test_AITeam_Guard_ChargerFromUSDConverts1to1(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("10.00")})
	charger := g.ChargerFromUSD()
	charger("alice", 0.05) // $0.05 USD ≈ 0.05 USDT
	charger("alice", 0.07)
	snap := g.SnapshotJSON()
	if used := snap.Agents["alice"].UsedDailyUSDT; used != "0.12" {
		t.Fatalf("expected 0.12 USDT after 1:1 USD charges, got %q", used)
	}
}

func Test_AITeam_Guard_SnapshotShape(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00"), GlobalDailyUSDT: usdt("5.00"), Cooldown: time.Hour})
	g.Charge("alice", "s1", usdt("0.25"))
	snap := g.SnapshotJSON()
	if !snap.Enabled {
		t.Fatal("snapshot should report enabled=true when flag on")
	}
	if snap.GlobalUsed != "0.25" {
		t.Fatalf("global used: %q", snap.GlobalUsed)
	}
	if !snap.Limits.PerAgentDailyUSDT.Equal(usdt("1.00")) {
		t.Fatalf("limits round-trip: %+v", snap.Limits)
	}
	if _, ok := snap.Agents["alice"]; !ok {
		t.Fatal("alice missing from snapshot")
	}
}
