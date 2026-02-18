<template>
  <el-container class="dashboard">
    <!-- Sidebar -->
    <el-aside width="240px" class="sidebar">
      <div class="sidebar-header">
        <el-icon :size="20"><Monitor /></el-icon>
        <span class="title">AI 员工</span>
      </div>
      <el-menu :default-active="''" class="sidebar-menu">
        <el-menu-item
          v-for="agent in store.list"
          :key="agent.id"
          :index="agent.id"
          @click="$router.push(`/agents/${agent.id}`)"
        >
          <el-icon><User /></el-icon>
          <span>{{ agent.name }}</span>
          <el-tag
            :type="statusType(agent.status)"
            size="small"
            style="margin-left: auto"
          >{{ statusLabel(agent.status) }}</el-tag>
        </el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <el-button type="primary" @click="showCreate = true" style="width: 100%">
          <el-icon><Plus /></el-icon> 新建员工
        </el-button>
      </div>
    </el-aside>

    <!-- Main -->
    <el-main class="main-area">
      <div class="top-bar">
        <h2>仪表盘</h2>
        <el-button @click="$router.push('/config')">
          <el-icon><Setting /></el-icon> 配置中心
        </el-button>
      </div>

      <el-row :gutter="20">
        <el-col :span="8" v-for="agent in store.list" :key="agent.id">
          <el-card class="agent-card" shadow="hover" @click="$router.push(`/agents/${agent.id}`)">
            <div class="agent-card-header">
              <div>
                <h3>{{ agent.name }}</h3>
                <el-text type="info" size="small">{{ agent.model }}</el-text>
              </div>
              <el-tag :type="statusType(agent.status)">{{ statusLabel(agent.status) }}</el-tag>
            </div>
            <div class="agent-card-stats">
              <div class="stat">
                <span class="stat-label">ID</span>
                <span class="stat-value" style="font-size: 14px">{{ agent.id }}</span>
              </div>
              <div class="stat">
                <span class="stat-label">状态</span>
                <span class="stat-value">{{ statusLabel(agent.status) }}</span>
              </div>
            </div>
            <el-button type="primary" size="small" style="width: 100%; margin-top: 12px">
              <el-icon><ChatDotRound /></el-icon> 对话
            </el-button>
          </el-card>
        </el-col>
      </el-row>

      <el-empty v-if="!store.loading && store.list.length === 0" description="暂无 AI 员工，点击「新建员工」开始" />
    </el-main>

    <!-- Create Dialog -->
    <el-dialog v-model="showCreate" title="新建 AI 员工" width="480px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="ID">
          <el-input v-model="form.id" placeholder="英文标识，如 analyst" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如：数据分析师" />
        </el-form-item>
        <el-form-item label="模型">
          <el-select v-model="form.model" style="width: 100%">
            <el-option label="Claude Sonnet 4" value="anthropic/claude-sonnet-4-6" />
            <el-option label="Claude Opus 4" value="anthropic/claude-opus-4-0" />
            <el-option label="GPT-4o" value="openai/gpt-4o" />
            <el-option label="DeepSeek V3" value="deepseek/deepseek-chat" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createAgent" :loading="creating">创建</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useAgentsStore } from '../stores/agents'

const store = useAgentsStore()
const showCreate = ref(false)
const creating = ref(false)
const form = ref({ id: '', name: '', model: 'anthropic/claude-sonnet-4-6' })

let refreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  store.fetchAll()
  // Periodic refresh every 30s
  refreshTimer = setInterval(() => store.fetchAll(), 30000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

function statusType(s: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
}

async function createAgent() {
  if (!form.value.id || !form.value.name) {
    ElMessage.warning('请填写 ID 和名称')
    return
  }
  creating.value = true
  try {
    await store.createAgent(form.value.id, form.value.name, form.value.model)
    ElMessage.success('创建成功')
    showCreate.value = false
    form.value = { id: '', name: '', model: 'anthropic/claude-sonnet-4-6' }
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.dashboard {
  min-height: 100vh;
}
.sidebar {
  background: #fff;
  border-right: 1px solid #e4e7ed;
  display: flex;
  flex-direction: column;
}
.sidebar-header {
  padding: 20px 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid #e4e7ed;
}
.sidebar-header .title {
  font-weight: 600;
  font-size: 16px;
}
.sidebar-menu {
  flex: 1;
  border-right: none;
}
.sidebar-footer {
  padding: 16px;
}
.main-area {
  background: #f5f7fa;
}
.top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.top-bar h2 {
  margin: 0;
}
.agent-card {
  cursor: pointer;
  margin-bottom: 20px;
}
.agent-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.agent-card-header h3 {
  margin: 0 0 4px;
}
.agent-card-stats {
  display: flex;
  gap: 24px;
  margin-top: 16px;
}
.stat {
  display: flex;
  flex-direction: column;
}
.stat-label {
  font-size: 12px;
  color: #909399;
}
.stat-value {
  font-size: 18px;
  font-weight: 600;
}
</style>
