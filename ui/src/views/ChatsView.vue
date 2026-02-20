<template>
  <div class="chats-page">
    <!-- Header -->
    <div class="page-header">
      <div>
        <h2 style="margin: 0"><el-icon style="vertical-align:-2px;margin-right:6px"><ChatLineRound /></el-icon>对话管理</h2>
        <div style="font-size: 13px; color: #909399; margin-top: 4px">
          跨所有 AI 成员的对话记录
        </div>
      </div>
      <el-button @click="reload" :loading="loading" :icon="Refresh">刷新</el-button>
    </div>

    <el-tabs v-model="activeTab" @tab-change="reload">

      <!-- ── Tab 1: 渠道对话 ─────────────────────────────────────────── -->
      <el-tab-pane label="渠道对话" name="channel">
        <template #label>
          <span>渠道对话</span>
          <el-badge :value="channelRows.length" :hidden="!channelRows.length" type="primary" style="margin-left: 6px" />
        </template>

        <!-- Filter bar -->
        <div class="filter-bar">
          <el-select v-model="chFilterAgent" placeholder="全部成员" clearable style="width: 160px" @change="loadChannelConvs">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>

          <el-select v-model="chFilterType" placeholder="全部渠道类型" clearable style="width: 160px" @change="loadChannelConvs">
            <el-option label="Telegram" value="telegram" />
            <el-option label="Web 聊天页" value="web" />
          </el-select>

          <el-input v-model="chSearch" placeholder="搜索渠道 ID…" clearable style="width: 200px" :prefix-icon="Search" />
        </div>

        <el-card shadow="never" style="margin-top: 12px">
          <el-table :data="filteredChannelRows" stripe v-loading="loading" @row-click="openChannelDetail">
            <el-table-column label="AI 成员" width="130">
              <template #default="{ row }">
                <el-tag size="small" type="primary" effect="plain">{{ row.agentName }}</el-tag>
              </template>
            </el-table-column>

            <el-table-column label="渠道类型" width="110">
              <template #default="{ row }">
                <el-tag :type="row.channelType === 'telegram' ? 'success' : 'warning'" size="small">
                  {{ row.channelType === 'telegram' ? 'Telegram' : row.channelType === 'web' ? 'Web' : row.channelType }}
                </el-tag>
              </template>
            </el-table-column>

            <el-table-column label="渠道 ID" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">
                <span style="font-size: 12px; font-family: monospace; color: #606266">{{ row.channelId }}</span>
              </template>
            </el-table-column>

            <el-table-column label="消息数" width="90" align="center">
              <template #default="{ row }">
                <el-badge :value="row.messageCount" type="info" />
              </template>
            </el-table-column>

            <el-table-column label="首条消息" width="145">
              <template #default="{ row }">
                <span style="font-size: 13px; color: #909399">{{ formatTime(row.firstAt) }}</span>
              </template>
            </el-table-column>

            <el-table-column label="最后活跃" width="145">
              <template #default="{ row }">
                <span style="font-size: 13px">{{ formatRelative(row.lastAt) }}</span>
              </template>
            </el-table-column>

            <el-table-column label="操作" width="80">
              <template #default="{ row }">
                <el-button size="small" @click.stop="openChannelDetail(row)">查看</el-button>
              </template>
            </el-table-column>

            <template #empty>
              <el-empty description="暂无渠道对话记录" />
            </template>
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- ── Tab 2: 面板会话 ─────────────────────────────────────────── -->
      <el-tab-pane label="面板会话" name="session">
        <template #label>
          <span>面板会话</span>
          <el-badge :value="total" :hidden="!total" type="info" style="margin-left: 6px" />
        </template>

        <!-- Filter bar -->
        <div class="filter-bar">
          <el-select v-model="sesFilterAgent" placeholder="全部成员" clearable style="width: 160px" @change="loadSessions">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>

          <el-input v-model="sesSearch" placeholder="搜索对话标题…" clearable style="width: 220px" :prefix-icon="Search" />

          <el-select v-model="sortBy" style="width: 130px" @change="loadSessions">
            <el-option label="最近活跃" value="lastAt" />
            <el-option label="消息最多" value="messageCount" />
            <el-option label="Token 最多" value="tokenEstimate" />
          </el-select>
        </div>

        <el-card shadow="never" style="margin-top: 12px">
          <el-table :data="filteredSessions" stripe :row-class-name="rowClassName"
            @row-click="openSessionDetail" style="cursor: pointer" v-loading="loading">
            <el-table-column label="对话标题" min-width="220">
              <template #default="{ row }">
                <div style="display: flex; flex-direction: column; gap: 2px">
                  <span style="font-weight: 500; font-size: 14px">{{ row.title || '（无标题）' }}</span>
                  <span style="font-size: 12px; color: #909399">ID: {{ row.id }}</span>
                </div>
              </template>
            </el-table-column>

            <el-table-column label="AI 成员" width="130">
              <template #default="{ row }">
                <el-tag size="small" type="primary" effect="plain">{{ row.agentName || row.agentId }}</el-tag>
              </template>
            </el-table-column>

            <el-table-column label="消息数" width="90" align="center">
              <template #default="{ row }">
                <el-badge :value="row.messageCount" type="info" />
              </template>
            </el-table-column>

            <el-table-column label="Token 用量" width="130" align="center">
              <template #default="{ row }">
                <el-tag :type="row.tokenEstimate > 60000 ? 'danger' : row.tokenEstimate > 30000 ? 'warning' : 'success'"
                  size="small" effect="plain">{{ formatTokens(row.tokenEstimate) }}</el-tag>
              </template>
            </el-table-column>

            <el-table-column label="创建时间" width="145">
              <template #default="{ row }">
                <span style="font-size: 13px; color: #606266">{{ formatTime(row.createdAt) }}</span>
              </template>
            </el-table-column>

            <el-table-column label="最后活跃" width="145">
              <template #default="{ row }">
                <span style="font-size: 13px">{{ formatRelative(row.lastAt) }}</span>
              </template>
            </el-table-column>

            <el-table-column label="操作" width="190" @click.stop>
              <template #default="{ row }">
                <el-button size="small" @click.stop="openSessionDetail(row)">查看</el-button>
                <el-button size="small" type="primary" @click.stop="continueSession(row)">继续</el-button>
                <el-popconfirm title="确认删除此对话？" @confirm="deleteSession(row)" width="200">
                  <template #reference>
                    <el-button size="small" type="danger" @click.stop>删除</el-button>
                  </template>
                </el-popconfirm>
              </template>
            </el-table-column>

            <template #empty>
              <el-empty description="暂无面板会话" />
            </template>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- ── Channel Detail Drawer ──────────────────────────────────────── -->
    <el-drawer v-model="channelDrawer" :title="channelDrawerTitle" size="50%" direction="rtl">
      <template #header>
        <div>
          <div style="font-weight:600;font-size:16px">{{ channelDrawerTitle }}</div>
          <div style="font-size:12px;color:#909399;margin-top:4px">
            {{ drawerChannelRow?.agentName }} · {{ drawerChannelRow?.messageCount }} 条消息
          </div>
        </div>
      </template>

      <div v-if="channelDetailLoading" style="text-align:center;padding:40px">
        <el-icon class="is-loading" style="font-size:32px;color:#409eff"><Loading /></el-icon>
        <div style="margin-top:12px;color:#909399">加载中…</div>
      </div>
      <div v-else class="message-list">
        <div v-for="(msg, idx) in channelMessages" :key="idx" :class="['message-item', `msg-${msg.role}`]">
          <div class="msg-avatar">
            <el-avatar :size="32" :style="{ background: msg.role === 'user' ? '#409eff' : '#67c23a' }">
              {{ msg.role === 'user' ? '用' : 'AI' }}
            </el-avatar>
          </div>
          <div class="msg-body">
            <div class="msg-meta">
              <span class="msg-role">{{ msg.role === 'user' ? (msg.sender || '用户') : 'AI 助手' }}</span>
              <span class="msg-time">{{ formatTime(new Date(msg.ts).getTime()) }}</span>
            </div>
            <div class="msg-text" v-html="renderText(msg.content)" />
          </div>
        </div>
        <el-empty v-if="!channelMessages.length" description="暂无消息" />
      </div>

      <template #footer>
        <el-pagination v-if="channelTotal > channelLimit"
          :current-page="channelPage" :page-size="channelLimit" :total="channelTotal"
          layout="prev, pager, next" @current-change="onChannelPageChange" small />
      </template>
    </el-drawer>

    <!-- ── Session Detail Drawer ─────────────────────────────────────── -->
    <el-drawer v-model="sessionDrawer" :title="drawerSession?.title || '对话详情'" size="50%" direction="rtl">
      <template #header>
        <div style="flex:1;min-width:0">
          <div v-if="!editingTitle" style="display:flex;align-items:center;gap:8px">
            <span style="font-weight:600;font-size:16px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
              {{ drawerSession?.title || '（无标题）' }}
            </span>
            <el-button :icon="EditPen" circle size="small" @click="startEditTitle" />
          </div>
          <div v-else style="display:flex;align-items:center;gap:8px">
            <el-input v-model="editTitle" size="small" style="flex:1" @keyup.enter="saveTitle" />
            <el-button type="primary" size="small" @click="saveTitle">保存</el-button>
            <el-button size="small" @click="editingTitle = false">取消</el-button>
          </div>
          <div style="font-size:12px;color:#909399;margin-top:4px">
            {{ drawerSession?.agentName }} · {{ drawerSession?.messageCount ?? 0 }} 条消息 · {{ formatTokens(drawerSession?.tokenEstimate ?? 0) }} tokens
          </div>
        </div>
      </template>

      <div v-if="detailLoading" style="text-align:center;padding:40px">
        <el-icon class="is-loading" style="font-size:32px;color:#409eff"><Loading /></el-icon>
        <div style="margin-top:12px;color:#909399">加载对话历史…</div>
      </div>
      <div v-else class="message-list">
        <div v-for="(msg, idx) in detailMessages" :key="idx" :class="['message-item', `msg-${msg.role}`]">
          <div v-if="msg.isCompact" class="compact-marker">
            <el-divider><el-icon><Fold /></el-icon><span style="margin-left:6px;font-size:12px;color:#909399">以上内容已压缩</span></el-divider>
            <el-card class="compact-summary" shadow="never">
              <div style="font-size:12px;color:#606266;line-height:1.6"><strong>摘要：</strong>{{ msg.text }}</div>
            </el-card>
          </div>
          <template v-else>
            <div class="msg-avatar">
              <el-avatar :size="32" :style="{ background: msg.role === 'user' ? '#409eff' : '#67c23a' }">
                {{ msg.role === 'user' ? '用' : 'AI' }}
              </el-avatar>
            </div>
            <div class="msg-body">
              <div class="msg-meta">
                <span class="msg-role">{{ msg.role === 'user' ? '用户' : 'AI 助手' }}</span>
                <span class="msg-time">{{ formatTime(msg.timestamp) }}</span>
              </div>
              <div class="msg-text" v-html="renderText(msg.text)" />
            </div>
          </template>
        </div>
        <el-empty v-if="!detailMessages.length && !detailLoading" description="暂无消息记录" />
      </div>

      <template #footer>
        <div style="display:flex;justify-content:flex-end;gap:10px;padding:12px 0 0">
          <el-button @click="sessionDrawer = false">关闭</el-button>
          <el-button type="primary" :icon="ChatLineRound" @click="continueSession(drawerSession!)" :disabled="!drawerSession">
            继续对话
          </el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Search, EditPen, Loading, Fold, ChatLineRound } from '@element-plus/icons-vue'
import {
  sessions as sessionsApi, agents as agentsApi, agentConversations,
  type SessionSummary, type ParsedMessage, type AgentInfo, type GlobalConvRow, type ConvEntry
} from '../api'

const router = useRouter()

// ── Shared ────────────────────────────────────────────────────────────────
const agentList = ref<AgentInfo[]>([])
const loading = ref(false)
const activeTab = ref('channel')

function reload() {
  if (activeTab.value === 'channel') loadChannelConvs()
  else loadSessions()
}

// ── Tab 1: Channel Conversations ──────────────────────────────────────────
const channelRows = ref<GlobalConvRow[]>([])
const chFilterAgent = ref('')
const chFilterType = ref('')
const chSearch = ref('')

const filteredChannelRows = computed(() => {
  let rows = channelRows.value
  if (chSearch.value) {
    const kw = chSearch.value.toLowerCase()
    rows = rows.filter(r => r.channelId.toLowerCase().includes(kw) || r.agentName.toLowerCase().includes(kw))
  }
  return rows
})

async function loadChannelConvs() {
  loading.value = true
  try {
    const res = await agentConversations.globalList({
      agentId: chFilterAgent.value || undefined,
      channelType: chFilterType.value || undefined,
    })
    channelRows.value = res.data || []
  } catch {
    ElMessage.error('加载渠道对话失败')
  } finally {
    loading.value = false
  }
}

// Channel detail drawer
const channelDrawer = ref(false)
const channelDrawerTitle = ref('')
const drawerChannelRow = ref<GlobalConvRow | null>(null)
const channelMessages = ref<ConvEntry[]>([])
const channelDetailLoading = ref(false)
const channelTotal = ref(0)
const channelPage = ref(1)
const channelLimit = 50

async function openChannelDetail(row: GlobalConvRow) {
  drawerChannelRow.value = row
  channelDrawerTitle.value = `${row.channelType === 'telegram' ? 'Telegram' : 'Web'} · ${row.channelId}`
  channelDrawer.value = true
  channelPage.value = 1
  await fetchChannelMessages(row, 1)
}

async function fetchChannelMessages(row: GlobalConvRow, page: number) {
  channelDetailLoading.value = true
  try {
    const offset = (page - 1) * channelLimit
    const res = await agentConversations.messages(row.agentId, row.channelId, { limit: channelLimit, offset })
    channelMessages.value = res.data.messages || []
    channelTotal.value = res.data.total
  } catch {
    ElMessage.error('加载消息失败')
  } finally {
    channelDetailLoading.value = false
  }
}

async function onChannelPageChange(page: number) {
  channelPage.value = page
  if (drawerChannelRow.value) await fetchChannelMessages(drawerChannelRow.value, page)
}

// ── Tab 2: Panel Sessions ─────────────────────────────────────────────────
const sessionList = ref<SessionSummary[]>([])
const total = ref(0)
const sesFilterAgent = ref('')
const sesSearch = ref('')
const sortBy = ref('lastAt')

// Session detail drawer
const sessionDrawer = ref(false)
const drawerSession = ref<SessionSummary | null>(null)
const detailMessages = ref<ParsedMessage[]>([])
const detailLoading = ref(false)
const editingTitle = ref(false)
const editTitle = ref('')

const filteredSessions = computed(() => {
  let list = sessionList.value
  if (sesSearch.value) {
    const kw = sesSearch.value.toLowerCase()
    list = list.filter(s =>
      (s.title || '').toLowerCase().includes(kw) ||
      s.id.toLowerCase().includes(kw) ||
      (s.agentName || '').toLowerCase().includes(kw)
    )
  }
  if (sortBy.value === 'messageCount') list = [...list].sort((a, b) => b.messageCount - a.messageCount)
  else if (sortBy.value === 'tokenEstimate') list = [...list].sort((a, b) => b.tokenEstimate - a.tokenEstimate)
  return list
})

async function loadSessions() {
  loading.value = true
  try {
    const res = await sessionsApi.list({ agentId: sesFilterAgent.value || undefined, limit: 200 })
    sessionList.value = res.data.sessions
    total.value = res.data.total
  } catch (e: any) {
    ElMessage.error('加载失败：' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

async function openSessionDetail(row: SessionSummary) {
  drawerSession.value = row
  sessionDrawer.value = true
  editingTitle.value = false
  detailMessages.value = []
  detailLoading.value = true
  try {
    const res = await sessionsApi.get(row.agentId, row.id)
    detailMessages.value = res.data.messages
  } catch (e: any) {
    ElMessage.error('加载对话失败：' + (e.message || ''))
  } finally {
    detailLoading.value = false
  }
}

function continueSession(row: SessionSummary) {
  if (!row) return
  router.push(`/agents/${row.agentId}?resumeSession=${row.id}`)
}

async function deleteSession(row: SessionSummary) {
  try {
    await sessionsApi.delete(row.agentId, row.id)
    ElMessage.success('已删除')
    if (drawerSession.value?.id === row.id) sessionDrawer.value = false
    loadSessions()
  } catch (e: any) {
    ElMessage.error('删除失败：' + (e.message || ''))
  }
}

function startEditTitle() {
  editTitle.value = drawerSession.value?.title || ''
  editingTitle.value = true
}

async function saveTitle() {
  if (!drawerSession.value) return
  try {
    await sessionsApi.rename(drawerSession.value.agentId, drawerSession.value.id, editTitle.value)
    drawerSession.value.title = editTitle.value
    const idx = sessionList.value.findIndex(s => s.id === drawerSession.value!.id)
    if (idx >= 0) sessionList.value[idx]!.title = editTitle.value
    editingTitle.value = false
    ElMessage.success('已重命名')
  } catch (e: any) {
    ElMessage.error('保存失败：' + (e.message || ''))
  }
}

function rowClassName({ row }: { row: SessionSummary }) {
  return drawerSession.value?.id === row.id ? 'active-row' : ''
}

// ── Formatting ────────────────────────────────────────────────────────────
function formatTime(ms: number | string): string {
  if (!ms) return '—'
  const d = typeof ms === 'string' ? new Date(ms) : new Date(ms)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function formatRelative(ms: number | string): string {
  if (!ms) return '—'
  const t = typeof ms === 'string' ? new Date(ms).getTime() : ms
  const diff = Date.now() - t
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`
  return formatTime(t)
}

function formatTokens(n: number): string {
  if (!n) return '0'
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}

function renderText(text: string): string {
  return text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/```[\s\S]*?```/g, m => `<pre class="code-block">${m.slice(3, -3)}</pre>`)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/\n/g, '<br>')
}

onMounted(async () => {
  const res = await agentsApi.list().catch(() => ({ data: [] as AgentInfo[] }))
  agentList.value = res.data || []
  loadChannelConvs()
})
</script>

<style scoped>
.chats-page { padding: 0; }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.filter-bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; padding: 4px 0 0; }

.message-list { display: flex; flex-direction: column; gap: 20px; padding: 8px 0; }
.message-item { display: flex; gap: 12px; align-items: flex-start; }
.msg-user { flex-direction: row-reverse; }
.msg-user .msg-body { align-items: flex-end; }
.msg-user .msg-meta { flex-direction: row-reverse; }
.msg-avatar { flex-shrink: 0; }
.msg-body { display: flex; flex-direction: column; gap: 4px; max-width: 85%; }
.msg-meta { display: flex; gap: 8px; align-items: center; }
.msg-role { font-size: 12px; font-weight: 600; color: #606266; }
.msg-time { font-size: 11px; color: #c0c4cc; }
.msg-text { background: #f4f4f5; border-radius: 8px; padding: 10px 14px; font-size: 14px; line-height: 1.6; word-break: break-word; }
.msg-user .msg-text { background: #409eff; color: #fff; }
.msg-text :deep(pre.code-block) { background: rgba(0,0,0,0.08); border-radius: 4px; padding: 8px; font-size: 12px; overflow-x: auto; white-space: pre-wrap; margin: 6px 0; }
.msg-text :deep(code) { background: rgba(0,0,0,0.08); border-radius: 3px; padding: 1px 4px; font-size: 12px; }
.compact-marker { width: 100%; }
.compact-summary { margin-top: 8px; background: #fdf6ec; border: 1px dashed #e6a23c; }
:deep(.active-row) { background: #ecf5ff !important; }
</style>
