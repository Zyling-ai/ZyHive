<template>
  <el-container class="agent-detail">
    <el-header class="detail-header">
      <div class="header-left">
        <el-button :icon="ArrowLeft" @click="$router.push('/agents')" circle />
        <h2>{{ agent?.name || '...' }}</h2>
        <el-tag :type="statusType(agent?.status)">{{ statusLabel(agent?.status) }}</el-tag>
      </div>
      <el-text type="info">{{ agent?.model }}</el-text>
    </el-header>

    <el-main>
      <el-tabs v-model="activeTab" type="border-card">
        <!-- Tab 1: Chat with session sidebar -->
        <el-tab-pane label="对话" name="chat">
          <div class="chat-layout">
            <!-- Session History Sidebar -->
            <div class="session-sidebar">
              <div class="session-sidebar-header">
                <span class="sidebar-title">历史对话</span>
                <el-button size="small" type="primary" plain @click="newSession" :icon="Plus">新建</el-button>
              </div>

              <div class="session-list" v-loading="sessionsLoading">
                <div
                  v-for="s in agentSessions"
                  :key="s.id"
                  :class="['session-item', { active: activeSessionId === s.id }]"
                  @click="resumeSession(s)"
                >
                  <div class="session-item-title">{{ s.title || '新对话' }}</div>
                  <div class="session-item-meta">
                    <span>{{ formatRelative(s.lastAt) }}</span>
                    <el-tag size="small" type="info" effect="plain" style="font-size: 10px; padding: 0 4px">
                      {{ s.messageCount }} 条
                    </el-tag>
                    <el-tag
                      v-if="s.tokenEstimate > 60000"
                      size="small"
                      type="warning"
                      effect="plain"
                      style="font-size: 10px; padding: 0 4px"
                    >~{{ Math.round(s.tokenEstimate / 1000) }}k</el-tag>
                  </div>
                </div>

                <div v-if="!sessionsLoading && !agentSessions.length" class="session-empty">
                  还没有对话记录
                </div>
              </div>

              <!-- @ 其他成员面板 -->
              <div class="at-panel">
                <el-button
                  size="small"
                  plain
                  class="at-toggle-btn"
                  @click="toggleAtPanel"
                >
                  <span class="at-icon">@</span> 其他成员
                </el-button>

                <!-- 内联转发表单 -->
                <div v-if="showAtPanel" class="at-form">
                  <el-select
                    v-model="atTargetId"
                    placeholder="选择成员"
                    size="small"
                    style="width: 100%; margin-bottom: 6px"
                    @change="onAtAgentSelect"
                  >
                    <el-option
                      v-for="a in otherAgents"
                      :key="a.id"
                      :label="a.name"
                      :value="a.id"
                    />
                  </el-select>

                  <el-input
                    v-model="atMessage"
                    type="textarea"
                    :rows="3"
                    placeholder="输入要转发的消息…"
                    size="small"
                    style="margin-bottom: 6px"
                  />

                  <el-button
                    type="primary"
                    size="small"
                    style="width: 100%"
                    :loading="atSending"
                    :disabled="!atTargetId || !atMessage.trim()"
                    @click="sendAtMessage"
                  >
                    转发
                  </el-button>
                </div>
              </div>
            </div>
          

            <!-- Chat Area -->
            <div class="chat-area">
              <AiChat
                ref="aiChatRef"
                :agent-id="agentId"
                :scenario="'agent-detail'"
                :welcome-message="`你好！我是 **${agent?.name || 'AI'}**，有什么可以帮你的？`"
                height="calc(100vh - 145px)"
                :show-thinking="true"
                @session-change="onSessionChange"
              />
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

        <!-- Tab 3: Relations -->
        <el-tab-pane label="关系" name="relations">
          <el-row :gutter="20">
            <!-- Left: editor -->
            <el-col :span="12">
              <el-card>
                <template #header>
                  <div style="display: flex; align-items: center; justify-content: space-between;">
                    <span>RELATIONS.md</span>
                    <el-button type="primary" size="small" @click="saveRelations" :loading="relationsSaving">保存</el-button>
                  </div>
                </template>
                <el-text type="info" size="small" style="display: block; margin-bottom: 8px; line-height: 1.6;">
                  格式：每行一条关系，五列：成员ID | 成员名称 | 关系类型（上级/下级/平级协作/支持）| 关系程度（核心/常用/偶尔）| 说明
                </el-text>
                <el-input
                  v-model="relationsContent"
                  type="textarea"
                  :rows="20"
                  style="font-family: monospace; font-size: 13px;"
                  @blur="saveRelations"
                />
              </el-card>
            </el-col>
            <!-- Right: preview -->
            <el-col :span="12">
              <el-card header="关系预览">
                <div v-if="parsedRelations.length === 0" style="text-align: center; color: #c0c4cc; padding: 40px 0;">
                  暂无关系数据，请在左侧编辑 RELATIONS.md
                </div>
                <div v-else class="relations-list">
                  <div v-for="row in parsedRelations" :key="row.agentId" class="relation-card">
                    <div class="relation-avatar" :style="{ background: avatarColor(row.agentId) }">
                      {{ row.agentId.charAt(0).toUpperCase() }}
                    </div>
                    <div class="relation-info">
                      <div class="relation-name">{{ row.agentName }}</div>
                      <div class="relation-tags">
                        <el-tag :type="relationTypeColor(row.relationType)" size="small">{{ row.relationType }}</el-tag>
                        <el-tag :type="strengthColor(row.strength)" size="small" effect="plain">{{ row.strength }}</el-tag>
                      </div>
                      <div class="relation-desc">{{ row.desc }}</div>
                    </div>
                  </div>
                </div>
              </el-card>
            </el-col>
          </el-row>
        </el-tab-pane>

        <!-- Tab 4: Memory Tree -->
        <el-tab-pane label="记忆" name="memory">
          <div class="memory-toolbar" style="margin-bottom: 12px; display: flex; gap: 8px;">
            <el-button type="primary" size="small" @click="showNewMemoryFile = true">
              <el-icon><Plus /></el-icon> 新建文件
            </el-button>
            <el-button size="small" @click="showDailyEntry = true">
              <el-icon><EditPen /></el-icon> 添加日志
            </el-button>
            <el-button size="small" @click="loadMemoryTree">
              <el-icon><Refresh /></el-icon> 刷新
            </el-button>
          </div>
          <el-row :gutter="16">
            <!-- Left: tree navigator (30%) -->
            <el-col :span="7">
              <el-card header="记忆目录" shadow="hover">
                <el-tree
                  :data="memoryTreeData"
                  :props="{ label: 'name', children: 'children', isLeaf: (d: any) => !d.isDir }"
                  @node-click="handleMemoryNodeClick"
                  highlight-current
                  default-expand-all
                  :expand-on-click-node="false"
                >
                  <template #default="{ data }">
                    <span style="display: flex; align-items: center; gap: 4px; font-size: 13px;">
                      <el-icon v-if="data.isDir" style="color: #E6A23C"><FolderOpened /></el-icon>
                      <el-icon v-else style="color: #409EFF"><Document /></el-icon>
                      <span>{{ data.name }}</span>
                      <el-text v-if="!data.isDir && data.size" type="info" size="small" style="margin-left: auto">
                        {{ formatSize(data.size) }}
                      </el-text>
                    </span>
                  </template>
                </el-tree>
                <el-empty v-if="memoryTreeData.length === 0" description="记忆树为空" :image-size="40" />
              </el-card>
            </el-col>
            <!-- Right: file editor (70%) -->
            <el-col :span="17">
              <el-card shadow="hover">
                <template #header>
                  <div style="display: flex; align-items: center; justify-content: space-between;">
                    <el-breadcrumb separator="/">
                      <el-breadcrumb-item>memory</el-breadcrumb-item>
                      <el-breadcrumb-item v-for="(seg, i) in memoryFileBreadcrumb" :key="i">{{ seg }}</el-breadcrumb-item>
                    </el-breadcrumb>
                    <el-button v-if="memoryEditPath" type="primary" size="small" @click="saveMemoryFile" :loading="memorySaving">保存</el-button>
                  </div>
                </template>
                <template v-if="memoryEditPath">
                  <el-input
                    v-model="memoryEditContent"
                    type="textarea"
                    :rows="22"
                    style="font-family: monospace;"
                  />
                </template>
                <template v-else>
                  <el-empty description="点击左侧文件查看和编辑" :image-size="60" />
                </template>
              </el-card>
            </el-col>
          </el-row>

          <!-- New memory file dialog -->
          <el-dialog v-model="showNewMemoryFile" title="新建记忆文件" width="480px">
            <el-form label-width="80px">
              <el-form-item label="路径">
                <el-input v-model="newMemoryPath" placeholder="例如: projects/my-project.md 或 topics/cooking.md" />
                <el-text type="info" size="small" style="margin-top: 4px">相对于 memory/ 目录</el-text>
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showNewMemoryFile = false">取消</el-button>
              <el-button type="primary" @click="createMemoryFile">创建</el-button>
            </template>
          </el-dialog>

          <!-- Daily log entry dialog -->
          <el-dialog v-model="showDailyEntry" title="添加今日日志" width="600px">
            <el-input
              v-model="dailyEntryContent"
              type="textarea"
              :rows="10"
              placeholder="记录今天的重要事项、学习心得、待办..."
            />
            <template #footer>
              <el-button @click="showDailyEntry = false">取消</el-button>
              <el-button type="primary" @click="submitDailyEntry">提交</el-button>
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
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, Plus, EditPen, Refresh, FolderOpened, Document } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { agents as agentsApi, files as filesApi, memoryApi, cron as cronApi, sessions as sessionsApi, relationsApi, type AgentInfo, type FileEntry, type CronJob, type SessionSummary, type RelationRow } from '../api'
import AiChat, { type ChatMsg } from '../components/AiChat.vue'

const route = useRoute()
const agentId = route.params.id as string
const agent = ref<AgentInfo | null>(null)
const activeTab = ref('chat')

// ── Session sidebar ────────────────────────────────────────────────────────
const aiChatRef = ref<InstanceType<typeof AiChat>>()
const agentSessions = ref<SessionSummary[]>([])
const sessionsLoading = ref(false)
const activeSessionId = ref<string | undefined>()

async function loadAgentSessions() {
  sessionsLoading.value = true
  try {
    const res = await sessionsApi.list({ agentId, limit: 50 })
    agentSessions.value = res.data.sessions
  } catch {}
  finally { sessionsLoading.value = false }
}

function resumeSession(s: SessionSummary) {
  activeSessionId.value = s.id
  aiChatRef.value?.resumeSession(s.id)
}

function newSession() {
  activeSessionId.value = undefined
  aiChatRef.value?.startNewSession()
}

function onSessionChange(sessionId: string) {
  activeSessionId.value = sessionId
  // Refresh session list to show new entry
  setTimeout(loadAgentSessions, 500)
}

function formatRelative(ms: number): string {
  if (!ms) return ''
  const diff = Date.now() - ms
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}分前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}小时前`
  return `${Math.floor(diff / 86_400_000)}天前`
}

// ── @ 其他成员 ─────────────────────────────────────────────────────────────
const showAtPanel   = ref(false)
const atTargetId    = ref('')
const atMessage     = ref('')
const atSending     = ref(false)
const otherAgents   = ref<AgentInfo[]>([])

function toggleAtPanel() {
  showAtPanel.value = !showAtPanel.value
  if (showAtPanel.value && !otherAgents.value.length) loadOtherAgents()
}

async function loadOtherAgents() {
  try {
    const res = await agentsApi.list()
    otherAgents.value = res.data.filter(a => a.id !== agentId)
  } catch {
    otherAgents.value = []
  }
}

function onAtAgentSelect(id: string) {
  // 同步在 AiChat 输入框填入 @AgentName: 前缀（方便用户知道当前 @ 模式）
  const target = otherAgents.value.find(a => a.id === id)
  if (target) {
    aiChatRef.value?.fillInput(`@${target.name}: `)
  }
}

async function sendAtMessage() {
  const targetId = atTargetId.value
  const msg = atMessage.value.trim()
  if (!targetId || !msg) return

  const targetAgent = otherAgents.value.find(a => a.id === targetId)
  const targetName  = targetAgent?.name ?? targetId

  atSending.value = true

  // 在对话区显示「转发」提示气泡
  const forwardBubble: ChatMsg = {
    role: 'user',
    text: `→ 转发给 ${targetName}：\n${msg}`,
  }
  aiChatRef.value?.appendMessage(forwardBubble)

  try {
    const res = await agentsApi.message(targetId, msg, agentId)
    const reply = res.data.response

    // 显示「回复」气泡
    const replyBubble: ChatMsg = {
      role: 'assistant',
      text: `← **${targetName}** 回复：\n\n${reply}`,
    }
    aiChatRef.value?.appendMessage(replyBubble)

    // 清空输入
    atMessage.value = ''
    atTargetId.value = ''
    showAtPanel.value = false
    ElMessage.success(`${targetName} 已回复`)
  } catch (e: any) {
    const errMsg: ChatMsg = {
      role: 'system',
      text: `❌ 转发失败：${e.response?.data?.error ?? e.message ?? '网络错误'}`,
    }
    aiChatRef.value?.appendMessage(errMsg)
    ElMessage.error('转发失败')
  } finally {
    atSending.value = false
  }
}

// Identity/Soul
const identityContent = ref('')
const soulContent = ref('')

// Memory tree
const memoryTreeData = ref<any[]>([])
const memoryEditPath = ref('')
const memoryEditContent = ref('')
const memorySaving = ref(false)
const memoryFileBreadcrumb = ref<string[]>([])
const showNewMemoryFile = ref(false)
const newMemoryPath = ref('')
const showDailyEntry = ref(false)
const dailyEntryContent = ref('')

// Workspace
const fileTreeData = ref<any[]>([])
const currentFile = ref('')
const currentFileContent = ref('')
const currentFileInfo = ref<FileEntry | null>(null)

// Relations
const relationsContent = ref('')
const parsedRelations = ref<RelationRow[]>([])
const relationsSaving = ref(false)

async function loadRelations() {
  try {
    const res = await relationsApi.get(agentId)
    relationsContent.value = res.data.content || ''
    parsedRelations.value = res.data.parsed || []
  } catch {
    relationsContent.value = ''
    parsedRelations.value = []
  }
}

async function saveRelations() {
  relationsSaving.value = true
  try {
    await relationsApi.put(agentId, relationsContent.value)
    // Re-parse after save
    const res = await relationsApi.get(agentId)
    parsedRelations.value = res.data.parsed || []
    ElMessage.success('RELATIONS.md 已保存')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    relationsSaving.value = false
  }
}

function avatarColor(id: string): string {
  const colors = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#909399', '#B45309', '#7C3AED', '#0891B2']
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length] ?? '#409EFF'
}

function relationTypeColor(type: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (type === '上级') return 'danger'
  if (type === '下级') return ''     // blue = default primary
  if (type === '平级协作') return 'success'
  return 'info'  // 支持
}

function strengthColor(s: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (s === '核心') return 'danger'
  if (s === '常用') return 'warning'
  return 'info'
}

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
  loadRelations()
  loadWorkspace()
  loadCron()
  await loadAgentSessions()

  // Handle ?resumeSession=<id> query param (from ChatsView 继续对话 button)
  const resumeId = route.query.resumeSession as string | undefined
  if (resumeId) {
    activeSessionId.value = resumeId
    // Give AiChat a tick to mount before calling resumeSession
    await new Promise(r => setTimeout(r, 100))
    aiChatRef.value?.resumeSession(resumeId)
    // Scroll the sidebar item into view by highlighting
    const target = agentSessions.value.find(s => s.id === resumeId)
    if (!target) {
      // Session not in list yet — still set active id so it highlights when list loads
      activeSessionId.value = resumeId
    }
  }
})

// Identity files
async function loadIdentityFiles() {
  try {
    const [id, soul] = await Promise.all([
      filesApi.read(agentId, 'IDENTITY.md'),
      filesApi.read(agentId, 'SOUL.md'),
    ])
    identityContent.value = id.data?.content || ''
    soulContent.value = soul.data?.content || ''
  } catch {}
  loadMemoryTree()
}

async function saveFile(name: string, content: string) {
  try {
    await filesApi.write(agentId, name, content)
    ElMessage.success(`${name} 已保存`)
  } catch {
    ElMessage.error(`保存 ${name} 失败`)
  }
}

// Memory tree functions
async function loadMemoryTree() {
  try {
    const res = await memoryApi.tree(agentId)
    memoryTreeData.value = res.data || []
  } catch {
    memoryTreeData.value = []
  }
}

async function handleMemoryNodeClick(data: any) {
  if (data.isDir) return
  memoryEditPath.value = data.path
  memoryFileBreadcrumb.value = data.path.split('/')
  try {
    const res = await memoryApi.readFile(agentId, data.path)
    memoryEditContent.value = res.data?.content || ''
  } catch {
    memoryEditContent.value = '(无法读取)'
  }
}

async function saveMemoryFile() {
  if (!memoryEditPath.value) return
  memorySaving.value = true
  try {
    await memoryApi.writeFile(agentId, memoryEditPath.value, memoryEditContent.value)
    ElMessage.success('记忆文件已保存')
    loadMemoryTree()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    memorySaving.value = false
  }
}

async function createMemoryFile() {
  const p = newMemoryPath.value.trim()
  if (!p) { ElMessage.warning('请输入路径'); return }
  try {
    await memoryApi.writeFile(agentId, p, `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`)
    ElMessage.success('文件已创建')
    showNewMemoryFile.value = false
    newMemoryPath.value = ''
    loadMemoryTree()
    // Open the new file
    memoryEditPath.value = p
    memoryFileBreadcrumb.value = p.split('/')
    memoryEditContent.value = `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`
  } catch {
    ElMessage.error('创建失败')
  }
}

async function submitDailyEntry() {
  const content = dailyEntryContent.value.trim()
  if (!content) { ElMessage.warning('请输入内容'); return }
  try {
    await memoryApi.dailyLog(agentId, content)
    ElMessage.success('日志已添加')
    showDailyEntry.value = false
    dailyEntryContent.value = ''
    loadMemoryTree()
  } catch {
    ElMessage.error('添加失败')
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

/* Chat + Session sidebar layout */
.chat-layout {
  display: flex;
  gap: 0;
  height: calc(100vh - 145px);
  overflow: hidden;
}

.session-sidebar {
  width: 220px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid #e4e7ed;
  background: #fafafa;
  overflow: hidden;
}

.session-sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid #e4e7ed;
  flex-shrink: 0;
}

.sidebar-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.session-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 4px;
}

.session-item {
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 2px;
  transition: background 0.15s;
}

.session-item:hover {
  background: #f0f2f5;
}

.session-item.active {
  background: #ecf5ff;
  border-left: 3px solid #409eff;
}

.session-item-title {
  font-size: 13px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
  margin-bottom: 4px;
}

.session-item-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: #909399;
}

.session-empty {
  text-align: center;
  color: #c0c4cc;
  font-size: 12px;
  padding: 20px 0;
}

.chat-area {
  flex: 1;
  overflow: hidden;
}

/* @ 其他成员面板 */
.at-panel {
  flex-shrink: 0;
  border-top: 1px solid #e4e7ed;
  padding: 8px 8px 10px;
  background: #f5f7fa;
}

.at-toggle-btn {
  width: 100%;
  justify-content: flex-start;
  color: #909399;
  font-size: 12px;
  border-color: #dcdfe6;
}

.at-toggle-btn:hover {
  color: #409eff;
  border-color: #b3d8ff;
  background: #ecf5ff;
}

.at-icon {
  font-weight: 700;
  color: #409eff;
  margin-right: 2px;
  font-size: 13px;
}

.at-form {
  margin-top: 8px;
}

/* Relations tab */
.relations-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.relation-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #fafafa;
}
.relation-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 14px;
  flex-shrink: 0;
}
.relation-info {
  flex: 1;
  min-width: 0;
}
.relation-name {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  margin-bottom: 4px;
}
.relation-tags {
  display: flex;
  gap: 6px;
  margin-bottom: 4px;
}
.relation-desc {
  font-size: 12px;
  color: #606266;
  line-height: 1.5;
}
</style>
