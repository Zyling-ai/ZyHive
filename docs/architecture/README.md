# ZyHive 架构文档

> 基准：仓库当前代码与 `26.8.2v1` 发布链，复核日期 2026-08-02。本文档描述“已经接线的事实”，规划能力会明确标注为未实现或实验性。

ZyHive 是单机、自托管、单 Go 进程的 AI 团队运行台。Go 二进制嵌入 Vue 构建产物，进程内装配 API、成员池、会话工作者、渠道机器人、Cron、工具、审批和本地文件存储。它不是微服务、不是多租户平台，也还不是具备持久化检查点的工作流引擎。

## 阅读顺序

1. [系统总览](system-overview.md)：组件边界、入口、数据目录和全局数据流。
2. [启动与依赖装配](startup-and-dependencies.md)：`main` 的启动顺序、依赖注入和关闭顺序。
3. [运行时与 Agent 循环](runtime-and-agent-loop.md)：`Runner`、模型流、工具循环、并行行为和管理端/Pool 差异。
4. [会话与上下文压缩](sessions-and-compaction.md)：JSONL、Worker/Broadcaster、恢复语义、新旧压缩实现。
5. [记忆与协作](memory-and-collaboration.md)：分层记忆、通讯录、项目、关系、子成员及未接线的 `SessionMemory`。
6. [工具策略与审批](tools-policy-and-approval.md)：动态注册、双层 Policy、Ask、Audit 和宿主机执行风险。
7. [渠道与公开聊天](channels-and-public-chat.md)：Telegram、飞书、公共 Web、身份和限额边界。
8. [持久化与一致性](persistence-and-consistency.md)：文件锁、原子替换、配置事务、Cron claim 和已知限制。
9. [安全与信任边界](security-and-trust-boundaries.md)：鉴权、路径、网络、Secret、外部输入和 sandbox 边界。
10. [发布架构](release-architecture.md)：Draft-first、可复现候选、供应链和升级回滚门禁。

## 架构事实的优先级

发生冲突时按以下顺序判断：

1. 当前可执行代码与测试；
2. CI、发布脚本和安装/升级 E2E；
3. 本目录文档；
4. README、CHANGELOG 和历史设计文档；
5. `proposals/`、路线图和注释中的未来计划。

## 关键限制速览

- 管理端聊天在 `internal/api/chat.go` 内自行装配 `Runner`，并经 `WorkerPool` 运行；Telegram、飞书、Cron、Heartbeat、Public Chat 和 Subagent 多数经 `agent.Pool` 的不同方法或各自闭包装配。当前没有唯一的 `TurnService/RunnerFactory`。
- `pkg/session/compaction.go` 是当前 Runner 使用的压缩实现；`pkg/compaction/compaction.go` 是旧实现，仍可编译但不在主运行路径。
- `pkg/memory/session_memory.go` 和 `runner.InjectSessionMemory` 已有实现与测试，但生产装配没有创建 manager、调用 `MaybeExtract` 或注入 `LoadForPrompt`，因此不能宣称自动会话记忆已生效。
- `ZYHIVE_EXPERIMENTAL_SANDBOX` 只启用弱加固：临时 HOME、进程组、超时、资源/输出约束；它没有容器、chroot、命名空间、文件系统隔离或网络防火墙，不是强安全边界。
- Cron 对计划 occurrence 写持久化 claim；中断后标记 `uncertain` 并停止自动重放未知副作用，不等于完整的 exactly-once 事务。
- 正式发布采用 Draft-first：候选先保持不可见，全部门禁成功后唯一一次转为 Published/latest；失败候选不得先公开再撤回。

## 图示引用约定

本目录当前不创建或假定不存在的图。以后若在 `docs/assets/diagrams/` 增加 SVG，从本目录引用必须写成 `../assets/diagrams/<name>.svg`。
