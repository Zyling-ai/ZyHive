# PR-003 · BudgetGuard 预算护栏 + panic-stop

> 状态: 🟡 初稿 v0（部分设计已定，几个关键单位/策略待用户确认）
> 优先级: 🔴 P0（aiteam Genesis 跑真业务前必备）
> 依赖: `pkg/usage`（已有）；与 ZyHive 26.5.10v1 的 `pkg/budget` brake 协同（非冲突，下文 § 4 详述）
> 默认 off：experimental flag `ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1`

---

## 0. 待用户决策（决定后即可写代码）

- [ ] **上限单位**
  - 选项 A: USD（金钱实数，与 `pkg/usage` 现有计费同单位）
  - 选项 B: token（更原始，适合 LLM 中立环境）
  - 选项 C: 内部 credits（与 PR-001 钱包共享单位）
  - 推荐 A（最直观，复用现有计费）
- [ ] **触发后的恢复策略**
  - 选项 A: 永久 panic-stop，需人类 UI 解封
  - 选项 B: 自动 cooldown N 小时后解除
  - 选项 C: 跨日重置（每日 0 点重置 panic 状态）
  - 推荐 B + C 复合：panic 后默认 1h cooldown，但跨日强制重置
- [ ] **panic-stop 触发后的具体行为**
  - 取消正在跑的 SSE 流（中断 token 输出）？
  - 拒绝新工具调用（含 LLM 调用本身）？
  - 清空 cron 队列里属于该 agent 的待执行任务？
  - 推荐：以上全部 + 推送一条 system 通知到主 UI
- [ ] **与 ZyHive 26.5.10v1 `pkg/budget` brake 的关系**
  - brake = 每日累计警告 + soft warn 注入 system prompt
  - guard = 强制熔断
  - 推荐：guard 复用 brake 的累计层（`pkg/usage`），只新加"硬上限+熔断"层

---

## 1. 背景

aiteam Genesis 让 agent 真接外部任务跑业务，意味着：

1. **金钱风险**：失控的 LLM 调用循环可能一夜烧掉 $$$
2. **失控风险**：被 prompt 注入或 bug 触发的死循环可能消耗集群资源
3. **责任边界**：哪个 agent 烧的钱、哪个任务烧的，必须清楚账面分得开

ZyHive 26.5.10v1 已有 `pkg/budget` brake：每日 USD 上限 + soft warn。但 brake **不强制熔断**，AI 看到警告也可以选择继续。

aiteam 需要的是：**硬熔断 + panic-stop**，AI 没选择，超就停。

## 2. 设计

### 2.1 BudgetGuard 数据模型

```go
// pkg/aiteam/budget/guard.go

type GuardLimits struct {
    PerAgentDailyUSD float64 // 单个 agent 24h 滚动窗口硬上限
    GlobalDailyUSD   float64 // 全体 agent 累计硬上限（防 fan-out 失控）
    PerSessionUSD    float64 // 单 session 硬上限（防单 prompt 死循环）
}

type GuardState struct {
    AgentID        string
    UsedUSD        float64
    LimitUSD       float64
    PanicTriggered bool
    PanicAt        time.Time
    PanicReason    string  // "agent_daily" | "session" | "global"
    CooldownUntil  time.Time
}

type Guard struct {
    limits GuardLimits
    states map[string]*GuardState // by agentID
    mu     sync.RWMutex
    usage  *usage.Store           // ZyHive 现有 usage 数据源
}
```

### 2.2 调用切点

```go
// pkg/runner/runner.go::Run
//
// 在每次 LLM call 之前检查:
//   1. 拉取 guard.Check(agentID, sessionID) → (canProceed bool, reason string)
//   2. 若 canProceed=false：emit RunEvent{Type:"budget_panic", reason}
//   3. 立即取消 ctx，触发 SSE 关闭
//   4. 持久化 panic state
```

### 2.3 解封机制

```go
// API
// POST /api/aiteam/guard/:agentId/release
// body: {"reason":"manual_review_passed", "operator":"user@host"}
//
// CLI
// $ zyhive guard list                       # 显示当前 panic 中的 agent
// $ zyhive guard release alice              # 手动解封
// $ zyhive guard set-limit alice 5.0        # 调上限
```

### 2.4 UI

主 UI 顶栏增加红色横幅（仅 panic 状态）："⚠️ Agent X 已触发预算上限熔断，[查看详情] [手动解封]"

## 3. 实施步骤

1. `pkg/aiteam/budget/guard.go` + `guard_test.go`（unit, ~10 case）
2. 改 `pkg/runner/runner.go` 加 `Guard` 字段，每次 LLM call 前调 `Check()`
3. 改 `pkg/agent/pool.go` 把全局 Guard 注入到所有 agent runner
4. `internal/api/aiteam_guard.go` REST handler
5. `cmd/aipanel/cli.go` 加 `guard` 子命令
6. UI banner（在 App.vue 顶部加 conditional 块）
7. CHANGELOG 条目 + experimental flag 文档
8. 集成测试：mock LLM 强制烧到 limit，验证下次调用被熔断 + SSE 立即收到 budget_panic 事件

## 4. 与现有系统的关系

| 系统 | 关系 | 说明 |
|------|------|------|
| `pkg/usage` | 数据源 | guard 读 usage 累计数据，不重复计费 |
| `pkg/budget`（26.5.10v1） | 互补 | brake = soft warn；guard = hard stop。两者可同时启用：先 warn 后 stop |
| PR-001 Wallet | 数据源潜在替代 | 若 PR-001 落地，guard 可改读 wallet 余额而非 usage 累计 |
| PR-004 Judge | 反馈源 | Judge 可触发临时降低 limit（"这个 agent 最近表现差，先减预算"） |

## 5. experimental flag
- `ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1` → 启用 guard
- `ZYHIVE_EXPERIMENTAL_BUDGETGUARD=` (默认) → guard.Check 直接返回 (true, "")，零开销
- 即使开启，所有 limit 字段默认为 `MaxFloat64` → 实际不熔断，必须显式配置
- 配置文件位置：`zyhive.json` 新加 `aiteam.guard` 段，仅在 flag=on 时读

## 6. 兼容性 / 回滚
- 数据存 `workspace/aiteam/guard/state.json`，回滚直接删
- 主 schema / 路由不变（新路由都在 `/api/aiteam/` 命名空间）
- 关闭 flag 即完全 no-op，与未编译同效
- pkg/runner 加的 Guard 字段为 `*Guard` 指针，nil 表示未启用，每个调用点 `if g != nil` 短路

## 7. 测试计划

- `pkg/aiteam/budget/guard_test.go`（10 case）
  - `TestGuard_NoLimitNoBlock`
  - `TestGuard_AgentDailyLimitTriggers`
  - `TestGuard_SessionLimitTriggers`
  - `TestGuard_GlobalLimitTriggers`
  - `TestGuard_PanicCooldownUnblocksAfterTime`
  - `TestGuard_CrossDayResetClearsCooldown`
  - `TestGuard_ManualRelease`
  - `TestGuard_ConcurrentChecksAreThreadSafe`
  - `TestGuard_StatePersistsAcrossRestart`
  - `TestGuard_NilGuardIsNoOp`
- 集成测试 `TestRunner_BudgetPanicHaltsLLM`：mock LLM 把 sessionUSD 推过阈值 → 第二次 Run 应立即返回 budget_panic 错误
