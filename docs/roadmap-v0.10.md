# 版本前瞻：26.3.17 — 团队规划 & 会议系统

> ZyHive 从「成员管理工具」迈向「AI 团队操作系统」的关键版本
> ⚠️ 前瞻设计稿：团队规划已以**不同方案**落地、会议系统尚未实现，详见下方「状态校正」。

---

## ⚠️ 状态校正（截至 26.5.16v1）

本文是 **2026-02-21 的前瞻设计稿**，后续实现发生了偏离，阅读时请注意：

- **「团队规划」已落地，但采用了与本文不同的方案**：实际实现见根目录 `GOALS_TASK.md` 与 `pkg/goal/`，并非本文设计的 `pkg/plans` / `team-plans/`。主要命名 / 路径映射：

  | 本文设计 | 实际实现 |
  |---------|---------|
  | `pkg/plans/` | `pkg/goal/`（`types.go` / `manager.go`） |
  | `/api/team/plans` | `/api/goals`（`internal/api/goals.go`、`router.go`） |
  | `team-plans/goals.json` + 每目标 `goal.md` + `reviews/{date}.md` | `cron/goals.json`；检查记录 `cron/goals-checks/{goalId}.jsonl`（无 per-goal Markdown） |
  | `PlanningView.vue` + 三级（长期/中期/短期） | `GoalsView.vue` + 甘特图；按状态 / 成员过滤 |
  | `horizon: short\|mid\|long` | `Goal.Type: personal\|team`（无 horizon 字段） |
  | Cron Job 新增顶层 `goalId` 字段 | 未加；改由 `GoalCheck` 各自挂独立 cron job（`cronJobId`） |
  | `Review Job` 读写 `goal.md` / `reviews` / @负责人 | `GoalCheck` 触发 agentTurn cron + JSONL 记录，不读写 goal.md、无 @ 逻辑 |
  | `GET .../reviews` API | `GET /api/goals/:id/check-records` |
  | `linkedMeetingIds` | 未实现（无会议系统） |

- **「会议系统」整体仍未实现**：代码中没有 `pkg/meeting/`、`MeetingRunner`、`/api/meetings`、`meetings.json`。本文 Feature 2 全部为未来设计。

- **版本号注意**：本文标题的「26.3.17」与已发布的 26.3.17（实际内容为工具生态升级 / ChatHomeView / CLI 等，见 `CHANGELOG.md` 与 `README.md`）**不是同一批内容**；Goals 规划实际随 **v0.9.18–v0.9.23** 上线。本文版本号仅为当时的前瞻命名，勿与 CHANGELOG 对应。

> 下文保留原始前瞻设计供参考；如需当前真实实现，请以 `GOALS_TASK.md` / `pkg/goal` / `GoalsView.vue` 为准。

---

## 背景

v0.9.0 完成了成员管理、团队图谱、项目系统等基础建设。  
26.3.17 进一步让 AI 团队**自我驱动**：设定目标、定期迭代、开会讨论、自主决策。

---

## Feature 1：团队规划（Team Planning）

> 📌 已落地，但实现方案与本节不同（实际为 `pkg/goal` / `/api/goals` / `GoalsView`，详见顶部映射表）。本节保留为原始设计稿，**命名 / 路径 / 数据结构请勿照此对照代码**。

### 定位

> 将「定时任务」升级为「以目标为核心的团队规划系统」——Cron 作为底层执行引擎，上层是目标、里程碑、自动迭代。

### 核心概念

| 概念 | 说明 |
|------|------|
| **目标（Goal）** | 团队的长期/中期/短期方向，有进度、负责人、关联里程碑 |
| **里程碑（Milestone）** | 目标分解为可追踪的阶段节点 |
| **迭代任务（Review Job）** | 关联到目标的 Cron 任务，由 AI 定期评估进度、写更新 |
| **规划视图（PlanningView）** | 时间线 + 目标看板，独立导航入口 |

### 数据结构

```
team-plans/
  goals.json                # 目标索引
  {goalId}/
    goal.md                 # 目标详情、里程碑、最新进度（Markdown，AI 可读写）
    reviews/
      {date}.md             # 每次 AI 迭代回顾日志
```

**goals.json 格式：**
```json
{
  "goals": [
    {
      "id": "goal-20260301-001",
      "title": "完成 ZyHive v1.0 正式发布",
      "horizon": "mid",           // short | mid | long
      "status": "active",         // active | completed | paused | cancelled
      "owner": "xiuliu",          // agentId 或 "team"
      "description": "完成所有核心功能，对外正式开源发布",
      "progress": 35,             // 0-100
      "targetDate": "2026-06-01",
      "milestones": [
        { "id": "ms-1", "title": "v0.10 发布", "done": false, "dueDate": "2026-03-15" },
        { "id": "ms-2", "title": "v0.11 发布", "done": false, "dueDate": "2026-04-30" },
        { "id": "ms-3", "title": "v1.0 发布公告", "done": false, "dueDate": "2026-06-01" }
      ],
      "linkedCronIds": ["cron-weekly-review"],   // 关联定时迭代任务
      "linkedMeetingIds": ["meeting-sprint"],    // 关联例会
      "createdAt": 1740000000000,
      "updatedAt": 1740001000000
    }
  ]
}
```

### 迭代任务（Review Job）

目标可绑定一个或多个 Cron 任务，到期后 AI 自动：
1. 读取 `goal.md`（当前进度、里程碑状态）
2. 结合近期工作记忆（`memory/`）评估实际进度
3. 更新进度百分比、里程碑完成状态
4. 写 `reviews/{date}.md`（本次迭代回顾）
5. 若进度滞后，自动生成风险提示并 @ 负责人

**Cron Job 复用现有标准化结构（不另起炉灶）：**
```json
{
  "id": "cron-weekly-review",
  "name": "目标周迭代 — ZyHive v1.0",
  "agentId": "xiuliu",
  "schedule": { "kind": "cron", "expr": "0 9 * * 1", "tz": "Asia/Shanghai" },
  "message": "请评估「完成 ZyHive v1.0 正式发布」目标的本周进度，更新 team-plans/goal-20260301-001/goal.md 并写本周迭代日志",
  "goalId": "goal-20260301-001",   // 新增字段，标记关联目标
  "enabled": true
}
```

### API 设计

```
GET    /api/team/plans                     → 目标列表（含进度、里程碑）
POST   /api/team/plans                     → 创建目标
GET    /api/team/plans/:id                 → 目标详情
PATCH  /api/team/plans/:id                 → 更新目标（进度/里程碑/状态）
DELETE /api/team/plans/:id                 → 删除目标
GET    /api/team/plans/:id/reviews         → 迭代回顾列表
GET    /api/team/plans/:id/reviews/:date   → 某次回顾详情
```

### UI 设计（PlanningView）

```
┌─────────────────────────────────────────────────────┐
│  团队规划                         [+ 新建目标]       │
├──────────┬──────────────────────────────────────────┤
│ 长期目标 │  目标卡片（进度环 + 里程碑时间线）         │
│ 中期目标 │  ─────────────────────────────────────── │
│ 短期目标 │  [目标标题]   负责人: 小流   进度: 35%    │
│          │  ████████░░░░░░░░░░░░  目标日期: 2026-06 │
│          │  里程碑: ✅ v0.9 · ○ v0.10 · ○ v1.0     │
│          │  关联定时迭代 / 关联例会  [查看回顾]       │
└──────────┴──────────────────────────────────────────┘
```

- 左侧三级分类（长期/中期/短期）
- 目标卡片：进度环、里程碑节点、负责 AI 成员头像
- 点击目标 → 侧边详情面板：goal.md 全文、历次回顾记录
- 新建目标对话框：标题、分类、负责人、目标日期、里程碑（动态增减）、关联迭代 Cron

---

## Feature 2：会议系统（Meeting System）

> 📌 截至 26.5.16v1 **尚未实现**：代码中无 `pkg/meeting/` / `MeetingRunner` / `/api/meetings` / `meetings.json`。本节为未来设计。

### 定位

> AI 团队成员自主开会：系统定时创建会议 Session，参会成员轮流发言讨论，到期投票形成决议，自动生成会议纪要。

### 核心概念

| 概念 | 说明 |
|------|------|
| **会议（Meeting）** | 有主题、目标、参会成员、议程、投票规则 |
| **会议 Session** | 会议到期时系统自动创建的特殊对话 Session |
| **主持人（Facilitator）** | 指定的 AI 成员，负责推进议程、发起投票、总结纪要 |
| **投票（Vote）** | 结构化投票（赞成/反对/弃权），到达票数阈值或超时后结束 |
| **会议纪要** | 会后自动写入 `meetings/{id}/minutes.md`，关联目标可同步 |

### 数据结构

```
meetings/
  meetings.json              # 会议列表索引
  {meetingId}/
    meeting.json             # 会议配置（主题/议程/参会者等）
    minutes/
      {sessionId}.md         # 每次会议纪要（AI 自动生成）
```

**meetings.json 格式：**
```json
{
  "meetings": [
    {
      "id": "meeting-sprint",
      "title": "产品周例会",
      "objective": "回顾本周进展，确认下周优先级",
      "facilitator": "xiuliu",
      "participants": ["xiuliu", "devbot", "pmbot"],
      "agenda": [
        "本周完成情况同步",
        "待解决问题讨论",
        "下周优先级投票"
      ],
      "schedule": {
        "kind": "cron",
        "expr": "0 10 * * 5",         // 每周五 10:00
        "tz": "Asia/Shanghai"
      },
      "duration": 30,                  // 讨论轮次上限（分钟等效，实际控轮次）
      "maxRoundsPerAgent": 3,          // 每个成员最多发言轮次
      "votingRule": "majority",        // majority | consensus | facilitator
      "linkedGoalIds": ["goal-20260301-001"],
      "status": "scheduled",           // scheduled | in-progress | completed
      "lastSessionId": "",
      "nextRunAt": 1741000000000,
      "createdAt": 1740000000000
    }
  ]
}
```

### 会议自动执行流程

```
Cron 到期
    ↓
创建 MeetingSession（特殊 sessionId: meeting-{meetingId}-{timestamp}）
    ↓
主持人（facilitator）发布开场消息：
  "【会议开始】主题：xxx / 目标：xxx / 议程：..."
    ↓
按参会顺序，系统依次触发各参会 Agent 发言
  - 每个 Agent 拿到：会议背景 + 完整对话历史 → 生成发言内容
  - 写入会议 Session JSONL
    ↓
N 轮讨论后（maxRoundsPerAgent × participants 轮）
    ↓
主持人发起投票：
  "【投票】下周优先级：A. 做 Feature X  B. 修复 Bug Y  C. 重构模块 Z"
    ↓
各成员在结构化格式中投票（{"vote": "A", "reason": "..."}）
    ↓
主持人统计结果，宣布决议
    ↓
主持人生成会议纪要 → 写 meetings/{id}/minutes/{sessionId}.md
如有关联目标 → 同步更新 goal.md（会议决议区块）
    ↓
会议状态 → completed，nextRunAt 更新到下次 Cron 触发时间
```

### 会议 Session 与普通 Session 的区别

| 特性 | 普通 Session | 会议 Session |
|------|-------------|-------------|
| 触发方式 | 用户发消息 | Cron 自动触发 |
| 参与者 | 1 个 Agent | N 个 Agent 轮流 |
| 存储位置 | `agents/{id}/sessions/` | `meetings/{id}/` |
| 可见性 | 仅该 Agent | 所有参会成员 + 管理员 |
| 结束方式 | 用户停止 | 投票完成 or 超轮次 |
| 产出物 | 无固定产出 | 会议纪要 + 目标更新 |

### API 设计

```
GET    /api/meetings                       → 会议列表
POST   /api/meetings                       → 创建会议
GET    /api/meetings/:id                   → 会议详情
PATCH  /api/meetings/:id                   → 更新配置
DELETE /api/meetings/:id                   → 删除会议
POST   /api/meetings/:id/run               → 立即召开一次（测试用）
GET    /api/meetings/:id/minutes           → 历次纪要列表
GET    /api/meetings/:id/minutes/:sid      → 某次纪要全文
GET    /api/meetings/live                  → 当前进行中的会议（SSE 实时进度）
```

### UI 设计（MeetingsView）

```
┌─────────────────────────────────────────────────────┐
│  会议                                  [+ 新建会议]  │
├─────────────────────────────────────────────────────┤
│ ● 进行中                                            │
│  产品周例会                    🔴 进行中  [观看直播] │
│  主持：小流 | 参会：3人 | 第2轮发言                  │
├─────────────────────────────────────────────────────┤
│ ○ 已排期                                            │
│  架构评审会         下次: 周一 10:00   [立即召开]    │
│  主持：小流 | 参会：xiuliu, devbot                  │
│                                           [查看纪要] │
├─────────────────────────────────────────────────────┤
│ ✅ 已完成（最近 5 次）                               │
│  产品周例会  2026-02-14  决议：优先做 Feature X      │
└─────────────────────────────────────────────────────┘
```

**新建会议对话框：**
- 会议标题、目标描述
- 主持人（选择 AI 成员）
- 参会成员（多选）
- 议程（动态增减条目）
- 会议计划：一次性 or 周期（复用 Cron expr 输入）
- 投票规则（多数通过/共识/主持人裁定）
- 关联目标（可选）

**会议直播面板（Live View）：**
- 实时展示各成员发言（SSE 推送）
- 左侧：参会成员 + 当前发言者高亮
- 右侧：对话流（与 AiChat 组件复用）
- 底部：当前议程进度 + 投票状态

---

## 两个 Feature 的关联关系

```
团队规划目标（Goal）
    ↕ linkedCronIds
定时迭代任务（Review Job）   ← 复用现有 Cron 结构
    ↕
    └── 每周 AI 自动更新进度 → goal.md

团队规划目标（Goal）
    ↕ linkedMeetingIds
例会（Meeting）              ← 每次开会自动关联目标
    ↕
    └── 会议决议 → 同步写入 goal.md（决议区块）
```

---

## 实现计划（26.3.17）

### Phase 1 — 团队规划后端　【已落地（方案不同）】
- [x] 目标 CRUD / 进度更新 / 里程碑管理 → 实际为 `pkg/goal/`（非 `pkg/plans/`）
- [x] 目标 API → 实际为 `/api/goals`（非 `/api/team/plans`）
- [~] 定期检查 → 由 `GoalCheck` 各挂独立 cron job 实现（Cron Job **未**新增顶层 `goalId` 字段）
- [x] 持久化 → 实际为 `cron/goals.json` + `cron/goals-checks/{goalId}.jsonl`（非 `team-plans/`，无 per-goal `goal.md`）

### Phase 2 — 团队规划前端　【已落地（方案不同）】
- [x] 规划页 → `GoalsView.vue`（甘特图 + 按状态 / 成员过滤，非三级 horizon 分类）
- [x] 新建 / 编辑目标弹窗（含里程碑动态增减）
- [~] 目标详情 → 有「定期检查 / 检查记录」tab；无 `goal.md` 全文 / `reviews` 列表
- [x] 关联定时检查 → 在 GoalsView 内添加 Check（非 CronView 的 `goalId` 下拉）

### Phase 3 — 会议系统后端　【未实现】
- [ ] `pkg/meeting/` — 会议 CRUD、Session 创建、发言轮次引擎
- [ ] `MeetingRunner` — 按顺序驱动各参会 Agent 发言，写 Session JSONL
- [ ] 投票解析器 — 识别结构化投票格式，统计结果
- [ ] 纪要生成器 — 会后主持人调用 LLM 生成 minutes.md
- [ ] Cron 集成 — 到期自动触发 `MeetingRunner.Run()`
- [ ] `/api/meetings` API + SSE `/api/meetings/live`

### Phase 4 — 会议系统前端　【未实现】
- [ ] `MeetingsView.vue` — 进行中/已排期/历史三段展示
- [ ] 新建会议弹窗（参会成员多选、议程动态增减、Cron 调度）
- [ ] 会议直播面板（SSE 实时发言流、参会者状态）
- [ ] 纪要详情页（Markdown 渲染，关联目标跳转）
- [ ] 导航栏新增「规划」「会议」两个入口

---

## 技术要点

### 团队规划
- `goals.json` 与成员工作区目录隔离，存放在根目录 `team-plans/`，所有 Agent 可通过工具访问
- 迭代 Cron 复用现有 `cron.Job` 结构，仅新增 `goalId` 可选字段，向后兼容
- AI 更新进度：Runner 执行时传入目标上下文（goal.md 内容），Agent 直接写文件，后端解析进度字段

### 会议系统
- `MeetingRunner` 与现有 `Agent Pool` 集成，调用 `pool.Run(ctx, agentId, message)` 驱动各成员发言
- 会议 Session 写入专用 JSONL，不污染 Agent 自身 session 历史
- 投票解析：约定结构化格式 `{"vote": "A", "reason": "..."}` + fallback 关键词匹配（赞成/反对）
- 会议直播：SSE 推送，前端直接复用 `AiChat.vue` 的消息渲染逻辑

---

*文档创建：2026-02-21 | 原定版本：26.3.17（前瞻命名，与 CHANGELOG 的 26.3.17 非同一批内容）*
*状态校正：截至 26.5.16v1 — 团队规划已以 `pkg/goal` / `GoalsView` 方案落地；会议系统未实现。详见顶部「状态校正」。*
