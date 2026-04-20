<template>
  <div class="chat-home">

    <!-- ══ 聊天顶部工具条（在 App.vue 内容区内） ══════════════════════════ -->
    <div class="chat-toolbar">
      <!-- Sidebar 展开/收起 -->
      <button class="sidebar-toggle-btn" @click="emit('toggle-sidebar')" title="展开/收起侧栏">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <rect x="3" y="3" width="18" height="18" rx="2"/>
          <line x1="9" y1="3" x2="9" y2="21"/>
        </svg>
      </button>

      <!-- 成员选择器 -->
      <el-select
        v-model="currentAgentId"
        size="small"
        class="agent-select"
        @change="onAgentChange"
      >
        <template #prefix>
          <div v-if="currentAgent" class="sel-avatar" :style="{ background: currentAgent.avatarColor || '#6366f1' }">
            {{ (currentAgent.name || '?')[0] }}
          </div>
        </template>
        <el-option v-for="ag in agents" :key="ag.id" :label="ag.name" :value="ag.id">
          <div style="display:flex;align-items:center;gap:8px">
            <div class="opt-avatar" :style="{ background: ag.avatarColor || '#6366f1' }">{{ (ag.name||'?')[0] }}</div>
            <span>{{ ag.name }}</span>
          </div>
        </el-option>
      </el-select>

      <!-- 模型选择器 -->
      <el-select
        v-model="currentModelId"
        size="small"
        class="model-select"
        placeholder="选择模型"
        @change="onModelChange"
      >
        <el-option v-for="m in allModels" :key="m.id" :label="m.name || m.model" :value="m.id" />
      </el-select>

      <!-- 历史会话 -->
      <el-select
        v-model="currentSessionId"
        size="small"
        class="session-select"
        placeholder="新对话"
        clearable
        @change="onSessionChange"
        popper-class="session-popper"
      >
        <el-option value="" label="＋ 新对话">
          <div class="sess-new-opt">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            新对话
          </div>
        </el-option>
        <el-option v-for="s in sessions" :key="s.key" :label="s.preview" :value="s.key">
          <div class="sess-opt">
            <div class="sess-opt-title">{{ s.preview }}</div>
            <div class="sess-opt-meta">
              <span class="sess-ch">
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
                {{ s.channel }}
              </span>
              <span class="sess-dot">·</span>
              <span class="sess-time">{{ fmtTime(s.lastAt) }}</span>
              <span class="sess-dot">·</span>
              <span class="sess-count">{{ s.messageCount }} 条</span>
            </div>
          </div>
        </el-option>
      </el-select>

      <div class="toolbar-flex" />

      <!-- 派遣区域 -->
      <Transition name="zone-fade">
        <div class="dispatch-zone" v-if="dispatched.length > 0">
          <span class="dz-label">派遣中</span>
          <TransitionGroup name="avatar-fly" tag="div" class="dz-avatars">
            <div v-for="d in dispatched" :key="d.taskId" class="dz-wrap">
              <div class="dz-avatar" :style="{ background: d.avatarColor }">
                {{ (d.agentName || '?')[0] }}
                <span class="dz-dot" :class="'dot-' + d.status" />
              </div>
              <div class="dz-bubble">
                <div class="dz-bname">{{ d.agentName }}</div>
                <div class="dz-bstatus">{{ statusText(d.status) }}</div>
                <div v-if="d.latestReport" class="dz-breport">{{ d.latestReport }}</div>
              </div>
            </div>
          </TransitionGroup>
        </div>
      </Transition>

      <!-- 新建对话 -->
      <el-button size="small" class="new-chat-btn" @click="newChat">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" style="margin-right:4px">
          <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
        </svg>
        新对话
      </el-button>
    </div>

    <!-- ══ AiChat 主体 ════════════════════════════════════════════════════ -->
    <div class="chat-body" v-if="currentAgentId">
      <AiChat
        ref="aiChatRef"
        :key="chatKey"
        :agent-id="currentAgentId"
        :session-id="currentSessionId || undefined"
        @dispatch="onDispatch"
        @task-handled="onTaskHandled"
        @session-change="onSessionCreated"
      />
    </div>

    <div class="chat-empty" v-else>
      <div class="empty-hint">
        <div class="empty-icon">🤖</div>
        <div>还没有 AI 成员，<a @click="$router.push('/agents/new')">点击创建</a></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElNotification } from 'element-plus'
import { agents as agentsApi, models as modelsApi, sessions as sessApi } from '../api'
import AiChat from '../components/AiChat.vue'
import type { AgentInfo, ModelEntry } from '../api'

const aiChatRef = ref<InstanceType<typeof AiChat>>()

const emit = defineEmits<{ (e: 'toggle-sidebar'): void }>()


const agents    = ref<AgentInfo[]>([])
const allModels = ref<ModelEntry[]>([])
const currentAgentId  = ref('')
const currentModelId  = ref('')
const currentSessionId = ref('')
const chatKey = ref(0)
const sessions = ref<SessionItem[]>([])

const currentAgent = computed(() => agents.value.find(a => a.id === currentAgentId.value))

// 历史会话
interface SessionItem {
  key: string
  preview: string
  createdAt: number
  lastAt: number
  messageCount: number
  channel: string
}

// 派遣
interface DispatchedTask {
  taskId: string; agentId: string; agentName: string
  avatarColor: string; status: 'running'|'done'|'error'; latestReport: string
  handled?: boolean  // true if LLM already processed result via agent_result tool
}
const dispatched = ref<DispatchedTask[]>([])

// ── 初始化 ────────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadAgents(), loadModels()])
})

async function loadAgents() {
  try {
    const res = await agentsApi.list()
    agents.value = res.data.filter((a: AgentInfo) => !a.system)
    const saved = localStorage.getItem('chat_home_agent')
    if (saved && agents.value.find(a => a.id === saved)) {
      currentAgentId.value = saved
    } else if (agents.value.length > 0) {
      currentAgentId.value = agents.value[0]?.id ?? ''
    }
    if (currentAgentId.value) {
      syncModel()
      await loadSessions(currentAgentId.value)
    }
  } catch {}
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    // 过滤掉 provider API Key 已测失败的模型（避免用户选了又报错）
    allModels.value = (res.data || []).filter((m: ModelEntry) => m.providerStatus !== 'error')
    syncModel()
  } catch {}
}

function syncModel() {
  const ag = currentAgent.value
  const saved = localStorage.getItem('chat_home_model')
  if (saved && allModels.value.find(m => m.id === saved)) {
    currentModelId.value = saved
  } else if (ag?.modelId) {
    currentModelId.value = ag.modelId
  } else {
    const def = allModels.value.find(m => m.isDefault) || allModels.value[0]
    if (def) currentModelId.value = def.id
  }
}

// 渠道来源 → 友好标签（用于会话下拉选项）
function channelLabel(source: string): string {
  const map: Record<string, string> = { feishu: '飞书', telegram: 'TG', web: 'Web', panel: '面板' }
  return map[source] || '面板'
}

function inferSessionSource(raw: string | undefined, id: string): string {
  const s = (raw || '').toLowerCase()
  if (s === 'feishu' || s === 'telegram' || s === 'web') return s
  if (id.startsWith('feishu-')) return 'feishu'
  if (id.startsWith('tg-')) return 'telegram'
  if (id.startsWith('web-')) return 'web'
  return 'panel'
}

async function loadSessions(agentId: string) {
  try {
    const res = await sessApi.list({ agentId, limit: 30 })
    sessions.value = (res.data.sessions || [])
      .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
      .map(s => {
        const src = inferSessionSource(s.source, s.id)
        const preview = (s.title && s.title.trim()) || (src === 'feishu' ? '飞书 · ' + s.id.slice(7, 15) : src === 'telegram' ? 'TG · ' + s.id.slice(3, 11) : '对话')
        return {
          key: s.id,
          preview: preview.slice(0, 40),
          createdAt: s.createdAt || 0,
          lastAt: s.lastAt || s.createdAt || 0,
          messageCount: s.messageCount || 0,
          channel: channelLabel(src),
        }
      })
  } catch {}
}

async function onAgentChange(id: string) {
  localStorage.setItem('chat_home_agent', id)
  currentSessionId.value = ''
  chatKey.value++
  syncModel()
  await loadSessions(id)
}

async function onModelChange(id: string) {
  localStorage.setItem('chat_home_model', id)
  if (currentAgentId.value) {
    try { await agentsApi.update(currentAgentId.value, { modelId: id }) } catch {}
  }
}

function onSessionChange(key: string) {
  currentSessionId.value = key
  chatKey.value++
}

function onSessionCreated(key: string) {
  currentSessionId.value = key
  if (!sessions.value.find(s => s.key === key)) {
    sessions.value.unshift({ key, preview: '新对话', createdAt: Date.now(), lastAt: Date.now(), messageCount: 0, channel: 'web' })
  }
}

function newChat() {
  currentSessionId.value = ''
  chatKey.value++
}

// ── 任务已被 LLM 内部处理（agent_result 调用成功）──────────────────────────
function onTaskHandled(taskId: string) {
  const task = dispatched.value.find(d => d.taskId === taskId)
  if (task) task.handled = true
}

// ── 派遣 ──────────────────────────────────────────────────────────────────
function onDispatch(agentId: string, agentName: string, avatarColor: string, taskId: string) {
  const agInfo = agents.value.find(a => a.id === agentId)
  const color = agInfo?.avatarColor || avatarColor || '#6366f1'
  if (dispatched.value.find(d => d.taskId === taskId)) return
  const task: DispatchedTask = {
    taskId, agentId, agentName: agInfo?.name || agentName, avatarColor: color, status: 'running', latestReport: '',
  }
  dispatched.value.push(task)
  pollTask(task)
}

function pollTask(task: DispatchedTask) {
  const token = localStorage.getItem('aipanel_token') || ''
  const base = (localStorage.getItem('aipanel_url') || '').replace(/\/$/, '')
  let tries = 0
  const tick = async () => {
    if (tries++ > 60) { task.status = 'error'; return }
    try {
      const r = await fetch(`${base}/api/tasks/${task.taskId}`, { headers: { Authorization: `Bearer ${token}` } })
      if (r.ok) {
        const d = await r.json()
        if (d.output) task.latestReport = (d.output as string).slice(-80)
        if (d.status === 'done') {
          task.status = 'done'
          const output = (d.output as string || '').trim()
          const label = d.label || task.agentName
          // 如果 LLM 已经通过 agent_result 主动处理了结果，跳过重复汇报
          if (!task.handled) {
            if (aiChatRef.value?.continueAfterSpawn) {
              aiChatRef.value.continueAfterSpawn(task.agentName, label, output)
            } else {
              ElNotification({ title: `✅ ${task.agentName} 完成了任务`, message: output.slice(0, 120) || '已完成', type: 'success', duration: 8000, position: 'bottom-right' })
            }
          }
          setTimeout(() => { dispatched.value = dispatched.value.filter(x => x.taskId !== task.taskId) }, 4000)
          return
        }
        if (d.status === 'error') {
          task.status = 'error'
          if (aiChatRef.value?.continueAfterSpawn) {
            aiChatRef.value.appendMessage?.({ role: 'system', text: `❌ ${task.agentName} 任务执行失败：${d.error || '未知错误'}` })
          } else {
            ElNotification({ title: `❌ ${task.agentName} 任务失败`, message: d.error || '执行出错', type: 'error', duration: 8000, position: 'bottom-right' })
          }
          setTimeout(() => { dispatched.value = dispatched.value.filter(x => x.taskId !== task.taskId) }, 6000)
          return
        }
      }
    } catch {}
    setTimeout(tick, 3000)
  }
  setTimeout(tick, 2000)
}

function statusText(s: string) {
  return ({ running: '执行中...', done: '已完成', error: '失败' } as any)[s] || s
}

function fmtTime(ts: number): string {
  if (!ts) return '-'
  const d = new Date(ts)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return Math.floor(diff / 60000) + ' 分钟前'
  if (diff < 86400000) return Math.floor(diff / 3600000) + ' 小时前'
  const m = d.getMonth() + 1, day = d.getDate()
  const h = String(d.getHours()).padStart(2, '0'), min = String(d.getMinutes()).padStart(2, '0')
  if (d.getFullYear() === now.getFullYear()) return `${m}/${day} ${h}:${min}`
  return `${d.getFullYear()}/${m}/${day}`
}
</script>

<style scoped>
.chat-home {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: #fafafa;
}

/* ── 顶部工具条 ───────────────────────────────────────────────────────── */
.chat-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: #fff;
  border-bottom: 1px solid #ececec;
  flex-shrink: 0;
  box-shadow: 0 1px 2px rgba(0,0,0,0.02);
}

.sidebar-toggle-btn {
  display: flex; align-items: center; justify-content: center;
  width: 30px; height: 30px; flex-shrink: 0;
  background: rgba(255,255,255,0.06); border: 1px solid rgba(255,255,255,0.1);
  border-radius: 6px; cursor: pointer; color: rgba(255,255,255,0.55);
  transition: background 0.15s, color 0.15s;
  padding: 0;
}
.sidebar-toggle-btn:hover { background: rgba(255,255,255,0.12); color: #fff; }

.agent-select   { width: 148px; flex-shrink: 0; }
.model-select   { width: 164px; flex-shrink: 0; }
.session-select { width: 184px; flex-shrink: 0; }

.sel-avatar, .opt-avatar {
  width: 20px; height: 20px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700; color: #fff;
  flex-shrink: 0;
}
.opt-avatar { width: 22px; height: 22px; }

:deep(.el-select .el-input__prefix-inner) { align-items: center; display: flex; }

.toolbar-flex { flex: 1; }

.new-chat-btn {
  background: rgba(99,102,241,0.15) !important;
  border-color: rgba(99,102,241,0.3) !important;
  color: #a5b4fc !important;
  font-size: 12px !important;
  display: flex; align-items: center;
}
.new-chat-btn:hover {
  background: rgba(99,102,241,0.25) !important;
  border-color: rgba(99,102,241,0.5) !important;
}

/* ── 派遣区域 ─────────────────────────────────────────────────────────── */
.dispatch-zone {
  display: flex; align-items: center; gap: 8px;
  padding: 0 10px;
  border-left: 1px solid rgba(255,255,255,0.08);
}
.dz-label {
  font-size: 11px; font-weight: 600;
  color: #fbbf24; letter-spacing: 0.5px; flex-shrink: 0;
}
.dz-avatars { display: flex; gap: 6px; }
.dz-wrap { position: relative; }
.dz-wrap:hover .dz-bubble { display: block; }

.dz-avatar {
  width: 28px; height: 28px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700; color: #fff; position: relative;
}
.dz-dot {
  position: absolute; bottom: -1px; right: -1px;
  width: 9px; height: 9px; border-radius: 50%;
  border: 1.5px solid #dcdfe6;
}
.dot-running { background: #f59e0b; animation: blink 1s ease-in-out infinite; }
.dot-done    { background: #22c55e; }
.dot-error   { background: #ef4444; }
@keyframes blink { 0%,100%{opacity:1} 50%{opacity:.25} }

.dz-bubble {
  display: none; position: absolute; top: 36px; left: 50%;
  transform: translateX(-50%);
  background: #1e2535; border: 1px solid rgba(255,255,255,0.1);
  border-radius: 10px; padding: 8px 12px; min-width: 150px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.5); z-index: 999; white-space: nowrap;
}
.dz-bname   { font-size: 13px; font-weight: 600; color: #e2e8f0; margin-bottom: 3px; }
.dz-bstatus { font-size: 12px; color: rgba(255,255,255,0.4); margin-bottom: 3px; }
.dz-breport { font-size: 12px; color: rgba(255,255,255,0.55); font-style: italic; white-space: normal; max-width: 200px; }

/* 飞入动画 */
.avatar-fly-enter-active { animation: flyIn 0.32s cubic-bezier(0.34,1.56,0.64,1); }
.avatar-fly-leave-active { animation: flyOut 0.25s ease-in forwards; }
@keyframes flyIn  { from{opacity:0;transform:scale(0.4) translateY(10px);}to{opacity:1;transform:scale(1);} }
@keyframes flyOut { from{opacity:1;transform:scale(1);}to{opacity:0;transform:scale(0.3);} }
.zone-fade-enter-active,.zone-fade-leave-active{transition:opacity 0.3s;}
.zone-fade-enter-from,.zone-fade-leave-to{opacity:0;}

/* ── 聊天主体 ─────────────────────────────────────────────────────────── */
.chat-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  position: relative;
}

.chat-empty {
  flex: 1; display: flex; align-items: center; justify-content: center;
  color: rgba(255,255,255,0.3);
}
.empty-hint { text-align: center; }
.empty-icon { font-size: 48px; margin-bottom: 12px; }
.empty-hint a { color: #818cf8; cursor: pointer; text-decoration: underline; }

/* ── 历史会话选项 ─────────────────────────────────────────────────────── */
:global(.session-popper .el-select-dropdown__item) {
  padding: 0 !important;
  height: auto !important;
  line-height: normal !important;
}
.sess-new-opt {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 12px; font-size: 13px; font-weight: 600; color: #818cf8;
}
.sess-opt {
  padding: 7px 12px;
}
.sess-opt-title {
  font-size: 13px; color: var(--el-text-color-primary);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  max-width: 220px; margin-bottom: 3px;
}
.sess-opt-meta {
  display: flex; align-items: center; gap: 4px;
  font-size: 11px; color: var(--el-text-color-secondary);
}
.sess-ch { display: flex; align-items: center; gap: 3px; }
.sess-dot { opacity: 0.4; }
.sess-count { opacity: 0.7; }
.sess-time { opacity: 0.7; }
</style>
