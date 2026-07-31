# ZyHive 全面优化路线图（2026—2027）

> 文档状态：执行基线  
> 制定日期：2026-07-31  
> 适用周期：未来 12 个月  
> 首要用户：个人与小团队的自托管 AI 团队  
> 核心原则：先恢复可信发布，再建设可靠任务闭环；停止无验证的功能扩张

---

## 0. 30 秒结论

ZyHive 当前不是“功能落后”，而是“产品能力过多、核心闭环不够现代”：

- 已经拥有单二进制、自托管、多成员、多模型、工具、记忆、渠道、定时任务和人工审批等扎实基础；
- 但正式版停在 `26.5.16v1`，截至本文约 11 周未发布；远端 `main` 最新 CI 仍失败；
- 核心 Runner、工具注册、API 路由和前端导航持续膨胀，缺少统一的任务运行模型；
- 没有持久化工作流检查点、完整 Trace、系统评测集、MCP/A2A 标准互操作；
- 高权限命令执行默认仍是宿主机进程，实验性 sandbox 没有达到真正容器隔离；
- 前端有 27 个 View，但只有 1 个前端测试文件，产品复杂度已超过现有质量保障能力。

未来 12 个月不应继续追逐“更多工具、更多页面、更多经济模块”，而应把产品收敛为：

> **一个能在个人电脑或小型服务器上长期运行、可暂停恢复、可审批、可追踪、可评测的 AI 小团队。**

最重要的四项升级：

1. 把“对话循环”升级为“持久化 Run / Step / Event 任务运行时”；
2. 把“80+ 内置工具”升级为“内置核心工具 + MCP 工具网关”；
3. 把“日志和用量”升级为“OpenTelemetry Trace + 数据集评测 + 质量回归”；
4. 把“27 个管理页面”收敛为“任务、团队、自动化、知识、设置”五个主入口，并逐步用 AG-UI 统一前端事件。

---

## 1. 已确认的现状

### 1.1 更新时间

- 制定本计划时的公开稳定版为 `26.5.16v1`，正式发布曾中断约 11 周；
- 2026-07-31 已连续发布质量、安全和发布链修复版本；
- 当前公开稳定版为 `26.8.1v1`，已恢复主分支 CI、四平台产物、校验和及严格安装更新全流程门禁；
- 原“约 3 个月没更新”问题已结束，后续重点转为持续交付可靠性和核心任务闭环。

### 1.2 当前质量记录

远端：

- 主分支 CI 已恢复并连续通过；
- `pkg/aiteam/genesis_test` 测试夹具与工资默认值失配已修复；
- CI 已覆盖 Go 全量测试、关键包竞态、前端测试与构建、嵌入 UI、脚本和发布安装更新流程；
- `main` 当前没有分支保护或 Ruleset，可绕过 CI 直接推送；
- 正式版已包含 Linux/macOS、AMD64/ARM64 四平台产物和 `SHA256SUMS`；
- SBOM、签名和可验证构建来源仍待补齐。

当前质量记录：

- `go test ./... -count=1 -timeout=10m`：通过；
- `go vet ./...`：通过；
- 前端 Vitest：2 个测试文件、5 个测试通过；
- `npm audit`：0 个已知漏洞；
- `git diff --check`：通过；
- Go 总语句覆盖率约 22.4%，39 个 Go 包中有 9 个没有测试；
- 前端只有 5 个用例，仍不能证明完整核心用户旅程；
- Release E2E 已覆盖全新安装、启动、鉴权、成员 CRUD、工作区文件跨版本持久化、严格镜像、错误校验和、配置保持和二进制备份；
- 当前回滚只覆盖下载、校验和二进制替换阶段，尚未完成“新进程启动后健康检查失败自动恢复旧版本”的闭环。

结论：

> 发布链基线已经恢复，但这不等于产品可靠性建设完成。下一阶段仍需补齐核心用户旅程 E2E、更新后健康回滚、SBOM、签名和分支保护。

### 1.3 规模和复杂度

仓库检索结果：

- Go 文件约 279 个；
- Go 测试文件 76 个；
- Vue / TypeScript 前端源文件 45 个；
- 前端 View 27 个；
- 前端测试文件只有 1 个；
- `docs/` 有 12 份设计文档；
- `proposals/` 有 49 份提案或问题文档；
- README 声明 80+ 工具、10+ Provider 和大量独立子系统。

复杂度集中位置：

- `pkg/runner/runner.go` 接近 1000 行，承担历史修复、上下文构建、预算、压缩、LLM 流、工具并行、续写、审计和自动命名；
- `pkg/tools/registry.go` 超过 1300 行，同时承担工具注册、权限、文件边界、命令执行、项目、子成员和自我管理；
- `internal/api/router.go` 超过 700 行，集中挂载几乎全部产品能力；
- `cmd/aipanel/main.go` 和 `pkg/agent/pool.go` 都超过 1200 行，多套入口各自装配 Runner、Store、Provider 和 Tool；
- 前端路由直接暴露 20 多个管理入口；
- Session 使用 JSONL + `sessions.json`，适合可读存档，但不适合作为可靠任务检查点和复杂查询主存储。

### 1.4 已有优势

这些能力应保留并强化：

- Go + Vue 3 + 单二进制嵌入 UI，安装和运维成本低；
- 单机、本地优先、自托管定位清晰；
- AI 成员拥有独立身份、灵魂、工作区、记忆、技能和渠道；
- Telegram、飞书、Web 多入口已经形成真实使用场景；
- Markdown 工作区和渐进式上下文适合人机共同维护；
- 多 Provider、流式对话、工具调用、预算、审批和审计基础已存在；
- 团队关系、联系人、群档案和共享项目构成了区别于普通聊天产品的资产；
- 当前本地质量修复证明项目仍可继续演进，不需要推倒重写。

---

## 2. 2026 年行业基线

### 2.1 可靠执行已经代替“能调用工具”

LangGraph、CrewAI Flows、Mastra 和 Temporal 当前共同强调：

- 任务状态持久化；
- 每一步检查点；
- 暂停、人工审批和恢复；
- 失败后从断点重试，而不是整段对话重跑；
- 长任务等待期间不占用执行资源；
- 可查看和修改运行状态；
- 运行历史可以回放、分叉和审计。

ZyHive 当前有 Session、Cron、审批和 Subagent，但它们没有统一成一个可持久恢复的 Run 模型。

### 2.2 MCP 已经成为工具接入标准

截至 2026-07-31：

- MCP 已发布 `2026-07-28` 规范；
- 核心协议转为无状态请求/响应；
- Tasks、Apps 等能力进入正式扩展体系；
- 官方 Tier 1 SDK 包含 Go；
- OAuth/OIDC 授权边界进一步明确。

ZyHive 继续手工增加大量第三方工具，会导致维护成本持续上升。正确方向是：

- 只保留本地文件、命令、浏览器、记忆、任务等核心原生工具；
- 第三方服务优先通过 MCP 接入；
- ZyHive 同时可把“团队、任务、记忆、审批”发布为 MCP Server 能力。

### 2.3 A2A 已成为跨系统 Agent 协作协议

A2A 1.0 已成为独立 Agent 之间发现、委派和结果交换的标准。它与 MCP 的关系是：

- MCP：Agent 到工具；
- A2A：独立 Agent 到独立 Agent。

ZyHive 的 `agent_spawn` 是内部成员协作，不应直接替换为 A2A；但未来应增加 A2A 网关，让外部 Agent 能发现和委派 ZyHive 成员。

### 2.4 AG-UI 正在统一 Agent 到前端的实时交互

AG-UI 面向 Agent 与用户界面之间的事件、状态快照、增量更新和中断恢复。ZyHive 当前有自定义 SSE 事件，但聊天、工具、审批、子成员和升级各自扩展，长期会增加前后端契约成本。

正确顺序是：

- 先稳定内部 Run / Step / Event 模型；
- 再增加 AG-UI 适配层；
- 保留现有 SSE 一段兼容期；
- 不让外部协议反向决定内部领域模型。

### 2.5 Trace 和 Eval 已成为生产门槛

OpenAI Agents SDK、Microsoft Agent Framework、LangGraph/LangSmith、Mastra、Dify、Langfuse 都把以下能力放在核心位置：

- 每次 Run 的完整 Trace；
- LLM、工具、审批、交接、检索的父子 Span；
- 延迟、Token、成本和错误原因；
- 基于真实任务的数据集回归；
- 代码规则、人工标注和 LLM-as-a-Judge 组合评测；
- Prompt 和 Agent 版本对比。

ZyHive 当前日志、用量和工具审计彼此分散，尚不能回答：

- 这次任务为什么失败；
- 哪一步最慢或最贵；
- 换模型、Prompt 或 Skill 后成功率是否提升；
- 新版本是否让已有任务退化。

### 2.6 安全执行默认走隔离环境

OpenHands 等高权限 Agent 产品已经把 Docker Sandbox 作为推荐默认值，宿主机进程模式只作为明确标记的不安全快速模式。

ZyHive 当前 `exec` 默认直接在宿主机执行：

- sandbox 依赖实验开关；
- 即使开启，也主要是进程组、临时 HOME、超时和输出限制；
- 还不是容器、远程或真正权限隔离的执行环境。

对于能执行命令、写文件、控制浏览器的 Agent，这是必须优先修复的基础问题。

---

## 3. 新产品定位

### 3.1 一句话定位

> ZyHive 是面向个人与小团队的本地优先 AI 团队运行台：把多个 AI 成员、消息渠道、知识和自动化任务放在一台可自托管设备上，所有关键动作可审批、可追踪、可恢复。

### 3.2 不再强调的内容

- 不再用“工具数量”证明产品价值；
- 不再把大量独立页面当作成熟度；
- 不再以 AI 钱包、工资、评分和自治经济体作为主线；
- 不再同时追求个人产品、企业平台、开发框架和多租户 SaaS；
- 不再承诺没有真实测试支撑的“全自动”“永不间断”“企业级”结果。

### 3.3 目标用户的三类核心任务

1. **个人工作助理团队**
   - 研究、整理、写作、编码、日程和消息跟进；
   - 不同成员分工，但用户只需要提交一个任务。

2. **小团队内部自动化**
   - 飞书或 Telegram 接收事件；
   - 定时执行汇总、监控、内容生产和跟进；
   - 高风险操作等待负责人批准。

3. **长期运行的本地知识团队**
   - 工作区、联系人、项目和记忆长期保留；
   - 任务可以跨重启继续；
   - 用户能知道做了什么、花了多少、结果是否可靠。

---

## 4. 产品架构目标

### 4.1 从“聊天中心”升级为“任务中心”

当前主对象是 Session。目标主对象应变为：

```text
Mission（用户目标）
  └─ Run（一次执行）
      ├─ Step（模型、工具、审批、交接、等待）
      ├─ Event（追加事件）
      ├─ Artifact（文件、报告、结构化结果）
      ├─ Checkpoint（可恢复状态）
      ├─ Interrupt（人工输入或外部事件）
      └─ Trace（耗时、成本、错误和质量）
```

Session 继续负责聊天历史，但不再承担所有任务运行状态。

### 4.2 目标分层

```text
交互层
  Web UI / 飞书 / Telegram / CLI / Public Chat

应用层
  Mission / Team / Automation / Knowledge / Settings

运行层
  Run Engine / Step Scheduler / Checkpoint / Retry / Approval / Handoff

Agent 层
  Identity / Prompt / Memory / Model / Policy / Skills

能力层
  Native Tools / MCP Client / Browser / Sandbox / Channels

数据层
  SQLite Runtime State + Markdown Workspace + JSONL Export

观测层
  OpenTelemetry Trace / Metrics / Eval Dataset / Audit

互操作层
  MCP Server / A2A Gateway / AG-UI Adapter（后期）
```

### 4.3 数据策略

保留 Markdown 和 JSONL 的可读优势，但调整职责：

- Markdown：身份、灵魂、长期记忆、项目说明、可交付文档；
- JSONL：导出、审计、兼容和冷存档；
- SQLite：Run、Step、Event、审批、任务状态、索引、迁移版本；
- 文件产物：统一登记为 Artifact，记录来源 Run 和校验摘要。

第一阶段仍坚持单机，不引入 PostgreSQL、Redis、Kafka 或 Kubernetes。

迁移规则：

- 同一绝对数据路径只能存在一个 Repository 和单写入口；
- JSON 快照必须临时文件写入、同步落盘后原子替换；
- SQLite 先影子写入并与旧数据校验，再切换读取；
- 旧 JSONL 至少保留一个稳定版本的导出和回滚能力；
- 连续多次 Compaction 必须保留最早摘要，不能重复消息或放大计数。

---

## 5. 能力取舍

### 5.1 保留并强化

- 单二进制和一键安装；
- AI 成员身份、灵魂和工作区；
- 本地记忆、联系人和共享项目；
- 飞书、Telegram、Web；
- 多模型 Provider；
- Cron 和主动通知；
- 工具审批、预算和审计；
- 子成员委派；
- 备份、恢复和在线升级。

### 5.2 必须重构

- Runner：拆成运行状态机、模型步骤、工具步骤和上下文构建器；
- Session：抽象存储接口，增加 Run/Checkpoint 存储；
- Tool Registry：按能力包拆分，统一策略、审批、审计和来源；
- API Router：按模块注册路由，不再由单函数装载全部系统；
- Provider：建立模型能力矩阵，不再只按供应商硬编码；
- 前端导航：从管理对象列表改为用户任务流；
- 日志、用量、工具审计：统一到 Trace 模型。

### 5.3 移入 Labs

- `pkg/aiteam/` 钱包、工资、评分、汇率、收益；
- SkillOpt 自动进化；
- ACP 编程代理；
- 高级 Goals 甘特图；
- 自动修改 SOUL、自动安装 Skill 等自修改能力。

Labs 必须满足：

- 默认不加载路由、不注册工具、不显示页面；
- 与稳定核心分开测试和发布状态；
- 明确实验数据可丢失或迁移承诺；
- 不能阻断稳定核心 CI。

### 5.4 暂停或停止

未来 90 天暂停：

- 新增消息渠道；
- 会议系统；
- Skill Marketplace；
- 深色主题；
- 多租户、RBAC、SSO；
- 自治经济模块扩展；
- 新的独立管理页面；
- 继续增加原生第三方工具。

明确不做：

- 重写为 Rust；
- 重写 Vue 前端；
- 自研模型和向量数据库；
- 为追赶 Dify 而建设复杂低代码画布；
- 为追赶 Temporal 而立即引入重型分布式基础设施。

---

## 6. 12 个月执行路线

## 阶段 0：30 天内关闭发布阻断并恢复可信主分支

目标：让公开仓库重新成为可交付状态。

必须完成：

1. 提交当前本地质量修复；
2. 远端 Go、race、前端、脚本和发布构建全部通过；
3. 主分支保护要求必要检查通过后才能合并；
4. 连续三个 `main` 提交 CI 全绿；
5. 更新 GitHub Actions 到 Node.js 24 兼容版本；
6. 前端开发与 CI 从已 EOL 的 Node.js 20 迁移到仍受支持的 Node.js 22/24，并验证依赖兼容；
7. 完成干净环境安装、升级失败回滚、备份恢复烟雾测试；
8. 发布一个只做质量恢复的稳定版本；
9. 官网、安装端点、README、CHANGELOG、Release 完全一致。

稳定版 Stop-Ship 缺陷：

1. 修复无令牌访问受保护路由仍然放行；
2. 统一远程服务器地址，确保 REST、对话流和审批流请求同一实例；
3. 修复设置页保存端口可能覆盖 `gateway.bind` 和 `publicUrl`；
4. 对齐 Ollama、`embedModel`、默认模型等 Provider 前后端契约；
5. 成员创建改为“成员、模型、渠道、能力”分项结果，禁止部分失败仍显示完整成功；
6. 移除全局渠道与成员渠道双轨，稳定版只保留成员级 Telegram、飞书和 Web；
7. 未实现渠道不得返回测试成功，iMessage、WhatsApp 等入口从稳定界面移除；
8. 审批 SSE 重连后必须重新拉取待办，并显示连接状态、超时和过期保护；
9. 更新只有在新进程通过健康检查后才显示完成，失败必须恢复旧版本；
10. 为登录、远程连接、模型、成员、首个对话、渠道、审批和更新建立 E2E 门禁。

架构 Stop-Ship 缺陷：

1. Project、Skill、Agent、Session ID 和所有文件访问统一经过 `safefs`/CapabilityFS，禁止字符串前缀判断和工作区外绝对路径；
2. 后台进程归属改为 `{agentID, sessionID, UUID}`，支持进程组取消、并发上限、输出上限和自动回收；
3. Session Worker 使用 `{agentID, sessionID}` 键，并为每次请求生成独立 `turnID/runID`；
4. 回放和实时事件使用单调序号，禁止断线重连时历史事件与实时事件乱序或串到下一次请求；
5. Public Chat、管理端、Telegram、飞书、Cron 和 Subagent 全部进入同一个 `TurnService/RunnerFactory`，统一 Provider、Tools、Usage、Budget 和 Audit；
6. 配置改为版本化不可变快照，磁盘保存成功后才能发布新配置，并明确 `appliedLive/restartRequired`；
7. Cron 的 `at`、时区和重叠策略必须真实生效；注册失败不能保存成“成功任务”；
8. Tool 增加 `ReadOnly / ParallelSafe / SideEffect / Approval` 元数据，只有明确并行安全的调用才能并发；
9. 修复重复 Compaction 丢失旧摘要、重复消息和计数膨胀；
10. Labs 关闭时不导入具体实现、不挂路由、不注册工具、不启动 goroutine、不创建数据目录。

安全 Stop-Ship 缺陷：

1. 空令牌不得 fail-open；缺少或无效令牌时管理 API、下载、审批和媒体全部拒绝访问；
2. `/api/download` 只能读取登记过的 Artifact，禁止用户提交任意绝对路径；
3. Public Chat 使用独立最小权限 Profile，默认没有 Shell、文件写入、技能安装、项目创建等高风险工具；
4. 飞书回调必须验证签名、时间戳和重放，验证失败不能进入 LLM 或工具链；
5. 全局 Policy、成员 Policy、审批 Broker 和审计必须在所有入口执行同一套决策；
6. 公共入口必须限制并发、请求频率、会话数量、Token/费用和断线后的后台运行时间；
7. `web_fetch`、模型探测和浏览器导航阻断回环、私网、云元数据、危险重定向和 DNS 重绑定；
8. systemd 服务改用专用低权限用户，并启用 `NoNewPrivileges`、文件系统保护和能力裁剪；
9. Token、API Key、Bot Token、对话、记忆和配置使用最小文件权限，API 响应默认脱敏；
10. URL 查询参数不再携带长期令牌；下载/审批凭证必须一次性、短时有效且不进入普通访问日志；
11. 备份恢复验证归档路径、校验摘要和兼容版本，禁止以 root 直接解压未验证 tar；
12. 高权限执行在真正容器隔离可用前不进入 Stable 默认能力。

验收：

- 新用户拿到的是已修复版本，不是 5 月旧版本；
- CI 徽章为绿色；
- 安装脚本产物有 SHA-256；
- 升级失败能自动回到旧二进制；
- 不把新功能混入质量恢复版本。

## 阶段 1：第 31—75 天完成产品收敛

目标：让第一次使用从“配置很多对象”变成“完成第一个任务”。

产品动作：

1. 首次启动向导只保留：
   - 连接或确认当前服务器；
   - 连接一个模型；
   - 选择一个团队模板；
   - 运行示例任务；
   - 查看结果和 Trace。
2. 提供三个默认模板：
   - 个人研究团队；
   - 内容与运营团队；
   - 开发与测试团队。
3. 主导航收敛为：
   - 任务；
   - 团队；
   - 自动化；
   - 知识；
   - 设置。
4. Logs、Usage、Tool Audit、Models、Tools 等进入设置或任务详情；
5. AITeam、SkillOpt、ACP 统一进入 Labs；
6. 建立 Feature Manifest，README 和 UI 都从同一状态定义区分：
   - stable；
   - beta；
   - labs；
   - disabled。
7. Labs 关闭时不注册对应前端路由和后端路由，不只隐藏侧栏；
8. 不生效的成员 `toolIds`、`skillIds` 选择器先移除，待运行时真正消费后再恢复。

工程动作：

- 给 `runner` 增加完整行为测试；
- 给首次配置、创建团队、首个任务增加 Playwright E2E；
- 前端测试从 1 个文件扩展到关键组件和五条核心旅程；
- 修正文档与代码冲突，例如 README/CHANGELOG 中 SDK 依赖与 `go.mod` 不一致。

关键指标：

- 从安装到首次成功任务中位数不超过 10 分钟，P90 不超过 15 分钟；
- 用户不需要理解 80+ 工具和 20+ 页面；
- 默认模板可以不编辑 Prompt 直接运行；
- 首次失败必须给出可操作的修复建议。

## 阶段 2：第 3—6 个月建设可靠 Run Engine

目标：让任务可暂停、可恢复、可追踪。

核心交付：

1. 新建 `pkg/run`：
   - Mission、Run、Step、Event、Artifact、Checkpoint、Interrupt；
   - 状态：queued / running / waiting / succeeded / failed / cancelled；
   - Step 类型：model / tool / approval / handoff / wait / output。
2. SQLite 运行状态：
   - 单写事务；
   - schema migration；
   - 崩溃恢复；
   - 幂等 step key；
   - JSONL 导出。
3. Runner 拆分：
   - ContextBuilder；
   - ModelStepExecutor；
   - ToolStepExecutor；
   - ApprovalStepExecutor；
   - RunCoordinator。
4. 建立唯一应用服务：
   - `TurnService/RunnerFactory` 负责所有入口的执行装配；
   - Transport 只解析身份、输入和输出编码；
   - 渠道统一转换为 `InboundMessage`，通过 `DeliveryPort` 输出；
   - 除统一工厂外不允许直接调用 `runner.New`。
5. 人工审批变成可持久等待：
   - 重启后仍能继续；
   - 有超时、拒绝、备注和审计；
   - 支持从 Web、飞书或 CLI 审批。
6. 错误策略：
   - 瞬时错误按步骤重试；
   - 非幂等工具不自动重放；
   - 支持取消和补偿动作；
   - 每次重试有原因和次数上限。
7. Provider 统一为 `ModelGateway`：
   - Endpoint、凭据解析、模型能力矩阵只定义一次；
   - Vision、Tools、Reasoning、JSON、Embedding、Context 明确声明；
   - 每个请求只有一个重试预算，避免内外两层重试放大成本；
   - Throttle 和 Health Cache 通过运行时依赖注入，不用包级全局状态。

验收：

- 在模型调用、工具执行和人工审批三个位置强制杀进程，重启后都能恢复；
- 已完成的非幂等步骤不会重复执行；
- 用户能看到当前卡在哪一步；
- 每个 Run 都能导出完整事件和产物。

## 阶段 3：第 6—9 个月接入现代生态

目标：停止手工维护第三方工具孤岛。

MCP：

- 使用官方 Go SDK，支持 MCP `2026-07-28`；
- 首期支持本地 stdio 和远程 HTTP；
- Server 配置必须有来源、权限、超时和网络范围；
- MCP 工具进入现有 policy / approval / audit；
- 支持工具列表缓存和能力健康检查；
- ZyHive 发布 MCP Server：
  - 列出成员；
  - 创建任务；
  - 查询 Run；
  - 获取 Artifact；
  - 提交审批。

AG-UI：

- 把 Run、Step、工具、审批和 Artifact 映射为统一前端事件；
- 支持断线后从状态快照恢复，不依赖前端猜测当前阶段；
- 先作为兼容适配层，不立即删除现有 SSE；
- 新前端功能不再创建互不兼容的私有流事件。

可观测：

- 采用 OpenTelemetry 数据模型；
- 每个 Run 为 Trace；
- Model、Tool、Approval、Handoff、Memory Retrieval 为 Span；
- 默认本地查看，可选 OTLP 导出到 Langfuse、Jaeger 等；
- Secret、完整文件和敏感消息默认脱敏。

评测：

- 建立 30—50 条真实任务黄金集；
- 覆盖研究、文件处理、工具审批、渠道消息、定时任务和多成员委派；
- 每次核心 Prompt、Runner、模型适配修改执行离线回归；
- 指标包括任务成功、结构正确、工具选择、成本、时延和人工接管次数。

安全：

- Docker Sandbox 成为命令执行推荐默认；
- Process 模式明确显示“不隔离”；
- 文件、网络、进程和 Secret 分别授权；
- 默认拒绝访问工作区外路径；
- 增加出站网络域名策略；
- Skill 和 MCP Server 不得自动获得宿主机全部环境变量。

## 阶段 4：第 9—12 个月形成 1.0

目标：形成稳定、可扩展但不臃肿的自托管产品。

交付方向：

- Mission 模板和轻量流程编排；
- Run 时间线、分叉重跑和失败步骤重试；
- 团队模板导入导出；
- Prompt、Skill、模型和工具配置版本化；
- 本地模型 / OpenAI Compatible 能力检测完善；
- A2A 1.0 实验网关；
- AG-UI 前端互操作层；
- 带签名、来源、权限声明和兼容版本的插件包；
- 跨设备备份、恢复和迁移检查；
- 插件开发文档和兼容性测试套件；
- 稳定的公开 API 和版本弃用策略。

1.0 准入条件：

- 连续 90 天没有数据损坏类问题；
- 核心 E2E 在 Linux amd64/arm64、macOS amd64/arm64 全部通过；
- 升级和回滚经过至少三个正式版本验证；
- 关键任务集成功率达到既定门槛；
- 所有 Stable 功能都有测试、文档、迁移和错误处理；
- Labs 不会阻断 Stable 的启动、测试和发布；
- 发布产物包含四个平台二进制、SHA-256、SBOM、签名和可验证构建来源；
- tag 必须精确指向通过受保护 CI 的提交。

---

## 7. 工程质量门槛

### 7.1 测试结构

Go：

- Runner、Run Engine、Session/SQLite、Policy、Sandbox 为最高优先级；
- Public Chat、Session Worker/Broadcaster、Cron `at/TZ` 和后台进程进入 P0 测试；
- 不以全仓覆盖率数字替代关键路径覆盖；
- 核心包要求分支和失败路径测试；
- 文件、网络、并发和迁移必须有故障注入测试。

前端：

- Composable 和 Store 单元测试；
- Mission、Run、Approval、Onboarding 组件测试；
- Playwright 覆盖五条核心旅程；
- 360、390、768 像素宽度下核心旅程无页面级横向滚动；
- 关键触控目标不小于 44×44 像素；
- 流式回复、审批到达和任务完成使用 `aria-live`；
- 核心旅程通过 WCAG 2.2 AA 自动检查且无严重问题；
- 每个线上缺陷必须补回归测试。

契约：

- Provider 统一事件契约；
- MCP 客户端兼容性契约；
- API OpenAPI/JSON Schema；
- 前后端错误码契约；
- 配置和数据库迁移契约。

### 7.2 CI 必需检查

- Go test；
- Go race 全仓，稳定后再按证据缩小为明确关键包；
- Go vet；
- golangci-lint 由 advisory 逐步转为阻断；
- 前端 test / typecheck / build；
- Playwright 核心旅程；
- 嵌入 UI 一致性；
- shellcheck / bash syntax / CLI 回归；
- secret scan；
- `govulncheck` 可达漏洞、`npm audit` 和依赖完整性；
- `gosec` 建立人工确认基线，禁止新增 High/Medium；
- CodeQL、许可证扫描和依赖自动更新；
- 四平台发布构建；
- 安装、升级和恢复 smoke。

初始覆盖率门槛：

- Go 总覆盖率先提升到 35%，安全关键包达到 70%，且任何合并不得降低；
- 前端行和分支覆盖率先达到 60%；
- 第二阶段目标为 Go 总覆盖率 50%、安全关键包 85%、前端 75%；
- 路径、协议、配置迁移和归档恢复增加 Fuzz；
- 前端主入口 gzip 不超过 350 KB，单异步页面 gzip 不超过 150 KB。

### 7.3 发布节奏

- 每周：允许补丁版本；
- 每月：一个 Beta 功能版本；
- 每季度：一个稳定里程碑；
- 不再一天发布多个无验证版本；
- 未来 90 天继续兼容现有日期版本，避免阻塞质量恢复发布；
- 1.0 前让升级器同时识别日期版本和 SemVer，经过至少一个版本的双格式验证；
- 1.0 起 Public Release 使用 SemVer，日期和提交 SHA 保留为构建信息；
- nightly、beta、stable 三条通道互不混用。

---

## 8. 产品指标

### 8.1 北极星指标

> 每周成功完成并产生可用 Artifact 的 Mission 数量。

不把“消息数”“工具数”“Token 数”当成核心成功指标。

### 8.2 激活指标

- 安装成功率；
- 模型连接成功率；
- 首次 Mission 成功率；
- 安装到首次成功的中位时间；
- 模板直接运行成功率。

### 8.3 可靠性指标

- Run 成功率；
- 崩溃恢复成功率；
- 重复执行非幂等步骤次数；
- 审批恢复成功率；
- 升级和回滚成功率；
- 数据迁移校验差异数。

### 8.4 质量指标

- 黄金任务集通过率；
- 用户人工接管率；
- 错误后自恢复率；
- 每个成功 Mission 的成本；
- P50/P95 完成时间；
- 用户对 Artifact 的接受、修改和废弃比例。

---

## 9. 执行资源建议

在人员有限的前提下，未来三个月建议：

- 60%：质量、Run Engine、存储和安全；
- 25%：首次体验、任务页和导航收敛；
- 15%：MCP 预研、评测集和文档；
- 0%：新的经济模块、渠道、主题和独立页面。

每个阶段必须先有：

- 一个明确用户问题；
- 一个可量化成功标准；
- 一个可回滚方案；
- 一组自动化测试；
- 一份同步更新的产品事实。

---

## 10. 前 3 个月具体任务单

### 第 1 周：先封住安全边界

- 提交现有本地修复；
- 禁止空令牌 fail-open 和任意路径下载；
- Public Chat 切换为最小权限工具 Profile；
- 飞书回调启用签名和重放验证；
- 统一路径约束并隔离后台进程所有权；
- 统一 Policy、Approval 和 Audit；
- 增加公共入口限流、成本上限和 SSRF 防护。

截至 2026-08-01 的实施状态：

- [x] 空令牌改为 fail-closed；
- [x] 下载、媒体和 `send_file` 先收紧到成员工作区；
- [x] Public Chat 改为默认零工具权限；
- [x] 飞书回调增加签名、时间窗和单机重放验证；
- [x] 第一批修复随 `26.7.31v1` 提交、推送并正式发布；
- [x] 下载改为 Artifact ID + 一次性短时凭证，并随 `26.7.31v2` 发布；
- [x] 媒体预览和审批 SSE 移除长期 URL 令牌，并随 `26.7.31v3` 发布；
- [x] Public Chat 增加请求频率、模型频率、会话、任务并发、SSE 并发、消息大小和运行时限，并随 `26.7.31v8` 公开发布；
- [x] 建立统一出站 URL/DNS 防护，首批接入 `web_fetch` 和浏览器初始导航，并随 `26.7.31v8` 公开发布；
- [x] Worker 增加成员/渠道所有权隔离、单会话互斥和唯一终止事件，并随 `26.7.31v9` 发布；
- [x] Chromium 全部 HTTP、HTTPS、WebSocket 和动态页面请求强制经过安全代理，并随 `26.7.31v10` 发布；
- [x] Web、渠道、Cron、Heartbeat 和 Subagent 统一执行分层 Policy、Approval 与 Audit，并随 `26.7.31v11` 发布；
- [x] Bash 与 ACP 后台进程统一绑定成员/会话所有权，补齐进程树回收、并发、输出、超时、目录和环境边界，并随 `26.8.1v1` 发布；
- [ ] 完成模型地址等剩余动态出站路径收口；
- [ ] 连续验证三个受保护 `main` 提交的远端 CI。

### 第 2 周：修复运行和持久化边界

- 为 Session 双请求、断线重连和 Public Chat 增加回归测试；
- 修复 Session 串流、Public Chat 终止事件和后台执行取消；
- 修复 Cron `at/TZ`、重复 Compaction 和配置原子提交；
- Labs 关闭时停止注册路由、工具和后台任务；
- 收紧 systemd、文件权限、秘密脱敏和备份恢复路径；
- 建立主分支保护和必要检查。

### 第 3 周：打通稳定产品旅程

- 修复登录、远程服务器地址和配置递归合并；
- 对齐 Provider 前后端契约；
- 修复成员创建部分失败；
- 统一成员级渠道；
- 修复审批断线补拉；
- 停止未实现渠道的假成功；
- 实现新进程健康检查失败自动回滚；
- 移除尚未实现的产品承诺。

### 第 4 周：全量验证并发布

- 修复远端 CI 并迁移 Node.js 22/24；
- 执行全仓 race、漏洞、覆盖率和前端包体门禁；
- 执行安装、升级、回滚和恢复；
- 增加 Stop-Ship E2E；
- 生成四平台产物、SHA-256、SBOM、签名和构建证明；
- 连续三个受保护 `main` 提交 CI 全绿；
- 发布质量恢复版。

### 第 2 个月：产品减法

- 创建 Feature Manifest；
- 把 AITeam、SkillOpt、ACP 移入 Labs；
- 合并导航；
- 重写首页为 Mission Inbox；
- 删除 README 的功能堆叠式首屏；
- 完成首次设置向导和三个默认团队模板；
- 扩展前端核心组件和旅程测试。

### 第 61—75 天：任务模型原型

- 确定 Mission / Run / Step / Event Schema；
- 建立 SQLite migration；
- 让现有单轮 Chat 生成 Run 记录；
- 把工具调用和审批映射为 Step；
- 提供 Run 时间线 API。

### 第 76—90 天：闭环验证

- 实现首次配置 E2E；
- 实现崩溃恢复原型；
- 建立首批 10 条黄金任务；
- 增加本地 Trace 页面；
- 发布第一个“可靠任务”Beta。

---

## 11. 主要风险

### 风险一：继续用功能数量判断进度

控制方式：

- 每个版本最多一个主用户价值；
- 没有任务成功率改善的能力不进入 Stable；
- 新页面必须证明不能合并到现有任务流。

### 风险二：运行时重构影响现有聊天

控制方式：

- 先旁路记录 Run，不立刻替换旧 Runner；
- 双写并比较 Session 与 Run Event；
- 一个版本后再切换默认读取；
- 保留 JSONL 导出和回滚工具。

### 风险三：MCP 扩大安全边界

控制方式：

- MCP 工具必须经过现有 Policy、Approval 和 Audit；
- 默认不继承全部 Secret；
- 远程 Server 需要域名和授权确认；
- 工具结果有大小、时间和内容限制。

### 风险四：为了“可靠”引入过重基础设施

控制方式：

- 1.0 前保持单机 SQLite；
- 不引入微服务；
- 不把 Kubernetes 当作成熟标志；
- 使用标准数据模型和接口，为未来替换留出口。

---

## 12. 决策结论

ZyHive 下一阶段最有价值的差异化，不是成为另一个 Dify、n8n、LangGraph 或 OpenHands，而是把这些优秀项目已经证明的可靠机制，收敛进 ZyHive 独特的“本地 AI 团队”模型：

- 像 LangGraph / Mastra 一样可暂停恢复；
- 像 OpenAI Agents SDK 一样有完整 Trace、Guardrail 和 Handoff；
- 像 OpenHands 一样明确隔离高权限执行；
- 像 Dify 一样让第一次使用清晰；
- 通过 MCP 接入工具，通过 A2A 与外部 Agent 互通；
- 保留 ZyHive 独有的成员身份、关系、记忆、渠道和本地工作区。

最终目标不是“看起来功能最多”，而是：

> **用户交给一支 AI 小团队一个真实任务，系统能安全地完成、留下可用结果，并且任何一步都能看见、暂停、审批、恢复和复盘。**

---

## 13. 公开参考

- OpenAI Agents SDK：https://openai.github.io/openai-agents-python/
- OpenAI Agents 指南：https://developers.openai.com/api/docs/guides/agents
- LangGraph：https://docs.langchain.com/oss/python/langgraph/overview
- Microsoft Agent Framework：https://learn.microsoft.com/en-us/agent-framework/overview/
- CrewAI Flows：https://docs.crewai.com/en/concepts/production-architecture
- Mastra Suspend / Resume：https://mastra.ai/docs/workflows/suspend-and-resume
- Temporal Human-in-the-loop：https://docs.temporal.io/ai-cookbook/human-in-the-loop-python
- OpenHands Sandbox：https://docs.openhands.dev/openhands/usage/sandboxes/overview
- Dify：https://dify.ai/
- Langfuse：https://langfuse.com/docs
- MCP 2026-07-28：https://blog.modelcontextprotocol.io/posts/2026-07-28/
- A2A 1.0：https://a2a-protocol.org/latest/
