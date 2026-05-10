/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { Search } from '@element-plus/icons-vue';
import { agents as agentsApi, agentSkills as skillsApi, files as filesApi, } from '../api';
const router = useRouter();
// ── State ──────────────────────────────────────────────────────────────────
const agents = ref([]);
const loading = ref(false);
const filterAgentId = ref('');
const filterKeyword = ref('');
const allRows = ref([]);
// ── Computed ───────────────────────────────────────────────────────────────
const activeAgentIds = computed(() => new Set(allRows.value.map(r => r.agentId)));
const filteredRows = computed(() => {
    let rows = allRows.value;
    if (filterAgentId.value)
        rows = rows.filter(r => r.agentId === filterAgentId.value);
    if (filterKeyword.value.trim()) {
        const kw = filterKeyword.value.trim().toLowerCase();
        rows = rows.filter(r => r.skill.name.toLowerCase().includes(kw) ||
            r.skill.id.toLowerCase().includes(kw) ||
            (r.skill.category || '').toLowerCase().includes(kw));
    }
    return rows;
});
// ── Load ───────────────────────────────────────────────────────────────────
async function loadAll() {
    loading.value = true;
    try {
        const agRes = await agentsApi.list();
        agents.value = agRes.data || [];
        const results = await Promise.allSettled(agents.value.map(ag => skillsApi.list(ag.id).then(res => (res.data || []).map((sk) => ({
            agentId: ag.id,
            agentName: ag.name,
            skill: sk,
        })))));
        const rows = [];
        for (const r of results) {
            if (r.status === 'fulfilled')
                rows.push(...r.value);
        }
        rows.sort((a, b) => a.agentName.localeCompare(b.agentName) || a.skill.name.localeCompare(b.skill.name));
        allRows.value = rows;
    }
    catch (e) {
        ElMessage.error('加载失败: ' + (e.message || ''));
    }
    finally {
        loading.value = false;
    }
}
// ── Toggle ─────────────────────────────────────────────────────────────────
async function toggleSkill(row, enabled) {
    try {
        await skillsApi.update(row.agentId, row.skill.id, { enabled });
        row.skill.enabled = enabled;
    }
    catch {
        ElMessage.error('操作失败');
    }
}
// ── Remove ─────────────────────────────────────────────────────────────────
async function removeSkill(row) {
    try {
        await skillsApi.remove(row.agentId, row.skill.id);
        allRows.value = allRows.value.filter(r => !(r.agentId === row.agentId && r.skill.id === row.skill.id));
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
// ── Edit ───────────────────────────────────────────────────────────────────
function goEdit(row) {
    router.push(`/agents/${row.agentId}?tab=skills&skill=${row.skill.id}`);
}
// ── Copy ───────────────────────────────────────────────────────────────────
const copyDialog = ref({
    visible: false,
    source: null,
    targetAgentId: '',
    newSkillId: '',
    newName: '',
    copying: false,
});
function openCopy(row) {
    copyDialog.value = {
        visible: true,
        source: row,
        targetAgentId: '',
        newSkillId: '',
        newName: '',
        copying: false,
    };
}
async function doCopy() {
    const { source, targetAgentId, newSkillId, newName } = copyDialog.value;
    if (!source || !targetAgentId) {
        ElMessage.warning('请选择目标成员');
        return;
    }
    copyDialog.value.copying = true;
    try {
        // 读取 SKILL.md 内容
        let promptContent = '';
        try {
            const mdRes = await filesApi.read(source.agentId, `skills/${source.skill.id}/SKILL.md`);
            promptContent = mdRes.data?.content || '';
        }
        catch { /* 无 SKILL.md 正常 */ }
        const targetId = newSkillId.trim() || source.skill.id;
        const targetName = newName.trim() || source.skill.name;
        await skillsApi.create(targetAgentId, {
            meta: {
                id: targetId,
                name: targetName,
                icon: source.skill.icon || '',
                category: source.skill.category || '',
                description: source.skill.description || '',
                version: source.skill.version || '1.0.0',
                enabled: source.skill.enabled,
                source: 'local',
                installedAt: '',
            },
            promptContent,
        });
        const targetAgent = agents.value.find(a => a.id === targetAgentId);
        ElMessage.success(`已复制「${targetName}」到「${targetAgent?.name}」`);
        copyDialog.value.visible = false;
        await loadAll();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '复制失败');
    }
    finally {
        copyDialog.value.copying = false;
    }
}
onMounted(loadAll);
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['skill-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "skills-page" },
});
/** @type {__VLS_StyleScopedClasses['skills-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "page-header" },
});
/** @type {__VLS_StyleScopedClasses['page-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "header-left" },
});
/** @type {__VLS_StyleScopedClasses['header-left']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ class: "page-title" },
});
/** @type {__VLS_StyleScopedClasses['page-title']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({}));
const __VLS_2 = __VLS_1({}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
let __VLS_6;
/** @ts-ignore @type { | typeof __VLS_components.Aim} */
Aim;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "header-stats" },
});
/** @type {__VLS_StyleScopedClasses['header-stats']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "stat-item" },
});
/** @type {__VLS_StyleScopedClasses['stat-item']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
(__VLS_ctx.allRows.length);
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "stat-sep" },
});
/** @type {__VLS_StyleScopedClasses['stat-sep']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "stat-item" },
});
/** @type {__VLS_StyleScopedClasses['stat-item']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
(__VLS_ctx.allRows.filter(r => r.skill.enabled).length);
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "stat-sep" },
});
/** @type {__VLS_StyleScopedClasses['stat-sep']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "stat-item" },
});
/** @type {__VLS_StyleScopedClasses['stat-item']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
(__VLS_ctx.activeAgentIds.size);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "header-acts" },
});
/** @type {__VLS_StyleScopedClasses['header-acts']} */ ;
let __VLS_11;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
    modelValue: (__VLS_ctx.filterAgentId),
    placeholder: "全部成员",
    clearable: true,
    size: "small",
    ...{ style: {} },
}));
const __VLS_13 = __VLS_12({
    modelValue: (__VLS_ctx.filterAgentId),
    placeholder: "全部成员",
    clearable: true,
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_12));
const { default: __VLS_16 } = __VLS_14.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agents))) {
    let __VLS_17;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_18 = __VLS_asFunctionalComponent1(__VLS_17, new __VLS_17({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_19 = __VLS_18({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_18));
    // @ts-ignore
    [allRows, allRows, activeAgentIds, filterAgentId, agents,];
}
// @ts-ignore
[];
var __VLS_14;
let __VLS_22;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_23 = __VLS_asFunctionalComponent1(__VLS_22, new __VLS_22({
    modelValue: (__VLS_ctx.filterKeyword),
    placeholder: "搜索技能…",
    clearable: true,
    size: "small",
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}));
const __VLS_24 = __VLS_23({
    modelValue: (__VLS_ctx.filterKeyword),
    placeholder: "搜索技能…",
    clearable: true,
    size: "small",
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}, ...__VLS_functionalComponentArgsRest(__VLS_23));
let __VLS_27;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_28 = __VLS_asFunctionalComponent1(__VLS_27, new __VLS_27({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    size: "small",
    circle: true,
}));
const __VLS_29 = __VLS_28({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    size: "small",
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_28));
let __VLS_32;
const __VLS_33 = ({ click: {} },
    { onClick: (__VLS_ctx.loadAll) });
const { default: __VLS_34 } = __VLS_30.slots;
let __VLS_35;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_36 = __VLS_asFunctionalComponent1(__VLS_35, new __VLS_35({}));
const __VLS_37 = __VLS_36({}, ...__VLS_functionalComponentArgsRest(__VLS_36));
const { default: __VLS_40 } = __VLS_38.slots;
let __VLS_41;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_42 = __VLS_asFunctionalComponent1(__VLS_41, new __VLS_41({}));
const __VLS_43 = __VLS_42({}, ...__VLS_functionalComponentArgsRest(__VLS_42));
// @ts-ignore
[filterKeyword, Search, loading, loadAll,];
var __VLS_38;
// @ts-ignore
[];
var __VLS_30;
var __VLS_31;
if (__VLS_ctx.loading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_46;
    /** @ts-ignore @type { | typeof __VLS_components.elSkeleton | typeof __VLS_components.ElSkeleton | typeof __VLS_components['el-skeleton']} */
    elSkeleton;
    // @ts-ignore
    const __VLS_47 = __VLS_asFunctionalComponent1(__VLS_46, new __VLS_46({
        rows: (3),
        animated: true,
    }));
    const __VLS_48 = __VLS_47({
        rows: (3),
        animated: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_47));
}
else if (__VLS_ctx.filteredRows.length === 0) {
    let __VLS_51;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_52 = __VLS_asFunctionalComponent1(__VLS_51, new __VLS_51({
        description: "暂无技能",
        ...{ style: {} },
    }));
    const __VLS_53 = __VLS_52({
        description: "暂无技能",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_52));
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "skills-grid" },
    });
    /** @type {__VLS_StyleScopedClasses['skills-grid']} */ ;
    for (const [row] of __VLS_vFor((__VLS_ctx.filteredRows))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (`${row.agentId}-${row.skill.id}`),
            ...{ class: "skill-card" },
        });
        /** @type {__VLS_StyleScopedClasses['skill-card']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "card-agent" },
        });
        /** @type {__VLS_StyleScopedClasses['card-agent']} */ ;
        let __VLS_56;
        /** @ts-ignore @type { | typeof __VLS_components.elAvatar | typeof __VLS_components.ElAvatar | typeof __VLS_components['el-avatar'] | typeof __VLS_components.elAvatar | typeof __VLS_components.ElAvatar | typeof __VLS_components['el-avatar']} */
        elAvatar;
        // @ts-ignore
        const __VLS_57 = __VLS_asFunctionalComponent1(__VLS_56, new __VLS_56({
            size: (18),
            ...{ style: {} },
        }));
        const __VLS_58 = __VLS_57({
            size: (18),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_57));
        const { default: __VLS_61 } = __VLS_59.slots;
        (row.agentName.charAt(0));
        // @ts-ignore
        [loading, filteredRows, filteredRows,];
        var __VLS_59;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "agent-name" },
        });
        /** @type {__VLS_StyleScopedClasses['agent-name']} */ ;
        (row.agentName);
        let __VLS_62;
        /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
        elSwitch;
        // @ts-ignore
        const __VLS_63 = __VLS_asFunctionalComponent1(__VLS_62, new __VLS_62({
            ...{ 'onChange': {} },
            modelValue: (row.skill.enabled),
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_64 = __VLS_63({
            ...{ 'onChange': {} },
            modelValue: (row.skill.enabled),
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_63));
        let __VLS_67;
        const __VLS_68 = ({ change: {} },
            { onChange: ((v) => __VLS_ctx.toggleSkill(row, v)) });
        var __VLS_65;
        var __VLS_66;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "card-body" },
        });
        /** @type {__VLS_StyleScopedClasses['card-body']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "skill-icon" },
        });
        /** @type {__VLS_StyleScopedClasses['skill-icon']} */ ;
        if (row.skill.icon) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            (row.skill.icon);
        }
        else {
            let __VLS_69;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_70 = __VLS_asFunctionalComponent1(__VLS_69, new __VLS_69({
                size: "20",
                color: "#c0c4cc",
            }));
            const __VLS_71 = __VLS_70({
                size: "20",
                color: "#c0c4cc",
            }, ...__VLS_functionalComponentArgsRest(__VLS_70));
            const { default: __VLS_74 } = __VLS_72.slots;
            let __VLS_75;
            /** @ts-ignore @type { | typeof __VLS_components.Tools} */
            Tools;
            // @ts-ignore
            const __VLS_76 = __VLS_asFunctionalComponent1(__VLS_75, new __VLS_75({}));
            const __VLS_77 = __VLS_76({}, ...__VLS_functionalComponentArgsRest(__VLS_76));
            // @ts-ignore
            [toggleSkill,];
            var __VLS_72;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "skill-info" },
        });
        /** @type {__VLS_StyleScopedClasses['skill-info']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "skill-name" },
        });
        /** @type {__VLS_StyleScopedClasses['skill-name']} */ ;
        (row.skill.name);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "skill-id" },
        });
        /** @type {__VLS_StyleScopedClasses['skill-id']} */ ;
        (row.skill.id);
        if (row.skill.category) {
            let __VLS_80;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
                size: "small",
                type: "primary",
                effect: "plain",
                ...{ style: {} },
            }));
            const __VLS_82 = __VLS_81({
                size: "small",
                type: "primary",
                effect: "plain",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_81));
            const { default: __VLS_85 } = __VLS_83.slots;
            (row.skill.category);
            // @ts-ignore
            [];
            var __VLS_83;
        }
        if (row.skill.description) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "card-desc" },
            });
            /** @type {__VLS_StyleScopedClasses['card-desc']} */ ;
            (row.skill.description);
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "card-acts" },
        });
        /** @type {__VLS_StyleScopedClasses['card-acts']} */ ;
        let __VLS_86;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_87 = __VLS_asFunctionalComponent1(__VLS_86, new __VLS_86({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }));
        const __VLS_88 = __VLS_87({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }, ...__VLS_functionalComponentArgsRest(__VLS_87));
        let __VLS_91;
        const __VLS_92 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.loading))
                        return;
                    if (!!(__VLS_ctx.filteredRows.length === 0))
                        return;
                    __VLS_ctx.goEdit(row);
                    // @ts-ignore
                    [goEdit,];
                } });
        const { default: __VLS_93 } = __VLS_89.slots;
        let __VLS_94;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_95 = __VLS_asFunctionalComponent1(__VLS_94, new __VLS_94({}));
        const __VLS_96 = __VLS_95({}, ...__VLS_functionalComponentArgsRest(__VLS_95));
        const { default: __VLS_99 } = __VLS_97.slots;
        let __VLS_100;
        /** @ts-ignore @type { | typeof __VLS_components.Edit} */
        Edit;
        // @ts-ignore
        const __VLS_101 = __VLS_asFunctionalComponent1(__VLS_100, new __VLS_100({}));
        const __VLS_102 = __VLS_101({}, ...__VLS_functionalComponentArgsRest(__VLS_101));
        // @ts-ignore
        [];
        var __VLS_97;
        // @ts-ignore
        [];
        var __VLS_89;
        var __VLS_90;
        let __VLS_105;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_106 = __VLS_asFunctionalComponent1(__VLS_105, new __VLS_105({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }));
        const __VLS_107 = __VLS_106({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }, ...__VLS_functionalComponentArgsRest(__VLS_106));
        let __VLS_110;
        const __VLS_111 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.loading))
                        return;
                    if (!!(__VLS_ctx.filteredRows.length === 0))
                        return;
                    __VLS_ctx.openCopy(row);
                    // @ts-ignore
                    [openCopy,];
                } });
        const { default: __VLS_112 } = __VLS_108.slots;
        let __VLS_113;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_114 = __VLS_asFunctionalComponent1(__VLS_113, new __VLS_113({}));
        const __VLS_115 = __VLS_114({}, ...__VLS_functionalComponentArgsRest(__VLS_114));
        const { default: __VLS_118 } = __VLS_116.slots;
        let __VLS_119;
        /** @ts-ignore @type { | typeof __VLS_components.CopyDocument} */
        CopyDocument;
        // @ts-ignore
        const __VLS_120 = __VLS_asFunctionalComponent1(__VLS_119, new __VLS_119({}));
        const __VLS_121 = __VLS_120({}, ...__VLS_functionalComponentArgsRest(__VLS_120));
        // @ts-ignore
        [];
        var __VLS_116;
        // @ts-ignore
        [];
        var __VLS_108;
        var __VLS_109;
        let __VLS_124;
        /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
        elPopconfirm;
        // @ts-ignore
        const __VLS_125 = __VLS_asFunctionalComponent1(__VLS_124, new __VLS_124({
            ...{ 'onConfirm': {} },
            title: (`从「${row.agentName}」删除「${row.skill.name}」？`),
        }));
        const __VLS_126 = __VLS_125({
            ...{ 'onConfirm': {} },
            title: (`从「${row.agentName}」删除「${row.skill.name}」？`),
        }, ...__VLS_functionalComponentArgsRest(__VLS_125));
        let __VLS_129;
        const __VLS_130 = ({ confirm: {} },
            { onConfirm: (...[$event]) => {
                    if (!!(__VLS_ctx.loading))
                        return;
                    if (!!(__VLS_ctx.filteredRows.length === 0))
                        return;
                    __VLS_ctx.removeSkill(row);
                    // @ts-ignore
                    [removeSkill,];
                } });
        const { default: __VLS_131 } = __VLS_127.slots;
        {
            const { reference: __VLS_132 } = __VLS_127.slots;
            let __VLS_133;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_134 = __VLS_asFunctionalComponent1(__VLS_133, new __VLS_133({
                size: "small",
                link: true,
                type: "danger",
            }));
            const __VLS_135 = __VLS_134({
                size: "small",
                link: true,
                type: "danger",
            }, ...__VLS_functionalComponentArgsRest(__VLS_134));
            const { default: __VLS_138 } = __VLS_136.slots;
            let __VLS_139;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_140 = __VLS_asFunctionalComponent1(__VLS_139, new __VLS_139({}));
            const __VLS_141 = __VLS_140({}, ...__VLS_functionalComponentArgsRest(__VLS_140));
            const { default: __VLS_144 } = __VLS_142.slots;
            let __VLS_145;
            /** @ts-ignore @type { | typeof __VLS_components.Delete} */
            Delete;
            // @ts-ignore
            const __VLS_146 = __VLS_asFunctionalComponent1(__VLS_145, new __VLS_145({}));
            const __VLS_147 = __VLS_146({}, ...__VLS_functionalComponentArgsRest(__VLS_146));
            // @ts-ignore
            [];
            var __VLS_142;
            // @ts-ignore
            [];
            var __VLS_136;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_127;
        var __VLS_128;
        // @ts-ignore
        [];
    }
}
let __VLS_150;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_151 = __VLS_asFunctionalComponent1(__VLS_150, new __VLS_150({
    modelValue: (__VLS_ctx.copyDialog.visible),
    title: "复制技能到其他成员",
    width: "420px",
}));
const __VLS_152 = __VLS_151({
    modelValue: (__VLS_ctx.copyDialog.visible),
    title: "复制技能到其他成员",
    width: "420px",
}, ...__VLS_functionalComponentArgsRest(__VLS_151));
const { default: __VLS_155 } = __VLS_153.slots;
if (__VLS_ctx.copyDialog.source) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "copy-source" },
    });
    /** @type {__VLS_StyleScopedClasses['copy-source']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
    (__VLS_ctx.copyDialog.source.agentName);
    let __VLS_156;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_157 = __VLS_asFunctionalComponent1(__VLS_156, new __VLS_156({
        ...{ style: {} },
    }));
    const __VLS_158 = __VLS_157({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_157));
    const { default: __VLS_161 } = __VLS_159.slots;
    let __VLS_162;
    /** @ts-ignore @type { | typeof __VLS_components.ArrowRight} */
    ArrowRight;
    // @ts-ignore
    const __VLS_163 = __VLS_asFunctionalComponent1(__VLS_162, new __VLS_162({}));
    const __VLS_164 = __VLS_163({}, ...__VLS_functionalComponentArgsRest(__VLS_163));
    // @ts-ignore
    [copyDialog, copyDialog, copyDialog,];
    var __VLS_159;
    (__VLS_ctx.copyDialog.source.skill.icon);
    (__VLS_ctx.copyDialog.source.skill.name);
    let __VLS_167;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_168 = __VLS_asFunctionalComponent1(__VLS_167, new __VLS_167({
        size: "small",
        type: "info",
        effect: "plain",
        ...{ style: {} },
    }));
    const __VLS_169 = __VLS_168({
        size: "small",
        type: "info",
        effect: "plain",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_168));
    const { default: __VLS_172 } = __VLS_170.slots;
    (__VLS_ctx.copyDialog.source.skill.id);
    // @ts-ignore
    [copyDialog, copyDialog, copyDialog,];
    var __VLS_170;
}
let __VLS_173;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_174 = __VLS_asFunctionalComponent1(__VLS_173, new __VLS_173({
    labelWidth: "80px",
    size: "small",
    ...{ style: {} },
}));
const __VLS_175 = __VLS_174({
    labelWidth: "80px",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_174));
const { default: __VLS_178 } = __VLS_176.slots;
let __VLS_179;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_180 = __VLS_asFunctionalComponent1(__VLS_179, new __VLS_179({
    label: "目标成员",
    required: true,
}));
const __VLS_181 = __VLS_180({
    label: "目标成员",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_180));
const { default: __VLS_184 } = __VLS_182.slots;
let __VLS_185;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_186 = __VLS_asFunctionalComponent1(__VLS_185, new __VLS_185({
    modelValue: (__VLS_ctx.copyDialog.targetAgentId),
    placeholder: "选择目标成员",
    ...{ style: {} },
}));
const __VLS_187 = __VLS_186({
    modelValue: (__VLS_ctx.copyDialog.targetAgentId),
    placeholder: "选择目标成员",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_186));
const { default: __VLS_190 } = __VLS_188.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agents.filter(a => a.id !== __VLS_ctx.copyDialog.source?.agentId)))) {
    let __VLS_191;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_192 = __VLS_asFunctionalComponent1(__VLS_191, new __VLS_191({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_193 = __VLS_192({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_192));
    // @ts-ignore
    [agents, copyDialog, copyDialog,];
}
// @ts-ignore
[];
var __VLS_188;
// @ts-ignore
[];
var __VLS_182;
let __VLS_196;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_197 = __VLS_asFunctionalComponent1(__VLS_196, new __VLS_196({
    label: "新技能 ID",
}));
const __VLS_198 = __VLS_197({
    label: "新技能 ID",
}, ...__VLS_functionalComponentArgsRest(__VLS_197));
const { default: __VLS_201 } = __VLS_199.slots;
let __VLS_202;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_203 = __VLS_asFunctionalComponent1(__VLS_202, new __VLS_202({
    modelValue: (__VLS_ctx.copyDialog.newSkillId),
    placeholder: (__VLS_ctx.copyDialog.source?.skill.id || '留空使用原 ID'),
}));
const __VLS_204 = __VLS_203({
    modelValue: (__VLS_ctx.copyDialog.newSkillId),
    placeholder: (__VLS_ctx.copyDialog.source?.skill.id || '留空使用原 ID'),
}, ...__VLS_functionalComponentArgsRest(__VLS_203));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
// @ts-ignore
[copyDialog, copyDialog,];
var __VLS_199;
let __VLS_207;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_208 = __VLS_asFunctionalComponent1(__VLS_207, new __VLS_207({
    label: "新名称",
}));
const __VLS_209 = __VLS_208({
    label: "新名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_208));
const { default: __VLS_212 } = __VLS_210.slots;
let __VLS_213;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_214 = __VLS_asFunctionalComponent1(__VLS_213, new __VLS_213({
    modelValue: (__VLS_ctx.copyDialog.newName),
    placeholder: (__VLS_ctx.copyDialog.source?.skill.name || '留空使用原名称'),
}));
const __VLS_215 = __VLS_214({
    modelValue: (__VLS_ctx.copyDialog.newName),
    placeholder: (__VLS_ctx.copyDialog.source?.skill.name || '留空使用原名称'),
}, ...__VLS_functionalComponentArgsRest(__VLS_214));
// @ts-ignore
[copyDialog, copyDialog,];
var __VLS_210;
// @ts-ignore
[];
var __VLS_176;
{
    const { footer: __VLS_218 } = __VLS_153.slots;
    let __VLS_219;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_220 = __VLS_asFunctionalComponent1(__VLS_219, new __VLS_219({
        ...{ 'onClick': {} },
    }));
    const __VLS_221 = __VLS_220({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_220));
    let __VLS_224;
    const __VLS_225 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.copyDialog.visible = false;
                // @ts-ignore
                [copyDialog,];
            } });
    const { default: __VLS_226 } = __VLS_222.slots;
    // @ts-ignore
    [];
    var __VLS_222;
    var __VLS_223;
    let __VLS_227;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_228 = __VLS_asFunctionalComponent1(__VLS_227, new __VLS_227({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.copyDialog.copying),
    }));
    const __VLS_229 = __VLS_228({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.copyDialog.copying),
    }, ...__VLS_functionalComponentArgsRest(__VLS_228));
    let __VLS_232;
    const __VLS_233 = ({ click: {} },
        { onClick: (__VLS_ctx.doCopy) });
    const { default: __VLS_234 } = __VLS_230.slots;
    let __VLS_235;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_236 = __VLS_asFunctionalComponent1(__VLS_235, new __VLS_235({}));
    const __VLS_237 = __VLS_236({}, ...__VLS_functionalComponentArgsRest(__VLS_236));
    const { default: __VLS_240 } = __VLS_238.slots;
    let __VLS_241;
    /** @ts-ignore @type { | typeof __VLS_components.CopyDocument} */
    CopyDocument;
    // @ts-ignore
    const __VLS_242 = __VLS_asFunctionalComponent1(__VLS_241, new __VLS_241({}));
    const __VLS_243 = __VLS_242({}, ...__VLS_functionalComponentArgsRest(__VLS_242));
    // @ts-ignore
    [copyDialog, doCopy,];
    var __VLS_238;
    // @ts-ignore
    [];
    var __VLS_230;
    var __VLS_231;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_153;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
