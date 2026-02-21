// cmd/aipanel/main.go — entry point for 引巢 · ZyHive (zyling AI 团队操作系统)
// Reference: openclaw/src/main.ts
package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/internal/api"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/channel"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"encoding/json"

	"github.com/sunhuihui6688-star/ai-panel/pkg/cron"
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
	"github.com/sunhuihui6688-star/ai-panel/pkg/session"
	"github.com/sunhuihui6688-star/ai-panel/pkg/subagent"
)

//go:embed all:ui_dist
var embeddedUI embed.FS

func main() {
	// Load config
	cfg, err := config.Load("aipanel.json")
	if err != nil {
		log.Printf("Warning: config not found, using defaults: %v", err)
		cfg = config.Default()
	}

	// Initialize agent manager
	agentsDir := cfg.Agents.Dir
	if agentsDir == "" {
		agentsDir = "./agents"
	}
	// Convert to absolute path so Remove(os.RemoveAll) works regardless of CWD changes
	if abs, err := filepath.Abs(agentsDir); err == nil {
		agentsDir = abs
	}
	mgr := agent.NewManager(agentsDir)
	if err := mgr.LoadAll(); err != nil {
		log.Printf("Warning: failed to load agents: %v", err)
	}

	// Initialize project manager (shared workspace for all agents)
	projectsDir := "projects"
	projectMgr := project.NewManager(projectsDir)
	if err := projectMgr.LoadAll(); err != nil {
		log.Printf("Warning: failed to load projects: %v", err)
	}

	// Always ensure the built-in config assistant exists (system agent, cannot be deleted)
	if err := mgr.EnsureSystemConfigAgent(cfg); err != nil {
		log.Printf("Warning: failed to ensure system config agent: %v", err)
	}

	// Create default "main" agent on first startup if no non-system agents exist
	nonSystem := 0
	for _, a := range mgr.List() {
		if !a.System {
			nonSystem++
		}
	}
	if nonSystem == 0 {
		defaultModel := "anthropic/claude-sonnet-4-6"
		defaultModelID := ""
		if m := cfg.DefaultModel(); m != nil {
			defaultModel = m.ProviderModel()
			defaultModelID = m.ID
		}
		if _, err := mgr.CreateWithOpts(agent.CreateOpts{
			ID: "main", Name: "主助手", Model: defaultModel, ModelID: defaultModelID,
		}); err != nil {
			log.Printf("Warning: failed to create default agent: %v", err)
		} else {
			log.Println("Created default agent: main (主助手)")
		}
	}

	// Initialize multi-agent runner pool
	pool := agent.NewPool(cfg, mgr)
	pool.SetProjectManager(projectMgr)

	// Initialize subagent manager — background task execution
	subagentStoreDir := filepath.Join(agentsDir, ".subagent-tasks")
	subagentMgr := subagent.New(pool.SubagentRunFunc(), subagentStoreDir)
	pool.SetSubagentManager(subagentMgr)
	log.Println("Subagent manager initialized")

	// Wire up completion notify: when a background task finishes, inject a message
	// into the parent session so the user sees the result on next open.
	subagentMgr.SetNotify(func(spawnedBy, spawnedBySession, taskID, label, output string, status subagent.TaskStatus) {
		if spawnedBy == "" || spawnedBySession == "" {
			return
		}
		ag, ok := mgr.Get(spawnedBy)
		if !ok {
			return
		}
		store := session.NewStore(ag.SessionDir)
		var statusIcon string
		switch status {
		case subagent.TaskDone:
			statusIcon = "✅"
		case subagent.TaskError:
			statusIcon = "❌"
		case subagent.TaskKilled:
			statusIcon = "🛑"
		default:
			statusIcon = "⚠️"
		}
		taskLabel := label
		if taskLabel == "" {
			taskLabel = taskID
		}
		summary := output
		if len(summary) > 500 {
			summary = summary[:500] + "\n…（截断，完整内容见后台任务）"
		}
		msg := fmt.Sprintf("[后台任务完成] %s **%s**（任务 ID: %s）\n\n%s", statusIcon, taskLabel, taskID, summary)
		content, _ := json.Marshal(msg)
		// Save as "assistant" so it renders on the left side (not as a user bubble)
		_ = store.AppendMessage(spawnedBySession, "assistant", content)
		log.Printf("[subagent] notify: task %s (%s) → session %s", taskID, status, spawnedBySession)
	})

	// Agent runner function — used by cron engine and telegram bot
	runnerFunc := func(ctx context.Context, agentID, message string) (string, error) {
		return pool.Run(ctx, agentID, message)
	}

	// Initialize cron engine
	cronDataDir := "cron"
	cronEngine := cron.NewEngine(cronDataDir, runnerFunc)
	if err := cronEngine.Load(); err != nil {
		log.Printf("Warning: failed to load cron jobs: %v", err)
	} else {
		cronEngine.Start()
		log.Printf("Cron engine started (%d jobs loaded)", len(cronEngine.ListJobs()))
	}

	// Initialize Telegram bot (if enabled)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// BotPool manages running Telegram bot goroutines — supports hot-add/remove.
	botPool := channel.NewBotPool(ctx)

	// startBotForChannel creates and starts a TelegramBot via the pool.
	// Safe to call at any time (API handler uses it when channels are updated).
	startBotForChannel := func(agentID, chID, token string) {
		aID := agentID
		cID := chID
		pdDir := filepath.Join(agentsDir, aID, "channels-pending")
		pending := channel.NewPendingStore(pdDir, cID)
		sf := func(ctx2 context.Context, aid, msg string, media []channel.MediaInput) (<-chan channel.StreamEvent, error) {
			return pool.RunStreamEvents(ctx2, aid, msg, media)
		}
		getAllowFrom := func() []int64 { return mgr.GetAllowFrom(aID, cID) }
		agentDir := filepath.Join(agentsDir, aID)
		bot := channel.NewTelegramBotWithStream(token, aID, agentDir, cID, getAllowFrom, sf, pending)
		// On successful getMe, mark channel status "ok" and save botName
		bot.SetOnConnected(func(botUsername string) {
			mgr.UpdateChannelStatus(aID, cID, "ok", botUsername)
		})
		botPool.StartBot(aID, cID, bot)
	}

	// Start Telegram bots — one per AI member (per-agent channel config)
	for _, ag := range mgr.List() {
		for _, ch := range ag.Channels {
			if ch.Type == "telegram" && ch.Enabled && ch.Config["botToken"] != "" {
				startBotForChannel(ag.ID, ch.ID, ch.Config["botToken"])
			}
		}
	}

	// Try to get embedded UI filesystem
	var uiFS fs.FS
	if sub, err := fs.Sub(embeddedUI, "ui_dist"); err == nil {
		if entries, err := fs.ReadDir(sub, "."); err == nil && len(entries) > 0 {
			uiFS = sub
			log.Println("Serving embedded Vue UI")
		}
	}

	// Initialize session worker pool — decouples runner lifecycle from HTTP connections.
	// Workers run in background goroutines; closing the browser does not stop generation.
	workerPool := session.NewWorkerPool()

	// Setup router
	r := gin.Default()
	botCtrl := api.BotControl{
		Start: startBotForChannel,
		Stop:  botPool.StopBot,
	}
	api.RegisterRoutes(r, cfg, mgr, pool, cronEngine, uiFS, runnerFunc, botCtrl, projectMgr, subagentMgr, workerPool)

	// Print access URLs
	port := cfg.Gateway.Port
	if port == 0 {
		port = 8080
	}
	addr := fmt.Sprintf(":%d", port)

	fmt.Println("")
	fmt.Println("✅ 引巢 · ZyHive 启动成功！")
	fmt.Println("")
	fmt.Printf("  本地访问：  http://localhost:%d\n", port)
	if ip := getLocalIP(); ip != "" {
		fmt.Printf("  内网访问：  http://%s:%d\n", ip, port)
	}
	if pub := getPublicIP(); pub != "" {
		fmt.Printf("  公网访问：  http://%s:%d\n", pub, port)
	}
	fmt.Println("")

	// Graceful shutdown
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel() // stop telegram bot

		workerPool.StopAll() // stop all background session workers

		shutdownCtx := cronEngine.Stop() // stop cron
		<-shutdownCtx.Done()

		srvCtx, srvCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer srvCancel()
		srv.Shutdown(srvCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func getPublicIP() string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return os.Getenv("PUBLIC_IP")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil || resp.StatusCode != 200 {
		return os.Getenv("PUBLIC_IP")
	}
	return string(body)
}
