# ZyHive 向 OpenClaw 借鉴清单

> 基于对 OpenClaw 架构的深度拆解，梳理 ZyHive 可落地的改进点。
> 按优先级排序：🔴 高优先 / 🟡 中优先 / 🟢 低优先

---

## 一、渐进式提示词披露（最核心）

### 现状
ZyHive 目前系统提示词构建较简单，工作区文件、Skills 全量注入，没有大小保护，没有分级策略。

### 借鉴内容

#### 🔴 1.1 工作区文件截断保护
OpenClaw 对每个注入文件设置 **20,000 字符上限**，超出时采用：
- 保留头部 70%（最重要的指令）
- 保留尾部 20%（最新的笔记/变更）
- 中间插入 `[...truncated, read FILENAME for full content...]`

**ZyHive 实施位置：** `pkg/runner/runner.go` → `buildSystemPrompt()`

```go
const maxFileChars = 20000
const headRatio = 0.7
const tailRatio = 0.2

func truncateFile(content string) string {
    if len(content) <= maxFileChars {
        return content
    }
    head := int(float64(maxFileChars) * headRatio)
    tail := int(float64(maxFileChars) * tailRatio)
    return content[:head] + "\n\n[...已截断，完整内容请用 read 工具读取...]\n\n" + content[len(content)-tail:]
}
```

---

#### 🔴 1.2 Skills：只注入目录索引，内容按需加载

**OpenClaw 做法：**
```xml
<available_skills>
  <skill>
    <name>git-commit</name>
    <description>Create conventional git commits</description>
    <location>/path/to/skill/SKILL.md</location>
  </skill>
</available_skills>
```
系统提示词只注入 name + description，Agent 判断需要时再 `read SKILL.md`。

**ZyHive 现状：** Skills 内容直接全量注入，每次都消耗大量 Token。

**ZyHive 实施：**
- 系统提示词只注入 Skills XML 目录
- 加一条规则："匹配到 1 个 Skill 时，先 read 其 SKILL.md 再执行"
- Token 消耗从 O(全部Skills内容) → O(目录大小)

---

#### 🔴 1.3 子 Agent 最小化提示词（PromptMode）

**OpenClaw：** 子 Agent 只拿到 AGENTS.md + TOOLS.md，SOUL.md、USER.md、MEMORY.md **强制过滤**。

**原因：**
1. 防止个人数据通过子 Agent 泄露（群聊/第三方场景）
2. 子 Agent 任务明确，不需要人格/主人信息
3. 节省 Token

**ZyHive 实施位置：** `pkg/runner/runner.go`

```go
type PromptMode string
const (
    PromptModeFull    PromptMode = "full"    // 主 Agent（直接对话）
    PromptModeMinimal PromptMode = "minimal" // 子 Agent（sessions_spawn 创建）
    PromptModeNone    PromptMode = "none"    // 纯 LLM 调用
)

// minimal 模式下只保留 AGENTS.md + TOOLS.md
var subAgentAllowlist = map[string]bool{
    "AGENTS.md": true,
    "TOOLS.md":  true,
}
```

---

#### 🟡 1.4 工具清单：单行摘要 + Schema 分离

**OpenClaw：** 系统提示词中每个工具只有一行描述，完整参数 Schema 由框架层注入给 LLM。

```
- read: Read file contents
- write: Create or overwrite files
- exec: Run shell commands (pty available for TTY-required CLIs)
```

**优势：** 用户可见提示词简洁，但 LLM 仍能看到完整 Schema（通过 tools 字段）。

**ZyHive 现状：** 工具 description 已经在 ToolDef 里，但系统提示词里的工具说明可以更精简。

---

## 二、会话与记忆隔离

### 🔴 2.1 主 Agent / 子 Agent 记忆隔离

| 文件 | 主 Agent | 子 Agent |
|------|---------|---------|
| AGENTS.md | ✅ | ✅ |
| TOOLS.md | ✅ | ✅ |
| SOUL.md | ✅ | ❌ 禁止 |
| USER.md | ✅ | ❌ 禁止 |
| MEMORY.md | ✅ | ❌ 禁止 |
| IDENTITY.md | ✅ | ❌ 禁止 |

**意义：** 子 Agent 被委托执行特定任务，不需要主人信息，也不应该暴露。
当子 Agent 被用于群聊、对外服务时，这是**安全隔离边界**。

**ZyHive 实施：** `pkg/runner/runner.go` → `buildSystemPrompt()` 增加 `isSubAgent bool` 参数，过滤注入文件列表。

---

## 三、多 API Key 轮换与故障转移

### 🔴 3.1 Auth Profile 机制

**OpenClaw：** 同一个 Provider 可以配置多个 API Key（Auth Profile），支持：
- 轮询负载均衡
- 单个 Key Rate Limit 后自动切换到下一个
- 冷却时间（被 429 的 Key 等待一段时间再重用）

**ZyHive 现状：** 单 Provider 单 Key，被 Rate Limit 直接报错。

**ZyHive 实施方案（ModelEntry 扩展）：**
```json
{
  "id": "claude-primary",
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "apiKeys": [
    "sk-ant-xxx1",
    "sk-ant-xxx2",
    "sk-ant-xxx3"
  ]
}
```

在 `pkg/llm/anthropic.go` 增加 Key 轮换逻辑：收到 429 → 切到下一个 Key → 记录冷却时间。

---

## 四、上下文窗口管理

### 🟡 4.1 自动检测上下文长度并触发 Compaction

**OpenClaw：** 运行前检测历史对话长度，当接近模型上下文窗口时：
- 触发摘要压缩（Compaction）：把历史对话压缩成摘要
- 压缩后的摘要作为新的起点继续对话

**ZyHive 现状：** 没有自动 Compaction，历史对话会无限增长，最终触发 API 上下文超出错误。

**实施思路：**
```go
// 在 runner.go agentic loop 前检查
func (r *Runner) checkContextAndCompact(ctx context.Context) error {
    // 估算当前 history token 数（粗略：字符数 / 4）
    totalChars := estimateChars(r.history)
    if totalChars > r.cfg.MaxContextChars * 0.8 { // 超过 80% 时压缩
        return r.compactHistory(ctx)
    }
    return nil
}
```

---

### 🟡 4.2 运行时信息紧凑编码

**OpenClaw：** 系统提示词最后一行是紧凑的运行时 KV：
```
Runtime: agent=main | host=my-server | model=anthropic/claude-opus-4-5 | channel=telegram | thinking=off
```

**优势：** Token 极少，但给了 Agent 完整运行时上下文。

**ZyHive 现状：** 运行时信息散落在提示词各处，或者根本没有。

**建议：** 在系统提示词最后固定加一行 Runtime 信息，格式参考 OpenClaw。

---

## 五、特殊 Token 与心跳节流

### 🟡 5.1 NO_REPLY Token

**OpenClaw：** Agent 无话可说时返回纯字符串 `NO_REPLY`，Gateway 拦截后不向用户发送消息，不消耗任何消息配额。

**ZyHive 现状：** 已部分实现（session worker 层面），但可以做得更严格：
- 只有整条消息等于 `NO_REPLY` 时才拦截
- 不能混在正文里出现

---

### 🟡 5.2 HEARTBEAT_OK 机制

心跳是定期触发的轻量检查（每 30 分钟）：
- 检查有无紧急邮件/日历/通知
- 无需行动时返回 `HEARTBEAT_OK`（不发消息给用户）
- 避免无意义打扰

**ZyHive 现状：** 定时任务系统已有，可以在 Cron 层面加 HEARTBEAT_OK 支持。

---

## 六、浏览器 + 沙箱（中长期）

### 🟢 6.1 Computer Use / 沙箱执行

**OpenClaw：** Agent 可以控制真实浏览器（Playwright）完成网页操作，这是"Computer Use"能力的基础。

**ZyHive 路线图：** 可作为 v1.0+ 的特性，需要：
- Playwright 依赖（或调用系统浏览器）
- 沙箱隔离（Docker 或 macOS 沙箱）
- 截图/视频 Artifact 收集

---

## 七、总结：按优先级的实施路线图

### 近期（v0.10 / v0.11）
| # | 项目 | 影响 | 工作量 |
|---|------|------|------|
| 1 | 工作区文件截断保护 | 防止大文件撑爆上下文 | 小 |
| 2 | Skills 目录化（只注入索引） | Token 节省 60%+ | 中 |
| 3 | 子 Agent 文件过滤（安全隔离） | 安全 + Token | 小 |
| 4 | Runtime 紧凑行 | Agent 自我感知 | 小 |

### 中期（v1.0）
| # | 项目 | 影响 | 工作量 |
|---|------|------|------|
| 5 | 多 API Key 轮换 | 高并发/稳定性 | 中 |
| 6 | 上下文自动 Compaction | 长对话不崩 | 中 |
| 7 | PromptMode 三级分层 | 架构整洁 | 中 |

### 长期（v2.0）
| # | 项目 | 影响 | 工作量 |
|---|------|------|------|
| 8 | 浏览器控制 / Computer Use | 差异化能力 | 大 |
| 9 | Memory 向量搜索 | 超长期记忆 | 大 |

---

## 八、ZyHive vs OpenClaw 能力对比

| 能力 | OpenClaw | ZyHive 现状 | 差距 |
|------|---------|------------|------|
| 渐进式提示词 | ✅ 完整 | ⚠️ 基础 | 中 |
| Skills 目录化 | ✅ | ❌ 全量注入 | 高优 |
| 子 Agent 隔离 | ✅ | ⚠️ 无文件过滤 | 高优 |
| 文件截断保护 | ✅ 20K限制 | ❌ 无限制 | 高优 |
| 多 Key 轮换 | ✅ | ❌ | 中优 |
| 上下文压缩 | ✅ 自动 | ❌ 手动 | 中优 |
| 浏览器控制 | ✅ Playwright | ❌ | 低优 |
| 向量记忆 | ✅ SQLite-vec | ❌ | 低优 |
| 多频道支持 | ✅ 12+ | ✅ Telegram | 持续 |
| Web 面板 | ✅ Lit | ✅ Vue 3 | 相当 |
| 团队关系图谱 | ❌ | ✅ | ZyHive 领先 |
| 可视化 Agent 管理 | ❌ | ✅ | ZyHive 领先 |

---

*文档生成时间：2026-02-25*
*参考：OpenClaw 架构分析（src/agents/system-prompt.ts, workspace.ts, attempt.ts, bootstrap.ts）*
