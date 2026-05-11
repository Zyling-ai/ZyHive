<!--
  aiteam · 评分 (PR-004 / Phase 2 § P2-S7)

  Layout:
    1. Header — title + refresh + "立即跑评分" button
    2. Agent picker
    3. SVG radar chart of most-recent 5 dimensions
    4. SVG line chart of last 30 days average
    5. Score history table + manual override action
-->
<template>
  <div class="aiteam-judge">
    <div class="page-header">
      <h1 style="margin:0">🧪 aiteam · 评分</h1>
      <div style="margin-left:auto;display:flex;gap:8px">
        <el-button @click="refresh" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="openRunDialog" :disabled="!selectedAgent">
          立即跑评分
        </el-button>
      </div>
    </div>

    <el-card shadow="never" style="margin-bottom:20px">
      <template #header><span>选 agent</span></template>
      <el-select v-model="selectedAgent" placeholder="选择 agent" filterable style="width:280px" @change="loadHistory">
        <el-option v-for="id in agentIds" :key="id" :label="id" :value="id" />
      </el-select>
      <span v-if="avg30" style="margin-left:24px;color:#666">
        30 日平均：<strong style="color:#67c23a">{{ avg30.toFixed(2) }} / 10</strong>
      </span>
    </el-card>

    <div v-if="selectedAgent && latestScore" class="charts-row">
      <!-- radar chart -->
      <el-card shadow="never" class="chart-card">
        <template #header><span>最近评分（5 维雷达）</span></template>
        <svg viewBox="-110 -110 220 220" width="100%" height="280">
          <!-- background pentagons -->
          <polygon v-for="ring in [2,4,6,8,10]" :key="ring"
            :points="radarPolygon(ring, 10)" fill="none" stroke="#eee" stroke-width="1" />
          <!-- spokes -->
          <line v-for="(_, i) in 5" :key="i"
            x1="0" y1="0"
            :x2="100 * Math.cos(spokeAngle(i))" :y2="100 * Math.sin(spokeAngle(i))"
            stroke="#eee" stroke-width="1" />
          <!-- value polygon -->
          <polygon :points="radarValuePolygon(latestScore)" fill="rgba(103,194,58,0.3)" stroke="#67c23a" stroke-width="2" />
          <!-- dim labels -->
          <text v-for="(label, i) in DIMS" :key="label"
            :x="116 * Math.cos(spokeAngle(i))" :y="116 * Math.sin(spokeAngle(i))"
            text-anchor="middle" dominant-baseline="middle" font-size="11" fill="#666">
            {{ label }}
          </text>
        </svg>
        <div class="radar-meta">
          <div>来源: <el-tag size="small" :type="sourceTagType(latestScore.source)">{{ latestScore.source }}</el-tag></div>
          <div style="font-size:12px;color:#888;margin-top:8px">{{ latestScore.rationale }}</div>
        </div>
      </el-card>

      <!-- line chart of avg -->
      <el-card shadow="never" class="chart-card">
        <template #header><span>30 日平均分趋势</span></template>
        <svg :viewBox="`0 0 ${lineWidth} ${lineHeight}`" width="100%" height="280" preserveAspectRatio="none">
          <line v-for="g in 5" :key="g" :x1="0" :y1="(g-1) * lineHeight/4" :x2="lineWidth" :y2="(g-1) * lineHeight/4"
            stroke="#eee" stroke-width="1" />
          <polyline :points="lineSeriesPoints" fill="none" stroke="#67c23a" stroke-width="2" />
          <circle v-for="(p, i) in lineSeries" :key="i" :cx="p.x" :cy="p.y" :r="3" fill="#67c23a" />
        </svg>
        <div style="font-size:12px;color:#888;text-align:center;margin-top:4px">
          纵轴 0-10 分；横轴 30 日时间序列
        </div>
      </el-card>
    </div>

    <el-card v-if="selectedAgent" shadow="never">
      <template #header><span>评分历史</span></template>
      <el-table :data="history" size="small" max-height="500">
        <el-table-column label="日期" width="100">
          <template #default="{ row }">{{ row.period }}</template>
        </el-table-column>
        <el-table-column prop="completion" label="完成度" width="80" />
        <el-table-column prop="quality" label="质量" width="80" />
        <el-table-column prop="communication" label="沟通" width="80" />
        <el-table-column prop="creativity" label="创造" width="80" />
        <el-table-column prop="cost" label="成本" width="80" />
        <el-table-column label="平均" width="100">
          <template #default="{ row }">
            <strong :style="{color: avgColor(row.average)}">{{ row.average.toFixed(1) }}</strong>
          </template>
        </el-table-column>
        <el-table-column prop="source" label="来源" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="sourceTagType(row.source)">{{ row.source }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button link size="small" @click="openOverride(row)">手动覆盖</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="history.length === 0" description="尚无评分记录" />
    </el-card>

    <!-- run dialog -->
    <el-dialog v-model="runDialog" :title="`对 ${selectedAgent} 跑启发式评分`" width="420px">
      <el-form label-width="120px">
        <el-form-item label="今日 usage USD">
          <el-input v-model="runForm.usageUSD" placeholder="例如 0.30" />
        </el-form-item>
        <el-form-item label="调用次数">
          <el-input v-model="runForm.callCount" placeholder="例如 12" />
        </el-form-item>
        <p style="color:#888;font-size:12px;margin-left:120px;max-width:240px">
          ⓘ 启发式 v0 主要看 usage USD 推算 cost 维分数；其它维数给中性 baseline。
          LLM-driven 评分留作后续 PR（pkg/aiteam/judge/llm_scorer.go 已就绪）。
        </p>
      </el-form>
      <template #footer>
        <el-button @click="runDialog = false">取消</el-button>
        <el-button type="primary" @click="submitRun" :loading="submitting">跑评分</el-button>
      </template>
    </el-dialog>

    <!-- override dialog -->
    <el-dialog v-model="overrideDialog" :title="`覆盖 ${overrideForm.agent_id} 评分`" width="420px">
      <el-form label-width="80px">
        <el-form-item label="日期"><strong>{{ overrideForm.period }}</strong></el-form-item>
        <el-form-item v-for="dim in DIMS_EN" :key="dim" :label="dimLabel(dim)">
          <el-slider v-model="(overrideForm as any)[dim]" :min="0" :max="10" :step="1" show-input />
        </el-form-item>
        <el-form-item label="操作员">
          <el-input v-model="overrideForm.operator" />
        </el-form-item>
        <el-form-item label="理由">
          <el-input v-model="overrideForm.rationale" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="overrideDialog = false">取消</el-button>
        <el-button type="primary" @click="submitOverride" :loading="submitting">应用覆盖</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getJudgeScores, runJudge, overrideJudge, listJudgeAgents,
  type JudgeScore,
} from '../api/aiteam'

const DIMS_EN = ['completion', 'quality', 'communication', 'creativity', 'cost'] as const
const DIMS = ['完成', '质量', '沟通', '创造', '成本']

const agentIds = ref<string[]>([])
const selectedAgent = ref('')
const history = ref<JudgeScore[]>([])
const avg30 = ref<number | null>(null)
const loading = ref(false)

const runDialog = ref(false)
const runForm = ref({ usageUSD: '0.30', callCount: '12' })
const overrideDialog = ref(false)
const overrideForm = ref({
  agent_id: '', period: '', operator: 'owner', rationale: '',
  completion: 7, quality: 7, communication: 7, creativity: 7, cost: 7,
})
const submitting = ref(false)

const lineWidth = 800
const lineHeight = 200

async function refresh() {
  loading.value = true
  try {
    const a = await listJudgeAgents()
    agentIds.value = a.agents
    if (!selectedAgent.value && agentIds.value.length > 0) {
      selectedAgent.value = agentIds.value[0]
    }
    if (selectedAgent.value) await loadHistory()
  } catch (e: any) {
    if (e?.response?.status === 404) {
      ElMessage.warning('Judge 未启用 — 设置 ZYHIVE_EXPERIMENTAL_JUDGE=1')
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
    const r = await getJudgeScores(selectedAgent.value)
    history.value = (r.history || []).slice().reverse()
    avg30.value = r.average_30d ?? null
  } catch {
    history.value = []
  }
}

const latestScore = computed<JudgeScore | null>(() => {
  if (history.value.length === 0) return null
  return history.value[history.value.length - 1]
})

// ── radar geometry ──────────────────────────────────────────────
function spokeAngle(i: number): number {
  // start at top (-pi/2), rotate clockwise 5 spokes
  return -Math.PI / 2 + (i * 2 * Math.PI) / 5
}
function radarPolygon(value: number, max: number): string {
  const r = (value / max) * 100
  return Array.from({ length: 5 }).map((_, i) => {
    return `${r * Math.cos(spokeAngle(i))},${r * Math.sin(spokeAngle(i))}`
  }).join(' ')
}
function radarValuePolygon(sc: JudgeScore): string {
  const vals = [sc.completion, sc.quality, sc.communication, sc.creativity, sc.cost]
  return vals.map((v, i) => {
    const r = (v / 10) * 100
    return `${r * Math.cos(spokeAngle(i))},${r * Math.sin(spokeAngle(i))}`
  }).join(' ')
}

// ── line chart geometry ─────────────────────────────────────────
const lineSeries = computed(() => {
  if (history.value.length === 0) return []
  const step = history.value.length > 1 ? lineWidth / (history.value.length - 1) : 0
  return history.value.map((sc, idx) => ({
    x: idx * step,
    y: lineHeight - (sc.average / 10) * lineHeight,
  }))
})
const lineSeriesPoints = computed(() =>
  lineSeries.value.map(p => `${p.x},${p.y}`).join(' ')
)

// ── helpers ─────────────────────────────────────────────────────
function dimLabel(dim: string): string {
  const m: Record<string, string> = {
    completion: '完成度', quality: '质量', communication: '沟通',
    creativity: '创造', cost: '成本',
  }
  return m[dim] || dim
}
function sourceTagType(s: string): 'success' | 'warning' | 'info' | 'primary' {
  if (s === 'manual') return 'warning'
  if (s === 'llm') return 'success'
  if (s === 'heuristic') return 'info'
  return 'primary'
}
function avgColor(avg: number): string {
  if (avg >= 8) return '#67c23a'
  if (avg >= 6) return '#e6a23c'
  if (avg >= 4) return '#f56c6c'
  return '#999'
}

// ── actions ─────────────────────────────────────────────────────
function openRunDialog() {
  if (!selectedAgent.value) return
  runDialog.value = true
}

async function submitRun() {
  submitting.value = true
  try {
    await runJudge(
      selectedAgent.value,
      parseFloat(runForm.value.usageUSD),
      parseInt(runForm.value.callCount, 10),
    )
    ElMessage.success(`已对 ${selectedAgent.value} 跑评分`)
    runDialog.value = false
    await loadHistory()
  } catch (e: any) {
    ElMessage.error('跑评分失败: ' + (e?.response?.data?.error || e.message))
  }
  submitting.value = false
}

function openOverride(row: JudgeScore) {
  overrideForm.value = {
    agent_id: row.agent_id,
    period: row.period,
    operator: 'owner',
    rationale: '',
    completion: row.completion,
    quality: row.quality,
    communication: row.communication,
    creativity: row.creativity,
    cost: row.cost,
  }
  overrideDialog.value = true
}

async function submitOverride() {
  submitting.value = true
  try {
    await overrideJudge(overrideForm.value)
    ElMessage.success(`已覆盖 ${overrideForm.value.agent_id} ${overrideForm.value.period} 评分`)
    overrideDialog.value = false
    await loadHistory()
  } catch (e: any) {
    ElMessage.error('覆盖失败: ' + (e?.response?.data?.error || e.message))
  }
  submitting.value = false
}

onMounted(refresh)
</script>

<style scoped>
.aiteam-judge { padding: 16px 24px; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.charts-row {
  display: grid;
  grid-template-columns: 1fr 1.5fr;
  gap: 16px;
  margin-bottom: 20px;
}
@media (max-width: 900px) {
  .charts-row { grid-template-columns: 1fr; }
}
.chart-card { display: flex; flex-direction: column; }
.radar-meta { padding: 12px 0; }
</style>
