# 引巢 · ZyHive

> zyling 旗下 AI 团队操作系统 — 让每一个 AI 成员各司其职、协同引领

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.9.0-brightgreen.svg)](CHANGELOG.md)

**以团队为核心，每个 AI Agent 是团队成员。**

一行命令安装，打开浏览器即可管理整个 AI 团队：配置每个成员的身份、灵魂、记忆、技能，设计组织架构，让成员之间互相协作讨论。

---

## 🚀 快速开始

```bash
curl -sSL https://raw.githubusercontent.com/sunhuihui6688-star/ai-panel/main/scripts/install.sh | bash
```

安装完成后，终端直接显示访问地址：

```
✅ 引巢 · ZyHive 安装成功！

  本地访问：  http://localhost:8080
  内网访问：  http://192.168.1.100:8080
  公网访问：  http://123.45.67.89:8080
```

---

## ✨ 核心功能（v0.9.0）

### 成员管理
- **多 AI 成员**：每个成员有独立的身份（IDENTITY.md）、灵魂（SOUL.md）、记忆、工作区、技能、定时任务
- **系统配置助手 `__config__`**：内置不可删除，启动时自动创建，专门负责全局配置问答
- **换模型**：身份 Tab 中为每个成员独立配置大模型
- **删除成员**：自动停止 Bot、清理工作区，前端确认弹窗防误操作
- **头像颜色**：每个成员有个性化颜色，图谱/对话均展示

### 对话 & 会话
- **SSE 流式对话**：与任意成员实时对话，支持工具调用（折叠卡展示）
- **会话持久化**：JSONL 格式存储，含消息历史、Token 估算、Compaction 摘要
- **历史对话侧边栏**：切换会话、继续历史对话
- **对话管理（ChatsView）**：跨成员查看全部历史对话，按渠道/成员双筛选
- **@ 其他成员**：对话中转发消息给指定成员，获取跨成员回复

### 工作区 & 知识
- **工作区文件管理**：文件树递归展示、在线编辑器、创建/删除任意文件、二进制文件检测
- **身份 & 灵魂编辑**：可视化编辑 IDENTITY.md / SOUL.md，失焦自动保存
- **记忆管理**：浏览和编辑 Agent 记忆文件，支持每日日志、自动整合（Cron 触发）
- **技能系统（SkillStudio）**：三栏布局（技能列表 | 文件编辑 | AI 协作），AI 实时推荐，沙箱隔离

### 团队协作
- **团队图谱（TeamView）**：可拖拽成员节点，拖放创建关系，SVG 精确坐标，4 种关系类型（上下级有向箭头/平级/支持/其他），自动整理排列
- **关系管理**：卡片式弹窗选择关系类型，支持「⇄ 翻转」方向，点击连线编辑/删除
- **AI 协同**：`@` 转发消息给其他成员，多 Agent 讨论

### 消息渠道
- **Telegram Bot**：每个成员独立配置 Bot，白名单管理，图片/视频/音频/文档/媒体组接收，群聊/话题线程支持
- **Web 渠道**：独立 URL `/chat/{agentId}/{channelId}`，支持密码保护，Session 隔离
- **热重载**：新增/修改渠道立即生效，Token 唯一性检测
- **白名单**：待审核用户列表，审核通过自动发送欢迎消息

### 全局项目系统
- **项目管理（ProjectsView）**：左侧项目列表 + 右侧文件浏览器 + 代码编辑器
- **文件树**：递归展示目录结构，支持创建/删除文件
- **标签/描述**：项目元信息管理

### 任务 & 调度
- **定时任务（Cron）**：可视化配置，每个成员独立任务，执行历史，一键运行
- **技能库**：跨成员汇总展示，按成员筛选，一键复制技能

### 模型 & 配置
- **多模型支持**：Anthropic / OpenAI / DeepSeek / 自定义 Base URL
- **在线测试**：API Key 验证、模型 Probe，失败原因实时展示
- **全局 Tools**：内置 read/write/edit/exec/grep，可按成员启用/禁用

---

## 🛠️ 技术架构

```
Vue 3 + Element Plus (SPA)
        ↓ REST API (SSE for streaming)
Go 后端 (Gin，单二进制，go:embed UI)
        ↓
  pkg/runner    ← Agent 对话主循环（工具调用循环）
  pkg/llm       ← Anthropic / OpenAI 流式客户端
  pkg/session   ← JSONL 会话存储，兼容 OpenClaw 格式
  pkg/tools     ← 内置工具（read/write/edit/exec/grep）
  pkg/agent     ← 多成员生命周期 + 工作区管理
  pkg/channel   ← Telegram / Web 渠道
  pkg/cron      ← 定时任务引擎
  pkg/memory    ← 记忆整合（自动 Compaction）
  pkg/skill     ← Skills 管理 + Runner 注入
  pkg/project   ← 全局项目系统
```

**架构参考：** [OpenClaw](https://github.com/openclaw/openclaw)

---

## 📁 项目结构

```
ai-panel/
├── cmd/aipanel/
│   ├── main.go             # 入口，服务启动 + __config__ 自动创建
│   └── ui_dist/            # go:embed 前端构建产物
├── internal/api/           # REST API (Gin handlers)
│   ├── agents.go           # 成员 CRUD，PATCH 换模型
│   ├── chat.go             # SSE 流式对话
│   ├── files.go            # 工作区文件 API（双模式写入）
│   ├── relations.go        # 团队关系 CRUD
│   ├── projects.go         # 全局项目系统
│   ├── sessions.go         # 会话列表 / 历史
│   ├── agent_channels.go   # per-agent 渠道配置
│   └── router.go           # 路由注册
├── pkg/
│   ├── config/config.go    # 配置文件解析
│   ├── agent/manager.go    # 成员生命周期（Create/Update/Remove）
│   ├── agent/pool.go       # Agent 对话池
│   ├── llm/anthropic.go    # Anthropic 流式客户端
│   ├── session/store.go    # JSONL 会话存储
│   ├── tools/tools.go      # 内置工具
│   ├── runner/runner.go    # 对话主循环
│   ├── channel/telegram.go # Telegram Bot
│   ├── channel/hub.go      # 渠道热重载
│   ├── cron/               # 定时任务引擎
│   ├── memory/memory.go    # 记忆整合
│   ├── skill/              # Skills 管理
│   └── project/manager.go  # 项目系统
├── agents/                 # 各成员数据目录
│   ├── __config__/         # 系统配置助手（自动创建）
│   └── {agentId}/
│       ├── config.json     # 成员元数据（id/name/model/channels）
│       ├── workspace/      # IDENTITY.md, SOUL.md, MEMORY.md, memory/
│       │   └── skills/     # 技能文件（skill.json + SKILL.md）
│       └── sessions/       # sessions.json 索引 + *.jsonl 历史
├── projects/               # 全局项目数据
├── ui/src/
│   ├── views/
│   │   ├── AgentDetailView.vue   # 成员详情（对话/身份/关系/记忆/工作区/Cron/渠道）
│   │   ├── AgentCreateView.vue   # 创建成员（含 per-agent 渠道表单）
│   │   ├── AgentsView.vue        # 成员列表（system badge，删除确认）
│   │   ├── TeamView.vue          # 团队图谱（拖拽/连线/整理）
│   │   ├── ProjectsView.vue      # 全局项目（文件树 + 编辑器）
│   │   ├── ChatsView.vue         # 对话管理
│   │   ├── SkillsView.vue        # 技能库（跨成员汇总）
│   │   ├── DashboardView.vue     # 仪表盘（统计卡片）
│   │   ├── ModelsView.vue        # 模型配置
│   │   ├── ChannelsView.vue      # 渠道管理
│   │   ├── CronView.vue          # 定时任务
│   │   └── LoginView.vue         # 登录（验证码）
│   └── components/
│       ├── AiChat.vue            # 通用对话组件（SSE + 工具折叠）
│       ├── SkillStudio.vue       # 技能工作室（三栏 + 并发生成）
│       └── RelTypeForm.vue       # 关系类型卡片选择
├── scripts/install.sh      # 一行安装脚本（多架构/systemd/launchd）
├── Makefile                # make build = sync-ui + go build
├── CHANGELOG.md
└── go.mod
```

---

## ⚙️ 配置

复制示例并填写 API Key：

```bash
cp aipanel.example.json aipanel.json
```

`aipanel.json` 结构：

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
  "models": [
    {
      "id": "claude-sonnet",
      "name": "Claude Sonnet",
      "provider": "anthropic",
      "model": "claude-sonnet-4-6",
      "apiKey": "sk-ant-YOUR-KEY",
      "isDefault": true
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `gateway.port` | HTTP 服务端口（默认 8080）|
| `gateway.bind` | 绑定模式：`localhost` / `lan` / `0.0.0.0` |
| `auth.token` | Bearer Token，用于 API 鉴权 |
| `agents.dir` | 成员数据根目录 |
| `models[].provider` | 模型提供商：`anthropic` / `openai` / `deepseek` |
| `models[].apiKey` | 对应提供商的 API Key |
| `models[].isDefault` | 是否为默认模型 |

---

## 🔨 开发构建

```bash
# 安装前端依赖
cd ui && npm install

# 完整构建（必须用 make，不能直接 go build）
make build
# 等价于: vite build + cp ui/dist → cmd/aipanel/ui_dist + go build

# 启动
./bin/aipanel
```

> ⚠️ 直接 `go build` 会缺少 UI 静态文件（go:embed），必须先 `make build`

---

## 📋 版本里程碑

| 版本 | 内容 | 状态 |
|------|------|------|
| v0.1–0.3 | 项目骨架、LLM 客户端、Session 存储、Tools、Runner、Agent Manager | ✅ |
| v0.4 | Vue 3 UI（仪表盘/对话/身份编辑/工作区/Cron）| ✅ |
| v0.5 | Auth 登录、Stats 端点、安装脚本、多 Agent 协同 | ✅ |
| v0.6 | Telegram 完整能力（图片/视频/群聊/白名单）、Web 渠道、历史对话系统 | ✅ |
| v0.7 | Skills 系统、SkillStudio、渠道热重载、多 Web 渠道隔离 | ✅ |
| v0.8 | SkillStudio 并发架构、UI 全面升级（去 emoji、登录验证码、极简仪表盘）| ✅ |
| v0.9 | 团队图谱交互、全局项目系统、成员管理增强（删除/换模型/系统助手）| ✅ |
| **v0.10** | **团队规划系统、会议系统** | 🔜 规划中 |

---

## 📄 License

引巢 · ZyHive 采用 **GNU Affero General Public License v3.0（AGPL-3.0）** 开源协议。

- ✅ 个人使用、学习、研究 — 完全免费
- ✅ 自托管私用 — 完全免费
- ✅ 修改和二次开发 — 必须以相同协议开源
- ⚠️ 基于本项目构建网络服务对外提供 — 必须开源全部改动
- 🚫 商业闭源集成或托管销售 — 需要商业授权

**zyling（智引领科技）** — 商业授权联系方式见官网
