<template>
  <el-container class="agent-detail">
    <el-header class="detail-header">
      <div class="header-left">
        <el-button :icon="ArrowLeft" @click="$router.push('/')" circle />
        <h2>{{ agent?.name || '...' }}</h2>
        <el-tag :type="statusType(agent?.status)">{{ statusLabel(agent?.status) }}</el-tag>
      </div>
      <el-text type="info">{{ agent?.model }}</el-text>
    </el-header>

    <el-main>
      <el-tabs v-model="activeTab" type="border-card">
        <!-- Tab 1: Chat -->
        <el-tab-pane label="对话" name="chat">
          <div class="chat-container">
            <div class="chat-messages" ref="chatMessagesRef">
              <div
                v-for="(msg, i) in chatMessages"
                :key="i"
                :class="['chat-msg', msg.role]"
              >
                <div class="msg-bubble">
                  <div v-if="msg.role === 'tool'" class="tool-block">
                    <el-collapse>
                      <el-collapse-item :title="'🔧 ' + (msg.toolName || 'tool')">
                        <pre class="tool-result">{{ msg.text?.slice(0, 500) }}</pre>
                      </el-collapse-item>
                    </el-collapse>
                  </div>
                  <div v-else class="msg-text" v-html="renderMarkdown(msg.text)"></div>
                </div>
              </div>
              <!-- Typing indicator -->
              <div v-if="streaming && !streamText" class="chat-msg assistant">
                <div class="msg-bubble">
                  <div class="typing-indicator">
                    <span></span><span></span><span></span>
                  </div>
                </div>
              </div>
              <!-- Streaming text -->
              <div v-if="streaming && streamText" class="chat-msg assistant">
                <div class="msg-bubble">
                  <div class="msg-text" v-html="renderMarkdown(streamText)"></div>
                  <span class="cursor">▊</span>
                </div>
              </div>
            </div>
            <div class="chat-input">
              <el-input
                v-model="chatInput"
                type="textarea"
                :rows="2"
                placeholder="输入消息... (Ctrl+Enter 发送)"
                @keydown.enter.ctrl="sendMessage"
              />
              <el-button type="primary" @click="sendMessage" :loading="streaming" style="margin-top: 8px">
                <el-icon><Promotion /></el-icon> 发送
              </el-button>
            </div>
          </div>
        </el-tab-pane>

        <!-- Tab 2: Identity & Soul -->
        <el-tab-pane label="身份 & 灵魂" name="identity">
          <el-row :gutter="20">
            <el-col :span="12">
              <el-card header="IDENTITY.md">
                <el-input
                  v-model="identityContent"
                  type="textarea"
                  :rows="15"
                  @blur="saveFile('IDENTITY.md', identityContent)"
                />
              </el-card>
            </el-col>
            <el-col :span="12">
              <el-card header="SOUL.md">
                <el-input
                  v-model="soulContent"
                  type="textarea"
                  :rows="15"
                  @blur="saveFile('SOUL.md', soulContent)"
                />
              </el-card>
            </el-col>
          </el-row>
        </el-tab-pane>

        <!-- Tab 3: Memory -->
        <el-tab-pane label="记忆" name="memory">
          <el-card header="MEMORY.md">
            <el-input
              v-model="memoryContent"
              type="textarea"
              :rows="10"
              @blur="saveFile('MEMORY.md', memoryContent)"
            />
          </el-card>
          <el-card header="每日记忆文件" style="margin-top: 16px">
            <div v-if="memoryFiles.length === 0">
              <el-empty description="暂无记忆文件" :image-size="60" />
            </div>
            <el-timeline v-else>
              <el-timeline-item
                v-for="f in memoryFiles"
                :key="f.name"
                :timestamp="f.date || f.name"
                placement="top"
              >
                <el-card shadow="hover" class="memory-card" @click="openMemoryFile(f.name)">
                  <div style="display: flex; justify-content: space-between; align-items: center">
                    <span>{{ f.name }}</span>
                    <el-text type="info" size="small">{{ formatSize(f.size) }}</el-text>
                  </div>
                </el-card>
              </el-timeline-item>
            </el-timeline>
          </el-card>

          <!-- Memory file editor dialog -->
          <el-dialog v-model="showMemoryEditor" :title="editingMemoryFile" width="700px">
            <el-input
              v-model="editingMemoryContent"
              type="textarea"
              :rows="20"
            />
            <template #footer>
              <el-button @click="showMemoryEditor = false">取消</el-button>
              <el-button type="primary" @click="saveMemoryFile">保存</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

        <!-- Tab 4: Workspace -->
        <el-tab-pane label="工作区" name="workspace">
          <el-row :gutter="20">
            <el-col :span="8">
              <el-card header="文件列表">
                <el-tree
                  :data="fileTreeData"
                  :props="{ label: 'name', children: 'children' }"
                  @node-click="handleFileClick"
                  highlight-current
                  default-expand-all
                />
              </el-card>
            </el-col>
            <el-col :span="16">
              <el-card :header="currentFile || '选择文件查看'">
                <template v-if="currentFile">
                  <el-input
                    v-model="currentFileContent"
                    type="textarea"
                    :rows="20"
                  />
                  <div style="margin-top: 8px; display: flex; gap: 8px; align-items: center;">
                    <el-button type="primary" @click="saveCurrentFile">保存</el-button>
                    <el-text type="info" size="small" v-if="currentFileInfo">
                      {{ formatSize(currentFileInfo.size) }} · {{ formatTime(currentFileInfo.modTime) }}
                    </el-text>
                  </div>
                </template>
              </el-card>
            </el-col>
          </el-row>
        </el-tab-pane>

        <!-- Tab 5: Cron -->
        <el-tab-pane label="定时任务" name="cron">
          <el-button type="primary" @click="showCronCreate = true" style="margin-bottom: 16px">
            <el-icon><Plus /></el-icon> 新建任务
          </el-button>
          <el-table :data="cronJobs" stripe>
            <el-table-column prop="name" label="名称" />
            <el-table-column label="调度">
              <template #default="{ row }">{{ row.schedule?.expr }} ({{ row.schedule?.tz }})</template>
            </el-table-column>
            <el-table-column label="最近运行" width="180">
              <template #default="{ row }">
                <template v-if="row.state?.lastRunAtMs">
                  <el-tag :type="row.state?.lastStatus === 'ok' ? 'success' : 'danger'" size="small">
                    {{ row.state?.lastStatus }}
                  </el-tag>
                  <el-text type="info" size="small" style="margin-left: 4px">
                    {{ formatTimestamp(row.state?.lastRunAtMs) }}
                  </el-text>
                </template>
                <el-text v-else type="info" size="small">未运行</el-text>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="80">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" @change="toggleCron(row)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="200">
              <template #default="{ row }">
                <el-button size="small" @click="runCronNow(row)">立即运行</el-button>
                <el-button size="small" type="danger" @click="deleteCron(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>

          <!-- Create Cron Dialog -->
          <el-dialog v-model="showCronCreate" title="新建定时任务" width="520px">
            <el-form :model="cronForm" label-width="100px">
              <el-form-item label="名称">
                <el-input v-model="cronForm.name" />
              </el-form-item>
              <el-form-item label="Cron 表达式">
                <el-input v-model="cronForm.expr" placeholder="30 3 * * *" />
              </el-form-item>
              <el-form-item label="时区">
                <el-select v-model="cronForm.tz">
                  <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
                  <el-option label="UTC" value="UTC" />
                  <el-option label="America/New_York" value="America/New_York" />
                </el-select>
              </el-form-item>
              <el-form-item label="消息">
                <el-input v-model="cronForm.message" type="textarea" :rows="3" />
              </el-form-item>
              <el-form-item label="启用">
                <el-switch v-model="cronForm.enabled" />
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showCronCreate = false">取消</el-button>
              <el-button type="primary" @click="createCron">创建</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>
      </el-tabs>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { agents as agentsApi, files as filesApi, cron as cronApi, chatSSE, type AgentInfo, type FileEntry, type CronJob } from '../api'

const route = useRoute()
const agentId = route.params.id as string
const agent = ref<AgentInfo | null>(null)
const activeTab = ref('chat')

// Chat state
interface ChatMsg { role: string; text: string; toolName?: string }
const chatMessages = ref<ChatMsg[]>([])
const chatInput = ref('')
const streaming = ref(false)
const streamText = ref('')
const chatMessagesRef = ref<HTMLElement>()

// Identity/Soul/Memory
const identityContent = ref('')
const soulContent = ref('')
const memoryContent = ref('')
const memoryFiles = ref<any[]>([])

// Memory editor
const showMemoryEditor = ref(false)
const editingMemoryFile = ref('')
const editingMemoryContent = ref('')

// Workspace
const fileTreeData = ref<any[]>([])
const currentFile = ref('')
const currentFileContent = ref('')
const currentFileInfo = ref<FileEntry | null>(null)

// Cron
const cronJobs = ref<CronJob[]>([])
const showCronCreate = ref(false)
const cronForm = ref({ name: '', expr: '0 9 * * *', tz: 'Asia/Shanghai', message: '', enabled: true })

function statusType(s?: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s?: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
}
function renderMarkdown(text: string) {
  return (text || '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.*?)`/g, '<code>$1</code>')
    .replace(/\n/g, '<br>')
}
function formatSize(bytes: number) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1048576).toFixed(1) + ' MB'
}
function formatTime(t: string) {
  return new Date(t).toLocaleString()
}
function formatTimestamp(ms: number) {
  if (!ms) return ''
  return new Date(ms).toLocaleString()
}

// Load agent
onMounted(async () => {
  try {
    const res = await agentsApi.get(agentId)
    agent.value = res.data
  } catch {
    ElMessage.error('加载 Agent 失败')
  }
  loadIdentityFiles()
  loadWorkspace()
  loadCron()
})

// Auto-scroll chat when streaming
watch(streamText, () => scrollChat())

// Chat — uses fetch + ReadableStream for SSE (supports auth headers)
async function sendMessage() {
  const msg = chatInput.value.trim()
  if (!msg || streaming.value) return
  
  chatMessages.value.push({ role: 'user', text: msg })
  chatInput.value = ''
  streaming.value = true
  streamText.value = ''
  
  await nextTick()
  scrollChat()
  
  chatSSE(agentId, msg, (ev) => {
    if (ev.type === 'text_delta') {
      streamText.value += ev.text
    } else if (ev.type === 'tool_call') {
      // Show tool call as collapsed card
      if (streamText.value) {
        chatMessages.value.push({ role: 'assistant', text: streamText.value })
        streamText.value = ''
      }
      chatMessages.value.push({ role: 'tool', text: '', toolName: ev.tool_call?.name || 'tool' })
      scrollChat()
    } else if (ev.type === 'tool_result') {
      const last = chatMessages.value[chatMessages.value.length - 1]
      if (last?.role === 'tool') last.text = ev.text
    } else if (ev.type === 'done') {
      if (streamText.value) {
        chatMessages.value.push({ role: 'assistant', text: streamText.value })
      }
      streamText.value = ''
      streaming.value = false
      scrollChat()
    } else if (ev.type === 'error') {
      ElMessage.error(ev.error || '请求失败')
      streaming.value = false
    }
  })
}

function scrollChat() {
  nextTick(() => {
    const el = chatMessagesRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

// Identity files
async function loadIdentityFiles() {
  try {
    const [id, soul, mem] = await Promise.all([
      filesApi.read(agentId, 'IDENTITY.md'),
      filesApi.read(agentId, 'SOUL.md'),
      filesApi.read(agentId, 'MEMORY.md'),
    ])
    identityContent.value = id.data?.content || ''
    soulContent.value = soul.data?.content || ''
    memoryContent.value = mem.data?.content || ''
  } catch {}
  // Load memory files
  try {
    const res = await filesApi.read(agentId, 'memory')
    if (Array.isArray(res.data)) {
      memoryFiles.value = res.data
        .filter((f: FileEntry) => !f.isDir)
        .sort((a: any, b: any) => new Date(b.modTime).getTime() - new Date(a.modTime).getTime())
        .map((f: FileEntry) => ({
          name: f.name,
          date: f.name.replace('.md', ''),
          size: f.size,
          modTime: f.modTime,
        }))
    }
  } catch {}
}

async function saveFile(name: string, content: string) {
  try {
    await filesApi.write(agentId, name, content)
    ElMessage.success(`${name} 已保存`)
  } catch {
    ElMessage.error(`保存 ${name} 失败`)
  }
}

async function openMemoryFile(name: string) {
  editingMemoryFile.value = name
  try {
    const res = await filesApi.read(agentId, `memory/${name}`)
    editingMemoryContent.value = res.data?.content || ''
    showMemoryEditor.value = true
  } catch {
    ElMessage.error('读取文件失败')
  }
}

async function saveMemoryFile() {
  try {
    await filesApi.write(agentId, `memory/${editingMemoryFile.value}`, editingMemoryContent.value)
    ElMessage.success('记忆文件已保存')
    showMemoryEditor.value = false
    loadIdentityFiles()
  } catch {
    ElMessage.error('保存失败')
  }
}

// Workspace
async function loadWorkspace() {
  try {
    const res = await filesApi.read(agentId, '/')
    if (Array.isArray(res.data)) {
      fileTreeData.value = res.data.map((f: FileEntry) => ({
        name: f.name,
        isDir: f.isDir,
        size: f.size,
        modTime: f.modTime,
        path: f.name,
      }))
    }
  } catch {}
}

async function handleFileClick(data: any) {
  if (data.isDir) return
  currentFile.value = data.path || data.name
  currentFileInfo.value = data
  try {
    const res = await filesApi.read(agentId, currentFile.value)
    currentFileContent.value = res.data?.content || ''
  } catch {
    currentFileContent.value = '(无法读取)'
  }
}

async function saveCurrentFile() {
  if (!currentFile.value) return
  await saveFile(currentFile.value, currentFileContent.value)
}

// Cron
async function loadCron() {
  try {
    const res = await cronApi.list()
    cronJobs.value = res.data || []
  } catch {}
}

async function createCron() {
  try {
    await cronApi.create({
      name: cronForm.value.name,
      enabled: cronForm.value.enabled,
      schedule: { kind: 'cron', expr: cronForm.value.expr, tz: cronForm.value.tz },
      payload: { kind: 'agentTurn', message: cronForm.value.message },
      delivery: { mode: 'announce' },
    } as any)
    ElMessage.success('任务创建成功')
    showCronCreate.value = false
    loadCron()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

async function toggleCron(job: any) {
  try {
    await cronApi.update(job.id, job)
  } catch {
    ElMessage.error('更新失败')
  }
}

async function runCronNow(job: any) {
  try {
    await cronApi.run(job.id)
    ElMessage.success('已触发运行')
    setTimeout(loadCron, 2000)
  } catch {
    ElMessage.error('运行失败')
  }
}

async function deleteCron(job: any) {
  try {
    await cronApi.delete(job.id)
    ElMessage.success('已删除')
    loadCron()
  } catch {
    ElMessage.error('删除失败')
  }
}
</script>

<style scoped>
.agent-detail {
  min-height: 100vh;
  background: #f5f7fa;
}
.detail-header {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.header-left h2 { margin: 0; }

/* Chat */
.chat-container {
  display: flex;
  flex-direction: column;
  height: 600px;
}
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  background: #fafafa;
  border-radius: 8px;
}
.chat-msg {
  display: flex;
  margin-bottom: 12px;
}
.chat-msg.user {
  justify-content: flex-end;
}
.chat-msg.assistant, .chat-msg.tool {
  justify-content: flex-start;
}
.msg-bubble {
  max-width: 70%;
  padding: 10px 14px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
}
.chat-msg.user .msg-bubble {
  background: #409EFF;
  color: #fff;
  border-bottom-right-radius: 4px;
}
.chat-msg.assistant .msg-bubble {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-bottom-left-radius: 4px;
}
.chat-msg.tool .msg-bubble {
  background: #f0f9eb;
  border: 1px solid #e1f3d8;
  width: 100%;
  max-width: 100%;
}
.tool-block { font-size: 13px; }
.tool-result { white-space: pre-wrap; font-size: 12px; max-height: 200px; overflow-y: auto; }
.cursor { animation: blink 1s infinite; color: #409EFF; }
@keyframes blink { 0%,100% { opacity: 1 } 50% { opacity: 0 } }

/* Typing indicator */
.typing-indicator {
  display: flex;
  gap: 4px;
  padding: 4px 0;
}
.typing-indicator span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #909399;
  animation: typing 1.4s infinite;
}
.typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.4s; }
@keyframes typing {
  0%, 100% { opacity: 0.3; transform: scale(0.8); }
  50% { opacity: 1; transform: scale(1); }
}

.chat-input {
  padding: 12px 0 0;
}
.msg-text :deep(code) {
  background: rgba(0,0,0,0.06);
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 13px;
}

/* Memory timeline */
.memory-card {
  cursor: pointer;
}
.memory-card:hover {
  border-color: #409EFF;
}
</style>
