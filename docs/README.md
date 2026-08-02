# ZyHive 文档中心

> 事实基线：`26.8.2v1`（2026-08-02）。发生冲突时，优先级为：同版本代码与测试 → 正式 Release 与 [`CHANGELOG`](../CHANGELOG.md) → 当前文档 → 历史设计和提案。

## 从这里开始

1. [安装 ZyHive](getting-started/installation.md)
2. [理解成员、会话、工作区与权限](getting-started/concepts.md)
3. [创建第一个团队并完成首次对话](getting-started/first-team.md)
4. [确认功能状态与已接受风险](reference/feature-status.md)

## 用户手册

- [用户功能总览](user-guide/README.md)
- [成员与聊天](user-guide/agents-and-chat.md)
- [工作区、记忆与通讯录](user-guide/workspace-memory-network.md)
- [模型、工具策略与审批](user-guide/models-tools-approvals.md)
- [技能与 SkillOpt](user-guide/skills-and-skillopt.md)
- [目标、Cron、项目与后台任务](user-guide/goals-cron-projects-tasks.md)
- [Telegram、飞书与公开 Web 渠道](user-guide/channels.md)
- [用量、审计与日志](user-guide/usage-audit-logs.md)
- [设置、更新与公开聊天](user-guide/settings-update-public-chat.md)

## 部署与运维

- [部署运维入口](admin/README.md)
- [Linux/macOS 部署与 TLS 反代](admin/deployment.md)
- [配置、SecretRef 与权限](admin/configuration.md)
- [安全加固与信任边界](admin/security-hardening.md)
- [备份和恢复](admin/backup-and-restore.md)
- [在线更新与回滚](admin/upgrade-and-rollback.md)
- [日志、探针与容量观察](admin/observability.md)
- [故障排查](admin/troubleshooting.md)

## 架构与实现原理

- [架构阅读入口](architecture/README.md)
- [系统总体架构](architecture/system-overview.md)
- [启动与依赖装配](architecture/startup-and-dependencies.md)
- [Runner 与 Agentic Loop](architecture/runtime-and-agent-loop.md)
- [Session 与 Compaction](architecture/sessions-and-compaction.md)
- [记忆、关系与协作](architecture/memory-and-collaboration.md)
- [工具、策略与审批](architecture/tools-policy-and-approval.md)
- [渠道与公开聊天](architecture/channels-and-public-chat.md)
- [持久化与一致性](architecture/persistence-and-consistency.md)
- [安全与信任边界](architecture/security-and-trust-boundaries.md)
- [Draft-first 发布架构](architecture/release-architecture.md)

## 开发者与集成方

- [开发者入口](developer/README.md)
- [本地开发](developer/setup.md) · [前端与嵌入资源](developer/frontend.md) · [测试分层](developer/testing.md)
- [扩展 Provider、Tool 与 Channel](developer/extending.md)
- [API、SSE 与 CLI](developer/api-and-cli.md)
- [发布流程](developer/releasing.md) · [贡献指南](developer/contributing.md)
- 参考：[配置 Schema](reference/configuration-schema.md) · [环境变量](reference/environment-variables.md) · [数据布局](reference/data-layout.md) · [API/SSE/CLI 对照](reference/api-sse-cli.md) · [兼容性](reference/compatibility.md) · [错误模型](reference/error-model.md)

## 治理、实验与历史

- [文档治理](governance/README.md) · [覆盖矩阵](governance/coverage-matrix.md) · [开源与商业边界](governance/commercial-boundaries.md)
- [Labs](labs/README.md)：aiteam 与 SkillOpt 等非稳定承诺能力
- [Archive](archive/README.md)：旧路线、审计、实现快照和 ZyStudio；仅供追溯
- [图表源与生成方式](assets/diagrams/README.md) · [脱敏截图说明](assets/screenshots/README.md)

提案、演示成功、旧版本截图或代码中存在的占位路由不能单独证明当前稳定支持。命令参数以 `zyhive --help` 为准，动态工具以运行时 Capabilities 和工具体检为准。
