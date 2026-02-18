// Package api registers all REST API routes.
// Reference: openclaw/src/gateway/server-*.ts
package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

// RegisterRoutes mounts all API handlers onto the Gin engine.
func RegisterRoutes(r *gin.Engine, cfg *config.Config, mgr *agent.Manager, uiFS fs.FS) {
	// ── API routes ────────────────────────────────────────────────────────
	v1 := r.Group("/api")
	v1.Use(authMiddleware(cfg.Auth.Token))

	// Agents
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

	// Chat (streaming SSE)
	chatH := &chatHandler{cfg: cfg, manager: mgr}
	agents.POST("/:id/chat", chatH.Chat)
	agents.GET("/:id/sessions", chatH.ListSessions)
	agents.GET("/:id/sessions/:sid", chatH.GetSession)

	// Workspace files
	fileH := &fileHandler{manager: mgr}
	agents.GET("/:id/files/*path", fileH.Read)
	agents.PUT("/:id/files/*path", fileH.Write)
	agents.DELETE("/:id/files/*path", fileH.Delete)

	// Cron jobs
	cronH := &cronHandler{cfg: cfg, manager: mgr}
	cron := v1.Group("/cron")
	{
		cron.GET("", cronH.List)
		cron.POST("", cronH.Create)
		cron.PATCH("/:jobId", cronH.Update)
		cron.DELETE("/:jobId", cronH.Delete)
		cron.POST("/:jobId/run", cronH.Run)
		cron.GET("/:jobId/runs", cronH.Runs)
	}

	// Config
	cfgH := &configHandler{cfg: cfg, configPath: "aipanel.json"}
	v1.GET("/config", cfgH.Get)
	v1.PATCH("/config", cfgH.Patch)
	v1.POST("/config/test-key", cfgH.TestKey)

	// Health & Stats
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	v1.GET("/stats", statsHandler)

	// WebSocket
	r.GET("/ws", wsHandler)

	// ── Serve embedded Vue SPA ────────────────────────────────────────────
	if uiFS != nil {
		fileServer := http.FileServer(http.FS(uiFS))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			// Try to serve the exact file
			if !strings.HasPrefix(path, "/api") && !strings.HasPrefix(path, "/ws") {
				// Check if file exists in the embedded FS
				f, err := uiFS.Open(strings.TrimPrefix(path, "/"))
				if err == nil {
					f.Close()
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
				// Fallback to index.html for client-side routing
				c.Request.URL.Path = "/"
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})
	} else {
		r.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.2.0", "message": "AI Company Panel — build UI with: cd ui && npm run build"})
		})
	}
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
