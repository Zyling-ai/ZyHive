# 文档覆盖矩阵

> 基线：`26.8.2v1`  
> 用法：本矩阵检查“实现证据是否有可发现的文档”，不以文档反向证明实现。状态定义见[文档政策](documentation-policy.md)。

## 判定说明

- **已覆盖**：存在当前入口和对应实现/测试证据。
- **部分覆盖**：已有说明，但入口分散、粒度不足或仍需按当前版本复核。
- **Labs**：有实验实现和边界文档，不进入稳定承诺。
- **历史**：只保留背景或当时快照，不参与当前能力判定。

更新实现路径、路由、CLI 注册、部署脚本、安全边界或版本流程时，变更作者应同步更新对应行。

## UI 路由

路由事实源：`ui/src/router/index.ts`；嵌入发行物由 `make build`/`make sync-ui` 从 `ui/dist` 同步到 `cmd/aipanel/ui_dist`。

| 范围 | 当前路由 | 标签 | 文档入口 | 覆盖 |
| --- | --- | --- | --- | --- |
| 登录与主对话 | `/login`、`/`、`/chats` | Stable | `docs/getting-started/first-team.md`、`docs/user-guide/agents-and-chat.md` | 已覆盖 |
| 总览与成员 | `/dashboard`、`/agents`、`/agents/new`、`/agents/:id` | Stable | `docs/user-guide/agents-and-chat.md` | 已覆盖 |
| 模型、渠道、工具、技能 | `/config/models`、`/config/channels`、`/config/tools`、`/skills` | Stable | `docs/user-guide/models-tools-approvals.md`、`docs/user-guide/channels.md`、`docs/user-guide/skills-and-skillopt.md` | 已覆盖 |
| 目标、自动化与任务 | `/goals`、`/cron`、`/tasks` | Stable | `docs/user-guide/goals-cron-projects-tasks.md` | 已覆盖 |
| 团队与项目 | `/team`、`/projects` | Stable | `docs/user-guide/workspace-memory-network.md`、`docs/user-guide/goals-cron-projects-tasks.md` | 已覆盖 |
| 日志、用量与审计 | `/logs`、`/usage`、`/tool-audit` | Stable | `docs/user-guide/usage-audit-logs.md`、`docs/admin/observability.md` | 已覆盖 |
| 设置 | `/settings` | Stable | `docs/user-guide/settings-update-public-chat.md`、`docs/admin/configuration.md` | 已覆盖 |
| 公开聊天 | `/chat/:agentId/:channelId`、`/chat/:agentId` | Stable | `docs/user-guide/settings-update-public-chat.md`、`docs/architecture/channels-and-public-chat.md` | 已覆盖 |
| 兼容重定向 | `/config/skills`、`/config` | Stable（兼容） | `docs/reference/compatibility.md` | 部分覆盖；删除前需记录弃用窗口 |
| aiteam | `/aiteam`、`/aiteam/wallet`、`/aiteam/fx`、`/aiteam/guard`、`/aiteam/judge`、`/aiteam/payroll` | Labs | `docs/labs/aiteam.md` | Labs |

## 后端模块

API 路由事实源为 `internal/api/router.go` 与各 handler；运行时模块事实源为 `pkg/` 实现及其 `*_test.go`。

| 能力域 | 实现与测试证据 | 标签 | 文档入口 | 覆盖 |
| --- | --- | --- | --- | --- |
| API、鉴权与限流 | `internal/api/`，尤其 `router.go`、`authcompare*`、`bodylimit*`、`public_limits*` | Stable | `docs/developer/api-and-cli.md`、`docs/reference/api-sse-cli.md`、`docs/architecture/security-and-trust-boundaries.md` | 已覆盖 |
| Agent、Runner 与协作 | `pkg/agent/`、`pkg/runner/`、`pkg/subagent/`、`pkg/network/`、`pkg/project/` | Stable | `docs/architecture/runtime-and-agent-loop.md`、`docs/architecture/memory-and-collaboration.md` | 已覆盖 |
| 会话、压缩与持久化 | `pkg/session/`、`pkg/persist/`、`pkg/compaction/`、`pkg/chatlog/`、`pkg/convlog/` | Stable | `docs/architecture/sessions-and-compaction.md`、`docs/architecture/persistence-and-consistency.md`、`docs/reference/data-layout.md` | 已覆盖 |
| 模型与外部调用 | `pkg/llm/`、`internal/api/models.go`、`providers.go` | Stable | `docs/user-guide/models-tools-approvals.md`、`docs/reference/configuration-schema.md` | 已覆盖 |
| 工具、审批与审计 | `pkg/tools/`、`pkg/toolaudit/`、`internal/api/approvals.go` | Stable；Sandbox 为 Labs | `docs/architecture/tools-policy-and-approval.md`、`docs/user-guide/usage-audit-logs.md` | 已覆盖 |
| 渠道与公开聊天 | `pkg/channel/`、`internal/api/public_chat.go`、`feishu_*` | Stable | `docs/architecture/channels-and-public-chat.md`、`docs/user-guide/channels.md` | 已覆盖 |
| Cron、Goal、任务 | `pkg/cron/`、`pkg/goal/`、`internal/api/cron.go`、`goals.go`、`subagents.go` | Stable | `docs/user-guide/goals-cron-projects-tasks.md` | 已覆盖 |
| 记忆与工作区 | `pkg/memory/`、`internal/api/memory.go`、`files.go` | Stable | `docs/user-guide/workspace-memory-network.md`、`docs/architecture/memory-and-collaboration.md` | 已覆盖 |
| 备份、恢复与更新 | `pkg/backup/`、`internal/api/update*`、`cmd/aipanel/backup_cli*` | Stable | `docs/admin/backup-and-restore.md`、`docs/admin/upgrade-and-rollback.md` | 已覆盖 |
| 配置与安全文件访问 | `pkg/config/`、`pkg/safefs/`、`pkg/netguard/` | Stable | `docs/admin/configuration.md`、`docs/reference/configuration-schema.md`、`docs/architecture/security-and-trust-boundaries.md` | 已覆盖 |
| SkillOpt | `pkg/skillopt/`、`internal/api/skillopt.go` | Labs | `docs/user-guide/skills-and-skillopt.md`、`docs/archive/designs/skillopt-design.md` | 部分覆盖；设计稿不能代替当前实验操作边界 |
| aiteam | `pkg/aiteam/`、`internal/api/aiteam_routes.go`、`aiteam_*_test.go` | Labs | `docs/labs/aiteam.md` | Labs |

## CLI

命令事实源：`cmd/aipanel/main.go`、`cmd/aipanel/cli.go`、`internal/agentcli/` 及当前二进制 `zyhive --help`。

| 范围 | 当前入口 | 标签 | 文档入口 | 覆盖 |
| --- | --- | --- | --- | --- |
| 交互运维与服务控制 | `zyhive`、`start`、`stop`、`restart`、`status`、`enable`、`disable`、`token`、`version` | Stable | `docs/admin/deployment.md`、`docs/admin/troubleshooting.md` | 部分覆盖；参数以 `--help` 为准 |
| 备份恢复 | `zyhive backup create|inspect|restore` | Stable | `docs/admin/backup-and-restore.md` | 已覆盖 |
| 核心资源 | `agent`、`chat`、`cron`、`memory`、`task`、`goal` | Stable | `docs/developer/api-and-cli.md` | 已覆盖 |
| 团队与知识 | `network`、`relation`、`project`、`file`、`session` | Stable | `docs/developer/api-and-cli.md` | 已覆盖 |
| 配置与可观测 | `model`、`provider`、`channel`、`tool`、`acp`、`skill`、`usage`、`system`、`conversations`、`approval` | Stable | `docs/developer/api-and-cli.md` | 已覆盖 |
| 原始 API 逃生舱 | `zyhive api <METHOD> <path>` | Stable（高级） | `docs/developer/api-and-cli.md`、`docs/developer/api-and-cli.md` | 已覆盖 |

## 部署与运行

| 范围 | 事实源 | 标签 | 文档入口 | 覆盖 |
| --- | --- | --- | --- | --- |
| 安装与平台支持 | `install.sh`、`install-worker/`、Release 资产与安装测试 | Stable | `docs/getting-started/installation.md` | 已覆盖 |
| 单机服务与反向代理 | `cmd/aipanel/`、服务模板/安装逻辑、配置 schema | Stable | `docs/admin/deployment.md`、`docs/admin/configuration.md` | 已覆盖 |
| 升级与回滚 | `internal/api/update*`、Release E2E | Stable | `docs/admin/upgrade-and-rollback.md` | 已覆盖 |
| 备份恢复 | `pkg/backup/`、`scripts/test/release-e2e/` | Stable | `docs/admin/backup-and-restore.md` | 已覆盖 |
| 可观测与排障 | `pkg/logging/`、日志/用量 API | Stable | `docs/admin/observability.md`、`docs/admin/troubleshooting.md` | 已覆盖 |
| aiteam AWS 部署稿 | `docs/labs/aiteam/aiteam-deploy-aws.md` | Labs/历史 | `docs/labs/aiteam.md`、`docs/archive/README.md` | 不作为当前生产部署说明 |

## 安全

| 范围 | 事实源 | 标签 | 文档入口 | 覆盖 |
| --- | --- | --- | --- | --- |
| 信任边界与管理员 Token | 鉴权中间件、配置访问测试、公开路由测试 | Stable + Accepted Risk | `docs/architecture/security-and-trust-boundaries.md`、`docs/admin/security-hardening.md` | 已覆盖 |
| 工具策略与人工审批 | `pkg/tools/policy*`、`approval*`、审计测试 | Stable + Accepted Risk | `docs/architecture/tools-policy-and-approval.md` | 已覆盖 |
| 文件、网络与敏感数据 | `pkg/safefs/`、`pkg/netguard/`、配置权限测试 | Stable + Accepted Risk | `docs/admin/security-hardening.md`、`docs/reference/data-layout.md` | 已覆盖 |
| 供应链 | 发布脚本、`.github/workflows/release-e2e.yml`、SBOM/签名/provenance 门禁 | Stable + Accepted Risk | `docs/architecture/release-architecture.md`、`docs/getting-started/installation.md` | 已覆盖 |
| 单机、非多租户、第三方与 Agent 副作用 | 当前架构和运行模型 | Accepted Risk | `docs/reference/feature-status.md` | 已覆盖；发布评审必须复核 |
| 实验 Sandbox 与 aiteam 资金语义 | `pkg/aiteam/` 及实验开关 | Labs + Accepted Risk | `docs/labs/aiteam.md`、`docs/architecture/tools-policy-and-approval.md` | Labs |

## 版本与发布

| 范围 | 事实源 | 文档入口 | 覆盖 |
| --- | --- | --- | --- |
| 当前版本 | 正式 Git Tag/Release、构建注入的 `main.Version` | `README.md`、`CHANGELOG.md` | 已覆盖 |
| 版本规则 | `CHANGELOG.md`、`scripts/release.sh` | `CHANGELOG.md`、`docs/developer/releasing.md` | 已覆盖 |
| 构建与质量门禁 | `Makefile`、`.github/workflows/ci.yml` | `docs/developer/testing.md`、`docs/developer/releasing.md` | 已覆盖 |
| Draft-first 发布与四平台验证 | 发布工作流、`.github/workflows/release-e2e.yml`、Release E2E | `docs/architecture/release-architecture.md`、`docs/developer/releasing.md` | 已覆盖 |
| 兼容、弃用与迁移 | 代码兼容层、Release notes、测试 | `docs/reference/compatibility.md`、`CHANGELOG.md` | 部分覆盖；每次删除兼容路径前复核 |

## 文档入口

| 读者/用途 | 入口 | 状态 |
| --- | --- | --- |
| 仓库首页与快速开始 | `README.md` | 当前入口 |
| 文档总目录 | `docs/README.md` | 当前入口 |
| 新用户 | `docs/getting-started/` | 当前入口 |
| 日常用户 | `docs/user-guide/README.md` | 当前入口 |
| 管理员与部署者 | `docs/admin/README.md` | 当前入口 |
| 开发者与贡献者 | `docs/developer/README.md` | 当前入口 |
| 架构与参考 | `docs/architecture/README.md`、`docs/reference/` | 当前入口 |
| 状态与风险 | `docs/reference/feature-status.md` | 当前入口 |
| 文档治理 | `docs/governance/README.md` | 当前入口 |
| 实验能力 | `docs/labs/README.md` | Labs 入口 |
| 历史材料 | `docs/archive/README.md` | 历史索引 |

## 发布前复核

1. 对比 `ui/src/router/index.ts`、`internal/api/router.go` 和 `internal/agentcli/`，确认新增/删除入口已进入矩阵。
2. 对比正式候选版本、`CHANGELOG.md`、安装和升级测试，确认版本说明一致。
3. 对 Stable 行检查实现、测试和用户文档三者均存在；缺任一项时降级或阻止稳定承诺。
4. 对 Labs 和 Accepted Risk 行复核开关、默认值、风险和回滚说明。
5. 检查 Archive 材料没有被当前入口误链为操作规范。
