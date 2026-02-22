// Built-in tool implementations.
// Reference: pi-coding-agent/dist/core/tools/read.js, write.js, edit.js, bash.js, grep.js
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	lllm "github.com/sunhuihui6688-star/ai-panel/pkg/llm"
)

// ── Read ────────────────────────────────────────────────────────────────────

var readToolDef = lllm.ToolDef{
	Name:        "read",
	Description: "Read the contents of a file. Supports text files. Output is truncated to 2000 lines or 50KB.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_path":{"type":"string","description":"Path to the file to read"},
			"offset":{"type":"number","description":"Line number to start reading from (1-indexed)"},
			"limit":{"type":"number","description":"Maximum number of lines to read"}
		},
		"required":["file_path"]
	}`),
}

func handleRead(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start, end := 0, len(lines)
	if p.Offset > 0 {
		start = p.Offset - 1
	}
	if p.Limit > 0 && start+p.Limit < end {
		end = start + p.Limit
	}
	if start > len(lines) {
		return "", fmt.Errorf("offset %d exceeds file length %d", p.Offset, len(lines))
	}
	return strings.Join(lines[start:end], "\n"), nil
}

// ── Write ───────────────────────────────────────────────────────────────────

var writeToolDef = lllm.ToolDef{
	Name:        "write",
	Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_path":{"type":"string"},
			"content":{"type":"string"}
		},
		"required":["file_path","content"]
	}`),
}

func handleWrite(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p.FilePath), 0755); err != nil {
		return "", err
	}
	return fmt.Sprintf("Written %d bytes to %s", len(p.Content), p.FilePath),
		os.WriteFile(p.FilePath, []byte(p.Content), 0644)
}

// ── Edit ────────────────────────────────────────────────────────────────────

var editToolDef = lllm.ToolDef{
	Name:        "edit",
	Description: "Edit a file by replacing exact text. The old_string must match exactly (including whitespace).",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"file_path":{"type":"string"},
			"old_string":{"type":"string"},
			"new_string":{"type":"string"}
		},
		"required":["file_path","old_string","new_string"]
	}`),
}

func handleEdit(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return "", err
	}
	src := string(data)
	if !strings.Contains(src, p.OldString) {
		return "", fmt.Errorf("old_string not found in %s", p.FilePath)
	}
	result := strings.Replace(src, p.OldString, p.NewString, 1)
	return fmt.Sprintf("Replaced 1 occurrence in %s", p.FilePath),
		os.WriteFile(p.FilePath, []byte(result), 0644)
}

// ── Bash ────────────────────────────────────────────────────────────────────

var bashToolDef = lllm.ToolDef{
	Name:        "exec",
	Description: "Execute shell commands. Times out after 120 seconds.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"command":{"type":"string","description":"Shell command to execute"},
			"timeout":{"type":"number","description":"Timeout in seconds (max 120)"}
		},
		"required":["command"]
	}`),
}

func handleBash(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	timeout := time.Duration(p.Timeout) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
	// Pass sanitized environment — strip API keys, tokens, secrets
	cmd.Env = sanitizeEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w\n%s", err, out)
	}
	return string(out), nil
}

// sanitizeEnv removes sensitive env vars (API keys, secrets, tokens) from the
// environment passed to agent subprocesses.
func sanitizeEnv(env []string) []string {
	blocked := []string{
		"_API_KEY", "_SECRET", "_TOKEN", "_PASSWORD", "_PASSWD",
		"_PRIVATE_KEY", "_ACCESS_KEY", "_AUTH_KEY", "ANTHROPIC_", "OPENAI_",
		"DEEPSEEK_", "OPENROUTER_",
	}
	result := make([]string, 0, len(env))
	for _, e := range env {
		sensitive := false
		upper := strings.ToUpper(e)
		for _, b := range blocked {
			if strings.Contains(upper, b) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			result = append(result, e)
		}
	}
	return result
}

// ── Grep ────────────────────────────────────────────────────────────────────

var grepToolDef = lllm.ToolDef{
	Name:        "grep",
	Description: "Search for a pattern in files using regular expressions.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"pattern":{"type":"string","description":"Regular expression pattern"},
			"path":{"type":"string","description":"File or directory to search"},
			"recursive":{"type":"boolean"}
		},
		"required":["pattern","path"]
	}`),
}

func handleGrep(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Pattern   string `json:"pattern"`
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	args := []string{"-n"}
	if p.Recursive {
		args = append(args, "-r")
	}
	_ = re // use stdlib grep via exec for now
	cmd := exec.Command("grep", append(args, p.Pattern, p.Path)...)
	out, _ := cmd.CombinedOutput()
	return string(out), nil
}

// ── Glob ────────────────────────────────────────────────────────────────────

var globToolDef = lllm.ToolDef{
	Name:        "glob",
	Description: "Find files matching a glob pattern.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"pattern":{"type":"string","description":"Glob pattern, e.g. **/*.go"},
			"base_dir":{"type":"string"}
		},
		"required":["pattern"]
	}`),
}

func handleGlob(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		BaseDir string `json:"base_dir"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	base := p.BaseDir
	if base == "" {
		base = "."
	}
	matches, err := filepath.Glob(filepath.Join(base, p.Pattern))
	if err != nil {
		return "", err
	}
	return strings.Join(matches, "\n"), nil
}

// ── Web Fetch ────────────────────────────────────────────────────────────────

var webFetchToolDef = lllm.ToolDef{
	Name:        "web_fetch",
	Description: "Fetch and extract readable content from a URL.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"url":{"type":"string"},
			"max_chars":{"type":"number"}
		},
		"required":["url"]
	}`),
}

func handleWebFetch(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		URL      string `json:"url"`
		MaxChars int    `json:"max_chars"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(p.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	maxChars := p.MaxChars
	if maxChars <= 0 {
		maxChars = 50000
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxChars)))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ── Self-Management Tools ────────────────────────────────────────────────────
// These tools let an agent manage its own skills, name, and soul.

var selfListSkillsDef = lllm.ToolDef{
	Name:        "self_list_skills",
	Description: "列出当前 Agent 已安装的所有技能。",
	InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
}

var selfInstallSkillDef = lllm.ToolDef{
	Name:        "self_install_skill",
	Description: "为当前 Agent 安装一个新技能。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"id":{"type":"string","description":"技能唯一ID，如 translate"},
			"name":{"type":"string","description":"技能名称"},
			"icon":{"type":"string","description":"图标 emoji"},
			"category":{"type":"string","description":"分类"},
			"description":{"type":"string","description":"技能描述"},
			"promptContent":{"type":"string","description":"注入系统提示的内容 (SKILL.md)，可选"}
		},
		"required":["id","name"]
	}`),
}

var selfUninstallSkillDef = lllm.ToolDef{
	Name:        "self_uninstall_skill",
	Description: "卸载当前 Agent 的指定技能。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"id":{"type":"string","description":"要卸载的技能ID"}
		},
		"required":["id"]
	}`),
}

var showImageDef = lllm.ToolDef{
	Name:        "show_image",
	Description: "在对话中显示一张图片或截图文件（支持 png/jpg/gif/webp）。调用后图片会直接展示在用户的聊天界面里。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"path":{"type":"string","description":"图片文件的绝对路径，例如 /tmp/screenshot.png"}
		},
		"required":["path"]
	}`),
}

func handleShowImage(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil || p.Path == "" {
		return "", fmt.Errorf("path required")
	}
	// Verify file exists and is a supported image type
	ext := strings.ToLower(filepath.Ext(p.Path))
	allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}
	if !allowed[ext] {
		return "", fmt.Errorf("unsupported file type %q; use png/jpg/gif/webp", ext)
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		return "", fmt.Errorf("file not found: %v", err)
	}
	// Return a media marker that AiChat.vue will render as an <img>
	return fmt.Sprintf("[media:%s] (%.1f KB)", p.Path, float64(info.Size())/1024), nil
}

// ── Send File ────────────────────────────────────────────────────────────────

const fileSizeLimit50MB = 50 * 1024 * 1024 // 50 MB

var sendFileDef = lllm.ToolDef{
	Name: "send_file",
	Description: "将本地文件发送给用户。支持所有文件类型（图片/视频/音频/文档/压缩包等）。" +
		"文件 ≤50 MB 直接发送；>50 MB 自动生成临时下载链接并返回。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"path":{"type":"string","description":"要发送的文件的绝对路径，例如 /tmp/report.pdf"}
		},
		"required":["path"]
	}`),
}

// handleSendFile implements the send_file tool.
// The tool is only registered when r.fileSender is non-nil (i.e. there is an active chat channel).
func (r *Registry) handleSendFile(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil || p.Path == "" {
		return "", fmt.Errorf("path required")
	}

	info, err := os.Stat(p.Path)
	if err != nil {
		return "", fmt.Errorf("file not found: %v", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	size := info.Size()
	baseName := filepath.Base(p.Path)

	// Files > 50 MB: generate download link instead of uploading
	if size > fileSizeLimit50MB {
		mb := float64(size) / (1024 * 1024)
		if r.serverBaseURL != "" && r.authToken != "" {
			dlURL := r.serverBaseURL + "/api/download?path=" + url.QueryEscape(p.Path) +
				"&token=" + url.QueryEscape(r.authToken)
			return fmt.Sprintf("⚠️ 文件 %s 过大 (%.1f MB，超过50MB限制)，无法直接发送。\n\n下载链接：%s", baseName, mb, dlURL), nil
		}
		return fmt.Sprintf("⚠️ 文件 %s 过大 (%.1f MB)，超过50MB限制，无法直接发送。\n文件路径：%s", baseName, mb, p.Path), nil
	}

	// Delegate to the channel's file sender (e.g. Telegram sendDocument/sendPhoto)
	if r.fileSender == nil {
		return "", fmt.Errorf("send_file: no active channel to deliver file")
	}
	return r.fileSender(p.Path)
}

// ── Self Env Vars ────────────────────────────────────────────────────────────

var selfSetEnvDef = lllm.ToolDef{
	Name:        "self_set_env",
	Description: "设置或更新当前 Agent 自己的环境变量（立即生效并持久化到 config.json）。下次会话即可用 os.Getenv 读取。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"key":{"type":"string","description":"环境变量名，如 WECHAT_APP_ID"},
			"value":{"type":"string","description":"环境变量值"}
		},
		"required":["key","value"]
	}`),
}

var selfDeleteEnvDef = lllm.ToolDef{
	Name:        "self_delete_env",
	Description: "删除当前 Agent 自己的某个环境变量。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"key":{"type":"string","description":"要删除的环境变量名"}
		},
		"required":["key"]
	}`),
}

func (r *Registry) handleSelfSetEnv(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(input, &p); err != nil || p.Key == "" {
		return "", fmt.Errorf("key and value required")
	}
	if r.envUpdater == nil {
		return "", fmt.Errorf("env update not available in this context")
	}
	if err := r.envUpdater(p.Key, p.Value, false); err != nil {
		return "", fmt.Errorf("set env %s: %w", p.Key, err)
	}
	// Also update the in-memory agentEnv so the current session sees it immediately
	if r.agentEnv == nil {
		r.agentEnv = make(map[string]string)
	}
	r.agentEnv[p.Key] = p.Value
	return fmt.Sprintf("✅ 已设置环境变量 %s（已持久化到 config.json，当前会话立即生效）", p.Key), nil
}

func (r *Registry) handleSelfDeleteEnv(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(input, &p); err != nil || p.Key == "" {
		return "", fmt.Errorf("key required")
	}
	if r.envUpdater == nil {
		return "", fmt.Errorf("env update not available in this context")
	}
	if err := r.envUpdater(p.Key, "", true); err != nil {
		return "", fmt.Errorf("delete env %s: %w", p.Key, err)
	}
	delete(r.agentEnv, p.Key)
	return fmt.Sprintf("✅ 已删除环境变量 %s", p.Key), nil
}

var selfRenameDef = lllm.ToolDef{
	Name:        "self_rename",
	Description: "修改当前 Agent 的名字。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"新名字"}
		},
		"required":["name"]
	}`),
}

var selfUpdateSoulDef = lllm.ToolDef{
	Name:        "self_update_soul",
	Description: "更新当前 Agent 的灵魂设定 (SOUL.md)。",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"content":{"type":"string","description":"新的 SOUL.md 内容"}
		},
		"required":["content"]
	}`),
}
