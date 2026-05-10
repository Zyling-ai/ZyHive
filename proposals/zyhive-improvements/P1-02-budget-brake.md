# P1-02 · 自主唤醒 budget 预算刹车

- 主题：A AI 自主性 & 用量自治
- 优先级：P1（README 已声明 P1 路线项）
- 规模：M（单包级，扩 `pkg/usage` + `pkg/runner` + UI）
- 状态：proposed

## 1. 背景与问题

当 agent 拥有以下能力后：

- `cron_add` / `self_schedule`（自主排程）
- `agent_spawn`（派遣子成员）
- 飞书/TG/Web 多渠道入站（被外部触发）

**潜在风险**：一次糟糕的循环（cron 设错 / 子任务死循环 / 群消息暴增）可能在用户察觉前烧掉数十美金 token。当前 `pkg/usage/store.go` 只做事后记录与展示（UsageView），**没有上游硬刹车**。

## 2. 目标 & 非目标

**目标**：

1. 引入 per-agent 日预算，单位为 USD（与 UsageView 计费一致）
2. 软警告（80% 阈值）：在系统提示词中注入"今日预算剩余 $0.20，请克制"
3. 硬刹车（100%）：runner 入口拦截，直接返回明确错误，**不进入 LLM Stream**；UI 显示横幅
4. 触发来源区分：用户主动对话 / cron 自唤醒 / 渠道入站 / 子成员派遣，可分别配限额（默认全部计入同一池）
5. 提供一键"今天我是 VIP，临时 +1 美金"的紧急放行
6. 全局总预算（保护账单）：所有 agent 加起来超过 globalCapUSD 也硬刹

**非目标**：

- 不实现按 model 分别计配额（先用 USD 统一计）
- 不实现按月/周配额（先做日；够用再扩）
- 不替代 Provider 端速率限制（C-02 自适应限流单独处理）

## 3. 设计要点

### 3.1 配置

`zyhive.json` 增：

```json
{
  "budget": {
    "enabled": true,
    "global_daily_usd": 5.0,
    "default_agent_daily_usd": 1.0,
    "warn_at_pct": 80,
    "tz": "Asia/Shanghai"
  }
}
```

每 agent 可单独设：写入 `agents/{id}/budget.json`，覆盖 default。

### 3.2 包结构

新增 `pkg/budget/`：

```
pkg/budget/
├── budget.go         // BudgetStore: Used(agentID) / Charge(agentID, usd) / Reset()
├── enforcer.go       // BeforeRun(ctx, agentID, source) error
└── budget_test.go
```

`BudgetStore` 内部用 in-memory map（按"今日 YYYY-MM-DD UTC"key 存累计），由 `pkg/usage/store.go` 的 `Append()` 同步触发 `Charge`。

### 3.3 Runner 接入

`pkg/runner/runner.go` 主入口：

```go
if err := budget.Enforcer.BeforeRun(ctx, agentID, source); err != nil {
    // 不进入 LLM Stream，直接 emit 一个错误事件给 SSE
    return budgetExceededEvent(err)
}
```

错误事件格式（前端易识别）：

```json
{ "type": "error", "code": "budget_exceeded", "message": "今日预算 $1.00 已用尽（用了 $1.03）", "scope": "agent" }
```

### 3.4 软警告注入

`pkg/runner/system_prompt.go` 在层 9 capabilities 之后追加（仅当 used >= warnAt%）：

```
## 预算提醒
今日预算剩余 $0.20 / $1.00（已用 80%）。请尽量给出精炼回答，避免不必要的工具循环。
```

### 3.5 紧急放行

UI `ChatHomeView.vue` 检测 `error.code=='budget_exceeded'` 时显示横幅：

```
⚠️ 今日预算已用尽   [+$1 临时提额（仅今天）]   [+$5]   [设置 →]
```

点击 `+$1` → POST `/api/budget/topup`，在 `BudgetStore` 当日记一笔减项（次日自动作废）。

### 3.6 来源区分

`source` 字段贯穿 runner，已在 `pkg/runner` 部分场景出现（cron / dispatch）。本提案统一为枚举：`user | cron | channel | dispatch`。

预留配置：

```json
"budget": {
  "limits_by_source": {
    "cron":     0.50,
    "channel":  0.50,
    "dispatch": 0.30
  }
}
```

未配置则不分维度，统一计入总池。

## 4. 影响面

| 路径 | 改动 |
|------|------|
| `pkg/budget/` | 新增 |
| `pkg/usage/store.go` | `Append` 内 hook 调 `budget.Charge` |
| `pkg/runner/runner.go` | 入口加 `BeforeRun` 检查；source 透传 |
| `pkg/runner/system_prompt.go` | 软警告注入层 |
| `internal/api/budget.go` | 新增 `GET /api/budget`、`POST /api/budget/topup`、`PATCH /api/agents/:id/budget` |
| `internal/api/router.go` | 注册路由 |
| `pkg/config/*` | 配置结构扩展 |
| `ui/src/views/UsageView.vue` | 加预算进度条 |
| `ui/src/views/AgentDetailView.vue` | "身份 & 灵魂" tab 之后加 "预算" 卡 |
| `ui/src/views/ChatHomeView.vue` | error 事件展示横幅 |

## 5. 迁移与兼容

- `budget.enabled=false` 时所有逻辑跳过，与今天等价
- 老 agent 没有 `budget.json` → 落入 default

## 6. 测试计划

- `pkg/budget/budget_test.go`：
  - Charge 累加正确
  - 跨日自动归零（mock clock）
  - topup 当日生效次日失效
- `pkg/runner/runner_test.go`（如已有则补充）：BeforeRun 拒绝时不调用 LLM client mock
- 手测：把 `default_agent_daily_usd` 设为 0.01，发一条对话，第二条直接被拦

## 7. 文档与 CHANGELOG

- README 加"预算与配额"章节
- CHANGELOG 单条
- `docs/system-prompt-and-flow.md` 加"预算软警告"小节

## 8. 风险与回滚

- 风险 1：误拦正常对话。缓解：默认 `enabled=false`，新装用户首次进 UsageView 时引导开启；已有用户保持 off。
- 风险 2：Provider 计费估算不准（pricing.go 内置单价可能落后）。缓解：保留每月手动 reconcile 的渠道（UsageView 加"以 Provider 官方账单覆盖"按钮，单独提案）。
- 回滚：配置 `budget.enabled=false`，所有路径短路。
