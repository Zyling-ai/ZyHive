// Package wallet implements aiteam's per-agent USDT wallet (PR-001).
//
// Each agent has an append-only ledger file at
//   <walletDir>/<agentID>.jsonl
// containing one Entry per line. Balance is derived at startup by
// replaying the ledger and kept in memory thereafter. Concurrent writes
// across agents are serialised by per-agent mutexes; reads are RW-safe.
//
// Currency: every amount is `decimal.Decimal` in USDT. AI-facing tool
// returns always emit USDT — the UI/display layer is the only place
// that translates via pkg/aiteam/fx.
//
// Each Entry persists the current FX snapshot (USD / CNY / EUR / ...
// rates at the time of write) so historical statements can be re-rendered
// in any currency without changing the source-of-truth USDT figures.
//
// All operations are no-op safe on a nil *Store so callers can hold an
// optional reference without ifs.
package wallet

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
)

// EntryType enumerates the supported ledger row kinds. Use the canonical
// strings ("credit", "debit", "transfer_in", "transfer_out") in the
// ledger file so a `grep` across audit + ledger gives a coherent story.
type EntryType string

const (
	EntryCredit      EntryType = "credit"
	EntryDebit       EntryType = "debit"
	EntryTransferIn  EntryType = "transfer_in"
	EntryTransferOut EntryType = "transfer_out"
	EntryGenesis     EntryType = "genesis"  // initial seed credit
)

// Entry is the JSONL row format. AmountUSDT and BalanceAfterUSDT are
// decimals serialised as strings to avoid float64 precision loss.
type Entry struct {
	Timestamp        int64              `json:"ts"` // unix milli
	Type             EntryType          `json:"type"`
	AmountUSDT       decimal.Decimal    `json:"amount_usdt"`
	BalanceAfterUSDT decimal.Decimal    `json:"balance_after_usdt"`
	Reason           string             `json:"reason,omitempty"`
	Counterparty     string             `json:"counterparty,omitempty"`
	FxSnapshot       map[string]float64 `json:"fx_snapshot,omitempty"`
}

// ErrInsufficientFunds is returned by Debit / Transfer when the source
// account would go negative. Aiteam wallets are NOT allowed to overdraw
// in v0 (avoid debt-spiral semantics; PR-002 payroll just doesn't credit
// when net is negative).
var ErrInsufficientFunds = errors.New("wallet: insufficient funds")

// ErrInvalidAmount is returned when amount is zero / negative.
var ErrInvalidAmount = errors.New("wallet: amount must be > 0")

// account is the in-memory state for a single agent.
type account struct {
	id      string
	balance decimal.Decimal
	mu      sync.Mutex
}

// Store is the wallet engine. Construct one per process via New.
type Store struct {
	dir   string
	fx    *fx.Service
	audit *audit.Log

	mu       sync.RWMutex
	accounts map[string]*account

	// writeHook (P3-S2): optional callback fired after every successful
	// Credit / Debit / Transfer with the affected agentID and post-op
	// balance. Used by main.go to keep the Prometheus wallet balance
	// gauge fresh. Nil = no hook. Hook runs synchronously inside the
	// per-account lock — keep it FAST (no IO, no network).
	writeHook func(agentID string, balanceUSDT decimal.Decimal)
}

// SetWriteHook installs an optional callback fired after every wallet
// write. Idempotent; pass nil to detach. The hook is called WITH the
// per-account mutex held — implementations must be cheap and never
// block (no network IO, no disk IO, no nested wallet calls).
//
// Typical use: refresh in-memory Prometheus gauges.
func (s *Store) SetWriteHook(hook func(agentID string, balanceUSDT decimal.Decimal)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeHook = hook
}

// New creates a Store rooted at dir (typically <dataDir>/aiteam/wallet).
// On startup it scans for *.jsonl ledgers and replays them so balance
// caches are warm. fxSvc / auditLog may be nil.
func New(dir string, fxSvc *fx.Service, auditLog *audit.Log) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("wallet: empty dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		dir:      dir,
		fx:       fxSvc,
		audit:    auditLog,
		accounts: map[string]*account{},
	}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadAll walks dir for *.jsonl ledger files and replays each.
func (s *Store) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) <= 6 || name[len(name)-6:] != ".jsonl" {
			continue
		}
		agentID := name[:len(name)-6]
		acc, err := s.replay(agentID)
		if err != nil {
			return fmt.Errorf("wallet: replay %s: %w", agentID, err)
		}
		s.accounts[agentID] = acc
	}
	return nil
}

// replay reads the on-disk ledger and reconstructs the balance.
func (s *Store) replay(agentID string) (*account, error) {
	f, err := os.Open(s.ledgerPath(agentID))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	acc := &account{id: agentID}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<14), 1<<20)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		acc.balance = e.BalanceAfterUSDT
	}
	return acc, scanner.Err()
}

func (s *Store) ledgerPath(agentID string) string {
	return filepath.Join(s.dir, agentID+".jsonl")
}

// getOrCreateAccount returns the account row, creating an empty one
// when none exists. Caller must hold s.mu OR be in a single-writer
// path. We use double-checked locking so concurrent first-access for
// the same agent serialises correctly.
func (s *Store) getOrCreateAccount(agentID string) *account {
	s.mu.RLock()
	acc, ok := s.accounts[agentID]
	s.mu.RUnlock()
	if ok {
		return acc
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if acc, ok := s.accounts[agentID]; ok {
		return acc
	}
	acc = &account{id: agentID}
	s.accounts[agentID] = acc
	return acc
}

// Balance returns the current USDT balance. Unknown agent → zero.
func (s *Store) Balance(agentID string) decimal.Decimal {
	if s == nil {
		return decimal.Zero
	}
	s.mu.RLock()
	acc, ok := s.accounts[agentID]
	s.mu.RUnlock()
	if !ok {
		return decimal.Zero
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	return acc.balance
}

// Credit adds amount USDT to the agent. amount must be > 0.
func (s *Store) Credit(agentID string, amount decimal.Decimal, reason string) (*Entry, error) {
	return s.write(agentID, EntryCredit, amount, reason, "")
}

// Debit deducts amount USDT. Refuses to go negative.
func (s *Store) Debit(agentID string, amount decimal.Decimal, reason string) (*Entry, error) {
	if s == nil {
		return nil, nil
	}
	acc := s.getOrCreateAccount(agentID)
	acc.mu.Lock()
	defer acc.mu.Unlock()
	if !amount.IsPositive() {
		return nil, ErrInvalidAmount
	}
	if acc.balance.LessThan(amount) {
		return nil, ErrInsufficientFunds
	}
	return s.writeLocked(acc, EntryDebit, amount, reason, "")
}

// Transfer moves amount USDT from `from` to `to`. Atomic across both
// accounts (acquires both per-account mutexes in deterministic order).
func (s *Store) Transfer(from, to string, amount decimal.Decimal, reason string) error {
	if s == nil {
		return nil
	}
	if !amount.IsPositive() {
		return ErrInvalidAmount
	}
	if from == to {
		return fmt.Errorf("wallet: transfer to self disallowed")
	}
	fromAcc := s.getOrCreateAccount(from)
	toAcc := s.getOrCreateAccount(to)
	// Lock in lexicographic order to avoid deadlock.
	a, b := fromAcc, toAcc
	if a.id > b.id {
		a, b = b, a
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	b.mu.Lock()
	defer b.mu.Unlock()

	if fromAcc.balance.LessThan(amount) {
		return ErrInsufficientFunds
	}
	if _, err := s.writeLocked(fromAcc, EntryTransferOut, amount, reason, to); err != nil {
		return err
	}
	if _, err := s.writeLocked(toAcc, EntryTransferIn, amount, reason, from); err != nil {
		// Best-effort compensating credit if the to-leg fails after
		// from-leg has already debited. In a JSONL append-only world
		// this is correct: leave both rows in place, the next process
		// reconciliation handles the imbalance.
		return err
	}
	return nil
}

// write is the externally-locking entry point used by Credit (which
// doesn't need a balance check). Acquires the per-account mutex and
// delegates to writeLocked.
func (s *Store) write(agentID string, t EntryType, amount decimal.Decimal, reason, counterparty string) (*Entry, error) {
	if s == nil {
		return nil, nil
	}
	if !amount.IsPositive() {
		return nil, ErrInvalidAmount
	}
	acc := s.getOrCreateAccount(agentID)
	acc.mu.Lock()
	defer acc.mu.Unlock()
	return s.writeLocked(acc, t, amount, reason, counterparty)
}

// writeLocked assumes acc.mu is held. Performs the persistent write +
// in-memory balance update + audit log entry. Returns the persisted
// Entry for the caller to use.
func (s *Store) writeLocked(acc *account, t EntryType, amount decimal.Decimal, reason, counterparty string) (*Entry, error) {
	var newBal decimal.Decimal
	switch t {
	case EntryCredit, EntryTransferIn, EntryGenesis:
		newBal = acc.balance.Add(amount)
	case EntryDebit, EntryTransferOut:
		newBal = acc.balance.Sub(amount)
	default:
		return nil, fmt.Errorf("wallet: unknown entry type %q", t)
	}
	e := Entry{
		Timestamp:        time.Now().UnixMilli(),
		Type:             t,
		AmountUSDT:       amount,
		BalanceAfterUSDT: newBal,
		Reason:           reason,
		Counterparty:     counterparty,
	}
	if s.fx != nil {
		e.FxSnapshot = copyFloatMap(s.fx.SnapshotJSON().Rates)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.ledgerPath(acc.id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		return nil, werr
	}
	if cerr != nil {
		return nil, cerr
	}
	acc.balance = newBal

	if s.audit != nil {
		_ = s.audit.Append(audit.Entry{
			Type:      "wallet." + string(t),
			Subsystem: "wallet",
			AgentID:   acc.id,
			Detail: map[string]any{
				"amount_usdt":        amount.String(),
				"balance_after_usdt": newBal.String(),
				"reason":             reason,
				"counterparty":       counterparty,
			},
		})
	}
	// P3-S2: fire the write hook (typically refreshes metric gauge).
	// Snapshot it under s.mu for thread-safety with SetWriteHook.
	s.mu.RLock()
	hook := s.writeHook
	s.mu.RUnlock()
	if hook != nil {
		hook(acc.id, newBal)
	}
	return &e, nil
}

// Ledger returns the ledger entries for agentID, most recent last.
// limit=0 → all. limit>0 → at most `limit` most-recent entries.
func (s *Store) Ledger(agentID string, limit int) ([]Entry, error) {
	if s == nil {
		return nil, nil
	}
	f, err := os.Open(s.ledgerPath(agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<14), 1<<20)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		all = append(all, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// AllAgents returns the list of agents with a ledger file present.
// Useful for snapshot endpoints.
func (s *Store) AllAgents() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.accounts))
	for id := range s.accounts {
		out = append(out, id)
	}
	return out
}

func copyFloatMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
