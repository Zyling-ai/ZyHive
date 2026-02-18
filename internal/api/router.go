// Package api registers all REST API routes.
// Reference: openclaw/src/gateway/server-*.ts
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

// RegisterRoutes mounts all API handlers onto the Gin engine.
// The agent.Manager is used by agent and chat handlers for real data.
func RegisterRoutes(r *gin.Engine, cfg *config.Config, mgr *agent.Manager) {
	// Serve embedded Vue 3 SPA (TODO: go:embed ui/dist)
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.1.0"})
	})

	v1 := r.Group("/api")
	// Auth middleware (token check)
	v1.Use(authMiddleware(cfg.Auth.Token))

	// ── Agents (AI Employees) ─────────────────────────────────────────────
	agentH := &agentHandler{cfg: cfg, manager: mgr}
	agents := v1.Group("/agents")
	{
		agents.GET("", agentH.List)
		agents.POST("", agentH.Create)
		agents.GET("/:id", agentH.Get)
		agents.PATCH("/:id", agentH.Update)
		agents.DELETE("/:id", agentH.Delete)
		agents.POST("/:id/start", agentH.Start)
		agents.POST("/:id/stop", agentH.Stop)
	}

	// ── Chat (streaming SSE) ──────────────────────────────────────────────
	chatH := &chatHandler{cfg: cfg, manager: mgr}
	agents.POST("/:id/chat", chatH.Chat)
	agents.GET("/:id/sessions", chatH.ListSessions)
	agents.GET("/:id/sessions/:sid", chatH.GetSession)

	// ── Workspace files ───────────────────────────────────────────────────
	fileH := &fileHandler{cfg: cfg}
	agents.GET("/:id/files/*path", fileH.Read)
	agents.PUT("/:id/files/*path", fileH.Write)
	agents.DELETE("/:id/files/*path", fileH.Delete)

	// ── Cron jobs ─────────────────────────────────────────────────────────
	cronH := &cronHandler{cfg: cfg}
	cron := v1.Group("/cron")
	{
		cron.GET("", cronH.List)
		cron.POST("", cronH.Create)
		cron.PATCH("/:jobId", cronH.Update)
		cron.DELETE("/:jobId", cronH.Delete)
		cron.POST("/:jobId/run", cronH.Run)
		cron.GET("/:jobId/runs", cronH.Runs)
	}

	// ── Config ────────────────────────────────────────────────────────────
	cfgH := &configHandler{cfg: cfg}
	v1.GET("/config", cfgH.Get)
	v1.PATCH("/config", cfgH.Patch)
	v1.POST("/config/test-key", cfgH.TestKey)

	// ── Stats & Health ────────────────────────────────────────────────────
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	v1.GET("/stats", statsHandler)

	// ── WebSocket ─────────────────────────────────────────────────────────
	r.GET("/ws", wsHandler)
}

func authMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" || token == "changeme" {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if auth != "Bearer "+token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func statsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "stats not yet implemented"})
}

func wsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "websocket not yet implemented"})
}
