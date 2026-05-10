// Package aiteam_test holds the Genesis end-to-end demo — proves that
// every subsystem (S0-S9) composes correctly without going through HTTP.
//
// Scenario:
//   1. owner credits alice $5 USDT (PR-001 wallet)
//   2. usage hook charges $0.30 for an LLM call → brake + guard + wallet
//      all see it (S4 / S5 / S6 wiring via SetBudgetCharger)
//   3. judge produces a heuristic score (PR-004)
//   4. payroll runs and credits net pay (PR-002 = base + bonus - offset)
//   5. revenue webhook delivers a $50 task payout split 60/40 with bob
//   6. final balances reconcile

package genesis_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	aiteamAudit "github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	aiteamBudget "github.com/Zyling-ai/zyhive/pkg/aiteam/budget"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
	aiteamFX "github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
	aiteamJudge "github.com/Zyling-ai/zyhive/pkg/aiteam/judge"
	aiteamPayroll "github.com/Zyling-ai/zyhive/pkg/aiteam/payroll"
	aiteamRevenue "github.com/Zyling-ai/zyhive/pkg/aiteam/revenue"
	aiteamWallet "github.com/Zyling-ai/zyhive/pkg/aiteam/wallet"
)

// fakeUsage implements payroll.UsageReader for the e2e demo.
type fakeUsage struct{ m map[string]float64 }

func (f *fakeUsage) UsageOn(agentID, _ string) float64 { return f.m[agentID] }

func usdt(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

func Test_AITeam_Genesis_E2E_FullScenario(t *testing.T) {
	// Turn every flag on for the duration of the test.
	t.Setenv(flags.EnvWallet, "1")
	t.Setenv(flags.EnvBudgetGuard, "1")
	t.Setenv(flags.EnvJudge, "1")
	t.Setenv(flags.EnvPayroll, "1")
	t.Setenv(flags.EnvRevenue, "1")

	root := t.TempDir()

	// ---- shared audit log (every subsystem feeds it) ----
	audit, err := aiteamAudit.New(root)
	if err != nil {
		t.Fatal(err)
	}

	// ---- FX (display-only, S5) ----
	fxSvc := aiteamFX.New("")

	// ---- Wallet (S5) ----
	wallet, err := aiteamWallet.New(root+"/wallet", fxSvc, audit)
	if err != nil {
		t.Fatal(err)
	}

	// ---- BudgetGuard (S4 + S6 linkage) ----
	guard, err := aiteamBudget.New(root+"/guard", aiteamBudget.Limits{
		PerAgentDailyUSDT: usdt("10.00"),
		Cooldown:          time.Hour,
	}, audit)
	if err != nil {
		t.Fatal(err)
	}
	guard.SetWallet(wallet) // S6: zero balance = panic

	// ---- Judge (S7) ----
	judge, err := aiteamJudge.New(root+"/judge", nil)
	if err != nil {
		t.Fatal(err)
	}

	// ---- Payroll (S8) — wired to judge + wallet + a fake usage source ----
	usage := &fakeUsage{m: map[string]float64{"alice": 0.30, "bob": 0.05}}
	payroll, err := aiteamPayroll.New(root+"/payroll", aiteamPayroll.DefaultConfig(),
		judge, func(id string, amt decimal.Decimal, reason string) error {
			_, e := wallet.Credit(id, amt, reason)
			return e
		}, usage, audit)
	if err != nil {
		t.Fatal(err)
	}

	// ---- Revenue ingester (S9) ----
	revIng, err := aiteamRevenue.New(root+"/revenue", aiteamRevenue.Config{
		Secret:          []byte("test-genesis-secret-32-bytes-padding"),
		FreshnessWindow: 5 * time.Minute,
	}, func(id string, amt decimal.Decimal, reason string) error {
		_, e := wallet.Credit(id, amt, reason)
		return e
	}, audit)
	if err != nil {
		t.Fatal(err)
	}

	// ---- 1. genesis credit: owner gives alice $5 ----
	if _, err := wallet.Credit("alice", usdt("5.00"), "genesis"); err != nil {
		t.Fatalf("genesis credit: %v", err)
	}
	if !wallet.Balance("alice").Equal(usdt("5")) {
		t.Fatalf("alice balance after genesis: %s, want 5", wallet.Balance("alice"))
	}

	// ---- 2. usage hook: alice runs an LLM call costing $0.30 ----
	// Simulate the SetBudgetCharger fan-out main.go installs.
	const usageCost = 0.30
	guard.Charge("alice", "session-1", decimal.NewFromFloat(usageCost))
	if _, err := wallet.Debit("alice", decimal.NewFromFloat(usageCost), "llm_call"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if !wallet.Balance("alice").Equal(usdt("4.7")) {
		t.Fatalf("alice after llm call: %s, want 4.7", wallet.Balance("alice"))
	}
	// Guard should still allow — we're well under the daily cap.
	if !guard.Check("alice", "session-1").Allowed {
		t.Fatal("guard blocked despite 0.30 << 10.00 daily cap")
	}

	// ---- 3. judge scores the day's work ----
	sc, err := judge.RunFor(aiteamJudge.Signals{
		AgentID: "alice", Period: "2026-05-10",
		UsageCostUSD: usageCost, CallCount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Average == 0 {
		t.Fatal("judge produced 0 average — should be neutral baseline")
	}

	// ---- 4. payroll runs ----
	entry, err := payroll.RunFor("alice", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Skipped {
		t.Fatalf("payroll should NOT skip when net>0: %+v", entry)
	}
	// net should be positive (base + small bonus - small offset)
	if !entry.NetUSDT.IsPositive() {
		t.Fatalf("net should be positive: %s", entry.NetUSDT)
	}
	// Wallet should be credited.
	// Balance now: 4.70 + net
	expected := usdt("4.7").Add(entry.NetUSDT)
	if !wallet.Balance("alice").Equal(expected) {
		t.Fatalf("alice after payroll: %s, want %s", wallet.Balance("alice"), expected)
	}

	// ---- 5. revenue webhook: external task pays $50, 60/40 alice/bob ----
	payload := aiteamRevenue.IncomingPayload{
		TaskID:     "studio-genesis-task-1",
		StudioID:   "studio-foo",
		AmountUSDT: "50.00",
		Split: []aiteamRevenue.SplitEntry{
			{AgentID: "alice", Ratio: "0.6"},
			{AgentID: "bob", Ratio: "0.4"},
		},
		Timestamp: time.Now().Unix(),
		Nonce:     "genesis-nonce-001",
	}
	body, _ := json.Marshal(payload)
	sig := aiteamRevenue.SignFor([]byte("test-genesis-secret-32-bytes-padding"), body)
	res, accErr := revIng.Accept(body, sig)
	if accErr != nil || !res.Accepted {
		t.Fatalf("revenue webhook should accept: err=%v res=%+v", accErr, res)
	}
	if len(res.Shares) != 2 {
		t.Fatalf("expected 2 shares, got %+v", res.Shares)
	}

	// ---- 6. reconcile ----
	// alice received: 5 (genesis) - 0.30 (llm) + payroll net + 30 (60% of 50)
	// bob received: 20 (40% of 50)
	if !wallet.Balance("bob").Equal(usdt("20")) {
		t.Fatalf("bob final balance: %s, want 20", wallet.Balance("bob"))
	}
	wantAlice := usdt("5").Sub(usdt("0.3")).Add(entry.NetUSDT).Add(usdt("30"))
	if !wallet.Balance("alice").Equal(wantAlice) {
		t.Fatalf("alice final balance: %s, want %s", wallet.Balance("alice"), wantAlice)
	}

	// ---- audit log should have a long trail ----
	// genesis credit + debit + payroll credit + revenue.incoming + 2×revenue.split + payroll.run + judge ... = 8+
	if c := audit.LineCount(); c < 8 {
		t.Fatalf("audit trail too short: %d lines", c)
	}

	t.Logf("✅ Genesis demo complete: alice=%s bob=%s (audit=%d rows)",
		wallet.Balance("alice"), wallet.Balance("bob"), audit.LineCount())
}

// Test_AITeam_Genesis_GuardPanicsOnZeroBalance proves the S6 wiring —
// once alice's wallet hits 0, guard.Check denies further work.
func Test_AITeam_Genesis_GuardPanicsOnZeroBalance(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	t.Setenv(flags.EnvWallet, "1")

	root := t.TempDir()
	wallet, _ := aiteamWallet.New(root+"/wallet", nil, nil)
	guard, _ := aiteamBudget.New(root+"/guard", aiteamBudget.Limits{
		PerAgentDailyUSDT: usdt("10.00"),
		Cooldown:          time.Hour,
	}, nil)
	guard.SetWallet(wallet)

	// Genesis $1 then debit all of it.
	_, _ = wallet.Credit("alice", usdt("1.00"), "g")
	_, _ = wallet.Debit("alice", usdt("1.00"), "llm")

	res := guard.Check("alice", "s1")
	if res.Allowed {
		t.Fatalf("zero-balance must panic; got %+v", res)
	}
	if res.PanicReason != "zero_balance" {
		t.Fatalf("wrong reason: %q", res.PanicReason)
	}
}

// Test_AITeam_Genesis_FullFlagsOffByteIdentical proves the zero-impact
// promise. When every flag is unset the entire aiteam subsystem chain
// reports as inactive.
func Test_AITeam_Genesis_FullFlagsOffByteIdentical(t *testing.T) {
	for _, env := range []string{
		flags.EnvWallet, flags.EnvBudgetGuard, flags.EnvJudge,
		flags.EnvPayroll, flags.EnvRevenue, flags.EnvSandbox,
		flags.EnvPromptDef, flags.EnvDashboard,
	} {
		t.Setenv(env, "")
	}
	if flags.AnyEnabled() {
		t.Fatal("expected no flags enabled by default")
	}
	for _, fn := range []func() bool{
		flags.WalletEnabled, flags.BudgetGuardEnabled, flags.JudgeEnabled,
		flags.PayrollEnabled, flags.RevenueEnabled, flags.SandboxEnabled,
		flags.PromptDefEnabled, flags.DashboardEnabled,
	} {
		if fn() {
			t.Fatal("flag unexpectedly enabled")
		}
	}
}

// Sanity: the package-level imports compile and link.
var _ = strings.Join // silence unused-import paranoia
