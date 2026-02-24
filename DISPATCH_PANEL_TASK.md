# 派遣面板 & 实时汇报系统 — 实现任务

## 功能目标
当 AI 成员在聊天中派遣子成员执行任务时，聊天页面（AiChat.vue）顶部自动出现「派遣面板」：
- 被派遣的成员以拟人化动画逐个「走进来」（从右侧飞入，弹性曲线）
- 每个成员实时显示执行状态和汇报内容
- 子成员可在执行中调用 `report_to_parent` 工具主动汇报
- 任务完成后成员「离开」，面板收起

## 必读参考文件
- `pkg/subagent/manager.go` — 现有 SubAgent 实现
- `pkg/subagent/types.go` — 现有类型定义
- `pkg/tools/tools.go` — 工具定义方式
- `pkg/tools/registry.go` — 工具执行方式
- `pkg/runner/runner.go` — Runner 上下文
- `pkg/session/worker.go` — 会话生命周期
- `pkg/session/broadcaster.go` — 广播机制
- `internal/api/subagents.go` — 现有 SubAgent API
- `internal/api/router.go` — 路由注册
- `ui/src/components/AiChat.vue` — 需要在顶部嵌入 DispatchPanel
- `ui/src/api/index.ts` — API 封装方式

---

## 第一步：后端 — 新增类型

**修改 `pkg/subagent/types.go`，补充：**

```go
// SubagentReport 子成员汇报
type SubagentReport struct {
    ID                string    `json:"id"`
    SubagentSessionID string    `json:"subagentSessionId"`
    AgentID           string    `json:"agentId"`
    ParentSessionID   string    `json:"parentSessionId"`
    Content           string    `json:"content"`
    Status            string    `json:"status"`   // "running" | "blocked" | "done"
    Progress          int       `json:"progress"` // 0-100
    Timestamp         time.Time `json:"timestamp"`
}

// SubagentEvent SSE 事件（统一格式，发往父会话）
type SubagentEvent struct {
    Type              string `json:"type"`              // "spawn"|"report"|"done"|"error"
    SubagentSessionID string `json:"subagentSessionId"`
    AgentID           string `json:"agentId"`
    AgentName         string `json:"agentName"`
    AvatarColor       string `json:"avatarColor"`
    Content           string `json:"content,omitempty"`
    Status            string `json:"status,omitempty"`
    Progress          int    `json:"progress,omitempty"`
    Timestamp         int64  `json:"timestamp"`
}
```

---

## 第二步：后端 — 广播事件

**修改 `pkg/subagent/manager.go`：**

在 `Spawn()` 方法成功创建子会话后，广播 `subagent_spawn` 事件：
```go
// 读取子 agent 信息（name, avatarColor）
// 通过 broadcaster 广播给父会话
broadcaster.Publish(parentSessionID, session.Event{
    Type: "subagent_spawn",
    Data: SubagentEvent{
        Type:              "spawn",
        SubagentSessionID: subSessionID,
        AgentID:           agentID,
        AgentName:         agentName,    // 从 agent manager 获取
        AvatarColor:       avatarColor,  // 从 agent manager 获取
        Timestamp:         time.Now().UnixMilli(),
    },
})
```

**修改 `pkg/session/worker.go`：**

在会话完成/失败回调中广播：
```go
// 完成时
if parentSessionID != "" {
    broadcaster.Publish(parentSessionID, session.Event{
        Type: "subagent_done",
        Data: subagent.SubagentEvent{
            Type:              "done",
            SubagentSessionID: sessionID,
            AgentID:           agentID,
            Timestamp:         time.Now().UnixMilli(),
        },
    })
}
// 失败时类似，Type 改为 "subagent_error"
```

---

## 第三步：后端 — report_to_parent 工具

**修改 `pkg/tools/tools.go`，新增工具定义：**

```go
{
    Name: "report_to_parent",
    Description: "向上级汇报当前执行进展。在完成重要步骤、遇到阻碍或任务完成时调用。上级会实时收到汇报内容显示在面板中。",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "content":  map[string]any{"type": "string", "description": "汇报内容，20-100字"},
            "progress": map[string]any{"type": "integer", "minimum": 0, "maximum": 100, "description": "完成进度 0-100"},
            "status":   map[string]any{"type": "string", "enum": []string{"running","blocked","done"}, "description": "running/blocked/done"},
        },
        "required": []string{"content", "status"},
    },
},
```

**修改 `pkg/tools/registry.go`，实现执行逻辑：**

```go
case "report_to_parent":
    content  := getString(input, "content")
    status   := getString(input, "status")
    progress := getInt(input, "progress")

    // 从 ctx 获取会话信息
    sessionID       := ctx.Value(ctxKeySessionID).(string)
    parentSessionID := ctx.Value(ctxKeyParentSessionID).(string)
    agentID         := ctx.Value(ctxKeyAgentID).(string)

    if parentSessionID == "" {
        return "（当前未在派遣任务中，无需汇报）", nil
    }

    event := subagent.SubagentEvent{
        Type:              "report",
        SubagentSessionID: sessionID,
        AgentID:           agentID,
        Content:           content,
        Status:            status,
        Progress:          progress,
        Timestamp:         time.Now().UnixMilli(),
    }
    broadcaster.Publish(parentSessionID, session.Event{Type: "subagent_report", Data: event})
    return "汇报已发送给上级", nil
```

---

## 第四步：后端 — Runner 注入上下文

**修改 `pkg/runner/runner.go`：**

1. `RunConfig` 结构体补充字段：
```go
type RunConfig struct {
    // ...现有字段...
    SessionID       string // 当前会话 ID
    ParentSessionID string // 父会话 ID（派遣时才有）
}
```

2. 工具执行时注入 ctx：
```go
ctx = context.WithValue(ctx, ctxKeySessionID, cfg.SessionID)
ctx = context.WithValue(ctx, ctxKeyParentSessionID, cfg.ParentSessionID)
ctx = context.WithValue(ctx, ctxKeyAgentID, agentID)
```

3. 当 ParentSessionID 非空时，在系统提示词末尾自动追加汇报指引：
```go
if cfg.ParentSessionID != "" {
    systemPrompt += "\n\n## 任务汇报\n你正在作为子成员执行上级委派的任务。请在完成重要步骤时调用 report_to_parent 工具汇报进展（20-100字，附上进度百分比）。任务全部完成时 status=done, progress=100。"
}
```

---

## 第五步：后端 — 新增 API

**修改 `internal/api/subagents.go`，新增：**

```go
// GET /api/sessions/:sessionId/subagent-events
// 返回该会话历史子会话事件（补偿查询，页面刷新后恢复面板状态）
func (h *subagentHandler) ListEvents(c *gin.Context) {
    sessionID := c.Param("sessionId")
    events := h.manager.ListEvents(sessionID) // 从内存/文件读取
    c.JSON(200, events)
}
```

在 `router.go` 注册：
```go
sessions.GET("/:sessionId/subagent-events", subagentH.ListEvents)
```

---

## 第六步：前端 — DispatchPanel 组件

**新建 `ui/src/components/DispatchPanel.vue`：**

```vue
<template>
  <!-- 整体面板，有活跃子成员时从顶部滑入 -->
  <Transition name="panel-slide">
    <div v-if="hasActive" class="dispatch-panel">

      <!-- 标题栏 -->
      <div class="dp-header">
        <span class="dp-pulse" />
        <span>派遣任务进行中</span>
        <span class="dp-count">{{ activeList.length }} 名成员执行中</span>
        <el-button link size="small" @click="collapsed = !collapsed" style="margin-left:auto">
          {{ collapsed ? '展开 ∨' : '收起 ∧' }}
        </el-button>
      </div>

      <!-- 成员列表 -->
      <Transition name="dp-expand">
        <div v-if="!collapsed" class="dp-body">
          <TransitionGroup name="member-fly" tag="div" class="dp-members">
            <div v-for="(d, idx) in sortedDispatchers" :key="d.subagentSessionId"
                 class="dp-member"
                 :style="{ transitionDelay: idx * 80 + 'ms' }">

              <!-- 头像 -->
              <div class="dp-avatar" :class="'status-' + d.status"
                   :style="{ background: d.avatarColor }">
                {{ d.agentName?.[0] ?? '?' }}
                <span v-if="d.status === 'done'" class="dp-done-badge">✓</span>
              </div>

              <!-- 信息 -->
              <div class="dp-info">
                <div class="dp-name-row">
                  <span class="dp-name">{{ d.agentName }}</span>
                  <el-tag size="small" :type="statusTagType(d.status)" effect="plain">
                    {{ statusLabel(d.status) }}
                  </el-tag>
                  <div v-if="d.progress > 0" class="dp-progress-wrap">
                    <div class="dp-progress-bar" :style="{ width: d.progress + '%' }" />
                    <span class="dp-progress-num">{{ d.progress }}%</span>
                  </div>
                </div>
                <!-- 最新汇报 -->
                <div v-if="d.latestReport" class="dp-report" :class="{ 'dp-report-new': d.reportNew }">
                  "{{ truncate(d.latestReport, 60) }}"
                  <el-button v-if="d.reports.length > 1" link size="small"
                    @click="viewReports(d)">全部</el-button>
                </div>
              </div>

            </div>
          </TransitionGroup>
        </div>
      </Transition>

    </div>
  </Transition>

  <!-- 汇报详情弹窗 -->
  <el-dialog v-model="reportDialogVisible" :title="reportDialogAgent + ' 的汇报记录'"
             width="480px" append-to-body>
    <el-timeline>
      <el-timeline-item v-for="r in reportDialogRecords" :key="r.timestamp"
        :timestamp="formatTime(r.timestamp)"
        :type="r.status === 'done' ? 'success' : r.status === 'blocked' ? 'danger' : 'primary'">
        {{ r.content }}
        <el-tag v-if="r.progress > 0" size="small" style="margin-left:8px">{{ r.progress }}%</el-tag>
      </el-timeline-item>
    </el-timeline>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

interface ReportEntry { content: string; status: string; progress: number; timestamp: number }
interface DispatcherState {
  subagentSessionId: string; agentId: string; agentName: string; avatarColor: string
  status: 'running' | 'blocked' | 'done' | 'error'
  progress: number; reports: ReportEntry[]; latestReport: string
  reportNew: boolean; spawnedAt: number; doneAt?: number
}

const props = defineProps<{ sessionId: string }>()

const dispatchers = ref<Map<string, DispatcherState>>(new Map())
const collapsed = ref(false)
const reportDialogVisible = ref(false)
const reportDialogAgent = ref('')
const reportDialogRecords = ref<ReportEntry[]>([])

const hasActive = computed(() => dispatchers.value.size > 0)
const activeList = computed(() => [...dispatchers.value.values()].filter(d => d.status !== 'done' && d.status !== 'error'))
const sortedDispatchers = computed(() =>
  [...dispatchers.value.values()].sort((a, b) => a.spawnedAt - b.spawnedAt)
)

function handleEvent(raw: any) {
  const data = typeof raw === 'string' ? JSON.parse(raw) : raw
  const id = data.subagentSessionId
  if (!id) return

  if (data.type === 'subagent_spawn') {
    dispatchers.value.set(id, {
      subagentSessionId: id, agentId: data.agentId,
      agentName: data.agentName, avatarColor: data.avatarColor || '#6366f1',
      status: 'running', progress: 0, reports: [],
      latestReport: '', reportNew: false, spawnedAt: data.timestamp,
    })
    dispatchers.value = new Map(dispatchers.value) // trigger reactivity
  } else if (data.type === 'subagent_report') {
    const d = dispatchers.value.get(id)
    if (d) {
      d.reports.push({ content: data.content, status: data.status, progress: data.progress || 0, timestamp: data.timestamp })
      d.latestReport = data.content
      d.progress = data.progress || d.progress
      d.status = data.status === 'done' ? 'done' : 'running'
      d.reportNew = true
      setTimeout(() => { if (d) d.reportNew = false }, 800)
      dispatchers.value = new Map(dispatchers.value)
    }
  } else if (data.type === 'subagent_done') {
    const d = dispatchers.value.get(id)
    if (d) {
      d.status = 'done'; d.doneAt = data.timestamp
      dispatchers.value = new Map(dispatchers.value)
      setTimeout(() => { dispatchers.value.delete(id); dispatchers.value = new Map(dispatchers.value) }, 3000)
    }
  } else if (data.type === 'subagent_error') {
    const d = dispatchers.value.get(id)
    if (d) { d.status = 'error'; dispatchers.value = new Map(dispatchers.value) }
  }
}

// 暴露给父组件调用
defineExpose({ handleEvent })

function statusTagType(s: string) {
  return s === 'done' ? 'success' : s === 'blocked' ? 'warning' : s === 'error' ? 'danger' : 'primary'
}
function statusLabel(s: string) {
  return { running: '执行中', blocked: '遇到阻碍', done: '已完成', error: '出错' }[s] ?? s
}
function truncate(s: string, n: number) { return s.length > n ? s.slice(0, n) + '…' : s }
function formatTime(ts: number) { return new Date(ts).toLocaleTimeString('zh-CN') }
function viewReports(d: DispatcherState) {
  reportDialogAgent.value = d.agentName
  reportDialogRecords.value = d.reports
  reportDialogVisible.value = true
}
</script>

<style scoped>
.dispatch-panel { background: var(--el-bg-color-page); border-bottom: 1px solid var(--el-border-color); }
.dp-header { display: flex; align-items: center; gap: 8px; padding: 8px 16px; font-size: 13px; font-weight: 500; }
.dp-pulse { width: 8px; height: 8px; border-radius: 50%; background: #409eff; animation: pulse 1.4s infinite; flex-shrink: 0; }
.dp-count { color: var(--el-text-color-secondary); font-size: 12px; }
.dp-body { padding: 6px 16px 10px; }
.dp-members { display: flex; flex-direction: column; gap: 8px; }
.dp-member { display: flex; align-items: flex-start; gap: 10px; }
.dp-avatar {
  width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center;
  justify-content: center; color: #fff; font-weight: 700; font-size: 15px;
  flex-shrink: 0; position: relative;
}
.dp-done-badge {
  position: absolute; bottom: -2px; right: -2px; width: 14px; height: 14px;
  border-radius: 50%; background: #67c23a; color: #fff; font-size: 9px;
  display: flex; align-items: center; justify-content: center;
}
.status-running { animation: breathing 1.8s ease-in-out infinite; }
.dp-info { flex: 1; min-width: 0; }
.dp-name-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.dp-name { font-size: 13px; font-weight: 500; }
.dp-progress-wrap { display: flex; align-items: center; gap: 4px; flex: 1; min-width: 80px; }
.dp-progress-bar { height: 4px; background: #409eff; border-radius: 2px; transition: width 0.4s; }
.dp-progress-num { font-size: 11px; color: var(--el-text-color-secondary); white-space: nowrap; }
.dp-report {
  margin-top: 3px; font-size: 12px; color: var(--el-text-color-secondary);
  font-style: italic; border-left: 2px solid var(--el-border-color); padding-left: 6px;
  transition: background 0.4s;
}
.dp-report-new { background: rgba(64,158,255,0.08); border-radius: 0 4px 4px 0; }

/* 动画 */
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:.4} }
@keyframes breathing { 0%,100%{box-shadow:0 0 0 0 rgba(64,158,255,.6)} 50%{box-shadow:0 0 0 5px rgba(64,158,255,0)} }

.panel-slide-enter-active, .panel-slide-leave-active { transition: all .3s ease; }
.panel-slide-enter-from, .panel-slide-leave-to { opacity: 0; transform: translateY(-100%); }
.dp-expand-enter-active, .dp-expand-leave-active { transition: all .25s ease; overflow: hidden; }
.dp-expand-enter-from, .dp-expand-leave-to { max-height: 0; opacity: 0; }
.dp-expand-enter-to, .dp-expand-leave-from { max-height: 500px; opacity: 1; }

.member-fly-enter-active { transition: all .4s cubic-bezier(.34,1.56,.64,1); }
.member-fly-enter-from { opacity: 0; transform: translateX(30px); }
.member-fly-leave-active { transition: all .3s ease; }
.member-fly-leave-to { opacity: 0; transform: translateX(30px); }
</style>
```

---

## 第七步：前端 — AiChat 集成 DispatchPanel

**修改 `ui/src/components/AiChat.vue`：**

1. 在 `<template>` 最顶部（聊天内容区上方）加入：
```vue
<DispatchPanel v-if="sessionKey" :session-id="sessionKey" ref="dispatchPanelRef" />
```

2. 在 `<script setup>` 中：
```typescript
import DispatchPanel from './DispatchPanel.vue'
const dispatchPanelRef = ref<InstanceType<typeof DispatchPanel> | null>(null)
```

3. 在现有 SSE 事件处理中，增加对 `subagent_*` 事件的转发：
```typescript
// 在处理 SSE data 的地方，找到解析 event.data 的逻辑
if (data.type?.startsWith('subagent_')) {
  dispatchPanelRef.value?.handleEvent(data)
}
```

---

## 第八步：前端 — API 补充

**修改 `ui/src/api/index.ts`，新增：**
```typescript
export const getSubagentEvents = (sessionId: string) =>
  request(`/api/sessions/${encodeURIComponent(sessionId)}/subagent-events`)
```

---

## 完成后
1. `cd ui && npm run build`
2. `make build`
3. 验证：在聊天中触发子任务，观察顶部面板动画

完成后执行：
openclaw system event --text "派遣面板系统实现完成，已build成功" --mode now
