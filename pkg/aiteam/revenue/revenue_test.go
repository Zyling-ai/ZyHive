package revenue

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func newIng(t *testing.T, wallet WalletCredit) *Ingester {
	t.Helper()
	ing, err := New(t.TempDir(), Config{
		Secret:          []byte("test-secret"),
		FreshnessWindow: 5 * time.Minute,
	}, wallet, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ing
}

// signedBody helper: marshal payload, compute signature.
func signedBody(t *testing.T, secret string, p IncomingPayload) ([]byte, string) {
	t.Helper()
	body, _ := json.Marshal(p)
	sig := SignFor([]byte(secret), body)
	return body, sig
}

func basePayload() IncomingPayload {
	return IncomingPayload{
		TaskID:     "task-1",
		StudioID:   "studio-foo",
		AmountUSDT: "50.000000",
		Split: []SplitEntry{
			{AgentID: "alice", Ratio: "0.6"},
			{AgentID: "bob", Ratio: "0.4"},
		},
		Timestamp: time.Now().Unix(),
		Nonce:     "nonce-1",
	}
}

func Test_AITeam_Revenue_AcceptsValidWebhook(t *testing.T) {
	credits := map[string]decimal.Decimal{}
	wallet := func(id string, amt decimal.Decimal, _ string) error {
		credits[id] = credits[id].Add(amt)
		return nil
	}
	ing := newIng(t, wallet)
	body, sig := signedBody(t, "test-secret", basePayload())
	res, err := ing.Accept(body, sig)
	if err != nil || !res.Accepted {
		t.Fatalf("expected accept, got err=%v res=%+v", err, res)
	}
	if !credits["alice"].Equal(decimal.NewFromFloat(30)) {
		t.Fatalf("alice should get 30 (60%% of 50), got %s", credits["alice"])
	}
	if !credits["bob"].Equal(decimal.NewFromFloat(20)) {
		t.Fatalf("bob should get 20, got %s", credits["bob"])
	}
}

func Test_AITeam_Revenue_RejectsBadHMAC(t *testing.T) {
	ing := newIng(t, nil)
	body, _ := signedBody(t, "test-secret", basePayload())
	_, err := ing.Accept(body, "deadbeef")
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func Test_AITeam_Revenue_RejectsStaleTimestamp(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.Timestamp = time.Now().Add(-1 * time.Hour).Unix() // far outside window
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if !errors.Is(err, ErrStaleTimestamp) {
		t.Fatalf("expected ErrStaleTimestamp, got %v", err)
	}
}

func Test_AITeam_Revenue_RejectsReplayedNonce(t *testing.T) {
	ing := newIng(t, nil)
	body, sig := signedBody(t, "test-secret", basePayload())
	if _, err := ing.Accept(body, sig); err != nil {
		t.Fatalf("first accept failed: %v", err)
	}
	_, err := ing.Accept(body, sig)
	if !errors.Is(err, ErrReplayedNonce) {
		t.Fatalf("expected ErrReplayedNonce, got %v", err)
	}
}

func Test_AITeam_Revenue_RejectsBadSplitSum(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.Split = []SplitEntry{
		{AgentID: "alice", Ratio: "0.7"},
		{AgentID: "bob", Ratio: "0.4"},
	}
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if !errors.Is(err, ErrInvalidSplit) {
		t.Fatalf("expected ErrInvalidSplit, got %v", err)
	}
}

func Test_AITeam_Revenue_SplitsExactlyOnePercent(t *testing.T) {
	credits := map[string]decimal.Decimal{}
	wallet := func(id string, amt decimal.Decimal, _ string) error {
		credits[id] = credits[id].Add(amt)
		return nil
	}
	ing := newIng(t, wallet)
	p := basePayload()
	p.AmountUSDT = "100.00"
	p.Split = []SplitEntry{
		{AgentID: "x1", Ratio: "0.25"},
		{AgentID: "x2", Ratio: "0.25"},
		{AgentID: "x3", Ratio: "0.5"},
	}
	body, sig := signedBody(t, "test-secret", p)
	res, err := ing.Accept(body, sig)
	if err != nil || !res.Accepted {
		t.Fatalf("not accepted: %v %+v", err, res)
	}
	if !credits["x1"].Equal(decimal.NewFromFloat(25)) || !credits["x3"].Equal(decimal.NewFromFloat(50)) {
		t.Fatalf("split math wrong: %+v", credits)
	}
}

func Test_AITeam_Revenue_WalletFailurePropagatesPerShare(t *testing.T) {
	wallet := func(id string, _ decimal.Decimal, _ string) error {
		if id == "alice" {
			return errors.New("simulated")
		}
		return nil
	}
	ing := newIng(t, wallet)
	body, sig := signedBody(t, "test-secret", basePayload())
	res, err := ing.Accept(body, sig)
	if err != nil || !res.Accepted {
		t.Fatalf("share failures should not fail overall accept: err=%v res=%+v", err, res)
	}
	var aliceShare, bobShare *ShareResult
	for i := range res.Shares {
		if res.Shares[i].AgentID == "alice" {
			aliceShare = &res.Shares[i]
		}
		if res.Shares[i].AgentID == "bob" {
			bobShare = &res.Shares[i]
		}
	}
	if aliceShare == nil || aliceShare.CreditErr == "" {
		t.Fatalf("alice should have credit_err: %+v", aliceShare)
	}
	if bobShare == nil || bobShare.CreditErr != "" {
		t.Fatalf("bob should succeed: %+v", bobShare)
	}
}

func Test_AITeam_Revenue_PersistsLedgerRow(t *testing.T) {
	ing := newIng(t, nil)
	body, sig := signedBody(t, "test-secret", basePayload())
	_, err := ing.Accept(body, sig)
	if err != nil {
		t.Fatal(err)
	}
	// Verify a file was created and contains a .jsonl row.
	entries, err := os.ReadDir(ing.dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range entries {
		if strings.HasSuffix(f.Name(), ".jsonl") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a .jsonl file in %s, got %d entries", ing.dir, len(entries))
	}
}

func Test_AITeam_Revenue_NilIngesterRejected(t *testing.T) {
	var ing *Ingester
	if _, err := ing.Accept(nil, ""); err == nil {
		t.Fatal("nil ingester should error")
	}
}

func Test_AITeam_Revenue_MissingSecretRejected(t *testing.T) {
	_, err := New(t.TempDir(), Config{Secret: nil}, nil, nil)
	if err == nil {
		t.Fatal("empty secret should error")
	}
}

func Test_AITeam_Revenue_BadJSONRejected(t *testing.T) {
	ing := newIng(t, nil)
	sig := SignFor([]byte("test-secret"), []byte("not json"))
	_, err := ing.Accept([]byte("not json"), sig)
	if err == nil {
		t.Fatal("invalid JSON should error")
	}
}

func Test_AITeam_Revenue_MissingNonceRejected(t *testing.T) {
	ing := newIng(t, nil)
	p := basePayload()
	p.Nonce = ""
	body, sig := signedBody(t, "test-secret", p)
	_, err := ing.Accept(body, sig)
	if err == nil {
		t.Fatal("missing nonce should error")
	}
}
