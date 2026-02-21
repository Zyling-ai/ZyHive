<template>
  <el-container style="height: 100vh; flex-direction: column;">
    <!-- Header -->
    <el-header style="display:flex; align-items:center; justify-content:space-between; padding: 0 20px; border-bottom: 1px solid #e2e8f0; background:#fff;">
      <div style="display:flex; align-items:center; gap:12px;">
        <span style="font-size:18px; font-weight:600; color:#1e293b;">⚡ 后台任务</span>
        <el-tag v-if="runningCount > 0" type="success" size="small" effect="dark">
          {{ runningCount }} 运行中
        </el-tag>
        <el-tag v-if="tasks.length === 0" type="info" size="small">暂无任务</el-tag>
      </div>
      <div style="display:flex; gap:8px;">
        <el-button size="small" @click="refresh" :loading="loading" icon="Refresh">刷新</el-button>
        <el-button size="small" type="primary" @click="showSpawnDialog = true" icon="Plus">
          派生任务
        </el-button>
      </div>
    </el-header>

    <el-main style="padding: 20px; overflow-y: auto;">
      <!-- Filter bar -->
      <div style="display:flex; gap:8px; margin-bottom:16px; align-items:center; flex-wrap:wrap;">
        <el-select v-model="filterStatus" placeholder="所有状态" clearable size="small" style="width:140px;">
          <el-option label="运行中" value="running" />
          <el-option label="已完成" value="done" />
          <el-option label="出错" value="error" />
          <el-option label="已终止" value="killed" />
          <el-option label="等待中" value="pending" />
        </el-select>
        <el-select v-model="filterAgent" placeholder="所有成员" clearable size="small" style="width:160px;">
          <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
        </el-select>
        <span style="color:#94a3b8; font-size:13px;">共 {{ filteredTasks.length }} 个任务</span>
      </div>

      <!-- Empty state -->
      <el-empty v-if="filteredTasks.length === 0 && !loading"
        description="暂无后台任务。点击「派生任务」让 AI 成员在后台执行任务。"
        style="margin-top:60px;"
      />

      <!-- Task cards -->
      <div v-else style="display:flex; flex-direction:column; gap:12px;">
        <div v-for="task in filteredTasks" :key="task.id" class="task-card" :class="'task-' + task.status">
          <!-- Card header -->
          <div style="display:flex; align-items:flex-start; justify-content:space-between; margin-bottom:10px;">
            <div style="display:flex; align-items:center; gap:10px; flex:1; min-width:0;">
              <el-tag :type="statusType(task.status)" size="small" effect="dark" style="flex-shrink:0;">
                {{ statusLabel(task.status) }}
              </el-tag>
              <span style="font-weight:600; font-size:14px; color:#1e293b; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                {{ task.label || '无标签' }}
              </span>
              <code style="font-size:11px; color:#94a3b8; flex-shrink:0;">{{ task.id }}</code>
            </div>
            <div style="display:flex; gap:6px; flex-shrink:0; margin-left:12px;">
              <el-button
                v-if="task.status === 'running' || task.status === 'pending'"
                size="small" type="danger" link
                @click="killTask(task.id)"
                :loading="killing === task.id"
              >终止</el-button>
              <el-button size="small" link @click="viewTask(task)">查看输出</el-button>
            </div>
          </div>

          <!-- Meta info -->
          <div style="display:flex; gap:16px; margin-bottom:10px; flex-wrap:wrap;">
            <span style="font-size:12px; color:#64748b;">
              🤖 <strong>{{ agentName(task.agentId) }}</strong>
            </span>
            <span v-if="task.spawnedBy" style="font-size:12px; color:#64748b;">
              ↑ 派发自 {{ agentName(task.spawnedBy) }}
            </span>
            <span style="font-size:12px; color:#64748b;">⏱ {{ task.duration || durationStr(task) }}</span>
            <span style="font-size:12px; color:#64748b;">
              📅 {{ formatTime(task.createdAt) }}
            </span>
            <el-tag v-if="task.model" size="small" type="info" effect="plain">{{ task.model }}</el-tag>
          </div>

          <!-- Task description -->
          <div style="background:#f8fafc; border-radius:6px; padding:8px 12px; margin-bottom:8px;">
            <p style="margin:0; font-size:13px; color:#475569; white-space:pre-wrap; word-break:break-all; max-height:60px; overflow:hidden;">{{ task.task }}</p>
          </div>

          <!-- Error -->
          <el-alert v-if="task.error" type="error" :description="task.error" :closable="false" show-icon style="margin-top:4px;" />

          <!-- Output preview (last 3 lines) -->
          <div v-if="task.output && task.status !== 'pending'" style="margin-top:6px;">
            <div style="font-size:11px; color:#94a3b8; margin-bottom:4px;">输出预览：</div>
            <pre style="margin:0; font-size:12px; color:#334155; background:#f1f5f9; padding:8px; border-radius:6px; max-height:80px; overflow:hidden; white-space:pre-wrap; word-break:break-all;">{{ outputPreview(task.output) }}</pre>
          </div>

          <!-- Running indicator -->
          <el-progress v-if="task.status === 'running'" :percentage="100" status="striped" striped striped-flow :duration="3" style="margin-top:8px;" />
        </div>
      </div>
    </el-main>

    <!-- Spawn Task Dialog -->
    <el-dialog v-model="showSpawnDialog" title="派生后台任务" width="560px" :close-on-click-modal="false">
      <el-form :model="spawnForm" label-width="80px" size="small">
        <el-form-item label="执行成员" required>
          <el-select v-model="spawnForm.agentId" placeholder="选择 AI 成员" style="width:100%;">
            <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id">
              <div style="display:flex; align-items:center; gap:8px;">
                <div style="width:8px; height:8px; border-radius:50%;" :style="{ background: a.avatarColor || '#6366f1' }"></div>
                <span>{{ a.name }}</span>
                <span style="color:#94a3b8; font-size:12px;">{{ a.id }}</span>
              </div>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="任务标签">
          <el-input v-model="spawnForm.label" placeholder="简短标签，便于识别（可选）" />
        </el-form-item>
        <el-form-item label="模型" >
          <el-input v-model="spawnForm.model" placeholder="留空使用默认模型（如 anthropic/claude-sonnet-4-6）" />
        </el-form-item>
        <el-form-item label="任务描述" required>
          <el-input
            v-model="spawnForm.task"
            type="textarea"
            :rows="6"
            placeholder="详细描述这个 AI 成员需要完成的任务..."
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showSpawnDialog = false">取消</el-button>
        <el-button type="primary" @click="spawnTask" :loading="spawning"
          :disabled="!spawnForm.agentId || !spawnForm.task.trim()">
          派生任务
        </el-button>
      </template>
    </el-dialog>

    <!-- Output Detail Dialog -->
    <el-dialog v-model="showOutputDialog" :title="`任务输出 — ${selectedTask?.label || selectedTask?.id}`"
      width="720px" top="5vh">
      <div v-if="selectedTask">
        <div style="display:flex; gap:12px; margin-bottom:12px; flex-wrap:wrap;">
          <el-tag :type="statusType(selectedTask.status)" effect="dark">{{ statusLabel(selectedTask.status) }}</el-tag>
          <span style="font-size:13px; color:#64748b;">🤖 {{ agentName(selectedTask.agentId) }}</span>
          <span style="font-size:13px; color:#64748b;">⏱ {{ durationStr(selectedTask) }}</span>
        </div>
        <div style="background:#f8fafc; border-radius:6px; padding:10px 14px; margin-bottom:12px; font-size:13px; color:#475569;">
          <strong>任务：</strong>{{ selectedTask.task }}
        </div>
        <el-alert v-if="selectedTask.error" type="error" :description="selectedTask.error" :closable="false" show-icon style="margin-bottom:12px;" />
        <div style="font-size:13px; font-weight:600; color:#475569; margin-bottom:6px;">输出：</div>
        <pre style="background:#0f172a; color:#e2e8f0; padding:16px; border-radius:8px; font-size:12px; white-space:pre-wrap; word-break:break-all; max-height:400px; overflow-y:auto; margin:0;">{{ selectedTask.output || '（暂无输出）' }}</pre>
      </div>
      <template #footer>
        <el-button @click="showOutputDialog = false">关闭</el-button>
        <el-button
          v-if="selectedTask && (selectedTask.status === 'running' || selectedTask.status === 'pending')"
          type="danger" @click="killTask(selectedTask.id); showOutputDialog = false">
          终止任务
        </el-button>
        <el-button v-if="selectedTask?.output" @click="copyOutput(selectedTask.output)">复制输出</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { tasks as tasksApi, agents as agentsApi } from '../api/index'
import type { AgentInfo } from '../api/index'

interface Task {
  id: string
  agentId: string
  label?: string
  task: string
  status: 'pending' | 'running' | 'done' | 'error' | 'killed'
  output: string
  error?: string
  sessionId: string
  spawnedBy?: string
  model?: string
  createdAt: number
  startedAt?: number
  endedAt?: number
  duration?: string
}

const tasks = ref<Task[]>([])
const agents = ref<AgentInfo[]>([])
const loading = ref(false)
const killing = ref<string | null>(null)
const filterStatus = ref('')
const filterAgent = ref('')
const showSpawnDialog = ref(false)
const showOutputDialog = ref(false)
const selectedTask = ref<Task | null>(null)
const spawning = ref(false)
let refreshTimer: number | undefined

const spawnForm = ref({
  agentId: '',
  label: '',
  task: '',
  model: '',
})

const filteredTasks = computed(() => {
  return tasks.value.filter(t => {
    if (filterStatus.value && t.status !== filterStatus.value) return false
    if (filterAgent.value && t.agentId !== filterAgent.value) return false
    return true
  })
})

const runningCount = computed(() => tasks.value.filter(t => t.status === 'running' || t.status === 'pending').length)

function statusType(status: string) {
  switch (status) {
    case 'running': return 'success'
    case 'done': return 'primary'
    case 'error': return 'danger'
    case 'killed': return 'warning'
    default: return 'info'
  }
}

function statusLabel(status: string) {
  switch (status) {
    case 'running': return '运行中'
    case 'done': return '已完成'
    case 'error': return '出错'
    case 'killed': return '已终止'
    case 'pending': return '等待中'
    default: return status
  }
}

function agentName(id: string) {
  return agents.value.find(a => a.id === id)?.name || id
}

function formatTime(ms: number) {
  if (!ms) return '—'
  const d = new Date(ms)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function durationStr(task: Task) {
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
  const last = lines.slice(-4)
  return last.join('\n')
}

async function refresh() {
  loading.value = true
  try {
    const res = await tasksApi.list()
    tasks.value = res.data
    // Also refresh selected task if viewing
    if (selectedTask.value) {
      const updated = tasks.value.find(t => t.id === selectedTask.value!.id)
      if (updated) selectedTask.value = updated
    }
  } catch {
    // silently fail
  } finally {
    loading.value = false
  }
}

async function loadAgents() {
  try {
    const res = await agentsApi.list()
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
  } finally {
    killing.value = null
  }
}

function viewTask(task: Task) {
  selectedTask.value = task
  showOutputDialog.value = true
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
    })
    ElMessage.success('任务已派生，在后台执行中')
    showSpawnDialog.value = false
    spawnForm.value = { agentId: '', label: '', task: '', model: '' }
    await refresh()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '派生失败')
  } finally {
    spawning.value = false
  }
}

function copyOutput(output: string) {
  navigator.clipboard?.writeText(output)
  ElMessage.success('已复制')
}

onMounted(async () => {
  await loadAgents()
  await refresh()
  // Auto-refresh every 3s while running tasks exist
  refreshTimer = window.setInterval(() => {
    if (runningCount.value > 0) refresh()
  }, 3000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.task-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
  transition: box-shadow 0.2s;
}
.task-card:hover {
  box-shadow: 0 2px 12px rgba(0,0,0,0.08);
}
.task-running {
  border-left: 3px solid #10b981;
}
.task-done {
  border-left: 3px solid #3b82f6;
}
.task-error {
  border-left: 3px solid #ef4444;
}
.task-killed {
  border-left: 3px solid #f59e0b;
}
.task-pending {
  border-left: 3px solid #94a3b8;
}
</style>
