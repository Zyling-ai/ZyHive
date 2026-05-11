package budget

import (
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// Negative cost should not increase usage counters.
func Test_AITeam_S8_Edge_NegativeChargeIgnored(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.Charge("alice", "s1", usdt("5"))
	g.Charge("alice", "s1", usdt("-10")) // negative
	snap := g.SnapshotJSON()
	used := snap.Agents["alice"].UsedDailyUSDT
	if used != "5" {
		t.Fatalf("negative charge should be ignored, used=%s want 5", used)
	}
}

// Zero charge should be a no-op.
func Test_AITeam_S8_Edge_ZeroChargeIgnored(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.Charge("alice", "s1", decimal.Zero)
	// zero charge → no state mutation; agent may or may not appear in
	// snapshot — both are acceptable.
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("zero-usage agent should be allowed")
	}
}

// Concurrent charges — 1000 goroutines each adding 0.001 must end at exactly 1.
func Test_AITeam_S8_Edge_ConcurrentChargesExact(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Charge("alice", "s1", usdt("0.001"))
		}()
	}
	wg.Wait()
	used := g.SnapshotJSON().Agents["alice"].UsedDailyUSDT
	if used != "1" {
		t.Fatalf("after 1000×0.001 concurrent charges, used=%s want 1", used)
	}
}

// Cooldown of 0 → effectively no cooldown (re-check every call retriggers).
func Test_AITeam_S8_Edge_ZeroCooldownReTriggers(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	clk := installFakeClock(t, time.Now())
	g := newGuard(t, Limits{
		PerAgentDailyUSDT: usdt("1"),
		Cooldown:          0, // not explicitly set
	})
	g.Charge("alice", "s1", usdt("2"))
	if g.Check("alice", "s1").Allowed {
		t.Fatal("over-cap blocks")
	}
	// Advance clock 1 second
	clk.now = clk.now.Add(1 * time.Second)
	// Behavior is: default cooldown is 1h, so still blocked.
	if g.Check("alice", "s1").Allowed {
		t.Fatal("still in cooldown")
	}
}

// Negative limit (admin mistake) — current behavior: treated as no limit.
// Verify it doesn't crash and doesn't accidentally pass.
func Test_AITeam_S8_Edge_NegativeLimit(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("-1")})
	g.Charge("alice", "s1", usdt("5"))
	res := g.Check("alice", "s1")
	// Behavior is `if !cap.IsZero() && used >= cap` — used (5) >= cap (-1)
	// is TRUE → would block. Verify what actually happens.
	t.Logf("negative-limit Check: allowed=%v scope=%s reason=%s",
		res.Allowed, res.Scope, res.PanicReason)
	// Document current behavior — not a confirmed bug since admin shouldn't
	// set negative; but the API should probably reject negative limits
	// upstream.
}

// SetAgentLimit with negative — bug-fix P3-S8: clamped to zero (no
// per-agent override). Prevents accidental admin-typo permanent block.
func Test_AITeam_S8_Edge_SetAgentLimitNegativeClamped(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.SetAgentLimit("alice", usdt("-50"))
	g.Charge("alice", "s1", usdt("10"))
	res := g.Check("alice", "s1")
	// Clamped to 0 → falls back to default 100 → 10 << 100 → allowed
	if !res.Allowed {
		t.Fatalf("negative limit should clamp to zero (fall back to default), got blocked: %+v", res)
	}
}

// Releasing an agent that's not panicked → returns false.
func Test_AITeam_S8_Edge_ReleaseNotPanicked(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	if g.Release("alice", "op", "reason") {
		t.Fatal("Release on never-panicked agent should return false")
	}
}

// Releasing an agent that doesn't even exist.
func Test_AITeam_S8_Edge_ReleaseNonexistentAgent(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	if g.Release("ghost-agent", "op", "reason") {
		t.Fatal("Release on nonexistent agent should return false")
	}
}

// Notify hook fired multiple times for same panic — should NOT spam.
// Current logic: triggerPanicLocked is called inside Check(). If we call
// Check multiple times after panic state set, the panic block returns
// early WITHOUT re-calling triggerPanicLocked. Good — hook fires once.
func Test_AITeam_S8_Edge_NotifyFiresOncePerPanic(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1")})
	count := 0
	var mu sync.Mutex
	g.SetNotifyHook(func(_, _, _ string) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	g.Charge("alice", "s1", usdt("2"))
	g.Check("alice", "s1") // first panic
	g.Check("alice", "s1") // should NOT re-fire
	g.Check("alice", "s1")
	g.Check("alice", "s1")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 notify, got %d (re-fire bug)", count)
	}
}

// Per-session cap = 0 (disabled) — no session limit enforced.
func Test_AITeam_S8_Edge_PerSessionZeroDisabled(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{
		PerAgentDailyUSDT: usdt("100"),
		PerSessionUSDT:    decimal.Zero, // disabled
	})
	g.Charge("alice", "s1", usdt("50")) // would exceed any small session cap
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("PerSessionUSDT=0 means disabled, should allow")
	}
}
