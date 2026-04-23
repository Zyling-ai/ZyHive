# Changelog — 引巢 · ZyHive

> 版本号规则：`年.月.日vn`，n 为当天第 n 个版本（如 `26.3.17v1` 为当天首版，`26.3.17v2` 为当天第二版）

---

## [26.4.23v6] — 2026-04-23 · AI 感知当前会话标题 + `session_rename` 工具

用户反馈：问 AI "这个 session 你能不能配置标题"，AI 回复"我没有直接工具"并写进 wishlist。实际上需要的是把**标题感知 + 编辑能力**都给 AI。

### 🎯 改造

**1. AI 能看到当前标题**

新字段 `runner.Config.CurrentSessionContext`，`system_prompt` 在 capabilities 之后注入：
```
## 当前会话
- 当前标题: xxx
- Session ID: ses-xxx
- 消息数: 26
- 创建于: 2026-04-23 07:26
- 标题状态: 自动生成, 可在必要时优化

💡 当前标题信息量不足, 若主题已清晰, 可调用 `session_rename` 设置 8-20 字新标题.
```

最后那行**仅在 title 被判定为 weak 时才注入**（titleLooksWeak：空 / < 6 chars / 以"你好/请问/Hello/OK"等开头），避免正常 title 时浪费 token。

**2. 新工具 `session_rename(title)`**

- AI 只能传 `title`，**sessionID 从 registry 拿**（`WithSessionID` 已有），防止 AI 改错 session
- 后端调 `session.Store.UpdateTitle`（自动设 `TitleOverridden=true`）
- 与用户手动 PATCH rename 等价：后续 `MaybeAutoRetitle` 不再覆盖
- 30 字符 rune-safe 截断
- Guardrails：
  - 相同 title → 返回"未变"，不写磁盘
  - 空 title → 报错
  - 空 sessionID（cron 等 ephemeral 场景）→ 报错"unavailable"

**3. 接入点**
- `pkg/tools/sessions_tool.go`：新 `SessionTitleWriter` 接口 + `SessionStoreAdapter` 实现 `UpdateTitle` / `GetMeta`
- `pkg/agent/pool.go::poolSessionAdapter`：跨所有 agent 查 session 对应的 Store，实现同接口
- `WithSessionTools(lister, reader, sender, titler)` 签名扩充 → pool.go 1 处改动
- `chat.go` + `pool.go` 4 个 runner.New 处统一塞 `CurrentSessionContext: BuildSessionContext(store, sessionID)`
- 公共 helper `pkg/agent/session_context.go::BuildSessionContext` 避免代码重复

### ✅ 测试

- `TestTitleLooksWeak`（weak/strong 识别 10 case）
- `TestSessionRename_Basic`（成功 + TitleOverridden=true）
- `TestSessionRename_NoOp`（相同 title 不写）
- `TestSessionRename_EmptyTitle`（拒绝空）
- `TestSessionRename_NoSession`（ephemeral 场景拒绝）
- `TestSessionRename_TruncatesLongTitle`（30 rune 截断）

全绿 + `go build ./...` ✅

### 预期效果

- 截图里 AI 说"我没有工具"的情况不再发生：它会看到自己的 tool list 有 `session_rename`，会看到当前 title，会判断是否改
- 现有 session 当 AI 被问"改下标题" → AI 直接 `session_rename("xxx")` → UI 侧边栏下次刷新即可看到新标题
- 对话开头如果 title 是"你好/OK"这类 weak 前缀 → prompt 提示 AI 主动更新

### 兼容性
- `WithSessionTools` 签名变化（加 titler），内部唯一调用方 pool.go 已同步
- runner.Config 新字段，老调用代码默认值 "" 不影响行为

---

## [26.4.23v5] — 2026-04-23 · 顶栏一键升级 + 全局进度横幅

用户反馈：网页 UI 顶部只显示「新版本 XXX」标签但点了要跳到设置页再点一次才能升级，希望**一键升级 + 进度在顶部实时显示**。

### 🔧 改造

**新 composable**：`ui/src/composables/useUpdater.ts`
- module-singleton 状态（不随组件销毁），跨路由不丢进度
- 封装 `initFromBackend` / `checkForUpdate` / `startUpgrade`
- 复用 500ms polling + 90s waitForRestart 兜底（移自 SettingsView）

**顶栏按钮（App.vue）**
- 「升级到 XXX」绿色按钮 → 直接弹确认 → 启动升级（不再跳 settings）
- 升级进行中按钮切为蓝色「下载中 45%」/「验证中」/「替换文件」
- 点击运行中按钮 → 跳 settings 看详细日志

**全局进度横幅**
- 顶栏下方出现整条 banner：状态文本 + 流动进度条 + 百分比
- 4 种状态配色：运行（蓝）/ 成功（绿）/ 失败（红）/ 回滚（黄）
- 升级成功 → banner 变绿 + 显眼「刷新页面」按钮
- 失败/回滚 → 「查看详情」按钮跳 settings
- 横幅在所有路由都可见（在 `<el-header>` 下、`<el-container class="app-body">` 前）

**SettingsView 重构**
- 去除本地 polling/waitForRestart 复制代码（~120 行）
- 改用 composable 的 state refs，state 在两处 UI 自动同步
- 去掉 onBeforeUnmount stopPolling（全局状态不该被单组件销毁打断）

### ✅ 效果

- 顶栏点一下 → 弹确认 → 立刻开始升级 → 横幅实时跟进度
- 切到任何路由（Dashboard / Goals / Chats）横幅都在顶
- 切到 Settings 页也看到同一份进度（不会两处 polling 打架）
- 刷新页面自动接管进行中的任务

### 验证
- `vue-tsc -b && vite build` ✅
- 所有 .vue 代码复用同一 composable 实例，进度状态一致

### 兼容性
- 零破坏：后端 `/api/update/*` 完全不变
- SettingsView 交互路径和以前一样，只是实现换了

---

## [26.4.23v4] — 2026-04-23 · 飞书图片消息接入视觉模型

用户反馈：在飞书渠道发图片给 AI，AI 收不到。

### 🐛 根因

`pkg/channel/feishu.go::handleMessageEvent` 第 335 行：
```go
if msg.MessageType != "text" {
    return
}
```

任何非文本类型的消息（image / post / audio / file / sticker）直接被丢弃。
Telegram 渠道早就支持 photo / document 等，飞书一直没跟上。

### 🔧 修复

**接纳三类消息**：`text` / `image` / `post`（富文本，可能含内嵌图）

**内容解析**（`handleMessageEvent`）：
- `image` → 解析 `{"image_key":"img_v3_..."}`，文本兜底 `[图片]`
- `post` → 解析 2D 内容数组，抽取所有 `tag="text"/"a"/"md"` 的文字 + 所有 `tag="img"` 的 image_key
- `text` → 保持不变

**下载图片**（新 helper `downloadMessageResource`）：
- `GET /im/v1/messages/:message_id/resources/:file_key?type=image`
- 使用 `app_access_token` 鉴权（复用现有 refreshToken）
- 10MB 上限保护（超过直接跳过该图）
- `sniffImageContentType` 按 magic bytes 识别 JPEG/PNG/GIF/WebP，默认 jpeg

**喂给模型**（`streamFunc` media 参数）：
- 包装 `MediaInput{FileName, ContentType, Data}` 数组
- 单条消息最多 5 张图（超过截断 + 记日志），保护 token 预算和 vision 模型限制
- pool 层 `normalizeVisionContentType` 已支持 image/jpeg|png|gif|webp

### ✅ 验证

- `go build ./... && go test ./pkg/channel/...` 全绿
- 新增测试：
  - `TestSniffImageContentType`（6 子 case：4 种 magic + 未知兜底 + 空兜底）
  - `TestExtFromContentType`（6 case）

### 效果

飞书用户直接发图/富文本带图给绑定视觉模型的 agent：
- 图片被下载并作为 base64 data URI 传给 LLM
- AI 能 "看到" 图片内容
- 多张图自动截到 5 张以内
- 非视觉模型（`supportsTools=false` 或 vision=false）：pool 层的 `normalizeVisionContentType` 会 skip，记日志 + 继续走文本流

### 兼容性

- 零破坏：文本消息路径完全不变
- 非 image/post/text 的其他类型（audio/file/sticker）继续 ignore，和之前行为一致

---

## [26.4.23v3] — 2026-04-23 · Session 自动主题命名 + UsageView 明细增强

用户反馈：
> 对话管理的标题应该要根据对话内容实时变化，每个 session 标题应该能直接反映
> token 调用明细里面，要有 session 标题、成员名字，细化这个页面，同时把筛选也做出来

### 🏷️ Session 自动主题命名（取代首字节摘要）

**问题**：旧实现 `extractTitle` 只取第一条 user message 的前 60 字符，导致"你好"/"请问"/"开始对话吧"这种寒暄开场 = 永久 title，对话管理里 37 个 session 有 20 个是这种无信息 title。

**新实现**（`pkg/session/retitle.go`）：
- `MaybeAutoRetitle(store, sessionID, summarizer)` — fire-and-forget 后台调用，不阻塞主对话
- 里程碑触发：消息数 crosses 4 / 12 / 30 / 80 时各触发一次
- 使用会话自己绑定的模型（`Runner.makeSimpleLLMCaller` 复用）
- Prompt 要求：8-20 字简洁中文标题，无"标题:"/引号等前缀
- 失败静默（`log.Printf` 记录，不影响对话）
- `TitleOverridden` 字段：用户手动 `PATCH /sessions/:id` rename 后永久尊重，auto-retitle 不再覆盖
- `TitledAtMsgCount` 字段：记录上次 auto retitle 时的 MessageCount，避免重复总结

**`pkg/session/types.go`**
- `SessionIndexEntry` 新增 `TitleOverridden bool` + `TitledAtMsgCount int`

**`pkg/session/store.go`**
- `UpdateTitle` → 人工 rename，设 `TitleOverridden=true`
- `UpdateAutoTitle` → 新方法，`TitleOverridden=true` 时跳过；自动写 `TitledAtMsgCount`
- `NeedsAutoRetitle` → 检查是否越过里程碑且未被人工覆盖
- `autoRetitleThresholds = []int{4, 12, 30, 80}`

**`pkg/runner/runner.go`**
- `done` event 发出之后追加 `session.MaybeAutoRetitle(...)`

**测试**
- `TestSanitizeTitle`（7 case 清洗：引号/「」/标题:/markdown 粗体/多行/超长截断）
- `TestBuildRetitleInput`（保序 + 最新优先）
- `TestBuildRetitleInput_TruncatesToLatest`（超长首条取 rune-safe 尾部）
- `TestNeedsAutoRetitle_Milestones`（4→触发→标记→12 才再触发）
- `TestMaybeAutoRetitle_RunsSummarizer`（summarizer 被调 + title 异步写入）

### 📊 UsageView 明细 + 筛选增强

**问题**：截图里 token 调用明细只有 `agent_id` / `provider` / `model` 等技术字段，没法快速定位哪个成员在哪个 session 烧了钱。

**`pkg/usage/store.go`**
- `Record` 新增 `SessionID string` 字段（向后兼容：旧数据缺省为空串）
- `QueryParams` 加 `SessionID` 过滤

**`pkg/runner/runner.go`**
- `UsageRecorder` 签名从 `(in, out, provider, model, agentID)` → `(in, out, provider, model, agentID, sessionID)`
- 调用点 `UsageRecorder(..., r.cfg.SessionID)`

**`pkg/agent/pool.go` + `internal/api/chat.go`**
- 两处 `usageRecorder` 实现同步扩展；`usage.Record.SessionID` 自动落盘

**`internal/api/usage.go`**
- `Records` 端点响应改为 `enrichedRecord = Record + agentName + sessionTitle`
- `agentName` 查自 `agent.Manager`（删除 agent 容错：缺省 ""）
- `sessionTitle` 查自 `session.Store.GetMeta`
- 单次请求内用 map 缓存，避免同一 agent/session 重复文件 IO
- 新增 `?sessionId=` 查询参数

**`ui/src/views/UsageView.vue`**
- 筛选栏新增「Session」下拉（filterable + 最多 200 条）
- 成员下拉改为显示 agent.name（原来只显示 agentId）
- 联动：选中成员 → 加载该成员的 sessions → 填充 Session 下拉
- 明细表：
  - 原「成员」列显示 `agentName` + 小字 `agent_id` 两行
  - 新增「Session」列显示 `sessionTitle` + 小字短 `session_id`
  - 模型列加宽到 220px

### ✅ 验证
- `go build ./... && go vet ./...` ✅
- `go test ./pkg/...` 全绿（新增 5 个 session 测试）
- `npx vue-tsc -b` ✅

### 兼容性
- `UsageRecorder` 签名 breaking —— 但仅 3 处内部调用，全部已更新
- `usage.Record.SessionID` 新字段，老数据 unmarshal 后 SessionID="" 不影响显示
- `SessionIndexEntry` 新字段默认值 ok

---

## [26.4.23v2] — 2026-04-22 · 生产稳定性地基（P0 全集）

基于 OpenClaw 对比 + 自审清单，围绕"生产稳定性"一口气补齐 9 项 P0。**不做**session 清理（= 删用户聊天记录，产品语义错误）、**不做**对用户隐藏的自动模型切换（让用户有完整控制权）、**不做**全插件化。

### 🔴 P0.1 — LLM 错误分层 + 瞬时重试

**`pkg/llm/errors.go`**
- `IsTransient(err)` 识别：429 / 500 / 502 / 503 / 504 · `rate limit` / `too many requests` / `service unavailable` · `connection reset` / `refused` / `broken pipe` / `no such host` · `i/o timeout` / `EOF` / `TLS handshake` / `stream error` · `net.Error.Timeout()`
- `IsAuthFailure(err)` 识别 401/403/invalid api key
- **保守策略**：`context length` / `content filter` / 400 / auth 全部**不重试**，立即上抛

**`pkg/llm/retry.go`**
- `WithRetry(client)` 装饰任意 `llm.Client`
- 默认 schedule：0.5s / 2s / 5s（最大 ~8s）
- **只在 initial call 错误时重试**：一旦开始 streaming tokens 后出错就直接上抛（避免重复计费和消息重发）
- 尊重 `context.Cancel`

### 🔴 P0.2 — SSE 自动重连

**前端 `AiChat.vue`**
- `isNetworkLayerError(err)` 区分网络层（502/`Failed to fetch`/`Load failed`/`connection reset`）和业务错误
- 网络层错误 + 有 sessionId → `appendReconnectNotice()` + `reconnectAndResume()` 指数退避 1s/3s/7s 连 3 次
- 用 `resumeSSE` 订阅同一 Broadcaster，事件回填到原 assistant bubble（用户无感）
- `idle` 返回表示任务期间已完成 → 静默清除
- 90s 兜底超时

### 🔴 P0.3 — 错误隔离

**前端 `AiChat.vue`**
- 旧行为：`cur.text = '[错误] xxx'` 覆盖已累积的 streamText → 用户看到半截回复丢失
- 新：`ChatMsg.truncatedByError` + `sysKind: 'error' | 'info'` 字段
- LLM 错误**不再覆盖主气泡**，原回复保留 + 打 "（因错误中断）" footer
- 错误信息发**独立系统气泡**（红色柔和色，`.msg-system.is-error`）
- `formatErrorMessage()` 把 429/401/5xx/timeout/context_length/content_filter 翻译为友好中文提示；原始消息封顶 240 字符

### 🔴 P0.4 — Abort fence

**前端 `AiChat.vue`**
- 新增 `activeFence: ref<{aborted, ctrl}>` 组件级可观测
- `abortActiveStream(reason)` 统一入口，保证：
  - 事件回调开头检查 `fence.aborted` → 丢弃晚到 event
  - 调 `ctrl.abort()` 真实断开 HTTP stream（省 token）
- 自动触发点：`onUnmounted` / `resumeSession` / `startNewSession` / 新 `runChat` 开头（防僵尸流）

### 🟠 P0.5 — LLM Provider live health

**`pkg/llm/health.go`**
- `Ping(ctx, provider, apiKey, baseURL, forceRefresh)` 发 max_tokens=1 最小请求（成本 ~10 input + 1 output tokens）
- 按 `provider | baseURL | apiKey 摘要`做 key，**30s 缓存**
- 分 provider 探测：Anthropic / OpenAI-compatible / Feishu 等
- 状态码语义：`200/404` = 存活；`401/403` = auth 失败；`429` = 限流；`5xx` = 厂商故障；`0` = 网络不可达

**`internal/api/agent_ext.go::ToolHealth`**
- 在响应里追加 `providerHealth: {provider, model, ok, latencyMs, statusCode, error, cached}`
- `?refresh=1` 旁路缓存强制重 ping

**前端 `AgentDetailView.vue`**
- 工具体检卡片顶部新加 Provider Live Health 条
- 🟢 / 🔴 状态 + 延迟 ms + "重新检测" 按钮
- 失败时 `providerHealthTip()` 分类提示（认证/限流/5xx/网络）

### 🟠 P0.6 — Compaction 同步事件

**`pkg/runner/runner.go::maybeCompactSync`**
- Compaction 触发时机从 "done 后异步" **改为 "turn 开头同步"**
- 当 `EstimateTokens >= CompactionThreshold`：发 `compaction_start` event → 同步调 `session.Compact()` → 发 `compaction_end` event → 重读 history 用压缩后版本
- 失败降级：打审计日志不中断 turn

**`internal/api/chat.go::runEventToJSON`**
- 新增 `compaction_start` / `compaction_end` SSE 事件（含 `tokens_before` / `tokens_after`）

**前端 `AiChat.vue`**
- 收到 `compaction_start` → 在 assistantMsg 之前插入 info 系统气泡 `🗜️ 正在压缩历史上下文 (~Xk tokens)…`
- 收到 `compaction_end` → 同位置更新为 `✓ 已压缩 Xk → Yk tokens`（或错误提示）

### 🟠 P0.7 — thinking_delta（审计后取消）

检查发现 `thinking_delta` 事件已实现（Anthropic extended thinking + DeepSeek reasoner + 前端 `streamThinking` 展示），OpenAI o1 streaming API 不返回 reasoning 文本（只有 `reasoning_tokens` 数字）不适用。**取消**此任务。

### 🟢 P0.8 — Throttle 接口抽象

**`pkg/channel/throttle.go`**
- `Throttle` interface：`Wait(chatID)` / `OnResponse(chatID, err)`
- `FixedThrottle` 默认实现（每 chat 独立 `time.Time`，mutex 保护并发）
- `IsRateLimitError` helper（429 / rate limit / flood_wait）

**故意不改**：现有 `telegram.go` / `feishu.go` 的 `time.NewTicker(1s)` 保持原样。目的是**留结构位**，未来大群场景反馈时只需新建 `AdaptiveThrottle` 实现（~30 行指数退避）并替换 `time.NewTicker`，不改调用方。

### 🟢 P0.9 — Cost 快照审计（审计后确认无需修）

审计 `pkg/runner/runner.go`+`pkg/agent/pool.go`+`pkg/usage/store.go`：每次 `runner.New` 都是全新 Config、session `WorkerPool` 保证 sessionID 单线执行、`UsageRecorder` 每轮 LLM 调用都是 marginal tokens。**不存在重复计费**，无需快照机制。

### ✅ 验证

- `go build ./... && go vet ./...` ✅
- `go test ./pkg/llm/... ./pkg/channel/... ./pkg/runner/...` ✅
- `npx vue-tsc -b` ✅
- 测试新增：`TestIsTransient`（13 case）· `TestIsAuthFailure` · `TestRetryClient_*`（4 case）· `TestFixedThrottle_*`（5 case）

### 无破坏性变更

- RunEvent 新增 `compaction_start/end` 类型，旧客户端默认忽略（switch default）
- ToolHealth 响应新增 `providerHealth` 字段，旧客户端按缺省处理
- `CompactIfNeeded` 签名加 optional `onEvent` callback

---

## [26.4.23v1] — 2026-04-22 · 通讯录 5 个漏网 bug 修复（P0.5）

26.4.22v1 通讯录 GA 上线后，自审 + 外部对比讨论（OpenClaw）发现 5 处已发布代码漏洞。本版一次清理，**不新增功能**。

### 🐛 Bug 1 — `IsOwner=true` 的 contact 没阻断 summary 注入

**现象**：用户在 TeamView 抽屉勾选「这是主人本人在该渠道的身份」后，设计文档里承诺"AI 用 `owner-profile.md` 而非注入本 contact 档案"，但 `pkg/channel/{telegram,feishu}.go` 实际无脑调 `Store.Summary(id)` → **双份档案 + 冲突描述**。

**修复**：`pkg/network/summary.go::Summary` 内部检查 `c.IsOwner`，true 则返回空串。调用方无需改动，owner-profile 接管。

### 🐛 Bug 2 — displayName fallback 链缺失

**现象**：
- 飞书 `getSenderName(openID)` 在刚加好友 / 群聊成员列表未拉取时返回 `""`
- TG `FirstName + Username` 可能都为空

**结果**：建档 `displayName=""` → UI 显示光秃秃的 contactId。

**修复**：`pkg/network/store.go` 新增 `FallbackDisplayName(externalID, candidates...)`，兜底链：`candidates → externalID[:8]`。飞书 / TG / Web Public 3 处入口统一用它。

### 🐛 Bug 3 — 合并后 alias 来消息会"复活"

**现象**：用户手动合并 `telegram:123` → primary `feishu:ou_boss`，alias 文件被删。TG 用户再发消息 → `Resolve("telegram", "123", ...)` 查不到 → **新建空档案**，合并白做。

**修复**：`pkg/network/store.go::Resolve` 没直接命中时，扫描所有 primary 档案的 `aliases` 字段：
- 命中 → `Touch` primary（`MsgCount++`, `LastSeenAt = now`）
- 不命中 → 新建档案

新增 `findPrimaryByAliasUnlocked()` helper。O(N) 扫描对几百 contact 的规模足够快。

### 🐛 Bug 4 — `Graph` / 派遣检查含 contact 行时行为异常

**现象**：26.4.22v1 CHANGELOG 声称"关系表扩展 toKind 字段"，但实际 `RelationRow` 仍只 5 列字段。如果 AI 或手工写入 contact 关系行：
- `Graph` 把 `row.AgentID = "feishu:ou_abc"` 当 agent 创建幽灵节点
- `allowedPeersFromRelations` 把 contact ID 误认为可派遣对象

**修复**（一次补齐）：
- `RelationRow` 正式加 `ToKind string` 字段（`"agent"` 默认 / `"contact"`）
- `parseRelationsMarkdown` 支持 6 列新格式 + 5 列 legacy（默认 `ToKind=agent`）
- `writeRelationsFile` 输出 6 列（`| 目标ID | 目标名称 | 类型 | 关系 | 程度 | 说明 |`）
- `validTypes` 扩充 contact 关系：`服务 / 客户 / 家人 / 朋友 / 同事 / 合作伙伴`
- `Graph` handler 跳过 `ToKind != "agent"` 的边（联系人在单独 tab 渲染）
- `allowedPeersFromRelations` 过滤带 `:` 的 ID + 6 列格式中 `kind != agent` 的行

### 🐛 瑕疵 5 — `network_note` 找不到 entity 时错误不友好

**现象**：AI 传了乱编的 `entityId="telegram:999"` → 返回 `contact %q not found`，AI 无法自我纠正。

**修复**：`pkg/tools/network.go` 新增 `suggestContactIDs(store, query, 3)`，基于：
1. 源前缀匹配（`feishu:xxx` 优先推荐其他 `feishu:` entries）
2. 子串匹配（ID / 显示名）
3. 字符重叠粗 fuzz

失败时 error 带上 `Did you mean: id1 / id2 / id3?` 提示。

### ✅ 测试

全部新增测试，`go test ./pkg/network/... ./pkg/tools/... ./internal/api/...` 全绿：
- `TestSummaryIsOwnerSkips`（Bug 1）
- `TestFallbackDisplayName`（Bug 2，6 子 case）
- `TestResolveRoutesThroughAliases`（Bug 3）
- `TestParseRelationsMarkdown6ColNewFormat`（Bug 4）
- `TestParseRelationsMarkdown5ColLegacy`（legacy 兼容）
- `TestWriteRelationsFileRoundTrip`（6 列往返）
- `TestSuggestContactIDs`（瑕疵 5）

### 数据迁移

- **无破坏性变更**。老 `RELATIONS.md`（5 列）继续可读，下次写入自动升级到 6 列格式
- **无 API 破坏性变更**。`RelationRow` JSON 新增 `toKind` 字段，缺省为 `""`（客户端按 `"agent"` 处理）

---

## [26.4.22v3] — 2026-04-21 · 在线升级进度条修复

用户反馈：Settings 页面「版本与更新」进度条不会自动刷新，跑一次之后不会继续跑。

### 🐛 四个叠加的根因

1. **后端 `downloadFile` 不汇报进度**
   `resp.ContentLength` 在 CF Worker 流式代理（或任何 chunked 传输）场景下常为 `-1`，但代码 `if total > 0 && progress != nil` 直接跳过 → **下载 22MB 过程中进度永远停在 10%**，结束瞬间才跳到 70%。
2. **前端首次拉取延迟 1.5s**
   `setInterval(tick, 1500)` 设完定时器后要等 1.5 秒第一次触发，而 `verify` / `applying` 阶段往往 1–2 秒就跑完 → UI 根本没机会显示中间态。
3. **进度条动画 10s 过渡**
   `el-progress :duration="10"` 让百分比变化花 10 秒跨过去，比实际升级都慢 → 看起来就是"卡住"。
4. **`stage='done'` 后主 polling 不停**
   旧逻辑死依赖 `/api/version` 返回新版本号才 `stopPolling`；服务 SIGTERM 重启间隙 502 反复，polling 异常重入，UI 冻结。
5. **刷新页面丢状态**
   `onMounted` 不主动拉 status，后台升级在跑也看不见。

### 🔧 修复

#### `internal/api/update.go::downloadFile`
- 新增 `estimatedSize = 32MB`（二进制实际 ~25MB 留冲）
- `total > 0` 走真实百分比，`total <= 0` 走估算百分比（封顶 95%，收尾 `progress(100)`）
- 节流：百分比变化 >= 1 才回调，避免锁竞争
- **效果**：下载阶段 10% → 70% 平滑推进（无论是否走 CF 代理）

#### `ui/src/views/SettingsView.vue`
- `startPolling`：抽出 `tick` 函数，**立即首次触发** + interval 1500ms → 500ms
- `stage === 'done'` → 立即 `stopPolling` + 触发 `waitForRestart()` 独立循环
- 新增 `waitForRestart()`：独立 1.5s 轮询 `/api/version`，拿到新版本号 → `restartDetected=true`，**90s 兜底超时**
- `el-progress :duration="10"` → `"1"`（动画跟得上）
- `onMounted` 主动 `updateApi.status()`：
  - 发现 `downloading/verifying/applying` 自动 `startPolling`（刷新页面不丢状态）
  - 发现已 `done` 自动 `waitForRestart`
- `onBeforeUnmount` 清 `restartWaitTimer`

### ✅ 效果

- 下载阶段进度平滑从 10% 走到 70%（非竟跳）
- verify / apply 中间态能被 UI 观察到
- 升级完成后不会永久卑月 polling，UI 不隐止大名重启
- 刷新页面 = 自动接上正在进行的升级

### 验证
- `go build ./... && go vet ./...` ✅
- `npx vue-tsc -b && vite build` ✅

---

## [26.4.22v2] — 2026-04-21 · 文档全面刷新

纯文档版本，无代码逻辑变动。为配合 26.4.20v1 → 26.4.22v1 的三轮重大功能扩展（CLI 修复 / 聊天 UI 重构 / AI 能力扩展 / 关系双向 / 极简自主三件套 / 通讯录 + 渐进式披露），一次性刷新所有面向开发者和使用者的文档。

### 📝 README.md 全面更新

- **功能清单补齐**：新增「通讯录 & 关系网」「Owner 档案」「渐进式披露」「工具体检 + WISHLIST」「档位 hashtag chip」「🌅 晨间例行 + NO_ALERT」「建议连接」章节
- **项目结构**：补 `pkg/network/` `pkg/convlog/` `pkg/chatlog/` · UI views 列表完整化（含 AgentCreateView / ChannelsView / PublicChatView 等）· 注释更具体到每个子包的职责
- **配置示例**：从旧 `models.primary: "provider/model"` 字符串 → 新 `models[]` 数组 + `default: true` 标记 · 附老版本迁移说明
- **新增「系统提示词工程」章节**：列出 9 层分层构建顺序（当下信息 → owner → IDENTITY/SOUL → memory INDEX → network INDEX + RELATIONS → 当前对话对方 → capabilities → AGENTS.md → projects）
- **工具生态** 数字：70+ → 80+，新增 `network_note` / `wish_*` / `feishu_*` 分组
- **P1 规划**（原 v0.11）更新为实际下版本目标：群档案 · 头像 API · AI 自动合并 · self_schedule · autonomy budget

### 📝 docs/system-prompt-and-flow.md 重写

- 标题锁定适用版本 26.4.22v1+
- 全面重写「系统提示词构建」为 **10 层渐进式披露模型**（图示 + 每层用途）
- 补充「渐进式披露含义」小节：不预喂 / 按需索取 / 文件式存储
- 新增 **Contact 档案完整模型**：ID 规范形式、frontmatter 字段、markdown body 四段（事实/偏好/最近话题/待跟进）
- 新增 **4 处 resolveContact 入口表** 与 **Summary 生成规则**
- 新增 **Capabilities Context 示例**（ready/blocked 工具 + WISHLIST 头部）
- 新增 **Cron NO_ALERT 静默机制** 段落 + 晨间例行样板 prompt
- 更新 RunEvent 类型表（tool_call_id 精准匹配）
- Anthropic 特殊处理记录 `message_delta` 的 `output_tokens` 从顶层 `event.Usage` 读

### 📝 docs/session-design.md 小幅补充

- 头部适用版本：v0.9.0 → 26.4.22v1
- ConvLog 目录补齐 `feishu-{chatId}.jsonl`
- ChatsView 统一 AiChat 组件渲染说明
- 只读模式（feishu/telegram 会话锁图标）说明
- 新增「通讯录联动」章节：指向 `docs/system-prompt-and-flow.md`

### ✅ 验证

- `go build ./... && go vet ./...` 全绿
- `go test ./pkg/network/... ./pkg/tools/...` 全绿
- README 链接校验 · CHANGELOG 格式校验

### 无破坏性变更

纯文档。

---

## [26.4.22v1] — 2026-04-21 · 通讯录（network/）· 渐进式披露 · 每 agent 一本关系网

本版 5 commit，主体重构：把"用户档案 / agent 关系 / 外部联系人"三套散乱概念**统一为一个「通讯录」模块**，物理上落在每个 agent 私有的 `workspace/network/` 目录，用"渐进式披露"模式管理提示词注入——真正让 AI "在每个来源都能准确回复"。

### 🎯 背景

上一版 `memory/core/user-profile.md` 只画一个人；但实际来源包括：
- 面板运营者（你）
- 飞书老板 / 同事 / 陌生人
- TG 群友 / 私信联系人
- Web 匿名访客

一份档案画所有人 → 都画不准。本版引入 **Entity 抽象**（每个外部人都是一个 contact）+ **渐进式披露**（轻量 INDEX + 运行时摘要 + 按需深读）。

### 🏗️ 架构（每 agent 一本通讯录）

```
workspace/network/
├── INDEX.md           ← system prompt 注入的轻量层（~500-800 chars）
├── INDEX.json         ← 机器索引（UI 读）
├── RELATIONS.md       ← 关系表（从 workspace 根自动迁入）
└── contacts/
    └── <source>-<externalId>.md   ← 完整档案（AI 按需 read）
```

每个 contact 档案含 frontmatter（id / source / tags / aliases / isOwner / msgCount / timestamps）+ 4 段 body（事实 / 偏好 / 最近话题 / 待跟进）。

### 🎚️ 三层渐进式披露

- **层 1：`network/INDEX.md`** — 永远注入 system prompt，只给"谁存在 + 一句话摘要 + 文件路径"。~500 chars，token 友好。
- **层 2：当前对话对方摘要** — `runner.Config.ExtraContext` 运行时动态注入（frontmatter + 事实前 3 条 + 偏好前 2 条），~300 chars，硬 cap 1200。
- **层 3：完整档案** — AI 主动用 `read("network/contacts/<id>.md")` 按需深读，不预占未来对话 token。

### 📦 新增模块

**`pkg/network/`** 新包（~1000 LOC + 4 单元测试全绿）：
- `Contact` / `ContactSummary` / `Index` 数据模型
- 手写 YAML-ish frontmatter codec（无 YAML 依赖）
- `Store` 线程安全：`Resolve(source, externalId, displayName)` upsert + `Touch` + `Get/Save/Delete/List/Summary`
- 每次 mutate 自动 `refreshIndex` 重建 INDEX.{md,json}
- `MigrateIfNeeded` idempotent：`workspace/RELATIONS.md` → `workspace/network/RELATIONS.md` + `user-profile.md` → `owner-profile.md`

**`internal/api/network.go`** REST：
- `GET /api/agents/:id/network/contacts` — 列表
- `GET /api/agents/:id/network/contacts/:cid` — 详情
- `PATCH /api/agents/:id/network/contacts/:cid` — 更新（displayName/tags/body/isOwner）
- `DELETE` / `POST .../merge` / `POST .../refresh`
- contactId 三种形式（`feishu:ou_abc` / URL-encoded / `feishu-ou_abc`）都接受

### 🔌 4 处消息入口自动建档

AI 见到一个新联系人时档案**已经存在**（不阻塞对话）：

- **Telegram**（`pkg/channel/telegram.go`）：`Resolve(telegram, userId, firstName)` → summary 作为 extraSystemContext 传入 streamFunc
- **飞书**（`pkg/channel/feishu.go`）：`Resolve(feishu, openId, senderName)` → append 到现有 extraCtx
- **Web Public**（`internal/api/public_chat.go`）：新增 `visitorToken` 参数，`Resolve(web, sessionToken)` → `runner.Config.ExtraContext`
- **面板**（chat.go）：**不接 contact**——运营者就是 owner，由现有 `memory/core/owner-profile.md` 负责

### 🛠️ `network_note` 工具

`network_note(entityId, section, text)` — 让 AI 把发现的事实/偏好/待跟进原子追加到联系人档案：
- `section` 严格枚举：`事实 | 偏好 | 最近话题 | 待跟进`
- 占位符（`- (AI 通过 network_note 工具追加此处)`）首次写入自动清除
- 缺失 section 自动补 `## 段` header
- 旁路 `network/changes.log` 审计（用户可 `read` 查 AI 改了什么）
- 4 个单元测试覆盖（存在 / 缺失 / 多条 / 部分匹配）

### 📝 system_prompt 重构

- IDENTITY/SOUL **之前** 注入 `memory/core/owner-profile.md`（兼容上版 user-profile.md）
- 新增 `network/INDEX.md` 注入（替代原本的 RELATIONS.md 直注）
- `network/RELATIONS.md` 优先，`RELATIONS.md` 根部 fallback（兼容未迁移 agent）
- 派遣规则文案更新指向 `network/RELATIONS.md`
- 新增使用约定："见到新人档案已存在 · `network_note` 追加 · `read` 按需深读"

### 🎨 TeamView 融合（菜单「团队」→「通讯录」）

- 顶部 tab 切换：「🧑‍🤝‍🧑 AI 成员网络」 | 「👥 联系人」
- **AI 成员网络** tab：**零改动**（保留原图谱交互 / Suggestions / Legend）
- **联系人** tab：跨 agent 聚合列表
  - 筛选栏：搜索（姓名/ID/tag/来源）+ 来源 radio + agent radio
  - 列表行：头像色块（deterministic hue from name hash）+ 姓名 + 来源 tag + 标签 chips + 消息数 + 最后活跃 + 所属 agent chip
  - 点击打开 540px drawer
- **联系人抽屉**：
  - 头像 + 显示名
  - 标签：可删 + 手动加 + 6 预设快捷（家人/同事/客户/合作伙伴/朋友/AI 成员）
  - `isOwner` checkbox："这是主人本人在该渠道的身份"
  - Markdown body 编辑（等宽字体 12 行）
  - 保存 / 删除

### 🔄 兼容性 / 迁移

- Agent 启动时自动跑 `network.MigrateIfNeeded`：一次性搬 `RELATIONS.md` + rename `user-profile.md` → `owner-profile.md`
- 3 处 Go 内部读 RELATIONS.md 都改为"优先 `network/`，fallback 根部"
- `internal/api/relations.go` 所有 handler 统一走 `relationsPath()` helper
- **老数据零丢失 + 老 API 零破坏**

### ✅ 验证

- `go build ./... && go vet ./...` 全绿
- `go test ./pkg/network/... ./pkg/tools/...` 全绿（8 + 4 个用例）
- `npx vue-tsc -b && vite build` 全绿

### 明确不做（留给 P1）

- ❌ 群档案（chats/）
- ❌ 头像 API 拉取（先用首字色块）
- ❌ AI 自动合并（人手合并即可）
- ❌ 跨 agent contact 聚合视图（列表已有 agent chip 但不合并）
- ❌ Web 访客升级为命名 contact

---

## [26.4.21v1] — 2026-04-21 · 极简 AI 自主三件套（用户档案 + 晨间例行 + 建议连接 + 档位 chip）

本版 2 commit 聚合，贯彻**极简信念**——砍掉 `self_schedule` 工具、`autonomy budget` 字段、`wishlist` 独立 tab、agent 类型分化 等 4 个冗余抽象，只做 4 件真正必要的小改动。背景：上一轮对话中 AI 自省"我是博尔赫斯图书馆里没窗的管理员"，并给出了自主唤醒 + 结构化认识用户两个诉求。本版对应实现。

### 👤 用户档案 `memory/core/user-profile.md`

让 AI 知道"我服务于谁"——提示词第三块基础设施（前两块：工具体检 + WISHLIST）。

- `pkg/runner/system_prompt.go`：在 IDENTITY.md / SOUL.md 注入**之前**加 `injectFile("memory/core/user-profile.md")`。认知顺序：先看"我服务的人"，再看"我是谁"。
- 文件不存在时 `injectFile` 静默跳过，不占 token。
- `ui/src/views/AgentDetailView.vue` "身份 & 灵魂" tab 新增第 3 张编辑卡：
  - 标题 `👤 用户档案`
  - textarea 14 行，空白时显示完整 placeholder 模板（基本 / 沟通偏好 / 在做的事 / 禁忌）
  - 复用 `filesApi.write`，**零新增后端代码**
  - `@blur` 自动保存，空白不弹提示（避免焦点切换骚扰）

### 🌅 CronView 晨间例行一键模板

用户不必学 cron 表达式——点击一个按钮即可给选中 AI 成员创建每天固定时间的自主唤醒。

- `ui/src/views/CronView.vue`：顶部新增"🌅 晨间例行"按钮 + 专用对话框。
- 用户只选：agent + 时间（HH:mm）+ 时区（默认 Asia/Shanghai）。
- 后台自动构造 `{MM} {HH} * * *` cron 表达式 + 预置 prompt：
  ```
  1. 扫描昨天的对话历史，值得长期记住的要点整理到 memory/core/
  2. 检查 WISHLIST.md 与 GOALS 看有没有进展
  3. 若发现世界状态需要更新，web_search / web_fetch
  4. 若有值得主动告诉用户的事，追加 memory/daily/notes-to-user.md
  5. 若今天没有值得汇报的事，请只回一个单词：NO_ALERT
  ```
- **关键刹车**：末尾 `NO_ALERT` 对接 `pkg/cron/engine.go` 已有的 `SilentToken` 机制 → 无事默认静默，不每天骚扰用户。
- **零新增后端代码**，复用 `POST /api/cron`。

### 💡 TeamView 建议连接

针对 AI 在对话中自己发现的 UX 缺口："我透过玻璃门看到 4 个同事但叫不动他们"——提供一键引导。

- `ui/src/views/TeamView.vue`：图谱卡片下方新增"💡 建议连接"折叠面板。
- 纯前端计算：所有节点两两组合 `-` 已有边 `=` 未连集合。
- 每行：`[成员A ↔ 成员B]` + `[自定义…]`（走现有关系编辑 dialog）+ `[建立平级关系]`（一键 `putEdge(A, B, '平级协作', '常用', '')`）。
- 大团队时只显示前 5 组，提示"还有 X 组未显示"，避免视觉爆炸。
- 复用现有 `relationsApi.putEdge`，**零新增后端代码**。

### 🎚️ AiChat 档位 hashtag chip

让用户用一键 `#急` / `#深思考` 等微语法调节 AI 回复风格。

- `ui/src/components/AiChat.vue`：输入框上方新增 5 个极小 chip：
  - `#简答` → 只给结论
  - `#深思考` → 展示多步推理
  - `#写代码` → 聚焦实现
  - `#闲聊` → 放松语气
  - `#急` → 最快可用方案
- 仅在输入为空 + 无附件时显示，不干扰正常打字。
- 点击追加到输入末尾并聚焦 textarea。
- `pkg/runner/system_prompt.go`：添加 hashtag 档位约定说明，AI 见到 tag 自动调节风格。

### 架构决策记录（为什么砍掉）

| 原建议 | 为什么不做 |
|--------|-----------|
| `self_schedule` 工具 | AI 已有 `cron_add` 工具，功能等价，再造一遍是复杂度污染 |
| Autonomy budget（每日醒 N 次 / token 上限） | cron expression 本身就是频率预算；CronView 能看 run 次数与成本；先不做独立系统 |
| WISHLIST 独立 tab | 已有 `GET /api/agents/:id/wishlist` + 身份 tab 底部卡片，别重复 |
| "工具型 vs 存在型" agent 分化 | 产品概念不是代码，现在做是过度抽象 |
| 用户档案表单化编辑 | 第一版就文本编辑 + placeholder 模板，简单直给 |
| 晨间模板库 / 任务模板市场 | 就 1 个硬编码模板，有需要再加 |
| 团队图谱智能关系推断 | 默认一键平级协作，其他关系走"自定义…"走现有 dialog |
| Hashtag 用户自定义 | 5 个硬编码够用，保持 UI 极简 |

### 验证

- `go build ./... && go vet ./...` ✅
- `npx vue-tsc -b` ✅
- 2 commit 总计 +353 / -4 行
- **本版无 API 破坏性变更**

---

## [26.4.20v3] — 2026-04-20 · 关系双向同步 · 派遣权限 · 对话 drawer 修复

本版 3 commit，聚焦关系图谱 + 派遣权限 + 一个细节 UI 修复。

### 🔗 关系系统全面双向化

用户反馈: "建立关系后，在成员的关系里面没有更新。这里的关系应该是双向的，在提示词里有关系要加进去。同时相应的关系对应着派遣的权限。"

- **关系全类型双向存储**（`internal/api/relations.go`）
  - 之前 `inverseRelationType` 对 `上下级` / `上级` / `下级` 返回空串，**跳过了反向写入** → A 标 B 为上级时，B 的 `RELATIONS.md` 完全没 A 的记录。
  - 修复：
    - `上级` → 反向 `下级`
    - `下级` → 反向 `上级`
    - `上下级`（A 是 B 的上级）→ 反向 `下级`（B 是 A 的下级）
    - `平级协作` / `支持` / `其他` 继续保持对称
  - 现在任何一侧加关系，双方 `RELATIONS.md` 都能看到对应记录。

- **前端关系 tab 自动刷新**（`ui/src/views/AgentDetailView.vue`）
  - `saveRelations` finally 加 `await loadRelations()`（磁盘回读，同步后端规范化 / 双向补全副作用）
  - `watch(activeTab)` 切换到 `relations` 时自动 `loadRelations()` → 看到别人给自己加的反向关系

### 🔒 agent_spawn 关系权限检查

- 之前：**任意 agent 可派遣任意 agent**，权限完全没做。
- 现在规则：派遣 **user agent** 时必须在当前 agent 的 `RELATIONS.md` 里有记录。
- 豁免：built-in agent type（`general-purpose` / `explore` / `plan` / `verification` / `coordinator`）不受此限制（coordinator 模式专用）。
- 实现（`pkg/tools/registry.go::handleAgentSpawn`）：
  - 判定 `targetIsUserAgent`：在 `agentLister` 里找到 = true
  - false 但 ID 匹配 `builtInAgentTypes` → 豁免
  - 未在关系表 → 拒绝："❌ 你与成员 X 之间没有建立关系，无法派遣。请先前往团队图谱建立关系，或用 `wish_add` 告知用户。"
  - 新增 helper：`pkg/tools/relations_helper.go::allowedPeersFromRelations`

### 💡 system prompt 注入派遣规则

- `pkg/runner/system_prompt.go` 在 `RELATIONS.md` 注入后追加派遣规则说明，让 AI 在派遣前就知道这个约束，不至于碰壁才发现。

### 🐛 对话管理 drawer 历史消息无法显示

用户反馈: 对话管理里点开 drawer 内容空白，但"X 条 · X tokens"正常。

- 根因时序:
  1. `sessionDrawer=true` 打开 drawer
  2. `detailLoading=true` → 模板 `v-if='detailLoading'` 显示 loading，AiChat 在 v-else 分支**根本没 mount**
  3. `await sessionsApi.get()` + `await nextTick()`
  4. 调 `sessionAiChatRef.value?.loadHistoryMessages()` → ref 是 `null`（AiChat 尚未 mount），静默失败
  5. `finally` 设 `detailLoading=false` → AiChat 此刻才 mount，但已错过 load 调用 → **永远空白**

- 修复（`ui/src/views/ChatsView.vue`）:
  - Template: AiChat 改为**始终 mount**（v-if 只看 drawer+row 是否存在），loading 改用 absolute 定位的 overlay 遮罩，不再卸载 AiChat
  - Logic: load 调用移到 finally 之后 + 两次 `nextTick()` + 兜底轮询 500ms（el-drawer 动画期间 mount 可能慢于 finally）
  - 同步修了 channel drawer 的同类问题

### ✅ capabilities context 完整闭环注入 runner

- 上个版本的 `工具体检 + WISHLIST` 信号已能写入 prompt，本版验证了 `chat.go::execRunner` / `pool.go` 所有分支（Web / Telegram / Feishu / Cron）都通过 `runner.Config.CapabilitiesContext` 拿到相同内容，AI 跨渠道对"自己有什么工具"的认知一致。

### 备注

本版**无 API 破坏性变更**，完全向后兼容。

---

## [26.4.20v2] — 2026-04-20 · AI 能力扩展 · 关键 bug 修复 · UI 全面升级

本版 13 commit 聚合，分 4 主题。

### 🎯 AI 能力扩展（新特性）

- **工具体检（Tool Health Check）**：新 API `GET /api/agents/:id/tool-health`，检查每个工具的 ready / blocked 状态
  - 识别 `web_search` 需 Brave API Key、`image` 需视觉模型、`feishu_*` 需飞书渠道、`send_message` 至少一个渠道
  - AgentDetailView 「工具权限」tab 新增「🏥 工具体检」卡片，一键检查 + 列出受阻工具 + 解决提示
  - 解决用户发现的"AI 不知道自己有哪些工具可用"问题
- **能力愿望清单（WISHLIST）**
  - 新 tools：`wish_add({title, reason, priority?})` / `wish_list({limit?})`
  - AI 主动写入 `workspace/WISHLIST.md`，表达能力需求
  - 新 API `GET /api/agents/:id/wishlist`，AgentDetailView 「身份 & 灵魂」tab 底部展示愿望卡片（P0/P1/P2 优先级 + 时间 + 理由）
  - 把 AI 从"被动执行者"升级为"能主动表达诉求的团队成员"
- **系统提示词注入"当下信息"**
  - 详细时间（含周几、年度第 N 天、ISO 周数）
  - `Platform: 你运行在 ZyHive ...`
  - ⚠️ 今天可能晚于训练截止，时事请用 `web_search` / `web_fetch`
  - 💡 缺能力请用 `wish_add` 记录愿望
  - 让 AI 意识到"此刻的位置"，不再被训练截止日期锁在过去

### 🐛 关键 Bug 修复

- **🔥 `output_tokens` 全 0 真·根因**（anthropic `message_delta`）
  - 之前所有对话的 output token 记录均为 0，523 条历史数据全错
  - 根因：`pkg/llm/anthropic.go` 的 `message_delta` 从 `event.Delta` 找 usage，但 Anthropic 实际结构是 `{type, delta, usage}` — `usage` 在**顶层**
  - `event.Usage` `json.RawMessage` 字段已声明但从未使用
  - 修复后新对话的 output 正确记录（验证 input=5604 / **output=4** ✅）
- **🔥 并行工具调用状态不更新**
  - 并行调用 3+ 个 tool 时，前两个卡永远显示空心圆 ○ 无时长
  - 根因：`RunEvent` 的 `tool_result` 只带 Text 无 ToolCallID，前端单一 `activeToolId` 会被后来的 tool_call 覆盖
  - 修复：`RunEvent.ToolCallID` + SSE `tool_call_id` 字段 + 前端按 ID 精准匹配
- **chat API 缺 UsageRecorder**：`internal/api/chat.go` 构造 `runner.Config` 漏了 `UsageRecorder`，所有 Web 聊天的 usage 根本未写入 usageStore。补上 + provider 字段
- **日志页面显示"暂无日志内容"**：`/api/logs` 只读 `/tmp/aipanel.log`，但 systemd 模式下日志在 journal。改为优先文件 → journalctl → `log show`（macOS）三级降级
- **Web source 面板自建对话被误判只读**：之前 `source !== 'panel'` 一律只读，但 `ses-xxx` session 会被 normalize 成 `web` → 误判。改为明确枚举外部客户端：`['feishu', 'telegram']`

### 🎨 UI 全面升级

- **全站菜单视觉一致性**
  - 双 padding 问题修复（SkillsView / SettingsView / CronView 自带 padding 与 `.app-main` 叠加）
  - 全屏 view 统一 `margin: -20px -24px; height: calc(100vh - 44px)` 逃脱 padding（Projects / Subagents / AgentCreate）
  - Dashboard stat cards 改 `::before` 色条 + hover 抬起，新配色盘
  - AgentsView 卡片加描边 + hover 抬起
  - AgentDetailView 多层 el-card 统一：去 box-shadow + 1px #ececec + card__header 浅灰底
  - 边框色全站批量 `#e4e7ed` → `#ececec`
  - 全局 6px 细滚动条（替换 Element 粗白滚动条）
- **对话管理 drawer 重构**：两个 drawer（渠道/面板会话）统一用 `<AiChat read-only>` 渲染，消除空气泡 + 对齐新 markdown 解析（代码高亮 / 表格 / blockquote / 工具卡可展开）
- **渠道识别 + 只读模式**：侧边栏 tag 按 session.source（飞书/TG/Web/面板 4 色区分），飞书/TG 会话自动只读 + 锁图标提示条
- **无效 provider 模型过滤**：Provider status=error 的模型不在下拉中出现；agent 绑定的模型失效时，AiChat 顶部显示黄色警告 + 前往配置按钮
- **聊天输入框 Cursor 风克制**：背景 `#f6f6f7`、去蓝色 focus 环、发送按钮默认灰透明 → 有内容时变 Cursor 经典深黑 `#18181b`、尺寸 36→28px
- **Enter 发送 / Shift+Enter 换行**：对齐主流聊天 App；IME 组词期间不拦截
- **task-notification XML 过滤**：Coordinator 内部协议 XML 不再以用户气泡暴露给用户（AiChat + ChatsView + AgentDetailView 多处过滤）

### 📊 UsageView 升级

- stat cards `::before` 色条（蓝/绿/橙/红）+ hover 描边
- chart / records 卡片去 shadow + 1px 描边 + 统一 card__header 浅灰底
- 厂商分布 / 成员用量饼图：legend 加 `type: scroll` 防挤压、pie center 给 legend 更多空间、白色 1px 描边
- stat-value 字号 22→24 + letter-spacing -0.5px（Cursor 风数字）

### 🚀 部署基建

- `scripts/deploy-hive.sh` 强制 sync-ui + 二进制完整性 marker 检查
- `logs` endpoint 改 journalctl 后 `systemd` 下线上日志也能流畅查看

### 备注

本版**无 API 破坏性变更**，完全向后兼容。重点是修好了历史上一直没发现的 `output_tokens=0` 与并行工具状态两个深层 bug。

---

## [26.4.20v1] — 2026-04-20 · CLI 全面测试 · 聊天 UI 重构 · 部署基建

### 🖥️ CLI 交互面板修复（6 处 bug）

- **双 pause 修复**：配置管理菜单选项 1（查看完整配置）和选项 7（Providers 子菜单）返回后会触发两次 `pause()`，用户要按两次 Enter 才能返回。
- **成员列表隐藏目录 + 序号跳号**：`menuAgentManage` 不再列出 `.subagent-tasks` 等隐藏内部目录；序号改为基于有效成员列表的 1-based 索引，不再跳号。
- **在线更新 URL 文件名错误**：下载地址拼 `aipanel-{os}-{arch}`，但实际 release asset 叫 `zyhive-{os}-{arch}`。整个在线更新功能之前一直 404，现修复。
- **备份目录修改未持久化**：`menuBackup` 选项 4 修改目录下次进入就被重置为 `/var/backups/zyhive`。新增 `loadBackupDir` / `saveBackupDir`，写入 config 同目录的 `backup-dir` 状态文件。
- **`--help`/`-h` 显示英文 Go flag Usage**：Go `flag.Parse()` 抢先处理 help 参数，`case "help"` 走不到。在 Parse 前拦截，并设置 `flag.Usage = printSubcmdHelp`。
- **在线更新拒绝无反馈立即清屏**：`confirm` 返回 false 时直接 `return`，无视觉提示。加 "已取消更新" + pause。

### 🧪 CLI 回归测试脚本

- 新增 `scripts/test/cli_regression.sh`（161 行，42 个 stdin 驱动断言），覆盖 9 大主菜单 + 全部子菜单。
- 可通过 `TEST_BIN` / `TEST_HOME` / `TEST_DATA` 环境变量覆盖默认路径。
- 自动识别"已是最新"和"已取消更新"两种在线更新分支。

### 🎨 聊天界面 UI 全面重构（Cursor 极简风）

- **全局细滚动条**（替换 Element Plus 默认白色粗滚动条）：6px，`rgba(0,0,0,0.08)` 半透明，hover 加深；深色容器用白色半透明。
- **Sidebar active 态**：从整块蓝色填充改为**左侧 2px 色条 + 柔和高亮**，菜单项高度 50px→40px。
- **AI 消息去气泡**：完全移除白色卡片背景和阴影，直接铺在 `#fafafa` 背景上，Cursor 风"文档流式"阅读。
- **用户消息弱化**：从渐变蓝大块改为浅蓝胶囊 `#e8f3ff`，文字深色。
- **Markdown 渲染增强**：
  - GFM 表格（斑马纹 + 圆角边框 + 浅色 thead）
  - Blockquote（左 3px 蓝条 + 斜体灰字）
  - 代码块右上角 language badge（大写小字）
  - 极简 syntax highlight（无第三方库，手写 regex）：js/ts/go/py/sh/bash/rust 关键字 + 字符串 + 数字 + 注释 4 色
  - h1-h4 清晰层次、水平线、列表行距统一
- **工具卡淡化**：默认无边框 + 透底 `rgba(0,0,0,0.02)`，只 hover/running/error 态显边框。
- **输入区重构**：单个圆角 14px 胶囊，focus 时 3px 柔光环。

### 🔐 渠道识别与只读模式

- **后端 `session.source` 正确透传前端**：`SessionSummary` 接口补 `source?: string`。
- **AgentDetailView / ChatsView / ChatHomeView 按 source 统一识别**：飞书（紫）/ TG（绿）/ Web（橙）/ 面板（灰）。
- **session 标题剥离 `[发送者]:` 前缀**：飞书群聊标题更干净。
- **`<task-notification>` XML 过滤**：Coordinator 注入给 LLM 的内部协议 XML 不再以用户气泡暴露给用户。应用到 AiChat 主渲染 + 3 处历史加载 + ChatsView drawer + AgentDetailView。
- **非面板会话只读**：AiChat 新增 `readOnly` + `readOnlyReason` props；非 panel 来源 session（feishu/telegram/web）输入区被替换为锁图标提示条："此对话来自 XX 渠道 · 仅可查看历史"。`send()` 加双保险。
- **统一只读渲染**：`type='channel'`（convlog）和 `type='panel'` 但 `source!=='panel'` 的会话**都走 AiChat** 渲染；移除冗余的 history-viewer DOM（-80 行）。AiChat 新增 `loadHistoryMessages(msgs)` 方法：清空 streaming → 替换 messages → nextTick 后强制 scrollBottom(true) 自动滚底。
- **对话侧边栏极简化**：每项左侧 2px 渠道色条代替 `el-tag`；meta 行小圆点 + 渠道名 + 时间紧凑排版。

### 🚀 部署基建

- **修复 CGO glibc 兼容**：`make release` 所有平台加 `CGO_ENABLED=0` 产出**纯静态二进制**，解决 Ubuntu 24.04 glibc 2.39 构建在 CentOS 7 glibc 2.17 上 `GLIBC_2.34 not found` 的部署问题（已在 26.4.19v1 合并，本版强化）。
- **新增 `scripts/deploy-hive.sh`**：一键热部署脚本，**强制 sync ui_dist → cmd/aipanel/ui_dist** 再编译，避免二进制内嵌旧 UI 的坑；内置 `readonly-banner` marker 的二进制完整性检查。

### 备注

本版为维护 + UI 体验版。所有后端 API 保持向后兼容，运行时行为变更仅在 UI 层（渠道识别 / 只读模式 / 消息渲染）。

---

## [26.4.19v1] — 2026-04-19 · 测试基础设施修复与仓库清理

### 修复（仓库维护）

- **测试套件全绿**：修正 `cmd/aipanel/main_test.go`
  - `TestConfigLoad` 适配 `Config.Models` 新结构（`[]ModelEntry` 取代旧 `legacyModelsConfig{Primary}`），解决 `go vet ./cmd/...` 与 `go test ./cmd/aipanel/...` 构建失败
  - `TestAgentManager` 适配四层 memory tree 目录结构（`workspace/memory/{INDEX.md,core,projects,daily,topics}` 取代平面 `MEMORY.md`）
- **仓库清理**：移除 `projects/zyhive/` 下 15 个冗余 Go 源码快照（-5300 行），解决 `go build ./...` 失败于 `pattern all:ui_dist: no matching files found`
- **Agent 创建页 UX**：新建 agent 时若无模型配置，禁用输入框并显示提示卡片（b0ba1cc）
- **构建系统**：`make release` 所有目标平台加 `CGO_ENABLED=0`，产出**纯静态二进制**。此前 Ubuntu 24.04（glibc 2.39）构建的 linux 二进制无法在 CentOS 7（glibc 2.17）上运行（`GLIBC_2.34 not found`），影响生产部署

### 验证矩阵

| 命令 | 结果 |
|------|------|
| `go build ./...` | ✅ exit 0 |
| `go vet ./...` | ✅ exit 0 |
| `go test -count=1 ./...` | ✅ 6/6 包 PASS |
| `go test -race -count=1 ./...` | ✅ 6/6 包 PASS（无 race） |
| `npx vue-tsc -b` | ✅ |
| `npx vite build` | ✅ |
| `make release` | ✅ 6 个平台二进制 |

### 备注

本版本仅涉及测试套件修复与仓库清理，**无运行时行为变更**。

---

## [26.4.1v20] — 2026-04-01 · 飞书渠道全面接入

### 新功能

#### 飞书长连接（WebSocket）
- 实现飞书 WS 长连接，基于 protobuf 帧解码（pbbp2.Frame）
- 无需公网 Webhook，本地服务器直连飞书推送
- 自动 token 刷新、断线重连

#### 飞书流式回复（卡片模式）
- 收到消息立即显示「⌛ 正在思考...」占位卡片（类 Telegram typing 效果）
- 使用飞书 Interactive Card + PATCH 实现流式更新
- 支持 Markdown 渲染（加粗、列表、表格、代码块）

#### 飞书 7 大能力工具（配置渠道后自动注入）
- `feishu_send_message` — 发送消息（文本/卡片）给用户或群组
- `feishu_create_chat` — 创建群聊并邀请成员
- `feishu_create_bitable_app` — 创建新多维表格应用
- `feishu_create_bitable_table` — 在已有 Bitable 中创建表格
- `feishu_list_bitable_records` — 读取多维表格记录
- `feishu_create_bitable_record` — 新增多维表格记录
- `feishu_get_user_info` — 查询用户信息
- `feishu_create_calendar_event` — 创建日历日程
- `feishu_create_task` — 创建任务

#### 飞书群聊响应模式配置
- 默认：仅响应 @提及
- 支持 `@机器人 /listen all` 切换为响应全部消息
- 支持 `@机器人 /listen mention` 切换回仅响应 @
- 支持 `@机器人 /status` 查看当前模式
- 每个群独立配置，持久化到磁盘
- 多机器人共存：命令只有被 @ 的机器人处理

#### 飞书配对授权体验优化
- 未授权用户直接引导至管理面板授权链接
- 管理员在面板一键批准，用户立即收到「✅ 授权成功」通知
- 无需重启即可生效

### 修复

- **飞书会话持久化**：修复 feishu- 前缀 session 未写入索引导致对话列表不显示的问题
- **用户信息感知**：AI 通过 ExtraContext 自动感知当前用户 open_id，无需手动告知
- **群聊多人区分**：群聊消息加 `[发送者名字]:` 前缀，AI 能区分多人对话上下文
- **事件去重持久化**：seenEvents 写入磁盘，重启后不再重复处理积压消息
- **并发串行化**：同一聊天的消息串行处理，避免并发 LLM 调用互相覆盖

### 技术细节

- 飞书 WS 端点：`POST /callback/ws/endpoint`（AppID + AppSecret）→ 返回 `wss://msg-frontier.feishu.cn/ws/v2?...`
- 帧格式：protobuf pbbp2.Frame（method: 1=data, 2=control, 3=ping, 4=pong）
- 卡片 schema: `2.0`，元素类型 `markdown`，config `update_multi: true`
- session ID 格式：`feishu-{chat_id}`（群/私聊统一前缀）
- 工具注册：配置飞书渠道后 `configureToolRegistry` 自动调用 `WithFeishu(appID, appSecret)`


## [26.3.18v8] — 2026-03-18 · WorkspaceChatLayout + AgentDetailView 全站浅色主题

### 修复
- **WorkspaceChatLayout.vue 浅色主题**：全文件 53 处深色硬编码色值（`#1a1a2e`、`#0d0d1a` 等）替换为浅色等价值（`#f4f6f9`、`#fff`、`#e4e7ed`、`#303133`），消除工作区内嵌聊天页面的黑色残留
- **AgentDetailView.vue 浅色主题**：移除 `:deep` 覆盖的 El Plus Tab 深色变量，恢复 El Plus 标准浅色 Tab 配色

---

## [26.3.18v7] — 2026-03-18 · LogsView 浅色主题

### 修复
- **LogsView 浅色终端**：`.log-container` 背景从 `#1a1a2e`（深色终端）改为 `#f8f9fa`（浅色），日志文本颜色适配浅色背景（灰色日志、绿色 INFO、红色 ERROR）

---

## [26.3.18v6] — 2026-03-18 · 全站浅色主题恢复（Revert dark mode）

### 修复（回滚）
- **移除 `<html class="dark">`**：撤销 v3/v4/v5 错误引入的全局 dark class
- **移除 Element Plus dark CSS vars 导入**：`main.ts` 中删除 `element-plus/theme-chalk/dark/css-vars.css` 导入
- **AiChat.vue 恢复浅色**：`.msg-bubble.assistant { background:#fff; color:#303133 }`；输入区 `background:#fff; border-top:1px solid #e4e7ed`；chip/textarea 全部恢复浅色硬编码值
- **ChatHomeView.vue 恢复浅色**：toolbar `background:#fff`、chat-home `background:#f4f6f9`

---

## [26.3.18v5] — 2026-03-18 · 对话区颜色对比度调整（已被 v6 回滚）

### 修复
- 消息气泡对比度优化（使用 El Plus dark CSS vars，v6 已回滚）

---

## [26.3.18v4] — 2026-03-18 · AiChat 深色主题 + Token 输出修复（部分已被 v6 回滚）

### 修复
- **Token OutputTokens 为 0**：`runEventToJSON` 新增 `"usage"` case，将 usage RunEvent 序列化为 SSE 事件推送给前端；`"done"` 事件仅在 `ev.InputTokens > 0 && ev.OutputTokens > 0` 时才携带 token 字段，防止覆盖前端已累计的有效数据
- AiChat 深色主题（v6 已回滚，当前为浅色）

---

## [26.3.18v3] — 2026-03-18 · 高度链修复（height:100vh）

### 修复
- **`app-layout` 高度链**：`App.vue` 中 `.app-layout { height:100vh; overflow:hidden }` 替代 `min-height:100vh`，为子组件提供明确父高度，使 `flex:1` 的子元素能正确撑满视口
- **`app-right-container`**：移除 `height:0` 覆盖，改用默认 `align-items:stretch`，防止 flex 子项高度塌陷
- **`app-main.is-chat-page`**：`flex:1; min-height:0`（不再使用 `height:0`）

---

## [26.3.18v2] — 2026-03-18 · Token 用量 SSE 透传

### 新功能
- **Token 用量实时透传**：`runner.go` 新增 `RunEvent.InputTokens` / `OutputTokens` 字段；`EventUsage` 时累计 `totalInputToks` / `totalOutputToks` 并 emit `RunEvent{Type:"usage"}`；`runEventToJSON` 处理 `"usage"` 事件，前端实时更新每条消息的 token 计数

---

## [26.3.18v1] — 2026-03-18 · 侧边栏折叠修复 + 工具条折叠按钮

### 修复
- **侧边栏被遮挡**：`ChatHomeView.vue` 使用 `position:absolute` 覆盖了父容器，导致侧边栏不可见；改用正确的 flex 布局
- **侧边栏折叠按钮**：顶部工具条新增折叠/展开侧边栏按钮（`☰` / `✕`），响应式切换

---

## [26.3.17v1] — 2026-03-17 · 全新聊天首页 + CLI 子命令

### 新功能
- **全新聊天首页**：默认打开即聊天，侧边栏第一项为「聊天」，第二项为「仪表盘」
- **顶部工具条**：成员下拉选择器（含头像）、模型选择、历史会话（显示渠道/时间/消息数）、新对话按钮
- **派遣任务动画**：`agent_spawn` 触发时被派遣成员头像飞入顶部区域，橙灯=执行中/绿灯=完成/红灯=失败，悬浮显示进度气泡
- **历史会话自动加载**：选择历史对话后自动拉取完整消息记录
- **每条消息 token 显示**：每条助手消息底部显示 `↑ input ↓ output tokens`
- **CLI 子命令**：`zyhive token`、`zyhive start/stop/restart/status/enable/disable/help`

### 修复
- 历史会话选中后消息不显示（AiChat onMounted 补充 resumeSession 调用）
- 成员选择器改为下拉（原为头像列表）
- 配色统一深色系

### 以往版本
> 历史版本（v0.9.x – v0.10.x）采用语义化版本号，详见下方记录

---

## [26.3.17v1] — 2026-03-17 · 工具生态全面升级

### 修复
- **工具名称错误**：`policy.go` 中 `group:runtime` 的工具名 `bash` → 实际注册名为 `exec`，导致 deny/allow 策略对执行工具无效
- **AgentLister 未注册**：`pool.go` 未调用 `WithAgentLister`，导致 `agent_list` 工具不可用

---

## [26.3.17v1] — 2026-03-17 · agent_list 工具注册修复

### 修复
- **`agent_list` 工具缺失**：`pool.go configureToolRegistry()` 中未调用 `reg.WithAgentLister()`，导致 AI 成员无法通过工具查询同伴列表；现在注册时自动从 `manager.List()` 获取成员摘要注入

---

## [26.3.17v1] — 2026-03-17 · 工具调用全路径修复

### 修复
- **`SupportsTools` 三处缺失**：`pool.go` 中 `Run()`、`RunStreamEvents()`（无图片路径）、`RunStream()` 构造 `runner.Config` 时均未设置 `SupportsTools` 字段，导致这三条路径下工具调用始终为 `off`；`RunStreamEvents`（有图片路径）和子代理路径已正确设置，本次补全全部路径

---

## [26.3.17v1] — 2026-03-17 · 安装向导 + 类型修复

### 新增
- **`install.sh` 安装向导**（Linux / macOS）：交互式三步向导，支持 8 大 provider 预设，`--flag` 无人值守模式，安装完成后展示访问地址和 Token
- **`install.ps1` 安装向导**（Windows PowerShell）：等效功能，修复 `irm|iex` 管道崩溃，服务注册改用 `C:\ProgramData\ZyHive`

### 修复
- **TypeScript 类型错误**：`ui/src/api/index.ts` 补充 `HeartbeatConfig` interface，解决编译报错
- **Vue 模板插值冲突**：`ToolsView.vue` 中 `{{task}}` 占位符使用 `v-pre` 指令阻止 Vue 解析

---

## [26.3.17v1] — 2026-03-03 · MiniMax 探测修复

### 修复
- **MiniMax 测试返回 404**：MiniMax API 不支持 `GET /v1/models`（OpenAI 标准探测端点），改为 `POST /v1/chat/completions + max_tokens=1` 轻量探测；401/403 正确识别 Key 无效，其余 2xx/4xx 视为连接成功

---

## [26.3.17v1] — 2026-03-03 · Provider 测试修复

### 修复
- **MiniMax / Kimi / 智谱等厂商测试显示"未配置调用地址"**：`providers.go` 的 `Test()` 函数未对空 `baseURL` 做兜底，导致未显式填写转发地址时测试必然失败；现与 `models.go` 保持一致，自动补全已知厂商默认地址（`defaultBaseURLForProvider`）

---

## [26.3.17v1] — 2026-03-02 · Windows 安装脚本双修

### 修复
- **PowerShell `irm|iex` 崩溃**（`PropertyNotFoundException on .Path`）：`Set-StrictMode -Version Latest` 下，管道执行时 `$MyInvocation.MyCommand` 为 `ScriptBlock`，不含 `.Path` 属性，改用 `try/catch` 安全访问
- **Windows 服务注册失败**（`Start-Service: NoServiceFoundForGivenName`）：
  - 安装目录从 `C:\Program Files\ZyHive`（含空格）改为 `C:\ProgramData\ZyHive`，消除 `sc.exe binPath=` 引号解析问题
  - 改用 `cmd /c "sc create ..."` 代理调用，加 `$LASTEXITCODE` 检查，失败时明确报错而非静默跳过

---

## [26.3.17v1] — 2026-03-02 · 稳定性修复

### 修复
- **新实例登录死循环**：App.vue 中 `/api/update/check` 在未登录状态下被触发，返回 401 后拦截器跳转 `/login`，登录页再次触发检查形成无限刷新循环。修复方案：
  - `api.interceptors.response.use`：401 跳转前检查 `pathname`，已在 `/login` 则不再跳转
  - `App.vue`：update check 开头加 `if (!token) return`，未登录时跳过检查

---

## [v0.9.27] — 2026-02-28 · 全工具测试套件 + agent_spawn 始终注册

### 新增
- **58 个工具单元测试**（`pkg/tools/tools_test.go`）：覆盖所有内置工具的边界情况，包括：
  - `read`：正常读取 / offset+limit / 文件不存在 / 缺参数 / 非法 JSON / offset 超界
  - `write`：正常写 / 自动建目录 / 缺参数 / 空内容
  - `edit`：正常替换 / old_string 未找到（附文件预览和字节数提示）/ 文件不存在 / 只替换第一处
  - `exec`：成功 / 无输出提示 / 失败退出码保留 / stdout 不丢失 / 多行 / stderr 合并
  - `grep`：匹配 / 无匹配明确提示 / 非法正则 / 路径不存在 / 缺参数 / 递归
  - `glob` / `web_fetch` / `show_image` / `self_*` / `env_vars` / `agent_spawn` 系列

### 修复
- **`agent_spawn` 始终注册**：之前没有 SubagentManager 时工具根本不出现在工具列表，LLM 收到 "unknown tool" 完全不知道该工具存在；现在 `registerSubagentTools()` 在 `New()` 时就调用，无 manager 时执行返回明确的 "not configured" 错误
- **`agent_tasks` / `agent_kill` / `agent_result` 同步修复**：与 `agent_spawn` 同类问题，同步解决

---

## [v0.9.26] — 2026-02-28 · Cron 隔离会话 + 统一会话侧边栏 + 工具错误信息

### 新增
- **`send_message` 工具**（`pkg/tools/messaging.go`）：AI 成员可在隔离 session 中主动向 Telegram 渠道发消息，供 Cron 任务中的 delivery=none 模式使用
- **NO_ALERT 抑制**：Cron 任务输出以 `NO_ALERT` 开头时，自动跳过 announce delivery，减少无效推送
- **`memory_search` 工具**（`pkg/tools/memory_search.go`）：向量 + BM25 混合检索工作区 `memory/` 目录下的所有 `.md` 文件；无 embedding provider 时自动降级为纯 BM25；支持 `top_k` 参数（默认 5，最大 20）
- **Cron 隔离会话**：每次 Cron 任务执行都在独立 `sessionID = "cron-{jobID}-{runID}"` 的 session 中运行，不污染主对话历史

### 变更
- **统一会话侧边栏**（`AgentDetailView.vue`）：面板会话与 Telegram / Web 渠道会话合并为单一列表，按最后活动时间排序；面板会话保持交互式 AiChat 组件，渠道会话显示"此会话来自 Telegram，只读"横幅，去掉"历史对话"独立 Tab

### 修复
- **SubAgent API Key 解析**（`pkg/agent/pool.go`）：替换全部 5 处 `apiKey := modelEntry.APIKey` 为 `config.ResolveCredentials(modelEntry, cfg.Providers)`，修复 v3 config 格式下子代理报"no API key"的错误
- **在线更新版本比较**：改用语义化版本（semver）比较替代字符串比较，修复 v0.9.9 > v0.9.19 误判；同步修复 `App.vue` stale localStorage cache 导致新版本检测失效
- **工具错误信息精细化**：
  - `exec`：失败时返回 `❌ Command exited with code N.\n<output>` 作为 result 而非 Go error，确保 LLM 同时看到退出码和完整输出
  - `edit`：`old_string` 未找到时附带文件字节数 + 200 字符预览，提示检查空白字符
  - `web_fetch`：HTTP 4xx/5xx 返回 `"HTTP 404 Not Found\nURL: ...\nResponse: <snippet>"`
  - `grep`：区分 exit code 1（无匹配，返回 "No matches found for pattern X in Y"）与真实错误
  - `read`：明确区分"file not found"与其他 OS 错误
  - `registry.Execute`：所有错误加 `[toolname]` 前缀；unknown tool 时列出所有已注册工具名；`agent_spawn` 验证 agentId 是否在已知列表中

---

## [v0.9.25] — 2026-02-28 · 浏览器自动化 + memory_search + 版本更新角标

### 新增
- **浏览器自动化工具**（`pkg/browser/manager.go` + `pkg/tools/browser_tools.go`）：基于 go-rod（纯 Go，无 Node.js 依赖）的 16 个浏览器工具：
  - `browser_navigate` / `browser_snapshot` / `browser_screenshot` / `browser_click`
  - `browser_type` / `browser_fill` / `browser_press` / `browser_scroll` / `browser_select`
  - `browser_hover` / `browser_wait` / `browser_evaluate` / `browser_close`
  - ARIA 快照：JS 注入 `data-zy-ref` 属性标记所有可交互元素，生成结构化 ARIA 树
  - 每个 Agent 有独立 `AgentSession`（Tab 列表 + 当前激活 Tab），所有 Agent 共享同一 Rod 浏览器进程
  - 截图自动保存到 `{workspaceDir}/.browser_screenshots/screenshot_{timestamp}.png`
- **Chromium 自动下载**：首次使用浏览器工具时自动下载 Chromium，零系统依赖
- **版本更新角标**（Header）：后台定期检测 GitHub 最新 Release，有新版本时 Header 右上角显示橙色角标，点击跳转到设置页升级

### 修复
- **Web 面板在线升级进程残留**：改用 `syscall.Exec` 原地替换进程（PID 不变），配合 `Restart=always` 彻底解决升级后服务挂死问题

---

## [v0.9.24] — 2026-02-26 · 甘特图全面重构

### 新增
- **7 级时间颗粒度缩放**：年 → 季度 → 月 → 双周 → 周 → 天 → 小时，滚轮缩放无级切换
- **惯性平滑拖拽**：地图式连续交互，松手后惯性滑动，速度按屏幕宽度归一化（`maxV = screenW/400`）
- **今日线锚定**：初始视图以今日线为参考点，左侧 10% 位置显示
- **目标摘要面板**：点击甘特条弹出目标详情侧边栏
- **「← 甘特图」返回按钮**：目标详情编辑器工具栏新增返回按钮，快速切回甘特视图
- **时间进度条**：甘特条颜色填充按时间进度（`timeProgress`）而非手动填写的 `progress` 字段

### 修复
- **密集网格线 Bug**：根因为 `v-for` key 使用天数字导致跨月重复，改用时间戳 `ts` 作为 key
- **TICK_STEPS 大数字溢出**：月份常量使用 `Math.round(30.44 * 86400_000)` 替代 `| 0` 位运算（32位溢出导致负数）
- **甘特条宽度冻结**：起始日期滚动到左侧可视区外时条宽被截断，修复边界计算
- **快速滑动时间穿越**：限幅惯性速度防止极端滑动跳跃到遥远时间
- **双层标题**：年份（小字）在上，月/周（主标签）在下，不再堆叠显示
- **版本号显示**：去掉 Header 版本号前重复的 "v"
- **Star 按钮样式**：降低视觉权重

---

## [v0.9.23] — 2026-02-25 · Goals 聊天 session 隔离 + 面板高度修复

### 修复
- **目标聊天 session 隔离**：每次点击「新建目标」都生成新的 session，不再复用上次创建流程的聊天记录；每个已保存目标有独立 session，切换目标即切换历史对话
- **右侧聊天面板高度溢出**：`.goals-studio` 改用 `height: calc(100vh - 44px)` 并逃脱 `.app-main` padding，彻底解决聊天框超出窗口 100% 的问题

---

## [v0.9.22] — 2026-02-25 · 甘特图双层标题 + 滚轮缩放

### 新增
- **甘特图双层时间轴**：年份（小字，仅在年份切换处显示）在上，月/周数字（主标签）在下，不再出现"2026/3 2026/4..."紧凑堆叠
- **滚轮缩放颗粒度**：在甘特图区域滚动鼠标滚轮可在 4 个时间颗粒度之间切换：季度 ↔ 月 ↔ 双周 ↔ 周
- 左上角显示当前颗粒度提示（月/双周/周/季）

---

## [v0.9.21] — 2026-02-25 · 修复「获取可用模型」API Key 未传问题

### 修复
- **获取可用模型正确读取 Provider API Key**：点击「获取可用模型」时传入 `providerId`，后端从 `cfg.Providers` 中查找对应 API Key，不再依赖环境变量，修复提示「未配置 API Key」的错误

---

## [v0.9.20] — 2026-02-25 · zyling.ai 官网提交次数柱状图 + 镜像替换

### 新增 / 修复
- **官网柱状图显示具体次数**：zyling.ai 近 14 天 Commits 柱状图每根柱子顶部显示实际提交次数（0 提交天留空）
- **国内更新镜像替换**：`mirror.ghproxy.com`（已失效）→ `install.zyling.ai/dl`（自控 CF Worker，稳定可靠）
- **MiniMax 等 provider 模型列表兜底**：`/v1/models` 返回非 200 时自动回退内置模型列表（MiniMax / Zhipu / Kimi / Qwen）

---

## [v0.9.19] — 2026-02-25 · MiniMax 工具调用 400 修复

### 修复
- **OpenAI-compatible assistant 消息 `content: null` → `""`**：部分 provider（MiniMax 等）不接受 `content: null`，导致工具调用后续请求报 400「Messages with role tool must be a response to a preceding message with tool_calls」

---

## [v0.9.18] — 2026-02-25 · Config 迁移系统 + 工具调用模型标注

### 新增
- **Config 版本化迁移系统**：启动时自动执行 `applyMigrations()`，v0→v1 补全所有 ID/Status/默认值，v1→v2 自动标记不支持工具调用模型（`deepseek-reasoner` 等）并确保有默认模型
- **不支持工具调用模型前端警告**：模型选择器灰显 + 选中时显示警告提示
- **OpenAI-compatible 工具消息格式修复**：`tool_use` → `tool_calls`，`tool_result` → `role:"tool"` 独立消息，解决 DeepSeek 400 错误

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

## [v0.9.16] - 2026-02-25

### Fixed
- FetchModels `/v1` 重复拼接导致 DeepSeek/Kimi 等 OpenAI-compatible 接口 404
- Anthropic 客户端支持自定义转发地址，解决国内 IP 403 forbidden 问题

### Changed
- 模型提供商卡片 logo 换用 GitHub 官方 org 头像（真实品牌标识，统一 48×48 PNG）
- 修复 kimi/minimax logo 格式问题（JPEG→PNG），确保所有浏览器正确渲染

## [v0.9.17] - 2026-02-25

### Fixed
- 版本更新下载 404：文件名从 `aipanel-*` 修正为 `zyhive-*`
- 国内网络无法连接 GitHub 时自动切换 ghproxy 镜像下载
- 下载进度提示显示当前使用的下载源（GitHub / 国内镜像）
