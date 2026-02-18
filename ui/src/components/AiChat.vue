<!--
  AiChat — 统一 AI 对话组件
  支持：流式输出 · 多模态输入(图片粘贴/拖拽/上传) · 思考过程展开 · 工具调用展示 · 多尺寸自适应
  调用方通过 props 传入 agentId + context/scenario 完成定制
-->
<template>
  <div class="ai-chat" :class="{ compact, 'has-thinking': showThinking }" :style="{ height }">

    <!-- ─── 消息列表 ─── -->
    <div class="chat-messages" ref="messagesRef">
      <!-- 欢迎语 -->
      <div v-if="!messages.length && welcomeMessage" class="welcome-msg">
        <div class="welcome-icon">🤖</div>
        <div class="welcome-text" v-html="renderMd(welcomeMessage)" />
      </div>

      <!-- 示例 chips（仅无消息时且有 examples 时） -->
      <div v-if="!messages.length && examples.length" class="examples-wrap">
        <div v-for="ex in examples" :key="ex" class="example-chip" @click="fillInput(ex)">
          {{ ex }}
        </div>
      </div>

      <!-- 消息列表 -->
      <template v-for="(msg, i) in messages" :key="i">
        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="msg-row user">
          <div class="msg-bubble user-bubble">
            <!-- 附图预览 -->
            <div v-if="msg.images?.length" class="msg-images">
              <img v-for="(img, ii) in msg.images" :key="ii" :src="img" class="msg-img-thumb"
                @click="lightboxSrc = img" />
            </div>
            <div class="msg-text" v-html="renderMd(msg.text)" />
          </div>
        </div>

        <!-- AI 消息 -->
        <div v-else-if="msg.role === 'assistant'" class="msg-row assistant">
          <div class="msg-bubble assistant-bubble">
            <!-- 思考过程（可折叠） -->
            <details v-if="msg.thinking" class="thinking-block" :open="false">
              <summary class="thinking-summary">
                <span class="thinking-icon">💭</span> 思考过程
                <span class="thinking-len">{{ wordCount(msg.thinking) }} 词</span>
              </summary>
              <div class="thinking-content" v-html="renderMd(msg.thinking)" />
            </details>

            <!-- 工具调用列表 -->
            <div v-for="(tc, ti) in msg.toolCalls" :key="ti" class="tool-call-block">
              <details class="tool-details">
                <summary class="tool-summary">
                  <span class="tool-icon">🔧</span>
                  <span class="tool-name">{{ tc.name }}</span>
                  <span v-if="tc.status === 'running'" class="tool-status running">运行中…</span>
                  <span v-else-if="tc.status === 'ok'" class="tool-status ok">✓</span>
                  <span v-else-if="tc.status === 'error'" class="tool-status error">✗</span>
                </summary>
                <div class="tool-body">
                  <div v-if="tc.input" class="tool-section">
                    <div class="tool-section-label">输入</div>
                    <pre class="tool-pre">{{ formatJson(tc.input) }}</pre>
                  </div>
                  <div v-if="tc.result" class="tool-section">
                    <div class="tool-section-label">输出</div>
                    <pre class="tool-pre">{{ tc.result.slice(0, 800) }}{{ tc.result.length > 800 ? '\n…（截断）' : '' }}</pre>
                  </div>
                </div>
              </details>
            </div>

            <!-- 正文 -->
            <div v-if="msg.text" class="msg-text" v-html="renderMd(msg.text)" />

            <!-- 操作栏 -->
            <div class="msg-actions">
              <button class="action-btn" @click="copyText(msg.text)" title="复制">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
              </button>
            </div>

            <!-- Apply 卡（如果有结构化 applyData） -->
            <div v-if="msg.applyData" class="apply-card">
              <div class="apply-fields">
                <div v-for="(val, key) in msg.applyData" :key="key" class="apply-row">
                  <span class="apply-key">{{ fieldLabel(String(key)) }}</span>
                  <span class="apply-val">{{ String(val).slice(0, 50) }}{{ String(val).length > 50 ? '…' : '' }}</span>
                </div>
              </div>
              <button class="apply-btn" @click="emit('apply', msg.applyData)">
                应用到表单 ↙
              </button>
            </div>
          </div>
        </div>
      </template>

      <!-- 流式：思考中 -->
      <div v-if="streaming && streamThinking" class="msg-row assistant">
        <div class="msg-bubble assistant-bubble">
          <details class="thinking-block" open>
            <summary class="thinking-summary">
              <span class="thinking-icon">💭</span> 思考中…
            </summary>
            <div class="thinking-content streaming-thinking">{{ streamThinking }}<span class="cursor">▊</span></div>
          </details>
        </div>
      </div>

      <!-- 流式：正文 -->
      <div v-if="streaming && (streamText || (!streamThinking && !streamText))" class="msg-row assistant">
        <div class="msg-bubble assistant-bubble">
          <div v-if="streamText" class="msg-text" v-html="renderMd(streamText)" />
          <div v-else class="typing-dots"><span/><span/><span/></div>
          <span v-if="streamText" class="cursor">▊</span>
        </div>
      </div>
    </div>

    <!-- ─── 图片灯箱 ─── -->
    <div v-if="lightboxSrc" class="lightbox" @click="lightboxSrc = ''">
      <img :src="lightboxSrc" class="lightbox-img" />
    </div>

    <!-- ─── 输入区 ─── -->
    <div class="chat-input-area" @dragover.prevent="dragOver = true" @dragleave="dragOver = false"
      @drop.prevent="handleDrop" :class="{ 'drag-over': dragOver }">

      <!-- 附图预览条 -->
      <div v-if="pendingImages.length" class="pending-images">
        <div v-for="(img, i) in pendingImages" :key="i" class="pending-img-wrap">
          <img :src="img" class="pending-img" />
          <button class="remove-img" @click="pendingImages.splice(i, 1)">×</button>
        </div>
      </div>

      <div class="input-row">
        <!-- 附图按钮 -->
        <button class="input-icon-btn" title="附加图片" @click="imgInput?.click()">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="3" width="18" height="18" rx="2"/>
            <circle cx="8.5" cy="8.5" r="1.5"/>
            <polyline points="21 15 16 10 5 21"/>
          </svg>
        </button>
        <input ref="imgInput" type="file" accept="image/*" multiple style="display:none"
          @change="handleFileSelect" />

        <textarea ref="inputRef" v-model="inputText"
          class="chat-textarea"
          :placeholder="placeholder || '输入消息... (Ctrl+Enter 发送)'"
          :disabled="streaming"
          rows="1"
          @keydown.enter.ctrl.prevent="send"
          @paste="handlePaste"
          @input="autoResize" />

        <button class="send-btn" :disabled="streaming || (!inputText.trim() && !pendingImages.length)"
          @click="send">
          <svg v-if="!streaming" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
          </svg>
          <span v-else class="spin">⟳</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { chatSSE, type ChatParams } from '../api'

// ── Props ─────────────────────────────────────────────────────────────────
const props = withDefaults(defineProps<{
  agentId: string
  context?: string        // extra system context (injected per page)
  scenario?: string       // label for logging
  placeholder?: string
  welcomeMessage?: string
  examples?: string[]     // quick-start example chips
  showThinking?: boolean  // whether to surface thinking blocks (default: true)
  height?: string         // CSS height, default "100%"
  compact?: boolean       // reduces padding/font for sidepanel use
  initialMessages?: ChatMessage[]
}>(), {
  showThinking: true,
  height: '100%',
  compact: false,
  examples: () => [],
})

// ── Emits ─────────────────────────────────────────────────────────────────
const emit = defineEmits<{
  message: [text: string]
  response: [text: string]
  apply: [data: Record<string, string>]
  error: [msg: string]
}>()

// ── Types ─────────────────────────────────────────────────────────────────
interface ToolCallEntry {
  name: string
  input?: string
  result?: string
  status: 'running' | 'ok' | 'error'
}

interface ChatMessage {
  role: 'user' | 'assistant'
  text: string
  thinking?: string
  toolCalls?: ToolCallEntry[]
  images?: string[]
  applyData?: Record<string, string>
}

// ── State ─────────────────────────────────────────────────────────────────
const messages = ref<ChatMessage[]>(props.initialMessages ? [...props.initialMessages] : [])
const inputText = ref('')
const pendingImages = ref<string[]>([])
const streaming = ref(false)
const streamText = ref('')
const streamThinking = ref('')
const messagesRef = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()
const imgInput = ref<HTMLInputElement>()
const dragOver = ref(false)
const lightboxSrc = ref('')

// Active tool during streaming
let streamingToolCalls: ToolCallEntry[] = []

// ── Input handling ────────────────────────────────────────────────────────
function fillInput(text: string) {
  inputText.value = text
  nextTick(() => inputRef.value?.focus())
}

function autoResize() {
  const el = inputRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}

async function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  for (const item of Array.from(items)) {
    if (item.type.startsWith('image/')) {
      e.preventDefault()
      const file = item.getAsFile()
      if (file) await addImageFile(file)
    }
  }
}

function handleDrop(e: DragEvent) {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (!files) return
  for (const f of Array.from(files)) {
    if (f.type.startsWith('image/')) addImageFile(f)
  }
}

function handleFileSelect(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files) return
  for (const f of Array.from(files)) addImageFile(f)
  ;(e.target as HTMLInputElement).value = ''
}

function addImageFile(file: File): Promise<void> {
  return new Promise(resolve => {
    const reader = new FileReader()
    reader.onload = (ev) => {
      if (ev.target?.result) {
        pendingImages.value.push(ev.target.result as string)
      }
      resolve()
    }
    reader.readAsDataURL(file)
  })
}

// ── Send ──────────────────────────────────────────────────────────────────
async function send() {
  const msg = inputText.value.trim()
  if ((!msg && !pendingImages.value.length) || streaming.value) return

  const imgs = [...pendingImages.value]
  inputText.value = ''
  pendingImages.value = []
  nextTick(autoResize)

  messages.value.push({
    role: 'user',
    text: msg,
    images: imgs.length ? imgs : undefined,
  })
  emit('message', msg)
  scrollBottom()

  streaming.value = true
  streamText.value = ''
  streamThinking.value = ''
  streamingToolCalls = []

  const params: ChatParams = {
    context: props.context,
    scenario: props.scenario,
    images: imgs.length ? imgs : undefined,
  }

  chatSSE(props.agentId, msg, (ev) => {
    if (ev.type === 'thinking_delta') {
      streamThinking.value += ev.text
      scrollBottom()
    } else if (ev.type === 'text_delta') {
      streamText.value += ev.text
      scrollBottom()
    } else if (ev.type === 'tool_call') {
      const tc: ToolCallEntry = {
        name: ev.tool_call?.name || 'tool',
        input: ev.tool_call?.input ? JSON.stringify(ev.tool_call.input, null, 2) : undefined,
        status: 'running',
      }
      streamingToolCalls.push(tc)
    } else if (ev.type === 'tool_result') {
      const last = streamingToolCalls[streamingToolCalls.length - 1]
      if (last) { last.result = ev.text; last.status = 'ok' }
    } else if (ev.type === 'done' || ev.type === 'error') {
      if (ev.type === 'error') emit('error', ev.error || 'Unknown error')

      // Parse apply data from response text
      let applyData: Record<string, string> | undefined
      const jsonMatch = streamText.value.match(/```json\s*([\s\S]+?)\s*```/)
      if (jsonMatch) {
        try {
          applyData = JSON.parse(jsonMatch[1] as string)
          streamText.value = streamText.value.replace(/```json[\s\S]+?```/, '').trim()
        } catch {}
      }

      const finalMsg: ChatMessage = {
        role: 'assistant',
        text: streamText.value,
        thinking: streamThinking.value || undefined,
        toolCalls: streamingToolCalls.length ? [...streamingToolCalls] : undefined,
        applyData,
      }
      messages.value.push(finalMsg)
      emit('response', streamText.value)

      streamText.value = ''
      streamThinking.value = ''
      streaming.value = false
      scrollBottom()
    }
  }, params)
}

function scrollBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
    }
  })
}

// ── Helpers ───────────────────────────────────────────────────────────────
function copyText(text: string) {
  navigator.clipboard.writeText(text).catch(() => {})
}

function wordCount(text: string) {
  return text.trim().split(/\s+/).length
}

function formatJson(raw: string) {
  try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
}

function fieldLabel(key: string): string {
  const map: Record<string, string> = {
    name: '名称', id: 'ID', description: '描述',
    identity: 'IDENTITY', soul: 'SOUL',
  }
  return map[key] || key
}

// ── Markdown renderer (lightweight, no extra deps) ────────────────────────
function renderMd(text: string): string {
  if (!text) return ''
  let html = text
    // code block
    .replace(/```(\w*)\n?([\s\S]*?)```/g, (_, lang, code) => {
      const escaped = escHtml(code.trim())
      return `<pre class="code-block"><code class="lang-${lang || 'text'}">${escaped}</code></pre>`
    })
    // inline code
    .replace(/`([^`]+)`/g, (_, c) => `<code class="inline-code">${escHtml(c)}</code>`)
    // bold
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    // italic
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    // heading
    .replace(/^### (.+)$/gm, '<h4>$1</h4>')
    .replace(/^## (.+)$/gm, '<h3>$1</h3>')
    .replace(/^# (.+)$/gm, '<h2>$1</h2>')
    // list items
    .replace(/^[-*] (.+)$/gm, '<li>$1</li>')
    // newlines → <br> (outside block elements)
    .replace(/\n/g, '<br>')
    // wrap consecutive <li> in <ul>
    .replace(/(<li>.*?<\/li><br>)+/g, (m) => `<ul>${m.replace(/<br>/g, '')}</ul>`)

  return html
}

function escHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// ── Init ──────────────────────────────────────────────────────────────────
onMounted(() => {
  scrollBottom()
})

// expose so parent can call programmatically
defineExpose({ send, fillInput, messages })
</script>

<style scoped>
/* ── Layout ── */
.ai-chat {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #f5f7fa;
  font-size: 14px;
  container-type: inline-size;
}

/* compact mode */
.ai-chat.compact { font-size: 13px; }
.ai-chat.compact .msg-bubble { padding: 8px 10px; }
.ai-chat.compact .chat-input-area { padding: 8px; }

/* ── Messages ── */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  scroll-behavior: smooth;
}
.ai-chat.compact .chat-messages { padding: 10px; gap: 8px; }

.welcome-msg {
  text-align: center;
  padding: 24px 16px;
  color: #606266;
}
.welcome-icon { font-size: 32px; margin-bottom: 8px; }
.welcome-text { line-height: 1.7; }

.examples-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
  padding: 0 8px 8px;
}
.example-chip {
  background: #fff;
  border: 1px solid #dcdfe6;
  border-radius: 18px;
  padding: 6px 14px;
  cursor: pointer;
  color: #409eff;
  font-size: 13px;
  transition: all 0.15s;
}
.example-chip:hover { background: #ecf5ff; border-color: #409eff; }

/* ── Message rows ── */
.msg-row { display: flex; }
.msg-row.user { justify-content: flex-end; }
.msg-row.assistant { justify-content: flex-start; }

.msg-bubble {
  max-width: 80%;
  padding: 10px 14px;
  border-radius: 12px;
  line-height: 1.65;
  position: relative;
}
.user-bubble {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
}
.assistant-bubble {
  background: #fff;
  color: #303133;
  border-bottom-left-radius: 4px;
  box-shadow: 0 1px 4px rgba(0,0,0,.08);
}

/* Responsive: narrow screens → wider bubbles */
@container (max-width: 400px) {
  .msg-bubble { max-width: 95%; }
}

/* ── Images in messages ── */
.msg-images { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px; }
.msg-img-thumb {
  width: 80px; height: 80px;
  object-fit: cover;
  border-radius: 6px;
  cursor: pointer;
  border: 2px solid rgba(255,255,255,.3);
}

/* ── Thinking block ── */
.thinking-block {
  margin-bottom: 8px;
  border: 1px solid #e0e6ef;
  border-radius: 8px;
  overflow: hidden;
  background: #f9fbff;
}
.thinking-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  cursor: pointer;
  font-size: 12px;
  color: #909399;
  user-select: none;
  list-style: none;
}
.thinking-summary::-webkit-details-marker { display: none; }
.thinking-icon { font-size: 14px; }
.thinking-len { margin-left: auto; font-size: 11px; }
.thinking-content {
  padding: 8px 12px 10px;
  font-size: 12px;
  color: #606266;
  border-top: 1px solid #e0e6ef;
  white-space: pre-wrap;
  max-height: 300px;
  overflow-y: auto;
}
.streaming-thinking { font-style: italic; }

/* ── Tool call block ── */
.tool-call-block { margin-bottom: 6px; }
.tool-details {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  overflow: hidden;
  background: #fafafa;
}
.tool-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  cursor: pointer;
  font-size: 12px;
  list-style: none;
  user-select: none;
}
.tool-summary::-webkit-details-marker { display: none; }
.tool-icon { font-size: 13px; }
.tool-name { font-family: monospace; font-size: 12px; }
.tool-status { margin-left: auto; font-size: 11px; }
.tool-status.running { color: #e6a23c; }
.tool-status.ok { color: #67c23a; }
.tool-status.error { color: #f56c6c; }
.tool-body { border-top: 1px solid #e4e7ed; padding: 8px; }
.tool-section { margin-bottom: 6px; }
.tool-section-label { font-size: 11px; color: #909399; margin-bottom: 3px; }
.tool-pre {
  margin: 0;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  background: #f0f0f0;
  border-radius: 4px;
  padding: 6px 8px;
  max-height: 160px;
  overflow-y: auto;
}

/* ── Message text / markdown ── */
.msg-text :deep(pre.code-block) {
  margin: 8px 0 4px;
  padding: 10px 12px;
  background: rgba(0,0,0,.07);
  border-radius: 6px;
  overflow-x: auto;
  font-size: 12px;
  line-height: 1.5;
}
.user-bubble .msg-text :deep(pre.code-block) { background: rgba(0,0,0,.15); }
.msg-text :deep(code.inline-code) {
  background: rgba(0,0,0,.08);
  border-radius: 3px;
  padding: 1px 4px;
  font-size: .9em;
}
.msg-text :deep(h2), .msg-text :deep(h3), .msg-text :deep(h4) {
  margin: 10px 0 4px;
  font-size: 1em;
}
.msg-text :deep(ul) { margin: 4px 0; padding-left: 20px; }
.msg-text :deep(li) { margin: 2px 0; }

/* ── Message actions ── */
.msg-actions {
  display: flex;
  gap: 4px;
  margin-top: 6px;
  opacity: 0;
  transition: opacity 0.15s;
}
.assistant-bubble:hover .msg-actions { opacity: 1; }
.action-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: #909399;
  padding: 2px 4px;
  border-radius: 4px;
}
.action-btn:hover { color: #409eff; background: #f0f2f5; }

/* ── Apply card ── */
.apply-card {
  margin-top: 10px;
  border-top: 1px solid #e4e7ed;
  padding-top: 8px;
}
.apply-fields { margin-bottom: 8px; }
.apply-row {
  display: flex;
  gap: 8px;
  font-size: 12px;
  padding: 2px 0;
}
.apply-key { color: #909399; min-width: 60px; flex-shrink: 0; }
.apply-val { color: #303133; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.apply-btn {
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 5px 14px;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s;
}
.apply-btn:hover { background: #337ecc; }

/* ── Streaming cursor ── */
@keyframes blink { 50% { opacity: 0; } }
.cursor { animation: blink 0.7s infinite; font-size: 13px; }

/* ── Typing dots ── */
.typing-dots {
  display: flex;
  gap: 4px;
  align-items: center;
  padding: 4px 0;
}
.typing-dots span {
  width: 7px; height: 7px;
  background: #c0c4cc;
  border-radius: 50%;
  animation: bounce 1.2s infinite;
}
.typing-dots span:nth-child(2) { animation-delay: .2s; }
.typing-dots span:nth-child(3) { animation-delay: .4s; }
@keyframes bounce { 0%,80%,100% { transform: scale(0.6); } 40% { transform: scale(1); } }

/* ── Input area ── */
.chat-input-area {
  padding: 10px 12px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  flex-shrink: 0;
  transition: box-shadow 0.15s;
}
.chat-input-area.drag-over {
  box-shadow: inset 0 0 0 2px #409eff;
  background: #ecf5ff;
}

.pending-images {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  padding-bottom: 8px;
}
.pending-img-wrap { position: relative; }
.pending-img {
  width: 56px; height: 56px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid #dcdfe6;
}
.remove-img {
  position: absolute;
  top: -4px; right: -4px;
  width: 16px; height: 16px;
  border-radius: 50%;
  background: #f56c6c;
  color: #fff;
  border: none;
  cursor: pointer;
  font-size: 10px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.input-row {
  display: flex;
  align-items: flex-end;
  gap: 6px;
}

.input-icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: #909399;
  padding: 6px;
  border-radius: 6px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
}
.input-icon-btn:hover { color: #409eff; background: #f0f2f5; }

.chat-textarea {
  flex: 1;
  border: 1px solid #dcdfe6;
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 13px;
  resize: none;
  outline: none;
  line-height: 1.5;
  min-height: 36px;
  max-height: 160px;
  font-family: inherit;
  transition: border-color 0.15s;
  overflow-y: auto;
}
.chat-textarea:focus { border-color: #409eff; }
.chat-textarea:disabled { background: #f5f7fa; }

.send-btn {
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 8px;
  width: 36px; height: 36px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: background 0.15s;
}
.send-btn:hover:not(:disabled) { background: #337ecc; }
.send-btn:disabled { background: #c0c4cc; cursor: not-allowed; }

@keyframes spin { to { transform: rotate(360deg); } }
.spin { display: inline-block; animation: spin 0.8s linear infinite; }

/* ── Lightbox ── */
.lightbox {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.8);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  cursor: zoom-out;
}
.lightbox-img {
  max-width: 90vw;
  max-height: 90vh;
  border-radius: 8px;
}
</style>
