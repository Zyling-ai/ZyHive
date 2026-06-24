package skillopt

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// fakeLLM handles both the critic and evolver prompts deterministically.
// critic → one attribution per "ID:" line; evolver → fixed rules/lessons.
func fakeLLM(rules, lessons string) CallLLM {
	return func(_ context.Context, system, user string) (string, error) {
		if strings.Contains(system, "复盘官") { // critic
			var arr []map[string]any
			for _, ln := range strings.Split(user, "\n") {
				ln = strings.TrimSpace(ln)
				if strings.HasPrefix(ln, "ID: ") {
					id := strings.TrimSpace(strings.TrimPrefix(ln, "ID: "))
					arr = append(arr, map[string]any{"entryId": id, "tags": []string{"忽略主场"}, "lesson": "重视主场优势"})
				}
			}
			b, _ := json.Marshal(arr)
			return string(b), nil
		}
		// evolver
		out, _ := json.Marshal(map[string]string{"rules": rules, "lessons": lessons})
		return string(out), nil
	}
}

// ── ledger / oracle / hit-rate ───────────────────────────────────────────────

func TestLedgerOracleHitRate(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	e1, err := s.Append(LedgerEntry{Prediction: "a"})
	if err != nil {
		t.Fatal(err)
	}
	e2, _ := s.Append(LedgerEntry{Prediction: "b"})
	e3, _ := s.Append(LedgerEntry{Prediction: "c"})

	if e1.Version != 1 || e1.Shadow {
		t.Fatalf("expected baseline v1 non-shadow, got v%d shadow=%v", e1.Version, e1.Shadow)
	}

	pend, _ := s.PendingOracle()
	if len(pend) != 3 {
		t.Fatalf("want 3 pending, got %d", len(pend))
	}

	if err := s.Oracle(e1.ID, "hit", true); err != nil {
		t.Fatal(err)
	}
	_ = s.Oracle(e2.ID, "miss", false)
	_ = s.Oracle(e3.ID, "hit", true)

	rate, n := s.HitRate(false)
	if n != 3 || rate < 0.66 || rate > 0.67 {
		t.Fatalf("want rate ~0.667 over 3, got %.3f over %d", rate, n)
	}
	if pend, _ := s.PendingOracle(); len(pend) != 0 {
		t.Fatalf("want 0 pending after backfill, got %d", len(pend))
	}

	// unknown id → error
	if err := s.Oracle("nope", "x", true); err == nil {
		t.Fatal("expected error for unknown entry id")
	}
}

// ── evolver bounded edit ─────────────────────────────────────────────────────

func TestEvolveBoundedEditPreservesOutside(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	orig := "# My Skill\n\n关键正文，绝不能被改动。\n"
	if err := s.WriteSkillMD(orig); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	attrs := []Attribution{{EntryID: "x", Tags: []string{"t"}, Lesson: "learn X"}}
	p, err := Evolve(context.Background(), s, attrs, fakeLLM("- 永远先评估主场优势", "- 重视主场优势"))
	if err != nil {
		t.Fatalf("evolve: %v", err)
	}
	if p == nil {
		t.Fatal("expected a proposal")
	}
	if !strings.Contains(p.NewContent, "关键正文，绝不能被改动。") {
		t.Fatal("original body must be preserved in evolved content")
	}
	if !strings.Contains(p.NewContent, rulesStart) || !strings.Contains(p.NewContent, lessonsStart) {
		t.Fatal("evolved content must contain controlled regions")
	}
	if got, _ := regionInner(p.NewContent, rulesStart, rulesEnd); got != "- 永远先评估主场优势" {
		t.Fatalf("rules region mismatch: %q", got)
	}
	if p.Status != StatusPending {
		t.Fatalf("want pending, got %s", p.Status)
	}
}

func TestEvolveRejectsOverBudget(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.WriteSkillMD("# S\n")
	_ = s.Init()

	// 20 rule lines >> maxRuleLines
	var big strings.Builder
	for i := 0; i < 20; i++ {
		big.WriteString("- rule\n")
	}
	attrs := []Attribution{{EntryID: "x", Lesson: "l"}}
	_, err := Evolve(context.Background(), s, attrs, fakeLLM(big.String(), "- l"))
	if err == nil {
		t.Fatal("expected over-budget rejection error")
	}
}

func TestEvolveDedupRejectedFingerprint(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.WriteSkillMD("# S\n")
	_ = s.Init()

	rules, lessons := "- only rule", "- only lesson"
	fp := fingerprint(rules, lessons)
	ep, _ := s.ReadEpoch()
	ep.RejectionBuffer = []string{fp}
	_ = s.WriteEpoch(ep)

	attrs := []Attribution{{EntryID: "x", Lesson: "l"}}
	p, err := Evolve(context.Background(), s, attrs, fakeLLM(rules, lessons))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil proposal for a previously-rejected fingerprint")
	}
}

// ── epoch gating ─────────────────────────────────────────────────────────────

func TestMaybeEvolveThresholdGate(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.WriteSkillMD("# S\n")
	_ = s.Init()

	// 2 backfilled misses, threshold default 20 → no evolve without force.
	for _, txt := range []string{"a", "b"} {
		e, _ := s.Append(LedgerEntry{Prediction: txt})
		_ = s.Oracle(e.ID, "wrong", false)
	}
	llm := fakeLLM("- r", "- l")

	p, err := MaybeEvolve(context.Background(), s, llm, false)
	if err != nil || p != nil {
		t.Fatalf("want no evolve under threshold, got p=%v err=%v", p, err)
	}

	// force → evolves.
	p, err = MaybeEvolve(context.Background(), s, llm, true)
	if err != nil {
		t.Fatalf("force evolve err: %v", err)
	}
	if p == nil {
		t.Fatal("force evolve should produce a proposal")
	}

	// attributions were recorded onto the miss entries.
	all, _ := s.AllEntries()
	tagged := 0
	for _, e := range all {
		if len(e.AttributionTags) > 0 {
			tagged++
		}
	}
	if tagged != 2 {
		t.Fatalf("want 2 entries tagged by critic, got %d", tagged)
	}

	// a second evolve is blocked by the pending proposal.
	p2, _ := MaybeEvolve(context.Background(), s, llm, true)
	if p2 != nil {
		t.Fatal("should not create a second proposal while one is pending")
	}
}

// ── shadow canary: promote + rollback ────────────────────────────────────────

func TestShadowPromote(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.WriteSkillMD("# S\n")
	_ = s.Init()

	p, err := Evolve(context.Background(), s, []Attribution{{EntryID: "x", Lesson: "l"}}, fakeLLM("- r", "- l"))
	if err != nil || p == nil {
		t.Fatalf("evolve: p=%v err=%v", p, err)
	}
	if err := AcceptProposal(s, p.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	ep, _ := s.ReadEpoch()
	if ep.ShadowVersion == 0 || ep.ActiveProposal != p.ID {
		t.Fatalf("expected active shadow, got %+v", ep)
	}
	live, _ := s.ReadSkillMD()
	if live != p.NewContent {
		t.Fatal("canary swap: live SKILL.md should equal evolved content")
	}

	// shadow predictions, all hits → should promote.
	for i := 0; i < ep.ShadowMinSample; i++ {
		e, _ := s.Append(LedgerEntry{Prediction: "p"})
		if !e.Shadow {
			t.Fatal("predictions during shadow window must be flagged shadow")
		}
		_ = s.Oracle(e.ID, "right", true)
	}
	verdict, err := EvaluateShadow(s)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.Contains(verdict, "晋升") {
		t.Fatalf("expected promotion verdict, got %q", verdict)
	}
	ep2, _ := s.ReadEpoch()
	if ep2.ShadowVersion != 0 || ep2.CurrentEpoch != 2 || ep2.BaselineVersion != ep.ShadowVersion {
		t.Fatalf("post-promote epoch wrong: %+v", ep2)
	}
}

func TestShadowRollbackOnRegression(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	orig := "# S\n\n原始内容\n"
	_ = s.WriteSkillMD(orig)
	_ = s.Init()

	p, _ := Evolve(context.Background(), s, []Attribution{{EntryID: "x", Lesson: "l"}}, fakeLLM("- r", "- l"))
	if p == nil {
		t.Fatal("need a proposal")
	}
	_ = AcceptProposal(s, p.ID)
	ep, _ := s.ReadEpoch()

	// shadow predictions, all misses → should roll back + reject.
	for i := 0; i < ep.ShadowMinSample; i++ {
		e, _ := s.Append(LedgerEntry{Prediction: "p"})
		_ = s.Oracle(e.ID, "wrong", false)
	}
	verdict, err := EvaluateShadow(s)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.Contains(verdict, "回滚") {
		t.Fatalf("expected rollback verdict, got %q", verdict)
	}
	live, _ := s.ReadSkillMD()
	if live != orig {
		t.Fatalf("rollback should restore original SKILL.md, got %q", live)
	}
	ep2, _ := s.ReadEpoch()
	if ep2.ShadowVersion != 0 {
		t.Fatal("shadow must be cleared after rollback")
	}
	if !ep2.IsRejected(p.Fingerprint) {
		t.Fatal("rolled-back proposal fingerprint must enter the rejection buffer")
	}
	pp, _ := s.ReadProposal(p.ID)
	if pp.Status != StatusRejected {
		t.Fatalf("want rejected proposal, got %s", pp.Status)
	}
}

// ── rejection helpers ────────────────────────────────────────────────────────

func TestRejectionBufferDedup(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.Init()
	p := Proposal{ID: "p1", Fingerprint: "fp", Status: StatusPending}
	_ = s.WriteProposal(p)
	if err := Reject(s, p); err != nil {
		t.Fatal(err)
	}
	_ = Reject(s, p) // idempotent
	ep, _ := s.ReadEpoch()
	count := 0
	for _, fp := range ep.RejectionBuffer {
		if fp == "fp" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("fingerprint should be buffered exactly once, got %d", count)
	}
}

// ── version snapshots ────────────────────────────────────────────────────────

func TestVersionSnapshotImmutable(t *testing.T) {
	s := NewStore(t.TempDir(), "sk")
	_ = s.SnapshotVersion(1, "first")
	_ = s.SnapshotVersion(1, "second") // must not clobber
	got, _ := s.ReadVersion(1)
	if got != "first" {
		t.Fatalf("snapshot must be immutable, got %q", got)
	}
	vs, _ := s.ListVersions()
	if len(vs) != 1 || vs[0] != 1 {
		t.Fatalf("want [1], got %v", vs)
	}
}
