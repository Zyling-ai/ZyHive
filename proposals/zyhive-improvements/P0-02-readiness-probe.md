# P0-02 · `/readyz` 就绪探针 + Provider/cron/session 健康指标补全

- 主题：C 生产稳定性
- 优先级：P0
- 规模：S（单文件级，扩 `internal/api/healthz.go`）
- 状态：proposed

## 1. 背景与问题

仓库已有：

- `GET /healthz`（无鉴权，进程层面"我活着"）
- `GET /api/status`（鉴权，含 agent / cron 详细列表）

当前缺：

- **就绪 vs 存活的区分**：进程起来 ≠ 业务就绪。例如 cron engine 还没启动，或所有 Provider 全部 401，此时仍返回 `status=ok`，对编排器（systemd / k8s readinessProbe / nginx upstream healthcheck）不友好。
- **Provider 健康真值**：`pkg/llm/health.go` 已有 30s 缓存的 Ping，但 `/healthz` 没暴露
- **session pool 心跳**：派遣中的 subagent 数 / 长时间未 yield 的 worker 没暴露
- **last_message_at**：当前是 `nil` 占位（`healthz.go:58`），未实现

## 2. 目标 & 非目标

**目标**：

1. 新增 `GET /readyz`（无鉴权），明确返回 200/503：
   - 200：所有"必需依赖"健康（至少 1 个 Provider Ping ok、cron engine 心跳 < 60s 内、session pool 不死锁）
   - 503：任意必需依赖不健康，body 列出问题 ID
2. 扩展 `/healthz`：补 `providers[]` 简表 + `last_message_at` 真值
3. 新增 `GET /api/status` 的 `subagents` / `sessions_active` 字段
4. 配套：`pkg/cron/engine.go` 暴露 `LastTickAt() time.Time`；`pkg/session` 暴露 `ActiveCount()`

**非目标**：

- 不实现 Prometheus `/metrics`（留 P1，C-01 之后做）
- 不改变 `/healthz` 现有字段名（向后兼容）

## 3. 设计要点

### 3.1 `/readyz` 响应

成功（200）：

```json
{ "ready": true, "checks": { "providers": "ok", "cron": "ok", "sessions": "ok" } }
```

失败（503）：

```json
{
  "ready": false,
  "checks": {
    "providers": "fail: anthropic-1 401, openai-1 timeout",
    "cron": "ok",
    "sessions": "ok"
  },
  "since_ms": 15234
}
```

### 3.2 Provider 探活策略

复用 `pkg/llm/health.go` 已有 `Ping()` + 30s 缓存：

- `/readyz` 不主动触发 Ping（避免 DDoS 自家 Provider），只读缓存
- 缓存为空（启动后冷启动）→ 视作"未知"，不算 fail（但首 60 秒后还未填充则降级为 fail）
- 至少有 1 个 default-eligible Provider 健康即视作 "providers ok"

### 3.3 cron heartbeat

`pkg/cron/engine.go` 内部 ticker 每秒执行调度循环；新增：

```go
type Engine struct {
    // ...
    lastTickAt atomic.Int64 // unix ms
}
func (e *Engine) LastTickAt() time.Time
```

每次 tick 末尾 store。`/readyz` 判定：`now - lastTickAt < 60s` 算 ok。

### 3.4 session pool

`pkg/session` 已有 worker 池（参考 README 描述）。新增 `ActiveCount()`：

- 当前正在处理消息的 worker 数
- 队列 backlog 数

`/readyz` 阈值（可配）：backlog > 100 视作 fail（默认值）。

### 3.5 `last_message_at`

通过 `pkg/channel` 给 telegram / 飞书 各自挂一个全局 `lastInboundAt atomic.Int64`，`/healthz` 读出渲染。

## 4. 影响面

| 路径 | 改动 |
|------|------|
| `internal/api/healthz.go` | 加 `readyzHandler`、扩展现有字段 |
| `internal/api/router.go` | 注册 `GET /readyz` |
| `pkg/cron/engine.go` | 加 `lastTickAt` + `LastTickAt()` 方法 |
| `pkg/session/*.go` | 加 `ActiveCount()` |
| `pkg/llm/health.go` | 暴露 `LastResult(providerID)`（只读快照） |
| `pkg/channel/*` | 入站消息时更新 `lastInboundAt` |
| `scripts/deploy-hive.sh` | systemd unit 可选加 `ExecStartPost` 等 readyz |

## 5. 迁移与兼容

- `/healthz` 字段只增不减，老监控继续可用
- `/readyz` 是全新端点

## 6. 测试计划

- `internal/api/healthz_test.go`：构造 mock manager + cronEngine，断言 ready=true/false 各分支
- `pkg/cron/engine_test.go`（如已有则补充）：tick 后 `LastTickAt` 更新

## 7. 文档与 CHANGELOG

- README 增加"运维端点"小节，列 `/healthz` `/readyz` `/api/status` 三连
- CHANGELOG 单条

## 8. 风险与回滚

- 风险：探针变严格后误判把好端服务摘掉。缓解：所有 fail 阈值提供 `zyhive.json` 配置项 `readyz.{cron_max_lag_sec, sessions_backlog_max, providers_required: bool}`，默认从宽。
- 回滚：删 `/readyz` 端点，监控配置改回 `/healthz`。
