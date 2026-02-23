# 引巢 · ZyHive

> zyling 旗下 AI 团队操作系统 — 让每一个 AI 成员各司其职、协同引领

[![GitHub Stars](https://img.shields.io/github/stars/Zyling-ai/zyhive?style=flat&logo=github&color=yellow)](https://github.com/Zyling-ai/zyhive/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/Zyling-ai/zyhive?style=flat&logo=github&color=orange)](https://github.com/Zyling-ai/zyhive/network/members)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![Version](https://img.shields.io/badge/version-v0.9.11-brightgreen.svg)](CHANGELOG.md)
[![官网](https://img.shields.io/badge/官网-zyling.ai-6366f1?logo=globe)](https://zyling.ai)

**以团队为核心，每个 AI Agent 是团队成员。**

一行命令安装，打开浏览器即可管理整个 AI 团队：配置每个成员的身份、灵魂、记忆、技能，设计组织架构，让成员之间互相协作讨论。

---

## 🚀 快速开始

> 一条命令，自动识别平台（Windows / macOS / Linux 通用）

**Windows（PowerShell）：**
```powershell
irm https://install.zyling.ai/install | iex
```

**macOS / Linux：**
```bash
curl -sSL https://install.zyling.ai/install | bash
```

**Windows Git Bash / MSYS2 / Cygwin** 与 macOS/Linux 命令相同，脚本会自动检测并调用系统 PowerShell 完成安装。

安装完成后，终端直接显示访问地址和访问令牌：

```
╔══════════════════════════════════════════════╗
║  ✅  ZyHive 安装成功！版本: v0.9.11          ║
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
| `https://install.zyling.ai/install` | **通用端点**，按 User-Agent 自动返回 `.sh` 或 `.ps1` |
| `https://install.zyling.ai/zyhive.sh` | Linux / macOS bash 脚本 |
| `https://install.zyling.ai/zyhive.ps1` | Windows PowerShell 脚本 |
| `https://install.zyling.ai/latest` | 最新版本号 JSON |
| `https://install.zyling.ai/dl/{ver}/{file}` | 二进制下载代理（国内加速） |

> 国内用户通过 Cloudflare 全球节点加速下载，无需访问 GitHub。

---

## ✨ 核心功能

### 成员管理
- **多 AI 成员**：每个成员有独立的身份（IDENTITY.md）、灵魂（SOUL.md）、记忆、工作区、技能、定时任务、消息渠道
- **系统配置助手 `__config__`**：内置不可删除，启动时自动创建，专门负责全局配置问答
- **独立模型**：每个成员可单独配置大模型（身份 Tab 下拉选择）
- **删除成员**：自动停止 Bot、清理工作区，前端确认弹窗防误操作
- **头像颜色**：每个成员有个性化颜色，图谱/对话均展示

### 对话 & 会话
- **SSE 流式对话**：与任意成员实时对话，支持工具调用（折叠卡展示，含进行中呼吸灯动画）
- **会话持久化**：JSONL 格式存储，含消息历史、Token 估算、Compaction 摘要
- **历史对话侧边栏**：切换会话、继续历史对话，过滤内部 session（skill-studio / subagent）
- **对话管理（ChatsView）**：跨成员查看全部历史对话，工具调用卡片展示，按渠道/成员双筛选
- **@ 其他成员**：对话中转发消息给指定成员，获取跨成员回复

### 工作区 & 知识
- **工作区文件管理**：文件树递归展示（VSCode 风格，Catppuccin Mocha 深色，SVG 矢量图标）、在线编辑器、创建/删除文件、二进制文件检测
- **身份 & 灵魂编辑**：可视化编辑 IDENTITY.md / SOUL.md，失焦自动保存
- **记忆管理**：浏览和编辑 Agent 记忆文件，每日日志，自动整合（Cron 触发）
- **成员环境变量**：AI 成员可通过 `self_set_env` / `self_delete_env` 工具自行持久化私有变量

### 技能系统（SkillStudio）
- 三栏布局：技能列表 | 文件编辑器 | AI 协作聊天
- AI 实时推荐技能方向，自动生成 `skill.json` + `SKILL.md`
- 沙箱隔离：工具操作严格限制在 `skills/{skillId}/` 目录
- 并发后台生成：多技能同时生成不互相阻塞，左侧绿色呼吸点指示
- 技能历史持久化到后端 session

### 团队协作
- **团队图谱（TeamView）**：可拖拽成员节点，拖放创建关系，SVG 精确坐标，4 种关系类型（上下级有向箭头 / 平级协作 / 支持 / 其他），一键整理排列
- **关系管理**：卡片式弹窗选择关系类型，支持「⇄ 翻转」方向，点击连线编辑/删除，双向自动同步
- **后台任务系统（SubagentsView）**：基于关系权限（上级可派遣下级，下级可汇报，平级协作互发），任务卡片显示类型/关系/流向

### 消息渠道
- **Telegram Bot**：每个成员独立配置 Bot，白名单管理，图片/视频/音频/文档/媒体组，群聊/话题线程，主动推送（`/api/agents/:id/notify`）
- **Per-chat 持久会话**：每个 Telegram chat 绑定独立 session，Bot 有完整对话记忆
- **Web 渠道**：独立 URL `/chat/{agentId}/{channelId}`，支持密码保护，Session 隔离
- **热重载**：新增/修改渠道立即生效，Token 唯一性检测

### 任务 & 调度
- **定时任务（Cron）**：可视化配置，每个成员独立任务，执行历史，一键运行
- **技能库**：跨成员汇总展示，按成员筛选，一键复制

### 全局项目系统
- 左侧项目列表 + 右侧文件浏览器 + 代码编辑器三栏布局
- 递归文件树，支持创建/删除文件，标签/描述元信息

### 模型 & 配置
- **多模型支持**：Anthropic / OpenAI / DeepSeek / 自定义 Base URL
- **在线测试**：API Key 验证、模型 Probe，失败原因实时展示
- **全局 Tools**：内置 read / write / edit / exec / grep，可按成员启用/禁用

### CLI 管理面板（类宝塔风格）
直接运行 `zyhive`（无参数）进入交互式管理菜单：

```
┌─ 操作菜单 ─────────────────────────────────────┐
│ [1] 系统状态                                    │
│ [2] 服务管理（启动 / 停止 / 重启）               │
│ [3] 配置管理（访问令牌 / 端口 / 绑定模式）        │
│ [4] 成员管理（查看 / 重置 AI 成员）               │
│ [5] 日志查看                                    │
│ [6] 在线更新（一键升级到最新版）                  │
│ [7] Nginx 管理                                  │
│ [8] SSL 证书管理                                │
│ [9] 备份与恢复                                  │
│ [0] 退出                                       │
└────────────────────────────────────────────────┘
```

> 服务管理完整支持 Linux（systemd）/ macOS（launchd）/ Windows（sc.exe）三平台。

---

## 🖥️ 平台支持

| 平台 | 安装方式 | 服务管理 | 备注 |
|------|----------|----------|------|
| Linux (x86_64) | bash 脚本 | systemd | ✅ 推荐 |
| Linux (arm64) | bash 脚本 | systemd | ✅ 支持 |
| macOS (Apple Silicon) | bash 脚本 | launchd | ✅ 支持 |
| macOS (Intel) | bash 脚本 | launchd | ✅ 支持 |
| Windows (x86_64) | PowerShell 脚本 | sc.exe | ✅ 支持 |
| Windows (arm64) | PowerShell 脚本 | sc.exe | ✅ 支持 |
| Windows Git Bash | bash 脚本（自动转 PS） | — | ✅ 自动适配 |

---

## 🛠️ 技术架构

```
Vue 3 + Element Plus (SPA, go:embed 单二进制)
        ↓ REST API + SSE
Go 后端 (Gin，单二进制)
        │
  pkg/runner    ← Agent 对话主循环（工具调用循环，并行执行）
  pkg/llm       ← Anthropic / OpenAI 流式客户端
  pkg/session   ← JSONL 会话存储（broadcaster fan-out + replay buffer）
  pkg/tools     ← 内置工具（read/write/edit/exec/grep）
  pkg/agent     ← 多成员生命周期 + 工作区管理 + 关系图
  pkg/channel   ← Telegram / Web 渠道（热重载，per-chat session）
  pkg/cron      ← 定时任务引擎
  pkg/memory    ← 记忆整合（自动 Compaction）
  pkg/skill     ← Skills 管理 + Runner 注入
  pkg/project   ← 全局项目系统
  cmd/aipanel   ← 入口 + CLI 管理面板（700+ 行，跨平台服务管理）
```

**架构参考：** [OpenClaw](https://github.com/openclaw/openclaw)

---

## ⚙️ 配置文件

默认位置（一键安装后自动生成）：
- Linux / macOS root：`/etc/zyhive/zyhive.json`
- macOS 用户：`~/.config/zyhive/zyhive.json`
- Windows：`C:\ProgramData\ZyHive\zyhive.json`

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
  }
}
```

| 字段 | 说明 |
|------|------|
| `gateway.port` | HTTP 服务端口（默认 8080） |
| `gateway.bind` | 绑定模式：`localhost` / `lan` / `0.0.0.0` |
| `auth.token` | Bearer Token，用于 API 鉴权 |
| `agents.dir` | 成员数据根目录 |
| `models.primary` | 默认模型（`provider/model` 格式） |

---

## 🔨 开发构建

```bash
# 前端依赖
cd ui && npm install

# 完整构建（必须用 make，不能直接 go build）
make build
# 等价于: vite build + cp ui/dist → cmd/aipanel/ui_dist + go build

# 多平台发布构建（需先构建 UI）
cd ui && npm run build && cd ..
make release

# 启动
./bin/aipanel --config aipanel.json
```

> ⚠️ 直接 `go build` 会缺少 UI 静态文件（go:embed），**必须用 `make build`**

---

## 📋 版本里程碑

| 版本 | 内容 | 状态 |
|------|------|------|
| v0.1–0.4 | 项目骨架、LLM 客户端、Session 存储、Tools、Runner、Vue 3 UI | ✅ |
| v0.5 | Auth、Stats、安装脚本、多 Agent 协同 | ✅ |
| v0.6 | 记忆模块、团队关系图谱、Telegram 完整能力 | ✅ |
| v0.7 | 消息渠道下沉成员级别、per-agent 独立 Bot | ✅ |
| v0.8 | SkillStudio 技能工作室、Web 多渠道隔离、历史对话系统 | ✅ |
| v0.9.0 | 团队图谱交互、全局项目系统、成员管理增强 | ✅ |
| v0.9.1 | 后台任务系统、移动端响应式、Telegram 持久会话、成员 Env 自管理 | ✅ |
| v0.9.8 | install.zyling.ai CF 加速节点正式上线 | ✅ |
| v0.9.9 | 安装脚本自动获取 root 权限（sudo 密码提示）；CLI macOS 服务状态修复 | ✅ |
| v0.9.10 | **Windows 完整支持**（PowerShell 安装、UAC 自动提权、sc.exe 服务管理） | ✅ |
| v0.9.11 | **通用安装端点**（`/install` 按 UA 自动分流；Git Bash 自动转 PowerShell） | ✅ |
| **v0.10** | 团队规划系统、会议系统 | 🔜 规划中 |

---

## 📄 License

引巢 · ZyHive 采用 **GNU Affero General Public License v3.0（AGPL-3.0）** 开源协议。

- ✅ 个人使用、学习、研究 — 完全免费
- ✅ 自托管私用 — 完全免费
- ✅ 修改和二次开发 — 必须以相同协议开源
- ⚠️ 基于本项目构建网络服务对外提供 — 必须开源全部改动
- 🚫 商业闭源集成或托管销售 — 需要商业授权

**zyling（智引领科技）** — 商业授权联系方式见 [zyling.ai](https://zyling.ai)
