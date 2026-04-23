<template>
  <div class="usage-studio">
    <!-- 过滤栏 -->
    <div class="usage-filter">
      <el-space wrap>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="~"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          :shortcuts="dateShortcuts"
          value-format="x"
          style="width: 260px"
          @change="load"
        />
        <el-select
          v-model="filterProvider"
          clearable
          placeholder="全部厂商"
          style="width: 130px"
          @change="load"
        >
          <el-option v-for="p in providerOptions" :key="p" :label="p" :value="p" />
        </el-select>
        <el-select
          v-model="filterAgent"
          clearable
          placeholder="全部成员"
          style="width: 160px"
          @change="load"
        >
          <el-option v-for="a in agentOptions" :key="a.id" :label="a.name" :value="a.id" />
        </el-select>
        <el-select
          v-model="filterSession"
          clearable
          filterable
          placeholder="全部 Session"
          style="width: 260px"
          @change="() => { page = 1; loadRecords() }"
        >
          <el-option
            v-for="s in sessionOptions"
            :key="s.id"
            :label="s.label"
            :value="s.id"
          />
        </el-select>
        <el-button :loading="loading" :icon="Refresh" @click="load">刷新</el-button>
      </el-space>
    </div>

    <!-- 汇总卡片 -->
    <div class="stat-cards">
      <div class="stat-card">
        <div class="stat-label">API 调用次数</div>
        <div class="stat-value">{{ (summary.total_calls ?? 0).toLocaleString() }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">输入 Token</div>
        <div class="stat-value">{{ fmtTokens(summary.input_tokens ?? 0) }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">输出 Token</div>
        <div class="stat-value">{{ fmtTokens(summary.output_tokens ?? 0) }}</div>
      </div>
      <div class="stat-card highlight">
        <div class="stat-label">预计花费 (USD)</div>
        <div class="stat-value">${{ (summary.total_cost ?? 0).toFixed(4) }}</div>
      </div>
    </div>

    <!-- 图表区 -->
    <div class="usage-charts">
      <el-card class="chart-card" shadow="never">
        <template #header><span class="card-title">每日调用趋势</span></template>
        <div ref="timelineChartEl" class="chart-area" />
      </el-card>
      <div class="pie-col">
        <el-card class="chart-card-sm" shadow="never">
          <template #header><span class="card-title">厂商分布</span></template>
          <div ref="providerChartEl" class="chart-area-sm" />
        </el-card>
        <el-card class="chart-card-sm" shadow="never">
          <template #header><span class="card-title">成员用量</span></template>
          <div ref="agentChartEl" class="chart-area-sm" />
        </el-card>
      </div>
    </div>

    <!-- 明细表格 -->
    <el-card class="records-card" shadow="never">
      <template #header><span class="card-title">调用明细</span></template>
      <el-table
        :data="records"
        v-loading="loadingRecords"
        size="small"
        stripe
        style="width: 100%"
      >
        <el-table-column prop="created_at" label="时间" width="160"
          :formatter="(r: any) => new Date(r.created_at * 1000).toLocaleString('zh-CN')" />
        <el-table-column label="成员" width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="col-agent-name">{{ row.agentName || row.agent_id }}</span>
            <span v-if="row.agentName" class="col-agent-id">{{ row.agent_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="Session" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div v-if="row.sessionTitle" class="col-session">
              <span class="col-session-title">{{ row.sessionTitle }}</span>
              <span class="col-session-id">{{ shortSid(row.session_id) }}</span>
            </div>
            <span v-else-if="row.session_id" class="col-session-id">{{ shortSid(row.session_id) }}</span>
            <span v-else class="col-session-none">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="provider" label="厂商" width="110">
          <template #default="{ row }">
            <el-tag :type="providerColor(row.provider)" size="small">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="model" label="模型" width="220" show-overflow-tooltip />
        <el-table-column prop="input_tokens" label="输入 Token" width="110"
          :formatter="(r: any) => fmtTokens(r.input_tokens)" />
        <el-table-column prop="output_tokens" label="输出 Token" width="110"
          :formatter="(r: any) => fmtTokens(r.output_tokens)" />
        <el-table-column prop="cost" label="费用 (USD)" width="110"
          :formatter="(r: any) => '$' + (r.cost ?? 0).toFixed(5)" />
      </el-table>
      <div class="table-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="totalRecords"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          background
          small
          @current-change="loadRecords"
          @size-change="() => { page = 1; loadRecords() }"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { usageApi, agents as agentsApi, sessions as sessionsApi, type SessionSummary } from '../api'
import * as echarts from 'echarts/core'
import { LineChart, PieChart, BarChart } from 'echarts/charts'
import {
  GridComponent, TooltipComponent, LegendComponent, DataZoomComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([LineChart, PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer])

// ── state ──────────────────────────────────────────────────────────
const loading = ref(false)
const loadingRecords = ref(false)
const dateRange = ref<[number, number] | null>(null)
const filterProvider = ref<string>('')
const filterAgent = ref<string>('')
const filterSession = ref<string>('')

const summary = ref<Record<string, any>>({})
const timeline = ref<any[]>([])
const records = ref<any[]>([])
const totalRecords = ref(0)
const page = ref(1)
const pageSize = ref(50)

// All agents, for id→name map + filter dropdown
const allAgents = ref<Array<{ id: string; name: string }>>([])
const agentNameMap = computed(() => {
  const m: Record<string, string> = {}
  for (const a of allAgents.value) m[a.id] = a.name
  return m
})

// Sessions for the currently-selected agent (for session filter dropdown)
const agentSessions = ref<SessionSummary[]>([])

const timelineChartEl = ref<HTMLElement | null>(null)
const providerChartEl  = ref<HTMLElement | null>(null)
const agentChartEl     = ref<HTMLElement | null>(null)
let timelineChart: echarts.ECharts | null = null
let providerChart: echarts.ECharts | null = null
let agentChart: echarts.ECharts   | null = null

// ── shortcuts ──────────────────────────────────────────────────────
const dateShortcuts = [
  { text: '今天',    value: () => { const n = new Date(); n.setHours(0,0,0,0); return [n, new Date()] } },
  { text: '最近7天', value: () => [new Date(Date.now()-7*86400_000), new Date()] },
  { text: '最近30天',value: () => [new Date(Date.now()-30*86400_000), new Date()] },
  { text: '本月',    value: () => { const n = new Date(); return [new Date(n.getFullYear(), n.getMonth(), 1), n] } },
]

// ── filter options ─────────────────────────────────────────────────
const providerOptions = computed(() => Object.keys(summary.value?.by_provider ?? {}))
// agent dropdown uses real display names; fall back to IDs that have usage
// but aren't registered (deleted agents) for completeness.
const agentOptions = computed<Array<{ id: string; name: string }>>(() => {
  const ids = new Set<string>(Object.keys(summary.value?.by_agent ?? {}))
  for (const a of allAgents.value) ids.add(a.id)
  const out: Array<{ id: string; name: string }> = []
  for (const id of ids) {
    out.push({ id, name: agentNameMap.value[id] || id })
  }
  out.sort((a, b) => a.name.localeCompare(b.name))
  return out
})

const sessionOptions = computed<Array<{ id: string; label: string }>>(() => {
  // Only show sessions belonging to the selected agent (otherwise too many)
  if (!filterAgent.value) return []
  return (agentSessions.value || []).map(s => ({
    id: s.id,
    label: (s.title || s.id) + '  ·  ' + (s.messageCount || 0) + ' 条',
  }))
})

function shortSid(sid: string): string {
  if (!sid) return ''
  if (sid.length > 24) return sid.slice(0, 20) + '…'
  return sid
}

// ── params helper ──────────────────────────────────────────────────
function buildParams() {
  const p: Record<string, any> = {}
  if (dateRange.value) {
    p.from = Math.floor(Number(dateRange.value[0]) / 1000)
    p.to   = Math.floor(Number(dateRange.value[1]) / 1000)
  }
  if (filterProvider.value) p.provider  = filterProvider.value
  if (filterAgent.value)    p.agentId   = filterAgent.value
  if (filterSession.value)  p.sessionId = filterSession.value
  return p
}

async function loadAgentList() {
  try {
    const res = await agentsApi.list()
    allAgents.value = (res.data as any[]).map(a => ({ id: a.id, name: a.name || a.id }))
  } catch { allAgents.value = [] }
}

async function loadAgentSessions() {
  if (!filterAgent.value) {
    agentSessions.value = []
    return
  }
  try {
    const res = await sessionsApi.list({ agentId: filterAgent.value, limit: 200 })
    const d = res.data as any
    agentSessions.value = (d?.sessions ?? d ?? []) as SessionSummary[]
  } catch {
    agentSessions.value = []
  }
}

// When agent filter changes, load its sessions for the session dropdown and
// clear any stale session selection.
watch(filterAgent, () => {
  filterSession.value = ''
  loadAgentSessions()
})

// ── load ───────────────────────────────────────────────────────────
async function loadSummary() {
  const res = await usageApi.summary(buildParams())
  summary.value = res.data
}
async function loadTimeline() {
  const res = await usageApi.timeline(buildParams())
  timeline.value = (res.data as any).points ?? []
}
async function loadRecords() {
  loadingRecords.value = true
  try {
    const res = await usageApi.records({ ...buildParams(), page: page.value, pageSize: pageSize.value })
    const d = res.data as any
    records.value = d.records ?? []
    totalRecords.value = d.total ?? 0
  } finally {
    loadingRecords.value = false
  }
}

async function load() {
  loading.value = true
  try {
    await Promise.all([loadSummary(), loadTimeline(), loadRecords()])
    await nextTick()
    renderCharts()
  } finally {
    loading.value = false
  }
}

// ── charts ─────────────────────────────────────────────────────────
function initCharts() {
  if (timelineChartEl.value && !timelineChart) timelineChart = echarts.init(timelineChartEl.value)
  if (providerChartEl.value  && !providerChart)  providerChart  = echarts.init(providerChartEl.value)
  if (agentChartEl.value     && !agentChart)      agentChart     = echarts.init(agentChartEl.value)
}

function renderCharts() {
  initCharts()
  // Timeline bar+line
  const pts = timeline.value
  timelineChart?.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['调用次数', '花费(USD)'], top: 2, textStyle: { fontSize: 11 } },
    grid: { left: 44, right: 56, top: 36, bottom: 28 },
    xAxis: { type: 'category', data: pts.map((p: any) => p.date), axisLabel: { fontSize: 10 } },
    yAxis: [
      { type: 'value', name: '次数', nameTextStyle: { fontSize: 10 } },
      { type: 'value', name: 'USD', nameTextStyle: { fontSize: 10 }, axisLabel: { fontSize: 10, formatter: (v: number) => '$'+v.toFixed(3) } },
    ],
    series: [
      { name: '调用次数', type: 'bar', data: pts.map((p: any) => p.calls), itemStyle: { color: '#6366f1' } },
      { name: '花费(USD)', type: 'line', yAxisIndex: 1, smooth: true,
        data: pts.map((p: any) => +(p.cost ?? 0).toFixed(5)), itemStyle: { color: '#f59e0b' }, symbol: 'circle', symbolSize: 4 },
    ],
  }, true)
  // Provider pie
  renderPie(providerChart, summary.value?.by_provider ?? {})
  renderPie(agentChart,    summary.value?.by_agent    ?? {})
}

function renderPie(chart: echarts.ECharts | null, map: Record<string, any>) {
  if (!chart) return
  const data = Object.entries(map).map(([name, s]: [string, any]) => ({
    name, value: s.calls,
    extra: `$${(s.cost??0).toFixed(4)} | ${fmtTokens((s.input_tokens??0)+(s.output_tokens??0))} tokens`
  }))
  chart.setOption({
    tooltip: {
      trigger: 'item',
      formatter: (p: any) => `${p.name}<br/>调用: ${p.value}<br/>${p.data.extra}`,
    },
    legend: {
      orient: 'vertical',
      right: 8,
      top: 'middle',
      itemGap: 6,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { fontSize: 11, color: '#475569' },
      // 条目太多时允许滚动 (不再挤成一团叠在饼图上)
      type: 'scroll',
      pageIconSize: 10,
      pageTextStyle: { fontSize: 10 },
    },
    series: [{
      type: 'pie',
      radius: ['38%', '62%'],
      // 给左侧饼图更多空间, 不让 label 和 legend 重叠
      center: ['30%', '50%'],
      data,
      label: { show: false },
      labelLine: { show: false },
      itemStyle: { borderColor: '#fff', borderWidth: 1 },
      emphasis: {
        label: { show: true, fontSize: 11, fontWeight: 600 },
        scaleSize: 6,
      },
    }],
  }, true)
}

// ── utils ──────────────────────────────────────────────────────────
function fmtTokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return (n/1_000_000).toFixed(2)+'M'
  if (n >= 1_000)     return (n/1_000).toFixed(1)+'K'
  return String(n)
}

type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger' | ''
function providerColor(p: string): TagType {
  const m: Record<string, TagType> = {
    anthropic:'warning', openai:'success', deepseek:'primary',
    minimax:'info', moonshot:'info', zhipu:'info',
  }
  return m[p] ?? ''
}

// ── lifecycle ──────────────────────────────────────────────────────
onMounted(async () => {
  dateRange.value = [Date.now() - 30*86400_000, Date.now()]
  await loadAgentList()
  await load()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  timelineChart?.dispose(); providerChart?.dispose(); agentChart?.dispose()
})
function onResize() {
  timelineChart?.resize(); providerChart?.resize(); agentChart?.resize()
}
</script>

<style scoped>
.usage-studio {
  display: flex;
  flex-direction: column;
  gap: 14px;
  /* padding 由 .app-main 统一提供 */
}
.usage-filter {
  background: #fff;
  padding: 12px 16px;
  border-radius: 10px;
  border: 1px solid #ececec;
}
.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
.stat-card {
  position: relative;
  background: #fff;
  border: 1px solid #ececec;
  border-radius: 10px;
  padding: 16px 20px;
  overflow: hidden;
  transition: border-color .15s, box-shadow .15s;
}
.stat-card:hover {
  border-color: #d1d5db;
  box-shadow: 0 2px 8px rgba(0,0,0,0.04);
}
.stat-card::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
  background: #cbd5e1;
}
.stat-card:nth-child(1)::before { background: #6366f1; }
.stat-card:nth-child(2)::before { background: #10b981; }
.stat-card:nth-child(3)::before { background: #f59e0b; }
.stat-card.highlight::before   { background: #ef4444; }
.stat-card.highlight {
  background: rgba(239,68,68,0.03);
}
.stat-label { font-size: 11px; color: #64748b; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 500; }
.stat-value { font-size: 24px; font-weight: 700; color: #1e293b; letter-spacing: -0.5px; }
.usage-charts {
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: 12px;
}
.chart-card, .chart-card-sm, .records-card {
  border: 1px solid #ececec !important;
  border-radius: 10px !important;
  box-shadow: none !important;
}
.chart-card  { height: 300px; }
.chart-area  { width: 100%; height: 220px; }
.pie-col     { display: flex; flex-direction: column; gap: 12px; }
.chart-card-sm { height: 170px; }
.chart-area-sm { width: 100%; height: 108px; }
.card-title  { font-size: 13px; font-weight: 600; color: #334155; }
.records-card { flex: 1; min-height: 0; }
.table-pagination { margin-top: 12px; display: flex; justify-content: flex-end; }

.col-agent-name { font-weight: 500; color: #334155; }
.col-agent-id {
  display: block;
  font-size: 10px;
  color: #94a3b8;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  margin-top: 1px;
}
.col-session { display: flex; flex-direction: column; gap: 1px; }
.col-session-title { color: #1e293b; }
.col-session-id {
  font-size: 10px;
  color: #94a3b8;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.col-session-none { color: #cbd5e1; }
:deep(.el-card__header) {
  padding: 10px 16px;
  background: #fafbfc;
  border-bottom: 1px solid #ececec;
}
:deep(.el-card__body)   { padding: 12px 16px; }
@media (max-width: 900px) {
  .stat-cards    { grid-template-columns: repeat(2, 1fr); }
  .usage-charts  { grid-template-columns: 1fr; }
}
</style>
