// Package api registers all REST API routes.
// Reference: openclaw/src/gateway/server-*.ts
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

// RegisterRoutes mounts all API handlers onto the Gin engine.
func RegisterRoutes(r *gin.Engine, cfg *config.Config) {
	// Serve embedded Vue 3 SPA (TODO: go:embed ui/dist)
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.1.0"})
	})

	v1 := r.Group("/api")
	// Auth middleware (token check)
	v1.Use(authMiddleware(cfg.Auth.Token))

	// ── Agents (AI Employees) ─────────────────────────────────────────────
	// GET    /api/agents
	// POST   /api/agents
	// GET    /api/agents/:id
	// PATCH  /api/agents/:id
	// DELETE /api/agents/:id
	// POST   /api/agents/:id/start
	// POST   /api/agents/:id/stop
	agentH := &agentHandler{cfg: cfg}
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
	// POST /api/agents/:id/chat
	// GET  /api/agents/:id/sessions
	// GET  /api/agents/:id/sessions/:sid
	chatH := &chatHandler{cfg: cfg}
	agents.POST("/:id/chat", chatH.Chat)
	agents.GET("/:id/sessions", chatH.ListSessions)
	agents.GET("/:id/sessions/:sid", chatH.GetSession)

	// ── Workspace files ───────────────────────────────────────────────────
	// GET    /api/agents/:id/files/*path
	// PUT    /api/agents/:id/files/*path
	// DELETE /api/agents/:id/files/*path
	fileH := &fileHandler{cfg: cfg}
	agents.GET("/:id/files/*path", fileH.Read)
	agents.PUT("/:id/files/*path", fileH.Write)
	agents.DELETE("/:id/files/*path", fileH.Delete)

	// ── Cron jobs ─────────────────────────────────────────────────────────
	// GET    /api/cron
	// POST   /api/cron
	// PATCH  /api/cron/:jobId
	// DELETE /api/cron/:jobId
	// POST   /api/cron/:jobId/run
	// GET    /api/cron/:jobId/runs
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
	// GET    /api/config
	// PATCH  /api/config
	// POST   /api/config/test-key
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
	// WS /ws  (real-time events: agent_status, message_delta, tool_call)
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
	// TODO: return token usage aggregates from SQLite/file
	c.JSON(http.StatusOK, gin.H{"message": "stats not yet implemented"})
}

func wsHandler(c *gin.Context) {
	// TODO: upgrade to WebSocket and register client in hub
	c.JSON(http.StatusOK, gin.H{"message": "websocket not yet implemented"})
}
