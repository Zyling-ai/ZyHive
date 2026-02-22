<template>
  <div class="ai-chat" :class="{ compact, 'has-bg': bgColor, 'drag-active': isDragOver }" :style="rootStyle"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent
    @dragleave="onDragLeave"
    @drop.prevent.stop="handleGlobalDrop">

    <!-- ── 全局拖拽覆盖层（pointer-events:none 避免吸走事件）── -->
    <Transition name="drag-fade">
      <div v-if="isDragOver" class="drag-overlay">
        <div class="drag-overlay-content">
          <div class="drag-overlay-icon">📎</div>
          <div class="drag-overlay-title">松开以附加文件</div>
          <div class="drag-overlay-hint">支持图片 · 代码 · 文本文件</div>
        </div>
      </div>
    </Transition>

    <!-- ── 消息列表 ── -->
    <!-- 后台任务运行中 banner -->
    <div v-if="runningTaskCount > 0" class="running-tasks-banner">
      <span class="running-dot" />
      <span>后台有 {{ runningTaskCount }} 个任务正在执行中，关闭窗口后仍会继续运行</span>
      <span v-if="resumedTasks.length" class="resumed-list">
        <span v-for="rt in resumedTasks.filter(t => !['done','error','killed'].includes(t.status))" :key="rt.id"
          class="resumed-chip" :class="rt.status">
          {{ rt.status === 'running' ? '⟳' : '🟡' }} {{ rt.label }}
        </span>
      </span>
    </div>

    <div class="chat-messages" ref="msgListRef">
      <!-- 历史加载中 -->
      <div v-if="historyLoading" class="history-loading">
        <div class="history-loading-dots">
          <span /><span /><span />
        </div>
        <div class="history-loading-text">加载历史对话…</div>
      </div>

      <!-- 欢迎语 / 空状态 -->
      <div v-if="!messages.length && !historyLoading" class="chat-empty">
        <div v-if="welcomeMessage" class="welcome-msg">{{ welcomeMessage }}</div>
        <div v-if="examples.length" class="examples">
          <div v-for="(ex, i) in examples" :key="i"
            class="example-chip" @click="fillInput(ex)">{{ ex }}</div>
        </div>
      </div>

      <!-- streaming 期间跳过最后一条（正在构建的 assistant 消息），由流式占位符渲染 -->
      <template v-for="(msg, i) in (streaming ? messages.slice(0, -1) : messages)" :key="i">

        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="msg-row user">
          <div class="msg-bubble user">
            <!-- 图片附件 -->
            <div v-if="msg.images?.length" class="msg-images">
              <img v-for="(src, j) in msg.images" :key="j" :src="src" class="msg-img" @click="previewImg(src)" />
            </div>
            <div class="msg-text">{{ msg.text }}</div>
          </div>
        </div>

        <!-- AI 消息 -->
        <div v-else-if="msg.role === 'assistant'" class="msg-row assistant">
          <div class="msg-col">
            <!-- 思考过程 -->
            <details v-if="msg.thinking" class="thinking-block" :open="showThinking">
              <summary class="thinking-summary">
                <el-icon class="thinking-icon"><ChatRound /></el-icon> 思考过程
                <span class="thinking-len">{{ msg.thinking.length }} 字符</span>
              </summary>
              <pre class="thinking-content">{{ msg.thinking }}</pre>
            </details>

            <!-- ── 工具调用时间线（气泡外，独立展示）── -->
            <div v-if="msg.toolCalls?.length" class="tool-timeline">
              <div v-for="(tc, ti) in msg.toolCalls" :key="ti"
                class="tool-step" :class="tc.status"
                @click="tc._expanded = !tc._expanded">
                <div class="tool-step-header">
                  <!-- 状态指示 -->
                  <span class="tool-step-dot" :class="tc.status">
                    <span v-if="tc.status==='running'" class="tool-spin">⟳</span>
                    <span v-else-if="tc.status==='done'">✓</span>
                    <span v-else-if="tc.status==='error'">✗</span>
                    <span v-else>○</span>
                  </span>
                  <!-- 工具图标 + 名称 -->
                  <span class="tool-step-icon">{{ toolIcon(tc.name) }}</span>
                  <code class="tool-step-name">{{ tc.name }}</code>
                  <!-- 参数摘要 -->
                  <span v-if="tc.input" class="tool-step-summary">{{ toolSummary(tc.name, tc.input) }}</span>
                  <span class="tool-step-flex"/>
                  <!-- 耗时 -->
                  <span v-if="tc.duration" class="tool-step-dur">{{ tc.duration }}</span>
                  <!-- agent_spawn 后台任务实时状态 -->
                  <span v-if="tc.taskId" class="task-badge" :class="tc.taskStatus">
                    <span v-if="tc.taskStatus === 'pending'">🟡 排队中</span>
                    <span v-else-if="tc.taskStatus === 'running'">
                      <span class="tool-spin">⟳</span> 执行中
                    </span>
                    <span v-else-if="tc.taskStatus === 'done'">✅ 完成</span>
                    <span v-else-if="tc.taskStatus === 'error'">❌ 失败</span>
                    <span v-else-if="tc.taskStatus === 'killed'">🛑 已终止</span>
                  </span>
                  <!-- 展开箭头 -->
                  <span class="tool-step-chevron">{{ tc._expanded ? '▲' : '▼' }}</span>
                </div>
                <!-- 详情（可展开）-->
                <div v-if="tc._expanded" class="tool-step-body" @click.stop>
                  <div v-if="tc.input" class="tool-section">
                    <div class="tool-label">INPUT</div>
                    <pre class="tool-pre">{{ fmtJson(tc.input) }}</pre>
                  </div>
                  <div v-if="tc.result" class="tool-section">
                    <div class="tool-label">OUTPUT</div>
                    <pre class="tool-pre result">{{ tc.result.slice(0, 3000) }}{{ tc.result.length > 3000 ? '\n… (截断)' : '' }}</pre>
                  </div>
                </div>
              </div>
            </div>

            <!-- 消息气泡（仅文字，无工具内容）-->
            <div class="msg-bubble assistant">
              <!-- 正文 -->
              <div v-if="msg.text" class="msg-text" v-html="renderMd(msg.text)" />

              <!-- Apply card（给 agent-creation 页用） -->
              <div v-if="msg.applyData && props.applyable" class="apply-card">
                <div class="apply-preview">
                  <div v-for="(val, key) in msg.applyData" :key="key" class="apply-row">
                    <span class="apply-key">{{ key }}</span>
                    <span class="apply-val">{{ String(val).slice(0, 60) }}{{ String(val).length > 60 ? '…' : '' }}</span>
                  </div>
                </div>
                <button class="apply-btn" @click="$emit('apply', msg.applyData!)">
                  应用到表单 ↙
                </button>
              </div>

              <!-- 操作栏 -->
              <div class="msg-actions">
                <button class="act-btn" @click="copyMsg(msg.text)" :title="copied === i ? '已复制' : '复制'">
                  <el-icon v-if="copied === i"><Check /></el-icon><el-icon v-else><CopyDocument /></el-icon>
                </button>
                <button class="act-btn" @click="retryMsg(i)" title="重试">↺</button>
                <!-- 手动触发：当自动解析失败时可手动点 -->
                <button v-if="props.applyable && !msg.applyData && hasJsonBlock(msg.text)"
                  class="act-btn apply-manual-btn"
                  @click="manualApply(msg)"
                  title="检测到配置 JSON，点击应用">
                  <el-icon><Setting /></el-icon> 应用配置
                </button>
              </div>

            </div><!-- end msg-bubble -->

            <!-- Option chips：AI 给出选项时显示为可点击按钮 -->
            <div v-if="msg.options && msg.options.length" class="option-chips">
              <button v-for="(opt, oi) in msg.options" :key="oi"
                class="option-chip"
                @click="fillInput(opt)">
                {{ opt }}
              </button>
            </div>
          </div><!-- /msg-col -->
        </div><!-- /msg-row.assistant -->

        <!-- 系统提示 / 错误 -->
        <div v-else-if="msg.role === 'system'" class="msg-row system">
          <div class="msg-system">{{ msg.text }}</div>
        </div>

      </template>

      <!-- 流式占位 -->
      <div v-if="streaming" class="msg-row assistant">
        <div class="msg-col">
          <!-- 流式思考 -->
          <details v-if="streamThinking && showThinking" class="thinking-block" open>
            <summary class="thinking-summary">
              <el-icon class="thinking-icon"><ChatRound /></el-icon> 思考中…
            </summary>
            <pre class="thinking-content">{{ streamThinking }}<span class="blink">▊</span></pre>
          </details>
          <!-- 流式工具调用时间线 -->
          <div v-if="streamToolCalls.length" class="tool-timeline">
            <div v-for="(tc, ti) in streamToolCalls" :key="ti"
              class="tool-step" :class="tc.status"
              @click="tc._expanded = !tc._expanded">
              <div class="tool-step-header">
                <span class="tool-step-dot" :class="tc.status">
                  <span v-if="tc.status==='running'" class="tool-spin">⟳</span>
                  <span v-else-if="tc.status==='done'">✓</span>
                  <span v-else-if="tc.status==='error'">✗</span>
                </span>
                <span class="tool-step-icon">{{ toolIcon(tc.name) }}</span>
                <code class="tool-step-name">{{ tc.name }}</code>
                <span v-if="tc.input" class="tool-step-summary">{{ toolSummary(tc.name, tc.input) }}</span>
                <span class="tool-step-flex"/>
                <span v-if="tc.duration" class="tool-step-dur">{{ tc.duration }}</span>
                <span v-if="tc.taskId" class="task-badge" :class="tc.taskStatus">
                  <span v-if="tc.taskStatus === 'pending'">🟡 排队中</span>
                  <span v-else-if="tc.taskStatus === 'running'"><span class="tool-spin">⟳</span> 执行中</span>
                  <span v-else-if="tc.taskStatus === 'done'">✅ 完成</span>
                  <span v-else-if="tc.taskStatus === 'error'">❌ 失败</span>
                  <span v-else-if="tc.taskStatus === 'killed'">🛑 已终止</span>
                </span>
                <span class="tool-step-chevron">{{ tc._expanded ? '▲' : '▼' }}</span>
              </div>
              <div v-if="tc._expanded" class="tool-step-body" @click.stop>
                <div v-if="tc.input" class="tool-section">
                  <div class="tool-label">INPUT</div>
                  <pre class="tool-pre">{{ fmtJson(tc.input) }}</pre>
                </div>
                <div v-if="tc.result" class="tool-section">
                  <div class="tool-label">OUTPUT</div>
                  <pre class="tool-pre result">{{ tc.result.slice(0, 3000) }}</pre>
                </div>
              </div>
            </div>
          </div>
          <!-- 流式文字气泡 -->
          <div class="msg-bubble assistant" v-if="streamText || !streamToolCalls.length">
            <div v-if="!streamText && !streamToolCalls.length" class="typing-dots">
              <span /><span /><span />
            </div>
            <div v-if="streamText" class="msg-text" v-html="renderMd(streamText)" />
            <span v-if="streamText" class="blink">▊</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ── 图片预览弹窗 ── -->
    <div v-if="previewSrc" class="img-preview-mask" @click="previewSrc = ''">
      <img :src="previewSrc" class="img-preview-full" />
    </div>

    <!-- ── 输入区 ── -->
    <div class="chat-input-area">
      <!-- 附件预览条（图片 + 文件）-->
      <div v-if="pendingImages.length || pendingFiles.length" class="attachments-bar">
        <!-- 图片缩略图 -->
        <div v-for="(src, i) in pendingImages" :key="'img-'+i" class="attach-thumb">
          <img :src="src" />
          <button class="remove-attach" @click="removeImage(i)">×</button>
        </div>
        <!-- 文本文件芯片 -->
        <div v-for="(f, i) in pendingFiles" :key="'file-'+i" class="attach-file-chip">
          <span class="attach-file-icon">{{ fileTypeIcon(f.name) }}</span>
          <span class="attach-file-name">{{ f.name }}</span>
          <span class="attach-file-size">{{ formatFileSize(f.content.length) }}</span>
          <button class="attach-file-remove" @click="pendingFiles.splice(i, 1)">×</button>
        </div>
      </div>

      <div class="input-row">
        <div class="textarea-wrap">
          <textarea
            ref="inputRef"
            v-model="inputText"
            :placeholder="placeholder || '输入消息… 支持拖拽图片或文件 (Ctrl+Enter 发送)'"
            :disabled="streaming || historyLoading"
            rows="1"
            class="chat-textarea"
            @keydown.enter.ctrl.prevent="send"
            @keydown.enter.meta.prevent="send"
            @paste="handlePaste"
            @input="autoGrow"
          />
        </div>
        <div class="input-actions">
          <!-- 通用文件上传 -->
          <label class="icon-btn" title="附加文件（图片/代码/文本）">
            <el-icon><Paperclip /></el-icon>
            <input type="file" multiple hidden @change="handleFileSelect" />
          </label>
          <!-- 发送 -->
          <button class="send-btn" :disabled="streaming || historyLoading || (!inputText.trim() && !pendingImages.length && !pendingFiles.length)"
            @click="send">
            <span v-if="streaming" class="spinner" />
            <span v-else>↑</span>
          </button>
        </div>
      </div>

      <div class="input-hint">Ctrl+Enter 发送 · 支持拖拽图片/文件</div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { chatSSE, resumeSSE, getSessionStatus, sessions as sessionsApi, tasks as tasksApi, type ChatParams } from '../api'

// ── Props ─────────────────────────────────────────────────────────────────
interface Props {
  agentId: string
  /** 指定要续接的 session ID（可选），不传则自动新建 */
  sessionId?: string
  /** 注入到系统提示的额外上下文（页面场景、表单状态等） */
  context?: string
  /** 场景标签，传给后端用于日志 */
  scenario?: string
  /** skill-studio 专用：限制工具操作到该技能目录（沙箱） */
  skillId?: string
  placeholder?: string
  welcomeMessage?: string
  /** 快捷示例 chips */
  examples?: string[]
  /** 是否展开显示思考过程 */
  showThinking?: boolean
  /** 紧凑模式（用于侧边栏等窄场景） */
  compact?: boolean
  /** 预设背景色（可选） */
  bgColor?: string
  /** 组件高度（CSS 值），默认 100% */
  height?: string
  /** 初始消息列表 */
  initialMessages?: ChatMsg[]
  /** 是否允许在 apply card 上显示「应用」按钮 */
  applyable?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  examples: () => [],
  showThinking: false,
  compact: false,
  applyable: false,
})

// ── Emits ─────────────────────────────────────────────────────────────────
const emit = defineEmits<{
  (e: 'message', text: string, images: string[]): void
  (e: 'response', text: string): void
  (e: 'apply', data: Record<string, string>): void
  (e: 'session-change', sessionId: string): void  // fired when a new session is created
  (e: 'streaming-change', streaming: boolean): void  // fired when streaming starts/stops
}>()

// ── Types ─────────────────────────────────────────────────────────────────
interface ToolCallEntry {
  id: string
  name: string
  input?: string
  result?: string
  status: 'running' | 'done' | 'error'
  _expanded?: boolean
  duration?: string
  _startedAt?: number
  // agent_spawn specific: background task tracking
  taskId?: string
  taskStatus?: 'pending' | 'running' | 'done' | 'error' | 'killed'
}

interface PendingFile {
  name: string
  content: string  // text content
}

export interface ChatMsg {
  role: 'user' | 'assistant' | 'system'
  text: string
  images?: string[]
  thinking?: string
  toolCalls?: ToolCallEntry[]
  applyData?: Record<string, string>
  /** Quick-reply option chips parsed from AI response */
  options?: string[]
}

// ── State ─────────────────────────────────────────────────────────────────
const messages = ref<ChatMsg[]>(props.initialMessages ? [...props.initialMessages] : [])
const inputText = ref('')
const pendingImages = ref<string[]>([])
const pendingFiles = ref<PendingFile[]>([])
const streaming = ref(false)
watch(streaming, (v) => emit('streaming-change', v))
const streamText = ref('')
const streamThinking = ref('')
const streamToolCalls = ref<ToolCallEntry[]>([])  // active tool calls during streaming

// ── Background task tracking (agent_spawn) ─────────────────────────────────
// Maps toolCallId → background taskId for live status polling (current session)
const spawnedTaskMap = reactive<Map<string, string>>(new Map())
// Tasks re-attached after page reload (no tool call card, just status tracking)
const resumedTasks = ref<Array<{ id: string; label: string; status: string }>>([])
let taskPollTimer: ReturnType<typeof setInterval> | null = null

const runningTaskCount = computed(() => {
  let count = 0
  for (const msg of messages.value) {
    for (const tc of msg.toolCalls ?? []) {
      if (tc.taskId && tc.taskStatus && !['done','error','killed'].includes(tc.taskStatus)) count++
    }
  }
  count += resumedTasks.value.filter(t => !['done','error','killed'].includes(t.status)).length
  return count
})
const isDragOver  = ref(false)
let   _dragDepth  = 0  // counter to handle child element drag enter/leave
const copied = ref<number | null>(null)
const previewSrc = ref('')

// Session management — server-side persistent history
// Once set, subsequent requests use sessionId instead of sending full history[]
const currentSessionId = ref<string | undefined>(props.sessionId)
const historyLoading = ref(false)

const msgListRef = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()

// ── Computed ──────────────────────────────────────────────────────────────
const rootStyle = computed(() => ({
  height: props.height ?? '100%',
  '--bg': props.bgColor ?? 'transparent',
}))

// ── Helpers ───────────────────────────────────────────────────────────────
// ── agent_spawn task polling ────────────────────────────────────────────────

function startTaskPolling() {
  if (taskPollTimer) return
  taskPollTimer = setInterval(pollTasks, 3000)
}

async function pollTasks() {
  const allIdle = spawnedTaskMap.size === 0 &&
    resumedTasks.value.every(t => ['done','error','killed'].includes(t.status))
  if (allIdle) {
    if (taskPollTimer) { clearInterval(taskPollTimer); taskPollTimer = null }
    return
  }
  // Poll tool-call-linked tasks
  const doneIds: string[] = []
  let spawnedJustCompleted = false
  for (const [tcId, taskId] of spawnedTaskMap) {
    try {
      const res = await tasksApi.get(taskId)
      const info = res.data
      for (const msg of messages.value) {
        const tc = msg.toolCalls?.find(t => t.id === tcId)
        if (tc) {
          const wasRunning = !['done','error','killed'].includes(tc.taskStatus ?? '')
          tc.taskStatus = info.status as ToolCallEntry['taskStatus']
          if (['done','error','killed'].includes(info.status)) {
            doneIds.push(tcId)
            if (wasRunning) spawnedJustCompleted = true
          }
        }
      }
    } catch { doneIds.push(tcId); spawnedJustCompleted = true }
  }
  for (const id of doneIds) spawnedTaskMap.delete(id)

  // Poll resumed tasks (page-reload re-attached)
  let anyJustCompleted = false
  for (const rt of resumedTasks.value) {
    if (['done','error','killed'].includes(rt.status)) continue
    try {
      const res = await tasksApi.get(rt.id)
      const prevStatus = rt.status
      rt.status = res.data.status
      if (['done','error','killed'].includes(rt.status) && prevStatus !== rt.status) {
        anyJustCompleted = true
      }
    } catch { rt.status = 'error'; anyJustCompleted = true }
  }

  const stillRunning = spawnedTaskMap.size > 0 ||
    resumedTasks.value.some(t => !['done','error','killed'].includes(t.status))
  if (!stillRunning && taskPollTimer) {
    clearInterval(taskPollTimer); taskPollTimer = null
  }

  // When any task just completed, reload session messages to pick up the [后台任务完成] notification
  if ((anyJustCompleted || spawnedJustCompleted) && currentSessionId.value && !streaming.value) {
    const sid = currentSessionId.value
    setTimeout(async () => {
      if (currentSessionId.value !== sid) return // stale
      try {
        const res = await sessionsApi.get(props.agentId, sid)
        if (currentSessionId.value !== sid) return
        const parsed = res.data.messages ?? []
        const loaded: ChatMsg[] = []
        if (parsed.some((m: any) => m.isCompact || m.role === 'compaction')) {
          loaded.push({ role: 'system', text: '更早的内容已压缩' })
        }
        for (const m of parsed) {
          if (m.role === 'compaction') continue
          loaded.push({ role: m.role as 'user' | 'assistant', text: m.text })
        }
        messages.value = loaded
        scrollBottom()
      } catch {}
    }, 1500) // small delay to let server write the notification first
  }
}

onUnmounted(() => {
  if (taskPollTimer) { clearInterval(taskPollTimer); taskPollTimer = null }
})

// After page reload, re-attach any still-running tasks spawned in this session
async function reattachSessionTasks(sessionId: string) {
  try {
    const res = await tasksApi.list({ sessionId })
    const all = (res.data as any[])
    const active = all.filter(t => !['done','error','killed'].includes(t.status))
    const justDone = all.filter(t => ['done','error','killed'].includes(t.status))

    // If some tasks already completed but we don't have their notifications yet
    // (e.g. page was closed while subagent was running), do a reload to catch up.
    if (justDone.length > 0) {
      setTimeout(async () => {
        if (currentSessionId.value !== sessionId) return
        try {
          const r = await sessionsApi.get(props.agentId, sessionId)
          if (currentSessionId.value !== sessionId) return
          const parsed = r.data.messages ?? []
          const loaded: ChatMsg[] = []
          if (parsed.some((m: any) => m.isCompact || m.role === 'compaction')) {
            loaded.push({ role: 'system', text: '更早的内容已压缩' })
          }
          for (const m of parsed) {
            if (m.role === 'compaction') continue
            loaded.push({ role: m.role as 'user' | 'assistant', text: m.text })
          }
          messages.value = loaded
          scrollBottom()
        } catch {}
      }, 500)
    }

    if (active.length === 0) return
    resumedTasks.value = active.map(t => ({
      id: t.id,
      label: t.label || t.id.slice(0, 8),
      status: t.status,
    }))
    startTaskPolling()
  } catch { /* ignore */ }
}

function scrollBottom() {
  nextTick(() => {
    if (msgListRef.value) {
      msgListRef.value.scrollTop = msgListRef.value.scrollHeight
    }
  })
}

function autoGrow() {
  if (!inputRef.value) return
  inputRef.value.style.height = 'auto'
  inputRef.value.style.height = Math.min(inputRef.value.scrollHeight, 160) + 'px'
}

function fillInput(text: string) {
  inputText.value = text
  nextTick(() => inputRef.value?.focus())
}

function fmtJson(raw: string) {
  try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
}

function copyMsg(text: string) {
  navigator.clipboard?.writeText(text)
  const idx = messages.value.findIndex(m => m.text === text)
  copied.value = idx
  setTimeout(() => { copied.value = null }, 1500)
}

function retryMsg(idx: number) {
  for (let i = idx - 1; i >= 0; i--) {
    const m = messages.value[i]
    if (m && m.role === 'user') {
      const text = m.text
      const imgs = m.images ?? []
      messages.value.splice(i, messages.value.length - i)
      runChat(text, imgs)
      return
    }
  }
}

function previewImg(src: string) { previewSrc.value = src }

// ── Markdown renderer (lightweight) ──────────────────────────────────────
function renderMd(text: string): string {
  if (!text) return ''
  let html = text
    // Escape HTML first
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    // Code blocks (```lang\n...\n```)
    .replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) =>
      `<pre class="code-block${lang ? ' lang-' + lang : ''}"><code>${code}</code></pre>`)
    // Inline code
    .replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>')
    // Bold
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    // Italic
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    // Links
    .replace(/\[(.+?)\]\((https?:\/\/[^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
    // Headings
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    // Unordered list items
    .replace(/^[-*] (.+)$/gm, '<li>$1</li>')
    // Ordered list items
    .replace(/^\d+\. (.+)$/gm, '<li>$1</li>')
    // Wrap consecutive <li> in <ul>
    .replace(/(<li>[\s\S]+?<\/li>)(\n(?!<li>)|$)/g, '<ul>$1</ul>$2')
    // Newlines → <br> (outside block elements)
    .replace(/([^>])\n([^<])/g, '$1<br>$2')

  return html
}

// ── Apply data extractor (robust, multi-strategy) ────────────────────────
/**
 * Returns precomputed applyData from msg, OR tries to extract a JSON object
 * from the message text using multiple fallback strategies.
 * Returns null if nothing parseable found, or if not applyable mode.
 */
/**
 * 从 AI 回复中提取选项行，变成 quick-reply chips。
 * 检测模式：以 emoji 开头 + 空格 + 中文描述 的行（如 "🎙 想要更偏向英超"）
 */
function extractOptions(text: string): string[] {
  const lines = text.split('\n')
  const opts: string[] = []
  const emojiLineRe = /^([🎙😄🌐🛎📚🎨💼🤖⚽🎯✅❌🔥💡🎁🚀🌟💎🎪🎭🎬🎤]|[\u{1F300}-\u{1FFFF}]|[\u{2600}-\u{27BF}])\s+(.{4,40})/u
  for (const line of lines) {
    const trimmed = line.replace(/^[-*•]\s*/, '').trim()
    const m = trimmed.match(emojiLineRe)
    if (m) {
      // 去掉末尾标点
      const opt = trimmed.replace(/[：:。，,]$/, '').trim()
      if (opt.length >= 5) opts.push(opt)
    }
  }
  // 最多返回 5 个选项
  return opts.slice(0, 5)
}

/** 判断文本中是否含有 JSON 块（快速检测，不解析） */
function hasJsonBlock(text?: string): boolean {
  if (!text) return false
  return /\{[\s\S]{30,}\}/.test(text) &&
    (text.includes('"name"') || text.includes('"identity"') ||
     text.includes('"soul"') || text.includes('"IDENTITY"') || text.includes('"SOUL"'))
}

/** 用户手动触发解析并 emit apply */
function manualApply(msg: ChatMsg) {
  const data = tryExtractJson(msg.text)
  console.log('[AiChat] manualApply result:', data)
  if (data) {
    msg.applyData = data   // 缓存到消息，让卡片出现
    nextTick(() => emit('apply', data))
  } else {
    alert('未能从消息中提取到配置 JSON，请手动复制')
  }
}

/**
 * 用括号平衡计数从文本中提取第一个合法 JSON 对象字符串。
 * 比正则更可靠：能正确处理值中含 `}` 的情况。
 */
function extractBalancedJson(text: string, fromIndex = 0): { raw: string; end: number } | null {
  const start = text.indexOf('{', fromIndex)
  if (start === -1) return null
  let depth = 0
  let inStr = false
  let esc = false
  for (let i = start; i < text.length; i++) {
    const c = text[i]!
    if (esc) { esc = false; continue }
    if (c === '\\' && inStr) { esc = true; continue }
    if (c === '"') { inStr = !inStr; continue }
    if (!inStr) {
      if (c === '{') depth++
      else if (c === '}') { depth--; if (depth === 0) return { raw: text.slice(start, i + 1), end: i + 1 } }
    }
  }
  return null
}

function tryExtractJson(text: string): Record<string, string> | null {
  // Strategy 1: all ```(json)? ... ``` fence blocks — try LAST one first (most likely final config)
  const fenceRe = /```(?:json)?\s*\n?([\s\S]*?)```/g
  const fenceBlocks: string[] = []
  let fm: RegExpExecArray | null
  while ((fm = fenceRe.exec(text)) !== null) {
    const inner = (fm[1] ?? '').trim()
    if (inner.startsWith('{')) fenceBlocks.push(inner)
  }
  for (let i = fenceBlocks.length - 1; i >= 0; i--) {
    const raw = fenceBlocks[i]!
    const balanced = extractBalancedJson(raw)
    if (!balanced) continue
    const r = safeParse(balanced.raw) ?? safeParse(escapeJsonNewlines(balanced.raw))
    if (r) return r
  }

  // Strategy 2: balanced brace scan over full text — collect all, try last first
  const candidates: string[] = []
  let pos = 0
  while (pos < text.length) {
    const found = extractBalancedJson(text, pos)
    if (!found) break
    candidates.push(found.raw)
    pos = found.end
  }
  for (let i = candidates.length - 1; i >= 0; i--) {
    const raw = candidates[i]!
    const r = safeParse(raw) ?? safeParse(escapeJsonNewlines(raw))
    if (r) return r
  }
  return null
}

function safeParse(raw: string): Record<string, string> | null {
  try {
    const obj = JSON.parse(raw)
    if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
      // Only return if it has at least one string-valued known field
      const knownKeys = ['name','id','description','identity','soul','IDENTITY','SOUL','NAME','DESCRIPTION']
      if (Object.keys(obj).some(k => knownKeys.includes(k))) return obj
    }
  } catch { /* ignore */ }
  return null
}

function escapeJsonNewlines(raw: string): string {
  // Replace actual newlines inside JSON string values only
  // Split by quote pairs and only escape within strings
  let result = ''
  let inString = false
  let escape = false
  for (let i = 0; i < raw.length; i++) {
    const c = raw[i]!
    if (escape) { result += c; escape = false; continue }
    if (c === '\\' && inString) { result += c; escape = true; continue }
    if (c === '"') { inString = !inString; result += c; continue }
    if (inString && c === '\n') { result += '\\n'; continue }
    if (inString && c === '\r') { result += '\\r'; continue }
    result += c
  }
  return result
}

// ── Tool helpers ──────────────────────────────────────────────────────────
const TOOL_ICONS: Record<string, string> = {
  exec: '⚡', bash: '⚡',
  read: '📖', write: '✏️', edit: '✏️',
  web_search: '🌐', web_fetch: '🌐', browser: '🌐',
  agent_spawn: '🚀', agent_tasks: '📋', agent_kill: '🛑', agent_result: '📊',
  project_read: '📁', project_write: '📁', project_list: '📁', project_create: '📁', project_glob: '📁',
  memory_search: '🧠', memory_get: '🧠',
  image: '🖼️', tts: '🔊',
  cron: '⏱️',
}
function toolIcon(name: string): string {
  return TOOL_ICONS[name] ?? '⚙️'
}

function toolSummary(name: string, rawInput: string): string {
  try {
    const inp = JSON.parse(rawInput)
    if (name === 'exec' || name === 'bash') return (inp.command ?? '').slice(0, 60)
    if (name === 'read') return inp.file_path ?? inp.path ?? ''
    if (name === 'write') return (inp.file_path ?? inp.path ?? '') + (inp.content ? ` (${inp.content.length}B)` : '')
    if (name === 'edit') return inp.file_path ?? inp.path ?? ''
    if (name === 'web_search') return inp.query ?? ''
    if (name === 'web_fetch') return inp.url ?? ''
    if (name === 'agent_spawn') return `→ ${inp.agentId}: ${(inp.task ?? '').slice(0, 40)}`
    if (name === 'project_read') return inp.path ?? ''
    if (name === 'project_write') return inp.path ?? ''
    if (name === 'memory_search') return inp.query ?? ''
  } catch {}
  return ''
}

// ── File type helpers ──────────────────────────────────────────────────────
const TEXT_EXTS = new Set([
  'txt','md','markdown','js','ts','jsx','tsx','vue','go','py','rs','java','kt','swift',
  'html','css','scss','less','json','yaml','yml','toml','ini','cfg','env',
  'sh','bash','zsh','fish','ps1','bat','cmd','dockerfile','makefile',
  'sql','graphql','proto','xml','svg','gitignore','gitattributes',
])

function isTextFile(name: string): boolean {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return TEXT_EXTS.has(ext)
}

function fileTypeIcon(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  const icons: Record<string, string> = {
    js:'🟨', ts:'🔵', vue:'💚', go:'🐹', py:'🐍', rs:'🦀',
    html:'🌐', css:'🎨', json:'📋', md:'📝', sh:'⚡',
    sql:'🗄️', yaml:'⚙️', yml:'⚙️', dockerfile:'🐳',
  }
  return icons[ext] ?? '📄'
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / 1024 / 1024).toFixed(1)}MB`
}

// ── Image handling ────────────────────────────────────────────────────────
function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of Array.from(items)) {
    if (item.type.startsWith('image/')) {
      e.preventDefault()
      const file = item.getAsFile()
      if (file) readImageFile(file)
    }
  }
}

// Use depth counter to avoid flicker when dragging over child elements
function onDragEnter(e: DragEvent) {
  e.preventDefault()
  _dragDepth++
  isDragOver.value = true
}
function onDragLeave(e: DragEvent) {
  e.preventDefault()
  _dragDepth--
  if (_dragDepth <= 0) {
    _dragDepth = 0
    isDragOver.value = false
  }
}

function handleGlobalDrop(e: DragEvent) {
  _dragDepth = 0
  isDragOver.value = false
  const files = e.dataTransfer?.files
  if (!files) return
  for (const file of Array.from(files)) {
    if (file.type.startsWith('image/')) {
      readImageFile(file)
    } else if (isTextFile(file.name)) {
      readTextFile(file)
    }
    // else: unsupported, silently ignore
  }
}

function handleFileSelect(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files) return
  for (const file of Array.from(files)) {
    if (file.type.startsWith('image/')) {
      readImageFile(file)
    } else if (isTextFile(file.name)) {
      readTextFile(file)
    }
  }
  // Reset the input so the same file can be selected again
  ;(e.target as HTMLInputElement).value = ''
}

function readTextFile(file: File) {
  const reader = new FileReader()
  reader.onload = () => {
    if (typeof reader.result === 'string') {
      pendingFiles.value.push({ name: file.name, content: reader.result })
    }
  }
  reader.readAsText(file)
}

function readImageFile(file: File) {
  const reader = new FileReader()
  reader.onload = () => {
    if (typeof reader.result === 'string') pendingImages.value.push(reader.result)
  }
  reader.readAsDataURL(file)
}

function removeImage(i: number) { pendingImages.value.splice(i, 1) }

// ── Send ──────────────────────────────────────────────────────────────────
function send() {
  const text = inputText.value.trim()
  const imgs = [...pendingImages.value]
  const files = [...pendingFiles.value]
  if (!text && !imgs.length && !files.length) return
  if (streaming.value) return

  // Build final message text: append file contents as code blocks
  let finalText = text
  if (files.length > 0) {
    const fileBlocks = files.map(f => {
      const ext = f.name.split('.').pop() ?? 'text'
      return `\n\n📎 **${f.name}**\n\`\`\`${ext}\n${f.content}\n\`\`\``
    }).join('')
    finalText = (text ? text + fileBlocks : fileBlocks.trimStart())
  }

  inputText.value = ''
  pendingImages.value = []
  pendingFiles.value = []
  nextTick(() => {
    if (inputRef.value) { inputRef.value.style.height = 'auto' }
  })

  emit('message', finalText, imgs)
  runChat(finalText, imgs)
}

function runChat(text: string, imgs: string[], silent = false) {
  if (!silent) {
    messages.value.push({ role: 'user', text, images: imgs.length ? imgs : undefined })
    scrollBottom()
  }

  streaming.value = true
  streamText.value = ''
  streamThinking.value = ''
  streamToolCalls.value = []

  // Current assistant message being built
  const assistantMsg: ChatMsg = { role: 'assistant', text: '', toolCalls: [] }
  messages.value.push(assistantMsg)
  if (silent) scrollBottom()
  const msgIdx = messages.value.length - 1

  // Track active tool call
  let activeToolId = ''

  // Session-aware history:
  //   - sessionId exists → server already owns full history; never send history[] to avoid duplication.
  //   - no sessionId    → legacy mode: build client-side history (capped at 20 turns).
  let historyParam: { role: 'user' | 'assistant'; content: string }[] | undefined
  if (currentSessionId.value) {
    historyParam = undefined  // server owns history — explicit, not sent
  } else {
    const historyMsgs = messages.value
      .slice(0, -1)
      .filter(m => (m.role === 'user' || m.role === 'assistant') && m.text)
      .slice(-20)
      .map(m => ({ role: m.role as 'user' | 'assistant', content: m.text }))
    historyParam = historyMsgs.length > 0 ? historyMsgs : undefined
  }

  const params: ChatParams = {
    sessionId: currentSessionId.value,
    context: props.context,
    scenario: props.scenario,
    skillId: props.skillId,
    images: imgs.length ? imgs : undefined,
    history: historyParam,
  }

  chatSSE(props.agentId, text, (ev) => {
    switch (ev.type) {
      case 'thinking_delta':
        streamThinking.value += ev.text
        scrollBottom()
        break

      case 'text':
      case 'text_delta':
        streamText.value += ev.text
        scrollBottom()
        break

      case 'tool_call': {
        const tc: ToolCallEntry = {
          id: ev.tool_call?.id ?? String(Date.now()),
          name: ev.tool_call?.name ?? 'tool',
          input: ev.tool_call?.input ? JSON.stringify(ev.tool_call.input) : undefined,
          status: 'running',
          _startedAt: Date.now(),
          _expanded: false,
        }
        messages.value[msgIdx]!.toolCalls!.push(tc)
        streamToolCalls.value.push(tc)
        activeToolId = tc.id
        scrollBottom()
        break
      }

      case 'tool_result': {
        const tc = messages.value[msgIdx]!.toolCalls?.find(t => t.id === activeToolId)
        if (tc) {
          tc.result = ev.text
          tc.status = 'done'
          if (tc._startedAt) {
            const ms = Date.now() - tc._startedAt
            tc.duration = ms < 1000 ? `${ms}ms` : `${(ms/1000).toFixed(1)}s`
          }
          // agent_spawn: extract task ID from result and start polling
          if (tc.name === 'agent_spawn' && ev.text) {
            const m = ev.text.match(/任务\s*ID[：:]\s*([a-f0-9-]{8,})/i)
            if (m) {
              tc.taskId = m[1]
              tc.taskStatus = 'pending'
              spawnedTaskMap.set(activeToolId, m[1])
              startTaskPolling()
            }
          }
          // Sync into streamToolCalls
          const stc = streamToolCalls.value.find(t => t.id === activeToolId)
          if (stc) { stc.result = tc.result; stc.status = 'done'; stc.duration = tc.duration }
        }
        scrollBottom()
        break
      }

      case 'done':
      case 'error': {
        // Capture server-side sessionId for subsequent requests
        if (ev.type === 'done' && ev.sessionId) {
          const isNew = !currentSessionId.value
          currentSessionId.value = ev.sessionId
          if (isNew) emit('session-change', ev.sessionId)
        }

        const cur = messages.value[msgIdx]!
        cur.text = streamText.value
        cur.thinking = streamThinking.value || undefined

        if (props.applyable) {
          const extracted = tryExtractJson(streamText.value)
          if (extracted) {
            cur.applyData = extracted
            console.log('[AiChat] applyData extracted:', extracted)
          } else {
            console.log('[AiChat] no applyData found in text length:', streamText.value.length)
          }
        }

        // Extract quick-reply options from the response
        const opts = extractOptions(streamText.value)
        if (opts.length >= 2) cur.options = opts

        if (ev.type === 'error') {
          cur.text = `[错误] ${ev.error}`
          const tc = cur.toolCalls?.find(t => t.status === 'running')
          if (tc) tc.status = 'error'
        }

        streaming.value = false
        streamText.value = ''
        streamThinking.value = ''
        streamToolCalls.value = []
        emit('response', cur.text)
        scrollBottom()
        break
      }
    }
  }, params)
}

// ── Public API (expose for parent use) ───────────────────────────────────
function clearMessages() { messages.value = [] }
function appendMessage(msg: ChatMsg) { messages.value.push(msg); scrollBottom() }

/** Resume an existing session — immediately loads history from server */
async function resumeSession(sessionId: string) {
  currentSessionId.value = sessionId
  messages.value = []
  historyLoading.value = true
  // Snapshot the sessionId at call time so we can detect stale closures
  const mySessionId = sessionId
  try {
    const res = await sessionsApi.get(props.agentId, sessionId)
    // Guard: user may have switched sessions while waiting for response
    if (currentSessionId.value !== mySessionId) return
    const parsed = res.data.messages ?? []
    const loaded: ChatMsg[] = []
    // Insert a compaction marker if any compaction entry exists
    const hasCompaction = parsed.some(m => m.isCompact || m.role === 'compaction')
    if (hasCompaction) {
      loaded.push({ role: 'system', text: '更早的内容已压缩' })
    }
    for (const m of parsed) {
      if (m.role === 'compaction') continue  // skip raw compaction entries
      loaded.push({
        role: m.role as 'user' | 'assistant',
        text: m.text,
      })
    }
    messages.value = loaded
    scrollBottom()
    // Re-attach any still-running background tasks from this session
    reattachSessionTasks(sessionId)
    // Check if a generation is still running in the background → reconnect
    reconnectIfGenerating(sessionId)
  } catch (e: any) {
    // 404 = 新 session，正常情况，直接留空
    if (e?.response?.status === 404) {
      messages.value = []
    } else {
      console.error('[AiChat] resumeSession failed', e)
      messages.value = [{ role: 'system', text: '历史加载失败，继续对话仍可接续' }]
    }
  } finally {
    historyLoading.value = false
  }
}

/**
 * Check if a session has an in-progress generation in the background.
 * If so, attach to the broadcaster and show the streaming response.
 * Called automatically on page load / tab refocus when a sessionId is known.
 */
async function reconnectIfGenerating(sessionId: string) {
  if (streaming.value) return // already streaming

  const status = await getSessionStatus(props.agentId, sessionId)

  // Stale-closure guard: user may have switched sessions while we were waiting for status.
  // If currentSessionId changed, our update would overwrite the wrong session's UI.
  if (currentSessionId.value !== sessionId) return

  if (!status.hasWorker) return // no active worker at all

  if (status.status !== 'generating') {
    // Worker exists but is idle — generation just finished (or just became idle).
    // Reload history once now, then again after a short delay in case the runner
    // saved to disk just as we were checking (race between AppendMessage and IsBusy).
    const doReload = async () => {
      try {
        const res = await sessionsApi.get(props.agentId, sessionId)
        if (currentSessionId.value !== sessionId) return
        const parsed = res.data.messages ?? []
        const loaded: ChatMsg[] = []
        if (parsed.some((m: any) => m.isCompact || m.role === 'compaction')) {
          loaded.push({ role: 'system', text: '更早的内容已压缩' })
        }
        for (const m of parsed) {
          if (m.role === 'compaction') continue
          loaded.push({ role: m.role as 'user' | 'assistant', text: m.text })
        }
        messages.value = loaded
        scrollBottom()
      } catch {}
    }
    await doReload()
    // Second reload after 1s — catches the case where the runner saved just after our first reload
    setTimeout(async () => {
      if (currentSessionId.value !== sessionId) return
      await doReload()
    }, 1000)
    return
  }

  // Worker is actively generating — subscribe to live stream.
  // Guard: only proceed if still on the same session
  if (currentSessionId.value !== sessionId) return

  streaming.value = true
  streamText.value = ''
  streamThinking.value = ''
  streamToolCalls.value = []

  const assistantMsg: ChatMsg = { role: 'assistant', text: '', toolCalls: [] }
  messages.value.push(assistantMsg)
  const msgIdx = messages.value.length - 1
  scrollBottom()

  let activeToolId = ''

  const ctrl = resumeSSE(props.agentId, sessionId, (ev: any) => {
    switch (ev.type) {
      case 'idle':
        // Generation already finished before we connected — nothing to do
        messages.value.splice(msgIdx, 1) // remove empty bubble
        streaming.value = false
        break

      case 'thinking_delta':
        streamThinking.value += ev.text
        scrollBottom()
        break

      case 'text':
      case 'text_delta':
        streamText.value += ev.text
        scrollBottom()
        break

      case 'tool_call': {
        const tc: ToolCallEntry = {
          id: ev.tool_call?.id ?? String(Date.now()),
          name: ev.tool_call?.name ?? 'tool',
          input: ev.tool_call?.input ? JSON.stringify(ev.tool_call.input) : undefined,
          status: 'running',
          _startedAt: Date.now(),
          _expanded: false,
        }
        messages.value[msgIdx]!.toolCalls!.push(tc)
        streamToolCalls.value.push(tc)
        activeToolId = tc.id
        scrollBottom()
        break
      }

      case 'tool_result': {
        const tc = messages.value[msgIdx]!.toolCalls?.find(t => t.id === activeToolId)
        if (tc) {
          tc.result = ev.text
          tc.status = 'done'
          const stc = streamToolCalls.value.find(t => t.id === activeToolId)
          if (stc) { stc.result = tc.result; stc.status = 'done' }
        }
        scrollBottom()
        break
      }

      case 'done':
      case 'error': {
        if (ev.type === 'done' && ev.sessionId) {
          const isNew = !currentSessionId.value
          currentSessionId.value = ev.sessionId
          if (isNew) emit('session-change', ev.sessionId)
        }
        const cur = messages.value[msgIdx]!
        cur.text = streamText.value
        cur.thinking = streamThinking.value || undefined
        if (ev.type === 'error') cur.text = `[错误] ${ev.error}`
        streaming.value = false
        streamText.value = ''
        streamThinking.value = ''
        streamToolCalls.value = []
        scrollBottom()
        break
      }
    }
  })

  // Store abort controller so it can be cancelled if needed
  // (reuse the existing abortCtrl pattern if present, otherwise just store locally)
  onUnmounted(() => ctrl.abort())
}

/** Start a brand new session (clears sessionId + messages) */
function startNewSession() {
  currentSessionId.value = undefined
  messages.value = []
}
function sendText(text: string) { fillInput(text); nextTick(send) }

/** 静默发送：只显示 AI 回复，不在聊天中添加用户消息（用于自动触发场景） */
function sendSilent(text: string) { runChat(text, [], true) }

defineExpose({ clearMessages, appendMessage, sendText, sendSilent, fillInput, messages, streaming, currentSessionId, resumeSession, startNewSession })

// ── Init ─────────────────────────────────────────────────────────────────
onMounted(() => {
  scrollBottom()
  // On page load: if a session is already active, check for ongoing background generation
  if (currentSessionId.value) {
    reconnectIfGenerating(currentSessionId.value)
  }
})
</script>

<style scoped>
.ai-chat {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg, transparent);
  container-type: inline-size;
  font-size: 14px;
}

/* ── Messages ── */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  scroll-behavior: smooth;
}

.chat-empty {
  margin: auto;
  text-align: center;
  color: #909399;
}
.welcome-msg { font-size: 15px; margin-bottom: 12px; color: #606266; }
.examples { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin-top: 8px; }
.example-chip {
  padding: 6px 14px;
  background: #fff;
  border: 1px solid #dcdfe6;
  border-radius: 16px;
  cursor: pointer;
  color: #409eff;
  transition: all .15s;
  font-size: 13px;
}
.example-chip:hover { background: #ecf5ff; border-color: #409eff; }

/* ── Message rows ── */
.msg-row { display: flex; }
.msg-row.user  { justify-content: flex-end; }
.msg-row.assistant { justify-content: flex-start; }
.msg-row.system { justify-content: center; }
.msg-col { display: flex; flex-direction: column; gap: 6px; max-width: 82%; }

.msg-system {
  background: #fdf6ec;
  color: #e6a23c;
  border-radius: 6px;
  padding: 4px 12px;
  font-size: 12px;
}

/* ── Bubbles ── */
.msg-bubble {
  position: relative;
  padding: 10px 14px;
  border-radius: 14px;
  line-height: 1.65;
  word-break: break-word;
}
.msg-bubble.user {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
  max-width: 72cqi; /* container query units */
}
.msg-bubble.assistant {
  background: #fff;
  color: #303133;
  border-bottom-left-radius: 4px;
  box-shadow: 0 1px 4px rgba(0,0,0,.08);
}

/* narrow containers → full width */
@container (max-width: 480px) {
  .msg-bubble { max-width: 92cqi !important; }
  .msg-col    { max-width: 94%; }
}

/* ── Thinking ── */
.thinking-block {
  background: #f8f9fa;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  overflow: hidden;
}
.thinking-summary {
  padding: 6px 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #606266;
  user-select: none;
  list-style: none;
}
.thinking-summary::-webkit-details-marker { display: none; }
.thinking-icon { font-size: 14px; vertical-align: -2px; }
.thinking-len  { margin-left: auto; color: #c0c4cc; }
.thinking-content {
  padding: 8px 12px;
  font-size: 12px;
  white-space: pre-wrap;
  color: #606266;
  max-height: 200px;
  overflow-y: auto;
  border-top: 1px solid #f0f0f0;
  margin: 0;
}

/* ── Tool timeline（新设计）── */
.tool-timeline {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-width: 82%;
  margin-bottom: 4px;
}
.tool-step {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: border-color .15s, background .15s;
  user-select: none;
}
.tool-step:hover { border-color: #cbd5e1; background: #f1f5f9; }
.tool-step.running { border-color: #fbbf24; background: #fffbeb; }
.tool-step.done    { border-color: #86efac; }
.tool-step.error   { border-color: #fca5a5; background: #fff5f5; }

.tool-step-header {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 10px;
  font-size: 12px;
  min-height: 30px;
}
.tool-step-dot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  flex-shrink: 0;
  background: #e2e8f0;
  color: #64748b;
}
.tool-step-dot.running { background: #fef3c7; color: #d97706; }
.tool-step-dot.done    { background: #dcfce7; color: #16a34a; }
.tool-step-dot.error   { background: #fee2e2; color: #dc2626; }
.tool-spin { display: inline-block; animation: spin .8s linear infinite; }
.tool-step-icon { font-size: 13px; flex-shrink: 0; }
.tool-step-name { font-family: monospace; font-size: 12px; font-weight: 600; color: #334155; }
.tool-step-summary {
  font-size: 11px;
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 300px;
}
.tool-step-flex { flex: 1; }
.tool-step-dur  { font-size: 11px; color: #94a3b8; font-family: monospace; flex-shrink: 0; }

/* ── agent_spawn task badge ── */
.task-badge {
  display: inline-flex; align-items: center; gap: 3px;
  font-size: 11px; padding: 1px 7px; border-radius: 10px;
  font-weight: 600; flex-shrink: 0; white-space: nowrap;
  background: #f1f5f9; color: #64748b;
}
.task-badge.pending  { background: #fef9c3; color: #a16207; }
.task-badge.running  { background: #dbeafe; color: #1d4ed8; }
.task-badge.done     { background: #dcfce7; color: #15803d; }
.task-badge.error    { background: #fee2e2; color: #b91c1c; }
.task-badge.killed   { background: #f1f5f9; color: #475569; }

/* ── Running tasks banner ── */
.running-tasks-banner {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  padding: 8px 16px; background: #eff6ff; border-bottom: 1px solid #bfdbfe;
  font-size: 12px; color: #1d4ed8; font-weight: 500; flex-shrink: 0;
}
.resumed-list { display: flex; gap: 6px; flex-wrap: wrap; margin-left: 4px; }
.resumed-chip {
  padding: 1px 8px; border-radius: 10px; font-size: 11px; font-weight: 600;
  background: #dbeafe; color: #1d4ed8;
}
.resumed-chip.running { background: #dbeafe; color: #1d4ed8; }
.resumed-chip.pending  { background: #fef9c3; color: #a16207; }
.running-dot {
  width: 8px; height: 8px; border-radius: 50%; background: #3b82f6;
  flex-shrink: 0;
  animation: pulse-dot 1.5s ease-in-out infinite;
}
@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.75); }
}
.tool-step-chevron { font-size: 9px; color: #94a3b8; flex-shrink: 0; }

.tool-step-body {
  border-top: 1px solid #e2e8f0;
  padding: 8px 10px;
  cursor: default;
}
.tool-section { margin-bottom: 6px; }
.tool-label  { font-size: 10px; color: #94a3b8; margin-bottom: 3px; text-transform: uppercase; letter-spacing: .5px; font-weight: 600; }
.tool-pre {
  margin: 0;
  font-size: 11px;
  background: #0f172a;
  color: #94a3b8;
  border-radius: 6px;
  padding: 8px 10px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
  font-family: 'Menlo', 'Monaco', monospace;
}
.tool-pre.result { color: #86efac; }

/* ── Markdown ── */
.msg-text :deep(pre.code-block) {
  background: #1e1e2e;
  color: #cdd6f4;
  border-radius: 8px;
  padding: 12px;
  overflow-x: auto;
  font-size: 13px;
  margin: 8px 0;
}
.msg-text :deep(.inline-code) {
  background: rgba(0,0,0,.08);
  padding: 1px 5px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 0.9em;
}
.msg-bubble.user .msg-text :deep(.inline-code) { background: rgba(255,255,255,.25); }
.msg-text :deep(ul)  { margin: 4px 0 4px 16px; }
.msg-text :deep(li)  { margin: 2px 0; }
.msg-text :deep(h1, h2, h3) { margin: 8px 0 4px; }
.msg-text :deep(a)   { color: #409eff; }

/* ── Apply card ── */
.apply-card {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid #f0f0f0;
}
.apply-preview { margin-bottom: 8px; }
.apply-row { display: flex; gap: 8px; font-size: 12px; padding: 2px 0; }
.apply-key { color: #909399; flex-shrink: 0; min-width: 70px; }
.apply-val { color: #303133; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.apply-btn {
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 5px 14px;
  cursor: pointer;
  font-size: 13px;
  transition: background .15s;
}
.apply-btn:hover { background: #337ecc; }

/* ── Msg actions ── */
.msg-actions {
  display: flex;
  gap: 4px;
  margin-top: 6px;
  opacity: 0;
  transition: opacity .2s;
}
.msg-bubble.assistant:hover .msg-actions { opacity: 1; }
.act-btn {
  background: none;
  border: 1px solid #e4e7ed;
  border-radius: 5px;
  padding: 2px 8px;
  cursor: pointer;
  font-size: 12px;
  color: #606266;
  transition: all .15s;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.act-btn:hover { background: #f0f2f5; color: #303133; }
.apply-manual-btn { color: #409eff !important; border-color: #b3d8ff !important; font-weight: 500; }

/* ── Option chips ── */
.option-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  margin-top: 8px;
  padding-left: 2px;
}
.option-chip {
  padding: 6px 14px;
  background: #fff;
  border: 1.5px solid #d0e8ff;
  border-radius: 20px;
  cursor: pointer;
  font-size: 13px;
  color: #409eff;
  transition: all .15s;
  text-align: left;
  line-height: 1.4;
}
.option-chip:hover {
  background: #ecf5ff;
  border-color: #409eff;
  transform: translateY(-1px);
  box-shadow: 0 2px 6px rgba(64,158,255,.15);
}

/* ── Images in user msg ── */
.msg-images { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 6px; }
.msg-img {
  max-width: 160px;
  max-height: 120px;
  border-radius: 6px;
  cursor: zoom-in;
  object-fit: cover;
}

/* ── Preview overlay ── */
.img-preview-mask {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  cursor: zoom-out;
}
.img-preview-full { max-width: 90vw; max-height: 90vh; border-radius: 8px; }

/* ── Streaming ── */
.typing-dots { display: flex; gap: 4px; align-items: center; padding: 2px 0; }
.typing-dots span {
  width: 6px; height: 6px;
  border-radius: 50%;
  background: #c0c4cc;
  animation: bounce 1.2s infinite;
}
.typing-dots span:nth-child(2) { animation-delay: .2s; }
.typing-dots span:nth-child(3) { animation-delay: .4s; }
@keyframes bounce { 0%, 80%, 100% { transform: scale(0.7); } 40% { transform: scale(1.1); } }

@keyframes blink { 50% { opacity: 0; } }
.blink { animation: blink .8s infinite; font-size: 12px; }

/* ── Input area ── */
.chat-input-area {
  flex-shrink: 0;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  padding: 10px 12px;
}

.attachments-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}
/* ── Drag overlay ── */
.drag-overlay {
  position: absolute;
  inset: 0;
  z-index: 100;
  background: rgba(15, 23, 42, 0.65);  /* 深色半透明，醒目 */
  border: 2px dashed #60a5fa;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(4px);
  pointer-events: none;  /* 不拦截事件，由父层统一处理 drop */
}
.drag-overlay-content {
  text-align: center;
  pointer-events: none;
}
.drag-overlay-icon   { font-size: 48px; margin-bottom: 12px; }
.drag-overlay-title  { font-size: 18px; font-weight: 700; color: #fff; margin-bottom: 6px; text-shadow: 0 1px 4px rgba(0,0,0,.4); }
.drag-overlay-hint   { font-size: 13px; color: rgba(255,255,255,.7); }
.drag-fade-enter-active, .drag-fade-leave-active { transition: opacity .15s; }
.drag-fade-enter-from, .drag-fade-leave-to { opacity: 0; }

.ai-chat { position: relative; }

/* ── Attachments ── */
.attach-thumb { position: relative; display: inline-block; }
.attach-thumb img { width: 48px; height: 48px; object-fit: cover; border-radius: 6px; border: 1px solid #e4e7ed; }
.remove-attach {
  position: absolute;
  top: -5px; right: -5px;
  width: 16px; height: 16px;
  background: #f56c6c;
  color: #fff;
  border: none;
  border-radius: 50%;
  cursor: pointer;
  font-size: 11px;
  line-height: 1;
  display: flex; align-items: center; justify-content: center;
  padding: 0;
}

.attach-file-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 20px;
  padding: 4px 10px 4px 8px;
  font-size: 12px;
}
.attach-file-icon  { font-size: 14px; }
.attach-file-name  { color: #334155; font-weight: 500; max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attach-file-size  { color: #94a3b8; font-size: 11px; }
.attach-file-remove {
  background: none;
  border: none;
  color: #94a3b8;
  cursor: pointer;
  font-size: 14px;
  padding: 0;
  line-height: 1;
  margin-left: 2px;
}
.attach-file-remove:hover { color: #f56c6c; }

.input-row { display: flex; gap: 8px; align-items: flex-end; }
.textarea-wrap { flex: 1; }
.chat-textarea {
  width: 100%;
  resize: none;
  border: 1px solid #dcdfe6;
  border-radius: 10px;
  padding: 9px 12px;
  font-size: 14px;
  font-family: inherit;
  color: #303133;
  background: #f5f7fa;
  outline: none;
  transition: border-color .2s;
  box-sizing: border-box;
  line-height: 1.5;
  overflow-y: hidden;
}
.chat-textarea:focus { border-color: #409eff; background: #fff; }
.chat-textarea:disabled { opacity: .6; cursor: not-allowed; }

.input-actions { display: flex; flex-direction: column; gap: 4px; }
.icon-btn {
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  border-radius: 8px;
  cursor: pointer;
  font-size: 16px;
  transition: background .15s;
}
.icon-btn:hover { background: #f0f2f5; }
.send-btn {
  width: 36px; height: 36px;
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 10px;
  cursor: pointer;
  font-size: 18px;
  display: flex; align-items: center; justify-content: center;
  transition: background .15s;
  flex-shrink: 0;
}
.send-btn:hover:not(:disabled) { background: #337ecc; }
.send-btn:disabled { background: #c0c4cc; cursor: not-allowed; }

.input-hint { font-size: 11px; color: #c0c4cc; margin-top: 5px; text-align: right; }

/* ── Spinner ── */
@keyframes spin { to { transform: rotate(360deg); } }
.spinner {
  width: 14px; height: 14px;
  border: 2px solid rgba(255,255,255,.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin .6s linear infinite;
  display: inline-block;
}

/* ── History loading ── */
.history-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px 0;
  color: #909399;
}
.history-loading-dots {
  display: flex;
  gap: 5px;
}
.history-loading-dots span {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: #c0c4cc;
  animation: bounce 1.2s infinite;
}
.history-loading-dots span:nth-child(2) { animation-delay: .2s; }
.history-loading-dots span:nth-child(3) { animation-delay: .4s; }
.history-loading-text { font-size: 13px; }

/* ── Compact mode ── */
.compact .chat-messages { padding: 10px; gap: 10px; }
.compact .msg-bubble { padding: 8px 11px; font-size: 13px; }
.compact .chat-input-area { padding: 8px; }
.compact .chat-textarea { font-size: 13px; padding: 7px 10px; }
.compact .input-hint { display: none; }
</style>
