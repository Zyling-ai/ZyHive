<template>
  <div class="models-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h2 style="margin: 0">模型配置</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加模型
      </el-button>
    </div>

    <!-- 环境变量检测横幅 -->
    <el-alert v-if="envKeys.length" type="success" :closable="false" style="margin-bottom: 16px">
      <template #title>
        <span style="font-weight: 600"><el-icon style="vertical-align:-2px;margin-right:4px"><Key /></el-icon>检测到系统环境变量中的 API Key</span>
      </template>
      <div style="display: flex; flex-wrap: wrap; gap: 8px; margin-top: 6px; align-items: center">
        <span v-for="ek in envKeys" :key="ek.envVar" style="display: flex; align-items: center; gap: 6px">
          <el-tag type="success" size="small">{{ ek.envVar }}</el-tag>
          <span style="font-size: 12px; color: #606266">{{ ek.masked }}</span>
          <el-button size="small" type="success" plain @click="quickAddFromEnv(ek)" :loading="quickAdding === ek.envVar">一键添加</el-button>
        </span>
      </div>
      <div style="font-size: 12px; color: #909399; margin-top: 6px">已配置的 Key 无需重复添加，系统会自动识别。</div>
    </el-alert>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column label="提供商" width="110">
          <template #default="{ row }">
            <el-tag size="small">{{ providerMeta[row.provider]?.label || row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="130" />
        <el-table-column label="模型 ID" min-width="190">
          <template #default="{ row }"><el-text type="info" size="small">{{ row.model }}</el-text></template>
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
        <el-table-column label="API Key" width="160">
          <template #default="{ row }">
            <el-tag v-if="!row.apiKey" type="info" size="small" style="font-size: 11px">
              <el-icon style="vertical-align:-2px;margin-right:4px"><Connection /></el-icon>使用环境变量
            </el-tag>
            <code v-else style="font-size: 12px; color: #909399">{{ row.apiKey }}</code>
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
          <template #default="{ row }"><el-tag v-if="row.isDefault" type="warning" size="small">默认</el-tag></template>
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
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑模型' : '添加模型'" width="600px" align-center>
      <el-form :model="form" label-width="90px" style="padding-right: 8px">

        <!-- 提供商网格 -->
        <el-form-item label="提供商" required>
          <div class="provider-grid">
            <button
              v-for="p in providers"
              :key="p.key"
              type="button"
              class="provider-card"
              :class="{ active: form.provider === p.key }"
              @click="setProvider(p.key)"
            >
              <span class="provider-icon">{{ p.icon }}</span>
              <span class="provider-label">{{ p.label }}</span>
            </button>
          </div>
        </el-form-item>

        <!-- 提供商引导信息 -->
        <el-form-item label=" " label-width="90px" v-if="currentMeta">
          <div class="provider-guide">
            <div class="guide-row">
              <span class="guide-icon">🔑</span>
              <span class="guide-text">{{ currentMeta.apiKeyHint }}</span>
              <a :href="currentMeta.apiKeyUrl" target="_blank" class="guide-link">获取 API Key →</a>
            </div>
            <div v-if="currentMeta.compatible" class="guide-row">
              <span class="guide-icon">🔗</span>
              <span class="guide-text">OpenAI 兼容接口，也支持其他兼容此格式的中转服务</span>
            </div>
            <div v-if="currentMeta.keyFormat" class="guide-row">
              <span class="guide-icon">📋</span>
              <span class="guide-text">Key 格式：<code>{{ currentMeta.keyFormat }}</code></span>
            </div>
            <div class="guide-row">
              <span class="guide-icon">🌐</span>
              <a :href="currentMeta.website" target="_blank" class="guide-link">访问官网</a>
            </div>
          </div>
        </el-form-item>

        <!-- 调用地址 -->
        <el-form-item label="调用地址" required>
          <el-input v-model="form.baseUrl" :placeholder="currentMeta?.baseUrl || 'https://...'" clearable>
            <template #append>
              <el-tooltip content="恢复提供商默认地址" placement="top">
                <el-button @click="form.baseUrl = defaultBaseUrl(form.provider)" :icon="Refresh" />
              </el-tooltip>
            </template>
          </el-input>
          <div class="field-hint">中转服务填这里，比如 https://your-relay.com</div>
        </el-form-item>

        <!-- API Key -->
        <el-form-item label="API Key">
          <el-alert v-if="currentEnvKey" type="success" :closable="false" style="margin-bottom: 8px; padding: 6px 12px">
            <span style="font-size: 13px">
              <el-icon style="vertical-align:-2px;margin-right:4px"><CircleCheck /></el-icon>检测到 <code>{{ currentEnvKey.envVar }}</code>（{{ currentEnvKey.masked }}）— <strong>不填 API Key 即可自动使用</strong>
            </span>
          </el-alert>
          <el-input
            v-model="form.apiKey"
            type="password"
            show-password
            :placeholder="currentEnvKey ? '留空 = 自动读取 ' + currentEnvKey.envVar : (currentMeta?.keyFormat || 'sk-...')"
          />
          <div class="field-hint">
            <span v-if="!form.apiKey && currentEnvKey" style="color: var(--el-color-success)">✓ 留空后将自动使用 {{ currentEnvKey.envVar }} 环境变量</span>
            <span v-else>手动填写优先级高于环境变量</span>
          </div>
        </el-form-item>

        <!-- 获取模型 -->
        <el-form-item label=" " label-width="90px">
          <div style="display: flex; gap: 8px; width: 100%; align-items: center">
            <el-button @click="probeModels" :loading="probing" type="primary" plain style="flex-shrink: 0">
              <el-icon style="vertical-align:-2px;margin-right:4px"><Search /></el-icon>获取可用模型
            </el-button>
            <span v-if="probeError" style="font-size: 12px; color: var(--el-color-danger)">{{ probeError }}</span>
            <span v-else-if="probedModels.length" style="font-size: 12px; color: var(--el-color-success)">获取到 {{ probedModels.length }} 个模型</span>
            <span v-else style="font-size: 12px; color: #909399">填写 Key 后点击获取，或直接手动填写模型 ID</span>
          </div>
        </el-form-item>

        <!-- 模型选择 -->
        <el-form-item label="模型 ID" required>
          <el-select v-if="probedModels.length" v-model="form.model" filterable placeholder="搜索或选择模型" style="width: 100%" @change="onModelSelect">
            <el-option v-for="m in probedModels" :key="m.id" :label="m.name !== m.id ? `${m.name}  (${m.id})` : m.id" :value="m.id" />
          </el-select>
          <el-input v-else v-model="form.model" :placeholder="currentMeta?.modelHint || '如 claude-sonnet-4-6'" @input="autoFillName" />
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
import { Refresh, Plus, Key, Connection, CircleCheck, Search } from '@element-plus/icons-vue'
import { models as modelsApi, type ModelEntry, type ProbeModelInfo } from '../api'

// ── 提供商元数据 ──────────────────────────────────────────────────────────────
interface ProviderMeta {
  key: string
  label: string
  icon: string
  baseUrl: string
  website: string
  apiKeyUrl: string
  apiKeyHint: string
  keyFormat?: string
  modelHint?: string
  compatible?: boolean  // OpenAI-compatible
}

const providerMetaList: ProviderMeta[] = [
  {
    key: 'anthropic',
    label: 'Anthropic',
    icon: '🔮',
    baseUrl: 'https://api.anthropic.com/v1',
    website: 'https://anthropic.com',
    apiKeyUrl: 'https://console.anthropic.com/settings/keys',
    apiKeyHint: '在 Anthropic Console 创建 API Key',
    keyFormat: 'sk-ant-api03-...',
    modelHint: '如 claude-sonnet-4-6',
    compatible: false,
  },
  {
    key: 'openai',
    label: 'OpenAI',
    icon: '🤖',
    baseUrl: 'https://api.openai.com/v1',
    website: 'https://openai.com',
    apiKeyUrl: 'https://platform.openai.com/api-keys',
    apiKeyHint: '在 OpenAI Platform 创建 API Key',
    keyFormat: 'sk-proj-...',
    modelHint: '如 gpt-4o、o1-mini',
    compatible: true,
  },
  {
    key: 'deepseek',
    label: 'DeepSeek',
    icon: '🌊',
    baseUrl: 'https://api.deepseek.com/v1',
    website: 'https://deepseek.com',
    apiKeyUrl: 'https://platform.deepseek.com/api_keys',
    apiKeyHint: '在 DeepSeek Platform 创建 API Key',
    keyFormat: 'sk-...',
    modelHint: '如 deepseek-chat、deepseek-reasoner',
    compatible: true,
  },
  {
    key: 'moonshot',
    label: 'Kimi',
    icon: '🌙',
    baseUrl: 'https://api.moonshot.cn/v1',
    website: 'https://kimi.moonshot.cn',
    apiKeyUrl: 'https://platform.moonshot.cn/console/api-keys',
    apiKeyHint: '在月之暗面开放平台创建 API Key',
    keyFormat: 'sk-...',
    modelHint: '如 moonshot-v1-8k、moonshot-v1-32k',
    compatible: true,
  },
  {
    key: 'zhipu',
    label: '智谱 GLM',
    icon: '🧠',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    website: 'https://open.bigmodel.cn',
    apiKeyUrl: 'https://open.bigmodel.cn/usercenter/apikeys',
    apiKeyHint: '在智谱 AI 开放平台创建 API Key',
    keyFormat: '随机字符串',
    modelHint: '如 glm-4、glm-4-flash',
    compatible: true,
  },
  {
    key: 'minimax',
    label: 'MiniMax',
    icon: '✨',
    baseUrl: 'https://api.minimax.chat/v1',
    website: 'https://minimax.io',
    apiKeyUrl: 'https://platform.minimax.io/user-center/basic-information/interface-key',
    apiKeyHint: '在 MiniMax 开放平台创建 API Key',
    keyFormat: 'eyJ...',
    modelHint: '如 abab6.5s-chat、MiniMax-Text-01',
    compatible: true,
  },
  {
    key: 'qwen',
    label: '通义千问',
    icon: '☁️',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    website: 'https://tongyi.aliyun.com',
    apiKeyUrl: 'https://dashscope.console.aliyun.com/apiKey',
    apiKeyHint: '在阿里云 DashScope 控制台获取 API Key',
    keyFormat: 'sk-...',
    modelHint: '如 qwen-turbo、qwen-plus、qwen-max',
    compatible: true,
  },
  {
    key: 'openrouter',
    label: 'OpenRouter',
    icon: '🔀',
    baseUrl: 'https://openrouter.ai/api/v1',
    website: 'https://openrouter.ai',
    apiKeyUrl: 'https://openrouter.ai/keys',
    apiKeyHint: '在 OpenRouter 创建 API Key，可访问数百个模型',
    keyFormat: 'sk-or-v1-...',
    modelHint: '点击「获取可用模型」列出所有可用模型',
    compatible: true,
  },
  {
    key: 'custom',
    label: '自定义',
    icon: '⚙️',
    baseUrl: '',
    website: '',
    apiKeyUrl: '',
    apiKeyHint: '填写任意 OpenAI-compatible 接口地址和对应的 API Key',
    modelHint: '手动填写模型 ID',
    compatible: true,
  },
]

// key → meta map
const providerMeta: Record<string, ProviderMeta> = Object.fromEntries(
  providerMetaList.map(p => [p.key, p])
)
const providers = providerMetaList

// ── State ─────────────────────────────────────────────────────────────────────
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

const form = reactive({
  id: '',
  name: '',
  provider: 'anthropic',
  model: '',
  apiKey: '',
  baseUrl: 'https://api.anthropic.com',
  isDefault: false,
})

// ── Computed ──────────────────────────────────────────────────────────────────
const currentMeta = computed<ProviderMeta | null>(() => providerMeta[form.provider] || null)

const currentEnvKey = computed<EnvKey | null>(() =>
  envKeys.value.find(ek => ek.provider === form.provider) || null
)

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadList(), loadEnvKeys()])
})

// ── Helpers ───────────────────────────────────────────────────────────────────
function defaultBaseUrl(provider: string) {
  return providerMeta[provider]?.baseUrl || '—'
}

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

function setProvider(key: string) {
  form.provider = key
  form.baseUrl = defaultBaseUrl(key)
  form.model = ''
  probedModels.value = []
  probeError.value = ''
  if (key === 'openrouter') probeModels()
}

function onModelSelect(modelId: string) {
  const found = probedModels.value.find(m => m.id === modelId)
  if (found) form.name = (found.name && found.name !== found.id) ? found.name : modelId
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
  if (!form.baseUrl) { probeError.value = '请先填写调用地址'; return }
  probing.value = true
  probeError.value = ''
  probedModels.value = []
  try {
    const res = await modelsApi.probe(form.baseUrl, form.apiKey || undefined, form.provider)
    probedModels.value = res.data.models || []
    if (!probedModels.value.length) probeError.value = '未获取到模型列表（接口返回为空）'
  } catch (e: any) {
    probeError.value = e.response?.data?.error || e.message || '获取失败'
  } finally {
    probing.value = false
  }
}

async function quickAddFromEnv(ek: EnvKey) {
  quickAdding.value = ek.envVar
  try {
    const existing = list.value.find(m => m.provider === ek.provider)
    if (existing) { ElMessage.warning(`${ek.provider} 已有配置，请直接编辑`); return }
    editingId.value = ''
    probedModels.value = []
    probeError.value = ''
    Object.assign(form, {
      id: ek.provider + '-default',
      name: (providerMeta[ek.provider]?.label || ek.provider) + ' (env)',
      provider: ek.provider,
      model: '',
      apiKey: '',
      baseUrl: ek.baseUrl || defaultBaseUrl(ek.provider),
      isDefault: list.value.length === 0,
    })
    dialogVisible.value = true
    if (ek.provider === 'openrouter') probeModels()
  } finally {
    quickAdding.value = ''
  }
}

function openAdd() {
  editingId.value = ''
  probedModels.value = []
  probeError.value = ''
  Object.assign(form, { id: '', name: '', provider: 'anthropic', model: '', apiKey: '', baseUrl: defaultBaseUrl('anthropic'), isDefault: false })
  dialogVisible.value = true
}

function openEdit(row: ModelEntry) {
  editingId.value = row.id
  probedModels.value = []
  probeError.value = ''
  Object.assign(form, {
    id: row.id, name: row.name, provider: row.provider, model: row.model,
    apiKey: row.apiKey, baseUrl: row.baseUrl || defaultBaseUrl(row.provider), isDefault: row.isDefault,
  })
  dialogVisible.value = true
}

async function saveModel() {
  if (!form.id || !form.provider || !form.model) {
    ElMessage.warning('请填写必要字段（唯一ID / 提供商 / 模型 ID）'); return
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
    if (res.data.valid) ElMessage.success('连接成功！')
    else ElMessage.error('连接失败: ' + (res.data.error || ''))
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

/* ── 提供商网格 ── */
.provider-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  width: 100%;
}
.provider-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 4px;
  border: 1.5px solid #e4e7ed;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
  transition: border-color .15s, background .15s, box-shadow .15s;
  font-size: 12px;
  color: #606266;
  line-height: 1.3;
}
.provider-card:hover {
  border-color: #409eff;
  background: #ecf5ff;
  color: #409eff;
}
.provider-card.active {
  border-color: #409eff;
  background: #ecf5ff;
  color: #409eff;
  font-weight: 600;
  box-shadow: 0 0 0 2px rgba(64,158,255,.15);
}
.provider-icon { font-size: 20px; line-height: 1; }
.provider-label { white-space: nowrap; }

/* ── 引导信息 ── */
.provider-guide {
  width: 100%;
  background: #f8f9fa;
  border: 1px solid #e9ecef;
  border-radius: 8px;
  padding: 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.guide-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
}
.guide-icon { flex-shrink: 0; font-size: 14px; }
.guide-text { flex: 1; }
.guide-link {
  flex-shrink: 0;
  color: #409eff;
  text-decoration: none;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.guide-link:hover { text-decoration: underline; }
.guide-row code {
  background: #e9ecef;
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 12px;
  color: #495057;
}

.field-hint {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 4px;
  line-height: 1.4;
}
</style>
