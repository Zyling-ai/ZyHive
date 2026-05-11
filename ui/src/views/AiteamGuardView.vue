<!--
  aiteam · 护栏 (PR-003 / Phase 2 § P2-S6)

  Layout:
    1. Header — title + refresh
    2. Overall cards — today's global usage + panicked count + limits
    3. Per-agent table — used / limit / panic / cooldown / actions
    4. Release dialog — operator + reason → POST /release
    5. Limit dialog — limit_usdt → PATCH /:id/limit
-->
<template>
  <div class="aiteam-guard">
    <div class="page-header">
      <h1 style="margin:0">🧪 aiteam · 护栏</h1>
      <div style="margin-left:auto">
        <el-button @click="refresh" :loading="loading">刷新</el-button>
      </div>
    </div>

    <div v-if="!snapshot" style="padding:40px;text-align:center;color:#999">
      加载中...
    </div>

    <template v-else>
      <!-- overall cards -->
      <div class="overall-cards">
        <el-card shadow="never" class="overall-card">
          <div class="card-title">今日全局支出</div>
          <div class="card-value">{{ formatMoney(snapshot.global_used_usdt) }}</div>
          <div class="card-sub">
            上限 {{ snapshot.limits.global_daily_usdt === '0' ? '不限' : formatMoney(snapshot.limits.global_daily_usdt) }}
          </div>
        </el-card>
        <el-card shadow="never" class="overall-card">
          <div class="card-title">Panic 计数</div>
          <div class="card-value" :style="{color: panickedCount > 0 ? '#f56c6c' : '#67c23a'}">
            {{ panickedCount }}
          </div>
          <div class="card-sub">{{ panickedCount > 0 ? '需要手动解封' : '系统健康' }}</div>
        </el-card>
        <el-card shadow="never" class="overall-card">
          <div class="card-title">默认 per-agent 上限</div>
          <div class="card-value" style="font-size:22px">
            {{ snapshot.limits.per_agent_daily_usdt === '0' ? '不限' : formatMoney(snapshot.limits.per_agent_daily_usdt) }}
          </div>
          <div class="card-sub">时区 {{ snapshot.tz }} · 当前日 {{ snapshot.day_key }}</div>
        </el-card>
      </div>

      <!-- per-agent table -->
      <el-card shadow="never">
        <template #header><span>各 agent 当前状态</span></template>
        <el-table :data="agentRows" size="small">
          <el-table-column prop="id" label="Agent" width="120">
            <template #default="{ row }"><strong>{{ row.id }}</strong></template>
          </el-table-column>
          <el-table-column label="今日已用">
            <template #default="{ row }">{{ formatMoney(row.used) }}</template>
          </el-table-column>
          <el-table-column label="上限">
            <template #default="{ row }">
              {{ row.limit === '0' ? '不限' : formatMoney(row.limit) }}
            </template>
          </el-table-column>
          <el-table-column label="状态" width="140">
            <template #default="{ row }">
              <el-tag v-if="row.panicked" type="danger" size="small">⚠️ {{ row.panic_reason }}</el-tag>
              <el-tag v-else type="success" size="small">正常</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="冷却到" width="180">
            <template #default="{ row }">
              <span v-if="row.cooldown_until && row.cooldown_until !== '0001-01-01T00:00:00Z'">
                {{ new Date(row.cooldown_until).toLocaleString() }}
              </span>
              <span v-else style="color:#ccc">—</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200">
            <template #default="{ row }">
              <el-button
                v-if="row.panicked"
                link size="small" type="danger"
                @click="openRelease(row.id)"
              >手动解封</el-button>
              <el-button link size="small" @click="openLimit(row.id, row.limit)">调上限</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="agentRows.length === 0" description="尚无 agent 进入护栏视野" />
      </el-card>
    </template>

    <!-- release dialog -->
    <el-dialog v-model="releaseDialog" :title="`手动解封 ${releaseForm.agentId}`" width="420px">
      <el-form label-width="80px">
        <el-form-item label="操作员">
          <el-input v-model="releaseForm.operator" placeholder="例如 owner-name" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="releaseForm.reason" type="textarea" :rows="3" placeholder="例如 manual_review_passed" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="releaseDialog = false">取消</el-button>
        <el-button type="danger" @click="submitRelease" :loading="submitting">解封</el-button>
      </template>
    </el-dialog>

    <!-- limit dialog -->
    <el-dialog v-model="limitDialog" :title="`调整 ${limitForm.agentId} 上限`" width="400px">
      <el-form label-width="80px">
        <el-form-item label="上限 USDT">
          <el-input v-model="limitForm.amount" placeholder="例如 10.00；填 0 表示不限" />
        </el-form-item>
        <p style="color:#888;font-size:12px;margin-left:80px">
          ⓘ 调整后立即生效；不影响已记录的当日累计。
        </p>
      </el-form>
      <template #footer>
        <el-button @click="limitDialog = false">取消</el-button>
        <el-button type="primary" @click="submitLimit" :loading="submitting">应用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getGuard, releaseGuard, setGuardLimit, type GuardSnapshot } from '../api/aiteam'
import { useCurrency } from '../composables/useCurrency'

const { formatMoney } = useCurrency()
const snapshot = ref<GuardSnapshot | null>(null)
const loading = ref(false)
const releaseDialog = ref(false)
const releaseForm = ref({ agentId: '', operator: '', reason: '' })
const limitDialog = ref(false)
const limitForm = ref({ agentId: '', amount: '' })
const submitting = ref(false)

async function refresh() {
  loading.value = true
  try {
    snapshot.value = await getGuard()
  } catch (e: any) {
    if (e?.response?.status === 404) {
      ElMessage.warning('护栏未启用 — 设置 ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1')
    } else {
      ElMessage.error('加载失败')
    }
  }
  loading.value = false
}

const agentRows = computed(() => {
  if (!snapshot.value?.agents) return []
  return Object.entries(snapshot.value.agents)
    .map(([id, a]) => ({
      id,
      used: a.used_daily_usdt,
      limit: a.effective_limit_usdt,
      panicked: a.panicked,
      panic_reason: a.panic_reason || '',
      cooldown_until: a.cooldown_until,
    }))
    .sort((a, b) => {
      // panicked first, then by USDT used desc
      if (a.panicked !== b.panicked) return a.panicked ? -1 : 1
      return parseFloat(b.used) - parseFloat(a.used)
    })
})

const panickedCount = computed(() => agentRows.value.filter(r => r.panicked).length)

function openRelease(agentId: string) {
  releaseForm.value = { agentId, operator: 'owner', reason: 'manual_review_passed' }
  releaseDialog.value = true
}

async function submitRelease() {
  submitting.value = true
  try {
    await releaseGuard(releaseForm.value.agentId, releaseForm.value.operator, releaseForm.value.reason)
    ElMessage.success(`已解封 ${releaseForm.value.agentId}`)
    releaseDialog.value = false
    await refresh()
  } catch (e: any) {
    ElMessage.error('解封失败: ' + (e?.response?.data?.error || e.message))
  }
  submitting.value = false
}

function openLimit(agentId: string, currentLimit: string) {
  limitForm.value = { agentId, amount: currentLimit }
  limitDialog.value = true
}

async function submitLimit() {
  // B028 fix: validate input is a non-negative finite number.
  const amt = parseFloat(limitForm.value.amount)
  if (!Number.isFinite(amt)) {
    ElMessage.warning('上限必须是数字，例如 10.00；填 0 表示不限')
    return
  }
  if (amt < 0) {
    ElMessage.warning('上限不能为负值；填 0 表示不限')
    return
  }
  if (amt > 1e9) {
    ElMessage.warning('上限不可超过 10 亿 USDT')
    return
  }
  submitting.value = true
  try {
    await setGuardLimit(limitForm.value.agentId, limitForm.value.amount)
    ElMessage.success(`已更新 ${limitForm.value.agentId} 上限 → ${limitForm.value.amount} USDT`)
    limitDialog.value = false
    await refresh()
  } catch (e: any) {
    ElMessage.error('更新失败: ' + (e?.response?.data?.error || e.message))
  }
  submitting.value = false
}

onMounted(refresh)
</script>

<style scoped>
.aiteam-guard { padding: 16px 24px; }
.page-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.overall-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}
.overall-card .card-title { font-size: 13px; color: #888; margin-bottom: 8px; }
.overall-card .card-value { font-size: 28px; font-weight: 600; color: #18181b; }
.overall-card .card-sub { font-size: 12px; color: #999; margin-top: 4px; }
</style>
