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
          style="width: 130px"
          @change="load"
        >
          <el-option v-for="a in agentOptions" :key="a" :label="a" :value="a" />
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
        <el-table-column prop="agent_id" label="成员" width="100" />
        <el-table-column prop="provider" label="厂商" width="110">
          <template #default="{ row }">
            <el-tag :type="providerColor(row.provider)" size="small">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="model" label="模型" show-overflow-tooltip />
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
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { usageApi } from '../api'
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

const summary = ref<Record<string, any>>({})
const timeline = ref<any[]>([])
const records = ref<any[]>([])
const totalRecords = ref(0)
const page = ref(1)
const pageSize = ref(50)

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
const agentOptions    = computed(() => Object.keys(summary.value?.by_agent    ?? {}))

// ── params helper ──────────────────────────────────────────────────
function buildParams() {
  const p: Record<string, any> = {}
  if (dateRange.value) {
    p.from = Math.floor(Number(dateRange.value[0]) / 1000)
    p.to   = Math.floor(Number(dateRange.value[1]) / 1000)
  }
  if (filterProvider.value) p.provider = filterProvider.value
  if (filterAgent.value)    p.agentId  = filterAgent.value
  return p
}

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
    tooltip: { trigger: 'item', formatter: (p: any) => `${p.name}<br/>调用: ${p.value}<br/>${p.data.extra}` },
    legend: { orient: 'vertical', right: 4, top: 'middle', textStyle: { fontSize: 10 } },
    series: [{
      type: 'pie', radius: ['40%', '65%'], center: ['36%', '50%'],
      data, label: { show: false },
      emphasis: { label: { show: true, fontSize: 11 } },
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
  background: var(--el-bg-color);
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid var(--el-border-color-light);
}
.stat-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
.stat-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  padding: 16px 20px;
}
.stat-card.highlight {
  border-color: #6366f1;
  background: rgba(99,102,241,0.06);
}
.stat-label { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 6px; }
.stat-value { font-size: 22px; font-weight: 600; color: var(--el-text-color-primary); }
.usage-charts {
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: 12px;
}
.chart-card  { height: 260px; }
.chart-area  { width: 100%; height: 185px; }
.pie-col     { display: flex; flex-direction: column; gap: 12px; }
.chart-card-sm { height: 124px; }
.chart-area-sm { width: 100%; height: 64px; }
.card-title  { font-size: 13px; font-weight: 600; }
.records-card { flex: 1; min-height: 0; }
.table-pagination { margin-top: 12px; display: flex; justify-content: flex-end; }
:deep(.el-card__header) { padding: 10px 16px; }
:deep(.el-card__body)   { padding: 12px 16px; }
@media (max-width: 900px) {
  .stat-cards    { grid-template-columns: repeat(2, 1fr); }
  .usage-charts  { grid-template-columns: 1fr; }
}
</style>
