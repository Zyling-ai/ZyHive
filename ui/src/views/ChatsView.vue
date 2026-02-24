<template>
  <div class="chats-page">

    <!-- ── 顶部标题 + 操作 ── -->
    <div class="page-header">
      <div>
        <h2 style="margin:0">对话管理</h2>
        <div style="font-size:13px;color:#909399;margin-top:2px">所有渠道对话与面板会话统一视图</div>
      </div>
      <el-button :loading="loading" :icon="Refresh" @click="loadAll">刷新</el-button>
    </div>

    <!-- ── 筛选栏 ── -->
    <div class="filter-bar">
      <!-- 类型 -->
      <el-select v-model="filterType" placeholder="全部类型" clearable size="small" style="width:130px">
        <el-option label="渠道对话" value="channel" />
        <el-option label="面板会话" value="session" />
      </el-select>

      <!-- AI 成员 -->
      <el-select v-model="filterAgent" placeholder="全部成员" clearable size="small" style="width:140px"
        @change="loadAll">
        <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
      </el-select>

      <!-- 渠道类型（仅渠道有效） -->
      <el-select v-model="filterChannel" placeholder="全部渠道" clearable size="small" style="width:130px"
        @change="loadAll">
        <el-option label="Telegram" value="telegram" />
        <el-option label="Web 聊天" value="web" />
      </el-select>

      <!-- 关键词搜索 -->
      <el-input v-model="searchKw" placeholder="搜索标题 / ID / 成员…" clearable size="small"
        style="width:220px" :prefix-icon="Search" />

      <!-- 排序 -->
      <el-select v-model="sortBy" size="small" style="width:120px">
        <el-option label="最近活跃" value="lastAt" />
        <el-option label="消息最多" value="messageCount" />
        <el-option label="Token 最多" value="tokenEstimate" />
      </el-select>

      <!-- 统计 -->
      <span class="filter-count">共 {{ filteredRows.length }} 条</span>
    </div>

    <!-- ── 统一列表 ── -->
    <el-card shadow="never" class="list-card">
      <el-table
        :data="filteredRows"
        stripe
        v-loading="loading"
        @row-click="openDetail"
        style="cursor:pointer"
        :row-class-name="rowClassName"
        size="default"
      >
        <!-- 类型标识 -->
        <el-table-column label="类型" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.source === 'channel'"
              :type="row.channelType === 'telegram' ? 'success' : 'warning'"
              size="small" effect="plain">
              {{ row.channelType === 'telegram' ? 'TG' : 'Web' }}
            </el-tag>
            <el-tag v-else type="primary" size="small" effect="plain">面板</el-tag>
          </template>
        </el-table-column>

        <!-- 标题 / 渠道ID -->
        <el-table-column label="标题 / 渠道" min-width="240">
          <template #default="{ row }">
            <div class="row-title-cell">
              <span class="row-title">{{ row.title || row.channelId || '（无标题）' }}</span>
              <span class="row-id">{{ row.id }}</span>
            </div>
          </template>
        </el-table-column>

        <!-- AI 成员 -->
        <el-table-column label="成员" width="110">
          <template #default="{ row }">
            <div class="agent-cell">
              <span class="agent-dot" :style="{ background: agentColorMap[row.agentId] || '#6366f1' }" />
              {{ row.agentName }}
            </div>
          </template>
        </el-table-column>

        <!-- 消息数 -->
        <el-table-column label="消息" width="70" align="center">
          <template #default="{ row }">
            <span style="font-size:13px;font-weight:500">{{ row.messageCount }}</span>
          </template>
        </el-table-column>

        <!-- Token（仅面板会话有） -->
        <el-table-column label="Token" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.source === 'session' && row.tokenEstimate"
              :type="row.tokenEstimate > 60000 ? 'danger' : row.tokenEstimate > 30000 ? 'warning' : 'info'"
              size="small" effect="plain">
              {{ formatTokens(row.tokenEstimate) }}
            </el-tag>
            <span v-else style="color:#c0c4cc">—</span>
          </template>
        </el-table-column>

        <!-- 最后活跃 -->
        <el-table-column label="最后活跃" width="130">
          <template #default="{ row }">
            <span style="font-size:12px;color:#606266">{{ formatRelative(row.lastAt) }}</span>
          </template>
        </el-table-column>

        <!-- 创建时间 -->
        <el-table-column label="创建时间" width="120">
          <template #default="{ row }">
            <span style="font-size:12px;color:#909399">{{ formatDate(row.firstAt || row.createdAt) }}</span>
          </template>
        </el-table-column>

        <!-- 操作 -->
        <el-table-column label="操作" width="160" @click.stop>
          <template #default="{ row }">
            <el-button size="small" link @click.stop="openDetail(row)">查看</el-button>
            <template v-if="row.source === 'session'">
              <el-button size="small" link type="primary" @click.stop="continueSession(row)">继续</el-button>
              <el-popconfirm title="确认删除此对话？" @confirm="deleteSession(row)" width="180">
                <template #reference>
                  <el-button size="small" link type="danger" @click.stop>删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </template>
        </el-table-column>

        <template #empty>
          <el-empty description="暂无对话记录" />
        </template>
      </el-table>
    </el-card>

    <!-- ══════════ 渠道对话详情 Drawer ══════════════════════════════════ -->
    <el-drawer v-model="channelDrawer" size="50%" direction="rtl">
      <template #header>
        <div>
          <div style="font-weight:600;font-size:15px">
            {{ drawerChannelRow?.channelType === 'telegram' ? 'Telegram' : 'Web' }} · {{ drawerChannelRow?.channelId }}
          </div>
          <div style="font-size:12px;color:#909399;margin-top:3px">
            {{ drawerChannelRow?.agentName }} · {{ drawerChannelRow?.messageCount }} 条消息
          </div>
        </div>
      </template>

      <div v-if="channelDetailLoading" class="drawer-loading">
        <el-icon class="is-loading" size="32"><Loading /></el-icon>
      </div>
      <div v-else class="message-list">
        <div v-for="(msg, idx) in channelMessages" :key="idx" :class="['message-item', `msg-${msg.role}`]">
          <div class="msg-avatar">
            <el-avatar :size="30" :style="{ background: msg.role === 'user' ? '#409eff' : '#67c23a' }">
              {{ msg.role === 'user' ? '用' : 'AI' }}
            </el-avatar>
          </div>
          <div class="msg-body">
            <div class="msg-meta">
              <span class="msg-role">{{ msg.role === 'user' ? (msg.sender || '用户') : 'AI' }}</span>
              <span class="msg-time">{{ formatDate(new Date(msg.ts).getTime()) }}</span>
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

    <!-- ══════════ 面板会话详情 Drawer ══════════════════════════════════ -->
    <el-drawer v-model="sessionDrawer" size="50%" direction="rtl">
      <template #header>
        <div style="flex:1;min-width:0">
          <div v-if="!editingTitle" style="display:flex;align-items:center;gap:8px">
            <span style="font-weight:600;font-size:15px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
              {{ drawerSession?.title || '（无标题）' }}
            </span>
            <el-button :icon="EditPen" circle size="small" @click="startEditTitle" />
          </div>
          <div v-else style="display:flex;align-items:center;gap:6px">
            <el-input v-model="editTitle" size="small" style="flex:1" @keyup.enter="saveTitle" />
            <el-button type="primary" size="small" @click="saveTitle">保存</el-button>
            <el-button size="small" @click="editingTitle = false">取消</el-button>
          </div>
          <div style="font-size:12px;color:#909399;margin-top:3px">
            {{ drawerSession?.agentName }} · {{ drawerSession?.messageCount ?? 0 }} 条 · {{ formatTokens(drawerSession?.tokenEstimate ?? 0) }} tokens
          </div>
        </div>
      </template>

      <div v-if="detailLoading" class="drawer-loading">
        <el-icon class="is-loading" size="32"><Loading /></el-icon>
      </div>
      <div v-else class="message-list">
        <div v-for="(msg, idx) in detailMessages" :key="idx" :class="['message-item', `msg-${msg.role}`]">
          <div v-if="msg.isCompact" class="compact-marker">
            <el-divider><el-icon><Fold /></el-icon><span style="margin-left:6px;font-size:12px;color:#909399">以上内容已压缩</span></el-divider>
            <div class="compact-summary">{{ msg.text }}</div>
          </div>
          <template v-else>
            <div class="msg-avatar">
              <el-avatar :size="30" :style="{ background: msg.role === 'user' ? '#409eff' : '#67c23a' }">
                {{ msg.role === 'user' ? '用' : 'AI' }}
              </el-avatar>
            </div>
            <div class="msg-body">
              <div class="msg-meta">
                <span class="msg-role">{{ msg.role === 'user' ? '用户' : 'AI' }}</span>
                <span class="msg-time">{{ formatDate(msg.timestamp) }}</span>
              </div>
              <div v-if="msg.toolCalls?.length" class="hist-tool-timeline">
                <div v-for="tc in msg.toolCalls" :key="tc.id" class="hist-tool-step">
                  <span class="hist-tool-dot">✓</span>
                  <span class="hist-tool-name">{{ tc.name }}</span>
                  <span class="hist-tool-summary">{{ histToolSummary(tc.name, tc.input || '') }}</span>
                </div>
              </div>
              <div v-else class="msg-text" v-html="renderText(msg.text || '')" />
            </div>
          </template>
        </div>
        <el-empty v-if="!detailMessages.length && !detailLoading" description="暂无消息记录" />
      </div>

      <template #footer>
        <div style="display:flex;justify-content:flex-end;gap:8px">
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

// ── 统一行类型 ────────────────────────────────────────────────────────────
interface UnifiedRow {
  source: 'channel' | 'session'
  id: string
  agentId: string
  agentName: string
  messageCount: number
  lastAt: number
  firstAt?: number
  createdAt?: number
  // channel
  channelType?: string
  channelId?: string
  // session
  title?: string
  tokenEstimate?: number
  // original refs
  _channel?: GlobalConvRow
  _session?: SessionSummary
}

// ── 状态 ─────────────────────────────────────────────────────────────────
const agentList   = ref<AgentInfo[]>([])
const loading     = ref(false)
const allRows     = ref<UnifiedRow[]>([])

// 筛选
const filterType    = ref('')
const filterAgent   = ref('')
const filterChannel = ref('')
const searchKw      = ref('')
const sortBy        = ref('lastAt')

// ── 计算：筛选+排序 ───────────────────────────────────────────────────────
const filteredRows = computed(() => {
  let list = allRows.value

  if (filterType.value)  list = list.filter(r => r.source === filterType.value)
  if (filterAgent.value) list = list.filter(r => r.agentId === filterAgent.value)
  if (filterChannel.value) list = list.filter(r => r.channelType === filterChannel.value)

  if (searchKw.value) {
    const kw = searchKw.value.toLowerCase()
    list = list.filter(r =>
      (r.title || '').toLowerCase().includes(kw) ||
      (r.channelId || '').toLowerCase().includes(kw) ||
      r.id.toLowerCase().includes(kw) ||
      r.agentName.toLowerCase().includes(kw)
    )
  }

  const sorted = [...list]
  if (sortBy.value === 'messageCount') sorted.sort((a, b) => b.messageCount - a.messageCount)
  else if (sortBy.value === 'tokenEstimate') sorted.sort((a, b) => (b.tokenEstimate || 0) - (a.tokenEstimate || 0))
  else sorted.sort((a, b) => b.lastAt - a.lastAt)

  return sorted
})

const agentColorMap = computed(() => {
  const m: Record<string, string> = {}
  agentList.value.forEach(a => { m[a.id] = a.avatarColor || '#6366f1' })
  return m
})

// ── 数据加载 ─────────────────────────────────────────────────────────────
async function loadAll() {
  loading.value = true
  try {
    const [agRes, chRes, sesRes] = await Promise.all([
      agentsApi.list().catch(() => ({ data: [] as AgentInfo[] })),
      agentConversations.globalList({ agentId: filterAgent.value || undefined, channelType: filterChannel.value || undefined })
        .catch(() => ({ data: [] as GlobalConvRow[] })),
      sessionsApi.list({ agentId: filterAgent.value || undefined, limit: 300 })
        .catch(() => ({ data: { sessions: [] as SessionSummary[], total: 0 } })),
    ])
    agentList.value = agRes.data || []

    const agentNameMap: Record<string, string> = {}
    agentList.value.forEach(a => { agentNameMap[a.id] = a.name })

    // 渠道对话
    const chRows: UnifiedRow[] = (chRes.data || []).map(r => ({
      source: 'channel' as const,
      id: r.channelId,
      agentId: r.agentId,
      agentName: r.agentName || agentNameMap[r.agentId] || r.agentId,
      messageCount: r.messageCount,
      lastAt: typeof r.lastAt === 'string' ? new Date(r.lastAt).getTime() : r.lastAt,
      firstAt: typeof r.firstAt === 'string' ? new Date(r.firstAt).getTime() : r.firstAt,
      channelType: r.channelType,
      channelId: r.channelId,
      _channel: r,
    }))

    // 面板会话（过滤掉内部系统 session）
    const sesRows: UnifiedRow[] = (sesRes.data?.sessions || [])
      .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
      .map(s => ({
        source: 'session' as const,
        id: s.id,
        agentId: s.agentId,
        agentName: s.agentName || agentNameMap[s.agentId] || s.agentId,
        messageCount: s.messageCount,
        lastAt: typeof s.lastAt === 'string' ? new Date(s.lastAt).getTime() : (s.lastAt || 0),
        createdAt: typeof s.createdAt === 'string' ? new Date(s.createdAt).getTime() : (s.createdAt || 0),
        title: s.title,
        tokenEstimate: s.tokenEstimate,
        _session: s,
      }))

    allRows.value = [...chRows, ...sesRows]
  } catch (e: any) {
    ElMessage.error('加载失败')
  } finally {
    loading.value = false
  }
}

// ── 打开详情 ─────────────────────────────────────────────────────────────
function openDetail(row: UnifiedRow) {
  if (row.source === 'channel' && row._channel) {
    openChannelDetail(row._channel)
  } else if (row.source === 'session' && row._session) {
    openSessionDetail(row._session)
  }
}

function rowClassName({ row }: { row: UnifiedRow }) {
  if (row.source === 'session' && row._session && drawerSession.value?.id === row._session.id) return 'active-row'
  return ''
}

// ── 渠道对话详情 ─────────────────────────────────────────────────────────
const channelDrawer        = ref(false)
const drawerChannelRow     = ref<GlobalConvRow | null>(null)
const channelMessages      = ref<ConvEntry[]>([])
const channelDetailLoading = ref(false)
const channelTotal         = ref(0)
const channelPage          = ref(1)
const channelLimit         = 50

async function openChannelDetail(row: GlobalConvRow) {
  drawerChannelRow.value = row
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
  } catch { ElMessage.error('加载消息失败') }
  finally { channelDetailLoading.value = false }
}

async function onChannelPageChange(page: number) {
  channelPage.value = page
  if (drawerChannelRow.value) await fetchChannelMessages(drawerChannelRow.value, page)
}

// ── 面板会话详情 ─────────────────────────────────────────────────────────
const sessionDrawer  = ref(false)
const drawerSession  = ref<SessionSummary | null>(null)
const detailMessages = ref<ParsedMessage[]>([])
const detailLoading  = ref(false)
const editingTitle   = ref(false)
const editTitle      = ref('')

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
    ElMessage.error('加载对话失败')
  } finally {
    detailLoading.value = false
  }
}

function continueSession(row: SessionSummary) {
  if (!row) return
  router.push(`/agents/${row.agentId}?resumeSession=${row.id}`)
}

async function deleteSession(row: UnifiedRow) {
  if (!row._session) return
  try {
    await sessionsApi.delete(row._session.agentId, row._session.id)
    ElMessage.success('已删除')
    if (drawerSession.value?.id === row._session.id) sessionDrawer.value = false
    allRows.value = allRows.value.filter(r => r.id !== row.id || r.source !== 'session')
  } catch { ElMessage.error('删除失败') }
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
    const row = allRows.value.find(r => r.source === 'session' && r.id === drawerSession.value!.id)
    if (row) row.title = editTitle.value
    editingTitle.value = false
    ElMessage.success('已重命名')
  } catch { ElMessage.error('保存失败') }
}

// ── 辅助 ─────────────────────────────────────────────────────────────────
function histToolSummary(name: string, input: string): string {
  try {
    const p = JSON.parse(input)
    if (name === 'bash' || name === 'exec') return (p.command ?? '').slice(0, 40)
    if (name === 'read' || name === 'write') return (p.path ?? p.file_path ?? '').split('/').pop() ?? ''
    if (name === 'agent_spawn') return `→ ${p.agentId ?? '?'}: ${(p.task ?? '').slice(0, 30)}…`
    if (name === 'web_search') return (p.query ?? '').slice(0, 40)
  } catch {}
  return input.slice(0, 40)
}

function formatDate(ms: number | string | undefined): string {
  if (!ms) return '—'
  const d = typeof ms === 'string' ? new Date(ms) : new Date(ms)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function formatRelative(ms: number | string | undefined): string {
  if (!ms) return '—'
  const t = typeof ms === 'string' ? new Date(ms).getTime() : ms
  const diff = Date.now() - t
  if (diff < 60_000)          return '刚刚'
  if (diff < 3_600_000)       return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000)      return `${Math.floor(diff / 3_600_000)} 小时前`
  if (diff < 7 * 86_400_000)  return `${Math.floor(diff / 86_400_000)} 天前`
  return formatDate(t)
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

onMounted(() => loadAll())
</script>

<style scoped>
.chats-page { padding: 0; height: 100%; display: flex; flex-direction: column; }

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
  flex-shrink: 0;
}

/* 筛选栏 */
.filter-bar {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 12px;
  flex-shrink: 0;
}
.filter-count {
  font-size: 12px;
  color: #909399;
  margin-left: 4px;
}

/* 列表卡片 */
.list-card {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.list-card :deep(.el-card__body) {
  padding: 0;
  flex: 1;
  overflow: auto;
}

/* 行样式 */
.row-title-cell { display: flex; flex-direction: column; gap: 2px; }
.row-title { font-size: 13px; font-weight: 500; color: #303133; }
.row-id   { font-size: 11px; color: #c0c4cc; font-family: monospace; }

.agent-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #303133;
}
.agent-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

:deep(.active-row) { background: #ecf5ff !important; }
:deep(.el-table__row) { cursor: pointer; }

/* Drawer */
.drawer-loading { display: flex; align-items: center; justify-content: center; padding: 60px; }

/* 消息列表 */
.message-list { display: flex; flex-direction: column; gap: 16px; padding: 4px 0; }
.message-item { display: flex; gap: 10px; align-items: flex-start; }
.msg-user { flex-direction: row-reverse; }
.msg-user .msg-body { align-items: flex-end; }
.msg-user .msg-meta { flex-direction: row-reverse; }
.msg-avatar { flex-shrink: 0; }
.msg-body { display: flex; flex-direction: column; gap: 4px; max-width: 85%; }
.msg-meta { display: flex; gap: 8px; align-items: center; }
.msg-role { font-size: 12px; font-weight: 600; color: #606266; }
.msg-time { font-size: 11px; color: #c0c4cc; }
.msg-text {
  background: #f4f4f5;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
}
.msg-user .msg-text { background: #409eff; color: #fff; }
.msg-text :deep(pre.code-block) {
  background: rgba(0,0,0,0.08);
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  margin: 4px 0;
}
.msg-text :deep(code) { background: rgba(0,0,0,0.08); border-radius: 3px; padding: 1px 4px; font-size: 12px; }

/* 压缩摘要 */
.compact-marker { width: 100%; }
.compact-summary {
  background: #fdf6ec;
  border: 1px dashed #e6a23c;
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  color: #606266;
  line-height: 1.6;
  margin-top: 4px;
}

/* 工具调用时间线 */
.hist-tool-timeline { display: flex; flex-direction: column; gap: 3px; margin-bottom: 4px; }
.hist-tool-step {
  display: flex; align-items: center; gap: 6px;
  background: #f0faf0; border: 1px solid #b7eb8f;
  border-radius: 6px; padding: 3px 8px; font-size: 12px;
  max-width: 480px;
}
.hist-tool-dot  { color: #52c41a; font-weight: bold; }
.hist-tool-name { color: #237804; font-family: monospace; }
.hist-tool-summary { color: #606266; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 220px; }
</style>
