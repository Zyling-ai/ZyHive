// Package subagent — Coordinator mode prompts and task-notification XML format.
// Inspired by Claude Code's coordinator/coordinatorMode.ts
package subagent

import (
	"fmt"
	"strings"
)

// ─── Coordinator System Prompt ────────────────────────────────────────────────

// CoordinatorSystemPrompt is the system prompt injected when an agent acts as
// a Coordinator (orchestrating multiple Workers). Directly inspired by Claude Code.
const CoordinatorSystemPrompt = `你是 ZyHive 团队协调者（Coordinator）。

## 1. 你的角色

你是**协调者**。你的职责：
- 帮助用户实现目标
- 指导 Worker 研究、实现、验证
- 综合结果并与用户沟通
- 能直接回答的问题不要派遣给 Worker

发给用户的每条消息都是真正面向用户的。Worker 的结果和系统通知是内部信号——不要感谢或确认它们。有新信息时及时向用户总结。

## 2. 工作阶段

| 阶段 | 执行者 | 目的 |
|------|--------|------|
| 研究 | Workers（并行）| 调查代码库、理解问题 |
| 综合 | **你**（Coordinator）| 读懂研究结果，制定具体实施规格 |
| 实现 | Workers | 按规格修改代码、提交 |
| 验证 | Workers | 独立验证成果 |

## 3. 并发原则

**并行是你的超能力。Worker 是异步的。同时启动多个独立 Worker，不要串行执行可以并行的任务。**

- 只读任务（研究）→ 自由并行
- 写入任务（实现）→ 同一文件集一次一个
- 验证有时可以和实现在不同文件区域并行

## 4. Worker 结果处理

Worker 完成后发来 XML 格式通知。根据 task_id 使用 dispatch_task 的 continue_task 继续该 Worker，或者 spawn 新 Worker。

## 5. Continue vs Spawn 决策

| 情形 | 策略 | 原因 |
|------|------|------|
| 研究探索的文件正好要改 | Continue | Worker 已有上下文 |
| 研究范围宽，实现很窄 | Spawn | 清洁上下文更好 |
| 纠正失败的尝试 | Continue | Worker 有错误上下文 |
| 验证另一 Worker 的代码 | Spawn | 新鲜视角，不带实现假设 |
| 方法完全错误 | Spawn | 避免锚定效应 |
| 完全无关的任务 | Spawn | 没有可复用的上下文 |

## 6. 综合原则（最重要的工作）

研究完成后，**必须是你来理解**结果，然后写包含具体文件路径、行号、修改内容的规格说明。
禁止写"根据你的发现"或"根据研究"——这是把理解外包给 Worker。
好的规格说明一眼就能看出你真正读懂了研究结果。

## 7. Worker 提示词要点

Workers 看不到你和用户的对话。每个提示词必须自包含：
- 包含具体文件路径、行号、错误信息
- 说明"完成"意味着什么
- 实现任务末尾加：运行相关测试和类型检查，提交并报告 commit hash
- 研究任务末尾加：报告发现，不修改文件
- 禁止写"根据你的发现"——综合是你的工作`

// ─── Task Notification XML ────────────────────────────────────────────────────

// TaskNotification holds the result of a completed Worker task.
// Serialized to <task-notification> XML and injected into the Coordinator's message stream.
type TaskNotification struct {
	TaskID   string
	Status   string // "completed" | "failed" | "killed"
	Summary  string
	Result   string
	Usage    NotificationUsage
}

// NotificationUsage carries performance metadata for a completed task.
type NotificationUsage struct {
	TotalTokens int
	ToolUses    int
	DurationMs  int64
}

// FormatXML renders the notification as a <task-notification> XML block.
// This format mirrors Claude Code's coordinator task notification format.
func (n TaskNotification) FormatXML() string {
	var sb strings.Builder
	sb.WriteString("<task-notification>\n")
	sb.WriteString(fmt.Sprintf("<task-id>%s</task-id>\n", n.TaskID))
	sb.WriteString(fmt.Sprintf("<status>%s</status>\n", n.Status))
	sb.WriteString(fmt.Sprintf("<summary>%s</summary>\n", escapeXML(n.Summary)))
	if n.Result != "" {
		sb.WriteString(fmt.Sprintf("<result>%s</result>\n", escapeXML(n.Result)))
	}
	sb.WriteString("<usage>\n")
	sb.WriteString(fmt.Sprintf("  <total_tokens>%d</total_tokens>\n", n.Usage.TotalTokens))
	sb.WriteString(fmt.Sprintf("  <tool_uses>%d</tool_uses>\n", n.Usage.ToolUses))
	sb.WriteString(fmt.Sprintf("  <duration_ms>%d</duration_ms>\n", n.Usage.DurationMs))
	sb.WriteString("</usage>\n")
	sb.WriteString("</task-notification>")
	return sb.String()
}

// BuildTaskNotification creates a TaskNotification from a completed Task.
func BuildTaskNotification(task *Task) TaskNotification {
	status := "completed"
	switch task.Status {
	case TaskError:
		status = "failed"
	case TaskKilled:
		status = "killed"
	}

	summary := fmt.Sprintf("Agent \"%s\" %s", task.Label, statusSummary(task.Status))
	if task.ErrorMsg != "" {
		summary += ": " + task.ErrorMsg
	}

	durationMs := int64(0)
	if task.StartedAt > 0 && task.EndedAt > 0 {
		durationMs = task.EndedAt - task.StartedAt
	}

	return TaskNotification{
		TaskID:  task.ID,
		Status:  status,
		Summary: summary,
		Result:  task.Output,
		Usage: NotificationUsage{
			DurationMs: durationMs,
		},
	}
}

func statusSummary(s TaskStatus) string {
	switch s {
	case TaskDone:
		return "completed"
	case TaskError:
		return "failed"
	case TaskKilled:
		return "was stopped"
	default:
		return string(s)
	}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
