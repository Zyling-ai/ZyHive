/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { usageApi, agents as agentsApi, sessions as sessionsApi } from '../api';
import * as echarts from 'echarts/core';
import { LineChart, PieChart, BarChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
echarts.use([LineChart, PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer]);
// ── state ──────────────────────────────────────────────────────────
const loading = ref(false);
const loadingRecords = ref(false);
const dateRange = ref(null);
const filterProvider = ref('');
const filterAgent = ref('');
const filterSession = ref('');
const summary = ref({});
const timeline = ref([]);
const records = ref([]);
const totalRecords = ref(0);
const page = ref(1);
const pageSize = ref(50);
// All agents, for id→name map + filter dropdown
const allAgents = ref([]);
const agentNameMap = computed(() => {
    const m = {};
    for (const a of allAgents.value)
        m[a.id] = a.name;
    return m;
});
// Sessions for the currently-selected agent (for session filter dropdown)
const agentSessions = ref([]);
const timelineChartEl = ref(null);
const providerChartEl = ref(null);
const agentChartEl = ref(null);
let timelineChart = null;
let providerChart = null;
let agentChart = null;
// ── shortcuts ──────────────────────────────────────────────────────
const dateShortcuts = [
    { text: '今天', value: () => { const n = new Date(); n.setHours(0, 0, 0, 0); return [n, new Date()]; } },
    { text: '最近7天', value: () => [new Date(Date.now() - 7 * 86400_000), new Date()] },
    { text: '最近30天', value: () => [new Date(Date.now() - 30 * 86400_000), new Date()] },
    { text: '本月', value: () => { const n = new Date(); return [new Date(n.getFullYear(), n.getMonth(), 1), n]; } },
];
// ── filter options ─────────────────────────────────────────────────
const providerOptions = computed(() => Object.keys(summary.value?.by_provider ?? {}));
// agent dropdown uses real display names; fall back to IDs that have usage
// but aren't registered (deleted agents) for completeness.
const agentOptions = computed(() => {
    const ids = new Set(Object.keys(summary.value?.by_agent ?? {}));
    for (const a of allAgents.value)
        ids.add(a.id);
    const out = [];
    for (const id of ids) {
        out.push({ id, name: agentNameMap.value[id] || id });
    }
    out.sort((a, b) => a.name.localeCompare(b.name));
    return out;
});
const sessionOptions = computed(() => {
    // Only show sessions belonging to the selected agent (otherwise too many)
    if (!filterAgent.value)
        return [];
    return (agentSessions.value || []).map(s => ({
        id: s.id,
        label: (s.title || s.id) + '  ·  ' + (s.messageCount || 0) + ' 条',
    }));
});
function shortSid(sid) {
    if (!sid)
        return '';
    if (sid.length > 24)
        return sid.slice(0, 20) + '…';
    return sid;
}
// ── params helper ──────────────────────────────────────────────────
function buildParams() {
    const p = {};
    if (dateRange.value) {
        p.from = Math.floor(Number(dateRange.value[0]) / 1000);
        p.to = Math.floor(Number(dateRange.value[1]) / 1000);
    }
    if (filterProvider.value)
        p.provider = filterProvider.value;
    if (filterAgent.value)
        p.agentId = filterAgent.value;
    if (filterSession.value)
        p.sessionId = filterSession.value;
    return p;
}
async function loadAgentList() {
    try {
        const res = await agentsApi.list();
        allAgents.value = res.data.map(a => ({ id: a.id, name: a.name || a.id }));
    }
    catch {
        allAgents.value = [];
    }
}
async function loadAgentSessions() {
    if (!filterAgent.value) {
        agentSessions.value = [];
        return;
    }
    try {
        const res = await sessionsApi.list({ agentId: filterAgent.value, limit: 200 });
        const d = res.data;
        agentSessions.value = (d?.sessions ?? d ?? []);
    }
    catch {
        agentSessions.value = [];
    }
}
// When agent filter changes, load its sessions for the session dropdown and
// clear any stale session selection.
watch(filterAgent, () => {
    filterSession.value = '';
    loadAgentSessions();
});
// ── load ───────────────────────────────────────────────────────────
async function loadSummary() {
    const res = await usageApi.summary(buildParams());
    summary.value = res.data;
}
async function loadTimeline() {
    const res = await usageApi.timeline(buildParams());
    timeline.value = res.data.points ?? [];
}
async function loadRecords() {
    loadingRecords.value = true;
    try {
        const res = await usageApi.records({ ...buildParams(), page: page.value, pageSize: pageSize.value });
        const d = res.data;
        records.value = d.records ?? [];
        totalRecords.value = d.total ?? 0;
    }
    finally {
        loadingRecords.value = false;
    }
}
async function load() {
    loading.value = true;
    try {
        await Promise.all([loadSummary(), loadTimeline(), loadRecords()]);
        await nextTick();
        renderCharts();
    }
    finally {
        loading.value = false;
    }
}
// ── charts ─────────────────────────────────────────────────────────
function initCharts() {
    if (timelineChartEl.value && !timelineChart)
        timelineChart = echarts.init(timelineChartEl.value);
    if (providerChartEl.value && !providerChart)
        providerChart = echarts.init(providerChartEl.value);
    if (agentChartEl.value && !agentChart)
        agentChart = echarts.init(agentChartEl.value);
}
function renderCharts() {
    initCharts();
    // Timeline bar+line
    const pts = timeline.value;
    timelineChart?.setOption({
        tooltip: { trigger: 'axis' },
        legend: { data: ['调用次数', '花费(USD)'], top: 2, textStyle: { fontSize: 11 } },
        grid: { left: 44, right: 56, top: 36, bottom: 28 },
        xAxis: { type: 'category', data: pts.map((p) => p.date), axisLabel: { fontSize: 10 } },
        yAxis: [
            { type: 'value', name: '次数', nameTextStyle: { fontSize: 10 } },
            { type: 'value', name: 'USD', nameTextStyle: { fontSize: 10 }, axisLabel: { fontSize: 10, formatter: (v) => '$' + v.toFixed(3) } },
        ],
        series: [
            { name: '调用次数', type: 'bar', data: pts.map((p) => p.calls), itemStyle: { color: '#6366f1' } },
            { name: '花费(USD)', type: 'line', yAxisIndex: 1, smooth: true,
                data: pts.map((p) => +(p.cost ?? 0).toFixed(5)), itemStyle: { color: '#f59e0b' }, symbol: 'circle', symbolSize: 4 },
        ],
    }, true);
    // Provider pie
    renderPie(providerChart, summary.value?.by_provider ?? {});
    renderPie(agentChart, summary.value?.by_agent ?? {});
}
function renderPie(chart, map) {
    if (!chart)
        return;
    const data = Object.entries(map).map(([name, s]) => ({
        name, value: s.calls,
        extra: `$${(s.cost ?? 0).toFixed(4)} | ${fmtTokens((s.input_tokens ?? 0) + (s.output_tokens ?? 0))} tokens`
    }));
    chart.setOption({
        tooltip: {
            trigger: 'item',
            formatter: (p) => `${p.name}<br/>调用: ${p.value}<br/>${p.data.extra}`,
        },
        legend: {
            orient: 'vertical',
            right: 8,
            top: 'middle',
            itemGap: 6,
            itemWidth: 10,
            itemHeight: 10,
            textStyle: { fontSize: 11, color: '#475569' },
            // 条目太多时允许滚动 (不再挤成一团叠在饼图上)
            type: 'scroll',
            pageIconSize: 10,
            pageTextStyle: { fontSize: 10 },
        },
        series: [{
                type: 'pie',
                radius: ['38%', '62%'],
                // 给左侧饼图更多空间, 不让 label 和 legend 重叠
                center: ['30%', '50%'],
                data,
                label: { show: false },
                labelLine: { show: false },
                itemStyle: { borderColor: '#fff', borderWidth: 1 },
                emphasis: {
                    label: { show: true, fontSize: 11, fontWeight: 600 },
                    scaleSize: 6,
                },
            }],
    }, true);
}
// ── utils ──────────────────────────────────────────────────────────
function fmtTokens(n) {
    if (!n)
        return '0';
    if (n >= 1_000_000)
        return (n / 1_000_000).toFixed(2) + 'M';
    if (n >= 1_000)
        return (n / 1_000).toFixed(1) + 'K';
    return String(n);
}
function providerColor(p) {
    const m = {
        anthropic: 'warning', openai: 'success', deepseek: 'primary',
        minimax: 'info', moonshot: 'info', zhipu: 'info',
    };
    return m[p] ?? '';
}
// ── lifecycle ──────────────────────────────────────────────────────
onMounted(async () => {
    dateRange.value = [Date.now() - 30 * 86400_000, Date.now()];
    await loadAgentList();
    await load();
    window.addEventListener('resize', onResize);
});
onUnmounted(() => {
    window.removeEventListener('resize', onResize);
    timelineChart?.dispose();
    providerChart?.dispose();
    agentChart?.dispose();
});
function onResize() {
    timelineChart?.resize();
    providerChart?.resize();
    agentChart?.resize();
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['highlight']} */ ;
/** @type {__VLS_StyleScopedClasses['chart-card']} */ ;
/** @type {__VLS_StyleScopedClasses['chart-card-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['records-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-cards']} */ ;
/** @type {__VLS_StyleScopedClasses['usage-charts']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "usage-studio" },
});
/** @type {__VLS_StyleScopedClasses['usage-studio']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "usage-filter" },
});
/** @type {__VLS_StyleScopedClasses['usage-filter']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elSpace | typeof __VLS_components.ElSpace | typeof __VLS_components['el-space'] | typeof __VLS_components.elSpace | typeof __VLS_components.ElSpace | typeof __VLS_components['el-space']} */
elSpace;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    wrap: true,
}));
const __VLS_2 = __VLS_1({
    wrap: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
let __VLS_6;
/** @ts-ignore @type { | typeof __VLS_components.elDatePicker | typeof __VLS_components.ElDatePicker | typeof __VLS_components['el-date-picker']} */
elDatePicker;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.dateRange),
    type: "daterange",
    rangeSeparator: "~",
    startPlaceholder: "开始日期",
    endPlaceholder: "结束日期",
    shortcuts: (__VLS_ctx.dateShortcuts),
    valueFormat: "x",
    ...{ style: {} },
}));
const __VLS_8 = __VLS_7({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.dateRange),
    type: "daterange",
    rangeSeparator: "~",
    startPlaceholder: "开始日期",
    endPlaceholder: "结束日期",
    shortcuts: (__VLS_ctx.dateShortcuts),
    valueFormat: "x",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_7));
let __VLS_11;
const __VLS_12 = ({ change: {} },
    { onChange: (__VLS_ctx.load) });
var __VLS_9;
var __VLS_10;
let __VLS_13;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_14 = __VLS_asFunctionalComponent1(__VLS_13, new __VLS_13({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterProvider),
    clearable: true,
    placeholder: "全部厂商",
    ...{ style: {} },
}));
const __VLS_15 = __VLS_14({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterProvider),
    clearable: true,
    placeholder: "全部厂商",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_14));
let __VLS_18;
const __VLS_19 = ({ change: {} },
    { onChange: (__VLS_ctx.load) });
const { default: __VLS_20 } = __VLS_16.slots;
for (const [p] of __VLS_vFor((__VLS_ctx.providerOptions))) {
    let __VLS_21;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_22 = __VLS_asFunctionalComponent1(__VLS_21, new __VLS_21({
        key: (p),
        label: (p),
        value: (p),
    }));
    const __VLS_23 = __VLS_22({
        key: (p),
        label: (p),
        value: (p),
    }, ...__VLS_functionalComponentArgsRest(__VLS_22));
    // @ts-ignore
    [dateRange, dateShortcuts, load, load, filterProvider, providerOptions,];
}
// @ts-ignore
[];
var __VLS_16;
var __VLS_17;
let __VLS_26;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_27 = __VLS_asFunctionalComponent1(__VLS_26, new __VLS_26({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgent),
    clearable: true,
    placeholder: "全部成员",
    ...{ style: {} },
}));
const __VLS_28 = __VLS_27({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgent),
    clearable: true,
    placeholder: "全部成员",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_27));
let __VLS_31;
const __VLS_32 = ({ change: {} },
    { onChange: (__VLS_ctx.load) });
const { default: __VLS_33 } = __VLS_29.slots;
for (const [a] of __VLS_vFor((__VLS_ctx.agentOptions))) {
    let __VLS_34;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_35 = __VLS_asFunctionalComponent1(__VLS_34, new __VLS_34({
        key: (a.id),
        label: (a.name),
        value: (a.id),
    }));
    const __VLS_36 = __VLS_35({
        key: (a.id),
        label: (a.name),
        value: (a.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_35));
    // @ts-ignore
    [load, filterAgent, agentOptions,];
}
// @ts-ignore
[];
var __VLS_29;
var __VLS_30;
let __VLS_39;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_40 = __VLS_asFunctionalComponent1(__VLS_39, new __VLS_39({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterSession),
    clearable: true,
    filterable: true,
    placeholder: "全部 Session",
    ...{ style: {} },
}));
const __VLS_41 = __VLS_40({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterSession),
    clearable: true,
    filterable: true,
    placeholder: "全部 Session",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_40));
let __VLS_44;
const __VLS_45 = ({ change: {} },
    { onChange: (() => { __VLS_ctx.page = 1; __VLS_ctx.loadRecords(); }) });
const { default: __VLS_46 } = __VLS_42.slots;
for (const [s] of __VLS_vFor((__VLS_ctx.sessionOptions))) {
    let __VLS_47;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_48 = __VLS_asFunctionalComponent1(__VLS_47, new __VLS_47({
        key: (s.id),
        label: (s.label),
        value: (s.id),
    }));
    const __VLS_49 = __VLS_48({
        key: (s.id),
        label: (s.label),
        value: (s.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_48));
    // @ts-ignore
    [filterSession, page, loadRecords, sessionOptions,];
}
// @ts-ignore
[];
var __VLS_42;
var __VLS_43;
let __VLS_52;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_53 = __VLS_asFunctionalComponent1(__VLS_52, new __VLS_52({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}));
const __VLS_54 = __VLS_53({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}, ...__VLS_functionalComponentArgsRest(__VLS_53));
let __VLS_57;
const __VLS_58 = ({ click: {} },
    { onClick: (__VLS_ctx.load) });
const { default: __VLS_59 } = __VLS_55.slots;
// @ts-ignore
[load, loading, Refresh,];
var __VLS_55;
var __VLS_56;
// @ts-ignore
[];
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-cards" },
});
/** @type {__VLS_StyleScopedClasses['stat-cards']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-card" },
});
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
((__VLS_ctx.summary.total_calls ?? 0).toLocaleString());
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-card" },
});
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.fmtTokens(__VLS_ctx.summary.input_tokens ?? 0));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-card" },
});
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.fmtTokens(__VLS_ctx.summary.output_tokens ?? 0));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-card highlight" },
});
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['highlight']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
((__VLS_ctx.summary.total_cost ?? 0).toFixed(4));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "usage-charts" },
});
/** @type {__VLS_StyleScopedClasses['usage-charts']} */ ;
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    ...{ class: "chart-card" },
    shadow: "never",
}));
const __VLS_62 = __VLS_61({
    ...{ class: "chart-card" },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
/** @type {__VLS_StyleScopedClasses['chart-card']} */ ;
const { default: __VLS_65 } = __VLS_63.slots;
{
    const { header: __VLS_66 } = __VLS_63.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "card-title" },
    });
    /** @type {__VLS_StyleScopedClasses['card-title']} */ ;
    // @ts-ignore
    [summary, summary, summary, summary, fmtTokens, fmtTokens,];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ref: "timelineChartEl",
    ...{ class: "chart-area" },
});
/** @type {__VLS_StyleScopedClasses['chart-area']} */ ;
// @ts-ignore
[];
var __VLS_63;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "pie-col" },
});
/** @type {__VLS_StyleScopedClasses['pie-col']} */ ;
let __VLS_67;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_68 = __VLS_asFunctionalComponent1(__VLS_67, new __VLS_67({
    ...{ class: "chart-card-sm" },
    shadow: "never",
}));
const __VLS_69 = __VLS_68({
    ...{ class: "chart-card-sm" },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_68));
/** @type {__VLS_StyleScopedClasses['chart-card-sm']} */ ;
const { default: __VLS_72 } = __VLS_70.slots;
{
    const { header: __VLS_73 } = __VLS_70.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "card-title" },
    });
    /** @type {__VLS_StyleScopedClasses['card-title']} */ ;
    // @ts-ignore
    [];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ref: "providerChartEl",
    ...{ class: "chart-area-sm" },
});
/** @type {__VLS_StyleScopedClasses['chart-area-sm']} */ ;
// @ts-ignore
[];
var __VLS_70;
let __VLS_74;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({
    ...{ class: "chart-card-sm" },
    shadow: "never",
}));
const __VLS_76 = __VLS_75({
    ...{ class: "chart-card-sm" },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_75));
/** @type {__VLS_StyleScopedClasses['chart-card-sm']} */ ;
const { default: __VLS_79 } = __VLS_77.slots;
{
    const { header: __VLS_80 } = __VLS_77.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "card-title" },
    });
    /** @type {__VLS_StyleScopedClasses['card-title']} */ ;
    // @ts-ignore
    [];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ref: "agentChartEl",
    ...{ class: "chart-area-sm" },
});
/** @type {__VLS_StyleScopedClasses['chart-area-sm']} */ ;
// @ts-ignore
[];
var __VLS_77;
let __VLS_81;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({
    ...{ class: "records-card" },
    shadow: "never",
}));
const __VLS_83 = __VLS_82({
    ...{ class: "records-card" },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_82));
/** @type {__VLS_StyleScopedClasses['records-card']} */ ;
const { default: __VLS_86 } = __VLS_84.slots;
{
    const { header: __VLS_87 } = __VLS_84.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "card-title" },
    });
    /** @type {__VLS_StyleScopedClasses['card-title']} */ ;
    // @ts-ignore
    [];
}
let __VLS_88;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_89 = __VLS_asFunctionalComponent1(__VLS_88, new __VLS_88({
    data: (__VLS_ctx.records),
    size: "small",
    stripe: true,
    ...{ style: {} },
}));
const __VLS_90 = __VLS_89({
    data: (__VLS_ctx.records),
    size: "small",
    stripe: true,
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_89));
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.loadingRecords) }, null, null);
const { default: __VLS_93 } = __VLS_91.slots;
let __VLS_94;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_95 = __VLS_asFunctionalComponent1(__VLS_94, new __VLS_94({
    prop: "created_at",
    label: "时间",
    width: "160",
    formatter: ((r) => new Date(r.created_at * 1000).toLocaleString('zh-CN')),
}));
const __VLS_96 = __VLS_95({
    prop: "created_at",
    label: "时间",
    width: "160",
    formatter: ((r) => new Date(r.created_at * 1000).toLocaleString('zh-CN')),
}, ...__VLS_functionalComponentArgsRest(__VLS_95));
let __VLS_99;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_100 = __VLS_asFunctionalComponent1(__VLS_99, new __VLS_99({
    label: "成员",
    width: "140",
    showOverflowTooltip: true,
}));
const __VLS_101 = __VLS_100({
    label: "成员",
    width: "140",
    showOverflowTooltip: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_100));
const { default: __VLS_104 } = __VLS_102.slots;
{
    const { default: __VLS_105 } = __VLS_102.slots;
    const [{ row }] = __VLS_vSlot(__VLS_105);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "col-agent-name" },
    });
    /** @type {__VLS_StyleScopedClasses['col-agent-name']} */ ;
    (row.agentName || row.agent_id);
    if (row.agentName) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "col-agent-id" },
        });
        /** @type {__VLS_StyleScopedClasses['col-agent-id']} */ ;
        (row.agent_id);
    }
    // @ts-ignore
    [records, vLoading, loadingRecords,];
}
// @ts-ignore
[];
var __VLS_102;
let __VLS_106;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_107 = __VLS_asFunctionalComponent1(__VLS_106, new __VLS_106({
    label: "Session",
    minWidth: "200",
    showOverflowTooltip: true,
}));
const __VLS_108 = __VLS_107({
    label: "Session",
    minWidth: "200",
    showOverflowTooltip: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_107));
const { default: __VLS_111 } = __VLS_109.slots;
{
    const { default: __VLS_112 } = __VLS_109.slots;
    const [{ row }] = __VLS_vSlot(__VLS_112);
    if (row.sessionTitle) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "col-session" },
        });
        /** @type {__VLS_StyleScopedClasses['col-session']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "col-session-title" },
        });
        /** @type {__VLS_StyleScopedClasses['col-session-title']} */ ;
        (row.sessionTitle);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "col-session-id" },
        });
        /** @type {__VLS_StyleScopedClasses['col-session-id']} */ ;
        (__VLS_ctx.shortSid(row.session_id));
    }
    else if (row.session_id) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "col-session-id" },
        });
        /** @type {__VLS_StyleScopedClasses['col-session-id']} */ ;
        (__VLS_ctx.shortSid(row.session_id));
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "col-session-none" },
        });
        /** @type {__VLS_StyleScopedClasses['col-session-none']} */ ;
    }
    // @ts-ignore
    [shortSid, shortSid,];
}
// @ts-ignore
[];
var __VLS_109;
let __VLS_113;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_114 = __VLS_asFunctionalComponent1(__VLS_113, new __VLS_113({
    prop: "provider",
    label: "厂商",
    width: "110",
}));
const __VLS_115 = __VLS_114({
    prop: "provider",
    label: "厂商",
    width: "110",
}, ...__VLS_functionalComponentArgsRest(__VLS_114));
const { default: __VLS_118 } = __VLS_116.slots;
{
    const { default: __VLS_119 } = __VLS_116.slots;
    const [{ row }] = __VLS_vSlot(__VLS_119);
    let __VLS_120;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_121 = __VLS_asFunctionalComponent1(__VLS_120, new __VLS_120({
        type: (__VLS_ctx.providerColor(row.provider)),
        size: "small",
    }));
    const __VLS_122 = __VLS_121({
        type: (__VLS_ctx.providerColor(row.provider)),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_121));
    const { default: __VLS_125 } = __VLS_123.slots;
    (row.provider);
    // @ts-ignore
    [providerColor,];
    var __VLS_123;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_116;
let __VLS_126;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_127 = __VLS_asFunctionalComponent1(__VLS_126, new __VLS_126({
    prop: "model",
    label: "模型",
    width: "220",
    showOverflowTooltip: true,
}));
const __VLS_128 = __VLS_127({
    prop: "model",
    label: "模型",
    width: "220",
    showOverflowTooltip: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_127));
let __VLS_131;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_132 = __VLS_asFunctionalComponent1(__VLS_131, new __VLS_131({
    prop: "input_tokens",
    label: "输入 Token",
    width: "110",
    formatter: ((r) => __VLS_ctx.fmtTokens(r.input_tokens)),
}));
const __VLS_133 = __VLS_132({
    prop: "input_tokens",
    label: "输入 Token",
    width: "110",
    formatter: ((r) => __VLS_ctx.fmtTokens(r.input_tokens)),
}, ...__VLS_functionalComponentArgsRest(__VLS_132));
let __VLS_136;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_137 = __VLS_asFunctionalComponent1(__VLS_136, new __VLS_136({
    prop: "output_tokens",
    label: "输出 Token",
    width: "110",
    formatter: ((r) => __VLS_ctx.fmtTokens(r.output_tokens)),
}));
const __VLS_138 = __VLS_137({
    prop: "output_tokens",
    label: "输出 Token",
    width: "110",
    formatter: ((r) => __VLS_ctx.fmtTokens(r.output_tokens)),
}, ...__VLS_functionalComponentArgsRest(__VLS_137));
let __VLS_141;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_142 = __VLS_asFunctionalComponent1(__VLS_141, new __VLS_141({
    prop: "cost",
    label: "费用 (USD)",
    width: "110",
    formatter: ((r) => '$' + (r.cost ?? 0).toFixed(5)),
}));
const __VLS_143 = __VLS_142({
    prop: "cost",
    label: "费用 (USD)",
    width: "110",
    formatter: ((r) => '$' + (r.cost ?? 0).toFixed(5)),
}, ...__VLS_functionalComponentArgsRest(__VLS_142));
// @ts-ignore
[fmtTokens, fmtTokens,];
var __VLS_91;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "table-pagination" },
});
/** @type {__VLS_StyleScopedClasses['table-pagination']} */ ;
let __VLS_146;
/** @ts-ignore @type { | typeof __VLS_components.elPagination | typeof __VLS_components.ElPagination | typeof __VLS_components['el-pagination']} */
elPagination;
// @ts-ignore
const __VLS_147 = __VLS_asFunctionalComponent1(__VLS_146, new __VLS_146({
    ...{ 'onCurrentChange': {} },
    ...{ 'onSizeChange': {} },
    currentPage: (__VLS_ctx.page),
    pageSize: (__VLS_ctx.pageSize),
    total: (__VLS_ctx.totalRecords),
    pageSizes: ([20, 50, 100]),
    layout: "total, sizes, prev, pager, next",
    background: true,
    small: true,
}));
const __VLS_148 = __VLS_147({
    ...{ 'onCurrentChange': {} },
    ...{ 'onSizeChange': {} },
    currentPage: (__VLS_ctx.page),
    pageSize: (__VLS_ctx.pageSize),
    total: (__VLS_ctx.totalRecords),
    pageSizes: ([20, 50, 100]),
    layout: "total, sizes, prev, pager, next",
    background: true,
    small: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_147));
let __VLS_151;
const __VLS_152 = ({ currentChange: {} },
    { onCurrentChange: (__VLS_ctx.loadRecords) });
const __VLS_153 = ({ sizeChange: {} },
    { onSizeChange: (() => { __VLS_ctx.page = 1; __VLS_ctx.loadRecords(); }) });
var __VLS_149;
var __VLS_150;
// @ts-ignore
[page, page, loadRecords, loadRecords, pageSize, totalRecords,];
var __VLS_84;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
