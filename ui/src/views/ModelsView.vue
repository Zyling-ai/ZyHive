<template>
  <div class="models-page">
    <div class="two-col-layout">

      <!-- ── 左侧：厂商列表 ── -->
      <div class="col-list">
        <div class="col-list-header">
          <span class="col-list-title">已配置厂商</span>
          <el-button size="small" type="primary" @click="openAddProvider">
            <el-icon><Plus /></el-icon> 添加
          </el-button>
        </div>
        <div v-if="providerList.length === 0" class="list-empty">
          暂未配置，点击「添加」开始
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
          <el-icon v-if="providerTestingIds.has(p.id)" class="pitem-status is-loading" style="color:#909399"><Loading /></el-icon>
          <el-tag v-else :type="p.status==='ok'?'success':p.status==='error'?'danger':'info'" size="small" class="pitem-status">
            {{ p.status==='ok' ? '✓' : p.status==='error' ? '✗' : '?' }}
          </el-tag>
        </div>
      </div>

      <!-- ── 右侧：详情 / 表单 ── -->
      <div class="col-form">

        <!-- ① 添加 / 编辑表单 -->
        <template v-if="providerForm.mode === 'add' || providerForm.mode === 'edit'">
          <div class="form-title">{{ providerForm.mode === 'add' ? '添加 API Key' : '编辑 ' + selectedProvider?.name }}</div>

          <div class="field-label">选择提供商 <span class="required">*</span></div>
          <div class="provider-grid">
            <button
              v-for="p in providerMetaList" :key="p.key" type="button"
              class="provider-card" :class="{ active: providerForm.provider === p.key }"
              @click="selectProviderType(p.key)" :disabled="providerForm.mode === 'edit'"
            >
              <img :src="p.logo" :alt="p.label" class="provider-logo" />
              <span class="provider-label">{{ p.label }}</span>
            </button>
          </div>

          <div v-if="currentProviderMeta" class="provider-guide">
            <div class="guide-row">
              <span>🔑</span><span>{{ currentProviderMeta.apiKeyHint }}</span>
              <a v-if="currentProviderMeta.apiKeyUrl" :href="currentProviderMeta.apiKeyUrl" target="_blank" class="guide-link">获取 API Key →</a>
            </div>
            <div v-if="currentProviderMeta.keyFormat" class="guide-row">
              <span>📋</span><span>格式：<code>{{ currentProviderMeta.keyFormat }}</code></span>
            </div>
          </div>

          <div class="field-label">名称</div>
          <el-input v-model="providerForm.name" :placeholder="currentProviderMeta?.label || '如 我的 DeepSeek'" />

          <div class="field-label">API Key <span class="required">*</span></div>
          <el-input v-model="providerForm.apiKey" type="password" show-password :placeholder="currentProviderMeta?.keyFormat || 'sk-...'" />

          <div class="relay-toggle" @click="providerForm.showRelay = !providerForm.showRelay">
            <el-switch :model-value="providerForm.showRelay" size="small" style="pointer-events:none" />
            <span class="relay-toggle-label">使用转发地址 <span class="hint">（国内绕过限制）</span></span>
          </div>
          <template v-if="providerForm.showRelay">
            <el-input v-model="providerForm.baseUrl" placeholder="填写中转地址，如 https://your-relay.com" clearable style="margin-top:6px" />
          </template>

          <div class="form-actions">
            <el-button @click="cancelProviderForm">取消</el-button>
            <el-button type="primary" @click="saveProvider" :loading="providerSaving">保存并测试</el-button>
          </div>
          <el-alert v-if="providerTestResult" :type="providerTestResult.ok?'success':'error'" :title="providerTestResult.msg" :closable="false" style="margin-top:12px" />
        </template>

        <!-- ② 已选中 Provider 详情 + 模型管理 -->
        <template v-else-if="selectedProvider">
          <!-- 基本信息 -->
          <div class="detail-header">
            <img :src="getProviderLogo(selectedProvider.provider)" class="detail-logo" />
            <div>
              <div class="form-title" style="margin-bottom:2px">{{ selectedProvider.name }}</div>
              <el-tag :type="selectedProvider.status==='ok'?'success':selectedProvider.status==='error'?'danger':'info'" size="small">
                {{ selectedProvider.status==='ok' ? '✓ 有效' : selectedProvider.status==='error' ? '✗ 无效' : '未测试' }}
              </el-tag>
            </div>
          </div>
          <div class="detail-grid">
            <div class="detail-row"><span class="detail-label">提供商</span><span>{{ getProviderLabel(selectedProvider.provider) }}</span></div>
            <div class="detail-row"><span class="detail-label">API Key</span><code>{{ selectedProvider.apiKey }}</code></div>
            <div v-if="selectedProvider.baseUrl" class="detail-row"><span class="detail-label">转发地址</span><span>{{ selectedProvider.baseUrl }}</span></div>
          </div>
          <div class="form-actions">
            <el-button @click="openEditProvider(selectedProvider)">编辑</el-button>
            <el-button type="success" @click="testProviderById(selectedProvider.id)" :loading="providerTesting">
              <el-icon><Refresh /></el-icon> 重新测试
            </el-button>
            <el-button type="danger" plain @click="deleteProvider(selectedProvider)">删除</el-button>
          </div>
          <el-alert v-if="providerTestResult" :type="providerTestResult.ok?'success':'error'" :title="providerTestResult.msg" :closable="false" style="margin-top:8px" />

          <!-- 模型管理区 -->
          <div class="section-divider"></div>
          <div class="section-title">
            <span>模型</span>
            <el-button size="small" type="primary" plain @click="fetchModelsForProvider" :loading="probing">
              <el-icon><Search /></el-icon> 获取可用模型
            </el-button>
          </div>

          <!-- 已添加的模型 -->
          <div v-if="providerModels.length" class="model-tags">
            <div v-for="m in providerModels" :key="m.id" class="model-tag-item">
              <div class="model-tag-info">
                <span class="model-tag-name">{{ m.name }}</span>
                <span class="model-tag-id">{{ m.model }}</span>
                <el-tag v-if="m.isDefault" type="warning" size="small">默认</el-tag>
                <el-tooltip v-if="m.supportsTools===false" content="不支持工具调用" placement="top">
                  <el-tag type="warning" size="small">⚠ 无工具</el-tag>
                </el-tooltip>
              </div>
              <el-button link type="danger" size="small" @click="deleteModel(m)">删除</el-button>
            </div>
          </div>
          <div v-else-if="!probedModels.length" class="list-empty" style="padding:12px 0">
            暂未添加模型，点击「获取可用模型」
          </div>

          <!-- 可选模型列表（获取后显示） -->
          <template v-if="probedModels.length">
            <div class="probed-header">
              <span style="font-size:13px;color:#606266">获取到 {{ probedModels.length }} 个模型，选择后批量添加：</span>
              <div style="display:flex;gap:6px">
                <el-button link size="small" @click="selectAllProbed">全选</el-button>
                <el-button link size="small" @click="selectedProbed = []">取消</el-button>
              </div>
            </div>
            <div class="probed-list">
              <label
                v-for="m in probedModels" :key="m.id"
                class="probed-item"
                :class="{ added: isModelAdded(m.id), selected: selectedProbed.includes(m.id) }"
              >
                <el-checkbox
                  :model-value="selectedProbed.includes(m.id) || isModelAdded(m.id)"
                  :disabled="isModelAdded(m.id)"
                  @change="toggleProbed(m.id)"
                />
                <span class="probed-name">{{ m.name && m.name !== m.id ? m.name : m.id }}</span>
                <span v-if="isModelAdded(m.id)" class="probed-added-tag">已添加</span>
              </label>
            </div>
            <div class="probed-actions">
              <span style="font-size:12px;color:#909399">已选 {{ selectedProbed.length }} 个</span>
              <el-button type="primary" :disabled="!selectedProbed.length" @click="batchAddModels" :loading="saving">
                添加选中模型
              </el-button>
            </div>
            <span v-if="probeError" style="font-size:12px;color:var(--el-color-danger)">{{ probeError }}</span>
          </template>
          <span v-else-if="probeError" style="font-size:12px;color:var(--el-color-danger);display:block;margin-top:8px">{{ probeError }}</span>
        </template>

        <!-- ③ 空状态 -->
        <template v-else>
          <div class="form-empty">
            <el-icon style="font-size:48px;color:#dcdfe6"><Key /></el-icon>
            <div style="margin-top:12px;color:#909399">从左侧选择厂商，或点击「添加」配置新的 API Key</div>
            <el-button type="primary" style="margin-top:16px" @click="openAddProvider">
              <el-icon><Plus /></el-icon> 添加第一个 API Key
            </el-button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Key, Search, Refresh, Loading } from '@element-plus/icons-vue'
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

interface ProviderMeta {
  key: string; label: string; logo: string; baseUrl: string
  apiKeyUrl: string; apiKeyHint: string; keyFormat?: string; modelHint?: string
}
const providerMetaList: ProviderMeta[] = [
  { key:'anthropic',  label:'Anthropic',   logo:iconAnthropic,  baseUrl:'https://api.anthropic.com',
    apiKeyUrl:'https://console.anthropic.com/settings/keys',  apiKeyHint:'在 Anthropic Console 创建 API Key', keyFormat:'sk-ant-api03-...' },
  { key:'openai',     label:'OpenAI',       logo:iconOpenAI,     baseUrl:'https://api.openai.com/v1',
    apiKeyUrl:'https://platform.openai.com/api-keys',          apiKeyHint:'在 OpenAI Platform 创建 API Key', keyFormat:'sk-proj-...' },
  { key:'deepseek',   label:'DeepSeek',     logo:iconDeepSeek,   baseUrl:'https://api.deepseek.com/v1',
    apiKeyUrl:'https://platform.deepseek.com/api_keys',        apiKeyHint:'在 DeepSeek Platform 创建 API Key', keyFormat:'sk-...' },
  { key:'kimi',       label:'Kimi',         logo:iconKimi,       baseUrl:'https://api.moonshot.cn/v1',
    apiKeyUrl:'https://platform.moonshot.cn/console/api-keys', apiKeyHint:'在月之暗面开放平台创建 API Key', keyFormat:'sk-...' },
  { key:'zhipu',      label:'智谱 GLM',     logo:iconZhipu,      baseUrl:'https://open.bigmodel.cn/api/paas/v4',
    apiKeyUrl:'https://open.bigmodel.cn/usercenter/apikeys',   apiKeyHint:'在智谱 AI 开放平台获取 API Key', keyFormat:'随机字符串' },
  { key:'minimax',    label:'MiniMax',      logo:iconMiniMax,    baseUrl:'https://api.minimax.chat/v1',
    apiKeyUrl:'https://platform.minimax.io/user-center/basic-information/interface-key', apiKeyHint:'在 MiniMax 平台获取 API Key', keyFormat:'eyJ...' },
  { key:'qwen',       label:'通义千问',     logo:iconQwen,       baseUrl:'https://dashscope.aliyuncs.com/compatible-mode/v1',
    apiKeyUrl:'https://dashscope.console.aliyun.com/apiKey',   apiKeyHint:'在阿里云 DashScope 控制台获取', keyFormat:'sk-...' },
  { key:'openrouter', label:'OpenRouter',   logo:iconOpenRouter, baseUrl:'https://openrouter.ai/api/v1',
    apiKeyUrl:'https://openrouter.ai/keys',                    apiKeyHint:'在 OpenRouter 创建 API Key，可访问数百个模型', keyFormat:'sk-or-v1-...' },
  { key:'custom',     label:'自定义',       logo:iconCustom,     baseUrl:'',
    apiKeyUrl:'',                                               apiKeyHint:'填写任意 OpenAI-compatible 接口地址和 API Key' },
]
const providerMetaMap = Object.fromEntries(providerMetaList.map(p => [p.key, p]))

function getProviderLogo(key: string)  { return providerMetaMap[key]?.logo  || iconCustom }
function getProviderLabel(key: string) { return providerMetaMap[key]?.label || key }

// ── State ─────────────────────────────────────────────────────────────────────
const providerList       = ref<ProviderEntry[]>([])
const selectedProvider   = ref<ProviderEntry | null>(null)
const providerSaving     = ref(false)
const providerTesting    = ref(false)
const providerTestingIds = ref<Set<string>>(new Set())
const providerTestResult = ref<{ ok: boolean; msg: string } | null>(null)
const providerForm = reactive({
  mode: 'idle' as 'idle' | 'add' | 'edit',
  provider: 'anthropic', name: '', apiKey: '', baseUrl: '', showRelay: false,
})

const allModels    = ref<ModelEntry[]>([])
const probing      = ref(false)
const probeError   = ref('')
const probedModels = ref<ProbeModelInfo[]>([])
const selectedProbed = ref<string[]>([])
const saving       = ref(false)

// ── Computed ──────────────────────────────────────────────────────────────────
const currentProviderMeta = computed<ProviderMeta | null>(() => providerMetaMap[providerForm.provider] || null)

// 当前选中 provider 下已添加的模型
const providerModels = computed<ModelEntry[]>(() =>
  selectedProvider.value
    ? allModels.value.filter(m => m.providerId === selectedProvider.value!.id)
    : []
)

function isModelAdded(modelId: string): boolean {
  return providerModels.value.some(m => m.model === modelId)
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  await loadProviders()
  await loadModels()
  autoTestAllProviders()
})

// ── Provider 操作 ─────────────────────────────────────────────────────────────
async function loadProviders() {
  try {
    const res = await providersApi.list()
    providerList.value = res.data.providers || []
  } catch {}
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    allModels.value = res.data
  } catch {}
}

function openAddProvider() {
  selectedProvider.value = null
  providerTestResult.value = null
  probedModels.value = []; selectedProbed.value = []; probeError.value = ''
  Object.assign(providerForm, { mode:'add', provider:'anthropic', name:'', apiKey:'', baseUrl:'', showRelay:false })
}

function openEditProvider(p: ProviderEntry) {
  providerTestResult.value = null
  Object.assign(providerForm, { mode:'edit', provider:p.provider, name:p.name, apiKey:'', baseUrl:p.baseUrl||'', showRelay:!!p.baseUrl })
}

function selectProvider(p: ProviderEntry) {
  selectedProvider.value = p
  providerForm.mode = 'idle'
  providerTestResult.value = null
  probedModels.value = []; selectedProbed.value = []; probeError.value = ''
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
    let savedId = ''
    if (providerForm.mode === 'edit' && selectedProvider.value) {
      const res = await providersApi.update(selectedProvider.value.id, payload)
      selectedProvider.value = res.data.provider
      savedId = res.data.provider.id
      ElMessage.success('已更新')
    } else {
      const res = await providersApi.create(payload)
      selectedProvider.value = res.data.provider
      savedId = res.data.provider.id
      ElMessage.success('已添加')
    }
    providerForm.mode = 'idle'
    await loadProviders()
    if (savedId) testProviderById(savedId)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    providerSaving.value = false
  }
}

async function testProviderById(id: string) {
  providerTesting.value = true
  providerTestResult.value = null
  try {
    const res = await providersApi.test(id)
    providerTestResult.value = { ok: res.data.status === 'ok', msg: res.data.message }
    await loadProviders()
    const updated = providerList.value.find(p => p.id === id)
    if (updated) selectedProvider.value = updated
  } catch (e: any) {
    providerTestResult.value = { ok: false, msg: e.response?.data?.error || '测试失败' }
  } finally {
    providerTesting.value = false
  }
}

async function deleteProvider(p: ProviderEntry) {
  if (p.modelCount > 0) { ElMessage.warning(`该 API Key 被 ${p.modelCount} 个模型使用，请先删除这些模型`); return }
  try {
    await ElMessageBox.confirm(`确定删除 "${p.name}" 的 API Key？`, '确认删除', { type: 'warning' })
    await providersApi.delete(p.id)
    selectedProvider.value = null; providerTestResult.value = null
    await loadProviders()
    ElMessage.success('已删除')
  } catch {}
}

// ── 自动测试 ──────────────────────────────────────────────────────────────────
async function autoTestAllProviders() {
  const ids = providerList.value.map(p => p.id)
  if (!ids.length) return
  await Promise.allSettled(ids.map(async id => {
    await testProviderSilent(id)
    await loadProviders()
    if (selectedProvider.value?.id === id) {
      const updated = providerList.value.find(p => p.id === id)
      if (updated) selectedProvider.value = updated
    }
  }))
}

async function testProviderSilent(id: string) {
  providerTestingIds.value = new Set([...providerTestingIds.value, id])
  try { await providersApi.test(id) } catch {}
  finally {
    const s = new Set(providerTestingIds.value); s.delete(id)
    providerTestingIds.value = s
  }
}

// ── 模型管理 ──────────────────────────────────────────────────────────────────
async function fetchModelsForProvider() {
  if (!selectedProvider.value) return
  probing.value = true; probeError.value = ''; probedModels.value = []; selectedProbed.value = []
  try {
    const p = selectedProvider.value
    const baseUrl = p.baseUrl || providerMetaMap[p.provider]?.baseUrl || ''
    const res = await modelsApi.probe(baseUrl, undefined, p.provider, p.id)
    probedModels.value = res.data.models || []
    if (!probedModels.value.length) probeError.value = '未获取到模型列表（接口返回为空）'
  } catch (e: any) {
    probeError.value = e.response?.data?.error || e.message || '获取失败'
  } finally {
    probing.value = false
  }
}

function toggleProbed(modelId: string) {
  const idx = selectedProbed.value.indexOf(modelId)
  if (idx >= 0) selectedProbed.value.splice(idx, 1)
  else selectedProbed.value.push(modelId)
}

function selectAllProbed() {
  selectedProbed.value = probedModels.value
    .filter(m => !isModelAdded(m.id))
    .map(m => m.id)
}

async function batchAddModels() {
  if (!selectedProvider.value || !selectedProbed.value.length) return
  saving.value = true
  const p = selectedProvider.value
  const toAdd = probedModels.value.filter(m => selectedProbed.value.includes(m.id))
  let added = 0
  for (const m of toAdd) {
    const id = m.id.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '')
    try {
      await modelsApi.create({
        id,
        name: (m.name && m.name !== m.id) ? m.name : m.id,
        provider: p.provider,
        model: m.id,
        providerId: p.id,
        isDefault: allModels.value.length === 0 && added === 0,
        status: 'untested',
      } as any)
      added++
    } catch {}
  }
  ElMessage.success(`已添加 ${added} 个模型`)
  selectedProbed.value = []
  await loadModels()
  // 刷新 provider 引用计数
  await loadProviders()
  const updated = providerList.value.find(pp => pp.id === p.id)
  if (updated) selectedProvider.value = updated
  saving.value = false
}

async function deleteModel(m: ModelEntry) {
  try {
    await ElMessageBox.confirm(`确定删除模型 "${m.name}"？`, '确认删除', { type: 'warning' })
    await modelsApi.delete(m.id)
    ElMessage.success('已删除')
    await loadModels()
    await loadProviders()
    const updated = providerList.value.find(pp => pp.id === selectedProvider.value?.id)
    if (updated) selectedProvider.value = updated
  } catch {}
}
</script>

<style scoped>
.models-page { padding: 0; }

/* ── 两栏布局 ── */
.two-col-layout {
  display: flex;
  min-height: 600px;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  overflow: hidden;
  background: var(--el-bg-color);
}

/* 左列 */
.col-list {
  width: 220px;
  flex-shrink: 0;
  border-right: 1px solid var(--el-border-color);
  display: flex;
  flex-direction: column;
}
.col-list-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 14px; border-bottom: 1px solid var(--el-border-color);
}
.col-list-title { font-weight: 600; font-size: 14px; }
.list-empty { padding: 20px 16px; font-size: 13px; color: #909399; text-align: center; }
.provider-item {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 14px; cursor: pointer; transition: background .15s;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.provider-item:hover { background: var(--el-fill-color-light); }
.provider-item.active { background: var(--el-color-primary-light-9); }
.pitem-logo { width: 26px; height: 26px; object-fit: contain; border-radius: 5px; flex-shrink: 0; }
.pitem-info { flex: 1; min-width: 0; }
.pitem-name { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pitem-sub  { font-size: 11px; color: #909399; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pitem-status { flex-shrink: 0; }

/* 右列 */
.col-form { flex: 1; padding: 24px 28px; overflow-y: auto; }
.form-title { font-size: 16px; font-weight: 600; margin-bottom: 12px; color: var(--el-text-color-primary); }
.form-empty {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  height: 100%; min-height: 400px; color: #909399;
}
.field-label { font-size: 13px; color: #606266; margin: 14px 0 6px; font-weight: 500; }
.required { color: var(--el-color-danger); }
.hint { font-weight: 400; color: #909399; font-size: 12px; }
.form-actions { display: flex; gap: 8px; margin-top: 18px; }

/* 详情 */
.detail-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.detail-logo { width: 36px; height: 36px; object-fit: contain; border-radius: 8px; }
.detail-grid { margin-bottom: 4px; }
.detail-row { display: flex; gap: 12px; padding: 7px 0; border-bottom: 1px solid var(--el-border-color-lighter); font-size: 14px; }
.detail-label { width: 80px; flex-shrink: 0; color: #909399; }

/* 分割线 + 区块标题 */
.section-divider { height: 1px; background: var(--el-border-color); margin: 24px 0 16px; }
.section-title { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; font-weight: 600; font-size: 14px; }

/* 已添加模型 */
.model-tags { display: flex; flex-direction: column; gap: 6px; margin-bottom: 4px; }
.model-tag-item { display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; background: var(--el-fill-color-light); border-radius: 6px; }
.model-tag-info { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; }
.model-tag-name { font-size: 13px; font-weight: 500; }
.model-tag-id { font-size: 12px; color: #909399; }

/* 探测列表 */
.probed-header { display: flex; align-items: center; justify-content: space-between; margin: 16px 0 8px; }
.probed-list { display: flex; flex-direction: column; gap: 2px; max-height: 280px; overflow-y: auto; border: 1px solid var(--el-border-color); border-radius: 6px; padding: 6px; }
.probed-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 8px; border-radius: 4px; cursor: pointer; transition: background .12s;
}
.probed-item:hover:not(.added) { background: var(--el-fill-color-light); }
.probed-item.added { opacity: .6; cursor: default; }
.probed-item.selected { background: var(--el-color-primary-light-9); }
.probed-name { font-size: 13px; flex: 1; }
.probed-added-tag { font-size: 11px; color: var(--el-color-success); font-weight: 500; }
.probed-actions { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; }

/* Provider 网格 */
.provider-grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 8px; margin-bottom: 12px; }
.provider-card {
  display: flex; flex-direction: column; align-items: center; gap: 4px;
  padding: 8px 4px; border: 1.5px solid var(--el-border-color); border-radius: 8px;
  background: var(--el-bg-color); cursor: pointer; transition: border-color .15s, background .15s;
  font-size: 12px; color: var(--el-text-color-regular);
}
.provider-card:hover { border-color: var(--el-color-primary); background: var(--el-color-primary-light-9); }
.provider-card.active { border-color: var(--el-color-primary); background: var(--el-color-primary-light-9); color: var(--el-color-primary); font-weight: 600; }
.provider-card:disabled { opacity: .5; cursor: not-allowed; }
.provider-logo { width: 28px; height: 28px; object-fit: contain; border-radius: 6px; }
.provider-label { white-space: nowrap; }

/* 引导信息 */
.provider-guide { background: var(--el-fill-color-light); border: 1px solid var(--el-border-color); border-radius: 8px; padding: 10px 14px; display: flex; flex-direction: column; gap: 6px; margin-bottom: 4px; }
.guide-row { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--el-text-color-regular); }
.guide-link { color: var(--el-color-primary); text-decoration: none; font-size: 12px; white-space: nowrap; }
.guide-link:hover { text-decoration: underline; }
.guide-row code { background: var(--el-fill-color); padding: 1px 5px; border-radius: 3px; font-size: 12px; }

/* 转发开关 */
.relay-toggle { display: flex; align-items: center; gap: 8px; margin-top: 14px; cursor: pointer; user-select: none; }
.relay-toggle-label { font-size: 13px; color: #606266; font-weight: 500; }
</style>
