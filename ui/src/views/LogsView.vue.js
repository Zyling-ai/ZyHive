/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue';
import { Search, Refresh } from '@element-plus/icons-vue';
import { logsApi } from '../api';
const lines = ref([]);
const keyword = ref('');
const loading = ref(false);
const autoScroll = ref(true);
const logContainer = ref(null);
let timer = null;
const filteredLines = computed(() => {
    if (!keyword.value)
        return lines.value;
    const kw = keyword.value.toLowerCase();
    return lines.value.filter(l => l.toLowerCase().includes(kw));
});
function logLevel(line) {
    const u = line.toUpperCase();
    if (u.includes('ERROR') || u.includes('[ERR]') || u.includes('FATAL'))
        return 'level-error';
    if (u.includes('WARN') || u.includes('[WARN]') || u.includes('WARNING'))
        return 'level-warn';
    if (u.includes('INFO') || u.includes('[INFO]'))
        return 'level-info';
    return 'level-default';
}
async function fetchLogs() {
    loading.value = true;
    try {
        const res = await logsApi.get(500);
        lines.value = res.data.lines ?? [];
        if (autoScroll.value) {
            await nextTick();
            scrollToBottom();
        }
    }
    catch {
        // silently fail — log file may not exist yet
    }
    finally {
        loading.value = false;
    }
}
function scrollToBottom() {
    const el = logContainer.value;
    if (el)
        el.scrollTop = el.scrollHeight;
}
watch(autoScroll, (val) => {
    if (val)
        scrollToBottom();
});
watch(filteredLines, async () => {
    if (autoScroll.value) {
        await nextTick();
        scrollToBottom();
    }
});
onMounted(() => {
    fetchLogs();
    timer = setInterval(fetchLogs, 5000);
});
onUnmounted(() => {
    if (timer)
        clearInterval(timer);
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['log-card']} */ ;
/** @type {__VLS_StyleScopedClasses['log-text']} */ ;
/** @type {__VLS_StyleScopedClasses['log-text']} */ ;
/** @type {__VLS_StyleScopedClasses['log-text']} */ ;
/** @type {__VLS_StyleScopedClasses['log-text']} */ ;
/** @type {__VLS_StyleScopedClasses['log-empty']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "logs-page" },
});
/** @type {__VLS_StyleScopedClasses['logs-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "logs-header" },
});
/** @type {__VLS_StyleScopedClasses['logs-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
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
/** @ts-ignore @type { | typeof __VLS_components.List} */
List;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
(__VLS_ctx.filteredLines.length);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_11;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
    modelValue: (__VLS_ctx.keyword),
    placeholder: "关键词过滤…",
    clearable: true,
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}));
const __VLS_13 = __VLS_12({
    modelValue: (__VLS_ctx.keyword),
    placeholder: "关键词过滤…",
    clearable: true,
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}, ...__VLS_functionalComponentArgsRest(__VLS_12));
let __VLS_16;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_17 = __VLS_asFunctionalComponent1(__VLS_16, new __VLS_16({
    modelValue: (__VLS_ctx.autoScroll),
    activeText: "自动滚动",
}));
const __VLS_18 = __VLS_17({
    modelValue: (__VLS_ctx.autoScroll),
    activeText: "自动滚动",
}, ...__VLS_functionalComponentArgsRest(__VLS_17));
let __VLS_21;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_22 = __VLS_asFunctionalComponent1(__VLS_21, new __VLS_21({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}));
const __VLS_23 = __VLS_22({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}, ...__VLS_functionalComponentArgsRest(__VLS_22));
let __VLS_26;
const __VLS_27 = ({ click: {} },
    { onClick: (__VLS_ctx.fetchLogs) });
const { default: __VLS_28 } = __VLS_24.slots;
// @ts-ignore
[filteredLines, keyword, Search, autoScroll, loading, Refresh, fetchLogs,];
var __VLS_24;
var __VLS_25;
let __VLS_29;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_30 = __VLS_asFunctionalComponent1(__VLS_29, new __VLS_29({
    shadow: "never",
    ...{ class: "log-card" },
}));
const __VLS_31 = __VLS_30({
    shadow: "never",
    ...{ class: "log-card" },
}, ...__VLS_functionalComponentArgsRest(__VLS_30));
/** @type {__VLS_StyleScopedClasses['log-card']} */ ;
const { default: __VLS_34 } = __VLS_32.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ref: "logContainer",
    ...{ class: "log-container" },
});
/** @type {__VLS_StyleScopedClasses['log-container']} */ ;
for (const [line, idx] of __VLS_vFor((__VLS_ctx.filteredLines))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        key: (idx),
        ...{ class: (['log-line', __VLS_ctx.logLevel(line)]) },
    });
    /** @type {__VLS_StyleScopedClasses['log-line']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "log-text" },
    });
    /** @type {__VLS_StyleScopedClasses['log-text']} */ ;
    (line);
    // @ts-ignore
    [filteredLines, logLevel,];
}
if (__VLS_ctx.filteredLines.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "log-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['log-empty']} */ ;
    let __VLS_35;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_36 = __VLS_asFunctionalComponent1(__VLS_35, new __VLS_35({
        description: (__VLS_ctx.keyword ? '无匹配日志' : '暂无日志内容'),
    }));
    const __VLS_37 = __VLS_36({
        description: (__VLS_ctx.keyword ? '无匹配日志' : '暂无日志内容'),
    }, ...__VLS_functionalComponentArgsRest(__VLS_36));
}
// @ts-ignore
[filteredLines, keyword,];
var __VLS_32;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
