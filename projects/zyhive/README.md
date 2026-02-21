# 引巢 · ZyHive

> zyling 旗下 AI 团队操作系统 — 让每一个 AI 成员各司其职、协同引领

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)

**以团队为核心，每个 AI Agent 是团队成员。**

一行命令安装，打开浏览器即可管理整个 AI 团队：配置每个 AI 的身份、灵魂、记忆、技能，设计组织架构，让 AI 之间互相协作讨论。

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

## 📸 界面预览

> 截图占位 — 启动后访问 http://localhost:8080 查看完整 UI

| 仪表盘 | 对话界面 | 配置中心 |
|--------|---------|---------|
| 成员卡片网格，状态一目了然 | SSE 流式对话 + 工具调用折叠卡 | API Key 测试、模型选择、Telegram 配置 |

---

## ✨ 核心功能

| 功能 | 状态 | 说明 |
|------|------|------|
| 🧑‍💼 **成员管理** | ✅ | 创建多个 AI Agent，每个有独立身份、灵魂、记忆、工作区 |
| 💬 **SSE 对话** | ✅ | 与任意 Agent 实时对话，支持工具调用和流式输出 |
| 📝 **身份编辑** | ✅ | 可视化编辑 IDENTITY.md / SOUL.md，自动保存 |
| 🧠 **记忆管理** | ✅ | 可视化浏览和编辑 Agent 的记忆文件 |
| 📁 **工作区浏览** | ✅ | 文件树 + 在线编辑器，管理 Agent 工作区 |
| ⏰ **定时任务** | ✅ | 可视化配置 Cron 任务，查看执行历史，一键运行 |
| 🔑 **API Key 管理** | ✅ | 支持 Anthropic / OpenAI / DeepSeek，在线测试验证 |
| 🔌 **消息渠道** | 🔧 | 接入 Telegram Bot、iMessage 等 |
| 🎯 **Skills** | 技能安装与管理 |
| 🏢 **组织架构** | 拖拽设计 AI 团队组织图 |
| 🤝 **AI 协同** | 多 Agent 群聊频道，任务委派 |
| 📊 **监控** | Token 用量统计，费用估算 |

---

## 🛠️ 技术架构

```
Vue 3 + Element Plus (SPA)
        ↓ REST API + WebSocket
Go 后端 (Gin, 单二进制)
        ↓
  pkg/runner  ← Agent 对话主循环（工具调用循环）
  pkg/llm     ← Anthropic / OpenAI 流式客户端
  pkg/session ← JSONL 会话存储（兼容 OpenClaw 格式）
  pkg/tools   ← 内置工具（read/write/edit/exec/grep）
  pkg/agent   ← 多 Agent 生命周期 + 工作区管理
  pkg/channel ← Telegram / iMessage 渠道
  pkg/cron    ← 定时任务引擎
```

**架构参考：** [OpenClaw](https://github.com/openclaw/openclaw)

---

## 📁 项目结构

```
ai-panel/
├── cmd/aipanel/main.go     # 入口
├── internal/
│   ├── api/                # REST API (Gin handlers)
│   └── ws/                 # WebSocket hub
├── pkg/
│   ├── config/             # 配置文件解析
│   ├── agent/              # Agent 生命周期
│   ├── llm/                # LLM 客户端
│   ├── session/            # JSONL 会话存储
│   ├── tools/              # 内置工具
│   ├── runner/             # 对话主循环
│   ├── compaction/         # 上下文压缩
│   ├── channel/            # 消息渠道
│   ├── cron/               # 定时任务
│   ├── memory/             # 记忆管理
│   └── skill/              # Skills 管理
├── ui/                     # Vue 3 前端
├── scripts/install.sh      # 一行安装脚本
└── go.mod
```

---

## ⚙️ Configuration

Copy the example config and fill in your API key:

```bash
cp aipanel.example.json aipanel.json
```

Edit `aipanel.json`:

```json
{
  "gateway": {"port": 8080, "bind": "lan"},
  "agents": {"dir": "./agents"},
  "models": {"primary": "anthropic/claude-sonnet-4-6", "apiKeys": {"anthropic": "sk-ant-YOUR-KEY"}},
  "auth": {"mode": "token", "token": "change-me-in-production"}
}
```

| Field | Description |
|-------|-------------|
| `gateway.port` | HTTP server port (default: 8080) |
| `gateway.bind` | Bind mode: `"localhost"`, `"lan"`, or `"0.0.0.0"` |
| `agents.dir` | Root directory for agent workspaces and sessions |
| `models.primary` | Default LLM model in `provider/model` format |
| `models.apiKeys` | Map of provider name → API key (e.g. `{"anthropic": "sk-ant-..."}`) |
| `models.fallbacks` | Optional list of fallback model names |
| `auth.mode` | Authentication mode (`"token"` is the only supported mode currently) |
| `auth.token` | Bearer token required for API access. Set to `"changeme"` to disable auth. |

---

## 📋 实现计划

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 0 | 项目骨架 + 目录结构（15个模块） | ✅ 完成 |
| Phase 1 | LLM 客户端 + Session 存储 + Tools + Runner + Agent Manager + Chat SSE API | ✅ 完成 |
| Phase 2 | Vue 3 UI（仪表盘 / 对话 / 身份编辑器 / 工作区 / Cron）| ✅ 完成 |
| Phase 3 | Telegram 渠道 + Cron 引擎 + 上下文压缩 + 多 Agent 池 | ✅ 完成 |
| Phase 4 | 双栏 AgentCreate + 统一 AiChat 组件 + Session 持久化 + 对话管理页 | ✅ 完成 |
| Phase 5 | Auth 登录页 + Stats 端点 + Telegram 集成测试 + 安装脚本 | 🔨 进行中 |

---

## 📄 License

引巢 · ZyHive 采用 **GNU Affero General Public License v3.0（AGPL-3.0）** 开源协议。

- ✅ 个人使用、学习、研究 — 完全免费
- ✅ 自托管私用 — 完全免费
- ✅ 修改和二次开发 — 必须以相同协议开源
- ⚠️ 基于本项目构建网络服务对外提供 — 必须开源全部改动
- 🚫 商业闭源集成或托管销售 — 需要商业授权

### 商业授权

如需在商业产品中使用，或作为托管 SaaS 服务销售，请联系我们获取商业授权：

**zyling（智引领科技）** — 邮件 / Telegram 联系方式见官网

> 完整协议见 [LICENSE](LICENSE) 文件。
