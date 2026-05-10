/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, reactive, onMounted, computed } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { Plus } from '@element-plus/icons-vue';
import { cron as cronApi, agents as agentsApi } from '../api';
const router = useRouter();
const jobs = ref([]);
const agentList = ref([]);
const filterAgentId = ref('');
const showCreate = ref(false);
const showMorning = ref(false);
// Logs
const showLogs = ref(false);
const currentJob = ref(null);
const runLogs = ref([]);
const loadingLogs = ref(false);
const agentNameMap = computed(() => {
    const m = {};
    for (const ag of agentList.value)
        m[ag.id] = ag.name;
    return m;
});
const form = reactive({
    agentId: '',
    name: '',
    remark: '',
    expr: '0 0 9 * * *',
    tz: 'Asia/Shanghai',
    message: '',
    enabled: true,
});
// 晨间例行表单
const morning = reactive({
    agentId: '',
    timeStr: '08:00',
    tz: 'Asia/Shanghai',
});
onMounted(async () => {
    const res = await agentsApi.list().catch(() => ({ data: [] }));
    agentList.value = res.data || [];
    loadJobs();
});
async function loadJobs() {
    try {
        const res = await cronApi.list(filterAgentId.value || undefined);
        jobs.value = res.data || [];
    }
    catch (e) {
        ElMessage.error('加载定时任务失败: ' + (e?.message || '未知错误'));
    }
}
function formatTime(ms) {
    return ms ? new Date(ms).toLocaleString('zh-CN') : '';
}
function isMemoryJob(row) {
    return row.payload?.message === '__MEMORY_CONSOLIDATE__';
}
function goToAgent(row) {
    if (row.agentId) {
        router.push({ path: `/agents/${row.agentId}`, query: { tab: 'cron' } });
    }
}
function openCreate() {
    form.agentId = '';
    form.name = '';
    form.remark = '';
    form.expr = '0 0 9 * * *';
    form.tz = 'Asia/Shanghai';
    form.message = '';
    form.enabled = true;
    showCreate.value = true;
}
// 打开晨间例行对话框：默认填选中的 filter 或第一个成员
function openMorningRoutine() {
    // 若筛选栏已选中某成员，默认填入；否则第一个
    if (filterAgentId.value && filterAgentId.value !== '__global__') {
        morning.agentId = filterAgentId.value;
    }
    else {
        morning.agentId = agentList.value[0]?.id || '';
    }
    morning.timeStr = '08:00';
    morning.tz = 'Asia/Shanghai';
    showMorning.value = true;
}
// 晨间例行 prompt 模板（末尾 NO_ALERT 指令对接 cron engine SilentToken 机制）
const MORNING_PROMPT = `晨间例行（每日自动唤醒）：

1. 扫描昨天的对话历史（conversations/INDEX.md），把值得长期记住的要点整理到 memory/core/ 或 memory/daily/ 相应文件。
2. 检查 WISHLIST.md 与 GOALS，看有没有进展或新的机会点。
3. 若发现世界状态相关（时事、价格、版本等）需要更新，可用 web_search / web_fetch 查一下。
4. 若有值得主动告诉用户的事（进展、风险、提醒），追加到 memory/daily/notes-to-user.md 并在本次回复中简要汇报。
5. 若今天没有任何值得汇报的事，请只回一个单词：NO_ALERT（系统会静默处理，不打扰用户）。

保持简洁、克制、有用。不要为了汇报而汇报。`;
async function createMorningRoutine() {
    if (!morning.agentId) {
        ElMessage.warning('请选择 AI 成员');
        return;
    }
    // 解析 HH:mm → 标准 5 字段 cron：分 时 日 月 周
    const m = /^(\d{1,2}):(\d{1,2})$/.exec(morning.timeStr || '');
    if (!m || !m[1] || !m[2]) {
        ElMessage.error('时间格式错误，应为 HH:mm');
        return;
    }
    const HH = parseInt(m[1], 10);
    const MM = parseInt(m[2], 10);
    const expr = `${MM} ${HH} * * *`;
    const agentName = agentNameMap.value[morning.agentId] || morning.agentId;
    try {
        await cronApi.create({
            name: '晨间例行',
            remark: `每天 ${morning.timeStr} 自动唤醒 ${agentName}：整理记忆 · 检查愿望 · 给你留便条`,
            agentId: morning.agentId,
            enabled: true,
            schedule: { kind: 'cron', expr, tz: morning.tz },
            payload: { kind: 'agentTurn', message: MORNING_PROMPT },
            delivery: { mode: 'announce' },
        });
        ElMessage.success(`已为 ${agentName} 创建晨间例行（每天 ${morning.timeStr}）`);
        showMorning.value = false;
        loadJobs();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
}
// #16 fix: validate cron expression before submit
function isValidCronExpr(expr) {
    if (!expr || !expr.trim())
        return false;
    const parts = expr.trim().split(/\s+/);
    if (parts.length < 5 || parts.length > 6)
        return false;
    // basic field range check (loose)
    return parts.every(p => /^[\d,\-\*\/]+$/.test(p) || p === '?');
}
async function createCron() {
    if (!form.name?.trim()) {
        ElMessage.warning('请填写任务名称');
        return;
    }
    if (!isValidCronExpr(form.expr)) {
        ElMessage.error('Cron 表达式格式错误，格式为：分 时 日 月 周（如 0 9 * * 1）');
        return;
    }
    if (!form.message?.trim()) {
        ElMessage.warning('请填写任务内容');
        return;
    }
    try {
        await cronApi.create({
            name: form.name,
            remark: form.remark || undefined,
            agentId: form.agentId || undefined,
            enabled: form.enabled,
            schedule: { kind: 'cron', expr: form.expr.trim(), tz: form.tz },
            payload: { kind: 'agentTurn', message: form.message },
            delivery: { mode: 'announce' },
        });
        ElMessage.success('创建成功');
        showCreate.value = false;
        loadJobs();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
}
async function toggleCron(job) {
    try {
        await cronApi.update(job.id, job);
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function runNow(job) {
    try {
        await cronApi.run(job.id);
        ElMessage.success('已触发');
        setTimeout(loadJobs, 2000);
    }
    catch {
        ElMessage.error('触发失败');
    }
}
async function deleteCron(job) {
    try {
        await cronApi.delete(job.id);
        ElMessage.success('已删除');
        loadJobs();
    }
    catch {
        ElMessage.error('删除失败');
    }
}
async function openLogs(job) {
    currentJob.value = job;
    showLogs.value = true;
    loadingLogs.value = true;
    try {
        const res = await cronApi.runs(job.id);
        runLogs.value = (res.data || []).slice().reverse(); // newest first
    }
    catch {
        ElMessage.error('获取日志失败');
        runLogs.value = [];
    }
    finally {
        loadingLogs.value = false;
    }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "cron-page" },
});
/** @type {__VLS_StyleScopedClasses['cron-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ style: {} },
});
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ style: {} },
}));
const __VLS_2 = __VLS_1({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
let __VLS_6;
/** @ts-ignore @type { | typeof __VLS_components.Timer} */
Timer;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_11;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
    ...{ 'onClick': {} },
}));
const __VLS_13 = __VLS_12({
    ...{ 'onClick': {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_12));
let __VLS_16;
const __VLS_17 = ({ click: {} },
    { onClick: (__VLS_ctx.openMorningRoutine) });
const { default: __VLS_18 } = __VLS_14.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
// @ts-ignore
[openMorningRoutine,];
var __VLS_14;
var __VLS_15;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    ...{ 'onClick': {} },
    type: "primary",
}));
const __VLS_21 = __VLS_20({
    ...{ 'onClick': {} },
    type: "primary",
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
let __VLS_24;
const __VLS_25 = ({ click: {} },
    { onClick: (__VLS_ctx.openCreate) });
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
[openCreate,];
var __VLS_30;
// @ts-ignore
[];
var __VLS_22;
var __VLS_23;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_38;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_40 = __VLS_39({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_39));
const { default: __VLS_43 } = __VLS_41.slots;
// @ts-ignore
[];
var __VLS_41;
let __VLS_44;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgentId),
    size: "small",
}));
const __VLS_46 = __VLS_45({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgentId),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_45));
let __VLS_49;
const __VLS_50 = ({ change: {} },
    { onChange: (__VLS_ctx.loadJobs) });
const { default: __VLS_51 } = __VLS_47.slots;
let __VLS_52;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_53 = __VLS_asFunctionalComponent1(__VLS_52, new __VLS_52({
    value: "",
}));
const __VLS_54 = __VLS_53({
    value: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_53));
const { default: __VLS_57 } = __VLS_55.slots;
// @ts-ignore
[filterAgentId, loadJobs,];
var __VLS_55;
let __VLS_58;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_59 = __VLS_asFunctionalComponent1(__VLS_58, new __VLS_58({
    value: "__global__",
}));
const __VLS_60 = __VLS_59({
    value: "__global__",
}, ...__VLS_functionalComponentArgsRest(__VLS_59));
const { default: __VLS_63 } = __VLS_61.slots;
// @ts-ignore
[];
var __VLS_61;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_64;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({
        key: (ag.id),
        value: (ag.id),
    }));
    const __VLS_66 = __VLS_65({
        key: (ag.id),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_65));
    const { default: __VLS_69 } = __VLS_67.slots;
    (ag.name);
    // @ts-ignore
    [agentList,];
    var __VLS_67;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_47;
var __VLS_48;
let __VLS_70;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_71 = __VLS_asFunctionalComponent1(__VLS_70, new __VLS_70({
    shadow: "hover",
}));
const __VLS_72 = __VLS_71({
    shadow: "hover",
}, ...__VLS_functionalComponentArgsRest(__VLS_71));
const { default: __VLS_75 } = __VLS_73.slots;
let __VLS_76;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_77 = __VLS_asFunctionalComponent1(__VLS_76, new __VLS_76({
    data: (__VLS_ctx.jobs),
    stripe: true,
}));
const __VLS_78 = __VLS_77({
    data: (__VLS_ctx.jobs),
    stripe: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_77));
const { default: __VLS_81 } = __VLS_79.slots;
let __VLS_82;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_83 = __VLS_asFunctionalComponent1(__VLS_82, new __VLS_82({
    prop: "name",
    label: "名称",
    minWidth: "150",
}));
const __VLS_84 = __VLS_83({
    prop: "name",
    label: "名称",
    minWidth: "150",
}, ...__VLS_functionalComponentArgsRest(__VLS_83));
let __VLS_87;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_88 = __VLS_asFunctionalComponent1(__VLS_87, new __VLS_87({
    label: "所属成员",
    width: "120",
}));
const __VLS_89 = __VLS_88({
    label: "所属成员",
    width: "120",
}, ...__VLS_functionalComponentArgsRest(__VLS_88));
const { default: __VLS_92 } = __VLS_90.slots;
{
    const { default: __VLS_93 } = __VLS_90.slots;
    const [{ row }] = __VLS_vSlot(__VLS_93);
    if (row.agentId) {
        let __VLS_94;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_95 = __VLS_asFunctionalComponent1(__VLS_94, new __VLS_94({
            ...{ 'onClick': {} },
            size: "small",
            type: "primary",
            ...{ style: {} },
        }));
        const __VLS_96 = __VLS_95({
            ...{ 'onClick': {} },
            size: "small",
            type: "primary",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_95));
        let __VLS_99;
        const __VLS_100 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(row.agentId))
                        return;
                    __VLS_ctx.goToAgent(row);
                    // @ts-ignore
                    [jobs, goToAgent,];
                } });
        const { default: __VLS_101 } = __VLS_97.slots;
        (__VLS_ctx.agentNameMap[row.agentId] || row.agentId);
        // @ts-ignore
        [agentNameMap,];
        var __VLS_97;
        var __VLS_98;
    }
    else {
        let __VLS_102;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_103 = __VLS_asFunctionalComponent1(__VLS_102, new __VLS_102({
            size: "small",
            type: "info",
        }));
        const __VLS_104 = __VLS_103({
            size: "small",
            type: "info",
        }, ...__VLS_functionalComponentArgsRest(__VLS_103));
        const { default: __VLS_107 } = __VLS_105.slots;
        // @ts-ignore
        [];
        var __VLS_105;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_90;
let __VLS_108;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_109 = __VLS_asFunctionalComponent1(__VLS_108, new __VLS_108({
    label: "备注",
    minWidth: "150",
    showOverflowTooltip: true,
}));
const __VLS_110 = __VLS_109({
    label: "备注",
    minWidth: "150",
    showOverflowTooltip: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_109));
const { default: __VLS_113 } = __VLS_111.slots;
{
    const { default: __VLS_114 } = __VLS_111.slots;
    const [{ row }] = __VLS_vSlot(__VLS_114);
    if (row.remark) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (row.remark);
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_111;
let __VLS_115;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({
    label: "调度",
    minWidth: "160",
}));
const __VLS_117 = __VLS_116({
    label: "调度",
    minWidth: "160",
}, ...__VLS_functionalComponentArgsRest(__VLS_116));
const { default: __VLS_120 } = __VLS_118.slots;
{
    const { default: __VLS_121 } = __VLS_118.slots;
    const [{ row }] = __VLS_vSlot(__VLS_121);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (row.schedule?.expr);
    let __VLS_122;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_123 = __VLS_asFunctionalComponent1(__VLS_122, new __VLS_122({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_124 = __VLS_123({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_123));
    const { default: __VLS_127 } = __VLS_125.slots;
    (row.schedule?.tz);
    // @ts-ignore
    [];
    var __VLS_125;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_118;
let __VLS_128;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_129 = __VLS_asFunctionalComponent1(__VLS_128, new __VLS_128({
    label: "最近运行",
    width: "170",
}));
const __VLS_130 = __VLS_129({
    label: "最近运行",
    width: "170",
}, ...__VLS_functionalComponentArgsRest(__VLS_129));
const { default: __VLS_133 } = __VLS_131.slots;
{
    const { default: __VLS_134 } = __VLS_131.slots;
    const [{ row }] = __VLS_vSlot(__VLS_134);
    if (row.state?.lastRunAtMs) {
        let __VLS_135;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_136 = __VLS_asFunctionalComponent1(__VLS_135, new __VLS_135({
            type: (row.state?.lastStatus === 'ok' ? 'success' : 'danger'),
            size: "small",
        }));
        const __VLS_137 = __VLS_136({
            type: (row.state?.lastStatus === 'ok' ? 'success' : 'danger'),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_136));
        const { default: __VLS_140 } = __VLS_138.slots;
        (row.state?.lastStatus);
        // @ts-ignore
        [];
        var __VLS_138;
        let __VLS_141;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_142 = __VLS_asFunctionalComponent1(__VLS_141, new __VLS_141({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_143 = __VLS_142({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_142));
        const { default: __VLS_146 } = __VLS_144.slots;
        (__VLS_ctx.formatTime(row.state?.lastRunAtMs));
        // @ts-ignore
        [formatTime,];
        var __VLS_144;
    }
    else {
        let __VLS_147;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_148 = __VLS_asFunctionalComponent1(__VLS_147, new __VLS_147({
            type: "info",
            size: "small",
        }));
        const __VLS_149 = __VLS_148({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_148));
        const { default: __VLS_152 } = __VLS_150.slots;
        // @ts-ignore
        [];
        var __VLS_150;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_131;
let __VLS_153;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_154 = __VLS_asFunctionalComponent1(__VLS_153, new __VLS_153({
    label: "启用",
    width: "70",
}));
const __VLS_155 = __VLS_154({
    label: "启用",
    width: "70",
}, ...__VLS_functionalComponentArgsRest(__VLS_154));
const { default: __VLS_158 } = __VLS_156.slots;
{
    const { default: __VLS_159 } = __VLS_156.slots;
    const [{ row }] = __VLS_vSlot(__VLS_159);
    let __VLS_160;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_161 = __VLS_asFunctionalComponent1(__VLS_160, new __VLS_160({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
        disabled: (__VLS_ctx.isMemoryJob(row)),
    }));
    const __VLS_162 = __VLS_161({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
        disabled: (__VLS_ctx.isMemoryJob(row)),
    }, ...__VLS_functionalComponentArgsRest(__VLS_161));
    let __VLS_165;
    const __VLS_166 = ({ change: {} },
        { onChange: (...[$event]) => {
                __VLS_ctx.toggleCron(row);
                // @ts-ignore
                [isMemoryJob, toggleCron,];
            } });
    var __VLS_163;
    var __VLS_164;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_156;
let __VLS_167;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_168 = __VLS_asFunctionalComponent1(__VLS_167, new __VLS_167({
    label: "操作",
    width: "240",
}));
const __VLS_169 = __VLS_168({
    label: "操作",
    width: "240",
}, ...__VLS_functionalComponentArgsRest(__VLS_168));
const { default: __VLS_172 } = __VLS_170.slots;
{
    const { default: __VLS_173 } = __VLS_170.slots;
    const [{ row }] = __VLS_vSlot(__VLS_173);
    if (__VLS_ctx.isMemoryJob(row)) {
        let __VLS_174;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_175 = __VLS_asFunctionalComponent1(__VLS_174, new __VLS_174({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_176 = __VLS_175({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_175));
        const { default: __VLS_179 } = __VLS_177.slots;
        // @ts-ignore
        [isMemoryJob,];
        var __VLS_177;
        let __VLS_180;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_181 = __VLS_asFunctionalComponent1(__VLS_180, new __VLS_180({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_182 = __VLS_181({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_181));
        let __VLS_185;
        const __VLS_186 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.isMemoryJob(row)))
                        return;
                    __VLS_ctx.goToAgent(row);
                    // @ts-ignore
                    [goToAgent,];
                } });
        const { default: __VLS_187 } = __VLS_183.slots;
        // @ts-ignore
        [];
        var __VLS_183;
        var __VLS_184;
    }
    else {
        let __VLS_188;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_189 = __VLS_asFunctionalComponent1(__VLS_188, new __VLS_188({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_190 = __VLS_189({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_189));
        let __VLS_193;
        const __VLS_194 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.isMemoryJob(row)))
                        return;
                    __VLS_ctx.runNow(row);
                    // @ts-ignore
                    [runNow,];
                } });
        const { default: __VLS_195 } = __VLS_191.slots;
        // @ts-ignore
        [];
        var __VLS_191;
        var __VLS_192;
        let __VLS_196;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_197 = __VLS_asFunctionalComponent1(__VLS_196, new __VLS_196({
            ...{ 'onClick': {} },
            size: "small",
            type: "info",
        }));
        const __VLS_198 = __VLS_197({
            ...{ 'onClick': {} },
            size: "small",
            type: "info",
        }, ...__VLS_functionalComponentArgsRest(__VLS_197));
        let __VLS_201;
        const __VLS_202 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.isMemoryJob(row)))
                        return;
                    __VLS_ctx.openLogs(row);
                    // @ts-ignore
                    [openLogs,];
                } });
        const { default: __VLS_203 } = __VLS_199.slots;
        // @ts-ignore
        [];
        var __VLS_199;
        var __VLS_200;
        let __VLS_204;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
        }));
        const __VLS_206 = __VLS_205({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
        }, ...__VLS_functionalComponentArgsRest(__VLS_205));
        let __VLS_209;
        const __VLS_210 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.isMemoryJob(row)))
                        return;
                    __VLS_ctx.deleteCron(row);
                    // @ts-ignore
                    [deleteCron,];
                } });
        const { default: __VLS_211 } = __VLS_207.slots;
        // @ts-ignore
        [];
        var __VLS_207;
        var __VLS_208;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_170;
// @ts-ignore
[];
var __VLS_79;
if (__VLS_ctx.jobs.length === 0) {
    let __VLS_212;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_213 = __VLS_asFunctionalComponent1(__VLS_212, new __VLS_212({
        description: "暂无定时任务",
    }));
    const __VLS_214 = __VLS_213({
        description: "暂无定时任务",
    }, ...__VLS_functionalComponentArgsRest(__VLS_213));
}
// @ts-ignore
[jobs,];
var __VLS_73;
let __VLS_217;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_218 = __VLS_asFunctionalComponent1(__VLS_217, new __VLS_217({
    modelValue: (__VLS_ctx.showLogs),
    title: (`执行日志 — ${__VLS_ctx.currentJob?.name}`),
    width: "780px",
}));
const __VLS_219 = __VLS_218({
    modelValue: (__VLS_ctx.showLogs),
    title: (`执行日志 — ${__VLS_ctx.currentJob?.name}`),
    width: "780px",
}, ...__VLS_functionalComponentArgsRest(__VLS_218));
const { default: __VLS_222 } = __VLS_220.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_223;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_224 = __VLS_asFunctionalComponent1(__VLS_223, new __VLS_223({
    type: "info",
    size: "small",
}));
const __VLS_225 = __VLS_224({
    type: "info",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_224));
const { default: __VLS_228 } = __VLS_226.slots;
// @ts-ignore
[showLogs, currentJob,];
var __VLS_226;
let __VLS_229;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_230 = __VLS_asFunctionalComponent1(__VLS_229, new __VLS_229({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loadingLogs),
}));
const __VLS_231 = __VLS_230({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loadingLogs),
}, ...__VLS_functionalComponentArgsRest(__VLS_230));
let __VLS_234;
const __VLS_235 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.openLogs(__VLS_ctx.currentJob);
            // @ts-ignore
            [openLogs, currentJob, loadingLogs,];
        } });
const { default: __VLS_236 } = __VLS_232.slots;
// @ts-ignore
[];
var __VLS_232;
var __VLS_233;
let __VLS_237;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_238 = __VLS_asFunctionalComponent1(__VLS_237, new __VLS_237({
    data: (__VLS_ctx.runLogs),
    stripe: true,
    size: "small",
    maxHeight: "460",
}));
const __VLS_239 = __VLS_238({
    data: (__VLS_ctx.runLogs),
    stripe: true,
    size: "small",
    maxHeight: "460",
}, ...__VLS_functionalComponentArgsRest(__VLS_238));
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.loadingLogs) }, null, null);
const { default: __VLS_242 } = __VLS_240.slots;
let __VLS_243;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_244 = __VLS_asFunctionalComponent1(__VLS_243, new __VLS_243({
    label: "运行时间",
    width: "170",
}));
const __VLS_245 = __VLS_244({
    label: "运行时间",
    width: "170",
}, ...__VLS_functionalComponentArgsRest(__VLS_244));
const { default: __VLS_248 } = __VLS_246.slots;
{
    const { default: __VLS_249 } = __VLS_246.slots;
    const [{ row }] = __VLS_vSlot(__VLS_249);
    (__VLS_ctx.formatTime(row.startedAt));
    // @ts-ignore
    [formatTime, loadingLogs, runLogs, vLoading,];
}
// @ts-ignore
[];
var __VLS_246;
let __VLS_250;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_251 = __VLS_asFunctionalComponent1(__VLS_250, new __VLS_250({
    label: "耗时",
    width: "80",
}));
const __VLS_252 = __VLS_251({
    label: "耗时",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_251));
const { default: __VLS_255 } = __VLS_253.slots;
{
    const { default: __VLS_256 } = __VLS_253.slots;
    const [{ row }] = __VLS_vSlot(__VLS_256);
    let __VLS_257;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_258 = __VLS_asFunctionalComponent1(__VLS_257, new __VLS_257({
        size: "small",
    }));
    const __VLS_259 = __VLS_258({
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_258));
    const { default: __VLS_262 } = __VLS_260.slots;
    (((row.endedAt - row.startedAt) / 1000).toFixed(1));
    // @ts-ignore
    [];
    var __VLS_260;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_253;
let __VLS_263;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_264 = __VLS_asFunctionalComponent1(__VLS_263, new __VLS_263({
    label: "状态",
    width: "75",
}));
const __VLS_265 = __VLS_264({
    label: "状态",
    width: "75",
}, ...__VLS_functionalComponentArgsRest(__VLS_264));
const { default: __VLS_268 } = __VLS_266.slots;
{
    const { default: __VLS_269 } = __VLS_266.slots;
    const [{ row }] = __VLS_vSlot(__VLS_269);
    let __VLS_270;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_271 = __VLS_asFunctionalComponent1(__VLS_270, new __VLS_270({
        type: (row.status === 'ok' ? 'success' : 'danger'),
        size: "small",
    }));
    const __VLS_272 = __VLS_271({
        type: (row.status === 'ok' ? 'success' : 'danger'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_271));
    const { default: __VLS_275 } = __VLS_273.slots;
    (row.status);
    // @ts-ignore
    [];
    var __VLS_273;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_266;
let __VLS_276;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_277 = __VLS_asFunctionalComponent1(__VLS_276, new __VLS_276({
    label: "推送",
    width: "60",
}));
const __VLS_278 = __VLS_277({
    label: "推送",
    width: "60",
}, ...__VLS_functionalComponentArgsRest(__VLS_277));
const { default: __VLS_281 } = __VLS_279.slots;
{
    const { default: __VLS_282 } = __VLS_279.slots;
    const [{ row }] = __VLS_vSlot(__VLS_282);
    if (row.announced) {
        let __VLS_283;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_284 = __VLS_asFunctionalComponent1(__VLS_283, new __VLS_283({
            type: "success",
            size: "small",
            effect: "plain",
        }));
        const __VLS_285 = __VLS_284({
            type: "success",
            size: "small",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_284));
        const { default: __VLS_288 } = __VLS_286.slots;
        // @ts-ignore
        [];
        var __VLS_286;
    }
    else {
        let __VLS_289;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_290 = __VLS_asFunctionalComponent1(__VLS_289, new __VLS_289({
            type: "info",
            size: "small",
        }));
        const __VLS_291 = __VLS_290({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_290));
        const { default: __VLS_294 } = __VLS_292.slots;
        // @ts-ignore
        [];
        var __VLS_292;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_279;
let __VLS_295;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_296 = __VLS_asFunctionalComponent1(__VLS_295, new __VLS_295({
    label: "输出 / 错误",
    minWidth: "200",
}));
const __VLS_297 = __VLS_296({
    label: "输出 / 错误",
    minWidth: "200",
}, ...__VLS_functionalComponentArgsRest(__VLS_296));
const { default: __VLS_300 } = __VLS_298.slots;
{
    const { default: __VLS_301 } = __VLS_298.slots;
    const [{ row }] = __VLS_vSlot(__VLS_301);
    if (row.status === 'error') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        (row.error);
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        (row.output || '—');
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_298;
// @ts-ignore
[];
var __VLS_240;
if (!__VLS_ctx.loadingLogs && __VLS_ctx.runLogs.length === 0) {
    let __VLS_302;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_303 = __VLS_asFunctionalComponent1(__VLS_302, new __VLS_302({
        description: "暂无执行记录",
    }));
    const __VLS_304 = __VLS_303({
        description: "暂无执行记录",
    }, ...__VLS_functionalComponentArgsRest(__VLS_303));
}
{
    const { footer: __VLS_307 } = __VLS_220.slots;
    let __VLS_308;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_309 = __VLS_asFunctionalComponent1(__VLS_308, new __VLS_308({
        ...{ 'onClick': {} },
    }));
    const __VLS_310 = __VLS_309({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_309));
    let __VLS_313;
    const __VLS_314 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showLogs = false;
                // @ts-ignore
                [showLogs, loadingLogs, runLogs,];
            } });
    const { default: __VLS_315 } = __VLS_311.slots;
    // @ts-ignore
    [];
    var __VLS_311;
    var __VLS_312;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_220;
let __VLS_316;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_317 = __VLS_asFunctionalComponent1(__VLS_316, new __VLS_316({
    modelValue: (__VLS_ctx.showCreate),
    title: "新建定时任务",
    width: "520px",
}));
const __VLS_318 = __VLS_317({
    modelValue: (__VLS_ctx.showCreate),
    title: "新建定时任务",
    width: "520px",
}, ...__VLS_functionalComponentArgsRest(__VLS_317));
const { default: __VLS_321 } = __VLS_319.slots;
let __VLS_322;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_323 = __VLS_asFunctionalComponent1(__VLS_322, new __VLS_322({
    model: (__VLS_ctx.form),
    labelWidth: "110px",
}));
const __VLS_324 = __VLS_323({
    model: (__VLS_ctx.form),
    labelWidth: "110px",
}, ...__VLS_functionalComponentArgsRest(__VLS_323));
const { default: __VLS_327 } = __VLS_325.slots;
let __VLS_328;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_329 = __VLS_asFunctionalComponent1(__VLS_328, new __VLS_328({
    label: "所属成员",
}));
const __VLS_330 = __VLS_329({
    label: "所属成员",
}, ...__VLS_functionalComponentArgsRest(__VLS_329));
const { default: __VLS_333 } = __VLS_331.slots;
let __VLS_334;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_335 = __VLS_asFunctionalComponent1(__VLS_334, new __VLS_334({
    modelValue: (__VLS_ctx.form.agentId),
    placeholder: "不选则为全局任务",
    clearable: true,
    ...{ style: {} },
}));
const __VLS_336 = __VLS_335({
    modelValue: (__VLS_ctx.form.agentId),
    placeholder: "不选则为全局任务",
    clearable: true,
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_335));
const { default: __VLS_339 } = __VLS_337.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_340;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_341 = __VLS_asFunctionalComponent1(__VLS_340, new __VLS_340({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_342 = __VLS_341({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_341));
    // @ts-ignore
    [agentList, showCreate, form, form,];
}
// @ts-ignore
[];
var __VLS_337;
let __VLS_345;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_346 = __VLS_asFunctionalComponent1(__VLS_345, new __VLS_345({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_347 = __VLS_346({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_346));
const { default: __VLS_350 } = __VLS_348.slots;
let __VLS_351;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_352 = __VLS_asFunctionalComponent1(__VLS_351, new __VLS_351({
    ...{ style: {} },
}));
const __VLS_353 = __VLS_352({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_352));
const { default: __VLS_356 } = __VLS_354.slots;
let __VLS_357;
/** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
InfoFilled;
// @ts-ignore
const __VLS_358 = __VLS_asFunctionalComponent1(__VLS_357, new __VLS_357({}));
const __VLS_359 = __VLS_358({}, ...__VLS_functionalComponentArgsRest(__VLS_358));
// @ts-ignore
[];
var __VLS_354;
// @ts-ignore
[];
var __VLS_348;
// @ts-ignore
[];
var __VLS_331;
let __VLS_362;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_363 = __VLS_asFunctionalComponent1(__VLS_362, new __VLS_362({
    label: "名称",
}));
const __VLS_364 = __VLS_363({
    label: "名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_363));
const { default: __VLS_367 } = __VLS_365.slots;
let __VLS_368;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_369 = __VLS_asFunctionalComponent1(__VLS_368, new __VLS_368({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "任务名称",
}));
const __VLS_370 = __VLS_369({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "任务名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_369));
// @ts-ignore
[form,];
var __VLS_365;
let __VLS_373;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_374 = __VLS_asFunctionalComponent1(__VLS_373, new __VLS_373({
    label: "备注",
}));
const __VLS_375 = __VLS_374({
    label: "备注",
}, ...__VLS_functionalComponentArgsRest(__VLS_374));
const { default: __VLS_378 } = __VLS_376.slots;
let __VLS_379;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_380 = __VLS_asFunctionalComponent1(__VLS_379, new __VLS_379({
    modelValue: (__VLS_ctx.form.remark),
    placeholder: "可选，说明这个任务的用途",
}));
const __VLS_381 = __VLS_380({
    modelValue: (__VLS_ctx.form.remark),
    placeholder: "可选，说明这个任务的用途",
}, ...__VLS_functionalComponentArgsRest(__VLS_380));
// @ts-ignore
[form,];
var __VLS_376;
let __VLS_384;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_385 = __VLS_asFunctionalComponent1(__VLS_384, new __VLS_384({
    label: "Cron 表达式",
}));
const __VLS_386 = __VLS_385({
    label: "Cron 表达式",
}, ...__VLS_functionalComponentArgsRest(__VLS_385));
const { default: __VLS_389 } = __VLS_387.slots;
let __VLS_390;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_391 = __VLS_asFunctionalComponent1(__VLS_390, new __VLS_390({
    modelValue: (__VLS_ctx.form.expr),
    placeholder: "0 9 * * *",
}));
const __VLS_392 = __VLS_391({
    modelValue: (__VLS_ctx.form.expr),
    placeholder: "0 9 * * *",
}, ...__VLS_functionalComponentArgsRest(__VLS_391));
let __VLS_395;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_396 = __VLS_asFunctionalComponent1(__VLS_395, new __VLS_395({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_397 = __VLS_396({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_396));
const { default: __VLS_400 } = __VLS_398.slots;
// @ts-ignore
[form,];
var __VLS_398;
// @ts-ignore
[];
var __VLS_387;
let __VLS_401;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_402 = __VLS_asFunctionalComponent1(__VLS_401, new __VLS_401({
    label: "时区",
}));
const __VLS_403 = __VLS_402({
    label: "时区",
}, ...__VLS_functionalComponentArgsRest(__VLS_402));
const { default: __VLS_406 } = __VLS_404.slots;
let __VLS_407;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_408 = __VLS_asFunctionalComponent1(__VLS_407, new __VLS_407({
    modelValue: (__VLS_ctx.form.tz),
    ...{ style: {} },
}));
const __VLS_409 = __VLS_408({
    modelValue: (__VLS_ctx.form.tz),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_408));
const { default: __VLS_412 } = __VLS_410.slots;
let __VLS_413;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_414 = __VLS_asFunctionalComponent1(__VLS_413, new __VLS_413({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}));
const __VLS_415 = __VLS_414({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}, ...__VLS_functionalComponentArgsRest(__VLS_414));
let __VLS_418;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_419 = __VLS_asFunctionalComponent1(__VLS_418, new __VLS_418({
    label: "UTC",
    value: "UTC",
}));
const __VLS_420 = __VLS_419({
    label: "UTC",
    value: "UTC",
}, ...__VLS_functionalComponentArgsRest(__VLS_419));
let __VLS_423;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_424 = __VLS_asFunctionalComponent1(__VLS_423, new __VLS_423({
    label: "America/New_York",
    value: "America/New_York",
}));
const __VLS_425 = __VLS_424({
    label: "America/New_York",
    value: "America/New_York",
}, ...__VLS_functionalComponentArgsRest(__VLS_424));
// @ts-ignore
[form,];
var __VLS_410;
// @ts-ignore
[];
var __VLS_404;
let __VLS_428;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_429 = __VLS_asFunctionalComponent1(__VLS_428, new __VLS_428({
    label: "消息内容",
}));
const __VLS_430 = __VLS_429({
    label: "消息内容",
}, ...__VLS_functionalComponentArgsRest(__VLS_429));
const { default: __VLS_433 } = __VLS_431.slots;
let __VLS_434;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_435 = __VLS_asFunctionalComponent1(__VLS_434, new __VLS_434({
    modelValue: (__VLS_ctx.form.message),
    type: "textarea",
    rows: (3),
    placeholder: "发送给 Agent 的消息内容",
}));
const __VLS_436 = __VLS_435({
    modelValue: (__VLS_ctx.form.message),
    type: "textarea",
    rows: (3),
    placeholder: "发送给 Agent 的消息内容",
}, ...__VLS_functionalComponentArgsRest(__VLS_435));
// @ts-ignore
[form,];
var __VLS_431;
let __VLS_439;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_440 = __VLS_asFunctionalComponent1(__VLS_439, new __VLS_439({
    label: "启用",
}));
const __VLS_441 = __VLS_440({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_440));
const { default: __VLS_444 } = __VLS_442.slots;
let __VLS_445;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_446 = __VLS_asFunctionalComponent1(__VLS_445, new __VLS_445({
    modelValue: (__VLS_ctx.form.enabled),
}));
const __VLS_447 = __VLS_446({
    modelValue: (__VLS_ctx.form.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_446));
// @ts-ignore
[form,];
var __VLS_442;
// @ts-ignore
[];
var __VLS_325;
{
    const { footer: __VLS_450 } = __VLS_319.slots;
    let __VLS_451;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_452 = __VLS_asFunctionalComponent1(__VLS_451, new __VLS_451({
        ...{ 'onClick': {} },
    }));
    const __VLS_453 = __VLS_452({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_452));
    let __VLS_456;
    const __VLS_457 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showCreate = false;
                // @ts-ignore
                [showCreate,];
            } });
    const { default: __VLS_458 } = __VLS_454.slots;
    // @ts-ignore
    [];
    var __VLS_454;
    var __VLS_455;
    let __VLS_459;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_460 = __VLS_asFunctionalComponent1(__VLS_459, new __VLS_459({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_461 = __VLS_460({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_460));
    let __VLS_464;
    const __VLS_465 = ({ click: {} },
        { onClick: (__VLS_ctx.createCron) });
    const { default: __VLS_466 } = __VLS_462.slots;
    // @ts-ignore
    [createCron,];
    var __VLS_462;
    var __VLS_463;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_319;
let __VLS_467;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_468 = __VLS_asFunctionalComponent1(__VLS_467, new __VLS_467({
    modelValue: (__VLS_ctx.showMorning),
    title: "🌅 晨间例行（一键模板）",
    width: "560px",
}));
const __VLS_469 = __VLS_468({
    modelValue: (__VLS_ctx.showMorning),
    title: "🌅 晨间例行（一键模板）",
    width: "560px",
}, ...__VLS_functionalComponentArgsRest(__VLS_468));
const { default: __VLS_472 } = __VLS_470.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
let __VLS_473;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_474 = __VLS_asFunctionalComponent1(__VLS_473, new __VLS_473({
    labelWidth: "90px",
    size: "default",
}));
const __VLS_475 = __VLS_474({
    labelWidth: "90px",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_474));
const { default: __VLS_478 } = __VLS_476.slots;
let __VLS_479;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_480 = __VLS_asFunctionalComponent1(__VLS_479, new __VLS_479({
    label: "所属成员",
    required: true,
}));
const __VLS_481 = __VLS_480({
    label: "所属成员",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_480));
const { default: __VLS_484 } = __VLS_482.slots;
let __VLS_485;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_486 = __VLS_asFunctionalComponent1(__VLS_485, new __VLS_485({
    modelValue: (__VLS_ctx.morning.agentId),
    placeholder: "选择 AI 成员",
    ...{ style: {} },
}));
const __VLS_487 = __VLS_486({
    modelValue: (__VLS_ctx.morning.agentId),
    placeholder: "选择 AI 成员",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_486));
const { default: __VLS_490 } = __VLS_488.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_491;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_492 = __VLS_asFunctionalComponent1(__VLS_491, new __VLS_491({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_493 = __VLS_492({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_492));
    // @ts-ignore
    [agentList, showMorning, morning,];
}
// @ts-ignore
[];
var __VLS_488;
// @ts-ignore
[];
var __VLS_482;
let __VLS_496;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_497 = __VLS_asFunctionalComponent1(__VLS_496, new __VLS_496({
    label: "时间",
}));
const __VLS_498 = __VLS_497({
    label: "时间",
}, ...__VLS_functionalComponentArgsRest(__VLS_497));
const { default: __VLS_501 } = __VLS_499.slots;
let __VLS_502;
/** @ts-ignore @type { | typeof __VLS_components.elTimePicker | typeof __VLS_components.ElTimePicker | typeof __VLS_components['el-time-picker']} */
elTimePicker;
// @ts-ignore
const __VLS_503 = __VLS_asFunctionalComponent1(__VLS_502, new __VLS_502({
    modelValue: (__VLS_ctx.morning.timeStr),
    format: "HH:mm",
    valueFormat: "HH:mm",
    placeholder: "HH:mm",
    clearable: (false),
    ...{ style: {} },
}));
const __VLS_504 = __VLS_503({
    modelValue: (__VLS_ctx.morning.timeStr),
    format: "HH:mm",
    valueFormat: "HH:mm",
    placeholder: "HH:mm",
    clearable: (false),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_503));
let __VLS_507;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_508 = __VLS_asFunctionalComponent1(__VLS_507, new __VLS_507({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_509 = __VLS_508({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_508));
const { default: __VLS_512 } = __VLS_510.slots;
// @ts-ignore
[morning,];
var __VLS_510;
// @ts-ignore
[];
var __VLS_499;
let __VLS_513;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_514 = __VLS_asFunctionalComponent1(__VLS_513, new __VLS_513({
    label: "时区",
}));
const __VLS_515 = __VLS_514({
    label: "时区",
}, ...__VLS_functionalComponentArgsRest(__VLS_514));
const { default: __VLS_518 } = __VLS_516.slots;
let __VLS_519;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_520 = __VLS_asFunctionalComponent1(__VLS_519, new __VLS_519({
    modelValue: (__VLS_ctx.morning.tz),
    ...{ style: {} },
}));
const __VLS_521 = __VLS_520({
    modelValue: (__VLS_ctx.morning.tz),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_520));
const { default: __VLS_524 } = __VLS_522.slots;
let __VLS_525;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_526 = __VLS_asFunctionalComponent1(__VLS_525, new __VLS_525({
    label: "Asia/Shanghai（UTC+8）",
    value: "Asia/Shanghai",
}));
const __VLS_527 = __VLS_526({
    label: "Asia/Shanghai（UTC+8）",
    value: "Asia/Shanghai",
}, ...__VLS_functionalComponentArgsRest(__VLS_526));
let __VLS_530;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_531 = __VLS_asFunctionalComponent1(__VLS_530, new __VLS_530({
    label: "UTC",
    value: "UTC",
}));
const __VLS_532 = __VLS_531({
    label: "UTC",
    value: "UTC",
}, ...__VLS_functionalComponentArgsRest(__VLS_531));
let __VLS_535;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_536 = __VLS_asFunctionalComponent1(__VLS_535, new __VLS_535({
    label: "America/New_York",
    value: "America/New_York",
}));
const __VLS_537 = __VLS_536({
    label: "America/New_York",
    value: "America/New_York",
}, ...__VLS_functionalComponentArgsRest(__VLS_536));
let __VLS_540;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_541 = __VLS_asFunctionalComponent1(__VLS_540, new __VLS_540({
    label: "Europe/London",
    value: "Europe/London",
}));
const __VLS_542 = __VLS_541({
    label: "Europe/London",
    value: "Europe/London",
}, ...__VLS_functionalComponentArgsRest(__VLS_541));
// @ts-ignore
[morning,];
var __VLS_522;
// @ts-ignore
[];
var __VLS_516;
// @ts-ignore
[];
var __VLS_476;
{
    const { footer: __VLS_545 } = __VLS_470.slots;
    let __VLS_546;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_547 = __VLS_asFunctionalComponent1(__VLS_546, new __VLS_546({
        ...{ 'onClick': {} },
    }));
    const __VLS_548 = __VLS_547({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_547));
    let __VLS_551;
    const __VLS_552 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showMorning = false;
                // @ts-ignore
                [showMorning,];
            } });
    const { default: __VLS_553 } = __VLS_549.slots;
    // @ts-ignore
    [];
    var __VLS_549;
    var __VLS_550;
    let __VLS_554;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_555 = __VLS_asFunctionalComponent1(__VLS_554, new __VLS_554({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_556 = __VLS_555({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_555));
    let __VLS_559;
    const __VLS_560 = ({ click: {} },
        { onClick: (__VLS_ctx.createMorningRoutine) });
    const { default: __VLS_561 } = __VLS_557.slots;
    // @ts-ignore
    [createMorningRoutine,];
    var __VLS_557;
    var __VLS_558;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_470;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
