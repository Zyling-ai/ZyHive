# ZyHive 开发计划 · INDEX

> 文档定位：基于当前仓库（截至 `26.4.23v7`，main 分支）真实代码与文档结构，沉淀一份**可落地、可拆条目执行**的中长期开发计划，作为后续 proposals 子文件的总目录。
>
> 命名约定：每个具体提案单独成文，文件名为 `P{优先级}-{编号}-{kebab-name}.md`，本 INDEX 只做条目摘要 + 总览。

---

## 1. 背景与现状盘点

ZyHive（引巢）是一个 AI 团队操作系统（Go 后端 + Vue 3 前端 + 单二进制嵌入 UI），到 `26.4.23v7` 已经具备：

- 完整成员生命周期（`pkg/agent/`）：身份/灵魂/工作区/能力/关系
- 对话主循环 + 系统提示词 10 层渐进披露（`pkg/runner/`、`docs/system-prompt-and-flow.md`）
- 10+ Provider 抽象 + 重试 + 健康检查（`pkg/llm/{retry,health,errors}.go`）
- 80+ 工具、工具策略、工具体检（`pkg/tools/`）
- 四层分层记忆 + 蒸馏 + 语义检索（`pkg/memory/`）
- Per-agent 私有通讯录 + 渐进披露 + 自动建档（`pkg/network/`）
- 渠道：Telegram、飞书（WS + 流式卡片）、Web 公开聊天（`pkg/channel/`、`internal/api/`）
- 目标规划（甘特图）、子成员派遣、Cron 隔离会话、ACP 编程代理、共享项目
- Token 用量统计 + 多 Provider 计费 + UsageView 明细
- SSE 自动重连、错误隔离、Compaction 同步事件等"生产稳定性地基"
- CLI 子命令 + 一键安装脚本 + CF 加速代理 + 在线升级

仓库目录关键路径速查：

```
cmd/aipanel/                  服务/CLI 入口
internal/api/                 REST 路由（chat/agents/sessions/relations/network/...）
pkg/{agent,runner,session,llm,tools,memory,network,channel,convlog,
     chatlog,cron,goal,subagent,browser,skill,project,usage,config,compaction}/
ui/src/{views,components}/    Vue 3 前端
projects/{zyhive,Zyling,zyling-mp}/  内部子项目
docs/                         设计文档
scripts/{deploy-hive.sh,install.sh,release.sh,test/}
```

CHANGELOG 已声明的 **P1 规划中** 项（散落在 README）：
> Chat Profile（群档案）· 跨 agent 联系人聚合视图 · 头像 API 拉取 · AI 自动合并联系人 · Web 访客升级为命名 contact · `self_schedule` 自主闹钟工具 · 自主唤醒 budget 预算刹车

本计划在此基础上做**结构化扩展**，覆盖：稳定性、产品价值、AI 自主性、生态、工程化。

---

## 2. 总体目标（North-Star）

1. **从"工具"到"伙伴"**：让每个 agent 能更主动、更克制、更有时间感（schedule + budget + 自主总结）。
2. **从"单机"到"团队"**：让 N 个 agent 真正在一个项目中协同（会议、共享上下文、跨 agent 任务流转）。
3. **从"能跑"到"能放心放生产"**：可观测性、限流降级、配额预算、灰度回滚、备份与迁移。
4. **从"功能堆"到"开发者平台"**：稳定的 SDK / Plugin API / Skill Marketplace。

---

## 3. 主题与提案矩阵

下表为本计划全部条目总览。具体细则各自在 `proposals/zyhive-improvements/P{x}-{NN}-*.md`。

> 优先级：P0=立刻、P1=下一程、P2=条件成熟后。
>
> 改造规模：S=单文件级、M=单包级、L=跨包/跨前后端、XL=新增子系统。

### 3.1 主题 A · AI 自主性 & 用量自治

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| A-01 | `self_schedule` 自主闹钟工具（agent 自己 cron_add） | P1 | M | `pkg/cron`、`pkg/tools` |
| A-02 | 自主唤醒 budget 预算刹车（每日 token / 调用次数硬上限 + 软警告） | P1 | M | `pkg/usage`、`pkg/runner` |
| A-03 | AI 自动合并联系人（基于多字段相似度 + LLM 二次确认） | P1 | M | `pkg/network` |
| A-04 | 自主记忆体检：每周自检 `memory/INDEX.md` 索引完整性，过期/失效 link 清理建议 | P2 | M | `pkg/memory`、`Consolidator` |
| A-05 | "请教其他成员"主动 escalation（runner 在置信度低时自动 `agent_spawn`） | P2 | L | `pkg/runner`、`pkg/subagent` |

### 3.2 主题 B · 团队协同与会议

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| B-01 | 会议系统 MVP（按 `docs/roadmap-v0.10.md` Feature 2 落地） | P1 | XL | `pkg/cron`、`pkg/agent.Pool`、新增 `pkg/meeting/` |
| B-02 | Chat Profile（群档案）：飞书群/TG 群级别上下文聚合 | P1 | M | `pkg/network`、`pkg/channel` |
| B-03 | 跨 agent 联系人聚合视图 + 全局统一搜索 | P1 | M | `internal/api/network.go`、`TeamView.vue` |
| B-04 | 跨 agent 任务流转 baton：A 把"某 sessionID + 上下文"原子转交 B | P2 | L | `pkg/session`、`pkg/agent.Pool`、新协议事件 |
| B-05 | Web 访客升级为命名 contact：`web-visitor-xxx` 一键 promote | P1 | S | `pkg/network` |

### 3.3 主题 C · 生产稳定性

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| C-01 | 可观测性 P0：结构化日志（slog）+ 请求 trace id 贯穿 SSE/工具调用 | P0 | M | `internal/api`、`pkg/runner` |
| C-02 | Provider 自适应限流（替换现 `FixedThrottle`，按 429/Retry-After 学习） | P1 | M | `pkg/llm`、已预留 `Throttle` interface |
| C-03 | 配额与多租户雏形：per-token / per-agent 日预算（HTTP 429 + UI 提示） | P1 | M | `pkg/usage`、`internal/api` |
| C-04 | 备份/恢复 CLI：`zyhive backup / restore`（agents、sessions、network、cron、goals） | P1 | M | `cmd/aipanel/cli.go` |
| C-05 | 升级失败回滚（保留上一版二进制 + 一键 rollback） | P1 | S | `internal/api/update.go`、CLI |
| C-06 | 在已有 `/healthz` + `/api/status` 基础上补 `/readyz`：Provider 探活 + cron worker 心跳 + session pool 健康 | P0 | S | 已有 `internal/api/healthz.go`，扩展即可 |
| C-07 | 配置热重载（信号 SIGHUP 重新加载 `zyhive.json`，Provider/Token 改后无需重启） | P2 | M | `pkg/config` |

### 3.4 主题 D · 数据层与迁移

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| D-01 | Session 存储抽象层：抽 interface，未来可切 SQLite / Postgres，当前默认 JSONL | P1 | M | `pkg/session` |
| D-02 | 全局索引（SQLite）作为查询面：消息全文 / 联系人 / 目标，写时同步双写 | P2 | L | `pkg/session`、`pkg/network`、`pkg/goal` |
| D-03 | 大型 session 自动归档（满 1000 msg 滚动 + 索引懒加载） | P1 | M | `pkg/session`、`pkg/compaction` |
| D-04 | 全量数据迁移工具（agents 目录跨机迁移：路径修复、绝对路径剥离、版本兼容声明） | P2 | M | `cmd/aipanel/cli.go` |

### 3.5 主题 E · 渠道与外部集成

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| E-01 | 头像 API 拉取（飞书/TG 头像缓存到本地，UI 直链兜底） | P1 | S | `pkg/network`、`pkg/channel` |
| E-02 | 飞书群 @ 模式增强：未 @ 时静默 + 群档案上下文（依赖 B-02） | P1 | S | `pkg/channel/feishu` |
| E-03 | 微信公众号 / 微信客服接入 | P2 | L | 新建 `pkg/channel/wechat` |
| E-04 | Webhook 入站：通用 HTTP 接收器，把任意系统事件转入指定 agent | P2 | M | `internal/api`、`pkg/channel` |
| E-05 | 出站 SMTP/邮件渠道：日报、告警、纪要邮发 | P2 | M | 新建 `pkg/channel/email` |

### 3.6 主题 F · 工具生态与权限

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| F-01 | 用户审批模式（`policy=ask`）端到端：UI 弹窗 + 超时拒绝 + 审计日志 | P1 | M | `pkg/tools/policy.go`、`internal/api/chat.go`、UI |
| F-02 | 工具沙箱化：`exec` 默认在临时目录 + 资源/时间硬上限 | P1 | M | `pkg/tools` |
| F-03 | 工具调用结构化日志（`pkg/convlog`）+ UI 工具卡可点击查看完整 input/output | P1 | M | `pkg/convlog`、`AiChat.vue` |
| F-04 | MCP（Model Context Protocol）兼容客户端：把外部 MCP server 暴露为本地工具 | P2 | L | 新建 `pkg/tools/mcp/` |
| F-05 | Skill Marketplace：远程 SKILL.md 索引 + 一键安装 + 签名校验 | P2 | XL | `pkg/skill`、`SkillStudio` |

### 3.7 主题 G · 前端体验

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| G-01 | 统一全局搜索（⌘K）：成员 / 会话 / 联系人 / 目标 / 文件 | P1 | M | UI + `internal/api` |
| G-02 | 通知中心（站内信）：派遣完成、cron 输出、wish 状态、版本升级 | P1 | M | UI + 新表 `pkg/notification` |
| G-03 | 移动端二次打磨（侧边栏抽屉、消息列表虚拟滚动） | P2 | M | UI |
| G-04 | i18n（zh-CN / en-US 双语骨架，先把字符串集中） | P2 | M | UI |
| G-05 | 主题切换（保留浅色为默认，加可选深色） | P2 | S | UI |

### 3.8 主题 H · 工程化

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| H-01 | CI 化（GitHub Actions：`go test ./...` + `vite build` + lint） | P0 | S | `.github/workflows/` |
| H-02 | 覆盖率门槛：核心包（runner / llm / tools / network / agent）≥ 70% | P1 | M | 现有测试基础 |
| H-03 | 端到端冒烟测试：起服务 + 创建 agent + 发一条消息 + 解析 SSE | P1 | M | `scripts/test/` |
| H-04 | Release 自动化：tag 触发 → 多平台二进制 + CHANGELOG 摘要 → 上传 GitHub Release | P1 | S | `scripts/release.sh`、Actions |
| H-05 | 代码风格统一：`golangci-lint` + `eslint`（已有 vite，但缺 lint 配置） | P0 | S | 配置文件 |

### 3.9 主题 I · 安全与合规

| ID | 提案 | 优先级 | 规模 | 关键依赖 |
|----|------|-------|-----|---------|
| I-01 | Token 轮换：`zyhive token --rotate`，强制所有 SSE 重新握手 | P1 | S | `pkg/config`、CLI |
| I-02 | API key at-rest 加密（`zyhive.json` 内 Provider key 默认 AES-GCM，机器绑定密钥） | P2 | M | `pkg/config` |
| I-03 | 工具调用审计：高危工具（exec/messaging/feishu_send）独立 `audit.log`，可按 agent 过滤 | P1 | S | `pkg/convlog`、`pkg/tools` |
| I-04 | 内容安全：飞书/TG 入站消息可选过敏词/PII 标记，注入到 system prompt 让 AI 知情 | P2 | M | 新增 `pkg/safety/`、channel 接入 |

---

## 4. 推荐执行顺序（按依赖排）

执行顺序按"先稳后扩、先共用基础设施后业务功能"组织。每"程"内部条目可并行。

**第 1 程 · 生产基线**
- C-01 结构化日志 + trace id
- C-06 健康监控 endpoint
- H-01 CI workflow
- H-05 lint 配置

**第 2 程 · 自治与配额**
- A-02 budget 预算刹车
- C-02 自适应限流
- C-03 per-token / per-agent 日预算
- I-03 高危工具审计

**第 3 程 · 资产可恢复**
- C-04 备份/恢复 CLI
- C-05 升级失败回滚
- D-01 session 存储抽象 + D-03 大 session 滚动归档

**第 4 程 · 用户能感知到的产品价值**
- A-01 `self_schedule`
- B-02 Chat Profile + B-03 跨 agent 联系人聚合 + B-05 Web 访客升级
- E-01 头像 API
- F-01 审批模式 + F-03 工具调用日志查看

**第 5 程 · 团队协同**
- B-01 会议系统 MVP
- A-03 AI 自动合并联系人
- G-01 全局搜索 + G-02 通知中心

**第 6 程 · 生态扩展**
- F-04 MCP 客户端
- F-05 Skill Marketplace
- E-03 / E-04 / E-05 渠道扩展
- D-02 全局 SQLite 索引
- A-05 主动 escalation

每完成一程，建议产出一篇 CHANGELOG 单元 + 该程下所有 proposal 文档迁入 `proposals/zyhive-improvements/done/` 归档。

---

## 5. 跨主题约束（每个提案都应满足）

1. **向后兼容**：磁盘格式与配置只能新增字段；老结构必须有 idempotent migrate（参考 `pkg/agent/manager.go` 的迁移 hook）。
2. **可观测**：新增子系统必须接入 C-01 结构化日志，并暴露至少 1 个健康指标到 C-06 endpoint。
3. **可关闭**：每个新功能在 `zyhive.json` 下挂一个 `features.{name}: bool`，默认值显式声明（新功能默认 off，稳定后默认 on）。
4. **测试要求**：新代码至少 1 个单元测试 + 一处集成路径覆盖；前端组件至少手动跑过 `npm run build`。
5. **文档要求**：实现完成时同步更新 `README.md` 功能清单 + 对应 `docs/*.md` 设计章节 + 写入 `CHANGELOG.md`（按现有"年.月.日vN"规则）。

---

## 6. 待办：拆分子提案

下一步应在本目录追加：

```
proposals/zyhive-improvements/
├── INDEX.md                        ← 当前文件
├── P0-01-structured-logging.md     ← ✅ 已落 proposal
├── P0-02-readiness-probe.md        ← ✅ 已落 proposal（取代 health-endpoint，仓库已有 /healthz）
├── P0-03-ci-workflow.md            ← ✅ 已落 proposal
├── P1-01-self-schedule-tool.md     ← ✅ 已落 proposal
├── P1-02-budget-brake.md           ← ✅ 已落 proposal
├── P1-03-adaptive-throttle.md      ← ✅ 已落 proposal
├── P1-04-quota-per-agent.md        ← TODO
├── P1-05-backup-restore-cli.md
├── P1-06-update-rollback.md
├── P1-07-session-store-abstraction.md
├── P1-08-large-session-archive.md
├── P1-09-meeting-system-mvp.md
├── P1-10-chat-profile.md
├── P1-11-contacts-cross-agent.md
├── P1-12-web-visitor-promote.md
├── P1-13-avatar-api.md
├── P1-14-tool-approval-mode.md
├── P1-15-tool-audit-log.md
├── P1-16-global-search.md
├── P1-17-notification-center.md
├── P1-18-tool-sandbox.md
├── P1-19-token-rotate.md
├── P1-20-release-automation.md
├── P2-01-memory-self-check.md
├── P2-02-escalation.md
├── P2-03-baton-handoff.md
├── P2-04-mcp-client.md
├── P2-05-skill-marketplace.md
├── P2-06-wechat-channel.md
├── P2-07-webhook-inbound.md
├── P2-08-email-channel.md
├── P2-09-sqlite-index.md
├── P2-10-data-migration.md
├── P2-11-config-hot-reload.md
├── P2-12-apikey-encryption.md
├── P2-13-content-safety.md
├── P2-14-mobile-polish.md
├── P2-15-i18n.md
└── P2-16-dark-theme.md
```

每个子提案模板（建议）：

```markdown
# {ID} · {标题}

- 主题：{A/B/C/...}
- 优先级：P{x}
- 规模：{S/M/L/XL}
- 状态：proposed | accepted | in-progress | done | abandoned

## 1. 背景与问题
## 2. 目标 & 非目标
## 3. 设计要点（数据结构 / API / UI）
## 4. 影响面（涉及的包 / 文件 / 配置）
## 5. 迁移与兼容
## 6. 测试计划
## 7. 文档与 CHANGELOG
## 8. 风险与回滚
```

---

## 7. 关键风险

- **AI 自主性 vs 安全边界**：A-01 / A-05 给 AI 更大主动权，必须配合 A-02 budget 才能上线。
- **MCP / Skill Marketplace 引入外部代码**：F-04 / F-05 需要签名校验、隔离执行、白名单 Provider，不能在 P1 仓促落地。
- **存储抽象**（D-01/D-02）切换风险高：必须双写灰度 + 校验脚本，且至少跨一个版本保留 JSONL 真源。
- **多租户雏形**（C-03）当前 auth 是单 token；多租户彻底化是跨 v 的工作，本计划先做"配额"不做"租户隔离"。

---

## 8. 不在本计划内（明确划清边界）

- 重写为 Rust / 重写前端为 React：无收益，不做。
- 自研模型 / 自研 embedding：维持 Provider 适配模式。
- IDE 插件 / 桌面客户端壳：现 Web UI 已满足，暂不投入。
- 商业化授权后台 / 计费门户：与 zyling 商业线相关，独立项目处理。

---

*文档创建：2026-05-09 · 对应 main 分支 `26.4.23v7` · 维护者：随提案推进同步更新本 INDEX*
