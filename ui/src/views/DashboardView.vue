<template>
  <div class="dashboard-page">
    <h2 style="margin: 0 0 20px">仪表盘</h2>

    <!-- Stats cards -->
    <el-row :gutter="16" style="margin-bottom: 24px">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #ecf5ff; color: #409eff">👥</div>
          <div class="stat-info">
            <div class="stat-value">{{ agentStore.list.length }}</div>
            <div class="stat-label">AI 成员</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #f0f9eb; color: #67c23a">✅</div>
          <div class="stat-info">
            <div class="stat-value">{{ runningCount }}</div>
            <div class="stat-label">运行中</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #fdf6ec; color: #e6a23c">🤖</div>
          <div class="stat-info">
            <div class="stat-value">{{ modelCount }}</div>
            <div class="stat-label">已配置模型</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-icon" style="background: #fef0f0; color: #f56c6c">📡</div>
          <div class="stat-info">
            <div class="stat-value">{{ channelCount }}</div>
            <div class="stat-label">消息通道</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

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
import { ref, computed, onMounted } from 'vue'
import { useAgentsStore } from '../stores/agents'
import { models as modelsApi, channels as channelsApi } from '../api'

const agentStore = useAgentsStore()
const modelCount = ref(0)
const channelCount = ref(0)

const runningCount = computed(() => agentStore.list.filter(a => a.status === 'running').length)

onMounted(async () => {
  agentStore.fetchAll()
  try {
    const [mRes, cRes] = await Promise.all([modelsApi.list(), channelsApi.list()])
    modelCount.value = mRes.data.length
    channelCount.value = cRes.data.length
  } catch {}
})

function statusType(s: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
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
