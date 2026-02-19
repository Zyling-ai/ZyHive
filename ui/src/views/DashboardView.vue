<template>
  <div class="dashboard-page">
    <h2 style="margin: 0 0 20px">仪表盘</h2>

    <!-- Stats cards -->
    <el-row :gutter="16" style="margin-bottom: 24px">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #ecf5ff; color: #409eff">👥</div>
          <div class="stat-info">
            <div class="stat-value">{{ stats?.agents.total ?? agentStore.list.length }}</div>
            <div class="stat-label">AI 成员</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #f0f9eb; color: #67c23a">💬</div>
          <div class="stat-info">
            <div class="stat-value">{{ stats?.sessions.total ?? 0 }}</div>
            <div class="stat-label">对话总数</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #fdf6ec; color: #e6a23c">📨</div>
          <div class="stat-info">
            <div class="stat-value">{{ stats?.sessions.totalMessages ?? 0 }}</div>
            <div class="stat-label">消息总数</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #fef0f0; color: #f56c6c">🔢</div>
          <div class="stat-info">
            <div class="stat-value">{{ formatTokens(stats?.sessions.totalTokens ?? 0) }}</div>
            <div class="stat-label">Token 用量</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- Top Agents card -->
    <el-card shadow="hover" style="margin-bottom: 24px" v-if="stats?.topAgents?.length">
      <template #header>
        <span style="font-weight: 600">📊 成员用量排行</span>
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
import { statsApi, type StatsResult } from '../api'

const agentStore = useAgentsStore()
const stats = ref<StatsResult | null>(null)

onMounted(async () => {
  agentStore.fetchAll()
  try {
    const res = await statsApi.get()
    stats.value = res.data
  } catch {}
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
.stat-card {
  display: flex;
  align-items: center;
  padding: 0;
}
.stat-card :deep(.el-card__body) {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
}
.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}
.stat-info { flex: 1; }
.stat-value { font-size: 24px; font-weight: 700; line-height: 1.2; }
.stat-label { font-size: 13px; color: #909399; }
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
</style>
