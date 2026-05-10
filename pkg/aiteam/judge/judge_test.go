package judge

import (
	"strings"
	"testing"
)

func newMgr(t *testing.T) *Manager {
	t.Helper()
	m, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func Test_AITeam_Judge_HeuristicScorerInRange(t *testing.T) {
	sc := HeuristicScorer{}.Score(Signals{AgentID: "alice", Period: "2026-05-10", UsageCostUSD: 0.50})
	if sc.Average < 0 || sc.Average > 10 {
		t.Fatalf("average out of range: %v", sc.Average)
	}
	for _, v := range []int{sc.Completion, sc.Quality, sc.Communication, sc.Creativity, sc.Cost} {
		if v < 0 || v > 10 {
			t.Fatalf("dimension out of [0,10]: %d", v)
		}
	}
}

func Test_AITeam_Judge_CostDimensionSlidesWithUsage(t *testing.T) {
	cases := []struct {
		usage float64
		want  int
	}{
		{0.01, 10}, {0.10, 10},
		{0.30, 8}, {0.50, 8},
		{0.80, 6}, {1.00, 6},
		{2.00, 4}, {2.50, 4},
		{4.00, 2}, {5.00, 2},
		{10.00, 0},
	}
	for _, c := range cases {
		sc := HeuristicScorer{}.Score(Signals{UsageCostUSD: c.usage})
		if sc.Cost != c.want {
			t.Errorf("usage $%.2f → cost score %d, want %d", c.usage, sc.Cost, c.want)
		}
	}
}

func Test_AITeam_Judge_RunForPersistsAndReads(t *testing.T) {
	m := newMgr(t)
	sc, err := m.RunFor(Signals{AgentID: "alice", Period: "2026-05-10", UsageCostUSD: 0.20})
	if err != nil {
		t.Fatal(err)
	}
	if sc.AgentID != "alice" {
		t.Fatalf("agent id: %s", sc.AgentID)
	}
	if sc.Average == 0 {
		t.Fatal("average not computed")
	}

	latest, err := m.Latest("alice", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.AgentID != "alice" {
		t.Fatalf("Latest returned %+v", latest)
	}
}

func Test_AITeam_Judge_OverrideClampsToRange(t *testing.T) {
	m := newMgr(t)
	sc, err := m.Override("alice", "2026-05-10", "human", "good work", 20, -5, 7, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Completion != 10 {
		t.Fatalf("clamped completion = %d, want 10", sc.Completion)
	}
	if sc.Quality != 0 {
		t.Fatalf("clamped quality = %d, want 0", sc.Quality)
	}
	if sc.Source != "manual" || sc.Operator != "human" {
		t.Fatalf("unexpected meta: %+v", sc)
	}
}

func Test_AITeam_Judge_LatestPicksMostRecent(t *testing.T) {
	m := newMgr(t)
	_, _ = m.RunFor(Signals{AgentID: "alice", Period: "2026-05-10", UsageCostUSD: 1.50})
	_, _ = m.Override("alice", "2026-05-10", "owner", "manual override", 10, 10, 10, 10, 10)
	latest, _ := m.Latest("alice", "2026-05-10")
	if latest == nil || latest.Source != "manual" {
		t.Fatalf("expected most recent (manual), got %+v", latest)
	}
	if latest.Average != 10.0 {
		t.Fatalf("average = %v, want 10", latest.Average)
	}
}

func Test_AITeam_Judge_ReadAllRowsOldestFirst(t *testing.T) {
	m := newMgr(t)
	_, _ = m.RunFor(Signals{AgentID: "alice", Period: "2026-05-10", UsageCostUSD: 0.10})
	_, _ = m.Override("alice", "2026-05-10", "human", "override", 5, 5, 5, 5, 5)
	rows, _ := m.Read("alice", "2026-05-10")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Source != "heuristic" || rows[1].Source != "manual" {
		t.Fatalf("rows out of order: %+v", rows)
	}
}

func Test_AITeam_Judge_LatestEmpty(t *testing.T) {
	m := newMgr(t)
	latest, err := m.Latest("bob", "2026-05-10")
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Fatalf("expected nil for unknown agent, got %+v", latest)
	}
}

func Test_AITeam_Judge_AverageOverBlendsHistory(t *testing.T) {
	m := newMgr(t)
	// Today: low cost score (high usage)
	_, _ = m.Override("alice", "", "x", "low avg", 0, 0, 0, 0, 0) // average=0
	// AverageOver(1) → 0
	if v := m.AverageOver("alice", 1); v != 0 {
		t.Fatalf("expected avg=0, got %v", v)
	}
}

func Test_AITeam_Judge_HistoryRespectsLimit(t *testing.T) {
	m := newMgr(t)
	_, _ = m.RunFor(Signals{AgentID: "alice", Period: "", UsageCostUSD: 0.5})
	hist, _ := m.History("alice", 5)
	if len(hist) != 1 {
		t.Fatalf("expected exactly today's score, got %d", len(hist))
	}
}

func Test_AITeam_Judge_AllAgentsLists(t *testing.T) {
	m := newMgr(t)
	_, _ = m.RunFor(Signals{AgentID: "alice", UsageCostUSD: 0.1})
	_, _ = m.RunFor(Signals{AgentID: "bob", UsageCostUSD: 0.1})
	got := m.AllAgents()
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("AllAgents = %v", got)
	}
}

func Test_AITeam_Judge_NilManagerSafe(t *testing.T) {
	var m *Manager
	if _, err := m.RunFor(Signals{AgentID: "x"}); err == nil {
		t.Fatal("nil manager RunFor should error")
	}
	if v := m.AverageOver("x", 7); v != 0 {
		t.Fatalf("nil manager AverageOver should be 0, got %v", v)
	}
	if got := m.AllAgents(); got != nil {
		t.Fatalf("nil manager AllAgents = %v", got)
	}
}

func Test_AITeam_Judge_FormatBreakdownReadable(t *testing.T) {
	sc := Score{AgentID: "alice", Period: "2026-05-10", Completion: 8, Quality: 7,
		Communication: 9, Creativity: 6, Cost: 7, Average: 7.4, Source: "heuristic",
		Rationale: "test"}
	out := sc.FormatBreakdown()
	if !strings.Contains(out, "alice") || !strings.Contains(out, "8/10") ||
		!strings.Contains(out, "7.40") {
		t.Fatalf("breakdown missing key fields:\n%s", out)
	}
}

func Test_AITeam_Judge_EmptyAgentIDRejected(t *testing.T) {
	m := newMgr(t)
	if _, err := m.RunFor(Signals{AgentID: "", Period: "2026-05-10"}); err == nil {
		t.Fatal("empty agent_id should error")
	}
}
