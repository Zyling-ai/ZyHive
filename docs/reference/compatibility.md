# 兼容性与迁移

## 平台与工具链

正式发布支持：

- Linux amd64、arm64
- macOS amd64、arm64

正式二进制使用 `CGO_ENABLED=0`。Windows 源码中存在服务和锁兼容实现，但当前发布脚本、安装器和候选矩阵不产出 Windows 正式资产，因此不属于正式支持矩阵。

开发/可复现构建：

- Go 1.25.10
- Node.js >= 22.13.0
- npm 锁文件安装

## 配置兼容

当前 `configVersion=3`。加载会自动执行：

- 旧 `models.primary/apiKeys/fallbacks` 对象 → `models[]`
- v0→v1：为注册表补 ID/状态，补 gateway/auth 默认值
- v1→v2：推断 `supportsTools`，确保默认模型
- v2→v3：模型 API Key 提取为共享 `providers[]`，模型改用 `providerId`

迁移幂等且成功后写回配置。升级前备份；回滚旧二进制时，旧版本未必理解新字段或新数据语义，即使 JSON 能解析也不保证安全写回。

JSON 对未知字段通常宽容，但这不是双向兼容保证。新字段的零值应保持旧行为；需要改变默认行为时必须提升 schema 并提供迁移。

## SecretRef

明文凭据继续兼容。SecretRef 是 credential 字符串中的 JSON 对象文本：

```json
"{\"$env\":\"KEY\"}"
```

读取后内存为明文，未修改凭据保存时保留原引用。直接把对象写到 string 字段不是兼容 wire 形式，会导致 JSON unmarshal 失败。

SecretRef 只覆盖实现列出的字段；未来新增敏感字段在未显式接入前不能假定支持。

## 成员与工作区迁移

Agent Manager 启动时会幂等执行：

- flat `MEMORY.md` → 分层 `memory/`
- 根 `RELATIONS.md` → `workspace/network/`
- `memory/core/user-profile.md` → `owner-profile.md`
- 确保 network 索引存在

成员目录 ID 必须与 `config.json.id` 一致且通过安全 ID 校验，否则启动时忽略。迁移失败通常记录 warning，并不意味着所有数据已可用；升级后检查日志。

## 会话兼容

- 会话使用版本化 append-only JSONL。
- 读取器忽略无法解析或未知的行，并只把 user/assistant 消息用于 LLM 历史。
- compaction 行保留旧历史，但读取上下文从最后压缩摘要继续。
- `sessions.json` 是派生索引，JSONL 是消息事实源。
- Usage 的旧记录可缺少 `session_id`。
- 子成员会话可能位于 `sessions/subagent/`，API 读取有 fallback。

不要用旧版本重写新版本产生的会话文件，除非已验证 entry 类型兼容。

## Network 兼容

- 联系人/群档案是 Markdown frontmatter + body。
- `INDEX.json.chats` 使用 `omitempty`；旧索引没有该字段可读取。
- 联系人与群可以有相同逻辑 ID，因为分别位于 `contacts/`、`chats/`。
- RELATIONS Markdown 解析兼容旧列格式，新格式增加 `toKind` 以区分 agent/contact。
- `INDEX.md`/`INDEX.json` 可重建，不应作为唯一档案。

## API 兼容

- 管理 API 基础路径保持 `/api`。
- Web 公开聊天保留省略 channelId 的 legacy 路由，映射到第一个启用渠道。
- 全局 channels 注册表保留，但主要渠道配置已转为 per-agent。
- 常见错误 wire 为 `{"error":"..."}`，Agent CLI 依赖该字段提取消息。
- `GET /ws` 只是未实现占位，不应被客户端当作稳定 WebSocket。

新增响应字段通常是向后兼容的；删除/改名、状态码变化、公开/鉴权边界变化或分页默认变化需要显式版本策略和客户端同步。

## SSE 兼容

客户端应：

- 忽略未知 `type`；
- 忽略 SSE 注释和非 `data:` 行；
- 以 `done`/`error`/`idle` 判断业务终止；
- 不把网络 EOF 当作成功；
- 使用 `tool_call_id` 关联并行工具结果；
- 对 done 中省略 token 数保持兼容。

服务端新增事件可兼容旧客户端；改变现有字段类型、把多行 data 改为其他 framing、取消 sessionId 重连则属于破坏性变化。

## CLI 兼容

Agent CLI 的稳定自动化契约：

- 全局 flag 可出现在位置参数前后；
- `--json` 输出可机器解析；
- 副作用动作需要 `--yes`；
- 退出码 0/1/2/3/4/5；
- host/token 解析优先级；
- `zyhive api` 可访问尚未封装的 REST。

人类可读文案、列宽和交互式菜单不应被脚本解析。脚本只依赖 `--json`、退出码和明确字段。

## 发布与更新兼容

- 版本格式 `YY.M.DvN`。
- 资产命名固定为 `zyhive-{linux|darwin}-{amd64|arm64}`。
- 安装器和在线更新都先验证 SHA-256。
- 只有管理 API 在线更新要求新进程健康且版本匹配，并在失败时尝试恢复 `.bak`；安装器不提供 watchdog 自动回滚。
- 正式候选在四平台验证前保持 Draft。
- 安装器重复运行同版本必须保留已有配置。

`.bak` 只保证二进制回滚；如果新版本已迁移配置/数据，旧版运行能力仍取决于数据兼容性。因此高风险升级必须先做完整备份并验证恢复。

## 兼容性变更检查

涉及 wire 或磁盘格式时至少验证：

```bash
go test ./pkg/config ./pkg/session ./pkg/network ./internal/api ./internal/agentcli -count=1
make check
make release-e2e RELEASE_VERSION=00.0.0v1
```

并使用上一稳定版产物运行 Release E2E 的真实更新路径，而不只测试新版本全新安装。
