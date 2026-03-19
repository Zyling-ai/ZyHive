# 系统提示词构建与对话流程

> 适用版本：26.3.18v8+

---

## 一、系统提示词构建（BuildSystemPrompt）

每次对话开始前，`pkg/runner/system_prompt.go` 的 `BuildSystemPrompt()` 函数按以下顺序动态组装系统提示词：

```
当前时间（Asia/Shanghai 时区）
    ↓
IDENTITY.md（客观身份档案）
    ↓
SOUL.md（主观性格灵魂）
    ↓
memory/INDEX.md（记忆索引，轻量注入）
    ↓
RELATIONS.md（团队关系图谱，可选）
    ↓
skills/INDEX.md（已安装技能索引，可选）
    ↓
conversations/INDEX.md（历史对话索引，可选）
    ↓
AGENTS.md（工作区指令文档，可选）
    ↓
共享项目工作区（BuildProjectContext，可选）
```

### 截断保护

注入文件超过 **20,000 字符**时自动截断：
- 保留头部 **70%**（重要指令集中在开头）
- 保留尾部 **20%**（最新内容）
- 中间插入 `[...内容已截断...]` 标记
- 完整内容可通过 `read` 工具读取

### 记忆树结构

```
{workspace}/
├── IDENTITY.md          ← 客观身份档案（名称、职责、所属团队）
├── SOUL.md              ← 主观灵魂（性格、准则、语言风格）
├── RELATIONS.md         ← 团队关系图谱（上下级/平级协作/支持）
├── AGENTS.md            ← 工作区级指令文档
├── memory/
│   ├── INDEX.md         ← 轻量索引（注入系统提示词）
│   ├── core/
│   │   ├── personality.md   ← 性格特质与偏好
│   │   ├── knowledge.md     ← 领域知识与经验
│   │   └── relationships.md ← 人际关系记录
│   ├── projects/        ← 项目相关记忆
│   ├── daily/           ← 每日日志（短期记忆）
│   └── topics/          ← 主题归档
├── skills/
│   ├── INDEX.md         ← 技能索引
│   └── {skillId}/
│       ├── skill.json   ← 技能元数据
│       └── SKILL.md     ← 技能内容（注入系统提示词）
└── conversations/
    ├── INDEX.md         ← 历史对话索引
    └── {sessionId}__{channelId}.jsonl  ← 完整对话记录
```

---

## 二、对话主循环（Runner）

`pkg/runner/runner.go` 实现多轮工具调用自动循环：

```
用户消息
    ↓
构建 System Prompt
    ↓
调用 LLM（StreamChat）→ StreamEvent 流
    ↓ text_delta → 实时推送文本给前端
    ↓ tool_call  → 并行执行工具
    ↓ usage      → 累计 Token 用量
    ↓ stop
    ↓
有工具调用？
  是 → 将工具结果注入对话历史 → 重新调用 LLM（最多 N 轮）
  否 → 对话完成，推送 done 事件（含总 Token 用量）
```

### RunEvent 类型

| Type | 内容 | 前端行为 |
|------|------|---------|
| `text` | 文本增量 | 追加到气泡 |
| `tool_call` | 工具调用（名称/参数/结果） | 展示折叠卡 |
| `usage` | InputTokens / OutputTokens | 更新 Token 计数 |
| `thinking` | 推理过程文本 | 折叠显示 |
| `done` | 对话完成（含总 Token） | 标记完成状态 |
| `error` | 错误信息 | 显示错误气泡 |

---

## 三、会话工作者架构（SessionWorker）

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

## 四、Provider 适配层（LLM）

`pkg/llm/` 目录下各 Provider 均实现同一接口：

```go
type Client interface {
    StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}
```

各 Provider 将私有 SSE/流式协议转换为统一 `StreamEvent`：

| Provider | 文件 | 特殊处理 |
|----------|------|---------|
| Anthropic | `anthropic.go` | 双 EventUsage 分段累计 |
| OpenAI 兼容 | `base_openai.go` | 通用实现，支持自定义 baseUrl |
| MiniMax | `minimax.go` | POST /chat 探测（不支持 GET /models） |
| DeepSeek | `deepseek.go` | OpenAI 兼容适配 |
| 智谱 AI | `zhipu.go` | 自定义鉴权头 |
| Moonshot | `moonshot.go` | Kimi 系列适配 |
| Qwen | `qwen.go` | 通义千问适配 |
| OpenRouter | `openrouter.go` | 聚合代理 |
