<template>
  <div class="dispatch-studio">

    <!-- ── 左：任务列表 ── -->
    <div class="ds-sidebar" :style="{ width: sideW + 'px' }">
      <div class="sidebar-top">
        <span class="sidebar-title">派遣任务</span>
        <div class="sidebar-acts">
          <el-button size="small" :loading="loading" circle @click="refresh">
            <el-icon><Refresh /></el-icon>
          </el-button>
          <el-button size="small" type="primary" circle @click="openNew">
            <el-icon><Plus /></el-icon>
          </el-button>
        </div>
      </div>

      <!-- 过滤条 -->
      <div class="ds-filter">
        <el-select v-model="filterStatus" placeholder="状态" clearable size="small" class="filter-sel">
          <el-option label="运行中" value="running" />
          <el-option label="已完成" value="done" />
          <el-option label="出错" value="error" />
          <el-option label="已终止" value="killed" />
          <el-option label="等待中" value="pending" />
        </el-select>
        <el-select v-model="filterType" placeholder="类型" clearable size="small" class="filter-sel">
          <el-option label="派遣" value="task" />
          <el-option label="汇报" value="report" />
          <el-option label="系统" value="system" />
        </el-select>
      </div>

      <!-- 任务列表 -->
      <div class="task-list">
        <div v-if="!loading && filteredTasks.length === 0" class="list-empty">暂无任务</div>
        <div
          v-for="t in filteredTasks" :key="t.id"
          :class="['task-item', { active: selected?.id === t.id }]"
          @click="selectTask(t)"
        >
          <!-- 头像 -->
          <div class="ti-avatar"
               :style="{ background: agentColor(t.agentId) }"
               :class="{ 'ti-avatar-running': t.status === 'running' }">
            {{ agentInitial(t.agentId) }}
          </div>
          <!-- 信息 -->
          <div class="ti-info">
            <div class="ti-name-row">
              <span class="ti-name">{{ agentName(t.agentId) }}</span>
              <span class="ti-tag" :class="'tag-' + t.status">{{ statusLabel(t.status) }}</span>
            </div>
            <div class="ti-label">{{ t.label || truncate(t.task, 36) }}</div>
            <div class="ti-meta">
              <span class="ti-type">{{ typeLabel(t.taskType) }}</span>
              <span class="ti-time">{{ relativeTime(t.createdAt) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 拖拽手柄 1 -->
    <div class="ds-handle" @mousedown="startResize($event, 'side')" :class="{ dragging: dragging === 'side' }">
      <div class="ds-handle-bar" />
    </div>

    <!-- ── 中：表单/详情 ── -->
    <div class="ds-editor">

      <!-- 空态 -->
      <div v-if="!selected && !creating" class="editor-empty">
        <el-icon size="48" color="#c0c4cc"><ChatLineRound /></el-icon>
        <p>从左侧选择任务查看详情<br>或新建派遣</p>
        <el-button type="primary" @click="openNew"><el-icon><Plus /></el-icon> 新建派遣</el-button>
      </div>

      <!-- 新建表单 -->
      <template v-else-if="creating">
        <div class="editor-toolbar">
          <div class="editor-breadcrumb">
            <el-icon style="color:#909399"><Plus /></el-icon>
            <span class="crumb-sep">新建</span>
            <span class="crumb-name">{{ spawnForm.taskType === 'task' ? '派遣任务' : '发起汇报' }}</span>
          </div>
          <div class="toolbar-acts">
            <el-button size="small" @click="creating = false; selected = null">取消</el-button>
            <el-button size="small" type="primary" :loading="spawning" @click="doSpawn">
              <el-icon><VideoPlay /></el-icon>
              {{ spawnForm.taskType === 'task' ? '派遣' : '汇报' }}
            </el-button>
          </div>
        </div>

        <div class="editor-form">
          <!-- 类型切换 -->
          <div class="form-group">
            <label class="form-label">类型</label>
            <el-radio-group v-model="spawnForm.taskType" size="small" @change="onTypeChange">
              <el-radio-button value="task">🚀 派遣任务</el-radio-button>
              <el-radio-button value="report">📋 发起汇报</el-radio-button>
            </el-radio-group>
          </div>

          <!-- 发起成员 -->
          <div class="form-group">
            <label class="form-label">发起成员</label>
            <el-select
              v-model="spawnForm.spawnedBy" placeholder="选择发起者（可选）"
              clearable size="small" class="form-full"
              @change="onSpawnedByChange"
            >
              <el-option v-for="a in agents" :key="a.id" :value="a.id">
                <div class="agent-opt">
                  <span class="agent-opt-dot" :style="{ background: a.avatarColor || '#6366f1' }"></span>
                  {{ a.name }}
                </div>
              </el-option>
            </el-select>
          </div>

          <!-- 目标成员 -->
          <div class="form-group">
            <label class="form-label">
              {{ spawnForm.taskType === 'task' ? '目标成员（被派遣）' : '目标成员（汇报对象）' }}
            </label>
            <el-select
              v-model="spawnForm.agentId" placeholder="选择目标 AI 成员" size="small" class="form-full"
              :loading="eligibleLoading"
            >
              <template v-if="spawnForm.spawnedBy">
                <el-option v-for="t in eligibleTargets" :key="t.agentId" :value="t.agentId">
                  <div class="agent-opt">
                    <span class="agent-opt-dot" :style="{ background: agentColor(t.agentId) }"></span>
                    {{ agentName(t.agentId) }}
                    <el-tag size="small" effect="plain" style="margin-left:6px;font-size:11px">{{ t.relation }}</el-tag>
                  </div>
                </el-option>
                <div v-if="eligibleTargets.length === 0 && !eligibleLoading" style="padding:8px 12px;font-size:12px;color:#94a3b8">
                  无可用目标（检查关系配置）
                </div>
              </template>
              <template v-else>
                <el-option v-for="a in agents" :key="a.id" :value="a.id">
                  <div class="agent-opt">
                    <span class="agent-opt-dot" :style="{ background: a.avatarColor || '#6366f1' }"></span>
                    {{ a.name }}
                  </div>
                </el-option>
              </template>
            </el-select>
          </div>

          <!-- 标签 -->
          <div class="form-group">
            <label class="form-label">标签（可选）</label>
            <el-input v-model="spawnForm.label" placeholder="简短描述，方便识别" size="small" />
          </div>

          <!-- 任务描述 -->
          <div class="form-group form-grow">
            <label class="form-label">
              {{ spawnForm.taskType === 'task' ? '任务描述' : '汇报内容' }}
            </label>
            <el-input
              v-model="spawnForm.task"
              type="textarea"
              :rows="8"
              :placeholder="spawnForm.taskType === 'task' ? '描述要派遣的具体任务…' : '描述要汇报的内容…'"
              resize="none"
              class="task-textarea"
            />
          </div>

          <!-- 模型（高级） -->
          <el-collapse class="adv-collapse">
            <el-collapse-item title="高级选项" name="adv">
              <div class="form-group">
                <label class="form-label">模型覆盖（可选）</label>
                <el-input v-model="spawnForm.model" placeholder="留空使用成员默认模型" size="small" />
              </div>
            </el-collapse-item>
          </el-collapse>
        </div>
      </template>

      <!-- 任务详情 -->
      <template v-else-if="selected">
        <div class="editor-toolbar">
          <div class="editor-breadcrumb">
            <div class="crumb-avatar" :style="{ background: agentColor(selected.agentId) }">
              {{ agentInitial(selected.agentId) }}
            </div>
            <span class="crumb-sep">{{ agentName(selected.agentId) }}</span>
            <span class="crumb-name">{{ selected.label || truncate(selected.task, 24) }}</span>
          </div>
          <div class="toolbar-acts">
            <el-button size="small" @click="openNew">
              <el-icon><Plus /></el-icon> 新建
            </el-button>
            <el-button
              v-if="selected.status === 'running' || selected.status === 'pending'"
              size="small" type="danger" plain :loading="killing"
              @click="killTask"
            >
              <el-icon><VideoPause /></el-icon> 终止
            </el-button>
            <el-popconfirm title="确认删除该任务记录？" @confirm="deleteTask">
              <template #reference>
                <el-button size="small" type="danger" plain><el-icon><Delete /></el-icon></el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>

        <div class="detail-body">
          <!-- 状态栏 -->
          <div class="detail-status-bar">
            <span class="ds-tag" :class="'tag-' + selected.status">{{ statusLabel(selected.status) }}</span>
            <span class="ds-badge">{{ typeLabel(selected.taskType) }}</span>
            <span v-if="selected.relation" class="ds-badge">{{ selected.relation }}</span>
            <span v-if="selected.model" class="ds-badge ds-badge-model">{{ selected.model }}</span>
          </div>

          <!-- 关系链 -->
          <div v-if="selected.spawnedBy" class="detail-chain">
            <span class="chain-from">{{ agentName(selected.spawnedBy) }}</span>
            <span class="chain-arrow">{{ selected.taskType === 'report' ? '↑' : '↓' }}</span>
            <span class="chain-to">{{ agentName(selected.agentId) }}</span>
          </div>

          <!-- 时间 -->
          <div class="detail-times">
            <div class="dt-item">
              <span class="dt-key">创建</span>
              <span class="dt-val">{{ formatTime(selected.createdAt) }}</span>
            </div>
            <div v-if="selected.startedAt" class="dt-item">
              <span class="dt-key">开始</span>
              <span class="dt-val">{{ formatTime(selected.startedAt) }}</span>
            </div>
            <div v-if="selected.endedAt" class="dt-item">
              <span class="dt-key">结束</span>
              <span class="dt-val">{{ formatTime(selected.endedAt) }}</span>
            </div>
            <div class="dt-item">
              <span class="dt-key">耗时</span>
              <span class="dt-val">{{ selected.duration || '—' }}</span>
            </div>
          </div>

          <!-- 任务描述 -->
          <div class="detail-section">
            <div class="section-label">任务描述</div>
            <div class="section-content task-desc">{{ selected.task }}</div>
          </div>

          <!-- 错误信息 -->
          <div v-if="selected.error" class="detail-section">
            <div class="section-label section-error">错误信息</div>
            <div class="section-content error-content">{{ selected.error }}</div>
          </div>

          <!-- 输出（折叠，当右侧聊天可用时退为次要） -->
          <div v-if="selected.output" class="detail-section detail-output">
            <div class="section-label">输出摘要</div>
            <div class="section-content output-content">{{ truncate(selected.output, 300) }}</div>
          </div>
        </div>
      </template>

    </div>

    <!-- 拖拽手柄 2 -->
    <div class="ds-handle" @mousedown="startResize($event, 'chat')" :class="{ dragging: dragging === 'chat' }">
      <div class="ds-handle-bar" />
    </div>

    <!-- ── 右：对话框 ── -->
    <div class="ds-chat" :style="{ width: chatW + 'px' }">
      <div class="chat-panel-head">
        <el-icon><ChatLineRound /></el-icon>
        <span>{{ selected ? agentName(selected.agentId) + ' 的会话' : '实时对话' }}</span>
        <span v-if="selected?.status === 'running'" class="chat-live-dot" />
      </div>
      <div class="chat-wrap">
        <div v-if="!selected" class="chat-empty">
          <el-icon size="36" color="#c0c4cc"><ChatLineRound /></el-icon>
          <p>选择任务后<br>在此查看实时对话</p>
        </div>
        <AiChat
          v-else
          :key="selected.sessionId"
          :agent-id="selected.agentId"
          :session-id="selected.sessionId"
          height="100%"
        />
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Plus, Refresh, ChatLineRound, VideoPlay, VideoPause, Delete
} from '@element-plus/icons-vue'
import { tasks as tasksApi, agents as agentsApi } from '../api/index'
import type { AgentInfo, TaskInfo, EligibleTarget } from '../api/index'
import AiChat from '../components/AiChat.vue'

// ── State ─────────────────────────────────────────────────────────────────
const allTasks    = ref<TaskInfo[]>([])
const agents      = ref<AgentInfo[]>([])
const loading     = ref(false)
const spawning    = ref(false)
const killing     = ref(false)
const selected    = ref<TaskInfo | null>(null)
const creating    = ref(false)

// Panel widths
const sideW    = ref(260)
const chatW    = ref(400)
const dragging = ref<'side' | 'chat' | ''>('')

// Filter
const filterStatus = ref('')
const filterType   = ref('')

// New spawn form
const spawnForm = ref({
  agentId: '',
  spawnedBy: '',
  taskType: 'task' as 'task' | 'report',
  task: '',
  label: '',
  model: '',
})

// Eligible targets
const eligibleTargets = ref<EligibleTarget[]>([])
const eligibleLoading = ref(false)

// Polling
let pollTimer: ReturnType<typeof setInterval> | null = null

// ── Computed ──────────────────────────────────────────────────────────────
const filteredTasks = computed(() => {
  let list = [...allTasks.value].sort((a, b) => b.createdAt - a.createdAt)
  if (filterStatus.value) list = list.filter(t => t.status === filterStatus.value)
  if (filterType.value)   list = list.filter(t => t.taskType === filterType.value)
  return list
})

// Agent lookup maps
const agentMap = computed(() => {
  const m: Record<string, AgentInfo> = {}
  agents.value.forEach(a => { m[a.id] = a })
  return m
})

// ── Lifecycle ─────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadAgents(), refresh()])
  // Poll running tasks every 5s
  pollTimer = setInterval(() => {
    const hasRunning = allTasks.value.some(t => t.status === 'running' || t.status === 'pending')
    if (hasRunning) refresh(true)
  }, 5000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  document.removeEventListener('mousemove', onMouseMove)
  document.removeEventListener('mouseup', onMouseUp)
})

// ── Data loading ──────────────────────────────────────────────────────────
async function loadAgents() {
  try {
    const res = await agentsApi.list()
    agents.value = res.data.filter(a => !a.system)
  } catch {}
}

async function refresh(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await tasksApi.list()
    allTasks.value = res.data
    // Refresh selected task if it's in the list
    if (selected.value) {
      const updated = res.data.find(t => t.id === selected.value!.id)
      if (updated) selected.value = updated
    }
  } catch {
    if (!silent) ElMessage.error('加载任务失败')
  } finally {
    loading.value = false
  }
}

// ── Selection ─────────────────────────────────────────────────────────────
function selectTask(t: TaskInfo) {
  selected.value = t
  creating.value = false
}

function openNew() {
  creating.value = true
  selected.value = null
  spawnForm.value = { agentId: '', spawnedBy: '', taskType: 'task', task: '', label: '', model: '' }
  eligibleTargets.value = []
}

// ── Spawn ─────────────────────────────────────────────────────────────────
async function onSpawnedByChange() {
  spawnForm.value.agentId = ''
  eligibleTargets.value = []
  if (!spawnForm.value.spawnedBy) return
  eligibleLoading.value = true
  try {
    const res = await tasksApi.eligibleTargets(spawnForm.value.spawnedBy, spawnForm.value.taskType)
    eligibleTargets.value = res.data
  } catch {} finally {
    eligibleLoading.value = false
  }
}

async function onTypeChange() {
  if (!spawnForm.value.spawnedBy) return
  await onSpawnedByChange()
}

async function doSpawn() {
  if (!spawnForm.value.agentId) {
    ElMessage.warning('请选择目标成员')
    return
  }
  if (!spawnForm.value.task.trim()) {
    ElMessage.warning(spawnForm.value.taskType === 'task' ? '请填写任务描述' : '请填写汇报内容')
    return
  }
  spawning.value = true
  try {
    const res = await tasksApi.spawn({
      agentId: spawnForm.value.agentId,
      task: spawnForm.value.task,
      label: spawnForm.value.label || undefined,
      model: spawnForm.value.model || undefined,
      spawnedBy: spawnForm.value.spawnedBy || undefined,
      taskType: spawnForm.value.taskType,
    })
    ElMessage.success('派遣成功')
    allTasks.value.unshift(res.data)
    creating.value = false
    selected.value = res.data
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '派遣失败')
  } finally {
    spawning.value = false
  }
}

// ── Kill / Delete ──────────────────────────────────────────────────────────
async function killTask() {
  if (!selected.value) return
  killing.value = true
  try {
    await tasksApi.kill(selected.value.id)
    ElMessage.success('任务已终止')
    await refresh()
  } catch {
    ElMessage.error('终止失败')
  } finally {
    killing.value = false
  }
}

async function deleteTask() {
  if (!selected.value) return
  try {
    await tasksApi.kill(selected.value.id)
  } catch {}
  allTasks.value = allTasks.value.filter(t => t.id !== selected.value!.id)
  selected.value = null
}

// ── Drag resize ────────────────────────────────────────────────────────────
let startX = 0
let startW = 0

function startResize(e: MouseEvent, target: 'side' | 'chat') {
  dragging.value = target
  startX = e.clientX
  startW = target === 'side' ? sideW.value : chatW.value
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
  e.preventDefault()
}

function onMouseMove(e: MouseEvent) {
  const d = e.clientX - startX
  if (dragging.value === 'side') {
    sideW.value = Math.max(200, Math.min(400, startW + d))
  } else if (dragging.value === 'chat') {
    chatW.value = Math.max(280, Math.min(600, startW - d))
  }
}

function onMouseUp() {
  dragging.value = ''
  document.removeEventListener('mousemove', onMouseMove)
  document.removeEventListener('mouseup', onMouseUp)
}

// ── Helpers ────────────────────────────────────────────────────────────────
function agentName(id: string)    { return agentMap.value[id]?.name || id }
function agentColor(id: string)   { return agentMap.value[id]?.avatarColor || '#6366f1' }
function agentInitial(id: string) { return (agentMap.value[id]?.name || id)[0]?.toUpperCase() || '?' }

function statusLabel(s: string) {
  return ({ pending: '等待中', running: '执行中', done: '已完成', error: '出错', killed: '已终止' } as Record<string, string>)[s] ?? s
}

function typeLabel(t?: string) {
  return ({ task: '派遣', report: '汇报', system: '系统' } as Record<string, string>)[t ?? ''] ?? '任务'
}

function truncate(s: string, n: number) {
  return s && s.length > n ? s.slice(0, n) + '…' : (s || '')
}

function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function relativeTime(ts: number) {
  const diff = Date.now() - ts
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return Math.floor(diff / 60000) + '分钟前'
  if (diff < 86400000) return Math.floor(diff / 3600000) + '小时前'
  return Math.floor(diff / 86400000) + '天前'
}
</script>

<style scoped>
/* ── 整体三栏容器 ─────────────────────────────────────────────────────── */
.dispatch-studio {
  display: flex;
  height: 100%;
  overflow: hidden;
  background: #f5f7fa;
  user-select: none;
}

/* ── 左侧边栏 ────────────────────────────────────────────────────────── */
.ds-sidebar {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  overflow: hidden;
}

.sidebar-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 8px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.sidebar-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}
.sidebar-acts {
  display: flex;
  gap: 4px;
}

/* 过滤条 */
.ds-filter {
  display: flex;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.filter-sel {
  flex: 1;
}

/* 任务列表 */
.task-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
}

.list-empty {
  text-align: center;
  padding: 32px 12px;
  font-size: 13px;
  color: #94a3b8;
}

.task-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  cursor: pointer;
  transition: background 0.15s;
  border-left: 3px solid transparent;
}
.task-item:hover {
  background: #f5f7fa;
}
.task-item.active {
  background: #ecf5ff;
  border-left-color: #409eff;
}

/* 头像 */
.ti-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  color: #fff;
  font-weight: 700;
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.ti-avatar-running {
  animation: avatar-breathing 2s ease-in-out infinite;
}

@keyframes avatar-breathing {
  0%, 100% { box-shadow: 0 0 0 0 rgba(64,158,255,0.5); }
  50% { box-shadow: 0 0 0 4px rgba(64,158,255,0); }
}

/* 任务信息 */
.ti-info {
  flex: 1;
  min-width: 0;
}
.ti-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}
.ti-name {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ti-label {
  font-size: 12px;
  color: #606266;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 3px;
}
.ti-meta {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: #94a3b8;
}

/* 状态标签 */
.ti-tag, .ds-tag {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  font-weight: 500;
  white-space: nowrap;
  flex-shrink: 0;
}
.tag-pending { background: rgba(144,147,153,0.12); color: #909399; }
.tag-running { background: rgba(64,158,255,0.12);  color: #409eff; }
.tag-done    { background: rgba(103,194,58,0.12);  color: #67c23a; }
.tag-error   { background: rgba(245,108,108,0.12); color: #f56c6c; }
.tag-killed  { background: rgba(230,162,60,0.12);  color: #e6a23c; }

/* ── 拖拽手柄 ────────────────────────────────────────────────────────── */
.ds-handle {
  width: 4px;
  background: #e4e7ed;
  cursor: col-resize;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s;
  z-index: 10;
}
.ds-handle:hover, .ds-handle.dragging { background: #409eff; }
.ds-handle-bar {
  width: 2px;
  height: 28px;
  background: rgba(255,255,255,0.6);
  border-radius: 2px;
}

/* ── 中：编辑区 ──────────────────────────────────────────────────────── */
.ds-editor {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
  border-right: 1px solid #e4e7ed;
}

/* 空态 */
.editor-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
}
.editor-empty p {
  margin: 0;
  line-height: 1.7;
}

/* 工具栏 */
.editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.editor-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  overflow: hidden;
}
.crumb-avatar {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  color: #fff;
  font-weight: 700;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.crumb-sep  { color: #909399; }
.crumb-name { font-weight: 600; color: #303133; }
.toolbar-acts { display: flex; gap: 6px; }

/* 新建表单 */
.editor-form {
  flex: 1;
  overflow-y: auto;
  padding: 20px 20px 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form-group.form-grow {
  flex: 1;
}
.form-label {
  font-size: 12px;
  font-weight: 600;
  color: #606266;
}
.form-full {
  width: 100%;
}
.task-textarea :deep(.el-textarea__inner) {
  font-size: 13px;
  line-height: 1.7;
  font-family: inherit;
  resize: none;
  flex: 1;
}

/* Agent 选项 */
.agent-opt {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}
.agent-opt-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* 高级折叠 */
.adv-collapse {
  border: none;
  --el-collapse-header-bg-color: transparent;
}
.adv-collapse :deep(.el-collapse-item__header) {
  font-size: 12px;
  color: #909399;
  padding: 0;
  border: none;
}
.adv-collapse :deep(.el-collapse-item__content) {
  padding-bottom: 0;
}
.adv-collapse :deep(.el-collapse-item__wrap) {
  border: none;
}

/* 任务详情 */
.detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-status-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.ds-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: #f0f2f5;
  color: #606266;
  font-weight: 500;
}
.ds-badge-model {
  font-family: monospace;
  background: #e8f4fd;
  color: #409eff;
}

.detail-chain {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #606266;
  padding: 8px 12px;
  background: #f5f7fa;
  border-radius: 8px;
}
.chain-from { font-weight: 600; color: #409eff; }
.chain-arrow { color: #94a3b8; font-size: 16px; }
.chain-to   { font-weight: 600; color: #67c23a; }

.detail-times {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.dt-item { display: flex; gap: 4px; font-size: 12px; }
.dt-key  { color: #94a3b8; }
.dt-val  { color: #606266; font-weight: 500; }

.detail-section { display: flex; flex-direction: column; gap: 6px; }
.section-label  {
  font-size: 11px;
  font-weight: 600;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.section-error { color: #f56c6c; }
.section-content {
  font-size: 13px;
  color: #303133;
  line-height: 1.7;
  background: #f5f7fa;
  border-radius: 8px;
  padding: 10px 12px;
  white-space: pre-wrap;
}
.error-content  { background: #fff5f5; color: #f56c6c; }
.output-content { max-height: 160px; overflow-y: auto; }
.task-desc { font-family: inherit; }

/* ── 右：对话框 ──────────────────────────────────────────────────────── */
.ds-chat {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}

.chat-panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 11px 14px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}

.chat-live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #67c23a;
  animation: live-pulse 1.4s ease-in-out infinite;
  margin-left: 2px;
}
@keyframes live-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.chat-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.chat-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #94a3b8;
  font-size: 13px;
  text-align: center;
}
.chat-empty p { margin: 0; line-height: 1.7; }
</style>
