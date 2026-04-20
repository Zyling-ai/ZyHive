// Package tools provides the built-in tool registry.
// Reference: pi-coding-agent/dist/core/tools/index.js
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/project"
	"github.com/Zyling-ai/zyhive/pkg/skill"
	"github.com/Zyling-ai/zyhive/pkg/subagent"
)



// Handler executes a tool call and returns the result string.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Registry maps tool names to their definition and handler.
type Registry struct {
	defs          []llm.ToolDef
	handlers      map[string]Handler
	workspaceDir  string // agent-specific working directory for path resolution
	agentDir      string // parent dir of workspace (contains config.json)
	agentID       string // agent ID (used for self-management tools)
	sessionID     string // current session ID (passed to spawn so NotifyFunc can reply)
	projectMgr    *project.Manager             // shared project workspace (nil = no project access)
	agentEnv      map[string]string            // per-agent env vars injected into exec (bypass sanitize)
	subagentMgr   *subagent.Manager            // background task manager (nil = no subagent tools)
	agentLister   func() []AgentSummary        // optional: lists available agents for agent_list tool
	fileSender    func(string) (string, error)                   // optional: sends a file to the current chat (e.g. Telegram)
	serverBaseURL string                                         // base URL for generating download links (files > 50 MB)
	authToken     string                                         // auth token for download link generation
	envUpdater    func(key, value string, remove bool) error     // optional: lets the agent update its own env vars

	// Dispatch panel: report_to_parent support
	parentSessionID string                // non-empty when this agent was spawned by a parent session
	broadcastFn     subagent.BroadcastFn // optional: publish subagent events to parent broadcaster

	// report_result support: callback to update task artifacts in the manager
	taskArtifactFn func(artifacts []subagent.TaskArtifact) // optional

	cronEngine   CronEngine      // optional: cron_* tools
	sessionTools *sessionToolSet // optional: sessions_* tools
	acpLister    ACPAgentLister  // optional: acp_list + acp_spawn tools
}

// AgentSummary is the minimal agent info exposed through the agent_list tool.
type AgentSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// New creates a Registry pre-loaded with all built-in tools.
// workspaceDir is the agent's workspace; relative file paths are resolved against it.
// agentDir is the parent directory of workspace (contains config.json).
// agentID is the agent's unique identifier.
func New(workspaceDir, agentDir, agentID string) *Registry {
	r := &Registry{
		handlers:     make(map[string]Handler),
		workspaceDir: workspaceDir,
		agentDir:     agentDir,
		agentID:      agentID,
	}
	r.register(readToolDef, r.handleReadWS)
	r.register(writeToolDef, r.handleWriteWS)
	r.register(editToolDef, r.handleEditWS)
	r.register(bashToolDef, r.handleBashWS)
	r.register(processToolDef, handleProcess)
	r.register(grepToolDef, r.handleGrepWS)
	r.register(globToolDef, r.handleGlobWS)
	r.register(webFetchToolDef, handleWebFetch)
	r.register(showImageDef, func(ctx context.Context, input json.RawMessage) (string, error) { return handleShowImage(ctx, input) })
	// Self-management tools (available to all agents)
	r.register(selfListSkillsDef, r.handleSelfListSkills)
	r.register(selfInstallSkillDef, r.handleSelfInstallSkill)
	r.register(selfUninstallSkillDef, r.handleSelfUninstallSkill)
	r.register(selfRenameDef, r.handleSelfRename)
	r.register(selfUpdateSoulDef, r.handleSelfUpdateSoul)
	// Wishlist tools — AI 自主表达能力诉求
	r.register(wishAddDef, r.handleWishAdd)
	r.register(wishListDef, r.handleWishList)
	// Always register subagent tools (nil mgr returns "not configured" error)
	r.registerSubagentTools()
	return r
}

// NewSkillStudio creates a sandboxed Registry for the SkillStudio AI.
// File writes are restricted to skills/{skillID}/ only.
// Bash is enabled (needed to test CLI skills). Self-management tools are disabled.
func NewSkillStudio(workspaceDir, agentDir, agentID, skillID string) *Registry {
	r := &Registry{
		handlers:     make(map[string]Handler),
		workspaceDir: workspaceDir,
		agentDir:     agentDir,
		agentID:      agentID,
	}
	allowedPrefix := filepath.Join(workspaceDir, "skills", skillID)

	// Sandboxed write: only allowed within skills/{skillID}/
	r.register(writeToolDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		resolved := r.resolveFilePathInInput(input, "file_path")
		var m map[string]json.RawMessage
		if err := json.Unmarshal(resolved, &m); err == nil {
			var path string
			if err2 := json.Unmarshal(m["file_path"], &path); err2 == nil {
				if !(strings.HasPrefix(path, allowedPrefix+string(filepath.Separator)) || path == allowedPrefix) {
					return "", fmt.Errorf("🚫 沙箱限制：只允许写入 skills/%s/ 目录，拒绝路径: %s", skillID, path)
				}
			}
		}
		return handleWrite(ctx, resolved)
	})

	// Sandboxed edit: only allowed within skills/{skillID}/
	r.register(editToolDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		resolved := r.resolveFilePathInInput(input, "file_path")
		var m map[string]json.RawMessage
		if err := json.Unmarshal(resolved, &m); err == nil {
			var path string
			if err2 := json.Unmarshal(m["file_path"], &path); err2 == nil {
				if !(strings.HasPrefix(path, allowedPrefix+string(filepath.Separator)) || path == allowedPrefix) {
					return "", fmt.Errorf("🚫 沙箱限制：只允许编辑 skills/%s/ 目录，拒绝路径: %s", skillID, path)
				}
			}
		}
		return handleEdit(ctx, resolved)
	})

	// Read and search: allowed everywhere (read-only is safe)
	r.register(readToolDef, r.handleReadWS)
	r.register(grepToolDef, r.handleGrepWS)
	r.register(globToolDef, r.handleGlobWS)
	r.register(webFetchToolDef, handleWebFetch)
	r.register(showImageDef, func(ctx context.Context, input json.RawMessage) (string, error) { return handleShowImage(ctx, input) })
	// List skills is read-only, allow it
	r.register(selfListSkillsDef, r.handleSelfListSkills)
	// Bash: enabled in skill-studio so the AI can test CLI tools and verify skill behaviour.
	// CWD is set to the agent workspace, same as the normal chat context.
	r.register(bashToolDef, r.handleBashWS)
	// self_install_skill, self_uninstall_skill, self_rename, self_update_soul: NOT registered (disabled)
	return r
}

// WithEnv configures per-agent environment variables that are injected into
// exec/bash tool calls. These vars override the sanitized system env, allowing
// agents to use credentials like GITHUB_TOKEN, GIT_AUTHOR_NAME, etc.
func (r *Registry) WithEnv(env map[string]string) {
	r.agentEnv = env
}

// WithSessionID records the current session ID so agent_spawn can include it
// in SpawnOpts, enabling the NotifyFunc to deliver results back to this session.
func (r *Registry) WithSessionID(id string) {
	r.sessionID = id
}

// WithEnvUpdater registers self_set_env and self_delete_env tools backed by the given updater.
// updater(key, value, remove) sets or deletes a single env var for the agent.
func (r *Registry) WithEnvUpdater(updater func(key, value string, remove bool) error) {
	r.envUpdater = updater
	r.register(selfSetEnvDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSelfSetEnv(ctx, input)
	})
	r.register(selfDeleteEnvDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSelfDeleteEnv(ctx, input)
	})
}

// WithParentSession registers the report_to_parent tool so a subagent can send
// live progress updates back to its parent session's DispatchPanel.
// parentSessionID is the session ID of the spawning agent; fn broadcasts events.
func (r *Registry) WithParentSession(parentSessionID string, fn subagent.BroadcastFn) {
	if parentSessionID == "" || fn == nil {
		return
	}
	r.parentSessionID = parentSessionID
	r.broadcastFn = fn
	r.register(reportToParentDef, r.handleReportToParent)
}

// handleReportToParent publishes a report event to the parent session's broadcaster.
func (r *Registry) handleReportToParent(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Content  string `json:"content"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if r.parentSessionID == "" {
		return "（当前未在派遣任务中，无需汇报）", nil
	}

	ev := subagent.SubagentEvent{
		Type:              "report",
		SubagentSessionID: r.sessionID,
		AgentID:           r.agentID,
		AgentName:         r.agentID,
		AvatarColor:       "#6366f1",
		Content:           p.Content,
		Status:            p.Status,
		Progress:          p.Progress,
		Timestamp:         time.Now().UnixMilli(),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	r.broadcastFn(r.parentSessionID, "subagent_report", data)
	return "汇报已发送给上级", nil
}

// WithFileSender registers the send_file tool backed by the given sender function.
// sender is a closure that delivers a local file to the active chat (e.g. via Telegram).
// serverBaseURL and authToken are used to generate download links for files > 50 MB.
func (r *Registry) WithFileSender(sender func(string) (string, error), serverBaseURL, authToken string) {
	r.fileSender = sender
	r.serverBaseURL = serverBaseURL
	r.authToken = authToken
	r.register(sendFileDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleSendFile(ctx, input)
	})
}

// WithAgentLister registers an agent_list tool that lets the AI look up available
// agent IDs before calling agent_spawn.
func (r *Registry) WithAgentLister(lister func() []AgentSummary) {
	r.agentLister = lister
	r.register(llm.ToolDef{
		Name:        "agent_list",
		Description: "列出所有可用的 AI 成员（返回 id、name、description）。在调用 agent_spawn 前先调用此工具确认正确的 agentId。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(_ context.Context, _ json.RawMessage) (string, error) {
		agents := r.agentLister()
		if len(agents) == 0 {
			return "（暂无可用成员）", nil
		}
		var sb strings.Builder
		sb.WriteString("可用 AI 成员列表：\n\n")
		for _, a := range agents {
			sb.WriteString(fmt.Sprintf("- **%s** (id: `%s`)", a.Name, a.ID))
			if a.Description != "" {
				sb.WriteString(" — " + a.Description)
			}
			sb.WriteString("\n")
		}
		return sb.String(), nil
	})
}

// registerSubagentTools registers agent_spawn/tasks/kill/result tools.
// Called both from New() (with nil mgr, so tools always appear) and from
// WithSubagentManager (with real mgr, which overwrites the nil-mgr handlers).
func (r *Registry) registerSubagentTools() {
	r.register(llm.ToolDef{
		Name:        "agent_spawn",
		Description: "在后台派生一个 AI 成员执行任务。任务异步执行，不阻塞当前对话。完成后自动通知。返回任务 ID。⚠️ 务必先调用 agent_list 确认正确的 agentId，不要猜测。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agentId":{"type":"string","description":"执行任务的 AI 成员 ID"},
				"task":{"type":"string","description":"详细的任务指令"},
				"label":{"type":"string","description":"任务简短标签，便于识别（可选）"},
				"model":{"type":"string","description":"覆盖默认模型（可选，格式: provider/model，如 anthropic/claude-opus-4-5）"},
				"timeout":{"type":"integer","description":"任务超时秒数（0=不限，默认不限）"},
				"max_turns":{"type":"integer","description":"最大 LLM 对话轮次（0=不限，默认不限）"},
				"isolated":{"type":"boolean","description":"隔离模式：不继承父对话上下文，以全新 session 启动（默认 false）"},
				"background":{"type":"string","description":"任务背景说明（可选）"},
				"deliverable":{"type":"string","description":"期望的交付物描述（可选）"},
				"priority":{"type":"string","enum":["high","normal","low"],"description":"任务优先级（可选，默认 normal）"},
				"attachments":{"type":"array","description":"参考资料列表（可选）","items":{
					"type":"object",
					"properties":{
						"name":{"type":"string","description":"资料名称/标题"},
						"content":{"type":"string","description":"资料文本内容（与 path 二选一）"},
						"path":{"type":"string","description":"工作区文件相对路径，自动读取内容（与 content 二选一）"}
					},
					"required":["name"]
				}},
				"context_turns":{"type":"integer","minimum":0,"maximum":10,"description":"携带当前对话最近 N 轮作为背景上下文（0=不携带，默认0；isolated=true 时忽略）"},
				"shared_project_id":{"type":"string","description":"共享项目 ID — 授权执行方读写此项目，可将产出文件写入（可选）"}
			},
			"required":["agentId","task"]
		}`),
	}, r.handleAgentSpawn)

	r.register(llm.ToolDef{
		Name:        "agent_tasks",
		Description: "查看所有后台任务的状态列表（含任务ID、状态、执行者、标签、耗时）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agentId":{"type":"string","description":"仅查看该 AI 成员的任务（可选，不填则看全部）"},
				"status":{"type":"string","description":"按状态过滤: pending/running/done/error/killed（可选）"}
			}
		}`),
	}, r.handleAgentTasks)

	r.register(llm.ToolDef{
		Name:        "agent_kill",
		Description: "终止一个正在运行的后台任务。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"taskId":{"type":"string","description":"要终止的任务 ID"}
			},
			"required":["taskId"]
		}`),
	}, r.handleAgentKill)

	r.register(llm.ToolDef{
		Name:        "agent_result",
		Description: "获取后台任务的完整输出内容。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"taskId":{"type":"string","description":"任务 ID"}
			},
			"required":["taskId"]
		}`),
	}, r.handleAgentResult)
}

// WithSubagentManager sets the subagent manager and re-registers tools with the real manager.
// Calling this after New() upgrades the nil-mgr stubs to fully functional handlers.
func (r *Registry) WithSubagentManager(mgr *subagent.Manager) {
	r.subagentMgr = mgr
	r.registerSubagentTools() // overwrites nil-mgr stubs with real handlers
}

// WithTaskArtifactFn registers a callback so the report_result tool can update task artifacts.
// Called by pool.go when wiring a subagent task runner.
func (r *Registry) WithTaskArtifactFn(fn func(artifacts []subagent.TaskArtifact)) {
	r.taskArtifactFn = fn
	r.registerReportResultTool()
}

// registerReportResultTool registers (or overwrites) the report_result tool.
func (r *Registry) registerReportResultTool() {
	r.register(llm.ToolDef{
		Name:        "report_result",
		Description: "提交任务的结构化产出。列出已写入共享项目的文件，让派遣方可以直接查看。完成后调用一次即可。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"summary":{"type":"string","description":"任务完成摘要（1-3句话）"},
				"project_id":{"type":"string","description":"共享项目 ID（产出文件所在项目）"},
				"files":{"type":"array","description":"产出文件列表","items":{
					"type":"object",
					"properties":{
						"name":{"type":"string","description":"文件显示名"},
						"path":{"type":"string","description":"项目内相对路径"},
						"type":{"type":"string","enum":["code","report","data","file"],"description":"文件类型"}
					},
					"required":["name","path"]
				}}
			},
			"required":["summary"]
		}`),
	}, r.handleReportResult)
}

func (r *Registry) handleReportResult(_ context.Context, input json.RawMessage) (string, error) {
	// Parse with both files array and flat single-file params (AI often passes flat format)
	var p struct {
		Summary   string `json:"summary"`
		ProjectID string `json:"project_id"`
		Files     []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"files"`
		// Flat single-file fallback (AI sometimes passes these at top level)
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %v", err)
	}

	// If files array is empty but flat params are provided, treat as single file
	if len(p.Files) == 0 && p.Path != "" {
		p.Files = append(p.Files, struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
		}{Name: p.Name, Path: p.Path, Type: p.Type})
	}

	if r.taskArtifactFn == nil {
		return "✅ 摘要已记录（当前上下文不支持产出文件追踪）", nil
	}
	artifacts := make([]subagent.TaskArtifact, 0, len(p.Files))
	for _, f := range p.Files {
		if f.Path == "" {
			continue
		}
		ft := f.Type
		if ft == "" {
			ft = "file"
		}
		name := f.Name
		if name == "" {
			name = filepath.Base(f.Path)
		}
		a := subagent.TaskArtifact{
			Name:      name,
			Path:      f.Path,
			ProjectID: p.ProjectID,
			Type:      ft,
		}
		// Get file size from project
		if p.ProjectID != "" && r.projectMgr != nil {
			if proj, ok := r.projectMgr.Get(p.ProjectID); ok {
				if data, err := os.ReadFile(filepath.Join(proj.FilesDir, f.Path)); err == nil {
					a.Size = len(data)
				}
			}
		}
		artifacts = append(artifacts, a)
	}
	r.taskArtifactFn(artifacts)
	msg := fmt.Sprintf("✅ 任务产出已登记：%s", p.Summary)
	if len(artifacts) > 0 {
		msg += fmt.Sprintf("\n共 %d 个文件已记录，派遣方可在任务面板查看。", len(artifacts))
	} else {
		msg += "\n提示：如有产出文件请在 files 数组中指定 path（如 [{\"name\":\"报告\",\"path\":\"report.md\"}]）"
	}
	return msg, nil
}

// WithProjectAccess registers project_list, project_read, and (if permitted)
// project_write tools backed by the given project.Manager.
// Call after New() to enable shared project workspace access.
func (r *Registry) WithProjectAccess(mgr *project.Manager) {
	r.projectMgr = mgr

	// project_list — always available (read-only metadata)
	r.register(llm.ToolDef{
		Name:        "project_list",
		Description: "列出所有共享团队项目，返回 ID、名称、描述和当前 agent 的写入权限。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, r.handleProjectList)

	// project_read — always available
	r.register(llm.ToolDef{
		Name:        "project_read",
		Description: "读取共享项目中的文件内容。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"project_id":{"type":"string","description":"项目 ID"},
				"file_path":{"type":"string","description":"项目内的文件路径，如 README.md 或 src/main.go"}
			},
			"required":["project_id","file_path"]
		}`),
	}, r.handleProjectRead)

	// project_write — always registered; permission checked at execute time
	r.register(llm.ToolDef{
		Name:        "project_write",
		Description: "写入内容到共享项目的文件（需要该项目的编辑权限）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"project_id":{"type":"string","description":"项目 ID"},
				"file_path":{"type":"string","description":"项目内的文件路径"},
				"content":{"type":"string","description":"写入的内容"}
			},
			"required":["project_id","file_path","content"]
		}`),
	}, r.handleProjectWrite)

	// project_create — create a new shared project
	r.register(llm.ToolDef{
		Name:        "project_create",
		Description: "创建一个新的共享团队项目。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"项目唯一 ID，小写字母/数字/连字符，如 my-project"},
				"name":{"type":"string","description":"项目名称"},
				"description":{"type":"string","description":"项目描述（可选）"},
				"tags":{"type":"array","items":{"type":"string"},"description":"标签列表（可选）"}
			},
			"required":["id","name"]
		}`),
	}, r.handleProjectCreate)

	// project_glob — list files in a project
	r.register(llm.ToolDef{
		Name:        "project_glob",
		Description: "列出共享项目中的文件列表（支持 glob 模式）。",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"project_id":{"type":"string","description":"项目 ID"},
				"pattern":{"type":"string","description":"glob 模式，如 **/*.go，默认 *"}
			},
			"required":["project_id"]
		}`),
	}, r.handleProjectGlob)
}

// handleProjectCreate creates a new shared project.
func (r *Registry) handleProjectCreate(_ context.Context, input json.RawMessage) (string, error) {
	if r.projectMgr == nil {
		return "", fmt.Errorf("project manager not available")
	}
	var p struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	proj, err := r.projectMgr.Create(project.CreateOpts{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Tags:        p.Tags,
	})
	if err != nil {
		return "", fmt.Errorf("创建项目失败: %w", err)
	}
	return fmt.Sprintf("✅ 项目「%s」(id: %s) 已创建", proj.Name, proj.ID), nil
}

// handleProjectList lists all projects with write permission info.
func (r *Registry) handleProjectList(_ context.Context, _ json.RawMessage) (string, error) {
	if r.projectMgr == nil {
		return "", fmt.Errorf("project manager not available")
	}
	projects := r.projectMgr.List()
	if len(projects) == 0 {
		return "（暂无共享项目）", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个共享项目：\n\n", len(projects)))
	for _, p := range projects {
		canWrite := p.CanWrite(r.agentID)
		perm := "可读写"
		if !canWrite {
			perm = "只读"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (`%s`)\n", p.Name, p.ID))
		if p.Description != "" {
			sb.WriteString(fmt.Sprintf("  描述: %s\n", p.Description))
		}
		if len(p.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  标签: %s\n", strings.Join(p.Tags, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  权限: %s\n", perm))
		sb.WriteString(fmt.Sprintf("  更新: %s\n\n", p.UpdatedAt.Format("2006-01-02 15:04")))
	}
	return sb.String(), nil
}

// handleProjectRead reads a file from a shared project.
func (r *Registry) handleProjectRead(_ context.Context, input json.RawMessage) (string, error) {
	if r.projectMgr == nil {
		return "", fmt.Errorf("project manager not available")
	}
	var p struct {
		ProjectID string `json:"project_id"`
		FilePath  string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	proj, ok := r.projectMgr.Get(p.ProjectID)
	if !ok {
		return "", fmt.Errorf("项目 %q 不存在", p.ProjectID)
	}
	fullPath := filepath.Join(proj.FilesDir, filepath.Clean(p.FilePath))
	// safety: must remain within project dir
	if !strings.HasPrefix(fullPath, proj.FilesDir) {
		return "", fmt.Errorf("路径越界")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}
	content := string(data)
	const maxBytes = 50000
	if len(content) > maxBytes {
		content = content[:maxBytes] + "\n[已截断]"
	}
	return content, nil
}

// handleProjectWrite writes a file to a shared project (permission checked).
func (r *Registry) handleProjectWrite(_ context.Context, input json.RawMessage) (string, error) {
	if r.projectMgr == nil {
		return "", fmt.Errorf("project manager not available")
	}
	var p struct {
		ProjectID string `json:"project_id"`
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	proj, ok := r.projectMgr.Get(p.ProjectID)
	if !ok {
		return "", fmt.Errorf("项目 %q 不存在", p.ProjectID)
	}
	if !proj.CanWrite(r.agentID) {
		return "", fmt.Errorf("🚫 权限不足：你没有编辑项目 %q 的权限", p.ProjectID)
	}
	fullPath := filepath.Join(proj.FilesDir, filepath.Clean(p.FilePath))
	if !strings.HasPrefix(fullPath, proj.FilesDir) {
		return "", fmt.Errorf("路径越界")
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, []byte(p.Content), 0644); err != nil {
		return "", fmt.Errorf("写入失败: %w", err)
	}
	return fmt.Sprintf("✅ 已写入 %s/%s", p.ProjectID, p.FilePath), nil
}

// handleProjectGlob lists files in a shared project.
func (r *Registry) handleProjectGlob(_ context.Context, input json.RawMessage) (string, error) {
	if r.projectMgr == nil {
		return "", fmt.Errorf("project manager not available")
	}
	var p struct {
		ProjectID string `json:"project_id"`
		Pattern   string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	proj, ok := r.projectMgr.Get(p.ProjectID)
	if !ok {
		return "", fmt.Errorf("项目 %q 不存在", p.ProjectID)
	}
	pattern := p.Pattern
	if pattern == "" {
		pattern = "*"
	}
	matches, err := filepath.Glob(filepath.Join(proj.FilesDir, pattern))
	if err != nil {
		return "", err
	}
	var lines []string
	for _, m := range matches {
		rel, _ := filepath.Rel(proj.FilesDir, m)
		info, _ := os.Stat(m)
		if info != nil && !info.IsDir() {
			lines = append(lines, fmt.Sprintf("%s (%d bytes)", rel, info.Size()))
		} else {
			lines = append(lines, rel+"/")
		}
	}
	if len(lines) == 0 {
		return "（没有匹配文件）", nil
	}
	return strings.Join(lines, "\n"), nil
}

// resolvePath resolves p relative to the workspace directory.
// Absolute paths are returned unchanged.
func (r *Registry) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.workspaceDir, p)
}

// Definitions returns all tool definitions for inclusion in LLM requests.
func (r *Registry) Definitions() []llm.ToolDef {
	return r.defs
}

// Execute runs the named tool with the given input.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	h, ok := r.handlers[name]
	if !ok {
		var available []string
		for _, d := range r.defs {
			available = append(available, d.Name)
		}
		return "", fmt.Errorf("unknown tool %q — available tools: [%s]", name, strings.Join(available, ", "))
	}
	result, err := h(ctx, input)
	if err != nil {
		// Wrap with tool name so the LLM knows exactly which tool failed
		return result, fmt.Errorf("[%s] %w", name, err)
	}
	return result, nil
}

func (r *Registry) register(def llm.ToolDef, h Handler) {
	// Update existing def if already registered (avoids duplicate tool names)
	for i, d := range r.defs {
		if d.Name == def.Name {
			r.defs[i] = def
			r.handlers[def.Name] = h
			return
		}
	}
	r.defs = append(r.defs, def)
	r.handlers[def.Name] = h
}

// resolveFilePathInInput rewrites "file_path" (and optionally "path") fields
// in a JSON object to be absolute, relative to workspaceDir.
func (r *Registry) resolveFilePathInInput(input json.RawMessage, fields ...string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return input
	}
	for _, field := range fields {
		raw, ok := m[field]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || s == "" {
			continue
		}
		resolved := r.resolvePath(s)
		b, err := json.Marshal(resolved)
		if err == nil {
			m[field] = b
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return input
	}
	return out
}

func (r *Registry) handleReadWS(ctx context.Context, input json.RawMessage) (string, error) {
	return handleRead(ctx, r.resolveFilePathInInput(input, "file_path"))
}

func (r *Registry) handleWriteWS(ctx context.Context, input json.RawMessage) (string, error) {
	return handleWrite(ctx, r.resolveFilePathInInput(input, "file_path"))
}

func (r *Registry) handleEditWS(ctx context.Context, input json.RawMessage) (string, error) {
	return handleEdit(ctx, r.resolveFilePathInInput(input, "file_path"))
}

func (r *Registry) handleGrepWS(ctx context.Context, input json.RawMessage) (string, error) {
	// Default path to workspaceDir if not specified
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err == nil {
		if raw, ok := m["path"]; !ok || string(raw) == `""` || string(raw) == "null" {
			b, _ := json.Marshal(r.workspaceDir)
			m["path"] = b
			if out, err := json.Marshal(m); err == nil {
				input = out
			}
		}
	}
	return handleGrep(ctx, r.resolveFilePathInInput(input, "path"))
}

func (r *Registry) handleGlobWS(ctx context.Context, input json.RawMessage) (string, error) {
	// If base_dir is empty, default to workspaceDir
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err == nil {
		if _, ok := m["base_dir"]; !ok {
			b, _ := json.Marshal(r.workspaceDir)
			m["base_dir"] = b
			if out, err := json.Marshal(m); err == nil {
				input = out
			}
		}
	}
	return handleGlob(ctx, r.resolveFilePathInInput(input, "base_dir"))
}

// handleBashWS runs bash commands in the agent's workspace directory,
// injecting any per-agent env vars (agentEnv) on top of the sanitized system env.
func (r *Registry) handleBashWS(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command    string `json:"command"`
		Timeout    int    `json:"timeout"`
		Background bool   `json:"background"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	// Handle background execution
	if p.Background {
		command := p.Command
		if r.workspaceDir != "" && command != "" {
			command = fmt.Sprintf("cd %q && %s", r.workspaceDir, command)
		}
		id, err := startBackground(command)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Background session started: %s\nUse process(action=poll/log, sessionId=%s) to monitor.", id, id), nil
	}

	// Prepend workspace cd so relative paths work
	command := p.Command
	if r.workspaceDir != "" && command != "" {
		command = fmt.Sprintf("cd %q && %s", r.workspaceDir, command)
	}

	timeout := time.Duration(p.Timeout) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	// Start with sanitized system env, then overlay agent-configured env vars
	env := sanitizeEnv(os.Environ())
	for k, v := range r.agentEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	output := strings.TrimRight(string(out), "\n")
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			// Timeout
			if output != "" {
				return fmt.Sprintf("❌ Command timed out after %v.\n\nPartial output:\n%s", timeout, output), nil
			}
			return fmt.Sprintf("❌ Command timed out after %v (no output).", timeout), nil
		}
		if output != "" {
			return fmt.Sprintf("❌ Command exited with code %d.\n\n%s", exitCode, output), nil
		}
		return fmt.Sprintf("❌ Command exited with code %d (no output).", exitCode), nil
	}
	if output == "" {
		return "(command completed successfully, no output)", nil
	}
	return output, nil
}

// ── Self-Management Handlers ─────────────────────────────────────────────────

func (r *Registry) handleSelfListSkills(_ context.Context, _ json.RawMessage) (string, error) {
	metas, err := skill.ScanSkills(r.workspaceDir)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *Registry) handleSelfInstallSkill(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Icon          string `json:"icon"`
		Category      string `json:"category"`
		Description   string `json:"description"`
		PromptContent string `json:"promptContent"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	meta := skill.Meta{
		ID:          p.ID,
		Name:        p.Name,
		Icon:        p.Icon,
		Category:    p.Category,
		Description: p.Description,
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		Source:      "local",
	}
	if err := skill.WriteSkill(r.workspaceDir, meta); err != nil {
		return "", fmt.Errorf("write skill: %w", err)
	}
	// Write SKILL.md
	skillMdPath := filepath.Join(r.workspaceDir, "skills", p.ID, "SKILL.md")
	promptContent := p.PromptContent
	if promptContent == "" {
		promptContent = fmt.Sprintf("# %s\n\n%s\n", p.Name, p.Description)
	}
	if err := os.WriteFile(skillMdPath, []byte(promptContent), 0644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}
	// Rebuild skills INDEX.md so system prompt injects updated summary
	_ = skill.RebuildIndex(r.workspaceDir)
	return fmt.Sprintf("✅ 技能 \"%s\" 已安装", p.Name), nil
}

func (r *Registry) handleSelfUninstallSkill(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if err := skill.RemoveSkill(r.workspaceDir, p.ID); err != nil {
		return "", err
	}
	// Rebuild skills INDEX.md so system prompt injects updated summary
	_ = skill.RebuildIndex(r.workspaceDir)
	return fmt.Sprintf("✅ 技能 \"%s\" 已卸载", p.ID), nil
}

func (r *Registry) handleSelfRename(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	configPath := filepath.Join(r.agentDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config.json: %w", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse config.json: %w", err)
	}
	cfg["name"] = p.Name
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return "", fmt.Errorf("write config.json: %w", err)
	}
	return fmt.Sprintf("已将名字更改为：%s", p.Name), nil
}

func (r *Registry) handleSelfUpdateSoul(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	soulPath := filepath.Join(r.workspaceDir, "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(p.Content), 0644); err != nil {
		return "", fmt.Errorf("write SOUL.md: %w", err)
	}
	return "SOUL.md 已更新", nil
}

// ── Subagent Tools ────────────────────────────────────────────────────────────

func (r *Registry) handleAgentSpawn(_ context.Context, input json.RawMessage) (string, error) {
	if r.subagentMgr == nil {
		return "", fmt.Errorf("subagent manager not configured — cannot dispatch tasks in this context")
	}
	var p struct {
		AgentID         string `json:"agentId"`
		Task            string `json:"task"`
		Label           string `json:"label"`
		Model           string `json:"model"`
		Timeout         int    `json:"timeout"`
		MaxTurns        int    `json:"max_turns"`
		Isolated        bool   `json:"isolated"`
		Background      string `json:"background"`
		Deliverable     string `json:"deliverable"`
		Priority        string `json:"priority"`
		SharedProjectID string `json:"shared_project_id"`
		Attachments     []struct {
			Name    string `json:"name"`
			Content string `json:"content"`
			Path    string `json:"path"` // workspace-relative path (alternative to content)
		} `json:"attachments"`
		ContextTurns int `json:"context_turns"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %v", err)
	}
	if p.AgentID == "" {
		return "", fmt.Errorf("agentId is required — use agent_list to get available agent IDs")
	}
	if p.Task == "" {
		return "", fmt.Errorf("task is required — provide a detailed task description")
	}
	// Validate agentID against known agents
	var targetIsUserAgent bool // true = user-managed agent (需走关系检查); false = built-in agentType (豁免)
	if r.agentLister != nil {
		agents := r.agentLister()
		for _, a := range agents {
			if a.ID == p.AgentID {
				targetIsUserAgent = true
				break
			}
		}
		if !targetIsUserAgent {
			// 可能是 coordinator 模式的内置 agent type (explore/plan/general-purpose 等)
			// 这些不在 agentLister 里, 豁免关系检查
			builtInAgentTypes := map[string]bool{
				"general-purpose": true,
				"explore":         true,
				"plan":            true,
				"verification":    true,
				"coordinator":     true,
			}
			if !builtInAgentTypes[p.AgentID] {
				var available []string
				for _, a := range agents {
					available = append(available, fmt.Sprintf("%s (id: %s)", a.Name, a.ID))
				}
				hint := strings.Join(available, ", ")
				if hint == "" {
					hint = "（无可用成员）"
				}
				return "", fmt.Errorf("agent %q not found.\nAvailable agents: %s\nUse agent_list for full details.", p.AgentID, hint)
			}
		}
	}

	// ── 关系权限检查 ────────────────────────────────────────────────────
	// 规则: 派遣用户 agent 时, 必须在当前 agent 的 RELATIONS.md 里有记录.
	// 豁免: built-in agent type (上面 targetIsUserAgent==false 的分支)
	if targetIsUserAgent {
		peers := allowedPeersFromRelations(r.workspaceDir)
		if peers == nil || !peers[p.AgentID] {
			return "", fmt.Errorf("❌ 你与成员 %q 之间没有建立关系，无法派遣。\n"+
				"请先前往「团队」页面在成员图谱上给你们建立关系（上下级/平级协作/支持等），\n"+
				"或使用 wish_add 工具把这个需求写入愿望清单告知用户。",
				p.AgentID)
		}
	}

	// Build Brief
	var brief *subagent.TaskBrief
	if p.Background != "" || p.Deliverable != "" || p.Priority != "" {
		brief = &subagent.TaskBrief{
			Background:  p.Background,
			Deliverable: p.Deliverable,
			Priority:    p.Priority,
		}
	}

	// Build Attachments — resolve workspace_file paths if needed
	attachments := make([]subagent.Attachment, 0, len(p.Attachments))
	for _, a := range p.Attachments {
		content := a.Content
		if content == "" && a.Path != "" {
			// Read file from spawner's workspace
			absPath := filepath.Join(r.workspaceDir, filepath.Clean(a.Path))
			data, err := os.ReadFile(absPath)
			if err != nil {
				return "", fmt.Errorf("读取附件文件失败 %q: %v", a.Path, err)
			}
			content = string(data)
			if len(content) > 20000 {
				content = content[:20000] + "\n…（内容已截断，超过20000字符）"
			}
		}
		if content != "" {
			attachments = append(attachments, subagent.Attachment{Name: a.Name, Content: content})
		}
	}

	// Read context snapshot from current session if requested
	var contextSnapshot string
	if p.ContextTurns > 0 && r.subagentMgr != nil {
		contextSnapshot = r.subagentMgr.ReadContext(r.sessionID, p.ContextTurns)
	}

	opts := subagent.SpawnOpts{
		AgentID:          p.AgentID,
		Label:            p.Label,
		Task:             p.Task,
		Model:            p.Model,
		TimeoutSec:       p.Timeout,
		MaxTurns:         p.MaxTurns,
		Isolated:         p.Isolated,
		SpawnedBy:        r.agentID,
		SpawnedBySession: r.sessionID,
		Brief:            brief,
		Attachments:      attachments,
		ContextSnapshot:  contextSnapshot,
		SharedProjectID:  p.SharedProjectID,
	}

	task, err := r.subagentMgr.Spawn(opts)
	if err != nil {
		return "", err
	}

	// Build confirmation message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ 任务已派生\n- 任务 ID: %s\n- 执行者: %s\n- 标签: %s\n- 状态: %s",
		task.ID, task.AgentID, task.Label, task.Status))
	if brief != nil {
		if brief.Priority != "" && brief.Priority != "normal" {
			sb.WriteString("\n- 优先级: " + brief.Priority)
		}
	}
	if len(attachments) > 0 {
		sb.WriteString(fmt.Sprintf("\n- 附件: %d 份参考资料已传递", len(attachments)))
	}
	if p.SharedProjectID != "" {
		sb.WriteString("\n- 共享项目: " + p.SharedProjectID + "（执行方已获写入权限）")
	}
	if contextSnapshot != "" {
		sb.WriteString(fmt.Sprintf("\n- 上下文: 最近 %d 轮对话已注入", p.ContextTurns))
	}
	sb.WriteString("\n\n任务在后台异步执行，使用 agent_tasks 查看状态，agent_result 获取结果。")
	return sb.String(), nil
}

func (r *Registry) handleAgentTasks(_ context.Context, input json.RawMessage) (string, error) {
	if r.subagentMgr == nil {
		return "", fmt.Errorf("subagent manager not configured")
	}
	var p struct {
		AgentID string `json:"agentId"`
		Status  string `json:"status"`
	}
	_ = json.Unmarshal(input, &p)

	tasks := r.subagentMgr.List(p.AgentID)
	if len(tasks) == 0 {
		return "暂无后台任务", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个任务：\n\n", len(tasks)))
	for _, t := range tasks {
		if p.Status != "" && string(t.Status) != p.Status {
			continue
		}
		label := t.Label
		if label == "" {
			label = "(无标签)"
		}
		sb.WriteString(fmt.Sprintf("• [%s] %s | %s | 执行者: %s | 耗时: %s\n  任务: %s\n",
			t.Status, t.ID, label, t.AgentID, t.Duration(),
			truncate(t.Description, 80)))
	}
	return sb.String(), nil
}

func (r *Registry) handleAgentKill(_ context.Context, input json.RawMessage) (string, error) {
	if r.subagentMgr == nil {
		return "", fmt.Errorf("subagent manager not configured")
	}
	var p struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if err := r.subagentMgr.Kill(p.TaskID); err != nil {
		return "", err
	}
	return fmt.Sprintf("任务 %s 已终止", p.TaskID), nil
}

func (r *Registry) handleAgentResult(_ context.Context, input json.RawMessage) (string, error) {
	if r.subagentMgr == nil {
		return "", fmt.Errorf("subagent manager not configured")
	}
	var p struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	task, ok := r.subagentMgr.Get(p.TaskID)
	if !ok {
		return "", fmt.Errorf("任务 %q 不存在", p.TaskID)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("任务 ID: %s\n状态: %s\n执行者: %s\n耗时: %s\n", task.ID, task.Status, task.AgentID, task.Duration()))
	if task.ErrorMsg != "" {
		sb.WriteString(fmt.Sprintf("错误: %s\n", task.ErrorMsg))
	}
	sb.WriteString("\n--- 输出 ---\n")
	if task.Output == "" {
		sb.WriteString("（无输出）")
	} else {
		sb.WriteString(task.Output)
	}
	return sb.String(), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
