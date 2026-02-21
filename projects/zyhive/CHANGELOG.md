# Changelog — 引巢 · ZyHive

> 所有重要版本变更记录。版本号遵循 [Semantic Versioning](https://semver.org/)。

---

## [v0.8.0] — 2026-02-20 · SkillStudio 技能工作室

### 新增
#### SkillStudio — 三栏技能工作室
- 专业三栏布局：技能列表 | 文件编辑器 | AI 协作聊天
- 点 "+" 直接创建空白技能，无弹窗，右侧 AI 实时推荐技能方向（`sendSilent` 后台触发）
- 动态文件树：递归展示技能目录，支持打开/编辑/删除 AI 生成的任意文件（含子目录）
- **AI 沙箱**：工具操作严格限制在 `skills/{skillId}/` 目录，禁用 `self_install_skill` 等危险工具
- **并发后台生成**：每个 skill 独立 AiChat 实例（v-show），切换不打断任何流；左侧绿色呼吸点指示后台生成
- 技能对话历史持久化到后端 session（`skill-studio-{skillId}`）；首次选中自动加载
- AI 创建技能时同时写 `skill.json`（名称/分类/描述）和 `SKILL.md`（提示词）
- `chatContext` 注入当前 `skill.json` 模板、路径规则、已有 SKILL.md 内容

#### Telegram 完整能力
- 图片 / 视频 / 音频 / 文档 / 贴纸 / 媒体组 接收解析
- 群聊 / 话题线程 / 内联键盘 callback / Reactions / HTML 流式输出
- 转发消息 / 回复消息上下文注入（`forward_origin` / `ReplyToMessage`）
- 图片传给 Anthropic 全链路修复（Content-Type 标准化、ReplyToMessage.Photo 下载）

#### Skill 系统
- `skill.json` 元数据 + `SKILL.md` 提示词双文件格式
- Runner 启动时自动注入所有 enabled 技能到 system prompt
- 自管理工具：`self_install_skill` / `self_uninstall_skill` / `self_list_skills`
- AgentDetailView 技能 Tab：启用/禁用切换，Tab 切换自动刷新

#### 历史对话系统
- 永久对话日志 `convlogs/`，按渠道隔离（`telegram-{chatId}.jsonl` / `web-{channelId}.jsonl`）
- 管理员 ChatsView 可查看全部历史；Agent 侧历史与 session 完全隔离

#### Web 渠道多渠道隔离
- 每个 Web 渠道独立 URL `/chat/{agentId}/{channelId}`、独立 Session、独立 ConvLog
- `sessionToken` 通过 `localStorage` 跨刷新持久化，per-visitor session 历史压缩
- 添加/编辑弹窗实时展示访问链接，支持密码保护

#### 渠道管理
- BotPool 热重载：新增渠道立即生效，Token 更改后自动同步
- Bot Token 唯一性检测（防止 409 冲突）
- Dialog 内 Token 自动验证 + 内联反馈（800ms 防抖）
- 白名单用户管理：移除按钮、待审核列表、审核通过发送欢迎消息
- 渠道卡片展示 Telegram @botname

### 变更
- **全 UI 去 emoji**：App logo 改为蓝色六边形 SVG，所有图标统一用 Element Plus icons
- 全页面统一版权 footer（侧边栏 / 登录页 / 公开聊天页）
- 对话管理双 Tab（按渠道 / 按成员）+ 双筛选
- 定时任务按成员隔离（`Job.agentId` 字段，`ListJobsByAgent` 过滤）

### 修复
- SkillStudio：切换技能时右侧 AI 聊天窗口正确重置
- SkillStudio：选中技能时预加载 SKILL.md，AI 上下文不再为空
- SkillStudio：AI 上下文中明确路径规则，防止 AI 写入错误目录
- 团队图谱布局每次刷新结果一致（去随机化）
- 白名单留空改为配对模式（而非接受所有人）
- 三项修复：pending 渠道删除清理 / web 密码 sessionStorage / TG 媒体消息记录

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
