# ZyHive — AI 成员系统提示词与对话流程

> 本文档描述 ZyHive 当前（v0.9.11）的实际实现，供二次开发和调优参考。

---

## 一、System Prompt 结构

每次 AI 对话开始前，`BuildSystemPrompt()` 会动态组装完整的系统提示词。**每个对话 turn 重新构建一次**（文件只读一次，缓存在内存里）。

最终 system prompt 的拼接顺序：

```
[1] 当前时间（Asia/Shanghai）
[2] IDENTITY.md      ← 成员身份定义
[3] SOUL.md          ← 成员性格/行为准则
[4] memory/INDEX.md  ← 记忆目录索引（轻量）
[5] 记忆树提示        ← 告知 AI 可以用 read 工具访问完整记忆
[6] RELATIONS.md     ← 团队关系表
[7] 技能提示词        ← 所有已启用技能的 SKILL.md 内容
[8] AGENTS.md        ← 工作区协议（如有）
[9] 项目上下文       ← 共享团队项目列表（运行时注入）
[10] 额外上下文      ← 调用方注入（如页面场景）
[11] 环境变量列表    ← 成员私有 env 的 key 名（不含值）
[12] Runtime 元数据  ← Model / AgentID / WorkspaceDir
```

### 各段详细说明

#### [1] 当前时间
```
Current date and time: 2026-02-23 23:45:00 CST
```

#### [2] IDENTITY.md
```
--- IDENTITY.md ---
# IDENTITY.md - 我的身份
- 名称：小流
- 定位：一流团队 AI 助手
...
```

#### [3] SOUL.md
```
--- SOUL.md ---
# SOUL.md - 我是谁
## 核心准则
真诚帮助，不做表演…
```

#### [4] memory/INDEX.md（轻量记忆索引）
只注入索引文件（不注入全部记忆），避免 context 过长。完整记忆通过 `read` 工具按需读取。

如果 `memory/INDEX.md` 不存在，回退到注入整个 `MEMORY.md`（兼容旧格式）。

```
--- memory/INDEX.md ---
## 近期主题
- 2026-02-23: ZyHive v0.9.11 发布
…
```

#### [5] 记忆树提示
```
[Memory tree available. Use read tool to access: memory/core/, memory/projects/, memory/daily/, memory/topics/]
```

#### [6] RELATIONS.md（团队关系）
```
--- RELATIONS.md ---
| 成员 | 关系类型 | 对象 |
|------|----------|------|
| 引引 | 上级     | 小流 |
…
```

#### [7] 技能提示词
每个**已启用**的技能，将其 `SKILL.md` 内容注入：
```
--- Skill: 搜索助手 ---
你有网络搜索能力，使用 web_fetch 工具…
```

#### [8] AGENTS.md（工作区协议）
若工作区存在 `AGENTS.md`，完整注入。同时解析其中的文件引用行，自动注入被引用文件内容。

#### [9] 项目上下文（运行时注入）
```
--- 共享团队项目工作区 ---
• ZyHive 源码 (id: proj_xxx, 权限: 可读写) — 主项目代码
• 收款日历  (id: proj_yyy, 权限: 只读)
```

#### [10] 额外上下文
调用方（如 Telegram handler）可注入额外说明，如 "当前用户在群聊 XXX 中发言"。

#### [11] 环境变量（只注入 key 名）
```
## 可用环境变量
以下环境变量已配置，exec 工具运行时自动可用：
- ANTHROPIC_API_KEY
- GITHUB_TOKEN
```

> **注意**：只告诉 AI 有哪些变量，不暴露变量值。

#### [12] Runtime 元数据
```
## Runtime
Model: claude-sonnet-4-6 | Agent: xiuliu | Workspace: /var/lib/zyhive/agents/xiuliu/workspace
```

---

## 二、完整对话流程

```
用户消息
   │
   ▼
[1] 追加到 history（含图片则构建多模态 content 数组）
   │ 持久化到 session JSONL
   ▼
[2] 加载历史（session 文件 → JSONL）
   │  如有 compaction 摘要 → 以 user/assistant 对话形式前置
   │  sanitize: 去重连续同角色消息，修复孤立 tool_use/tool_result
   ▼
[3] 构建 System Prompt（BuildSystemPrompt，整个 run 只读一次）
   │  + 项目上下文 / 额外上下文 / 环境变量列表 / Runtime 元数据
   ▼
[4] LLM 流式调用（SSE）
   │  ┌── text_delta → 实时推送到前端 / Telegram
   │  ├── tool_use   → 收集工具调用列表
   │  └── stop_reason
   ▼
[5] 判断 stop_reason
   ├── end_turn（纯文本回复）→ 持久化 assistant 消息 → 触发 compaction 检查 → 结束
   └── tool_use（有工具调用）
          │
          ▼
       [6] 并行执行所有工具（goroutine + WaitGroup）
          │  每个工具调用独立 goroutine，结果按原始顺序收集
          │  持久化 assistant(tool_use) + user(tool_result) 到 session
          ▼
       [7] 回到 [4] 重新调用 LLM（携带工具结果）
          │  最多 30 次循环
```

### Compaction（上下文压缩）
当 session token 估算超过阈值时，异步触发 compaction：
1. 调用 LLM 对历史对话生成摘要
2. 将摘要写入 session，清除旧消息
3. 下次对话开始时摘要以 `user/assistant` 对形式前置

---

## 三、工具列表

| 工具名 | 说明 |
|--------|------|
| `read` | 读取文件内容（工作区内） |
| `write` | 创建/覆盖文件 |
| `edit` | 精确替换文件中的文本段 |
| `exec` | 执行 shell 命令（沙箱限制） |
| `grep` | 在文件中搜索文本 |
| `glob` | 列举匹配路径的文件 |
| `web_fetch` | 抓取网页内容（markdown 格式返回） |
| `show_image` | 在对话中展示图片 |
| `send_file` | 通过 Telegram 发送文件（≤50MB 上传，>50MB 返回链接） |
| `self_list_skills` | 查看当前成员已安装的技能 |
| `self_install_skill` | 安装新技能 |
| `self_uninstall_skill` | 卸载技能 |
| `self_set_env` | 持久化私有环境变量 |
| `self_delete_env` | 删除私有环境变量 |
| `self_rename` | 修改自己的显示名 |
| `self_update_soul` | 更新自己的 SOUL.md |
| `project_*` | 共享团队项目工作区读写（需写入权限） |

> SkillStudio 沙箱模式：工具操作限制在 `skills/{skillId}/` 目录内，部分危险工具（`self_install_skill` 等）被禁用。

---

## 四、Session 存储格式

每个 session 对应一个 `.jsonl` 文件，每行一条消息：

```jsonl
{"role":"user","content":"\"你好\"","ts":1740000000}
{"role":"assistant","content":"[{\"type\":\"text\",\"text\":\"你好！\"}]","ts":1740000001,"toolCalls":[...]}
{"role":"user","content":"[{\"type\":\"tool_result\",\"tool_use_id\":\"tu_01\",\"content\":\"文件内容\"}]","ts":1740000002}
```

Session 索引（`sessions.json`）记录所有 session 的元数据（ID、标题、token 估算、最后时间）。

---

## 五、已知问题 & 改进空间

### 现状
| 问题 | 影响 |
|------|------|
| 每次 turn 重建 system prompt | 文件 IO，但已在 run 内缓存 |
| MEMORY.md 全量注入（旧格式兼容） | context 长，推荐迁移到 memory/INDEX.md |
| 技能提示词全量注入 | 技能多时 system prompt 变长 |
| exec 工具无沙箱隔离 | AI 可执行任意命令（信任边界依赖 SOUL.md） |
| Compaction 异步执行 | 极端情况下 compaction 未完成时新消息进来会丢上下文 |

### 可优化方向
- **按需加载记忆**：system prompt 只注入索引，全部记忆按 AI 主动 `read` 调用获取（已部分实现）
- **技能按需激活**：通过 RAG 或关键词匹配，只注入当前对话相关的技能
- **exec 沙箱**：Docker / nsjail 隔离，限制文件系统访问范围
- **流式 Compaction**：在 turn 结束后同步压缩，确保下次对话上下文完整
- **多模型路由**：根据任务类型（代码/对话/分析）自动选择合适模型

---

## 六、关键代码位置

| 功能 | 文件 |
|------|------|
| System prompt 构建 | `pkg/runner/system_prompt.go` |
| 对话主循环（工具调用循环） | `pkg/runner/runner.go` |
| 历史记录清洗（孤立 tool_use 修复） | `pkg/runner/runner.go` → `sanitizeHistory()` |
| Session 存储/读取 | `pkg/session/store.go` |
| Compaction 触发 | `pkg/session/store.go` → `CompactIfNeeded()` |
| 工具并行执行 | `pkg/runner/runner.go` → `executeTools()` |
| Anthropic 流式客户端 | `pkg/llm/anthropic.go` |
| 工具定义与执行 | `pkg/tools/tools.go` |
| Chat API handler | `internal/api/chat.go` |
