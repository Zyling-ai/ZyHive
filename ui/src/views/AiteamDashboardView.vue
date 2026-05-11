<!--
  aiteam · 总览 (PR-006 + Phase 2 § P2-S4 / S7)

  P2-S4 (this commit) ships the skeleton: layout + flag-gated 4-card
  overview pulled from /api/aiteam/overview, plus a 50-row audit log
  tail at the bottom. P2-S7 will add a Genesis demo walkthrough video
  link + polish.

  All money values pass through useCurrency.formatMoney() so the
  user's choice in the top-bar 💱 dropdown re-renders this view.
-->
<template>
  <div class="aiteam-dashboard">
    <h1 style="margin:0 0 16px">🧪 aiteam · 总览</h1>

    <div v-if="loading" class="aiteam-loading">
      <el-skeleton :rows="4" animated />
    </div>

    <div v-else-if="!overview?.any" class="aiteam-disabled">
      <el-empty description="aiteam 实验性功能尚未启用">
        <template #image>
          <span style="font-size:48px">🧪</span>
        </template>
        <p style="color:#888;margin-top:8px;font-size:13px;max-width:480px">
          设置一个或多个 ZYHIVE_EXPERIMENTAL_* 环境变量启用对应子系统，
          重启 ZyHive 后此页面会自动出现总览数据。
        </p>
        <p style="color:#888;font-size:12px">
          详见 <a href="https://github.com/Zyling-ai/zyhive/blob/main/docs/aiteam-architecture.md" target="_blank">docs/aiteam-architecture.md</a>
        </p>
      </el-empty>
    </div>

    <template v-else>
      <div class="aiteam-cards">
        <el-card class="aiteam-card" shadow="hover" @click="$router.push('/aiteam/wallet')">
          <div class="aiteam-card-title">💰 钱包总额</div>
          <div class="aiteam-card-value">
            {{ overview.wallet
                ? formatMoney(overview.wallet.total_balance_usdt)
                : '—' }}
          </div>
          <div class="aiteam-card-sub">
            {{ overview.wallet ? `${overview.wallet.count} 名持有者` : '钱包未启用' }}
          </div>
        </el-card>

        <el-card class="aiteam-card" shadow="hover" @click="$router.push('/aiteam/guard')">
          <div class="aiteam-card-title">🛡 当日支出</div>
          <div class="aiteam-card-value">
            {{ overview.guard
                ? formatMoney(overview.guard.global_used_usdt)
                : '—' }}
          </div>
          <div class="aiteam-card-sub">
            {{ panickedCount > 0 ? `⚠️ ${panickedCount} agent 触发熔断` : '无 panic' }}
          </div>
        </el-card>

        <el-card class="aiteam-card" shadow="hover" @click="$router.push('/aiteam/judge')">
          <div class="aiteam-card-title">⭐ Judge 平均分</div>
          <div class="aiteam-card-value">
            {{ judgeAvg !== null ? judgeAvg.toFixed(2) + ' / 10' : '—' }}
          </div>
          <div class="aiteam-card-sub">
            {{ overview.judge ? `${overview.judge.agents.length} agent 已评` : 'judge 未启用' }}
          </div>
        </el-card>

        <el-card class="aiteam-card" shadow="hover" @click="$router.push('/aiteam/payroll')">
          <div class="aiteam-card-title">💳 工资 / 收入</div>
          <div class="aiteam-card-value">
            {{ overview.payroll?.enabled ? '已配置' : '未启用' }}
            <span v-if="overview.revenue?.enabled" style="margin-left:8px;font-size:14px;color:#67c23a">+收入</span>
          </div>
          <div class="aiteam-card-sub">详情见专属页面</div>
        </el-card>
      </div>

      <!-- 启用子系统状态 -->
      <el-card style="margin-top:24px" shadow="never">
        <template #header><span>子系统启用状态</span></template>
        <div class="aiteam-flags-grid">
          <span v-for="(on, key) in overview.flags" :key="key" :class="['aiteam-flag', on ? 'on' : 'off']">
            {{ flagLabel(key) }}
            <span class="aiteam-flag-state">{{ on ? '✓' : '○' }}</span>
          </span>
        </div>
      </el-card>

      <!-- Audit timeline -->
      <el-card style="margin-top:24px" shadow="never">
        <template #header>
          <span>近期审计日志（50 条）</span>
          <el-button link size="small" @click="refresh" style="float:right">刷新</el-button>
        </template>
        <el-table :data="auditTail" size="small" max-height="360">
          <el-table-column prop="ts" label="时间" width="170">
            <template #default="{ row }">{{ new Date(row.ts).toLocaleString() }}</template>
          </el-table-column>
          <el-table-column prop="type" label="事件" width="220" />
          <el-table-column prop="agentId" label="Agent" width="120">
            <template #default="{ row }">{{ row.agentId || '—' }}</template>
          </el-table-column>
          <el-table-column label="详情">
            <template #default="{ row }">
              <code style="font-size:11px">{{ JSON.stringify(row.detail || {}).slice(0, 240) }}</code>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-if="!auditTail.length" description="暂无审计事件" />
      </el-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getOverview, getAuditTail, type OverviewResponse, type AuditEntry } from '../api/aiteam'
import { useCurrency } from '../composables/useCurrency'

const overview = ref<OverviewResponse | null>(null)
const auditTail = ref<AuditEntry[]>([])
const loading = ref(true)
const { formatMoney } = useCurrency()

async function refresh() {
  loading.value = true
  try {
    overview.value = await getOverview()
  } catch {
    overview.value = { flags: {}, any: false }
  }
  try {
    const r = await getAuditTail(50)
    // newest first for the dashboard timeline
    auditTail.value = (r.entries || []).slice().reverse()
  } catch {
    auditTail.value = []
  }
  loading.value = false
}

const panickedCount = computed(() => {
  if (!overview.value?.guard?.agents) return 0
  return Object.values(overview.value.guard.agents).filter(a => a.panicked).length
})

const judgeAvg = computed<number | null>(() => {
  const j = overview.value?.judge
  if (!j || !j.avg_7d_by_agent) return null
  const vals = Object.values(j.avg_7d_by_agent)
  if (vals.length === 0) return null
  return vals.reduce((a, b) => a + b, 0) / vals.length
})

const FLAG_LABELS: Record<string, string> = {
  ZYHIVE_EXPERIMENTAL_WALLET: '钱包',
  ZYHIVE_EXPERIMENTAL_BUDGETGUARD: '护栏',
  ZYHIVE_EXPERIMENTAL_JUDGE: 'Judge',
  ZYHIVE_EXPERIMENTAL_PAYROLL: '工资',
  ZYHIVE_EXPERIMENTAL_REVENUE: '收入',
  ZYHIVE_EXPERIMENTAL_SANDBOX: '沙箱',
  ZYHIVE_EXPERIMENTAL_PROMPTDEF: '注入防御',
  ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD: '总览面板',
}
function flagLabel(key: string | number): string {
  const k = String(key)
  return FLAG_LABELS[k] ?? k.replace(/^ZYHIVE_EXPERIMENTAL_/, '')
}

onMounted(refresh)
</script>

<style scoped>
.aiteam-dashboard { padding: 16px 24px; }
.aiteam-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}
.aiteam-card { cursor: pointer; transition: transform 0.15s; }
.aiteam-card:hover { transform: translateY(-2px); }
.aiteam-card-title { font-size: 13px; color: #888; margin-bottom: 8px; }
.aiteam-card-value { font-size: 28px; font-weight: 600; color: #18181b; }
.aiteam-card-sub { font-size: 12px; color: #999; margin-top: 4px; }
.aiteam-flags-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.aiteam-flag {
  padding: 6px 12px;
  border-radius: 16px;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.aiteam-flag.on { background: #f0f9eb; color: #67c23a; }
.aiteam-flag.off { background: #f5f5f5; color: #999; }
.aiteam-flag-state { font-weight: 600; }
.aiteam-loading { padding: 40px; }
.aiteam-disabled { padding: 40px; text-align: center; }
</style>
