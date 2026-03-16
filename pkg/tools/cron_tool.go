package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/cron"
	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// CronEngine abstracts the cron scheduler for dependency injection into tools.
type CronEngine interface {
	Add(job *cron.Job) error
	Remove(id string) error
	ListJobs() []*cron.Job
	ListJobsByAgent(agentID string) []*cron.Job
}

// ── Tool definitions ──────────────────────────────────────────────────────────

var cronListDef = llm.ToolDef{
	Name:        "cron_list",
	Description: "列出当前 Agent 的所有定时任务（含任务 ID、名称、调度规则、状态、下次/上次执行时间）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"all": {
				"type": "boolean",
				"description": "是否显示所有 Agent 的任务（默认仅显示当前 Agent 的任务）"
			}
		}
	}`),
}

var cronAddDef = llm.ToolDef{
	Name:        "cron_add",
	Description: "添加一个定时任务。支持三种调度方式：every（每隔 N 毫秒）、cron（标准 cron 表达式）、at（一次性 ISO-8601 时间戳）。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "任务名称（便于识别）"
			},
			"kind": {
				"type": "string",
				"enum": ["every", "cron", "at"],
				"description": "调度类型：every=间隔执行，cron=表达式，at=一次性"
			},
			"everyMs": {
				"type": "integer",
				"description": "间隔毫秒数（kind=every 时必填，如 300000 = 5分钟）"
			},
			"expr": {
				"type": "string",
				"description": "Cron 表达式（kind=cron 时必填，如 '0 9 * * 1-5'）或 ISO-8601 时间戳（kind=at 时必填）"
			},
			"tz": {
				"type": "string",
				"description": "时区（可选，如 Asia/Shanghai，默认 UTC）"
			},
			"message": {
				"type": "string",
				"description": "任务触发时发送给 Agent 的提示内容"
			},
			"model": {
				"type": "string",
				"description": "覆盖默认模型（可选）"
			},
			"delivery": {
				"type": "string",
				"enum": ["announce", "none"],
				"description": "交付模式：announce=输出推送给用户，none=静默记录（默认 announce）"
			},
			"remark": {
				"type": "string",
				"description": "任务备注（可选）"
			}
		},
		"required": ["name", "kind", "message"]
	}`),
}

var cronRemoveDef = llm.ToolDef{
	Name:        "cron_remove",
	Description: "删除一个定时任务（通过任务 ID）。使用 cron_list 获取任务 ID。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "要删除的任务 ID"
			}
		},
		"required": ["id"]
	}`),
}

// ── WithCronEngine ─────────────────────────────────────────────────────────────

// WithCronEngine registers cron_list, cron_add, cron_remove tools backed by the given engine.
// If engine is nil, no cron tools are registered.
func (r *Registry) WithCronEngine(engine CronEngine) {
	if engine == nil {
		return
	}
	r.cronEngine = engine

	r.register(cronListDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleCronList(ctx, input)
	})
	r.register(cronAddDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleCronAdd(ctx, input)
	})
	r.register(cronRemoveDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return r.handleCronRemove(ctx, input)
	})
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (r *Registry) handleCronList(_ context.Context, input json.RawMessage) (string, error) {
	if r.cronEngine == nil {
		return "", fmt.Errorf("cron engine not configured")
	}
	var p struct {
		All bool `json:"all"`
	}
	_ = json.Unmarshal(input, &p)

	var jobs []*cron.Job
	if p.All {
		jobs = r.cronEngine.ListJobs()
	} else {
		jobs = r.cronEngine.ListJobsByAgent(r.agentID)
	}

	if len(jobs) == 0 {
		return "（暂无定时任务）", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个定时任务：\n\n", len(jobs)))
	for _, j := range jobs {
		status := "✅ 启用"
		if !j.Enabled {
			status = "⏸ 禁用"
		}
		sb.WriteString(fmt.Sprintf("• [%s] %s\n", status, j.Name))
		sb.WriteString(fmt.Sprintf("  ID: %s\n", j.ID))
		sb.WriteString(fmt.Sprintf("  Agent: %s\n", j.AgentID))
		// Schedule info
		switch j.Schedule.Kind {
		case "every":
			sb.WriteString(fmt.Sprintf("  调度: 每 %dms 执行一次\n", j.Schedule.EveryMs))
		case "cron":
			tz := j.Schedule.TZ
			if tz == "" {
				tz = "UTC"
			}
			sb.WriteString(fmt.Sprintf("  调度: cron(%s) tz=%s\n", j.Schedule.Expr, tz))
		case "at":
			sb.WriteString(fmt.Sprintf("  调度: 一次性 at=%s\n", j.Schedule.Expr))
		default:
			sb.WriteString(fmt.Sprintf("  调度: %+v\n", j.Schedule))
		}
		sb.WriteString(fmt.Sprintf("  消息: %s\n", truncate(j.Payload.Message, 80)))
		if j.State.NextRunAtMs > 0 {
			sb.WriteString(fmt.Sprintf("  下次执行: %s\n", msToTime(j.State.NextRunAtMs)))
		}
		if j.State.LastRunAtMs > 0 {
			sb.WriteString(fmt.Sprintf("  上次执行: %s (%s)\n", msToTime(j.State.LastRunAtMs), j.State.LastStatus))
		}
		if j.Remark != "" {
			sb.WriteString(fmt.Sprintf("  备注: %s\n", j.Remark))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (r *Registry) handleCronAdd(_ context.Context, input json.RawMessage) (string, error) {
	if r.cronEngine == nil {
		return "", fmt.Errorf("cron engine not configured")
	}
	var p struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		EveryMs  int64  `json:"everyMs"`
		Expr     string `json:"expr"`
		TZ       string `json:"tz"`
		Message  string `json:"message"`
		Model    string `json:"model"`
		Delivery string `json:"delivery"`
		Remark   string `json:"remark"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid input: %v", err)
	}
	if p.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if p.Message == "" {
		return "", fmt.Errorf("message is required")
	}

	// Build Schedule
	var sched cron.Schedule
	switch p.Kind {
	case "every":
		if p.EveryMs <= 0 {
			return "", fmt.Errorf("everyMs must be > 0 for kind=every")
		}
		sched = cron.Schedule{Kind: "every", EveryMs: p.EveryMs}
	case "cron":
		if p.Expr == "" {
			return "", fmt.Errorf("expr is required for kind=cron")
		}
		sched = cron.Schedule{Kind: "cron", Expr: p.Expr, TZ: p.TZ}
	case "at":
		if p.Expr == "" {
			return "", fmt.Errorf("expr (ISO-8601 timestamp) is required for kind=at")
		}
		sched = cron.Schedule{Kind: "at", Expr: p.Expr}
	default:
		return "", fmt.Errorf("kind must be one of: every, cron, at")
	}

	// Build Delivery
	deliveryMode := p.Delivery
	if deliveryMode == "" {
		deliveryMode = "announce"
	}

	job := &cron.Job{
		Name:    p.Name,
		Enabled: true,
		Remark:  p.Remark,
		AgentID: r.agentID,
		Schedule: sched,
		Payload: cron.Payload{
			Kind:    "agentTurn",
			Message: p.Message,
			Model:   p.Model,
		},
		Delivery: cron.Delivery{Mode: deliveryMode},
	}

	if err := r.cronEngine.Add(job); err != nil {
		return "", fmt.Errorf("添加任务失败: %w", err)
	}

	return fmt.Sprintf("✅ 定时任务「%s」已创建 (ID: %s)", job.Name, job.ID), nil
}

func (r *Registry) handleCronRemove(_ context.Context, input json.RawMessage) (string, error) {
	if r.cronEngine == nil {
		return "", fmt.Errorf("cron engine not configured")
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if err := r.cronEngine.Remove(p.ID); err != nil {
		return "", fmt.Errorf("删除任务失败: %w", err)
	}
	return fmt.Sprintf("✅ 定时任务 %s 已删除", p.ID), nil
}

// msToTime converts a Unix millisecond timestamp to a readable string.
func msToTime(ms int64) string {
	t := time.UnixMilli(ms)
	return t.Format("2006-01-02 15:04:05 MST")
}
