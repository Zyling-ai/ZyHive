# API、SSE 与 CLI 开发

ZyHive 有三个相关但职责不同的操作面：

- REST API：服务端权威操作面。
- SSE：REST 上的增量事件流，主要用于聊天和审批通知。
- Agent CLI：REST/SSE 的机器友好瘦客户端，不直连磁盘。

## 启动联调服务

```bash
./bin/aipanel --serve --config /tmp/zyhive-dev.json
export ZYHIVE_HOST=http://localhost:8080
export ZYHIVE_TOKEN=dev-only-change-me
```

REST：

```bash
curl -fsS \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  "$ZYHIVE_HOST/api/status"
```

CLI：

```bash
./bin/aipanel system status --json \
  --host "$ZYHIVE_HOST" \
  --token "$ZYHIVE_TOKEN"
```

安装后的二进制名通常是 `zyhive`；仓库本地构建名是 `bin/aipanel`，命令树相同。

## 鉴权和公开路由

管理 API 位于 `/api`，发送：

```text
Authorization: Bearer <auth.token>
```

缺失或错误 token 返回 `401`。服务端未配置 token 时不是“关闭鉴权”，而是管理 API 返回 `503 authentication is not configured`。

无需管理 token 的主要端点包括：

- `GET /api/version`
- `GET /api/update/status`
- `GET /healthz`
- `GET /readyz`
- `GET /api/download`、`GET /api/media`：依赖短期、一次性 ticket
- `/pub/chat/...`：公开 Web 渠道自身的密码和限流边界
- 飞书回调
- 条件启用的 `/metrics`

不要仅凭 URL 前缀推断公开性，以 `internal/api/router.go` 的实际挂载位置为准。

## SSE 聊天

发起：

```bash
curl -N \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d '{"message":"只回复 OK"}' \
  "$ZYHIVE_HOST/api/agents/main/chat"
```

请求可带 `sessionId`、`context`、`scenario`、`skillId`、`images` 和 legacy `history`。`message` 必填。

重连：

```bash
curl -N \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  "$ZYHIVE_HOST/api/agents/main/chat/stream?sessionId=ses-..."
```

状态：

```bash
curl -fsS \
  -H "Authorization: Bearer $ZYHIVE_TOKEN" \
  "$ZYHIVE_HOST/api/agents/main/chat/status?sessionId=ses-..."
```

后端每 15 秒发送 `: keepalive`。断开客户端不会取消后台生成；Broadcaster 会先回放缓冲事件再推送实时事件。Worker 已结束或不存在时，重连收到 `{"type":"idle"}`。

核心事件：

- `thinking_delta`、`text_delta`：`text`
- `tool_call`：`tool_call`
- `tool_result`：`text`，并可带 `tool_call_id`
- `usage`：`input_tokens`、`output_tokens`
- `compaction_start`：`tokens_before`
- `compaction_end`：`tokens_before`、`tokens_after`，失败说明目前放在 `error`
- `done`：`sessionId`、`tokenEstimate`，可能带完整 token 数
- `error`：`error`
- `idle`：无活跃 Worker

## Agent CLI

发现命令：

```bash
zyhive --help
zyhive agent --help
zyhive chat send --help
```

连接优先级：

1. `--host`、`--token`、`--config`
2. `ZYHIVE_HOST`、`ZYHIVE_TOKEN`、`AIPANEL_CONFIG`
3. 本机配置
4. host 默认 `http://localhost:8080`

机器调用：

```bash
zyhive agent list --json
zyhive chat send main '总结今日进展' --json
zyhive cron list --agent main --json
```

有副作用的动作需要确认；CI/非 TTY 中显式添加 `--yes`：

```bash
zyhive agent create --id reviewer --name 审阅员 --yes --json
```

任意 API 逃生舱：

```bash
zyhive api GET /api/status --json
zyhive api POST /api/goals '{"id":"g1","title":"目标"}' --json
echo '{"title":"新标题"}' |
  zyhive api PATCH /api/sessions/main/ses-123 - --json
```

`zyhive api` 是原始逃生舱，当前不会执行 `--yes` 确认；脚本必须自行限制有副作用的 method。专用资源命令才通过 `c.confirm` 实施确认语义。

稳定退出码：

- `0` 成功
- `1` 通用或服务端业务错误
- `2` 参数/用法错误
- `3` 401/403
- `4` 404
- `5` 连接失败

## 变更契约时

端点、字段或事件变更要同时审查：

1. handler 和 router；
2. `ui/src/api/index.ts`；
3. `internal/agentcli`；
4. API/SSE 测试；
5. [REST、SSE 与 CLI 参考](../reference/api-sse-cli.md)；
6. [错误模型](../reference/error-model.md)。

CLI 可以落后于新增 API，因为有 `zyhive api` 兜底；但已有命令的退出码、JSON 输出和确认语义属于兼容契约，不应静默改变。
