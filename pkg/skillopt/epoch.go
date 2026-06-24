package skillopt

import (
	"context"
	"time"
)

// Init prepares a skill for evolution (idempotent, non-destructive):
// writes a default epoch.json and snapshots the current SKILL.md as version 1.
// It does NOT modify the live SKILL.md — controlled regions are only added when
// the first evolution is accepted.
func (s *Store) Init() error {
	if s.Exists() {
		return nil
	}
	ep := DefaultEpoch()
	if err := s.WriteEpoch(ep); err != nil {
		return err
	}
	live, err := s.ReadSkillMD()
	if err != nil {
		return err
	}
	return s.SnapshotVersion(ep.BaselineVersion, live)
}

// MaybeEvolve runs the slow-cadence evolution gate for one skill:
//   - skips when a proposal is already pending or a shadow canary is in flight;
//   - requires >= SampleThreshold backfilled samples since the last evolve
//     (unless force);
//   - critiques the misses, then produces a bounded-edit proposal.
//
// Returns the new pending proposal, or nil when nothing was produced.
func MaybeEvolve(ctx context.Context, s *Store, callLLM CallLLM, force bool) (*Proposal, error) {
	ep, err := s.ReadEpoch()
	if err != nil {
		return nil, err
	}

	// One change in flight at a time.
	if ep.ShadowVersion > 0 {
		return nil, nil
	}
	if pend, _ := s.PendingProposal(); pend != nil {
		return nil, nil
	}

	backfilled, err := s.backfilledSince(ep.LastEvolvedAt, false)
	if err != nil {
		return nil, err
	}
	if !force && len(backfilled) < ep.SampleThreshold {
		return nil, nil
	}

	misses, err := s.backfilledSince(ep.LastEvolvedAt, true)
	if err != nil {
		return nil, err
	}
	if len(misses) == 0 {
		// Nothing to learn from — advance the window so we don't re-scan forever.
		ep.LastEvolvedAt = time.Now().UnixMilli()
		_ = s.WriteEpoch(ep)
		return nil, nil
	}

	attrs, err := Critique(ctx, misses, callLLM)
	if err != nil {
		return nil, err
	}
	for _, a := range attrs {
		_ = s.recordAttribution(a)
	}

	prop, evErr := Evolve(ctx, s, attrs, callLLM)

	// Advance the learning window regardless, so a duplicate/over-budget evolve
	// doesn't loop on every maintenance tick.
	ep, _ = s.ReadEpoch()
	ep.LastEvolvedAt = time.Now().UnixMilli()
	_ = s.WriteEpoch(ep)

	if evErr != nil {
		return nil, evErr
	}
	return prop, nil
}
