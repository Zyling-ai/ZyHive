/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { ElMessage } from 'element-plus';
import { Plus, Refresh, ChatLineRound, VideoPlay, VideoPause, Delete } from '@element-plus/icons-vue';
import { tasks as tasksApi, agents as agentsApi } from '../api/index';
import AiChat from '../components/AiChat.vue';
// ── State ─────────────────────────────────────────────────────────────────
const allTasks = ref([]);
const agents = ref([]);
const loading = ref(false);
const spawning = ref(false);
const killing = ref(false);
const selected = ref(null);
const creating = ref(false);
// Panel widths
const sideW = ref(260);
const chatW = ref(400);
const dragging = ref('');
// Filter
const filterStatus = ref('');
const filterType = ref('');
// New spawn form
const spawnForm = ref({
    agentId: '',
    spawnedBy: '',
    taskType: 'task',
    task: '',
    label: '',
    model: '',
});
// Eligible targets
const eligibleTargets = ref([]);
const eligibleLoading = ref(false);
// Polling
let pollTimer = null;
// ── Computed ──────────────────────────────────────────────────────────────
const filteredTasks = computed(() => {
    let list = [...allTasks.value].sort((a, b) => b.createdAt - a.createdAt);
    if (filterStatus.value)
        list = list.filter(t => t.status === filterStatus.value);
    if (filterType.value)
        list = list.filter(t => t.taskType === filterType.value);
    return list;
});
// Agent lookup maps
const agentMap = computed(() => {
    const m = {};
    agents.value.forEach(a => { m[a.id] = a; });
    return m;
});
// ── Lifecycle ─────────────────────────────────────────────────────────────
onMounted(async () => {
    await Promise.all([loadAgents(), refresh()]);
    // Poll running tasks every 5s
    pollTimer = setInterval(() => {
        const hasRunning = allTasks.value.some(t => t.status === 'running' || t.status === 'pending');
        if (hasRunning)
            refresh(true);
    }, 5000);
});
onUnmounted(() => {
    if (pollTimer)
        clearInterval(pollTimer);
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
});
// ── Data loading ──────────────────────────────────────────────────────────
async function loadAgents() {
    try {
        const res = await agentsApi.list();
        agents.value = res.data.filter(a => !a.system);
    }
    catch { }
}
async function refresh(silent = false) {
    if (!silent)
        loading.value = true;
    try {
        const res = await tasksApi.list();
        allTasks.value = res.data;
        // Refresh selected task if it's in the list
        if (selected.value) {
            const updated = res.data.find(t => t.id === selected.value.id);
            if (updated)
                selected.value = updated;
        }
    }
    catch {
        if (!silent)
            ElMessage.error('加载任务失败');
    }
    finally {
        loading.value = false;
    }
}
// ── Selection ─────────────────────────────────────────────────────────────
function selectTask(t) {
    selected.value = t;
    creating.value = false;
}
function openNew() {
    creating.value = true;
    selected.value = null;
    spawnForm.value = { agentId: '', spawnedBy: '', taskType: 'task', task: '', label: '', model: '' };
    eligibleTargets.value = [];
}
// ── Spawn ─────────────────────────────────────────────────────────────────
async function onSpawnedByChange() {
    spawnForm.value.agentId = '';
    eligibleTargets.value = [];
    if (!spawnForm.value.spawnedBy)
        return;
    eligibleLoading.value = true;
    try {
        const res = await tasksApi.eligibleTargets(spawnForm.value.spawnedBy, spawnForm.value.taskType);
        eligibleTargets.value = res.data;
    }
    catch { }
    finally {
        eligibleLoading.value = false;
    }
}
async function onTypeChange() {
    if (!spawnForm.value.spawnedBy)
        return;
    await onSpawnedByChange();
}
async function doSpawn() {
    if (!spawnForm.value.agentId) {
        ElMessage.warning('请选择目标成员');
        return;
    }
    if (!spawnForm.value.task.trim()) {
        ElMessage.warning(spawnForm.value.taskType === 'task' ? '请填写任务描述' : '请填写汇报内容');
        return;
    }
    spawning.value = true;
    try {
        const res = await tasksApi.spawn({
            agentId: spawnForm.value.agentId,
            task: spawnForm.value.task,
            label: spawnForm.value.label || undefined,
            model: spawnForm.value.model || undefined,
            spawnedBy: spawnForm.value.spawnedBy || undefined,
            taskType: spawnForm.value.taskType,
        });
        ElMessage.success('派遣成功');
        allTasks.value.unshift(res.data);
        creating.value = false;
        selected.value = res.data;
    }
    catch (e) {
        ElMessage.error(e?.response?.data?.error || '派遣失败');
    }
    finally {
        spawning.value = false;
    }
}
// ── Kill / Delete ──────────────────────────────────────────────────────────
async function killTask() {
    if (!selected.value)
        return;
    killing.value = true;
    try {
        await tasksApi.kill(selected.value.id);
        ElMessage.success('任务已终止');
        await refresh();
    }
    catch {
        ElMessage.error('终止失败');
    }
    finally {
        killing.value = false;
    }
}
async function deleteTask() {
    if (!selected.value)
        return;
    try {
        await tasksApi.kill(selected.value.id);
    }
    catch { }
    allTasks.value = allTasks.value.filter(t => t.id !== selected.value.id);
    selected.value = null;
}
// ── Drag resize ────────────────────────────────────────────────────────────
let startX = 0;
let startW = 0;
function startResize(e, target) {
    dragging.value = target;
    startX = e.clientX;
    startW = target === 'side' ? sideW.value : chatW.value;
    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
    e.preventDefault();
}
function onMouseMove(e) {
    const d = e.clientX - startX;
    if (dragging.value === 'side') {
        sideW.value = Math.max(200, Math.min(400, startW + d));
    }
    else if (dragging.value === 'chat') {
        chatW.value = Math.max(280, Math.min(600, startW - d));
    }
}
function onMouseUp() {
    dragging.value = '';
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
}
// ── Helpers ────────────────────────────────────────────────────────────────
function agentName(id) { return agentMap.value[id]?.name || id; }
function agentColor(id) { return agentMap.value[id]?.avatarColor || '#6366f1'; }
function agentInitial(id) { return (agentMap.value[id]?.name || id)[0]?.toUpperCase() || '?'; }
function statusLabel(s) {
    return { pending: '等待中', running: '执行中', done: '已完成', error: '出错', killed: '已终止' }[s] ?? s;
}
function typeLabel(t) {
    return { task: '派遣', report: '汇报', system: '系统' }[t ?? ''] ?? '任务';
}
function truncate(s, n) {
    return s && s.length > n ? s.slice(0, n) + '…' : (s || '');
}
function formatTime(ts) {
    if (!ts)
        return '—';
    return new Date(ts).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}
function relativeTime(ts) {
    const diff = Date.now() - ts;
    if (diff < 60000)
        return '刚刚';
    if (diff < 3600000)
        return Math.floor(diff / 60000) + '分钟前';
    if (diff < 86400000)
        return Math.floor(diff / 3600000) + '小时前';
    return Math.floor(diff / 86400000) + '天前';
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['task-item']} */ ;
/** @type {__VLS_StyleScopedClasses['task-item']} */ ;
/** @type {__VLS_StyleScopedClasses['ds-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['ds-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-empty']} */ ;
/** @type {__VLS_StyleScopedClasses['form-group']} */ ;
/** @type {__VLS_StyleScopedClasses['adv-collapse']} */ ;
/** @type {__VLS_StyleScopedClasses['adv-collapse']} */ ;
/** @type {__VLS_StyleScopedClasses['adv-collapse']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-empty']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "dispatch-studio" },
});
/** @type {__VLS_StyleScopedClasses['dispatch-studio']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "ds-sidebar" },
    ...{ style: ({ width: __VLS_ctx.sideW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['ds-sidebar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-top" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-top']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sidebar-title" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-acts" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-acts']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loading),
    circle: true,
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loading),
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (__VLS_ctx.refresh) });
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[sideW, loading, refresh,];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
}));
const __VLS_21 = __VLS_20({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
let __VLS_24;
const __VLS_25 = ({ click: {} },
    { onClick: (__VLS_ctx.openNew) });
const { default: __VLS_26 } = __VLS_22.slots;
let __VLS_27;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_28 = __VLS_asFunctionalComponent1(__VLS_27, new __VLS_27({}));
const __VLS_29 = __VLS_28({}, ...__VLS_functionalComponentArgsRest(__VLS_28));
const { default: __VLS_32 } = __VLS_30.slots;
let __VLS_33;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_34 = __VLS_asFunctionalComponent1(__VLS_33, new __VLS_33({}));
const __VLS_35 = __VLS_34({}, ...__VLS_functionalComponentArgsRest(__VLS_34));
// @ts-ignore
[openNew,];
var __VLS_30;
// @ts-ignore
[];
var __VLS_22;
var __VLS_23;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "ds-filter" },
});
/** @type {__VLS_StyleScopedClasses['ds-filter']} */ ;
let __VLS_38;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({
    modelValue: (__VLS_ctx.filterStatus),
    placeholder: "状态",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}));
const __VLS_40 = __VLS_39({
    modelValue: (__VLS_ctx.filterStatus),
    placeholder: "状态",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}, ...__VLS_functionalComponentArgsRest(__VLS_39));
/** @type {__VLS_StyleScopedClasses['filter-sel']} */ ;
const { default: __VLS_43 } = __VLS_41.slots;
let __VLS_44;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
    label: "运行中",
    value: "running",
}));
const __VLS_46 = __VLS_45({
    label: "运行中",
    value: "running",
}, ...__VLS_functionalComponentArgsRest(__VLS_45));
let __VLS_49;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
    label: "已完成",
    value: "done",
}));
const __VLS_51 = __VLS_50({
    label: "已完成",
    value: "done",
}, ...__VLS_functionalComponentArgsRest(__VLS_50));
let __VLS_54;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_55 = __VLS_asFunctionalComponent1(__VLS_54, new __VLS_54({
    label: "出错",
    value: "error",
}));
const __VLS_56 = __VLS_55({
    label: "出错",
    value: "error",
}, ...__VLS_functionalComponentArgsRest(__VLS_55));
let __VLS_59;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_60 = __VLS_asFunctionalComponent1(__VLS_59, new __VLS_59({
    label: "已终止",
    value: "killed",
}));
const __VLS_61 = __VLS_60({
    label: "已终止",
    value: "killed",
}, ...__VLS_functionalComponentArgsRest(__VLS_60));
let __VLS_64;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({
    label: "等待中",
    value: "pending",
}));
const __VLS_66 = __VLS_65({
    label: "等待中",
    value: "pending",
}, ...__VLS_functionalComponentArgsRest(__VLS_65));
// @ts-ignore
[filterStatus,];
var __VLS_41;
let __VLS_69;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_70 = __VLS_asFunctionalComponent1(__VLS_69, new __VLS_69({
    modelValue: (__VLS_ctx.filterType),
    placeholder: "类型",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}));
const __VLS_71 = __VLS_70({
    modelValue: (__VLS_ctx.filterType),
    placeholder: "类型",
    clearable: true,
    size: "small",
    ...{ class: "filter-sel" },
}, ...__VLS_functionalComponentArgsRest(__VLS_70));
/** @type {__VLS_StyleScopedClasses['filter-sel']} */ ;
const { default: __VLS_74 } = __VLS_72.slots;
let __VLS_75;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_76 = __VLS_asFunctionalComponent1(__VLS_75, new __VLS_75({
    label: "派遣",
    value: "task",
}));
const __VLS_77 = __VLS_76({
    label: "派遣",
    value: "task",
}, ...__VLS_functionalComponentArgsRest(__VLS_76));
let __VLS_80;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
    label: "汇报",
    value: "report",
}));
const __VLS_82 = __VLS_81({
    label: "汇报",
    value: "report",
}, ...__VLS_functionalComponentArgsRest(__VLS_81));
let __VLS_85;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_86 = __VLS_asFunctionalComponent1(__VLS_85, new __VLS_85({
    label: "系统",
    value: "system",
}));
const __VLS_87 = __VLS_86({
    label: "系统",
    value: "system",
}, ...__VLS_functionalComponentArgsRest(__VLS_86));
// @ts-ignore
[filterType,];
var __VLS_72;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "task-list" },
});
/** @type {__VLS_StyleScopedClasses['task-list']} */ ;
if (!__VLS_ctx.loading && __VLS_ctx.filteredTasks.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "list-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['list-empty']} */ ;
}
for (const [t] of __VLS_vFor((__VLS_ctx.filteredTasks))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectTask(t);
                // @ts-ignore
                [loading, filteredTasks, filteredTasks, selectTask,];
            } },
        key: (t.id),
        ...{ class: (['task-item', { active: __VLS_ctx.selected?.id === t.id }]) },
    });
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    /** @type {__VLS_StyleScopedClasses['task-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "ti-avatar" },
        ...{ style: ({ background: __VLS_ctx.agentColor(t.agentId) }) },
        ...{ class: ({ 'ti-avatar-running': t.status === 'running' }) },
    });
    /** @type {__VLS_StyleScopedClasses['ti-avatar']} */ ;
    /** @type {__VLS_StyleScopedClasses['ti-avatar-running']} */ ;
    (__VLS_ctx.agentInitial(t.agentId));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "ti-info" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "ti-name-row" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-name-row']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ti-name" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-name']} */ ;
    (__VLS_ctx.agentName(t.agentId));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ti-tag" },
        ...{ class: ('tag-' + t.status) },
    });
    /** @type {__VLS_StyleScopedClasses['ti-tag']} */ ;
    (__VLS_ctx.statusLabel(t.status));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "ti-label" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-label']} */ ;
    (t.label || __VLS_ctx.truncate(t.task, 36));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "ti-meta" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-meta']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ti-type" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-type']} */ ;
    (__VLS_ctx.typeLabel(t.taskType));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ti-time" },
    });
    /** @type {__VLS_StyleScopedClasses['ti-time']} */ ;
    (__VLS_ctx.relativeTime(t.createdAt));
    // @ts-ignore
    [selected, agentColor, agentInitial, agentName, statusLabel, truncate, typeLabel, relativeTime,];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'side');
            // @ts-ignore
            [startResize,];
        } },
    ...{ class: "ds-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'side' }) },
});
/** @type {__VLS_StyleScopedClasses['ds-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "ds-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['ds-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "ds-editor" },
});
/** @type {__VLS_StyleScopedClasses['ds-editor']} */ ;
if (!__VLS_ctx.selected && !__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-empty']} */ ;
    let __VLS_90;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_91 = __VLS_asFunctionalComponent1(__VLS_90, new __VLS_90({
        size: "48",
        color: "#c0c4cc",
    }));
    const __VLS_92 = __VLS_91({
        size: "48",
        color: "#c0c4cc",
    }, ...__VLS_functionalComponentArgsRest(__VLS_91));
    const { default: __VLS_95 } = __VLS_93.slots;
    let __VLS_96;
    /** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
    ChatLineRound;
    // @ts-ignore
    const __VLS_97 = __VLS_asFunctionalComponent1(__VLS_96, new __VLS_96({}));
    const __VLS_98 = __VLS_97({}, ...__VLS_functionalComponentArgsRest(__VLS_97));
    // @ts-ignore
    [selected, dragging, creating,];
    var __VLS_93;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
    let __VLS_101;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_102 = __VLS_asFunctionalComponent1(__VLS_101, new __VLS_101({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_103 = __VLS_102({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_102));
    let __VLS_106;
    const __VLS_107 = ({ click: {} },
        { onClick: (__VLS_ctx.openNew) });
    const { default: __VLS_108 } = __VLS_104.slots;
    let __VLS_109;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_110 = __VLS_asFunctionalComponent1(__VLS_109, new __VLS_109({}));
    const __VLS_111 = __VLS_110({}, ...__VLS_functionalComponentArgsRest(__VLS_110));
    const { default: __VLS_114 } = __VLS_112.slots;
    let __VLS_115;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({}));
    const __VLS_117 = __VLS_116({}, ...__VLS_functionalComponentArgsRest(__VLS_116));
    // @ts-ignore
    [openNew,];
    var __VLS_112;
    // @ts-ignore
    [];
    var __VLS_104;
    var __VLS_105;
}
else if (__VLS_ctx.creating) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-toolbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-toolbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-breadcrumb" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-breadcrumb']} */ ;
    let __VLS_120;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_121 = __VLS_asFunctionalComponent1(__VLS_120, new __VLS_120({
        ...{ style: {} },
    }));
    const __VLS_122 = __VLS_121({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_121));
    const { default: __VLS_125 } = __VLS_123.slots;
    let __VLS_126;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_127 = __VLS_asFunctionalComponent1(__VLS_126, new __VLS_126({}));
    const __VLS_128 = __VLS_127({}, ...__VLS_functionalComponentArgsRest(__VLS_127));
    // @ts-ignore
    [creating,];
    var __VLS_123;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-sep" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-sep']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-name" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-name']} */ ;
    (__VLS_ctx.spawnForm.taskType === 'task' ? '派遣任务' : '发起汇报');
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "toolbar-acts" },
    });
    /** @type {__VLS_StyleScopedClasses['toolbar-acts']} */ ;
    let __VLS_131;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_132 = __VLS_asFunctionalComponent1(__VLS_131, new __VLS_131({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_133 = __VLS_132({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_132));
    let __VLS_136;
    const __VLS_137 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selected && !__VLS_ctx.creating))
                    return;
                if (!(__VLS_ctx.creating))
                    return;
                __VLS_ctx.creating = false;
                __VLS_ctx.selected = null;
                // @ts-ignore
                [selected, creating, spawnForm,];
            } });
    const { default: __VLS_138 } = __VLS_134.slots;
    // @ts-ignore
    [];
    var __VLS_134;
    var __VLS_135;
    let __VLS_139;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_140 = __VLS_asFunctionalComponent1(__VLS_139, new __VLS_139({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.spawning),
    }));
    const __VLS_141 = __VLS_140({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.spawning),
    }, ...__VLS_functionalComponentArgsRest(__VLS_140));
    let __VLS_144;
    const __VLS_145 = ({ click: {} },
        { onClick: (__VLS_ctx.doSpawn) });
    const { default: __VLS_146 } = __VLS_142.slots;
    let __VLS_147;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_148 = __VLS_asFunctionalComponent1(__VLS_147, new __VLS_147({}));
    const __VLS_149 = __VLS_148({}, ...__VLS_functionalComponentArgsRest(__VLS_148));
    const { default: __VLS_152 } = __VLS_150.slots;
    let __VLS_153;
    /** @ts-ignore @type { | typeof __VLS_components.VideoPlay} */
    VideoPlay;
    // @ts-ignore
    const __VLS_154 = __VLS_asFunctionalComponent1(__VLS_153, new __VLS_153({}));
    const __VLS_155 = __VLS_154({}, ...__VLS_functionalComponentArgsRest(__VLS_154));
    // @ts-ignore
    [spawning, doSpawn,];
    var __VLS_150;
    (__VLS_ctx.spawnForm.taskType === 'task' ? '派遣' : '汇报');
    // @ts-ignore
    [spawnForm,];
    var __VLS_142;
    var __VLS_143;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-form" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-form']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    let __VLS_158;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
    elRadioGroup;
    // @ts-ignore
    const __VLS_159 = __VLS_asFunctionalComponent1(__VLS_158, new __VLS_158({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.spawnForm.taskType),
        size: "small",
    }));
    const __VLS_160 = __VLS_159({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.spawnForm.taskType),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_159));
    let __VLS_163;
    const __VLS_164 = ({ change: {} },
        { onChange: (__VLS_ctx.onTypeChange) });
    const { default: __VLS_165 } = __VLS_161.slots;
    let __VLS_166;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_167 = __VLS_asFunctionalComponent1(__VLS_166, new __VLS_166({
        value: "task",
    }));
    const __VLS_168 = __VLS_167({
        value: "task",
    }, ...__VLS_functionalComponentArgsRest(__VLS_167));
    const { default: __VLS_171 } = __VLS_169.slots;
    // @ts-ignore
    [spawnForm, onTypeChange,];
    var __VLS_169;
    let __VLS_172;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_173 = __VLS_asFunctionalComponent1(__VLS_172, new __VLS_172({
        value: "report",
    }));
    const __VLS_174 = __VLS_173({
        value: "report",
    }, ...__VLS_functionalComponentArgsRest(__VLS_173));
    const { default: __VLS_177 } = __VLS_175.slots;
    // @ts-ignore
    [];
    var __VLS_175;
    // @ts-ignore
    [];
    var __VLS_161;
    var __VLS_162;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    let __VLS_178;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_179 = __VLS_asFunctionalComponent1(__VLS_178, new __VLS_178({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.spawnForm.spawnedBy),
        placeholder: "选择发起者（可选）",
        clearable: true,
        size: "small",
        ...{ class: "form-full" },
    }));
    const __VLS_180 = __VLS_179({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.spawnForm.spawnedBy),
        placeholder: "选择发起者（可选）",
        clearable: true,
        size: "small",
        ...{ class: "form-full" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_179));
    let __VLS_183;
    const __VLS_184 = ({ change: {} },
        { onChange: (__VLS_ctx.onSpawnedByChange) });
    /** @type {__VLS_StyleScopedClasses['form-full']} */ ;
    const { default: __VLS_185 } = __VLS_181.slots;
    for (const [a] of __VLS_vFor((__VLS_ctx.agents))) {
        let __VLS_186;
        /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
        elOption;
        // @ts-ignore
        const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({
            key: (a.id),
            value: (a.id),
        }));
        const __VLS_188 = __VLS_187({
            key: (a.id),
            value: (a.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_187));
        const { default: __VLS_191 } = __VLS_189.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "agent-opt" },
        });
        /** @type {__VLS_StyleScopedClasses['agent-opt']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "agent-opt-dot" },
            ...{ style: ({ background: a.avatarColor || '#6366f1' }) },
        });
        /** @type {__VLS_StyleScopedClasses['agent-opt-dot']} */ ;
        (a.name);
        // @ts-ignore
        [spawnForm, onSpawnedByChange, agents,];
        var __VLS_189;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_181;
    var __VLS_182;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    (__VLS_ctx.spawnForm.taskType === 'task' ? '目标成员（被派遣）' : '目标成员（汇报对象）');
    let __VLS_192;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({
        modelValue: (__VLS_ctx.spawnForm.agentId),
        placeholder: "选择目标 AI 成员",
        size: "small",
        ...{ class: "form-full" },
        loading: (__VLS_ctx.eligibleLoading),
    }));
    const __VLS_194 = __VLS_193({
        modelValue: (__VLS_ctx.spawnForm.agentId),
        placeholder: "选择目标 AI 成员",
        size: "small",
        ...{ class: "form-full" },
        loading: (__VLS_ctx.eligibleLoading),
    }, ...__VLS_functionalComponentArgsRest(__VLS_193));
    /** @type {__VLS_StyleScopedClasses['form-full']} */ ;
    const { default: __VLS_197 } = __VLS_195.slots;
    if (__VLS_ctx.spawnForm.spawnedBy) {
        for (const [t] of __VLS_vFor((__VLS_ctx.eligibleTargets))) {
            let __VLS_198;
            /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
            elOption;
            // @ts-ignore
            const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({
                key: (t.agentId),
                value: (t.agentId),
            }));
            const __VLS_200 = __VLS_199({
                key: (t.agentId),
                value: (t.agentId),
            }, ...__VLS_functionalComponentArgsRest(__VLS_199));
            const { default: __VLS_203 } = __VLS_201.slots;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "agent-opt" },
            });
            /** @type {__VLS_StyleScopedClasses['agent-opt']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "agent-opt-dot" },
                ...{ style: ({ background: __VLS_ctx.agentColor(t.agentId) }) },
            });
            /** @type {__VLS_StyleScopedClasses['agent-opt-dot']} */ ;
            (__VLS_ctx.agentName(t.agentId));
            let __VLS_204;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
                size: "small",
                effect: "plain",
                ...{ style: {} },
            }));
            const __VLS_206 = __VLS_205({
                size: "small",
                effect: "plain",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_205));
            const { default: __VLS_209 } = __VLS_207.slots;
            (t.relation);
            // @ts-ignore
            [agentColor, agentName, spawnForm, spawnForm, spawnForm, eligibleLoading, eligibleTargets,];
            var __VLS_207;
            // @ts-ignore
            [];
            var __VLS_201;
            // @ts-ignore
            [];
        }
        if (__VLS_ctx.eligibleTargets.length === 0 && !__VLS_ctx.eligibleLoading) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ style: {} },
            });
        }
    }
    else {
        for (const [a] of __VLS_vFor((__VLS_ctx.agents))) {
            let __VLS_210;
            /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
            elOption;
            // @ts-ignore
            const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({
                key: (a.id),
                value: (a.id),
            }));
            const __VLS_212 = __VLS_211({
                key: (a.id),
                value: (a.id),
            }, ...__VLS_functionalComponentArgsRest(__VLS_211));
            const { default: __VLS_215 } = __VLS_213.slots;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "agent-opt" },
            });
            /** @type {__VLS_StyleScopedClasses['agent-opt']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "agent-opt-dot" },
                ...{ style: ({ background: a.avatarColor || '#6366f1' }) },
            });
            /** @type {__VLS_StyleScopedClasses['agent-opt-dot']} */ ;
            (a.name);
            // @ts-ignore
            [agents, eligibleLoading, eligibleTargets,];
            var __VLS_213;
            // @ts-ignore
            [];
        }
    }
    // @ts-ignore
    [];
    var __VLS_195;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    let __VLS_216;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({
        modelValue: (__VLS_ctx.spawnForm.label),
        placeholder: "简短描述，方便识别",
        size: "small",
    }));
    const __VLS_218 = __VLS_217({
        modelValue: (__VLS_ctx.spawnForm.label),
        placeholder: "简短描述，方便识别",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_217));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group form-grow" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    /** @type {__VLS_StyleScopedClasses['form-grow']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    (__VLS_ctx.spawnForm.taskType === 'task' ? '任务描述' : '汇报内容');
    let __VLS_221;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_222 = __VLS_asFunctionalComponent1(__VLS_221, new __VLS_221({
        modelValue: (__VLS_ctx.spawnForm.task),
        type: "textarea",
        rows: (8),
        placeholder: (__VLS_ctx.spawnForm.taskType === 'task' ? '描述要派遣的具体任务…' : '描述要汇报的内容…'),
        resize: "none",
        ...{ class: "task-textarea" },
    }));
    const __VLS_223 = __VLS_222({
        modelValue: (__VLS_ctx.spawnForm.task),
        type: "textarea",
        rows: (8),
        placeholder: (__VLS_ctx.spawnForm.taskType === 'task' ? '描述要派遣的具体任务…' : '描述要汇报的内容…'),
        resize: "none",
        ...{ class: "task-textarea" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_222));
    /** @type {__VLS_StyleScopedClasses['task-textarea']} */ ;
    let __VLS_226;
    /** @ts-ignore @type { | typeof __VLS_components.elCollapse | typeof __VLS_components.ElCollapse | typeof __VLS_components['el-collapse'] | typeof __VLS_components.elCollapse | typeof __VLS_components.ElCollapse | typeof __VLS_components['el-collapse']} */
    elCollapse;
    // @ts-ignore
    const __VLS_227 = __VLS_asFunctionalComponent1(__VLS_226, new __VLS_226({
        ...{ class: "adv-collapse" },
    }));
    const __VLS_228 = __VLS_227({
        ...{ class: "adv-collapse" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_227));
    /** @type {__VLS_StyleScopedClasses['adv-collapse']} */ ;
    const { default: __VLS_231 } = __VLS_229.slots;
    let __VLS_232;
    /** @ts-ignore @type { | typeof __VLS_components.elCollapseItem | typeof __VLS_components.ElCollapseItem | typeof __VLS_components['el-collapse-item'] | typeof __VLS_components.elCollapseItem | typeof __VLS_components.ElCollapseItem | typeof __VLS_components['el-collapse-item']} */
    elCollapseItem;
    // @ts-ignore
    const __VLS_233 = __VLS_asFunctionalComponent1(__VLS_232, new __VLS_232({
        title: "高级选项",
        name: "adv",
    }));
    const __VLS_234 = __VLS_233({
        title: "高级选项",
        name: "adv",
    }, ...__VLS_functionalComponentArgsRest(__VLS_233));
    const { default: __VLS_237 } = __VLS_235.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "form-group" },
    });
    /** @type {__VLS_StyleScopedClasses['form-group']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
        ...{ class: "form-label" },
    });
    /** @type {__VLS_StyleScopedClasses['form-label']} */ ;
    let __VLS_238;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_239 = __VLS_asFunctionalComponent1(__VLS_238, new __VLS_238({
        modelValue: (__VLS_ctx.spawnForm.model),
        placeholder: "留空使用成员默认模型",
        size: "small",
    }));
    const __VLS_240 = __VLS_239({
        modelValue: (__VLS_ctx.spawnForm.model),
        placeholder: "留空使用成员默认模型",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_239));
    // @ts-ignore
    [spawnForm, spawnForm, spawnForm, spawnForm, spawnForm,];
    var __VLS_235;
    // @ts-ignore
    [];
    var __VLS_229;
}
else if (__VLS_ctx.selected) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-toolbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-toolbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-breadcrumb" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-breadcrumb']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "crumb-avatar" },
        ...{ style: ({ background: __VLS_ctx.agentColor(__VLS_ctx.selected.agentId) }) },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-avatar']} */ ;
    (__VLS_ctx.agentInitial(__VLS_ctx.selected.agentId));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-sep" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-sep']} */ ;
    (__VLS_ctx.agentName(__VLS_ctx.selected.agentId));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-name" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-name']} */ ;
    (__VLS_ctx.selected.label || __VLS_ctx.truncate(__VLS_ctx.selected.task, 24));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "toolbar-acts" },
    });
    /** @type {__VLS_StyleScopedClasses['toolbar-acts']} */ ;
    let __VLS_243;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_244 = __VLS_asFunctionalComponent1(__VLS_243, new __VLS_243({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_245 = __VLS_244({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_244));
    let __VLS_248;
    const __VLS_249 = ({ click: {} },
        { onClick: (__VLS_ctx.openNew) });
    const { default: __VLS_250 } = __VLS_246.slots;
    let __VLS_251;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_252 = __VLS_asFunctionalComponent1(__VLS_251, new __VLS_251({}));
    const __VLS_253 = __VLS_252({}, ...__VLS_functionalComponentArgsRest(__VLS_252));
    const { default: __VLS_256 } = __VLS_254.slots;
    let __VLS_257;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_258 = __VLS_asFunctionalComponent1(__VLS_257, new __VLS_257({}));
    const __VLS_259 = __VLS_258({}, ...__VLS_functionalComponentArgsRest(__VLS_258));
    // @ts-ignore
    [openNew, selected, selected, selected, selected, selected, selected, agentColor, agentInitial, agentName, truncate,];
    var __VLS_254;
    // @ts-ignore
    [];
    var __VLS_246;
    var __VLS_247;
    if (__VLS_ctx.selected.status === 'running' || __VLS_ctx.selected.status === 'pending') {
        let __VLS_262;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_263 = __VLS_asFunctionalComponent1(__VLS_262, new __VLS_262({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
            plain: true,
            loading: (__VLS_ctx.killing),
        }));
        const __VLS_264 = __VLS_263({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
            plain: true,
            loading: (__VLS_ctx.killing),
        }, ...__VLS_functionalComponentArgsRest(__VLS_263));
        let __VLS_267;
        const __VLS_268 = ({ click: {} },
            { onClick: (__VLS_ctx.killTask) });
        const { default: __VLS_269 } = __VLS_265.slots;
        let __VLS_270;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_271 = __VLS_asFunctionalComponent1(__VLS_270, new __VLS_270({}));
        const __VLS_272 = __VLS_271({}, ...__VLS_functionalComponentArgsRest(__VLS_271));
        const { default: __VLS_275 } = __VLS_273.slots;
        let __VLS_276;
        /** @ts-ignore @type { | typeof __VLS_components.VideoPause} */
        VideoPause;
        // @ts-ignore
        const __VLS_277 = __VLS_asFunctionalComponent1(__VLS_276, new __VLS_276({}));
        const __VLS_278 = __VLS_277({}, ...__VLS_functionalComponentArgsRest(__VLS_277));
        // @ts-ignore
        [selected, selected, killing, killTask,];
        var __VLS_273;
        // @ts-ignore
        [];
        var __VLS_265;
        var __VLS_266;
    }
    let __VLS_281;
    /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
    elPopconfirm;
    // @ts-ignore
    const __VLS_282 = __VLS_asFunctionalComponent1(__VLS_281, new __VLS_281({
        ...{ 'onConfirm': {} },
        title: "确认删除该任务记录？",
    }));
    const __VLS_283 = __VLS_282({
        ...{ 'onConfirm': {} },
        title: "确认删除该任务记录？",
    }, ...__VLS_functionalComponentArgsRest(__VLS_282));
    let __VLS_286;
    const __VLS_287 = ({ confirm: {} },
        { onConfirm: (__VLS_ctx.deleteTask) });
    const { default: __VLS_288 } = __VLS_284.slots;
    {
        const { reference: __VLS_289 } = __VLS_284.slots;
        let __VLS_290;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_291 = __VLS_asFunctionalComponent1(__VLS_290, new __VLS_290({
            size: "small",
            type: "danger",
            plain: true,
        }));
        const __VLS_292 = __VLS_291({
            size: "small",
            type: "danger",
            plain: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_291));
        const { default: __VLS_295 } = __VLS_293.slots;
        let __VLS_296;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_297 = __VLS_asFunctionalComponent1(__VLS_296, new __VLS_296({}));
        const __VLS_298 = __VLS_297({}, ...__VLS_functionalComponentArgsRest(__VLS_297));
        const { default: __VLS_301 } = __VLS_299.slots;
        let __VLS_302;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_303 = __VLS_asFunctionalComponent1(__VLS_302, new __VLS_302({}));
        const __VLS_304 = __VLS_303({}, ...__VLS_functionalComponentArgsRest(__VLS_303));
        // @ts-ignore
        [deleteTask,];
        var __VLS_299;
        // @ts-ignore
        [];
        var __VLS_293;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_284;
    var __VLS_285;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-body" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-body']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-status-bar" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-status-bar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ds-tag" },
        ...{ class: ('tag-' + __VLS_ctx.selected.status) },
    });
    /** @type {__VLS_StyleScopedClasses['ds-tag']} */ ;
    (__VLS_ctx.statusLabel(__VLS_ctx.selected.status));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ds-badge" },
    });
    /** @type {__VLS_StyleScopedClasses['ds-badge']} */ ;
    (__VLS_ctx.typeLabel(__VLS_ctx.selected.taskType));
    if (__VLS_ctx.selected.relation) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "ds-badge" },
        });
        /** @type {__VLS_StyleScopedClasses['ds-badge']} */ ;
        (__VLS_ctx.selected.relation);
    }
    if (__VLS_ctx.selected.model) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "ds-badge ds-badge-model" },
        });
        /** @type {__VLS_StyleScopedClasses['ds-badge']} */ ;
        /** @type {__VLS_StyleScopedClasses['ds-badge-model']} */ ;
        (__VLS_ctx.selected.model);
    }
    if (__VLS_ctx.selected.spawnedBy) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "detail-chain" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-chain']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "chain-from" },
        });
        /** @type {__VLS_StyleScopedClasses['chain-from']} */ ;
        (__VLS_ctx.agentName(__VLS_ctx.selected.spawnedBy));
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "chain-arrow" },
        });
        /** @type {__VLS_StyleScopedClasses['chain-arrow']} */ ;
        (__VLS_ctx.selected.taskType === 'report' ? '↑' : '↓');
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "chain-to" },
        });
        /** @type {__VLS_StyleScopedClasses['chain-to']} */ ;
        (__VLS_ctx.agentName(__VLS_ctx.selected.agentId));
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-times" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-times']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dt-item" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dt-key" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-key']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dt-val" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-val']} */ ;
    (__VLS_ctx.formatTime(__VLS_ctx.selected.createdAt));
    if (__VLS_ctx.selected.startedAt) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dt-item" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-item']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dt-key" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-key']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dt-val" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-val']} */ ;
        (__VLS_ctx.formatTime(__VLS_ctx.selected.startedAt));
    }
    if (__VLS_ctx.selected.endedAt) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dt-item" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-item']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dt-key" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-key']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dt-val" },
        });
        /** @type {__VLS_StyleScopedClasses['dt-val']} */ ;
        (__VLS_ctx.formatTime(__VLS_ctx.selected.endedAt));
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dt-item" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dt-key" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-key']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dt-val" },
    });
    /** @type {__VLS_StyleScopedClasses['dt-val']} */ ;
    (__VLS_ctx.selected.duration || '—');
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "detail-section" },
    });
    /** @type {__VLS_StyleScopedClasses['detail-section']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "section-label" },
    });
    /** @type {__VLS_StyleScopedClasses['section-label']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "section-content task-desc" },
    });
    /** @type {__VLS_StyleScopedClasses['section-content']} */ ;
    /** @type {__VLS_StyleScopedClasses['task-desc']} */ ;
    (__VLS_ctx.selected.task);
    if (__VLS_ctx.selected.error) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "detail-section" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-section']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "section-label section-error" },
        });
        /** @type {__VLS_StyleScopedClasses['section-label']} */ ;
        /** @type {__VLS_StyleScopedClasses['section-error']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "section-content error-content" },
        });
        /** @type {__VLS_StyleScopedClasses['section-content']} */ ;
        /** @type {__VLS_StyleScopedClasses['error-content']} */ ;
        (__VLS_ctx.selected.error);
    }
    if (__VLS_ctx.selected.output) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "detail-section detail-output" },
        });
        /** @type {__VLS_StyleScopedClasses['detail-section']} */ ;
        /** @type {__VLS_StyleScopedClasses['detail-output']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "section-label" },
        });
        /** @type {__VLS_StyleScopedClasses['section-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "section-content output-content" },
        });
        /** @type {__VLS_StyleScopedClasses['section-content']} */ ;
        /** @type {__VLS_StyleScopedClasses['output-content']} */ ;
        (__VLS_ctx.truncate(__VLS_ctx.selected.output, 600));
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'chat');
            // @ts-ignore
            [selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, selected, agentName, agentName, statusLabel, truncate, typeLabel, startResize, formatTime, formatTime, formatTime,];
        } },
    ...{ class: "ds-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'chat' }) },
});
/** @type {__VLS_StyleScopedClasses['ds-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "ds-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['ds-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "ds-chat" },
    ...{ style: ({ width: __VLS_ctx.chatW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['ds-chat']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-panel-head" },
});
/** @type {__VLS_StyleScopedClasses['chat-panel-head']} */ ;
let __VLS_307;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_308 = __VLS_asFunctionalComponent1(__VLS_307, new __VLS_307({}));
const __VLS_309 = __VLS_308({}, ...__VLS_functionalComponentArgsRest(__VLS_308));
const { default: __VLS_312 } = __VLS_310.slots;
let __VLS_313;
/** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
ChatLineRound;
// @ts-ignore
const __VLS_314 = __VLS_asFunctionalComponent1(__VLS_313, new __VLS_313({}));
const __VLS_315 = __VLS_314({}, ...__VLS_functionalComponentArgsRest(__VLS_314));
// @ts-ignore
[dragging, chatW,];
var __VLS_310;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
(__VLS_ctx.selected ? __VLS_ctx.agentName(__VLS_ctx.selected.agentId) + ' 的会话' : '实时对话');
if (__VLS_ctx.selected?.status === 'running') {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "chat-live-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-live-dot']} */ ;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-wrap" },
});
/** @type {__VLS_StyleScopedClasses['chat-wrap']} */ ;
if (!__VLS_ctx.selected) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "chat-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['chat-empty']} */ ;
    let __VLS_318;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_319 = __VLS_asFunctionalComponent1(__VLS_318, new __VLS_318({
        size: "36",
        color: "#c0c4cc",
    }));
    const __VLS_320 = __VLS_319({
        size: "36",
        color: "#c0c4cc",
    }, ...__VLS_functionalComponentArgsRest(__VLS_319));
    const { default: __VLS_323 } = __VLS_321.slots;
    let __VLS_324;
    /** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
    ChatLineRound;
    // @ts-ignore
    const __VLS_325 = __VLS_asFunctionalComponent1(__VLS_324, new __VLS_324({}));
    const __VLS_326 = __VLS_325({}, ...__VLS_functionalComponentArgsRest(__VLS_325));
    // @ts-ignore
    [selected, selected, selected, selected, agentName,];
    var __VLS_321;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
}
else {
    const __VLS_329 = AiChat;
    // @ts-ignore
    const __VLS_330 = __VLS_asFunctionalComponent1(__VLS_329, new __VLS_329({
        key: (__VLS_ctx.selected.sessionId),
        agentId: (__VLS_ctx.selected.agentId),
        sessionId: (__VLS_ctx.selected.sessionId),
        height: "100%",
    }));
    const __VLS_331 = __VLS_330({
        key: (__VLS_ctx.selected.sessionId),
        agentId: (__VLS_ctx.selected.agentId),
        sessionId: (__VLS_ctx.selected.sessionId),
        height: "100%",
    }, ...__VLS_functionalComponentArgsRest(__VLS_330));
}
// @ts-ignore
[selected, selected, selected,];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
