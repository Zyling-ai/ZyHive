<template>
  <div class="chat-home">
    <!-- ── 顶部工具栏 ── -->
    <header class="top-bar">
      <!-- 左：Logo -->
      <div class="topbar-left">
        <span class="logo-text">引巢 · ZyHive</span>
        <span v-if="appVersion" class="logo-version">{{ appVersion }}</span>
      </div>

      <!-- 中：成员选择器 + 模型选择器 + 历史会话选择器 -->
      <div class="topbar-center">
        <!-- 成员头像列表 -->
        <div class="member-list">
          <div
            v-for="agent in agentList"
            :key="agent.id"
            class="member-avatar"
            :class="{ active: selectedAgentId === agent.id }"
            :style="{ background: agent.avatarColor || '#409eff' }"
            :title="agent.name"
            @click="selectAgent(agent.id)"
          >
            {{ agent.name.charAt(0) }}
          </div>
        </div>

        <!-- 模型选择器 -->
        <el-select
          v-model="selectedModel"
          placeholder="默认模型"
          size="small"
          class="model-select"
          clearable
          @change="onModelChange"
        >
          <el-option
            v-for="m in modelList"
            :key="m.id"
            :label="m.name || m.id"
            :value="m.id"
          />
        </el-select>

        <!-- 历史会话选择器 -->
        <el-select
          v-model="selectedSessionId"
          placeholder="新对话"
          size="small"
          class="session-select"
          clearable
          @change="onSessionChange"
        >
          <el-option
            v-for="s in sessionList"
            :key="s.id"
            :label="s.title || s.id.slice(0, 12)"
            :value="s.id"
          />
        </el-select>
      </div>

      <!-- 右：派遣区域 + 汉堡菜单 -->
      <div class="topbar-right">
        <!-- 派遣区域 -->
        <div class="dispatch-zone">
          <TransitionGroup name="dispatch-avatar">
            <div
              v-for="item in dispatchedAgents"
              :key="item.taskId"
              class="dispatch-avatar"
              :style="{ background: item.avatarColor || '#409eff' }"
              :title="item.agentName"
            >
              {{ (item.agentName || item.agentId).charAt(0) }}
              <!-- 状态指示灯 -->
              <span
                class="dispatch-status-dot"
                :class="{
                  'status-running': item.taskStatus === 'running' || item.taskStatus === 'pending',
                  'status-done': item.taskStatus === 'done',
                  'status-error': item.taskStatus === 'error' || item.taskStatus === 'killed',
                }"
              />
              <!-- 悬停气泡 -->
              <div class="dispatch-tooltip">
                <div class="tooltip-name">{{ item.agentName || item.agentId }}</div>
                <div class="tooltip-status">
                  {{ item.taskStatus === 'running' || item.taskStatus === 'pending' ? '执行中…' :
                     item.taskStatus === 'done' ? '✅ 已完成' :
                     item.taskStatus === 'error' ? '❌ 失败' :
                     item.taskStatus === 'killed' ? '🛑 已终止' : item.taskStatus }}
                </div>
              </div>
            </div>
          </TransitionGroup>
        </div>

        <!-- 汉堡菜单 -->
        <div class="hamburger-wrap">
          <button class="hamburger-btn" @click="menuOpen = !menuOpen" :class="{ open: menuOpen }">
            <span /><span /><span />
          </button>
          <Transition name="menu-fade">
            <div v-if="menuOpen" class="hamburger-menu" @click.stop>
              <router-link v-for="item in menuItems" :key="item.path" :to="item.path" class="menu-item" @click="menuOpen = false">
                <span class="menu-icon">{{ item.icon }}</span>
                <span>{{ item.label }}</span>
              </router-link>
            </div>
          </Transition>
        </div>
      </div>
    </header>

    <!-- ── 聊天区域 ── -->
    <div class="chat-body">
      <AiChat
        v-if="selectedAgentId"
        ref="aiChatRef"
        :agentId="selectedAgentId"
        :sessionId="chatSessionId"
        height="100%"
        bgColor="transparent"
        @dispatch="onDispatch"
        @session-change="onSessionCreated"
      />
      <div v-else class="no-agent-hint">
        <div class="hint-icon">🤖</div>
        <div class="hint-text">请先在顶部选择一个 AI 成员</div>
        <router-link to="/agents/new" class="hint-link">新建成员 →</router-link>
      </div>
    </div>

    <!-- 飞行动画的头像（用于入场动画效果） -->
    <div
      v-for="fly in flyingAvatars"
      :key="fly.id"
      class="flying-avatar"
      :style="fly.style"
    >
      {{ fly.char }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import AiChat from '../components/AiChat.vue'
import { agents as agentsApi, models as modelsApi, sessions as sessionsApi, tasks as tasksApi } from '../api'
import type { AgentInfo, ModelEntry, SessionSummary } from '../api'

// ── State ─────────────────────────────────────────────────────────────────
const agentList = ref<AgentInfo[]>([])
const modelList = ref<ModelEntry[]>([])
const sessionList = ref<SessionSummary[]>([])
const selectedAgentId = ref('')
const selectedModel = ref('')
const selectedSessionId = ref('')
const chatSessionId = ref<string | undefined>(undefined)
const menuOpen = ref(false)
const appVersion = ref('')

const aiChatRef = ref<InstanceType<typeof AiChat> | null>(null)


// ── Dispatched agents 状态 ─────────────────────────────────────────────────
interface DispatchedAgent {
  taskId: string
  agentId: string
  agentName: string
  avatarColor: string
  taskStatus: 'pending' | 'running' | 'done' | 'error' | 'killed'
}
const dispatchedAgents = ref<DispatchedAgent[]>([])
let dispatchPollTimer: ReturnType<typeof setInterval> | null = null

// 飞行动画
interface FlyingAvatar {
  id: string
  char: string
  color: string
  style: Record<string, string>
}
const flyingAvatars = ref<FlyingAvatar[]>([])

// ── Menu items ──────────────────────────────────────────────────────────────
const menuItems = [
  { path: '/dashboard', label: '仪表盘', icon: '📊' },
  { path: '/agents', label: '成员管理', icon: '🤖' },
  { path: '/team', label: '团队', icon: '👥' },
  { path: '/goals', label: '目标/规划', icon: '🎯' },
  { path: '/projects', label: '项目', icon: '📁' },
  { path: '/chats', label: '对话管理', icon: '💬' },
  { path: '/skills', label: '技能', icon: '⚡' },
  { path: '/config/tools', label: '工具', icon: '🔧' },
  { path: '/settings', label: '设置', icon: '⚙️' },
  { path: '/config/models', label: '模型配置', icon: '🔑' },
  { path: '/usage', label: '用量统计', icon: '📈' },
  { path: '/logs', label: '日志', icon: '📋' },
]

// ── Methods ───────────────────────────────────────────────────────────────
async function loadAgents() {
  try {
    const res = await agentsApi.list()
    agentList.value = res.data
    if (!selectedAgentId.value && res.data.length > 0) {
      selectedAgentId.value = res.data[0]!.id
    }
  } catch {}
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    modelList.value = res.data
  } catch {}
}

async function loadSessions() {
  if (!selectedAgentId.value) return
  try {
    const res = await sessionsApi.list({ agentId: selectedAgentId.value, limit: 30 })
    sessionList.value = res.data.sessions ?? []
  } catch {}
}

function selectAgent(id: string) {
  if (selectedAgentId.value === id) return
  selectedAgentId.value = id
  selectedSessionId.value = ''
  chatSessionId.value = undefined
  loadSessions()
}

function onModelChange(_val: string) {
  // Model selection: in future could pass to AiChat
}

function onSessionChange(val: string) {
  if (!val) {
    chatSessionId.value = undefined
    aiChatRef.value?.startNewSession()
  } else {
    chatSessionId.value = val
    aiChatRef.value?.resumeSession(val)
  }
}

function onSessionCreated(sessionId: string) {
  chatSessionId.value = sessionId
  // Reload session list so new session appears
  loadSessions()
}

// ── Dispatch handler ──────────────────────────────────────────────────────
function onDispatch(agentId: string, agentName: string, avatarColor: string, taskId: string) {
  // Find agent name from list if not provided
  const agent = agentList.value.find(a => a.id === agentId)
  const name = agent?.name || agentName || agentId
  const color = agent?.avatarColor || avatarColor || '#409eff'

  // Add to dispatched list
  dispatchedAgents.value.push({
    taskId,
    agentId,
    agentName: name,
    avatarColor: color,
    taskStatus: 'pending',
  })

  // Trigger fly-in animation (CSS transition handles the visual)
  startDispatchPolling()
}

function startDispatchPolling() {
  if (dispatchPollTimer) return
  dispatchPollTimer = setInterval(pollDispatchedTasks, 3000)
}

async function pollDispatchedTasks() {
  const active = dispatchedAgents.value.filter(
    d => !['done', 'error', 'killed'].includes(d.taskStatus)
  )
  if (active.length === 0) {
    if (dispatchPollTimer) {
      clearInterval(dispatchPollTimer)
      dispatchPollTimer = null
    }
    return
  }
  for (const item of active) {
    try {
      const res = await tasksApi.get(item.taskId)
      item.taskStatus = res.data.status as DispatchedAgent['taskStatus']
    } catch {}
  }
}

// ── Close menu on outside click ────────────────────────────────────────────
function onDocumentClick() {
  menuOpen.value = false
}

// ── App version ─────────────────────────────────────────────────────────────
async function loadAppVersion() {
  try {
    const res = await fetch('/api/version')
    if (res.ok) {
      const data = await res.json()
      appVersion.value = data.version ?? ''
    }
  } catch {}
}

// ── Watch agent change → reload sessions ──────────────────────────────────
watch(selectedAgentId, () => {
  loadSessions()
})

// ── Lifecycle ─────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadAgents(), loadModels(), loadAppVersion()])
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  if (dispatchPollTimer) {
    clearInterval(dispatchPollTimer)
    dispatchPollTimer = null
  }
})
</script>

<style scoped>
/* ── 整体布局 ── */
.chat-home {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #0f0f1a;
  color: #e2e8f0;
  overflow: hidden;
  position: relative;
}

/* ── 顶部工具栏 ── */
.top-bar {
  display: flex;
  align-items: center;
  height: 56px;
  padding: 0 16px;
  background: rgba(28, 29, 46, 0.85);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
  gap: 12px;
  z-index: 100;
}

/* 左侧 Logo */
.topbar-left {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.logo-text {
  font-size: 15px;
  font-weight: 600;
  color: #e2e8f0;
  letter-spacing: 0.3px;
  white-space: nowrap;
}
.logo-version {
  font-size: 11px;
  color: #64748b;
  background: rgba(255,255,255,0.06);
  padding: 1px 6px;
  border-radius: 4px;
}

/* 中间区域 */
.topbar-center {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

/* 成员头像列表 */
.member-list {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.member-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  cursor: pointer;
  flex-shrink: 0;
  transition: box-shadow 0.15s, transform 0.15s;
  border: 2px solid transparent;
  user-select: none;
}
.member-avatar:hover {
  transform: scale(1.08);
}
.member-avatar.active {
  border-color: #60a5fa;
  box-shadow: 0 0 0 2px rgba(96, 165, 250, 0.35);
}

/* 模型/会话选择器 */
.model-select {
  width: 150px;
  flex-shrink: 0;
}
.session-select {
  width: 160px;
  min-width: 0;
  flex-shrink: 1;
}

:deep(.el-select .el-input__wrapper) {
  background: rgba(255, 255, 255, 0.06) !important;
  border: 1px solid rgba(255, 255, 255, 0.1) !important;
  box-shadow: none !important;
  border-radius: 8px !important;
}
:deep(.el-select .el-input__inner) {
  color: #e2e8f0 !important;
  font-size: 13px !important;
}
:deep(.el-select .el-input__placeholder) {
  color: #64748b !important;
}

/* 右侧区域 */
.topbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

/* ── 派遣区域 ── */
.dispatch-zone {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex-wrap: nowrap;
}

/* 派遣头像 */
.dispatch-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 600;
  color: #fff;
  position: relative;
  cursor: default;
  flex-shrink: 0;
  transition: transform 0.15s;
}
.dispatch-avatar:hover {
  transform: scale(1.12);
  z-index: 10;
}

/* 状态指示灯 */
.dispatch-status-dot {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  border: 1.5px solid #1c1d2e;
}
.status-running {
  background: #f59e0b;
  animation: pulse-dot 1.2s infinite;
}
.status-done {
  background: #22c55e;
}
.status-error {
  background: #ef4444;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.6; transform: scale(0.85); }
}

/* 悬停气泡 */
.dispatch-tooltip {
  display: none;
  position: absolute;
  top: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  background: rgba(15, 23, 42, 0.96);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 8px;
  padding: 6px 10px;
  white-space: nowrap;
  font-size: 12px;
  color: #e2e8f0;
  z-index: 200;
  box-shadow: 0 4px 16px rgba(0,0,0,0.4);
  pointer-events: none;
}
.dispatch-avatar:hover .dispatch-tooltip {
  display: block;
}
.tooltip-name {
  font-weight: 600;
  margin-bottom: 2px;
}
.tooltip-status {
  color: #94a3b8;
  font-size: 11px;
}

/* TransitionGroup for dispatch avatars */
.dispatch-avatar-enter-active {
  animation: dispatch-fly-in 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.dispatch-avatar-leave-active {
  animation: dispatch-fly-in 0.2s reverse ease-in;
}
@keyframes dispatch-fly-in {
  from {
    opacity: 0;
    transform: scale(0.3) translateY(20px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

/* ── 汉堡菜单 ── */
.hamburger-wrap {
  position: relative;
}
.hamburger-btn {
  width: 32px;
  height: 32px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 0;
  transition: background 0.15s;
}
.hamburger-btn:hover {
  background: rgba(255, 255, 255, 0.1);
}
.hamburger-btn span {
  display: block;
  width: 14px;
  height: 1.5px;
  background: #94a3b8;
  border-radius: 2px;
  transition: all 0.2s;
}
.hamburger-btn.open span:nth-child(1) {
  transform: rotate(45deg) translate(4px, 4px);
}
.hamburger-btn.open span:nth-child(2) {
  opacity: 0;
}
.hamburger-btn.open span:nth-child(3) {
  transform: rotate(-45deg) translate(4px, -4px);
}

.hamburger-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  background: rgba(20, 21, 35, 0.98);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  padding: 6px;
  min-width: 160px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  z-index: 300;
}
.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  text-decoration: none;
  color: #cbd5e1;
  font-size: 13px;
  transition: background 0.15s, color 0.15s;
  cursor: pointer;
}
.menu-item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #f1f5f9;
}
.menu-icon {
  font-size: 14px;
  width: 18px;
  text-align: center;
}

/* Menu animation */
.menu-fade-enter-active,
.menu-fade-leave-active {
  transition: opacity 0.15s, transform 0.15s;
}
.menu-fade-enter-from,
.menu-fade-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.97);
}

/* ── 聊天区域 ── */
.chat-body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* Override AiChat colors for dark theme */
.chat-body :deep(.ai-chat) {
  background: transparent;
  color: #e2e8f0;
}
.chat-body :deep(.msg-bubble.assistant) {
  background: #1c1d2e;
  color: #e2e8f0;
  box-shadow: none;
  border: 1px solid rgba(255,255,255,0.06);
}
.chat-body :deep(.msg-bubble.user) {
  background: #3b82f6;
}
.chat-body :deep(.msg-system) {
  background: rgba(255,255,255,0.04);
  color: #64748b;
  border: none;
}
.chat-body :deep(.chat-messages) {
  background: transparent;
}
.chat-body :deep(.chat-input-area) {
  background: rgba(28, 29, 46, 0.9);
  border-top: 1px solid rgba(255,255,255,0.06);
}
.chat-body :deep(.chat-input-area textarea) {
  background: rgba(255,255,255,0.05);
  color: #e2e8f0;
  border: 1px solid rgba(255,255,255,0.1);
}
.chat-body :deep(.chat-input-area textarea::placeholder) {
  color: #64748b;
}
.chat-body :deep(.send-btn) {
  background: #3b82f6;
  border-color: #3b82f6;
}
.chat-body :deep(.send-btn:hover:not(:disabled)) {
  background: #2563eb;
}
.chat-body :deep(.msg-token-usage) {
  color: #475569;
}
.chat-body :deep(.running-tasks-banner) {
  background: rgba(245, 158, 11, 0.1);
  border-color: rgba(245, 158, 11, 0.3);
  color: #fbbf24;
}
.chat-body :deep(.thinking-block) {
  background: rgba(255,255,255,0.04);
  border-color: rgba(255,255,255,0.08);
}
.chat-body :deep(.thinking-summary),
.chat-body :deep(.thinking-content) {
  color: #94a3b8;
}
.chat-body :deep(.tool-step) {
  background: rgba(255,255,255,0.04);
  border-color: rgba(255,255,255,0.08);
}
.chat-body :deep(.tool-step-name),
.chat-body :deep(.tool-step-summary),
.chat-body :deep(.tool-step-dur) {
  color: #94a3b8;
}
.chat-body :deep(.act-btn) {
  border-color: rgba(255,255,255,0.1);
  color: #64748b;
}
.chat-body :deep(.act-btn:hover) {
  background: rgba(255,255,255,0.08);
  color: #e2e8f0;
}
.chat-body :deep(.msg-actions) {
  opacity: 0;
}
.chat-body :deep(.msg-bubble.assistant:hover .msg-actions) {
  opacity: 1;
}
.chat-body :deep(.history-loading-text) {
  color: #64748b;
}
.chat-body :deep(.chat-empty) {
  color: #64748b;
}
.chat-body :deep(.example-chip) {
  background: rgba(255,255,255,0.06);
  border-color: rgba(255,255,255,0.1);
  color: #60a5fa;
}
.chat-body :deep(.example-chip:hover) {
  background: rgba(96,165,250,0.15);
  border-color: #60a5fa;
}
.chat-body :deep(.option-chip) {
  background: rgba(255,255,255,0.06);
  border-color: rgba(255,255,255,0.1);
  color: #60a5fa;
}
.chat-body :deep(.option-chip:hover) {
  background: rgba(96,165,250,0.15);
  border-color: #60a5fa;
}

/* ── 无成员提示 ── */
.no-agent-hint {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
  color: #64748b;
}
.hint-icon {
  font-size: 48px;
  opacity: 0.4;
}
.hint-text {
  font-size: 15px;
}
.hint-link {
  color: #3b82f6;
  text-decoration: none;
  font-size: 14px;
}
.hint-link:hover {
  text-decoration: underline;
}

/* ── 飞行动画头像 ── */
.flying-avatar {
  position: fixed;
  z-index: 9999;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  pointer-events: none;
  transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}

/* ── Responsive ── */
@media (max-width: 640px) {
  .topbar-center {
    gap: 6px;
  }
  .model-select {
    width: 110px;
  }
  .session-select {
    width: 110px;
  }
  .member-avatar {
    width: 28px;
    height: 28px;
    font-size: 11px;
  }
  .logo-version {
    display: none;
  }
}
</style>
