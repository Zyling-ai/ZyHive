<template>
  <div class="agents-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">AI 成员</h2>
      <el-button type="primary" @click="$router.push('/agents/new')">
        <el-icon><Plus /></el-icon> 新建 Agent
      </el-button>
    </div>

    <!-- Agent grid -->
    <el-row :gutter="16">
      <el-col :span="8" v-for="agent in store.list" :key="agent.id">
        <el-card class="agent-card" shadow="hover">
          <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
            <div class="avatar-circle" :style="{ background: agent.avatarColor || '#409eff' }">
              {{ agent.name.charAt(0) }}
            </div>
            <div style="flex: 1">
              <div style="font-weight: 600; font-size: 16px">{{ agent.name }}</div>
              <el-text type="info" size="small">{{ agent.id }}</el-text>
            </div>
            <el-tag v-if="agent.system" size="small" type="warning">系统</el-tag>
            <el-tag :type="statusType(agent.status)" size="small">{{ statusLabel(agent.status) }}</el-tag>
          </div>
          <div style="margin-bottom: 12px">
            <el-tag size="small" type="info" style="margin-right: 4px">
              {{ agent.modelId || agent.model || '未配置' }}
            </el-tag>
            <el-tag v-for="ch in (agent.channelIds || [])" :key="ch" size="small" style="margin-right: 4px">
              {{ ch }}
            </el-tag>
          </div>
          <el-text v-if="agent.description" type="info" size="small" style="display: block; margin-bottom: 12px">
            {{ agent.description }}
          </el-text>
          <div style="display:flex;gap:8px">
            <el-button type="primary" style="flex:1" @click="$router.push(`/agents/${agent.id}`)">
              <el-icon><ChatDotRound /></el-icon> 进入
            </el-button>
            <el-button v-if="!agent.system" type="danger" plain @click.stop="confirmDelete(agent.id, agent.name)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-empty v-if="!store.loading && store.list.length === 0" description="暂无 AI 成员，点击「新建 Agent」开始" />

    <!-- ═══ Agent Creation Wizard ═══ -->
    <el-dialog v-model="wizardVisible" title="新建 AI 成员" width="680px" :close-on-click-modal="false">
      <el-steps :active="wizardStep" finish-status="success" simple style="margin-bottom: 24px">
        <el-step title="基本信息" />
        <el-step title="选择模型" />
        <el-step title="消息通道" />
        <el-step title="开启能力" />
        <el-step title="安装 Skills" />
      </el-steps>

      <!-- Step 1: Basic Info -->
      <div v-show="wizardStep === 0">
        <el-form :model="wizardForm" label-width="90px">
          <el-form-item label="名称" required>
            <el-input v-model="wizardForm.name" placeholder="如：数据分析师" @input="autoId" />
          </el-form-item>
          <el-form-item label="ID">
            <el-input v-model="wizardForm.id" placeholder="英文标识" />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="wizardForm.description" type="textarea" :rows="2" placeholder="简短描述这个 Agent 的职责" />
          </el-form-item>
          <el-form-item label="头像颜色">
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <div
                v-for="color in avatarColors"
                :key="color"
                class="color-swatch"
                :class="{ active: wizardForm.avatarColor === color }"
                :style="{ background: color }"
                @click="wizardForm.avatarColor = color"
              />
            </div>
          </el-form-item>
        </el-form>
      </div>

      <!-- Step 2: Select Model -->
      <div v-show="wizardStep === 1">
        <div v-if="modelsList.length === 0" style="text-align: center; padding: 20px">
          <el-empty description="暂无已配置模型" :image-size="60" />
          <el-button type="primary" @click="$router.push('/config/models'); wizardVisible = false">
            前往配置模型
          </el-button>
        </div>
        <el-radio-group v-model="wizardForm.modelId" style="width: 100%">
          <el-card
            v-for="m in modelsList"
            :key="m.id"
            shadow="hover"
            class="select-card"
            :class="{ selected: wizardForm.modelId === m.id }"
            @click="wizardForm.modelId = m.id"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <el-radio :value="m.id" style="margin-right: 0" />
              <div style="flex: 1">
                <div style="font-weight: 600">{{ m.name }}</div>
                <el-text type="info" size="small">{{ m.provider }} / {{ m.model }}</el-text>
              </div>
              <el-tag :type="m.status === 'ok' ? 'success' : m.status === 'error' ? 'danger' : 'info'" size="small">
                {{ m.status === 'ok' ? '✓ 已配置' : m.status === 'error' ? '✗ 错误' : '? 未测试' }}
              </el-tag>
            </div>
          </el-card>
        </el-radio-group>
        <el-button link type="primary" style="margin-top: 12px" @click="$router.push('/config/models'); wizardVisible = false">
          + 配置新模型
        </el-button>
      </div>

      <!-- Step 3: Bind Channels -->
      <div v-show="wizardStep === 2">
        <div v-if="channelsList.length === 0" style="text-align: center; padding: 20px">
          <el-empty description="暂无消息通道（可跳过）" :image-size="60" />
          <el-button link type="primary" @click="$router.push('/config/channels'); wizardVisible = false">
            前往添加通道
          </el-button>
        </div>
        <el-checkbox-group v-model="wizardForm.channelIds">
          <el-card
            v-for="ch in channelsList"
            :key="ch.id"
            shadow="hover"
            class="select-card"
            :class="{ selected: wizardForm.channelIds.includes(ch.id) }"
            @click="toggleArray(wizardForm.channelIds, ch.id)"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <el-checkbox :value="ch.id" @click.stop />
              <div style="flex: 1">
                <div style="font-weight: 600">{{ ch.name }}</div>
                <el-text type="info" size="small">{{ ch.type }}</el-text>
              </div>
              <el-tag :type="ch.enabled ? 'success' : 'info'" size="small">
                {{ ch.enabled ? '启用' : '停用' }}
              </el-tag>
            </div>
          </el-card>
        </el-checkbox-group>
      </div>

      <!-- Step 4: Enable Tools -->
      <div v-show="wizardStep === 3">
        <div v-if="toolsList.length === 0" style="text-align: center; padding: 20px">
          <el-empty description="暂无能力配置（可跳过）" :image-size="60" />
          <el-button link type="primary" @click="$router.push('/config/tools'); wizardVisible = false">
            前往添加能力
          </el-button>
        </div>
        <el-checkbox-group v-model="wizardForm.toolIds">
          <el-card
            v-for="t in toolsList"
            :key="t.id"
            shadow="hover"
            class="select-card"
            :class="{ selected: wizardForm.toolIds.includes(t.id) }"
            @click="toggleArray(wizardForm.toolIds, t.id)"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <el-checkbox :value="t.id" @click.stop />
              <div style="flex: 1">
                <div style="font-weight: 600">{{ t.name }}</div>
                <el-text type="info" size="small">{{ t.type }}</el-text>
              </div>
              <el-tag :type="t.status === 'ok' ? 'success' : 'info'" size="small">
                {{ t.status === 'ok' ? '✓' : '?' }}
              </el-tag>
            </div>
          </el-card>
        </el-checkbox-group>
      </div>

      <!-- Step 5: Install Skills -->
      <div v-show="wizardStep === 4">
        <div v-if="skillsList.length === 0" style="text-align: center; padding: 20px">
          <el-empty description="暂无已安装 Skills（可跳过）" :image-size="60" />
          <el-button link type="primary" @click="$router.push('/config/skills'); wizardVisible = false">
            前往安装 Skill
          </el-button>
        </div>
        <el-checkbox-group v-model="wizardForm.skillIds">
          <el-card
            v-for="s in skillsList"
            :key="s.id"
            shadow="hover"
            class="select-card"
            :class="{ selected: wizardForm.skillIds.includes(s.id) }"
            @click="toggleArray(wizardForm.skillIds, s.id)"
          >
            <div style="display: flex; align-items: center; gap: 12px">
              <el-checkbox :value="s.id" @click.stop />
              <div style="flex: 1">
                <div style="font-weight: 600">{{ s.name }}</div>
                <el-text type="info" size="small">{{ s.description }}</el-text>
              </div>
              <el-tag size="small" type="info">v{{ s.version }}</el-tag>
            </div>
          </el-card>
        </el-checkbox-group>
      </div>

      <template #footer>
        <el-button v-if="wizardStep > 0" @click="wizardStep--">上一步</el-button>
        <el-button v-if="wizardStep < 4" type="primary" @click="nextStep">
          下一步
        </el-button>
        <el-button v-if="wizardStep === 4" type="primary" @click="createAgent" :loading="creating">
          创建 Agent
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAgentsStore } from '../stores/agents'
import {
  models as modelsApi,
  channels as channelsApi,
  tools as toolsApi,
  skills as skillsApi,
  agents as agentsApi,
  type ModelEntry,
  type ChannelEntry,
  type ToolEntry,
  type SkillEntry,
} from '../api'

const router = useRouter()
const store = useAgentsStore()

const wizardVisible = ref(false)
const wizardStep = ref(0)
const creating = ref(false)

const avatarColors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c']

const wizardForm = reactive({
  id: '',
  name: '',
  description: '',
  avatarColor: '#409eff',
  modelId: '',
  channelIds: [] as string[],
  toolIds: [] as string[],
  skillIds: [] as string[],
})

const modelsList = ref<ModelEntry[]>([])
const channelsList = ref<ChannelEntry[]>([])
const toolsList = ref<ToolEntry[]>([])
const skillsList = ref<SkillEntry[]>([])

onMounted(() => {
  store.fetchAll()
})

// kept for potential reuse
// @ts-ignore
async function openWizard() {
  wizardStep.value = 0
  Object.assign(wizardForm, {
    id: '', name: '', description: '', avatarColor: '#409eff',
    modelId: '', channelIds: [], toolIds: [], skillIds: [],
  })
  // Preload registries
  try {
    const [mRes, cRes, tRes, sRes] = await Promise.all([
      modelsApi.list(), channelsApi.list(), toolsApi.list(), skillsApi.list(),
    ])
    modelsList.value = mRes.data
    channelsList.value = cRes.data
    toolsList.value = tRes.data
    skillsList.value = sRes.data
  } catch {}
  wizardVisible.value = true
}

function autoId() {
  if (!wizardForm.id || wizardForm.id === slugify(wizardForm.name.slice(0, -1))) {
    wizardForm.id = slugify(wizardForm.name)
  }
}

function slugify(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '').slice(0, 30)
}

function nextStep() {
  if (wizardStep.value === 0 && (!wizardForm.name || !wizardForm.id)) {
    ElMessage.warning('请填写名称和 ID')
    return
  }
  wizardStep.value++
}

function toggleArray(arr: string[], val: string) {
  const idx = arr.indexOf(val)
  if (idx === -1) arr.push(val)
  else arr.splice(idx, 1)
}

async function createAgent() {
  creating.value = true
  try {
    await agentsApi.create({
      id: wizardForm.id,
      name: wizardForm.name,
      description: wizardForm.description,
      modelId: wizardForm.modelId,
      channelIds: wizardForm.channelIds,
      toolIds: wizardForm.toolIds,
      skillIds: wizardForm.skillIds,
      avatarColor: wizardForm.avatarColor,
    })
    ElMessage.success('Agent 创建成功！')
    wizardVisible.value = false
    store.fetchAll()
    router.push(`/agents/${wizardForm.id}`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    creating.value = false
  }
}

function statusType(s: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
}

async function confirmDelete(id: string, name: string) {
  try {
    await ElMessageBox.confirm(
      `删除成员「${name}」将同时删除其工作区、对话记录和所有配置，且无法恢复。确认吗？`,
      '删除 AI 成员',
      { confirmButtonText: '确认删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
  } catch { return }
  try {
    await agentsApi.delete(id)
    ElMessage.success(`已删除「${name}」`)
    await store.fetchAll()
  } catch (e: any) {
    ElMessage.error('删除失败：' + (e?.response?.data?.error ?? e?.message ?? '未知错误'))
  }
}
</script>

<style scoped>
.agent-card {
  margin-bottom: 16px;
}
.avatar-circle {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 18px;
  flex-shrink: 0;
}
.color-swatch {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  cursor: pointer;
  border: 3px solid transparent;
  transition: border-color 0.15s;
}
.color-swatch.active {
  border-color: #303133;
}
.select-card {
  margin-bottom: 8px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.select-card.selected {
  border-color: #409eff;
}
.select-card :deep(.el-card__body) {
  padding: 12px 16px;
}
</style>
