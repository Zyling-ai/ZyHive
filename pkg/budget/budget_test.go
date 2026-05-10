package budget

import (
	"strings"
	"testing"
)

// TestBeforeRun_DisabledNoOp — disabled store always allows, never warns.
func TestBeforeRun_DisabledNoOp(t *testing.T) {
	s := NewStore(Config{Enabled: false})
	res := s.BeforeRun("alice")
	if !res.Allowed {
		t.Fatalf("disabled store must allow, got %+v", res)
	}
	if res.WarnInjection != "" {
		t.Fatalf("disabled store must not warn, got %q", res.WarnInjection)
	}
}

// TestCharge_Accumulates — charges accumulate per agent and globally.
func TestCharge_Accumulates(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 1.0})
	s.Charge("alice", 0.10)
	s.Charge("alice", 0.20)
	s.Charge("bob", 0.05)

	snap := s.SnapshotFor(nil)
	if got := snap.GlobalUsed; got < 0.349 || got > 0.351 {
		t.Fatalf("GlobalUsed = %v, want ~0.35", got)
	}
	var alice, bob *AgentSnapshot
	for i := range snap.Agents {
		switch snap.Agents[i].AgentID {
		case "alice":
			alice = &snap.Agents[i]
		case "bob":
			bob = &snap.Agents[i]
		}
	}
	if alice == nil || alice.Used < 0.299 || alice.Used > 0.301 {
		t.Fatalf("alice.Used = %+v", alice)
	}
	if bob == nil || bob.Used < 0.049 || bob.Used > 0.051 {
		t.Fatalf("bob.Used = %+v", bob)
	}
}

// TestBeforeRun_AgentExhausted — once an agent crosses its cap, BeforeRun
// returns Allowed=false with scope=agent.
func TestBeforeRun_AgentExhausted(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 0.10})
	s.Charge("alice", 0.10) // exactly at limit
	res := s.BeforeRun("alice")
	if res.Allowed {
		t.Fatalf("expected blocked at cap, got %+v", res)
	}
	if res.Scope != "agent" {
		t.Fatalf("Scope = %q, want agent", res.Scope)
	}
}

// TestBeforeRun_GlobalExhausted — global cap blocks even when per-agent limit
// is not yet hit.
func TestBeforeRun_GlobalExhausted(t *testing.T) {
	s := NewStore(Config{
		Enabled:              true,
		GlobalDailyUSD:       0.20,
		DefaultAgentDailyUSD: 1.00,
	})
	s.Charge("alice", 0.15)
	s.Charge("bob", 0.10) // global = 0.25 > 0.20

	res := s.BeforeRun("alice")
	if res.Allowed {
		t.Fatalf("expected blocked by global, got %+v", res)
	}
	if res.Scope != "global" {
		t.Fatalf("Scope = %q, want global", res.Scope)
	}
}

// TestBeforeRun_WarnInjection — past WarnAtPct, Allowed=true but
// WarnInjection populated.
func TestBeforeRun_WarnInjection(t *testing.T) {
	s := NewStore(Config{
		Enabled:              true,
		DefaultAgentDailyUSD: 1.00,
		WarnAtPct:            80,
	})
	s.Charge("alice", 0.85) // 85% of 1.00

	res := s.BeforeRun("alice")
	if !res.Allowed {
		t.Fatalf("warn level should still allow, got %+v", res)
	}
	if res.WarnInjection == "" {
		t.Fatalf("expected warn injection")
	}
	if !strings.Contains(res.WarnInjection, "预算提醒") {
		t.Fatalf("warn injection should contain 预算提醒, got %q", res.WarnInjection)
	}
}

// TestTopup_AllowsOverflow — emergency topup raises the effective cap.
func TestTopup_AllowsOverflow(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 0.10})
	s.Charge("alice", 0.10) // at cap → blocked
	if s.BeforeRun("alice").Allowed {
		t.Fatalf("should be blocked before topup")
	}
	s.Topup("alice", 0.50) // grants extra credit
	if !s.BeforeRun("alice").Allowed {
		t.Fatalf("should be allowed after topup")
	}
}

// TestSetLimit_PerAgentOverridesDefault — per-agent SetLimit takes precedence
// over DefaultAgentDailyUSD.
func TestSetLimit_PerAgentOverridesDefault(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 0.10})
	s.SetLimit("vip", 5.00)
	s.Charge("vip", 0.50)
	res := s.BeforeRun("vip")
	if !res.Allowed {
		t.Fatalf("vip should remain allowed under raised limit, got %+v", res)
	}
	// removing the override falls back to default
	s.SetLimit("vip", 0)
	res = s.BeforeRun("vip")
	if res.Allowed {
		t.Fatalf("vip should now be blocked by default limit, got %+v", res)
	}
}

// TestNoCaps_NeverBlocks — when no global and no per-agent cap is set,
// BeforeRun never blocks even after substantial spend.
func TestNoCaps_NeverBlocks(t *testing.T) {
	s := NewStore(Config{Enabled: true})
	s.Charge("alice", 999.99)
	if !s.BeforeRun("alice").Allowed {
		t.Fatalf("uncapped agent must be allowed")
	}
}

// TestSnapshot_ListsKnownAgents — snapshot enumerates agents that have
// charged or have explicit limits.
func TestSnapshot_ListsKnownAgents(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 1})
	s.Charge("alice", 0.05)
	s.SetLimit("admin", 5)

	snap := s.SnapshotFor(nil)
	ids := map[string]bool{}
	for _, a := range snap.Agents {
		ids[a.AgentID] = true
	}
	if !ids["alice"] || !ids["admin"] {
		t.Fatalf("snapshot missing agents: %+v", ids)
	}
}

// TestRotateClearsTopupOnDayChange — internal helper exercise: faking the
// dayKey simulates rollover; topups for the previous day must be cleared.
func TestRotateClearsTopupOnDayChange(t *testing.T) {
	s := NewStore(Config{Enabled: true, DefaultAgentDailyUSD: 0.10})
	s.Charge("alice", 0.05)
	s.Topup("alice", 1.00)
	// force a day rollover by mutating dayKey directly (test-only access via
	// same package).
	s.mu.Lock()
	s.dayKey = "1999-01-01"
	s.mu.Unlock()

	// Next operation triggers rotateIfNeededLocked which resets agents+topups.
	res := s.BeforeRun("alice")
	if !res.Allowed {
		t.Fatalf("after rollover alice should be at 0/0.10, allowed; got %+v", res)
	}
	snap := s.SnapshotFor([]string{"alice"})
	if got := snap.Agents[0].Used; got != 0 {
		t.Fatalf("after rollover Used=%v want 0", got)
	}
	if got := snap.Agents[0].Topup; got != 0 {
		t.Fatalf("after rollover Topup=%v want 0", got)
	}
}
