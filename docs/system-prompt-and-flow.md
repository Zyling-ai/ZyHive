# 系统提示词构建与对话流程

> 适用版本：26.4.22v1+

---

## 一、系统提示词构建（BuildSystemPrompt）— 渐进式披露

每次对话开始前，`pkg/runner/system_prompt.go::BuildSystemPrompt()` 按以下**严格分层**动态组装系统提示词。核心设计：**轻量首层 + 按需深读**。

```
层 1 · 当下信息（always）
    ↓   · 日期/时间/周数/年度第 N 天（Asia/Shanghai）
    ↓   · Platform: ZyHive ...
    ↓   · 训练截止警告（涉及时事主动 web_search）
    ↓   · wish_add 使用提示
    ↓   · 档位 hashtag 约定（#简答 / #深思考 / #写代码 / #闲聊 / #急）

层 2 · Owner 档案（optional, per-agent）
    ↓   memory/core/owner-profile.md  (上版兼容 user-profile.md)

层 3 · Agent 自我身份
    ↓   IDENTITY.md · SOUL.md

层 4 · 记忆索引
    ↓   memory/INDEX.md（轻量，完整记忆需 read 四层子目录）

层 5 · 通讯录（渐进式披露三层）
    ↓   network/INDEX.md      — 轻量列表（所有联系人名片）
    ↓   network/RELATIONS.md  — 关系表（双向同步）
    ↓   当前对话对方摘要      — 运行时 network.Store.Summary 注入 ExtraContext

层 6 · 已装技能索引
    ↓   skills/INDEX.md

层 7 · 历史对话索引
    ↓   conversations/INDEX.md

层 8 · 工作区指令
    ↓   AGENTS.md（及其引用的其它文件链）

层 9 · Capabilities（通过 Config 注入）
    ↓   工具体检（ready / blocked）
    ↓   WISHLIST 头部（AI 自己记的诉求）

层 10 · 共享项目工作区（如有）
    ↓   BuildProjectContext(ag.ID)
```

### 渐进式披露的含义

> **"不预喂信息，按需索取"**

- 层 4/5/6/7 都是 **`INDEX.md` 首层轻量注入**（每个 ~500 chars）
- AI 看到索引知道"谁存在 / 哪里有什么"
- 真正需要深度信息时用 **通用 `read` 工具**按需读取完整文件
- **优点**：不预占未来对话 token · 老数据不膨胀 prompt · 文件式存储 AI 可 `edit` 也可 `read`

### 截断保护（`truncateForPrompt`）

任一层注入文件超过 **20,000 字符**时自动截断：
- 保留头部 **70%**（重要指令集中在开头）
- 保留尾部 **20%**（最新内容）
- 中间插入 `[...内容已截断（原文件 N 字符），完整内容请用 read 工具读取: <文件名>...]` 标记

### Agent 工作区完整结构

```
{workspace}/
├── IDENTITY.md              ← 客观身份档案（名称/职责/所属团队）
├── SOUL.md                  ← 主观灵魂（性格/准则/语言风格）
├── AGENTS.md                ← 工作区级指令文档
├── WISHLIST.md              ← AI 主动表达的能力诉求（wish_add 写入）
├── memory/
│   ├── INDEX.md             ← 轻量记忆索引（注入系统提示词）
│   ├── core/
│   │   ├── owner-profile.md ← 主人档案（我服务于谁）
│   │   ├── personality.md   ← 性格特质与偏好
│   │   ├── knowledge.md     ← 领域知识与经验
│   │   └── relationships.md ← 人际关系记录
│   ├── projects/            ← 项目相关记忆
│   ├── daily/               ← 每日日志（短期记忆，含 notes-to-user.md）
│   └── topics/              ← 主题归档
├── network/                 ← ★ 通讯录（26.4.22v1 新增）
│   ├── INDEX.md             ← 通讯录轻量索引（注入系统提示词）
│   ├── INDEX.json           ← 机器索引（UI 读）
│   ├── RELATIONS.md         ← 关系表（双向同步 · 含 toKind 字段）
│   ├── changes.log          ← AI 改动审计日志（network_note 每次追加）
│   └── contacts/
│       ├── feishu-ou_abc.md ← 飞书联系人完整档案（read 按需）
│       ├── telegram-123.md
│       └── web-sid-xxx.md
├── skills/
│   ├── INDEX.md             ← 技能索引
│   └── {skillId}/
│       ├── skill.json       ← 技能元数据
│       └── SKILL.md         ← 技能内容（注入系统提示词）
└── conversations/
    ├── INDEX.md             ← 历史对话索引
    └── {sessionId}__{channelId}.jsonl  ← 完整对话记录
```

### 迁移兼容

Agent 启动时 `manager.go` 自动跑两个迁移（idempotent，零数据丢失）：

1. `network.MigrateIfNeeded(wsDir)`：
   - `workspace/RELATIONS.md` → `workspace/network/RELATIONS.md`
   - `memory/core/user-profile.md` → `memory/core/owner-profile.md`
2. `memory.MigrateFromFlatMemory`：
   - 老的 flat `MEMORY.md` → 分层 `memory/{core,projects,daily,topics}`

所有 Go 代码读 RELATIONS.md 时**优先 network/，fallback 根部**，保障过渡期不破。

---

## 二、通讯录 / 联系人系统（network/）

### 核心概念：Contact

> 每个 agent 一本私有通讯录。消息来自哪个渠道 / 哪个发送者，就自动在 `network/contacts/` 里建一个档案。

**Contact ID** 规范形式：`{source}:{externalId}`
- `feishu:ou_abc123`
- `telegram:123456789`
- `web:sid-xxxxx`（visitor sessionToken）
- `panel:*`（面板用 owner-profile 不走 contact）

**Contact 档案（markdown + YAML-ish frontmatter）**：

```markdown
---
id: feishu:ou_abc
source: feishu
externalId: ou_abc
displayName: 张三
tags:
  - 客户
  - 合作伙伴
aliases: []
primary: true
isOwner: false
createdAt: 2026-04-21T08:00:00Z
lastSeenAt: 2026-04-21T18:30:00Z
msgCount: 47
---
# 张三

## 事实
- 公司 A 法务合伙人
- 在北京

## 偏好（AI 观察）
- 简短直给

## 最近话题
- 2026-04-20 讨论 xxx

## 待跟进
-
```

### resolveContact 入口

4 处消息入口都接 `network.NewStore(wsDir).Resolve(source, externalId, displayName)`：

| 入口 | 源 | 位置 |
|------|----|----|
| 面板 | 不建 contact（owner 本人） | `internal/api/chat.go` |
| Telegram | `telegram:{userId}` | `pkg/channel/telegram.go::generateAndSend` |
| 飞书 | `feishu:{open_id}` | `pkg/channel/feishu.go` |
| Web Public | `web:{sessionToken}` | `internal/api/public_chat.go::runPublic` |

**Resolve 语义**：upsert — 存在则 `LastSeenAt` + `MsgCount++`，不存在则建空档案 + frontmatter。

### AI 主动工具：`network_note`

```go
network_note(entityId, section, text)
```

- `section` 枚举：`事实` / `偏好` / `最近话题` / `待跟进`
- 原子 append 到对应 `## 段` 下；段不存在自动补建
- 首次写入时清除占位符 `- (AI 通过 network_note 工具追加此处)`
- 旁路 `network/changes.log` 审计

### 运行时摘要注入（层 2）

每次 channel 消息进来，除了 Resolve 外还会调用：

```go
summary := store.Summary(contactID)  // ~300 chars
// 通过 runner.Config.ExtraContext 传入
```

`Summary` 含：
- 姓名 / 来源 / tags
- 累计对话次数 + 最后时间
- 事实前 3 条 + 偏好前 2 条
- 提示 `[完整档案 read("network/contacts/<file>.md")]`

---

## 三、对话主循环（Runner）

`pkg/runner/runner.go` 实现多轮工具调用自动循环：

```
用户消息
    ↓
构建 System Prompt（分层，见第一节）
    ↓
调用 LLM（StreamChat）→ StreamEvent 流
    ↓ text_delta → 实时推送文本给前端
    ↓ tool_call  → 并行执行工具（含 tool_call_id 精准匹配）
    ↓ usage      → 累计 Token 用量（UsageRecorder 写库）
    ↓ stop
    ↓
有工具调用？
  是 → 将工具结果注入对话历史 → 重新调用 LLM（最多 N 轮）
  否 → 对话完成，推送 done 事件（含总 Token 用量）
```

### RunEvent 类型

| Type | 内容 | 前端行为 |
|------|------|---------|
| `text_delta` | 文本增量 | 追加到气泡 |
| `tool_call` | 工具调用（ID/名称/参数） | 展示折叠卡（呼吸灯） |
| `tool_result` | 工具结果（带 ToolCallID 精准匹配） | 关闭对应卡片显示耗时 |
| `usage` | InputTokens / OutputTokens | 更新 Token 计数 |
| `thinking` | 推理过程文本 | 折叠显示 |
| `done` | 对话完成（含总 Token） | 标记完成状态 |
| `error` | 错误信息 | 显示错误气泡 |

---

## 四、会话工作者架构（SessionWorker）

`pkg/session/worker.go` 实现 HTTP 解耦的独立会话协程：

```
HTTP POST /chat → 构建 RunFn → 放入 SessionWorker.inputChan
                               SessionWorker（后台协程）
                                    ↓
HTTP SSE 连接  → 订阅 Broadcaster ← RunFn 执行 → RunEvent → Broadcaster
（可随时断开/重连）                   （独立于 HTTP 连接）
```

- **WorkerPool**：懒加载，每个 sessionID 独占一个 Worker
- **空闲超时**：30 分钟无请求自动关闭协程
- **队列容量**：8（防止过载）
- **断线续传**：重连后订阅同一 Broadcaster，获取已缓冲事件

---

## 五、Capabilities Context（工具体检 + WISHLIST）

`pkg/agent/capabilities.go::BuildCapabilitiesContext` 在每次对话构造 `runner.Config.CapabilitiesContext`：

```
--- 你当前可用的工具 (实时体检) ---
✅ 可用 N 个（按分组）:
  • 文件/命令: read, write, edit, exec, grep, glob, process
  • 浏览器: browser_navigate, browser_screenshot, ...
  • 通讯录: network_note
  • 愿望清单: wish_add, wish_list
  • 派遣协作: agent_list, agent_spawn, agent_tasks, agent_result
  • ...

⚠️ 当前不可用的工具 M 个:
  • web_search: 未配置 Brave Search API Key
    → 涉及时事时诚实告知用户"需要配置 web_search key"
  • image: 当前模型非视觉模型
    → 需用户切换到支持 vision 的模型

📋 关键约束:
  • agent_spawn 必须派 RELATIONS.md 中有记录的成员

--- 你之前记录的愿望 (WISHLIST.md 头部) ---
1. [P0] 联网搜索 - 信息是一切的基础...
2. [P1] 主动时间唤醒 - 让我能被定时推醒
（共 X 条，完整列表用 wish_list 工具读取）
```

**效果**：AI 不再猜"我应该有什么工具"，而是**从 runtime 状态读取真实能力清单**。

---

## 六、Provider 适配层（LLM）

`pkg/llm/` 目录下各 Provider 均实现同一接口：

```go
type Client interface {
    StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}
```

各 Provider 将私有 SSE/流式协议转换为统一 `StreamEvent`：

| Provider | 文件 | 特殊处理 |
|----------|------|---------|
| Anthropic | `anthropic.go` | `message_delta` 的 output_tokens 从顶层 `event.Usage` 读（不是 `event.Delta`） |
| OpenAI 兼容 | `base_openai.go` | 通用实现，支持自定义 baseUrl |
| MiniMax | `minimax.go` | POST /chat 探测（不支持 GET /models） |
| DeepSeek | `deepseek.go` | OpenAI 兼容适配 |
| 智谱 AI | `zhipu.go` | 自定义鉴权头 |
| Moonshot | `moonshot.go` | Kimi 系列适配 |
| Qwen | `qwen.go` | 通义千问适配 |
| OpenRouter | `openrouter.go` | 聚合代理 |

---

## 七、Cron 自主唤醒 + NO_ALERT 静默

`pkg/cron/engine.go` 实现 per-agent 定时任务 + 隔离 session：

- 每次 cron 触发 **新建独立 session**（不污染主对话历史）
- Payload `message` 是发给 agent 的 prompt
- 若 agent 回复以 **`NO_ALERT`** 开头或等于该 token → 结果**记录但不推送**给 delivery（announce/Telegram 等）
- CronView 提供「🌅 晨间例行」一键模板，预置如下 prompt：
  ```
  晨间例行：
  1. 扫描昨天的对话 → memory/core/
  2. 检查 WISHLIST.md 看有没有进展
  3. 若有值得告诉用户的事 → memory/daily/notes-to-user.md
  4. 若今天没有值得汇报的事，请只回一个单词：NO_ALERT
  ```

**配合档位：cron expr 本身就是频率预算，不需要额外 autonomy budget 字段。**
