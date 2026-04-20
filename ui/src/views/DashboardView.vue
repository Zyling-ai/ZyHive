<template>
  <div class="dashboard-page">
    <h2 style="margin: 0 0 20px">仪表盘</h2>

    <!-- 未配置模型提示 -->
    <!-- 默认模型连接失败警告 -->
    <div v-if="!modelsLoading && defaultModelFailed" class="no-model-banner warn-model-banner">
      <div class="no-model-banner-left">
        <span class="no-model-banner-icon">🚫</span>
        <div>
          <div class="no-model-banner-title">默认模型「{{ defaultModelName }}」连接失败</div>
          <div class="no-model-banner-desc">当前 IP 可能被该模型提供商屏蔽（如 Anthropic 限制国内 IP）。请切换默认模型，或为 Anthropic 配置转发地址。</div>
        </div>
      </div>
      <router-link to="/config/models" class="no-model-banner-btn">去设置 →</router-link>
    </div>

    <div v-if="!modelsLoading && modelCount === 0" class="no-model-banner">
      <div class="no-model-banner-left">
        <span class="no-model-banner-icon">⚠️</span>
        <div>
          <div class="no-model-banner-title">还没有配置 AI 模型</div>
          <div class="no-model-banner-desc">添加模型 API Key 后，AI 成员才能开始工作。支持 Claude、DeepSeek、GPT-4 等。</div>
        </div>
      </div>
      <router-link to="/config/models" class="no-model-banner-btn">立即配置 →</router-link>
    </div>

    <!-- Stats cards -->
    <el-row :gutter="12" style="margin-bottom: 20px">
      <el-col :xs="12" :sm="12" :md="6" :lg="6">
        <el-card shadow="never" class="stat-card stat-card--members">
          <div class="stat-label">AI 成员</div>
          <div class="stat-value">{{ stats?.agents.total ?? agentStore.list.length }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="12" :md="6" :lg="6">
        <el-card shadow="never" class="stat-card stat-card--sessions">
          <div class="stat-label">对话总数</div>
          <div class="stat-value">{{ stats?.sessions.total ?? 0 }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="12" :md="6" :lg="6" style="margin-top: 0">
        <el-card shadow="never" class="stat-card stat-card--messages">
          <div class="stat-label">消息总数</div>
          <div class="stat-value">{{ stats?.sessions.totalMessages ?? 0 }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="12" :md="6" :lg="6" style="margin-top: 0">
        <el-card shadow="never" class="stat-card stat-card--tokens">
          <div class="stat-label">Token 用量</div>
          <div class="stat-value">{{ formatTokens(stats?.sessions.totalTokens ?? 0) }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- Top Agents card -->
    <el-card shadow="hover" style="margin-bottom: 24px" v-if="stats?.topAgents?.length">
      <template #header>
        <span style="font-weight: 600"><el-icon style="vertical-align:-2px;margin-right:4px"><DataAnalysis /></el-icon>成员用量排行</span>
      </template>
      <el-table :data="stats!.topAgents" stripe style="width: 100%">
        <el-table-column label="成员" min-width="140">
          <template #default="{ row }">
            <el-button type="primary" link @click="$router.push(`/agents/${row.id}`)">{{ row.name }}</el-button>
          </template>
        </el-table-column>
        <el-table-column label="对话数" width="100" align="center">
          <template #default="{ row }"><el-tag size="small" type="info">{{ row.sessions }}</el-tag></template>
        </el-table-column>
        <el-table-column label="消息数" width="100" align="center">
          <template #default="{ row }">{{ row.messages }}</template>
        </el-table-column>
        <el-table-column label="Token 用量" width="130" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="row.tokens > 100000 ? 'danger' : row.tokens > 50000 ? 'warning' : 'success'" effect="plain">
              {{ formatTokens(row.tokens) }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Agent status table -->
    <el-card shadow="hover">
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <span style="font-weight: 600">成员状态</span>
          <el-button type="primary" size="small" @click="$router.push('/agents')">
            管理成员
          </el-button>
        </div>
      </template>
      <el-table :data="agentStore.list" stripe style="width: 100%">
        <el-table-column label="名称" min-width="150">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px;">
              <div
                class="avatar-dot"
                :style="{ background: row.avatarColor || '#409eff' }"
              >{{ row.name.charAt(0) }}</div>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="模型" min-width="180">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.modelId || row.model || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="通道" min-width="140">
          <template #default="{ row }">
            <template v-if="row.channelIds?.length">
              <el-tag v-for="ch in row.channelIds" :key="ch" size="small" style="margin-right: 4px">{{ ch }}</el-tag>
            </template>
            <el-text v-else type="info" size="small">—</el-text>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="primary" size="small" link @click="$router.push(`/agents/${row.id}`)">
              对话
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="agentStore.list.length === 0" description="暂无 AI 成员" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { statsApi, models as modelsApi, type StatsResult } from '../api'

const agentStore = useAgentsStore()
const stats = ref<StatsResult | null>(null)
const modelCount = ref<number>(-1)   // -1 = 未加载
const modelsLoading = ref(true)
const defaultModelFailed = ref(false)  // 默认模型连接失败（403 / error）
const defaultModelName = ref('')

onMounted(async () => {
  agentStore.fetchAll()
  // 并行拉取
  await Promise.allSettled([
    statsApi.get().then(r => { stats.value = r.data }).catch(() => {}),
    modelsApi.list().then(r => {
      const list = r.data ?? []
      modelCount.value = list.length
      const def = list.find((m: any) => m.isDefault) ?? list[0]
      if (def && def.status === 'error') {
        defaultModelFailed.value = true
        defaultModelName.value = def.name || def.provider
      }
    }).catch(() => { modelCount.value = 0 }),
  ])
  modelsLoading.value = false
})

function statusType(s: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
}
function formatTokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}
</script>

<style scoped>
.warn-model-banner { background: #fef2f2; border-color: #fca5a5; }
.warn-model-banner .no-model-banner-title { color: #991b1b; }
.warn-model-banner .no-model-banner-desc { color: #7f1d1d; }
.warn-model-banner .no-model-banner-btn { background: #ef4444; }
.warn-model-banner .no-model-banner-btn:hover { background: #dc2626; }

.no-model-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 20px;
  margin-bottom: 20px;
  background: #fff8e6;
  border: 1px solid #f5a623;
  border-radius: 10px;
}
.no-model-banner-left { display: flex; align-items: center; gap: 12px; }
.no-model-banner-icon { font-size: 24px; flex-shrink: 0; }
.no-model-banner-title { font-size: 14px; font-weight: 700; color: #b45309; }
.no-model-banner-desc { font-size: 13px; color: #92400e; margin-top: 2px; line-height: 1.5; }
.no-model-banner-btn {
  flex-shrink: 0;
  padding: 8px 18px;
  background: #f59e0b;
  color: #fff;
  border-radius: 7px;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  transition: background .2s;
  white-space: nowrap;
}
.no-model-banner-btn:hover { background: #d97706; }

.stat-card {
  display: flex;
  align-items: center;
  padding: 0;
  border: 1px solid #ececec !important;
  border-radius: 10px !important;
  position: relative;
  overflow: hidden;
  transition: border-color .15s, box-shadow .15s;
}
.stat-card:hover { border-color: #d1d5db !important; box-shadow: 0 2px 8px rgba(0,0,0,0.04); }
.stat-card::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--stat-color, #cbd5e1);
}
.stat-card--members  { --stat-color: #6366f1; }
.stat-card--sessions { --stat-color: #10b981; }
.stat-card--messages { --stat-color: #f59e0b; }
.stat-card--tokens   { --stat-color: #ef4444; }
.stat-card :deep(.el-card__body) { padding: 16px 20px !important; }
.stat-value { font-size: 26px; font-weight: 700; color: #1e293b; margin-top: 6px; letter-spacing: -0.5px; }
.stat-label { font-size: 11px; color: #64748b; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 500; }
.avatar-dot {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 14px;
  flex-shrink: 0;
}
@media (max-width: 768px) {
  .stat-card :deep(.el-card__body) { padding: 12px 14px !important; }
  .stat-value { font-size: 22px; }
  .el-row { row-gap: 10px; }
}
</style>
