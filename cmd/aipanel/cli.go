// cmd/aipanel/cli.go — 引巢 · ZyHive 命令行管理面板（类宝塔风格）
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sunhuihui6688-star/ai-panel/pkg/config"
)

// ── ANSI 颜色 ──────────────────────────────────────────────────────────────
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"
	ansiBgBlue  = "\033[44m"
)

var cliReader = bufio.NewReader(os.Stdin)

// RunCLI 是整个管理面板的入口
func RunCLI() {
	for {
		clearScreen()
		printHeader()
		printStatus()
		printMainMenu()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			menuSystemInfo()
		case "2":
			menuServiceManage()
		case "3":
			menuConfigManage()
		case "4":
			menuAgentManage()
		case "5":
			menuLogs()
		case "6":
			menuUpdate()
		case "7":
			menuNginx()
		case "8":
			menuSSL()
		case "9":
			menuBackup()
		case "0", "q", "Q", "quit", "exit":
			fmt.Println(ansiGreen + "\n  再见！" + ansiReset)
			os.Exit(0)
		default:
			printWarn("无效选项，请重新输入")
			pause()
		}
	}
}

// ── 打印工具 ───────────────────────────────────────────────────────────────
func clearScreen() {
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "cls").Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

func printHeader() {
	w := 52
	line := strings.Repeat("═", w)
	fmt.Printf(ansiBold+ansiBlue+"╔%s╗\n", line)
	fmt.Printf("║%s║\n", center("  引巢 · ZyHive  AI 团队操作系统", w))
	fmt.Printf("║%s║\n", center("  https://github.com/Zyling-ai/zyhive", w))
	fmt.Printf("╚%s╝\n"+ansiReset, line)
}

func printStatus() {
	running := isServiceRunning()
	status := ansiGreen + "● 运行中" + ansiReset
	if !running {
		status = ansiRed + "● 已停止" + ansiReset
	}

	cfg := loadConfigQuiet()
	port := 8080
	if cfg != nil {
		port = cfg.Gateway.Port
	}
	token := "(未配置)"
	if cfg != nil && cfg.Auth.Token != "" {
		t := cfg.Auth.Token
		if len(t) > 8 {
			token = t[:4] + strings.Repeat("*", len(t)-8) + t[len(t)-4:]
		} else {
			token = strings.Repeat("*", len(t))
		}
	}

	fmt.Printf("\n  状态: %s   端口: %s%d%s   访问令牌: %s%s%s\n\n",
		status,
		ansiBold, port, ansiReset,
		ansiYellow, token, ansiReset,
	)
}

func printMainMenu() {
	items := []string{
		"1", "系统状态",
		"2", "服务管理（启动 / 停止 / 重启）",
		"3", "配置管理（访问令牌 / 端口 / 绑定模式）",
		"4", "成员管理（查看 / 重置 AI 成员）",
		"5", "日志查看",
		"6", "在线更新",
		"7", "Nginx 管理",
		"8", "SSL 证书管理",
		"9", "备份与恢复",
		"0", "退出",
	}
	fmt.Println(ansiBold + "  ┌─ 操作菜单 " + strings.Repeat("─", 40) + "┐" + ansiReset)
	for i := 0; i < len(items); i += 2 {
		key := items[i]
		label := items[i+1]
		fmt.Printf("  │  %s[%s]%s  %s\n", ansiCyan+ansiBold, key, ansiReset, label)
	}
	fmt.Println(ansiBold + "  └" + strings.Repeat("─", 51) + "┘" + ansiReset)
	fmt.Println()
}

// ── 1. 系统状态 ────────────────────────────────────────────────────────────
func menuSystemInfo() {
	clearScreen()
	printSectionTitle("访问入口 & 系统状态")

	cfg := loadConfigQuiet()
	configPath := findConfigPath()

	// ── 访问入口（最重要，放最顶部）────────────────────────────────────────
	port := 8080
	if cfg != nil {
		port = cfg.Gateway.Port
	}

	// 获取所有 IP
	localIP := strings.TrimSpace(runCmd("hostname", "-I"))
	if localIP == "" {
		localIP = strings.TrimSpace(runCmd("hostname", "-i"))
	}
	firstLocalIP := strings.Fields(localIP)
	lanIP := ""
	if len(firstLocalIP) > 0 {
		lanIP = firstLocalIP[0]
	}
	publicIP := strings.TrimSpace(runCmd("curl", "-fsSL", "--max-time", "4", "https://api.ipify.org"))

	// Token（脱敏）
	token := "(未配置)"
	tokenFull := ""
	if cfg != nil && cfg.Auth.Token != "" {
		tokenFull = cfg.Auth.Token
		t := cfg.Auth.Token
		if len(t) > 8 {
			token = t[:4] + strings.Repeat("*", len(t)-8) + t[len(t)-4:]
		} else {
			token = strings.Repeat("*", len(t))
		}
	}

	w := 52
	line := strings.Repeat("─", w)
	fmt.Printf(ansiBold+ansiGreen+"  ┌─ 访问入口 %s┐\n"+ansiReset, strings.Repeat("─", w-7))

	if publicIP != "" {
		fmt.Printf(ansiBold+ansiGreen+"  │  🌐 公网：  http://%s:%d\n"+ansiReset, publicIP, port)
	}
	if lanIP != "" {
		fmt.Printf(ansiBold+ansiBlue+"  │  🏠 内网：  http://%s:%d\n"+ansiReset, lanIP, port)
	}
	fmt.Printf(ansiBold+"  │  💻 本机：  http://localhost:%d\n"+ansiReset, port)

	// Nginx 域名
	domain := detectDomain()
	if domain != "" {
		fmt.Printf(ansiBold+ansiMagenta+"  │  🔒 域名：  https://%s\n"+ansiReset, domain)
	}

	fmt.Printf(ansiBold+"  │\n"+ansiReset)
	fmt.Printf(ansiBold+ansiYellow+"  │  🔑 访问令牌： %s\n"+ansiReset, token)
	if tokenFull != "" {
		fmt.Printf(ansiYellow+"  │  （完整令牌）%s\n"+ansiReset, tokenFull)
	}
	fmt.Printf(ansiBold+"  └%s┘\n\n"+ansiReset, line)

	// ── 服务状态 ────────────────────────────────────────────────────────────
	running := isServiceRunning()
	status := ansiGreen + "● 运行中" + ansiReset
	if !running {
		status = ansiRed + "● 已停止" + ansiReset
	}

	printKV("服务状态", status)
	if cfg != nil {
		printKV("监听端口", fmt.Sprintf("%d", cfg.Gateway.Port))
		printKV("绑定模式", cfg.Gateway.Bind)
	}
	printKV("配置文件", configPath)
	agentsDir := "/var/lib/zyhive/agents"
	if cfg != nil && cfg.Agents.Dir != "" {
		agentsDir = cfg.Agents.Dir
	}
	printKV("成员目录", agentsDir)
	binaryPath, _ := os.Executable()
	printKV("二进制路径", binaryPath)

	// ── 系统信息 ────────────────────────────────────────────────────────────
	fmt.Println()
	hostname := strings.TrimSpace(runCmd("hostname"))
	uptime := strings.TrimSpace(runCmd("uptime", "-p"))
	cpuCores := strings.TrimSpace(runCmd("nproc"))
	printKV("主机名", hostname)
	printKV("运行时长", uptime)
	printKV("CPU 核心", cpuCores)

	fmt.Println()
	fmt.Println(ansiBold + "  内存信息：" + ansiReset)
	fmt.Print(ansiCyan + runCmd("free", "-h") + ansiReset)
	fmt.Println(ansiBold + "  磁盘信息：" + ansiReset)
	fmt.Print(ansiCyan + runCmd("df", "-h", "/") + ansiReset)

	pause()
}

// detectDomain 从 Nginx conf 中提取 server_name
func detectDomain() string {
	confDirs := []string{
		"/etc/nginx/conf.d/",
		"/etc/nginx/sites-enabled/",
	}
	for _, dir := range confDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.Contains(e.Name(), "zyhive") || strings.Contains(e.Name(), "aipanel") {
				data, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					continue
				}
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "server_name") {
						parts := strings.Fields(line)
						if len(parts) >= 2 {
							domain := strings.TrimRight(parts[1], ";")
							if domain != "_" && domain != "localhost" {
								return domain
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// ── 2. 服务管理 ────────────────────────────────────────────────────────────
func menuServiceManage() {
	for {
		clearScreen()
		printSectionTitle("服务管理")

		running := isServiceRunning()
		if running {
			fmt.Println("  服务状态：" + ansiGreen + "● 运行中" + ansiReset)
		} else {
			fmt.Println("  服务状态：" + ansiRed + "● 已停止" + ansiReset)
		}
		fmt.Println()
		printMenuItem("1", "启动服务")
		printMenuItem("2", "停止服务")
		printMenuItem("3", "重启服务")
		printMenuItem("4", "查看服务状态")
		printMenuItem("5", "设置开机自启")
		printMenuItem("6", "取消开机自启")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			systemctlAction("start", "zyhive")
		case "2":
			if confirm("确认停止 ZyHive 服务？") {
				systemctlAction("stop", "zyhive")
			}
		case "3":
			systemctlAction("restart", "zyhive")
		case "4":
			out := runCmd("systemctl", "status", "zyhive", "--no-pager", "-l")
			fmt.Println(ansiCyan + out + ansiReset)
			pause()
		case "5":
			systemctlAction("enable", "zyhive")
		case "6":
			systemctlAction("disable", "zyhive")
		case "0", "q":
			return
		}
	}
}

// ── 3. 配置管理 ────────────────────────────────────────────────────────────
func menuConfigManage() {
	for {
		clearScreen()
		printSectionTitle("配置管理")

		cfg := loadConfigQuiet()
		configPath := findConfigPath()

		fmt.Printf("  配置文件：%s%s%s\n\n", ansiYellow, configPath, ansiReset)
		if cfg != nil {
			printKV("监听端口", fmt.Sprintf("%d", cfg.Gateway.Port))
			printKV("绑定模式", cfg.Gateway.Bind)
			printKV("成员目录", cfg.Agents.Dir)
			printKV("Auth Mode", cfg.Auth.Mode)
			if cfg.Auth.Token != "" {
				t := cfg.Auth.Token
				masked := t
				if len(t) > 8 {
					masked = t[:4] + strings.Repeat("*", len(t)-8) + t[len(t)-4:]
				}
				printKV("访问令牌", masked)
			}
			fmt.Printf("  已配置模型: %s%d 个%s\n", ansiBold, len(cfg.Models), ansiReset)
		}

		fmt.Println()
		printMenuItem("1", "查看完整配置（明文）")
		printMenuItem("2", "修改访问令牌")
		printMenuItem("3", "修改监听端口")
		printMenuItem("4", "修改绑定模式（lan / localhost / all）")
		printMenuItem("5", "修改成员目录")
		printMenuItem("6", "用编辑器编辑原始 JSON")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			data, err := os.ReadFile(configPath)
			if err != nil {
				printError("读取配置失败：" + err.Error())
			} else {
				fmt.Println(ansiCyan + string(data) + ansiReset)
			}
			pause()
		case "2":
			newToken := readInput("输入新访问令牌（直接回车生成随机令牌）")
			newToken = strings.TrimSpace(newToken)
			if newToken == "" {
				newToken = generateToken()
				fmt.Printf("  已生成随机访问令牌：%s%s%s\n", ansiGreen+ansiBold, newToken, ansiReset)
			}
			if cfg != nil && patchConfig(configPath, cfg, func(c *config.Config) { c.Auth.Token = newToken }) {
				printSuccess("访问令牌已更新，需要重启服务生效")
				if confirm("立即重启服务？") {
					systemctlAction("restart", "zyhive")
				}
			}
		case "3":
			portStr := readInput("输入新端口号（当前：" + fmt.Sprintf("%d", func() int {
				if cfg != nil {
					return cfg.Gateway.Port
				}
				return 8080
			}()) + "）")
			port, err := strconv.Atoi(strings.TrimSpace(portStr))
			if err != nil || port < 1 || port > 65535 {
				printError("端口号无效")
			} else if cfg != nil && patchConfig(configPath, cfg, func(c *config.Config) { c.Gateway.Port = port }) {
				printSuccess(fmt.Sprintf("端口已修改为 %d，需要重启生效", port))
				if confirm("立即重启服务？") {
					systemctlAction("restart", "zyhive")
				}
			}
		case "4":
			fmt.Println("  可选模式：")
			fmt.Println("    lan       — 仅局域网（默认）")
			fmt.Println("    localhost — 仅本机（配合 Nginx 使用）")
			fmt.Println("    all       — 监听所有接口（公网直接访问）")
			bindMode := readInput("输入绑定模式")
			bindMode = strings.TrimSpace(bindMode)
			if bindMode != "lan" && bindMode != "localhost" && bindMode != "all" {
				printError("无效的绑定模式")
			} else if cfg != nil && patchConfig(configPath, cfg, func(c *config.Config) { c.Gateway.Bind = bindMode }) {
				printSuccess("绑定模式已修改，需要重启生效")
				if confirm("立即重启服务？") {
					systemctlAction("restart", "zyhive")
				}
			}
		case "5":
			newDir := readInput("输入新的成员目录路径")
			newDir = strings.TrimSpace(newDir)
			if newDir == "" {
				printError("路径不能为空")
			} else {
				if err := os.MkdirAll(newDir, 0755); err != nil {
					printError("创建目录失败：" + err.Error())
				} else if cfg != nil && patchConfig(configPath, cfg, func(c *config.Config) { c.Agents.Dir = newDir }) {
					printSuccess("成员目录已修改，需要重启生效")
				}
			}
		case "6":
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			cmd := exec.Command(editor, configPath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		case "0", "q":
			return
		}
		if choice != "0" && choice != "q" {
			pause()
		}
	}
}

// ── 4. 成员管理 ────────────────────────────────────────────────────────────
func menuAgentManage() {
	for {
		clearScreen()
		printSectionTitle("成员管理")

		cfg := loadConfigQuiet()
		agentsDir := "/var/lib/zyhive/agents"
		if cfg != nil && cfg.Agents.Dir != "" {
			agentsDir = cfg.Agents.Dir
		}

		printKV("成员目录", agentsDir)

		// 列出成员
		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			printError("无法读取成员目录：" + err.Error())
		} else {
			fmt.Printf("\n  %s已有 %d 个 AI 成员：%s\n", ansiBold, len(entries), ansiReset)
			for i, e := range entries {
				if e.IsDir() {
					// 读取 identity.json 获取名称
					name := e.Name()
					idPath := filepath.Join(agentsDir, e.Name(), "identity.json")
					if data, err := os.ReadFile(idPath); err == nil {
						var id struct {
							Name string `json:"name"`
						}
						if json.Unmarshal(data, &id) == nil && id.Name != "" {
							name = id.Name + " (" + e.Name() + ")"
						}
					}
					fmt.Printf("    %s%d.%s %s\n", ansiCyan, i+1, ansiReset, name)
				}
			}
		}

		fmt.Println()
		printMenuItem("1", "查看成员详情")
		printMenuItem("2", "删除成员数据（危险）")
		printMenuItem("3", "打开成员工作目录")
		printMenuItem("4", "查看成员记忆文件")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			agentID := readInput("输入成员 ID（目录名）")
			agentID = strings.TrimSpace(agentID)
			agentDir := filepath.Join(agentsDir, agentID)
			if _, err := os.Stat(agentDir); os.IsNotExist(err) {
				printError("成员不存在：" + agentID)
			} else {
				out := runCmd("ls", "-la", agentDir)
				fmt.Println(ansiCyan + out + ansiReset)
				idPath := filepath.Join(agentDir, "identity.json")
				if data, err := os.ReadFile(idPath); err == nil {
					fmt.Println(ansiBold + "\n  identity.json：" + ansiReset)
					fmt.Println(ansiCyan + string(data) + ansiReset)
				}
			}
			pause()
		case "2":
			agentID := readInput("输入要删除的成员 ID")
			agentID = strings.TrimSpace(agentID)
			if agentID == "__config__" || agentID == "main" {
				printError("系统成员 " + agentID + " 不可删除")
			} else if confirm("确认删除成员 " + agentID + " 的所有数据？此操作不可恢复！") {
				agentDir := filepath.Join(agentsDir, agentID)
				if err := os.RemoveAll(agentDir); err != nil {
					printError("删除失败：" + err.Error())
				} else {
					printSuccess("成员 " + agentID + " 数据已删除")
				}
			}
			pause()
		case "3":
			agentID := readInput("输入成员 ID")
			agentID = strings.TrimSpace(agentID)
			agentDir := filepath.Join(agentsDir, agentID, "workspace")
			out := runCmd("ls", "-la", agentDir)
			fmt.Println(ansiCyan + out + ansiReset)
			pause()
		case "4":
			agentID := readInput("输入成员 ID")
			agentID = strings.TrimSpace(agentID)
			memDir := filepath.Join(agentsDir, agentID, "workspace", "memory")
			out := runCmd("ls", "-la", memDir)
			fmt.Println(ansiCyan + out + ansiReset)
			pause()
		case "0", "q":
			return
		}
	}
}

// ── 5. 日志查看 ────────────────────────────────────────────────────────────
func menuLogs() {
	for {
		clearScreen()
		printSectionTitle("日志查看")

		printMenuItem("1", "实时查看服务日志（tail -f，Ctrl+C 退出）")
		printMenuItem("2", "查看最近 100 行日志")
		printMenuItem("3", "查看今日日志")
		printMenuItem("4", "搜索日志关键词")
		printMenuItem("5", "查看 Nginx 错误日志")
		printMenuItem("6", "查看 Nginx 访问日志")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			fmt.Println(ansiYellow + "  按 Ctrl+C 退出日志查看" + ansiReset)
			runInteractive("journalctl", "-u", "zyhive", "-f", "--no-pager")
		case "2":
			out := runCmd("journalctl", "-u", "zyhive", "-n", "100", "--no-pager")
			paginate(out)
		case "3":
			out := runCmd("journalctl", "-u", "zyhive", "--since", "today", "--no-pager")
			paginate(out)
		case "4":
			keyword := readInput("输入搜索关键词")
			out := runCmd("journalctl", "-u", "zyhive", "--no-pager", "-n", "500")
			lines := strings.Split(out, "\n")
			var matched []string
			for _, l := range lines {
				if strings.Contains(strings.ToLower(l), strings.ToLower(keyword)) {
					matched = append(matched, l)
				}
			}
			if len(matched) == 0 {
				printWarn("未找到匹配 [" + keyword + "] 的日志")
			} else {
				paginate(strings.Join(matched, "\n"))
			}
		case "5":
			out := runCmd("tail", "-n", "100", "/var/log/nginx/error.log")
			paginate(out)
		case "6":
			out := runCmd("tail", "-n", "100", "/var/log/nginx/access.log")
			paginate(out)
		case "0", "q":
			return
		}
	}
}

// ── 6. 在线更新 ────────────────────────────────────────────────────────────
func menuUpdate() {
	clearScreen()
	printSectionTitle("在线更新")

	fmt.Println("  当前安装的版本信息：")
	binaryPath, _ := os.Executable()
	printKV("二进制路径", binaryPath)

	fmt.Println()
	fmt.Println("  检查最新版本...")

	latest := fetchLatestVersion()
	if latest == "" {
		printError("无法获取最新版本，请检查网络")
		pause()
		return
	}
	printKV("最新版本", ansiGreen+latest+ansiReset)

	fmt.Println()
	if !confirm("确认更新到 " + latest + "？服务将短暂中断") {
		return
	}

	// 确定下载 URL
	osName := "linux"
	arch := "amd64"
	switch runtime.GOARCH {
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm"
	}
	url := fmt.Sprintf("https://github.com/Zyling-ai/zyhive/releases/download/%s/aipanel-%s-%s",
		latest, osName, arch)

	fmt.Printf("  下载 %s...\n", url)

	tmpPath := "/tmp/zyhive-update"
	out := runCmd("curl", "-fsSL", "--progress-bar", url, "-o", tmpPath)
	if out != "" {
		fmt.Println(out)
	}

	if _, err := os.Stat(tmpPath); os.IsNotExist(err) {
		printError("下载失败")
		pause()
		return
	}

	// 赋权
	os.Chmod(tmpPath, 0755)

	// 停服 → 替换 → 启服
	fmt.Println("  停止服务...")
	runCmd("systemctl", "stop", "zyhive")

	fmt.Println("  替换二进制...")
	runCmd("cp", tmpPath, binaryPath)
	os.Remove(tmpPath)

	fmt.Println("  启动服务...")
	runCmd("systemctl", "start", "zyhive")
	time.Sleep(2 * time.Second)

	if isServiceRunning() {
		printSuccess("更新成功！服务已重启")
	} else {
		printError("服务启动失败，请检查日志：journalctl -u zyhive -n 50")
	}
	pause()
}

// ── 7. Nginx 管理 ──────────────────────────────────────────────────────────
func menuNginx() {
	if !commandExists("nginx") {
		printWarn("Nginx 未安装，安装命令：apt-get install nginx 或 yum install nginx")
		pause()
		return
	}

	for {
		clearScreen()
		printSectionTitle("Nginx 管理")

		nginxStatus := runCmd("systemctl", "is-active", "nginx")
		status := ansiRed + "● 未运行" + ansiReset
		if strings.TrimSpace(nginxStatus) == "active" {
			status = ansiGreen + "● 运行中" + ansiReset
		}
		fmt.Println("  Nginx 状态：" + status)
		fmt.Println()

		printMenuItem("1", "启动 Nginx")
		printMenuItem("2", "停止 Nginx")
		printMenuItem("3", "重载配置（reload）")
		printMenuItem("4", "测试配置语法")
		printMenuItem("5", "查看当前配置")
		printMenuItem("6", "查看所有站点配置")
		printMenuItem("7", "添加反向代理配置")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			systemctlAction("start", "nginx")
		case "2":
			if confirm("确认停止 Nginx？") {
				systemctlAction("stop", "nginx")
			}
		case "3":
			out := runCmd("nginx", "-t")
			fmt.Println(ansiCyan + out + ansiReset)
			if strings.Contains(out, "successful") {
				runCmd("systemctl", "reload", "nginx")
				printSuccess("Nginx 配置已重载")
			} else {
				printError("配置有误，请先修复")
			}
			pause()
		case "4":
			out := runCmd("nginx", "-t")
			fmt.Println(ansiCyan + out + ansiReset)
			pause()
		case "5":
			// 检测 conf.d 还是 sites-enabled
			confPath := "/etc/nginx/conf.d/zyhive.conf"
			if _, err := os.Stat("/etc/nginx/sites-enabled/zyhive"); err == nil {
				confPath = "/etc/nginx/sites-enabled/zyhive"
			}
			data, err := os.ReadFile(confPath)
			if err != nil {
				printWarn("未找到 ZyHive 的 Nginx 配置：" + confPath)
			} else {
				fmt.Println(ansiCyan + string(data) + ansiReset)
			}
			pause()
		case "6":
			out := runCmd("ls", "-la", "/etc/nginx/conf.d/")
			fmt.Println(ansiCyan + out + ansiReset)
			out2 := runCmd("ls", "-la", "/etc/nginx/sites-enabled/")
			if out2 != "" {
				fmt.Println(ansiCyan + out2 + ansiReset)
			}
			pause()
		case "7":
			domain := readInput("输入域名（如 hive.example.com）")
			domain = strings.TrimSpace(domain)
			cfg := loadConfigQuiet()
			port := 8080
			if cfg != nil {
				port = cfg.Gateway.Port
			}
			generateNginxConfig(domain, port)
			pause()
		case "0", "q":
			return
		}
	}
}

// ── 8. SSL 证书管理 ────────────────────────────────────────────────────────
func menuSSL() {
	if !commandExists("certbot") {
		printWarn("certbot 未安装。安装命令：apt-get install certbot 或 yum install certbot")
		pause()
		return
	}

	for {
		clearScreen()
		printSectionTitle("SSL 证书管理")

		printMenuItem("1", "查看所有证书")
		printMenuItem("2", "申请新证书（Let's Encrypt）")
		printMenuItem("3", "手动续期证书")
		printMenuItem("4", "证书到期时间检查")
		printMenuItem("5", "删除证书")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			out := runCmd("certbot", "certificates")
			paginate(out)
		case "2":
			domain := readInput("输入域名（如 hive.example.com）")
			domain = strings.TrimSpace(domain)
			email := readInput("输入管理员邮箱")
			email = strings.TrimSpace(email)
			fmt.Printf("\n  正在为 %s 申请证书...\n", domain)
			runInteractive("certbot", "--nginx", "-d", domain,
				"--non-interactive", "--agree-tos", "--email", email, "--redirect")
		case "3":
			fmt.Println("  正在续期所有证书...")
			out := runCmd("certbot", "renew", "--dry-run")
			fmt.Println(ansiCyan + out + ansiReset)
			if confirm("执行真实续期？") {
				runInteractive("certbot", "renew")
			}
		case "4":
			certDir := "/etc/letsencrypt/live"
			entries, err := os.ReadDir(certDir)
			if err != nil {
				printError("无法读取证书目录：" + err.Error())
			} else {
				fmt.Println()
				for _, e := range entries {
					if e.IsDir() {
						certFile := filepath.Join(certDir, e.Name(), "cert.pem")
						out := runCmd("openssl", "x509", "-enddate", "-noout", "-in", certFile)
						fmt.Printf("  %s%-30s%s %s\n", ansiGreen, e.Name(), ansiReset, strings.TrimSpace(out))
					}
				}
			}
			pause()
		case "5":
			domain := readInput("输入要删除证书的域名")
			domain = strings.TrimSpace(domain)
			if confirm("确认删除 " + domain + " 的证书？") {
				runInteractive("certbot", "delete", "--cert-name", domain)
			}
		case "0", "q":
			return
		}
	}
}

// ── 9. 备份与恢复 ──────────────────────────────────────────────────────────
func menuBackup() {
	for {
		clearScreen()
		printSectionTitle("备份与恢复")

		cfg := loadConfigQuiet()
		agentsDir := "/var/lib/zyhive/agents"
		if cfg != nil && cfg.Agents.Dir != "" {
			agentsDir = cfg.Agents.Dir
		}
		configPath := findConfigPath()
		backupDir := "/var/backups/zyhive"

		printKV("成员目录", agentsDir)
		printKV("配置文件", configPath)
		printKV("备份目录", backupDir)
		fmt.Println()

		printMenuItem("1", "创建完整备份（成员 + 配置）")
		printMenuItem("2", "查看现有备份")
		printMenuItem("3", "从备份恢复")
		printMenuItem("4", "修改备份目录")
		printMenuItem("0", "返回主菜单")
		fmt.Println()

		choice := readInput("请输入选项")
		switch strings.TrimSpace(choice) {
		case "1":
			os.MkdirAll(backupDir, 0755)
			ts := time.Now().Format("20060102-150405")
			backupFile := filepath.Join(backupDir, "zyhive-backup-"+ts+".tar.gz")
			fmt.Printf("  创建备份：%s\n", backupFile)
			out := runCmd("tar", "-czf", backupFile, agentsDir, configPath)
			if out != "" {
				fmt.Println(out)
			}
			if _, err := os.Stat(backupFile); err == nil {
				info, _ := os.Stat(backupFile)
				printSuccess(fmt.Sprintf("备份成功！文件大小：%.2f MB", float64(info.Size())/1024/1024))
			} else {
				printError("备份失败")
			}
			pause()
		case "2":
			out := runCmd("ls", "-lht", backupDir)
			fmt.Println(ansiCyan + out + ansiReset)
			pause()
		case "3":
			out := runCmd("ls", "-1", backupDir)
			fmt.Println("  可用备份：\n" + ansiCyan + out + ansiReset)
			backupFile := readInput("输入备份文件名（含路径）")
			backupFile = strings.TrimSpace(backupFile)
			if !strings.HasPrefix(backupFile, "/") {
				backupFile = filepath.Join(backupDir, backupFile)
			}
			if _, err := os.Stat(backupFile); os.IsNotExist(err) {
				printError("备份文件不存在：" + backupFile)
			} else if confirm("恢复会覆盖现有数据，确认继续？") {
				fmt.Println("  停止服务...")
				runCmd("systemctl", "stop", "zyhive")
				out := runCmd("tar", "-xzf", backupFile, "-C", "/")
				if out != "" {
					fmt.Println(out)
				}
				runCmd("systemctl", "start", "zyhive")
				printSuccess("恢复完成，服务已重启")
			}
			pause()
		case "4":
			newDir := readInput("输入新备份目录")
			newDir = strings.TrimSpace(newDir)
			if newDir != "" {
				backupDir = newDir
				os.MkdirAll(backupDir, 0755)
				printSuccess("备份目录已设置为：" + backupDir)
			}
			pause()
		case "0", "q":
			return
		}
	}
}

// ── 辅助工具函数 ───────────────────────────────────────────────────────────

func readInput(prompt string) string {
	fmt.Printf(ansiCyan+"  %s: "+ansiReset, prompt)
	line, _ := cliReader.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func confirm(msg string) bool {
	fmt.Printf(ansiYellow+"  ⚠ %s [y/N]: "+ansiReset, msg)
	line, _ := cliReader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func pause() {
	fmt.Print(ansiBlue + "\n  按 Enter 返回..." + ansiReset)
	cliReader.ReadString('\n')
}

func printSectionTitle(title string) {
	fmt.Println(ansiBold + ansiBlue + "  ══════ " + title + " ══════" + ansiReset)
	fmt.Println()
}

func printMenuItem(key, label string) {
	fmt.Printf("  %s[%s]%s  %s\n", ansiCyan+ansiBold, key, ansiReset, label)
}

func printKV(key, val string) {
	fmt.Printf("  %s%-16s%s %s\n", ansiBold, key+"：", ansiReset, val)
}

func printSuccess(msg string) {
	fmt.Println(ansiGreen + "  ✅ " + msg + ansiReset)
}

func printError(msg string) {
	fmt.Println(ansiRed + "  ❌ " + msg + ansiReset)
}

func printWarn(msg string) {
	fmt.Println(ansiYellow + "  ⚠  " + msg + ansiReset)
}

func center(s string, width int) string {
	runeLen := len([]rune(s))
	if runeLen >= width {
		return s
	}
	pad := (width - runeLen) / 2
	return strings.Repeat(" ", pad) + s + strings.Repeat(" ", width-runeLen-pad)
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func runInteractive(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isServiceRunning() bool {
	out := runCmd("systemctl", "is-active", "zyhive")
	return strings.TrimSpace(out) == "active"
}

func systemctlAction(action, service string) {
	out := runCmd("systemctl", action, service)
	if out != "" {
		fmt.Println(ansiCyan + out + ansiReset)
	}
	switch action {
	case "start":
		time.Sleep(time.Second)
		if isServiceRunning() {
			printSuccess(service + " 已启动")
		} else {
			printError(service + " 启动失败，请检查日志")
		}
	case "stop":
		printSuccess(service + " 已停止")
	case "restart":
		time.Sleep(time.Second)
		if isServiceRunning() {
			printSuccess(service + " 已重启")
		} else {
			printError(service + " 重启失败")
		}
	case "enable":
		printSuccess(service + " 已设置开机自启")
	case "disable":
		printSuccess(service + " 已取消开机自启")
	}
	pause()
}

func loadConfigQuiet() *config.Config {
	path := findConfigPath()
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}
	return cfg
}

func findConfigPath() string {
	// 优先级：环境变量 > 系统路径 > 用户路径 > 当前目录
	candidates := []string{
		os.Getenv("AIPANEL_CONFIG"),
		"/etc/zyhive/zyhive.json",
		"/usr/local/etc/zyhive/zyhive.json",
		os.ExpandEnv("$HOME/.config/zyhive/zyhive.json"),
		"aipanel.json",
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/etc/zyhive/zyhive.json"
}

func patchConfig(path string, cfg *config.Config, fn func(*config.Config)) bool {
	fn(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		printError("序列化配置失败：" + err.Error())
		return false
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		printError("写入配置失败：" + err.Error())
		return false
	}
	return true
}

func generateToken() string {
	out := runCmd("openssl", "rand", "-hex", "16")
	out = strings.TrimSpace(out)
	if out != "" {
		return out
	}
	// 降级：用时间戳
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func fetchLatestVersion() string {
	out := runCmd("curl", "-fsSL",
		"https://api.github.com/repos/Zyling-ai/zyhive/releases/latest")
	// 简单提取 tag_name
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `"tag_name"`) {
			parts := strings.SplitN(line, `"`, 4)
			if len(parts) >= 4 {
				tag := strings.Split(parts[3], `"`)[0]
				return tag
			}
		}
	}
	return ""
}

func paginate(content string) {
	lines := strings.Split(content, "\n")
	pageSize := 30
	for i := 0; i < len(lines); i += pageSize {
		end := i + pageSize
		if end > len(lines) {
			end = len(lines)
		}
		fmt.Println(ansiCyan + strings.Join(lines[i:end], "\n") + ansiReset)
		if end < len(lines) {
			fmt.Print(ansiBlue + "  -- 按 Enter 继续，输入 q 退出 -- " + ansiReset)
			line, _ := cliReader.ReadString('\n')
			if strings.TrimSpace(line) == "q" {
				return
			}
		}
	}
	pause()
}

func generateNginxConfig(domain string, port int) {
	confPath := "/etc/nginx/conf.d/" + domain + ".conf"
	if _, err := os.Stat("/etc/nginx/sites-available"); err == nil {
		confPath = "/etc/nginx/sites-available/" + domain
	}

	content := fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %s;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        proxy_pass         http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade $http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $http_cf_connecting_ip;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $http_x_forwarded_proto;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering    off;
        proxy_cache        off;
    }
}
`, domain, port)

	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		printError("写入 Nginx 配置失败：" + err.Error())
		return
	}

	// sites-enabled 软链接
	if _, err := os.Stat("/etc/nginx/sites-available"); err == nil {
		enabledPath := "/etc/nginx/sites-enabled/" + domain
		os.Symlink(confPath, enabledPath)
	}

	printSuccess("Nginx 配置已生成：" + confPath)
	fmt.Printf("  下一步：申请 SSL 证书\n")
	fmt.Printf("    certbot --nginx -d %s --agree-tos --email admin@%s\n", domain, domain)

	out := runCmd("nginx", "-t")
	if strings.Contains(out, "successful") {
		runCmd("systemctl", "reload", "nginx")
		printSuccess("Nginx 配置已重载")
	}
}
