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
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
	aiteamFX "github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
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
		svc := fxSvc()
		if svc == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fx not initialised"})
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

	// -- Judge (PR-004, lands S7) -----------------------------------------
	g.POST("/judge/run", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S7"})
	})
	g.GET("/judge/scores/:agentId", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S7"})
	})
	g.POST("/judge/override", func(c *gin.Context) {
		if !flags.JudgeEnabled() {
			notEnabled(c, "judge")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S7"})
	})

	// -- Payroll (PR-002, lands S8) ---------------------------------------
	g.GET("/payroll/:agentId", func(c *gin.Context) {
		if !flags.PayrollEnabled() {
			notEnabled(c, "payroll")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S8"})
	})
	g.POST("/payroll/run", func(c *gin.Context) {
		if !flags.PayrollEnabled() {
			notEnabled(c, "payroll")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S8"})
	})

	// -- Revenue webhook (PR-005, lands S9) -------------------------------
	// NOTE: in real handler, revenue webhook authenticates via HMAC, NOT
	// the bearer token middleware. The stub still requires auth to keep
	// the negative-path 404 deterministic.
	g.POST("/revenue/incoming", func(c *gin.Context) {
		if !flags.RevenueEnabled() {
			notEnabled(c, "revenue")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S9"})
	})

	// -- Dashboard overview (PR-006, lands S10) ---------------------------
	g.GET("/overview", func(c *gin.Context) {
		if !flags.DashboardEnabled() {
			notEnabled(c, "dashboard")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S10"})
	})
	g.GET("/audit", func(c *gin.Context) {
		if !flags.DashboardEnabled() {
			notEnabled(c, "dashboard")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S10"})
	})
}
