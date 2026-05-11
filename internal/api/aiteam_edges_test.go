package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// Bug-fix #4 (A1): super-long agent ID must be rejected with 400.
func Test_AITeam_S8_API_LongAgentIDRejected(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	longID := strings.Repeat("a", 4000)
	req := httptest.NewRequest("GET", "/api/aiteam/wallet/"+longID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("4000-char agent_id should 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid agent_id") {
		t.Fatalf("expected validation error, got %s", w.Body.String())
	}
}

// Bug-fix #5 (A2): SQL-injection-looking agent ID rejected.
//
// We test URL-safe encodings of the malicious payloads (real attackers
// would URL-encode anyway). The validator runs after gin parses the
// path param, so it sees the decoded raw bytes.
func Test_AITeam_S8_API_SQLishAgentIDRejected(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	cases := []string{
		"alice%27OR%271",  // alice'OR'1
		"alice%3BDROP",    // alice;DROP
		"alice%20space",   // alice space
		"a%40b.com",       // a@b.com
		"emoji%F0%9F%8E%89", // emoji🎉
	}
	for _, id := range cases {
		req := httptest.NewRequest("GET", "/api/aiteam/wallet/"+id, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("agent_id encoded %q should 400, got %d body=%s",
				id, w.Code, w.Body.String())
		}
	}
}

// Defensive: agent IDs with slashes are routed differently by gin
// (path collision) and produce 404. That's still secure — they never
// reach the wallet store. Document the current behavior.
func Test_AITeam_S8_API_SlashAgentIDFromGinNotMyHandler(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	req := httptest.NewRequest("GET", "/api/aiteam/wallet/alice/etc/passwd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// gin returns 404 not found (path doesn't match any route)
	if w.Code != http.StatusNotFound {
		t.Fatalf("slash-containing path: gin should 404, got %d", w.Code)
	}
}

// Bug-fix #5 (A2): valid agent IDs accepted.
func Test_AITeam_S8_API_ValidAgentIDsAccepted(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	// These all match the regex; pool is nil so we'll get 503 not 400.
	cases := []string{"alice", "bob-2", "agent_x", "a", "MixedCase123"}
	for _, id := range cases {
		req := httptest.NewRequest("GET", "/api/aiteam/wallet/"+id, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("agent_id %q should pass validation, got %d body=%s",
				id, w.Code, w.Body.String())
		}
	}
}

// Bug-fix #6 (A7): FX override with insane rate rejected.
func Test_AITeam_S8_API_FXHugeRateRejected(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	body, _ := json.Marshal(map[string]any{"currency": "CNY", "rate": 1e30})
	req := httptest.NewRequest("POST", "/api/aiteam/fx/override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("huge rate should 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "out of acceptable range") {
		t.Fatalf("expected range error, got %s", w.Body.String())
	}
}

// Bug-fix #6 (A7): FX override with tiny rate rejected.
func Test_AITeam_S8_API_FXTinyRateRejected(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	body, _ := json.Marshal(map[string]any{"currency": "CNY", "rate": 1e-10})
	req := httptest.NewRequest("POST", "/api/aiteam/fx/override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("tiny rate should 400, got %d", w.Code)
	}
}

// Bug-fix #6 (A7): normal rate accepted.
func Test_AITeam_S8_API_FXNormalRateAccepted(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	body, _ := json.Marshal(map[string]any{"currency": "CNY", "rate": 7.2})
	req := httptest.NewRequest("POST", "/api/aiteam/fx/override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// pool is nil → 503; but we're testing the validation passed
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("normal rate should pass validation; got %d body=%s",
			w.Code, w.Body.String())
	}
}

// Bug-fix #6 (A7): zero rate (= clear override) still accepted.
func Test_AITeam_S8_API_FXZeroRateAccepted(t *testing.T) {
	t.Setenv(flags.EnvWallet, "1")
	r := newAITeamRouter(t)
	body, _ := json.Marshal(map[string]any{"currency": "CNY", "rate": 0})
	req := httptest.NewRequest("POST", "/api/aiteam/fx/override", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 0 means "clear override" — should pass validation
	if w.Code == http.StatusBadRequest {
		t.Fatalf("zero rate is the documented 'clear' command, should not 400: %s",
			w.Body.String())
	}
}

// truncForError caps echo of attacker input.
func Test_AITeam_S8_API_TruncForError(t *testing.T) {
	short := truncForError("short")
	if short != "short" {
		t.Errorf("short echo: %q", short)
	}
	long := truncForError(strings.Repeat("X", 200))
	if !strings.HasSuffix(long, "…(truncated)") {
		t.Errorf("long should be truncated: %q", long)
	}
	if len(long) > 100 {
		t.Errorf("truncated length too long: %d", len(long))
	}
}
