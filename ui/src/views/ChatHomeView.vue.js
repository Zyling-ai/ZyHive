/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted } from 'vue';
import { ElNotification } from 'element-plus';
import { agents as agentsApi, models as modelsApi, sessions as sessApi } from '../api';
import AiChat from '../components/AiChat.vue';
const aiChatRef = ref();
const emit = defineEmits();
const agents = ref([]);
const allModels = ref([]);
const currentAgentId = ref('');
const currentModelId = ref('');
const currentSessionId = ref('');
const chatKey = ref(0);
const sessions = ref([]);
const currentAgent = computed(() => agents.value.find(a => a.id === currentAgentId.value));
const dispatched = ref([]);
// ── 初始化 ────────────────────────────────────────────────────────────────
onMounted(async () => {
    await Promise.all([loadAgents(), loadModels()]);
});
async function loadAgents() {
    try {
        const res = await agentsApi.list();
        agents.value = res.data.filter((a) => !a.system);
        const saved = localStorage.getItem('chat_home_agent');
        if (saved && agents.value.find(a => a.id === saved)) {
            currentAgentId.value = saved;
        }
        else if (agents.value.length > 0) {
            currentAgentId.value = agents.value[0]?.id ?? '';
        }
        if (currentAgentId.value) {
            syncModel();
            await loadSessions(currentAgentId.value);
        }
    }
    catch { }
}
async function loadModels() {
    try {
        const res = await modelsApi.list();
        // 过滤掉 provider API Key 已测失败的模型（避免用户选了又报错）
        allModels.value = (res.data || []).filter((m) => m.providerStatus !== 'error');
        syncModel();
    }
    catch { }
}
function syncModel() {
    const ag = currentAgent.value;
    const saved = localStorage.getItem('chat_home_model');
    if (saved && allModels.value.find(m => m.id === saved)) {
        currentModelId.value = saved;
    }
    else if (ag?.modelId) {
        currentModelId.value = ag.modelId;
    }
    else {
        const def = allModels.value.find(m => m.isDefault) || allModels.value[0];
        if (def)
            currentModelId.value = def.id;
    }
}
// 渠道来源 → 友好标签（用于会话下拉选项）
function channelLabel(source) {
    const map = { feishu: '飞书', telegram: 'TG', web: 'Web', panel: '面板' };
    return map[source] || '面板';
}
function inferSessionSource(raw, id) {
    const s = (raw || '').toLowerCase();
    if (s === 'feishu' || s === 'telegram' || s === 'web')
        return s;
    if (id.startsWith('feishu-'))
        return 'feishu';
    if (id.startsWith('tg-'))
        return 'telegram';
    if (id.startsWith('web-'))
        return 'web';
    return 'panel';
}
async function loadSessions(agentId) {
    try {
        const res = await sessApi.list({ agentId, limit: 30 });
        sessions.value = (res.data.sessions || [])
            .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
            .map(s => {
            const src = inferSessionSource(s.source, s.id);
            const preview = (s.title && s.title.trim()) || (src === 'feishu' ? '飞书 · ' + s.id.slice(7, 15) : src === 'telegram' ? 'TG · ' + s.id.slice(3, 11) : '对话');
            return {
                key: s.id,
                preview: preview.slice(0, 40),
                createdAt: s.createdAt || 0,
                lastAt: s.lastAt || s.createdAt || 0,
                messageCount: s.messageCount || 0,
                channel: channelLabel(src),
            };
        });
    }
    catch { }
}
async function onAgentChange(id) {
    localStorage.setItem('chat_home_agent', id);
    currentSessionId.value = '';
    chatKey.value++;
    syncModel();
    await loadSessions(id);
}
async function onModelChange(id) {
    localStorage.setItem('chat_home_model', id);
    if (currentAgentId.value) {
        try {
            await agentsApi.update(currentAgentId.value, { modelId: id });
        }
        catch { }
    }
}
function onSessionChange(key) {
    currentSessionId.value = key;
    chatKey.value++;
}
function onSessionCreated(key) {
    currentSessionId.value = key;
    if (!sessions.value.find(s => s.key === key)) {
        sessions.value.unshift({ key, preview: '新对话', createdAt: Date.now(), lastAt: Date.now(), messageCount: 0, channel: 'web' });
    }
}
function newChat() {
    currentSessionId.value = '';
    chatKey.value++;
}
// ── 任务已被 LLM 内部处理（agent_result 调用成功）──────────────────────────
function onTaskHandled(taskId) {
    const task = dispatched.value.find(d => d.taskId === taskId);
    if (task)
        task.handled = true;
}
// ── 派遣 ──────────────────────────────────────────────────────────────────
function onDispatch(agentId, agentName, avatarColor, taskId) {
    const agInfo = agents.value.find(a => a.id === agentId);
    const color = agInfo?.avatarColor || avatarColor || '#6366f1';
    if (dispatched.value.find(d => d.taskId === taskId))
        return;
    const task = {
        taskId, agentId, agentName: agInfo?.name || agentName, avatarColor: color, status: 'running', latestReport: '',
    };
    dispatched.value.push(task);
    pollTask(task);
}
function pollTask(task) {
    const token = localStorage.getItem('aipanel_token') || '';
    const base = (localStorage.getItem('aipanel_url') || '').replace(/\/$/, '');
    let tries = 0;
    const tick = async () => {
        if (tries++ > 60) {
            task.status = 'error';
            return;
        }
        try {
            const r = await fetch(`${base}/api/tasks/${task.taskId}`, { headers: { Authorization: `Bearer ${token}` } });
            if (r.ok) {
                const d = await r.json();
                if (d.output)
                    task.latestReport = d.output.slice(-80);
                if (d.status === 'done') {
                    task.status = 'done';
                    const output = (d.output || '').trim();
                    const label = d.label || task.agentName;
                    // 如果 LLM 已经通过 agent_result 主动处理了结果，跳过重复汇报
                    if (!task.handled) {
                        if (aiChatRef.value?.continueAfterSpawn) {
                            aiChatRef.value.continueAfterSpawn(task.agentName, label, output);
                        }
                        else {
                            ElNotification({ title: `✅ ${task.agentName} 完成了任务`, message: output.slice(0, 120) || '已完成', type: 'success', duration: 8000, position: 'bottom-right' });
                        }
                    }
                    setTimeout(() => { dispatched.value = dispatched.value.filter(x => x.taskId !== task.taskId); }, 4000);
                    return;
                }
                if (d.status === 'error') {
                    task.status = 'error';
                    if (aiChatRef.value?.continueAfterSpawn) {
                        aiChatRef.value.appendMessage?.({ role: 'system', text: `❌ ${task.agentName} 任务执行失败：${d.error || '未知错误'}` });
                    }
                    else {
                        ElNotification({ title: `❌ ${task.agentName} 任务失败`, message: d.error || '执行出错', type: 'error', duration: 8000, position: 'bottom-right' });
                    }
                    setTimeout(() => { dispatched.value = dispatched.value.filter(x => x.taskId !== task.taskId); }, 6000);
                    return;
                }
            }
        }
        catch { }
        setTimeout(tick, 3000);
    };
    setTimeout(tick, 2000);
}
function statusText(s) {
    return { running: '执行中...', done: '已完成', error: '失败' }[s] || s;
}
function fmtTime(ts) {
    if (!ts)
        return '-';
    const d = new Date(ts);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    if (diff < 60000)
        return '刚刚';
    if (diff < 3600000)
        return Math.floor(diff / 60000) + ' 分钟前';
    if (diff < 86400000)
        return Math.floor(diff / 3600000) + ' 小时前';
    const m = d.getMonth() + 1, day = d.getDate();
    const h = String(d.getHours()).padStart(2, '0'), min = String(d.getMinutes()).padStart(2, '0');
    if (d.getFullYear() === now.getFullYear())
        return `${m}/${day} ${h}:${min}`;
    return `${d.getFullYear()}/${m}/${day}`;
}
const __VLS_ctx = {
    ...{},
    ...{},
    ...{},
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['sidebar-toggle-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['opt-avatar']} */ ;
/** @type {__VLS_StyleScopedClasses['new-chat-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['dz-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['dz-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['empty-hint']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-home" },
});
/** @type {__VLS_StyleScopedClasses['chat-home']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-toolbar" },
});
/** @type {__VLS_StyleScopedClasses['chat-toolbar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.emit('toggle-sidebar');
            // @ts-ignore
            [emit,];
        } },
    ...{ class: "sidebar-toggle-btn" },
    title: "展开/收起侧栏",
});
/** @type {__VLS_StyleScopedClasses['sidebar-toggle-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "16",
    height: "16",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    'stroke-width': "2",
    'stroke-linecap': "round",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.rect)({
    x: "3",
    y: "3",
    width: "18",
    height: "18",
    rx: "2",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.line)({
    x1: "9",
    y1: "3",
    x2: "9",
    y2: "21",
});
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentAgentId),
    size: "small",
    ...{ class: "agent-select" },
}));
const __VLS_2 = __VLS_1({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentAgentId),
    size: "small",
    ...{ class: "agent-select" },
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ change: {} },
    { onChange: (__VLS_ctx.onAgentChange) });
/** @type {__VLS_StyleScopedClasses['agent-select']} */ ;
const { default: __VLS_7 } = __VLS_3.slots;
{
    const { prefix: __VLS_8 } = __VLS_3.slots;
    if (__VLS_ctx.currentAgent) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "sel-avatar" },
            ...{ style: ({ background: __VLS_ctx.currentAgent.avatarColor || '#6366f1' }) },
        });
        /** @type {__VLS_StyleScopedClasses['sel-avatar']} */ ;
        ((__VLS_ctx.currentAgent.name || '?')[0]);
    }
    // @ts-ignore
    [currentAgentId, onAgentChange, currentAgent, currentAgent, currentAgent,];
}
for (const [ag] of __VLS_vFor((__VLS_ctx.agents))) {
    let __VLS_9;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_10 = __VLS_asFunctionalComponent1(__VLS_9, new __VLS_9({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_11 = __VLS_10({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_10));
    const { default: __VLS_14 } = __VLS_12.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "opt-avatar" },
        ...{ style: ({ background: ag.avatarColor || '#6366f1' }) },
    });
    /** @type {__VLS_StyleScopedClasses['opt-avatar']} */ ;
    ((ag.name || '?')[0]);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (ag.name);
    // @ts-ignore
    [agents,];
    var __VLS_12;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
let __VLS_15;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_16 = __VLS_asFunctionalComponent1(__VLS_15, new __VLS_15({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentModelId),
    size: "small",
    ...{ class: "model-select" },
    placeholder: "选择模型",
}));
const __VLS_17 = __VLS_16({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentModelId),
    size: "small",
    ...{ class: "model-select" },
    placeholder: "选择模型",
}, ...__VLS_functionalComponentArgsRest(__VLS_16));
let __VLS_20;
const __VLS_21 = ({ change: {} },
    { onChange: (__VLS_ctx.onModelChange) });
/** @type {__VLS_StyleScopedClasses['model-select']} */ ;
const { default: __VLS_22 } = __VLS_18.slots;
for (const [m] of __VLS_vFor((__VLS_ctx.allModels))) {
    let __VLS_23;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_24 = __VLS_asFunctionalComponent1(__VLS_23, new __VLS_23({
        key: (m.id),
        label: (m.name || m.model),
        value: (m.id),
    }));
    const __VLS_25 = __VLS_24({
        key: (m.id),
        label: (m.name || m.model),
        value: (m.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_24));
    // @ts-ignore
    [currentModelId, onModelChange, allModels,];
}
// @ts-ignore
[];
var __VLS_18;
var __VLS_19;
let __VLS_28;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_29 = __VLS_asFunctionalComponent1(__VLS_28, new __VLS_28({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentSessionId),
    size: "small",
    ...{ class: "session-select" },
    placeholder: "新对话",
    clearable: true,
    popperClass: "session-popper",
}));
const __VLS_30 = __VLS_29({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentSessionId),
    size: "small",
    ...{ class: "session-select" },
    placeholder: "新对话",
    clearable: true,
    popperClass: "session-popper",
}, ...__VLS_functionalComponentArgsRest(__VLS_29));
let __VLS_33;
const __VLS_34 = ({ change: {} },
    { onChange: (__VLS_ctx.onSessionChange) });
/** @type {__VLS_StyleScopedClasses['session-select']} */ ;
const { default: __VLS_35 } = __VLS_31.slots;
let __VLS_36;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_37 = __VLS_asFunctionalComponent1(__VLS_36, new __VLS_36({
    value: "",
    label: "＋ 新对话",
}));
const __VLS_38 = __VLS_37({
    value: "",
    label: "＋ 新对话",
}, ...__VLS_functionalComponentArgsRest(__VLS_37));
const { default: __VLS_41 } = __VLS_39.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sess-new-opt" },
});
/** @type {__VLS_StyleScopedClasses['sess-new-opt']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "13",
    height: "13",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    'stroke-width': "2.5",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.line)({
    x1: "12",
    y1: "5",
    x2: "12",
    y2: "19",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.line)({
    x1: "5",
    y1: "12",
    x2: "19",
    y2: "12",
});
// @ts-ignore
[currentSessionId, onSessionChange,];
var __VLS_39;
for (const [s] of __VLS_vFor((__VLS_ctx.sessions))) {
    let __VLS_42;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_43 = __VLS_asFunctionalComponent1(__VLS_42, new __VLS_42({
        key: (s.key),
        label: (s.preview),
        value: (s.key),
    }));
    const __VLS_44 = __VLS_43({
        key: (s.key),
        label: (s.preview),
        value: (s.key),
    }, ...__VLS_functionalComponentArgsRest(__VLS_43));
    const { default: __VLS_47 } = __VLS_45.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sess-opt" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-opt']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sess-opt-title" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-opt-title']} */ ;
    (s.preview);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sess-opt-meta" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-opt-meta']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sess-ch" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-ch']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "11",
        height: "11",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z",
    });
    (s.channel);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sess-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sess-time" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-time']} */ ;
    (__VLS_ctx.fmtTime(s.lastAt));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sess-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sess-count" },
    });
    /** @type {__VLS_StyleScopedClasses['sess-count']} */ ;
    (s.messageCount);
    // @ts-ignore
    [sessions, fmtTime,];
    var __VLS_45;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_31;
var __VLS_32;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "toolbar-flex" },
});
/** @type {__VLS_StyleScopedClasses['toolbar-flex']} */ ;
let __VLS_48;
/** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
Transition;
// @ts-ignore
const __VLS_49 = __VLS_asFunctionalComponent1(__VLS_48, new __VLS_48({
    name: "zone-fade",
}));
const __VLS_50 = __VLS_49({
    name: "zone-fade",
}, ...__VLS_functionalComponentArgsRest(__VLS_49));
const { default: __VLS_53 } = __VLS_51.slots;
if (__VLS_ctx.dispatched.length > 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dispatch-zone" },
    });
    /** @type {__VLS_StyleScopedClasses['dispatch-zone']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dz-label" },
    });
    /** @type {__VLS_StyleScopedClasses['dz-label']} */ ;
    let __VLS_54;
    /** @ts-ignore @type { | typeof __VLS_components.TransitionGroup | typeof __VLS_components.TransitionGroup} */
    TransitionGroup;
    // @ts-ignore
    const __VLS_55 = __VLS_asFunctionalComponent1(__VLS_54, new __VLS_54({
        name: "avatar-fly",
        tag: "div",
        ...{ class: "dz-avatars" },
    }));
    const __VLS_56 = __VLS_55({
        name: "avatar-fly",
        tag: "div",
        ...{ class: "dz-avatars" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_55));
    /** @type {__VLS_StyleScopedClasses['dz-avatars']} */ ;
    const { default: __VLS_59 } = __VLS_57.slots;
    for (const [d] of __VLS_vFor((__VLS_ctx.dispatched))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (d.taskId),
            ...{ class: "dz-wrap" },
        });
        /** @type {__VLS_StyleScopedClasses['dz-wrap']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dz-avatar" },
            ...{ style: ({ background: d.avatarColor }) },
        });
        /** @type {__VLS_StyleScopedClasses['dz-avatar']} */ ;
        ((d.agentName || '?')[0]);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
            ...{ class: "dz-dot" },
            ...{ class: ('dot-' + d.status) },
        });
        /** @type {__VLS_StyleScopedClasses['dz-dot']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dz-bubble" },
        });
        /** @type {__VLS_StyleScopedClasses['dz-bubble']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dz-bname" },
        });
        /** @type {__VLS_StyleScopedClasses['dz-bname']} */ ;
        (d.agentName);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dz-bstatus" },
        });
        /** @type {__VLS_StyleScopedClasses['dz-bstatus']} */ ;
        (__VLS_ctx.statusText(d.status));
        if (d.latestReport) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "dz-breport" },
            });
            /** @type {__VLS_StyleScopedClasses['dz-breport']} */ ;
            (d.latestReport);
        }
        // @ts-ignore
        [dispatched, dispatched, statusText,];
    }
    // @ts-ignore
    [];
    var __VLS_57;
}
// @ts-ignore
[];
var __VLS_51;
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    ...{ 'onClick': {} },
    size: "small",
    ...{ class: "new-chat-btn" },
}));
const __VLS_62 = __VLS_61({
    ...{ 'onClick': {} },
    size: "small",
    ...{ class: "new-chat-btn" },
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
let __VLS_65;
const __VLS_66 = ({ click: {} },
    { onClick: (__VLS_ctx.newChat) });
/** @type {__VLS_StyleScopedClasses['new-chat-btn']} */ ;
const { default: __VLS_67 } = __VLS_63.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "14",
    height: "14",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    'stroke-width': "2.5",
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.line)({
    x1: "12",
    y1: "5",
    x2: "12",
    y2: "19",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.line)({
    x1: "5",
    y1: "12",
    x2: "19",
    y2: "12",
});
// @ts-ignore
[newChat,];
var __VLS_63;
var __VLS_64;
if (__VLS_ctx.currentAgentId) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-body" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-body']} */ ;
    const __VLS_68 = AiChat;
    // @ts-ignore
    const __VLS_69 = __VLS_asFunctionalComponent1(__VLS_68, new __VLS_68({
        ...{ 'onDispatch': {} },
        ...{ 'onTaskHandled': {} },
        ...{ 'onSessionChange': {} },
        ref: "aiChatRef",
        key: (__VLS_ctx.chatKey),
        agentId: (__VLS_ctx.currentAgentId),
        sessionId: (__VLS_ctx.currentSessionId || undefined),
    }));
    const __VLS_70 = __VLS_69({
        ...{ 'onDispatch': {} },
        ...{ 'onTaskHandled': {} },
        ...{ 'onSessionChange': {} },
        ref: "aiChatRef",
        key: (__VLS_ctx.chatKey),
        agentId: (__VLS_ctx.currentAgentId),
        sessionId: (__VLS_ctx.currentSessionId || undefined),
    }, ...__VLS_functionalComponentArgsRest(__VLS_69));
    let __VLS_73;
    const __VLS_74 = ({ dispatch: {} },
        { onDispatch: (__VLS_ctx.onDispatch) });
    const __VLS_75 = ({ taskHandled: {} },
        { onTaskHandled: (__VLS_ctx.onTaskHandled) });
    const __VLS_76 = ({ sessionChange: {} },
        { onSessionChange: (__VLS_ctx.onSessionCreated) });
    var __VLS_77 = {};
    var __VLS_71;
    var __VLS_72;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-empty']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-hint']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
        ...{ onClick: (...[$event]) => {
                if (!!(__VLS_ctx.currentAgentId))
                    return;
                __VLS_ctx.$router.push('/agents/new');
                // @ts-ignore
                [currentAgentId, currentAgentId, currentSessionId, chatKey, onDispatch, onTaskHandled, onSessionCreated, $router,];
            } },
    });
}
// @ts-ignore
var __VLS_78 = __VLS_77;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({
    __typeEmits: {},
});
export default {};
