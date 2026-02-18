# Session 管理设计

> 以 OpenClaw pi-coding-agent 为蓝图，适配 ai-panel 的 Go 实现

---

## 当前问题

| 问题 | 现状 |
|---|---|
| 无服务端会话 | 每次请求都靠客户端传 `req.History`，刷新即丢失 |
| Session 未使用 | `session.Store` 已接入 runner，但从未写入/读取历史 |
| 无 Compaction | `CompactionEntry` 类型有但未触发，长对话会打爆 token |
| 无会话列表 | 无法浏览/恢复历史对话 |

---

## 设计目标（以 OpenClaw 为参照）

```
客户端只发消息 + sessionId
服务端负责：会话创建 / 历史持久化 / Token 追踪 / 上下文压缩
```

---

## 存储结构

```
agents/{agentId}/sessions/
  sessions.json          # 索引文件（元数据，不含消息体）
  {sessionId}.jsonl      # JSONL 追加日志
```

### sessions.json
```json
{
  "sessions": {
    "ses-1708300000000": {
      "id": "ses-1708300000000",
      "agentId": "python-reviewer",
      "filePath": "ses-1708300000000.jsonl",
      "title": "分析 payment.py 的并发问题",
      "messageCount": 12,
      "createdAt": 1708300000000,
      "lastAt": 1708301234567,
      "tokenEstimate": 45000
    }
  }
}
```

### {sessionId}.jsonl 条目类型

```jsonl
{"type":"session","version":3,"agentId":"python-reviewer","createdAt":1708300000000}
{"type":"message","message":{"role":"user","content":"你好，介绍一下自己"},"timestamp":1708300001000}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"我是..."}]},"timestamp":1708300002000}
{"type":"compaction","summary":"用户询问了模型身份...前10轮已压缩","firstKeptEntryId":"msg-11","tokensBefore":80000,"tokensAfter":5000,"timestamp":1708300100000}
{"type":"message","message":{"role":"user","content":"继续分析这段代码"},"timestamp":1708300101000}
```

---

## API 设计

### Chat（主入口）
```
POST /api/agents/:id/chat
Body: {
  message: string,
  sessionId?: string,   // 续接已有会话；不传则创建新会话
  context?: string,
  images?: string[],
}

SSE Events → 同现在
最后一个 done 事件携带: { type:"done", sessionId:"ses-xxx", tokenEstimate: 12345 }
```

### 会话管理
```
GET    /api/agents/:id/sessions           → 会话列表（含元数据）
GET    /api/agents/:id/sessions/:sid      → 会话完整历史
DELETE /api/agents/:id/sessions/:sid      → 删除会话
PATCH  /api/agents/:id/sessions/:sid      → 改标题等
```

---

## Runner 流程（修改后）

```
1. 接收 sessionId（由 chat.go 生成或从请求读取）
2. 从 JSONL 加载历史 → r.history
   - 若有 compaction 条目：把 summary 注入为系统消息前缀
   - 只取 compaction 之后的消息
3. 追加用户消息 → r.history + 写 JSONL
4. LLM 循环（现有逻辑）
5. 每轮 assistant 回复 → r.history + 写 JSONL
6. 工具调用结果 → 写 JSONL（可选，用于审计）
7. 完成后：
   a. 估算 token 数，更新 sessions.json
   b. 若 tokenEstimate > 80000 → 触发 Compaction
```

---

## Compaction 逻辑

```
触发条件：tokenEstimate > 80,000

执行步骤：
1. 读取所有 message 条目
2. 找到保留边界（最近 20 轮）
3. 把边界前的消息发给 LLM，生成摘要
   System: "请将以下对话历史压缩为简洁摘要，保留关键信息"
4. 写入 CompactionEntry { summary, firstKeptEntryId, tokensBefore, tokensAfter }
5. 重写 sessions.json 中的 tokenEstimate

下次加载：
- 遇到 compaction 条目 → 把 summary 作为 [system] 前缀注入
- 只加载 firstKeptEntryId 之后的消息
```

---

## Token 估算

```go
// 粗算：每个字符约 0.25 token（中文约 0.5 token/字）
// 足够触发 compaction，不需要精确
func estimateTokens(messages []llm.ChatMessage) int {
    total := 0
    for _, m := range messages {
        total += len(m.Content) / 4
    }
    return total
}
```

---

## 实现阶段

### Phase 1（当前实现）
- [x] JSONL 存储结构（已有）
- [ ] session.Store 增加 `AppendMessage` / `ReadHistory` / `UpdateMeta`
- [ ] Runner 持久化每轮消息到 JSONL
- [ ] Chat API 生成并返回 sessionId
- [ ] sessions.json 索引追踪 title / lastAt / tokenEstimate

### Phase 2（下一步）
- [ ] Compaction 触发（80k token 阈值）
- [ ] 前端 sessionId 管理（发消息带 sessionId，无需传 history）
- [ ] 会话列表 UI（侧边栏切换历史对话）
- [ ] 会话标题自动生成（首条消息前30字）

---

## 与 OpenClaw 的对照

| 特性 | OpenClaw | ai-panel |
|---|---|---|
| Session key | `agent:main:telegram:group:-xxx` | `ses-{timestamp}` |
| 存储格式 | JSONL v3 | JSONL v3（相同）|
| 历史所有权 | 服务端 | 服务端（Phase 1） |
| Compaction | 自动，200k token | 自动，80k token（Phase 2） |
| 会话索引 | sessions.json | sessions.json（相同）|
| 多渠道会话 | 每个渠道独立 session | agentId:sessionId 隔离 |
