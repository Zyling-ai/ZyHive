<template>
  <div class="models-page">
    <el-tabs v-model="activeTab" class="main-tabs">

      <!-- ═══════════════════════════════════════════════════
           Tab 1: API Key 管理
      ════════════════════════════════════════════════════ -->
      <el-tab-pane label="API Key 管理" name="providers">
        <div class="two-col-layout">

          <!-- 左侧：已配置列表 -->
          <div class="col-list">
            <div class="col-list-header">
              <span class="col-list-title">已配置厂商</span>
              <el-button size="small" type="primary" @click="openAddProvider">
                <el-icon><Plus /></el-icon> 添加
              </el-button>
            </div>
            <div v-if="providerList.length === 0" class="list-empty">
              暂未配置，点击右侧添加
            </div>
            <div
              v-for="p in providerList"
              :key="p.id"
              class="provider-item"
              :class="{ active: selectedProvider?.id === p.id }"
              @click="selectProvider(p)"
            >
              <img :src="getProviderLogo(p.provider)" class="pitem-logo" />
              <div class="pitem-info">
                <div class="pitem-name">{{ p.name }}</div>
                <div class="pitem-sub">{{ p.apiKey }}</div>
              </div>
              <el-tag
                :type="p.status === 'ok' ? 'success' : p.status === 'error' ? 'danger' : 'info'"
                size="small"
                class="pitem-status"
              >
                {{ p.status === 'ok' ? '✓' : p.status === 'error' ? '✗' : '?' }}
              </el-tag>
            </div>
          </div>

          <!-- 右侧：添加 / 编辑表单 -->
          <div class="col-form">
            <template v-if="providerForm.mode === 'add' || providerForm.mode === 'edit'">
              <div class="form-title">{{ providerForm.mode === 'add' ? '添加 API Key' : '编辑 ' + selectedProvider?.name }}</div>

              <!-- 提供商选择网格 -->
              <div class="field-label">选择提供商 <span class="required">*</span></div>
              <div class="provider-grid">
                <button
                  v-for="p in providerMetaList"
                  :key="p.key"
                  type="button"
                  class="provider-card"
                  :class="{ active: providerForm.provider === p.key }"
                  @click="selectProviderType(p.key)"
                  :disabled="providerForm.mode === 'edit'"
                >
                  <img :src="p.logo" :alt="p.label" class="provider-logo" />
                  <span class="provider-label">{{ p.label }}</span>
                </button>
              </div>

              <!-- 引导信息 -->
              <div v-if="currentProviderMeta" class="provider-guide">
                <div class="guide-row">
                  <span>🔑</span>
                  <span>{{ currentProviderMeta.apiKeyHint }}</span>
                  <a v-if="currentProviderMeta.apiKeyUrl" :href="currentProviderMeta.apiKeyUrl" target="_blank" class="guide-link">获取 API Key →</a>
                </div>
                <div v-if="currentProviderMeta.keyFormat" class="guide-row">
                  <span>📋</span>
                  <span>格式：<code>{{ currentProviderMeta.keyFormat }}</code></span>
                </div>
              </div>

              <!-- 名称 -->
              <div class="field-label">名称</div>
              <el-input v-model="providerForm.name" :placeholder="currentProviderMeta?.label || '如 我的 DeepSeek'" />

              <!-- API Key -->
              <div class="field-label">API Key <span class="required">*</span></div>
              <el-input
                v-model="providerForm.apiKey"
                type="password"
                show-password
                :placeholder="currentProviderMeta?.keyFormat || 'sk-...'"
              />

              <!-- 转发地址（折叠） -->
              <div class="relay-toggle" @click="providerForm.showRelay = !providerForm.showRelay">
                <el-switch :model-value="providerForm.showRelay" size="small" style="pointer-events:none" />
                <span class="relay-toggle-label">使用转发地址 <span class="hint">（国内绕过限制）</span></span>
              </div>
              <template v-if="providerForm.showRelay">
                <el-input v-model="providerForm.baseUrl" placeholder="填写中转地址，如 https://your-relay.com" clearable style="margin-top:6px" />
              </template>

              <!-- 操作按钮 -->
              <div class="form-actions">
                <el-button @click="cancelProviderForm">取消</el-button>
                <el-button type="primary" @click="saveProvider" :loading="providerSaving">保存</el-button>
                <el-button type="success" @click="testProvider" :loading="providerTesting" :disabled="!selectedProvider && providerForm.mode !== 'add'">
                  <el-icon><CircleCheck /></el-icon> 测试连通
                </el-button>
              </div>

              <!-- 测试结果 -->
              <el-alert
                v-if="providerTestResult"
                :type="providerTestResult.ok ? 'success' : 'error'"
                :title="providerTestResult.msg"
                :closable="false"
                style="margin-top: 12px"
              />
            </template>

            <!-- 已选中 provider 详情 -->
            <template v-else-if="selectedProvider">
              <div class="form-title">{{ selectedProvider.name }}</div>
              <div class="detail-row"><span class="detail-label">提供商</span><span>{{ getProviderLabel(selectedProvider.provider) }}</span></div>
              <div class="detail-row"><span class="detail-label">API Key</span><code>{{ selectedProvider.apiKey }}</code></div>
              <div class="detail-row" v-if="selectedProvider.baseUrl"><span class="detail-label">转发地址</span><span>{{ selectedProvider.baseUrl }}</span></div>
              <div class="detail-row"><span class="detail-label">引用模型数</span><span>{{ selectedProvider.modelCount }} 个</span></div>
              <div class="detail-row"><span class="detail-label">状态</span>
                <el-tag :type="selectedProvider.status === 'ok' ? 'success' : selectedProvider.status === 'error' ? 'danger' : 'info'" size="small">
                  {{ selectedProvider.status === 'ok' ? '✓ 有效' : selectedProvider.status === 'error' ? '✗ 无效' : '未测试' }}
                </el-tag>
              </div>
              <div class="form-actions">
                <el-button @click="openEditProvider(selectedProvider)">编辑</el-button>
                <el-button type="success" @click="testProviderById(selectedProvider.id)" :loading="providerTesting">
                  <el-icon><CircleCheck /></el-icon> 测试连通
                </el-button>
                <el-button type="danger" plain @click="deleteProvider(selectedProvider)">删除</el-button>
              </div>
              <el-alert
                v-if="providerTestResult"
                :type="providerTestResult.ok ? 'success' : 'error'"
                :title="providerTestResult.msg"
                :closable="false"
                style="margin-top: 12px"
              />
            </template>

            <!-- 空状态 -->
            <template v-else>
              <div class="form-empty">
                <el-icon style="font-size: 48px; color: #dcdfe6"><Key /></el-icon>
                <div style="margin-top: 12px; color: #909399">从左侧选择一个厂商查看详情，或点击「添加」配置新的 API Key</div>
                <el-button type="primary" style="margin-top: 16px" @click="openAddProvider">
                  <el-icon><Plus /></el-icon> 添加第一个 API Key
                </el-button>
              </div>
            </template>
          </div>
        </div>
      </el-tab-pane>

      <!-- ═══════════════════════════════════════════════════
           Tab 2: 模型列表
      ════════════════════════════════════════════════════ -->
      <el-tab-pane name="models">
        <template #label>
          模型列表
          <el-badge v-if="list.length" :value="list.length" :max="99" style="margin-left:4px" />
        </template>

        <!-- 无 provider 时提示 -->
        <el-alert
          v-if="providerList.length === 0"
          type="warning"
          :closable="false"
          style="margin-bottom: 16px"
        >
          <template #title>
            请先在「API Key 管理」中添加至少一个厂商的 API Key，再来添加模型。
            <el-button size="small" style="margin-left: 8px" @click="activeTab = 'providers'">去添加 →</el-button>
          </template>
        </el-alert>

        <div style="display: flex; justify-content: flex-end; margin-bottom: 12px">
          <el-button type="primary" @click="openAddModel" :disabled="providerList.length === 0">
            <el-icon><Plus /></el-icon> 添加模型
          </el-button>
        </div>

        <el-card shadow="never">
          <el-table :data="list" stripe>
            <el-table-column label="提供商" width="100">
              <template #default="{ row }">
                <div style="display:flex;align-items:center;gap:6px">
                  <img :src="getProviderLogo(row.provider)" style="width:18px;height:18px;object-fit:contain;border-radius:3px" />
                  <span style="font-size:12px">{{ getProviderLabel(row.provider) }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="名称" min-width="130" />
            <el-table-column label="模型 ID" min-width="190">
              <template #default="{ row }"><el-text type="info" size="small">{{ row.model }}</el-text></template>
            </el-table-column>
            <el-table-column label="API Key" width="160">
              <template #default="{ row }">
                <span v-if="row.providerId" style="font-size:12px;color:#67c23a">
                  ✓ {{ getProviderName(row.providerId) }}
                </span>
                <code v-else-if="row.apiKey" style="font-size: 12px; color: #909399">{{ row.apiKey }}</code>
                <el-tag v-else type="info" size="small" style="font-size:11px">环境变量</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="140">
              <template #default="{ row }">
                <div style="display:flex;gap:4px;flex-wrap:wrap;align-items:center">
                  <el-tag :type="row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'" size="small">
                    {{ row.status === 'ok' ? '✓ 有效' : row.status === 'error' ? '✗ 无效' : '? 未测' }}
                  </el-tag>
                  <el-tooltip v-if="row.supportsTools === false" content="该模型不支持工具调用" placement="top">
                    <el-tag type="warning" size="small" style="cursor:help">⚠ 无工具</el-tag>
                  </el-tooltip>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="默认" width="60">
              <template #default="{ row }"><el-tag v-if="row.isDefault" type="warning" size="small">默认</el-tag></template>
            </el-table-column>
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button size="small" @click="testModel(row)" :loading="testing === row.id">测试</el-button>
                <el-button size="small" @click="openEditModel(row)">编辑</el-button>
                <el-button size="small" type="danger" @click="deleteModel(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- ── 添加 / 编辑模型 Dialog ── -->
    <el-dialog v-model="modelDialogVisible" :title="editingModelId ? '编辑模型' : '添加模型'" width="560px" align-center>
      <el-form :model="modelForm" label-width="90px" style="padding-right: 8px">

        <!-- 选择 API Key（厂商）-->
        <el-form-item label="API Key" required>
          <el-select v-model="modelForm.providerId" placeholder="选择已配置的 API Key" style="width:100%" @change="onProviderChange">
            <el-option
              v-for="p in providerList"
              :key="p.id"
              :label="p.name + ' · ' + getProviderLabel(p.provider)"
              :value="p.id"
            >
              <div style="display:flex;align-items:center;gap:8px">
                <img :src="getProviderLogo(p.provider)" style="width:16px;height:16px;object-fit:contain" />
                <span>{{ p.name }}</span>
                <el-tag :type="p.status === 'ok' ? 'success' : p.status === 'error' ? 'danger' : 'info'" size="small">
                  {{ p.status === 'ok' ? '✓' : p.status === 'error' ? '✗' : '?' }}
                </el-tag>
              </div>
            </el-option>
          </el-select>
          <div class="field-hint">
            没有想要的厂商？
            <el-button link type="primary" @click="modelDialogVisible=false; activeTab='providers'; openAddProvider()">去添加 API Key →</el-button>
          </div>
        </el-form-item>

        <!-- 调用地址（可覆盖） -->
        <el-form-item label="调用地址">
          <el-input v-model="modelForm.baseUrl" placeholder="留空使用厂商默认地址" clearable />
          <div class="field-hint">仅需覆盖时填写（如使用中转地址）</div>
        </el-form-item>

        <!-- 获取模型 -->
        <el-form-item label=" ">
          <div style="display:flex;gap:8px;align-items:center;width:100%">
            <el-button @click="probeModels" :loading="probing" type="primary" plain style="flex-shrink:0">
              <el-icon><Search /></el-icon> 获取可用模型
            </el-button>
            <span v-if="probeError" style="font-size:12px;color:var(--el-color-danger)">{{ probeError }}</span>
            <span v-else-if="probedModels.length" style="font-size:12px;color:var(--el-color-success)">{{ probedModels.length }} 个模型</span>
            <span v-else style="font-size:12px;color:#909399">或直接手动填写模型 ID</span>
          </div>
        </el-form-item>

        <!-- 模型选择 / 输入 -->
        <el-form-item label="模型 ID" required>
          <el-select v-if="probedModels.length" v-model="modelForm.model" filterable placeholder="搜索或选择模型" style="width:100%" @change="onModelSelect">
            <el-option v-for="m in probedModels" :key="m.id" :label="m.name !== m.id ? `${m.name}  (${m.id})` : m.id" :value="m.id" />
          </el-select>
          <el-input v-else v-model="modelForm.model" placeholder="如 claude-sonnet-4-6 / deepseek-chat" @input="autoFillName" />
        </el-form-item>

        <!-- 显示名称 -->
        <el-form-item label="显示名称">
          <el-input v-model="modelForm.name" placeholder="如 Claude Sonnet 4.6" />
        </el-form-item>

        <!-- 唯一 ID -->
        <el-form-item label="唯一 ID">
          <el-input v-model="modelForm.id" placeholder="如 claude-sonnet（Agent 引用时使用）" />
        </el-form-item>

        <!-- 设为默认 -->
        <el-form-item label="设为默认">
          <el-switch v-model="modelForm.isDefault" />
        </el-form-item>

        <!-- 工具调用 -->
        <el-form-item label="工具调用">
          <el-select v-model="modelForm.supportsTools" style="width:180px">
            <el-option :value="null" label="🔍 自动判断（推荐）" />
            <el-option :value="true" label="✅ 支持工具调用" />
            <el-option :value="false" label="⚠️ 不支持（禁用工具）" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveModel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Key, CircleCheck, Search } from '@element-plus/icons-vue'
import { models as modelsApi, providers as providersApi, type ModelEntry, type ProviderEntry, type ProbeModelInfo } from '../api'

// ── Provider logo imports ─────────────────────────────────────────────────────
import iconAnthropic  from '../assets/providers/anthropic.svg'
import iconOpenAI     from '../assets/providers/openai.png'
import iconDeepSeek   from '../assets/providers/deepseek.png'
import iconKimi       from '../assets/providers/kimi.png'
import iconZhipu      from '../assets/providers/zhipu.png'
import iconMiniMax    from '../assets/providers/minimax.png'
import iconQwen       from '../assets/providers/qwen.png'
import iconOpenRouter from '../assets/providers/openrouter.svg'
import iconCustom     from '../assets/providers/custom.svg'

// ── Provider 元数据 ───────────────────────────────────────────────────────────
interface ProviderMeta {
  key: string; label: string; logo: string; baseUrl: string
  apiKeyUrl: string; apiKeyHint: string; keyFormat?: string; modelHint?: string
}
const providerMetaList: ProviderMeta[] = [
  { key: 'anthropic',  label: 'Anthropic',    logo: iconAnthropic,  baseUrl: 'https://api.anthropic.com',
    apiKeyUrl: 'https://console.anthropic.com/settings/keys',  apiKeyHint: '在 Anthropic Console 创建 API Key', keyFormat: 'sk-ant-api03-...', modelHint: 'claude-sonnet-4-6' },
  { key: 'openai',     label: 'OpenAI',        logo: iconOpenAI,     baseUrl: 'https://api.openai.com/v1',
    apiKeyUrl: 'https://platform.openai.com/api-keys',          apiKeyHint: '在 OpenAI Platform 创建 API Key', keyFormat: 'sk-proj-...', modelHint: 'gpt-4o' },
  { key: 'deepseek',   label: 'DeepSeek',      logo: iconDeepSeek,   baseUrl: 'https://api.deepseek.com/v1',
    apiKeyUrl: 'https://platform.deepseek.com/api_keys',        apiKeyHint: '在 DeepSeek Platform 创建 API Key', keyFormat: 'sk-...', modelHint: 'deepseek-chat' },
  { key: 'kimi',       label: 'Kimi',          logo: iconKimi,       baseUrl: 'https://api.moonshot.cn/v1',
    apiKeyUrl: 'https://platform.moonshot.cn/console/api-keys', apiKeyHint: '在月之暗面开放平台创建 API Key', keyFormat: 'sk-...', modelHint: 'moonshot-v1-8k' },
  { key: 'zhipu',      label: '智谱 GLM',      logo: iconZhipu,      baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    apiKeyUrl: 'https://open.bigmodel.cn/usercenter/apikeys',   apiKeyHint: '在智谱 AI 开放平台获取 API Key', keyFormat: '随机字符串', modelHint: 'glm-4' },
  { key: 'minimax',    label: 'MiniMax',       logo: iconMiniMax,    baseUrl: 'https://api.minimax.chat/v1',
    apiKeyUrl: 'https://platform.minimax.io/user-center/basic-information/interface-key', apiKeyHint: '在 MiniMax 平台获取 API Key', keyFormat: 'eyJ...', modelHint: 'abab6.5s-chat' },
  { key: 'qwen',       label: '通义千问',      logo: iconQwen,       baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    apiKeyUrl: 'https://dashscope.console.aliyun.com/apiKey',   apiKeyHint: '在阿里云 DashScope 控制台获取', keyFormat: 'sk-...', modelHint: 'qwen-max' },
  { key: 'openrouter', label: 'OpenRouter',    logo: iconOpenRouter, baseUrl: 'https://openrouter.ai/api/v1',
    apiKeyUrl: 'https://openrouter.ai/keys',                    apiKeyHint: '在 OpenRouter 创建 API Key，可访问数百个模型', keyFormat: 'sk-or-v1-...', modelHint: '点击「获取可用模型」' },
  { key: 'custom',     label: '自定义',        logo: iconCustom,     baseUrl: '',
    apiKeyUrl: '',                                               apiKeyHint: '填写任意 OpenAI-compatible 接口地址和 API Key', modelHint: '手动填写模型 ID' },
]
const providerMetaMap = Object.fromEntries(providerMetaList.map(p => [p.key, p]))

function getProviderLogo(key: string)  { return providerMetaMap[key]?.logo  || iconCustom }
function getProviderLabel(key: string) { return providerMetaMap[key]?.label || key }
function getProviderName(pid: string)  { return providerList.value.find(p => p.id === pid)?.name || pid }

// ── State ─────────────────────────────────────────────────────────────────────
const activeTab = ref('providers')

// Providers
const providerList = ref<ProviderEntry[]>([])
const selectedProvider = ref<ProviderEntry | null>(null)
const providerSaving   = ref(false)
const providerTesting  = ref(false)
const providerTestResult = ref<{ ok: boolean; msg: string } | null>(null)
const providerForm = reactive({
  mode: 'idle' as 'idle' | 'add' | 'edit',
  provider: 'anthropic',
  name: '',
  apiKey: '',
  baseUrl: '',
  showRelay: false,
})

// Models
const list = ref<ModelEntry[]>([])
const modelDialogVisible = ref(false)
const editingModelId     = ref('')
const saving             = ref(false)
const testing            = ref('')
const probing            = ref(false)
const probeError         = ref('')
const probedModels       = ref<ProbeModelInfo[]>([])
const modelForm = reactive({
  id: '', name: '', provider: '', model: '',
  providerId: '', baseUrl: '', isDefault: false,
  supportsTools: null as boolean | null,
})

// ── Computed ──────────────────────────────────────────────────────────────────
const currentProviderMeta = computed<ProviderMeta | null>(() =>
  providerMetaMap[providerForm.provider] || null
)

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(() => { loadProviders(); loadModels() })

// ── Provider 操作 ─────────────────────────────────────────────────────────────
async function loadProviders() {
  try {
    const res = await providersApi.list()
    providerList.value = res.data.providers || []
  } catch {}
}

function openAddProvider() {
  selectedProvider.value = null
  providerTestResult.value = null
  Object.assign(providerForm, { mode: 'add', provider: 'anthropic', name: '', apiKey: '', baseUrl: '', showRelay: false })
}

function openEditProvider(p: ProviderEntry) {
  selectedProvider.value = p
  providerTestResult.value = null
  Object.assign(providerForm, { mode: 'edit', provider: p.provider, name: p.name, apiKey: '', baseUrl: p.baseUrl || '', showRelay: !!p.baseUrl })
}

function selectProvider(p: ProviderEntry) {
  selectedProvider.value = p
  providerForm.mode = 'idle'
  providerTestResult.value = null
}

function selectProviderType(key: string) {
  if (providerForm.mode === 'edit') return
  providerForm.provider = key
  if (!providerForm.name) providerForm.name = providerMetaMap[key]?.label || key
}

function cancelProviderForm() {
  providerForm.mode = 'idle'
  providerTestResult.value = null
}

async function saveProvider() {
  if (!providerForm.provider) { ElMessage.warning('请选择提供商'); return }
  if (!providerForm.apiKey && providerForm.mode === 'add') { ElMessage.warning('请填写 API Key'); return }
  providerSaving.value = true
  try {
    const payload = {
      provider: providerForm.provider,
      name: providerForm.name || providerMetaMap[providerForm.provider]?.label || providerForm.provider,
      apiKey: providerForm.apiKey,
      baseUrl: providerForm.baseUrl,
    }
    if (providerForm.mode === 'edit' && selectedProvider.value) {
      const res = await providersApi.update(selectedProvider.value.id, payload)
      selectedProvider.value = res.data.provider
      ElMessage.success('已更新')
    } else {
      const res = await providersApi.create(payload)
      selectedProvider.value = res.data.provider
      ElMessage.success('已添加')
    }
    providerForm.mode = 'idle'
    await loadProviders()
    // 自动跳到测试
    if (selectedProvider.value) testProviderById(selectedProvider.value.id)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    providerSaving.value = false
  }
}

async function testProvider() {
  // 保存后自动测试，或者在 add 模式下先保存再测
  if (providerForm.mode === 'add' || providerForm.mode === 'edit') {
    await saveProvider()
    return
  }
  if (selectedProvider.value) testProviderById(selectedProvider.value.id)
}

async function testProviderById(id: string) {
  providerTesting.value = true
  providerTestResult.value = null
  try {
    const res = await providersApi.test(id)
    providerTestResult.value = { ok: res.data.status === 'ok', msg: res.data.message }
    await loadProviders()
    // 同步更新 selectedProvider 状态
    const updated = providerList.value.find(p => p.id === id)
    if (updated) selectedProvider.value = updated
  } catch (e: any) {
    providerTestResult.value = { ok: false, msg: e.response?.data?.error || '测试失败' }
  } finally {
    providerTesting.value = false
  }
}

async function deleteProvider(p: ProviderEntry) {
  if (p.modelCount > 0) {
    ElMessage.warning(`该 API Key 被 ${p.modelCount} 个模型使用，请先删除或修改这些模型`)
    return
  }
  try {
    await ElMessageBox.confirm(`确定删除 "${p.name}" 的 API Key？`, '确认删除', { type: 'warning' })
    await providersApi.delete(p.id)
    selectedProvider.value = null
    providerTestResult.value = null
    await loadProviders()
    ElMessage.success('已删除')
  } catch {}
}

// ── Model 操作 ────────────────────────────────────────────────────────────────
async function loadModels() {
  try {
    const res = await modelsApi.list()
    list.value = res.data
  } catch {}
}

function openAddModel() {
  editingModelId.value = ''
  probedModels.value = []; probeError.value = ''
  const firstProvider = providerList.value[0]
  Object.assign(modelForm, {
    id: '', name: '', model: '', baseUrl: '', isDefault: list.value.length === 0,
    supportsTools: null,
    providerId: firstProvider?.id || '',
    provider: firstProvider?.provider || 'anthropic',
  })
  modelDialogVisible.value = true
}

function openEditModel(row: ModelEntry) {
  editingModelId.value = row.id
  probedModels.value = []; probeError.value = ''
  Object.assign(modelForm, {
    id: row.id, name: row.name, model: row.model,
    providerId: row.providerId || '',
    provider: row.provider,
    baseUrl: row.baseUrl || '',
    isDefault: row.isDefault,
    supportsTools: row.supportsTools ?? null,
  })
  modelDialogVisible.value = true
}

function onProviderChange(pid: string) {
  const p = providerList.value.find(pp => pp.id === pid)
  if (p) modelForm.provider = p.provider
  probedModels.value = []; probeError.value = ''
}

function onModelSelect(modelId: string) {
  const found = probedModels.value.find(m => m.id === modelId)
  if (found) modelForm.name = (found.name && found.name !== found.id) ? found.name : modelId
  if (!modelForm.id) {
    modelForm.id = modelId.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
  }
}

function autoFillName() {
  if (!modelForm.name) modelForm.name = modelForm.model
  if (!modelForm.id) {
    modelForm.id = modelForm.model.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
  }
}

async function probeModels() {
  const p = providerList.value.find(pp => pp.id === modelForm.providerId)
  if (!p) { probeError.value = '请先选择 API Key 厂商'; return }
  probing.value = true; probeError.value = ''; probedModels.value = []
  try {
    const baseUrl = modelForm.baseUrl || p.baseUrl || providerMetaMap[p.provider]?.baseUrl || ''
    const res = await modelsApi.probe(baseUrl, undefined, p.provider)
    probedModels.value = res.data.models || []
    if (!probedModels.value.length) probeError.value = '未获取到模型列表'
  } catch (e: any) {
    probeError.value = e.response?.data?.error || e.message || '获取失败'
  } finally {
    probing.value = false
  }
}

async function saveModel() {
  if (!modelForm.id || !modelForm.model) {
    ElMessage.warning('请填写唯一 ID 和模型 ID'); return
  }
  if (!modelForm.providerId) {
    ElMessage.warning('请选择 API Key 厂商'); return
  }
  saving.value = true
  try {
    const payload = {
      id: modelForm.id, name: modelForm.name || modelForm.model,
      provider: modelForm.provider, model: modelForm.model,
      providerId: modelForm.providerId,
      baseUrl: modelForm.baseUrl,
      isDefault: modelForm.isDefault,
      supportsTools: modelForm.supportsTools,
      status: 'untested',
    }
    if (editingModelId.value) {
      await modelsApi.update(editingModelId.value, payload as any)
    } else {
      await modelsApi.create(payload as any)
    }
    ElMessage.success('保存成功')
    modelDialogVisible.value = false
    loadModels()
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
    ElMessage[res.data.valid ? 'success' : 'error'](res.data.valid ? '连接成功！' : '连接失败: ' + (res.data.error || ''))
    await loadModels()
  } catch { ElMessage.error('测试请求失败') }
  finally { testing.value = '' }
}

async function deleteModel(row: ModelEntry) {
  try {
    await ElMessageBox.confirm(`确定删除模型 "${row.name}"？`, '确认删除', { type: 'warning' })
    await modelsApi.delete(row.id)
    ElMessage.success('已删除')
    loadModels()
  } catch {}
}
</script>

<style scoped>
.models-page { padding: 0; }

/* ── 主 Tabs ── */
.main-tabs :deep(.el-tabs__header) { margin-bottom: 16px; }

/* ── 两栏布局 ── */
.two-col-layout {
  display: flex;
  gap: 0;
  min-height: 500px;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  overflow: hidden;
  background: var(--el-bg-color);
}

/* 左列 */
.col-list {
  width: 240px;
  flex-shrink: 0;
  border-right: 1px solid var(--el-border-color);
  display: flex;
  flex-direction: column;
}
.col-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--el-border-color);
}
.col-list-title { font-weight: 600; font-size: 14px; }
.list-empty { padding: 24px 16px; font-size: 13px; color: #909399; text-align: center; }

.provider-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  transition: background .15s;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.provider-item:hover { background: var(--el-fill-color-light); }
.provider-item.active { background: var(--el-color-primary-light-9); }

.pitem-logo { width: 28px; height: 28px; object-fit: contain; border-radius: 6px; flex-shrink: 0; }
.pitem-info { flex: 1; min-width: 0; }
.pitem-name { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pitem-sub  { font-size: 11px; color: #909399; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pitem-status { flex-shrink: 0; }

/* 右列 */
.col-form {
  flex: 1;
  padding: 24px 28px;
  overflow-y: auto;
}
.form-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 20px;
  color: var(--el-text-color-primary);
}
.form-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 320px;
  color: #909399;
}
.detail-row {
  display: flex;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 14px;
}
.detail-label { width: 90px; flex-shrink: 0; color: #909399; }
.form-actions { display: flex; gap: 8px; margin-top: 20px; }

/* 字段标签 */
.field-label { font-size: 13px; color: #606266; margin: 14px 0 6px; font-weight: 500; }
.required { color: var(--el-color-danger); }
.hint { font-weight: 400; color: #909399; font-size: 12px; }

/* ── Provider 网格 ── */
.provider-grid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 8px;
  width: 100%;
  margin-bottom: 12px;
}
.provider-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 4px;
  border: 1.5px solid var(--el-border-color);
  border-radius: 8px;
  background: var(--el-bg-color);
  cursor: pointer;
  transition: border-color .15s, background .15s;
  font-size: 12px;
  color: var(--el-text-color-regular);
}
.provider-card:hover { border-color: var(--el-color-primary); background: var(--el-color-primary-light-9); }
.provider-card.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
}
.provider-card:disabled { opacity: .5; cursor: not-allowed; }
.provider-logo { width: 28px; height: 28px; object-fit: contain; border-radius: 6px; }
.provider-label { white-space: nowrap; }

/* 引导信息 */
.provider-guide {
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 4px;
}
.guide-row { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--el-text-color-regular); }
.guide-link { color: var(--el-color-primary); text-decoration: none; font-size: 12px; white-space: nowrap; }
.guide-link:hover { text-decoration: underline; }
.guide-row code { background: var(--el-fill-color); padding: 1px 5px; border-radius: 3px; font-size: 12px; }

.field-hint { font-size: 12px; color: var(--el-text-color-placeholder); margin-top: 4px; }

.relay-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  cursor: pointer;
  user-select: none;
}
.relay-toggle-label { font-size: 13px; color: #606266; font-weight: 500; }
</style>
