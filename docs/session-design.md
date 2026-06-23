# Session 管理设计

> 适配 ZyHive 的 Go 实现。本文档反映截至 **26.5.16v1** 的已实现状态。
> （历史版本：26.4.22v1 首版；本次按代码现状校正了 Compaction 阈值/时机、摘要注入角色、会话删除/改名路由、飞书历史存储位置等。）

---

## 存储结构（已实现）

```
agents/{agentId}/sessions/
  sessions.json              # 索引文件（元数据，不含消息体）
  {sessionId}.jsonl          # JSONL 追加日志
  subagent/                  # 子成员（subagent）会话子目录
```

会话 ID 前缀决定 `source`（见 `pkg/session/store.go` GetOrCreate）：

- `ses-{timestamp}` → 面板（web）
- `feishu-{chatId}` → 飞书
- `telegram-{chatId}` / `tg-{chatId}` → Telegram
- `web-{channelId}-{token}` → Web 公开聊天（每访客一条）

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
      "tokenEstimate": 45000,
      "source": "web",
      "active": false,
      "titleOverridden": false,
      "titledAtMsgCount": 4
    }
  }
}
```

> `source` / `active` / `titleOverridden` / `titledAtMsgCount` 为渠道识别、Reaper 保护、自动主题命名服务（见下文）。

### {sessionId}.jsonl 条目类型

```jsonl
{"type":"session","version":3,"agentId":"xiuliu","createdAt":1708300000000}
{"type":"message","message":{"role":"user","content":"你好"},"timestamp":1708300001000}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"你好！"}]},"timestamp":1708300002000}
{"type":"compaction","summary":"前若干轮已压缩...","firstKeptEntryId":"turn-42","tokensBefore":52000,"tokensAfter":5000,"timestamp":1708300100000}
```

> `firstKeptEntryId` 当前写为合成边界值 `"turn-{boundary}"`（`pkg/session/compaction.go`），**仅作记录用途**；实际加载边界由 `ReadHistory` 通过「遇到 compaction 条目即清空、只保留其后的 message」实现，并不解析该字段（见「Compaction」）。
> message 条目可携带 `toolCalls`（仅供 UI 时间线展示，不发送给 LLM）。

---

## API（已实现）

```
# agent 子路径（internal/api/router.go）
POST   /api/agents/:id/chat                → SSE 流式对话，返回 sessionId
GET    /api/agents/:id/chat/stream         → SSE 重连：订阅 Broadcaster 续传
GET    /api/agents/:id/chat/status         → 轮询当前会话运行状态
GET    /api/agents/:id/sessions            → 会话列表（裸数组）
GET    /api/agents/:id/sessions/:sid       → 完整历史消息
GET    /api/agents/:id/sessions/:sid/tool-audit → 工具调用审计
GET    /api/agents/:id/conversations       → 渠道历史（convlog）列表
GET    /api/agents/:id/conversations/:channelId → 某渠道完整历史

# 全局 sessions / conversations 组
GET    /api/sessions                       → 跨成员会话列表（{sessions,total}）
DELETE /api/sessions/:agentId/:sid         → 删除会话
PATCH  /api/sessions/:agentId/:sid         → 修改标题
GET    /api/conversations                  → 跨成员 convlog 管理视图
```

> ⚠️ 删除 / 改标题在**全局 `/api/sessions/:agentId/:sid`** 组（`globalSessionsHandler`），不在 agent 子路径下。

Chat 请求：

```json
{
  "message": "帮我看看这段代码",
  "sessionId": "ses-xxx",   // 续接已有会话；不传则创建新会话
  "context": "",            // 运行时额外上下文（ExtraContext）
  "images": [],
  "scenario": "",           // 可选：场景标记
  "skillId": "",            // 可选：指定技能
  "history": []             // 可选：legacy 预加载历史
}
```

SSE 事件（`done` 仅为其一）：

```json
{ "type": "done", "sessionId": "ses-xxx", "tokenEstimate": 12345, "input_tokens": 800, "output_tokens": 320 }
```

> 流中还可能出现 `thinking_delta` / `text_delta` / `tool_call` / `compaction_start` / `compaction_end` 等事件（见 `pkg/runner/runner.go`）。

---

## Runner 流程（已实现）

聊天入口不是同步线性调用，而是经 **SessionWorker 池 + Broadcaster** 解耦（见「运行时机制」）：

```
1. chat.go 解析/创建 sessionId，封装 RunRequest 入队 SessionWorker
2. worker goroutine 后台执行 runner（即使浏览器断开也跑完）
3. runner 开始：若 tokenEstimate ≥ 50,000 → 先同步执行 Compaction（见下）
4. 从 JSONL 加载历史 → r.history
   - 遇到 compaction 条目：清空已收集消息，仅保留其后的 message
   - 该会话的 summary 以「user + assistant」一对消息注入：
       user:      "[Previous conversation summary]\n<摘要>"
       assistant: "Understood. I have the context from the previous conversation."
     （注意：是 user/assistant 对，不是 system 角色）
5. 追加用户消息 → r.history + 写 JSONL
6. LLM 流式 agentic 循环（工具并行执行，最多 maxIter=30 轮）
7. 每轮 assistant 回复 / tool 结果 → r.history + 写 JSONL
8. 完成后更新 sessions.json（messageCount / lastAt / tokenEstimate）
9. done 后台触发自动主题命名（fire-and-forget）
```

> Compaction 触发点已移至**下一回合开始前同步执行**（`runner.maybeCompactSync`），而非「本回合结束后异步」，以便把 `compaction_start/end` 事件干净地串在同一次 SSE 内。

---

## Compaction（已实现）

```
触发条件：tokenEstimate ≥ 50,000（pkg/session/compaction.go: CompactionThreshold）
触发时机：下一回合开始前，由 runner.maybeCompactSync 同步执行

步骤：
1. ReadHistory 读取全部 message
2. 保留最近 20 条消息（keepTurns=20 按「消息条数」计，约等于 10 轮 user+assistant）
3. 边界之前的消息拼接后发给 LLM 生成摘要（max 500 words 提示）
4. 写入 CompactionEntry（summary / firstKeptEntryId="turn-{boundary}" / tokensBefore / tokensAfter）
5. 把最近 20 条消息重新 append 到 compaction marker 之后
6. 更新 sessions.json 的 tokenEstimate，并同步 chatlog 摘要
7. 发出 compaction_start / compaction_end 事件（UI 显示「压缩历史上下文中…」）

下次加载（ReadHistory）：
- 遇到 compaction 条目 → 清空已收集消息、置 afterCompaction
- 仅保留 compaction 之后的 message
- summary 由调用方（runner）以 user+assistant 对注入
```

> 说明：`session.CompactIfNeeded`（异步版本）仍存在，但 runner 实际走的是 `session.Compact` 同步路径。

---

## 运行时机制（已实现，补充）

- **SessionWorker + Broadcaster**（`worker.go` / `broadcaster.go`）：每会话一个后台 goroutine（懒创建），输入队列容量 8，空闲 `30 分钟`自动回收；SSE handler 仅订阅 Broadcaster，断线重连可续传。
- **自动主题命名 Auto-Retitle**（`retitle.go`）：消息数到达里程碑 `4 / 12 / 30 / 80` 时后台调用会话自身模型生成 8–20 字中文标题；用户手动 PATCH 改名会置 `titleOverridden=true`，自动命名不再覆盖。
- **Session Reaper**（`reaper.go`）：后台每 `24 小时`扫描一次，清理 mtime 超过 `30 天` 且 `active != true` 的会话文件，并同步移除索引中文件已不存在的条目。
- **chatlog / convlog 双轨**：`workspace/conversations/`（chatlog，供 AI system prompt 读取）与 `agents/{id}/convlogs/`（convlog，仅管理员可见）并行；Compaction 后会更新 chatlog 摘要。

---

## 历史对话系统（ConvLog，已实现）

渠道历史（管理员可见，与 AI 上下文隔离）：

```
agents/{agentId}/convlogs/
  telegram-{chatId}.jsonl    # Telegram 渠道历史（telegram.go 写入）
  web-{channelId}.jsonl      # Web 公开聊天历史（public_chat.go 写入，按 channelId 聚合）
```

> ⚠️ **飞书例外**：飞书会话直接以 `feishu-{chatId}` 写入 `sessions/`（JSONL session），**不写 convlog**；ChatsView 对飞书走 sessions 分支渲染。因此 `convlogs/feishu-*.jsonl` 当前并不存在。
> Web 的 convlog 按 `web-{channelId}` 聚合，而底层 session 粒度更细（`web-{channelId}-{token}`，每访客一条）。

- 管理员通过 ChatsView 查看全部渠道历史（统一 AiChat 组件渲染，GFM markdown / 代码高亮 / 工具卡展开）
- 支持按渠道 / 成员筛选
- 非面板来源（飞书 / Telegram）session 自动标记只读，UI 显示锁图标 + 提示条

---

## 通讯录联动（26.4.22v1+）

渠道消息进入 Runner 前会调用 `network.NewStore(wsDir).Resolve(source, externalId, displayName)`：

- 发送者档案自动 upsert 到 `workspace/network/contacts/{source}-{externalId}.md`（联系人 ID 形如 `source:externalId`，落盘文件名为 `source-externalId.md`）
- `Store.Summary(contactID)` 生成「当前对话对方」摘要（frontmatter + 事实前 3 条 + 偏好前 2 条；目标 ~300 字符，硬上限 1200）
- 摘要通过 `runner.Config.ExtraContext` 运行时注入 system prompt
- 每次 mutate 自动重建 `network/INDEX.md` + `INDEX.json`
- AI 可用 `network_note(entityId, section, text)` 工具追加事实到档案，或用通用 `read` 工具按需深读

详见 `docs/system-prompt-and-flow.md` 第二节。

---

## 与业界方案对照

| 特性 | 业界方案 | ZyHive |
|------|---------|--------|
| Session key | `agent:main:telegram:group:-xxx` | `ses-{timestamp}` / 渠道前缀 |
| 存储格式 | JSONL v3 | JSONL v3（相同）|
| 历史所有权 | 服务端 | 服务端 ✅ |
| Compaction | 自动，200k token | 自动，**50k** token ✅ |
| 会话索引 | sessions.json | sessions.json ✅ |
| 多渠道会话 | 每个渠道独立 session | agentId:sessionId 隔离 ✅ |
| 渠道历史 | convlog | convlogs/（飞书走 sessions/）✅ |
