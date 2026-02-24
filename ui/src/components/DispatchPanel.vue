<template>
  <!-- 整体面板：有活跃子成员时从顶部滑入 -->
  <Transition name="panel-slide">
    <div v-if="hasAny" class="dispatch-panel">

      <!-- 标题栏 -->
      <div class="dp-header">
        <span class="dp-pulse" />
        <span class="dp-title">派遣任务进行中</span>
        <span class="dp-count">{{ activeList.length }} 名成员执行中</span>
        <button class="dp-collapse-btn" @click="collapsed = !collapsed">
          {{ collapsed ? '展开 ∨' : '收起 ∧' }}
        </button>
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
                   :style="{ background: d.avatarColor || '#6366f1' }">
                {{ (d.agentName || '?')[0] }}
                <span v-if="d.status === 'done'" class="dp-done-badge">✓</span>
                <span v-if="d.status === 'error'" class="dp-error-badge">!</span>
              </div>

              <!-- 信息 -->
              <div class="dp-info">
                <div class="dp-name-row">
                  <span class="dp-name">{{ d.agentName }}</span>
                  <span class="dp-tag" :class="'tag-' + d.status">
                    {{ statusLabel(d.status) }}
                  </span>
                  <div v-if="d.progress > 0 || d.status === 'running'" class="dp-progress-wrap">
                    <div class="dp-progress-bar" :style="{ width: d.progress + '%' }" />
                    <span v-if="d.progress > 0" class="dp-progress-num">{{ d.progress }}%</span>
                  </div>
                </div>

                <!-- 最新汇报 -->
                <div v-if="d.latestReport" class="dp-report"
                     :class="{ 'dp-report-new': d.reportNew }">
                  "{{ truncate(d.latestReport, 60) }}"
                  <button v-if="d.reports.length > 1"
                          class="dp-view-all"
                          @click="viewReports(d)">
                    全部 ({{ d.reports.length }})
                  </button>
                </div>
              </div>

            </div>
          </TransitionGroup>
        </div>
      </Transition>

    </div>
  </Transition>

  <!-- 汇报详情弹窗 -->
  <Transition name="dialog-fade">
    <div v-if="reportDialogVisible" class="dp-dialog-mask" @click.self="reportDialogVisible = false">
      <div class="dp-dialog">
        <div class="dp-dialog-header">
          <span>{{ reportDialogAgent }} 的汇报记录</span>
          <button class="dp-dialog-close" @click="reportDialogVisible = false">×</button>
        </div>
        <div class="dp-dialog-body">
          <div v-for="r in reportDialogRecords" :key="r.timestamp" class="dp-timeline-item">
            <div class="dp-tl-dot" :class="'tl-' + r.status" />
            <div class="dp-tl-content">
              <div class="dp-tl-text">{{ r.content }}</div>
              <div class="dp-tl-meta">
                <span class="dp-tl-time">{{ formatTime(r.timestamp) }}</span>
                <span v-if="r.progress > 0" class="dp-tl-progress">{{ r.progress }}%</span>
                <span class="dp-tl-status dp-tag" :class="'tag-' + r.status">{{ statusLabel(r.status) }}</span>
              </div>
            </div>
          </div>
          <div v-if="!reportDialogRecords.length" class="dp-dialog-empty">暂无汇报记录</div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

interface ReportEntry {
  content: string
  status: string
  progress: number
  timestamp: number
}

interface DispatcherState {
  subagentSessionId: string
  agentId: string
  agentName: string
  avatarColor: string
  status: 'running' | 'blocked' | 'done' | 'error'
  progress: number
  reports: ReportEntry[]
  latestReport: string
  reportNew: boolean
  spawnedAt: number
  doneAt?: number
}

const props = defineProps<{ sessionId: string }>()

const dispatchers = ref<Map<string, DispatcherState>>(new Map())
const collapsed = ref(false)
const reportDialogVisible = ref(false)
const reportDialogAgent = ref('')
const reportDialogRecords = ref<ReportEntry[]>([])

const hasAny = computed(() => dispatchers.value.size > 0)
const activeList = computed(() =>
  [...dispatchers.value.values()].filter(d => d.status !== 'done' && d.status !== 'error')
)
const sortedDispatchers = computed(() =>
  [...dispatchers.value.values()].sort((a, b) => a.spawnedAt - b.spawnedAt)
)

// ── Event handler (called by AiChat.vue) ─────────────────────────────────────
function handleEvent(raw: any) {
  const data = typeof raw === 'string' ? JSON.parse(raw) : raw
  if (!data) return

  const id: string = data.subagentSessionId
  if (!id) return

  if (data.type === 'subagent_spawn' || data.type === 'spawn') {
    const entry: DispatcherState = {
      subagentSessionId: id,
      agentId: data.agentId || '',
      agentName: data.agentName || data.agentId || '未知成员',
      avatarColor: data.avatarColor || '#6366f1',
      status: 'running',
      progress: 0,
      reports: [],
      latestReport: '',
      reportNew: false,
      spawnedAt: data.timestamp || Date.now(),
    }
    dispatchers.value = new Map(dispatchers.value.set(id, entry))

  } else if (data.type === 'subagent_report' || data.type === 'report') {
    const d = dispatchers.value.get(id)
    if (d) {
      const rpt: ReportEntry = {
        content: data.content || '',
        status: data.status || 'running',
        progress: data.progress || 0,
        timestamp: data.timestamp || Date.now(),
      }
      d.reports.push(rpt)
      d.latestReport = data.content || ''
      if (data.progress) d.progress = data.progress
      if (data.status === 'done') d.status = 'done'
      else if (data.status === 'blocked') d.status = 'blocked'
      else d.status = 'running'
      d.reportNew = true
      setTimeout(() => { if (d) d.reportNew = false }, 900)
      dispatchers.value = new Map(dispatchers.value)
    }

  } else if (data.type === 'subagent_done' || data.type === 'done') {
    const d = dispatchers.value.get(id)
    if (d) {
      d.status = 'done'
      d.doneAt = data.timestamp || Date.now()
      dispatchers.value = new Map(dispatchers.value)
      // Auto-remove after 3s
      setTimeout(() => {
        dispatchers.value.delete(id)
        dispatchers.value = new Map(dispatchers.value)
      }, 3000)
    }

  } else if (data.type === 'subagent_error' || data.type === 'error') {
    const d = dispatchers.value.get(id)
    if (d) {
      d.status = 'error'
      dispatchers.value = new Map(dispatchers.value)
    }
  }
}

defineExpose({ handleEvent })

// ── Helpers ──────────────────────────────────────────────────────────────────
function statusLabel(s: string): string {
  return ({ running: '执行中', blocked: '遇到阻碍', done: '已完成', error: '出错' } as Record<string, string>)[s] ?? s
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + '…' : s
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString('zh-CN')
}

function viewReports(d: DispatcherState) {
  reportDialogAgent.value = d.agentName
  reportDialogRecords.value = [...d.reports].reverse()
  reportDialogVisible.value = true
}
</script>

<style scoped>
/* ── Panel container ─────────────────────────────────────────────────────── */
.dispatch-panel {
  background: var(--el-bg-color-overlay, #fff);
  border-bottom: 1px solid var(--el-border-color, #e4e7ed);
  flex-shrink: 0;
}

/* ── Header ──────────────────────────────────────────────────────────────── */
.dp-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 16px;
  font-size: 13px;
  font-weight: 500;
}

.dp-pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #409eff;
  animation: dp-pulse-anim 1.4s ease-in-out infinite;
  flex-shrink: 0;
}

.dp-title {
  font-weight: 600;
  color: var(--el-text-color-primary, #303133);
}

.dp-count {
  color: var(--el-text-color-secondary, #909399);
  font-size: 12px;
  font-weight: 400;
}

.dp-collapse-btn {
  margin-left: auto;
  background: none;
  border: none;
  cursor: pointer;
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
  padding: 2px 6px;
  border-radius: 4px;
  transition: background 0.2s;
}
.dp-collapse-btn:hover {
  background: var(--el-fill-color-light, #f5f7fa);
}

/* ── Body ────────────────────────────────────────────────────────────────── */
.dp-body {
  padding: 4px 16px 10px;
}

.dp-members {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* ── Member row ──────────────────────────────────────────────────────────── */
.dp-member {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.dp-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 14px;
  flex-shrink: 0;
  position: relative;
  user-select: none;
}

.status-running {
  animation: dp-breathing 2s ease-in-out infinite;
}

.dp-done-badge,
.dp-error-badge {
  position: absolute;
  bottom: -2px;
  right: -2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  color: #fff;
  font-size: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
}
.dp-done-badge  { background: #67c23a; }
.dp-error-badge { background: #f56c6c; }

/* ── Info ────────────────────────────────────────────────────────────────── */
.dp-info {
  flex: 1;
  min-width: 0;
}

.dp-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.dp-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary, #303133);
}

/* Status tags */
.dp-tag {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  font-weight: 500;
  white-space: nowrap;
}
.tag-running { background: rgba(64,158,255,0.12); color: #409eff; }
.tag-blocked { background: rgba(230,162,60,0.12); color: #e6a23c; }
.tag-done    { background: rgba(103,194,58,0.12); color: #67c23a; }
.tag-error   { background: rgba(245,108,108,0.12); color: #f56c6c; }

/* Progress bar */
.dp-progress-wrap {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 60px;
  max-width: 120px;
}
.dp-progress-bar {
  height: 4px;
  background: #409eff;
  border-radius: 2px;
  transition: width 0.5s ease;
  min-width: 4px;
}
.dp-progress-num {
  font-size: 11px;
  color: var(--el-text-color-secondary, #909399);
  white-space: nowrap;
}

/* Latest report */
.dp-report {
  margin-top: 3px;
  font-size: 12px;
  color: var(--el-text-color-secondary, #909399);
  font-style: italic;
  border-left: 2px solid var(--el-border-color, #e4e7ed);
  padding-left: 6px;
  transition: background 0.4s;
  border-radius: 0 3px 3px 0;
  line-height: 1.5;
}
.dp-report-new {
  background: rgba(64,158,255,0.07);
}

.dp-view-all {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 11px;
  color: #409eff;
  padding: 0 2px;
  text-decoration: underline;
  text-underline-offset: 2px;
}

/* ── Animations ──────────────────────────────────────────────────────────── */
@keyframes dp-pulse-anim {
  0%,100% { opacity: 1; }
  50% { opacity: 0.35; }
}

@keyframes dp-breathing {
  0%,100% { box-shadow: 0 0 0 0 rgba(64,158,255,0.5); }
  50% { box-shadow: 0 0 0 5px rgba(64,158,255,0); }
}

/* Panel slide in/out */
.panel-slide-enter-active,
.panel-slide-leave-active { transition: all 0.28s ease; }
.panel-slide-enter-from,
.panel-slide-leave-to { opacity: 0; transform: translateY(-100%); }

/* Expand / collapse body */
.dp-expand-enter-active,
.dp-expand-leave-active { transition: all 0.22s ease; overflow: hidden; }
.dp-expand-enter-from,
.dp-expand-leave-to { max-height: 0; opacity: 0; }
.dp-expand-enter-to,
.dp-expand-leave-from { max-height: 600px; opacity: 1; }

/* Member fly-in */
.member-fly-enter-active {
  transition: all 0.38s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.member-fly-enter-from {
  opacity: 0;
  transform: translateX(28px);
}
.member-fly-leave-active { transition: all 0.25s ease; }
.member-fly-leave-to {
  opacity: 0;
  transform: translateX(28px);
}

/* ── Dialog ──────────────────────────────────────────────────────────────── */
.dp-dialog-mask {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}
.dp-dialog {
  background: var(--el-bg-color-overlay, #fff);
  border-radius: 12px;
  width: 440px;
  max-width: 90vw;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 12px 40px rgba(0,0,0,0.18);
}
.dp-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  font-weight: 600;
  font-size: 14px;
  border-bottom: 1px solid var(--el-border-color, #e4e7ed);
}
.dp-dialog-close {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 18px;
  color: var(--el-text-color-secondary, #909399);
  line-height: 1;
  padding: 0;
}
.dp-dialog-body {
  overflow-y: auto;
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.dp-dialog-empty {
  color: var(--el-text-color-secondary, #909399);
  text-align: center;
  padding: 24px 0;
  font-size: 13px;
}

/* Timeline items */
.dp-timeline-item {
  display: flex;
  gap: 10px;
}
.dp-tl-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-top: 4px;
}
.tl-running { background: #409eff; }
.tl-blocked { background: #e6a23c; }
.tl-done    { background: #67c23a; }
.tl-error   { background: #f56c6c; }

.dp-tl-content { flex: 1; min-width: 0; }
.dp-tl-text {
  font-size: 13px;
  color: var(--el-text-color-primary, #303133);
  line-height: 1.5;
}
.dp-tl-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 3px;
}
.dp-tl-time {
  font-size: 11px;
  color: var(--el-text-color-secondary, #909399);
}
.dp-tl-progress {
  font-size: 11px;
  color: #409eff;
  font-weight: 600;
}

/* Dialog fade */
.dialog-fade-enter-active, .dialog-fade-leave-active { transition: opacity 0.2s; }
.dialog-fade-enter-from, .dialog-fade-leave-to { opacity: 0; }
</style>
