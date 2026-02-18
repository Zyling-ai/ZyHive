// System prompt builder — assembles identity, soul, memory, and agent files into a system prompt.
// Reference: pi-coding-agent/dist/core/agent-session.js (buildSystemPrompt)
package runner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildSystemPrompt reads IDENTITY.md, SOUL.md, MEMORY.md, and optionally files
// listed in AGENTS.md from the workspace directory, and returns the full system prompt.
// Missing files are silently skipped.
func BuildSystemPrompt(workspaceDir string) (string, error) {
	var sb strings.Builder

	// Inject current date/time in Asia/Shanghai timezone
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	sb.WriteString(fmt.Sprintf("Current date and time: %s\n\n", now.Format("2006-01-02 15:04:05 MST")))

	// Read core identity files in order
	for _, filename := range []string{"IDENTITY.md", "SOUL.md", "MEMORY.md"} {
		content, err := readFileIfExists(filepath.Join(workspaceDir, filename))
		if err != nil || content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", filename, strings.TrimSpace(content)))
	}

	// Read AGENTS.md — if it exists, also read any files it references (one per line)
	agentsContent, err := readFileIfExists(filepath.Join(workspaceDir, "AGENTS.md"))
	if err == nil && agentsContent != "" {
		sb.WriteString(fmt.Sprintf("--- AGENTS.md ---\n%s\n\n", strings.TrimSpace(agentsContent)))

		// Parse referenced files from AGENTS.md (lines that look like file paths)
		scanner := bufio.NewScanner(strings.NewReader(agentsContent))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			// Try to read the referenced file relative to workspace
			refPath := line
			if !filepath.IsAbs(refPath) {
				refPath = filepath.Join(workspaceDir, refPath)
			}
			refContent, err := readFileIfExists(refPath)
			if err == nil && refContent != "" {
				sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", line, strings.TrimSpace(refContent)))
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
