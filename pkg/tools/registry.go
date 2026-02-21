// Package tools provides the built-in tool registry.
// Reference: pi-coding-agent/dist/core/tools/index.js
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
	"github.com/sunhuihui6688-star/ai-panel/pkg/project"
	"github.com/sunhuihui6688-star/ai-panel/pkg/skill"
)

// Handler executes a tool call and returns the result string.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Registry maps tool names to their definition and handler.
type Registry struct {
	defs         []llm.ToolDef
	handlers     map[string]Handler
	workspaceDir string // agent-specific working directory for path resolution
	agentDir     string // parent dir of workspace (contains config.json)
	agentID      string // agent ID (used for self-management tools)
	projectMgr   *project.Manager // shared project workspace (nil = no project access)
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
	r.register(grepToolDef, r.handleGrepWS)
	r.register(globToolDef, r.handleGlobWS)
	r.register(webFetchToolDef, handleWebFetch)
	// Self-management tools (available to all agents)
	r.register(selfListSkillsDef, r.handleSelfListSkills)
	r.register(selfInstallSkillDef, r.handleSelfInstallSkill)
	r.register(selfUninstallSkillDef, r.handleSelfUninstallSkill)
	r.register(selfRenameDef, r.handleSelfRename)
	r.register(selfUpdateSoulDef, r.handleSelfUpdateSoul)
	return r
}

// NewSkillStudio creates a sandboxed Registry for the SkillStudio AI.
// File operations are restricted to skills/{skillID}/ only.
// Dangerous tools (self_install_skill, self_uninstall_skill, self_rename, self_update_soul, bash) are disabled.
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
	// List skills is read-only, allow it
	r.register(selfListSkillsDef, r.handleSelfListSkills)
	// Bash, self_install_skill, self_uninstall_skill, self_rename, self_update_soul: NOT registered (disabled)
	return r
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
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return h(ctx, input)
}

func (r *Registry) register(def llm.ToolDef, h Handler) {
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

// handleBashWS runs bash commands in the agent's workspace directory.
func (r *Registry) handleBashWS(ctx context.Context, input json.RawMessage) (string, error) {
	// Inject workspace dir as cwd by prepending a cd command
	var p struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return handleBash(ctx, input)
	}
	if r.workspaceDir != "" && p.Command != "" {
		p.Command = fmt.Sprintf("cd %q && %s", r.workspaceDir, p.Command)
		modified, err := json.Marshal(p)
		if err == nil {
			return handleBash(ctx, modified)
		}
	}
	return handleBash(ctx, input)
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
