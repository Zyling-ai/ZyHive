# ZyHive 用户手册

> 适用基线：公开版本 `26.8.2v1` 与当前仓库代码（2026-08-02）  
> 产品定位：单机、自托管 Beta。本文中的 **Stable** 表示稳定核心能力，不代表企业级 SLA；**Labs** 表示默认关闭、接口或数据格式可能变化的实验能力。

![ZyHive 稳定能力与实验能力总览](../assets/diagrams/system-overview.svg)

## 从这里开始

1. 使用安装输出的地址打开管理面板。
2. 在登录页输入管理员 Token。前端把服务器地址和 Token 分别保存在浏览器本地存储中；管理 API 使用 `Authorization: Bearer <token>`。
3. 到「模型配置」添加 Provider、测试连接并添加模型。
4. 到「成员」确认或编辑默认的“主助手”，再从首页开始第一段对话。
5. 需要外部消息时，在成员详情中绑定 Telegram、飞书或 Web 渠道。

默认配置文件由启动参数 `--config` 决定。一键安装的常见位置是 `/etc/zyhive/zyhive.json`（root）或 `~/.config/zyhive/zyhive.json`（普通 macOS 用户）。`agents.dir` 决定成员数据根目录；相对路径均相对于服务进程工作目录，不应按浏览器所在电脑推断。

## 文档导航

- [成员与对话](agents-and-chat.md)：成员身份、会话、流式输出、委派与常见错误。
- [工作区、记忆与通讯录](workspace-memory-network.md)：文件、四层记忆、联系人、群档案和关系图。
- [模型、工具与审批](models-tools-approvals.md)：Provider、模型能力、工具策略和人工审批。
- [技能与 SkillOpt](skills-and-skillopt.md)：Stable 静态技能与 Labs 自进化闭环。
- [规划、定时、项目与后台任务](goals-cron-projects-tasks.md)：目标、Cron、共享文件和子成员任务。
- [消息渠道](channels.md)：成员级 Telegram、飞书和 Web；全局渠道页的兼容边界。
- [用量、工具审计与系统日志](usage-audit-logs.md)：三类记录的含义、位置和保留边界。
- [设置、更新与公开聊天](settings-update-public-chat.md)：待重启设置、在线升级、公开入口与限额。

## Stable 与 Labs

### Stable 核心

成员、管理端聊天与会话、工作区、分层记忆、通讯录/群档案、关系图、Provider/模型、工具权限与审批、静态技能、Cron、共享项目、后台任务、Telegram/飞书/Web 渠道、用量、工具审计、日志、设置和在线升级属于当前稳定核心。它们仍受单机 Beta 边界约束：没有多租户、RBAC、SSO、多实例一致性或高可用承诺。

### Labs

- SkillOpt 自动进化。
- ACP 外部编程代理。
- Goals 高级甘特规划与模型助手。
- AITeam 钱包、汇率、预算护栏、评分、工资和收益。
- 自动修改 `SOUL.md`、自动安装技能等自修改能力。

当前代码尚未完全做到 Labs 的物理隔离：部分前后端路由会始终注册，再由管理器是否存在或环境开关决定是否可用。因此“侧栏没显示”不等于模块没有编译或没有路由；Labs 关闭时出现 `404`、空页面或“加载失败”通常是能力未启用，不应按 Stable 故障处理。实验数据可能迁移、重置或停止兼容。

## API、状态码与安全边界

管理 API 位于 `/api/*`，除 `/api/version`、`/api/update/status`、一次性下载/媒体票据和健康探针等少数入口外均要求管理员 Token。公开聊天位于 `/pub/chat/...`，不使用管理员 Token。

- `401 unauthorized`：Token 缺失、错误或已经修改；前端会清除本地 Token 并跳转登录。
- `503 authentication is not configured`：服务端没有配置认证 Token，管理 API 会失败关闭，不会匿名放行。
- `403`：Origin、策略、权限或资源边界拒绝。
- `404`：资源不存在，也可能是关闭的 Labs 处理器主动隐藏能力。
- `409`：常表示任务、运行或审批状态冲突。
- `429`：Provider 或公开入口限流。
- `5xx`：服务端、磁盘、Provider 或外部渠道失败；结合响应中的错误和 `X-Trace-Id` 查日志。

所有资源 ID 和文件访问均应经过服务端路径约束。不要直接手改运行中的 JSON/JSONL；配置和索引由服务端负责原子保存、锁和迁移。备份归档当前是明文，可能包含 Token、API Key、渠道凭据、对话和记忆，应按秘密文件保管。

## 图片占位清单

本文引用的是后续文档计划的确切文件名，统一放在 `docs/assets/`：

- `diagrams/system-overview.svg`
- `screenshots/chat-home.png`
- `diagrams/chat-sse-flow.svg`
- `screenshots/user-guide-agent-detail.png`
- `diagrams/memory-system.svg`
- `screenshots/team-network.png`
- `diagrams/tool-governance.svg`
- `screenshots/models.png`
- `diagrams/runner-loop.svg`
- `diagrams/cron-claims.svg`
- `screenshots/cron.png`
![项目工作区界面](../assets/screenshots/projects.png)

![后台任务实时对话界面](../assets/screenshots/tasks.png)
- `diagrams/channel-flow.svg`
- `diagrams/data-layout.svg`
- `screenshots/usage.png`
- `screenshots/tool-audit.png`
- `diagrams/update-rollback.svg`
- `screenshots/settings.png`

这些文件当前仅作为相对引用占位；手册不依赖不存在的外部图片地址。
