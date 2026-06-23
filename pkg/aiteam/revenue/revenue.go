// Package revenue implements aiteam PR-005: inbound revenue webhook
// from ZyStudio (or any compatible task-market). When a task is
// settled by the upstream market, it POSTs a signed payload here; we
// verify the HMAC signature, fan the amount out across the agents
// listed in `split`, and credit each share to their wallet ledger.
//
// Threat model:
//   * Endpoint lives inside the /api bearer-auth group, so the market
//     must supply BOTH the bearer token AND an HMAC-SHA256 over the raw
//     body using a shared secret (defence in depth). Replay defence is
//     timestamp + nonce, with a configurable window (default 5 minutes).
//   * Idempotency: nonce is cached in-memory (last 10k) so duplicate
//     deliveries are no-ops rather than double-credits.
//
// Audit:
//   * Every accepted webhook writes one audit row of type
//     "revenue.incoming" plus one "revenue.split" per share.
//
// Activation: ZYHIVE_EXPERIMENTAL_REVENUE=1.
//
// The webhook endpoint is registered in internal/api/aiteam_routes.go
// at POST /api/aiteam/revenue/incoming, inside the /api bearer-auth
// group; the HMAC is a secondary defence-in-depth layer on top of the
// bearer token.
package revenue

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
)

// IncomingPayload is the canonical JSON body the market POSTs to us.
// See docs/aiteam-revenue-protocol.md for the protocol spec.
type IncomingPayload struct {
	TaskID         string             `json:"task_id"`
	StudioID       string             `json:"studio_id,omitempty"`
	AmountUSDT     string             `json:"amount_usdt"`        // decimal string
	FxAtSettlement map[string]float64 `json:"fx_at_settlement,omitempty"`
	Split          []SplitEntry       `json:"split"`
	// Timestamp is unix seconds when the market signed the payload.
	// Combined with Nonce it defeats replays beyond the freshness window.
	Timestamp int64 `json:"ts"`
	Nonce     string `json:"nonce"`
}

// SplitEntry is one slice of the payout.
type SplitEntry struct {
	AgentID string `json:"agent_id"`
	// Ratio is a decimal string so the protocol stays human-readable.
	// Sum across split should be 1.0 ± 0.0001 (we verify).
	Ratio string `json:"ratio"`
}

// WalletCredit is the minimal wallet API revenue needs. Same isolation
// pattern as payroll — keeps revenue free of a wallet import.
type WalletCredit func(agentID string, amount decimal.Decimal, reason string) error

// Config bundles the secret + freshness window. Pass via NewIngester.
type Config struct {
	Secret           []byte        // shared with the market; non-empty required
	FreshnessWindow  time.Duration // reject payloads older / newer than this; default 5m
	NonceCacheSize   int           // default 10000
}

// Ingester verifies + dispatches incoming payloads. Safe for concurrent use.
type Ingester struct {
	cfg      Config
	dir      string
	wallet   WalletCredit
	audit    *audit.Log

	mu          sync.Mutex
	seenNonces  map[string]time.Time
	nonceOrder  []string // FIFO for cache eviction
}

// New constructs an Ingester. dir is created (0o700) if missing.
// Secret must be non-empty; wallet may be nil (dry-run audits only).
func New(dir string, cfg Config, wallet WalletCredit, log *audit.Log) (*Ingester, error) {
	if dir == "" {
		return nil, fmt.Errorf("revenue: empty dir")
	}
	if len(cfg.Secret) == 0 {
		return nil, fmt.Errorf("revenue: HMAC secret required")
	}
	if cfg.FreshnessWindow <= 0 {
		cfg.FreshnessWindow = 5 * time.Minute
	}
	if cfg.NonceCacheSize <= 0 {
		cfg.NonceCacheSize = 10_000
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Ingester{
		cfg:        cfg,
		dir:        dir,
		wallet:     wallet,
		audit:      log,
		seenNonces: map[string]time.Time{},
	}, nil
}

// ErrBadSignature is returned when HMAC verification fails.
var ErrBadSignature = errors.New("revenue: bad signature")

// ErrStaleTimestamp signals replay (out of freshness window).
var ErrStaleTimestamp = errors.New("revenue: stale timestamp")

// ErrReplayedNonce signals duplicate delivery.
var ErrReplayedNonce = errors.New("revenue: nonce already seen")

// ErrInvalidSplit signals split sums ≠ 1.0 (within tolerance).
var ErrInvalidSplit = errors.New("revenue: split ratios do not sum to 1.0")

// Result reports the outcome of an Accept call.
type Result struct {
	Accepted   bool
	TaskID     string
	AmountUSDT decimal.Decimal
	Shares     []ShareResult
	Reason     string // when Accepted=false
}

// ShareResult is one row of the per-agent fan-out.
type ShareResult struct {
	AgentID     string          `json:"agent_id"`
	ShareUSDT   decimal.Decimal `json:"share_usdt"`
	CreditErr   string          `json:"credit_err,omitempty"`
}

// Accept verifies the signature, freshness, nonce, and split, then
// (if all checks pass) fans the amount out across the agents and
// records audit + persistent ledger row. Returns Result.Accepted=true
// when at least the signature + freshness + nonce checks pass; share
// credit failures are reported per-row but do not invalidate the
// overall accept (matches "at-least-once + observable failure" semantics).
//
// rawBody is the exact bytes the client sent; we MUST hmac the raw
// body, not the re-marshalled payload, so any whitespace / key-order
// drift between client and server cannot break verification.
func (i *Ingester) Accept(rawBody []byte, signature string) (*Result, error) {
	if i == nil {
		return nil, fmt.Errorf("revenue: nil ingester")
	}
	// 1. HMAC verify.
	mac := hmac.New(sha256.New, i.cfg.Secret)
	mac.Write(rawBody)
	want := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(signature), []byte(want)) != 1 {
		return &Result{Reason: ErrBadSignature.Error()}, ErrBadSignature
	}

	// 2. Parse.
	var p IncomingPayload
	if err := json.Unmarshal(rawBody, &p); err != nil {
		return &Result{Reason: "invalid json"}, err
	}

	// 3. Freshness.
	now := time.Now().Unix()
	if abs(now-p.Timestamp) > int64(i.cfg.FreshnessWindow.Seconds()) {
		return &Result{Reason: ErrStaleTimestamp.Error() + " (now=" + strconv.FormatInt(now, 10) + ", ts=" + strconv.FormatInt(p.Timestamp, 10) + ")"}, ErrStaleTimestamp
	}

	// 4. Nonce uniqueness.
	if p.Nonce == "" {
		return &Result{Reason: "missing nonce"}, fmt.Errorf("revenue: missing nonce")
	}
	if err := i.recordNonce(p.Nonce); err != nil {
		return &Result{Reason: ErrReplayedNonce.Error()}, ErrReplayedNonce
	}

	// 5. Parse amount.
	total, err := decimal.NewFromString(p.AmountUSDT)
	if err != nil || !total.IsPositive() {
		return &Result{Reason: "invalid amount_usdt"}, fmt.Errorf("revenue: invalid amount_usdt")
	}

	// 6. Verify split ratios sum to 1.0.
	sum := decimal.Zero
	parsedRatios := make([]decimal.Decimal, len(p.Split))
	for idx, s := range p.Split {
		r, err := decimal.NewFromString(s.Ratio)
		if err != nil || r.IsNegative() {
			return &Result{Reason: "invalid ratio for " + s.AgentID}, fmt.Errorf("revenue: invalid ratio")
		}
		parsedRatios[idx] = r
		sum = sum.Add(r)
	}
	tolerance := decimal.NewFromFloat(0.0001)
	if sum.Sub(decimal.NewFromInt(1)).Abs().GreaterThan(tolerance) {
		return &Result{Reason: ErrInvalidSplit.Error() + " (sum=" + sum.String() + ")"}, ErrInvalidSplit
	}

	// 7. Fan out.
	shares := make([]ShareResult, 0, len(p.Split))
	for idx, s := range p.Split {
		shareAmt := total.Mul(parsedRatios[idx])
		share := ShareResult{AgentID: s.AgentID, ShareUSDT: shareAmt}
		if i.wallet != nil {
			if cerr := i.wallet(s.AgentID, shareAmt,
				fmt.Sprintf("revenue task=%s", p.TaskID)); cerr != nil {
				share.CreditErr = cerr.Error()
			}
		} else {
			share.CreditErr = "no wallet (dry-run)"
		}
		shares = append(shares, share)
		if i.audit != nil {
			_ = i.audit.Append(audit.Entry{
				Type:      "revenue.split",
				Subsystem: "revenue",
				AgentID:   s.AgentID,
				Detail: map[string]any{
					"task_id":    p.TaskID,
					"studio_id":  p.StudioID,
					"share_usdt": shareAmt.String(),
					"ratio":      parsedRatios[idx].String(),
					"credit_err": share.CreditErr,
				},
			})
		}
	}

	// 8. Persistent ledger.
	if perr := i.persist(&p, total, shares); perr != nil {
		// Don't fail the whole webhook — log via audit and continue.
		if i.audit != nil {
			_ = i.audit.Append(audit.Entry{
				Type: "revenue.persist_error", Subsystem: "revenue",
				Detail: map[string]any{"task_id": p.TaskID, "err": perr.Error()},
			})
		}
	}

	if i.audit != nil {
		_ = i.audit.Append(audit.Entry{
			Type:      "revenue.incoming",
			Subsystem: "revenue",
			Detail: map[string]any{
				"task_id":      p.TaskID,
				"studio_id":    p.StudioID,
				"amount_usdt":  total.String(),
				"nonce":        p.Nonce,
				"share_count":  len(shares),
			},
		})
	}

	return &Result{
		Accepted:   true,
		TaskID:     p.TaskID,
		AmountUSDT: total,
		Shares:     shares,
	}, nil
}

// recordNonce returns an error if the nonce was already seen, otherwise
// inserts it (with simple FIFO eviction to bound memory).
func (i *Ingester) recordNonce(nonce string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.seenNonces[nonce]; ok {
		return ErrReplayedNonce
	}
	i.seenNonces[nonce] = time.Now()
	i.nonceOrder = append(i.nonceOrder, nonce)
	for len(i.nonceOrder) > i.cfg.NonceCacheSize {
		evict := i.nonceOrder[0]
		i.nonceOrder = i.nonceOrder[1:]
		delete(i.seenNonces, evict)
	}
	return nil
}

// persist writes one row to <dir>/<period>.jsonl using the current UTC
// date as period. Append-only.
func (i *Ingester) persist(p *IncomingPayload, total decimal.Decimal, shares []ShareResult) error {
	period := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(i.dir, period+".jsonl")
	row := map[string]any{
		"ts":          time.Now().UnixMilli(),
		"task_id":     p.TaskID,
		"studio_id":   p.StudioID,
		"amount_usdt": total.String(),
		"nonce":       p.Nonce,
		"shares":      shares,
	}
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// SignFor is the helper a client (or our tests) calls to compute the
// HMAC over rawBody using the same secret. Exposed for symmetry +
// testing; the production market generates its own signatures with
// the shared secret.
func SignFor(secret, rawBody []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
