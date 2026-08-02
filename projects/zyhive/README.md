> **历史快照，不是当前产品文档。**
> 本目录保留早期内嵌项目结构，仅用于追溯；版本、安装命令、配置格式和能力说明均可能过期。
> 当前信息统一读取仓库根目录 [`README.md`](../../README.md)、[`CHANGELOG.md`](../../CHANGELOG.md) 和 GitHub Releases。

<div align="center">

# 引巢 · ZyHive

**AI Team Operating System**

*让每一个 AI 成员各司其职，协同引领*

[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](../../LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![Vue 3](https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js)](https://vuejs.org)
[![Version](https://img.shields.io/badge/version-v0.9.0-orange)]()

[English](#english) · [中文](#中文) · [Demo](#-界面预览) · [快速开始](#-快速开始)

</div>

---

## 是什么？

**ZyHive** 是一个 **AI 团队管理平台**，把多个 AI Agent 组织成真正的"团队"——每个成员有独立的身份、性格、记忆、技能，可以对话、执行任务、定时工作、互相协作。

> 你不再只是和一个 AI 对话，而是在管理一支 AI 团队。

```
一行命令安装 → 打开浏览器 → 管理整个 AI 团队
```

---

## ✨ 核心功能

### 已上线
| 功能 | 说明 |
|------|------|
| 🧑‍💼 **多 Agent 管理** | 创建多个 AI 成员，每个有独立身份（IDENTITY）、灵魂（SOUL）、记忆、工作区 |
| 💬 **实时对话** | SSE 流式输出，支持工具调用（读写文件、执行命令、搜索代码等） |
| 🧠 **记忆系统** | 可视化浏览和编辑每个 Agent 的长期记忆文件 |
| 📁 **工作区管理** | 文件树 + 在线编辑器，直接操作 Agent 工作区 |
| ⏰ **定时任务** | Cron 可视化配置，查看执行历史，支持手动触发 |
| 🔑 **API Key 管理** | 支持 Anthropic / OpenAI / DeepSeek，在线测试验证 |
| 🎯 **Skills 系统** | 为 Agent 安装/卸载技能模块 |
| 📝 **身份编辑器** | 可视化编辑 IDENTITY.md / SOUL.md，实时生效 |

### 开发中
| 功能 | 说明 |
|------|------|
| 🔌 **消息渠道** | Telegram Bot、iMessage 接入，让 Agent 主动触达 |
| 🤝 **多 Agent 协作** | 群聊频道，任务委派，AI 之间互相讨论 |
| 🏢 **组织架构** | 拖拽设计 AI 团队架构图 |
| 📊 **用量监控** | Token 统计，费用估算 |

---

## 🚀 快速开始

### 当前安全安装入口（替代历史 raw main 命令）

```bash
curl -fsSL https://install.zyling.ai/install | bash
```

安装完成后，终端会输出访问地址：

```
✅ ZyHive 安装成功！

  本地访问：  http://localhost:8080
  内网访问：  http://192.168.1.100:8080
  公网访问：  http://123.45.67.89:8080
```

### 配置

```bash
cp aipanel.example.json aipanel.json
```

编辑 `aipanel.json`：

```json
{
  "gateway": {
    "port": 8080,
    "bind": "lan"
  },
  "agents": {
    "dir": "./agents"
  },
  "models": {
    "primary": "anthropic/claude-sonnet-4-6",
    "apiKeys": {
      "anthropic": "sk-ant-YOUR-KEY"
    }
  },
  "auth": {
    "mode": "token",
    "token": "your-access-token"
  }
}
```

| 字段 | 说明 |
|------|------|
| `gateway.port` | HTTP 监听端口，默认 `8080` |
| `gateway.bind` | 绑定模式：`localhost` / `lan` / `0.0.0.0` |
| `agents.dir` | Agent 工作区根目录 |
| `models.primary` | 默认模型，格式 `provider/model` |
| `models.apiKeys` | 各 LLM 提供商的 API Key |
| `auth.token` | 历史配置字段。当前版本要求有效随机令牌，不存在通过 `changeme` 禁用验证的受支持方式 |

---

## 📸 界面预览

| 仪表盘 | 对话界面 | 配置中心 |
|--------|---------|---------|
| 成员卡片网格，状态一目了然 | SSE 流式对话 + 工具调用折叠展示 | API Key 测试、模型选择、渠道配置 |

> 🖼️ 截图即将上线，启动后访问 `http://localhost:8080` 查看完整 UI

---

## 🛠️ 技术架构

```
┌─────────────────────────────────────┐
│         Vue 3 + Element Plus        │  前端 SPA
└──────────────┬──────────────────────┘
               │ REST API + SSE + WebSocket
┌──────────────▼──────────────────────┐
│           Go 后端 (Gin)             │  单二进制，无依赖
├─────────────────────────────────────┤
│  runner   → Agent 对话主循环        │
│  llm      → Anthropic/OpenAI 客户端 │
│  agent    → 多 Agent 生命周期管理   │
│  session  → JSONL 会话持久化        │
│  tools    → 内置工具集              │
│  channel  → Telegram/iMessage 渠道  │
│  cron     → 定时任务引擎            │
│  memory   → 记忆文件管理            │
│  skill    → Skills 系统             │
└─────────────────────────────────────┘
```

### 目录结构

```
zyhive/
├── cmd/aipanel/main.go      # 程序入口
├── internal/
│   ├── api/                 # REST API (Gin handlers)
│   └── ws/                  # WebSocket hub
├── pkg/
│   ├── agent/               # Agent 生命周期
│   ├── runner/              # 对话主循环（工具调用）
│   ├── llm/                 # LLM 流式客户端
│   ├── session/             # 会话 JSONL 存储
│   ├── tools/               # 内置工具（read/write/exec/grep）
│   ├── channel/             # 消息渠道（Telegram 等）
│   ├── cron/                # Cron 定时任务
│   ├── memory/              # 记忆管理
│   ├── skill/               # Skills 系统
│   ├── compaction/          # 上下文压缩
│   └── config/              # 配置解析
├── ui/                      # Vue 3 前端源码
├── scripts/install.sh       # 一键安装脚本
└── go.mod
```

---

## 🗺️ 路线图

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 0 | 项目骨架（15个模块） | ✅ 完成 |
| Phase 1 | LLM 客户端 + Session + Tools + Runner + Chat API | ✅ 完成 |
| Phase 2 | Vue 3 UI（仪表盘 / 对话 / 编辑器 / 工作区 / Cron）| ✅ 完成 |
| Phase 3 | Telegram 渠道 + Cron 引擎 + 上下文压缩 + Agent 池 | ✅ 完成 |
| Phase 4 | Agent 创建向导 + 对话管理 + Session 持久化 | ✅ 完成 |
| Phase 5 | 登录认证 + 用量统计 + Telegram 集成 + 安装脚本 | 🔨 进行中 |
| Phase 6 | 多 Agent 协作 + 消息渠道 + 组织架构 | 📋 规划中 |

---

## 📄 License

历史版本曾写有“商业闭源集成或 SaaS 需要商业授权”，该表述已经废止，不能作为当前许可承诺。

当前 ZyHive 仅按仓库根目录 [LICENSE](../../LICENSE) 的 **AGPL-3.0** 发布，当前没有专有商业许可证或闭源企业版。部署、配置、培训、运维和定制服务可以收费；具体边界以[现行商业边界](../../docs/governance/commercial-boundaries.md)为准。

---

## 🤝 贡献

该历史快照不接受独立修改。问题与改进请提交到[当前仓库 Issues](https://github.com/Zyling-ai/ZyHive/issues)。

---

<div align="center">

Made with ❤️ by **[智引领 · zyling](https://zyling.ai)**

</div>
