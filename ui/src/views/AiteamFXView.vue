<!--
  aiteam · 汇率管理 (PR-001 § 2.7 / Phase 2 § P2-S5)

  Layout:
    1. Source status card — which fetcher won + when + reload button
    2. 9-currency table — current rate + manual override action
    3. Override dialog — set / clear a manual rate

  This page is admin-only (any aiteam REST endpoint requires the bearer
  token). The "FX is informational; AI never touches it" contract from
  docs/aiteam-fx-and-currency.md applies — overrides change DISPLAY,
  not the ledger source-of-truth USDT.
-->
<template>
  <div class="aiteam-fx">
    <div class="page-header">
      <h1 style="margin:0">🧪 aiteam · 汇率</h1>
      <div style="margin-left:auto;display:flex;gap:8px">
        <el-button @click="loadRates" :loading="loading">重读</el-button>
        <el-button type="primary" @click="refreshFromAPI" :loading="refreshing">
          立即刷新（拉远端）
        </el-button>
      </div>
    </div>

    <!-- source status -->
    <el-card shadow="never" style="margin-bottom:20px">
      <template #header><span>汇率源状态</span></template>
      <div v-if="!snapshot" style="padding:20px;color:#999">加载中...</div>
      <div v-else class="fx-source">
        <div class="fx-source-row">
          <span class="fx-source-label">活跃源</span>
          <el-tag :type="sourceTagType(snapshot.source)">{{ sourceLabel(snapshot.source) }}</el-tag>
        </div>
        <div class="fx-source-row">
          <span class="fx-source-label">基准币种</span>
          <code>{{ snapshot.base }}</code>
        </div>
        <div class="fx-source-row">
          <span class="fx-source-label">最近刷新</span>
          <span>{{ snapshot.fetched_at ? new Date(snapshot.fetched_at).toLocaleString() : '从未刷新' }}</span>
        </div>
        <div v-if="snapshot.source === 'hardcoded'" class="fx-warning">
          ⚠️ 当前用硬编码兜底汇率，远端 API 未能成功调用 — 点击「立即刷新」尝试再连一次
        </div>
        <div v-else-if="snapshot.source === 'disk_cache'" class="fx-warning">
          ⚠️ 用磁盘缓存中的汇率（启动时未发起刷新），数据可能略旧
        </div>
      </div>
    </el-card>

    <!-- rate table -->
    <el-card shadow="never">
      <template #header>
        <span>9 币种当前汇率 (1 USDT = ?)</span>
        <span style="float:right;color:#999;font-size:12px">点击「编辑」手动覆盖</span>
      </template>

      <el-table :data="rateRows" size="small">
        <el-table-column prop="code" label="币种" width="100">
          <template #default="{ row }">
            <strong>{{ row.code }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="当前汇率" width="180">
          <template #default="{ row }">
            <span style="font-family:monospace">{{ row.rate.toFixed(4) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.overridden" type="warning" size="small">手动覆盖</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">实时</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="$1 USDT 显示为">
          <template #default="{ row }">
            {{ row.preview }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button link size="small" @click="openOverride(row.code, row.rate)">编辑</el-button>
            <el-button v-if="row.overridden" link size="small" type="danger" @click="clearOverride(row.code)">清除覆盖</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- override dialog -->
    <el-dialog v-model="overrideDialog" :title="`覆盖 ${overrideForm.currency} 汇率`" width="400px">
      <el-form label-width="100px">
        <el-form-item label="币种">
          <strong>{{ overrideForm.currency }}</strong>
        </el-form-item>
        <el-form-item label="新汇率">
          <el-input v-model="overrideForm.rate" placeholder="例如 7.20" />
        </el-form-item>
        <p style="color:#888;font-size:12px;margin-top:8px">
          ⓘ 这只改变显示。Ledger 始终用 USDT 数值，历史每条 entry 都持久化了
          当时的 fx_snapshot，覆盖永远不会改写既有账目。
        </p>
      </el-form>
      <template #footer>
        <el-button @click="overrideDialog = false">取消</el-button>
        <el-button type="primary" @click="submitOverride" :loading="submitting">应用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getFxRates, refreshFx, overrideFx, clearFxOverride, type FXSnapshot } from '../api/aiteam'
import { useCurrency, SUPPORTED_CURRENCIES } from '../composables/useCurrency'

const { formatMoney, refresh: refreshGlobalCurrency } = useCurrency()
const snapshot = ref<FXSnapshot | null>(null)
const loading = ref(false)
const refreshing = ref(false)

const overrideDialog = ref(false)
const overrideForm = ref({ currency: '', rate: '' })
const submitting = ref(false)

async function loadRates() {
  loading.value = true
  try {
    snapshot.value = await getFxRates()
  } catch (e: any) {
    if (e?.response?.status === 404) {
      ElMessage.warning('FX 未启用 — 设置 ZYHIVE_EXPERIMENTAL_WALLET=1')
    } else {
      ElMessage.error('加载失败')
    }
  }
  loading.value = false
}

async function refreshFromAPI() {
  refreshing.value = true
  try {
    const r = await refreshFx()
    snapshot.value = r.snap
    ElMessage.success(`已刷新汇率（源：${sourceLabel(r.source)}）`)
    await refreshGlobalCurrency()
  } catch {
    ElMessage.error('刷新失败')
  }
  refreshing.value = false
}

const rateRows = computed(() => {
  if (!snapshot.value) return []
  const ov = snapshot.value.overrides || {}
  return SUPPORTED_CURRENCIES.map(code => {
    const rate = snapshot.value!.rates[code] ?? 0
    return {
      code,
      rate,
      overridden: code in ov,
      preview: formatMoney(1, { currency: code }),
    }
  })
})

function openOverride(currency: string, currentRate: number) {
  overrideForm.value = { currency, rate: currentRate.toFixed(4) }
  overrideDialog.value = true
}

async function submitOverride() {
  const r = parseFloat(overrideForm.value.rate)
  if (!Number.isFinite(r) || r <= 0) {
    ElMessage.warning('请输入大于 0 的数值')
    return
  }
  // B033 fix: match server-side guard `[1e-6, 1e6]` so the user gets a
  // helpful message instead of a generic 400 from the API.
  if (r < 1e-6 || r > 1e6) {
    ElMessage.warning('汇率必须在 1e-6 ~ 1e6 之间（极端值会破坏显示层）')
    return
  }
  submitting.value = true
  try {
    await overrideFx(overrideForm.value.currency, r)
    ElMessage.success(`${overrideForm.value.currency} 汇率覆盖为 ${r}`)
    overrideDialog.value = false
    await loadRates()
    await refreshGlobalCurrency()
  } catch {
    ElMessage.error('覆盖失败')
  }
  submitting.value = false
}

async function clearOverride(currency: string) {
  try {
    await clearFxOverride(currency)
    ElMessage.success(`已清除 ${currency} 覆盖`)
    await loadRates()
    await refreshGlobalCurrency()
  } catch {
    ElMessage.error('清除失败')
  }
}

function sourceLabel(s: string): string {
  return ({
    coingecko: 'CoinGecko (实时主源)',
    'exchangerate.host': 'exchangerate.host (备用)',
    hardcoded: '硬编码兜底',
    disk_cache: '磁盘缓存',
    fallback: '硬编码兜底',
  } as Record<string, string>)[s] || s
}

function sourceTagType(s: string): 'success' | 'warning' | 'info' | 'danger' {
  if (s === 'coingecko') return 'success'
  if (s === 'exchangerate.host') return 'info'
  if (s === 'disk_cache') return 'warning'
  return 'danger'
}

onMounted(loadRates)
</script>

<style scoped>
.aiteam-fx { padding: 16px 24px; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.fx-source-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
}
.fx-source-label { width: 100px; color: #888; font-size: 13px; }
.fx-warning {
  margin-top: 12px;
  padding: 10px 14px;
  background: #fdf6ec;
  color: #e6a23c;
  border-radius: 6px;
  font-size: 13px;
}
</style>
