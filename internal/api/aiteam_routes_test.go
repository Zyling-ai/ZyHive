package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
)

// newAITeamRouter builds a minimal Gin router with auth disabled (empty
// token → dev mode) and only the aiteam routes mounted. Other api
// dependencies (manager, pool, ...) are not needed for stub-level testing.
func newAITeamRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api")
	// no auth in test mode → mimics empty cfg.Auth.Token
	// pool=nil → guard handlers return 503 (init failed) when flag on
	registerAITeamRoutes(v1, nil)
	return r
}

func doAITeam(t *testing.T, r *gin.Engine, method, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v (raw=%s)", err, w.Body.String())
		}
	}
	return w.Code, body
}

func Test_AITeam_Routes_FlagsEndpointAlwaysAvailable(t *testing.T) {
	// /api/aiteam/flags must NEVER 404 — it is the discovery endpoint.
	r := newAITeamRouter(t)
	code, body := doAITeam(t, r, "GET", "/api/aiteam/flags")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if _, ok := body["flags"]; !ok {
		t.Fatalf("response missing flags key: %+v", body)
	}
	if _, ok := body["any"]; !ok {
		t.Fatalf("response missing any key: %+v", body)
	}
}

func Test_AITeam_Routes_FlagOffReturns404(t *testing.T) {
	// With no env flag set, every gated route must return 404.
	t.Setenv(flags.EnvWallet, "")
	t.Setenv(flags.EnvBudgetGuard, "")
	t.Setenv(flags.EnvJudge, "")
	t.Setenv(flags.EnvPayroll, "")
	t.Setenv(flags.EnvRevenue, "")
	t.Setenv(flags.EnvDashboard, "")

	r := newAITeamRouter(t)
	cases := []struct {
		method, path, subsystem string
	}{
		{"GET", "/api/aiteam/wallet/alice", "wallet"},
		{"POST", "/api/aiteam/wallet/alice/credit", "wallet"},
		{"GET", "/api/aiteam/fx/rates", "wallet"},
		{"GET", "/api/aiteam/guard", "budgetguard"},
		{"POST", "/api/aiteam/guard/alice/release", "budgetguard"},
		{"POST", "/api/aiteam/judge/run", "judge"},
		{"GET", "/api/aiteam/payroll/alice", "payroll"},
		{"POST", "/api/aiteam/revenue/incoming", "revenue"},
		{"GET", "/api/aiteam/overview", "dashboard"},
		{"GET", "/api/aiteam/audit", "dashboard"},
	}
	for _, c := range cases {
		code, body := doAITeam(t, r, c.method, c.path)
		if code != http.StatusNotFound {
			t.Fatalf("%s %s: expected 404, got %d body=%+v", c.method, c.path, code, body)
		}
		if body["error"] != "not enabled" {
			t.Fatalf("%s %s: expected error='not enabled', got %+v", c.method, c.path, body)
		}
		if body["subsystem"] != c.subsystem {
			t.Fatalf("%s %s: expected subsystem=%s, got %v", c.method, c.path, c.subsystem, body["subsystem"])
		}
	}
}

func Test_AITeam_Routes_FlagOnReturns501OrSvcUnavailable(t *testing.T) {
	// With the wallet flag on but pool=nil (no wallet store):
	//   - S5-and-prior subsystems with real handlers → 503 (not initialised)
	//   - Subsystems still on stubs → 501 (not implemented yet)
	// Both are accepted as "real handler exists and gated correctly".
	t.Setenv(flags.EnvWallet, "1")
	t.Setenv(flags.EnvJudge, "1")

	r := newAITeamRouter(t)

	// Wallet: handler is real (S5) → 503 because pool/wallet are nil.
	code, body := doAITeam(t, r, "GET", "/api/aiteam/wallet/alice")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (wallet not initialised), got %d body=%+v", code, body)
	}

	// Judge: handler is real (S7) → 503 because pool is nil.
	code, body = doAITeam(t, r, "POST", "/api/aiteam/judge/run")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (judge not initialised), got %d body=%+v", code, body)
	}

	// Payroll: handler is real (S8) → 503 (pool nil).
	t.Setenv(flags.EnvPayroll, "1")
	r = newAITeamRouter(t)
	code, body = doAITeam(t, r, "GET", "/api/aiteam/payroll/alice")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (payroll not initialised), got %d body=%+v", code, body)
	}

	// Revenue: handler is real (S9) → 503 (pool nil → no ingester).
	t.Setenv(flags.EnvRevenue, "1")
	r = newAITeamRouter(t)
	code, body = doAITeam(t, r, "POST", "/api/aiteam/revenue/incoming")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (revenue not initialised), got %d body=%+v", code, body)
	}
}

func Test_AITeam_Routes_PerSubsystemIsolation(t *testing.T) {
	// Flipping wallet ON must NOT enable guard/judge/payroll/...
	t.Setenv(flags.EnvWallet, "1")
	t.Setenv(flags.EnvBudgetGuard, "")
	t.Setenv(flags.EnvJudge, "")

	r := newAITeamRouter(t)

	// wallet is real handler but pool=nil → 503 (not 404)
	code, _ := doAITeam(t, r, "GET", "/api/aiteam/wallet/alice")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("wallet should be 503 (flag on, pool nil), got %d", code)
	}
	// guard should still be 404 (flag off)
	code, body := doAITeam(t, r, "GET", "/api/aiteam/guard")
	if code != http.StatusNotFound {
		t.Fatalf("guard should be 404 (flag off), got %d body=%+v", code, body)
	}
	if body["subsystem"] != "budgetguard" {
		t.Fatalf("subsystem mismatch: %+v", body)
	}
}
