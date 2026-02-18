<template>
  <div class="ai-chat" :class="{ compact, 'has-bg': bgColor }" :style="rootStyle">

    <!-- ── 消息列表 ── -->
    <div class="chat-messages" ref="msgListRef">
      <!-- 欢迎语 / 空状态 -->
      <div v-if="!messages.length" class="chat-empty">
        <div v-if="welcomeMessage" class="welcome-msg">{{ welcomeMessage }}</div>
        <div v-if="examples.length" class="examples">
          <div v-for="(ex, i) in examples" :key="i"
            class="example-chip" @click="fillInput(ex)">{{ ex }}</div>
        </div>
      </div>

      <template v-for="(msg, i) in messages" :key="i">

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
                <span class="thinking-icon">💭</span> 思考过程
                <span class="thinking-len">{{ msg.thinking.length }} 字符</span>
              </summary>
              <pre class="thinking-content">{{ msg.thinking }}</pre>
            </details>

            <!-- 消息气泡 -->
            <div class="msg-bubble assistant">
              <!-- Tool calls -->
              <div v-for="(tc, ti) in msg.toolCalls" :key="ti" class="tool-call-block">
                <details class="tool-details">
                  <summary class="tool-summary">
                    <span class="tool-icon">🔧</span>
                    <span class="tool-name">{{ tc.name }}</span>
                    <span v-if="tc.status === 'running'" class="tool-status running">运行中…</span>
                    <span v-else-if="tc.status === 'done'" class="tool-status done">完成</span>
                    <span v-else-if="tc.status === 'error'" class="tool-status error">失败</span>
                  </summary>
                  <div class="tool-body">
                    <div v-if="tc.input" class="tool-section">
                      <div class="tool-label">输入</div>
                      <pre class="tool-pre">{{ fmtJson(tc.input) }}</pre>
                    </div>
                    <div v-if="tc.result" class="tool-section">
                      <div class="tool-label">输出</div>
                      <pre class="tool-pre result">{{ tc.result.slice(0, 800) }}{{ tc.result.length > 800 ? '\n…(截断)' : '' }}</pre>
                    </div>
                  </div>
                </details>
              </div>

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
                  {{ copied === i ? '✓' : '⎘' }}
                </button>
                <button class="act-btn" @click="retryMsg(i)" title="重试">↺</button>
                <!-- 手动触发：当自动解析失败时可手动点 -->
                <button v-if="props.applyable && !msg.applyData && hasJsonBlock(msg.text)"
                  class="act-btn apply-manual-btn"
                  @click="manualApply(msg)"
                  title="检测到配置 JSON，点击应用">
                  ⚙ 应用配置
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
              <span class="thinking-icon">💭</span> 思考中…
            </summary>
            <pre class="thinking-content">{{ streamThinking }}<span class="blink">▊</span></pre>
          </details>
          <div class="msg-bubble assistant">
            <!-- 打字指示器 or 流式文字 -->
            <div v-if="!streamText" class="typing-dots">
              <span /><span /><span />
            </div>
            <div v-else class="msg-text" v-html="renderMd(streamText)" />
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
      <!-- 图片附件预览条 -->
      <div v-if="pendingImages.length" class="attachments-bar">
        <div v-for="(src, i) in pendingImages" :key="i" class="attach-thumb">
          <img :src="src" />
          <button class="remove-attach" @click="removeImage(i)">×</button>
        </div>
      </div>

      <div class="input-row">
        <div class="textarea-wrap">
          <textarea
            ref="inputRef"
            v-model="inputText"
            :placeholder="placeholder || '输入消息… (Ctrl+Enter 发送)'"
            :disabled="streaming"
            rows="1"
            class="chat-textarea"
            @keydown.enter.ctrl.prevent="send"
            @keydown.enter.meta.prevent="send"
            @paste="handlePaste"
            @input="autoGrow"
            @dragover.prevent
            @drop.prevent="handleDrop"
          />
        </div>
        <div class="input-actions">
          <!-- 图片上传 -->
          <label class="icon-btn" title="上传图片">
            📎
            <input type="file" accept="image/*" multiple hidden @change="handleFileSelect" />
          </label>
          <!-- 发送 -->
          <button class="send-btn" :disabled="streaming || (!inputText.trim() && !pendingImages.length)"
            @click="send">
            <span v-if="streaming" class="spinner" />
            <span v-else>↑</span>
          </button>
        </div>
      </div>

      <div class="input-hint">Ctrl+Enter 发送 · 支持粘贴图片</div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { chatSSE, type ChatParams } from '../api'

// ── Props ─────────────────────────────────────────────────────────────────
interface Props {
  agentId: string
  /** 指定要续接的 session ID（可选），不传则自动新建 */
  sessionId?: string
  /** 注入到系统提示的额外上下文（页面场景、表单状态等） */
  context?: string
  /** 场景标签，传给后端用于日志 */
  scenario?: string
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
}>()

// ── Types ─────────────────────────────────────────────────────────────────
interface ToolCallEntry {
  id: string
  name: string
  input?: string
  result?: string
  status: 'running' | 'done' | 'error'
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
const streaming = ref(false)
const streamText = ref('')
const streamThinking = ref('')
const copied = ref<number | null>(null)
const previewSrc = ref('')

// Session management — server-side persistent history
// Once set, subsequent requests use sessionId instead of sending full history[]
const currentSessionId = ref<string | undefined>(props.sessionId)

const msgListRef = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()

// ── Computed ──────────────────────────────────────────────────────────────
const rootStyle = computed(() => ({
  height: props.height ?? '100%',
  '--bg': props.bgColor ?? 'transparent',
}))

// ── Helpers ───────────────────────────────────────────────────────────────
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

function tryExtractJson(text: string): Record<string, string> | null {
  // Strategy 1: standard ```json ... ``` block
  const fenceMatch = text.match(/```(?:json)?\s*(\{[\s\S]*?\})\s*```/)
  if (fenceMatch?.[1]) {
    const r = safeParse(fenceMatch[1])
    if (r) return r
    // Strategy 3: same block but escape raw newlines inside string values
    const escaped = escapeJsonNewlines(fenceMatch[1])
    const r2 = safeParse(escaped)
    if (r2) return r2
  }
  // Strategy 2: last standalone {...} block in the text (handles no fence)
  const blockMatches = [...text.matchAll(/(\{[^{}]{20,}\})/g)]
  for (let i = blockMatches.length - 1; i >= 0; i--) {
    const raw = blockMatches[i]?.[1]
    if (!raw) continue
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

function handleDrop(e: DragEvent) {
  const files = e.dataTransfer?.files
  if (!files) return
  for (const file of Array.from(files)) {
    if (file.type.startsWith('image/')) readImageFile(file)
  }
}

function handleFileSelect(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files) return
  for (const file of Array.from(files)) readImageFile(file)
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
  if (!text && !imgs.length) return
  if (streaming.value) return

  inputText.value = ''
  pendingImages.value = []
  nextTick(() => {
    if (inputRef.value) { inputRef.value.style.height = 'auto' }
  })

  emit('message', text, imgs)
  runChat(text, imgs)
}

function runChat(text: string, imgs: string[]) {
  messages.value.push({ role: 'user', text, images: imgs.length ? imgs : undefined })
  scrollBottom()

  streaming.value = true
  streamText.value = ''
  streamThinking.value = ''

  // Current assistant message being built
  const assistantMsg: ChatMsg = { role: 'assistant', text: '', toolCalls: [] }
  messages.value.push(assistantMsg)
  const msgIdx = messages.value.length - 1

  // Track active tool call
  let activeToolId = ''

  // Session-aware history: if we have a server-side sessionId, the server owns history.
  // Otherwise fall back to client-side history (legacy, capped at 20 turns).
  let historyParam: { role: 'user' | 'assistant'; content: string }[] | undefined
  if (!currentSessionId.value) {
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
        }
        messages.value[msgIdx]!.toolCalls!.push(tc)
        activeToolId = tc.id
        scrollBottom()
        break
      }

      case 'tool_result': {
        const tc = messages.value[msgIdx]!.toolCalls?.find(t => t.id === activeToolId)
        if (tc) { tc.result = ev.text; tc.status = 'done' }
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
          cur.text = `❌ ${ev.error}`
          const tc = cur.toolCalls?.find(t => t.status === 'running')
          if (tc) tc.status = 'error'
        }

        streaming.value = false
        streamText.value = ''
        streamThinking.value = ''
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

/** Resume an existing session (clears messages, loads from server on next send) */
function resumeSession(sessionId: string) {
  currentSessionId.value = sessionId
  messages.value = []
}

/** Start a brand new session (clears sessionId + messages) */
function startNewSession() {
  currentSessionId.value = undefined
  messages.value = []
}
function sendText(text: string) { fillInput(text); nextTick(send) }

defineExpose({ clearMessages, appendMessage, sendText, messages, currentSessionId, resumeSession, startNewSession })

// ── Init ─────────────────────────────────────────────────────────────────
onMounted(() => {
  scrollBottom()
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
.thinking-icon { font-size: 14px; }
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

/* ── Tool calls ── */
.tool-call-block { margin-bottom: 6px; }
.tool-details { background: #fafafa; border: 1px solid #ebeef5; border-radius: 6px; overflow: hidden; }
.tool-summary {
  padding: 5px 10px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  list-style: none;
}
.tool-summary::-webkit-details-marker { display: none; }
.tool-icon  { font-size: 13px; }
.tool-name  { font-weight: 500; color: #303133; flex: 1; }
.tool-status.running { color: #e6a23c; }
.tool-status.done    { color: #67c23a; }
.tool-status.error   { color: #f56c6c; }
.tool-body   { padding: 8px 10px; border-top: 1px solid #f0f0f0; }
.tool-section { margin-bottom: 6px; }
.tool-label  { font-size: 11px; color: #909399; margin-bottom: 3px; text-transform: uppercase; }
.tool-pre {
  margin: 0;
  font-size: 11px;
  background: #f5f7fa;
  border-radius: 4px;
  padding: 6px 8px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 150px;
  overflow-y: auto;
  color: #303133;
}
.tool-pre.result { color: #067065; }

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

/* ── Compact mode ── */
.compact .chat-messages { padding: 10px; gap: 10px; }
.compact .msg-bubble { padding: 8px 11px; font-size: 13px; }
.compact .chat-input-area { padding: 8px; }
.compact .chat-textarea { font-size: 13px; padding: 7px 10px; }
.compact .input-hint { display: none; }
</style>
