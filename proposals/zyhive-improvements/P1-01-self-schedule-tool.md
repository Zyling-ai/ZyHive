# P1-01 · `self_schedule` 自主闹钟工具

- 主题：A AI 自主性 & 用量自治
- 优先级：P1（README 已声明 P1 路线项）
- 规模：M（单包级，扩 `pkg/tools` + `pkg/cron`）
- 状态：proposed

## 1. 背景与问题

当前 AI 已有 `cron_add` / `cron_update` 等工具，但这些工具是给"管理 cron 任务"用的（多个时间槽 + 命名 + 显式发消息）。用户期待的"我让 AI 自己设个 30 分钟后提醒"，目前的体验：

- AI 必须用 `cron_add(kind=at, expr=ISO-8601, message=..., name=..., delivery=...)` 一长串参数
- AI 容易忘记带时区、漏 `delivery`、重复创建相似闹钟
- 没有"几分钟后/明天 9 点"这类自然语义的简化入口

`README.md` P1 列表显式提到 `self_schedule` 自主闹钟工具，本提案就是落地它。

## 2. 目标 & 非目标

**目标**：

1. 新增工具 `self_schedule(when, note)`：
   - `when`：人类语义字符串（"30m" / "2h" / "tomorrow 09:00" / ISO-8601），后端解析
   - `note`：到期时 AI 自己会收到的提示词
2. 复用现有 `pkg/cron.Engine.Add`，底层就是创建 `kind="at"` 的一次性 Job
3. 解析失败时返回明确错误 + 期望格式举例（让 AI 自纠）
4. 防滥用：每个 agent 同时未触发的 self-schedule 上限默认 20 个
5. UI（`CronView`）能识别 self_schedule 创建的任务，加 🔔 标识

**非目标**：

- 不替代 `cron_add`：复杂任务（周期、命名、群发）继续用原工具
- 不实现"取消我所有的闹钟"专用工具（用 `cron_list` + `cron_remove` 即可）

## 3. 设计要点

### 3.1 `when` 解析器

新增 `pkg/cron/whenparse.go`：

```go
func ParseWhen(input string, tz *time.Location, now time.Time) (time.Time, error)
```

支持：

| 输入示例 | 含义 |
|---------|------|
| `30m` / `2h` / `1h30m` | 相对当下（直接用 `time.ParseDuration`） |
| `tomorrow 09:00` / `tomorrow` | 明天某时（缺省 09:00） |
| `today 18:30` | 今天某时（若已过则报错） |
| `2026-05-10T09:00:00+08:00` | 完整 ISO-8601 |
| `next monday 10:00` | 下周一某时 |

时区：传入 agent 当前 tz（默认 `Asia/Shanghai`，可由 `zyhive.json` 全局覆盖）。

### 3.2 工具定义

```go
var selfScheduleDef = llm.ToolDef{
  Name: "self_schedule",
  Description: "设一个一次性提醒：到时给自己发一条 note。用于'X 分钟后再做'/'明天早上做 Y'等场景。要做周期性任务请用 cron_add。",
  InputSchema: json.RawMessage(`{
    "type":"object",
    "properties":{
      "when":{"type":"string","description":"何时触发：30m / 2h / tomorrow 09:00 / ISO-8601。时区默认 Asia/Shanghai。"},
      "note":{"type":"string","description":"到时自己会读到的提示词（建议含足够上下文，因为是新 session）"}
    },
    "required":["when","note"]
  }`),
}
```

底层执行：

```go
fireAt, err := cron.ParseWhen(input.When, tz, time.Now())
job := &cron.Job{
  ID:       genID(),
  AgentID:  ctx.AgentID,
  Name:     fmt.Sprintf("🔔 %s", truncate(input.Note, 20)),
  Schedule: cron.Schedule{Kind: "at", Expr: fireAt.Format(time.RFC3339)},
  Message:  input.Note,
  Source:   "self_schedule",  // 新增字段，区分 UI 展示
  Enabled:  true,
}
engine.Add(job)
```

返回给 AI：`已设定 1 次提醒：2026-05-10 09:00 +08:00（剩余 11h23m）。任务 ID: cron-xxx`

### 3.3 上限防滥用

`pkg/cron.Engine` 内部为每个 agent 维护未触发 `at` job 计数；`self_schedule` 在调用前检查：

```go
if engine.CountPendingSelfSchedules(agentID) >= maxPerAgent {
  return error: "已有 20 条未触发的提醒，请先用 cron_remove 清理"
}
```

`maxPerAgent` 走 `zyhive.json` 配置，默认 20。

### 3.4 UI

`ui/src/views/CronView.vue` 列表中：

- 若 `Source=="self_schedule"`，名字前加 🔔 emoji 标
- 列表筛选下拉新增"仅看 AI 自设提醒"

## 4. 影响面

| 路径 | 改动 |
|------|------|
| `pkg/cron/whenparse.go` | 新增 + 单测 |
| `pkg/cron/engine.go` | `Job` 加 `Source` 字段；新增 `CountPendingSelfSchedules` |
| `pkg/cron/types.go` | Schedule struct（如已有）扩字段 |
| `pkg/tools/self_schedule.go` | 新增 |
| `pkg/tools/registry.go` | 注册新工具，加入 `group:self` |
| `pkg/runner/system_prompt.go` | capabilities 列表自动包含（无需特殊处理） |
| `internal/api/cron.go` | 列表/详情透传 `Source` 字段 |
| `ui/src/views/CronView.vue` | 渲染调整 |
| `pkg/config/*` | 加 `cron.self_schedule_max_per_agent` 字段 |

## 5. 迁移与兼容

- `Job.Source` 是新字段，老 cron 数据反序列化为空字符串，UI 当作 `"manual"` 渲染
- `cron_add` 工具保持不变

## 6. 测试计划

- `pkg/cron/whenparse_test.go`：表驱动覆盖 10+ 输入（含错误格式 / 已过时间 / 时区切换）
- `pkg/tools/self_schedule_test.go`：成功路径 / 上限触发 / 解析失败错误信息
- 手测：发"15 分钟后提醒我喝水" → AI 调用 → CronView 看到 🔔 项 → 等待触发 → 收到自动续写消息

## 7. 文档与 CHANGELOG

- README 工具生态章节加一行
- `docs/system-prompt-and-flow.md` capabilities 例子更新
- CHANGELOG 单条

## 8. 风险与回滚

- 风险 1：自然语言解析歧义（"晚上"几点？）。缓解：解析器只接受白名单格式，模糊语义在错误信息中给"建议格式"，让 AI 自己再试一次。
- 风险 2：与 A-02 budget 刹车冲突——AI 可能用 self_schedule 绕过日预算。缓解：B-02 同时落地后，每次唤醒同样计入 budget。
- 回滚：从 `pkg/tools/registry.go` 摘掉注册即可，老数据保留无害。
