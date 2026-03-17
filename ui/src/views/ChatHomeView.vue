<template>
  <div class="chat-home">

    <!-- ══ 顶部工具栏 ══════════════════════════════════════════════════════ -->
    <header class="topbar">
      <!-- Logo -->
      <div class="topbar-logo" @click="$router.push('/dashboard')">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <path d="M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z" fill="url(#tg2)"/>
          <defs>
            <linearGradient id="tg2" x1="2.5" y1="2" x2="21.5" y2="22" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stop-color="#818cf8"/>
              <stop offset="100%" stop-color="#38bdf8"/>
            </linearGradient>
          </defs>
          <text x="12" y="16" text-anchor="middle" fill="white" font-size="8" font-weight="900" font-family="sans-serif">Z</text>
        </svg>
        <span class="topbar-brand">引巢</span>
        <span class="topbar-ver">v{{ version }}</span>
      </div>

      <div class="topbar-sep" />

      <!-- 成员选择器（下拉） -->
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
        <el-option
          v-for="ag in agents"
          :key="ag.id"
          :label="ag.name"
          :value="ag.id"
        >
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
        <el-option
          v-for="m in availableModels"
          :key="m.id"
          :label="m.name || m.model"
          :value="m.id"
        />
      </el-select>

      <!-- 历史会话 -->
      <el-select
        v-model="currentSessionId"
        size="small"
        class="session-select"
        placeholder="新对话"
        clearable
        @change="onSessionChange"
      >
        <el-option value="" label="＋ 新对话" />
        <el-option
          v-for="s in sessions"
          :key="s.key"
          :label="s.preview"
          :value="s.key"
        />
      </el-select>

      <!-- 派遣区域 -->
      <Transition name="zone-fade">
        <div class="dispatch-zone" v-if="dispatched.length > 0">
          <span class="dz-label">派遣中</span>
          <TransitionGroup name="avatar-fly" tag="div" class="dz-avatars">
            <div
              v-for="d in dispatched"
              :key="d.taskId"
              class="dz-wrap"
            >
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

      <div class="topbar-flex" />

      <!-- 汉堡菜单 -->
      <el-dropdown trigger="click" @command="onNav">
        <button class="menu-btn">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <line x1="3" y1="6" x2="21" y2="6"/>
            <line x1="3" y1="12" x2="21" y2="12"/>
            <line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="/dashboard">📊 仪表盘</el-dropdown-item>
            <el-dropdown-item command="/agents">🤖 成员管理</el-dropdown-item>
            <el-dropdown-item command="/chats">💬 对话管理</el-dropdown-item>
            <el-dropdown-item command="/tasks">🚀 后台任务</el-dropdown-item>
            <el-dropdown-item command="/team">🌐 团队图谱</el-dropdown-item>
            <el-dropdown-item command="/goals">🎯 团队规划</el-dropdown-item>
            <el-dropdown-item command="/projects">📁 项目</el-dropdown-item>
            <el-dropdown-item command="/skills">🧩 技能库</el-dropdown-item>
            <el-dropdown-item command="/cron">⏰ 定时任务</el-dropdown-item>
            <el-dropdown-item command="/config/models">🔑 模型配置</el-dropdown-item>
            <el-dropdown-item command="/config/channels">📡 消息渠道</el-dropdown-item>
            <el-dropdown-item command="/config/tools">🛠 工具权限</el-dropdown-item>
            <el-dropdown-item command="/usage">📈 用量统计</el-dropdown-item>
            <el-dropdown-item command="/logs">📄 日志</el-dropdown-item>
            <el-dropdown-item command="/settings">⚙️ 设置</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </header>

    <!-- ══ 聊天区域 ═══════════════════════════════════════════════════════ -->
    <main class="chat-main" v-if="currentAgentId">
      <AiChat
        :key="chatKey"
        :agent-id="currentAgentId"
        :session-id="currentSessionId || undefined"
        @dispatch="onDispatch"
        @session-change="onSessionCreated"
      />
    </main>

    <main class="chat-empty" v-else>
      <div class="empty-hint">
        <div class="empty-icon">🤖</div>
        <div>还没有 AI 成员，<a @click="$router.push('/agents/new')">点击创建</a></div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { agents as agentsApi, models as modelsApi } from '../api'
import AiChat from '../components/AiChat.vue'
import type { AgentInfo, ModelEntry } from '../api'

const router = useRouter()
const version = __APP_VERSION__ as string || ''

// ── 状态 ──────────────────────────────────────────────────────────────────
const agents = ref<AgentInfo[]>([])
const allModels = ref<ModelEntry[]>([])
const currentAgentId = ref('')
const currentModelId = ref('')
const currentSessionId = ref('')
const chatKey = ref(0)
const sessions = ref<{ key: string; preview: string }[]>([])

const currentAgent = computed(() => agents.value.find(a => a.id === currentAgentId.value))
const availableModels = computed(() => allModels.value)

// 派遣任务
interface DispatchedTask {
  taskId: string
  agentId: string
  agentName: string
  avatarColor: string
  status: 'running' | 'done' | 'error'
  latestReport: string
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
      currentAgentId.value = agents.value[0]?.id ?? ""
    }
    if (currentAgentId.value) {
      syncModelFromAgent()
      await loadSessions(currentAgentId.value)
    }
  } catch (e) { console.error(e) }
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    allModels.value = res.data
  } catch {}
}

function syncModelFromAgent() {
  const ag = currentAgent.value
  if (!ag) return
  const saved = localStorage.getItem('chat_home_model')
  if (saved && allModels.value.find(m => m.id === saved)) {
    currentModelId.value = saved
  } else if (ag.modelId) {
    currentModelId.value = ag.modelId
  } else {
    const def = allModels.value.find(m => m.isDefault) || allModels.value[0]
    if (def) currentModelId.value = def.id
  }
}

async function loadSessions(agentId: string) {
  try {
    const { sessions: sessApi } = await import('../api')
    const res = await sessApi.list({ agentId, limit: 30 })
    sessions.value = (res.data.sessions || []).map((s: any) => ({
      key: s.id || s.key,
      preview: (s.lastMessage || s.title || s.id || '').slice(0, 36) || '对话',
    }))
  } catch {}
}

// ── 切换成员 ──────────────────────────────────────────────────────────────
async function onAgentChange(id: string) {
  localStorage.setItem('chat_home_agent', id)
  currentSessionId.value = ''
  chatKey.value++
  syncModelFromAgent()
  await loadSessions(id)
}

// ── 切换模型 ──────────────────────────────────────────────────────────────
async function onModelChange(id: string) {
  localStorage.setItem('chat_home_model', id)
  if (currentAgentId.value) {
    try { await agentsApi.update(currentAgentId.value, { modelId: id }) } catch {}
  }
}

// ── 切换会话 ──────────────────────────────────────────────────────────────
function onSessionChange(key: string) {
  currentSessionId.value = key
  chatKey.value++
}

function onSessionCreated(key: string) {
  currentSessionId.value = key
  if (!sessions.value.find(s => s.key === key)) {
    sessions.value.unshift({ key, preview: '新对话' })
  }
}

// ── 派遣动画 ──────────────────────────────────────────────────────────────
function onDispatch(agentId: string, agentName: string, avatarColor: string, taskId: string) {
  const agInfo = agents.value.find(a => a.id === agentId)
  const color = agInfo?.avatarColor || avatarColor || '#6366f1'
  if (dispatched.value.find(d => d.taskId === taskId)) return

  const task: DispatchedTask = {
    taskId, agentId,
    agentName: agInfo?.name || agentName,
    avatarColor: color,
    status: 'running',
    latestReport: '',
  }
  dispatched.value.push(task)
  pollTaskStatus(task)
}

function pollTaskStatus(task: DispatchedTask) {
  const token = localStorage.getItem('aipanel_token') || ''
  const base = (localStorage.getItem('aipanel_url') || '').replace(/\/$/, '')
  let tries = 0

  const tick = async () => {
    if (tries++ > 60) { task.status = 'error'; return }
    try {
      const r = await fetch(`${base}/api/agents/${task.agentId}/tasks/${task.taskId}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (r.ok) {
        const d = await r.json()
        if (d.output) task.latestReport = (d.output as string).slice(-80)
        if (d.status === 'done') {
          task.status = 'done'
          setTimeout(() => { dispatched.value = dispatched.value.filter(x => x.taskId !== task.taskId) }, 4000)
          return
        }
        if (d.status === 'error') {
          task.status = 'error'
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

function onNav(path: string) { router.push(path) }
</script>

<!-- 让 vite 注入版本号 -->
<script lang="ts">
declare const __APP_VERSION__: string
</script>

<style scoped>
/* ── 整体 ─────────────────────────────────────────────────────────────── */
.chat-home {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #111827;
  overflow: hidden;
  color: #e2e8f0;
}

/* ── 顶部栏 ──────────────────────────────────────────────────────────── */
.topbar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 52px;
  padding: 0 16px;
  background: #1a1f2e;
  border-bottom: 1px solid rgba(255,255,255,0.07);
  flex-shrink: 0;
}

/* Logo */
.topbar-logo {
  display: flex;
  align-items: center;
  gap: 7px;
  cursor: pointer;
  flex-shrink: 0;
  user-select: none;
}
.topbar-brand {
  font-size: 14px;
  font-weight: 700;
  color: #c7d2fe;
  letter-spacing: 0.3px;
}
.topbar-ver {
  font-size: 11px;
  color: rgba(255,255,255,0.25);
  background: rgba(255,255,255,0.06);
  padding: 1px 6px;
  border-radius: 10px;
}

.topbar-sep {
  width: 1px;
  height: 24px;
  background: rgba(255,255,255,0.08);
  flex-shrink: 0;
}

/* 成员下拉 */
.agent-select { width: 148px; flex-shrink: 0; }
.sel-avatar {
  width: 20px; height: 20px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700; color: #fff;
}
.opt-avatar {
  width: 22px; height: 22px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700; color: #fff;
  flex-shrink: 0;
}

/* 模型 / 会话 */
.model-select   { width: 160px; flex-shrink: 0; }
.session-select { width: 180px; flex-shrink: 0; }

/* Element Plus select 深色覆盖 */
:deep(.el-select .el-input__wrapper) {
  background: rgba(255,255,255,0.05) !important;
  box-shadow: 0 0 0 1px rgba(255,255,255,0.1) !important;
  border-radius: 8px !important;
}
:deep(.el-select .el-input__wrapper:hover) {
  box-shadow: 0 0 0 1px rgba(129,140,248,0.4) !important;
}
:deep(.el-select .el-input__inner) {
  color: #c8cfe8 !important;
  font-size: 13px !important;
}
:deep(.el-select__caret) { color: rgba(255,255,255,0.3) !important; }
:deep(.el-select .el-input__prefix-inner) { align-items: center; }

/* ── 派遣区域 ─────────────────────────────────────────────────────────── */
.dispatch-zone {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-left: 1px solid rgba(255,255,255,0.07);
}
.dz-label {
  font-size: 11px;
  font-weight: 600;
  color: #fbbf24;
  letter-spacing: 0.5px;
  flex-shrink: 0;
}
.dz-avatars {
  display: flex;
  gap: 6px;
}

.dz-wrap {
  position: relative;
}
.dz-wrap:hover .dz-bubble { display: block; }

.dz-avatar {
  width: 30px; height: 30px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 12px; font-weight: 700; color: #fff;
  position: relative;
  border: 2px solid transparent;
  transition: border-color 0.3s;
}

.dz-dot {
  position: absolute;
  bottom: -1px; right: -1px;
  width: 9px; height: 9px;
  border-radius: 50%;
  border: 1.5px solid #1a1f2e;
}
.dot-running { background: #f59e0b; animation: blink 1s ease-in-out infinite; }
.dot-done    { background: #22c55e; }
.dot-error   { background: #ef4444; }
@keyframes blink { 0%,100%{opacity:1} 50%{opacity:.25} }

/* 悬浮气泡 */
.dz-bubble {
  display: none;
  position: absolute;
  top: 38px; left: 50%;
  transform: translateX(-50%);
  background: #1e2535;
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 10px;
  padding: 8px 12px;
  min-width: 150px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.5);
  z-index: 999;
  white-space: nowrap;
}
.dz-bname   { font-size: 13px; font-weight: 600; color: #e2e8f0; margin-bottom: 3px; }
.dz-bstatus { font-size: 12px; color: rgba(255,255,255,0.4); margin-bottom: 3px; }
.dz-breport { font-size: 12px; color: rgba(255,255,255,0.55); font-style: italic; white-space: normal; max-width: 200px; }

/* 飞入动画 */
.avatar-fly-enter-active { animation: flyIn 0.32s cubic-bezier(0.34,1.56,0.64,1); }
.avatar-fly-leave-active { animation: flyOut 0.25s ease-in forwards; }
@keyframes flyIn  { from { opacity:0; transform:scale(0.4) translateY(10px); } to { opacity:1; transform:scale(1); } }
@keyframes flyOut { from { opacity:1; transform:scale(1); } to { opacity:0; transform:scale(0.3); } }

.zone-fade-enter-active, .zone-fade-leave-active { transition: opacity 0.3s; }
.zone-fade-enter-from, .zone-fade-leave-to { opacity: 0; }

/* 右侧伸缩 & 菜单按钮 */
.topbar-flex { flex: 1; }
.menu-btn {
  width: 34px; height: 34px;
  border-radius: 8px;
  border: 1px solid rgba(255,255,255,0.1);
  background: rgba(255,255,255,0.05);
  color: rgba(255,255,255,0.45);
  display: flex; align-items: center; justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
}
.menu-btn:hover {
  background: rgba(129,140,248,0.15);
  border-color: rgba(129,140,248,0.4);
  color: #a5b4fc;
}

/* ── 聊天区域 ─────────────────────────────────────────────────────────── */
.chat-main {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 让 AiChat 组件充满剩余高度 */
:deep(.chat-main > .ai-chat-root),
:deep(.chat-main > div) {
  height: 100%;
  flex: 1;
}

.chat-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255,255,255,0.3);
}
.empty-hint { text-align: center; }
.empty-icon { font-size: 48px; margin-bottom: 12px; }
.empty-hint a { color: #818cf8; cursor: pointer; text-decoration: underline; }
</style>
