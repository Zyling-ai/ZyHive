// Package api registers all REST API routes.
package api

import (
	"bufio"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/channel"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/cron"
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
)

const configFilePath = "aipanel.json"

// BotControl groups the functions needed by the channel handler to manage running bots.
type BotControl struct {
	Start func(agentID, channelID, token string) // start or restart a bot
	Stop  func(agentID, channelID string)         // stop a bot
}

// RegisterRoutes mounts all API handlers onto the Gin engine.
func RegisterRoutes(r *gin.Engine, cfg *config.Config, mgr *agent.Manager, pool *agent.Pool, cronEngine *cron.Engine, uiFS fs.FS, runnerFunc channel.RunnerFunc, botCtrl BotControl, projectMgr *project.Manager) {
	rf := runnerFunc
	r.Use(corsMiddleware())
	r.Use(requestLogger())

	v1 := r.Group("/api")
	v1.Use(authMiddleware(cfg.Auth.Token))

	// Agents
	agentH := &agentHandler{cfg: cfg, manager: mgr, pool: pool, botCtrl: botCtrl}
	agents := v1.Group("/agents")
	{
		agents.GET("", agentH.List)
		agents.POST("", agentH.Create)
		agents.GET("/:id", agentH.Get)
		agents.PATCH("/:id", agentH.Update)
		agents.DELETE("/:id", agentH.Delete)
		agents.POST("/:id/start", agentH.Start)
		agents.POST("/:id/stop", agentH.Stop)
		agents.POST("/:id/message", agentH.Message) // Agent 间通信
	}

	// Per-agent channels (each member has its own bot tokens)
	agChH := &agentChannelHandler{manager: mgr, runnerFunc: rf, botCtrl: botCtrl}
	agents.GET("/:id/channels", agChH.GetChannels)
	agents.PUT("/:id/channels", agChH.SetChannels)
	agents.POST("/:id/channels/check-token", agChH.CheckToken)
	agents.POST("/:id/channels/:chId/test", agChH.TestChannel)
	// Pending users (users who messaged the bot but aren't in allowlist yet)
	agents.GET("/:id/channels/:chId/pending", agChH.ListPending)
	agents.POST("/:id/channels/:chId/pending/:userId/allow", agChH.AllowPending)
	agents.DELETE("/:id/channels/:chId/pending/:userId", agChH.DismissPending)
	// Whitelist management
	agents.DELETE("/:id/channels/:chId/allowed/:userId", agChH.RemoveAllowed)

	// Chat (streaming SSE)
	chatH := &chatHandler{cfg: cfg, manager: mgr, projectMgr: projectMgr}
	agents.POST("/:id/chat", chatH.Chat)
	agents.GET("/:id/sessions", chatH.ListSessions)
	agents.GET("/:id/sessions/:sid", chatH.GetSession)

	// Workspace files
	fileH := &fileHandler{manager: mgr}
	agents.GET("/:id/files/*path", fileH.Read)
	agents.PUT("/:id/files/*path", fileH.Write)
	agents.DELETE("/:id/files/*path", fileH.Delete)

	// Relations (RELATIONS.md per agent + team graph)
	relH := &relationsHandler{manager: mgr}
	agents.GET("/:id/relations", relH.Get)
	agents.PUT("/:id/relations", relH.Put)
	v1.GET("/team/graph", relH.Graph)
	v1.DELETE("/team/relations", relH.ClearAllRelations)
	v1.PUT("/team/relations/edge", relH.PutEdge)
	v1.DELETE("/team/relations/edge", relH.DeleteEdge)

	// Memory tree API
	memH := &memoryHandler{manager: mgr, cronEngine: cronEngine, pool: pool}
	agents.GET("/:id/memory/tree", memH.Tree)
	agents.GET("/:id/memory/file/*path", memH.ReadFile)
	agents.PUT("/:id/memory/file/*path", memH.WriteFile)
	agents.POST("/:id/memory/daily", memH.DailyLog)
	agents.GET("/:id/memory/config", memH.GetConfig)
	agents.PUT("/:id/memory/config", memH.SetConfig)
	agents.POST("/:id/memory/consolidate", memH.ConsolidateNow)
	agents.GET("/:id/memory/run-log", memH.RunLog)

	// ── Global Config Registries ──────────────────────────────────────────

	// Model registry
	modelH := &modelHandler{cfg: cfg, configPath: configFilePath}
	models := v1.Group("/models")
	{
		models.GET("", modelH.List)
		models.POST("", modelH.Create)
		models.PATCH("/:id", modelH.Update)
		models.DELETE("/:id", modelH.Delete)
		models.POST("/:id/test", modelH.Test)
		models.GET("/probe", modelH.FetchModels)   // GET /api/models/probe?baseUrl=...&apiKey=...
		models.GET("/env-keys", modelH.EnvKeys)    // GET /api/models/env-keys — detect system env API keys
	}

	// Channel registry
	channelH := &channelHandler{cfg: cfg, configPath: configFilePath}
	channels := v1.Group("/channels")
	{
		channels.GET("", channelH.List)
		channels.POST("", channelH.Create)
		channels.PATCH("/:id", channelH.Update)
		channels.DELETE("/:id", channelH.Delete)
		channels.POST("/:id/test", channelH.Test)
	}

	// Tool/capability registry
	toolH := &toolHandler{cfg: cfg, configPath: configFilePath}
	toolsGroup := v1.Group("/tools")
	{
		toolsGroup.GET("", toolH.List)
		toolsGroup.POST("", toolH.Create)
		toolsGroup.PATCH("/:id", toolH.Update)
		toolsGroup.DELETE("/:id", toolH.Delete)
		toolsGroup.POST("/:id/test", toolH.Test)
	}

	// Per-agent skill management
	agentSkillH := newAgentSkillHandler(mgr)
	agents.GET("/:id/skills", agentSkillH.List)
	agents.POST("/:id/skills", agentSkillH.Create)
	agents.PATCH("/:id/skills/:skillId", agentSkillH.Update)
	agents.DELETE("/:id/skills/:skillId", agentSkillH.Delete)

	// Conversation logs (permanent audit log — admin-only, agent-blind)
	convH := newConvHandler(mgr, mgr.AgentsDir())
	agents.GET("/:id/conversations", convH.List)
	agents.GET("/:id/conversations/:channelId", convH.Messages)
	// Global conversation view (all agents, all channels)
	v1.GET("/conversations", convH.GlobalList)

	// Skill registry
	skillH := &skillHandler{cfg: cfg, configPath: configFilePath}
	skillsGroup := v1.Group("/skills")
	{
		skillsGroup.GET("", skillH.List)
		skillsGroup.POST("/install", skillH.Install)
		skillsGroup.DELETE("/:id", skillH.Delete)
	}

	// Global Sessions (conversation management across all agents)
	sessH := &globalSessionsHandler{cfg: cfg, manager: mgr}
	globalSess := v1.Group("/sessions")
	{
		globalSess.GET("", sessH.List)
		globalSess.GET("/:agentId/:sid", sessH.Get)
		globalSess.DELETE("/:agentId/:sid", sessH.Delete)
		globalSess.PATCH("/:agentId/:sid", sessH.Patch)
	}

	// Cron jobs
	cronH := &cronHandler{engine: cronEngine}
	cronGroup := v1.Group("/cron")
	{
		cronGroup.GET("", cronH.List)
		cronGroup.POST("", cronH.Create)
		cronGroup.PATCH("/:jobId", cronH.Update)
		cronGroup.DELETE("/:jobId", cronH.Delete)
		cronGroup.POST("/:jobId/run", cronH.Run)
		cronGroup.GET("/:jobId/runs", cronH.Runs)
	}

	// Config (legacy)
	cfgH := &configHandler{cfg: cfg, configPath: configFilePath}
	v1.GET("/config", cfgH.Get)
	v1.PATCH("/config", cfgH.Patch)
	v1.POST("/config/test-key", cfgH.TestKey)

	// ── Public routes (no auth — web channel) ─────────────────────────────
	pubH := &publicChatHandler{manager: mgr, pool: pool}
	pub := r.Group("/pub")
	{
		// Per-channel routes (primary)
		pub.GET("/chat/:agentId/:channelId/info", pubH.Info)
		pub.POST("/chat/:agentId/:channelId/stream", pubH.Stream)
		// Legacy compat (first enabled web channel)
		pub.GET("/chat/:agentId/info", pubH.InfoLegacy)
		pub.POST("/chat/:agentId/stream", pubH.StreamLegacy)
	}

	// ── Projects (shared across all agents) ──────────────────────────────
	projH := &projectHandler{mgr: projectMgr}
	projFileH := &projectFileHandler{mgr: projectMgr}
	projects := v1.Group("/projects")
	{
		projects.GET("", projH.List)
		projects.POST("", projH.Create)
		projects.GET("/:id", projH.Get)
		projects.PATCH("/:id", projH.Update)
		projects.DELETE("/:id", projH.Delete)
		projects.PUT("/:id/permissions", projH.SetPermissions)
		projects.GET("/:id/files/*path", projFileH.Read)
		projects.PUT("/:id/files/*path", projFileH.Write)
		projects.DELETE("/:id/files/*path", projFileH.Delete)
	}

	// Health & Stats
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	statsH := &statsHandler{manager: mgr}
	v1.GET("/stats", statsH.Handle)

	// Logs
	v1.GET("/logs", logsHandler)

	// WebSocket
	r.GET("/ws", wsHandler)

	// ── Serve embedded Vue SPA ────────────────────────────────────────────
	if uiFS != nil {
		fileServer := http.FileServer(http.FS(uiFS))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if !strings.HasPrefix(path, "/api") && !strings.HasPrefix(path, "/ws") {
				f, err := uiFS.Open(strings.TrimPrefix(path, "/"))
				if err == nil {
					f.Close()
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
				c.Request.URL.Path = "/"
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})
	} else {
		r.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.4.0", "message": "AI Company Panel — build UI with: cd ui && npm run build"})
		})
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		log.Printf("[api] %s %s → %d (%s)", c.Request.Method, c.Request.URL.Path, status, latency.Round(time.Millisecond))
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

// statsHandler aggregates stats across all agents and their sessions.
type statsHandler struct {
	manager *agent.Manager
}

func (h *statsHandler) Handle(c *gin.Context) {
	agents := h.manager.List()

	type agentStats struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Sessions int    `json:"sessions"`
		Messages int    `json:"messages"`
		Tokens   int    `json:"tokens"`
	}

	totalSessions := 0
	totalMessages := 0
	totalTokens := 0
	runningCount := 0
	var topAgents []agentStats

	for _, ag := range agents {
		if ag.Status == "running" {
			runningCount++
		}
		store := session.NewStore(ag.SessionDir)
		sessions, err := store.ListSessions()
		if err != nil {
			continue
		}
		msgs := 0
		toks := 0
		for _, s := range sessions {
			msgs += s.MessageCount
			toks += s.TokenEstimate
		}
		totalSessions += len(sessions)
		totalMessages += msgs
		totalTokens += toks
		topAgents = append(topAgents, agentStats{
			ID:       ag.ID,
			Name:     ag.Name,
			Sessions: len(sessions),
			Messages: msgs,
			Tokens:   toks,
		})
	}

	// Sort topAgents by sessions desc
	sort.Slice(topAgents, func(i, j int) bool {
		return topAgents[i].Sessions > topAgents[j].Sessions
	})
	// Keep top 5
	if len(topAgents) > 5 {
		topAgents = topAgents[:5]
	}
	if topAgents == nil {
		topAgents = []agentStats{}
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": gin.H{
			"total":   len(agents),
			"running": runningCount,
		},
		"sessions": gin.H{
			"total":         totalSessions,
			"totalMessages": totalMessages,
			"totalTokens":   totalTokens,
		},
		"topAgents": topAgents,
	})
}

// logsHandler reads /tmp/aipanel.log and returns the last N lines.
// GET /api/logs?limit=200
func logsHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "200")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 2000 {
		limit = 200
	}

	const logPath = "/tmp/aipanel.log"
	f, err := os.Open(logPath)
	if err != nil {
		// Return empty lines if file doesn't exist
		c.JSON(http.StatusOK, gin.H{"lines": []string{}})
		return
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Return last N lines
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	if lines == nil {
		lines = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"lines": lines})
}

func wsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "websocket not yet implemented"})
}
