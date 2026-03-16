// cmd/aipanel/main.go — entry point for 引巢 · ZyHive (zyling AI 团队操作系统)
// Reference: openclaw/src/main.ts
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
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
	"github.com/Zyling-ai/zyhive/internal/api"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/project"
	"github.com/Zyling-ai/zyhive/pkg/session"
	"github.com/Zyling-ai/zyhive/pkg/subagent"
	"github.com/Zyling-ai/zyhive/pkg/tools"
	"github.com/Zyling-ai/zyhive/pkg/usage"
)

// Version 由 Makefile ldflags 在编译时注入：-X main.Version=v0.9.15
// 未注入时默认显示 "dev"
var Version = "dev"

//go:embed all:ui_dist
var embeddedUI embed.FS

func main() {
	// Parse flags
	defaultCfg := "aipanel.json"
	if env := os.Getenv("AIPANEL_CONFIG"); env != "" {
		defaultCfg = env
	}
	configPath := flag.String("config", defaultCfg, "path to aipanel.json config file")
	serveMode := flag.Bool("serve", false, "直接启动服务（跳过 CLI 菜单）")
	showVersion := flag.Bool("version", false, "打印版本号并退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("ZyHive " + Version)
		os.Exit(0)
	}

	// 无参数 且 无环境变量 → 进入 CLI 管理面板
	// 判断：config 是默认值 且 没有 --serve 且 没有 AIPANEL_CONFIG 环境变量
	configExplicitlySet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configExplicitlySet = true
		}
	})
	if !configExplicitlySet && !*serveMode && os.Getenv("AIPANEL_CONFIG") == "" {
		RunCLI()
		return
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("Warning: config not found at %s, using defaults: %v", *configPath, err)
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
		msg := fmt.Sprintf("[后台任务完成] %s **%s**（任务 ID: %s）\n\n%s", statusIcon, taskLabel, taskID, output)
		content, _ := json.Marshal(msg)
		// Save as "assistant" so it renders on the left side (not as a user bubble)
		_ = store.AppendMessage(spawnedBySession, "assistant", content)
		log.Printf("[subagent] notify: task %s (%s) → session %s", taskID, status, spawnedBySession)
	})

	// ── Cron: isolated session runner ────────────────────────────────────────
	// Each cron job invocation gets its own fresh session ("cron-{jobID}-{runID}"),
	// completely isolated from the main conversation history.
	// This mirrors OpenClaw's sessionTarget="isolated" pattern.
	cronRunFunc := func(ctx context.Context, agentID, model, jobID, runID, message string) (string, error) {
		sessionID := "cron-" + jobID + "-" + runID
		subRun := pool.SubagentRunFunc()
		ch := subRun(ctx, agentID, model, sessionID, "" /*no parent*/, message)
		var sb strings.Builder
		for ev := range ch {
			switch ev.Type {
			case "text_delta":
				sb.WriteString(ev.Text)
			case "error":
				if ev.Error != nil {
					return "", ev.Error
				}
			}
		}
		return sb.String(), nil
	}

	// ── Cron: announce delivery (botPool captured by closure, lazy eval) ──
	// botPool is initialised after cronEngine; using a closure ensures we always
	// reference the live botPool at call time (not at setup time).
	var botPool *channel.BotPool // forward-declared; assigned below
	cronAnnounceFunc := func(agentID, jobName, output string) {
		if botPool == nil {
			return
		}
		bot, _, ok := botPool.GetFirstBot(agentID)
		if !ok {
			return
		}
		header := fmt.Sprintf("📋 **%s**\n\n", jobName)
		_ = bot.ProactiveSend(header + output)
	}

	// runnerFunc: simple blocking runner for Telegram bot & API (runs in shared session).
	// Distinct from cronRunFunc which uses isolated sessions.
	runnerFunc := func(ctx context.Context, agentID, message string) (string, error) {
		return pool.Run(ctx, agentID, message)
	}
	_ = runnerFunc // used below in api.RegisterRoutes

	// Initialize cron engine with isolated runner + announce func
	cronDataDir := "cron"
	cronEngine := cron.NewEngine(cronDataDir, cronRunFunc, cronAnnounceFunc)
	if err := cronEngine.Load(); err != nil {
		log.Printf("Warning: failed to load cron jobs: %v", err)
	} else {
		cronEngine.Start()
		log.Printf("Cron engine started (%d jobs loaded)", len(cronEngine.ListJobs()))
	}

	// Wire cron engine into agent pool so agents can manage cron jobs via tools.
	pool.SetCronEngine(cronEngine)

	// Wire ACP agents (external coding CLIs) from global config.
	if len(cfg.ACPAgents) > 0 {
		pool.SetACPAgents(cfg.ACPAgents)
		log.Printf("ACP agents configured: %d", len(cfg.ACPAgents))
	}

	// Start built-in heartbeats for all agents that have heartbeat.enabled=true.
	pool.StartHeartbeats()
	log.Printf("Heartbeats started")

	// Initialize Telegram bot (if enabled)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// BotPool manages running Telegram bot goroutines — supports hot-add/remove.
	// Assigned here (not `:=`) because botPool is forward-declared above for the cron closure.
	botPool = channel.NewBotPool(ctx)

	// Wire send_message tool: agents (especially those in isolated cron sessions) can call
	// send_message to proactively push notifications to the agent's authorised Telegram users.
	// The closure captures botPool (now assigned) and looks up the live bot at call time.
	pool.SetMessageSenderFn(func(agentID string) tools.MessageSenderFunc {
		return func(ctx context.Context, text string) error {
			bot, _, ok := botPool.GetFirstBot(agentID)
			if !ok {
				return fmt.Errorf("send_message: no active Telegram bot for agent %q", agentID)
			}
			return bot.ProactiveSend(text)
		}
	})

	// startBotForChannel creates and starts a TelegramBot via the pool.
	// Safe to call at any time (API handler uses it when channels are updated).
	startBotForChannel := func(agentID, chID, token string) {
		aID := agentID
		cID := chID
		pdDir := filepath.Join(agentsDir, aID, "channels-pending")
		pending := channel.NewPendingStore(pdDir, cID)
		sf := func(ctx2 context.Context, aid, msg, sessionID string, media []channel.MediaInput, fileSender channel.FileSenderFunc) (<-chan channel.StreamEvent, error) {
			return pool.RunStreamEvents(ctx2, aid, msg, sessionID, media, fileSender)
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

	// Wire pool ↔ worker pool so subagent events can be broadcast to parent SSE subscribers.
	pool.SetWorkerPool(workerPool)

	// Wire agent info function so dispatch panel shows real names and avatar colors.
	subagentMgr.SetAgentInfoFn(func(agentID string) (name, avatarColor string) {
		ag, ok := mgr.Get(agentID)
		if !ok {
			return "", ""
		}
		return ag.Name, ag.AvatarColor
	})

	// Inject build version into API layer
	api.AppVersion = Version

	// Setup router
	r := gin.Default()
	botCtrl := api.BotControl{
		Start: startBotForChannel,
		Stop:  botPool.StopBot,
		Notify: func(ctx context.Context, agentID, channelID string, chatID, threadID int64, prompt string) error {
			var bot *channel.TelegramBot
			var ok bool
			if channelID != "" {
				bot, ok = botPool.GetBot(agentID, channelID)
			} else {
				bot, _, ok = botPool.GetFirstBot(agentID)
			}
			if !ok {
				return fmt.Errorf("no active Telegram bot found for agent %q", agentID)
			}
			return bot.Notify(ctx, chatID, threadID, prompt)
		},
	}
	// Usage store: records are written to {agentsDir}/.usage/YYYY-MM.jsonl
	usageStore := usage.NewStore(agentsDir)
	pool.SetUsageStore(usageStore)

	api.RegisterRoutes(r, cfg, *configPath, mgr, pool, cronEngine, uiFS, runnerFunc, botCtrl, projectMgr, subagentMgr, workerPool, usageStore)

	// Print access URLs
	port := cfg.Gateway.Port
	if port == 0 {
		port = 8080
	}
	addr := fmt.Sprintf(":%d", port)

	// 启动后台模型连通性检测（首次启动 / 升级后状态为 untested 时自动测试）
	go checkDefaultModelOnStartup(cfg, *configPath)

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

		pool.CloseBrowser() // shut down headless browser if running

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

// checkDefaultModelOnStartup 在服务启动后后台自动检测默认模型连通性。
// 若默认模型状态为 "untested"，则发起一次真实请求判断是否可达，
// 并将结果写回配置（"ok" 或 "error"）。
// 这解决了用户升级后从未手动测试、status 永远为 untested、仪表盘警告不触发的问题。
func checkDefaultModelOnStartup(cfg *config.Config, cfgPath string) {
	// 等待服务完全就绪
	time.Sleep(5 * time.Second)

	def := cfg.DefaultModel()
	if def == nil || def.Status != "untested" {
		return // 无模型 或 已测过，跳过
	}

	key := def.APIKey
	if key == "" {
		key = os.Getenv(envVarName(def.Provider))
	}
	if key == "" {
		return // 无 key，无法测试
	}

	log.Printf("[startup-check] 检测默认模型 %s/%s 连通性...", def.Provider, def.Model)

	var ok bool
	var errMsg string

	switch def.Provider {
	case "anthropic":
		ok, errMsg = startupTestAnthropic(key, def.BaseURL)
	default:
		// OpenAI-compatible providers
		baseURL := def.BaseURL
		if baseURL == "" {
			baseURL = startupDefaultBaseURL(def.Provider)
		}
		ok, errMsg = startupTestOpenAICompat(key, baseURL)
	}

	for i := range cfg.Models {
		if cfg.Models[i].ID == def.ID {
			if ok {
				cfg.Models[i].Status = "ok"
				log.Printf("[startup-check] ✅ 默认模型 %s 连通正常", def.ProviderModel())
			} else {
				cfg.Models[i].Status = "error"
				log.Printf("[startup-check] ❌ 默认模型 %s 连接失败: %s", def.ProviderModel(), errMsg)
			}
			break
		}
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		log.Printf("[startup-check] 保存配置失败: %v", err)
	}
}

func envVarName(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	default:
		return ""
	}
}

func startupTestAnthropic(key, baseURL string) (bool, string) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	payload := `{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/messages", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, body)
}

func startupTestOpenAICompat(key, baseURL string) (bool, string) {
	if baseURL == "" {
		return false, "no baseURL"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		strings.TrimSuffix(baseURL, "/")+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return false, fmt.Sprintf("status %d: %s", resp.StatusCode, body)
}

func startupDefaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "moonshot", "kimi":
		return "https://api.moonshot.cn/v1"
	case "zhipu", "glm":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "minimax":
		return "https://api.minimax.chat/v1"
	case "qwen", "dashscope":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}
