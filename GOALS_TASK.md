# ZyHive 目标&规划系统实现 Prompt

## 项目背景
ZyHive 是一个 Go + Vue 3 的 AI 团队管理系统。
- 后端：Go，Gin 框架，路径 `internal/api/`，`pkg/`
- 前端：Vue 3 + Element Plus + TypeScript，路径 `ui/src/`
- 模块路径：`github.com/Zyling-ai/zyhive`
- 代码风格：参考现有文件 `internal/api/agents.go`、`pkg/agent/manager.go`、`ui/src/views/AgentsView.vue`

---

## 任务：新增目标&规划系统（Goals & Planning）

### 第一步：先读懂这些现有文件（必须参考，不要偏离风格）
- `internal/api/router.go` — 路由注册方式
- `internal/api/cron.go` — handler 写法
- `pkg/cron/engine.go` — cron Job 结构体（需要集成）
- `ui/src/views/CronView.vue` — Vue 页面风格
- `ui/src/App.vue` — 侧边栏注册方式
- `ui/src/router/index.ts` — 路由注册方式
- `ui/src/api/index.ts` — API 调用封装方式

---

### 第二步：创建后端文件

#### `pkg/goal/types.go`
```go
package goal

import "time"

type Status string
const (
    StatusDraft     Status = "draft"
    StatusActive    Status = "active"
    StatusCompleted Status = "completed"
    StatusCancelled Status = "cancelled"
)

type GoalType string
const (
    GoalPersonal GoalType = "personal"
    GoalTeam     GoalType = "team"
)

type Milestone struct {
    ID       string    `json:"id"`
    Title    string    `json:"title"`
    DueAt    time.Time `json:"dueAt"`
    Done     bool      `json:"done"`
    AgentIDs []string  `json:"agentIds,omitempty"`
}

type Goal struct {
    ID          string      `json:"id"`
    Title       string      `json:"title"`
    Description string      `json:"description,omitempty"`
    Type        GoalType    `json:"type"` // "personal" | "team"
    AgentIDs    []string    `json:"agentIds"`
    Status      Status      `json:"status"`
    Progress    int         `json:"progress"` // 0-100
    StartAt     time.Time   `json:"startAt"`
    EndAt       time.Time   `json:"endAt"`
    StartCronID string      `json:"startCronId,omitempty"`
    EndCronID   string      `json:"endCronId,omitempty"`
    Milestones  []Milestone `json:"milestones"`
    CreatedAt   time.Time   `json:"createdAt"`
    UpdatedAt   time.Time   `json:"updatedAt"`
}
```

#### `pkg/goal/manager.go`
实现以下方法，持久化到 `<dataDir>/goals.json`（JSON 数组，和 cron/jobs.json 同风格）：
```go
type Manager struct { /* dataDir, goals map, mu sync.RWMutex, cronEngine */ }

func NewManager(dataDir string, cronEngine CronAdder) *Manager
func (m *Manager) Load() error
func (m *Manager) List() []*Goal
func (m *Manager) ListByAgent(agentID string) []*Goal
func (m *Manager) Get(id string) (*Goal, error)
func (m *Manager) Create(g *Goal) error    // 生成ID，若 StartAt 非零则创建 at-cron；若 EndAt 非零则创建 at-cron
func (m *Manager) Update(id string, patch *Goal) error
func (m *Manager) UpdateProgress(id string, progress int) error
func (m *Manager) SetMilestoneDone(goalID, milestoneID string, done bool) error
func (m *Manager) Delete(id string) error
func (m *Manager) save() error
```

**Cron 集成规则：**
- 创建目标时，若 `StartAt` 非零：自动创建一个 `schedule.kind="at"` 的 cron 任务，payload 为 `"目标「{title}」已开始，请通知相关成员并开始推进。"`，触发后自动设置 goal.Status = "active"
- 若 `EndAt` 非零：创建结束提醒 cron，payload 为 `"目标「{title}」已到截止日期，请检查完成情况。"`
- 注意：cron 引擎目前只支持 cron 表达式，暂时跳过 at 类型注册，仅保存 cronID 到 goal 字段，后续 cron 引擎支持 at 后再激活

**CronAdder 接口：**
```go
type CronAdder interface {
    Add(job *cronpkg.Job) error
}
```

#### `internal/api/goals.go`
参考 `internal/api/cron.go` 写法，实现：
```
GET    /api/goals              — List（支持 ?agentId=xxx 过滤）
POST   /api/goals              — Create
GET    /api/goals/:id          — Get
PATCH  /api/goals/:id          — Update
DELETE /api/goals/:id          — Delete
PATCH  /api/goals/:id/progress — body: {"progress": 80}
PATCH  /api/goals/:id/milestones/:mid — body: {"done": true}
```

#### `internal/api/router.go` 修改
在 `SetupRouter` 函数中：
1. 初始化 `goal.NewManager(goalDataDir, cronEngine)`
2. 注册路由到 `/api/goals` group

---

### 第三步：创建前端文件

#### `ui/src/views/GoalsView.vue`

**页面结构：**
```
顶部：标题「目标规划」+ 右侧「新建目标」按钮 + 视图切换（甘特图/列表）
成员过滤栏：和 CronView 一致的 radio-group
甘特图区域（默认视图）
列表区域（切换视图）
右侧抽屉：新建/编辑目标表单
```

**甘特图实现（纯 CSS + Vue，不引入外部库）：**
```
时间轴：
  - 自动计算所有 goal 的最早 startAt 和最晚 endAt 作为范围
  - 顶部显示月份标签
  - 每行一个 goal，左侧显示名称 + 成员头像
  - 横向色条：宽度 = (endAt - startAt) / totalRange * 100%
              left  = (startAt - rangeStart) / totalRange * 100%
  - 色条颜色：取第一个 agentId 的 avatarColor（从 agentStore 获取）
  - 团队目标：渐变色或多色分段
  - 里程碑：◆ 定位在对应日期，悬浮显示 title
  - 进度条：在色条内部用半透明白色覆盖显示 progress%
  - 点击色条：打开编辑抽屉

状态标签：
  - draft: el-tag type="info"
  - active: el-tag type="primary"（带呼吸灯动画，参考 ChatsView.vue）
  - completed: el-tag type="success"
  - cancelled: el-tag type="danger"
```

**新建/编辑表单（el-drawer）：**
```
- 标题（必填）
- 描述（textarea）
- 类型：个人/团队（el-radio-group）
- 参与成员：el-select multiple，选项从 agentList 获取
- 开始时间：el-date-picker type="datetime"
- 结束时间：el-date-picker type="datetime"
- 进度：el-slider 0-100（编辑时显示）
- 里程碑列表（可增删，每条有标题+日期+完成状态）
```

**API 调用：** 封装到 `ui/src/api/index.ts`，新增：
```typescript
// Goals
export const listGoals = (agentId?: string) => ...
export const createGoal = (data: any) => ...
export const updateGoal = (id: string, data: any) => ...
export const deleteGoal = (id: string) => ...
export const updateGoalProgress = (id: string, progress: number) => ...
export const updateMilestoneDone = (goalId: string, milestoneId: string, done: boolean) => ...
```

#### `ui/src/App.vue` 修改
在侧边栏菜单中，在「团队关系」和「定时任务」之间，插入：
```vue
<el-menu-item index="/goals">
  <el-icon><Flag /></el-icon>
  <span>目标规划</span>
</el-menu-item>
```
并在顶部引入 `Flag` icon（来自 `@element-plus/icons-vue`）。

#### `ui/src/router/index.ts` 修改
新增路由：
```typescript
{ path: '/goals', component: () => import('../views/GoalsView.vue') }
```

---

### 第四步：定期检查系统（GoalCheck）

每个目标可以绑定多个定期检查计划，本质是带目标上下文的 cron 任务。

#### `pkg/goal/types.go` 补充

```go
// GoalCheck 定期检查计划
type GoalCheck struct {
    ID        string    `json:"id"`
    GoalID    string    `json:"goalId"`
    Name      string    `json:"name"`             // 如「每周进度检查」
    Schedule  string    `json:"schedule"`          // cron 表达式，如 "0 9 * * 1"
    TZ        string    `json:"tz,omitempty"`      // 时区，默认 Asia/Shanghai
    AgentID   string    `json:"agentId"`           // 执行检查的 AI 成员
    Prompt    string    `json:"prompt"`            // 支持变量：{goal.title} {goal.progress} {goal.endAt}
    CronJobID string    `json:"cronJobId"`         // 关联 cron 任务 ID
    Enabled   bool      `json:"enabled"`
    CreatedAt time.Time `json:"createdAt"`
}

// CheckRecord 每次检查的执行记录
type CheckRecord struct {
    ID        string    `json:"id"`
    GoalID    string    `json:"goalId"`
    CheckID   string    `json:"checkId"`
    AgentID   string    `json:"agentId"`
    RunAt     time.Time `json:"runAt"`
    Output    string    `json:"output"`    // AI 回复摘要（截断 500 字）
    Status    string    `json:"status"`    // "ok" | "error"
}
```

同时在 `Goal` 结构体中补充：
```go
type Goal struct {
    // ...原有字段...
    Checks []GoalCheck `json:"checks"` // 绑定的定期检查计划
}
```

#### `pkg/goal/manager.go` 补充方法

```go
// 新增检查计划：自动创建 cron 任务，prompt 中变量替换后注入 goal 上下文
func (m *Manager) AddCheck(goalID string, check *GoalCheck) error
// 更新检查计划（先删旧 cron，再建新 cron）
func (m *Manager) UpdateCheck(goalID, checkID string, patch *GoalCheck) error
// 删除检查计划（同时删除关联 cron 任务）
func (m *Manager) RemoveCheck(goalID, checkID string) error
// 手动触发一次检查
func (m *Manager) RunCheckNow(goalID, checkID string) error
// 追加检查记录（由 cron 执行后回调）
func (m *Manager) AppendCheckRecord(record CheckRecord) error
// 读取检查记录（最近50条）
func (m *Manager) ListCheckRecords(goalID string) ([]CheckRecord, error)
```

**Prompt 变量替换逻辑：**
```go
func buildCheckPrompt(tmpl string, g *Goal) string {
    r := strings.NewReplacer(
        "{goal.title}",    g.Title,
        "{goal.progress}", fmt.Sprintf("%d%%", g.Progress),
        "{goal.endAt}",    g.EndAt.Format("2006-01-02"),
        "{goal.startAt}",  g.StartAt.Format("2006-01-02"),
        "{goal.status}",   string(g.Status),
    )
    return r.Replace(tmpl)
}
```

检查记录存储：`<dataDir>/goals-checks/<goalID>.jsonl`（追加写，同 cron runs 风格）

#### `internal/api/goals.go` 补充路由

```
GET    /api/goals/:id/checks              — 列出目标的所有检查计划
POST   /api/goals/:id/checks              — 新建检查计划
PATCH  /api/goals/:id/checks/:checkId     — 修改检查计划
DELETE /api/goals/:id/checks/:checkId     — 删除检查计划
POST   /api/goals/:id/checks/:checkId/run — 立即触发一次
GET    /api/goals/:id/check-records       — 查询检查历史记录（最近50条）
```

#### `GoalsView.vue` 前端补充

**目标详情抽屉（编辑模式下）新增「定期检查」Tab：**

```
Tab 1: 基本信息（现有表单）
Tab 2: 定期检查
  - 检查计划列表（表格：名称/频率/执行成员/启用状态/操作）
  - 「添加检查」按钮 → 弹出 el-dialog：
      名称（必填）
      执行成员（el-select，从 agentList 选）
      检查频率（el-select 预设 + 自定义 cron）：
        预设选项：
          每天上午9点    → "0 9 * * *"
          每周一上午9点  → "0 9 * * 1"
          每周五下午5点  → "0 17 * * 5"
          每月1日        → "0 9 1 * *"
          自定义 →       输入框
      时区（默认 Asia/Shanghai）
      检查提示词（textarea，显示可用变量提示）：
        提示文字：可用变量：{goal.title} {goal.progress} {goal.endAt}
        默认值：请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），
               距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。
  - 操作列：立即运行 / 启用开关 / 删除

Tab 3: 检查记录
  - 时间线展示（el-timeline）
  - 每条记录：时间 + 执行成员头像 + AI 回复内容（可展开）
  - 状态 badge：success / danger
```

---

### 注意事项（必须遵守）
1. **不引入任何新 npm 包**（只用 Element Plus + Vue 3 已有依赖）
2. **持久化风格**：goals.json 和 jobs.json 同目录（`<dataDir>/goals.json`）
3. **类型安全**：前端用 TypeScript interface 定义 Goal / Milestone 类型
4. **错误处理**：参考 cron.go，handler 统一返回 `gin.H{"error": ...}`
5. **空状态**：列表为空时显示 `<el-empty>` 组件
6. **时间格式**：后端统一 `time.Time`（JSON 序列化为 ISO 8601），前端用 `new Date()` 解析
7. **Build**：修改完成后运行 `cd ui && npm run build`，然后 `make build`

---

### 参考代码片段（甘特图核心计算）
```typescript
// 计算 goal 在甘特图中的位置
function calcGanttBar(goal: Goal, rangeStart: Date, rangeEnd: Date) {
  const total = rangeEnd.getTime() - rangeStart.getTime()
  const left = ((new Date(goal.startAt).getTime() - rangeStart.getTime()) / total) * 100
  const width = ((new Date(goal.endAt).getTime() - new Date(goal.startAt).getTime()) / total) * 100
  return { left: `${Math.max(0, left)}%`, width: `${Math.max(1, width)}%` }
}

// 计算时间轴月份标签
function calcMonthLabels(rangeStart: Date, rangeEnd: Date) {
  const months = []
  const cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1)
  while (cur <= rangeEnd) {
    const left = ((cur.getTime() - rangeStart.getTime()) / (rangeEnd.getTime() - rangeStart.getTime())) * 100
    months.push({ label: `${cur.getFullYear()}/${cur.getMonth()+1}`, left: `${left}%` })
    cur.setMonth(cur.getMonth() + 1)
  }
  return months
}
```

---

### 完成后
1. `cd ui && npm run build`
2. `make build`
3. 验证：访问 `/goals` 页面，能创建目标并在甘特图中显示
