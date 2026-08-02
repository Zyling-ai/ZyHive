# 渠道与公开聊天

![消息渠道处理链](../assets/diagrams/channel-flow.svg)


## 1. 渠道模型

稳定主线是成员级 Channel：每个 Agent 的配置中可有 Telegram、飞书和 Web 条目。`channel.BotPool` 管理运行中的 Telegram/飞书实例，支持按 `{agentID, channelID}` 启停。

全局 `Config.Channels` 仍保留兼容结构，但产品运行应以成员级渠道为准。历史双轨字段存在不代表两套入口都应继续扩展。

## 2. Telegram

启动时 `main` 遍历成员 Channel，针对启用且有 token 的条目：

1. 创建 per-channel PendingStore；
2. 构造调用 `Pool.RunStreamEvents` 的 stream function；
3. 注入 allowFrom 动态读取；
4. 创建 `TelegramBotWithStream`；
5. getMe 成功后更新 channel 状态和 botName；
6. 注册到 BotPool。

消息处理使用 chat/thread 导出的持久 sessionID，可下载媒体并提供 `FileSenderFunc`。联系人自动 `Resolve`，群聊可建群档案。授权名单必须在进入 LLM 前检查；Bot token 不应出现在 API 响应或日志。

## 3. 飞书

飞书使用 WebSocket 长连接接收消息，并有 HTTP card callback：

- 凭据和 scope 可通过 probe/向导检查；
- 群聊 @ 模式、sender/chat 摘要进入额外上下文；
- 流式输出可更新卡片；
- 动态飞书工具按配置注册；
- 回调入口在管理鉴权外，必须验证飞书签名、时间窗和重放。

WebSocket/卡片失败与模型执行失败是不同故障域。模型已经产生回复但卡片更新失败时，Session/convlog 可能已有结果，重试发送不能重新执行非幂等工具。

## 4. 管理端 Web 与“web”来源

管理端聊天走受 Bearer Token 保护的 `/api/agents/:id/chat`，但 chatlog 中 `ChannelType` 也写为 `"web"`。Public Chat 的 session 也以 `web-` 开头。

因此：

- `web` 可能表示浏览器介质，不等于匿名；
- 授权边界必须看路由组、Channel 和 worker owner；
- UI 来源标签可用 session/source 推断，但不能用于服务端授权。

管理端支持完整受 Policy 控制的工具、Skill Studio scenario、图片、共享项目、Usage/Budget 和 Artifact file sender。

## 5. Public Chat 路由

无管理员 token 的主要路由：

```text
GET  /pub/chat/:agentId/:channelId/info
GET  /pub/chat/:agentId/:channelId/history
POST /pub/chat/:agentId/:channelId/stream
GET  /pub/chat/:agentId/:channelId/reconnect
```

Public 路由仍经过 `configAccessGuard`、公共请求限流和可选 channel password。Password 是 Channel 级共享秘密，不等价于管理员认证。

Session ID 为 `web-<channelID>-<sanitized sessionToken>`；token 只保留字母数字、`-`、`_`，最多 64 字符。无 token 时服务端生成临时 ID。

## 6. Public 执行路径

```text
resolve agent/channel/password
  → 请求体与 message size 检查
  → 构造 sessionID
  → limiter.admit(source, owner+session)
  → acquireSSE
  → WorkerPool.GetOrCreate("public/agent/channel", session)
  → runPublic（带 run timeout）
  → 空工具 Registry + SupportsTools=false
  → 联系人自动建档与摘要注入
  → Runner → Broadcaster → SSE
```

虽然 `publicChatHandler` 持有 `agent.Pool`，`runPublic` 当前仍自行创建 LLM Client、Store、Registry 和 Runner；它不是 `Pool.RunStreamEvents` 的包装。这一点与管理端聊天一样构成重复装配。

Public Runner 当前未接入管理端/Pool 的完整 UsageRecorder、BudgetCheck 和 CapabilitiesContext；外层公共 limiter 负责请求、任务和时间限制。修改公共计费/治理时必须单独检查此路径。

## 7. 公共限额

默认限制包括：

- 每来源请求频率与模型执行频率；
- 每来源 24 小时会话数；
- 全局活跃会话；
- 模型任务并发；
- SSE 连接并发；
- 同一 public owner/session 单任务；
- 单消息大小；
- 单轮运行超时和 session TTL。

环境变量可调整，但提高限额会直接扩大模型费用和资源 DoS 面。只有明确处于可信反向代理后才可启用 `ZYHIVE_TRUST_PROXY_HEADERS=1`；实现读取 `CF-Connecting-IP` 和 `X-Real-IP`，若客户端可直接访问服务，伪造这两个 Header 会绕过来源限流。

## 8. 公共工具与数据边界

Public Registry 强制 `Deny:["*"]` 且 `SupportsTools=false`。即使成员在管理端拥有 full profile，匿名访客也不能：

- 执行 shell/process/ACP；
- 读写成员工作区；
- 控制浏览器；
- 修改技能、身份、环境；
- 创建项目、Cron 或发送消息。

Public 消息会：

- 写入该成员 Session；
- 写管理员可见 `convlog`；
- 可按 visitor token 自动创建 `network/contacts/web-*.md`；
- 将联系人短摘要注入本轮。

这意味着“无登录”不是“无持久数据”。部署方必须披露保留策略，并避免把 sessionToken 当作已验证真人身份。

## 9. Worker 与断线

管理端和 Public 都使用 Worker/Broadcaster：

- 客户端断线不会立即停止后台模型；
- Public 的 run context 额外有服务端 timeout；
- reconnect 必须使用属于当前 Channel 的 `web-<channelID>-` 前缀，并以 public owner 查 Worker；
- SSE 结束时释放连接计数，RunFn 结束时释放模型任务/会话 admission。

若 enqueue 后客户端立刻断线，任务仍可能完成并产生费用。限额必须统计任务而不只是在线 SSE 数。

## 10. 外部内容信任

Telegram、飞书、Public 消息、联系人和群档案都是不可信输入。实验 `PromptDef` 包装不是所有流式路径都可假定已统一覆盖，也不是安全解析器。真实边界应由：

- 渠道签名/allowlist/password；
- Public 最小工具；
- Policy/Approval；
- 路径和网络 guard；
- 低权限部署；
- 系统提示词中明确的外部资料标记

共同组成。任何单层提示词防注入都不能阻止已授权高权限工具被模型误用。
