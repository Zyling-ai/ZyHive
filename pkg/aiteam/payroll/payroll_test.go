package payroll

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
)

func usdt(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

type fakeJudge map[string]float64 // agentID → avg (0-10)

func (f fakeJudge) AverageOver(agentID string, _ int) float64 { return f[agentID] }

type fakeUsage map[string]float64 // agentID → USD spent today

func (f fakeUsage) UsageOn(agentID, _ string) float64 { return f[agentID] }

// fakeWallet collects credits for assertion. Returns ErrTooPoor when
// asked to credit "broke" agent so the failure path is exercised.
type fakeWallet struct {
	credits map[string]decimal.Decimal
}

func newFakeWallet() *fakeWallet { return &fakeWallet{credits: map[string]decimal.Decimal{}} }

func (f *fakeWallet) credit(agentID string, amt decimal.Decimal, _ string) error {
	if agentID == "broke" {
		return errors.New("simulated wallet failure")
	}
	f.credits[agentID] = f.credits[agentID].Add(amt)
	return nil
}

func newMgr(t *testing.T, cfg Config, judge JudgeReader, wallet WalletWriter, usage UsageReader) *Manager {
	t.Helper()
	m, err := New(t.TempDir(), cfg, judge, wallet, usage, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func Test_AITeam_Payroll_BaseOnlyWhenNoJudgeNoUsage(t *testing.T) {
	cfg := DefaultConfig()
	m := newMgr(t, cfg, nil, nil, nil)
	e := m.Compute("alice", "2026-05-10")
	if !e.BaseUSDT.Equal(cfg.DailyBaseUSDT) {
		t.Fatalf("base: %s", e.BaseUSDT)
	}
	if !e.BonusUSDT.IsZero() {
		t.Fatalf("bonus should be zero w/o judge: %s", e.BonusUSDT)
	}
	if !e.OffsetUSDT.IsZero() {
		t.Fatalf("offset should be zero w/o usage: %s", e.OffsetUSDT)
	}
	if !e.NetUSDT.Equal(cfg.DailyBaseUSDT) {
		t.Fatalf("net = %s, want = base = %s", e.NetUSDT, cfg.DailyBaseUSDT)
	}
}

func Test_AITeam_Payroll_BonusScalesWithJudgeAverage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DailyBaseUSDT = usdt("0.10")
	cfg.DailyBonusMaxUSDT = usdt("1.00")
	// avg 8/10 → bonus = 1.00 * 0.8 = 0.80
	m := newMgr(t, cfg, fakeJudge{"alice": 8.0}, nil, nil)
	e := m.Compute("alice", "2026-05-10")
	if e.BonusFactor != 0.8 {
		t.Fatalf("factor: %v", e.BonusFactor)
	}
	if !e.BonusUSDT.Equal(usdt("0.8")) {
		t.Fatalf("bonus: %s", e.BonusUSDT)
	}
	if !e.NetUSDT.Equal(usdt("0.9")) { // 0.10 + 0.80
		t.Fatalf("net: %s, want 0.9", e.NetUSDT)
	}
}

func Test_AITeam_Payroll_CostOffsetReducesNet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DailyBaseUSDT = usdt("1.00")
	cfg.DailyBonusMaxUSDT = usdt("0")
	cfg.CostOffsetRatio = usdt("0.5")
	// usage $0.40 → offset = -0.20 → net = 1.00 - 0.20 = 0.80
	m := newMgr(t, cfg, nil, nil, fakeUsage{"alice": 0.40})
	e := m.Compute("alice", "2026-05-10")
	if !e.OffsetUSDT.Equal(usdt("-0.2")) {
		t.Fatalf("offset: %s", e.OffsetUSDT)
	}
	if !e.NetUSDT.Equal(usdt("0.8")) {
		t.Fatalf("net: %s, want 0.8", e.NetUSDT)
	}
}

func Test_AITeam_Payroll_NetNegativeMarkedSkipped(t *testing.T) {
	cfg := Config{DailyBaseUSDT: usdt("0.10"), DailyBonusMaxUSDT: usdt("0"), CostOffsetRatio: usdt("1.0"), BonusLookbackDays: 7}
	m := newMgr(t, cfg, nil, newFakeWallet().credit, fakeUsage{"alice": 5.0})
	e, err := m.RunFor("alice", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Skipped {
		t.Fatalf("net should be negative → skipped; got %+v", e)
	}
	if e.SkippedNote == "" {
		t.Fatal("skipped_note should explain why")
	}
}

func Test_AITeam_Payroll_RunForCreditsWallet(t *testing.T) {
	cfg := DefaultConfig()
	fw := newFakeWallet()
	m := newMgr(t, cfg, fakeJudge{"alice": 10.0}, fw.credit, nil)
	e, err := m.RunFor("alice", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if e.Skipped {
		t.Fatalf("should not skip: %+v", e)
	}
	if !fw.credits["alice"].Equal(e.NetUSDT) {
		t.Fatalf("wallet credit mismatch: %s vs %s", fw.credits["alice"], e.NetUSDT)
	}
}

func Test_AITeam_Payroll_WalletFailureMarksSkipped(t *testing.T) {
	cfg := DefaultConfig()
	fw := newFakeWallet()
	m := newMgr(t, cfg, nil, fw.credit, nil)
	e, err := m.RunFor("broke", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Skipped {
		t.Fatalf("wallet failure should mark skipped: %+v", e)
	}
	if e.SkippedNote == "" {
		t.Fatal("skipped_note should describe failure")
	}
}

func Test_AITeam_Payroll_PersistsToJSONL(t *testing.T) {
	cfg := DefaultConfig()
	fw := newFakeWallet()
	m := newMgr(t, cfg, nil, fw.credit, nil)
	_, _ = m.RunFor("alice", "2026-05-10")
	_, _ = m.RunFor("bob", "2026-05-10")
	rows, err := m.readPeriod("2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	gotAgents := map[string]bool{}
	for _, r := range rows {
		gotAgents[r.AgentID] = true
	}
	if !gotAgents["alice"] || !gotAgents["bob"] {
		t.Fatalf("missing agents in payslip file: %+v", rows)
	}
}

func Test_AITeam_Payroll_HistoryFiltersAgent(t *testing.T) {
	cfg := DefaultConfig()
	fw := newFakeWallet()
	m := newMgr(t, cfg, nil, fw.credit, nil)
	_, _ = m.RunFor("alice", "") // today
	_, _ = m.RunFor("bob", "")   // today
	hist, _ := m.History("alice", 7)
	if len(hist) != 1 || hist[0].AgentID != "alice" {
		t.Fatalf("history filter broken: %+v", hist)
	}
}

func Test_AITeam_Payroll_RunForAllPaysEveryone(t *testing.T) {
	cfg := DefaultConfig()
	fw := newFakeWallet()
	m := newMgr(t, cfg, fakeJudge{"alice": 5, "bob": 7}, fw.credit, nil)
	entries, err := m.RunForAll([]string{"alice", "bob"}, "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if fw.credits["alice"].IsZero() || fw.credits["bob"].IsZero() {
		t.Fatalf("wallets not credited: %+v", fw.credits)
	}
	// bob should have larger credit (higher judge)
	if fw.credits["bob"].LessThanOrEqual(fw.credits["alice"]) {
		t.Fatalf("higher judge should pay more; alice=%s bob=%s",
			fw.credits["alice"], fw.credits["bob"])
	}
}

func Test_AITeam_Payroll_NilManagerSafe(t *testing.T) {
	var m *Manager
	if _, err := m.RunFor("x", ""); err == nil {
		t.Fatal("nil manager should error on RunFor")
	}
	if rows, _ := m.RunForAll([]string{"x"}, ""); rows != nil {
		t.Fatalf("nil manager RunForAll should be nil, got %+v", rows)
	}
	if rows, _ := m.History("x", 5); rows != nil {
		t.Fatalf("nil manager History should be nil, got %+v", rows)
	}
}

func Test_AITeam_Payroll_EmptyAgentRejected(t *testing.T) {
	m := newMgr(t, DefaultConfig(), nil, newFakeWallet().credit, nil)
	if _, err := m.RunFor("", ""); err == nil {
		t.Fatal("empty agent should error")
	}
}

func Test_AITeam_Payroll_ComputeIsDeterministic(t *testing.T) {
	m := newMgr(t, DefaultConfig(), fakeJudge{"alice": 5.0}, nil, fakeUsage{"alice": 0.30})
	e1 := m.Compute("alice", "2026-05-10")
	e2 := m.Compute("alice", "2026-05-10")
	if !e1.NetUSDT.Equal(e2.NetUSDT) {
		t.Fatalf("Compute non-deterministic: %s vs %s", e1.NetUSDT, e2.NetUSDT)
	}
}

func Test_AITeam_Payroll_DryRunWithoutWallet(t *testing.T) {
	m := newMgr(t, DefaultConfig(), nil, nil, nil) // no wallet
	e, err := m.RunFor("alice", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Skipped {
		t.Fatalf("no wallet → expected skipped, got %+v", e)
	}
	if e.SkippedNote == "" {
		t.Fatal("expected skipped_note explaining no wallet")
	}
}

// Sanity check: float→decimal multiplication accuracy.
func Test_AITeam_Payroll_DecimalAccuracy(t *testing.T) {
	cfg := DefaultConfig()
	// usage 0.123456 USD with offset 0.5 → -0.061728
	m := newMgr(t, cfg, nil, nil, fakeUsage{"alice": 0.123456})
	e := m.Compute("alice", "2026-05-10")
	want := decimal.NewFromFloat(-0.061728)
	if e.OffsetUSDT.Sub(want).Abs().GreaterThan(usdt("0.000001")) {
		t.Fatalf("offset precision drift: got %s want %s",
			e.OffsetUSDT, want)
	}
	_ = fmt.Sprintf // silence unused import paranoia (keeps fmt in scope for test)
}
