# P1-03 · Provider 自适应限流（替换 FixedThrottle）

- 主题：C 生产稳定性
- 优先级：P1
- 规模：M（单包级，扩 `pkg/llm`）
- 状态：proposed

## 1. 背景与问题

`26.4.23v2` 已经预留了 `Throttle` 接口与默认 `FixedThrottle` 实现（CHANGELOG 明确写 "Throttle 接口抽象 ... 留给未来 AdaptiveThrottle"）。当前 `FixedThrottle` 行为是固定窗口/QPS，不区分 Provider，不响应服务端 `Retry-After`。

实际场景：

- Anthropic 在突发 RPS 超限时返回 429 + `retry-after: 30` header
- DeepSeek 偶发 503 返回 `retry-after: 5`
- OpenRouter 不同子 Provider 限速差异大

理想行为：每个 Provider 自动学习自己的"舒适区"，命中 429 时迅速降速，连续成功时缓慢提速。

## 2. 目标 & 非目标

**目标**：

1. 实现 `AdaptiveThrottle`：per-provider 维护一个"当前最大 inflight"窗口
2. 收到 429/503 + `Retry-After` 时立即缩窗 + 等待对应秒数
3. 没有 `Retry-After` 但识别为 transient（已有 `pkg/llm/errors.go`）时按指数退避降速
4. 连续 N 次成功后逐步放大窗口，但有上限（per-provider 配置）
5. 暴露 `GET /api/llm/throttle` 给运维/UI 查看每个 Provider 当前状态

**非目标**：

- 不实现跨进程协调（多副本部署留 P2）
- 不替换现有 `RetryClient`（重试与限流分别正交）

## 3. 设计要点

### 3.1 算法（AIMD-like）

为每个 provider 维护：

```go
type providerState struct {
    maxInflight  int           // 当前允许并发，初始 = config.MinInflight
    inflight     int           // 当前在跑
    cooldownTill time.Time     // 收到 Retry-After 时设置
    consecOK     int           // 连续成功数
}
```

策略：

- 进入：`inflight >= maxInflight` 时阻塞等待，超 `cooldownTill` 才开始排队
- 收到 429/503 with Retry-After：`maxInflight = max(1, maxInflight/2)`、`cooldownTill = now + retryAfter`、`consecOK = 0`
- 收到其它 transient：`maxInflight = max(1, maxInflight - 1)`
- 成功：`consecOK++`，每 `growEvery` 次 `maxInflight = min(cap, maxInflight+1)`

### 3.2 配置

```json
"throttle": {
  "kind": "adaptive",
  "providers": {
    "anthropic": { "min": 1, "max": 8,  "init": 4, "grow_every": 10 },
    "openai":    { "min": 1, "max": 16, "init": 8, "grow_every": 10 },
    "*":         { "min": 1, "max": 4,  "init": 2, "grow_every": 20 }
  }
}
```

未列出的 provider 用 `*` 默认。

### 3.3 与 RetryClient 关系

```
ChatRequest → AdaptiveThrottle.Acquire(provider) → (release after stream done)
                                                         │
                                                         ▼
                                              RetryClient.Stream(ctx, req)
```

`AdaptiveThrottle` 只控制并发与冷却，不做 retry；`RetryClient` 拿到 transient 后在 throttle 当前许可下再次申请。

### 3.4 可观测

`GET /api/llm/throttle`：

```json
{
  "providers": {
    "anthropic": { "max_inflight": 4, "inflight": 2, "cooldown_remaining_s": 0, "consec_ok": 7 }
  }
}
```

UI `ModelsView.vue` 已经显示 provider 健康（来自 P0 已落地的 `pkg/llm/health.go`），加一行 "并发 2/4" 即可。

## 4. 影响面

| 路径 | 改动 |
|------|------|
| `pkg/llm/throttle.go`（已存在 `Throttle` interface）| 新增 `AdaptiveThrottle` 实现 |
| `pkg/llm/throttle_test.go` | 单测 |
| `pkg/llm/errors.go` | 解析 `Retry-After` header（如未实现）|
| 所有 Provider 实现（`anthropic.go` 等）| 改 transient error 路径，把 `retryAfter time.Duration` 透传给 throttle |
| `pkg/config/*` | 配置结构 |
| `internal/api/llm.go`（新或挂在 `models.go`）| 暴露 `/api/llm/throttle` |
| `cmd/aipanel/main.go` | 启动时根据配置选 `FixedThrottle` 还是 `AdaptiveThrottle` |
| `ui/src/views/ModelsView.vue` | 状态行 |

## 5. 迁移与兼容

- 默认 `throttle.kind="fixed"`（即不变）；用户在 `zyhive.json` 改为 `adaptive` 后启用
- 所有 Provider 实现保持 transient 判定语义不变；新增可选 `retryAfter` 副信息

## 6. 测试计划

- `pkg/llm/throttle_test.go`：
  - 命中 429 后 `maxInflight` 立即减半且 cooldown 生效
  - 连续 N 次成功后增长且不超过 cap
  - context 取消时 Acquire 立即返回
- 集成：mock provider 返回 429 + retry-after，验证总耗时与预期一致

## 7. 文档与 CHANGELOG

- README 加配置示例
- CHANGELOG 单条
- 在 P0-01 结构化日志已落地的前提下，自动落 `provider throttle:max=4 inflight=2` 字段

## 8. 风险与回滚

- 风险：算法过度敏感导致正常服务被限。缓解：`min=1` 不会饿死、`max=cap` 防止无限放大；冷却时间最大 60s 上限。
- 回滚：`throttle.kind="fixed"` 即恢复原行为。
