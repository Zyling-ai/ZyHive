# ZyHive 开发者文档

本目录面向修改 ZyHive 源码、集成 REST/SSE/CLI、扩展运行时能力以及准备发布的开发者。内容以当前仓库代码为准；若文档与实现不一致，以对应源码和自动化测试为事实依据。

## 阅读路径

1. [开发环境与本地启动](setup.md)：工具链、依赖安装、构建和最小配置。
2. [前端开发](frontend.md)：Vue/Vite 开发、API 基址与嵌入式 UI 同步。
3. [测试与质量门禁](testing.md)：本地检查、Race、脚本和 Release E2E。
4. [扩展后端能力](extending.md)：API、配置、工具、持久化和迁移的落点。
5. [API、SSE 与 CLI 开发](api-and-cli.md)：三种操作面的契约与联调方式。
6. [发布流程](releasing.md)：版本、构建、Draft 候选与供应链门禁。
7. [贡献指南](contributing.md)：改动范围、测试、生成物与提交要求。

稳定线级参考资料位于 `docs/reference/`：

- [配置结构](../reference/configuration-schema.md)
- [环境变量](../reference/environment-variables.md)
- [数据布局、权限、事实源与缓存](../reference/data-layout.md)
- [REST、SSE 与 CLI 契约](../reference/api-sse-cli.md)
- [兼容性与迁移](../reference/compatibility.md)
- [错误模型](../reference/error-model.md)

## 架构入口

- `cmd/aipanel/main.go`：进程入口、运维 CLI、服务装配。
- `internal/api/router.go`：REST、SSE、公开路由和鉴权边界。
- `internal/agentcli/`：面向 Agent/脚本的 REST 瘦客户端。
- `pkg/config/`：配置结构、迁移、SecretRef 和原子保存。
- `pkg/agent/`：成员生命周期与成员数据目录。
- `pkg/session/`：JSONL 会话、索引、Worker 和 Broadcaster。
- `pkg/runner/`：LLM/工具循环和统一流事件。
- `pkg/tools/`：工具注册、策略、审批与审计接线。
- `pkg/persist/`、`pkg/safefs/`：原子写、跨进程锁与路径约束。
- `ui/src/`：Vue 3 管理界面；`cmd/aipanel/ui_dist/` 是嵌入二进制的构建快照。

## 最短验证闭环

```bash
cd /path/to/ZyHive
cd ui && npm ci && cd ..
make build
make check
```

`make build` 会构建前端、同步 `ui/dist` 到 `cmd/aipanel/ui_dist`，再编译 `bin/aipanel`。不要把仅执行 `go build` 当作完整产品构建。

## 当前边界

ZyHive 当前是单机、自托管 Beta。代码中存在文件锁、原子替换和进程内并发控制，但这不等于多实例共享存储协议。扩展持久化时应继续遵循单机事实源、可恢复索引和默认拒绝的安全边界。
