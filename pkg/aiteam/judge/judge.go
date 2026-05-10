// Package judge implements the aiteam Judge subsystem (PR-004).
//
// Goal: produce a multi-dimensional 0-10 score for an agent's work
// over a period, so PR-002 Payroll can pay bonuses proportional to
// quality. v0 uses a *heuristic* scorer keyed off easily-available
// signals (usage cost, session activity, manual override). A future
// PR can replace the scorer with an LLM-driven evaluator without
// changing the Score / Manager API.
//
// Storage: <dataDir>/aiteam/judge/<agentID>/<period>.jsonl
//   one line per Score (so manual re-evaluation rows survive cleanly).
//
// Activation: ZYHIVE_EXPERIMENTAL_JUDGE=1 (zero impact when off).
//
// Threat model: when invoked over arbitrary user-supplied transcripts,
// the input passes through pkg/aiteam/promptdef before reaching any
// future LLM scorer. v0 only consumes numeric signals so injection is
// not a concern here.
//
// All ops nil-safe; Manager.Score returns the zero value for unknown
// agents.
package judge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Score holds the multi-dimensional evaluation. All dimensions are
// integers in [0, 10]; total = average. Rationale is a short human-
// readable line for the dashboard and the audit log.
//
// We pick five dimensions matching the PLAN § 3.5 spec:
//   completion    — did the agent finish what was asked
//   quality       — quality of output (code/text correctness)
//   communication — clarity / brevity of interactions
//   creativity    — novelty / cleverness of approach
//   cost          — token efficiency (low cost ↑)
type Score struct {
	AgentID       string    `json:"agent_id"`
	Period        string    `json:"period"`        // YYYY-MM-DD
	Completion    int       `json:"completion"`
	Quality       int       `json:"quality"`
	Communication int       `json:"communication"`
	Creativity    int       `json:"creativity"`
	Cost          int       `json:"cost"`
	Average       float64   `json:"average"` // computed
	Rationale     string    `json:"rationale,omitempty"`
	Source        string    `json:"source"` // "heuristic" | "manual" | "llm" (future)
	Operator      string    `json:"operator,omitempty"` // for manual overrides
	Timestamp     int64     `json:"ts"`     // unix milli
}

// computeAverage fills .Average from the dimension fields.
func (s *Score) computeAverage() {
	sum := s.Completion + s.Quality + s.Communication + s.Creativity + s.Cost
	s.Average = float64(sum) / 5.0
}

// Signals is the input bundle a Scorer consumes. The heuristic v0
// scorer fills it from pkg/usage; future LLM versions can use the same
// struct + transcript bytes.
type Signals struct {
	AgentID     string
	Period      string
	UsageCostUSD float64 // total USD cost over the period
	CallCount    int     // number of LLM calls
	ErrorCount   int     // recorded errors (currently always 0; reserved)
	Notes        string  // free-form (e.g. owner thumbs-down)
}

// Scorer is the pluggable evaluator interface. Implementations should
// be deterministic given identical Signals so re-runs do not flap.
type Scorer interface {
	Score(s Signals) Score
}

// HeuristicScorer is the v0 default. Mapping:
//
//   completion    — fixed 7 (we don't know; future LLM judge will read transcripts)
//   quality       — fixed 7 (same — placeholder, override via manual)
//   communication — fixed 7
//   creativity    — fixed 6
//   cost          — inverse of cost: 10 if ≤ $0.10, 5 if $1.00, 0 if ≥ $5.00
//                   linear interpolation between buckets
//
// The result is a *neutral baseline* score (~6-7 average) that
// reflects "the agent did SOMETHING worth tracking but we have no
// quality signal yet". Manual override or future LLM scoring will
// produce the meaningful variance.
type HeuristicScorer struct{}

func (HeuristicScorer) Score(s Signals) Score {
	cost := 10
	switch {
	case s.UsageCostUSD <= 0.10:
		cost = 10
	case s.UsageCostUSD <= 0.50:
		cost = 8
	case s.UsageCostUSD <= 1.00:
		cost = 6
	case s.UsageCostUSD <= 2.50:
		cost = 4
	case s.UsageCostUSD <= 5.00:
		cost = 2
	default:
		cost = 0
	}
	out := Score{
		AgentID:       s.AgentID,
		Period:        s.Period,
		Completion:    7,
		Quality:       7,
		Communication: 7,
		Creativity:    6,
		Cost:          cost,
		Source:        "heuristic",
		Rationale: fmt.Sprintf(
			"v0 heuristic: usage $%.4f / %d calls → cost score %d; other dims neutral baseline 7/6",
			s.UsageCostUSD, s.CallCount, cost,
		),
		Timestamp: time.Now().UnixMilli(),
	}
	out.computeAverage()
	return out
}

// Manager persists scores under dir/<agent>/<period>.jsonl and serves
// reads. Safe for concurrent use.
type Manager struct {
	dir    string
	scorer Scorer
	mu     sync.Mutex
}

// New constructs a Manager rooted at dir. dir is created (0o700) if
// missing.
func New(dir string, scorer Scorer) (*Manager, error) {
	if dir == "" {
		return nil, fmt.Errorf("judge: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if scorer == nil {
		scorer = HeuristicScorer{}
	}
	return &Manager{dir: dir, scorer: scorer}, nil
}

// RunFor evaluates the agent for the given period from the provided
// signals, persists the result, and returns it.
func (m *Manager) RunFor(s Signals) (*Score, error) {
	if m == nil {
		return nil, errors.New("judge: nil manager")
	}
	if s.AgentID == "" {
		return nil, errors.New("judge: empty agent_id")
	}
	if s.Period == "" {
		s.Period = time.Now().UTC().Format("2006-01-02")
	}
	sc := m.scorer.Score(s)
	if err := m.append(&sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

// Override records a manual evaluation (operator-supplied dimensions).
// Returns the persisted Score. operator and rationale are stored for
// audit.
func (m *Manager) Override(agentID, period, operator, rationale string,
	completion, quality, communication, creativity, cost int) (*Score, error) {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 10 {
			return 10
		}
		return v
	}
	if period == "" {
		period = time.Now().UTC().Format("2006-01-02")
	}
	sc := Score{
		AgentID:       agentID,
		Period:        period,
		Completion:    clamp(completion),
		Quality:       clamp(quality),
		Communication: clamp(communication),
		Creativity:    clamp(creativity),
		Cost:          clamp(cost),
		Source:        "manual",
		Operator:      operator,
		Rationale:     rationale,
		Timestamp:     time.Now().UnixMilli(),
	}
	sc.computeAverage()
	if err := m.append(&sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

// append writes one row to the agent's jsonl file under m.dir.
func (m *Manager) append(sc *Score) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sc.Average == 0 {
		sc.computeAverage()
	}

	agentDir := filepath.Join(m.dir, sc.AgentID)
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(agentDir, sc.Period+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	data, err := json.Marshal(sc)
	if err != nil {
		f.Close()
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// Latest returns the most recent Score for agentID in period, or nil
// if none. Period may be "" → today's period.
func (m *Manager) Latest(agentID, period string) (*Score, error) {
	if m == nil {
		return nil, errors.New("judge: nil manager")
	}
	if period == "" {
		period = time.Now().UTC().Format("2006-01-02")
	}
	scores, err := m.Read(agentID, period)
	if err != nil {
		return nil, err
	}
	if len(scores) == 0 {
		return nil, nil
	}
	return &scores[len(scores)-1], nil
}

// Read returns every score row for agentID in period, oldest first.
// Empty slice when no file exists.
func (m *Manager) Read(agentID, period string) ([]Score, error) {
	if m == nil {
		return nil, nil
	}
	if period == "" {
		period = time.Now().UTC().Format("2006-01-02")
	}
	path := filepath.Join(m.dir, agentID, period+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Score{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Score
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<14), 1<<20)
	for scanner.Scan() {
		var sc Score
		if err := json.Unmarshal(scanner.Bytes(), &sc); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, scanner.Err()
}

// History returns the last `n` daily Latest() scores across the last
// `n` calendar days (UTC), newest first. Missing days are skipped.
// Useful for the dashboard "trend" view (PR-006 S10).
func (m *Manager) History(agentID string, n int) ([]Score, error) {
	if m == nil || n <= 0 {
		return nil, nil
	}
	var out []Score
	today := time.Now().UTC()
	for i := 0; i < n; i++ {
		d := today.AddDate(0, 0, -i)
		latest, err := m.Latest(agentID, d.Format("2006-01-02"))
		if err != nil {
			return nil, err
		}
		if latest != nil {
			out = append(out, *latest)
		}
	}
	return out, nil
}

// AverageOver returns the mean of Score.Average across the last n
// daily snapshots. Returns 0 if no data. Used by PR-002 Payroll bonus.
func (m *Manager) AverageOver(agentID string, n int) float64 {
	hist, _ := m.History(agentID, n)
	if len(hist) == 0 {
		return 0
	}
	var sum float64
	for _, s := range hist {
		sum += s.Average
	}
	return sum / float64(len(hist))
}

// AllAgents lists every agentID with a score directory under m.dir.
// Sorted alphabetically.
func (m *Manager) AllAgents() []string {
	if m == nil {
		return nil
	}
	entries, _ := os.ReadDir(m.dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// String returns a compact one-line representation for logs.
func (s Score) String() string {
	return fmt.Sprintf("Score{agent=%s period=%s avg=%.1f src=%s}",
		s.AgentID, s.Period, s.Average, s.Source)
}

// FormatBreakdown returns a multi-line breakdown for human display.
func (s Score) FormatBreakdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "agent      %s\n", s.AgentID)
	fmt.Fprintf(&b, "period     %s\n", s.Period)
	fmt.Fprintf(&b, "average    %.2f (%s)\n", s.Average, s.Source)
	fmt.Fprintf(&b, "completion %d/10\n", s.Completion)
	fmt.Fprintf(&b, "quality    %d/10\n", s.Quality)
	fmt.Fprintf(&b, "communic.  %d/10\n", s.Communication)
	fmt.Fprintf(&b, "creativity %d/10\n", s.Creativity)
	fmt.Fprintf(&b, "cost       %d/10\n", s.Cost)
	if s.Rationale != "" {
		fmt.Fprintf(&b, "rationale  %s\n", s.Rationale)
	}
	return b.String()
}
