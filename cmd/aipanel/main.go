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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunhuihui6688-star/ai-panel/internal/api"
	"github.com/sunhuihui6688-star/ai-panel/pkg/agent"
	"github.com/sunhuihui6688-star/ai-panel/pkg/channel"
	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
	"github.com/sunhuihui6688-star/ai-panel/pkg/cron"
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
	mgr := agent.NewManager(agentsDir)
	if err := mgr.LoadAll(); err != nil {
		log.Printf("Warning: failed to load agents: %v", err)
	}

	// Create default "main" agent on first startup if no agents exist
	if len(mgr.List()) == 0 {
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

	// Start Telegram bots from channel registry
	for _, ch := range cfg.Channels {
		if ch.Type == "telegram" && ch.Enabled && ch.Config["botToken"] != "" {
			defaultAgent := ch.Config["defaultAgent"]
			if defaultAgent == "" {
				defaultAgent = "main"
			}
			bot := channel.NewTelegramBot(
				ch.Config["botToken"],
				defaultAgent,
				nil, // allowedFrom parsed separately if needed
				runnerFunc,
			)
			go bot.Start(ctx)
			log.Printf("Telegram bot started: %s", ch.Name)
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

	// Setup router
	r := gin.Default()
	api.RegisterRoutes(r, cfg, mgr, pool, cronEngine, uiFS)

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
