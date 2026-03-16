// Package tools — acp_list and acp_spawn tools for external coding-agent CLIs.
// ACP (Agent Control Protocol) lets agents invoke terminal-based coding CLIs
// such as `claude` (Claude Code), `codex`, or `gemini` as background subprocesses.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/config"
	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// ACPAgentLister returns the current list of configured ACP agents.
type ACPAgentLister func() []config.ACPAgentEntry

// WithACPAgents injects the ACP agent lister so acp_list and acp_spawn tools are available.
func (r *Registry) WithACPAgents(lister ACPAgentLister) {
	r.acpLister = lister
	r.register(llm.ToolDef{
		Name:        "acp_list",
		Description: "列出所有已配置的外部编程 AI 代理（ACP Agents），如 Claude Code、Codex、Gemini CLI 等。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, r.handleACPList)

	r.register(llm.ToolDef{
		Name:        "acp_spawn",
		Description: "在后台启动一个外部编程 AI 代理（ACP Agent）执行任务。代理作为子进程运行，输出通过 process 工具查看。返回进程 ID。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"acpId":{"type":"string","description":"ACP Agent ID（来自 acp_list）"},
				"task":{"type":"string","description":"要执行的任务描述（作为 stdin 传入，或替换 {{task}} 占位符）"},
				"workDir":{"type":"string","description":"工作目录（可选，覆盖 ACP Agent 默认目录）"},
				"timeout":{"type":"integer","description":"超时秒数（0=不限，默认600秒）"}
			},
			"required":["acpId","task"]
		}`),
	}, r.handleACPSpawn)
}

// ── acpSession tracks a running ACP subprocess ───────────────────────────────

var (
	acpSessionMu sync.Mutex
	acpSessions  = make(map[string]*acpSession)
)

type acpSession struct {
	ID        string
	ACPID     string
	Task      string
	Status    string // "running" | "done" | "error"
	Output    strings.Builder
	Err       string
	StartedAt int64
	EndedAt   int64
	cancel    context.CancelFunc
}

func genACPSessionID() string {
	return fmt.Sprintf("acp-%d", time.Now().UnixNano()%1_000_000_000)
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (r *Registry) handleACPList(_ context.Context, _ json.RawMessage) (string, error) {
	if r.acpLister == nil {
		return "[]", nil
	}
	agents := r.acpLister()
	if len(agents) == 0 {
		return "未配置任何 ACP Agent。请在「能力 → ACP 编程代理」中添加。", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个 ACP Agent：\n", len(agents)))
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- ID: %s | 名称: %s | 命令: %s", a.ID, a.Name, a.Binary))
		if len(a.Args) > 0 {
			sb.WriteString(" " + strings.Join(a.Args, " "))
		}
		if a.Status != "" && a.Status != "ok" {
			sb.WriteString(fmt.Sprintf(" [%s]", a.Status))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (r *Registry) handleACPSpawn(_ context.Context, input json.RawMessage) (string, error) {
	if r.acpLister == nil {
		return "", fmt.Errorf("ACP agents not configured")
	}
	var p struct {
		ACPID   string `json:"acpId"`
		Task    string `json:"task"`
		WorkDir string `json:"workDir"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %v", err)
	}
	if p.ACPID == "" {
		return "", fmt.Errorf("acpId is required — use acp_list to get available agent IDs")
	}
	if p.Task == "" {
		return "", fmt.Errorf("task is required")
	}

	// Find the ACP agent config.
	var found *config.ACPAgentEntry
	for _, a := range r.acpLister() {
		if a.ID == p.ACPID {
			cp := a
			found = &cp
			break
		}
	}
	if found == nil {
		return "", fmt.Errorf("ACP agent %q not found — use acp_list to see available agents", p.ACPID)
	}

	// Build command args: substitute {{task}} placeholder in Args.
	args := make([]string, len(found.Args))
	taskInjected := false
	for i, a := range found.Args {
		if strings.Contains(a, "{{task}}") {
			args[i] = strings.ReplaceAll(a, "{{task}}", p.Task)
			taskInjected = true
		} else {
			args[i] = a
		}
	}
	// If no placeholder, task will be passed via stdin.
	useStdin := !taskInjected

	// Resolve working directory.
	workDir := p.WorkDir
	if workDir == "" {
		workDir = found.WorkDir
	}
	if workDir == "" {
		workDir = r.workspaceDir
	}

	// Set up timeout.
	timeout := time.Duration(p.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}

	// Create session entry.
	sid := genACPSessionID()
	sess := &acpSession{
		ID:        sid,
		ACPID:     found.ID,
		Task:      p.Task,
		Status:    "running",
		StartedAt: time.Now().UnixMilli(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	sess.cancel = cancel

	acpSessionMu.Lock()
	acpSessions[sid] = sess
	acpSessionMu.Unlock()

	// Launch subprocess asynchronously.
	go func() {
		defer cancel()
		defer func() {
			acpSessionMu.Lock()
			sess.EndedAt = time.Now().UnixMilli()
			acpSessionMu.Unlock()
		}()

		cmd := exec.CommandContext(ctx, found.Binary, args...)
		cmd.Dir = workDir

		// Inject custom env vars.
		cmd.Env = append(os.Environ(), found.Env...)

		if useStdin {
			cmd.Stdin = strings.NewReader(p.Task)
		}

		out, err := cmd.CombinedOutput()

		acpSessionMu.Lock()
		defer acpSessionMu.Unlock()
		sess.Output.Write(out)
		if err != nil {
			sess.Status = "error"
			sess.Err = err.Error()
		} else {
			sess.Status = "done"
		}
	}()

	return fmt.Sprintf("✅ ACP Agent 已启动\n- 进程 ID: %s\n- Agent: %s (%s)\n- 超时: %s\n使用 process 工具查看输出：action=log / poll，sessionId=%s",
		sid, found.Name, found.ID, timeout, sid), nil
}

// acpSessionLookup is called by the process tool to fetch ACP session output.
// Returns (output, status, ok). Used by handleProcess for log/poll actions.
func acpSessionLookup(sid string) (output, status string, ok bool) {
	acpSessionMu.Lock()
	defer acpSessionMu.Unlock()
	sess, exists := acpSessions[sid]
	if !exists {
		return "", "", false
	}
	return sess.Output.String(), sess.Status, true
}
