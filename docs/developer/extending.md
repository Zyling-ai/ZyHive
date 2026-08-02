# 扩展后端能力

## 先确定扩展层

- 新 HTTP 能力：`internal/api/`，并在 `router.go` 注册。
- 新脚本/Agent 命令：`internal/agentcli/cmd_*.go`，调用已有 REST，不直接读写数据文件。
- 新全局配置：`pkg/config.Config` 及其子结构。
- 新成员配置：`pkg/config.AgentConfig`、`pkg/agent.agentConfig` 和 Manager 保存路径。
- 新工具：`pkg/tools/` 的定义、注册、策略分组、审批和审计。
- 新持久化域：独立 `pkg/<domain>/` Store/Manager，并使用 `pkg/persist`、`pkg/safefs`。
- 新 UI：`ui/src/api/` 类型与客户端、视图/组件、测试、嵌入快照。

## 新增 REST 端点

1. 明确路由属于公开面还是管理面。
2. 管理端挂到 `v1 := r.Group("/api")`，自动经过配置访问保护和 Bearer 鉴权。
3. 公开端点必须自行定义限流、凭证、正文上限和出站限制，不能因为“不带 token”而默认开放。
4. 统一返回 JSON；失败通常使用 `{"error":"..."}`。
5. 为资源 ID 和文件路径使用 `safefs.ValidateResourceID`、`ConfineResource` 或 `ConfineToBase`。
6. 在 `ui/src/api/index.ts` 和 Agent CLI 中复用同一端点，不建立旁路。

新增流式端点时还要设置：

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

每个 `data:` 载荷使用独立 JSON 对象，并提供明确终止事件。

## 新增配置字段

配置当前 schema 版本是 3。新增字段分两类：

- 可选且零值能保持旧行为：通常只需增加带 `omitempty` 的字段及测试。
- 改变语义、默认值或 wire 形状：提升 `CurrentConfigVersion`，在 `applyMigrations()` 增加幂等迁移并持久化。

必须同步考虑：

- `Default()` 的安全默认值；
- `Gateway.Validate()` 或新的验证函数；
- API 脱敏；
- `Clone`/事务保存；
- SecretRef 是否适用；
- 旧配置回归测试；
- `docs/reference/configuration-schema.md`。

不要在 handler 中直接 `json.Marshal` 后 `os.WriteFile` 修改全局配置。优先使用 `config.Transaction`：它在隔离快照上修改，原子保存成功后才发布到内存。

## SecretRef 扩展

SecretRef 只遍历代码明确列出的 credential 字符串字段。目前包括：

- `providers[].apiKey`
- `models[].apiKey`
- `tools[].apiKey`
- `channels[].config` 的每个值
- `auth.token`

新增敏感字段不会自动获得解析与保存保护。必须同时更新：

1. `ResolveSecretRefs`
2. `preserveSecretRefs`
3. ID 对齐/深拷贝辅助逻辑
4. 缺失 env、不可读 file、保存未改凭据引用的测试

## 新增持久化域

推荐模式：

```go
return persist.WriteFile(path, data, 0o600)
```

需要读改写时：

```go
return persist.WithFileLock(path, func() error {
    // 重新读取磁盘事实源，计算 candidate，再 AtomicWrite。
    return persist.AtomicWrite(path, data, 0o600)
})
```

约束：

- 敏感或用户数据目录默认 `0700`，文件默认 `0600`。
- 工作区中面向用户的普通文档有些路径使用 `0755/0644`；不要把该惯例复制到凭据、会话、审计或索引元数据。
- 原子写必须先完整写临时文件、`fsync`、rename、修正 mode，再同步父目录。
- 缓存/索引必须能由事实源重建；先落事实，再更新派生视图。
- 明确单进程锁和跨进程锁边界。
- 不跟随非预期符号链接，不允许路径逃出管理根目录。

## 新增工具

工具不仅是一个执行函数。完整接入应包含：

- JSON 输入结构和验证；
- 注册条件与 capability health；
- 所属 `group:*` 权限组；
- 全局策略与成员策略的交集；
- `allow`、`deny`、`ask` 行为；
- 审批不可用或超时时默认拒绝；
- 工具结果大小限制和 ToolAudit；
- 会话/成员所有权；
- 文件、网络和进程边界；
- 单元测试及 prompt definition 测试。

全局策略是上限，成员策略只能进一步收紧；任一层拒绝都不能通过另一层重新允许。

## 新增 Agent CLI 动作

在 `internal/agentcli` 注册 resource/action，并通过 `Client` 请求 REST：

- 读操作支持 `--json`。
- 有副作用的操作调用 `c.confirm`，使非交互调用必须显式 `--yes`。
- 参数错误返回 `usageErr`，不要退化为通用错误。
- HTTP 401/403、404 和连接失败保留稳定退出码。
- SSE 命令使用 `StreamSSE`，不要自行发明解析器。
- 未封装的端点可继续由 `zyhive api` 访问。

## 完成定义

一个扩展至少应满足：

```bash
gofmt -w <changed-go-files>
go test ./affected/package -count=1
make check
```

如果改变安装、更新、配置持久化、核心旅程或发布资产，再运行 `make release-e2e RELEASE_VERSION=00.0.0v1`。
