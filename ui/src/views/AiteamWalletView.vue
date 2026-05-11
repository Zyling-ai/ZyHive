<!--
  aiteam · 钱包 (PR-001 / Phase 2 § P2-S5)

  Layout:
    1. Header — page title + refresh button + (owner-only) "入金" button
    2. Top — balance leaderboard cards (top 5 by USDT balance)
    3. Middle — agent picker + selected agent's balance large display
    4. Bottom — full ledger table for selected agent

  All money values pass through useCurrency.formatMoney() so the
  display follows the user's top-bar 💱 selection.
-->
<template>
  <div class="aiteam-wallet">
    <div class="page-header">
      <h1 style="margin:0">🧪 aiteam · 钱包</h1>
      <div style="margin-left:auto;display:flex;gap:8px">
        <el-button @click="refreshOverview" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="creditDialog = true">+ 入金</el-button>
      </div>
    </div>

    <!-- 1. leaderboard -->
    <el-card shadow="never" style="margin-bottom:20px">
      <template #header><span>余额排行（前 5）</span></template>
      <div v-if="leaderboard.length === 0" style="color:#999;padding:20px;text-align:center">
        暂无钱包数据 — 通过"入金"按钮给 agent 创建钱包
      </div>
      <div v-else class="leaderboard">
        <div v-for="(row, idx) in leaderboard" :key="row.agentId" class="leader-item">
          <span class="leader-rank">#{{ idx + 1 }}</span>
          <span class="leader-id">{{ row.agentId }}</span>
          <span class="leader-bal">{{ formatMoney(row.balance) }}</span>
        </div>
      </div>
    </el-card>

    <!-- 2. agent picker + balance large -->
    <el-card shadow="never" style="margin-bottom:20px">
      <template #header><span>查看 agent</span></template>
      <el-select
        v-model="selectedAgent"
        placeholder="选择 agent"
        filterable
        style="width: 280px"
        @change="onAgentChange"
      >
        <el-option v-for="id in allAgentIds" :key="id" :label="id" :value="id" />
      </el-select>

      <div v-if="walletData" class="balance-display">
        <div class="balance-label">{{ selectedAgent }} 当前余额</div>
        <div class="balance-value">{{ formatMoney(walletData.balance_usdt) }}</div>
        <div class="balance-sub">{{ walletData.balance_usdt }} USDT</div>
      </div>
    </el-card>

    <!-- 3. ledger -->
    <el-card v-if="walletData" shadow="never">
      <template #header>
        <span>账本（最近 20 条）</span>
        <span style="float:right">
          <el-button link size="small" @click="downloadCSV" type="primary">📥 CSV 导出（全部账本）</el-button>
        </span>
      </template>
      <el-table :data="walletData.recent_ledger" size="small" max-height="500">
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ new Date(row.ts).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="type" label="类型" width="120">
          <template #default="{ row }">
            <el-tag :type="ledgerTagType(row.type)" size="small">{{ ledgerLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="金额" width="160">
          <template #default="{ row }">
            <span :style="{ color: isDebit(row.type) ? '#f56c6c' : '#67c23a' }">
              {{ isDebit(row.type) ? '-' : '+' }}{{ formatMoney(row.amount_usdt) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="变动后余额" width="160">
          <template #default="{ row }">{{ formatMoney(row.balance_after_usdt) }}</template>
        </el-table-column>
        <el-table-column prop="reason" label="备注" min-width="180" />
        <el-table-column prop="counterparty" label="对方" width="120" />
      </el-table>
    </el-card>

    <!-- credit dialog (owner-only operation) -->
    <el-dialog v-model="creditDialog" title="给 agent 钱包入金" width="420px">
      <el-form label-width="100px">
        <el-form-item label="agent">
          <el-select v-model="creditForm.agentId" filterable placeholder="选择 agent" style="width:100%">
            <el-option v-for="id in allAgentIds" :key="id" :label="id" :value="id" />
          </el-select>
        </el-form-item>
        <el-form-item label="金额 (USDT)">
          <el-input v-model="creditForm.amount" placeholder="例如 5.00" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="creditForm.reason" placeholder="例如 genesis / monthly_topup" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="creditDialog = false">取消</el-button>
        <el-button type="primary" @click="submitCredit" :loading="submitting">入金</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getWallet, creditWallet, getOverview, type WalletResponse } from '../api/aiteam'
import { useCurrency } from '../composables/useCurrency'

const { formatMoney } = useCurrency()

const loading = ref(false)
const overview = ref<{ agents: Record<string, string>; count: number } | null>(null)
const selectedAgent = ref('')
const walletData = ref<WalletResponse | null>(null)
const allAgentIds = ref<string[]>([])

const creditDialog = ref(false)
const creditForm = ref({ agentId: '', amount: '', reason: '' })
const submitting = ref(false)

const leaderboard = computed(() => {
  if (!overview.value?.agents) return []
  return Object.entries(overview.value.agents)
    .map(([agentId, balance]) => ({ agentId, balance, n: parseFloat(balance) }))
    .filter(r => Number.isFinite(r.n))
    .sort((a, b) => b.n - a.n)
    .slice(0, 5)
})

async function refreshOverview() {
  loading.value = true
  try {
    const o = await getOverview()
    overview.value = o.wallet || { agents: {}, count: 0 }
    allAgentIds.value = Object.keys(overview.value.agents || {}).sort()
    // Auto-pick first agent if none selected.
    if (!selectedAgent.value && allAgentIds.value.length > 0) {
      selectedAgent.value = allAgentIds.value[0] || ''
      await onAgentChange(selectedAgent.value)
    } else if (selectedAgent.value) {
      // Refresh selected agent too.
      await onAgentChange(selectedAgent.value)
    }
  } catch (e: any) {
    if (e?.response?.status === 404) {
      ElMessage.warning('钱包未启用 — 设置 ZYHIVE_EXPERIMENTAL_WALLET=1')
    } else {
      ElMessage.error('加载失败')
    }
  }
  loading.value = false
}

async function onAgentChange(agentId: string) {
  if (!agentId) {
    walletData.value = null
    return
  }
  try {
    walletData.value = await getWallet(agentId)
  } catch {
    walletData.value = null
  }
}

function downloadCSV() {
  if (!selectedAgent.value) return
  // Build URL with auth token in query (CSV download is a navigation,
  // bearer header won't be sent). We use a query token pattern.
  // For simplicity, just open a new tab; modern browsers will use the
  // already-authenticated session via API's auth middleware. If the
  // bearer token approach is required, generate a one-time download
  // token endpoint — out of scope for P3-S3.
  const token = localStorage.getItem('aipanel_token') || ''
  // axios-style header isn't applicable for browser nav. We embed a
  // temporary form POST or just rely on Authorization header via
  // fetch + blob conversion.
  fetch(`/api/aiteam/wallet/${encodeURIComponent(selectedAgent.value)}/ledger.csv`, {
    headers: { 'Authorization': `Bearer ${token}` },
  }).then(r => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`)
    return r.blob()
  }).then(blob => {
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `ledger-${selectedAgent.value}-${new Date().toISOString().slice(0,10)}.csv`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    ElMessage.success('CSV 导出完成')
  }).catch((e) => {
    ElMessage.error(`导出失败: ${e.message}`)
  })
}

async function submitCredit() {
  if (!creditForm.value.agentId || !creditForm.value.amount) {
    ElMessage.warning('请填 agent + 金额')
    return
  }
  // B025 fix: validate amount is a positive finite number before sending.
  const amt = parseFloat(creditForm.value.amount)
  if (!Number.isFinite(amt)) {
    ElMessage.warning('金额必须是数字，例如 5.00')
    return
  }
  if (amt <= 0) {
    ElMessage.warning('金额必须大于 0')
    return
  }
  // Reject absurd inputs that would corrupt the ledger (>1e9 USDT in a single op).
  if (amt > 1e9) {
    ElMessage.warning('单次入金不可超过 10 亿 USDT')
    return
  }
  submitting.value = true
  try {
    await creditWallet(creditForm.value.agentId, creditForm.value.amount, creditForm.value.reason)
    ElMessage.success(`已给 ${creditForm.value.agentId} 入金 ${creditForm.value.amount} USDT`)
    creditDialog.value = false
    creditForm.value = { agentId: '', amount: '', reason: '' }
    await refreshOverview()
  } catch (e: any) {
    ElMessage.error('入金失败: ' + (e?.response?.data?.error || e.message))
  }
  submitting.value = false
}

function ledgerLabel(type: string): string {
  return ({
    genesis: '初始',
    credit: '入金',
    debit: '扣款',
    transfer_in: '转入',
    transfer_out: '转出',
  } as Record<string, string>)[type] || type
}

function ledgerTagType(type: string): 'success' | 'info' | 'warning' | 'danger' | 'primary' {
  if (type === 'credit' || type === 'genesis' || type === 'transfer_in') return 'success'
  if (type === 'debit' || type === 'transfer_out') return 'warning'
  return 'info'
}

function isDebit(type: string): boolean {
  return type === 'debit' || type === 'transfer_out'
}

onMounted(refreshOverview)
</script>

<style scoped>
.aiteam-wallet { padding: 16px 24px; }
.page-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}
.leaderboard {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}
.leader-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  background: #fafafa;
  border-radius: 8px;
}
.leader-rank { font-weight: 600; color: #999; font-size: 13px; }
.leader-id { flex: 1; font-weight: 500; }
.leader-bal { color: #18181b; font-weight: 600; }
.balance-display {
  margin-top: 16px;
  padding: 24px;
  background: linear-gradient(135deg, #f3f4f6, #ffffff);
  border-radius: 12px;
}
.balance-label { font-size: 13px; color: #888; }
.balance-value { font-size: 36px; font-weight: 700; color: #18181b; margin: 4px 0; }
.balance-sub { font-size: 12px; color: #999; font-family: monospace; }
</style>
