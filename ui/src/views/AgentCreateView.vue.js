/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, reactive } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { ArrowLeft, Plus, Close, InfoFilled } from '@element-plus/icons-vue';
import { agents as agentsApi, files as filesApi, models, channels, tools, skills, memoryConfigApi, agentChannels as agentChannelsApi } from '../api';
import AiChat from '../components/AiChat.vue';
const router = useRouter();
// ── Form state ───────────────────────────────────────────────────────────
const form = reactive({
    name: '',
    id: '',
    description: '',
    avatarColor: '#409eff',
    identity: '',
    soul: '',
    modelId: '',
    channelIds: [],
    toolIds: [],
    skillIds: [],
    agentChannels: [],
});
// ── Per-agent channel add form ────────────────────────────────────────────
const showAddChannel = ref(false);
const newChannelForm = reactive({
    type: 'telegram',
    name: '',
    botToken: '',
    allowedFrom: '',
    webPassword: '',
    webWelcome: '',
});
const newTokenCheck = reactive({ loading: false, status: '', botName: '', error: '' });
function openAddChannelInline() {
    Object.assign(newChannelForm, { type: 'telegram', name: '', botToken: '', allowedFrom: '', webPassword: '', webWelcome: '' });
    Object.assign(newTokenCheck, { loading: false, status: '', botName: '', error: '' });
    showAddChannel.value = true;
}
async function checkNewToken() {
    if (!newChannelForm.botToken)
        return;
    newTokenCheck.loading = true;
    newTokenCheck.status = '';
    try {
        // Use a temp check via the __config__ agent (any agent will do for token validation)
        const tmpId = form.id || '__config__';
        const res = await agentChannelsApi.checkToken(tmpId, newChannelForm.botToken);
        const d = res.data;
        if (d.duplicate) {
            Object.assign(newTokenCheck, { loading: false, status: 'duplicate', botName: '', error: `已被「${d.usedBy}」使用` });
        }
        else if (d.valid) {
            Object.assign(newTokenCheck, { loading: false, status: 'ok', botName: d.botName || '', error: '' });
            if (!newChannelForm.name && d.botName)
                newChannelForm.name = d.botName;
        }
        else {
            Object.assign(newTokenCheck, { loading: false, status: 'error', botName: '', error: d.error || 'Token 无效' });
        }
    }
    catch {
        Object.assign(newTokenCheck, { loading: false, status: 'error', botName: '', error: '验证失败' });
    }
}
function confirmAddChannel() {
    const t = newChannelForm.type;
    const cfg = {};
    if (t === 'telegram') {
        if (!newChannelForm.botToken) {
            ElMessage.warning('请填写 Bot Token');
            return;
        }
        cfg.botToken = newChannelForm.botToken;
        if (newTokenCheck.botName)
            cfg.botName = newTokenCheck.botName;
        if (newChannelForm.allowedFrom)
            cfg.allowedFrom = newChannelForm.allowedFrom;
    }
    else if (t === 'web') {
        if (newChannelForm.webPassword)
            cfg.password = newChannelForm.webPassword;
        if (newChannelForm.webWelcome)
            cfg.welcome = newChannelForm.webWelcome;
    }
    const chId = `${t}-${Date.now()}`;
    form.agentChannels.push({ id: chId, type: t, name: newChannelForm.name || chId, enabled: true, config: cfg });
    showAddChannel.value = false;
    ElMessage.success('已添加，保存 Agent 后生效');
}
function removeChannel(idx) {
    form.agentChannels.splice(idx, 1);
}
// Track which fields were AI-filled (show badge + revert btn)
const aiFilledFields = reactive(new Set());
const aiFilledSnapshot = {};
const saving = ref(false);
const avatarColors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c'];
function autoId() {
    const raw = form.name.toLowerCase()
        .replace(/[^a-z0-9\u4e00-\u9fff\s-]/g, '')
        .trim();
    // 1. 先尝试保留 ASCII 字母数字
    let slug = raw.replace(/[\u4e00-\u9fff]/g, '').replace(/[\s_]+/g, '-').replace(/-+/g, '-').replace(/^-+|-+$/g, '');
    // 2. ASCII 不足时：用中文字符数量生成语义 ID（取每个字对应的拼音首字母近似）
    if (slug.length < 2) {
        // 用名字长度 + 时间戳尾缀做 fallback
        const ts = Date.now().toString(36).slice(-4);
        // 中文名取字符数作为前缀描述
        const zhLen = (raw.match(/[\u4e00-\u9fff]/g) || []).length;
        slug = zhLen > 0 ? `agent-${zhLen}ch-${ts}` : `agent-${ts}`;
    }
    form.id = slug.slice(0, 32);
}
function revertField(field) {
    const key = field;
    form[key] = aiFilledSnapshot[field] || '';
    aiFilledFields.delete(field);
}
function applyToForm(data) {
    // support both lowercase (name/identity/soul) and uppercase (IDENTITY/SOUL)
    const fieldMap = {
        name: 'name', NAME: 'name',
        id: 'id', ID: 'id',
        description: 'description', DESCRIPTION: 'description', desc: 'description',
        identity: 'identity', IDENTITY: 'identity',
        soul: 'soul', SOUL: 'soul',
    };
    let applied = 0;
    for (const [key, val] of Object.entries(data)) {
        const formKey = fieldMap[key];
        if (formKey && val) {
            aiFilledSnapshot[key.toLowerCase()] = form[formKey];
            form[formKey] = val;
            aiFilledFields.add(key.toLowerCase());
            if (key.toLowerCase() === 'name')
                autoId();
            applied++;
        }
    }
    if (applied > 0) {
        ElMessage.success(`已填入 ${applied} 个字段到左侧表单 ✓`);
    }
    else {
        ElMessage.warning('未识别到可填入的字段，请手动复制');
    }
}
async function save() {
    if (!form.name.trim()) {
        ElMessage.warning('请填写名称');
        return;
    }
    if (!form.id.trim() || form.id === '-' || !/^[a-z0-9][a-z0-9-_]{0,30}$/.test(form.id)) {
        ElMessage.warning('ID 格式不对，请手动填写（只能用小写字母、数字、连字符）');
        return;
    }
    if (saving.value)
        return; // 防重复提交
    saving.value = true;
    try {
        // 1. 创建 Agent 基本信息
        await agentsApi.create({
            ...form,
            model: form.modelId || '',
        });
        // 2. 写入 IDENTITY.md / SOUL.md（如果有内容）
        const writes = [];
        if (form.identity.trim()) {
            writes.push(filesApi.write(form.id, 'IDENTITY.md', form.identity));
        }
        if (form.soul.trim()) {
            writes.push(filesApi.write(form.id, 'SOUL.md', form.soul));
        }
        if (writes.length)
            await Promise.all(writes);
        // 3. 保存 per-agent 消息通道
        if (form.agentChannels.length) {
            try {
                await agentChannelsApi.set(form.id, form.agentChannels);
            }
            catch { /* 非致命错误，忽略 */ }
        }
        // 4. 默认开启自动记忆
        try {
            await memoryConfigApi.setConfig(form.id, {
                enabled: true,
                schedule: 'daily',
                keepTurns: 3,
                focusHint: '',
                cronJobId: '',
            });
        }
        catch { /* 非致命错误，忽略 */ }
        ElMessage.success('Agent 创建成功！');
        router.push(`/agents/${form.id}`);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
    finally {
        saving.value = false;
    }
}
// ── Config lists ─────────────────────────────────────────────────────────
const modelList = ref([]);
const modelsLoaded = ref(false);
const channelList = ref([]);
const toolList = ref([]);
const skillList = ref([]);
const allAgentsFull = ref([]);
// ── Right panel: Agent tabs ──────────────────────────────────────────────
const activeAgentTab = ref('__assist__');
const openedAgentIds = ref([]); // agents opened as tabs
const agentList = computed(() => allAgentsFull.value.filter(a => openedAgentIds.value.includes(a.id)));
const allAgents = computed(() => allAgentsFull.value.filter(a => !openedAgentIds.value.includes(a.id)));
// 配置助手固定使用系统内置 agent
const assistAgentId = '__config__';
// 实时将左侧表单状态注入对话上下文
const assistContext = computed(() => {
    const parts = [
        '你是一个 AI 配置助手，帮助用户设计和生成 AI Agent 的配置文件（IDENTITY 和 SOUL）。',
        '用户正在新建一个 Agent，当前表单状态如下（未填字段为空）：',
        `- 名称: ${form.name || '（未填）'}`,
        `- ID: ${form.id || '（未填）'}`,
        `- 描述: ${form.description || '（未填）'}`,
        form.identity ? `- IDENTITY（已填）: ${form.identity.slice(0, 100)}...` : '- IDENTITY: （未填）',
        form.soul ? `- SOUL（已填）: ${form.soul.slice(0, 100)}...` : '- SOUL: （未填）',
        '',
        '当你为用户生成配置时，请在回答末尾附上如下格式的 JSON 块，方便用户一键应用：',
        '```json',
        '{"name":"...","description":"...","identity":"...","soul":"..."}',
        '```',
        '如果某个字段不需要更改，就省略它。',
    ];
    return parts.join('\n');
});
function switchTab(id) {
    activeAgentTab.value = id;
}
function openTab(id) {
    if (!openedAgentIds.value.includes(id))
        openedAgentIds.value.push(id);
    switchTab(id);
}
function closeTab(id) {
    openedAgentIds.value = openedAgentIds.value.filter(x => x !== id);
    if (activeAgentTab.value === id)
        switchTab('__assist__');
}
// ── Init ─────────────────────────────────────────────────────────────────
onMounted(async () => {
    const [ml, cl, tl, sl, al] = await Promise.allSettled([
        models.list(), channels.list(), tools.list(), skills.list(), agentsApi.list()
    ]);
    if (ml.status === 'fulfilled')
        modelList.value = (ml.value.data || []).filter((m) => m.providerStatus !== 'error');
    modelsLoaded.value = true;
    if (cl.status === 'fulfilled')
        channelList.value = cl.value.data;
    if (tl.status === 'fulfilled')
        toolList.value = tl.value.data;
    if (sl.status === 'fulfilled')
        skillList.value = sl.value.data;
    if (al.status === 'fulfilled')
        allAgentsFull.value = al.value.data;
    if (modelList.value.length > 0)
        form.modelId = modelList.value[0]?.id ?? '';
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['ai-filled']} */ ;
/** @type {__VLS_StyleScopedClasses['color-swatch']} */ ;
/** @type {__VLS_StyleScopedClasses['color-swatch']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs-scroll']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
/** @type {__VLS_StyleScopedClasses['tab-close']} */ ;
/** @type {__VLS_StyleScopedClasses['example-chip']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['no-agent-hint']} */ ;
/** @type {__VLS_StyleScopedClasses['no-agent-hint']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "create-layout" },
});
/** @type {__VLS_StyleScopedClasses['create-layout']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "create-left" },
});
/** @type {__VLS_StyleScopedClasses['create-left']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "create-header" },
});
/** @type {__VLS_StyleScopedClasses['create-header']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    text: true,
    ...{ class: "back-btn" },
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    text: true,
    ...{ class: "back-btn" },
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.$router.push('/agents');
            // @ts-ignore
            [$router,];
        } });
/** @type {__VLS_StyleScopedClasses['back-btn']} */ ;
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.ArrowLeft} */
ArrowLeft;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ style: {} },
});
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    model: (__VLS_ctx.form),
    labelPosition: "top",
    ...{ class: "create-form" },
}));
const __VLS_21 = __VLS_20({
    model: (__VLS_ctx.form),
    labelPosition: "top",
    ...{ class: "create-form" },
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
/** @type {__VLS_StyleScopedClasses['create-form']} */ ;
const { default: __VLS_24 } = __VLS_22.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
let __VLS_25;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({
    label: "名称",
    required: true,
}));
const __VLS_27 = __VLS_26({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_26));
const { default: __VLS_30 } = __VLS_28.slots;
let __VLS_31;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_32 = __VLS_asFunctionalComponent1(__VLS_31, new __VLS_31({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如：电商客服助手",
}));
const __VLS_33 = __VLS_32({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如：电商客服助手",
}, ...__VLS_functionalComponentArgsRest(__VLS_32));
let __VLS_36;
const __VLS_37 = ({ input: {} },
    { onInput: (__VLS_ctx.autoId) });
var __VLS_34;
var __VLS_35;
// @ts-ignore
[form, form, autoId,];
var __VLS_28;
let __VLS_38;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({
    label: "ID",
}));
const __VLS_40 = __VLS_39({
    label: "ID",
}, ...__VLS_functionalComponentArgsRest(__VLS_39));
const { default: __VLS_43 } = __VLS_41.slots;
let __VLS_44;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "英文标识（自动生成）",
}));
const __VLS_46 = __VLS_45({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "英文标识（自动生成）",
}, ...__VLS_functionalComponentArgsRest(__VLS_45));
// @ts-ignore
[form,];
var __VLS_41;
let __VLS_49;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
    label: "描述",
}));
const __VLS_51 = __VLS_50({
    label: "描述",
}, ...__VLS_functionalComponentArgsRest(__VLS_50));
const { default: __VLS_54 } = __VLS_52.slots;
let __VLS_55;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_56 = __VLS_asFunctionalComponent1(__VLS_55, new __VLS_55({
    modelValue: (__VLS_ctx.form.description),
    type: "textarea",
    rows: (2),
    placeholder: "简短描述这个 Agent 的职责",
}));
const __VLS_57 = __VLS_56({
    modelValue: (__VLS_ctx.form.description),
    type: "textarea",
    rows: (2),
    placeholder: "简短描述这个 Agent 的职责",
}, ...__VLS_functionalComponentArgsRest(__VLS_56));
// @ts-ignore
[form,];
var __VLS_52;
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    label: "头像颜色",
}));
const __VLS_62 = __VLS_61({
    label: "头像颜色",
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
const { default: __VLS_65 } = __VLS_63.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "color-row" },
});
/** @type {__VLS_StyleScopedClasses['color-row']} */ ;
for (const [color] of __VLS_vFor((__VLS_ctx.avatarColors))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.form.avatarColor = color;
                // @ts-ignore
                [form, avatarColors,];
            } },
        key: (color),
        ...{ class: "color-swatch" },
        ...{ class: ({ active: __VLS_ctx.form.avatarColor === color }) },
        ...{ style: ({ background: color }) },
    });
    /** @type {__VLS_StyleScopedClasses['color-swatch']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    // @ts-ignore
    [form,];
}
// @ts-ignore
[];
var __VLS_63;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
if (__VLS_ctx.aiFilledFields.has('identity') || __VLS_ctx.aiFilledFields.has('soul')) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ai-badge" },
    });
    /** @type {__VLS_StyleScopedClasses['ai-badge']} */ ;
}
let __VLS_66;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_67 = __VLS_asFunctionalComponent1(__VLS_66, new __VLS_66({}));
const __VLS_68 = __VLS_67({}, ...__VLS_functionalComponentArgsRest(__VLS_67));
const { default: __VLS_71 } = __VLS_69.slots;
{
    const { label: __VLS_72 } = __VLS_69.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "field-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['field-hint']} */ ;
    if (__VLS_ctx.aiFilledFields.has('identity')) {
        let __VLS_73;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_74 = __VLS_asFunctionalComponent1(__VLS_73, new __VLS_73({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            ...{ class: "revert-btn" },
        }));
        const __VLS_75 = __VLS_74({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            ...{ class: "revert-btn" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_74));
        let __VLS_78;
        const __VLS_79 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.aiFilledFields.has('identity')))
                        return;
                    __VLS_ctx.revertField('identity');
                    // @ts-ignore
                    [aiFilledFields, aiFilledFields, aiFilledFields, revertField,];
                } });
        /** @type {__VLS_StyleScopedClasses['revert-btn']} */ ;
        const { default: __VLS_80 } = __VLS_76.slots;
        // @ts-ignore
        [];
        var __VLS_76;
        var __VLS_77;
    }
    // @ts-ignore
    [];
}
let __VLS_81;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.identity),
    type: "textarea",
    rows: (5),
    ...{ class: ({ 'ai-filled': __VLS_ctx.aiFilledFields.has('identity') }) },
    placeholder: "你是一个...（描述 Agent 的角色和能力）",
}));
const __VLS_83 = __VLS_82({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.identity),
    type: "textarea",
    rows: (5),
    ...{ class: ({ 'ai-filled': __VLS_ctx.aiFilledFields.has('identity') }) },
    placeholder: "你是一个...（描述 Agent 的角色和能力）",
}, ...__VLS_functionalComponentArgsRest(__VLS_82));
let __VLS_86;
const __VLS_87 = ({ input: {} },
    { onInput: (...[$event]) => {
            __VLS_ctx.aiFilledFields.delete('identity');
            // @ts-ignore
            [form, aiFilledFields, aiFilledFields,];
        } });
/** @type {__VLS_StyleScopedClasses['ai-filled']} */ ;
var __VLS_84;
var __VLS_85;
// @ts-ignore
[];
var __VLS_69;
let __VLS_88;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_89 = __VLS_asFunctionalComponent1(__VLS_88, new __VLS_88({}));
const __VLS_90 = __VLS_89({}, ...__VLS_functionalComponentArgsRest(__VLS_89));
const { default: __VLS_93 } = __VLS_91.slots;
{
    const { label: __VLS_94 } = __VLS_91.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "field-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['field-hint']} */ ;
    if (__VLS_ctx.aiFilledFields.has('soul')) {
        let __VLS_95;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_96 = __VLS_asFunctionalComponent1(__VLS_95, new __VLS_95({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            ...{ class: "revert-btn" },
        }));
        const __VLS_97 = __VLS_96({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            ...{ class: "revert-btn" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_96));
        let __VLS_100;
        const __VLS_101 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.aiFilledFields.has('soul')))
                        return;
                    __VLS_ctx.revertField('soul');
                    // @ts-ignore
                    [aiFilledFields, revertField,];
                } });
        /** @type {__VLS_StyleScopedClasses['revert-btn']} */ ;
        const { default: __VLS_102 } = __VLS_98.slots;
        // @ts-ignore
        [];
        var __VLS_98;
        var __VLS_99;
    }
    // @ts-ignore
    [];
}
let __VLS_103;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_104 = __VLS_asFunctionalComponent1(__VLS_103, new __VLS_103({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.soul),
    type: "textarea",
    rows: (5),
    ...{ class: ({ 'ai-filled': __VLS_ctx.aiFilledFields.has('soul') }) },
    placeholder: "语气亲切，回答简洁...（描述 Agent 的个性风格）",
}));
const __VLS_105 = __VLS_104({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.form.soul),
    type: "textarea",
    rows: (5),
    ...{ class: ({ 'ai-filled': __VLS_ctx.aiFilledFields.has('soul') }) },
    placeholder: "语气亲切，回答简洁...（描述 Agent 的个性风格）",
}, ...__VLS_functionalComponentArgsRest(__VLS_104));
let __VLS_108;
const __VLS_109 = ({ input: {} },
    { onInput: (...[$event]) => {
            __VLS_ctx.aiFilledFields.delete('soul');
            // @ts-ignore
            [form, aiFilledFields, aiFilledFields,];
        } });
/** @type {__VLS_StyleScopedClasses['ai-filled']} */ ;
var __VLS_106;
var __VLS_107;
// @ts-ignore
[];
var __VLS_91;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
let __VLS_110;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_111 = __VLS_asFunctionalComponent1(__VLS_110, new __VLS_110({
    label: "选择模型",
}));
const __VLS_112 = __VLS_111({
    label: "选择模型",
}, ...__VLS_functionalComponentArgsRest(__VLS_111));
const { default: __VLS_115 } = __VLS_113.slots;
let __VLS_116;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_117 = __VLS_asFunctionalComponent1(__VLS_116, new __VLS_116({
    modelValue: (__VLS_ctx.form.modelId),
    placeholder: "选择模型",
    ...{ style: {} },
}));
const __VLS_118 = __VLS_117({
    modelValue: (__VLS_ctx.form.modelId),
    placeholder: "选择模型",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_117));
const { default: __VLS_121 } = __VLS_119.slots;
for (const [m] of __VLS_vFor((__VLS_ctx.modelList))) {
    let __VLS_122;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_123 = __VLS_asFunctionalComponent1(__VLS_122, new __VLS_122({
        key: (m.id),
        label: (`${m.name}（${m.provider}）`),
        value: (m.id),
    }));
    const __VLS_124 = __VLS_123({
        key: (m.id),
        label: (`${m.name}（${m.provider}）`),
        value: (m.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_123));
    // @ts-ignore
    [form, modelList,];
}
// @ts-ignore
[];
var __VLS_119;
// @ts-ignore
[];
var __VLS_113;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
    ...{ style: {} },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
let __VLS_127;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_128 = __VLS_asFunctionalComponent1(__VLS_127, new __VLS_127({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    plain: true,
}));
const __VLS_129 = __VLS_128({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    plain: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_128));
let __VLS_132;
const __VLS_133 = ({ click: {} },
    { onClick: (__VLS_ctx.openAddChannelInline) });
const { default: __VLS_134 } = __VLS_130.slots;
let __VLS_135;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_136 = __VLS_asFunctionalComponent1(__VLS_135, new __VLS_135({}));
const __VLS_137 = __VLS_136({}, ...__VLS_functionalComponentArgsRest(__VLS_136));
const { default: __VLS_140 } = __VLS_138.slots;
let __VLS_141;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_142 = __VLS_asFunctionalComponent1(__VLS_141, new __VLS_141({}));
const __VLS_143 = __VLS_142({}, ...__VLS_functionalComponentArgsRest(__VLS_142));
// @ts-ignore
[openAddChannelInline,];
var __VLS_138;
// @ts-ignore
[];
var __VLS_130;
var __VLS_131;
if (__VLS_ctx.form.agentChannels.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    for (const [ch, idx] of __VLS_vFor((__VLS_ctx.form.agentChannels))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (idx),
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_146;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_147 = __VLS_asFunctionalComponent1(__VLS_146, new __VLS_146({
            size: "small",
        }));
        const __VLS_148 = __VLS_147({
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_147));
        const { default: __VLS_151 } = __VLS_149.slots;
        (ch.type === 'telegram' ? 'Telegram' : 'Web');
        // @ts-ignore
        [form, form,];
        var __VLS_149;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (ch.name || '未命名');
        if (ch.config?.botName) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ style: {} },
            });
            (ch.config.botName);
        }
        let __VLS_152;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_153 = __VLS_asFunctionalComponent1(__VLS_152, new __VLS_152({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            type: "danger",
        }));
        const __VLS_154 = __VLS_153({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            type: "danger",
        }, ...__VLS_functionalComponentArgsRest(__VLS_153));
        let __VLS_157;
        const __VLS_158 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.form.agentChannels.length))
                        return;
                    __VLS_ctx.removeChannel(idx);
                    // @ts-ignore
                    [removeChannel,];
                } });
        const { default: __VLS_159 } = __VLS_155.slots;
        let __VLS_160;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_161 = __VLS_asFunctionalComponent1(__VLS_160, new __VLS_160({}));
        const __VLS_162 = __VLS_161({}, ...__VLS_functionalComponentArgsRest(__VLS_161));
        const { default: __VLS_165 } = __VLS_163.slots;
        let __VLS_166;
        /** @ts-ignore @type { | typeof __VLS_components.Close} */
        Close;
        // @ts-ignore
        const __VLS_167 = __VLS_asFunctionalComponent1(__VLS_166, new __VLS_166({}));
        const __VLS_168 = __VLS_167({}, ...__VLS_functionalComponentArgsRest(__VLS_167));
        // @ts-ignore
        [];
        var __VLS_163;
        // @ts-ignore
        [];
        var __VLS_155;
        var __VLS_156;
        // @ts-ignore
        [];
    }
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-hint']} */ ;
}
if (__VLS_ctx.showAddChannel) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "add-channel-form" },
    });
    /** @type {__VLS_StyleScopedClasses['add-channel-form']} */ ;
    let __VLS_171;
    /** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
    elForm;
    // @ts-ignore
    const __VLS_172 = __VLS_asFunctionalComponent1(__VLS_171, new __VLS_171({
        model: (__VLS_ctx.newChannelForm),
        labelWidth: "90px",
        size: "small",
    }));
    const __VLS_173 = __VLS_172({
        model: (__VLS_ctx.newChannelForm),
        labelWidth: "90px",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_172));
    const { default: __VLS_176 } = __VLS_174.slots;
    let __VLS_177;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_178 = __VLS_asFunctionalComponent1(__VLS_177, new __VLS_177({
        label: "类型",
    }));
    const __VLS_179 = __VLS_178({
        label: "类型",
    }, ...__VLS_functionalComponentArgsRest(__VLS_178));
    const { default: __VLS_182 } = __VLS_180.slots;
    let __VLS_183;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_184 = __VLS_asFunctionalComponent1(__VLS_183, new __VLS_183({
        modelValue: (__VLS_ctx.newChannelForm.type),
        ...{ style: {} },
    }));
    const __VLS_185 = __VLS_184({
        modelValue: (__VLS_ctx.newChannelForm.type),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_184));
    const { default: __VLS_188 } = __VLS_186.slots;
    let __VLS_189;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_190 = __VLS_asFunctionalComponent1(__VLS_189, new __VLS_189({
        label: "Telegram",
        value: "telegram",
    }));
    const __VLS_191 = __VLS_190({
        label: "Telegram",
        value: "telegram",
    }, ...__VLS_functionalComponentArgsRest(__VLS_190));
    let __VLS_194;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_195 = __VLS_asFunctionalComponent1(__VLS_194, new __VLS_194({
        label: "Web 聊天页",
        value: "web",
    }));
    const __VLS_196 = __VLS_195({
        label: "Web 聊天页",
        value: "web",
    }, ...__VLS_functionalComponentArgsRest(__VLS_195));
    // @ts-ignore
    [showAddChannel, newChannelForm, newChannelForm,];
    var __VLS_186;
    // @ts-ignore
    [];
    var __VLS_180;
    let __VLS_199;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_200 = __VLS_asFunctionalComponent1(__VLS_199, new __VLS_199({
        label: "名称",
    }));
    const __VLS_201 = __VLS_200({
        label: "名称",
    }, ...__VLS_functionalComponentArgsRest(__VLS_200));
    const { default: __VLS_204 } = __VLS_202.slots;
    let __VLS_205;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_206 = __VLS_asFunctionalComponent1(__VLS_205, new __VLS_205({
        modelValue: (__VLS_ctx.newChannelForm.name),
        placeholder: "如：客服 Bot",
    }));
    const __VLS_207 = __VLS_206({
        modelValue: (__VLS_ctx.newChannelForm.name),
        placeholder: "如：客服 Bot",
    }, ...__VLS_functionalComponentArgsRest(__VLS_206));
    // @ts-ignore
    [newChannelForm,];
    var __VLS_202;
    if (__VLS_ctx.newChannelForm.type === 'telegram') {
        let __VLS_210;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({
            label: "Bot Token",
        }));
        const __VLS_212 = __VLS_211({
            label: "Bot Token",
        }, ...__VLS_functionalComponentArgsRest(__VLS_211));
        const { default: __VLS_215 } = __VLS_213.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_216;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({
            modelValue: (__VLS_ctx.newChannelForm.botToken),
            type: "password",
            showPassword: true,
            placeholder: "从 @BotFather 获取",
            ...{ style: {} },
            status: (__VLS_ctx.newTokenCheck.status === 'error' ? 'error' : __VLS_ctx.newTokenCheck.status === 'ok' ? 'success' : ''),
        }));
        const __VLS_218 = __VLS_217({
            modelValue: (__VLS_ctx.newChannelForm.botToken),
            type: "password",
            showPassword: true,
            placeholder: "从 @BotFather 获取",
            ...{ style: {} },
            status: (__VLS_ctx.newTokenCheck.status === 'error' ? 'error' : __VLS_ctx.newTokenCheck.status === 'ok' ? 'success' : ''),
        }, ...__VLS_functionalComponentArgsRest(__VLS_217));
        let __VLS_221;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_222 = __VLS_asFunctionalComponent1(__VLS_221, new __VLS_221({
            ...{ 'onClick': {} },
            size: "small",
            loading: (__VLS_ctx.newTokenCheck.loading),
        }));
        const __VLS_223 = __VLS_222({
            ...{ 'onClick': {} },
            size: "small",
            loading: (__VLS_ctx.newTokenCheck.loading),
        }, ...__VLS_functionalComponentArgsRest(__VLS_222));
        let __VLS_226;
        const __VLS_227 = ({ click: {} },
            { onClick: (__VLS_ctx.checkNewToken) });
        const { default: __VLS_228 } = __VLS_224.slots;
        // @ts-ignore
        [newChannelForm, newChannelForm, newTokenCheck, newTokenCheck, newTokenCheck, checkNewToken,];
        var __VLS_224;
        var __VLS_225;
        if (__VLS_ctx.newTokenCheck.status === 'ok') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ style: {} },
            });
            (__VLS_ctx.newTokenCheck.botName);
        }
        else if (__VLS_ctx.newTokenCheck.status === 'error') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ style: {} },
            });
            (__VLS_ctx.newTokenCheck.error);
        }
        // @ts-ignore
        [newTokenCheck, newTokenCheck, newTokenCheck, newTokenCheck,];
        var __VLS_213;
        let __VLS_229;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_230 = __VLS_asFunctionalComponent1(__VLS_229, new __VLS_229({
            label: "白名单 ID",
        }));
        const __VLS_231 = __VLS_230({
            label: "白名单 ID",
        }, ...__VLS_functionalComponentArgsRest(__VLS_230));
        const { default: __VLS_234 } = __VLS_232.slots;
        let __VLS_235;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_236 = __VLS_asFunctionalComponent1(__VLS_235, new __VLS_235({
            modelValue: (__VLS_ctx.newChannelForm.allowedFrom),
            placeholder: "Telegram 用户 ID，多个用逗号分隔（留空=配对模式）",
        }));
        const __VLS_237 = __VLS_236({
            modelValue: (__VLS_ctx.newChannelForm.allowedFrom),
            placeholder: "Telegram 用户 ID，多个用逗号分隔（留空=配对模式）",
        }, ...__VLS_functionalComponentArgsRest(__VLS_236));
        // @ts-ignore
        [newChannelForm,];
        var __VLS_232;
    }
    if (__VLS_ctx.newChannelForm.type === 'web') {
        let __VLS_240;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_241 = __VLS_asFunctionalComponent1(__VLS_240, new __VLS_240({
            label: "访问密码",
        }));
        const __VLS_242 = __VLS_241({
            label: "访问密码",
        }, ...__VLS_functionalComponentArgsRest(__VLS_241));
        const { default: __VLS_245 } = __VLS_243.slots;
        let __VLS_246;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_247 = __VLS_asFunctionalComponent1(__VLS_246, new __VLS_246({
            modelValue: (__VLS_ctx.newChannelForm.webPassword),
            type: "password",
            showPassword: true,
            placeholder: "留空则无需密码",
        }));
        const __VLS_248 = __VLS_247({
            modelValue: (__VLS_ctx.newChannelForm.webPassword),
            type: "password",
            showPassword: true,
            placeholder: "留空则无需密码",
        }, ...__VLS_functionalComponentArgsRest(__VLS_247));
        // @ts-ignore
        [newChannelForm, newChannelForm,];
        var __VLS_243;
        let __VLS_251;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_252 = __VLS_asFunctionalComponent1(__VLS_251, new __VLS_251({
            label: "欢迎语",
        }));
        const __VLS_253 = __VLS_252({
            label: "欢迎语",
        }, ...__VLS_functionalComponentArgsRest(__VLS_252));
        const { default: __VLS_256 } = __VLS_254.slots;
        let __VLS_257;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_258 = __VLS_asFunctionalComponent1(__VLS_257, new __VLS_257({
            modelValue: (__VLS_ctx.newChannelForm.webWelcome),
            placeholder: "你好！有什么可以帮你的？",
        }));
        const __VLS_259 = __VLS_258({
            modelValue: (__VLS_ctx.newChannelForm.webWelcome),
            placeholder: "你好！有什么可以帮你的？",
        }, ...__VLS_functionalComponentArgsRest(__VLS_258));
        // @ts-ignore
        [newChannelForm,];
        var __VLS_254;
    }
    // @ts-ignore
    [];
    var __VLS_174;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_262;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_263 = __VLS_asFunctionalComponent1(__VLS_262, new __VLS_262({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_264 = __VLS_263({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_263));
    let __VLS_267;
    const __VLS_268 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.showAddChannel))
                    return;
                __VLS_ctx.showAddChannel = false;
                // @ts-ignore
                [showAddChannel,];
            } });
    const { default: __VLS_269 } = __VLS_265.slots;
    // @ts-ignore
    [];
    var __VLS_265;
    var __VLS_266;
    let __VLS_270;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_271 = __VLS_asFunctionalComponent1(__VLS_270, new __VLS_270({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
    }));
    const __VLS_272 = __VLS_271({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_271));
    let __VLS_275;
    const __VLS_276 = ({ click: {} },
        { onClick: (__VLS_ctx.confirmAddChannel) });
    const { default: __VLS_277 } = __VLS_273.slots;
    // @ts-ignore
    [confirmAddChannel,];
    var __VLS_273;
    var __VLS_274;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
if (__VLS_ctx.toolList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-hint']} */ ;
    let __VLS_278;
    /** @ts-ignore @type { | typeof __VLS_components.elLink | typeof __VLS_components.ElLink | typeof __VLS_components['el-link'] | typeof __VLS_components.elLink | typeof __VLS_components.ElLink | typeof __VLS_components['el-link']} */
    elLink;
    // @ts-ignore
    const __VLS_279 = __VLS_asFunctionalComponent1(__VLS_278, new __VLS_278({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_280 = __VLS_279({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_279));
    let __VLS_283;
    const __VLS_284 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.toolList.length === 0))
                    return;
                __VLS_ctx.$router.push('/config/tools');
                // @ts-ignore
                [$router, toolList,];
            } });
    const { default: __VLS_285 } = __VLS_281.slots;
    // @ts-ignore
    [];
    var __VLS_281;
    var __VLS_282;
}
else {
    let __VLS_286;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
    elCheckboxGroup;
    // @ts-ignore
    const __VLS_287 = __VLS_asFunctionalComponent1(__VLS_286, new __VLS_286({
        modelValue: (__VLS_ctx.form.toolIds),
    }));
    const __VLS_288 = __VLS_287({
        modelValue: (__VLS_ctx.form.toolIds),
    }, ...__VLS_functionalComponentArgsRest(__VLS_287));
    const { default: __VLS_291 } = __VLS_289.slots;
    for (const [t] of __VLS_vFor((__VLS_ctx.toolList))) {
        let __VLS_292;
        /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox'] | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
        elCheckbox;
        // @ts-ignore
        const __VLS_293 = __VLS_asFunctionalComponent1(__VLS_292, new __VLS_292({
            key: (t.id),
            label: (t.id),
            value: (t.id),
        }));
        const __VLS_294 = __VLS_293({
            key: (t.id),
            label: (t.id),
            value: (t.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_293));
        const { default: __VLS_297 } = __VLS_295.slots;
        (t.name);
        // @ts-ignore
        [form, toolList,];
        var __VLS_295;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_289;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "form-section" },
});
/** @type {__VLS_StyleScopedClasses['form-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "section-title" },
});
/** @type {__VLS_StyleScopedClasses['section-title']} */ ;
if (__VLS_ctx.skillList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-hint']} */ ;
}
else {
    let __VLS_298;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
    elCheckboxGroup;
    // @ts-ignore
    const __VLS_299 = __VLS_asFunctionalComponent1(__VLS_298, new __VLS_298({
        modelValue: (__VLS_ctx.form.skillIds),
    }));
    const __VLS_300 = __VLS_299({
        modelValue: (__VLS_ctx.form.skillIds),
    }, ...__VLS_functionalComponentArgsRest(__VLS_299));
    const { default: __VLS_303 } = __VLS_301.slots;
    for (const [s] of __VLS_vFor((__VLS_ctx.skillList))) {
        let __VLS_304;
        /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox'] | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
        elCheckbox;
        // @ts-ignore
        const __VLS_305 = __VLS_asFunctionalComponent1(__VLS_304, new __VLS_304({
            key: (s.id),
            label: (s.id),
            value: (s.id),
        }));
        const __VLS_306 = __VLS_305({
            key: (s.id),
            label: (s.id),
            value: (s.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_305));
        const { default: __VLS_309 } = __VLS_307.slots;
        (s.name);
        let __VLS_310;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_311 = __VLS_asFunctionalComponent1(__VLS_310, new __VLS_310({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_312 = __VLS_311({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_311));
        const { default: __VLS_315 } = __VLS_313.slots;
        (s.version);
        // @ts-ignore
        [form, skillList, skillList,];
        var __VLS_313;
        // @ts-ignore
        [];
        var __VLS_307;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_301;
}
// @ts-ignore
[];
var __VLS_22;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "create-footer" },
});
/** @type {__VLS_StyleScopedClasses['create-footer']} */ ;
let __VLS_316;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_317 = __VLS_asFunctionalComponent1(__VLS_316, new __VLS_316({
    ...{ 'onClick': {} },
}));
const __VLS_318 = __VLS_317({
    ...{ 'onClick': {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_317));
let __VLS_321;
const __VLS_322 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.$router.push('/agents');
            // @ts-ignore
            [$router,];
        } });
const { default: __VLS_323 } = __VLS_319.slots;
// @ts-ignore
[];
var __VLS_319;
var __VLS_320;
let __VLS_324;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_325 = __VLS_asFunctionalComponent1(__VLS_324, new __VLS_324({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.saving),
}));
const __VLS_326 = __VLS_325({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.saving),
}, ...__VLS_functionalComponentArgsRest(__VLS_325));
let __VLS_329;
const __VLS_330 = ({ click: {} },
    { onClick: (__VLS_ctx.save) });
const { default: __VLS_331 } = __VLS_327.slots;
// @ts-ignore
[saving, save,];
var __VLS_327;
var __VLS_328;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "create-right" },
});
/** @type {__VLS_StyleScopedClasses['create-right']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "agent-tabs-bar" },
});
/** @type {__VLS_StyleScopedClasses['agent-tabs-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "agent-tabs-scroll" },
});
/** @type {__VLS_StyleScopedClasses['agent-tabs-scroll']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.switchTab('__assist__');
            // @ts-ignore
            [switchTab,];
        } },
    ...{ class: "agent-tab" },
    ...{ class: ({ active: __VLS_ctx.activeAgentTab === '__assist__' }) },
});
/** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
let __VLS_332;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_333 = __VLS_asFunctionalComponent1(__VLS_332, new __VLS_332({
    ...{ class: "tab-icon" },
}));
const __VLS_334 = __VLS_333({
    ...{ class: "tab-icon" },
}, ...__VLS_functionalComponentArgsRest(__VLS_333));
/** @type {__VLS_StyleScopedClasses['tab-icon']} */ ;
const { default: __VLS_337 } = __VLS_335.slots;
let __VLS_338;
/** @ts-ignore @type { | typeof __VLS_components.User} */
User;
// @ts-ignore
const __VLS_339 = __VLS_asFunctionalComponent1(__VLS_338, new __VLS_338({}));
const __VLS_340 = __VLS_339({}, ...__VLS_functionalComponentArgsRest(__VLS_339));
// @ts-ignore
[activeAgentTab,];
var __VLS_335;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.switchTab(ag.id);
                // @ts-ignore
                [switchTab, agentList,];
            } },
        key: (ag.id),
        ...{ class: "agent-tab" },
        ...{ class: ({ active: __VLS_ctx.activeAgentTab === ag.id }) },
    });
    /** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tab-avatar" },
        ...{ style: ({ background: ag.avatarColor || '#409eff' }) },
    });
    /** @type {__VLS_StyleScopedClasses['tab-avatar']} */ ;
    (ag.name.charAt(0));
    (ag.name);
    let __VLS_343;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_344 = __VLS_asFunctionalComponent1(__VLS_343, new __VLS_343({
        ...{ 'onClick': {} },
        ...{ class: "tab-close" },
    }));
    const __VLS_345 = __VLS_344({
        ...{ 'onClick': {} },
        ...{ class: "tab-close" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_344));
    let __VLS_348;
    const __VLS_349 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.closeTab(ag.id);
                // @ts-ignore
                [activeAgentTab, closeTab,];
            } });
    /** @type {__VLS_StyleScopedClasses['tab-close']} */ ;
    const { default: __VLS_350 } = __VLS_346.slots;
    let __VLS_351;
    /** @ts-ignore @type { | typeof __VLS_components.Close} */
    Close;
    // @ts-ignore
    const __VLS_352 = __VLS_asFunctionalComponent1(__VLS_351, new __VLS_351({}));
    const __VLS_353 = __VLS_352({}, ...__VLS_functionalComponentArgsRest(__VLS_352));
    // @ts-ignore
    [];
    var __VLS_346;
    var __VLS_347;
    // @ts-ignore
    [];
}
let __VLS_356;
/** @ts-ignore @type { | typeof __VLS_components.elDropdown | typeof __VLS_components.ElDropdown | typeof __VLS_components['el-dropdown'] | typeof __VLS_components.elDropdown | typeof __VLS_components.ElDropdown | typeof __VLS_components['el-dropdown']} */
elDropdown;
// @ts-ignore
const __VLS_357 = __VLS_asFunctionalComponent1(__VLS_356, new __VLS_356({
    ...{ 'onCommand': {} },
    trigger: "click",
}));
const __VLS_358 = __VLS_357({
    ...{ 'onCommand': {} },
    trigger: "click",
}, ...__VLS_functionalComponentArgsRest(__VLS_357));
let __VLS_361;
const __VLS_362 = ({ command: {} },
    { onCommand: (__VLS_ctx.openTab) });
const { default: __VLS_363 } = __VLS_359.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "agent-tab add-tab" },
});
/** @type {__VLS_StyleScopedClasses['agent-tab']} */ ;
/** @type {__VLS_StyleScopedClasses['add-tab']} */ ;
let __VLS_364;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_365 = __VLS_asFunctionalComponent1(__VLS_364, new __VLS_364({}));
const __VLS_366 = __VLS_365({}, ...__VLS_functionalComponentArgsRest(__VLS_365));
const { default: __VLS_369 } = __VLS_367.slots;
let __VLS_370;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_371 = __VLS_asFunctionalComponent1(__VLS_370, new __VLS_370({}));
const __VLS_372 = __VLS_371({}, ...__VLS_functionalComponentArgsRest(__VLS_371));
// @ts-ignore
[openTab,];
var __VLS_367;
{
    const { dropdown: __VLS_375 } = __VLS_359.slots;
    let __VLS_376;
    /** @ts-ignore @type { | typeof __VLS_components.elDropdownMenu | typeof __VLS_components.ElDropdownMenu | typeof __VLS_components['el-dropdown-menu'] | typeof __VLS_components.elDropdownMenu | typeof __VLS_components.ElDropdownMenu | typeof __VLS_components['el-dropdown-menu']} */
    elDropdownMenu;
    // @ts-ignore
    const __VLS_377 = __VLS_asFunctionalComponent1(__VLS_376, new __VLS_376({}));
    const __VLS_378 = __VLS_377({}, ...__VLS_functionalComponentArgsRest(__VLS_377));
    const { default: __VLS_381 } = __VLS_379.slots;
    for (const [ag] of __VLS_vFor((__VLS_ctx.allAgents))) {
        let __VLS_382;
        /** @ts-ignore @type { | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item'] | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item']} */
        elDropdownItem;
        // @ts-ignore
        const __VLS_383 = __VLS_asFunctionalComponent1(__VLS_382, new __VLS_382({
            key: (ag.id),
            command: (ag.id),
        }));
        const __VLS_384 = __VLS_383({
            key: (ag.id),
            command: (ag.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_383));
        const { default: __VLS_387 } = __VLS_385.slots;
        (ag.name);
        // @ts-ignore
        [allAgents,];
        var __VLS_385;
        // @ts-ignore
        [];
    }
    if (__VLS_ctx.allAgents.length === 0) {
        let __VLS_388;
        /** @ts-ignore @type { | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item'] | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item']} */
        elDropdownItem;
        // @ts-ignore
        const __VLS_389 = __VLS_asFunctionalComponent1(__VLS_388, new __VLS_388({
            disabled: true,
        }));
        const __VLS_390 = __VLS_389({
            disabled: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_389));
        const { default: __VLS_393 } = __VLS_391.slots;
        // @ts-ignore
        [allAgents,];
        var __VLS_391;
    }
    // @ts-ignore
    [];
    var __VLS_379;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_359;
var __VLS_360;
if (__VLS_ctx.activeAgentTab === '__assist__') {
    if (__VLS_ctx.assistAgentId) {
        const __VLS_394 = AiChat;
        // @ts-ignore
        const __VLS_395 = __VLS_asFunctionalComponent1(__VLS_394, new __VLS_394({
            ...{ 'onApply': {} },
            agentId: (__VLS_ctx.assistAgentId),
            context: (__VLS_ctx.assistContext),
            scenario: "agent-creation",
            placeholder: "告诉我这个 Agent 要做什么...",
            examples: ([
                '我需要一个电商客服 Agent，负责解答订单问题，语气亲切',
                '帮我创建一个代码审查助手，专注于 Python 代码规范',
                '创建一个每天早上发送天气报告的 Agent',
            ]),
            height: "100%",
            compact: (true),
            showThinking: (true),
            applyable: (true),
            noModel: (__VLS_ctx.modelsLoaded && __VLS_ctx.modelList.length === 0),
        }));
        const __VLS_396 = __VLS_395({
            ...{ 'onApply': {} },
            agentId: (__VLS_ctx.assistAgentId),
            context: (__VLS_ctx.assistContext),
            scenario: "agent-creation",
            placeholder: "告诉我这个 Agent 要做什么...",
            examples: ([
                '我需要一个电商客服 Agent，负责解答订单问题，语气亲切',
                '帮我创建一个代码审查助手，专注于 Python 代码规范',
                '创建一个每天早上发送天气报告的 Agent',
            ]),
            height: "100%",
            compact: (true),
            showThinking: (true),
            applyable: (true),
            noModel: (__VLS_ctx.modelsLoaded && __VLS_ctx.modelList.length === 0),
        }, ...__VLS_functionalComponentArgsRest(__VLS_395));
        let __VLS_399;
        const __VLS_400 = ({ apply: {} },
            { onApply: (__VLS_ctx.applyToForm) });
        var __VLS_397;
        var __VLS_398;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "no-agent-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['no-agent-hint']} */ ;
        let __VLS_401;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_402 = __VLS_asFunctionalComponent1(__VLS_401, new __VLS_401({
            size: "32",
        }));
        const __VLS_403 = __VLS_402({
            size: "32",
        }, ...__VLS_functionalComponentArgsRest(__VLS_402));
        const { default: __VLS_406 } = __VLS_404.slots;
        let __VLS_407;
        /** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
        InfoFilled;
        // @ts-ignore
        const __VLS_408 = __VLS_asFunctionalComponent1(__VLS_407, new __VLS_407({}));
        const __VLS_409 = __VLS_408({}, ...__VLS_functionalComponentArgsRest(__VLS_408));
        // @ts-ignore
        [modelList, activeAgentTab, assistAgentId, assistAgentId, assistContext, modelsLoaded, applyToForm,];
        var __VLS_404;
        __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
        __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
            ...{ class: "hint-sub" },
        });
        /** @type {__VLS_StyleScopedClasses['hint-sub']} */ ;
    }
}
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    const __VLS_412 = AiChat;
    // @ts-ignore
    const __VLS_413 = __VLS_asFunctionalComponent1(__VLS_412, new __VLS_412({
        agentId: (ag.id),
        scenario: "general",
        welcomeMessage: (`你好，我是 **${ag.name}**，有什么需要帮忙的？`),
        height: "100%",
        compact: (true),
        showThinking: (true),
        key: (ag.id),
    }));
    const __VLS_414 = __VLS_413({
        agentId: (ag.id),
        scenario: "general",
        welcomeMessage: (`你好，我是 **${ag.name}**，有什么需要帮忙的？`),
        height: "100%",
        compact: (true),
        showThinking: (true),
        key: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_413));
    __VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.activeAgentTab === ag.id) }, null, null);
    // @ts-ignore
    [activeAgentTab, agentList,];
}
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
