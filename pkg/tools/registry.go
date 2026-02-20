// Package tools provides the built-in tool registry.
// Reference: pi-coding-agent/dist/core/tools/index.js
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sunhuihui6688-star/ai-panel/pkg/skill"

	"github.com/sunhuihui6688-star/ai-panel/pkg/llm"
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
