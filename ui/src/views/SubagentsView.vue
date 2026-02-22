<template>
  <div class="tasks-page">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h2 style="margin:0">后台任务</h2>
        <el-tag v-if="runningCount > 0" type="success" size="small" effect="dark">
          {{ runningCount }} 运行中
        </el-tag>
      </div>
      <div class="header-actions">
        <el-button size="small" @click="refresh" :loading="loading">刷新</el-button>
        <el-button size="small" type="primary" @click="openSpawnDialog('task')">
          <el-icon><Plus /></el-icon> 派遣任务
        </el-button>
        <el-button size="small" type="warning" plain @click="openSpawnDialog('report')">
          <el-icon><ChatLineRound /></el-icon> 汇报
        </el-button>
      </div>
    </div>

    <!-- Filter bar -->
    <div class="filter-bar">
      <el-select v-model="filterStatus" placeholder="所有状态" clearable size="small" style="width:120px;">
        <el-option label="运行中" value="running" />
        <el-option label="已完成" value="done" />
        <el-option label="出错" value="error" />
        <el-option label="已终止" value="killed" />
        <el-option label="等待中" value="pending" />
      </el-select>
      <el-select v-model="filterType" placeholder="所有类型" clearable size="small" style="width:120px;">
        <el-option label="派遣任务" value="task" />
        <el-option label="汇报" value="report" />
        <el-option label="系统" value="system" />
      </el-select>
      <el-select v-model="filterAgent" placeholder="所有成员" clearable size="small" style="width:140px;">
        <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
      </el-select>
      <span class="filter-count">共 {{ filteredTasks.length }} 个</span>
    </div>

    <!-- Empty state -->
    <el-empty v-if="filteredTasks.length === 0 && !loading"
      description="暂无后台任务"
      style="margin-top: 60px"
    >
      <template #description>
        <p style="color:#94a3b8; font-size:13px; text-align:center; margin:0">
          上级可向下级「派遣任务」<br>
          下级可向上级「汇报」<br>
          平级协作成员可互相派遣与汇报
        </p>
      </template>
    </el-empty>

    <!-- Task cards -->
    <div v-else class="task-list">
      <div v-for="task in filteredTasks" :key="task.id"
        class="task-card"
        :class="`task-${task.status}`"
      >
        <!-- Card header -->
        <div class="task-header">
          <div class="task-meta-row">
            <!-- Status -->
            <el-tag :type="statusType(task.status)" size="small" effect="dark" style="flex-shrink:0">
              {{ statusLabel(task.status) }}
            </el-tag>
            <!-- Task type badge -->
            <el-tag
              :type="taskTypeTagType(task.taskType)"
              size="small"
              effect="plain"
              style="flex-shrink:0"
            >
              {{ taskTypeLabel(task.taskType) }}
            </el-tag>
            <!-- Relation badge -->
            <el-tag v-if="task.relation" size="small" type="info" effect="plain" style="flex-shrink:0">
              {{ task.relation }}
            </el-tag>
            <!-- Label -->
            <span class="task-label">{{ task.label || '（无标签）' }}</span>
            <code class="task-id">{{ task.id }}</code>
          </div>
          <div class="task-actions">
            <el-button
              v-if="task.status === 'running' || task.status === 'pending'"
              size="small" type="danger" link
              @click="killTask(task.id)"
              :loading="killing === task.id"
            >终止</el-button>
            <el-button size="small" link @click="viewTask(task)">查看输出</el-button>
          </div>
        </div>

        <!-- Agent flow: who → who -->
        <div class="agent-flow">
          <template v-if="task.spawnedBy">
            <div class="agent-chip" :style="{ background: agentColor(task.spawnedBy) + '22', borderColor: agentColor(task.spawnedBy) }">
              <div class="agent-dot" :style="{ background: agentColor(task.spawnedBy) }">{{ agentInitial(task.spawnedBy) }}</div>
              <span>{{ agentName(task.spawnedBy) }}</span>
            </div>
            <span class="flow-arrow">{{ task.taskType === 'report' ? '⬆ 汇报' : '⬇ 派遣' }}</span>
          </template>
          <div class="agent-chip" :style="{ background: agentColor(task.agentId) + '22', borderColor: agentColor(task.agentId) }">
            <div class="agent-dot" :style="{ background: agentColor(task.agentId) }">{{ agentInitial(task.agentId) }}</div>
            <span>{{ agentName(task.agentId) }}</span>
          </div>
          <span class="task-time">⏱ {{ durationStr(task) }} · {{ formatTime(task.createdAt) }}</span>
        </div>

        <!-- Task description -->
        <div class="task-desc">{{ task.task }}</div>

        <!-- Error -->
        <el-alert v-if="task.error" type="error" :description="task.error" :closable="false" show-icon style="margin-top:6px;" />

        <!-- Output preview -->
        <div v-if="task.output && task.status !== 'pending'" class="output-preview">
          <pre>{{ outputPreview(task.output) }}</pre>
        </div>

        <!-- Running progress -->
        <el-progress v-if="task.status === 'running'" :percentage="100"
          status="striped" striped striped-flow :duration="3"
          style="margin-top:8px;"
        />
      </div>
    </div>

    <!-- ═══ Spawn / Report Dialog ═══ -->
    <el-dialog
      v-model="showSpawnDialog"
      :title="spawnMode === 'report' ? '向上级汇报' : '派遣任务'"
      width="560px"
      :close-on-click-modal="false"
    >
      <!-- Mode explainer -->
      <el-alert
        :type="spawnMode === 'report' ? 'warning' : 'info'"
        :title="spawnMode === 'report' ? '汇报：下级向上级发送任务完成情况或定期汇报' : '派遣：上级向下级分配任务，或平级协作互相委托'"
        :closable="false"
        show-icon
        style="margin-bottom: 16px"
      />

      <el-form :model="spawnForm" label-width="80px" size="small">
        <!-- 发起成员 -->
        <el-form-item label="发起成员" required>
          <el-select
            v-model="spawnForm.spawnedBy"
            placeholder="选择发起成员"
            style="width:100%"
            @change="onSpawnedByChange"
            clearable
          >
            <el-option
              v-for="a in allAgents"
              :key="a.id"
              :label="a.name"
              :value="a.id"
            >
              <div class="agent-option">
                <div class="agent-dot-sm" :style="{ background: a.avatarColor || '#6366f1' }">{{ a.name.charAt(0) }}</div>
                <span>{{ a.name }}</span>
                <span class="agent-option-id">{{ a.id }}</span>
              </div>
            </el-option>
          </el-select>
        </el-form-item>

        <!-- 目标成员 -->
        <el-form-item label="目标成员" required>
          <el-select
            v-model="spawnForm.agentId"
            :placeholder="spawnForm.spawnedBy ? (eligibleTargets.length ? '选择目标成员' : '无可用目标（检查关系配置）') : '请先选择发起成员'"
            style="width:100%"
            :disabled="!spawnForm.spawnedBy"
          >
            <el-option
              v-for="t in eligibleTargets"
              :key="t.agentId"
              :label="agentName(t.agentId)"
              :value="t.agentId"
            >
              <div class="agent-option">
                <div class="agent-dot-sm" :style="{ background: agentColor(t.agentId) }">{{ agentInitial(t.agentId) }}</div>
                <span>{{ agentName(t.agentId) }}</span>
                <el-tag size="small" type="info" effect="plain" style="font-size:11px;">{{ t.relation }}</el-tag>
              </div>
            </el-option>
          </el-select>
          <div v-if="spawnForm.spawnedBy && eligibleTargets.length === 0" class="permission-hint">
            <el-icon><WarningFilled /></el-icon>
            {{ spawnMode === 'report' ? '该成员没有上级或平级协作关系' : '该成员没有下级或平级协作关系' }}
            — 请先在「团队」页面配置关系
          </div>
        </el-form-item>

        <!-- 任务标签 -->
        <el-form-item label="标签">
          <el-input v-model="spawnForm.label" :placeholder="spawnMode === 'report' ? '如：月度汇报、任务完成通知' : '简短标签（可选）'" />
        </el-form-item>

        <!-- 模型 -->
        <el-form-item label="模型">
          <el-input v-model="spawnForm.model" placeholder="留空使用目标成员默认模型" />
        </el-form-item>

        <!-- 任务描述 -->
        <el-form-item :label="spawnMode === 'report' ? '汇报内容' : '任务描述'" required>
          <el-input
            v-model="spawnForm.task"
            type="textarea"
            :rows="6"
            :placeholder="spawnMode === 'report'
              ? '汇报内容：任务进展、完成情况、需要上级了解的事项...'
              : '详细描述任务目标、要求、预期结果...'"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showSpawnDialog = false">取消</el-button>
        <el-button
          :type="spawnMode === 'report' ? 'warning' : 'primary'"
          @click="spawnTask"
          :loading="spawning"
          :disabled="!spawnForm.agentId || !spawnForm.task.trim()"
        >
          {{ spawnMode === 'report' ? '发送汇报' : '派遣任务' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Output Detail Dialog -->
    <el-dialog
      v-model="showOutputDialog"
      :title="`输出 — ${selectedTask?.label || selectedTask?.id}`"
      width="720px"
      top="5vh"
    >
      <div v-if="selectedTask">
        <div style="display:flex; gap:10px; margin-bottom:12px; flex-wrap:wrap; align-items:center;">
          <el-tag :type="statusType(selectedTask.status)" effect="dark">{{ statusLabel(selectedTask.status) }}</el-tag>
          <el-tag :type="taskTypeTagType(selectedTask.taskType)" effect="plain">{{ taskTypeLabel(selectedTask.taskType) }}</el-tag>
          <el-tag v-if="selectedTask.relation" type="info" effect="plain">{{ selectedTask.relation }}</el-tag>
          <template v-if="selectedTask.spawnedBy">
            <span style="font-size:13px;color:#64748b;">{{ agentName(selectedTask.spawnedBy) }}</span>
            <span style="color:#94a3b8;">{{ selectedTask.taskType === 'report' ? '⬆' : '⬇' }}</span>
          </template>
          <span style="font-size:13px;color:#64748b;">🤖 {{ agentName(selectedTask.agentId) }}</span>
          <span style="font-size:13px;color:#94a3b8;">⏱ {{ durationStr(selectedTask) }}</span>
        </div>
        <div class="output-task-desc">{{ selectedTask.task }}</div>
        <el-alert v-if="selectedTask.error" type="error" :description="selectedTask.error" :closable="false" show-icon style="margin-bottom:12px;" />
        <div style="font-size:13px;font-weight:600;color:#475569;margin-bottom:6px;">输出：</div>
        <pre class="output-full">{{ selectedTask.output || '（暂无输出）' }}</pre>
      </div>
      <template #footer>
        <el-button @click="showOutputDialog = false">关闭</el-button>
        <el-button
          v-if="selectedTask && (selectedTask.status === 'running' || selectedTask.status === 'pending')"
          type="danger"
          @click="killTask(selectedTask.id); showOutputDialog = false"
        >终止</el-button>
        <el-button v-if="selectedTask?.output" @click="copyOutput(selectedTask.output)">复制</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, ChatLineRound, WarningFilled } from '@element-plus/icons-vue'
import { tasks as tasksApi, agents as agentsApi } from '../api/index'
import type { AgentInfo, TaskInfo, EligibleTarget } from '../api/index'

const taskList = ref<TaskInfo[]>([])
const agents = ref<AgentInfo[]>([])
const allAgents = ref<AgentInfo[]>([])
const loading = ref(false)
const killing = ref<string | null>(null)
const filterStatus = ref('')
const filterType = ref('')
const filterAgent = ref('')
const showSpawnDialog = ref(false)
const showOutputDialog = ref(false)
const selectedTask = ref<TaskInfo | null>(null)
const spawning = ref(false)
const spawnMode = ref<'task' | 'report'>('task')
const eligibleTargets = ref<EligibleTarget[]>([])
let refreshTimer: number | undefined

const spawnForm = ref({
  spawnedBy: '',
  agentId: '',
  label: '',
  task: '',
  model: '',
})

// ── Computed ──────────────────────────────────────────────────────────────────

const filteredTasks = computed(() => {
  return taskList.value.filter(t => {
    if (filterStatus.value && t.status !== filterStatus.value) return false
    if (filterType.value && (t.taskType || 'task') !== filterType.value) return false
    if (filterAgent.value && t.agentId !== filterAgent.value && t.spawnedBy !== filterAgent.value) return false
    return true
  }).sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0))
})

const runningCount = computed(() =>
  taskList.value.filter(t => t.status === 'running' || t.status === 'pending').length
)

// ── Helpers ───────────────────────────────────────────────────────────────────

function agentName(id: string) {
  return allAgents.value.find(a => a.id === id)?.name || id
}

function agentColor(id: string) {
  return allAgents.value.find(a => a.id === id)?.avatarColor || '#6366f1'
}

function agentInitial(id: string) {
  const name = agentName(id)
  return name.charAt(0)
}

function statusType(status: string) {
  const m: Record<string, string> = { running: 'success', done: 'primary', error: 'danger', killed: 'warning', pending: 'info' }
  return m[status] || 'info'
}

function statusLabel(status: string) {
  const m: Record<string, string> = { running: '运行中', done: '已完成', error: '出错', killed: '已终止', pending: '等待中' }
  return m[status] || status
}

function taskTypeLabel(type?: string) {
  switch (type) {
    case 'report': return '汇报'
    case 'system': return '系统'
    default: return '派遣'
  }
}

function taskTypeTagType(type?: string) {
  switch (type) {
    case 'report': return 'warning'
    case 'system': return 'info'
    default: return 'primary'
  }
}

function formatTime(ms: number) {
  if (!ms) return '—'
  const d = new Date(ms)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function durationStr(task: TaskInfo) {
  if (!task.startedAt) return '—'
  const end = task.endedAt || Date.now()
  const ms = end - task.startedAt
  if (ms < 1000) return '< 1s'
  if (ms < 60000) return `${Math.round(ms / 1000)}s`
  const m = Math.floor(ms / 60000)
  const s = Math.round((ms % 60000) / 1000)
  return `${m}m${s}s`
}

function outputPreview(output: string) {
  const lines = output.trim().split('\n')
  return lines.slice(-3).join('\n')
}

// ── Actions ───────────────────────────────────────────────────────────────────

async function refresh() {
  loading.value = true
  try {
    const res = await tasksApi.list()
    taskList.value = res.data
    if (selectedTask.value) {
      const updated = taskList.value.find(t => t.id === selectedTask.value!.id)
      if (updated) selectedTask.value = updated
    }
  } catch { /* silent */ }
  finally { loading.value = false }
}

async function loadAgents() {
  try {
    const res = await agentsApi.list()
    allAgents.value = res.data
    agents.value = res.data.filter(a => !a.system)
  } catch {}
}

async function killTask(id: string) {
  killing.value = id
  try {
    await tasksApi.kill(id)
    ElMessage.success('任务已终止')
    await refresh()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '终止失败')
  } finally { killing.value = null }
}

function viewTask(task: TaskInfo) {
  selectedTask.value = task
  showOutputDialog.value = true
}

function openSpawnDialog(mode: 'task' | 'report') {
  spawnMode.value = mode
  spawnForm.value = { spawnedBy: '', agentId: '', label: '', task: '', model: '' }
  eligibleTargets.value = []
  showSpawnDialog.value = true
}

async function onSpawnedByChange(id: string) {
  spawnForm.value.agentId = ''
  eligibleTargets.value = []
  if (!id) return
  try {
    const res = await tasksApi.eligibleTargets(id, spawnMode.value)
    eligibleTargets.value = res.data
  } catch {}
}

async function spawnTask() {
  if (!spawnForm.value.agentId || !spawnForm.value.task.trim()) return
  spawning.value = true
  try {
    await tasksApi.spawn({
      agentId: spawnForm.value.agentId,
      label: spawnForm.value.label,
      task: spawnForm.value.task,
      model: spawnForm.value.model,
      spawnedBy: spawnForm.value.spawnedBy || undefined,
      taskType: spawnMode.value,
    })
    ElMessage.success(spawnMode.value === 'report' ? '汇报已发送' : '任务已派遣，后台执行中')
    showSpawnDialog.value = false
    await refresh()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '操作失败')
  } finally { spawning.value = false }
}

function copyOutput(output: string) {
  navigator.clipboard?.writeText(output)
  ElMessage.success('已复制')
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

onMounted(async () => {
  await loadAgents()
  await refresh()
  refreshTimer = window.setInterval(() => {
    if (runningCount.value > 0) refresh()
  }, 3000)
})

onUnmounted(() => { if (refreshTimer) clearInterval(refreshTimer) })
</script>

<style scoped>
.tasks-page { padding: 20px; max-width: 900px; }

/* ── Header ── */
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 10px;
}
.header-left { display: flex; align-items: center; gap: 10px; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; }

/* ── Filter bar ── */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.filter-count { color: #94a3b8; font-size: 13px; margin-left: 4px; }

/* ── Task list ── */
.task-list { display: flex; flex-direction: column; gap: 12px; }

.task-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-left: 4px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
  transition: box-shadow 0.2s;
}
.task-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,0.08); }
.task-running  { border-left-color: #10b981; }
.task-done     { border-left-color: #3b82f6; }
.task-error    { border-left-color: #ef4444; }
.task-killed   { border-left-color: #f59e0b; }
.task-pending  { border-left-color: #94a3b8; }

/* ── Task header ── */
.task-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 10px;
  gap: 8px;
}
.task-meta-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  flex: 1;
  min-width: 0;
}
.task-label {
  font-weight: 600;
  font-size: 14px;
  color: #1e293b;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
}
.task-id { font-size: 11px; color: #94a3b8; font-family: monospace; flex-shrink: 0; }
.task-actions { display: flex; gap: 4px; flex-shrink: 0; }

/* ── Agent flow ── */
.agent-flow {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.agent-chip {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 2px 8px 2px 4px;
  border-radius: 20px;
  border: 1px solid;
  font-size: 12px;
  color: #475569;
}
.agent-dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  flex-shrink: 0;
}
.flow-arrow {
  font-size: 12px;
  color: #64748b;
  font-weight: 600;
}
.task-time { font-size: 12px; color: #94a3b8; margin-left: auto; white-space: nowrap; }

/* ── Task description ── */
.task-desc {
  background: #f8fafc;
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 13px;
  color: #475569;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 60px;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

/* ── Output preview ── */
.output-preview {
  margin-top: 8px;
  background: #f1f5f9;
  border-radius: 6px;
  padding: 8px;
}
.output-preview pre {
  margin: 0;
  font-size: 12px;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 60px;
  overflow: hidden;
  font-family: monospace;
}

/* ── Spawn dialog ── */
.agent-option {
  display: flex;
  align-items: center;
  gap: 8px;
}
.agent-dot-sm {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  flex-shrink: 0;
}
.agent-option-id { color: #94a3b8; font-size: 12px; margin-left: auto; }
.permission-hint {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: #f59e0b;
  margin-top: 5px;
}

/* ── Output dialog ── */
.output-task-desc {
  background: #f8fafc;
  border-radius: 6px;
  padding: 10px 14px;
  margin-bottom: 12px;
  font-size: 13px;
  color: #475569;
}
.output-full {
  background: #0f172a;
  color: #e2e8f0;
  padding: 16px;
  border-radius: 8px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow-y: auto;
  margin: 0;
  font-family: monospace;
}

/* ── Mobile ── */
@media (max-width: 768px) {
  .tasks-page { padding: 12px; }
  .task-label { max-width: 130px; }
  .task-time { margin-left: 0; }
  .header-actions .el-button { font-size: 12px; }
}
</style>
