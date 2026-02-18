<template>
  <div class="models-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h2 style="margin: 0">🤖 模型配置</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加模型
      </el-button>
    </div>

    <!-- 环境变量检测横幅 -->
    <el-alert
      v-if="envKeys.length"
      type="success"
      :closable="false"
      style="margin-bottom: 16px"
    >
      <template #title>
        <span style="font-weight: 600">🔑 检测到系统环境变量中的 API Key</span>
      </template>
      <div style="display: flex; flex-wrap: wrap; gap: 8px; margin-top: 6px; align-items: center">
        <span v-for="ek in envKeys" :key="ek.envVar" style="display: flex; align-items: center; gap: 6px">
          <el-tag type="success" size="small">{{ ek.envVar }}</el-tag>
          <span style="font-size: 12px; color: #606266">{{ ek.masked }}</span>
          <el-button
            size="small"
            type="success"
            plain
            @click="quickAddFromEnv(ek)"
            :loading="quickAdding === ek.envVar"
          >一键添加</el-button>
        </span>
      </div>
      <div style="font-size: 12px; color: #909399; margin-top: 6px">
        已配置的 Key 无需重复添加，系统会自动识别。你也可以在下方单独配置覆盖。
      </div>
    </el-alert>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column label="提供商" width="110">
          <template #default="{ row }">
            <el-tag size="small">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="130" />
        <el-table-column label="模型 ID" min-width="190">
          <template #default="{ row }">
            <el-text type="info" size="small">{{ row.model }}</el-text>
          </template>
        </el-table-column>
        <el-table-column label="调用地址" min-width="190">
          <template #default="{ row }">
            <el-tooltip :content="row.baseUrl || defaultBaseUrl(row.provider)" placement="top">
              <el-text type="info" size="small" truncated style="max-width: 180px; display: block">
                {{ row.baseUrl || defaultBaseUrl(row.provider) }}
              </el-text>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="API Key" width="140">
          <template #default="{ row }">
            <code style="font-size: 12px; color: #909399">{{ row.apiKey }}</code>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'" size="small">
              {{ row.status === 'ok' ? '✓ 有效' : row.status === 'error' ? '✗ 无效' : '? 未测' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="默认" width="60">
          <template #default="{ row }">
            <el-tag v-if="row.isDefault" type="warning" size="small">默认</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="testModel(row)" :loading="testing === row.id">测试</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteModel(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑模型' : '添加模型'" width="580px" align-center>
      <el-form :model="form" label-width="90px" style="padding-right: 8px">

        <!-- 提供商 -->
        <el-form-item label="提供商" required>
          <el-radio-group v-model="form.provider" @change="onProviderChange" size="small">
            <el-radio-button value="anthropic">Anthropic</el-radio-button>
            <el-radio-button value="openai">OpenAI</el-radio-button>
            <el-radio-button value="deepseek">DeepSeek</el-radio-button>
            <el-radio-button value="openrouter">OpenRouter</el-radio-button>
            <el-radio-button value="custom">自定义</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 调用地址 -->
        <el-form-item label="调用地址" required>
          <el-input v-model="form.baseUrl" placeholder="https://api.anthropic.com" clearable>
            <template #append>
              <el-tooltip content="恢复提供商默认地址" placement="top">
                <el-button @click="form.baseUrl = providerPresets[form.provider] || ''" :icon="Refresh" />
              </el-tooltip>
            </template>
          </el-input>
          <div class="field-hint">中转服务填这里，比如 https://your-relay.com</div>
        </el-form-item>

        <!-- API Key -->
        <el-form-item label="API Key">
          <!-- 检测到环境变量提示 -->
          <el-alert
            v-if="currentEnvKey"
            type="info"
            :closable="false"
            style="margin-bottom: 8px; padding: 6px 10px"
          >
            <span style="font-size: 13px">
              检测到系统环境变量 <code>{{ currentEnvKey.envVar }}</code>
              <span style="color: #909399; margin-left: 4px">{{ currentEnvKey.masked }}</span>
            </span>
            <el-button
              link
              type="primary"
              size="small"
              style="margin-left: 8px"
              @click="useEnvKey"
            >使用此 Key</el-button>
            <el-button
              link
              type="info"
              size="small"
              @click="form.apiKey = '__env__'"
            >留空（自动读取）</el-button>
          </el-alert>

          <el-input
            v-model="form.apiKey"
            type="password"
            show-password
            :placeholder="currentEnvKey ? '不填则自动使用环境变量 ' + currentEnvKey.envVar : 'sk-...'"
          />
          <div class="field-hint">
            <span v-if="form.apiKey === '__env__'" style="color: var(--el-color-primary)">
              ✓ 将自动读取 {{ currentEnvKey?.envVar }} 环境变量
            </span>
            <span v-else>手动填写优先级高于环境变量</span>
          </div>
        </el-form-item>

        <!-- 获取模型 -->
        <el-form-item label=" " label-width="90px">
          <div style="display: flex; gap: 8px; width: 100%; align-items: center">
            <el-button
              @click="probeModels"
              :loading="probing"
              type="primary"
              plain
              style="flex-shrink: 0"
            >
              🔍 获取可用模型
            </el-button>
            <span v-if="probeError" style="font-size: 12px; color: var(--el-color-danger)">{{ probeError }}</span>
            <span v-else-if="probedModels.length" style="font-size: 12px; color: var(--el-color-success)">
              ✓ 获取到 {{ probedModels.length }} 个模型
            </span>
            <span v-else style="font-size: 12px; color: #909399">填写 Key 后点击获取，或直接手动填写模型 ID</span>
          </div>
        </el-form-item>

        <!-- 模型选择 -->
        <el-form-item label="模型 ID" required>
          <el-select
            v-if="probedModels.length"
            v-model="form.model"
            filterable
            placeholder="搜索或选择模型"
            style="width: 100%"
            @change="onModelSelect"
          >
            <el-option
              v-for="m in probedModels"
              :key="m.id"
              :label="m.name !== m.id ? `${m.name}  (${m.id})` : m.id"
              :value="m.id"
            />
          </el-select>
          <el-input
            v-else
            v-model="form.model"
            placeholder="如 claude-sonnet-4-6（点上方「获取可用模型」自动列出）"
            @input="autoFillName"
          />
        </el-form-item>

        <!-- 显示名称 -->
        <el-form-item label="显示名称">
          <el-input v-model="form.name" placeholder="如 Claude Sonnet 4.6" />
        </el-form-item>

        <!-- ID -->
        <el-form-item label="唯一 ID">
          <el-input v-model="form.id" placeholder="如 claude-sonnet（Agent 引用时使用）" />
        </el-form-item>

        <!-- 设为默认 -->
        <el-form-item label="设为默认">
          <el-switch v-model="form.isDefault" />
        </el-form-item>

      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveModel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { models as modelsApi, type ModelEntry, type ProbeModelInfo } from '../api'

const list = ref<ModelEntry[]>([])
const dialogVisible = ref(false)
const editingId = ref('')
const saving = ref(false)
const testing = ref('')
const probing = ref(false)
const probeError = ref('')
const probedModels = ref<ProbeModelInfo[]>([])
const quickAdding = ref('')

type EnvKey = { provider: string; envVar: string; masked: string; baseUrl: string }
const envKeys = ref<EnvKey[]>([])

// Provider → default base URL
const providerPresets: Record<string, string> = {
  anthropic:  'https://api.anthropic.com',
  openai:     'https://api.openai.com',
  deepseek:   'https://api.deepseek.com',
  openrouter: 'https://openrouter.ai/api',
  custom:     '',
}

function defaultBaseUrl(provider: string) {
  return providerPresets[provider] || '—'
}

// Currently detected env key for the selected provider
const currentEnvKey = computed<EnvKey | null>(() => {
  return envKeys.value.find(ek => ek.provider === form.provider) || null
})

const form = reactive({
  id: '',
  name: '',
  provider: 'anthropic',
  model: '',
  apiKey: '',
  baseUrl: 'https://api.anthropic.com',
  isDefault: false,
})

onMounted(async () => {
  await Promise.all([loadList(), loadEnvKeys()])
})

async function loadList() {
  try {
    const res = await modelsApi.list()
    list.value = res.data
  } catch {}
}

async function loadEnvKeys() {
  try {
    const res = await modelsApi.envKeys()
    envKeys.value = res.data.envKeys || []
  } catch {}
}

function onProviderChange() {
  form.baseUrl = providerPresets[form.provider] || ''
  form.model = ''
  probedModels.value = []
  probeError.value = ''
  // OpenRouter auto-probe since it's public (no key needed)
  if (form.provider === 'openrouter') {
    probeModels()
  }
}

function useEnvKey() {
  // Fill in a placeholder so user knows it's set, but actual value isn't shown
  if (currentEnvKey.value) {
    form.apiKey = currentEnvKey.value.masked
    ElMessage.info('已填入（显示为掩码，实际使用环境变量值）')
  }
}

function onModelSelect(modelId: string) {
  const found = probedModels.value.find(m => m.id === modelId)
  if (found) {
    form.name = (found.name && found.name !== found.id) ? found.name : modelId
  }
  if (!form.id) {
    form.id = modelId.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
  }
}

function autoFillName() {
  if (!form.name) form.name = form.model
  if (!form.id) {
    form.id = form.model.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
  }
}

async function probeModels() {
  if (!form.baseUrl) {
    probeError.value = '请先填写调用地址'
    return
  }
  probing.value = true
  probeError.value = ''
  probedModels.value = []

  // Resolve API key: use form value if non-masked, otherwise try env key
  let apiKey = form.apiKey
  if (!apiKey || apiKey.includes('***')) {
    // Try to use env key value via backend (it knows the real value)
    apiKey = ''
  }

  try {
    const res = await modelsApi.probe(form.baseUrl, apiKey || undefined, form.provider)
    probedModels.value = res.data.models || []
    if (!probedModels.value.length) {
      probeError.value = '未获取到模型列表（接口返回为空）'
    }
  } catch (e: any) {
    probeError.value = e.response?.data?.error || e.message || '获取失败'
  } finally {
    probing.value = false
  }
}

// One-click add from env key banner
async function quickAddFromEnv(ek: EnvKey) {
  quickAdding.value = ek.envVar
  try {
    // Check if already added for this provider
    const existing = list.value.find(m => m.provider === ek.provider)
    if (existing) {
      ElMessage.warning(`${ek.provider} 已有配置，请直接编辑`)
      return
    }
    // Open dialog pre-filled
    editingId.value = ''
    probedModels.value = []
    probeError.value = ''
    Object.assign(form, {
      id: ek.provider + '-default',
      name: capitalize(ek.provider) + ' (env)',
      provider: ek.provider,
      model: '',
      apiKey: ek.masked,
      baseUrl: ek.baseUrl,
      isDefault: list.value.length === 0,
    })
    dialogVisible.value = true
    // Auto-probe if OpenRouter
    if (ek.provider === 'openrouter') probeModels()
  } finally {
    quickAdding.value = ''
  }
}

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function openAdd() {
  editingId.value = ''
  probedModels.value = []
  probeError.value = ''
  Object.assign(form, {
    id: '', name: '', provider: 'anthropic', model: '',
    apiKey: '', baseUrl: providerPresets.anthropic, isDefault: false,
  })
  dialogVisible.value = true
}

function openEdit(row: ModelEntry) {
  editingId.value = row.id
  probedModels.value = []
  probeError.value = ''
  Object.assign(form, {
    id: row.id,
    name: row.name,
    provider: row.provider,
    model: row.model,
    apiKey: row.apiKey,
    baseUrl: row.baseUrl || providerPresets[row.provider] || '',
    isDefault: row.isDefault,
  })
  dialogVisible.value = true
}

async function saveModel() {
  if (!form.id || !form.provider || !form.model) {
    ElMessage.warning('请填写必要字段（唯一ID / 提供商 / 模型 ID）')
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    if (editingId.value) {
      await modelsApi.update(editingId.value, payload as any)
    } else {
      await modelsApi.create({ ...payload, status: 'untested' } as any)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    loadList()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function testModel(row: ModelEntry) {
  testing.value = row.id
  try {
    const res = await modelsApi.test(row.id)
    if (res.data.valid) {
      ElMessage.success('连接成功！')
    } else {
      ElMessage.error('连接失败: ' + (res.data.error || ''))
    }
    loadList()
  } catch {
    ElMessage.error('测试请求失败')
  } finally {
    testing.value = ''
  }
}

async function deleteModel(row: ModelEntry) {
  try {
    await ElMessageBox.confirm(`确定删除模型 "${row.name}"？`, '确认删除', { type: 'warning' })
    await modelsApi.delete(row.id)
    ElMessage.success('已删除')
    loadList()
  } catch {}
}
</script>

<style scoped>
.models-page { padding: 0; }
.field-hint {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 4px;
  line-height: 1.4;
}
</style>
