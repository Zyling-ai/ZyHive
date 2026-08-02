# REST、SSE 与 CLI 参考

权威路由表是 `internal/api/router.go`；请求/响应类型可结合 `ui/src/api/index.ts`，CLI 命令以 `zyhive --help` 为准。

## REST 通用约定

管理请求：

```text
Authorization: Bearer <token>
Content-Type: application/json
```

默认普通请求正文上限 4 MiB，可由 `ZYHIVE_MAX_REQUEST_BODY_MB` 调整。响应包含 `X-Trace-Id`，日志可按该值串联。

成功状态由动作决定：常见为 200、201、204。失败通常是：

```json
{"error":"message"}
```

## 公开端点

- `GET /api/version`：版本。
- `GET /api/update/status`：更新状态。
- `GET /healthz`：存活/基础健康。
- `GET /readyz`：readiness；运行时过载或关键子系统不健康可返回 503。
- `GET /metrics`：仅实验 metrics registry 存在时注册。
- `GET /api/download?ticket=...`、`GET /api/media?ticket=...`：一次性短期 ticket。
- `GET|POST /feishu/card-callback`：飞书回调。
- `/pub/chat/...`：公开 Web 渠道，使用渠道密码、来源限流和会话容量，不使用管理 token。
- `GET /ws` 当前只返回 `websocket not yet implemented`，不是实时协议。

`GET /api/update/status` 和 `/api/version` 公开是为了登录/重启轮询，不代表 `/api/update/apply` 公开。

## 管理 API 资源

以下都位于 `/api` 且需要 Bearer token。

### 成员与对话

- `/agents`：成员 CRUD。
- `/agents/:id/start|stop|message|notify`
- `/agents/:id/chat`：POST SSE。
- `/agents/:id/chat/stream`：GET SSE 重连。
- `/agents/:id/chat/status`
- `/agents/:id/sessions`、`/agents/:id/sessions/:sid`
- `/sessions`、`/sessions/:agentId/:sid`：全局列表、删除、重命名。
- `/conversations`、`/agents/:id/conversations/...`：管理员对话审计。

### 成员能力

- `/agents/:id/files/*path`
- `/agents/:id/memory/...`
- `/agents/:id/network/contacts...`
- `/agents/:id/network/chats...`
- `/agents/:id/relations`
- `/agents/:id/skills...` 与 SkillOpt 子路由
- `/agents/:id/channels...`
- `/agents/:id/wishlist`
- `/agents/:id/tool-health`
- `/agents/:id/tool-audit...`

### 全局注册表与管理

- `/providers`、`/models`、`/channels`、`/tools`、`/skills`
- `/acp`
- `/config`
- `/cron`
- `/goals`
- `/projects`
- `/tasks`、`/subagent-events`
- `/network/contacts|chats`：跨成员聚合
- `/approvals/...`
- `/usage/summary|timeline|records`
- `/budget`、`/llm/throttle`
- `/status`、`/stats`、`/health`、`/logs`
- `/update/check|apply`
- `/team/graph`、`/team/relations...`
- `/feishu/probe|test-connect`

实验 aiteam 路由也挂在管理组，但每个 handler 再检查自身环境开关；关闭时返回 404。

## SSE 聊天协议

### 创建/继续生成

```http
POST /api/agents/:id/chat
```

请求：

```json
{
  "message": "必填",
  "sessionId": "可选",
  "context": "可选额外系统上下文",
  "scenario": "可选场景",
  "skillId": "可选",
  "images": [],
  "history": [{"role":"user","content":"legacy only"}]
}
```

消息入队后，HTTP 连接订阅 Session Worker 的 Broadcaster。断开只取消订阅，不取消生成；同一会话 Worker 负责串行/容量控制。

### 重连与状态

```http
GET /api/agents/:id/chat/stream?sessionId=...
GET /api/agents/:id/chat/status?sessionId=...
```

重连先收到 Broadcaster 缓冲，再收到实时事件。Worker 不存在时返回 SSE `idle`。状态响应：

```json
{
  "status": "idle|generating",
  "hasWorker": true,
  "bufferedEvents": 12
}
```

### 帧格式

```text
data: {"type":"text_delta","text":"你"}

: keepalive

```

keepalive 每 15 秒发送，客户端忽略注释行。当前实现使用 JSON 中的 `type`，不依赖 SSE `event:` 字段。

事件：

- `thinking_delta`：`text`
- `text_delta`：`text`
- `tool_call`：`tool_call` 对象
- `tool_result`：`text`，可有 `tool_call_id`
- `usage`：`input_tokens`、`output_tokens`
- `compaction_start`：`tokens_before`
- `compaction_end`：`tokens_before`、`tokens_after`，可有 `error`
- `done`：`sessionId`、`tokenEstimate`，token 数仅在完整时附带
- `error`：`error`
- `idle`：无活动流

`done`/`error` 关闭服务端订阅。网络 EOF 本身不等于 done；客户端应使用 sessionId 重连或查询状态。

## 审批 SSE

浏览器 EventSource 不能设置 Authorization header，因此流程是：

1. `POST /api/approvals/stream-ticket`（Bearer 鉴权）
2. `GET /api/approvals/stream?ticket=...`

ticket 短期、一次性、只授权该 stream。事件包括初始 `hello` 和 `approval_snapshot`。不要把 ticket 当作通用管理 token。

## 公开聊天 SSE

主路由：

- `GET /pub/chat/:agentId/:channelId/info`
- `GET /pub/chat/:agentId/:channelId/history`
- `POST /pub/chat/:agentId/:channelId/stream`
- `GET /pub/chat/:agentId/:channelId/reconnect`

legacy 路由省略 channelId 并选择第一个启用的 Web 渠道。公开流有独立密码、消息大小、来源频率、并发、SSE 和 session TTL 限制。

## Agent CLI

结构：

```bash
zyhive <资源> <动作> [参数] \
  [--json] [--quiet] [--yes] \
  [--host URL] [--token TOKEN] [--config PATH]
```

资源：

- `agent`、`chat`、`session`
- `cron`、`goal`、`task`
- `memory`、`network`、`relation`、`project`、`file`
- `model`、`provider`、`channel`、`tool`、`skill`、`acp`
- `usage`、`system`、`conversations`、`approval`
- `api`：任意 REST 逃生舱

动态查看动作：

```bash
zyhive cron --help
zyhive cron add --help
```

CLI 是 HTTP 客户端：

- 不直接读写业务文件；
- `--json` 用于机器解析；
- 专用资源命令的修改/删除动作要求 `--yes` 或 TTY 确认；
- 原始 `zyhive api` 逃生舱不执行确认，即使 method 有副作用；
- `chat send` 消费 SSE，并在 JSON 模式返回聚合 text、sessionId 和原始 events；
- 非流请求默认有 60 秒 context deadline，SSE 客户端无全局超时；
- 单个普通响应读取上限 64 MiB，SSE 单行 scanner 上限 8 MiB。

退出码：0 成功、1 通用错误、2 用法错误、3 鉴权错误、4 不存在、5 连接失败。

## 运维 CLI

同一二进制还提供：

```bash
zyhive
zyhive token
zyhive start|stop|restart|status|enable|disable
zyhive version
zyhive backup create|inspect|restore ...
zyhive --serve --config /path/config.json
```

无参数且未显式指定配置/serve 时进入交互面板。运维 CLI 可直接管理系统服务和备份，不属于 REST 瘦客户端命令树。
