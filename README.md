# 引巢 · ZyHive

> zyling 旗下 AI 团队操作系统 — 让每一个 AI 成员各司其职、协同引领

[![GitHub Stars](https://img.shields.io/github/stars/Zyling-ai/zyhive?style=flat&logo=github&color=yellow)](https://github.com/Zyling-ai/zyhive/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/Zyling-ai/zyhive?style=flat&logo=github&color=orange)](https://github.com/Zyling-ai/zyhive/network/members)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-26.4.22v1-brightgreen.svg)](CHANGELOG.md)
[![官网](https://img.shields.io/badge/官网-zyling.ai-6366f1?logo=globe)](https://zyling.ai)

**以团队为核心，每个 AI Agent 是团队成员。**

一行命令安装，打开浏览器即可管理整个 AI 团队：配置每个成员的身份、灵魂、记忆、技能，设计组织架构，让成员之间互相协作讨论。

---

## 🚀 快速开始

> 支持 macOS / Linux（x86_64 / ARM64）

**macOS / Linux：**
```bash
curl -sSL https://install.zyling.ai/install | bash
```

安装完成后，终端直接显示访问地址和访问令牌：

```
╔══════════════════════════════════════════════╗
║  ✅  ZyHive 安装成功！版本: 26.4.1v20         ║
╚══════════════════════════════════════════════╝

  📍 本地访问：  http://localhost:8080
  🏠 内网访问：  http://192.168.1.100:8080
  🌐 公网访问：  http://123.45.67.89:8080
  🔑 管理员 Token：xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## 🌐 智能安装端点

| URL | 说明 |
|-----|------|
| `https://install.zyling.ai/install` | **通用端点**，返回 Linux/macOS bash 脚本 |
| `https://install.zyling.ai/zyhive.sh` | Linux / macOS bash 脚本 |
| `https://install.zyling.ai/latest` | 最新版本号 JSON |
| `https://install.zyling.ai/dl/{ver}/{file}` | 二进制下载代理（国内加速） |

> 国内用户通过 Cloudflare 全球节点加速下载，无需访问 GitHub。

---

## ✨ 核心功能

### 成员管理
- **多 AI 成员**：每个成员有独立的身份（IDENTITY.md）、灵魂（SOUL.md）、记忆、工作区、技能、定时任务、消息渠道
- **系统配置助手 `__config__`**：内置不可删除，启动时自动创建，专门负责全局配置问答
- **独立模型**：每个成员可单独配置大模型（身份 Tab 下拉选择），支持 10+ Provider
- **删除成员**：自动停止 Bot、清理工作区，前端确认弹窗防误操作
- **头像颜色**：每个成员有个性化颜色，图谱 / 对话均展示

### 对话 & 会话
- **流式对话首页（ChatHomeView）**：默认首页即聊天，成员下拉选择器、模型切换、历史会话选择、新对话按钮
- **SSE 流式输出**：打字机效果实时输出，工具调用折叠卡展示（含进行中呼吸灯动画）
- **Token 用量实时显示**：每条助手消息底部显示 `↑ input ↓ output tokens`，done 事件汇总
- **会话持久化**：JSONL 格式存储，含消息历史、Token 估算、上下文压缩（Compaction）摘要
- **统一会话侧边栏**：面板会话与 Telegram / Web 渠道会话合并为单一列表，按最后活动时间排序
- **对话管理（ChatsView）**：跨成员查看全部历史对话，工具调用卡片展示，按渠道 / 成员双筛选
- **@ 其他成员**：对话中 @ 转发消息给指定成员，获取跨成员回复
- **派遣任务面板（DispatchPanel）**：`agent_spawn` 触发时被派遣成员头像飞入顶部，橙灯=执行中 / 绿灯=完成 / 红灯=失败

### 工作区 & 知识
- **文件管理**：文件树递归展示（SVG 矢量图标）、在线编辑器、创建 / 删除文件
- **分层记忆系统**：`memory/core/` + `memory/projects/` + `memory/daily/` + `memory/topics/` 四层目录，轻量 INDEX.md 注入系统提示词
- **memory_search 工具**：向量 + BM25 双模式语义检索，有 Embedding API 时向量检索，无则 BM25 降级
- **记忆蒸馏（Consolidator）**：自动将 daily 层短期日志提炼合并到 core 层长期记忆
- **共享团队工作区（Projects）**：多成员共享项目文件夹，支持 per-agent 读写权限配置

### 工具生态（70+ 工具）
- **执行工具**：`exec`（bash 命令）、`read` / `write` / `edit`（文件操作）、`glob`（文件匹配）
- **浏览器自动化（go-rod）**：`browser_navigate` / `snapshot` / `screenshot` / `click` / `type` / `fill` / `press` / `hover` / `scroll` / `select` / `eval` / `wait`，支持 ARIA 快照
- **进程管理**：`process`（管理后台命令会话，list / poll / log / write / kill）
- **记忆检索**：`memory_search`（向量 + BM25 语义检索）
- **网络工具**：`web_search`（Brave Search API）、`web_fetch`（抓取页面内容）
- **图像分析**：`image`（Vision 模型分析图片）
- **消息推送**：`messaging`（向 Telegram 等渠道发消息）
- **定时任务**：`cron_list` / `cron_add` / `cron_update` / `cron_remove` / `cron_run`
- **多会话管理**：`sessions_list` / `sessions_history` / `sessions_send` / `sessions_spawn`（派遣子成员）
- **ACP 编程代理**：`acp_*`（spawn ACP 代理 session，用于长任务编程委派）
- **项目工作区**：`project_list` / `project_read` / `project_write` / `project_glob`

### 工具权限系统
- 每个成员可独立配置工具策略：`allow`（默认允许）/ `deny`（默认拒绝）+ 精细白名单 / 黑名单
- 工具按组管理：`group:filesystem` / `group:runtime` / `group:browser` / `group:network` 等
- 高危工具（如 `exec`）支持需用户审批模式（`ask`）

### 定时任务（Cron）
- **隔离会话**：每次 Cron 任务在独立 session 中执行，不污染主对话历史
- **表达式支持**：标准 cron 表达式 + 时区配置
- **Cron 管理 UI（CronView）**：可视化创建、编辑、立即执行、查看历史记录

### 目标规划（Goals）
- **甘特图（GoalsView）**：可拖拽时间线，7 级缩放（今 / 周 / 月 / 季 / 半年 / 年 / 三年），惯性滑动，今日锚定
- **里程碑管理**：目标分解为可追踪里程碑节点，关联负责成员
- **AI 迭代评审**：关联 Cron 任务，AI 定期自动写进度评审报告
- **Goals 聊天**：每个目标独立聊天 session，不污染其他对话

### 子成员（Subagents）
- **Subagents 管理（SubagentsView）**：查看所有派遣中的子成员任务，状态 / 模型 / 耗时实时显示
- **派遣结果回传**：子成员完成后自动将结果推送回主成员对话

### 消息渠道
- **Telegram Bot**：每个成员可绑定独立 Bot（per-agent），支持 per-chat 持久会话、命令菜单、图片媒体处理
- **Web 公开聊天（PublicChatView）**：无需登录的公开对话页面，适合对外展示
- **渠道管理（ChannelsView）**：可视化管理 Telegram token 配置，实时测试连接

### 多模型支持（10+ Provider）
- Anthropic Claude（claude-3-5/3-7 系列）
- OpenAI（GPT-4o / o1 / o3 系列）
- DeepSeek（deepseek-chat / deepseek-reasoner）
- MiniMax（abab 系列，特殊 POST 探测适配）
- 智谱 AI（GLM-4 系列）
- Moonshot（kimi 系列）
- Qwen（通义千问系列）
- OpenRouter（聚合多家 Provider）
- 自定义 OpenAI 兼容端点（Custom）
- **Provider API Key 管理（ModelsView）**：可视化管理所有 Provider 配置，实时测试连通性

### Token 用量统计
- **UsageView**：按成员 / 日期 / Provider 统计 Token 消耗与费用
- **实时计费**：每次对话 done 事件返回 inputTokens + outputTokens + 估算费用
- **多 Provider 计费单价**：内置主流 Provider 官方计费标准

### 系统管理
- **在线升级（UpdateView）**：检测 GitHub 最新版本，一键在线升级，五阶段进度显示（下载→验证→应用→完成）
- **日志查看（LogsView）**：实时系统日志，浅色主题终端风格
- **技能工作室（SkillStudio）**：安装、启用、编辑成员技能（SKILL.md）
- **设置（SettingsView）**：全局配置、Provider 管理、模型选择、系统提示词调试

---

## 🗂 项目结构

```
zyhive/
├── cmd/aipanel/
│   ├── main.go          ← 主入口（服务启动 / 平台服务注册）
│   └── cli.go           ← CLI 子命令（start/stop/restart/status/enable/disable/token）
├── internal/api/
│   ├── router.go        ← 路由注册（所有 REST API）
│   ├── chat.go          ← SSE 流式对话端点
│   ├── agents.go        ← 成员 CRUD
│   ├── sessions.go      ← 会话管理
│   ├── relations.go     ← 关系图谱 + SVG 渲染
│   ├── update.go        ← 在线升级（五阶段状态机）
│   ├── goals.go         ← 目标规划 API
│   ├── projects.go      ← 共享项目工作区 API
│   ├── subagents.go     ← 子成员 API
│   ├── usage.go         ← Token 用量统计 API
│   └── ...
├── pkg/
│   ├── agent/           ← 成员生命周期 + 工作区 + IDENTITY/SOUL + 关系图
│   ├── runner/          ← 对话主循环（工具调用循环）+ 系统提示词构建
│   ├── session/         ← 会话工作者池 + Broadcaster + 持久化
│   ├── llm/             ← 10+ Provider 适配（StreamEvent 统一抽象）
│   ├── tools/           ← 70+ 工具注册 + 权限策略（ToolPolicy）
│   ├── memory/          ← 四层记忆树 + 索引构建 + 语义检索
│   ├── channel/         ← Telegram Bot + 渠道路由
│   ├── cron/            ← Cron 引擎（隔离会话）
│   ├── goal/            ← 目标规划数据结构
│   ├── subagent/        ← 子成员派遣管理
│   ├── browser/         ← 浏览器自动化（go-rod）
│   ├── skill/           ← 技能元数据管理
│   ├── project/         ← 共享项目工作区
│   ├── usage/           ← Token 计费与存储
│   ├── config/          ← 配置结构（ProviderEntry 列表）
│   └── compaction/      ← 上下文压缩
└── ui/src/
    ├── views/
    │   ├── ChatHomeView.vue      ← 对话首页（默认页面）
    │   ├── AgentDetailView.vue   ← 成员详情（身份/灵魂/工作区/Cron/渠道）
    │   ├── ChatsView.vue         ← 全局对话管理
    │   ├── GoalsView.vue         ← 目标规划 + 甘特图
    │   ├── SubagentsView.vue     ← 子成员任务监控
    │   ├── TeamView.vue          ← 团队关系图谱
    │   ├── ModelsView.vue        ← Provider & 模型管理
    │   ├── UsageView.vue         ← Token 用量统计
    │   ├── ProjectsView.vue      ← 共享项目工作区
    │   ├── LogsView.vue          ← 系统日志
    │   ├── ToolsView.vue         ← 工具权限管理
    │   └── ...
    └── components/
        ├── AiChat.vue            ← 核心对话组件（SSE + 工具卡）
        ├── WorkspaceChatLayout.vue ← 工作区内嵌对话布局
        ├── DispatchPanel.vue     ← 子成员派遣状态面板
        └── SkillStudio.vue       ← 技能工作室
```

---

## ⚙️ 配置文件

默认位置（一键安装后自动生成）：
- Linux / macOS root：`/etc/zyhive/zyhive.json`
- macOS 用户：`~/.config/zyhive/zyhive.json`

```json
{
  "gateway": {
    "port": 8080,
    "bind": "lan"
  },
  "auth": {
    "mode": "token",
    "token": "your-token-here"
  },
  "agents": {
    "dir": "./agents"
  },
  "models": {
    "primary": "anthropic/claude-sonnet-4-6"
  },
  "providers": [
    {
      "id": "anthropic-1",
      "type": "anthropic",
      "apiKey": "sk-ant-...",
      "name": "Anthropic"
    },
    {
      "id": "openai-1",
      "type": "openai",
      "apiKey": "sk-...",
      "name": "OpenAI"
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `gateway.port` | HTTP 服务端口（默认 8080） |
| `gateway.bind` | 绑定模式：`localhost` / `lan` / `0.0.0.0` |
| `auth.token` | Bearer Token，用于 API 鉴权 |
| `agents.dir` | 成员数据根目录 |
| `models.primary` | 默认模型（`provider/model` 格式） |
| `providers[]` | Provider 列表（type / apiKey / baseUrl 等） |

---

## 🔨 开发构建

```bash
# 前端依赖
cd ui && npm install

# 完整构建（必须用 make，不能直接 go build）
make build
# 等价于: vite build + make sync-ui + go build

# sync-ui：将 ui/dist 同步到 cmd/aipanel/ui_dist（go:embed 读取此目录）
make sync-ui

# 多平台发布构建
cd ui && npm run build && cd ..
make release

# 启动
./bin/aipanel --config aipanel.json
```

> ⚠️ 直接 `go build` 会缺少 UI 静态文件（go:embed ui_dist），**必须用 `make build`**

---

## 📋 版本里程碑

| 版本 | 内容 | 状态 |
|------|------|------|
| v0.1–v0.4 | 项目骨架、LLM 客户端、Session 存储、Tools、Runner、Vue 3 UI | ✅ |
| v0.5 | Auth、Stats、安装脚本、多 Agent 协同 | ✅ |
| v0.6 | 记忆模块、团队关系图谱、Telegram 完整能力 | ✅ |
| v0.7 | 消息渠道下沉成员级别、per-agent 独立 Bot | ✅ |
| v0.8 | SkillStudio 技能工作室、Web 多渠道隔离、历史对话系统 | ✅ |
| v0.9.0 | 团队图谱交互、全局项目系统、成员管理增强 | ✅ |
| v0.9.1–v0.9.11 | 后台任务系统、移动端响应式、Telegram 持久会话、CF 加速节点、稳定版 | ✅ |
| v0.9.12–v0.9.17 | 三级记忆系统、多 Provider 支持、Config migration v1→v2、OpenAI-compat 工具修复 | ✅ |
| v0.9.18–v0.9.23 | MiniMax / DeepSeek 修复、Provider API Key 管理 UI、Goals 目标规划（甘特图）、Cron 隔离会话 | ✅ |
| v0.9.24 | 甘特图全面重构（7 级缩放、惯性拖拽、今日锚定、v-for key 重复修复）、memory_search 工具 | ✅ |
| v0.9.25 | 浏览器自动化（go-rod，16 工具，ARIA 快照）、Cron 隔离 session、send_message 工具 | ✅ |
| v0.9.26 | localStorage 版本检查缓存 bug 修复（semver 比较）| ✅ |
| v0.9.27 | 58 个工具单元测试、agent_spawn 始终注册修复 | ✅ |
| v0.10.x | Provider 测试修复、MiniMax POST 探测、新登录页 | ✅ |
| v0.10.15 | CLI 子命令（zyhive start / stop / restart / status / enable / disable / token） | ✅ |
| v0.10.16–v0.10.20 | 全新聊天首页（ChatHomeView）、历史会话选择、成员下拉、Token 用量显示 | ✅ |
| 26.3.17v1 | 版本号格式变更（年.月.日vN）；工具生态全面升级（web_search / image / process / cron_* / sessions_* / acp_*）；工具权限策略系统；内置心跳；ACP 编程代理 | ✅ |
| 26.3.17v2 | 对话区高度修复（is-chat-page flex 链）| ✅ |
| 26.3.17v3 | AgentDetailView + WorkspaceChatLayout 深色主题统一 | ✅ |
| **26.3.18v1–v8** | **全站浅色主题**：移除 dark mode，恢复所有页面浅色配色；侧边栏折叠按钮；Token 用量 SSE 正确透传；LogsView 浅色终端风格 | ✅ |
| **26.3.29v1–v15** | **派遣任务体验**：计时显示、LLM 续写汇报；空白气泡彻底修复；输入框超长滚动 | ✅ |
| **26.3.31v1** | **Coordinator 模式**：多 Agent 协调者提示词、task-notification XML、AgentDefinition 标准化、SessionMemory 后台提取 | ✅ |
| **26.4.1v1–v20** | **飞书渠道全面接入**：WS 长连接（protobuf）、流式卡片回复、7 大飞书能力工具、群聊模式配置、配对授权优化、多人对话上下文区分 | ✅ |
| **26.4.11–26.4.19** | **会话与内存增强**：四层分层记忆树（core/projects/daily/topics + INDEX.md）、Consolidator 蒸馏、一键安装脚本加固、测试基础设施修复、CGO_ENABLED=0 静态二进制（CentOS 7 兼容） | ✅ |
| **26.4.20v1** | **CLI 全面测试**：6 处 CLI 交互 bug 修复（双 pause / 隐藏目录 / 更新 URL / 备份目录持久化 / 中文 help / 取消更新提示）+ 42 断言回归脚本；**聊天 UI 重构**：Cursor 极简风（细滚动条 / 气泡 / Markdown GFM+ 代码高亮 / 工具卡淡化 / 输入区胶囊 / Enter 发送）；渠道识别 + 只读模式 | ✅ |
| **26.4.20v2** | **AI 能力扩展**：工具体检（ready/blocked）+ 愿望清单（wish_add/wish_list）+ 系统提示词注入"当下信息"；修复 anthropic `output_tokens=0` 真·根因 + 并行工具调用状态 + UsageView 饼图 legend 挤压 + Web 面板会话误判只读；全站视觉一致性 + 边框色统一 | ✅ |
| **26.4.20v3** | **关系双向同步 & 派遣权限**：RELATIONS.md 全类型双向（上下级/平级/支持/其他）、agent_spawn 必须在关系表内（built-in 类型豁免）、前端切换 tab 自动刷新、派遣规则写入系统提示词；**对话管理 drawer 历史消息**修复（AiChat 始终 mount + loading overlay）；capabilities context 完整注入 runner | ✅ |
| **26.4.21v1** | **极简 AI 自主三件套**：用户档案 `memory/core/user-profile.md`（AgentDetailView 编辑卡 + system prompt 注入，让 AI 知道"我服务于谁"）；CronView 🌅 晨间例行一键模板（选 agent + 时间，末尾 `NO_ALERT` 对接 cron engine 静默机制，无事不打扰）；TeamView 💡 建议连接（未建立关系的 agent 对 · 一键平级协作）；AiChat 档位 hashtag chip（#简答 / #深思考 / #写代码 / #闲聊 / #急，system prompt 约定自动调节风格） | ✅ |
| **26.4.22v1** | **通讯录（network/）+ 渐进式披露**：每个 agent 一本私有通讯录 `workspace/network/{INDEX.md, RELATIONS.md, contacts/*.md}`；4 处消息入口（面板/TG/飞书/Web）自动识别来源 + 建档 `{source}:{externalId}` → 按会话注入「当前对话对方」摘要（~300 chars），完整档案 AI 通过 `read` 按需读取（~500 chars INDEX 首层 + 运行时摘要第二层）；`network_note` 工具让 AI 原子追加事实/偏好/待跟进；TeamView 加 tab 切换：「AI 成员网络」图谱 + 「联系人」聚合列表 + 抽屉编辑（显示名/标签 6 预设/`isOwner`/Markdown body）；菜单「团队」→「通讯录」；`memory/core/user-profile.md` → `owner-profile.md` + `RELATIONS.md` 迁入 `network/` 自动 idempotent 迁移 | ✅ |
| v0.11（规划中）| 团队规划系统增强、会议系统、ChatsView 统一重写、共享工作区权限 UI | 🔜 |

---

## 📄 License

引巢 · ZyHive 采用 **GNU Affero General Public License v3.0（AGPL-3.0）** 开源协议。

- ✅ 个人使用、学习、研究 — 完全免费
- ✅ 自托管私用 — 完全免费
- ✅ 修改和二次开发 — 必须以相同协议开源
- ⚠️ 基于本项目构建网络服务对外提供 — 必须开源全部改动
- 🚫 商业闭源集成或托管销售 — 需要商业授权

**zyling（智引领科技）** — 商业授权联系方式见 [zyling.ai](https://zyling.ai)
