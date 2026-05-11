// Package api — aiteam (autonomous-economy experimental subsystem) route stubs.
//
// All routes mount under /api/aiteam/* and are gated by per-subsystem
// experimental flags (see pkg/aiteam/flags). When a subsystem is disabled
// (default), the route returns 404 with body {"error":"not enabled"} so
// clients can detect cleanly without leaking that the route exists.
//
// S0 (this file): just the stubs. Real handlers land in subsequent stages:
//   S4 — guard handlers
//   S5 — wallet + fx handlers
//   S7 — judge handlers
//   S8 — payroll handlers
//   S9 — revenue webhook handler
//   S10 — dashboard overview handler
//
// Design constraints:
//   * When flag is OFF the route MUST behave identically to a non-existent
//     route from the client's perspective (HTTP 404 + JSON body).
//   * The /api/aiteam group is registered after the global auth middleware
//     so authenticated callers only — webhook /api/aiteam/revenue/incoming
//     re-implements its own HMAC auth and is exempt.
//   * Every handler logs a structured event when invoked while ON so
//     audit/debugging is straightforward.
package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	aiteamAudit "github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
	aiteamFX "github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
	aiteamJudge "github.com/Zyling-ai/zyhive/pkg/aiteam/judge"
	aiteamPayroll "github.com/Zyling-ai/zyhive/pkg/aiteam/payroll"
	aiteamRevenue "github.com/Zyling-ai/zyhive/pkg/aiteam/revenue"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/wallet"
)

// notEnabled is the canonical disabled-subsystem 404 response.
//
// Returning 404 (not 403) is intentional: callers should not be able to
// distinguish "feature not built" from "feature disabled" from "this URL
// never existed".
func notEnabled(c *gin.Context, subsystem string) {
	c.JSON(http.StatusNotFound, gin.H{
		"error":     "not enabled",
		"subsystem": subsystem,
	})
}

// BUG-FIX P3-S8 (A1, A2): agent IDs come from URL path params and are
// passed to wallet ledger / guard state / payroll filenames. Without
// validation we accept arbitrarily long IDs, SQL-injection-looking
// IDs, IDs with shell metacharacters etc. They don't currently break
// the system (safefs / no-shell-exec on this path) but they:
//   * pollute the wallet directory with garbage files
//   * confuse the audit log
//   * waste disk space (4 KB ledger × N nonsense IDs)
//
// Real agent IDs in ZyHive are alphanumeric + dash/underscore via the
// pkg/agent.NewID path (see pkg/agent/manager.go). Match that here.
var validAgentIDRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// validateAgentIDParam returns true and writes a 400 response when the
// :agentId path param is malformed. Caller should bail out on false.
func validateAgentIDParam(c *gin.Context) bool {
	id := c.Param("agentId")
	if !validAgentIDRE.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "invalid agent_id",
			"hint":      "must match ^[a-zA-Z0-9_-]{1,64}$",
			"rejected":  truncForError(id),
		})
		return false
	}
	return true
}

// truncForError caps the echo length so error responses can't be used
// to reflect arbitrary attacker-supplied data to logs.
func truncForError(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "…(truncated)"
}

// registerAITeamRoutes mounts every /api/aiteam/* handler onto the auth-
// protected v1 group. Called from RegisterRoutes after the main aipanel
// routes are wired so it can pick up any shared dependencies that get
// added later.
//
// pool may be nil during early bring-up / tests; in that case all
// gated handlers fall back to 501 even with the flag on.
//
// v1 is the authenticated /api group (i.e. r.Group("/api") with auth
// middleware applied).
func registerAITeamRoutes(v1 *gin.RouterGroup, pool *agent.Pool) {
	g := v1.Group("/aiteam")
	walletStore := func() *wallet.Store {
		if pool == nil {
			return nil
		}
		return pool.AITeamWallet()
	}
	fxSvc := func() *aiteamFX.Service {
		if pool == nil {
			return nil
		}
		return pool.AITeamFX()
	}
	judgeMgr := func() *aiteamJudge.Manager {
		if pool == nil {
			return nil
		}
		return pool.AITeamJudge()
	}
	payrollMgr := func() *aiteamPayroll.Manager {
		if pool == nil {
			return nil
		}
		return pool.AITeamPayroll()
	}
	revenueIng := func() *aiteamRevenue.Ingester {
		if pool == nil {
			return nil
		}
		return pool.AITeamRevenue()
	}
	auditLog := func() *aiteamAudit.Log {
		if pool == nil {
			return nil
		}
		return pool.AITeamAudit()
	}

	// -- Flags status (always available, never gated) ---------------------
	// Public to authenticated callers; useful for UI to decide which
	// menus to show. Never reveals secrets — just booleans.
	g.GET("/flags", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"flags": flags.Snapshot(),
			"any":   flags.AnyEnabled(),
		})
	})

	// -- Wallet (PR-001, S5) — REAL handlers -------------------------------
	g.GET("/wallet/:agentId", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		ws := walletStore()
		if ws == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wallet not initialised"})
			return
		}
		agentID := c.Param("agentId")
		bal := ws.Balance(agentID)
		ledger, _ := ws.Ledger(agentID, 20)
		c.JSON(http.StatusOK, gin.H{
			"agentId":         agentID,
			"balance_usdt":    bal.String(),
			"recent_ledger":   ledger,
		})
	})
	g.POST("/wallet/:agentId/credit", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		ws := walletStore()
		if ws == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wallet not initialised"})
			return
		}
		var body struct {
			AmountUSDT string `json:"amount_usdt"`
			Reason     string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		amt, err := decimal.NewFromString(body.AmountUSDT)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount_usdt", "detail": err.Error()})
			return
		}
		e, err := ws.Credit(c.Param("agentId"), amt, body.Reason)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, e)
	})
	g.POST("/wallet/:agentId/transfer", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		ws := walletStore()
		if ws == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wallet not initialised"})
			return
		}
		var body struct {
			To         string `json:"to"`
			AmountUSDT string `json:"amount_usdt"`
			Reason     string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		amt, err := decimal.NewFromString(body.AmountUSDT)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount_usdt", "detail": err.Error()})
			return
		}
		if err := ws.Transfer(c.Param("agentId"), body.To, amt, body.Reason); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"transferred": true, "from": c.Param("agentId"), "to": body.To, "amount_usdt": amt.String()})
	})
	g.GET("/wallet/:agentId/ledger", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		ws := walletStore()
		if ws == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wallet not initialised"})
			return
		}
		entries, err := ws.Ledger(c.Param("agentId"), 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"agentId": c.Param("agentId"), "entries": entries})
	})
	// P3-S3: CSV export for accounting / tax season download. Includes
	// FX snapshot columns when present so historical multi-currency
	// rendering is preserved at row level.
	g.GET("/wallet/:agentId/ledger.csv", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		ws := walletStore()
		if ws == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wallet not initialised"})
			return
		}
		agentID := c.Param("agentId")
		entries, err := ws.Ledger(agentID, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition",
			fmt.Sprintf(`attachment; filename="ledger-%s-%s.csv"`,
				agentID, time.Now().UTC().Format("2006-01-02")))
		w := csv.NewWriter(c.Writer)
		_ = w.Write([]string{
			"timestamp_ms", "iso8601", "type", "amount_usdt",
			"balance_after_usdt", "reason", "counterparty",
		})
		for _, e := range entries {
			isoTime := time.UnixMilli(e.Timestamp).UTC().Format(time.RFC3339)
			_ = w.Write([]string{
				strconv.FormatInt(e.Timestamp, 10),
				isoTime,
				string(e.Type),
				e.AmountUSDT.String(),
				e.BalanceAfterUSDT.String(),
				e.Reason,
				e.Counterparty,
			})
		}
		w.Flush()
	})

	// -- FX / Currency layer (PR-001 § 2.7) — REAL handlers ---------------
	g.GET("/fx/rates", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		svc := fxSvc()
		if svc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fx not initialised"})
			return
		}
		c.JSON(http.StatusOK, svc.SnapshotJSON())
	})
	g.POST("/fx/refresh", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		svc := fxSvc()
		if svc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fx not initialised"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
		defer cancel()
		src := svc.RefreshSync(ctx)
		c.JSON(http.StatusOK, gin.H{"source": src, "snap": svc.SnapshotJSON()})
	})
	g.POST("/fx/override", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		var body struct {
			Currency string  `json:"currency"`
			Rate     float64 `json:"rate"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		// BUG-FIX P3-S8 (A7): clamp rate to sensible range. A
		// rate of 1e30 corrupts the display layer permanently
		// until manual delete; force humans to send something
		// realistic. Real-world FX rates between any two
		// currencies on Earth stay within [1e-6, 1e6].
		// Validation runs BEFORE service-nil check so bad input
		// gets a 400 even on uninitialised setups.
		if body.Rate != 0 && (body.Rate < 1e-6 || body.Rate > 1e6) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "rate out of acceptable range (1e-6 .. 1e6)",
				"hint":       "for realistic FX rates between any two world currencies",
				"rejected":   body.Rate,
			})
			return
		}
		svc := fxSvc()
		if svc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fx not initialised"})
			return
		}
		svc.SetOverride(body.Currency, body.Rate)
		c.JSON(http.StatusOK, gin.H{"currency": body.Currency, "rate": body.Rate})
	})
	g.DELETE("/fx/override/:currency", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		svc := fxSvc()
		if svc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fx not initialised"})
			return
		}
		svc.SetOverride(c.Param("currency"), 0)
		c.JSON(http.StatusOK, gin.H{"cleared": c.Param("currency")})
	})

	// -- BudgetGuard (PR-003, S4) — REAL handlers --------------------------
	g.GET("/guard", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		if pool == nil || pool.AITeamGuard() == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guard not initialised"})
			return
		}
		c.JSON(http.StatusOK, pool.AITeamGuard().SnapshotJSON())
	})
	g.POST("/guard/:agentId/release", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		if pool == nil || pool.AITeamGuard() == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guard not initialised"})
			return
		}
		var body struct {
			Operator string `json:"operator"`
			Reason   string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Operator == "" {
			body.Operator = "api"
		}
		ok := pool.AITeamGuard().Release(c.Param("agentId"), body.Operator, body.Reason)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not panicked or not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"released": true, "agentId": c.Param("agentId")})
	})
	g.PATCH("/guard/:agentId/limit", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		if pool == nil || pool.AITeamGuard() == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guard not initialised"})
			return
		}
		var body struct {
			LimitUSDT string `json:"limit_usdt"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		d, err := decimal.NewFromString(body.LimitUSDT)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit_usdt", "detail": err.Error()})
			return
		}
		pool.AITeamGuard().SetAgentLimit(c.Param("agentId"), d)
		c.JSON(http.StatusOK, gin.H{"updated": true, "agentId": c.Param("agentId"), "limit_usdt": d.String()})
	})

	// -- Judge (PR-004, S7) — REAL handlers --------------------------------
	g.POST("/judge/run", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		jm := judgeMgr()
		if jm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "judge not initialised"})
			return
		}
		var body struct {
			AgentID      string  `json:"agent_id"`
			Period       string  `json:"period"`
			UsageCostUSD float64 `json:"usage_cost_usd"`
			CallCount    int     `json:"call_count"`
			Notes        string  `json:"notes"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		sc, err := jm.RunFor(aiteamJudge.Signals{
			AgentID:      body.AgentID,
			Period:       body.Period,
			UsageCostUSD: body.UsageCostUSD,
			CallCount:    body.CallCount,
			Notes:        body.Notes,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sc)
	})
	g.GET("/judge/scores/:agentId", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		jm := judgeMgr()
		if jm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "judge not initialised"})
			return
		}
		agentID := c.Param("agentId")
		period := c.Query("period")
		// If period is provided → return all rows that period; else
		// return last 30 daily latests.
		if period != "" {
			rows, err := jm.Read(agentID, period)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"agentId": agentID, "period": period, "rows": rows})
			return
		}
		hist, err := jm.History(agentID, 30)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"agentId": agentID, "history": hist, "average_30d": jm.AverageOver(agentID, 30)})
	})
	g.POST("/judge/override", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		jm := judgeMgr()
		if jm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "judge not initialised"})
			return
		}
		var body struct {
			AgentID       string `json:"agent_id"`
			Period        string `json:"period"`
			Operator      string `json:"operator"`
			Rationale     string `json:"rationale"`
			Completion    int    `json:"completion"`
			Quality       int    `json:"quality"`
			Communication int    `json:"communication"`
			Creativity    int    `json:"creativity"`
			Cost          int    `json:"cost"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad json"})
			return
		}
		if body.AgentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id required"})
			return
		}
		if body.Operator == "" {
			body.Operator = "api"
		}
		sc, err := jm.Override(body.AgentID, body.Period, body.Operator, body.Rationale,
			body.Completion, body.Quality, body.Communication, body.Creativity, body.Cost)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sc)
	})
	// Convenience: list all judged agents (for dashboard sidebar)
	g.GET("/judge/agents", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		jm := judgeMgr()
		if jm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "judge not initialised"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"agents": jm.AllAgents()})
	})

	// -- Payroll (PR-002, S8) — REAL handlers ------------------------------
	g.GET("/payroll/:agentId", func(c *gin.Context) {
		if !flags.PayrollEnabled() {
			notEnabled(c, "payroll")
			return
		}
		if !validateAgentIDParam(c) {
			return
		}
		pm := payrollMgr()
		if pm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payroll not initialised"})
			return
		}
		hist, err := pm.History(c.Param("agentId"), 30)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"agentId": c.Param("agentId"), "history": hist})
	})
	g.POST("/payroll/run", func(c *gin.Context) {
		if !flags.PayrollEnabled() {
			notEnabled(c, "payroll")
			return
		}
		pm := payrollMgr()
		if pm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payroll not initialised"})
			return
		}
		var body struct {
			AgentID  string   `json:"agent_id"`  // empty → all
			AgentIDs []string `json:"agent_ids"` // explicit list overrides AgentID
			Period   string   `json:"period"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.AgentID != "" && len(body.AgentIDs) == 0 {
			body.AgentIDs = []string{body.AgentID}
		}
		if len(body.AgentIDs) == 0 && pool != nil {
			for _, a := range pool.Manager().List() {
				// BUG-FIX P3-S8 (A9): system agents (__config__ etc.)
				// should NOT receive payroll. They have no human
				// owner and no real work; paying them just inflates
				// the wallet count meaninglessly.
				if a.System {
					continue
				}
				body.AgentIDs = append(body.AgentIDs, a.ID)
			}
		}
		entries, err := pm.RunForAll(body.AgentIDs, body.Period)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"period": body.Period, "entries": entries})
	})

	// -- Revenue webhook (PR-005, S9) — REAL handler -----------------------
	// In production the market posts to this endpoint with an
	// HMAC-SHA256 signature in the `X-Revenue-Signature` header.
	// Although this lives inside the bearer-auth group, the market is
	// expected to supply BOTH the bearer token AND the HMAC, so the
	// HMAC check is the secondary defence-in-depth layer.
	g.POST("/revenue/incoming", func(c *gin.Context) {
		if !flags.RevenueEnabled() {
			notEnabled(c, "revenue")
			return
		}
		ing := revenueIng()
		if ing == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "revenue not initialised"})
			return
		}
		// Raw body — needed for HMAC over the exact bytes.
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
			return
		}
		sig := c.GetHeader("X-Revenue-Signature")
		if sig == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-Revenue-Signature header"})
			return
		}
		res, accErr := ing.Accept(body, sig)
		if accErr != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(accErr, aiteamRevenue.ErrBadSignature):
				status = http.StatusUnauthorized
			case errors.Is(accErr, aiteamRevenue.ErrStaleTimestamp):
				status = http.StatusGone
			case errors.Is(accErr, aiteamRevenue.ErrReplayedNonce):
				status = http.StatusConflict
			case errors.Is(accErr, aiteamRevenue.ErrInvalidSplit):
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": accErr.Error(), "result": res})
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// -- Dashboard overview (PR-006, S10) — REAL handlers ------------------
	g.GET("/overview", func(c *gin.Context) {
		if !flags.DashboardEnabled() {
			notEnabled(c, "dashboard")
			return
		}
		// Aggregate across every aiteam subsystem in one shot.
		// Each subsystem may be nil (its own flag off); we render
		// what's available.
		out := gin.H{
			"flags": flags.Snapshot(),
			"any":   flags.AnyEnabled(),
		}
		if ws := walletStore(); ws != nil {
			agents := ws.AllAgents()
			balances := make(map[string]string, len(agents))
			var totalBal decimal.Decimal
			for _, id := range agents {
				b := ws.Balance(id)
				balances[id] = b.String()
				totalBal = totalBal.Add(b)
			}
			out["wallet"] = gin.H{
				"total_balance_usdt": totalBal.String(),
				"agents":             balances,
				"count":              len(agents),
			}
		}
		if svc := fxSvc(); svc != nil {
			out["fx"] = svc.SnapshotJSON()
		}
		if pool != nil && pool.AITeamGuard() != nil {
			out["guard"] = pool.AITeamGuard().SnapshotJSON()
		}
		if jm := judgeMgr(); jm != nil {
			ids := jm.AllAgents()
			avgs := make(map[string]float64, len(ids))
			for _, id := range ids {
				avgs[id] = jm.AverageOver(id, 7)
			}
			out["judge"] = gin.H{
				"agents":          ids,
				"avg_7d_by_agent": avgs,
			}
		}
		if pm := payrollMgr(); pm != nil {
			// Best-effort: just show the payroll dir is wired; per-agent
			// history is available via /api/aiteam/payroll/:id.
			out["payroll"] = gin.H{"enabled": true}
		}
		if revenueIng() != nil {
			out["revenue"] = gin.H{"enabled": true}
		}
		c.JSON(http.StatusOK, out)
	})
	g.GET("/audit", func(c *gin.Context) {
		if !flags.DashboardEnabled() {
			notEnabled(c, "dashboard")
			return
		}
		log := auditLog()
		if log == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "audit log not initialised (no aiteam flag enabled)",
			})
			return
		}
		limit := 200
		if l := c.Query("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 5000 {
				limit = n
			}
		}
		entries, err := log.Tail(limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"entries": entries,
			"count":   len(entries),
			"limit":   limit,
		})
	})
}
