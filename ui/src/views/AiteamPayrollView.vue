<!--
  aiteam · 工资 (PR-002 / Phase 2 § P2-S6)

  Layout:
    1. Header — title + "立即跑工资" button (owner action)
    2. Agent picker
    3. 30-day payslip timeline (SVG hand-drawn line chart, no echarts)
    4. Per-payslip table — breakdown of base/bonus/offset/net
-->
<template>
  <div class="aiteam-payroll">
    <div class="page-header">
      <h1 style="margin:0">🧪 aiteam · 工资</h1>
      <div style="margin-left:auto;display:flex;gap:8px">
        <el-button @click="refresh" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="runAll" :loading="running">立即跑工资（全员）</el-button>
      </div>
    </div>

    <el-card shadow="never" style="margin-bottom:20px">
      <template #header><span>选 agent</span></template>
      <el-select
        v-model="selectedAgent"
        placeholder="选择 agent"
        filterable
        style="width:280px"
        @change="loadHistory"
      >
        <el-option v-for="id in agentIds" :key="id" :label="id" :value="id" />
      </el-select>
    </el-card>

    <el-card v-if="selectedAgent" shadow="never" style="margin-bottom:20px">
      <template #header>
        <span>{{ selectedAgent }} 最近 30 日工资轨迹</span>
        <span style="float:right;font-size:12px;color:#999">
          月内总收 {{ formatMoney(totalNetUSDT) }}
        </span>
      </template>
      <div v-if="history.length === 0" style="padding:40px;text-align:center;color:#999">
        暂无工资记录 — 点击「立即跑工资」触发一次
      </div>
      <div v-else class="chart-wrap">
        <!-- SVG hand-drawn line chart, no echarts dependency -->
        <svg :viewBox="`0 0 ${chartWidth} ${chartHeight}`" width="100%" preserveAspectRatio="none">
          <!-- gridlines -->
          <line v-for="g in 5" :key="g" :x1="0" :y1="(g-1) * chartHeight/4" :x2="chartWidth" :y2="(g-1) * chartHeight/4"
            stroke="#eee" stroke-width="1" />
          <!-- line -->
          <polyline :points="linePoints" fill="none" stroke="#67c23a" stroke-width="2" />
          <!-- dots -->
          <circle v-for="(p, idx) in chartPoints" :key="idx"
            :cx="p.x" :cy="p.y" :r="3" :fill="p.skipped ? '#f56c6c' : '#67c23a'" />
        </svg>
        <div class="chart-legend">
          <span style="color:#67c23a">● 正常工资</span>
          <span style="color:#f56c6c;margin-left:12px">● 跳过 (net ≤ 0)</span>
        </div>
      </div>
    </el-card>

    <el-card v-if="selectedAgent" shadow="never">
      <template #header><span>工资单明细</span></template>
      <el-table :data="history" size="small">
        <el-table-column label="日期" width="120">
          <template #default="{ row }">{{ row.period }}</template>
        </el-table-column>
        <el-table-column label="基本">
          <template #default="{ row }">{{ formatMoney(row.base_usdt) }}</template>
        </el-table-column>
        <el-table-column label="奖金">
          <template #default="{ row }">
            <span style="color:#67c23a">{{ formatMoney(row.bonus_usdt) }}</span>
            <span style="font-size:11px;color:#999"> × {{ row.bonus_factor.toFixed(2) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="成本扣减">
          <template #default="{ row }">
            <span style="color:#f56c6c">{{ formatMoney(row.offset_usdt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="净额" width="150">
          <template #default="{ row }">
            <strong :style="{color: row.skipped ? '#f56c6c' : '#18181b'}">
              {{ formatMoney(row.net_usdt) }}
            </strong>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.skipped" type="warning" size="small">已跳过</el-tag>
            <el-tag v-else type="success" size="small">已发放</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="180">
          <template #default="{ row }">
            <span v-if="row.skipped_note" style="font-size:12px;color:#888">{{ row.skipped_note }}</span>
            <span v-else style="color:#ccc">—</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getPayroll, runPayroll, getOverview, type PayslipEntry } from '../api/aiteam'
import { useCurrency } from '../composables/useCurrency'

const { formatMoney } = useCurrency()
const agentIds = ref<string[]>([])
const selectedAgent = ref('')
const history = ref<PayslipEntry[]>([])
const loading = ref(false)
const running = ref(false)

const chartWidth = 800
const chartHeight = 200

async function refresh() {
  loading.value = true
  try {
    const o = await getOverview()
    agentIds.value = Object.keys(o.wallet?.agents || {}).sort()
    if (!selectedAgent.value && agentIds.value.length > 0) {
      selectedAgent.value = agentIds.value[0] || ''
      await loadHistory()
    } else if (selectedAgent.value) {
      await loadHistory()
    }
  } catch (e: any) {
    if (e?.response?.status === 404) {
      ElMessage.warning('工资模块未启用 — 设置 ZYHIVE_EXPERIMENTAL_PAYROLL=1')
    } else {
      ElMessage.error('加载失败')
    }
  }
  loading.value = false
}

async function loadHistory() {
  if (!selectedAgent.value) {
    history.value = []
    return
  }
  try {
    const r = await getPayroll(selectedAgent.value)
    // sorted newest-first by the API; for the chart we need oldest-first
    history.value = (r.history || []).slice().reverse()
  } catch {
    history.value = []
  }
}

async function runAll() {
  // B030 fix: mass payroll mutates wallets for every agent — require explicit confirm.
  try {
    await ElMessageBox.confirm(
      `将对当前 ${agentIds.value.length} 个 agent 立即结算今日工资（base + bonus − cost）。\n\n` +
      `此操作会写入钱包账本，无法撤销。确认继续？`,
      '确认跑工资（全员）',
      { confirmButtonText: '立即结算', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return  // user cancelled
  }
  running.value = true
  try {
    const r = await runPayroll() // no agent_ids → all
    ElMessage.success(`已对 ${r.entries.length} 个 agent 跑工资`)
    await refresh()
  } catch (e: any) {
    ElMessage.error('跑工资失败: ' + (e?.response?.data?.error || e.message))
  }
  running.value = false
}

const totalNetUSDT = computed<string>(() => {
  const sum = history.value.reduce((acc, e) => acc + (e.skipped ? 0 : parseFloat(e.net_usdt)), 0)
  return sum.toFixed(6)
})

const chartPoints = computed(() => {
  if (history.value.length === 0) return []
  const nets = history.value.map(e => parseFloat(e.net_usdt))
  const maxAbs = Math.max(0.01, ...nets.map(Math.abs))
  const step = history.value.length > 1 ? chartWidth / (history.value.length - 1) : 0
  return history.value.map((e, idx) => {
    const x = idx * step
    const v = parseFloat(e.net_usdt)
    const norm = (v + maxAbs) / (2 * maxAbs)
    const y = chartHeight - norm * chartHeight
    return { x, y, skipped: e.skipped, value: v }
  })
})

const linePoints = computed(() =>
  chartPoints.value.map(p => `${p.x},${p.y}`).join(' ')
)

onMounted(refresh)
</script>

<style scoped>
.aiteam-payroll { padding: 16px 24px; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.chart-wrap {
  padding: 16px 0;
}
.chart-legend {
  display: flex;
  gap: 8px;
  font-size: 12px;
  color: #888;
  margin-top: 8px;
  justify-content: center;
}
</style>
