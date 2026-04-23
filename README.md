# 引巢 · ZyHive

> zyling 旗下 AI 团队操作系统 — 让每一个 AI 成员各司其职、协同引领

[![GitHub Stars](https://img.shields.io/github/stars/Zyling-ai/zyhive?style=flat&logo=github&color=yellow)](https://github.com/Zyling-ai/zyhive/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/Zyling-ai/zyhive?style=flat&logo=github&color=orange)](https://github.com/Zyling-ai/zyhive/network/members)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-26.4.23v5-brightgreen.svg)](CHANGELOG.md)
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
║  ✅  ZyHive 安装成功！版本: 26.4.23v5         ║
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
- **能力愿望清单 WISHLIST**：AI 主动用 `wish_add` 工具表达能力缺口（例 "我希望能联网"），用户在身份 tab 底部可见愿望卡（P0/P1/P2 优先级 + 理由 + 时间）
- **工具体检**：实时列出 ready / blocked 工具（例 `web_search: 未配置 Brave Key`），AI 不再猜能力边界

### 对话 & 会话
- **流式对话首页（ChatHomeView）**：默认首页即聊天，成员下拉选择器、模型切换、历史会话选择、新对话按钮
- **SSE 流式输出**：打字机效果实时输出，工具调用折叠卡展示（含进行中呼吸灯动画）
- **Token 用量实时显示**：每条助手消息底部显示 `↑ input ↓ output tokens`，done 事件汇总
- **会话持久化**：JSONL 格式存储，含消息历史、Token 估算、上下文压缩（Compaction）摘要
- **统一会话侧边栏**：面板会话与 Telegram / 飞书 / Web 渠道会话合并为单一列表，按最后活动时间排序
- **对话管理（ChatsView）**：跨成员查看全部历史对话，统一 AiChat 渲染（GFM 表格/代码高亮/blockquote/工具卡可展开），按渠道 / 成员双筛选
- **@ 其他成员**：对话中 @ 转发消息给指定成员，获取跨成员回复
- **派遣任务面板（DispatchPanel）**：`agent_spawn` 触发时被派遣成员头像飞入顶部，橙灯=执行中 / 绿灯=完成 / 红灯=失败
- **档位 hashtag chip**：输入框上方 5 个极小 chip（`#简答` / `#深思考` / `#写代码` / `#闲聊` / `#急`），点击追加到输入末尾，system prompt 约定 AI 见到 hashtag 自动调节风格
- **聊天快捷键**：Enter 发送 / Shift+Enter 换行；只读模式（飞书/TG 会话面板只看）显示锁图标提示条
- **Cursor 风输入区**：克制配色（发送按钮有内容时变 Cursor 黑 `#18181b`）、去蓝色 focus 环、细滚动条、AI 消息去气泡（文档流阅读）

### 工作区 & 知识
- **文件管理**：文件树递归展示（SVG 矢量图标）、在线编辑器、创建 / 删除文件
- **分层记忆系统**：`memory/core/` + `memory/projects/` + `memory/daily/` + `memory/topics/` 四层目录，轻量 `memory/INDEX.md` 注入系统提示词
- **Owner 档案（`memory/core/owner-profile.md`）**：AgentDetailView "身份 & 灵魂" tab 第 3 张卡，让 AI 知道"我服务于谁"，每次对话开始自动读取，空白完整 placeholder 模板可参考
- **memory_search 工具**：向量 + BM25 双模式语义检索，有 Embedding API 时向量检索，无则 BM25 降级
- **记忆蒸馏（Consolidator）**：自动将 daily 层短期日志提炼合并到 core 层长期记忆
- **共享团队工作区（Projects）**：多成员共享项目文件夹，支持 per-agent 读写权限配置

### 通讯录 & 关系网（network/）— 26.4.22v1 新增
让 AI 在"每个消息来源都能准确回复" —— 把用户档案 / 关系网 / 外部联系人统一为一张图。

- **每个 agent 一本私有通讯录** `workspace/network/{INDEX.md, RELATIONS.md, contacts/*.md}`
- **三层渐进式披露**：
  - 层 1：`network/INDEX.md` 永远注入（~500 chars 轻量列表）
  - 层 2：当前对话对方摘要（frontmatter + 事实前 3 条，~300 chars）运行时注入
  - 层 3：完整档案，AI 按需用 `read("network/contacts/<id>.md")` 自取
- **自动建档**：面板/飞书/TG/Web Public 4 处消息入口检测到新 sender，立刻生成 `{source}:{externalId}.md`（例 `feishu-ou_abc.md`）
- **`network_note(entityId, section, text)` 工具**：AI 发现重要事实/偏好/待跟进时原子追加（section 严格枚举），旁路 `network/changes.log` 审计
- **TeamView 融合**：菜单「团队」→「📇 通讯录」，顶部 tab 切换「🧑‍🤝‍🧑 AI 成员网络」｜「👥 联系人」。联系人列表跨 agent 聚合，可搜索 / 按来源 / 按 agent 筛选；点击开 540px 抽屉编辑（显示名、6 预设标签快捷 `#家人/#同事/#客户/#合作伙伴/#朋友/#AI 成员`、`isOwner` 标记、Markdown body）
- **关系双向同步**：`RELATIONS.md` 所有类型（上下级/平级/支持/其他/服务/客户/...）自动双向写入；`agent_spawn` 必须在关系表内（内置 agent 类型豁免）
- **团队图谱 💡 建议连接**：未建立关系的成员对一键建立平级协作关系

### 工具生态（80+ 工具）
- **执行工具**：`exec`（bash 命令）、`read` / `write` / `edit`（文件操作）、`glob`（文件匹配）、`grep`
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
- **飞书能力**：`feishu_send_message` / `feishu_create_chat` / `feishu_calendar_*` / `feishu_sheets_*` / `feishu_upload_image` / `feishu_reply_with_card`（7 大飞书工具）
- **自我管理**：`self_list_skills` / `self_install_skill` / `self_uninstall_skill` / `self_rename` / `self_update_soul`
- **愿望清单**：`wish_add(title, reason, priority)` / `wish_list` — AI 主动记录能力诉求
- **通讯录维护**：`network_note(entityId, section, text)` — AI 追加事实/偏好/待跟进到联系人档案

### 工具权限系统
- 每个成员可独立配置工具策略：`allow`（默认允许）/ `deny`（默认拒绝）+ 精细白名单 / 黑名单
- 工具按组管理：`group:filesystem` / `group:runtime` / `group:browser` / `group:network` 等
- 高危工具（如 `exec`）支持需用户审批模式（`ask`）

### 定时任务（Cron）
- **隔离会话**：每次 Cron 任务在独立 session 中执行，不污染主对话历史
- **表达式支持**：标准 cron 表达式 + 时区配置
- **`NO_ALERT` 静默机制**：AI 若无事可汇报，只回一个 `NO_ALERT` 即被 cron engine 识别并静默，不打扰用户
- **Cron 管理 UI（CronView）**：可视化创建、编辑、立即执行、查看历史记录
- **🌅 晨间例行一键模板**：选 agent + 时间（HH:mm）+ 时区，自动构造 cron 表达式 + 预置 prompt（整理昨日 / 检查 WISHLIST / 留便条到 `memory/daily/notes-to-user.md`），对接 NO_ALERT 无事静默

### 目标规划（Goals）
- **甘特图（GoalsView）**：可拖拽时间线，7 级缩放（今 / 周 / 月 / 季 / 半年 / 年 / 三年），惯性滑动，今日锚定
- **里程碑管理**：目标分解为可追踪里程碑节点，关联负责成员
- **AI 迭代评审**：关联 Cron 任务，AI 定期自动写进度评审报告
- **Goals 聊天**：每个目标独立聊天 session，不污染其他对话

### 子成员（Subagents）
- **Subagents 管理（SubagentsView）**：查看所有派遣中的子成员任务，状态 / 模型 / 耗时实时显示
- **派遣结果回传**：子成员完成后自动将结果推送回主成员对话

### 消息渠道
- **Telegram Bot**：每个成员可绑定独立 Bot（per-agent），支持 per-chat 持久会话、命令菜单、图片媒体处理；发送者自动建档到通讯录
- **飞书（Lark）**：WebSocket 长连接 + 流式卡片回复 + 7 大飞书能力工具 + 群聊 @ 模式配置 + 多人对话上下文区分 + 发送者自动建档
- **Web 公开聊天（PublicChatView）**：无需登录的公开对话页面，visitor sessionToken 自动建档到 `network/contacts/web-*.md`
- **渠道管理（ChannelsView）**：可视化管理 Telegram / 飞书 token 配置，实时测试连接
- **`source` 精细识别**：侧边栏按来源打标签（飞书/TG/Web/面板 4 色区分），飞书/TG 会话自动只读 + 锁图标

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
- **日志查看（LogsView）**：三级降级读取（`/tmp/aipanel.log` → `journalctl` → macOS `log show`），浅色主题终端风格
- **技能工作室（SkillStudio）**：安装、启用、编辑成员技能（SKILL.md）
- **设置（SettingsView）**：全局配置、Provider 管理、模型选择、系统提示词调试
- **交互式 CLI 面板**：`zyhive` 直接进入终端管理面板（配置 / 成员 / 更新 / 备份 / 日志 / 状态），支持中文 help；`zyhive token / start / stop / restart / status / enable / disable` 子命令

### 系统提示词工程（渐进式披露）
每次对话 system prompt **严格分层**构建：
1. **当下信息**：日期/时间/周数/年度第 N 天 · Platform · 训练截止警告 · wish_add 提示 · 档位 hashtag 约定
2. **Owner profile**：`memory/core/owner-profile.md`（你是谁）
3. **IDENTITY + SOUL**：AI 自己是谁
4. **memory/INDEX.md**：记忆轻量索引
5. **network/INDEX.md + RELATIONS.md**：通讯录轻量索引 + 关系表
6. **当前会话对方摘要**（渠道来的对话才注入）：运行时 Store.Summary 动态填
7. **Capabilities context**：工具体检 + WISHLIST 头部
8. **AGENTS.md 引用链**：自动读取 AGENTS.md 里引用的其他文件
9. **Projects context**：共享项目可读写情况
每一层都有 `truncateForPrompt` 截断保护（~20K chars 上限），总体控制在合理 token 预算内。

---

## 🗂 项目结构

```
zyhive/
├── cmd/aipanel/
│   ├── main.go          ← 主入口（服务启动 / 平台服务注册）
│   ├── cli.go           ← 交互式 CLI 面板 + 子命令（start/stop/restart/status/enable/disable/token）
│   └── ui_dist/         ← go:embed 前端构建产物
├── internal/api/
│   ├── router.go        ← 路由注册（所有 REST API）
│   ├── chat.go          ← SSE 流式对话端点 + UsageRecorder
│   ├── agents.go        ← 成员 CRUD
│   ├── sessions.go      ← 会话管理
│   ├── relations.go     ← 关系图谱 + 全类型双向同步 + `relationsPath()` 迁移兼容
│   ├── network.go       ← 通讯录 REST（list/get/patch/delete/merge/refresh）
│   ├── update.go        ← 在线升级（五阶段状态机）
│   ├── goals.go         ← 目标规划 API
│   ├── projects.go      ← 共享项目工作区 API
│   ├── subagents.go     ← 子成员 API
│   ├── usage.go         ← Token 用量统计 API
│   ├── public_chat.go   ← Web 公开聊天入口
│   ├── feishu_callback.go ← 飞书回调
│   └── ...
├── pkg/
│   ├── agent/           ← 成员生命周期 + 工作区 + IDENTITY/SOUL + 关系图 + manager.go 自动迁移 hook
│   ├── runner/          ← 对话主循环（工具调用循环）+ system_prompt.go 分层构建（9 层）
│   ├── session/         ← 会话工作者池 + Broadcaster + 持久化
│   ├── llm/             ← 10+ Provider 适配（StreamEvent 统一抽象）
│   ├── tools/           ← 80+ 工具注册 + 权限策略（ToolPolicy）+ wish / network_note / capabilities
│   ├── memory/          ← 四层记忆树 + INDEX.md + Consolidator 蒸馏 + 语义检索
│   ├── network/         ← 通讯录（contact Store + codec + summary + migrate），每 agent 私有
│   ├── channel/         ← Telegram / 飞书 / 渠道路由 + Feishu WS 长连接
│   ├── convlog/         ← 管理员可见的对话全量日志（JSONL）
│   ├── chatlog/         ← 渠道消息日志
│   ├── cron/            ← Cron 引擎（隔离会话 + NO_ALERT 静默机制）
│   ├── goal/            ← 目标规划数据结构
│   ├── subagent/        ← 子成员派遣管理
│   ├── browser/         ← 浏览器自动化（go-rod，16 工具）
│   ├── skill/           ← 技能元数据管理
│   ├── project/         ← 共享项目工作区
│   ├── usage/           ← Token 计费与存储
│   ├── config/          ← 配置结构（ProviderEntry 列表 + 模型条目）
│   └── compaction/      ← 上下文压缩
└── ui/src/
    ├── views/
    │   ├── ChatHomeView.vue      ← 对话首页（默认页面）
    │   ├── AgentDetailView.vue   ← 成员详情（身份/灵魂/工作区/Cron/渠道/工具权限/环境变量）
    │   ├── AgentsView.vue        ← 成员列表
    │   ├── AgentCreateView.vue   ← 创建成员向导
    │   ├── ChatsView.vue         ← 全局对话管理（统一 AiChat 渲染）
    │   ├── GoalsView.vue         ← 目标规划 + 甘特图
    │   ├── SubagentsView.vue     ← 子成员任务监控
    │   ├── TeamView.vue          ← 通讯录（AI 成员网络 + 联系人 tab）
    │   ├── ModelsView.vue        ← Provider & 模型管理
    │   ├── UsageView.vue         ← Token 用量统计（stat cards + pie charts）
    │   ├── ProjectsView.vue      ← 共享项目工作区
    │   ├── LogsView.vue          ← 系统日志（journalctl 读）
    │   ├── ToolsView.vue         ← 工具权限管理
    │   ├── ChannelsView.vue      ← 渠道管理
    │   ├── CronView.vue          ← 定时任务（含🌅晨间例行一键）
    │   ├── SettingsView.vue      ← 全局设置
    │   ├── SkillsView.vue        ← 技能
    │   ├── PublicChatView.vue    ← Web 公开对话
    │   ├── LoginView.vue
    │   └── DashboardView.vue
    └── components/
        ├── AiChat.vue            ← 核心对话组件（SSE + 工具卡 + 档位 chip + Markdown GFM）
        ├── WorkspaceChatLayout.vue ← 工作区内嵌对话布局
        ├── DispatchPanel.vue     ← 子成员派遣状态面板
        ├── RelTypeForm.vue       ← 关系类型编辑表单
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
  ],
  "models": [
    {
      "id": "claude-sonnet-4-6",
      "provider": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "name": "Claude Sonnet 4",
      "default": true
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `gateway.port` | HTTP 服务端口（默认 8080） |
| `gateway.bind` | 绑定模式：`localhost` / `lan` / `0.0.0.0` |
| `auth.token` | Bearer Token，用于 API 鉴权 |
| `agents.dir` | 成员数据根目录（每 agent 一个子目录，内含 workspace/memory/network/...） |
| `providers[]` | Provider 列表（type / apiKey / baseUrl 等） |
| `models[]` | 模型条目列表，`default:true` 的作为全局默认（agent 未绑定模型时 fallback） |

> **注**：旧版本用 `models.primary: "provider/model"` 字符串字段，新结构化为 `models[]` 数组 + `default` 标记。配置迁移自动完成，老数据无损。

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
| **26.4.22v2** | **文档全面刷新**：README 功能清单补齐（通讯录/愿望清单/工具体检/档位 chip/晨间例行/Owner 档案/建议连接/渐进式披露）、项目结构加 `pkg/network/` `pkg/convlog/` `pkg/chatlog/`、UI views 补齐、配置示例改 `models[]` 结构、新增「系统提示词工程」章节；`docs/system-prompt-and-flow.md` 重写为 10 层分层渐进披露设计 + Contact 档案模型 + Capabilities Context + Cron NO_ALERT；`docs/session-design.md` 补飞书渠道 + network 联动段落 | ✅ |
| **26.4.22v3** | **在线升级进度条修复**：后端 `downloadFile` 在 CF Worker 流式代理（Content-Length=-1）场景下不再卡进度（按预估 32MB 上限走 0→95%，收尾 progress(100)）；前端 polling 1500ms → 500ms 且首次立即触发；`el-progress :duration 10 → 1`；`stage='done'` 立即 `stopPolling`，新增独立 `waitForRestart` 循环（轮询 `/api/version` 检测新版本，90s 兜底）；页面 mount 自动接管进行中的升级任务（刷新不丢状态） | ✅ |
| **26.4.23v1** | **通讯录 5 个漏网 bug 修复**：`IsOwner=true` contact 跳过 summary 注入（防 owner-profile 双份）；displayName fallback 链（firstName/username 空时自动兜底到 externalID 前缀）；**合并后 alias 自动路由到 primary**（之前会复活空档案，合并白做）；`RELATIONS.md` 正式加 `toKind` 字段（6 列新格式 + 5 列 legacy 兼容），Graph 自动过滤 contact 边（不再创建幽灵节点），`agent_spawn` 权限检查正确忽略 contact ID；`network_note` 失败时返回最接近的 3 个 contact ID 作为 Did-you-mean 提示。测试覆盖：3 新 `TestParseRelationsMarkdown*` + `TestSummaryIsOwnerSkips` + `TestFallbackDisplayName`（6 子 case）+ `TestResolveRoutesThroughAliases` + `TestSuggestContactIDs` | ✅ |
| **26.4.23v3** | **Session 自动主题命名 + UsageView 明细增强**：会话 title 不再只是"第一条 user message 前 60 字"；每当消息数到达 4 / 12 / 30 / 80 时自动后台调 LLM 总结主题（使用会话自己绑定的模型，fire-and-forget 不阻塞对话），生成 8-20 字中文主题；用户手动 PATCH rename → `TitleOverridden=true` 永不被 auto 覆盖；`usage.Record` 扩 `SessionID` 字段，`/api/usage/records` 响应附 `agentName` + `sessionTitle`；UsageView 明细列新增「成员」「Session」列，筛选栏加 Session 下拉（依选中成员联动）；测试：`TestSanitizeTitle`（7 case）+ `TestBuildRetitleInput*`（2）+ `TestNeedsAutoRetitle_Milestones` + `TestMaybeAutoRetitle_RunsSummarizer` | ✅ |
| **26.4.23v2** | **生产稳定性地基（P0 全集）**：**LLM 错误分层 + 瞬时重试**（`pkg/llm/errors.go` 识别 429/5xx/connection reset/eof/tls 等 transient，`pkg/llm/retry.go` 指数退避 0.5s/2s/5s，业务错误 401/context length/content filter 立即上抛）；**SSE 自动重连**（网络断时按 1s/3s/7s 重试 resumeSSE，90s 兜底，含 Did-you-mean 信息）；**错误隔离**（LLM error 不再覆盖部分回复，原气泡打"因错误中断"footer + 独立系统气泡显示友好错误，`formatErrorMessage` 把 429/401/5xx/timeout/context_length 翻译为中文）；**Abort fence**（activeFence 组件级可观测，onUnmounted / session 切换自动 abort 在流 SSE，防止 late-arriving events 污染新会话 + 省 token）；**LLM Provider live health**（`pkg/llm/health.go` Ping 带 30s 缓存，max_tokens=1 最小请求；ToolHealth 响应追加 `providerHealth`，UI 显示 🟢/🔴 + 延迟 + "重新检测" 按钮，401/429/5xx 分类提示）；**Compaction 同步事件**（移到 turn 开头 同步跑 + `compaction_start` / `compaction_end` event，UI 显示"🗜️ 正在压缩..."→"✓ 已压缩 Xk→Yk tokens"）；Throttle 接口抽象（`Throttle` interface + `FixedThrottle` 默认实现，行为 100% 等同今天，留给未来 AdaptiveThrottle）。测试：`TestIsTransientMatches`（13 case）+ `TestRetryClient*`（4 case）+ `TestFixedThrottle*`（5 case） | ✅ |
| P1（规划中）| Chat Profile（群档案）· 跨 agent 联系人聚合视图 · 头像 API 拉取 · AI 自动合并联系人 · Web 访客升级为命名 contact · `self_schedule` 自主闹钟工具 · 自主唤醒 budget 预算刹车 | 🔜 |

---

## 📄 License

引巢 · ZyHive 采用 **GNU Affero General Public License v3.0（AGPL-3.0）** 开源协议。

- ✅ 个人使用、学习、研究 — 完全免费
- ✅ 自托管私用 — 完全免费
- ✅ 修改和二次开发 — 必须以相同协议开源
- ⚠️ 基于本项目构建网络服务对外提供 — 必须开源全部改动
- 🚫 商业闭源集成或托管销售 — 需要商业授权

**zyling（智引领科技）** — 商业授权联系方式见 [zyling.ai](https://zyling.ai)
