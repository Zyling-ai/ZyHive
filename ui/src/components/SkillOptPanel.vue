<template>
  <div class="skillopt-panel">
    <!-- ── 头部状态 ── -->
    <div class="so-header">
      <div class="so-title">
        <span class="so-emoji">🧬</span>
        <span>技能进化 · SkillOpt</span>
        <el-tag v-if="ov?.shadowActive" type="warning" size="small" effect="dark">影子灰度中</el-tag>
        <el-tag v-else-if="ov?.initialized" type="success" size="small" effect="plain">epoch {{ ov?.epoch }}</el-tag>
      </div>
      <div class="so-acts">
        <el-button size="small" :loading="loading" circle @click="reload">
          <el-icon><Refresh /></el-icon>
        </el-button>
        <el-button size="small" type="primary" :loading="evolving" @click="doEvolve">
          <el-icon><MagicStick /></el-icon> 立即进化
        </el-button>
      </div>
    </div>

    <!-- 指标卡 -->
    <div class="so-metrics">
      <div class="metric">
        <div class="metric-val">{{ pct(ov?.hitRateBaseline) }}</div>
        <div class="metric-label">基线命中率（{{ ledgerStats.baselineSamples }} 样本）</div>
      </div>
      <div class="metric" :class="{ dim: !ov?.shadowActive }">
        <div class="metric-val">{{ ov?.shadowActive ? pct(ov?.hitRateShadow) : '—' }}</div>
        <div class="metric-label">影子命中率（{{ ledgerStats.shadowSamples }} 样本）</div>
      </div>
      <div class="metric">
        <div class="metric-val">{{ ov?.pendingOracle ?? 0 }}</div>
        <div class="metric-label">待回填预测</div>
      </div>
      <div class="metric">
        <div class="metric-val">{{ ov?.pendingProposals ?? 0 }}</div>
        <div class="metric-label">待审提案</div>
      </div>
    </div>

    <!-- 进化进度 -->
    <div class="so-progress" v-if="ov">
      <span class="prog-label">距下次自动进化</span>
      <el-progress
        :percentage="evolveProgress"
        :stroke-width="10"
        :format="() => `${ov?.sinceEvolveSamples ?? 0}/${ov?.sampleThreshold ?? 0}`"
        style="flex:1"
      />
    </div>

    <!-- 配置条 -->
    <div class="so-config">
      <label class="cfg-item">
        <el-switch :model-value="ov?.maintenanceEnabled" size="small" @change="(v: boolean) => toggleEnabled(v)" />
        <span>开启进化维护（每日定时）</span>
      </label>
      <label class="cfg-item">
        <el-switch :model-value="ov?.autoAccept" size="small" :disabled="!ov?.initialized" @change="(v: boolean) => save({ autoAccept: v })" />
        <span>全自动（自动接受提案进灰度）</span>
      </label>
      <label class="cfg-item">
        <span>触发阈值</span>
        <el-input-number :model-value="ov?.sampleThreshold" :min="1" :max="500" size="small" controls-position="right"
          style="width:90px" @change="(v: number) => save({ sampleThreshold: v })" />
      </label>
      <label class="cfg-item">
        <span>晋升优势</span>
        <el-input-number :model-value="ov?.promoteMargin" :min="0" :max="1" :step="0.01" size="small" controls-position="right"
          style="width:100px" @change="(v: number) => save({ promoteMargin: v })" />
      </label>
      <label class="cfg-item">
        <span>影子最小样本</span>
        <el-input-number :model-value="ov?.shadowMinSample" :min="1" :max="200" size="small" controls-position="right"
          style="width:90px" @change="(v: number) => save({ shadowMinSample: v })" />
      </label>
    </div>

    <!-- ── 子标签 ── -->
    <el-tabs v-model="tab" class="so-tabs">
      <!-- 台账 -->
      <el-tab-pane name="ledger">
        <template #label><span>台账 <el-badge v-if="ledger.length" :value="ledger.length" type="info" /></span></template>

        <!-- 记录预测 -->
        <div class="predict-form">
          <el-input v-model="predForm.prediction" size="small" placeholder="新预测（可被事实检验，如“主队2:1获胜”）" style="flex:2" />
          <el-input v-model="predForm.contextDigest" size="small" placeholder="依据/上下文（可选）" style="flex:1.5" />
          <el-button size="small" type="primary" :loading="predicting" @click="doPredict">记录预测</el-button>
        </div>

        <el-table :data="ledger" size="small" max-height="340" empty-text="暂无预测记录">
          <el-table-column label="预测" min-width="180">
            <template #default="{ row }">
              <div class="cell-pred">{{ row.prediction }}</div>
              <div v-if="row.contextDigest" class="cell-sub">{{ row.contextDigest }}</div>
            </template>
          </el-table-column>
          <el-table-column label="真实结果" min-width="150">
            <template #default="{ row }">
              <span v-if="row.hit !== undefined && row.hit !== null">{{ row.oracle || '—' }}</span>
              <span v-else class="cell-muted">待回填</span>
              <div v-if="row.attributionTags?.length" class="cell-tags">
                <el-tag v-for="t in row.attributionTags" :key="t" size="small" type="danger" effect="plain">{{ t }}</el-tag>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="结果" width="120" align="center">
            <template #default="{ row }">
              <template v-if="row.hit === true"><el-tag type="success" size="small">命中</el-tag></template>
              <template v-else-if="row.hit === false"><el-tag type="danger" size="small">未中</el-tag></template>
              <template v-else>
                <el-button-group>
                  <el-button size="small" type="success" plain @click="doOracle(row, true)">中</el-button>
                  <el-button size="small" type="danger" plain @click="doOracle(row, false)">未中</el-button>
                </el-button-group>
              </template>
            </template>
          </el-table-column>
          <el-table-column label="版本" width="70" align="center">
            <template #default="{ row }">
              <el-tag size="small" :type="row.shadow ? 'warning' : 'info'" effect="plain">v{{ row.version }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 提案 -->
      <el-tab-pane name="proposals">
        <template #label><span>提案 <el-badge v-if="pendingProps.length" :value="pendingProps.length" type="warning" /></span></template>
        <div v-if="proposals.length === 0" class="so-empty">暂无进化提案。攒够失败样本后会自动生成，或点「立即进化」。</div>
        <div v-for="p in proposals" :key="p.id" class="prop-card" :class="p.status">
          <div class="prop-top">
            <span class="prop-id">{{ p.id }}</span>
            <el-tag size="small" :type="propTagType(p.status)">{{ propStatusLabel(p.status) }}</el-tag>
            <span class="prop-diff">{{ p.diffSummary }}</span>
            <span class="prop-spacer" />
            <template v-if="p.status === 'pending'">
              <el-button size="small" type="primary" @click="accept(p)">接受 → 灰度</el-button>
              <el-button size="small" type="danger" plain @click="reject(p)">拒绝</el-button>
            </template>
          </div>
          <div class="prop-rationale">{{ p.rationale }}</div>
          <ul class="prop-lessons">
            <li v-for="(l, i) in p.lessons" :key="i">{{ l }}</li>
          </ul>
          <el-collapse v-if="p.newContent">
            <el-collapse-item title="查看进化后 SKILL.md">
              <pre class="prop-content">{{ p.newContent }}</pre>
            </el-collapse-item>
          </el-collapse>
        </div>
      </el-tab-pane>

      <!-- 版本 -->
      <el-tab-pane name="versions" label="版本">
        <div v-if="ov?.shadowActive" class="shadow-bar">
          <span>影子版本 v{{ ov?.shadowVersion }} 正在灰度。命中率达标将自动晋升，未达标自动回滚。</span>
          <el-button size="small" type="warning" @click="promote">强制晋升</el-button>
        </div>
        <el-table :data="versionRows" size="small" empty-text="暂无版本快照">
          <el-table-column label="版本" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="row.v === ov?.baselineVersion ? 'success' : (row.v === ov?.shadowVersion ? 'warning' : 'info')">
                v{{ row.v }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="角色">
            <template #default="{ row }">
              <span v-if="row.v === ov?.baselineVersion">当前基线</span>
              <span v-else-if="row.v === ov?.shadowVersion">影子灰度</span>
              <span v-else class="cell-muted">历史快照</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" align="right">
            <template #default="{ row }">
              <el-button v-if="row.v !== ov?.baselineVersion" size="small" plain @click="rollback(row.v)">回滚到此</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { skillopt, type SkillOptOverview, type SkillOptLedgerEntry, type SkillOptProposal } from '../api'

const props = defineProps<{ agentId: string; skillId: string }>()

const loading = ref(false)
const evolving = ref(false)
const predicting = ref(false)
const tab = ref('ledger')

const ov = ref<SkillOptOverview | null>(null)
const ledger = ref<SkillOptLedgerEntry[]>([])
const ledgerStats = reactive({ baselineSamples: 0, shadowSamples: 0 })
const proposals = ref<SkillOptProposal[]>([])
const versions = ref<number[]>([])
const predForm = reactive({ prediction: '', contextDigest: '' })

const pendingProps = computed(() => proposals.value.filter(p => p.status === 'pending'))
const versionRows = computed(() => versions.value.map(v => ({ v })).sort((a, b) => b.v - a.v))
const evolveProgress = computed(() => {
  if (!ov.value || !ov.value.sampleThreshold) return 0
  return Math.min(100, Math.round((ov.value.sinceEvolveSamples / ov.value.sampleThreshold) * 100))
})

function pct(v?: number) {
  if (v === undefined || v === null) return '—'
  return `${Math.round(v * 100)}%`
}
function propTagType(s: string) {
  return ({ pending: 'warning', accepted: 'primary', promoted: 'success', rejected: 'info' } as Record<string, any>)[s] || 'info'
}
function propStatusLabel(s: string) {
  return ({ pending: '待审', accepted: '灰度中', promoted: '已晋升', rejected: '已拒绝' } as Record<string, string>)[s] || s
}

async function reload() {
  if (!props.skillId) return
  loading.value = true
  try {
    const [o, l, p, v] = await Promise.all([
      skillopt.overview(props.agentId, props.skillId),
      skillopt.ledger(props.agentId, props.skillId),
      skillopt.proposals(props.agentId, props.skillId),
      skillopt.versions(props.agentId, props.skillId),
    ])
    ov.value = o.data
    ledger.value = (l.data.entries || []).slice().reverse()
    ledgerStats.baselineSamples = l.data.baselineSamples
    ledgerStats.shadowSamples = l.data.shadowSamples
    proposals.value = p.data || []
    versions.value = v.data.versions || []
  } catch (e: any) {
    ElMessage.error('加载进化数据失败：' + (e?.response?.data?.error || e.message))
  } finally {
    loading.value = false
  }
}

async function save(cfg: Record<string, any>) {
  try {
    const res = await skillopt.setConfig(props.agentId, props.skillId, cfg)
    ov.value = res.data
    ElMessage.success('已更新')
  } catch (e: any) {
    ElMessage.error('更新失败：' + (e?.response?.data?.error || e.message))
  }
}

async function toggleEnabled(v: boolean) {
  await save({ enabled: v })
  await reload()
}

async function doEvolve() {
  evolving.value = true
  try {
    const res = await skillopt.evolve(props.agentId, props.skillId)
    if (res.data.proposal) {
      ElMessage.success('已生成进化提案 ' + res.data.proposal.id)
      tab.value = 'proposals'
    } else {
      ElMessage.info(res.data.message || '暂无可进化内容')
    }
    await reload()
  } catch (e: any) {
    ElMessage.error('进化失败：' + (e?.response?.data?.error || e.message))
  } finally {
    evolving.value = false
  }
}

async function doPredict() {
  if (!predForm.prediction.trim()) { ElMessage.warning('请填写预测内容'); return }
  predicting.value = true
  try {
    await skillopt.predict(props.agentId, props.skillId, { prediction: predForm.prediction, contextDigest: predForm.contextDigest })
    predForm.prediction = ''
    predForm.contextDigest = ''
    await reload()
  } catch (e: any) {
    ElMessage.error('记录失败：' + (e?.response?.data?.error || e.message))
  } finally {
    predicting.value = false
  }
}

async function doOracle(row: SkillOptLedgerEntry, hit: boolean) {
  try {
    let result = hit ? '命中' : ''
    if (!hit) {
      const r = await ElMessageBox.prompt('请填写真实结果（用于失败复盘）', '回填结果', { inputPlaceholder: '真实结果…' }).catch(() => null)
      if (r === null) return
      result = (r as any).value || '未命中'
    }
    await skillopt.oracle(props.agentId, props.skillId, { entryId: row.id, result, hit })
    await reload()
  } catch (e: any) {
    ElMessage.error('回填失败：' + (e?.response?.data?.error || e.message))
  }
}

async function accept(p: SkillOptProposal) {
  try {
    await skillopt.acceptProposal(props.agentId, props.skillId, p.id)
    ElMessage.success('已接受，进入影子灰度')
    await reload()
  } catch (e: any) {
    ElMessage.error('接受失败：' + (e?.response?.data?.error || e.message))
  }
}
async function reject(p: SkillOptProposal) {
  try {
    await skillopt.rejectProposal(props.agentId, props.skillId, p.id)
    await reload()
  } catch (e: any) {
    ElMessage.error('拒绝失败：' + (e?.response?.data?.error || e.message))
  }
}
async function promote() {
  try {
    const res = await skillopt.promoteShadow(props.agentId, props.skillId)
    ElMessage.success(res.data.message)
    await reload()
  } catch (e: any) {
    ElMessage.error('晋升失败：' + (e?.response?.data?.error || e.message))
  }
}
async function rollback(v: number) {
  try {
    await ElMessageBox.confirm(`确认回滚到版本 v${v}？当前内容将被替换。`, '回滚确认', { type: 'warning' })
  } catch {
    return // user cancelled
  }
  try {
    await skillopt.rollback(props.agentId, props.skillId, v)
    ElMessage.success(`已回滚到 v${v}`)
    await reload()
  } catch (e: any) {
    ElMessage.error('回滚失败：' + (e?.response?.data?.error || e.message))
  }
}

watch(() => props.skillId, reload)
onMounted(reload)
</script>

<style scoped>
.skillopt-panel { padding: 12px 16px; height: 100%; overflow-y: auto; }
.so-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.so-title { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 15px; }
.so-emoji { font-size: 18px; }
.so-acts { display: flex; gap: 8px; }
.so-metrics { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; margin-bottom: 12px; }
.metric { background: var(--el-fill-color-light); border-radius: 8px; padding: 10px 12px; text-align: center; }
.metric.dim { opacity: 0.5; }
.metric-val { font-size: 22px; font-weight: 700; color: var(--el-color-primary); }
.metric-label { font-size: 11px; color: var(--el-text-color-secondary); margin-top: 2px; }
.so-progress { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.prog-label { font-size: 12px; color: var(--el-text-color-secondary); white-space: nowrap; }
.so-config { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; padding: 10px 12px; background: var(--el-fill-color-lighter); border-radius: 8px; margin-bottom: 8px; }
.cfg-item { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--el-text-color-regular); }
.so-tabs { margin-top: 4px; }
.predict-form { display: flex; gap: 8px; margin-bottom: 10px; }
.cell-pred { font-size: 13px; }
.cell-sub { font-size: 11px; color: var(--el-text-color-secondary); margin-top: 2px; }
.cell-muted { color: var(--el-text-color-placeholder); }
.cell-tags { margin-top: 4px; display: flex; flex-wrap: wrap; gap: 4px; }
.so-empty { color: var(--el-text-color-secondary); font-size: 13px; padding: 20px; text-align: center; }
.prop-card { border: 1px solid var(--el-border-color-light); border-radius: 8px; padding: 10px 12px; margin-bottom: 10px; }
.prop-card.pending { border-left: 3px solid var(--el-color-warning); }
.prop-card.promoted { border-left: 3px solid var(--el-color-success); }
.prop-card.rejected { opacity: 0.6; }
.prop-top { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.prop-id { font-family: monospace; font-size: 12px; color: var(--el-text-color-secondary); }
.prop-diff { font-size: 12px; color: var(--el-text-color-regular); }
.prop-spacer { flex: 1; }
.prop-rationale { font-size: 13px; margin-bottom: 6px; }
.prop-lessons { margin: 0 0 6px; padding-left: 18px; font-size: 12px; color: var(--el-text-color-regular); }
.prop-content { background: var(--el-fill-color-light); padding: 10px; border-radius: 6px; font-size: 12px; white-space: pre-wrap; max-height: 300px; overflow: auto; }
.shadow-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; background: var(--el-color-warning-light-9); border: 1px solid var(--el-color-warning-light-5); border-radius: 8px; padding: 8px 12px; margin-bottom: 10px; font-size: 13px; }
</style>
