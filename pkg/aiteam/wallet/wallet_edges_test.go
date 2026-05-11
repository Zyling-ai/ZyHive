package wallet

import (
	"strings"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
)

// ── Test_AITeam_S8_Edge_*: hardening edge cases for the wallet ──────────────

// Negative amounts must be rejected (not just zero).
func Test_AITeam_S8_Edge_NegativeAmountRejected(t *testing.T) {
	s := newStore(t)
	cases := []decimal.Decimal{usdt("-1"), usdt("-0.000001"), usdt("-1000000")}
	for _, amt := range cases {
		if _, err := s.Credit("alice", amt, "neg"); err != ErrInvalidAmount {
			t.Errorf("Credit(%s) → %v, want ErrInvalidAmount", amt, err)
		}
		if _, err := s.Debit("alice", amt, "neg"); err != ErrInvalidAmount {
			t.Errorf("Debit(%s) → %v, want ErrInvalidAmount", amt, err)
		}
	}
}

// Extremely large amounts (uint64 max-ish) should not panic.
func Test_AITeam_S8_Edge_HugeAmountHandled(t *testing.T) {
	s := newStore(t)
	huge := usdt("999999999999999999999999.999999") // 24 digit integer + 6 fraction
	if _, err := s.Credit("alice", huge, "huge"); err != nil {
		t.Fatalf("huge credit must succeed (decimal is unbounded): %v", err)
	}
	if !s.Balance("alice").Equal(huge) {
		t.Fatalf("balance after huge credit: %s", s.Balance("alice"))
	}
}

// Empty agent ID — current API allows it (creates account with empty key).
// Test we either reject or handle deterministically. The wallet accepts —
// document expected behavior: it CREATES an account with ID "".
func Test_AITeam_S8_Edge_EmptyAgentID(t *testing.T) {
	s := newStore(t)
	// Current behavior: creates an "" account. Verify it doesn't crash.
	if _, err := s.Credit("", usdt("1"), "empty"); err != nil {
		t.Fatalf("empty agent id should be allowed (current behavior), got %v", err)
	}
	if !s.Balance("").Equal(usdt("1")) {
		t.Fatalf("empty-key balance: %s", s.Balance(""))
	}
	// Note: filing as documentation that empty-id needs validation upstream.
}

// Agent IDs with special chars / path traversal / unicode.
func Test_AITeam_S8_Edge_AgentIDSpecialChars(t *testing.T) {
	s := newStore(t)
	cases := []string{
		"../escape",
		"with space",
		"with/slash",
		"with\\backslash",
		"with:colon",
		"中文ID",
		"emoji🎉",
	}
	for _, id := range cases {
		_, err := s.Credit(id, usdt("0.01"), "char-test")
		// Filesystem may reject some patterns; verify it doesn't crash.
		// "../escape" would be the most concerning (path traversal).
		if err != nil {
			t.Logf("Credit(%q) → %v (handled, not crashing)", id, err)
		} else {
			// Verify balance retrievable
			if !s.Balance(id).Equal(usdt("0.01")) {
				t.Errorf("balance(%q) = %s, want 0.01", id, s.Balance(id))
			}
		}
	}
}

// Many small credits should not accumulate float errors. decimal-Decimal
// guarantees exact accumulation — verify 10000 iterations stay exact.
func Test_AITeam_S8_Edge_TenThousandCreditsExact(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 10000; i++ {
		_, _ = s.Credit("alice", usdt("0.0001"), "")
	}
	// 10000 × 0.0001 = exactly 1.0
	if !s.Balance("alice").Equal(usdt("1")) {
		t.Fatalf("after 10000×0.0001 credits, want exactly 1.0, got %s",
			s.Balance("alice"))
	}
}

// Transfer with concurrent same-pair attempts — Verify lock ordering
// (lexicographic) prevents deadlock.
func Test_AITeam_S8_Edge_ConcurrentTransferSamePairNoDeadlock(t *testing.T) {
	s := newStore(t)
	_, _ = s.Credit("alice", usdt("100"), "g")
	_, _ = s.Credit("bob", usdt("100"), "g")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		// alice → bob in one goroutine
		go func() {
			defer wg.Done()
			_ = s.Transfer("alice", "bob", usdt("0.01"), "concurrent")
		}()
		// bob → alice in another (same pair, reverse direction)
		go func() {
			defer wg.Done()
			_ = s.Transfer("bob", "alice", usdt("0.01"), "concurrent")
		}()
	}
	wg.Wait()
	// Both balances should sum to original 200 USDT (no money created or
	// lost in transfers between participants).
	total := s.Balance("alice").Add(s.Balance("bob"))
	if !total.Equal(usdt("200")) {
		t.Fatalf("total balance changed under concurrent transfers: %s", total)
	}
}

// Ledger entry count beyond pagination — Ledger(0) should return all.
func Test_AITeam_S8_Edge_LedgerLargeReturnsAll(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 1500; i++ {
		_, _ = s.Credit("alice", usdt("0.0001"), "")
	}
	entries, err := s.Ledger("alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1500 {
		t.Fatalf("expected 1500 entries with limit=0 (all), got %d", len(entries))
	}
}

// Replay after corrupted line (mid-file) — must SKIP bad lines and
// continue. Bug-fix P3-S8: previously aborted entire replay.
func Test_AITeam_S8_Edge_CorruptedLineMidLedgerSkipsAndContinues(t *testing.T) {
	dir := t.TempDir()
	s1, _ := New(dir, nil, nil)
	_, _ = s1.Credit("alice", usdt("1"), "g")
	_, _ = s1.Credit("alice", usdt("2"), "g2")
	_, _ = s1.Credit("alice", usdt("3"), "g3")

	// Inject a garbage line in the middle of the JSONL file.
	path := s1.ledgerPath("alice")
	data, _ := readFile(path)
	corrupted := strings.Replace(data, "\n", "\nthisisnotjson\n", 1)
	writeFile(path, corrupted)

	// Re-open — replay must skip the bad line and recover the rest.
	s2, err := New(dir, nil, nil)
	if err != nil {
		t.Fatalf("replay should skip corrupted lines, not abort: %v", err)
	}
	// Balance after replay should reflect the LAST valid entry.
	// (Either 1+2+3=6 if all 3 valid lines parsed, OR 1+3=4 if first credit
	// was scrambled. Either is acceptable; what we MUST avoid is total
	// abort.)
	bal := s2.Balance("alice")
	if bal.IsZero() {
		t.Fatalf("expected positive balance after skip-and-continue, got 0")
	}
	t.Logf("recovered balance after corrupt-line skip: %s", bal)
}

// Hook panics — must not corrupt ledger or block subsequent writes.
// Bug-fix P3-S8: hook is now run inside recover() so its panic cannot
// reach the caller.
func Test_AITeam_S8_Edge_HookPanicDoesNotBreakLedger(t *testing.T) {
	s := newStore(t)
	s.SetWriteHook(func(_ string, _ decimal.Decimal) {
		panic("misbehaving hook")
	})

	// MUST NOT panic.
	_, err := s.Credit("alice", usdt("1"), "g")
	if err != nil {
		t.Fatalf("Credit should succeed despite hook panic: %v", err)
	}

	// Verify ledger row was actually written.
	if !s.Balance("alice").Equal(usdt("1")) {
		t.Fatalf("balance after credit (despite hook panic): %s", s.Balance("alice"))
	}

	// Second write should also succeed.
	if _, err := s.Credit("alice", usdt("0.5"), "g2"); err != nil {
		t.Fatalf("second credit should succeed: %v", err)
	}
	if !s.Balance("alice").Equal(usdt("1.5")) {
		t.Fatalf("ledger broken after hook panic: %s", s.Balance("alice"))
	}
}

// helpers
func readFile(path string) (string, error) {
	b, err := readFileImpl(path)
	return string(b), err
}
func writeFile(path, content string) {
	writeFileImpl(path, []byte(content))
}
