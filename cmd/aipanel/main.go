// cmd/aipanel/main.go — entry point for 引巢 · ZyHive (zyling AI 团队操作系统)
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
	"github.com/shopspring/decimal"

	"github.com/Zyling-ai/zyhive/internal/api"
	"github.com/Zyling-ai/zyhive/pkg/agent"
	aiteamAudit "github.com/Zyling-ai/zyhive/pkg/aiteam/audit"
	aiteamBudget "github.com/Zyling-ai/zyhive/pkg/aiteam/budget"
	"github.com/Zyling-ai/zyhive/pkg/aiteam/flags"
	aiteamFXPkg "github.com/Zyling-ai/zyhive/pkg/aiteam/fx"
	aiteamJudgePkg "github.com/Zyling-ai/zyhive/pkg/aiteam/judge"
	aiteamMetricsPkg "github.com/Zyling-ai/zyhive/pkg/aiteam/metrics"
	aiteamPayrollPkg "github.com/Zyling-ai/zyhive/pkg/aiteam/payroll"
	aiteamPromptDef "github.com/Zyling-ai/zyhive/pkg/aiteam/promptdef"
	aiteamRevenuePkg "github.com/Zyling-ai/zyhive/pkg/aiteam/revenue"
	aiteamWalletPkg "github.com/Zyling-ai/zyhive/pkg/aiteam/wallet"
	"github.com/Zyling-ai/zyhive/pkg/budget"
	"github.com/Zyling-ai/zyhive/pkg/channel"
	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/logging"
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
	// ── help/--help/-h 提前拦截 ────────────────────────────────────────────
	// Go flag.Parse() 会把 --help/-h 当作 "帮助请求" 并打印它自己的 Usage，
	// 导致永远走不到后面的 case "help"。这里在 Parse 前先检查，统一用
	// printSubcmdHelp() 给出中文帮助。
	for _, a := range os.Args[1:] {
		if a == "help" || a == "--help" || a == "-help" || a == "-h" {
			printSubcmdHelp()
			os.Exit(0)
		}
	}

	// Parse flags
	defaultCfg := "aipanel.json"
	if env := os.Getenv("AIPANEL_CONFIG"); env != "" {
		defaultCfg = env
	}
	// 自定义 Usage（万一有人用 `zyhive -unknownflag` 触发 flag 包自己的 Usage）
	flag.Usage = printSubcmdHelp
	configPath := flag.String("config", defaultCfg, "path to aipanel.json config file")
	serveMode := flag.Bool("serve", false, "直接启动服务（跳过 CLI 菜单）")
	showVersion := flag.Bool("version", false, "打印版本号并退出")
	flag.Parse()

	// P0-01: structured logging facade. Honours LOG_FORMAT (text|json) and
	// LOG_LEVEL (debug|info|warn|error) env vars. Idempotent — fine even if
	// a subcommand later re-Inits.
	logging.Init(os.Getenv("LOG_FORMAT"), os.Getenv("LOG_LEVEL"))

	if *showVersion {
		fmt.Println("ZyHive " + Version)
		os.Exit(0)
	}

	// ── 子命令处理 ──────────────────────────────────────────────────────────
	// 支持：zyhive token / start / stop / restart / status / version
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Println("ZyHive " + Version)
			os.Exit(0)

		case "token":
			// 打印当前访问令牌（明文）
			cfg := loadConfigForSubcmd(defaultCfg)
			if cfg == nil || cfg.Auth.Token == "" {
				fmt.Fprintln(os.Stderr, "❌ 未找到访问令牌，请先运行 zyhive 进入管理面板配置")
				os.Exit(1)
			}
			fmt.Println(cfg.Auth.Token)
			os.Exit(0)

		case "start", "stop", "restart", "status", "enable", "disable":
			runServiceSubcmd(args[0])
			os.Exit(0)

		case "help", "--help", "-h":
			printSubcmdHelp()
			os.Exit(0)

		default:
			fmt.Fprintf(os.Stderr, "❌ 未知子命令：%s\n\n", args[0])
			printSubcmdHelp()
			os.Exit(1)
		}
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
	subagentMgr.SetNotify(func(spawnedBy, spawnedBySession, taskID, label, output, notifXML string, status subagent.TaskStatus) {
		if spawnedBy == "" || spawnedBySession == "" {
			return
		}
		ag, ok := mgr.Get(spawnedBy)
		if !ok {
			return
		}
		store := session.NewStore(ag.SessionDir)

		// Inject <task-notification> XML as a user-role message into the parent session.
		// This follows the Coordinator pattern: the XML block is delivered as a
		// "user" message so the Coordinator agent sees it on its next turn and can
		// continue or spawn follow-up workers accordingly.
		//
		// Format: <task-notification>...</task-notification> (see subagent/coordinator.go)
		//
		// Legacy fallback: also append a human-readable assistant message so the UI
		// shows the result even if the agent hasn't processed the XML yet.
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

		// 1. Inject XML notification as "user" message (for Coordinator to process)
		if notifXML != "" {
			xmlContent, _ := json.Marshal(notifXML)
			_ = store.AppendMessage(spawnedBySession, "user", xmlContent)
		}

		// 2. Append human-readable summary as "assistant" for UI display
		msg := fmt.Sprintf("[后台任务完成] %s **%s**（任务 ID: %s）\n\n%s", statusIcon, taskLabel, taskID, output)
		content, _ := json.Marshal(msg)
		_ = store.AppendMessage(spawnedBySession, "assistant", content)

		log.Printf("[subagent] notify: task %s (%s) → session %s", taskID, status, spawnedBySession)
	})

	// ── Cron: isolated session runner ────────────────────────────────────────
	// Each cron job invocation gets its own fresh session ("cron-{jobID}-{runID}"),
	// completely isolated from the main conversation history.
	// Isolated session pattern: subagent runs in its own context without inheriting parent history.
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

	// Shared aiteam audit log — created here so the channel-promptdef
	// wrap below can reference it. When all aiteam flags are off this
	// is nil and audit.Append is a no-op.
	var aiteamAuditLog *aiteamAudit.Log
	// P3-S2: shared Prometheus metrics registry, same lifecycle as audit log.
	var aiteamMetricsReg *aiteamMetricsPkg.Registry
	if flags.AnyEnabled() {
		auditDir := filepath.Join(agentsDir, "aiteam")
		var aerr error
		aiteamAuditLog, aerr = aiteamAudit.New(auditDir)
		if aerr != nil {
			log.Printf("[aiteam] audit log init failed: %v", aerr)
		}
		aiteamMetricsReg = aiteamMetricsPkg.New()
		log.Printf("[aiteam] P3-S2 metrics registry initialised (/metrics endpoint live)")
	}

	// runnerFunc: simple blocking runner for Telegram bot & API (runs in shared session).
	// Distinct from cronRunFunc which uses isolated sessions.
	// P2-S2: channel-aware promptdef wrap. When ZYHIVE_EXPERIMENTAL_PROMPTDEF
	// is on, every inbound message from Telegram / Feishu / public chat
	// is wrapped with <untrusted_external_content> before reaching the
	// runner. This protects the agent from injection attacks where an
	// external user might say "ignore your system prompt".
	//
	// We construct the guard once and reuse it for every inbound message.
	// Audit log is wired so jailbreak hits show up in the dashboard.
	channelPromptGuard := aiteamPromptDef.New(aiteamAuditLog)
	runnerFunc := func(ctx context.Context, agentID, message string) (string, error) {
		// Wrap.Wrap is a no-op when the flag is off → byte-identical
		// to today's behaviour by default.
		res := channelPromptGuard.Wrap(message, aiteamPromptDef.SourceChannel, agentID, "")
		return pool.Run(ctx, agentID, res.Wrapped)
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
		sf := func(ctx2 context.Context, aid, msg, sessionID string, media []channel.MediaInput, fileSender channel.FileSenderFunc, extraCtx ...string) (<-chan channel.StreamEvent, error) {
			return pool.RunStreamEvents(ctx2, aid, msg, sessionID, media, fileSender, extraCtx...)
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

	// startFeishuBotForChannel creates and starts a FeishuBot via the pool.
	startFeishuBotForChannel := func(agentID, chID, appID, appSecret string) {
		aID := agentID
		cID := chID
		sf := func(ctx2 context.Context, aid, msg, sessionID string, media []channel.MediaInput, fileSender channel.FileSenderFunc, extraCtx ...string) (<-chan channel.StreamEvent, error) {
			return pool.RunStreamEvents(ctx2, aid, msg, sessionID, media, fileSender, extraCtx...)
		}
		getAllowFrom := func() []string {
			raw := mgr.GetAllowFromStr(aID, cID)
			return raw
		}
		agentDir := filepath.Join(agentsDir, aID)
		pdDir := filepath.Join(agentsDir, aID, "channels-pending")
		pending := channel.NewPendingStoreStr(pdDir, cID)
		bot := channel.NewFeishuBotWithStream(appID, appSecret, aID, agentDir, cID, getAllowFrom, sf, pending)
		bot.SetPanelBaseURL(cfg.Gateway.BaseURL())
		bot.SetOnConnected(func(name string) {
			mgr.UpdateChannelStatus(aID, cID, "ok", name)
		})
		botPool.StartFeishuBot(aID, cID, bot)
	}

	// Start Telegram bots — one per AI member (per-agent channel config)
	for _, ag := range mgr.List() {
		for _, ch := range ag.Channels {
			if ch.Type == "telegram" && ch.Enabled && ch.Config["botToken"] != "" {
				startBotForChannel(ag.ID, ch.ID, ch.Config["botToken"])
			}
			if ch.Type == "feishu" && ch.Enabled && ch.Config["appId"] != "" && ch.Config["appSecret"] != "" {
				startFeishuBotForChannel(ag.ID, ch.ID, ch.Config["appId"], ch.Config["appSecret"])
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

	// Start session reaper for every agent — cleans up stale session files every 24h.
	for _, ag := range mgr.List() {
		reaper := session.NewReaper(session.NewStore(ag.SessionDir))
		reaper.Start(ctx)
	}

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

	// P1-02: Budget store. Disabled by default; reads cfg.Budget. Wired to
	// usageStore via SetBudgetCharger so every recorded LLM call is also
	// charged to the running daily total.
	budgetStore := budget.NewStore(budget.Config{
		Enabled:              cfg.Budget.Enabled,
		GlobalDailyUSD:       cfg.Budget.GlobalDailyUSD,
		DefaultAgentDailyUSD: cfg.Budget.DefaultAgentDailyUSD,
		WarnAtPct:            cfg.Budget.WarnAtPct,
		TZ:                   cfg.Budget.TZ,
	})
	// Two-tier budget tracking:
	//   1. pkg/budget — soft warn + simple hard stop (P1-02, USD float64,
	//      ephemeral, no cooldown). Always on by default.
	//   2. pkg/aiteam/budget — hard panic-stop state machine with
	//      cooldown, per-session ceiling, persistent state (PR-003,
	//      USDT decimal). Gated by ZYHIVE_EXPERIMENTAL_BUDGETGUARD.
	// Both subscribe to the same usage stream below.
	var aiteamGuard *aiteamBudget.Guard
	if flags.BudgetGuardEnabled() {
		guardDir := filepath.Join(agentsDir, "aiteam", "guard")
		var gErr error
		aiteamGuard, gErr = aiteamBudget.New(guardDir, aiteamBudget.Limits{
			TZ:       cfg.Budget.TZ,
			Cooldown: time.Hour,
		}, aiteamAuditLog)
		if gErr != nil {
			log.Printf("[aiteam] guard init failed: %v (guard disabled)", gErr)
			aiteamGuard = nil
		} else {
			log.Printf("[aiteam] PR-003 budget guard ENABLED (state dir: %s)", guardDir)
		}
	}

	// aiteam PR-001 (S5): wallet + FX layer, gated on
	// ZYHIVE_EXPERIMENTAL_WALLET. The wallet automatically charges
	// per-LLM-call cost to the agent's ledger when on. FX is unrelated
	// to billing — it only powers the multi-currency display.
	var aiteamWalletStore *aiteamWalletPkg.Store
	var aiteamFXSvc *aiteamFXPkg.Service
	if flags.WalletEnabled() {
		fxCache := filepath.Join(agentsDir, "aiteam", "fx-cache.json")
		aiteamFXSvc = aiteamFXPkg.New(fxCache)
		aiteamFXSvc.RefreshAsync() // best-effort warm-up

		walletDir := filepath.Join(agentsDir, "aiteam", "wallet")
		var werr error
		aiteamWalletStore, werr = aiteamWalletPkg.New(walletDir, aiteamFXSvc, aiteamAuditLog)
		if werr != nil {
			log.Printf("[aiteam] wallet init failed: %v (wallet disabled)", werr)
			aiteamWalletStore = nil
			aiteamFXSvc = nil
		} else {
			log.Printf("[aiteam] PR-001 wallet ENABLED (ledger dir: %s)", walletDir)
		}
	}

	usageStore.SetBudgetCharger(func(agentID string, costUSD float64) {
		// brake (P1-02) — always wired
		budgetStore.Charge(agentID, costUSD)
		// guard (PR-003) — only when flag on and init succeeded
		if aiteamGuard != nil {
			aiteamGuard.Charge(agentID, "", decimal.NewFromFloat(costUSD))
		}
		// wallet (PR-001) — debit the agent's USDT balance 1:1 with USD
		// cost. Insufficient funds are logged but not fatal — Guard
		// (S4/S6) handles the panic-stop policy; wallet just records.
		if aiteamWalletStore != nil && costUSD > 0 {
			if _, err := aiteamWalletStore.Debit(agentID, decimal.NewFromFloat(costUSD), "llm_call"); err != nil {
				// ErrInsufficientFunds expected during over-spend; keep
				// the ledger consistent and let guard fire.
				if err != aiteamWalletPkg.ErrInsufficientFunds {
					log.Printf("[aiteam] wallet debit failed agent=%s cost=%v err=%v", agentID, costUSD, err)
				}
			}
			// P3-S2: refresh wallet balance gauge after debit.
			if aiteamMetricsReg != nil {
				bal, _ := aiteamWalletStore.Balance(agentID).Float64()
				aiteamMetricsReg.SetGauge(aiteamMetricsPkg.NameWalletBalance,
					map[string]string{"agent_id": agentID}, bal)
			}
		}
	})
	pool.SetBudgetStore(budgetStore)
	pool.SetAITeamGuard(aiteamGuard)
	pool.SetAITeamWallet(aiteamWalletStore)
	pool.SetAITeamFX(aiteamFXSvc)

	// S6: link guard ← wallet so guard.Check() also panics on
	// zero-balance. Only meaningful when both subsystems exist; if
	// only one flag is on, the other is nil and SetWallet(nil) is a
	// safe no-op pattern.
	if aiteamGuard != nil && aiteamWalletStore != nil {
		aiteamGuard.SetWallet(aiteamWalletStore)
		log.Printf("[aiteam] S6 guard×wallet linkage ENABLED (zero balance triggers panic)")
	}

	// P3-S1: wire a panic notification hook to the Guard. When set,
	// every panic transition pushes a formatted message to:
	//   1. journalctl / stderr (always, with [PANIC] prefix for grep)
	//   2. owner Telegram chat (only when ZYHIVE_AITEAM_PANIC_TG_CHAT is
	//      configured AND the panicked agent has an active Telegram bot)
	if aiteamGuard != nil {
		ownerTGChatStr := os.Getenv("ZYHIVE_AITEAM_PANIC_TG_CHAT")
		var ownerTGChat int64
		if ownerTGChatStr != "" {
			fmt.Sscanf(ownerTGChatStr, "%d", &ownerTGChat)
		}
		aiteamGuard.SetNotifyHook(func(agentID, reason, message string) {
			// 1. Always log (operators with monitoring grep this).
			log.Printf("[PANIC] aiteam/budget agent=%s reason=%s\n%s", agentID, reason, message)

			// 2. P3-S2: increment Prometheus panic counter.
			if aiteamMetricsReg != nil {
				aiteamMetricsReg.IncCounter(aiteamMetricsPkg.NameGuardPanic,
					map[string]string{"reason": reason}, 1)
			}

			// 3. Try Telegram if owner chat configured.
			if ownerTGChat != 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				// Try the panicked agent's own bot first, fall back to
				// any active bot in the pool.
				if err := botCtrl.Notify(ctx, agentID, "", ownerTGChat, 0, message); err != nil {
					log.Printf("[PANIC] tg push failed (agent=%s): %v", agentID, err)
				}
			}
		})
		log.Printf("[aiteam] P3-S1 panic notify hook installed (tg_chat=%s)",
			func() string {
				if ownerTGChatStr == "" {
					return "<unset; stderr-only>"
				}
				return ownerTGChatStr
			}())
	}

	// S7 + P3-S0: aiteam Judge — heuristic v0 + LLM-driven v1.
	// When cfg.Aiteam.Judge.Model is non-empty, build an LLMScorer
	// (with HeuristicScorer fallback). Otherwise pure heuristic.
	var aiteamJudgeMgr *aiteamJudgePkg.Manager
	if flags.JudgeEnabled() {
		judgeDir := filepath.Join(agentsDir, "aiteam", "judge")
		var scorer aiteamJudgePkg.Scorer = aiteamJudgePkg.HeuristicScorer{}

		// P3-S0: try to build an LLMScorer if a judge model is configured.
		if cfg.Aiteam.Judge.Model != "" {
			modelEntry := cfg.FindModel(cfg.Aiteam.Judge.Model)
			if modelEntry == nil {
				log.Printf("[aiteam] judge model id %q not found in cfg.Models — heuristic fallback", cfg.Aiteam.Judge.Model)
			} else {
				// Reuse config.ResolveCredentials (same code path as the
				// main /chat handler) so the judge model gets keys + base
				// url from Provider registry / model.apiKey / env var
				// fallback in a consistent way.
				apiKey, baseURL := config.ResolveCredentials(modelEntry, cfg.Providers)
				if apiKey == "" {
					log.Printf("[aiteam] judge model %q has no api key wired — heuristic fallback", cfg.Aiteam.Judge.Model)
				} else {
					llmClient := llm.NewClient(modelEntry.Provider, baseURL)
					timeout := 30 * time.Second
					if cfg.Aiteam.Judge.TimeoutMs > 0 {
						timeout = time.Duration(cfg.Aiteam.Judge.TimeoutMs) * time.Millisecond
					}
					call := aiteamJudgePkg.LLMCallFromClient(
						llmClient,
						modelEntry.Model,
						apiKey,
						cfg.Aiteam.Judge.MaxTokens,
						timeout,
					)
					scorer = aiteamJudgePkg.LLMScorer{
						Call:        call,
						PromptGuard: aiteamPromptDef.New(aiteamAuditLog),
						Fallback:    aiteamJudgePkg.HeuristicScorer{},
					}
					log.Printf("[aiteam] judge LLMScorer ENABLED (model=%s provider=%s)",
						modelEntry.Model, modelEntry.Provider)
				}
			}
		}

		var jErr error
		aiteamJudgeMgr, jErr = aiteamJudgePkg.New(judgeDir, scorer)
		if jErr != nil {
			log.Printf("[aiteam] judge init failed: %v (judge disabled)", jErr)
			aiteamJudgeMgr = nil
		} else {
			log.Printf("[aiteam] PR-004 judge ENABLED (state dir: %s)", judgeDir)
		}
	}
	pool.SetAITeamJudge(aiteamJudgeMgr)

	// S8: aiteam Payroll — daily base + bonus(judge) - cost_offset(usage),
	// credited to wallet. Needs at least the wallet to do anything useful;
	// without it, RunFor marks entries skipped (dry-run mode).
	var aiteamPayrollMgr *aiteamPayrollPkg.Manager
	if flags.PayrollEnabled() {
		payrollDir := filepath.Join(agentsDir, "aiteam", "payroll")

		var walletCredit aiteamPayrollPkg.WalletWriter
		if aiteamWalletStore != nil {
			walletCredit = func(agentID string, amt decimal.Decimal, reason string) error {
				_, err := aiteamWalletStore.Credit(agentID, amt, reason)
				return err
			}
		}
		var usageReader aiteamPayrollPkg.UsageReader = usageStore

		var pErr error
		aiteamPayrollMgr, pErr = aiteamPayrollPkg.New(
			payrollDir,
			aiteamPayrollPkg.DefaultConfig(),
			aiteamJudgeMgr,    // may be nil
			walletCredit,      // may be nil
			usageReader,
			aiteamAuditLog,
		)
		if pErr != nil {
			log.Printf("[aiteam] payroll init failed: %v (payroll disabled)", pErr)
			aiteamPayrollMgr = nil
		} else {
			log.Printf("[aiteam] PR-002 payroll ENABLED (state dir: %s)", payrollDir)
		}
	}
	pool.SetAITeamPayroll(aiteamPayrollMgr)

	// P2-S1: daily payroll cron — when payroll is on, kick off a
	// background goroutine that fires at the configured local time
	// every day. Anti-double-fire via Manager-level period dedupe.
	if aiteamPayrollMgr != nil {
		fireTime := os.Getenv("ZYHIVE_AITEAM_PAYROLL_TIME")
		if fireTime == "" {
			fireTime = "23:30"
		}
		tz := os.Getenv("ZYHIVE_AITEAM_PAYROLL_TZ")
		if tz == "" {
			tz = "Asia/Shanghai"
		}
		cron, cErr := aiteamPayrollPkg.NewCron(aiteamPayrollMgr, aiteamPayrollPkg.CronConfig{
			FireTime: fireTime,
			TZ:       tz,
			AgentLister: func() []string {
				ids := make([]string, 0)
				for _, a := range mgr.List() {
					ids = append(ids, a.ID)
				}
				return ids
			},
		})
		if cErr != nil {
			log.Printf("[aiteam] payroll cron init failed: %v", cErr)
		} else {
			cron.Start(context.Background())
			log.Printf("[aiteam] payroll cron started — next fire: %s",
				cron.NextFireAt().Format(time.RFC3339))
		}
	}

	// S9: aiteam Revenue webhook — accepts signed payouts from upstream
	// task market (e.g. ZyStudio). Requires shared HMAC secret via env
	// `ZYHIVE_AITEAM_REVENUE_SECRET`. Without the secret revenue is
	// silently disabled even if the flag is on.
	var aiteamRevenueIng *aiteamRevenuePkg.Ingester
	if flags.RevenueEnabled() {
		secret := os.Getenv("ZYHIVE_AITEAM_REVENUE_SECRET")
		if secret == "" {
			log.Printf("[aiteam] revenue flag ON but ZYHIVE_AITEAM_REVENUE_SECRET unset; disabling")
		} else {
			revDir := filepath.Join(agentsDir, "aiteam", "revenue")

			var walletCredit aiteamRevenuePkg.WalletCredit
			if aiteamWalletStore != nil {
				walletCredit = func(agentID string, amt decimal.Decimal, reason string) error {
					_, err := aiteamWalletStore.Credit(agentID, amt, reason)
					return err
				}
			}

			var rErr error
			aiteamRevenueIng, rErr = aiteamRevenuePkg.New(revDir, aiteamRevenuePkg.Config{
				Secret:          []byte(secret),
				FreshnessWindow: 5 * time.Minute,
			}, walletCredit, aiteamAuditLog)
			if rErr != nil {
				log.Printf("[aiteam] revenue init failed: %v (revenue disabled)", rErr)
				aiteamRevenueIng = nil
			} else {
				log.Printf("[aiteam] PR-005 revenue webhook ENABLED at /api/aiteam/revenue/incoming (state dir: %s)", revDir)
			}
		}
	}
	pool.SetAITeamRevenue(aiteamRevenueIng)
	pool.SetAITeamAudit(aiteamAuditLog)
	pool.SetAITeamMetrics(aiteamMetricsReg)

	// P1-03: Install the configured LLM throttle (process-global). When
	// kind="" or "fixed" with GlobalMaxInflight=0, behaviour is identical
	// to today (no gating). kind="adaptive" enables AIMD per-provider.
	if t := buildLLMThrottle(cfg.Throttle); t != nil {
		llm.SetGlobalThrottle(t)
	}

	api.RegisterRoutes(r, cfg, *configPath, mgr, pool, cronEngine, uiFS, runnerFunc, botCtrl, projectMgr, subagentMgr, workerPool, usageStore, budgetStore)

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

	// Graceful shutdown.
	//
	// 26.5.10v5 (B004): set timeouts to defeat Slowloris-style attacks where a
	// malicious client opens many TCP connections and sends headers byte-by-byte,
	// holding file descriptors forever.
	//
	// - ReadHeaderTimeout: 10s — kills connections stuck in header read
	// - IdleTimeout:       120s — closes idle keep-alive connections
	// - ReadTimeout / WriteTimeout: NOT set, because SSE streams (chat) need
	//   to stay open for minutes. Per-request body size is bounded by the
	//   bodyLimitMiddleware (B003 fix).
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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

// buildLLMThrottle constructs the configured throttle. Returns nil when
// no enforcement is requested (kind="" or "fixed" with GlobalMaxInflight=0)
// so callers can leave the global slot empty (identical to pre-P1-03 behaviour).
func buildLLMThrottle(cfg config.ThrottleConfig) llm.Throttle {
	switch cfg.Kind {
	case "", "fixed":
		if cfg.GlobalMaxInflight <= 0 {
			return nil
		}
		return llm.NewFixedThrottle(cfg.GlobalMaxInflight)
	case "adaptive":
		def := convertProviderConfig(cfg.Default)
		perProv := map[string]llm.AdaptiveConfig{}
		for k, v := range cfg.Providers {
			perProv[k] = convertProviderConfig(v)
		}
		return llm.NewAdaptiveThrottle(def, perProv)
	default:
		log.Printf("[throttle] unknown kind %q, falling back to no-op", cfg.Kind)
		return nil
	}
}

func convertProviderConfig(p config.ThrottleProviderConfig) llm.AdaptiveConfig {
	out := llm.AdaptiveConfig{
		Min:       p.Min,
		Max:       p.Max,
		Init:      p.Init,
		GrowEvery: p.GrowEvery,
	}
	if p.MaxBackoffMs > 0 {
		out.MaxBackoff = time.Duration(p.MaxBackoffMs) * time.Millisecond
	}
	return out
}
