package wallet

import (
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
)

func usdt(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func Test_AITeam_Wallet_StartsEmpty(t *testing.T) {
	s := newStore(t)
	if !s.Balance("alice").IsZero() {
		t.Fatalf("expected zero balance, got %s", s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_CreditAddsBalance(t *testing.T) {
	s := newStore(t)
	if _, err := s.Credit("alice", usdt("1.50"), "genesis"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Credit("alice", usdt("0.25"), "topup"); err != nil {
		t.Fatal(err)
	}
	if !s.Balance("alice").Equal(usdt("1.75")) {
		t.Fatalf("expected 1.75, got %s", s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_DebitRefusesOverdraft(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("1.00"), "g")
	if _, err := s.Debit("alice", usdt("1.50"), "x"); err != ErrInsufficientFunds {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
	if !s.Balance("alice").Equal(usdt("1.00")) {
		t.Fatalf("balance should be untouched, got %s", s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_DebitDeducts(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("5.00"), "g")
	if _, err := s.Debit("alice", usdt("1.25"), "llm"); err != nil {
		t.Fatal(err)
	}
	if !s.Balance("alice").Equal(usdt("3.75")) {
		t.Fatalf("expected 3.75, got %s", s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_TransferMovesBetweenAgents(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("10.00"), "g")
	if err := s.Transfer("alice", "bob", usdt("3.00"), "split"); err != nil {
		t.Fatal(err)
	}
	if !s.Balance("alice").Equal(usdt("7.00")) {
		t.Fatalf("alice = %s, want 7.00", s.Balance("alice"))
	}
	if !s.Balance("bob").Equal(usdt("3.00")) {
		t.Fatalf("bob = %s, want 3.00", s.Balance("bob"))
	}
}

func Test_AITeam_Wallet_TransferToSelfDisallowed(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("10.00"), "g")
	if err := s.Transfer("alice", "alice", usdt("1.00"), "x"); err == nil {
		t.Fatal("expected self-transfer error")
	}
}

func Test_AITeam_Wallet_InvalidAmountsRejected(t *testing.T) {
	s := newStore(t)
	if _, err := s.Credit("alice", usdt("0"), "x"); err != ErrInvalidAmount {
		t.Fatalf("expected ErrInvalidAmount on zero credit")
	}
	if _, err := s.Debit("alice", usdt("-5"), "x"); err != ErrInvalidAmount {
		t.Fatalf("expected ErrInvalidAmount on negative debit")
	}
}

func Test_AITeam_Wallet_BalancePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s1, _ := New(dir, nil, nil)
	_, _ = s1.Credit("alice", usdt("2.50"), "g")
	_, _ = s1.Debit("alice", usdt("0.50"), "x")
	if !s1.Balance("alice").Equal(usdt("2.00")) {
		t.Fatalf("pre-restart balance = %s", s1.Balance("alice"))
	}
	// Re-open
	s2, err := New(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Balance("alice").Equal(usdt("2.00")) {
		t.Fatalf("post-restart balance = %s, want 2.00", s2.Balance("alice"))
	}
}

func Test_AITeam_Wallet_LedgerReturnsEntries(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("1.00"), "g1")
	_, _ = s.Credit("alice", usdt("2.00"), "g2")
	_, _ = s.Debit("alice", usdt("0.50"), "d1")
	entries, err := s.Ledger("alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Type != EntryCredit || entries[2].Type != EntryDebit {
		t.Fatalf("entry types out of order: %+v", entries)
	}
	if !entries[2].BalanceAfterUSDT.Equal(usdt("2.50")) {
		t.Fatalf("final balance row = %s, want 2.50", entries[2].BalanceAfterUSDT)
	}
}

func Test_AITeam_Wallet_LedgerLimit(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 10; i++ {
		_, _ = s.Credit("alice", usdt("0.10"), "")
	}
	entries, _ := s.Ledger("alice", 3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 most-recent entries, got %d", len(entries))
	}
	// Last entry's balance must reflect all 10 credits.
	if !entries[2].BalanceAfterUSDT.Equal(usdt("1.0")) {
		t.Fatalf("final balance = %s", entries[2].BalanceAfterUSDT)
	}
}

func Test_AITeam_Wallet_ConcurrentCreditsAreSafe(t *testing.T) {
	s := newStore(t)
	var wg sync.WaitGroup
	const N = 200
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = s.Credit("alice", usdt("0.01"), "concurrent")
		}()
	}
	wg.Wait()
	if !s.Balance("alice").Equal(usdt("2")) {
		t.Fatalf("after 200 concurrent 0.01 credits, balance = %s want 2", s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_DecimalPrecisionPreserved(t *testing.T) {
	s := newStore(t)
	// 1000 random-ish micro-credits should accumulate without float64
	// drift. Using rational fractions to expose any decimal loss.
	for i := 0; i < 1000; i++ {
		_, _ = s.Credit("alice", usdt("0.000123"), "")
	}
	want := usdt("0.123")
	if !s.Balance("alice").Equal(want) {
		t.Fatalf("expected exact %s after 1000×0.000123, got %s", want, s.Balance("alice"))
	}
}

func Test_AITeam_Wallet_FxSnapshotInLedger(t *testing.T) {
	fxSvc := fx.New("")
	s, _ := New(t.TempDir(), fxSvc, nil)
	_, _ = s.Credit("alice", usdt("1.00"), "g")
	entries, _ := s.Ledger("alice", 0)
	if entries[0].FxSnapshot == nil {
		t.Fatal("expected fx_snapshot in entry")
	}
	if entries[0].FxSnapshot["CNY"] == 0 {
		t.Fatalf("CNY rate should be present, got %+v", entries[0].FxSnapshot)
	}
}

func Test_AITeam_Wallet_AuditLogged(t *testing.T) {
	dir := t.TempDir()
	log, _ := audit.New(dir)
	s, _ := New(t.TempDir(), nil, log)
	_, _ = s.Credit("alice", usdt("1.00"), "g")
	_, _ = s.Debit("alice", usdt("0.25"), "d")
	if log.LineCount() != 2 {
		t.Fatalf("expected 2 audit rows, got %d", log.LineCount())
	}
}

func Test_AITeam_Wallet_NilStoreSafe(t *testing.T) {
	var s *Store
	if !s.Balance("alice").IsZero() {
		t.Fatal("nil store should return zero balance")
	}
	if _, err := s.Credit("alice", usdt("1"), "x"); err != nil {
		t.Fatalf("nil store Credit should not error, got %v", err)
	}
}
