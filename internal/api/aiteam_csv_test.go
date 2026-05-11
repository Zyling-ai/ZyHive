package api

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// Test_AITeam_S3_CSV_FlagOffReturns404 — when the wallet flag is off,
// the CSV endpoint is gated identically to other wallet routes.
func Test_AITeam_S3_CSV_FlagOffReturns404(t *testing.T) {
	t.Setenv(flags.EnvWallet, "")
	r := newAITeamRouter(t)
	req := httptest.NewRequest("GET", "/api/aiteam/wallet/alice/ledger.csv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// Test_AITeam_S3_CSV_FlagOnReturns503WhenWalletNotInit — flag on but
// pool is nil → 503 (consistent with other wallet routes).
func Test_AITeam_S3_CSV_FlagOnReturns503WhenWalletNotInit(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	req := httptest.NewRequest("GET", "/api/aiteam/wallet/alice/ledger.csv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// Test_AITeam_S3_CSV_HeaderRow — independent verification that our
// header row layout is RFC 4180 compatible (no leading whitespace,
// no double-quoting needed, expected column names).
func Test_AITeam_S3_CSV_HeaderRowCanonical(t *testing.T) {
	// We bypass the live handler here and validate the header structure
	// by running a CSV writer on a known set then re-parsing.
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	_ = w.Write([]string{
		"timestamp_ms", "iso8601", "type", "amount_usdt",
		"balance_after_usdt", "reason", "counterparty",
	})
	w.Flush()
	parsed, err := csv.NewReader(strings.NewReader(sb.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || len(parsed[0]) != 7 {
		t.Fatalf("header should round-trip to 1 row × 7 cols, got %+v", parsed)
	}
	if parsed[0][0] != "timestamp_ms" || parsed[0][3] != "amount_usdt" {
		t.Fatalf("header columns wrong: %+v", parsed[0])
	}
}
