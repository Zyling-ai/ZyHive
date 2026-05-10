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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
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

// registerAITeamRoutes mounts every /api/aiteam/* stub onto the auth-
// protected v1 group. Called from RegisterRoutes after the main aipanel
// routes are wired so it can pick up any shared dependencies that get
// added later.
//
// v1 is the authenticated /api group (i.e. r.Group("/api") with auth
// middleware applied).
func registerAITeamRoutes(v1 *gin.RouterGroup) {
	g := v1.Group("/aiteam")

	// -- Flags status (always available, never gated) ---------------------
	// Public to authenticated callers; useful for UI to decide which
	// menus to show. Never reveals secrets — just booleans.
	g.GET("/flags", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"flags": flags.Snapshot(),
			"any":   flags.AnyEnabled(),
		})
	})

	// -- Wallet (PR-001, lands S5) ----------------------------------------
	g.GET("/wallet/:agentId", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":      "not implemented yet",
			"subsystem":  "wallet",
			"lands_in":   "S5",
			"agentId":    c.Param("agentId"),
		})
	})
	g.POST("/wallet/:agentId/credit", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})
	g.POST("/wallet/:agentId/transfer", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})
	g.GET("/wallet/:agentId/ledger", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})

	// -- FX / Currency layer (PR-001 § 2.7, lands S5) ---------------------
	// FX runs under wallet flag — they ship together.
	g.GET("/fx/rates", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})
	g.POST("/fx/override", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})
	g.DELETE("/fx/override/:currency", func(c *gin.Context) {
		if !flags.WalletEnabled() {
			notEnabled(c, "wallet")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S5"})
	})

	// -- BudgetGuard (PR-003, lands S4) -----------------------------------
	g.GET("/guard", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S4"})
	})
	g.POST("/guard/:agentId/release", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S4"})
	})
	g.PATCH("/guard/:agentId/limit", func(c *gin.Context) {
		if !flags.BudgetGuardEnabled() {
			notEnabled(c, "budgetguard")
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet", "lands_in": "S4"})
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
