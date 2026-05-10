// Package payroll implements PR-002 (S8): daily payroll calculation
// that pays each agent base salary + bonus (scaled by Judge score) -
// cost offset (a fraction of LLM usage), then credits the net to the
// agent's wallet.
//
// The math is deliberately simple in v0:
//
//   base    = config.DailyBaseUSDT             (e.g. 0.10 USDT)
//   bonus   = config.DailyBonusMaxUSDT *
//             (judge.AverageOver(agent, BonusLookbackDays) / 10)
//   offset  = -1 * usage_today_usdt * config.CostOffsetRatio
//   net     = base + bonus + offset
//
// If `net <= 0` no transfer happens (avoids debt-spiral semantics —
// see PLAN § 3.6 §1 + B015 audit notes). The skipped payroll is still
// persisted to <dataDir>/aiteam/payroll/<period>.jsonl with
// "skipped":true so operators can see why.
//
// Wallet integration: payroll.Manager.RunOnce ultimately calls
// wallet.Credit(agentID, net, "payroll YYYY-MM-DD"). We rely on the
// wallet's audit log writing the row; payroll also writes its own
// daily summary jsonl independent of wallet.
//
// Cron integration: pkg/cron.Engine doesn't have a Go-callable
// scheduler today (jobs run agent sessions). So we instead start a
// dedicated goroutine in cmd/aipanel/main.go that wakes up at the
// configured time once per day. The Manager is deliberately
// stateless re. scheduling; it just exposes RunOnce(period) so a
// trigger can pull it.
//
// Activation: ZYHIVE_EXPERIMENTAL_PAYROLL=1.
package payroll

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
)

// Config controls payout policy. All amounts are USDT decimals.
//
// Defaults are documented in DefaultConfig().
type Config struct {
	// DailyBaseUSDT is paid every day to every agent regardless of work.
	DailyBaseUSDT decimal.Decimal `json:"daily_base_usdt"`

	// DailyBonusMaxUSDT is the cap of the judge-scaled bonus. The actual
	// bonus is BonusMax * (avgScore / 10).
	DailyBonusMaxUSDT decimal.Decimal `json:"daily_bonus_max_usdt"`

	// BonusLookbackDays is how many recent days of Judge scores to
	// average over. 7 is the v0 default — smooths out one-off bad days.
	BonusLookbackDays int `json:"bonus_lookback_days"`

	// CostOffsetRatio is the fraction of today's USD usage that is
	// deducted from net pay (0.0 → no offset, 1.0 → 100% offset).
	// Default 0.5 → agent eats half of its own LLM cost.
	CostOffsetRatio decimal.Decimal `json:"cost_offset_ratio"`
}

// DefaultConfig returns sensible v0 defaults:
//   base = 0.10 USDT
//   bonus max = 0.50 USDT
//   lookback = 7 days
//   cost offset = 50%
func DefaultConfig() Config {
	return Config{
		DailyBaseUSDT:     decimal.NewFromFloat(0.10),
		DailyBonusMaxUSDT: decimal.NewFromFloat(0.50),
		BonusLookbackDays: 7,
		CostOffsetRatio:   decimal.NewFromFloat(0.50),
	}
}

// JudgeReader is the minimal Judge API payroll needs. Defined here to
// avoid an import cycle and keep payroll testable in isolation.
type JudgeReader interface {
	AverageOver(agentID string, n int) float64
}

// WalletWriter is the minimal Wallet API for payroll. We model it as a
// thin function adapter (not an interface) so the concrete
// *wallet.Store doesn't need to match a specific signature with its
// own *Entry return — keeps payroll free of a wallet import.
type WalletWriter func(agentID string, amount decimal.Decimal, reason string) error

// UsageReader returns the agent's USD cost spent on `period` (YYYY-MM-DD,
// UTC). Returns 0 when unknown. Avoids importing pkg/usage so payroll
// is fully isolated.
type UsageReader interface {
	UsageOn(agentID, period string) float64
}

// PayslipEntry is the persisted row format.
type PayslipEntry struct {
	Period       string          `json:"period"` // YYYY-MM-DD
	AgentID      string          `json:"agent_id"`
	BaseUSDT     decimal.Decimal `json:"base_usdt"`
	BonusUSDT    decimal.Decimal `json:"bonus_usdt"`
	BonusFactor  float64         `json:"bonus_factor"` // judge avg/10
	OffsetUSDT   decimal.Decimal `json:"offset_usdt"`
	NetUSDT      decimal.Decimal `json:"net_usdt"`
	Skipped      bool            `json:"skipped"`
	SkippedNote  string          `json:"skipped_note,omitempty"`
	Timestamp    int64           `json:"ts"`
}

// Manager is the payroll engine. Construct one per process via New.
type Manager struct {
	dir       string
	cfg       Config
	judge     JudgeReader
	walletFn  WalletWriter
	usage     UsageReader
	audit     *audit.Log

	mu sync.Mutex
}

// New constructs a payroll Manager. Any of judge / wallet / usage may
// be nil; missing pieces degrade gracefully (no bonus, no credit, no
// offset respectively).
func New(dir string, cfg Config, judge JudgeReader, walletCredit WalletWriter, usage UsageReader, log *audit.Log) (*Manager, error) {
	if dir == "" {
		return nil, fmt.Errorf("payroll: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if cfg.BonusLookbackDays <= 0 {
		cfg.BonusLookbackDays = 7
	}
	if cfg.CostOffsetRatio.IsZero() {
		cfg.CostOffsetRatio = decimal.NewFromFloat(0.50)
	}
	return &Manager{
		dir: dir, cfg: cfg, judge: judge, walletFn: walletCredit, usage: usage, audit: log,
	}, nil
}

// Compute is the pure math: produce a PayslipEntry from inputs without
// touching wallet or disk. Exposed for tests + dashboard previews.
func (m *Manager) Compute(agentID, period string) PayslipEntry {
	if period == "" {
		period = time.Now().UTC().Format("2006-01-02")
	}
	bonusFactor := 0.0
	if m.judge != nil {
		bonusFactor = m.judge.AverageOver(agentID, m.cfg.BonusLookbackDays) / 10.0
	}
	bonusFactorDec := decimal.NewFromFloat(bonusFactor)
	bonus := m.cfg.DailyBonusMaxUSDT.Mul(bonusFactorDec)

	var offset decimal.Decimal
	if m.usage != nil {
		usd := m.usage.UsageOn(agentID, period)
		offset = decimal.NewFromFloat(usd).Mul(m.cfg.CostOffsetRatio).Neg()
	}

	net := m.cfg.DailyBaseUSDT.Add(bonus).Add(offset)

	return PayslipEntry{
		Period:      period,
		AgentID:     agentID,
		BaseUSDT:    m.cfg.DailyBaseUSDT,
		BonusUSDT:   bonus,
		BonusFactor: bonusFactor,
		OffsetUSDT:  offset,
		NetUSDT:     net,
		Timestamp:   time.Now().UnixMilli(),
	}
}

// RunFor computes + credits a single agent. Returns the persisted entry.
func (m *Manager) RunFor(agentID, period string) (*PayslipEntry, error) {
	if m == nil {
		return nil, fmt.Errorf("payroll: nil manager")
	}
	if agentID == "" {
		return nil, fmt.Errorf("payroll: empty agent_id")
	}
	entry := m.Compute(agentID, period)

	// Skip when net <= 0 (no debt). Still persist so we can see why.
	if entry.NetUSDT.LessThanOrEqual(decimal.Zero) {
		entry.Skipped = true
		entry.SkippedNote = "net <= 0 (cost offset exceeds base+bonus)"
	} else if m.walletFn != nil {
		if err := m.walletFn(agentID, entry.NetUSDT,
			fmt.Sprintf("payroll %s", entry.Period)); err != nil {
			entry.Skipped = true
			entry.SkippedNote = "wallet credit failed: " + err.Error()
		}
	} else {
		entry.Skipped = true
		entry.SkippedNote = "no wallet configured (dry run)"
	}

	if err := m.persist(&entry); err != nil {
		return nil, err
	}

	if m.audit != nil {
		_ = m.audit.Append(audit.Entry{
			Type:      "payroll.run",
			Subsystem: "payroll",
			AgentID:   agentID,
			Detail: map[string]any{
				"period":     entry.Period,
				"net_usdt":   entry.NetUSDT.String(),
				"base":       entry.BaseUSDT.String(),
				"bonus":      entry.BonusUSDT.String(),
				"offset":     entry.OffsetUSDT.String(),
				"bonus_factor": entry.BonusFactor,
				"skipped":    entry.Skipped,
			},
		})
	}

	return &entry, nil
}

// RunForAll iterates over a snapshot of agent IDs (typically from
// pool.manager.List()) and pays each. Returns slice of entries (one
// per agent, in input order).
func (m *Manager) RunForAll(agentIDs []string, period string) ([]PayslipEntry, error) {
	if m == nil {
		return nil, nil
	}
	if period == "" {
		period = time.Now().UTC().Format("2006-01-02")
	}
	out := make([]PayslipEntry, 0, len(agentIDs))
	for _, id := range agentIDs {
		e, err := m.RunFor(id, period)
		if err != nil {
			return out, err
		}
		out = append(out, *e)
	}
	return out, nil
}

// persist appends an entry to <dir>/<period>.jsonl.
func (m *Manager) persist(e *PayslipEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := filepath.Join(m.dir, e.Period+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		_ = f.Close()
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// History reads up to `days` previous payslip files for agentID,
// newest-first.
func (m *Manager) History(agentID string, days int) ([]PayslipEntry, error) {
	if m == nil || days <= 0 {
		return nil, nil
	}
	var out []PayslipEntry
	today := time.Now().UTC()
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -i)
		period := d.Format("2006-01-02")
		rows, err := m.readPeriod(period)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.AgentID == agentID {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// readPeriod returns every row in a given day's payslip file.
func (m *Manager) readPeriod(period string) ([]PayslipEntry, error) {
	path := filepath.Join(m.dir, period+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []PayslipEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<14), 1<<20)
	for scanner.Scan() {
		var e PayslipEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, scanner.Err()
}
