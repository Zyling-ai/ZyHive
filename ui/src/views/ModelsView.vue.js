/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, reactive, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus, Key, Search, Refresh, Loading } from '@element-plus/icons-vue';
import { models as modelsApi, providers as providersApi } from '../api';
// ── Provider logo imports ─────────────────────────────────────────────────────
import iconAnthropic from '../assets/providers/anthropic.svg';
import iconOpenAI from '../assets/providers/openai.png';
import iconDeepSeek from '../assets/providers/deepseek.png';
import iconKimi from '../assets/providers/kimi.png';
import iconZhipu from '../assets/providers/zhipu.png';
import iconMiniMax from '../assets/providers/minimax.png';
import iconQwen from '../assets/providers/qwen.png';
import iconOpenRouter from '../assets/providers/openrouter.svg';
import iconCustom from '../assets/providers/custom.svg';
const providerMetaList = [
    { key: 'anthropic', label: 'Anthropic', logo: iconAnthropic, baseUrl: 'https://api.anthropic.com',
        apiKeyUrl: 'https://console.anthropic.com/settings/keys', apiKeyHint: '在 Anthropic Console 创建 API Key', keyFormat: 'sk-ant-api03-...' },
    { key: 'openai', label: 'OpenAI', logo: iconOpenAI, baseUrl: 'https://api.openai.com/v1',
        apiKeyUrl: 'https://platform.openai.com/api-keys', apiKeyHint: '在 OpenAI Platform 创建 API Key', keyFormat: 'sk-proj-...' },
    { key: 'deepseek', label: 'DeepSeek', logo: iconDeepSeek, baseUrl: 'https://api.deepseek.com/v1',
        apiKeyUrl: 'https://platform.deepseek.com/api_keys', apiKeyHint: '在 DeepSeek Platform 创建 API Key', keyFormat: 'sk-...' },
    { key: 'kimi', label: 'Kimi', logo: iconKimi, baseUrl: 'https://api.moonshot.cn/v1',
        apiKeyUrl: 'https://platform.moonshot.cn/console/api-keys', apiKeyHint: '在月之暗面开放平台创建 API Key', keyFormat: 'sk-...' },
    { key: 'zhipu', label: '智谱 GLM', logo: iconZhipu, baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
        apiKeyUrl: 'https://open.bigmodel.cn/usercenter/apikeys', apiKeyHint: '在智谱 AI 开放平台获取 API Key', keyFormat: '随机字符串' },
    { key: 'minimax', label: 'MiniMax', logo: iconMiniMax, baseUrl: 'https://api.minimax.chat/v1',
        apiKeyUrl: 'https://platform.minimax.io/user-center/basic-information/interface-key', apiKeyHint: '在 MiniMax 平台获取 API Key', keyFormat: 'eyJ...' },
    { key: 'qwen', label: '通义千问', logo: iconQwen, baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
        apiKeyUrl: 'https://dashscope.console.aliyun.com/apiKey', apiKeyHint: '在阿里云 DashScope 控制台获取', keyFormat: 'sk-...' },
    { key: 'openrouter', label: 'OpenRouter', logo: iconOpenRouter, baseUrl: 'https://openrouter.ai/api/v1',
        apiKeyUrl: 'https://openrouter.ai/keys', apiKeyHint: '在 OpenRouter 创建 API Key，可访问数百个模型', keyFormat: 'sk-or-v1-...' },
    { key: 'ollama', label: 'Ollama (本地)', logo: iconCustom, baseUrl: 'http://localhost:11434',
        apiKeyUrl: 'https://ollama.com', apiKeyHint: 'Ollama 本地服务，无需 API Key。需先运行 ollama serve',
        keyFormat: '（留空即可）', modelHint: 'nomic-embed-text / mxbai-embed-large' },
    { key: 'custom', label: '自定义', logo: iconCustom, baseUrl: '',
        apiKeyUrl: '', apiKeyHint: '填写任意 OpenAI-compatible 接口地址和 API Key' },
];
const providerMetaMap = Object.fromEntries(providerMetaList.map(p => [p.key, p]));
function getProviderLogo(key) { return providerMetaMap[key]?.logo || iconCustom; }
function getProviderLabel(key) { return providerMetaMap[key]?.label || key; }
// ── State ─────────────────────────────────────────────────────────────────────
const providerList = ref([]);
const selectedProvider = ref(null);
const providerSaving = ref(false);
const providerTesting = ref(false);
const providerTestingIds = ref(new Set());
const providerTestResult = ref(null);
const providerForm = reactive({
    mode: 'idle',
    provider: 'anthropic', name: '', apiKey: '', baseUrl: '', embedModel: '', showRelay: false,
});
const allModels = ref([]);
const probing = ref(false);
const probeError = ref('');
const probedModels = ref([]);
const selectedProbed = ref([]);
const saving = ref(false);
// ── Computed ──────────────────────────────────────────────────────────────────
const currentProviderMeta = computed(() => providerMetaMap[providerForm.provider] || null);
// 当前选中 provider 下已添加的模型
const providerModels = computed(() => selectedProvider.value
    ? allModels.value.filter(m => m.providerId === selectedProvider.value.id)
    : []);
function isModelAdded(modelId) {
    return providerModels.value.some(m => m.model === modelId);
}
// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
    await loadProviders();
    await loadModels();
    autoTestAllProviders();
});
// ── Provider 操作 ─────────────────────────────────────────────────────────────
async function loadProviders() {
    try {
        const res = await providersApi.list();
        providerList.value = res.data.providers || [];
    }
    catch { }
}
async function loadModels() {
    try {
        const res = await modelsApi.list();
        allModels.value = res.data;
    }
    catch { }
}
function openAddProvider() {
    selectedProvider.value = null;
    providerTestResult.value = null;
    probedModels.value = [];
    selectedProbed.value = [];
    probeError.value = '';
    Object.assign(providerForm, { mode: 'add', provider: 'anthropic', name: '', apiKey: '', baseUrl: '', showRelay: false });
}
function openEditProvider(p) {
    providerTestResult.value = null;
    Object.assign(providerForm, { mode: 'edit', provider: p.provider, name: p.name, apiKey: '', baseUrl: p.baseUrl || '', embedModel: p.embedModel || '', showRelay: !!p.baseUrl });
}
function selectProvider(p) {
    selectedProvider.value = p;
    providerForm.mode = 'idle';
    providerTestResult.value = null;
    probedModels.value = [];
    selectedProbed.value = [];
    probeError.value = '';
}
function selectProviderType(key) {
    if (providerForm.mode === 'edit')
        return;
    providerForm.provider = key;
    if (!providerForm.name)
        providerForm.name = providerMetaMap[key]?.label || key;
}
function cancelProviderForm() {
    providerForm.mode = 'idle';
    providerTestResult.value = null;
}
// Providers that don't need an API key (local services)
const noKeyProviders = new Set(['ollama']);
async function saveProvider() {
    if (!providerForm.provider) {
        ElMessage.warning('请选择提供商');
        return;
    }
    if (!providerForm.apiKey && providerForm.mode === 'add' && !noKeyProviders.has(providerForm.provider)) {
        ElMessage.warning('请填写 API Key');
        return;
    }
    providerSaving.value = true;
    try {
        const payload = {
            provider: providerForm.provider,
            name: providerForm.name || providerMetaMap[providerForm.provider]?.label || providerForm.provider,
            apiKey: providerForm.apiKey,
            baseUrl: providerForm.baseUrl,
            embedModel: providerForm.embedModel || undefined,
        };
        let savedId = '';
        if (providerForm.mode === 'edit' && selectedProvider.value) {
            const res = await providersApi.update(selectedProvider.value.id, payload);
            selectedProvider.value = res.data.provider;
            savedId = res.data.provider.id;
            ElMessage.success('已更新');
        }
        else {
            const res = await providersApi.create(payload);
            selectedProvider.value = res.data.provider;
            savedId = res.data.provider.id;
            ElMessage.success('已添加');
        }
        providerForm.mode = 'idle';
        await loadProviders();
        if (savedId)
            testProviderById(savedId);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        providerSaving.value = false;
    }
}
async function testProviderById(id) {
    providerTesting.value = true;
    providerTestResult.value = null;
    try {
        const res = await providersApi.test(id);
        providerTestResult.value = { ok: res.data.status === 'ok', msg: res.data.message };
        await loadProviders();
        const updated = providerList.value.find(p => p.id === id);
        if (updated)
            selectedProvider.value = updated;
    }
    catch (e) {
        providerTestResult.value = { ok: false, msg: e.response?.data?.error || '测试失败' };
    }
    finally {
        providerTesting.value = false;
    }
}
async function deleteProvider(p) {
    if (p.modelCount > 0) {
        ElMessage.warning(`该 API Key 被 ${p.modelCount} 个模型使用，请先删除这些模型`);
        return;
    }
    try {
        await ElMessageBox.confirm(`确定删除 "${p.name}" 的 API Key？`, '确认删除', { type: 'warning' });
        await providersApi.delete(p.id);
        selectedProvider.value = null;
        providerTestResult.value = null;
        await loadProviders();
        ElMessage.success('已删除');
    }
    catch { }
}
// ── 自动测试 ──────────────────────────────────────────────────────────────────
async function autoTestAllProviders() {
    const ids = providerList.value.map(p => p.id);
    if (!ids.length)
        return;
    await Promise.allSettled(ids.map(async (id) => {
        await testProviderSilent(id);
        await loadProviders();
        if (selectedProvider.value?.id === id) {
            const updated = providerList.value.find(p => p.id === id);
            if (updated)
                selectedProvider.value = updated;
        }
    }));
}
async function testProviderSilent(id) {
    providerTestingIds.value = new Set([...providerTestingIds.value, id]);
    try {
        await providersApi.test(id);
    }
    catch { }
    finally {
        const s = new Set(providerTestingIds.value);
        s.delete(id);
        providerTestingIds.value = s;
    }
}
// ── 模型管理 ──────────────────────────────────────────────────────────────────
async function fetchModelsForProvider() {
    if (!selectedProvider.value)
        return;
    probing.value = true;
    probeError.value = '';
    probedModels.value = [];
    selectedProbed.value = [];
    try {
        const p = selectedProvider.value;
        const baseUrl = p.baseUrl || providerMetaMap[p.provider]?.baseUrl || '';
        const res = await modelsApi.probe(baseUrl, undefined, p.provider, p.id);
        probedModels.value = res.data.models || [];
        if (!probedModels.value.length)
            probeError.value = '未获取到模型列表（接口返回为空）';
    }
    catch (e) {
        probeError.value = e.response?.data?.error || e.message || '获取失败';
    }
    finally {
        probing.value = false;
    }
}
function toggleProbed(modelId) {
    const idx = selectedProbed.value.indexOf(modelId);
    if (idx >= 0)
        selectedProbed.value.splice(idx, 1);
    else
        selectedProbed.value.push(modelId);
}
function selectAllProbed() {
    selectedProbed.value = probedModels.value
        .filter(m => !isModelAdded(m.id))
        .map(m => m.id);
}
async function batchAddModels() {
    if (!selectedProvider.value || !selectedProbed.value.length)
        return;
    saving.value = true;
    const p = selectedProvider.value;
    const toAdd = probedModels.value.filter(m => selectedProbed.value.includes(m.id));
    let added = 0;
    for (const m of toAdd) {
        const id = m.id.replace(/[^a-z0-9]/gi, '-').toLowerCase().replace(/-+/g, '-').replace(/^-|-$/g, '');
        try {
            await modelsApi.create({
                id,
                name: (m.name && m.name !== m.id) ? m.name : m.id,
                provider: p.provider,
                model: m.id,
                providerId: p.id,
                isDefault: allModels.value.length === 0 && added === 0,
                status: 'untested',
            });
            added++;
        }
        catch { }
    }
    ElMessage.success(`已添加 ${added} 个模型`);
    selectedProbed.value = [];
    await loadModels();
    // 刷新 provider 引用计数
    await loadProviders();
    const updated = providerList.value.find(pp => pp.id === p.id);
    if (updated)
        selectedProvider.value = updated;
    saving.value = false;
}
async function deleteModel(m) {
    try {
        await ElMessageBox.confirm(`确定删除模型 "${m.name}"？`, '确认删除', { type: 'warning' });
        await modelsApi.delete(m.id);
        ElMessage.success('已删除');
        await loadModels();
        await loadProviders();
        const updated = providerList.value.find(pp => pp.id === selectedProvider.value?.id);
        if (updated)
            selectedProvider.value = updated;
    }
    catch { }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['pitem-name']} */ ;
/** @type {__VLS_StyleScopedClasses['embed-hint']} */ ;
/** @type {__VLS_StyleScopedClasses['probed-item']} */ ;
/** @type {__VLS_StyleScopedClasses['probed-item']} */ ;
/** @type {__VLS_StyleScopedClasses['added']} */ ;
/** @type {__VLS_StyleScopedClasses['probed-item']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-card']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-card']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['provider-card']} */ ;
/** @type {__VLS_StyleScopedClasses['guide-link']} */ ;
/** @type {__VLS_StyleScopedClasses['guide-row']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "models-page" },
});
/** @type {__VLS_StyleScopedClasses['models-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "two-col-layout" },
});
/** @type {__VLS_StyleScopedClasses['two-col-layout']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "col-list" },
});
/** @type {__VLS_StyleScopedClasses['col-list']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "col-list-header" },
});
/** @type {__VLS_StyleScopedClasses['col-list-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "col-list-title" },
});
/** @type {__VLS_StyleScopedClasses['col-list-title']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (__VLS_ctx.openAddProvider) });
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[openAddProvider,];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
if (__VLS_ctx.providerList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "list-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['list-empty']} */ ;
}
for (const [p] of __VLS_vFor((__VLS_ctx.providerList))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectProvider(p);
                // @ts-ignore
                [providerList, providerList, selectProvider,];
            } },
        key: (p.id),
        ...{ class: "provider-item" },
        ...{ class: ({ active: __VLS_ctx.selectedProvider?.id === p.id }) },
    });
    /** @type {__VLS_StyleScopedClasses['provider-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
        src: (__VLS_ctx.getProviderLogo(p.provider)),
        ...{ class: "pitem-logo" },
    });
    /** @type {__VLS_StyleScopedClasses['pitem-logo']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "pitem-info" },
    });
    /** @type {__VLS_StyleScopedClasses['pitem-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "pitem-name" },
    });
    /** @type {__VLS_StyleScopedClasses['pitem-name']} */ ;
    (p.name);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "pitem-sub" },
    });
    /** @type {__VLS_StyleScopedClasses['pitem-sub']} */ ;
    (p.apiKey);
    if (__VLS_ctx.providerTestingIds.has(p.id)) {
        let __VLS_19;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
            ...{ class: "pitem-status is-loading" },
            ...{ style: {} },
        }));
        const __VLS_21 = __VLS_20({
            ...{ class: "pitem-status is-loading" },
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_20));
        /** @type {__VLS_StyleScopedClasses['pitem-status']} */ ;
        /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
        const { default: __VLS_24 } = __VLS_22.slots;
        let __VLS_25;
        /** @ts-ignore @type { | typeof __VLS_components.Loading} */
        Loading;
        // @ts-ignore
        const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({}));
        const __VLS_27 = __VLS_26({}, ...__VLS_functionalComponentArgsRest(__VLS_26));
        // @ts-ignore
        [selectedProvider, getProviderLogo, providerTestingIds,];
        var __VLS_22;
    }
    else {
        let __VLS_30;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_31 = __VLS_asFunctionalComponent1(__VLS_30, new __VLS_30({
            type: (p.status === 'ok' ? 'success' : p.status === 'error' ? 'danger' : 'info'),
            size: "small",
            ...{ class: "pitem-status" },
        }));
        const __VLS_32 = __VLS_31({
            type: (p.status === 'ok' ? 'success' : p.status === 'error' ? 'danger' : 'info'),
            size: "small",
            ...{ class: "pitem-status" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_31));
        /** @type {__VLS_StyleScopedClasses['pitem-status']} */ ;
        const { default: __VLS_35 } = __VLS_33.slots;
        (p.status === 'ok' ? '✓' : p.status === 'error' ? '✗' : '?');
        // @ts-ignore
        [];
        var __VLS_33;
    }
    // @ts-ignore
    [];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "col-form" },
});
/** @type {__VLS_StyleScopedClasses['col-form']} */ ;
if (__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit') {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-title" },
    });
    /** @type {__VLS_StyleScopedClasses['form-title']} */ ;
    (__VLS_ctx.providerForm.mode === 'add' ? '添加 API Key' : '编辑 ' + __VLS_ctx.selectedProvider?.name);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "field-label" },
    });
    /** @type {__VLS_StyleScopedClasses['field-label']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "required" },
    });
    /** @type {__VLS_StyleScopedClasses['required']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "provider-grid" },
    });
    /** @type {__VLS_StyleScopedClasses['provider-grid']} */ ;
    for (const [p] of __VLS_vFor((__VLS_ctx.providerMetaList))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
            ...{ onClick: (...[$event]) => {
                    if (!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                        return;
                    __VLS_ctx.selectProviderType(p.key);
                    // @ts-ignore
                    [selectedProvider, providerForm, providerForm, providerForm, providerMetaList, selectProviderType,];
                } },
            key: (p.key),
            type: "button",
            ...{ class: "provider-card" },
            ...{ class: ({ active: __VLS_ctx.providerForm.provider === p.key }) },
            disabled: (__VLS_ctx.providerForm.mode === 'edit'),
        });
        /** @type {__VLS_StyleScopedClasses['provider-card']} */ ;
        /** @type {__VLS_StyleScopedClasses['active']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
            src: (p.logo),
            alt: (p.label),
            ...{ class: "provider-logo" },
        });
        /** @type {__VLS_StyleScopedClasses['provider-logo']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "provider-label" },
        });
        /** @type {__VLS_StyleScopedClasses['provider-label']} */ ;
        (p.label);
        // @ts-ignore
        [providerForm, providerForm,];
    }
    if (__VLS_ctx.currentProviderMeta) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "provider-guide" },
        });
        /** @type {__VLS_StyleScopedClasses['provider-guide']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "guide-row" },
        });
        /** @type {__VLS_StyleScopedClasses['guide-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        (__VLS_ctx.currentProviderMeta.apiKeyHint);
        if (__VLS_ctx.currentProviderMeta.apiKeyUrl) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
                href: (__VLS_ctx.currentProviderMeta.apiKeyUrl),
                target: "_blank",
                ...{ class: "guide-link" },
            });
            /** @type {__VLS_StyleScopedClasses['guide-link']} */ ;
        }
        if (__VLS_ctx.currentProviderMeta.keyFormat) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "guide-row" },
            });
            /** @type {__VLS_StyleScopedClasses['guide-row']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
            (__VLS_ctx.currentProviderMeta.keyFormat);
        }
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "field-label" },
    });
    /** @type {__VLS_StyleScopedClasses['field-label']} */ ;
    let __VLS_36;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_37 = __VLS_asFunctionalComponent1(__VLS_36, new __VLS_36({
        modelValue: (__VLS_ctx.providerForm.name),
        placeholder: (__VLS_ctx.currentProviderMeta?.label || '如 我的 DeepSeek'),
    }));
    const __VLS_38 = __VLS_37({
        modelValue: (__VLS_ctx.providerForm.name),
        placeholder: (__VLS_ctx.currentProviderMeta?.label || '如 我的 DeepSeek'),
    }, ...__VLS_functionalComponentArgsRest(__VLS_37));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "field-label" },
    });
    /** @type {__VLS_StyleScopedClasses['field-label']} */ ;
    if (!__VLS_ctx.noKeyProviders.has(__VLS_ctx.providerForm.provider)) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "required" },
        });
        /** @type {__VLS_StyleScopedClasses['required']} */ ;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "optional-tag" },
        });
        /** @type {__VLS_StyleScopedClasses['optional-tag']} */ ;
    }
    let __VLS_41;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_42 = __VLS_asFunctionalComponent1(__VLS_41, new __VLS_41({
        modelValue: (__VLS_ctx.providerForm.apiKey),
        type: "password",
        showPassword: true,
        placeholder: (__VLS_ctx.noKeyProviders.has(__VLS_ctx.providerForm.provider) ? '本地服务无需填写' : (__VLS_ctx.currentProviderMeta?.keyFormat || 'sk-...')),
    }));
    const __VLS_43 = __VLS_42({
        modelValue: (__VLS_ctx.providerForm.apiKey),
        type: "password",
        showPassword: true,
        placeholder: (__VLS_ctx.noKeyProviders.has(__VLS_ctx.providerForm.provider) ? '本地服务无需填写' : (__VLS_ctx.currentProviderMeta?.keyFormat || 'sk-...')),
    }, ...__VLS_functionalComponentArgsRest(__VLS_42));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                    return;
                __VLS_ctx.providerForm.showRelay = !__VLS_ctx.providerForm.showRelay;
                // @ts-ignore
                [providerForm, providerForm, providerForm, providerForm, providerForm, providerForm, currentProviderMeta, currentProviderMeta, currentProviderMeta, currentProviderMeta, currentProviderMeta, currentProviderMeta, currentProviderMeta, currentProviderMeta, noKeyProviders, noKeyProviders,];
            } },
        ...{ class: "relay-toggle" },
    });
    /** @type {__VLS_StyleScopedClasses['relay-toggle']} */ ;
    let __VLS_46;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_47 = __VLS_asFunctionalComponent1(__VLS_46, new __VLS_46({
        modelValue: (__VLS_ctx.providerForm.showRelay),
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_48 = __VLS_47({
        modelValue: (__VLS_ctx.providerForm.showRelay),
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_47));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "relay-toggle-label" },
    });
    /** @type {__VLS_StyleScopedClasses['relay-toggle-label']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "hint" },
    });
    /** @type {__VLS_StyleScopedClasses['hint']} */ ;
    if (__VLS_ctx.providerForm.showRelay) {
        let __VLS_51;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_52 = __VLS_asFunctionalComponent1(__VLS_51, new __VLS_51({
            modelValue: (__VLS_ctx.providerForm.baseUrl),
            placeholder: (__VLS_ctx.noKeyProviders.has(__VLS_ctx.providerForm.provider) ? 'http://localhost:11434' : 'https://your-relay.com'),
            clearable: true,
            ...{ style: {} },
        }));
        const __VLS_53 = __VLS_52({
            modelValue: (__VLS_ctx.providerForm.baseUrl),
            placeholder: (__VLS_ctx.noKeyProviders.has(__VLS_ctx.providerForm.provider) ? 'http://localhost:11434' : 'https://your-relay.com'),
            clearable: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_52));
    }
    if (['openai', 'zhipu', 'minimax', 'ollama', 'custom'].includes(__VLS_ctx.providerForm.provider)) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "field-label" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['field-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "optional-tag" },
        });
        /** @type {__VLS_StyleScopedClasses['optional-tag']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "hint" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['hint']} */ ;
        let __VLS_56;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_57 = __VLS_asFunctionalComponent1(__VLS_56, new __VLS_56({
            modelValue: (__VLS_ctx.providerForm.embedModel),
            placeholder: (__VLS_ctx.currentProviderMeta?.modelHint || '留空使用默认（如 text-embedding-3-small）'),
            clearable: true,
        }));
        const __VLS_58 = __VLS_57({
            modelValue: (__VLS_ctx.providerForm.embedModel),
            placeholder: (__VLS_ctx.currentProviderMeta?.modelHint || '留空使用默认（如 text-embedding-3-small）'),
            clearable: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_57));
        if (__VLS_ctx.providerForm.provider === 'ollama') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "embed-hint" },
            });
            /** @type {__VLS_StyleScopedClasses['embed-hint']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
        }
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['form-actions']} */ ;
    let __VLS_61;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_62 = __VLS_asFunctionalComponent1(__VLS_61, new __VLS_61({
        ...{ 'onClick': {} },
    }));
    const __VLS_63 = __VLS_62({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_62));
    let __VLS_66;
    const __VLS_67 = ({ click: {} },
        { onClick: (__VLS_ctx.cancelProviderForm) });
    const { default: __VLS_68 } = __VLS_64.slots;
    // @ts-ignore
    [providerForm, providerForm, providerForm, providerForm, providerForm, providerForm, providerForm, currentProviderMeta, noKeyProviders, cancelProviderForm,];
    var __VLS_64;
    var __VLS_65;
    let __VLS_69;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_70 = __VLS_asFunctionalComponent1(__VLS_69, new __VLS_69({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.providerSaving),
    }));
    const __VLS_71 = __VLS_70({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.providerSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_70));
    let __VLS_74;
    const __VLS_75 = ({ click: {} },
        { onClick: (__VLS_ctx.saveProvider) });
    const { default: __VLS_76 } = __VLS_72.slots;
    // @ts-ignore
    [providerSaving, saveProvider,];
    var __VLS_72;
    var __VLS_73;
    if (__VLS_ctx.providerTestResult) {
        let __VLS_77;
        /** @ts-ignore @type { | typeof __VLS_components.elAlert | typeof __VLS_components.ElAlert | typeof __VLS_components['el-alert']} */
        elAlert;
        // @ts-ignore
        const __VLS_78 = __VLS_asFunctionalComponent1(__VLS_77, new __VLS_77({
            type: (__VLS_ctx.providerTestResult.ok ? 'success' : 'error'),
            title: (__VLS_ctx.providerTestResult.msg),
            closable: (false),
            ...{ style: {} },
        }));
        const __VLS_79 = __VLS_78({
            type: (__VLS_ctx.providerTestResult.ok ? 'success' : 'error'),
            title: (__VLS_ctx.providerTestResult.msg),
            closable: (false),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_78));
    }
}
else if (__VLS_ctx.selectedProvider) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-header" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.img)({
        src: (__VLS_ctx.getProviderLogo(__VLS_ctx.selectedProvider.provider)),
        ...{ class: "detail-logo" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-logo']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-title" },
        ...{ style: {} },
    });
    /** @type {__VLS_StyleScopedClasses['form-title']} */ ;
    (__VLS_ctx.selectedProvider.name);
    let __VLS_82;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_83 = __VLS_asFunctionalComponent1(__VLS_82, new __VLS_82({
        type: (__VLS_ctx.selectedProvider.status === 'ok' ? 'success' : __VLS_ctx.selectedProvider.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }));
    const __VLS_84 = __VLS_83({
        type: (__VLS_ctx.selectedProvider.status === 'ok' ? 'success' : __VLS_ctx.selectedProvider.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_83));
    const { default: __VLS_87 } = __VLS_85.slots;
    (__VLS_ctx.selectedProvider.status === 'ok' ? '✓ 有效' : __VLS_ctx.selectedProvider.status === 'error' ? '✗ 无效' : '未测试');
    // @ts-ignore
    [selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, getProviderLogo, providerTestResult, providerTestResult, providerTestResult,];
    var __VLS_85;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-grid" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-grid']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-row" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-row']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "detail-label" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-label']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.getProviderLabel(__VLS_ctx.selectedProvider.provider));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-row" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-row']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "detail-label" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-label']} */ ;
    if (__VLS_ctx.selectedProvider.apiKey) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
        (__VLS_ctx.selectedProvider.apiKey.slice(0, 8));
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "hint" },
        });
        /** @type {__VLS_StyleScopedClasses['hint']} */ ;
    }
    if (__VLS_ctx.selectedProvider.baseUrl) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "detail-row" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "detail-label" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        (__VLS_ctx.selectedProvider.baseUrl);
    }
    if (__VLS_ctx.selectedProvider.embedModel) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "detail-row" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "detail-label" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
        (__VLS_ctx.selectedProvider.embedModel);
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['form-actions']} */ ;
    let __VLS_88;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_89 = __VLS_asFunctionalComponent1(__VLS_88, new __VLS_88({
        ...{ 'onClick': {} },
    }));
    const __VLS_90 = __VLS_89({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_89));
    let __VLS_93;
    const __VLS_94 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                    return;
                if (!(__VLS_ctx.selectedProvider))
                    return;
                __VLS_ctx.openEditProvider(__VLS_ctx.selectedProvider);
                // @ts-ignore
                [selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, selectedProvider, getProviderLabel, openEditProvider,];
            } });
    const { default: __VLS_95 } = __VLS_91.slots;
    // @ts-ignore
    [];
    var __VLS_91;
    var __VLS_92;
    let __VLS_96;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_97 = __VLS_asFunctionalComponent1(__VLS_96, new __VLS_96({
        ...{ 'onClick': {} },
        type: "success",
        loading: (__VLS_ctx.providerTesting),
    }));
    const __VLS_98 = __VLS_97({
        ...{ 'onClick': {} },
        type: "success",
        loading: (__VLS_ctx.providerTesting),
    }, ...__VLS_functionalComponentArgsRest(__VLS_97));
    let __VLS_101;
    const __VLS_102 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                    return;
                if (!(__VLS_ctx.selectedProvider))
                    return;
                __VLS_ctx.testProviderById(__VLS_ctx.selectedProvider.id);
                // @ts-ignore
                [selectedProvider, providerTesting, testProviderById,];
            } });
    const { default: __VLS_103 } = __VLS_99.slots;
    let __VLS_104;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_105 = __VLS_asFunctionalComponent1(__VLS_104, new __VLS_104({}));
    const __VLS_106 = __VLS_105({}, ...__VLS_functionalComponentArgsRest(__VLS_105));
    const { default: __VLS_109 } = __VLS_107.slots;
    let __VLS_110;
    /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
    Refresh;
    // @ts-ignore
    const __VLS_111 = __VLS_asFunctionalComponent1(__VLS_110, new __VLS_110({}));
    const __VLS_112 = __VLS_111({}, ...__VLS_functionalComponentArgsRest(__VLS_111));
    // @ts-ignore
    [];
    var __VLS_107;
    // @ts-ignore
    [];
    var __VLS_99;
    var __VLS_100;
    let __VLS_115;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
    }));
    const __VLS_117 = __VLS_116({
        ...{ 'onClick': {} },
        type: "danger",
        plain: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_116));
    let __VLS_120;
    const __VLS_121 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                    return;
                if (!(__VLS_ctx.selectedProvider))
                    return;
                __VLS_ctx.deleteProvider(__VLS_ctx.selectedProvider);
                // @ts-ignore
                [selectedProvider, deleteProvider,];
            } });
    const { default: __VLS_122 } = __VLS_118.slots;
    // @ts-ignore
    [];
    var __VLS_118;
    var __VLS_119;
    if (__VLS_ctx.providerTestResult) {
        let __VLS_123;
        /** @ts-ignore @type { | typeof __VLS_components.elAlert | typeof __VLS_components.ElAlert | typeof __VLS_components['el-alert']} */
        elAlert;
        // @ts-ignore
        const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({
            type: (__VLS_ctx.providerTestResult.ok ? 'success' : 'error'),
            title: (__VLS_ctx.providerTestResult.msg),
            closable: (false),
            ...{ style: {} },
        }));
        const __VLS_125 = __VLS_124({
            type: (__VLS_ctx.providerTestResult.ok ? 'success' : 'error'),
            title: (__VLS_ctx.providerTestResult.msg),
            closable: (false),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_124));
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "section-divider" },
    });
    /** @type {__VLS_StyleScopedClasses['section-divider']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "section-title" },
    });
    /** @type {__VLS_StyleScopedClasses['section-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    let __VLS_128;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_129 = __VLS_asFunctionalComponent1(__VLS_128, new __VLS_128({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        plain: true,
        loading: (__VLS_ctx.probing),
    }));
    const __VLS_130 = __VLS_129({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        plain: true,
        loading: (__VLS_ctx.probing),
    }, ...__VLS_functionalComponentArgsRest(__VLS_129));
    let __VLS_133;
    const __VLS_134 = ({ click: {} },
        { onClick: (__VLS_ctx.fetchModelsForProvider) });
    const { default: __VLS_135 } = __VLS_131.slots;
    let __VLS_136;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_137 = __VLS_asFunctionalComponent1(__VLS_136, new __VLS_136({}));
    const __VLS_138 = __VLS_137({}, ...__VLS_functionalComponentArgsRest(__VLS_137));
    const { default: __VLS_141 } = __VLS_139.slots;
    let __VLS_142;
    /** @ts-ignore @type { | typeof __VLS_components.Search} */
    Search;
    // @ts-ignore
    const __VLS_143 = __VLS_asFunctionalComponent1(__VLS_142, new __VLS_142({}));
    const __VLS_144 = __VLS_143({}, ...__VLS_functionalComponentArgsRest(__VLS_143));
    // @ts-ignore
    [providerTestResult, providerTestResult, providerTestResult, probing, fetchModelsForProvider,];
    var __VLS_139;
    // @ts-ignore
    [];
    var __VLS_131;
    var __VLS_132;
    if (__VLS_ctx.providerModels.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "model-tags" },
        });
        /** @type {__VLS_StyleScopedClasses['model-tags']} */ ;
        for (const [m] of __VLS_vFor((__VLS_ctx.providerModels))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (m.id),
                ...{ class: "model-tag-item" },
            });
            /** @type {__VLS_StyleScopedClasses['model-tag-item']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "model-tag-info" },
            });
            /** @type {__VLS_StyleScopedClasses['model-tag-info']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "model-tag-name" },
            });
            /** @type {__VLS_StyleScopedClasses['model-tag-name']} */ ;
            (m.name);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "model-tag-id" },
            });
            /** @type {__VLS_StyleScopedClasses['model-tag-id']} */ ;
            (m.model);
            if (m.isDefault) {
                let __VLS_147;
                /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
                elTag;
                // @ts-ignore
                const __VLS_148 = __VLS_asFunctionalComponent1(__VLS_147, new __VLS_147({
                    type: "warning",
                    size: "small",
                }));
                const __VLS_149 = __VLS_148({
                    type: "warning",
                    size: "small",
                }, ...__VLS_functionalComponentArgsRest(__VLS_148));
                const { default: __VLS_152 } = __VLS_150.slots;
                // @ts-ignore
                [providerModels, providerModels,];
                var __VLS_150;
            }
            if (m.supportsTools === false) {
                let __VLS_153;
                /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
                elTooltip;
                // @ts-ignore
                const __VLS_154 = __VLS_asFunctionalComponent1(__VLS_153, new __VLS_153({
                    content: "不支持工具调用",
                    placement: "top",
                }));
                const __VLS_155 = __VLS_154({
                    content: "不支持工具调用",
                    placement: "top",
                }, ...__VLS_functionalComponentArgsRest(__VLS_154));
                const { default: __VLS_158 } = __VLS_156.slots;
                let __VLS_159;
                /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
                elTag;
                // @ts-ignore
                const __VLS_160 = __VLS_asFunctionalComponent1(__VLS_159, new __VLS_159({
                    type: "warning",
                    size: "small",
                }));
                const __VLS_161 = __VLS_160({
                    type: "warning",
                    size: "small",
                }, ...__VLS_functionalComponentArgsRest(__VLS_160));
                const { default: __VLS_164 } = __VLS_162.slots;
                // @ts-ignore
                [];
                var __VLS_162;
                // @ts-ignore
                [];
                var __VLS_156;
            }
            let __VLS_165;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_166 = __VLS_asFunctionalComponent1(__VLS_165, new __VLS_165({
                ...{ 'onClick': {} },
                link: true,
                type: "danger",
                size: "small",
            }));
            const __VLS_167 = __VLS_166({
                ...{ 'onClick': {} },
                link: true,
                type: "danger",
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_166));
            let __VLS_170;
            const __VLS_171 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                            return;
                        if (!(__VLS_ctx.selectedProvider))
                            return;
                        if (!(__VLS_ctx.providerModels.length))
                            return;
                        __VLS_ctx.deleteModel(m);
                        // @ts-ignore
                        [deleteModel,];
                    } });
            const { default: __VLS_172 } = __VLS_168.slots;
            // @ts-ignore
            [];
            var __VLS_168;
            var __VLS_169;
            // @ts-ignore
            [];
        }
    }
    else if (!__VLS_ctx.probedModels.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "list-empty" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['list-empty']} */ ;
    }
    if (__VLS_ctx.probedModels.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "probed-header" },
        });
        /** @type {__VLS_StyleScopedClasses['probed-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.probedModels.length);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_173;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_174 = __VLS_asFunctionalComponent1(__VLS_173, new __VLS_173({
            ...{ 'onClick': {} },
            link: true,
            size: "small",
        }));
        const __VLS_175 = __VLS_174({
            ...{ 'onClick': {} },
            link: true,
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_174));
        let __VLS_178;
        const __VLS_179 = ({ click: {} },
            { onClick: (__VLS_ctx.selectAllProbed) });
        const { default: __VLS_180 } = __VLS_176.slots;
        // @ts-ignore
        [probedModels, probedModels, probedModels, selectAllProbed,];
        var __VLS_176;
        var __VLS_177;
        let __VLS_181;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_182 = __VLS_asFunctionalComponent1(__VLS_181, new __VLS_181({
            ...{ 'onClick': {} },
            link: true,
            size: "small",
        }));
        const __VLS_183 = __VLS_182({
            ...{ 'onClick': {} },
            link: true,
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_182));
        let __VLS_186;
        const __VLS_187 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                        return;
                    if (!(__VLS_ctx.selectedProvider))
                        return;
                    if (!(__VLS_ctx.probedModels.length))
                        return;
                    __VLS_ctx.selectedProbed = [];
                    // @ts-ignore
                    [selectedProbed,];
                } });
        const { default: __VLS_188 } = __VLS_184.slots;
        // @ts-ignore
        [];
        var __VLS_184;
        var __VLS_185;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "probed-list" },
        });
        /** @type {__VLS_StyleScopedClasses['probed-list']} */ ;
        for (const [m] of __VLS_vFor((__VLS_ctx.probedModels))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
                key: (m.id),
                ...{ class: "probed-item" },
                ...{ class: ({ added: __VLS_ctx.isModelAdded(m.id), selected: __VLS_ctx.selectedProbed.includes(m.id) }) },
            });
            /** @type {__VLS_StyleScopedClasses['probed-item']} */ ;
            /** @type {__VLS_StyleScopedClasses['added']} */ ;
            /** @type {__VLS_StyleScopedClasses['selected']} */ ;
            let __VLS_189;
            /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
            elCheckbox;
            // @ts-ignore
            const __VLS_190 = __VLS_asFunctionalComponent1(__VLS_189, new __VLS_189({
                ...{ 'onChange': {} },
                modelValue: (__VLS_ctx.selectedProbed.includes(m.id) || __VLS_ctx.isModelAdded(m.id)),
                disabled: (__VLS_ctx.isModelAdded(m.id)),
            }));
            const __VLS_191 = __VLS_190({
                ...{ 'onChange': {} },
                modelValue: (__VLS_ctx.selectedProbed.includes(m.id) || __VLS_ctx.isModelAdded(m.id)),
                disabled: (__VLS_ctx.isModelAdded(m.id)),
            }, ...__VLS_functionalComponentArgsRest(__VLS_190));
            let __VLS_194;
            const __VLS_195 = ({ change: {} },
                { onChange: (...[$event]) => {
                        if (!!(__VLS_ctx.providerForm.mode === 'add' || __VLS_ctx.providerForm.mode === 'edit'))
                            return;
                        if (!(__VLS_ctx.selectedProvider))
                            return;
                        if (!(__VLS_ctx.probedModels.length))
                            return;
                        __VLS_ctx.toggleProbed(m.id);
                        // @ts-ignore
                        [probedModels, selectedProbed, selectedProbed, isModelAdded, isModelAdded, isModelAdded, toggleProbed,];
                    } });
            var __VLS_192;
            var __VLS_193;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "probed-name" },
            });
            /** @type {__VLS_StyleScopedClasses['probed-name']} */ ;
            (m.name && m.name !== m.id ? m.name : m.id);
            if (__VLS_ctx.isModelAdded(m.id)) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "probed-added-tag" },
                });
                /** @type {__VLS_StyleScopedClasses['probed-added-tag']} */ ;
            }
            // @ts-ignore
            [isModelAdded,];
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "probed-actions" },
        });
        /** @type {__VLS_StyleScopedClasses['probed-actions']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.selectedProbed.length);
        let __VLS_196;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_197 = __VLS_asFunctionalComponent1(__VLS_196, new __VLS_196({
            ...{ 'onClick': {} },
            type: "primary",
            disabled: (!__VLS_ctx.selectedProbed.length),
            loading: (__VLS_ctx.saving),
        }));
        const __VLS_198 = __VLS_197({
            ...{ 'onClick': {} },
            type: "primary",
            disabled: (!__VLS_ctx.selectedProbed.length),
            loading: (__VLS_ctx.saving),
        }, ...__VLS_functionalComponentArgsRest(__VLS_197));
        let __VLS_201;
        const __VLS_202 = ({ click: {} },
            { onClick: (__VLS_ctx.batchAddModels) });
        const { default: __VLS_203 } = __VLS_199.slots;
        // @ts-ignore
        [selectedProbed, selectedProbed, saving, batchAddModels,];
        var __VLS_199;
        var __VLS_200;
        if (__VLS_ctx.probeError) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ style: {} },
            });
            (__VLS_ctx.probeError);
        }
    }
    else if (__VLS_ctx.probeError) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.probeError);
    }
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['form-empty']} */ ;
    let __VLS_204;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
        ...{ style: {} },
    }));
    const __VLS_206 = __VLS_205({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_205));
    const { default: __VLS_209 } = __VLS_207.slots;
    let __VLS_210;
    /** @ts-ignore @type { | typeof __VLS_components.Key} */
    Key;
    // @ts-ignore
    const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({}));
    const __VLS_212 = __VLS_211({}, ...__VLS_functionalComponentArgsRest(__VLS_211));
    // @ts-ignore
    [probeError, probeError, probeError, probeError,];
    var __VLS_207;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_215;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_216 = __VLS_asFunctionalComponent1(__VLS_215, new __VLS_215({
        ...{ 'onClick': {} },
        type: "primary",
        ...{ style: {} },
    }));
    const __VLS_217 = __VLS_216({
        ...{ 'onClick': {} },
        type: "primary",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_216));
    let __VLS_220;
    const __VLS_221 = ({ click: {} },
        { onClick: (__VLS_ctx.openAddProvider) });
    const { default: __VLS_222 } = __VLS_218.slots;
    let __VLS_223;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_224 = __VLS_asFunctionalComponent1(__VLS_223, new __VLS_223({}));
    const __VLS_225 = __VLS_224({}, ...__VLS_functionalComponentArgsRest(__VLS_224));
    const { default: __VLS_228 } = __VLS_226.slots;
    let __VLS_229;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_230 = __VLS_asFunctionalComponent1(__VLS_229, new __VLS_229({}));
    const __VLS_231 = __VLS_230({}, ...__VLS_functionalComponentArgsRest(__VLS_230));
    // @ts-ignore
    [openAddProvider,];
    var __VLS_226;
    // @ts-ignore
    [];
    var __VLS_218;
    var __VLS_219;
}
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
