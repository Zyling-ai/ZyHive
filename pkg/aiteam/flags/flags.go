// Package flags centralises every aiteam experimental feature flag.
//
// All aiteam (autonomous-economy) subsystems are gated behind environment
// variables that default to OFF. When a flag is off, the corresponding
// REST routes return 404, tools are not registered, and code paths short-
// circuit so behaviour is byte-identical to a build without aiteam.
//
// Recognised env values for ON: "1", "true", "TRUE", "yes", "on".
// Anything else (including unset) → OFF.
//
// Naming convention: ZYHIVE_EXPERIMENTAL_<SUBSYSTEM>=1.
package flags

import (
	"os"
	"strings"
)

// Env var names. Kept as constants so callers / tests / docs can reference
// them without typo risk.
const (
	EnvWallet      = "ZYHIVE_EXPERIMENTAL_WALLET"
	EnvPayroll    = "ZYHIVE_EXPERIMENTAL_PAYROLL"
	EnvBudgetGuard = "ZYHIVE_EXPERIMENTAL_BUDGETGUARD"
	EnvJudge      = "ZYHIVE_EXPERIMENTAL_JUDGE"
	EnvRevenue    = "ZYHIVE_EXPERIMENTAL_REVENUE"
	EnvSandbox    = "ZYHIVE_EXPERIMENTAL_SANDBOX"
	EnvPromptDef  = "ZYHIVE_EXPERIMENTAL_PROMPTDEF"
	EnvDashboard  = "ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD"
)

// boolEnv treats "1", "true", "yes", "on" (case-insensitive) as ON.
// Empty string or anything else → OFF.
func boolEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// WalletEnabled reports whether the wallet subsystem (PR-001) is active.
func WalletEnabled() bool { return boolEnv(EnvWallet) }

// PayrollEnabled reports whether payroll (PR-002) is active.
func PayrollEnabled() bool { return boolEnv(EnvPayroll) }

// BudgetGuardEnabled reports whether the hard-stop budget guard (PR-003) is active.
// Note: this is independent of pkg/budget (P1-02) which is the soft-warn brake.
func BudgetGuardEnabled() bool { return boolEnv(EnvBudgetGuard) }

// JudgeEnabled reports whether the judge-agent subsystem (PR-004) is active.
func JudgeEnabled() bool { return boolEnv(EnvJudge) }

// RevenueEnabled reports whether the revenue webhook ingest (PR-005) is active.
func RevenueEnabled() bool { return boolEnv(EnvRevenue) }

// SandboxEnabled reports whether tool execution sandboxing (PR-007) is active.
func SandboxEnabled() bool { return boolEnv(EnvSandbox) }

// PromptDefEnabled reports whether prompt-injection defence wrapping (PR-008) is active.
func PromptDefEnabled() bool { return boolEnv(EnvPromptDef) }

// DashboardEnabled reports whether the aiteam observability UI (PR-006) is active.
// (Backend routes for individual subsystems are gated by their own flags above; this
// only controls whether the dashboard menu/page is exposed in the frontend.)
func DashboardEnabled() bool { return boolEnv(EnvDashboard) }

// AnyEnabled reports whether at least one aiteam subsystem is enabled.
// Useful at startup to decide whether to log an "aiteam: experimental mode on" banner.
func AnyEnabled() bool {
	return WalletEnabled() || PayrollEnabled() || BudgetGuardEnabled() ||
		JudgeEnabled() || RevenueEnabled() || SandboxEnabled() ||
		PromptDefEnabled() || DashboardEnabled()
}

// Snapshot returns a map of flag name → current state for diagnostics.
// Order is stable so it can be logged deterministically.
func Snapshot() map[string]bool {
	return map[string]bool{
		EnvWallet:      WalletEnabled(),
		EnvPayroll:     PayrollEnabled(),
		EnvBudgetGuard: BudgetGuardEnabled(),
		EnvJudge:       JudgeEnabled(),
		EnvRevenue:     RevenueEnabled(),
		EnvSandbox:     SandboxEnabled(),
		EnvPromptDef:   PromptDefEnabled(),
		EnvDashboard:   DashboardEnabled(),
	}
}
