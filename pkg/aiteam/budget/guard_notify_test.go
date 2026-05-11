package budget

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// captureHook returns a hook + accessor to read what it captured.
// Uses a mutex since the hook runs on a goroutine.
func captureHook() (hook func(string, string, string), get func() []string) {
	var mu sync.Mutex
	var log []string
	hook = func(agentID, reason, message string) {
		mu.Lock()
		defer mu.Unlock()
		log = append(log, agentID+"|"+reason+"|"+message)
	}
	get = func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(log))
		copy(out, log)
		return out
	}
	return
}

func Test_AITeam_S1_NotifyFiresOnPanic(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	hook, get := captureHook()
	g.SetNotifyHook(hook)

	g.Charge("alice", "s1", usdt("2.00"))
	g.Check("alice", "s1") // triggers panic

	// Hook is async — give the goroutine a moment to fire.
	time.Sleep(50 * time.Millisecond)

	captured := get()
	if len(captured) == 0 {
		t.Fatal("hook should have fired")
	}
	if !strings.HasPrefix(captured[0], "alice|agent_daily|") {
		t.Fatalf("unexpected hook payload: %q", captured[0])
	}
	if !strings.Contains(captured[0], "⚠️") {
		t.Fatalf("hook message should contain warning icon: %q", captured[0])
	}
}

func Test_AITeam_S1_NotifyDoesNotFireOnAllow(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("100.00")})
	hook, get := captureHook()
	g.SetNotifyHook(hook)

	g.Charge("alice", "s1", usdt("0.10"))
	res := g.Check("alice", "s1")
	if !res.Allowed {
		t.Fatal("setup: should allow")
	}
	time.Sleep(50 * time.Millisecond)
	if len(get()) != 0 {
		t.Fatalf("hook should not fire on allow")
	}
}

func Test_AITeam_S1_NotifyNilHookIsNoOp(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	g.SetNotifyHook(nil)
	g.Charge("alice", "s1", usdt("2.00"))
	// must not panic on nil hook
	g.Check("alice", "s1")
}

func Test_AITeam_S1_NotifyPanicInHookDoesNotCrash(t *testing.T) {
	t.Setenv(flags.EnvBudgetGuard, "1")
	g := newGuard(t, Limits{PerAgentDailyUSDT: usdt("1.00")})
	g.SetNotifyHook(func(_, _, _ string) {
		panic("misbehaving hook")
	})

	g.Charge("alice", "s1", usdt("2.00"))
	g.Check("alice", "s1") // hook panics — recovered

	// Verify second Check still works (no broken state).
	time.Sleep(50 * time.Millisecond)
	res := g.Check("alice", "s1")
	if res.Allowed {
		t.Fatal("should still be panicked")
	}
}

func Test_AITeam_S1_NotifyMessageFormat(t *testing.T) {
	msg := formatPanicMessage(
		"alice", "agent_daily",
		usdt("1.50"), usdt("1.00"),
		time.Date(2026, 5, 10, 15, 30, 0, 0, time.UTC),
	)
	expects := []string{"alice", "agent 日累计超限", "1.5", "1", "2026-05-10 15:30"}
	for _, e := range expects {
		if !strings.Contains(msg, e) {
			t.Errorf("message missing %q: %s", e, msg)
		}
	}
}

func Test_AITeam_S1_NotifyZeroBalanceMessage(t *testing.T) {
	msg := formatPanicMessage(
		"bob", "zero_balance",
		usdt("0"), usdt("0"),
		time.Date(2026, 5, 10, 16, 0, 0, 0, time.UTC),
	)
	if !strings.Contains(msg, "钱包余额耗尽") {
		t.Errorf("zero_balance should map to Chinese label: %s", msg)
	}
}
