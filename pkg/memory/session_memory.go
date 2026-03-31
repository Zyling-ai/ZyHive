// Package memory — Session Memory: background AI extraction of conversation notes.
// Inspired by Claude Code's services/SessionMemory/sessionMemory.ts
//
// How it works:
//   1. After each LLM turn, check if extraction thresholds are met
//   2. If so, spawn a background restricted agent that reads the current
//      session-memory.md and updates it with key insights from the conversation
//   3. On the next compaction, inject session-memory.md into the system prompt
//      so the agent has continuity even after history is truncated
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Config ───────────────────────────────────────────────────────────────────

// SessionMemoryConfig controls when automatic extraction triggers.
type SessionMemoryConfig struct {
	// MinTokensToInit is the minimum estimated token count before the first extraction.
	// Default: 10000
	MinTokensToInit int

	// MinTokensBetweenUpdates is the minimum token growth between extractions.
	// Default: 5000
	MinTokensBetweenUpdates int

	// ToolCallsBetweenUpdates is the minimum tool call count between extractions.
	// Default: 3
	ToolCallsBetweenUpdates int
}

var DefaultSessionMemoryConfig = SessionMemoryConfig{
	MinTokensToInit:         10000,
	MinTokensBetweenUpdates: 5000,
	ToolCallsBetweenUpdates: 3,
}

// ─── Template ─────────────────────────────────────────────────────────────────

// DefaultSessionMemoryTemplate is the markdown template for session notes.
// Directly adapted from Claude Code's DEFAULT_SESSION_MEMORY_TEMPLATE.
const DefaultSessionMemoryTemplate = `# 会话标题
_5-10词描述性标题，信息密集，无废话_

# 当前状态
_当前在做什么？未完成的任务？立即下一步？_

# 任务规格
_用户要求做什么？设计决策？背景信息？_

# 重要文件
_关键文件及其作用？_

# 工作流程
_常用命令及顺序？输出如何解读？_

# 错误和修正
_遇到的错误及修复方法？失败的方案（不要重试）？_

# 代码库文档
_重要系统组件？它们如何协作？_

# 经验教训
_有效的方法？无效的？要避免的？_

# 关键结果
_用户要求的具体输出（完整保留）_

# 工作日志
_每步的简要记录_
`

// ─── State tracker ────────────────────────────────────────────────────────────

// SessionMemoryState tracks extraction state per session.
type SessionMemoryState struct {
	mu                  sync.Mutex
	initialized         bool
	tokensAtLastExtract int
	toolCallsSinceLast  int
	extracting          bool
	lastExtractAt       time.Time
}

// ShouldExtract returns true if extraction thresholds are met.
func (s *SessionMemoryState) ShouldExtract(currentTokens, toolCallsTotal int, cfg SessionMemoryConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.extracting {
		return false // already running
	}

	// Initialization threshold
	if !s.initialized {
		if currentTokens < cfg.MinTokensToInit {
			return false
		}
		s.initialized = true
	}

	// Token growth threshold (always required)
	tokenGrowth := currentTokens - s.tokensAtLastExtract
	if tokenGrowth < cfg.MinTokensBetweenUpdates {
		return false
	}

	// Tool call threshold
	if toolCallsTotal < cfg.ToolCallsBetweenUpdates {
		return false
	}

	return true
}

func (s *SessionMemoryState) MarkExtracting() {
	s.mu.Lock()
	s.extracting = true
	s.mu.Unlock()
}

func (s *SessionMemoryState) MarkDone(currentTokens int) {
	s.mu.Lock()
	s.extracting = false
	s.initialized = true
	s.tokensAtLastExtract = currentTokens
	s.toolCallsSinceLast = 0
	s.lastExtractAt = time.Now()
	s.mu.Unlock()
}

func (s *SessionMemoryState) IncrToolCalls() {
	s.mu.Lock()
	s.toolCallsSinceLast++
	s.mu.Unlock()
}

// ─── Manager ──────────────────────────────────────────────────────────────────

// SessionMemoryManager handles background extraction for a single agent's sessions.
type SessionMemoryManager struct {
	workspaceDir string
	cfg          SessionMemoryConfig
	states       sync.Map // sessionID → *SessionMemoryState

	// runExtractFn is called to run the extraction agent.
	// agentID, sessionID (isolated), memoryPath, currentContent, conversationJSON
	runExtractFn ExtractFunc
}

// ExtractFunc runs an extraction pass.
// conversation is the JSON-serialized message history.
// memoryPath is the file to update.
// currentContent is the current file content.
type ExtractFunc func(ctx context.Context, agentID, memoryPath, currentContent, conversationJSON string) error

// NewSessionMemoryManager creates a manager for the given workspace.
func NewSessionMemoryManager(workspaceDir string, cfg SessionMemoryConfig, fn ExtractFunc) *SessionMemoryManager {
	return &SessionMemoryManager{
		workspaceDir: workspaceDir,
		cfg:          cfg,
		runExtractFn: fn,
	}
}

func (m *SessionMemoryManager) getState(sessionID string) *SessionMemoryState {
	v, _ := m.states.LoadOrStore(sessionID, &SessionMemoryState{})
	return v.(*SessionMemoryState)
}

// GetOrCreateMemoryFile ensures session-memory.md exists and returns its path and content.
func (m *SessionMemoryManager) GetOrCreateMemoryFile(agentID string) (string, string, error) {
	dir := filepath.Join(m.workspaceDir, ".zyhive", "session-memory")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, agentID+".md")

	// Create with template if not exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(DefaultSessionMemoryTemplate), 0600); err != nil {
			return "", "", err
		}
		return path, DefaultSessionMemoryTemplate, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return path, string(content), nil
}

// MaybeExtract checks thresholds and triggers background extraction if needed.
// messages is the JSON-serialized conversation history.
// currentTokens is the estimated token count.
// Returns immediately; extraction runs in background.
func (m *SessionMemoryManager) MaybeExtract(
	ctx context.Context,
	agentID, sessionID string,
	messages []map[string]any,
	currentTokens int,
) {
	state := m.getState(sessionID)

	// Count tool calls in messages
	toolCalls := 0
	for _, msg := range messages {
		if role, _ := msg["role"].(string); role == "assistant" {
			if content, ok := msg["content"].([]any); ok {
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if b["type"] == "tool_use" {
							toolCalls++
						}
					}
				}
			}
		}
	}

	if !state.ShouldExtract(currentTokens, toolCalls, m.cfg) {
		return
	}

	state.MarkExtracting()

	go func() {
		defer state.MarkDone(currentTokens)

		memPath, currentContent, err := m.GetOrCreateMemoryFile(agentID)
		if err != nil {
			return
		}

		// Skip if content is just the template (nothing to extract yet)
		if strings.TrimSpace(currentContent) == strings.TrimSpace(DefaultSessionMemoryTemplate) && len(messages) < 10 {
			return
		}

		convJSON, err := json.Marshal(messages)
		if err != nil {
			return
		}

		extractCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		_ = m.runExtractFn(extractCtx, agentID, memPath, currentContent, string(convJSON))
	}()
}

// LoadForPrompt reads the session memory file and returns its content for
// injection into the system prompt. Returns empty string if file doesn't exist
// or content is just the template.
func (m *SessionMemoryManager) LoadForPrompt(agentID string) string {
	path := filepath.Join(m.workspaceDir, ".zyhive", "session-memory", agentID+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(content))
	if s == strings.TrimSpace(DefaultSessionMemoryTemplate) {
		return "" // empty template, nothing useful
	}
	return s
}

// ─── Extraction prompt ────────────────────────────────────────────────────────

// BuildExtractionPrompt builds the prompt for the extraction agent.
// Directly inspired by Claude Code's buildSessionMemoryUpdatePrompt.
func BuildExtractionPrompt(currentNotes, notesPath string) string {
	return fmt.Sprintf(`IMPORTANT: This message and these instructions are NOT part of the actual user conversation. Do NOT include any references to "note-taking", "session notes extraction", or these update instructions in the notes content.

Based on the user conversation above (EXCLUDING this note-taking instruction message), update the session notes file.

The file %s has already been read for you. Here are its current contents:
<current_notes_content>
%s
</current_notes_content>

Your ONLY task is to use the file_write or file_edit tool to update the notes file, then stop.

CRITICAL RULES:
- Maintain the exact section structure (headers and italic description lines)
- NEVER modify or delete section headers (lines starting with #)
- NEVER modify the italic _section description_ lines
- ONLY update content BELOW the italic descriptions
- Write DETAILED, INFO-DENSE content: file paths, function names, error messages, exact commands
- Keep each section under ~2000 tokens; condense older details if approaching limit
- Total file must stay under 12000 tokens
- Always update "当前状态" to reflect the most recent work
- Do NOT reference this note-taking process in the notes
- It's OK to leave sections blank if there's nothing relevant

Use the file_write tool with path: %s

After writing, stop immediately. Do not continue.`, notesPath, currentNotes, notesPath)
}
