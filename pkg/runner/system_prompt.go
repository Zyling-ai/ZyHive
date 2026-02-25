// System prompt builder — assembles identity, soul, memory index into a system prompt.
// Reference: pi-coding-agent/dist/core/agent-session.js (buildSystemPrompt)
package runner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/memory"
	"github.com/Zyling-ai/zyhive/pkg/project"
)

// ── 加载时截断保护 ────────────────────────────────────────────────────────────
// 每个工作区文件注入系统提示词时限制最大字符数，超出则保留头尾，中间插入截断标记。
// 策略：保留头部 70%（最重要的指令）+ 尾部 20%（最新内容），共 90% 可用空间。
const (
	promptFileMaxChars  = 20_000 // 单文件注入上限（字符数，约 5K token）
	promptFileHeadRatio = 0.70
	promptFileTailRatio = 0.20
)

// truncateForPrompt 对注入系统提示词的文件内容按 promptFileMaxChars 截断。
// 如未超限则原样返回。
func truncateForPrompt(content, filename string) string {
	if len(content) <= promptFileMaxChars {
		return content
	}
	headLen := int(float64(promptFileMaxChars) * promptFileHeadRatio)
	tailLen := int(float64(promptFileMaxChars) * promptFileTailRatio)
	head := content[:headLen]
	tail := content[len(content)-tailLen:]
	marker := fmt.Sprintf("\n\n[...内容已截断（原文件 %d 字符），完整内容请用 read 工具读取: %s...]\n\n",
		len(content), filename)
	return head + marker + tail
}

// BuildSystemPrompt reads IDENTITY.md, SOUL.md, and memory/INDEX.md from the
// workspace directory, and returns the full system prompt.
// Only INDEX.md is injected (lightweight). Full memory tree is accessible via tools.
func BuildSystemPrompt(workspaceDir string) (string, error) {
	var sb strings.Builder

	// Inject current date/time in Asia/Shanghai timezone
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	sb.WriteString(fmt.Sprintf("Current date and time: %s\n\n", now.Format("2006-01-02 15:04:05 MST")))

	// injectFile 是内部辅助：读取文件并以截断保护注入到系统提示词。
	injectFile := func(path, label string) {
		content, err := readFileIfExists(path)
		if err != nil || strings.TrimSpace(content) == "" {
			return
		}
		content = truncateForPrompt(strings.TrimSpace(content), label)
		sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", label, content))
	}

	// Read IDENTITY.md and SOUL.md
	for _, filename := range []string{"IDENTITY.md", "SOUL.md"} {
		injectFile(filepath.Join(workspaceDir, filename), filename)
	}

	// Read memory/INDEX.md (lightweight, always injected)
	mt := memory.NewMemoryTree(workspaceDir)
	indexContent, err := mt.GetIndex()
	if err == nil && strings.TrimSpace(indexContent) != "" {
		content := truncateForPrompt(strings.TrimSpace(indexContent), "memory/INDEX.md")
		sb.WriteString(fmt.Sprintf("--- memory/INDEX.md ---\n%s\n\n", content))
	}

	// Legacy: if MEMORY.md still exists and no INDEX.md, include it
	if strings.TrimSpace(indexContent) == "" {
		injectFile(filepath.Join(workspaceDir, "MEMORY.md"), "MEMORY.md")
	}

	// Memory tree hint for the agent
	sb.WriteString("[Memory tree available. Use read tool to access: memory/core/, memory/projects/, memory/daily/, memory/topics/]\n\n")

	// Conversation history hint for the agent
	sb.WriteString("[对话历史可查。使用 read 工具访问: conversations/INDEX.md 查看索引，conversations/{sessionId}__{channelId}.jsonl 查看完整对话]\n\n")

	// Inject RELATIONS.md if it exists
	injectFile(filepath.Join(workspaceDir, "RELATIONS.md"), "RELATIONS.md")

	// Inject skills/INDEX.md (lightweight summary instead of full SKILL.md content)
	injectFile(filepath.Join(workspaceDir, "skills", "INDEX.md"), "skills/INDEX.md")

	// Inject conversations/INDEX.md if it exists
	injectFile(filepath.Join(workspaceDir, "conversations", "INDEX.md"), "conversations/INDEX.md")

	// Read AGENTS.md — if it exists, also read any files it references (one per line)
	agentsContent, err := readFileIfExists(filepath.Join(workspaceDir, "AGENTS.md"))
	if err == nil && agentsContent != "" {
		content := truncateForPrompt(strings.TrimSpace(agentsContent), "AGENTS.md")
		sb.WriteString(fmt.Sprintf("--- AGENTS.md ---\n%s\n\n", content))

		// Parse referenced files from AGENTS.md (lines that look like file paths)
		scanner := bufio.NewScanner(strings.NewReader(agentsContent))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			refPath := line
			if !filepath.IsAbs(refPath) {
				refPath = filepath.Join(workspaceDir, refPath)
			}
			refContent, err := readFileIfExists(refPath)
			if err == nil && strings.TrimSpace(refContent) != "" {
				refContent = truncateForPrompt(strings.TrimSpace(refContent), line)
				sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", line, refContent))
			}
		}
	}

	return sb.String(), nil
}

// readFileIfExists reads a file and returns its content, or empty string if not found.
func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// BuildProjectContext builds the shared project workspace context string for system prompt injection.
// agentID is used to determine write permissions per project.
func BuildProjectContext(mgr *project.Manager, agentID string) string {
	if mgr == nil {
		return ""
	}
	projects := mgr.List()
	if len(projects) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- 共享团队项目工作区 ---\n")
	sb.WriteString("你可以使用 project_list / project_read / project_write / project_glob 工具访问以下项目：\n\n")

	for _, p := range projects {
		perm := "可读写"
		if !p.CanWrite(agentID) {
			perm = "只读"
		}
		sb.WriteString(fmt.Sprintf("• **%s** (id: `%s`, 权限: %s)", p.Name, p.ID, perm))
		if p.Description != "" {
			sb.WriteString(fmt.Sprintf(" — %s", p.Description))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n工具：project_create 新建项目，project_list 列出项目，project_read 读取文件，project_write 写入文件（需写入权限），project_glob 列举文件。")
	return sb.String()
}
