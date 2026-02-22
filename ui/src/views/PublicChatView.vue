<template>
  <div class="pub-chat-page">
    <!-- Password gate -->
    <div v-if="needPassword && !authed" class="password-gate">
      <div class="gate-card">
        <div class="gate-avatar" :style="{ background: info?.avatarColor || '#409EFF' }">
          {{ initial }}
        </div>
        <h2 class="gate-name">{{ info?.name || '...' }}</h2>
        <p class="gate-hint">此对话已设置访问密码</p>
        <input
          v-model="passwordInput"
          type="password"
          placeholder="请输入密码"
          class="gate-input"
          @keydown.enter="submitPassword"
        />
        <div v-if="passwordError" class="gate-error">密码错误，请重试</div>
        <button class="gate-btn" @click="submitPassword" :disabled="!passwordInput">进入对话</button>
      </div>
    </div>

    <!-- Channel closed/deleted -->
    <div v-else-if="channelDeleted" class="closed-page">
      <div class="closed-card">
        <div class="closed-icon">🔒</div>
        <h2 class="closed-title">此对话已关闭</h2>
        <p class="closed-hint">该 Web 渠道已被停用，无法继续访问。</p>
      </div>
    </div>

    <!-- Chat UI -->
    <div v-else-if="infoLoaded" class="chat-page">
      <!-- Header -->
      <div class="chat-header">
        <div class="chat-header-left">
          <div class="header-avatar" :style="{ background: info?.avatarColor || '#409EFF' }">
            {{ initial }}
          </div>
          <div class="header-info">
            <div class="header-name">{{ info?.name }}</div>
            <div class="header-subtitle">AI 助手 · ZyHive</div>
          </div>
        </div>
      </div>

      <!-- Messages -->
      <div class="messages-area" ref="messagesRef">
        <!-- Welcome -->
        <div v-if="!messages.length" class="welcome-msg">
          <div class="welcome-avatar" :style="{ background: info?.avatarColor || '#409EFF' }">{{ initial }}</div>
          <div class="welcome-bubble">
            {{ info?.welcomeMsg || `你好！我是 ${info?.name}，有什么可以帮你的？` }}
          </div>
        </div>

        <!-- Message list -->
        <div v-for="(msg, i) in messages" :key="i" :class="['msg-row', msg.role]">
          <div v-if="msg.role === 'assistant'" class="msg-avatar" :style="{ background: info?.avatarColor || '#409EFF' }">
            {{ initial }}
          </div>
          <div class="msg-bubble" v-html="renderText(msg.content)"></div>
        </div>

        <!-- Streaming indicator -->
        <div v-if="streaming" class="msg-row assistant">
          <div class="msg-avatar" :style="{ background: info?.avatarColor || '#409EFF' }">{{ initial }}</div>
          <div class="msg-bubble streaming">
            <span v-html="renderText(streamingText)"></span>
            <span class="cursor">▋</span>
          </div>
        </div>
      </div>

      <!-- Input + footer -->
      <div class="input-area">
        <textarea
          v-model="inputText"
          class="input-box"
          placeholder="输入消息…"
          rows="1"
          @keydown.enter.exact.prevent="sendMessage"
          @input="autoResize"
          ref="inputRef"
        />
        <button class="send-btn" @click="sendMessage" :disabled="!inputText.trim() || streaming">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
          </svg>
        </button>
      </div>
      <div class="chat-footer">Powered by <a href="https://github.com/sunhuihui6688-star/ai-panel" target="_blank" class="chat-footer-link">引巢 · ZyHive</a> · © 2025 zyling</div>
    </div>

    <!-- Loading -->
    <div v-else class="loading-page">
      <div class="loading-spinner"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const agentId = route.params.agentId as string
const channelId = route.params.channelId as string | undefined // undefined on legacy route

// Build API path prefix: /pub/chat/:agentId/:channelId  OR  /pub/chat/:agentId (legacy)
const apiBase = channelId
  ? `/pub/chat/${agentId}/${channelId}`
  : `/pub/chat/${agentId}`

// Per-channel session token stored in localStorage — identifies this browser/visitor.
// Enables server-side conversation history + memory compaction per visitor.
function getOrCreateSessionToken(): string {
  const key = `chat-session-${agentId}-${channelId || 'default'}`
  let token = localStorage.getItem(key)
  if (!token) {
    token = Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10)
    localStorage.setItem(key, token)
  }
  return token
}
const sessionToken = getOrCreateSessionToken()

interface ChatInfo {
  agentId: string
  channelId?: string
  name: string
  avatarColor: string
  hasPassword: boolean
  title?: string
  welcomeMsg?: string
}

interface Message { role: 'user' | 'assistant'; content: string }

const info = ref<ChatInfo | null>(null)
const infoLoaded = ref(false)
const channelDeleted = ref(false)
const needPassword = ref(false)
const authed = ref(false)
const passwordInput = ref('')
const passwordError = ref(false)
const password = ref('')

// Server-side session ID — persisted in localStorage for reconnect across page refreshes.
const sessionIdKey = `pub-session-id-${agentId}-${channelId || 'default'}`
const currentSessionId = ref(localStorage.getItem(sessionIdKey) ?? '')

const messages = ref<Message[]>([])
const inputText = ref('')
const streaming = ref(false)
const streamingText = ref('')
const messagesRef = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()

const initial = computed(() => (info.value?.name || '?').charAt(0).toUpperCase())

// Password storage key scoped to channel
const pwStorageKey = `chat-pw-${agentId}-${channelId || 'default'}`

async function loadInfo() {
  try {
    const res = await fetch(`${apiBase}/info`)
    if (res.status === 404 || res.status === 410) {
      channelDeleted.value = true
      infoLoaded.value = false
      return
    }
    if (!res.ok) { infoLoaded.value = true; return }
    const data: ChatInfo = await res.json()
    info.value = data
    if (data.title) document.title = data.title
    needPassword.value = data.hasPassword
    if (!data.hasPassword) {
      authed.value = true
      await loadHistory()
    } else {
      const saved = sessionStorage.getItem(pwStorageKey)
      if (saved) {
        password.value = saved
        authed.value = true
        await loadHistory()
      } else {
        authed.value = false
      }
    }
    infoLoaded.value = true
  } catch {
    infoLoaded.value = true
  }
}

// Load history from server on mount (or after password auth).
async function loadHistory() {
  if (!sessionToken) return
  try {
    const params = new URLSearchParams({ sessionToken })
    const headers: Record<string, string> = {}
    if (password.value) headers['X-Chat-Password'] = password.value
    const res = await fetch(`${apiBase}/history?${params}`, { headers })
    if (!res.ok) return
    const data = await res.json()
    if (data.sessionId) currentSessionId.value = data.sessionId
    if (Array.isArray(data.messages) && data.messages.length > 0) {
      messages.value = data.messages.map((m: any) => ({ role: m.role, content: m.content }))
      await scrollBottom()
    }
  } catch { /* no history yet */ }
}

async function submitPassword() {
  if (!passwordInput.value) return
  const pw = passwordInput.value
  try {
    const res = await fetch(`${apiBase}/info`, {
      headers: { 'X-Chat-Password': pw },
    })
    if (res.status === 401) {
      passwordError.value = true
      return
    }
  } catch {
    // Network error — proceed optimistically
  }
  password.value = pw
  sessionStorage.setItem(pwStorageKey, pw)
  authed.value = true
  passwordError.value = false
  await loadHistory()
}

async function sendMessage() {
  const text = inputText.value.trim()
  if (!text || streaming.value) return
  inputText.value = ''
  nextTick(() => autoResize())
  messages.value.push({ role: 'user', content: text })
  await scrollBottom()
  await streamResponse(text)
}

// consumeSSE reads an SSE stream and processes events.
// Returns true if generation completed normally.
async function consumeSSE(res: Response): Promise<boolean> {
  if (!res.body) return false
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  let done = false

  while (true) {
    const { done: streamDone, value } = await reader.read()
    if (streamDone) break
    buf += decoder.decode(value, { stream: true })
    const parts = buf.split('\n\n')
    buf = parts.pop() ?? ''
    for (const part of parts) {
      const line = part.startsWith('data: ') ? part.slice(6) : part
      if (!line.trim()) continue
      try {
        const ev = JSON.parse(line)
        if (ev.type === 'text_delta') {
          streamingText.value += ev.text
          await scrollBottom()
        } else if (ev.type === 'done') {
          if (ev.sessionId) {
            currentSessionId.value = ev.sessionId
            localStorage.setItem(sessionIdKey, ev.sessionId)
          }
          done = true
          break
        } else if (ev.type === 'idle') {
          done = true
          break
        } else if (ev.type === 'error') {
          streamingText.value += `\n[错误: ${ev.error ?? ev.text}]`
        }
      } catch {}
    }
    if (done) break
  }
  return done
}

async function streamResponse(message: string) {
  streaming.value = true
  streamingText.value = ''

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (password.value) headers['X-Chat-Password'] = password.value

  try {
    const res = await fetch(`${apiBase}/stream`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ message, sessionToken }),
    })

    if (res.status === 401) {
      password.value = ''
      sessionStorage.removeItem(pwStorageKey)
      authed.value = false
      passwordError.value = true
      streaming.value = false
      streamingText.value = ''
      messages.value.pop()
      return
    }
    if (res.status === 410) {
      channelDeleted.value = true
      infoLoaded.value = false
      return
    }
    if (!res.ok) throw new Error('Request failed')

    await consumeSSE(res)
  } catch {
    streamingText.value += '\n[连接错误，请重试]'
  } finally {
    if (streamingText.value) {
      messages.value.push({ role: 'assistant', content: streamingText.value })
    }
    streaming.value = false
    streamingText.value = ''
    await scrollBottom()
  }
}

// reconnectIfGenerating checks if there's an in-progress generation for our session
// and subscribes to receive its output (handles page refresh mid-generation).
async function reconnectIfGenerating() {
  if (!currentSessionId.value) return
  const headers: Record<string, string> = {}
  if (password.value) headers['X-Chat-Password'] = password.value
  try {
    const res = await fetch(`${apiBase}/reconnect?sessionId=${encodeURIComponent(currentSessionId.value)}`, { headers })
    if (!res.ok) return
    streaming.value = true
    streamingText.value = ''
    await consumeSSE(res)
    if (streamingText.value) {
      // Check if last message is the one being generated (avoid duplicate)
      const last = messages.value[messages.value.length - 1]
      if (!last || last.role !== 'assistant' || last.content !== streamingText.value) {
        messages.value.push({ role: 'assistant', content: streamingText.value })
      }
    }
    streaming.value = false
    streamingText.value = ''
    await scrollBottom()
    // Refresh history to ensure everything is up to date
    await loadHistory()
  } catch { /* not generating */ }
}

async function scrollBottom() {
  await nextTick()
  if (messagesRef.value) {
    messagesRef.value.scrollTop = messagesRef.value.scrollHeight
  }
}

function autoResize() {
  if (!inputRef.value) return
  inputRef.value.style.height = 'auto'
  inputRef.value.style.height = Math.min(inputRef.value.scrollHeight, 140) + 'px'
}

function renderText(text: string): string {
  return text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\n/g, '<br>')
}

onMounted(async () => {
  await loadInfo()
  // If a generation was in progress when the page was closed, reconnect to receive it
  if (authed.value && currentSessionId.value) {
    await reconnectIfGenerating()
  }
})
</script>

<style scoped>
* { box-sizing: border-box; margin: 0; padding: 0; }

.pub-chat-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

/* Password gate */
.password-gate {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.gate-card {
  background: #fff;
  border-radius: 16px;
  padding: 40px 36px;
  width: 360px;
  text-align: center;
  box-shadow: 0 20px 60px rgba(0,0,0,0.2);
}
.gate-avatar {
  width: 72px; height: 72px;
  border-radius: 50%;
  color: #fff;
  font-size: 28px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
}
.gate-name { font-size: 22px; font-weight: 700; color: #303133; margin-bottom: 8px; }
.gate-hint { font-size: 14px; color: #909399; margin-bottom: 24px; }
.gate-input {
  width: 100%;
  padding: 12px 16px;
  border: 1px solid #dcdfe6;
  border-radius: 8px;
  font-size: 15px;
  outline: none;
  transition: border-color 0.2s;
  margin-bottom: 8px;
}
.gate-input:focus { border-color: #409eff; }
.gate-error { font-size: 13px; color: #f56c6c; margin-bottom: 8px; }
.gate-btn {
  width: 100%;
  padding: 12px;
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  margin-top: 8px;
  transition: background 0.2s;
}
.gate-btn:hover:not(:disabled) { background: #337ecc; }
.gate-btn:disabled { opacity: 0.5; cursor: not-allowed; }

/* Chat UI */
.chat-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
}
.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  flex-shrink: 0;
}
.chat-header-left { display: flex; align-items: center; gap: 12px; }
.header-avatar {
  width: 40px; height: 40px;
  border-radius: 50%;
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.header-name { font-size: 16px; font-weight: 700; color: #303133; }
.header-subtitle { font-size: 12px; color: #909399; }

.messages-area {
  flex: 1;
  overflow-y: auto;
  padding: 24px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Welcome */
.welcome-msg {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.welcome-avatar {
  width: 36px; height: 36px;
  border-radius: 50%;
  color: #fff;
  font-size: 14px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.welcome-bubble {
  background: #fff;
  border-radius: 0 12px 12px 12px;
  padding: 12px 16px;
  font-size: 14px;
  color: #303133;
  line-height: 1.6;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
  max-width: 70%;
}

/* Message rows */
.msg-row {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.msg-row.user { flex-direction: row-reverse; }
.msg-avatar {
  width: 32px; height: 32px;
  border-radius: 50%;
  color: #fff;
  font-size: 13px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.msg-bubble {
  max-width: 68%;
  padding: 10px 14px;
  border-radius: 18px;
  font-size: 14px;
  line-height: 1.6;
  word-break: break-word;
}
.msg-row.user .msg-bubble {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
}
.msg-row.assistant .msg-bubble {
  background: #fff;
  color: #303133;
  border-bottom-left-radius: 4px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}
.msg-bubble.streaming { opacity: 0.9; }
.cursor {
  display: inline-block;
  animation: blink 1s step-end infinite;
  color: #409eff;
}
@keyframes blink { 0%, 100% { opacity: 1 } 50% { opacity: 0 } }

/* Input */
.input-area {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  padding: 16px 20px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  flex-shrink: 0;
}
.input-box {
  flex: 1;
  border: 1px solid #dcdfe6;
  border-radius: 12px;
  padding: 10px 14px;
  font-size: 14px;
  font-family: inherit;
  resize: none;
  outline: none;
  line-height: 1.5;
  max-height: 140px;
  transition: border-color 0.2s;
}
.input-box:focus { border-color: #409eff; }
.send-btn {
  width: 44px; height: 44px;
  border-radius: 50%;
  background: #409eff;
  color: #fff;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: background 0.2s, transform 0.1s;
}
.send-btn:hover:not(:disabled) { background: #337ecc; transform: scale(1.05); }
.send-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* Footer */
.chat-footer {
  text-align: center;
  font-size: 11px;
  color: #c0c4cc;
  padding: 6px 0 8px;
  background: #fff;
  border-top: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.chat-footer-link {
  color: #c0c4cc;
  text-decoration: none;
}
.chat-footer-link:hover { color: #409eff; }

/* Loading */
.loading-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
.loading-spinner {
  width: 40px; height: 40px;
  border: 3px solid #e4e7ed;
  border-top-color: #409eff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg) } }
.closed-page { height: 100vh; display: flex; align-items: center; justify-content: center; background: #f5f7fa; }
.closed-card { text-align: center; padding: 40px; background: #fff; border-radius: 16px; box-shadow: 0 4px 24px rgba(0,0,0,.08); max-width: 360px; }
.closed-icon { font-size: 48px; margin-bottom: 16px; }
.closed-title { margin: 0 0 8px; font-size: 20px; color: #303133; }
.closed-hint { margin: 0; color: #909399; font-size: 14px; }

/* Inline code */
:deep(code) {
  background: #f5f7fa;
  padding: 1px 5px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 13px;
  color: #e6773d;
}
.msg-row.user :deep(code) {
  background: rgba(255,255,255,0.2);
  color: #fff;
}
</style>
