<template>
  <div class="chat-home">

    <!-- ══ 顶部工具栏 ══════════════════════════════════════════════════════ -->
    <header class="topbar">
      <!-- Logo -->
      <div class="topbar-logo" @click="$router.push('/dashboard')">
        <svg width="26" height="26" viewBox="0 0 24 24" fill="none">
          <path d="M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z" fill="url(#tg)"/>
          <defs>
            <linearGradient id="tg" x1="2.5" y1="2" x2="21.5" y2="22" gradientUnits="userSpaceOnUse">
              <stop offset="0%" stop-color="#6366f1"/>
              <stop offset="100%" stop-color="#0ea5e9"/>
            </linearGradient>
          </defs>
          <text x="12" y="16" text-anchor="middle" fill="white" font-size="8.5" font-weight="900" font-family="sans-serif">Z</text>
        </svg>
        <span class="topbar-brand">引巢</span>
      </div>

      <!-- 成员选择器 -->
      <div class="topbar-members">
        <button
          v-for="ag in agents"
          :key="ag.id"
          class="member-btn"
          :class="{ active: ag.id === currentAgentId }"
          :title="ag.name"
          @click="switchAgent(ag.id)"
        >
          <div class="member-avatar" :style="{ background: ag.avatarColor || '#6366f1' }">
            {{ (ag.name || '?')[0] }}
          </div>
          <span class="member-name">{{ ag.name }}</span>
        </button>
      </div>

      <!-- 分隔 -->
      <div class="topbar-sep" />

      <!-- 模型选择器 -->
      <el-select
        v-model="currentModelId"
        size="small"
        class="model-select"
        placeholder="选择模型"
        @change="onModelChange"
      >
        <el-option
          v-for="m in models"
          :key="m.id"
          :label="m.name || m.model"
          :value="m.id"
        />
      </el-select>

      <!-- 历史会话 -->
      <el-select
        v-model="currentSessionKey"
        size="small"
        class="session-select"
        placeholder="当前会话"
        @change="onSessionChange"
      >
        <el-option value="" label="+ 新会话" />
        <el-option
          v-for="s in sessions"
          :key="s.key"
          :label="s.preview || s.key"
          :value="s.key"
        />
      </el-select>

      <!-- 派遣区域 -->
      <div class="dispatch-zone" v-if="dispatched.length > 0">
        <div class="dispatch-zone-label">派遣中</div>
        <TransitionGroup name="avatar-fly" tag="div" class="dispatch-avatars">
          <div
            v-for="d in dispatched"
            :key="d.taskId"
            class="dispatch-avatar-wrap"
            :title="`${d.agentName} — ${statusText(d.status)}`"
          >
            <div
              class="dispatch-avatar"
              :style="{ background: d.avatarColor || '#6366f1' }"
              :class="'dstatus-' + d.status"
            >
              {{ (d.agentName || '?')[0] }}
            </div>
            <span class="dispatch-dot" :class="'dot-' + d.status" />
            <!-- 悬浮进度气泡 -->
            <div class="dispatch-tooltip">
              <div class="dt-name">{{ d.agentName }}</div>
              <div class="dt-status">{{ statusText(d.status) }}</div>
              <div v-if="d.latestReport" class="dt-report">"{{ d.latestReport }}"</div>
            </div>
          </div>
        </TransitionGroup>
      </div>

      <div class="topbar-spacer" />

      <!-- 汉堡菜单 -->
      <el-dropdown trigger="click" @command="onNav">
        <button class="menu-btn">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
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
        :initial-session-key="currentSessionKey || undefined"
        :show-dispatch-panel="true"
        @dispatch="onDispatch"
        @session-change="onSessionCreated"
        @streaming-change="(v) => isStreaming = v"
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
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { agents as agentsApi, models as modelsApi } from '../api'
import AiChat from '../components/AiChat.vue'
import type { AgentInfo, ModelEntry } from '../api'

const router = useRouter()

// ── 状态 ──────────────────────────────────────────────────────────────────
const agents = ref<AgentInfo[]>([])
const models = ref<ModelEntry[]>([])
const currentAgentId = ref('')
const currentModelId = ref('')
const currentSessionKey = ref('')
const isStreaming = ref(false)
const chatKey = ref(0) // 用于强制重置 AiChat

// 历史会话（从 agent sessions 接口获取）
const sessions = ref<{ key: string; preview: string }[]>([])

// 派遣任务列表
interface DispatchedTask {
  taskId: string
  agentId: string
  agentName: string
  avatarColor: string
  status: 'running' | 'done' | 'error'
  latestReport: string
  _pollTimer?: number
}
const dispatched = ref<DispatchedTask[]>([])

// ── 初始化 ────────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadAgents(), loadModels()])
})

async function loadAgents() {
  try {
    const res = await agentsApi.list()
    agents.value = res.data.filter(a => !a.system)
    // 恢复上次选中的成员
    const saved = localStorage.getItem('chat_home_agent')
    if (saved && agents.value.find(a => a.id === saved)) {
      currentAgentId.value = saved
    } else if (agents.value.length > 0) {
      currentAgentId.value = agents.value[0]?.id ?? ""
    }
    if (currentAgentId.value) {
      await loadSessions(currentAgentId.value)
    }
  } catch {}
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    models.value = res.data
    // 恢复上次选中的模型
    const saved = localStorage.getItem('chat_home_model')
    if (saved && models.value.find(m => m.id === saved)) {
      currentModelId.value = saved
    } else {
      // 默认选第一个 supportsTools 的
      const def = models.value.find(m => m.isDefault) || models.value[0]
      if (def) currentModelId.value = def.id
    }
  } catch {}
}

async function loadSessions(agentId: string) {
  try {
    const res = await agentsApi.listSessions(agentId, 20)
    sessions.value = (res.data || []).map((s: any) => ({
      key: s.key,
      preview: s.lastMessage ? s.lastMessage.slice(0, 40) : s.key,
    }))
  } catch {}
}

// ── 切换成员 ──────────────────────────────────────────────────────────────
function switchAgent(id: string) {
  if (id === currentAgentId.value) return
  currentAgentId.value = id
  currentSessionKey.value = ''
  chatKey.value++
  localStorage.setItem('chat_home_agent', id)
  loadSessions(id)
}

// ── 切换模型 ──────────────────────────────────────────────────────────────
async function onModelChange(id: string) {
  localStorage.setItem('chat_home_model', id)
  // 更新当前成员的 modelId
  if (currentAgentId.value) {
    try {
      await agentsApi.update(currentAgentId.value, { modelId: id })
    } catch {}
  }
}

// ── 切换会话 ──────────────────────────────────────────────────────────────
function onSessionChange(key: string) {
  currentSessionKey.value = key
  chatKey.value++
}

function onSessionCreated(key: string) {
  currentSessionKey.value = key
  // 加入历史列表
  if (!sessions.value.find(s => s.key === key)) {
    sessions.value.unshift({ key, preview: '新会话' })
  }
}

// ── 派遣动画 ──────────────────────────────────────────────────────────────
function onDispatch(agentId: string, agentName: string, avatarColor: string, taskId: string) {
  // 查找真实头像颜色
  const agInfo = agents.value.find(a => a.id === agentId)
  const color = agInfo?.avatarColor || avatarColor || '#6366f1'

  // 防止重复
  if (dispatched.value.find(d => d.taskId === taskId)) return

  const task: DispatchedTask = {
    taskId,
    agentId,
    agentName: agInfo?.name || agentName,
    avatarColor: color,
    status: 'running',
    latestReport: '',
  }
  dispatched.value.push(task)

  // 开始轮询状态
  startPolling(task)
}

function startPolling(task: DispatchedTask) {
  const token = localStorage.getItem('aipanel_token') || ''
  const base = (localStorage.getItem('aipanel_url') || '').replace(/\/$/, '')

  let attempts = 0
  const poll = async () => {
    if (attempts++ > 60) {
      task.status = 'error'
      return
    }
    try {
      const res = await fetch(`${base}/api/agents/${task.agentId}/tasks/${task.taskId}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('not found')
      const data = await res.json()

      if (data.output) task.latestReport = data.output.slice(-100)
      if (data.status === 'done') {
        task.status = 'done'
        // 5秒后从派遣区域移除
        setTimeout(() => {
          dispatched.value = dispatched.value.filter(d => d.taskId !== task.taskId)
        }, 5000)
        return
      }
      if (data.status === 'error') {
        task.status = 'error'
        setTimeout(() => {
          dispatched.value = dispatched.value.filter(d => d.taskId !== task.taskId)
        }, 8000)
        return
      }
    } catch {}
    // 继续轮询
    setTimeout(poll, 3000)
  }
  setTimeout(poll, 2000)
}

function statusText(status: string) {
  return { running: '执行中', done: '已完成', error: '失败' }[status] || status
}

// ── 导航 ─────────────────────────────────────────────────────────────────
function onNav(path: string) {
  router.push(path)
}
</script>

<style scoped>
/* ── 整体布局 ─────────────────────────────────────────────────────────── */
.chat-home {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #0d0e1a;
  overflow: hidden;
}

/* ── 顶部栏 ──────────────────────────────────────────────────────────── */
.topbar {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 56px;
  padding: 0 16px;
  background: rgba(18, 19, 35, 0.92);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
  z-index: 100;
}

.topbar-logo {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  flex-shrink: 0;
  margin-right: 8px;
}
.topbar-brand {
  font-size: 15px;
  font-weight: 700;
  color: #e8e8f8;
  letter-spacing: 0.5px;
}

/* 成员列表 */
.topbar-members {
  display: flex;
  align-items: center;
  gap: 4px;
  overflow-x: auto;
  max-width: 360px;
  scrollbar-width: none;
}
.topbar-members::-webkit-scrollbar { display: none; }

.member-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 6px;
  border-radius: 20px;
  border: 1.5px solid transparent;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
  color: rgba(255,255,255,0.5);
}
.member-btn:hover {
  background: rgba(255,255,255,0.07);
  color: rgba(255,255,255,0.85);
}
.member-btn.active {
  border-color: rgba(99,102,241,0.6);
  background: rgba(99,102,241,0.12);
  color: #a5b4fc;
}

.member-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  color: white;
  flex-shrink: 0;
}
.member-name {
  font-size: 13px;
  font-weight: 500;
  max-width: 72px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar-sep {
  width: 1px;
  height: 28px;
  background: rgba(255,255,255,0.08);
  flex-shrink: 0;
  margin: 0 4px;
}

/* 模型/会话选择器 */
.model-select { width: 160px; flex-shrink: 0; }
.session-select { width: 180px; flex-shrink: 0; }

:deep(.model-select .el-input__wrapper),
:deep(.session-select .el-input__wrapper) {
  background: rgba(255,255,255,0.06) !important;
  box-shadow: 0 0 0 1px rgba(255,255,255,0.1) !important;
  color: rgba(255,255,255,0.7);
}
:deep(.model-select .el-input__inner),
:deep(.session-select .el-input__inner) {
  color: rgba(255,255,255,0.75) !important;
  font-size: 13px;
}
:deep(.el-select__caret) { color: rgba(255,255,255,0.4) !important; }

/* ── 派遣区域 ─────────────────────────────────────────────────────────── */
.dispatch-zone {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-left: 1px solid rgba(255,255,255,0.07);
  flex-shrink: 0;
}
.dispatch-zone-label {
  font-size: 11px;
  color: rgba(255,165,0,0.7);
  font-weight: 600;
  letter-spacing: 0.5px;
}

.dispatch-avatars {
  display: flex;
  align-items: center;
  gap: 6px;
}

.dispatch-avatar-wrap {
  position: relative;
  cursor: pointer;
}
.dispatch-avatar-wrap:hover .dispatch-tooltip {
  display: block;
}

.dispatch-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  color: white;
  border: 2px solid transparent;
  transition: border-color 0.3s;
}
.dispatch-avatar.dstatus-running { border-color: #f59e0b; animation: pulse-border 1.5s ease-in-out infinite; }
.dispatch-avatar.dstatus-done    { border-color: #22c55e; }
.dispatch-avatar.dstatus-error   { border-color: #ef4444; }

@keyframes pulse-border {
  0%, 100% { box-shadow: 0 0 0 0 rgba(245,158,11,0.4); }
  50%       { box-shadow: 0 0 0 4px rgba(245,158,11,0); }
}

.dispatch-dot {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  border: 1.5px solid #0d0e1a;
}
.dot-running { background: #f59e0b; animation: dot-blink 1s ease-in-out infinite; }
.dot-done    { background: #22c55e; }
.dot-error   { background: #ef4444; }
@keyframes dot-blink { 0%,100%{opacity:1} 50%{opacity:0.3} }

/* 悬浮气泡 */
.dispatch-tooltip {
  display: none;
  position: absolute;
  top: 36px;
  left: 50%;
  transform: translateX(-50%);
  background: #1e2035;
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 8px;
  padding: 8px 12px;
  min-width: 160px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.5);
  z-index: 999;
  white-space: nowrap;
}
.dt-name   { font-size: 13px; font-weight: 600; color: #e0e0f0; margin-bottom: 4px; }
.dt-status { font-size: 12px; color: rgba(255,255,255,0.4); margin-bottom: 4px; }
.dt-report { font-size: 12px; color: rgba(255,255,255,0.55); font-style: italic; white-space: normal; max-width: 200px; }

/* 派遣飞入动画 */
.avatar-fly-enter-active { animation: fly-in 0.35s cubic-bezier(0.34, 1.56, 0.64, 1); }
.avatar-fly-leave-active { animation: fly-out 0.3s ease-in forwards; }
@keyframes fly-in {
  from { opacity: 0; transform: translateY(12px) scale(0.6); }
  to   { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes fly-out {
  from { opacity: 1; transform: scale(1); }
  to   { opacity: 0; transform: scale(0.5); }
}

/* 工具栏右侧 */
.topbar-spacer { flex: 1; }
.menu-btn {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  border: 1px solid rgba(255,255,255,0.1);
  background: rgba(255,255,255,0.05);
  color: rgba(255,255,255,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
}
.menu-btn:hover {
  background: rgba(255,255,255,0.1);
  color: rgba(255,255,255,0.85);
}

/* ── 聊天主区域 ───────────────────────────────────────────────────────── */
.chat-main {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* 让 AiChat 撑满剩余空间 */
:deep(.ai-chat) {
  height: 100%;
  border-radius: 0 !important;
  border: none !important;
  background: transparent !important;
}
:deep(.ai-chat .chat-messages) {
  background: #0d0e1a !important;
}
:deep(.ai-chat .chat-input-area) {
  background: rgba(18,19,35,0.9) !important;
  border-top: 1px solid rgba(255,255,255,0.06) !important;
}

/* 空状态 */
.chat-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.empty-hint {
  text-align: center;
  color: rgba(255,255,255,0.35);
}
.empty-icon { font-size: 48px; margin-bottom: 12px; }
.empty-hint a {
  color: #6366f1;
  cursor: pointer;
  text-decoration: underline;
}
</style>
