package skillopt

import (
	"fmt"
	"time"
)

// AcceptProposal turns a pending proposal into an active shadow (canary):
// the baseline is snapshotted for rollback, the live SKILL.md is swapped to the
// evolved content, and new predictions from now on are tagged Shadow until the
// canary is promoted or rolled back.
//
// Note on semantics: because predictions come from a single live agent, true
// parallel "baseline serves / shadow predicts" is not possible — we run a
// canary instead (shadow goes live, measured against the baseline's historical
// hit-rate, auto-rolled-back on regression).
func AcceptProposal(s *Store, proposalID string) error {
	ep, err := s.ReadEpoch()
	if err != nil {
		return err
	}
	if ep.ShadowVersion > 0 {
		return fmt.Errorf("skillopt: a shadow is already active (proposal %s)", ep.ActiveProposal)
	}
	p, err := s.ReadProposal(proposalID)
	if err != nil {
		return err
	}
	if p.Status != StatusPending {
		return fmt.Errorf("skillopt: proposal %s is %s, not pending", proposalID, p.Status)
	}

	// Snapshot the current baseline for rollback (no-op if already snapshotted).
	live, err := s.ReadSkillMD()
	if err != nil {
		return err
	}
	if err := s.SnapshotVersion(ep.BaselineVersion, live); err != nil {
		return err
	}

	shadowV := s.nextVersion(ep)
	if err := s.SnapshotVersion(shadowV, p.NewContent); err != nil {
		return err
	}

	// Canary swap: shadow content goes live.
	if err := s.WriteSkillMD(p.NewContent); err != nil {
		return err
	}
	s.syncLessonsFrom(p.NewContent)

	rate, _ := s.HitRate(false)
	ep.ShadowVersion = shadowV
	ep.ShadowStartTS = time.Now().UnixMilli()
	ep.ActiveProposal = proposalID
	ep.HitRateBaseline = rate
	if err := s.WriteEpoch(ep); err != nil {
		return err
	}

	p.Status = StatusAccepted
	return s.WriteProposal(p)
}

// EvaluateShadow scores the active canary and promotes or rolls it back once
// enough shadow samples have accumulated. Returns a human-readable verdict.
func EvaluateShadow(s *Store) (string, error) {
	ep, err := s.ReadEpoch()
	if err != nil {
		return "", err
	}
	if ep.ShadowVersion == 0 {
		return "无活动影子版本", nil
	}
	rate, n := s.shadowHitRateSince(ep.ShadowStartTS)
	if n < ep.ShadowMinSample {
		return fmt.Sprintf("影子样本积累中（%d/%d）", n, ep.ShadowMinSample), nil
	}
	ep.HitRateShadow = rate
	_ = s.WriteEpoch(ep)

	if rate >= ep.HitRateBaseline+ep.PromoteMargin {
		return Promote(s)
	}
	if err := Rollback(s, ep.BaselineVersion); err != nil {
		return "", err
	}
	return fmt.Sprintf("影子命中率 %.0f%% 未超基线 %.0f%%+%.0f%%，已回滚并拒绝",
		rate*100, ep.HitRateBaseline*100, ep.PromoteMargin*100), nil
}

// Promote confirms the active shadow as the new baseline (epoch++).
func Promote(s *Store) (string, error) {
	ep, err := s.ReadEpoch()
	if err != nil {
		return "", err
	}
	if ep.ShadowVersion == 0 {
		return "", fmt.Errorf("skillopt: no shadow to promote")
	}
	rate, _ := s.shadowHitRateSince(ep.ShadowStartTS)
	pid := ep.ActiveProposal
	newBaseline := ep.ShadowVersion

	ep.BaselineVersion = newBaseline
	ep.CurrentEpoch++
	ep.HitRateBaseline = rate
	ep.HitRateShadow = 0
	ep.ShadowVersion = 0
	ep.ShadowStartTS = 0
	ep.ActiveProposal = ""
	ep.LastEvolvedAt = time.Now().UnixMilli()
	if err := s.WriteEpoch(ep); err != nil {
		return "", err
	}

	if pid != "" {
		if p, err := s.ReadProposal(pid); err == nil {
			p.Status = StatusPromoted
			p.HitRateAfter = rate
			_ = s.WriteProposal(p)
		}
	}
	return fmt.Sprintf("已晋升至 epoch %d（baseline v%d，命中率 %.0f%%）", ep.CurrentEpoch, newBaseline, rate*100), nil
}

// Rollback reverts the live SKILL.md to a version snapshot. If a shadow canary
// is active, its backing proposal is rejected (fingerprint buffered) and the
// shadow state is cleared.
func Rollback(s *Store, version int) error {
	content, err := s.ReadVersion(version)
	if err != nil {
		return fmt.Errorf("skillopt: read version v%d: %w", version, err)
	}
	if err := s.WriteSkillMD(content); err != nil {
		return err
	}
	s.syncLessonsFrom(content)

	ep, err := s.ReadEpoch()
	if err != nil {
		return err
	}
	activePid := ep.ActiveProposal
	ep.BaselineVersion = version
	ep.ShadowVersion = 0
	ep.ShadowStartTS = 0
	ep.ActiveProposal = ""
	ep.HitRateShadow = 0
	if r, _ := s.HitRate(false); r > 0 {
		ep.HitRateBaseline = r
	}
	if err := s.WriteEpoch(ep); err != nil {
		return err
	}

	if activePid != "" {
		if p, err := s.ReadProposal(activePid); err == nil && p.Status != StatusPromoted {
			_ = Reject(s, p)
		}
	}
	return nil
}

// nextVersion returns a fresh version number above both the baseline and any
// existing snapshot.
func (s *Store) nextVersion(ep EpochState) int {
	max := ep.BaselineVersion
	if vs, err := s.ListVersions(); err == nil {
		for _, v := range vs {
			if v > max {
				max = v
			}
		}
	}
	return max + 1
}

// syncLessonsFrom mirrors the lessons region of the given SKILL.md content into
// the per-skill lessons.md (used for the aggregated prompt injection).
func (s *Store) syncLessonsFrom(content string) {
	inner, err := regionInner(content, lessonsStart, lessonsEnd)
	if err != nil {
		return // no lessons region yet
	}
	_ = s.WriteLessons(inner)
}
