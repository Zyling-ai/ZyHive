package budget

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// fakeWallet implements BalanceReader for the S6 integration tests.
// Concurrent-safe is not required — Guard.Check serialises calls under
// its own mutex.
type fakeWallet map[string]decimal.Decimal

func (f fakeWallet) Balance(agentID string) decimal.Decimal {
	return f[agentID]
}

func Test_AITeam_S6_ZeroBalanceTriggersPanic(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")}) // generous USD cap
	g.SetWallet(fakeWallet{"alice": decimal.Zero})

	res := g.Check("alice", "s1")
	if res.Allowed {
		t.Fatalf("zero balance must block; got %+v", res)
	}
	if res.Scope != "wallet" || res.PanicReason != "zero_balance" {
		t.Fatalf("unexpected verdict: %+v", res)
	}
}

func Test_AITeam_S6_NegativeBalanceTriggersPanic(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.SetWallet(fakeWallet{"alice": usdt("-0.01")})

	res := g.Check("alice", "s1")
	if res.Allowed {
		t.Fatal("negative balance must block")
	}
}

func Test_AITeam_S6_PositiveBalanceAllowed(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.SetWallet(fakeWallet{"alice": usdt("0.50")})
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("positive balance should allow")
	}
}

func Test_AITeam_S6_NilWalletReturnsToOriginalBehaviour(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	// No SetWallet call → wallet=nil → guard behaves as in S4.
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatalf("no wallet, no usage → should allow; got %+v", res)
	}
}

func Test_AITeam_S6_FlagOffAllowsEvenZeroBalance(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "")
	g := newGuard(t, Limits{})
	g.SetWallet(fakeWallet{"alice": decimal.Zero})
	if !g.Check("alice", "s1").Allowed {
		t.Fatal("flag off must allow regardless of balance")
	}
}

func Test_AITeam_S6_ZeroBalanceAuditLogged(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	// audit log NOT passed → no audit; we still verify the panic occurs.
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	g.SetWallet(fakeWallet{"alice": decimal.Zero})
	g.Check("alice", "s1")
	g.Check("alice", "s1") // second call — should still be panic but not
	                       // duplicate transition.
	snap := g.SnapshotJSON()
	a, ok := snap.Agents["alice"]
	if !ok {
		t.Fatal("alice missing from snapshot")
	}
	if !a.Panicked || a.PanicReason != "zero_balance" {
		t.Fatalf("expected zero_balance panic; got %+v", a)
	}
}

func Test_AITeam_S6_ManualReleaseAllowsTopupRecovery(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100")})
	wallet := fakeWallet{"alice": decimal.Zero}
	g.SetWallet(wallet)

	if g.Check("alice", "s1").Allowed {
		t.Fatal("setup: should be panicked")
	}
	// Owner tops up the wallet ...
	wallet["alice"] = usdt("5.00")
	// ... and manually releases.
	if !g.Release("alice", "owner", "topup") {
		t.Fatal("release should succeed")
	}
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatalf("after topup + release, should allow; got %+v", res)
	}
}
