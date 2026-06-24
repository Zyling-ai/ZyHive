// Package skillopt turns a static ZyHive skill into a self-evolving one.
//
// Closed loop: predict → ledger → oracle backfill → critic attribution →
// bounded evolve → shadow (canary) A/B → promote / rollback (rejection buffer),
// driven on a slow Epoch cadence.
//
// The package is intentionally dependency-light (stdlib + uuid) so its core
// logic is table-test friendly. Anything that needs an LLM takes a CallLLM
// callback (dependency injection, mirroring pkg/memory.Consolidate).
package skillopt

import "context"

// CallLLM performs a single system+user completion and returns the text.
// Injected by the caller (pkg/agent.Pool.CallLLMOnce in production, a fake in tests).
type CallLLM func(ctx context.Context, system, user string) (string, error)

// ── Tunable defaults ────────────────────────────────────────────────────────
const (
	DefaultSampleThreshold = 20   // backfilled samples needed before an evolve fires
	DefaultPromoteMargin   = 0.05 // shadow must beat baseline hit-rate by this to promote
	DefaultShadowMinSample = 10   // backfilled shadow samples needed before evaluating

	maxRuleLines   = 8  // evolver: max net new lines in the rules region per evolve
	maxLessonLines = 12 // evolver: max net new lines in the lessons region per evolve
)

// ── SKILL.md bounded-edit region markers ────────────────────────────────────
// The evolver may only rewrite content *between* these markers; everything
// outside must remain byte-identical or the proposal is rejected.
const (
	rulesStart   = "<!-- skillopt:rules:start -->"
	rulesEnd     = "<!-- skillopt:rules:end -->"
	lessonsStart = "<!-- skillopt:lessons:start -->"
	lessonsEnd   = "<!-- skillopt:lessons:end -->"
)

// Proposal status values.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
	StatusPromoted = "promoted"
)

// LedgerEntry is one prediction (and, once known, its real-world outcome).
type LedgerEntry struct {
	ID              string   `json:"id"`
	TS              int64    `json:"ts"`                        // prediction time, UnixMilli
	SessionRef      string   `json:"sessionRef,omitempty"`      // originating session
	ContextDigest   string   `json:"contextDigest,omitempty"`   // key context at prediction time
	Prediction      string   `json:"prediction"`                // what was predicted
	Oracle          string   `json:"oracle,omitempty"`          // backfilled real outcome
	Hit             *bool    `json:"hit,omitempty"`             // nil = not backfilled yet
	OracleTS        int64    `json:"oracleTs,omitempty"`        // backfill time, UnixMilli
	AttributionTags []string `json:"attributionTags,omitempty"` // critic tags (miss only)
	Lesson          string   `json:"lesson,omitempty"`          // critic lesson (miss only)
	Version         int      `json:"version"`                   // epoch version live at prediction time
	Shadow          bool     `json:"shadow,omitempty"`          // recorded during a shadow/canary window
}

// EpochState is the per-skill evolution state machine (epoch.json).
type EpochState struct {
	CurrentEpoch    int      `json:"currentEpoch"`
	BaselineVersion int      `json:"baselineVersion"`
	ShadowVersion   int      `json:"shadowVersion,omitempty"` // 0 = no shadow active
	SampleThreshold int      `json:"sampleThreshold"`
	PromoteMargin   float64  `json:"promoteMargin"`
	ShadowMinSample int      `json:"shadowMinSample"`
	HitRateBaseline float64  `json:"hitRateBaseline"`
	HitRateShadow   float64  `json:"hitRateShadow"`
	LastEvolvedAt   int64    `json:"lastEvolvedAt,omitempty"`
	ShadowStartTS   int64    `json:"shadowStartTs,omitempty"`
	ActiveProposal  string   `json:"activeProposal,omitempty"`  // proposal id backing the current shadow
	RejectionBuffer []string `json:"rejectionBuffer,omitempty"` // rejected proposal fingerprints (dedupe)
	AutoAccept      bool     `json:"autoAccept,omitempty"`      // auto-accept proposals into a canary (full autonomy)
	CronJobID       string   `json:"cronJobId,omitempty"`       // cron job driving scheduled maintenance
}

// Proposal is one bounded-edit evolution candidate (proposals/{id}.json).
type Proposal struct {
	ID            string   `json:"id"`
	CreatedAt     int64    `json:"createdAt"`
	Status        string   `json:"status"`
	FromVersion   int      `json:"fromVersion"`
	Rationale     string   `json:"rationale"`
	Lessons       []string `json:"lessons"`
	DiffSummary   string   `json:"diffSummary"`
	NewContent    string   `json:"newContent"` // full evolved SKILL.md
	Fingerprint   string   `json:"fingerprint"`
	HitRateBefore float64  `json:"hitRateBefore"`
	HitRateAfter  float64  `json:"hitRateAfter,omitempty"`
}

// Attribution is the critic's verdict for one missed prediction.
type Attribution struct {
	EntryID string   `json:"entryId"`
	Tags    []string `json:"tags"`
	Lesson  string   `json:"lesson"`
}

// DefaultEpoch returns a fresh epoch state with sane thresholds.
func DefaultEpoch() EpochState {
	return EpochState{
		CurrentEpoch:    1,
		BaselineVersion: 1,
		SampleThreshold: DefaultSampleThreshold,
		PromoteMargin:   DefaultPromoteMargin,
		ShadowMinSample: DefaultShadowMinSample,
	}
}
