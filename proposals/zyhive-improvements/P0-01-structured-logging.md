# P0-01 · 结构化日志 + 请求 trace id

- 主题：C 生产稳定性
- 优先级：P0
- 规模：M（单包级，跨 `internal/api/` + `pkg/runner/` + `pkg/llm/` 改动）
- 状态：proposed

## 1. 背景与问题

当前 ZyHive 散布的日志方式混杂：

- `pkg/llm/retry.go` 用标准库 `log.Printf`
- `internal/api/*` 大多用 `gin` 默认 logger
- `pkg/runner/runner.go`、`pkg/cron/engine.go` 等也是 `log.Printf` / `fmt.Printf`
- 出问题时**很难把同一次对话**串起来：用户 SSE 请求 → 经过 runner → 触发 N 个工具调用 → N 次 LLM Stream → 多 provider，全无统一 ID
- 生产环境只能 `journalctl -u zyhive` 拉一大段文本 grep（参考 `LogsView` 的三级降级）

## 2. 目标 & 非目标

**目标**：

1. 全仓引入 `log/slog`（Go 1.21+ stdlib）作为唯一日志门面
2. 每条 SSE 请求分配唯一 `trace_id`，从 HTTP 中间件起 → runner → 工具 → LLM client 全程透传（context value）
3. 日志默认输出 JSON 一行一条，关键字段：`time level msg trace_id agent_id session_id provider model tool`
4. 提供 `LOG_FORMAT=text|json`、`LOG_LEVEL=debug|info|warn|error` 环境变量
5. `LogsView` 增加按 `trace_id` 过滤的搜索框

**非目标**：

- 不引入 OpenTelemetry / Jaeger（留给 P2）
- 不替换 gin 的 access log（gin 自身保持，但补一层 trace_id 注入）
- 不改变 `journalctl` / `/tmp/aipanel.log` 的写入路径

## 3. 设计要点

### 3.1 包结构

新增 `pkg/logging/`：

```
pkg/logging/
├── logger.go         // Init() / Default() / FromContext(ctx)
├── middleware.go     // gin middleware: 注入 trace_id 到 ctx + response header
└── logger_test.go
```

`logger.go` 关键 API：

```go
package logging

func Init(format, level string)              // 程序启动时调用一次
func Default() *slog.Logger                  // 全局默认
func FromContext(ctx context.Context) *slog.Logger  // 自带 trace_id/agent_id 字段
func WithAgent(ctx, agentID string) context.Context
func WithSession(ctx, sessionID string) context.Context
func TraceID(ctx context.Context) string
```

### 3.2 trace_id 来源

- 请求入站时，若 header `X-Trace-Id` 存在则沿用（外部系统调试用），否则生成 UUIDv4 短形式
- 写到 response header `X-Trace-Id`，前端 SSE 收到后存入消息卡片 footer（debug 模式可见）

### 3.3 改造点（非穷举）

| 文件 | 改动 |
|------|------|
| `cmd/aipanel/main.go` | `logging.Init(...)` 启动时调用 |
| `internal/api/router.go` | 注册 `logging.Middleware()` |
| `internal/api/chat.go` | SSE handler 早期 `ctx = logging.WithAgent(ctx, agentID)`、`WithSession(ctx, sessionID)` |
| `pkg/runner/runner.go` | 主循环中 `logging.FromContext(ctx).Info(...)` 替换 `log.Printf` |
| `pkg/llm/retry.go` | 现有 `log.Printf("[llm-retry] ...")` 替换为 slog（保留前缀风格） |
| `pkg/llm/health.go` | Ping 日志加 provider 字段 |
| `pkg/cron/engine.go` | Job 执行日志带 `cron_id` + 派生 trace_id |
| `pkg/tools/*` | 工具调用前/后 1 行 `Info` 日志（tool / status / dur_ms / err） |

### 3.4 LogsView 升级

`ui/src/views/LogsView.vue` 增：

- 顶部输入框：按 `trace_id` 过滤（前端先做客户端 grep，后端 endpoint 后续可加 `?trace_id=`）
- 行解析：若行是 JSON，渲染为可折叠卡片，关键字段高亮

## 4. 影响面

- 包：`internal/api/router.go` `chat.go` `agents.go` `sessions.go` `cron.go` 等大量日志点；`pkg/{runner,llm,tools,cron,session,channel}` 全量替换 `log.Printf`
- 配置：新增 `LOG_FORMAT`、`LOG_LEVEL` 两个环境变量；`zyhive.json` 新增 `logging.format` / `logging.level` 可选段（环境变量优先）
- 配置默认：`format=text level=info`（保持开发体验），生产环境部署脚本 `scripts/install.sh` 写 `LOG_FORMAT=json`

## 5. 迁移与兼容

- 默认 text 格式与现状几乎一致，老用户读 `journalctl` 不会感到差异
- JSON 模式开启时，`LogsView` 自动识别（首字符为 `{`）
- 不删除任何旧日志文件

## 6. 测试计划

- `pkg/logging/logger_test.go`：trace_id 生成 / context 串联 / 字段合并
- `internal/api/router_test.go`（新）：请求带 / 不带 `X-Trace-Id` 都能正确响应 header
- 手测：起服务 → 发一条对话 → `journalctl` 中能用 `grep "<trace_id>"` 拿到完整链路

## 7. 文档与 CHANGELOG

- 更新 `docs/system-prompt-and-flow.md` 末尾"可观测性"小节
- README 加一行可观测性说明
- CHANGELOG 单独条目：`P0 · 结构化日志 + trace id`

## 8. 风险与回滚

- 风险：日志量上升（每个工具调用 +2 行）。缓解：`level=info` 下工具调用只在 done 时记一行，warn/err 才详细。
- 回滚：`LOG_FORMAT=text LOG_LEVEL=info` 即恢复至与今天等价的人眼可读输出。
