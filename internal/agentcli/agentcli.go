package agentcli

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// action is a single verb under a resource command (e.g. `agent list`).
type action struct {
	name    string
	summary string
	usage   string // one-line usage shown in help (include key flags)
	run     func(c *ctx, args []string) error
}

// command is a resource command group (e.g. `agent`).
type command struct {
	name    string
	summary string
	actions []*action
}

func (cmd *command) find(name string) *action {
	for _, a := range cmd.actions {
		if strings.EqualFold(a.name, name) {
			return a
		}
	}
	return nil
}

var (
	cmdList  []*command
	cmdIndex = map[string]*command{}
)

// registerCommand is called from each cmd_*.go init() to add a resource group.
func registerCommand(c *command) {
	if _, dup := cmdIndex[c.name]; dup {
		panic("agentcli: duplicate command " + c.name)
	}
	cmdList = append(cmdList, c)
	cmdIndex[c.name] = c
}

// IsCommand reports whether name is a registered business command. main.go uses
// this to route only known resources into the agent CLI (ops commands stay put).
func IsCommand(name string) bool {
	return cmdIndex[name] != nil
}

// LooksLikeCommand reports whether args (os.Args[1:]) begin with a business
// command once global flags are stripped. main.go calls this before its own
// flag parsing so the CLI fully owns argument handling for its commands.
func LooksLikeCommand(args []string) bool {
	_, rest := extractGlobals(args)
	return len(rest) > 0 && IsCommand(rest[0])
}

// globalOpts holds flags that apply to every command.
type globalOpts struct {
	JSON, Quiet, Yes, Help bool
	Host, Token, Config    string
}

// extractGlobals pulls global flags out of args (in any position) so each
// subcommand's FlagSet only sees its own flags.
func extractGlobals(args []string) (*globalOpts, []string) {
	o := &globalOpts{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case a == "--json":
			o.JSON = true
		case a == "--quiet" || a == "-q":
			o.Quiet = true
		case a == "--yes" || a == "-y":
			o.Yes = true
		case a == "--help" || a == "-h":
			o.Help = true
		case a == "--host":
			o.Host = next()
		case strings.HasPrefix(a, "--host="):
			o.Host = a[len("--host="):]
		case a == "--token":
			o.Token = next()
		case strings.HasPrefix(a, "--token="):
			o.Token = a[len("--token="):]
		case a == "--config":
			o.Config = next()
		case strings.HasPrefix(a, "--config="):
			o.Config = a[len("--config="):]
		default:
			rest = append(rest, a)
		}
	}
	return o, rest
}

// Dispatch runs a business CLI command and returns the process exit code.
// rawArgs is os.Args minus the program name, e.g. ["agent","list","--json"].
func Dispatch(rawArgs []string) int {
	opts, rest := extractGlobals(rawArgs)
	c := &ctx{json: opts.JSON, quiet: opts.Quiet, yes: opts.Yes, out: os.Stdout, err: os.Stderr}

	if len(rest) == 0 {
		printTopHelp(c)
		return ExitOK
	}

	cmd := cmdIndex[rest[0]]
	if cmd == nil {
		return fail(c, usageErr("未知命令 %q，运行 `zyhive --help` 查看可用命令", rest[0]))
	}

	actionArgs := rest[1:]
	if len(actionArgs) == 0 {
		printCommandHelp(c, cmd)
		return ExitOK
	}

	act := cmd.find(actionArgs[0])
	if act == nil {
		return fail(c, usageErr("%s: 未知动作 %q，运行 `zyhive %s --help` 查看动作列表", cmd.name, actionArgs[0], cmd.name))
	}

	if opts.Help {
		printActionHelp(c, cmd, act)
		return ExitOK
	}

	client, err := resolveClient(opts)
	if err != nil {
		return fail(c, err)
	}
	c.client = client

	if err := act.run(c, actionArgs[1:]); err != nil {
		return fail(c, err)
	}
	return ExitOK
}

// ── help rendering ─────────────────────────────────────────────────────────

func printTopHelp(c *ctx) {
	w := c.out
	fmt.Fprintln(w, "引巢 · ZyHive — Agent 系统操作 CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法: zyhive <资源> <动作> [参数] [--json] [--host URL] [--token TOK]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "全局选项:")
	fmt.Fprintln(w, "  --json           输出机器可读 JSON（供 agent/脚本解析）")
	fmt.Fprintln(w, "  --host URL       目标服务地址（默认读本机配置 / ZYHIVE_HOST）")
	fmt.Fprintln(w, "  --token TOK      访问令牌（默认读本机配置 / ZYHIVE_TOKEN）")
	fmt.Fprintln(w, "  --config PATH    指定配置文件路径")
	fmt.Fprintln(w, "  --yes, -y        跳过确认（写/删操作在非交互环境必须带）")
	fmt.Fprintln(w, "  --quiet, -q      精简输出")
	fmt.Fprintln(w, "  --help, -h       查看帮助（可用于任意层级）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "资源命令:")

	sorted := make([]*command, len(cmdList))
	copy(sorted, cmdList)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })
	width := 0
	for _, cmd := range sorted {
		if n := len(cmd.name); n > width {
			width = n
		}
	}
	for _, cmd := range sorted {
		fmt.Fprintf(w, "  %-*s  %s\n", width, cmd.name, cmd.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "示例:")
	fmt.Fprintln(w, "  zyhive agent list --json")
	fmt.Fprintln(w, "  zyhive chat send main \"今天有哪些待办?\"")
	fmt.Fprintln(w, "  zyhive cron add --agent main --name 晨报 --expr \"0 9 * * *\" --message \"整理昨日\"")
	fmt.Fprintln(w, "  zyhive api GET /api/status")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "运行 `zyhive <资源> --help` 查看某个资源的全部动作。")
}

func printCommandHelp(c *ctx, cmd *command) {
	w := c.out
	fmt.Fprintf(w, "zyhive %s — %s\n\n", cmd.name, cmd.summary)
	fmt.Fprintln(w, "动作:")
	width := 0
	for _, a := range cmd.actions {
		if n := len(a.name); n > width {
			width = n
		}
	}
	for _, a := range cmd.actions {
		fmt.Fprintf(w, "  %-*s  %s\n", width, a.name, a.summary)
	}
	fmt.Fprintf(w, "\n运行 `zyhive %s <动作> --help` 查看用法。\n", cmd.name)
}

func printActionHelp(c *ctx, cmd *command, a *action) {
	w := c.out
	fmt.Fprintf(w, "zyhive %s %s — %s\n\n", cmd.name, a.name, a.summary)
	if a.usage != "" {
		fmt.Fprintln(w, "用法:")
		for _, line := range strings.Split(a.usage, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
}
