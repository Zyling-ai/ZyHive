<template>
  <div class="tool-audit-view">
    <div class="page-header">
      <h2>
        🔍 工具调用审计
        <el-text type="info" size="small" style="margin-left:8px;font-weight:400">
          每一次工具调用的完整 input / output（全部成员，按日切日志）
        </el-text>
      </h2>
      <el-button size="small" @click="reload">
        <el-icon><Refresh /></el-icon> 刷新
      </el-button>
    </div>

    <!-- Filter bar -->
    <div class="filter-bar">
      <el-select v-model="filterAgent" placeholder="全部成员" clearable size="small" style="width:180px">
        <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
      </el-select>
      <el-input v-model="filterSessionId" placeholder="Session ID 过滤" size="small" style="width:200px" clearable />
      <el-input v-model="filterTool" placeholder="工具名（如 read / exec）" size="small" style="width:180px" clearable />
      <el-date-picker v-model="filterDateFrom" type="date" placeholder="起始日期" size="small" style="width:140px" value-format="YYYY-MM-DD" />
      <el-date-picker v-model="filterDateTo" type="date" placeholder="截止日期" size="small" style="width:140px" value-format="YYYY-MM-DD" />
      <el-button size="small" type="primary" @click="reload">应用</el-button>
    </div>

    <!-- Stats -->
    <div class="stats-bar">
      <span>共 <strong>{{ total }}</strong> 条</span>
      <span>当前页 {{ entries.length }} 条</span>
      <span v-if="entries.length > 0 && entries[0]">
        最近一次：<code>{{ formatTs(entries[0].ts) }}</code>
      </span>
    </div>

    <!-- Table -->
    <el-table :data="entries" v-loading="loading" stripe size="small" style="width:100%" @row-click="openDrawer">
      <el-table-column label="时间" width="160">
        <template #default="{ row }">
          <span style="font-family: ui-monospace, monospace; font-size: 12px">{{ formatTs(row.ts) }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="agentName" label="成员" width="120" />
      <el-table-column prop="name" label="工具" width="160">
        <template #default="{ row }">
          <code style="color: #1e293b">{{ row.name }}</code>
        </template>
      </el-table-column>
      <el-table-column prop="sessionId" label="Session" min-width="180">
        <template #default="{ row }">
          <code style="font-size:11px;color:#94a3b8">{{ row.sessionId || '-' }}</code>
        </template>
      </el-table-column>
      <el-table-column label="耗时" width="90">
        <template #default="{ row }">
          <span :style="{ color: row.durationMs > 5000 ? '#f56c6c' : '#94a3b8' }">{{ row.durationMs }} ms</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.error" type="danger" size="small">失败</el-tag>
          <el-tag v-else type="success" size="small" effect="plain">OK</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="" width="60">
        <template #default="{ row }">
          <el-button size="small" link @click.stop="openDrawer(row)">详情</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Pagination -->
    <div class="pagination-bar">
      <el-pagination
        background
        layout="prev, pager, next, total"
        :total="total"
        :page-size="pageSize"
        :current-page="currentPage"
        @current-change="onPageChange"
      />
    </div>

    <!-- Detail drawer -->
    <el-drawer v-model="drawerOpen" :title="drawerTitle" direction="rtl" size="720px" destroy-on-close>
      <div v-if="drawerLoading" style="padding:20px;color:#94a3b8">加载中…</div>
      <div v-else-if="drawerError" style="padding:20px;color:#f56c6c">{{ drawerError }}</div>
      <div v-else-if="drawerData" class="tool-audit-drawer">
        <div class="ta-meta">
          <div><strong>工具：</strong> <code>{{ drawerData.name }}</code></div>
          <div><strong>耗时：</strong> {{ drawerData.durationMs ?? 0 }} ms</div>
          <div><strong>成员：</strong> {{ drawerData.agentId }}</div>
          <div v-if="drawerData.sessionId"><strong>Session：</strong> <code style="font-size:11px">{{ drawerData.sessionId }}</code></div>
          <div><strong>ID：</strong> <code style="font-size:11px">{{ drawerData.toolCallId }}</code></div>
          <div><strong>时间：</strong> <code style="font-size:11px">{{ formatTs(drawerData.ts) }}</code></div>
          <div v-if="drawerData.error" style="color:#f56c6c;grid-column:1/-1"><strong>错误：</strong> {{ drawerData.error }}</div>
        </div>
        <div class="ta-section">
          <div class="ta-label">
            <span>INPUT</span>
            <button class="ta-copy-btn" @click="copyText(formatVal(drawerData.input))">复制</button>
          </div>
          <pre class="ta-pre">{{ formatVal(drawerData.input) }}</pre>
        </div>
        <div class="ta-section">
          <div class="ta-label">
            <span>OUTPUT</span>
            <button class="ta-copy-btn" @click="copyText(drawerData.result || '')">复制</button>
            <span class="ta-size">{{ (drawerData.result?.length ?? 0).toLocaleString() }} chars</span>
          </div>
          <pre class="ta-pre result">{{ drawerData.result || '(空)' }}</pre>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { agents as agentsApi, type AgentInfo } from '../api'

interface AuditEntry {
  ts: number
  agentId: string
  agentName?: string
  sessionId?: string
  toolCallId: string
  name: string
  input?: any
  inputRef?: string
  result?: string
  resultRef?: string
  durationMs: number
  error?: string
}

const agentList = ref<AgentInfo[]>([])
const entries = ref<AuditEntry[]>([])
const total = ref(0)
const loading = ref(false)
const currentPage = ref(1)
const pageSize = ref(50)
const filterAgent = ref('')
const filterSessionId = ref('')
const filterTool = ref('')
const filterDateFrom = ref('')
const filterDateTo = ref('')

const drawerOpen = ref(false)
const drawerLoading = ref(false)
const drawerError = ref('')
const drawerData = ref<AuditEntry | null>(null)

const drawerTitle = computed(() => {
  if (!drawerData.value) return '工具调用详情'
  return `🔍 ${drawerData.value.name}（${drawerData.value.durationMs ?? 0} ms）`
})

async function loadAgents() {
  try {
    const res = await agentsApi.list()
    agentList.value = res.data || []
  } catch {}
}

async function reload(toPage = 1) {
  loading.value = true
  currentPage.value = toPage
  try {
    const offset = (toPage - 1) * pageSize.value
    const params = new URLSearchParams()
    if (filterAgent.value) params.set('agentId', filterAgent.value)
    if (filterSessionId.value) params.set('sessionId', filterSessionId.value)
    if (filterTool.value) params.set('tool', filterTool.value)
    if (filterDateFrom.value) params.set('dateFrom', filterDateFrom.value)
    if (filterDateTo.value) params.set('dateTo', filterDateTo.value)
    params.set('limit', String(pageSize.value))
    params.set('offset', String(offset))
    const token = localStorage.getItem('aipanel_token') || ''
    const resp = await fetch(`/api/tool-audit?${params}`, {
      headers: { 'Authorization': `Bearer ${token}` },
    })
    if (resp.ok) {
      const j = await resp.json()
      entries.value = j.entries || []
      total.value = j.total || 0
    } else {
      entries.value = []
      total.value = 0
    }
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  reload(p)
}

async function openDrawer(row: AuditEntry) {
  drawerOpen.value = true
  drawerLoading.value = true
  drawerError.value = ''
  drawerData.value = null
  try {
    const token = localStorage.getItem('aipanel_token') || ''
    const resp = await fetch(`/api/agents/${encodeURIComponent(row.agentId)}/tool-audit/${encodeURIComponent(row.toolCallId)}`, {
      headers: { 'Authorization': `Bearer ${token}` },
    })
    if (resp.ok) {
      drawerData.value = await resp.json()
    } else {
      const j = await resp.json().catch(() => ({}))
      drawerError.value = j.error || `HTTP ${resp.status}`
    }
  } catch (e: any) {
    drawerError.value = e?.message || '请求失败'
  } finally {
    drawerLoading.value = false
  }
}

function formatTs(ts: number) {
  if (!ts) return '-'
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

function formatVal(v: any) {
  if (v == null) return '(空)'
  if (typeof v === 'string') return v
  try { return JSON.stringify(v, null, 2) } catch { return String(v) }
}

async function copyText(s: string) {
  try { await navigator.clipboard.writeText(s) } catch {}
}

onMounted(async () => {
  await loadAgents()
  await reload()
})
</script>

<style scoped>
.tool-audit-view { padding: 20px 24px; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.page-header h2 { margin: 0; font-size: 18px; color: #1e293b; }
.filter-bar { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; align-items: center; }
.stats-bar {
  display: flex; gap: 18px;
  font-size: 12px; color: #94a3b8;
  margin-bottom: 8px;
  padding: 6px 12px;
  background: #f8f9fb;
  border-radius: 4px;
}
.stats-bar strong { color: #1e293b; font-weight: 600; }
.pagination-bar { margin-top: 14px; display: flex; justify-content: flex-end; }

.tool-audit-drawer { padding: 4px 12px 24px; }
.ta-meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 14px;
  padding: 8px 12px;
  background: #f6f8fa;
  border-radius: 6px;
  margin-bottom: 14px;
  font-size: 12px;
}
.ta-meta strong { color: #1e293b; font-weight: 600; margin-right: 4px; }
.ta-meta code { background: #fff; padding: 1px 4px; border-radius: 3px; font-size: 11px; }
.ta-section { margin-bottom: 16px; }
.ta-label {
  font-size: 11px; color: #64748b; font-weight: 600;
  margin-bottom: 6px; display: flex; align-items: center; gap: 8px;
}
.ta-label .ta-size { color: #94a3b8; font-weight: normal; margin-left: auto; }
.ta-copy-btn {
  border: 1px solid #d8dadf; background: #fff; color: #475569;
  padding: 2px 10px; border-radius: 10px; font-size: 11px; cursor: pointer;
}
.ta-copy-btn:hover { background: #f0f3f7; }
.ta-pre {
  background: #1e293b; color: #e2e8f0;
  padding: 10px 12px; border-radius: 6px;
  font-size: 12px; line-height: 1.5;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  white-space: pre-wrap; word-break: break-all;
  max-height: 520px; overflow: auto;
}
.ta-pre.result { background: #0f1729; }
</style>
