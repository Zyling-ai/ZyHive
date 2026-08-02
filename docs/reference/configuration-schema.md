# 配置结构参考

ZyHive 主配置是 JSON。当前 schema 版本为 `3`，权威定义在 `pkg/config/config.go`，SecretRef 行为在 `pkg/config/secretref.go`。

## 查找与启动

服务模式通常显式指定：

```bash
zyhive --serve --config /etc/zyhive/zyhive.json
```

CLI 自动查找顺序：

1. `AIPANEL_CONFIG`
2. `/etc/zyhive/zyhive.json`
3. `/usr/local/etc/zyhive/zyhive.json`
4. `$HOME/.config/zyhive/zyhive.json`
5. 当前目录 `aipanel.json`

新配置由 `config.Default()` 生成：端口 8080、`bind=lan`、`agents.dir=./agents`、随机 32 个十六进制字符 token，注册表为空。保存使用原子替换和 `0600`。

## 顶层结构

```json
{
  "configVersion": 3,
  "gateway": {},
  "agents": {},
  "providers": [],
  "models": [],
  "channels": [],
  "tools": [],
  "skills": [],
  "acpAgents": [],
  "auth": {},
  "toolPolicy": {},
  "budget": {},
  "throttle": {},
  "aiteam": {}
}
```

### `gateway`

- `port`：`0..65535`；`0` 的 URL fallback 是 8080。
- `bind`：空、`localhost`、`lan`、`all` 或合法 IP。注意当前校验不接受 README 历史示例中的字符串 `0.0.0.0` 以外的别名；`0.0.0.0` 本身作为 IP 合法。
- `publicUrl`：外部规范基址；非空时优先用于文件 ticket 和 CLI BaseURL。
- `cors.allowedOrigins[]`：允许的跨源浏览器 Origin。同源始终允许；值必须是无路径/查询/用户信息的完整 Origin。

### `agents`

- `dir`：成员根目录。启动时转为绝对路径；相对路径相对进程当前工作目录。

### `providers[]`

- `id`：注册表 ID。
- `name`：显示名。
- `provider`：如 `anthropic`、`openai`、`deepseek`、`openrouter`、`zhipu`、`kimi`、`minimax`、`qwen`、`custom`、`ollama`。
- `apiKey`：共享凭据，可为明文或 SecretRef 字符串。
- `baseUrl`：可选 Provider 基址。
- `embedModel`：可选 embedding 模型覆盖。
- `status`：`ok`、`error`、`untested`。

### `models[]`

- `id`、`name`
- `provider`、`model`
- `providerId`：优先引用 `providers[].id`。
- `apiKey`：旧版/模型级兼容字段。
- `baseUrl`：模型级覆盖；解析时优先于 Provider `baseUrl`。
- `isDefault`：全局默认标记；没有标记时运行时取第一项，迁移 v2 会补一个默认项。
- `status`：连通性状态。
- `supportsTools`：省略时按模型名推断；可显式覆盖。已知 `reasoner`、`o1-mini`、`o1-preview`、`o1-2024` 模式默认不支持工具。

凭据优先级为 `model.providerId` 指向的 Provider，其次才是 `model.apiKey`。

### `channels[]`

全局兼容注册表：

- `id`、`name`、`type`
- `config`：字符串键值；所有值都参与 SecretRef 解析。
- `enabled`
- `status`

主要渠道已是 per-agent 配置，存放在成员 `config.json` 的同形 `channels[]`；全局列表仍为兼容 API 保留。

### `tools[]`

- `id`、`name`、`type`
- `apiKey`：可用 SecretRef。
- `baseUrl`
- `enabled`
- `status`

### `skills[]`

- `id`、`name`、`description`、`version`
- `path`
- `enabled`

### `acpAgents[]`

- `id`、`name`
- `binary`：命令名或绝对路径。
- `args[]`：额外参数；可用 `{{task}}` 注入任务。
- `workDir`：空时使用成员工作区。
- `env[]`：`KEY=VALUE` 子进程环境。
- `status`

### `auth`

- `mode`：当前主要值为 `token`。
- `token`：Bearer token，可用 SecretRef。空 token 会令管理 API 返回 503，不会关闭鉴权。

### `toolPolicy`

由 `pkg/tools` 解释的原始 JSON：

```json
{
  "profile": "coding",
  "allow": ["read", "group:web"],
  "deny": ["exec"],
  "ask": ["write"]
}
```

Profile 为 `full`、`coding`、`messaging`、`minimal`。全局策略是权限上限，成员策略只能继续收紧；deny 优先，ask 需要审批，审批不可用默认拒绝。

### `budget`

- `enabled`
- `global_daily_usd`
- `default_agent_daily_usd`
- `warn_at_pct`
- `tz`

默认关闭。启用后 runner 在每轮 LLM 调用前执行预算检查。

### `throttle`

```json
{
  "kind": "adaptive",
  "global_max_inflight": 0,
  "default": {
    "min": 1,
    "max": 4,
    "init": 2,
    "grow_every": 20,
    "max_backoff_ms": 0
  },
  "providers": {
    "anthropic": {"min": 1, "max": 8, "init": 4, "grow_every": 10}
  }
}
```

- `kind`：`fixed`（默认）或 `adaptive`。
- fixed 且 `global_max_inflight=0` 表示不做全局 gate。
- adaptive 使用 Provider 级 AIMD，429/503 降低并发，并尊重 Retry-After。

### `aiteam`

当前稳定结构仅公开 `aiteam.judge`：

- `model`：`models[].id`
- `max_tokens`：默认 1024
- `timeout_ms`：默认 30000

只有相应 `ZYHIVE_EXPERIMENTAL_*` 开关启用时实验配置才生效。

## 成员 `config.json`

每个成员目录保存：

- `id`、`name`、`description`
- `model`：旧式 `provider/model`
- `modelId`：优先引用全局模型 ID
- per-agent `channels[]`
- `toolIds[]`、`skillIds[]`
- `avatarColor`
- `system`
- `env`：传给成员 exec 工具的字符串映射
- `heartbeat`：`enabled`、`intervalMin`、`prompt`
- `toolPolicy`

该文件由 Agent Manager 管理，使用 `0600`。不要手工同时修改磁盘文件和运行时对象；应走管理 API。

## SecretRef wire 语义

配置结构中的凭据字段是 Go `string`，因此 SecretRef 是“双层 JSON”：

```json
{
  "apiKey": "{\"$env\":\"ANTHROPIC_API_KEY\"}"
}
```

或：

```json
{
  "apiKey": "{\"$file\":\"/run/secrets/anthropic\"}"
}
```

不是：

```json
{
  "apiKey": {"$env":"ANTHROPIC_API_KEY"}
}
```

解析规则：

- 字符串去空白后不以 `{` 开头：原样使用。
- 可解析对象且 `$env` 非空：读取环境变量；空值也视为未设置并报错。
- 否则 `$file` 非空：读取文件并只去掉末尾 `\n`/`\r`。
- 非法 JSON、未知对象或两个键都空：按普通字符串保留。
- 若 `$env` 和 `$file` 同时存在，运行时解析优先 `$env`；但保存时引用识别要求恰好一个非空，故不要同时设置。

启动后内存中保存解析出的明文。配置事务保存时，如果该凭据相对修改前未变化，会从磁盘恢复原始 SecretRef 字符串，避免把解析后的明文写回；实际修改过的凭据会写入新值。

## 迁移

- legacy `models.primary/apiKeys/fallbacks` 对象会迁移为数组。
- v0→v1：补 ID、状态、bind 和 auth mode。
- v1→v2：补工具支持标记、默认模型。
- v2→v3：把模型 API Key 抽到共享 Provider，并设置 `providerId`。

迁移在 `Load()` 时幂等执行并保存。备份配置后再用新版本启动；不要把新版本写回的配置交给不理解新 schema 的旧版本长期运行。
