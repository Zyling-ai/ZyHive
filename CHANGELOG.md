# Changelog — 引巢 · ZyHive

> 所有重要版本变更记录。版本号遵循 [Semantic Versioning](https://semver.org/)。

---

## [v0.7.0] — 2026-02-19 · 消息通道下沉至成员级别

### 新增
- 每个 AI 成员独立配置自己的消息通道（Telegram Bot Token 等）
- `GET/PUT /api/agents/:id/channels` 成员级渠道管理 API
- `POST /api/agents/:id/channels/:chId/test` Telegram Bot Token 验证（调用 getMe）
- `AgentDetailView` 新增「渠道」Tab，支持增删改测试

### 变更
- 全局导航删除「消息通道」菜单项（全局通道注册表已废弃）
- `main.go` 启动逻辑改为按成员遍历 channels 起 TelegramBot

---

## [v0.6.0] — 2026-02-19 · 记忆模块 + 关系图谱完善

### 新增
- 记忆模块完整重构：`pkg/memory/config.go` + `consolidator.go`
  - 自动对话摘要（LLM 提炼）+ 会话裁剪（`TrimToLastN`）
  - `memory-run-log.jsonl` 日志，`GET /api/agents/:id/memory/run-log` API
- 定时任务备注字段（`Remark`）+ 全局 CronView 记忆任务只读展示
- 关系 Tab 改为可视化交互（下拉选择框，替代手动 markdown 输入）
- 团队图谱连线修复（箭头方向、线宽、双向去重）
- 关系双向自动同步（A→B 建立时，B 的 RELATIONS.md 自动补充反向关系）

### 修复
- 关系刷新丢失 Bug（序列化改为标准 markdown 表格格式）
- 整理日志无记录问题（`ConsolidateNow` 不再绕过 cron engine）
- 创建成员时默认开启记忆（daily + keepTurns=3）

---

## [v0.5.0] — 2026-02-19 · Phase 6 团队关系图谱 + Phase 5 收尾

### 新增
- 团队关系图谱页（`TeamView.vue`，纯 SVG 圆形布局，颜色/线粗反映关系类型/程度）
- RELATIONS.md 关系文档 + `GET /api/team/graph` 双向去重接口
- Stats 端点实现（按 Agent 汇总 token/消息/会话）
- DashboardView 接入真实统计数据 + 成员排行榜
- LogsView 实时日志（5秒刷新，关键词过滤，颜色染色）
- ChatsView「继续对话」按钮跳转 + AgentDetailView 自动 resume session
- 安装脚本（`scripts/install.sh`，289行，多架构 amd64/arm64，Linux systemd，macOS launchd）
- 多 Agent @成员转发协同基础版

### 修复
- App.vue 重复菜单项修复（`/chats`、`/config/models` 等各出现两次）
- Skills 注入 system_prompt 修复（loader 之前未调用）
- AiChat 有 sessionId 时停发 history[]（避免重复上下文）

---

## [v0.4.0] — 2026-02-18 · Phase 4 + 品牌命名

### 新增
- 项目正式命名：**引巢 · ZyHive**（zyling AI 团队操作系统）
- 核心概念更名：员工→**成员**，AI公司→**AI团队**
- 历史对话实时加载（Gemini 风格，点击侧边栏会话即刻渲染）
- 对话管理页（ChatsView）：跨 Agent 会话列表、详情抽屉、删除/重命名
- 新建向导（AgentCreateView）左右双栏：左侧表单 + 右侧 AI 辅助生成

---

## [v0.3.0] — 2026-02-18 · Phase 3 Telegram + Cron + 多 Agent

### 新增
- Telegram Bot 长轮询接入（`pkg/channel/telegram.go`）
- 真实 Cron 引擎（`pkg/cron/engine.go`），支持 cron 表达式、一次性任务
- 会话压缩（Compaction）：超过 80k token 自动 LLM 摘要压缩
- 多 Agent 并发池（`pkg/agent/pool.go`）
- 上下文注入：IDENTITY.md、SOUL.md、MEMORY.md 自动注入 system prompt

---

## [v0.2.0] — 2026-02-18 · Phase 2 Vue 3 UI

### 新增
- 完整 Vue 3 + Element Plus 前端
- 仪表盘、AI 成员管理、对话（SSE 流式）、身份编辑器、工作区文件管理、定时任务
- 单二进制嵌入 UI（`embed.FS`）

---

## [v0.1.0] — 2026-02-18 · Phase 0-1 核心引擎

### 新增
- Go 项目骨架（15个模块目录结构）
- LLM 客户端（Anthropic Claude，SSE 流式）
- Session 存储（JSONL v3 格式，sessions.json 索引）
- Agent 管理器（多 Agent 目录结构，config.json）
- Chat SSE API（`POST /api/agents/:id/chat`）
- 全局配置（模型、工具、Skills 注册表）
