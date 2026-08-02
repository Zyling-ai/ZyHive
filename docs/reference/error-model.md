# 错误模型

ZyHive 的错误跨 HTTP、SSE、CLI、Runner 和文件持久化传播。调用方应先区分“请求未开始”“业务流中断”“连接断开”和“数据写入失败”。

## HTTP 错误

多数 handler 返回：

```json
{"error":"可读消息"}
```

常见状态：

- `400 Bad Request`：JSON、必填字段、ID、配置或调度无效。
- `401 Unauthorized`：Bearer token、公开渠道密码或票据无效。
- `403 Forbidden`：Origin、权限或资源访问被拒绝。
- `404 Not Found`：资源不存在；实验功能关闭时也可故意返回 404。
- `409 Conflict`：状态/资源冲突（具体 handler 决定）。
- `413 Request Entity Too Large`：正文超过限制。
- `429 Too Many Requests`：公开接口/模型频率限制，通常带 `Retry-After`。
- `500 Internal Server Error`：未恢复的服务端/存储错误。
- `503 Service Unavailable`：未配置鉴权、Worker/公开聊天容量、readiness 或临时服务不可用。

所有 HTTP 请求有 `X-Trace-Id`。报告问题时同时记录 method、path、status、响应 error 和 trace ID，不要记录 token。

鉴权 token 为空时管理 API 返回 503，而不是允许匿名访问。未知路由在 UI 已启用时，非 `/api` 路径可能进入 SPA fallback；API 客户端必须使用正确 `/api` 前缀。

## SSE 错误

聊天请求在开始输出前失败时，返回普通非 2xx JSON，例如成员不存在、模型/Key 缺失或队列不可用。

流建立后，业务错误通过：

```text
data: {"type":"error","error":"..."}
```

`error` 是终止事件。已收到的 `text_delta` 仍是部分输出，客户端不应覆盖或误标为完整成功。

网络 EOF、浏览器 abort 或代理断线不是业务 `error`：

1. 保留已接收内容。
2. 用已知 `sessionId` 查询 `/chat/status`。
3. Worker 存在时连接 `/chat/stream` 获取缓冲和实时事件。
4. 收到 `idle` 时从持久会话重新读取最终历史。
5. 不无条件重发用户消息，否则可能重复执行工具/计费。

`compaction_end` 可能带 `error` 字段，但事件本身不等于整个聊天失败；按事件 `type` 判断。

## LLM 错误

LLM 层把错误分为可重试瞬时错误和业务/永久错误：

- 瞬时：典型 429、5xx、connection reset、EOF、TLS/网络瞬断；按退避策略重试，并在适用时尊重 Retry-After。
- 不应自动重试：401/鉴权、context length、content filter 等。

Provider `status=error` 且模型绑定该 Provider 时，聊天解析会提前返回明确错误，提示重新测试或替换 Key。没有模型或必需 Key 分别返回 `no model configured`、`no API key configured...`。

重试必须位于 LLM client 层，避免上层重复整个用户 turn 和工具副作用。

## 工具、审批与策略

- 全局或成员策略 deny：工具不可见或调用被拒。
- ask：等待审批；审批中心不可用、拒绝或超时都默认拒绝。
- 工具执行失败作为 `tool_result` 或 runner error 传播，具体取决于工具循环能否继续。
- 路径、网络、进程、成员/会话所有权检查失败不能自动降级。
- 工具输出可能被截断或落入审计 blob；“审计摘要不含完整结果”不等于执行失败。

对具有副作用的工具，不要在未知执行状态下盲目重试；先查询资源或审计记录。

## CLI 错误

Agent CLI 把非 2xx 封装为：

```text
HTTP <status>: <server error>
```

退出码：

- `0` 成功
- `1` 通用、服务端业务或流内错误
- `2` 参数/用法错误
- `3` HTTP 401/403
- `4` HTTP 404
- `5` DNS、拒绝连接等 transport failure

脚本示例：

```bash
if ! output="$(zyhive agent get worker --json 2>err.log)"; then
  code=$?
  case "$code" in
    3) echo "鉴权失败" ;;
    4) echo "成员不存在" ;;
    5) echo "服务不可达" ;;
    *) echo "调用失败" ;;
  esac
fi
```

注意 shell 的 `!` 会影响 `$?` 捕获；更稳妥写法是临时关闭 `set -e` 后直接执行并保存状态：

```bash
set +e
output="$(zyhive agent get worker --json 2>err.log)"
code=$?
set -e
```

`--json` 只保证成功输出机器可读；错误诊断写 stderr，调用方应同时检查退出码。

## 配置与 SecretRef 错误

- JSON 语法/wire 类型不匹配：配置加载失败，服务不启动。
- gateway port/bind 无效：加载失败。
- SecretRef env 未设置或值为空：加载失败。
- SecretRef file 不可读：加载失败。
- 未识别/非法的对象文本会按普通字符串保留，可能导致后续 Provider 鉴权错误。
- 同时含 `$env`、`$file` 时解析优先 env，但保存引用检测不认可该形式；视为无效配置用法。

配置事务先写磁盘再发布内存；写失败不应污染运行配置。直接手工修改磁盘则没有该保证。

## 持久化错误

标准持久化路径使用进程内 mutex、相邻 lock file、临时文件、fsync、rename、chmod 和父目录同步。任一阶段失败返回错误，旧事实源应保持不变。

不同域的恢复策略：

- 会话：JSONL 是事实源，索引更新失败可后续对账。
- Network：Markdown 档案是事实源，INDEX 可重建。
- Cron：无法解析 `jobs.json` 会阻止正常加载；单个无效 job 会被禁用并记录原因。
- Agent/Project：不安全 ID、ID/目录不一致或损坏 meta/config 在加载时被忽略或报错。
- 备份恢复：manifest、摘要、路径或完整性在提交前失败必须拒绝；分阶段替换中途失败会尽力回滚已替换数据，但断电或回滚自身失败时仍需人工恢复。

不要通过删除 lock、索引或校验文件来“修复”正在运行的实例。先停服务、备份现场，再按事实源恢复。

## Readiness 与健康

- `/healthz` 用于进程基础健康。
- `/readyz` 用于是否适合接收流量；Cron heartbeat 停滞、Worker 过载或已探测 Provider 全失败时可返回 503；冷启动无 Provider 探测数据时可返回 200。
- `/api/health` 是鉴权管理端健康接口。

发布和更新以 `/healthz`、版本匹配及完整候选旅程为门禁，不能只以进程存在或端口打开判断成功。

## 重试建议

- GET/幂等读：对连接失败、429、部分 5xx 使用有上限退避。
- 配置 PATCH、创建、工具/聊天：先判断服务端是否已接受；没有幂等键时不要自动整请求重试。
- SSE：重连现有 session，不重发 turn。
- CLI exit 2/3/4：修正输入/凭据/资源，不重试。
- CLI exit 5：确认 host 和服务状态后有限重试。
