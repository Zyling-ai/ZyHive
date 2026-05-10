package flags

import (
	"testing"
)

func Test_AITeam_Flags_DefaultAllOff(t *testing.T) {
	// Sanity: brand-new env (unit-test env is generally clean) has every
	// flag off. We don't `os.Unsetenv` here defensively because Go test
	// processes typically don't have these set.
	if WalletEnabled() {
		t.Fatal("WalletEnabled should be false by default")
	}
	if BudgetGuardEnabled() {
		t.Fatal("BudgetGuardEnabled should be false by default")
	}
	if AnyEnabled() {
		t.Fatal("AnyEnabled should be false by default")
	}
}

func Test_AITeam_Flags_AcceptedTruthyValues(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "YES", "on", "ON", " 1 ", "True"}
	for _, v := range truthy {
		t.Setenv(EnvWallet, v)
		if !WalletEnabled() {
			t.Fatalf("expected WalletEnabled=true for value %q", v)
		}
	}
}

func Test_AITeam_Flags_RejectedFalsyValues(t *testing.T) {
	falsy := []string{"", "0", "false", "no", "off", "two", "enable", "🚀"}
	for _, v := range falsy {
		t.Setenv(EnvWallet, v)
		if WalletEnabled() {
			t.Fatalf("expected WalletEnabled=false for value %q", v)
		}
	}
}

func Test_AITeam_Flags_IndependentSubsystems(t *testing.T) {
	t.Setenv(EnvWallet, "1")
	t.Setenv(EnvBudgetGuard, "0")
	if !WalletEnabled() {
		t.Fatal("wallet should be on")
	}
	if BudgetGuardEnabled() {
		t.Fatal("budget guard should be off")
	}
	if PayrollEnabled() {
		t.Fatal("payroll should be off (not set)")
	}
}

func Test_AITeam_Flags_AnyEnabled(t *testing.T) {
	// no flags set → false
	t.Setenv(EnvWallet, "")
	t.Setenv(EnvPayroll, "")
	t.Setenv(EnvBudgetGuard, "")
	t.Setenv(EnvJudge, "")
	t.Setenv(EnvRevenue, "")
	t.Setenv(EnvSandbox, "")
	t.Setenv(EnvPromptDef, "")
	t.Setenv(EnvDashboard, "")
	if AnyEnabled() {
		t.Fatal("AnyEnabled should be false when nothing is set")
	}
	t.Setenv(EnvSandbox, "true")
	if !AnyEnabled() {
		t.Fatal("AnyEnabled should be true after enabling sandbox")
	}
}

func Test_AITeam_Flags_Snapshot(t *testing.T) {
	t.Setenv(EnvWallet, "1")
	t.Setenv(EnvJudge, "yes")
	snap := Snapshot()
	if !snap[EnvWallet] {
		t.Fatal("wallet should be true in snapshot")
	}
	if !snap[EnvJudge] {
		t.Fatal("judge should be true in snapshot")
	}
	if snap[EnvPayroll] {
		t.Fatal("payroll should be false in snapshot")
	}
	if len(snap) != 8 {
		t.Fatalf("snapshot should have 8 entries, got %d", len(snap))
	}
}
