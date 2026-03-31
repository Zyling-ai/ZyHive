// Package agent — AgentDefinition: unified agent definition structure.
// Inspired by Claude Code's tools/AgentTool/loadAgentsDir.ts AgentDefinition type.
package agent

// AgentDefinition describes how a specialized agent should behave.
// Agents can be defined in .zyhive/agents/*.md (YAML frontmatter + system prompt body)
// or registered programmatically as built-in agents.
type AgentDefinition struct {
	// AgentType is the unique identifier (e.g. "general-purpose", "researcher").
	AgentType string `json:"agentType" yaml:"agentType"`

	// WhenToUse describes when this agent should be chosen.
	// Shown to the LLM so it can select the right agent.
	WhenToUse string `json:"whenToUse" yaml:"whenToUse"`

	// Description is a human-readable description shown in the UI.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// ── Tool permissions ──────────────────────────────────────────────────────

	// Tools lists allowed tool names. Use ["*"] to allow all.
	// If empty, inherits the default tool set.
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`

	// DisallowedTools lists tool names explicitly forbidden for this agent.
	DisallowedTools []string `json:"disallowedTools,omitempty" yaml:"disallowedTools,omitempty"`

	// ── Execution ─────────────────────────────────────────────────────────────

	// Model overrides the default model. "inherit" = use parent's model.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// MaxTurns caps the number of LLM turns (0 = no limit).
	MaxTurns int `json:"maxTurns,omitempty" yaml:"maxTurns,omitempty"`

	// PermissionMode controls how permission prompts are handled.
	// "" | "default" | "plan" | "acceptEdits" | "bypassPermissions"
	PermissionMode string `json:"permissionMode,omitempty" yaml:"permissionMode,omitempty"`

	// Background makes this agent always run in the background when spawned.
	Background bool `json:"background,omitempty" yaml:"background,omitempty"`

	// ── Context optimization ──────────────────────────────────────────────────

	// OmitProjectFiles skips injecting CLAUDE.md/project files into the system prompt.
	// Use for read-only agents (research, plan) that don't need commit/PR guidelines.
	// Saves significant tokens on high-volume spawns.
	OmitProjectFiles bool `json:"omitProjectFiles,omitempty" yaml:"omitProjectFiles,omitempty"`

	// CriticalSystemReminder is a short string re-injected at every user turn.
	// Use for constraints that must never be forgotten (e.g. "do not modify files").
	CriticalSystemReminder string `json:"criticalSystemReminder,omitempty" yaml:"criticalSystemReminder,omitempty"`

	// ── Memory and skills ─────────────────────────────────────────────────────

	// Skills lists skill names to preload when this agent starts.
	Skills []string `json:"skills,omitempty" yaml:"skills,omitempty"`

	// Memory sets the persistent memory scope for this agent.
	// "user" (~/.zyhive/agent-memory/), "project" (.zyhive/agent-memory/), "local"
	Memory string `json:"memory,omitempty" yaml:"memory,omitempty"`

	// ── Initial prompt ────────────────────────────────────────────────────────

	// InitialPrompt is prepended to the first user turn.
	// Useful for loading skill content or injecting bootstrapping context.
	InitialPrompt string `json:"initialPrompt,omitempty" yaml:"initialPrompt,omitempty"`

	// ── Source ────────────────────────────────────────────────────────────────

	// Source indicates where this definition came from.
	// "built-in" | "user" | "project" | "plugin"
	Source string `json:"source" yaml:"source"`

	// SystemPrompt is the agent's base system prompt (body of the .md file).
	// For built-in agents this is set programmatically.
	SystemPrompt string `json:"systemPrompt,omitempty" yaml:"-"`
}

// ─── Built-in Agent Definitions ──────────────────────────────────────────────
// Mirrors Claude Code's built-in agent types.

var BuiltInAgentDefinitions = []AgentDefinition{
	{
		AgentType: "general-purpose",
		WhenToUse: "通用 Agent，用于研究复杂问题、搜索代码、执行多步任务。当你不确定在哪里找到某个关键字或文件时，使用此 Agent 进行搜索。",
		Tools:     []string{"*"},
		Source:    "built-in",
		SystemPrompt: `你是一个通用 Agent。根据用户的消息，使用可用的工具完成任务。
彻底完成任务——不镀金，但也不半途而废。

**优势：**
- 在大型代码库中搜索代码、配置和模式
- 分析多个文件以理解系统架构
- 调查需要探索许多文件的复杂问题
- 执行多步研究任务

**准则：**
- 文件搜索：不知道位置时广泛搜索
- 分析：先宏观后细节，尝试多种搜索策略
- 要彻底：检查多个位置，考虑不同命名约定
- 除非绝对必要，否则不创建文件
- 完成后，简洁报告完成的内容和关键发现`,
	},
	{
		AgentType:        "explore",
		WhenToUse:        "只读探索 Agent，用于代码库调研和理解，不修改任何文件。",
		Tools:            []string{"Read", "Glob", "Grep", "WebSearch", "WebFetch"},
		OmitProjectFiles: true, // 只读Agent不需要提交/PR规范，省token
		CriticalSystemReminder: "你是只读探索 Agent。禁止创建、修改或删除任何文件。",
		Source:           "built-in",
		SystemPrompt: `你是一个探索 Agent，专门用于代码库调研。
**严格只读**：不得创建、修改或删除任何文件。
报告你的发现，不执行任何修改操作。`,
	},
	{
		AgentType:        "plan",
		WhenToUse:        "规划 Agent，制定详细实施方案。只读，不执行任何修改。",
		Tools:            []string{"Read", "Glob", "Grep"},
		OmitProjectFiles: true,
		PermissionMode:   "plan",
		CriticalSystemReminder: "你是规划 Agent。只制定计划，不执行修改。",
		Source:           "built-in",
		SystemPrompt: `你是一个规划 Agent。
阅读代码库，制定详细、可执行的实施计划。
包含具体文件路径、函数名、修改内容。
**不执行任何修改**——只输出计划。`,
	},
	{
		AgentType: "verification",
		WhenToUse: "验证 Agent，独立验证其他 Agent 的工作成果。用新鲜视角检验，不继承实现 Agent 的假设。",
		Tools:     []string{"Read", "Glob", "Grep", "Bash"},
		Source:    "built-in",
		SystemPrompt: `你是一个验证 Agent。
**真正验证**意味着证明代码有效，而不仅仅是确认它存在。

- 运行测试并关注失败
- 运行类型检查并调查错误
- 持怀疑态度——看起来不对就深入调查
- 独立验证——证明变更有效，不要橡皮图章
- 尝试边缘情况和错误路径`,
	},
	{
		AgentType: "coordinator",
		WhenToUse: "协调者 Agent，负责分解复杂任务、指挥多个 Worker 并行工作、综合结果。",
		Tools:     []string{"dispatch_task", "send_message_to_agent"},
		Source:    "built-in",
	},
}

// GetBuiltInAgent returns a built-in agent definition by type name.
// Returns nil if not found.
func GetBuiltInAgent(agentType string) *AgentDefinition {
	for i := range BuiltInAgentDefinitions {
		if BuiltInAgentDefinitions[i].AgentType == agentType {
			cp := BuiltInAgentDefinitions[i]
			return &cp
		}
	}
	return nil
}

// IsReadOnlyAgent returns true if this agent type is known to be read-only.
// Read-only agents skip project file injection to save tokens.
func (d *AgentDefinition) IsReadOnlyAgent() bool {
	return d.OmitProjectFiles ||
		d.AgentType == "explore" ||
		d.AgentType == "plan"
}

// EffectiveModel returns the model to use for this agent.
// "inherit" or "" means use the spawner's model.
func (d *AgentDefinition) EffectiveModel(parentModel string) string {
	if d.Model == "" || d.Model == "inherit" {
		return parentModel
	}
	return d.Model
}
