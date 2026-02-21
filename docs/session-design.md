# Session 管理设计

> 以 OpenClaw 为蓝本，适配 ZyHive 的 Go 实现。本文档反映 v0.9.0 已实现状态。

---

## 存储结构（已实现）

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
      "agentId": "xiuliu",
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
{"type":"session","version":3,"agentId":"xiuliu","createdAt":1708300000000}
{"type":"message","message":{"role":"user","content":"你好"},"timestamp":1708300001000}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"你好！"}]},"timestamp":1708300002000}
{"type":"compaction","summary":"前10轮已压缩...","firstKeptEntryId":"msg-11","tokensBefore":80000,"tokensAfter":5000,"timestamp":1708300100000}
```

---

## API（已实现）

```
POST   /api/agents/:id/chat                → SSE 流式对话，返回 sessionId
GET    /api/agents/:id/sessions            → 会话列表（含元数据）
GET    /api/agents/:id/sessions/:sid       → 完整历史消息
DELETE /api/agents/:id/sessions/:sid       → 删除会话
PATCH  /api/agents/:id/sessions/:sid       → 修改标题
```

Chat 请求：
```json
{
  "message": "帮我看看这段代码",
  "sessionId": "ses-xxx",   // 续接已有会话；不传则创建新会话
  "context": "",
  "images": []
}
```

SSE done 事件携带：
```json
{ "type": "done", "sessionId": "ses-xxx", "tokenEstimate": 12345 }
```

---

## Runner 流程（已实现）

```
1. chat.go 生成或接收 sessionId
2. Runner 从 JSONL 加载历史 → r.history
   - 遇到 compaction 条目：把 summary 作为 [system] 前缀注入
   - 只加载 firstKeptEntryId 之后的消息
3. 追加用户消息 → r.history + 写 JSONL
4. LLM 流式循环（工具调用支持）
5. 每轮 assistant 回复 → r.history + 写 JSONL
6. 完成后更新 sessions.json（messageCount / lastAt / tokenEstimate）
7. tokenEstimate > 80,000 → 自动触发 Compaction
```

---

## Compaction（已实现）

```
触发条件：tokenEstimate > 80,000

步骤：
1. 读取所有 message 条目
2. 保留最近 20 轮
3. 边界前的消息发给 LLM 生成摘要
4. 写入 CompactionEntry（summary / firstKeptEntryId / tokensBefore / tokensAfter）
5. 更新 sessions.json 中的 tokenEstimate

下次加载：
- 遇到 compaction → summary 作为 [system] 前缀注入
- 只加载 firstKeptEntryId 之后的消息
```

---

## 历史对话系统（ConvLog，已实现）

渠道历史（与 session 完全隔离）：

```
agents/{agentId}/convlogs/
  telegram-{chatId}.jsonl    # Telegram 渠道历史
  web-{channelId}.jsonl      # Web 渠道历史
```

- 管理员通过 ChatsView 查看全部渠道历史，支持按渠道/成员筛选
- Agent 侧对话历史与 convlog 完全隔离（convlog 仅管理员可读）

---

## 与 OpenClaw 对照

| 特性 | OpenClaw | ZyHive |
|------|---------|--------|
| Session key | `agent:main:telegram:group:-xxx` | `ses-{timestamp}` |
| 存储格式 | JSONL v3 | JSONL v3（相同）|
| 历史所有权 | 服务端 | 服务端 ✅ |
| Compaction | 自动，200k token | 自动，80k token ✅ |
| 会话索引 | sessions.json | sessions.json ✅ |
| 多渠道会话 | 每个渠道独立 session | agentId:sessionId 隔离 ✅ |
| 渠道历史 | convlog | convlogs/ ✅ |
