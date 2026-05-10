/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, onMounted } from 'vue';
import { useAgentsStore } from '../stores/agents';
import { statsApi, models as modelsApi } from '../api';
const agentStore = useAgentsStore();
const stats = ref(null);
const modelCount = ref(-1); // -1 = 未加载
const modelsLoading = ref(true);
const defaultModelFailed = ref(false); // 默认模型连接失败（403 / error）
const defaultModelName = ref('');
onMounted(async () => {
    agentStore.fetchAll();
    // 并行拉取
    await Promise.allSettled([
        statsApi.get().then(r => { stats.value = r.data; }).catch(() => { }),
        modelsApi.list().then(r => {
            const list = r.data ?? [];
            modelCount.value = list.length;
            const def = list.find((m) => m.isDefault) ?? list[0];
            if (def && def.status === 'error') {
                defaultModelFailed.value = true;
                defaultModelName.value = def.name || def.provider;
            }
        }).catch(() => { modelCount.value = 0; }),
    ]);
    modelsLoading.value = false;
});
function statusType(s) {
    return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info';
}
function statusLabel(s) {
    return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲';
}
function formatTokens(n) {
    if (!n)
        return '0';
    if (n >= 1_000_000)
        return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1000)
        return `${(n / 1000).toFixed(1)}k`;
    return String(n);
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['warn-model-banner']} */ ;
/** @type {__VLS_StyleScopedClasses['warn-model-banner']} */ ;
/** @type {__VLS_StyleScopedClasses['warn-model-banner']} */ ;
/** @type {__VLS_StyleScopedClasses['warn-model-banner']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-banner-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-banner-title']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-banner-desc']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-banner-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['no-model-banner-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['el-card__body']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "dashboard-page" },
});
/** @type {__VLS_StyleScopedClasses['dashboard-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ style: {} },
});
if (!__VLS_ctx.modelsLoading && __VLS_ctx.defaultModelFailed) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner warn-model-banner" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner']} */ ;
    /** @type {__VLS_StyleScopedClasses['warn-model-banner']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-left" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-left']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "no-model-banner-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-title" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-title']} */ ;
    (__VLS_ctx.defaultModelName);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-desc" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-desc']} */ ;
    let __VLS_0;
    /** @ts-ignore @type { | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link'] | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link']} */
    routerLink;
    // @ts-ignore
    const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
        to: "/config/models",
        ...{ class: "no-model-banner-btn" },
    }));
    const __VLS_2 = __VLS_1({
        to: "/config/models",
        ...{ class: "no-model-banner-btn" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1));
    /** @type {__VLS_StyleScopedClasses['no-model-banner-btn']} */ ;
    const { default: __VLS_5 } = __VLS_3.slots;
    // @ts-ignore
    [modelsLoading, defaultModelFailed, defaultModelName,];
    var __VLS_3;
}
if (!__VLS_ctx.modelsLoading && __VLS_ctx.modelCount === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-left" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-left']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "no-model-banner-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-title" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "no-model-banner-desc" },
    });
    /** @type {__VLS_StyleScopedClasses['no-model-banner-desc']} */ ;
    let __VLS_6;
    /** @ts-ignore @type { | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link'] | typeof __VLS_components.routerLink | typeof __VLS_components.RouterLink | typeof __VLS_components['router-link']} */
    routerLink;
    // @ts-ignore
    const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
        to: "/config/models",
        ...{ class: "no-model-banner-btn" },
    }));
    const __VLS_8 = __VLS_7({
        to: "/config/models",
        ...{ class: "no-model-banner-btn" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_7));
    /** @type {__VLS_StyleScopedClasses['no-model-banner-btn']} */ ;
    const { default: __VLS_11 } = __VLS_9.slots;
    // @ts-ignore
    [modelsLoading, modelCount,];
    var __VLS_9;
}
let __VLS_12;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_13 = __VLS_asFunctionalComponent1(__VLS_12, new __VLS_12({
    gutter: (12),
    ...{ style: {} },
}));
const __VLS_14 = __VLS_13({
    gutter: (12),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_13));
const { default: __VLS_17 } = __VLS_15.slots;
let __VLS_18;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_19 = __VLS_asFunctionalComponent1(__VLS_18, new __VLS_18({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
}));
const __VLS_20 = __VLS_19({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
}, ...__VLS_functionalComponentArgsRest(__VLS_19));
const { default: __VLS_23 } = __VLS_21.slots;
let __VLS_24;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_25 = __VLS_asFunctionalComponent1(__VLS_24, new __VLS_24({
    shadow: "never",
    ...{ class: "stat-card stat-card--members" },
}));
const __VLS_26 = __VLS_25({
    shadow: "never",
    ...{ class: "stat-card stat-card--members" },
}, ...__VLS_functionalComponentArgsRest(__VLS_25));
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card--members']} */ ;
const { default: __VLS_29 } = __VLS_27.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.stats?.agents.total ?? __VLS_ctx.agentStore.list.length);
// @ts-ignore
[stats, agentStore,];
var __VLS_27;
// @ts-ignore
[];
var __VLS_21;
let __VLS_30;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_31 = __VLS_asFunctionalComponent1(__VLS_30, new __VLS_30({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
}));
const __VLS_32 = __VLS_31({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
}, ...__VLS_functionalComponentArgsRest(__VLS_31));
const { default: __VLS_35 } = __VLS_33.slots;
let __VLS_36;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_37 = __VLS_asFunctionalComponent1(__VLS_36, new __VLS_36({
    shadow: "never",
    ...{ class: "stat-card stat-card--sessions" },
}));
const __VLS_38 = __VLS_37({
    shadow: "never",
    ...{ class: "stat-card stat-card--sessions" },
}, ...__VLS_functionalComponentArgsRest(__VLS_37));
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card--sessions']} */ ;
const { default: __VLS_41 } = __VLS_39.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.stats?.sessions.total ?? 0);
// @ts-ignore
[stats,];
var __VLS_39;
// @ts-ignore
[];
var __VLS_33;
let __VLS_42;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_43 = __VLS_asFunctionalComponent1(__VLS_42, new __VLS_42({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
    ...{ style: {} },
}));
const __VLS_44 = __VLS_43({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_43));
const { default: __VLS_47 } = __VLS_45.slots;
let __VLS_48;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_49 = __VLS_asFunctionalComponent1(__VLS_48, new __VLS_48({
    shadow: "never",
    ...{ class: "stat-card stat-card--messages" },
}));
const __VLS_50 = __VLS_49({
    shadow: "never",
    ...{ class: "stat-card stat-card--messages" },
}, ...__VLS_functionalComponentArgsRest(__VLS_49));
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card--messages']} */ ;
const { default: __VLS_53 } = __VLS_51.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.stats?.sessions.totalMessages ?? 0);
// @ts-ignore
[stats,];
var __VLS_51;
// @ts-ignore
[];
var __VLS_45;
let __VLS_54;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_55 = __VLS_asFunctionalComponent1(__VLS_54, new __VLS_54({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
    ...{ style: {} },
}));
const __VLS_56 = __VLS_55({
    xs: (12),
    sm: (12),
    md: (6),
    lg: (6),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_55));
const { default: __VLS_59 } = __VLS_57.slots;
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    shadow: "never",
    ...{ class: "stat-card stat-card--tokens" },
}));
const __VLS_62 = __VLS_61({
    shadow: "never",
    ...{ class: "stat-card stat-card--tokens" },
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
/** @type {__VLS_StyleScopedClasses['stat-card']} */ ;
/** @type {__VLS_StyleScopedClasses['stat-card--tokens']} */ ;
const { default: __VLS_65 } = __VLS_63.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-label" },
});
/** @type {__VLS_StyleScopedClasses['stat-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "stat-value" },
});
/** @type {__VLS_StyleScopedClasses['stat-value']} */ ;
(__VLS_ctx.formatTokens(__VLS_ctx.stats?.sessions.totalTokens ?? 0));
// @ts-ignore
[stats, formatTokens,];
var __VLS_63;
// @ts-ignore
[];
var __VLS_57;
// @ts-ignore
[];
var __VLS_15;
if (__VLS_ctx.stats?.topAgents?.length) {
    let __VLS_66;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_67 = __VLS_asFunctionalComponent1(__VLS_66, new __VLS_66({
        shadow: "hover",
        ...{ style: {} },
    }));
    const __VLS_68 = __VLS_67({
        shadow: "hover",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_67));
    const { default: __VLS_71 } = __VLS_69.slots;
    {
        const { header: __VLS_72 } = __VLS_69.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        let __VLS_73;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_74 = __VLS_asFunctionalComponent1(__VLS_73, new __VLS_73({
            ...{ style: {} },
        }));
        const __VLS_75 = __VLS_74({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_74));
        const { default: __VLS_78 } = __VLS_76.slots;
        let __VLS_79;
        /** @ts-ignore @type { | typeof __VLS_components.DataAnalysis} */
        DataAnalysis;
        // @ts-ignore
        const __VLS_80 = __VLS_asFunctionalComponent1(__VLS_79, new __VLS_79({}));
        const __VLS_81 = __VLS_80({}, ...__VLS_functionalComponentArgsRest(__VLS_80));
        // @ts-ignore
        [stats,];
        var __VLS_76;
        // @ts-ignore
        [];
    }
    let __VLS_84;
    /** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
    elTable;
    // @ts-ignore
    const __VLS_85 = __VLS_asFunctionalComponent1(__VLS_84, new __VLS_84({
        data: (__VLS_ctx.stats.topAgents),
        stripe: true,
        ...{ style: {} },
    }));
    const __VLS_86 = __VLS_85({
        data: (__VLS_ctx.stats.topAgents),
        stripe: true,
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_85));
    const { default: __VLS_89 } = __VLS_87.slots;
    let __VLS_90;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_91 = __VLS_asFunctionalComponent1(__VLS_90, new __VLS_90({
        label: "成员",
        minWidth: "140",
    }));
    const __VLS_92 = __VLS_91({
        label: "成员",
        minWidth: "140",
    }, ...__VLS_functionalComponentArgsRest(__VLS_91));
    const { default: __VLS_95 } = __VLS_93.slots;
    {
        const { default: __VLS_96 } = __VLS_93.slots;
        const [{ row }] = __VLS_vSlot(__VLS_96);
        let __VLS_97;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_98 = __VLS_asFunctionalComponent1(__VLS_97, new __VLS_97({
            ...{ 'onClick': {} },
            type: "primary",
            link: true,
        }));
        const __VLS_99 = __VLS_98({
            ...{ 'onClick': {} },
            type: "primary",
            link: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_98));
        let __VLS_102;
        const __VLS_103 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.stats?.topAgents?.length))
                        return;
                    __VLS_ctx.$router.push(`/agents/${row.id}`);
                    // @ts-ignore
                    [stats, $router,];
                } });
        const { default: __VLS_104 } = __VLS_100.slots;
        (row.name);
        // @ts-ignore
        [];
        var __VLS_100;
        var __VLS_101;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_93;
    let __VLS_105;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_106 = __VLS_asFunctionalComponent1(__VLS_105, new __VLS_105({
        label: "对话数",
        width: "100",
        align: "center",
    }));
    const __VLS_107 = __VLS_106({
        label: "对话数",
        width: "100",
        align: "center",
    }, ...__VLS_functionalComponentArgsRest(__VLS_106));
    const { default: __VLS_110 } = __VLS_108.slots;
    {
        const { default: __VLS_111 } = __VLS_108.slots;
        const [{ row }] = __VLS_vSlot(__VLS_111);
        let __VLS_112;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_113 = __VLS_asFunctionalComponent1(__VLS_112, new __VLS_112({
            size: "small",
            type: "info",
        }));
        const __VLS_114 = __VLS_113({
            size: "small",
            type: "info",
        }, ...__VLS_functionalComponentArgsRest(__VLS_113));
        const { default: __VLS_117 } = __VLS_115.slots;
        (row.sessions);
        // @ts-ignore
        [];
        var __VLS_115;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_108;
    let __VLS_118;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_119 = __VLS_asFunctionalComponent1(__VLS_118, new __VLS_118({
        label: "消息数",
        width: "100",
        align: "center",
    }));
    const __VLS_120 = __VLS_119({
        label: "消息数",
        width: "100",
        align: "center",
    }, ...__VLS_functionalComponentArgsRest(__VLS_119));
    const { default: __VLS_123 } = __VLS_121.slots;
    {
        const { default: __VLS_124 } = __VLS_121.slots;
        const [{ row }] = __VLS_vSlot(__VLS_124);
        (row.messages);
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_121;
    let __VLS_125;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_126 = __VLS_asFunctionalComponent1(__VLS_125, new __VLS_125({
        label: "Token 用量",
        width: "130",
        align: "center",
    }));
    const __VLS_127 = __VLS_126({
        label: "Token 用量",
        width: "130",
        align: "center",
    }, ...__VLS_functionalComponentArgsRest(__VLS_126));
    const { default: __VLS_130 } = __VLS_128.slots;
    {
        const { default: __VLS_131 } = __VLS_128.slots;
        const [{ row }] = __VLS_vSlot(__VLS_131);
        let __VLS_132;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_133 = __VLS_asFunctionalComponent1(__VLS_132, new __VLS_132({
            size: "small",
            type: (row.tokens > 100000 ? 'danger' : row.tokens > 50000 ? 'warning' : 'success'),
            effect: "plain",
        }));
        const __VLS_134 = __VLS_133({
            size: "small",
            type: (row.tokens > 100000 ? 'danger' : row.tokens > 50000 ? 'warning' : 'success'),
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_133));
        const { default: __VLS_137 } = __VLS_135.slots;
        (__VLS_ctx.formatTokens(row.tokens));
        // @ts-ignore
        [formatTokens,];
        var __VLS_135;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_128;
    // @ts-ignore
    [];
    var __VLS_87;
    // @ts-ignore
    [];
    var __VLS_69;
}
let __VLS_138;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_139 = __VLS_asFunctionalComponent1(__VLS_138, new __VLS_138({
    shadow: "hover",
}));
const __VLS_140 = __VLS_139({
    shadow: "hover",
}, ...__VLS_functionalComponentArgsRest(__VLS_139));
const { default: __VLS_143 } = __VLS_141.slots;
{
    const { header: __VLS_144 } = __VLS_141.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_145;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_146 = __VLS_asFunctionalComponent1(__VLS_145, new __VLS_145({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }));
    const __VLS_147 = __VLS_146({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_146));
    let __VLS_150;
    const __VLS_151 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.$router.push('/agents');
                // @ts-ignore
                [$router,];
            } });
    const { default: __VLS_152 } = __VLS_148.slots;
    // @ts-ignore
    [];
    var __VLS_148;
    var __VLS_149;
    // @ts-ignore
    [];
}
let __VLS_153;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_154 = __VLS_asFunctionalComponent1(__VLS_153, new __VLS_153({
    data: (__VLS_ctx.agentStore.list),
    stripe: true,
    ...{ style: {} },
}));
const __VLS_155 = __VLS_154({
    data: (__VLS_ctx.agentStore.list),
    stripe: true,
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_154));
const { default: __VLS_158 } = __VLS_156.slots;
let __VLS_159;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_160 = __VLS_asFunctionalComponent1(__VLS_159, new __VLS_159({
    label: "名称",
    minWidth: "150",
}));
const __VLS_161 = __VLS_160({
    label: "名称",
    minWidth: "150",
}, ...__VLS_functionalComponentArgsRest(__VLS_160));
const { default: __VLS_164 } = __VLS_162.slots;
{
    const { default: __VLS_165 } = __VLS_162.slots;
    const [{ row }] = __VLS_vSlot(__VLS_165);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "avatar-dot" },
        ...{ style: ({ background: row.avatarColor || '#409eff' }) },
    });
    /** @type {__VLS_StyleScopedClasses['avatar-dot']} */ ;
    (row.name.charAt(0));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (row.name);
    // @ts-ignore
    [agentStore,];
}
// @ts-ignore
[];
var __VLS_162;
let __VLS_166;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_167 = __VLS_asFunctionalComponent1(__VLS_166, new __VLS_166({
    label: "模型",
    minWidth: "180",
}));
const __VLS_168 = __VLS_167({
    label: "模型",
    minWidth: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_167));
const { default: __VLS_171 } = __VLS_169.slots;
{
    const { default: __VLS_172 } = __VLS_169.slots;
    const [{ row }] = __VLS_vSlot(__VLS_172);
    let __VLS_173;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_174 = __VLS_asFunctionalComponent1(__VLS_173, new __VLS_173({
        size: "small",
        type: "info",
    }));
    const __VLS_175 = __VLS_174({
        size: "small",
        type: "info",
    }, ...__VLS_functionalComponentArgsRest(__VLS_174));
    const { default: __VLS_178 } = __VLS_176.slots;
    (row.modelId || row.model || '-');
    // @ts-ignore
    [];
    var __VLS_176;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_169;
let __VLS_179;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_180 = __VLS_asFunctionalComponent1(__VLS_179, new __VLS_179({
    label: "通道",
    minWidth: "140",
}));
const __VLS_181 = __VLS_180({
    label: "通道",
    minWidth: "140",
}, ...__VLS_functionalComponentArgsRest(__VLS_180));
const { default: __VLS_184 } = __VLS_182.slots;
{
    const { default: __VLS_185 } = __VLS_182.slots;
    const [{ row }] = __VLS_vSlot(__VLS_185);
    if (row.channelIds?.length) {
        for (const [ch] of __VLS_vFor((row.channelIds))) {
            let __VLS_186;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({
                key: (ch),
                size: "small",
                ...{ style: {} },
            }));
            const __VLS_188 = __VLS_187({
                key: (ch),
                size: "small",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_187));
            const { default: __VLS_191 } = __VLS_189.slots;
            (ch);
            // @ts-ignore
            [];
            var __VLS_189;
            // @ts-ignore
            [];
        }
    }
    else {
        let __VLS_192;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({
            type: "info",
            size: "small",
        }));
        const __VLS_194 = __VLS_193({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_193));
        const { default: __VLS_197 } = __VLS_195.slots;
        // @ts-ignore
        [];
        var __VLS_195;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_182;
let __VLS_198;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({
    label: "状态",
    width: "100",
}));
const __VLS_200 = __VLS_199({
    label: "状态",
    width: "100",
}, ...__VLS_functionalComponentArgsRest(__VLS_199));
const { default: __VLS_203 } = __VLS_201.slots;
{
    const { default: __VLS_204 } = __VLS_201.slots;
    const [{ row }] = __VLS_vSlot(__VLS_204);
    let __VLS_205;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_206 = __VLS_asFunctionalComponent1(__VLS_205, new __VLS_205({
        type: (__VLS_ctx.statusType(row.status)),
        size: "small",
    }));
    const __VLS_207 = __VLS_206({
        type: (__VLS_ctx.statusType(row.status)),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_206));
    const { default: __VLS_210 } = __VLS_208.slots;
    (__VLS_ctx.statusLabel(row.status));
    // @ts-ignore
    [statusType, statusLabel,];
    var __VLS_208;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_201;
let __VLS_211;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_212 = __VLS_asFunctionalComponent1(__VLS_211, new __VLS_211({
    label: "操作",
    width: "100",
}));
const __VLS_213 = __VLS_212({
    label: "操作",
    width: "100",
}, ...__VLS_functionalComponentArgsRest(__VLS_212));
const { default: __VLS_216 } = __VLS_214.slots;
{
    const { default: __VLS_217 } = __VLS_214.slots;
    const [{ row }] = __VLS_vSlot(__VLS_217);
    let __VLS_218;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_219 = __VLS_asFunctionalComponent1(__VLS_218, new __VLS_218({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
        link: true,
    }));
    const __VLS_220 = __VLS_219({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
        link: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_219));
    let __VLS_223;
    const __VLS_224 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.$router.push(`/agents/${row.id}`);
                // @ts-ignore
                [$router,];
            } });
    const { default: __VLS_225 } = __VLS_221.slots;
    // @ts-ignore
    [];
    var __VLS_221;
    var __VLS_222;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_214;
// @ts-ignore
[];
var __VLS_156;
if (__VLS_ctx.agentStore.list.length === 0) {
    let __VLS_226;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_227 = __VLS_asFunctionalComponent1(__VLS_226, new __VLS_226({
        description: "暂无 AI 成员",
    }));
    const __VLS_228 = __VLS_227({
        description: "暂无 AI 成员",
    }, ...__VLS_functionalComponentArgsRest(__VLS_227));
}
// @ts-ignore
[agentStore,];
var __VLS_141;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
