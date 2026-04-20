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

      <!-- 渠道来源（按 source 过滤）-->
      <el-select v-model="filterChannel" placeholder="全部渠道" clearable size="small" style="width:130px">
        <el-option label="飞书" value="feishu" />
        <el-option label="Telegram" value="telegram" />
        <el-option label="Web 聊天" value="web" />
        <el-option label="面板（本地）" value="panel" />
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
        <el-table-column label="渠道" width="90" align="center">
          <template #default="{ row }">
            <el-tag
              :type="tagFor(row.source).type"
              :class="['src-tag', 'src-' + row.source]"
              size="small" effect="plain">
              {{ tagFor(row.source).label }}
            </el-tag>
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
            <el-tag v-if="row.kind === 'session' && row.tokenEstimate"
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
            <template v-if="row.kind === 'session'">
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

    <!-- ══════════ 渠道对话详情 Drawer (统一用 AiChat 只读渲染) ══════════ -->
    <el-drawer v-model="channelDrawer" size="55%" direction="rtl" :with-header="false">
      <div class="drawer-wrap">
        <div class="drawer-hd">
          <div class="drawer-hd-main">
            <div class="drawer-hd-title">
              <el-tag :type="tagFor((drawerChannelRow?.channelType || 'web').toLowerCase()).type"
                size="small" effect="plain" :class="['src-tag', 'src-' + (drawerChannelRow?.channelType || 'web').toLowerCase()]">
                {{ tagFor((drawerChannelRow?.channelType || 'web').toLowerCase()).label }}
              </el-tag>
              <span class="drawer-hd-id">{{ drawerChannelRow?.channelId }}</span>
            </div>
            <div class="drawer-hd-sub">
              {{ drawerChannelRow?.agentName }} · {{ drawerChannelRow?.messageCount }} 条消息
            </div>
          </div>
          <el-button :icon="Close" circle size="small" @click="channelDrawer = false" />
        </div>
        <div v-if="channelDetailLoading" class="drawer-loading">
          <el-icon class="is-loading" size="32" color="#94a3b8"><Loading /></el-icon>
        </div>
        <div v-else class="drawer-chat-body">
          <AiChat
            v-if="channelDrawer && drawerChannelRow"
            :key="'ch-' + drawerChannelRow.channelId"
            :agent-id="drawerChannelRow.agentId"
            :read-only="true"
            :read-only-reason="`${tagFor((drawerChannelRow?.channelType||'web').toLowerCase()).label} 渠道 · ${drawerChannelRow.channelId} · 只读`"
            ref="channelAiChatRef"
          />
        </div>
        <div v-if="channelTotal > channelLimit" class="drawer-ft">
          <el-pagination
            :current-page="channelPage" :page-size="channelLimit" :total="channelTotal"
            layout="prev, pager, next" @current-change="onChannelPageChange" small />
        </div>
      </div>
    </el-drawer>

    <!-- ══════════ 面板会话详情 Drawer (统一用 AiChat 只读渲染) ══════════ -->
    <el-drawer v-model="sessionDrawer" size="55%" direction="rtl" :with-header="false">
      <div class="drawer-wrap">
        <div class="drawer-hd">
          <div class="drawer-hd-main">
            <div v-if="!editingTitle" class="drawer-hd-title">
              <el-tag :type="tagFor(drawerSessionSource).type" size="small" effect="plain"
                :class="['src-tag', 'src-' + drawerSessionSource]">
                {{ tagFor(drawerSessionSource).label }}
              </el-tag>
              <span class="drawer-hd-title-text">{{ drawerSession?.title || '（无标题）' }}</span>
              <el-button :icon="EditPen" circle size="small" @click="startEditTitle" />
            </div>
            <div v-else class="drawer-hd-title">
              <el-input v-model="editTitle" size="small" style="flex:1;max-width:360px" @keyup.enter="saveTitle" />
              <el-button type="primary" size="small" @click="saveTitle">保存</el-button>
              <el-button size="small" @click="editingTitle = false">取消</el-button>
            </div>
            <div class="drawer-hd-sub">
              {{ drawerSession?.agentName }} · {{ drawerSession?.messageCount ?? 0 }} 条 · {{ formatTokens(drawerSession?.tokenEstimate ?? 0) }} tokens
            </div>
          </div>
          <el-button :icon="Close" circle size="small" @click="sessionDrawer = false" />
        </div>
        <div v-if="detailLoading" class="drawer-loading">
          <el-icon class="is-loading" size="32" color="#94a3b8"><Loading /></el-icon>
        </div>
        <div v-else class="drawer-chat-body">
          <AiChat
            v-if="sessionDrawer && drawerSession"
            :key="'sess-' + drawerSession.id"
            :agent-id="drawerSession.agentId"
            :read-only="true"
            :read-only-reason="`${tagFor(drawerSessionSource).label} · ${drawerSession.id} · 只读（如需继续请点右下角「继续对话」）`"
            ref="sessionAiChatRef"
          />
        </div>
        <div class="drawer-ft">
          <el-button @click="sessionDrawer = false">关闭</el-button>
          <el-button type="primary" :icon="ChatLineRound" @click="continueSession(drawerSession!)" :disabled="!drawerSession">
            继续对话
          </el-button>
        </div>
      </div>
    </el-drawer>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Search, EditPen, Loading, ChatLineRound, Close } from '@element-plus/icons-vue'
import {
  sessions as sessionsApi, agents as agentsApi, agentConversations,
  type SessionSummary, type ParsedMessage, type AgentInfo, type GlobalConvRow, type ConvEntry
} from '../api'
import AiChat, { type ChatMsg } from '../components/AiChat.vue'

const router = useRouter()

// ── 统一行类型 ────────────────────────────────────────────────────────────
interface UnifiedRow {
  /** channel = 渠道会话记录（convlog）, session = 面板/agent 会话 */
  kind: 'channel' | 'session'
  /** 实际来源："feishu" | "telegram" | "web" | "panel" */
  source: string
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

// source → tag 配置
type SrcTagType = 'primary' | 'success' | 'warning' | 'info'
interface SrcTag { label: string; type: SrcTagType }
function tagFor(source: string): SrcTag {
  switch ((source || '').toLowerCase()) {
    case 'feishu':   return { label: '飞书', type: 'primary' }
    case 'telegram': return { label: 'TG',   type: 'success' }
    case 'web':      return { label: 'Web',  type: 'warning' }
    default:         return { label: '面板', type: 'info'    }
  }
}

function normalizeSessionSource(raw: string | undefined, sessionId: string): string {
  const s = (raw || '').toLowerCase()
  if (s === 'feishu' || s === 'telegram' || s === 'web') return s
  if (sessionId.startsWith('feishu-')) return 'feishu'
  if (sessionId.startsWith('tg-')) return 'telegram'
  if (sessionId.startsWith('web-')) return 'web'
  return 'panel'
}

function sessionLabelFromId(id: string): string {
  if (id.startsWith('feishu-')) {
    const rest = id.slice(7)
    return '飞书 · ' + (rest.length > 14 ? rest.slice(0, 12) + '…' : rest)
  }
  if (id.startsWith('tg-')) return 'Telegram · ' + id.slice(3, 11)
  if (id.startsWith('web-')) return '网页 · ' + id.slice(4, 12)
  return ''
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

  if (filterType.value)  list = list.filter(r => r.kind === filterType.value)
  if (filterAgent.value) list = list.filter(r => r.agentId === filterAgent.value)
  if (filterChannel.value) list = list.filter(r => r.source === filterChannel.value)

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
      // globalList 的 channelType 只认 telegram/web；feishu 和 panel 走 sessions 分支，前端统一过滤
      agentConversations.globalList({ agentId: filterAgent.value || undefined, channelType: (filterChannel.value === 'telegram' || filterChannel.value === 'web') ? filterChannel.value : undefined })
        .catch(() => ({ data: [] as GlobalConvRow[] })),
      sessionsApi.list({ agentId: filterAgent.value || undefined, limit: 300 })
        .catch(() => ({ data: { sessions: [] as SessionSummary[], total: 0 } })),
    ])
    agentList.value = agRes.data || []

    const agentNameMap: Record<string, string> = {}
    agentList.value.forEach(a => { agentNameMap[a.id] = a.name })

    // 渠道对话（channelType 就是 source）
    const chRows: UnifiedRow[] = (chRes.data || []).map(r => ({
      kind: 'channel' as const,
      source: (r.channelType || 'web').toLowerCase(),
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
    // 注意：source 从 session.source 取（后端已写入），缺失时用 ID 前缀兜底
    const sesRows: UnifiedRow[] = (sesRes.data?.sessions || [])
      .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
      .map(s => ({
        kind: 'session' as const,
        source: normalizeSessionSource(s.source, s.id),
        id: s.id,
        agentId: s.agentId,
        agentName: s.agentName || agentNameMap[s.agentId] || s.agentId,
        messageCount: s.messageCount,
        lastAt: typeof s.lastAt === 'string' ? new Date(s.lastAt).getTime() : (s.lastAt || 0),
        createdAt: typeof s.createdAt === 'string' ? new Date(s.createdAt).getTime() : (s.createdAt || 0),
        title: s.title || sessionLabelFromId(s.id),
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
  if (row.kind === 'channel' && row._channel) {
    openChannelDetail(row._channel)
  } else if (row.kind === 'session' && row._session) {
    openSessionDetail(row._session)
  }
}

function rowClassName({ row }: { row: UnifiedRow }) {
  if (row.kind === 'session' && row._session && drawerSession.value?.id === row._session.id) return 'active-row'
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

// 把 ConvEntry / ParsedMessage 转成 AiChat 接受的 ChatMsg 结构
// 过滤掉空消息（只有 avatar 没正文 + 没工具）
function toChatMsgs(raws: { role: string; text?: string; content?: string; sender?: string; toolCalls?: any[]; isCompact?: boolean }[]): ChatMsg[] {
  const out: ChatMsg[] = []
  for (const m of raws) {
    if (m.isCompact) {
      out.push({ role: 'system', text: '— 以上内容已压缩 —' })
      continue
    }
    const text = (m.text ?? m.content ?? '').trim()
    const tools = (m.toolCalls || []).map((tc: any) => ({
      id: tc.id, name: tc.name, input: tc.input, result: tc.result,
      status: 'done' as const, _expanded: false,
    }))
    // 真·空消息（没文字 + 没工具 + 没图）→ 跳过, 避免空气泡
    if (!text && tools.length === 0) continue
    const role = (m.role === 'user' || m.role === 'assistant') ? m.role : 'assistant'
    // 渠道消息 (convlog) 有 sender 时前缀标明
    const prefixedText = role === 'user' && m.sender ? `[${m.sender}] ${text}` : text
    out.push({ role, text: prefixedText, toolCalls: tools.length ? tools : undefined })
  }
  return out
}

const channelAiChatRef = ref<any>(null)

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
    const raw = (res.data.messages || []).filter(m => !isSystemSignalMsg(m.content || ''))
    channelMessages.value = raw
    channelTotal.value = res.data.total
    // 装进 AiChat 只读渲染
    await nextTick()
    channelAiChatRef.value?.loadHistoryMessages(toChatMsgs(raw as any))
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

const sessionAiChatRef = ref<any>(null)

// 当前抽屉中 session 的来源（飞书/TG/Web/面板）
const drawerSessionSource = computed(() => {
  if (!drawerSession.value) return 'panel'
  return normalizeSessionSource((drawerSession.value as any).source, drawerSession.value.id)
})

async function openSessionDetail(row: SessionSummary) {
  drawerSession.value = row
  sessionDrawer.value = true
  editingTitle.value = false
  detailMessages.value = []
  detailLoading.value = true
  try {
    const res = await sessionsApi.get(row.agentId, row.id)
    const raw = (res.data.messages || []).filter(m => !isSystemSignalMsg(m.text || ''))
    detailMessages.value = raw
    // 装进 AiChat 只读渲染（工具卡可折叠展开、markdown 代码高亮、空气泡自动过滤）
    await nextTick()
    sessionAiChatRef.value?.loadHistoryMessages(toChatMsgs(raw as any))
  } catch (e: any) {
    ElMessage.error('加载对话失败')
  } finally {
    detailLoading.value = false
  }
}

function isSystemSignalMsg(text: string): boolean {
  const t = (text || '').trim()
  return t.startsWith('<task-notification>')
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
    allRows.value = allRows.value.filter(r => r.id !== row.id || r.kind !== 'session')
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
    const row = allRows.value.find(r => r.kind === 'session' && r.id === drawerSession.value!.id)
    if (row) row.title = editTitle.value
    editingTitle.value = false
    ElMessage.success('已重命名')
  } catch { ElMessage.error('保存失败') }
}

// ── 辅助 ─────────────────────────────────────────────────────────────────
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

/* 渠道 tag 专属色 */
.src-tag { font-weight: 600 !important; letter-spacing: 0.3px; }
.src-feishu   { background: #eef2ff !important; border-color: #c7d2fe !important; color: #4f46e5 !important; }
.src-telegram { background: #dcfce7 !important; border-color: #86efac !important; color: #15803d !important; }
.src-web      { background: #fef3c7 !important; border-color: #fcd34d !important; color: #b45309 !important; }
.src-panel    { background: #f1f5f9 !important; border-color: #cbd5e1 !important; color: #475569 !important; }
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

/* Drawer 布局 (统一抽屉内部容器) */
:deep(.el-drawer__body) { padding: 0 !important; overflow: hidden; }
.drawer-wrap { display: flex; flex-direction: column; height: 100%; background: #fafafa; }
.drawer-hd {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 18px;
  background: #fff;
  border-bottom: 1px solid #ececec;
  flex-shrink: 0;
}
.drawer-hd-main { flex: 1; min-width: 0; }
.drawer-hd-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.drawer-hd-title-text {
  font-weight: 600;
  font-size: 14px;
  color: #1e293b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.drawer-hd-id {
  font-size: 13px;
  color: #475569;
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.drawer-hd-sub {
  font-size: 11.5px;
  color: #94a3b8;
  margin-top: 4px;
}
.drawer-chat-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.drawer-ft {
  flex-shrink: 0;
  padding: 10px 16px;
  border-top: 1px solid #ececec;
  background: #fff;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.drawer-loading { display: flex; align-items: center; justify-content: center; padding: 60px; }
</style>
