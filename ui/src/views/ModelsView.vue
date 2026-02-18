<template>
  <div class="models-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">🤖 模型配置</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加模型
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column label="提供商" width="110">
          <template #default="{ row }">
            <el-tag size="small">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="模型 ID" min-width="200">
          <template #default="{ row }">
            <el-text type="info" size="small">{{ row.model }}</el-text>
          </template>
        </el-table-column>
        <el-table-column label="调用地址" min-width="200">
          <template #default="{ row }">
            <el-text v-if="row.baseUrl" type="info" size="small" truncated>{{ row.baseUrl }}</el-text>
            <el-text v-else type="placeholder" size="small">{{ defaultBaseUrl(row.provider) }}</el-text>
          </template>
        </el-table-column>
        <el-table-column label="API Key" width="150">
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
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑模型' : '添加模型'" width="560px" align-center>
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
              <el-tooltip content="使用提供商默认地址" placement="top">
                <el-button @click="form.baseUrl = providerPresets[form.provider] || ''" :icon="Refresh" />
              </el-tooltip>
            </template>
          </el-input>
          <div class="field-hint">模型 API 的 Base URL（中转地址填这里）</div>
        </el-form-item>

        <!-- API Key + 获取按钮 -->
        <el-form-item label="API Key">
          <div style="display: flex; gap: 8px; width: 100%">
            <el-input
              v-model="form.apiKey"
              type="password"
              show-password
              placeholder="sk-... （OpenRouter 公开列表无需填写）"
              style="flex: 1"
            />
            <el-button
              @click="probeModels"
              :loading="probing"
              type="primary"
              plain
              style="white-space: nowrap"
            >
              获取模型
            </el-button>
          </div>
          <div v-if="probeError" class="field-hint" style="color: var(--el-color-danger)">{{ probeError }}</div>
          <div v-else-if="probedModels.length" class="field-hint" style="color: var(--el-color-success)">
            ✓ 获取到 {{ probedModels.length }} 个模型
          </div>
        </el-form-item>

        <!-- 模型选择 -->
        <el-form-item label="模型" required>
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
            placeholder="手动填写模型 ID，或点「获取模型」自动列出"
            @input="autoFillName"
          />
        </el-form-item>

        <!-- 显示名称 -->
        <el-form-item label="显示名称">
          <el-input v-model="form.name" placeholder="如 Claude Sonnet 4.6" />
        </el-form-item>

        <!-- ID -->
        <el-form-item label="唯一 ID">
          <el-input v-model="form.id" placeholder="如 claude-sonnet" />
          <div class="field-hint">在 Agent 中引用此模型时使用的标识</div>
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
import { ref, reactive, onMounted } from 'vue'
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

const form = reactive({
  id: '',
  name: '',
  provider: 'anthropic',
  model: '',
  apiKey: '',
  baseUrl: 'https://api.anthropic.com',
  isDefault: false,
})

onMounted(loadList)

async function loadList() {
  try {
    const res = await modelsApi.list()
    list.value = res.data
  } catch {}
}

function onProviderChange() {
  form.baseUrl = providerPresets[form.provider] || ''
  form.model = ''
  probedModels.value = []
  probeError.value = ''
  // OpenRouter: auto-probe since it's public
  if (form.provider === 'openrouter') {
    probeModels()
  }
}

function onModelSelect(modelId: string) {
  const found = probedModels.value.find(m => m.id === modelId)
  if (found) {
    // Auto-fill display name (use name if it differs from id)
    if (found.name && found.name !== found.id) {
      form.name = found.name
    } else {
      form.name = modelId
    }
  }
  // Auto-generate ID if empty
  if (!form.id) {
    form.id = modelId.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
  }
}

function autoFillName() {
  // When manually typing model ID, auto-suggest name
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
  try {
    const res = await modelsApi.probe(form.baseUrl, form.apiKey || undefined)
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
    ElMessage.warning('请填写必要字段（ID / 提供商 / 模型）')
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
.models-page {
  padding: 0;
}
.field-hint {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 4px;
  line-height: 1.4;
}
</style>
