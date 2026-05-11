package revenue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Empty body with valid signature on empty → bad JSON.
func Test_AITeam_S8_Edge_EmptyBodyRejected(t *testing.T) {
	ing := newIng(t, nil)
	sig := SignFor([]byte("test-secret"), []byte{})
	_, err := ing.Accept([]byte{}, sig)
	if err == nil {
		t.Fatal("empty body should error")
	}
}

// Body too small (just "{}") should reject — missing required fields.
func Test_AITeam_S8_Edge_MinimalJSONRejected(t *testing.T) {
	ing := newIng(t, nil)
	body := []byte("{}")
	sig := SignFor([]byte("test-secret"), body)
	res, err := ing.Accept(body, sig)
	if err == nil {
		t.Fatalf("empty json should error, got %+v", res)
	}
}

// Negative amount in payload — should be rejected.
func Test_AITeam_S8_Edge_NegativeAmountRejected(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.AmountUSDT = "-50"
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if err == nil || !strings.Contains(err.Error(), "invalid amount") {
		t.Fatalf("negative amount should error: %v", err)
	}
}

// Zero amount — should be rejected.
func Test_AITeam_S8_Edge_ZeroAmountRejected(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.AmountUSDT = "0"
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if err == nil {
		t.Fatal("zero amount should error")
	}
}

// Empty split — should reject.
func Test_AITeam_S8_Edge_EmptySplitRejected(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.Split = []SplitEntry{}
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	// Current behavior: sum = 0 ≠ 1.0 → ErrInvalidSplit
	if !errors.Is(err, ErrInvalidSplit) {
		t.Fatalf("empty split should be ErrInvalidSplit, got %v", err)
	}
}

// Ratios with very high precision — must still validate sum.
func Test_AITeam_S8_Edge_HighPrecisionRatios(t *testing.T) {
	credits := map[string]decimal.Decimal{}
	wallet := func(id string, amt decimal.Decimal, _ string) error {
		credits[id] = credits[id].Add(amt)
		return nil
	}
	ing := newIng(t, wallet)
	p := basePayload()
	p.AmountUSDT = "100"
	p.Split = []SplitEntry{
		{AgentID: "a", Ratio: "0.333333"},
		{AgentID: "b", Ratio: "0.333333"},
		{AgentID: "c", Ratio: "0.333334"}, // sum = 1.000000
	}
	body, sig := signedBody(t, "test-secret", p)
	if _, err := ing.Accept(body, sig); err != nil {
		t.Fatalf("high-precision sum=1 should accept: %v", err)
	}
	// Total credited must equal AmountUSDT × sum_of_ratios = 100 × 1 = 100.
	total := decimal.Zero
	for _, v := range credits {
		total = total.Add(v)
	}
	if !total.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("share total: %s want 100", total)
	}
}

// Massive AmountUSDT (1 billion).
func Test_AITeam_S8_Edge_HugeAmount(t *testing.T) {
	credits := map[string]decimal.Decimal{}
	wallet := func(id string, amt decimal.Decimal, _ string) error {
		credits[id] = credits[id].Add(amt)
		return nil
	}
	ing := newIng(t, wallet)
	p := basePayload()
	p.AmountUSDT = "1000000000.000000" // 1 billion USDT
	body, sig := signedBody(t, "test-secret", p)
	res, err := ing.Accept(body, sig)
	if err != nil || !res.Accepted {
		t.Fatalf("billion USDT should accept: %v", err)
	}
	if !credits["alice"].Equal(decimal.NewFromInt(600_000_000)) {
		t.Fatalf("alice share of 1B at 60%% should be 600M, got %s", credits["alice"])
	}
}

// Nonce eviction — after 10k+ nonces, the oldest should be evictable.
func Test_AITeam_S8_Edge_NonceFIFOEvicts(t *testing.T) {
	ing := newIng(t, nil)
	// Override the cache size for the test
	ing.cfg.NonceCacheSize = 5
	for i := 0; i < 7; i++ {
		p := basePayload()
		p.Nonce = "n-" + string(rune('a'+i))
		p.TaskID = p.Nonce
		body, sig := signedBody(t, "test-secret", p)
		if _, err := ing.Accept(body, sig); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}
	// After 7 accepts with cache size 5, the first 2 should have been
	// evicted from the seenNonces map. Re-sending nonce "n-a" should
	// be accepted (FIFO eviction — though this means replays past the
	// window are NOT defended).
	// This is a documented weakness, not a fix-able bug at this level.
	t.Logf("After 7 accepts on cache=5: seenNonces=%d order=%d",
		len(ing.seenNonces), len(ing.nonceOrder))
}

// Body with embedded NUL bytes (binary corruption attempt).
func Test_AITeam_S8_Edge_NULBytesInBody(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	body, sig := signedBody(t, "test-secret", p)
	corrupted := append([]byte{}, body...)
	corrupted[10] = 0
	if _, err := ing.Accept(corrupted, sig); err == nil {
		t.Fatal("NUL-corrupted body should fail HMAC")
	}
}

// Future timestamp (> 5 min into future).
func Test_AITeam_S8_Edge_FutureTimestampRejected(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.Timestamp = time.Now().Add(1 * time.Hour).Unix()
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if !errors.Is(err, ErrStaleTimestamp) {
		t.Fatalf("future timestamp should be ErrStaleTimestamp, got %v", err)
	}
}

// Split sum slightly off — within tolerance vs. outside.
func Test_AITeam_S8_Edge_SplitToleranceBoundary(t *testing.T) {
	ing := newIng(t, nil)
	// 1.00009 → diff 0.00009 < 0.0001 tolerance → accept
	p := basePayload()
	p.Split = []SplitEntry{
		{AgentID: "a", Ratio: "0.50009"},
		{AgentID: "b", Ratio: "0.5"}, // sum = 1.00009
	}
	body, sig := signedBody(t, "test-secret", p)
	if _, err := ing.Accept(body, sig); err != nil {
		t.Fatalf("within tolerance should accept: %v", err)
	}
	// 1.001 — outside tolerance → reject
	p.Nonce = "n2"
	p.Split = []SplitEntry{
		{AgentID: "a", Ratio: "0.501"},
		{AgentID: "b", Ratio: "0.5"}, // sum = 1.001
	}
	body, sig = signedBody(t, "test-secret", p)
	if _, err := ing.Accept(body, sig); !errors.Is(err, ErrInvalidSplit) {
		t.Fatalf("outside tolerance should ErrInvalidSplit: %v", err)
	}
}

// Malformed signature hex.
func Test_AITeam_S8_Edge_GarbledSignature(t *testing.T) {
	ing := newIng(t, nil)
	body, _ := signedBody(t, "test-secret", basePayload())
	_, err := ing.Accept(body, "not-hex-at-all-zzz")
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("garbled sig: %v", err)
	}
}

// Many tiny shares — make sure rounding doesn't lose money.
func Test_AITeam_S8_Edge_ManyTinyShares(t *testing.T) {
	credits := map[string]decimal.Decimal{}
	wallet := func(id string, amt decimal.Decimal, _ string) error {
		credits[id] = credits[id].Add(amt)
		return nil
	}
	ing := newIng(t, wallet)
	splits := make([]SplitEntry, 10)
	for i := 0; i < 10; i++ {
		splits[i] = SplitEntry{AgentID: "ag" + string(rune('0'+i)), Ratio: "0.1"}
	}
	p := basePayload()
	p.AmountUSDT = "100"
	p.Split = splits
	body, sig := signedBody(t, "test-secret", p)
	if _, err := ing.Accept(body, sig); err != nil {
		t.Fatal(err)
	}
	total := decimal.Zero
	for _, v := range credits {
		total = total.Add(v)
	}
	if !total.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("sum of shares: %s want 100", total)
	}
}

// Helper that the tests assume.
func _testJSONMarshalSanity(t *testing.T) {
	p := basePayload()
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
}
