# Changelog — 引巢 · ZyHive

> 所有重要版本变更记录。版本号遵循 [Semantic Versioning](https://semver.org/)。

---

## [v0.9.15] — 2026-02-25 · 修复升级后 Anthropic 403 导致所有功能报错

### 修复

- **Anthropic 403 地区限制中文提示**：`testAnthropicKey` 支持自定义 baseURL，403 错误返回明确提示「当前 IP 被 Anthropic 屏蔽，请配置转发地址或切换模型」
- **模型测试覆盖国产模型**：新增 `testOpenAICompatKey` 通用函数，Kimi / GLM / MiniMax / 通义千问 等均可测试连通性
- **仪表盘警告横幅**：检测到默认模型 `status = error` 时，顶部展示红色横幅并提供「去设置」快捷入口
- **测试成功自动引导切换默认**：测试某模型连通成功后，若当前默认模型为 error 状态，弹窗询问是否将其设为默认模型

### 场景

> 用户从旧版升级，Anthropic 为默认模型，国内 IP 被封导致所有功能 403。添加 DeepSeek 后，测试 DeepSeek 连通性，系统自动弹出「是否设为默认？」，一键切换后所有功能恢复正常。

---

## [v0.9.14] — 2026-02-25 · 多模型支持 + 安装命令升级检测

### 新增

#### 多 Provider 模型支持
- 新增 Kimi（月之暗面）、智谱 GLM、MiniMax、通义千问 四大国产模型
- ModelsView 重构为提供商卡片网格 + API Key 引导，告别纯表单输入
- LLM 客户端按 provider 独立拆分：`anthropic.go / openai.go / deepseek.go / moonshot.go / zhipu.go / minimax.go / qwen.go / openrouter.go / custom.go`
- 工厂函数 `NewClient(provider, baseURL)` 统一路由，新增 provider 只需加文件

#### 一键安装命令升级检测
- 执行安装命令时自动检测是否已安装
- 已安装且为最新版本：显示"已是最新版本"并退出
- 发现新版本：提示 `是否更新 vX → vY？[Y/n]`，确认后自动停服务 → 下载 → 替换 → 重启
- 支持 Linux/macOS（bash）和 Windows（PowerShell）双脚本

#### Web 面板在线升级
- 设置页新增版本检查卡片，支持一键升级（进度条 + 自动回滚）
- 新建 `update.go / update_unix.go / update_windows.go`，跨平台 SIGTERM/os.Exit 重启

#### CLI 在线更新
- `--version` flag 显示当前版本
- 更新前版本对比；已是最新版本时提示；备份 `.bak`；下载失败自动回滚

### 修复

- **Web UI DeepSeek 401**：`chat.go / public_chat.go` execRunner() 修复硬编码 `NewAnthropicClient`，改为 `llm.NewClient(provider, baseURL)`
- **配置文件路径双轨**：`configFilePath` 从硬编码 `"aipanel.json"` 改为 var，`RegisterRoutes(cfgPath)` 传入，UI 写入与服务读取始终同一文件
- **配置助手跟随默认模型**：`__config__` 系统 agent 直接取当前默认模型，不再固化 Anthropic

### 变更

- 未配置模型时仪表盘顶部橙色 banner 引导 + AiChat 空态提示
- 版本号通过 ldflags `-X main.Version=$(VERSION)` 注入，`git describe --tags` 自动计算

---

## [v0.9.12] — 2026-02-23 · 三级记忆系统

### 新增

#### 对话历史实时索引（`pkg/chatlog`）
- 新包 `pkg/chatlog`：并发安全的 AI 可见对话历史管理器
- 每条 user/assistant 消息实时写入 `workspace/conversations/{sessionId}__{channelId}.jsonl`
- 自动维护 `workspace/conversations/index.json`（原子写入，mutex 保证并发安全）
- 自动生成 `workspace/conversations/INDEX.md`（最近20条，注入 system prompt）
- 支持按 session_id / channel_id 双维度筛选读取
- Compaction 完成后自动调用 `UpdateSummary()`，给对应会话写入 AI 生成摘要
- 接入点：Web chat（`internal/api/chat.go`）、Telegram（`pkg/channel/telegram.go` / `telegram_api.go`）

#### 技能索引（`pkg/skill/index.go`）
- `RebuildIndex(workspaceDir)` 扫描已安装技能，生成 `workspace/skills/INDEX.md`
- 技能安装/卸载后自动重建（`self_install_skill` / `self_uninstall_skill` 工具触发）
- INDEX.md 格式：名称 + 分类 + 描述 + 状态（启用/禁用）

### 变更

#### System Prompt 瘦身
- **移除**：全量注入所有已启用技能 `SKILL.md` 内容（context 臃肿）
- **改为**：注入轻量 `skills/INDEX.md`（只有名字+描述）
- **新增**：注入 `conversations/INDEX.md`（历史对话摘要索引）
- **新增**：提示 AI 可用 `read` 工具访问完整记忆和历史对话

#### 三级确认机制
AI 拿不准时可三步走：
1. **Level 1**：当前 session 上下文（自动在 prompt 里）
2. **Level 2**：`read memory/INDEX.md` → 具体记忆文件（记忆层）
3. **Level 3**：`read conversations/INDEX.md` → 具体对话 JSONL（历史对话层）

---

## [v0.9.11] — 2026-02-23 · 通用安装端点（全平台一条命令）

### 新增
- **`/install` 通用端点**：Cloudflare Worker 根据请求 User-Agent 自动分流
  - `User-Agent` 含 `PowerShell` → 返回 `install.ps1`
  - 其他（curl 等） → 返回 `install.sh`
- **Git Bash / MSYS2 / Cygwin 自动适配**：`install.sh` 开头检测 `uname -s`（`MINGW*` / `MSYS*` / `CYGWIN*`），自动调用系统 `powershell.exe` 或 `pwsh` 完成安装
- `/install.sh` 和 `/install.ps1` 作为类型固定的别名端点

### 统一安装命令
```bash
# Windows (PowerShell)
irm https://install.zyling.ai/install | iex

# macOS / Linux / Windows Git Bash（完全相同）
curl -sSL https://install.zyling.ai/install | bash
```

---

## [v0.9.10] — 2026-02-23 · Windows 完整支持

### 新增
- **`scripts/install.ps1`** — Windows PowerShell 安装脚本
  - 检测到非管理员 → 自动 `Start-Process powershell -Verb RunAs` 弹出 UAC 提权
  - 管道运行（`irm | iex`）时 → 先下载到临时文件，再以管理员身份重新执行
  - 二进制安装到 `C:\Program Files\ZyHive\zyhive.exe`
  - `sc create zyhive` 注册 Windows 服务（自动启动 + 故障三次递增重试）
  - 将安装目录加入系统 PATH（`Machine` 级别，对所有用户生效）
  - 支持 `-Uninstall` 卸载、`-NoService` 只安装二进制
- **CLI Windows 服务管理（`sc.exe`）**
  - `isServiceRunning()` → `sc query zyhive` 检查 "RUNNING" 字段
  - `systemctlAction()` 在 Windows 上路由到 `scAction()`
  - `scAction()`：start / stop / restart / enable（`start= auto`） / disable（`start= demand`） / status
  - `svcStop()` / `svcStart()` 跨平台 helper（Linux/macOS/Windows 各走对应命令）
- **Makefile** 新增 Windows 编译目标
  ```makefile
  GOOS=windows GOARCH=amd64 go build -o bin/release/aipanel-windows-amd64.exe
  GOOS=windows GOARCH=arm64 go build -o bin/release/aipanel-windows-arm64.exe
  ```
- **CF Worker** 新增 `/zyhive.ps1` 端点（代理 GitHub raw `scripts/install.ps1`）

### Release 产物（v0.9.10+）
| 文件 | 平台 |
|------|------|
| `aipanel-linux-amd64` | Linux x86_64 |
| `aipanel-linux-arm64` | Linux ARM64 |
| `aipanel-darwin-arm64` | macOS Apple Silicon |
| `aipanel-darwin-amd64` | macOS Intel |
| `aipanel-windows-amd64.exe` | Windows x86_64 |
| `aipanel-windows-arm64.exe` | Windows ARM64 |

---

## [v0.9.9] — 2026-02-23 · 安装脚本自动获取 root 权限

### 修复 / 改进
- **`install.sh` 权限逻辑重写**
  - 旧行为：`sudo -n true`（非交互），无密码 sudo 则静默降级到用户目录
  - 新行为：非 root 时调用 `sudo -v`（**弹出密码提示**），获取后统一安装到系统目录
  - sudo 保活：后台每 60 秒执行 `sudo -v` 刷新票据，防止长下载超时失效
  - 支持 `--no-root` 参数强制跳过，安装到用户目录（`~/.local/bin`）
- **CLI macOS 服务状态检测修复**
  - 旧：`systemctl is-active zyhive` → macOS 无 systemctl → 永远返回"已停止"
  - 新：`isServiceRunning()` switch 判断平台，macOS 用 `launchctl list com.zyhive.zyhive`，检查输出是否含 `"PID"` 字段
- **CLI macOS 服务管理完整支持**
  - 新增 `launchctlAction()`，覆盖 start / stop / restart / enable（load -w） / disable（unload -w）
  - LaunchDaemon（root 安装）/ LaunchAgent（用户安装）自动区分
  - `svcStop()` / `svcStart()` helper 替换所有硬编码 `systemctl stop/start`（在线更新、备份恢复均受益）

---

## [v0.9.8] — 2026-02-23 · install.zyling.ai CF 加速节点

### 新增
- **CF Workers 部署**（`zyling-website` repo，`_worker.js`）
  - `GET /zyhive.sh` — 实时代理 GitHub raw，永不缓存
  - `GET /zyhive.ps1` — PowerShell 脚本（v0.9.10 起）
  - `GET /latest` — GitHub release redirect 提取版本号（5 分钟缓存，不走 GitHub API 避免限流）
  - `GET /dl/{ver}/{file}` — 二进制下载代理（绕过 GitHub CDN 国内访问问题，24 小时缓存）
- **自定义域名**：`zyling.ai`、`www.zyling.ai`、`install.zyling.ai` 三域均绑定同一 Worker
- **`install.sh` 双回退逻辑**：版本查询和二进制下载均优先走 CF 镜像，失败自动回退 GitHub
- **GitHub Actions 自动部署**：`zyling-website` push → `wrangler deploy`（`CLOUDFLARE_API_TOKEN` / `CLOUDFLARE_ACCOUNT_ID` secrets）

---

## [v0.9.1] — 2026-02-22 · 移动端响应式 + 关系任务系统 + Telegram 持久会话

### 新增

#### 后台任务系统 — 关系权限驱动（SubagentsView 全新重写）
- **关系权限模型**：上级可向下级「派遣任务」，下级可向上级「汇报」，平级协作双向互发，支持/其他关系无权操作
- 新增 `pkg/agent/relations.go`：跨成员读取 RELATIONS.md，构建关系图，提供 `EligibleTargets()` / `CanSpawn()` 方法
- `GET /api/tasks/eligible?from=&mode=task|report`：返回可操作目标列表 + 关系类型
- Spawn API 权限校验：无权操作返回 403 + 具体中文错误（如"引引 没有权限向 小流 派遣任务"）
- Task 结构新增 `taskType`（task/report/system）和 `relation`（记录关系快照）
- 任务卡片：派遣/汇报/系统 badge + 关系类型 badge + 发起→执行流向箭头（含成员头像色）
- 筛选栏支持按任务类型过滤

#### Telegram 持久会话 + 主动推送
- Telegram 每个 chat 绑定持久 session（`telegram-{chatID}`），bot 有完整对话记忆
- `TelegramBot.Notify()` 方法：在指定 chat 的 session 中主动发消息，同时写入 convlog
- `POST /api/agents/:id/notify`：触发主动推送，cron/事件均可调用
- `BotPool.GetBot()` / `GetFirstBot()`：API 层获取运行中的 bot 实例

#### 移动端响应式（全面适配 ≤768px）
- **App.vue**：汉堡菜单 + 侧边栏 overlay 抽屉（点遮罩/菜单项自动关闭）
- Header 链接按屏宽分级隐藏（≤768px 隐藏 GitHub，≤480px 隐藏官网）
- **AgentsView**：卡片 1→2→3 列响应式，名字/ID 单行截断
- **AgentDetailView**：Tab 导航横向滑动（`el-tabs__nav-scroll` 强制 overflow-x），历史会话折叠抽屉，渠道按钮折行，环境变量输入纵向堆叠，表格横向滚动
- **ProjectsView**：文件树 + 编辑器纵向堆叠，项目列表固定顶部，加返回按钮
- **TeamView**：连接横幅正常折行，图谱横向滚动
- **DashboardView**：统计卡片 2×2 网格
- **AiChat**：发送按钮 48px 触控区，字号 15px，iOS 安全区兼容

#### 全局 Header 升级
- 官网按钮（zyling.ai，紫色风格）
- GitHub 链接更新为 `Zyling-ai/zyhive`
- Star 数量实时获取（GitHub API，10 分钟本地缓存），改为纯展示不可点击

#### 成员 Env 自管理工具
- `self_set_env` / `self_delete_env`：AI 成员可自行持久化更新私有环境变量
- `manager.SetAgentEnvVar()` 经由 manager 持久化（内存+磁盘），当前 session 立即生效
- UI 作用域说明：ToolsView 标注「全局共享」，AgentDetail env tab 标注「仅此成员可见」

#### 其他
- `send_file` 工具：Telegram ≤50MB multipart 上传，>50MB 返回下载链接；Web 端图片预览/文件卡片渲染
- `show_image` 工具：成员可在对话中展示截图/图片
- Web channel 历史持久化、background generation 支持、deleted 状态
- README 动态 Stars/Forks badge

### 修复
- stale broadcaster replay 导致新消息回复旧内容（StartGen 清空 buffer）
- processToolResult 统一 marker 检测（历史加载 + streaming 5 处全覆盖）
- session 历史侧边栏过滤内部 session（skill-studio-* / subagent-*）
- AgentCreate apply card 每次只保留最新一张
- skill-studio sandbox bash 工具开放

---

## [v0.9.0] — 2026-02-21 · 团队图谱 + 项目系统 + 成员管理增强

### 新增

#### 团队图谱交互（TeamView）
- 可拖拽节点：SVG 精确坐标（`getScreenCTM().inverse()`），拖拽完全跟手，左/上边界限制，右/下无限扩展
- 拖放创建关系：从一个节点拖到另一个节点，弹窗选择关系类型
- 点击连线打开编辑弹窗：修改关系类型/强度/描述，支持删除
- 「整理」按钮：自动层级排列，循环检测防止无限拉伸
- 关系类型合并为 4 种：**上下级**（有方向箭头，紫色）/ 平级协作 / 支持 / 其他
- 关系弹窗：卡片式 2×2 类型选择（RelTypeForm 组件），代入真实成员名展示含义
- 上下级关系支持「⇄ 翻转」按钮，可直接交换 from/to 方向
- 节点使用成员头像色（`avatarColor`），点击节点可直接编辑颜色

#### 全局项目系统（ProjectsView）
- 左侧项目列表 + 右侧文件浏览器三栏布局
- 文件树递归展示，文件/目录图标区分
- 代码编辑器：语法高亮预览、保存、创建/删除文件
- 项目支持标签、描述，增删改查完整闭环

#### 成员管理增强
- **支持删除成员**：停止 Telegram Bot，删除工作区，前端确认弹窗
- **系统配置助手 `__config__`**：内置成员，不可删除，启动时自动创建；API/Manager 双重拦截
- **换模型**：身份 & 灵魂 Tab 新增「基本设置」卡片，下拉选择模型并保存（`PATCH /api/agents/:id`）
- **工作区文件管理增强**：创建任意文件/目录、删除、二进制文件检测、空文件 placeholder
- **消息通道 per-agent 独立配置**：AgentCreateView 不再使用全局 channelIds，改为内联 Bot 表单

#### UI 整体升级
- 仪表盘极简卡片（去彩色图标框）、统计数据真实化
- 顶部 Header：GitHub 链接、Star 按钮、退出登录
- 登录页：必填校验 + 数学验证码，版权年 → 2026
- 技能库顶级菜单：跨成员汇总、按成员筛选、一键复制技能到其他成员

### 修复
- 图谱：SVG 坐标转换改用 `getScreenCTM().inverse()`，彻底修复拖拽/连线偏差
- 图谱：拖拽后不误触发连线（`lastDragId` ref 跨 mouseup/click 事件传递）
- 图谱：双向关系删除彻底清理（`removeInverseRelation`），一键清空全部关系
- 图谱：翻转保存前先删旧边，`computeLevels` 加循环检测（`maxLevel = nodes.length + 1`）
- 图谱：无关系时仍显示全部成员节点，底部加引导提示
- 工作区文件树：递归展示子目录（`?tree=true` 嵌套 `FileNode[]`）
- Write handler：同时支持 JSON `{content}` 和 raw text 双模式
- AgentCreateView：配置助手无成员时不传错误 `agentId`
- JSON 提取：括号平衡计数重写 `extractBalancedJson`，修复多代码块/特殊字符场景
- 登录页验证码：题目和输入框合为同一行
- 项目编辑器：右侧 `el-textarea` 高度填满容器（`:deep()` 穿透 Element Plus 内部样式）

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
