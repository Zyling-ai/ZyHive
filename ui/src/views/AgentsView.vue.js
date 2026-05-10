/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, reactive, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useAgentsStore } from '../stores/agents';
import { models as modelsApi, channels as channelsApi, tools as toolsApi, skills as skillsApi, agents as agentsApi, } from '../api';
const router = useRouter();
const store = useAgentsStore();
const wizardVisible = ref(false);
const wizardStep = ref(0);
const creating = ref(false);
const avatarColors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c'];
const wizardForm = reactive({
    id: '',
    name: '',
    description: '',
    avatarColor: '#409eff',
    modelId: '',
    channelIds: [],
    toolIds: [],
    skillIds: [],
});
const modelsList = ref([]);
const channelsList = ref([]);
const toolsList = ref([]);
const skillsList = ref([]);
onMounted(() => {
    store.fetchAll();
});
// kept for potential reuse
// @ts-ignore
async function openWizard() {
    wizardStep.value = 0;
    Object.assign(wizardForm, {
        id: '', name: '', description: '', avatarColor: '#409eff',
        modelId: '', channelIds: [], toolIds: [], skillIds: [],
    });
    // Preload registries
    try {
        const [mRes, cRes, tRes, sRes] = await Promise.all([
            modelsApi.list(), channelsApi.list(), toolsApi.list(), skillsApi.list(),
        ]);
        modelsList.value = mRes.data;
        channelsList.value = cRes.data;
        toolsList.value = tRes.data;
        skillsList.value = sRes.data;
    }
    catch { }
    wizardVisible.value = true;
}
function autoId() {
    if (!wizardForm.id || wizardForm.id === slugify(wizardForm.name.slice(0, -1))) {
        wizardForm.id = slugify(wizardForm.name);
    }
}
function slugify(s) {
    return s.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '').slice(0, 30);
}
function nextStep() {
    if (wizardStep.value === 0 && (!wizardForm.name || !wizardForm.id)) {
        ElMessage.warning('请填写名称和 ID');
        return;
    }
    wizardStep.value++;
}
function toggleArray(arr, val) {
    const idx = arr.indexOf(val);
    if (idx === -1)
        arr.push(val);
    else
        arr.splice(idx, 1);
}
async function createAgent() {
    creating.value = true;
    try {
        await agentsApi.create({
            id: wizardForm.id,
            name: wizardForm.name,
            description: wizardForm.description,
            modelId: wizardForm.modelId,
            channelIds: wizardForm.channelIds,
            toolIds: wizardForm.toolIds,
            skillIds: wizardForm.skillIds,
            avatarColor: wizardForm.avatarColor,
        });
        ElMessage.success('Agent 创建成功！');
        wizardVisible.value = false;
        store.fetchAll();
        router.push(`/agents/${wizardForm.id}`);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
    finally {
        creating.value = false;
    }
}
function statusType(s) {
    return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info';
}
function statusLabel(s) {
    return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲';
}
async function confirmDelete(id, name) {
    try {
        await ElMessageBox.confirm(`删除成员「${name}」将同时删除其工作区、对话记录和所有配置，且无法恢复。确认吗？`, '删除 AI 成员', { confirmButtonText: '确认删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' });
    }
    catch {
        return;
    }
    try {
        await agentsApi.delete(id);
        ElMessage.success(`已删除「${name}」`);
        await store.fetchAll();
    }
    catch (e) {
        ElMessage.error('删除失败：' + (e?.response?.data?.error ?? e?.message ?? '未知错误'));
    }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['agent-card']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-card']} */ ;
/** @type {__VLS_StyleScopedClasses['color-swatch']} */ ;
/** @type {__VLS_StyleScopedClasses['select-card']} */ ;
/** @type {__VLS_StyleScopedClasses['select-card']} */ ;
/** @type {__VLS_StyleScopedClasses['el-card__body']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "agents-page" },
});
/** @type {__VLS_StyleScopedClasses['agents-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ style: {} },
});
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    type: "primary",
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    type: "primary",
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.$router.push('/agents/new');
            // @ts-ignore
            [$router,];
        } });
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
[];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    gutter: (14),
}));
const __VLS_21 = __VLS_20({
    gutter: (14),
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
const { default: __VLS_24 } = __VLS_22.slots;
for (const [agent] of __VLS_vFor((__VLS_ctx.store.list))) {
    let __VLS_25;
    /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
    elCol;
    // @ts-ignore
    const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({
        xs: (24),
        sm: (12),
        md: (8),
        lg: (8),
        key: (agent.id),
    }));
    const __VLS_27 = __VLS_26({
        xs: (24),
        sm: (12),
        md: (8),
        lg: (8),
        key: (agent.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_26));
    const { default: __VLS_30 } = __VLS_28.slots;
    let __VLS_31;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_32 = __VLS_asFunctionalComponent1(__VLS_31, new __VLS_31({
        ...{ class: "agent-card" },
        shadow: "hover",
    }));
    const __VLS_33 = __VLS_32({
        ...{ class: "agent-card" },
        shadow: "hover",
    }, ...__VLS_functionalComponentArgsRest(__VLS_32));
    /** @type {__VLS_StyleScopedClasses['agent-card']} */ ;
    const { default: __VLS_36 } = __VLS_34.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "avatar-circle" },
        ...{ style: ({ background: agent.avatarColor || '#409eff' }) },
    });
    /** @type {__VLS_StyleScopedClasses['avatar-circle']} */ ;
    (agent.name.charAt(0));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "agent-card-info" },
    });
    /** @type {__VLS_StyleScopedClasses['agent-card-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "agent-card-name" },
    });
    /** @type {__VLS_StyleScopedClasses['agent-card-name']} */ ;
    (agent.name);
    let __VLS_37;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_38 = __VLS_asFunctionalComponent1(__VLS_37, new __VLS_37({
        type: "info",
        size: "small",
        ...{ class: "agent-card-id" },
    }));
    const __VLS_39 = __VLS_38({
        type: "info",
        size: "small",
        ...{ class: "agent-card-id" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_38));
    /** @type {__VLS_StyleScopedClasses['agent-card-id']} */ ;
    const { default: __VLS_42 } = __VLS_40.slots;
    (agent.id);
    // @ts-ignore
    [store,];
    var __VLS_40;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "agent-card-tags" },
    });
    /** @type {__VLS_StyleScopedClasses['agent-card-tags']} */ ;
    if (agent.system) {
        let __VLS_43;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_44 = __VLS_asFunctionalComponent1(__VLS_43, new __VLS_43({
            size: "small",
            type: "warning",
        }));
        const __VLS_45 = __VLS_44({
            size: "small",
            type: "warning",
        }, ...__VLS_functionalComponentArgsRest(__VLS_44));
        const { default: __VLS_48 } = __VLS_46.slots;
        // @ts-ignore
        [];
        var __VLS_46;
    }
    let __VLS_49;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
        type: (__VLS_ctx.statusType(agent.status)),
        size: "small",
    }));
    const __VLS_51 = __VLS_50({
        type: (__VLS_ctx.statusType(agent.status)),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_50));
    const { default: __VLS_54 } = __VLS_52.slots;
    (__VLS_ctx.statusLabel(agent.status));
    // @ts-ignore
    [statusType, statusLabel,];
    var __VLS_52;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_55;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_56 = __VLS_asFunctionalComponent1(__VLS_55, new __VLS_55({
        size: "small",
        type: "info",
        ...{ style: {} },
    }));
    const __VLS_57 = __VLS_56({
        size: "small",
        type: "info",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_56));
    const { default: __VLS_60 } = __VLS_58.slots;
    (agent.modelId || agent.model || '未配置');
    // @ts-ignore
    [];
    var __VLS_58;
    for (const [ch] of __VLS_vFor(((agent.channelIds || [])))) {
        let __VLS_61;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_62 = __VLS_asFunctionalComponent1(__VLS_61, new __VLS_61({
            key: (ch),
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_63 = __VLS_62({
            key: (ch),
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_62));
        const { default: __VLS_66 } = __VLS_64.slots;
        (ch);
        // @ts-ignore
        [];
        var __VLS_64;
        // @ts-ignore
        [];
    }
    if (agent.description) {
        let __VLS_67;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_68 = __VLS_asFunctionalComponent1(__VLS_67, new __VLS_67({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_69 = __VLS_68({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_68));
        const { default: __VLS_72 } = __VLS_70.slots;
        (agent.description);
        // @ts-ignore
        [];
        var __VLS_70;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_73;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_74 = __VLS_asFunctionalComponent1(__VLS_73, new __VLS_73({
        ...{ 'onClick': {} },
        type: "primary",
        ...{ style: {} },
    }));
    const __VLS_75 = __VLS_74({
        ...{ 'onClick': {} },
        type: "primary",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_74));
    let __VLS_78;
    const __VLS_79 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.$router.push(`/agents/${agent.id}`);
                // @ts-ignore
                [$router,];
            } });
    const { default: __VLS_80 } = __VLS_76.slots;
    let __VLS_81;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({}));
    const __VLS_83 = __VLS_82({}, ...__VLS_functionalComponentArgsRest(__VLS_82));
    const { default: __VLS_86 } = __VLS_84.slots;
    let __VLS_87;
    /** @ts-ignore @type { | typeof __VLS_components.ChatDotRound} */
    ChatDotRound;
    // @ts-ignore
    const __VLS_88 = __VLS_asFunctionalComponent1(__VLS_87, new __VLS_87({}));
    const __VLS_89 = __VLS_88({}, ...__VLS_functionalComponentArgsRest(__VLS_88));
    // @ts-ignore
    [];
    var __VLS_84;
    // @ts-ignore
    [];
    var __VLS_76;
    var __VLS_77;
    if (!agent.system) {
        let __VLS_92;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_93 = __VLS_asFunctionalComponent1(__VLS_92, new __VLS_92({
            ...{ 'onClick': {} },
            type: "danger",
            plain: true,
        }));
        const __VLS_94 = __VLS_93({
            ...{ 'onClick': {} },
            type: "danger",
            plain: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_93));
        let __VLS_97;
        const __VLS_98 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(!agent.system))
                        return;
                    __VLS_ctx.confirmDelete(agent.id, agent.name);
                    // @ts-ignore
                    [confirmDelete,];
                } });
        const { default: __VLS_99 } = __VLS_95.slots;
        let __VLS_100;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_101 = __VLS_asFunctionalComponent1(__VLS_100, new __VLS_100({}));
        const __VLS_102 = __VLS_101({}, ...__VLS_functionalComponentArgsRest(__VLS_101));
        const { default: __VLS_105 } = __VLS_103.slots;
        let __VLS_106;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_107 = __VLS_asFunctionalComponent1(__VLS_106, new __VLS_106({}));
        const __VLS_108 = __VLS_107({}, ...__VLS_functionalComponentArgsRest(__VLS_107));
        // @ts-ignore
        [];
        var __VLS_103;
        // @ts-ignore
        [];
        var __VLS_95;
        var __VLS_96;
    }
    // @ts-ignore
    [];
    var __VLS_34;
    // @ts-ignore
    [];
    var __VLS_28;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_22;
if (!__VLS_ctx.store.loading && __VLS_ctx.store.list.length === 0) {
    let __VLS_111;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_112 = __VLS_asFunctionalComponent1(__VLS_111, new __VLS_111({
        description: "暂无 AI 成员，点击「新建 Agent」开始",
    }));
    const __VLS_113 = __VLS_112({
        description: "暂无 AI 成员，点击「新建 Agent」开始",
    }, ...__VLS_functionalComponentArgsRest(__VLS_112));
}
let __VLS_116;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_117 = __VLS_asFunctionalComponent1(__VLS_116, new __VLS_116({
    modelValue: (__VLS_ctx.wizardVisible),
    title: "新建 AI 成员",
    width: "680px",
    closeOnClickModal: (false),
}));
const __VLS_118 = __VLS_117({
    modelValue: (__VLS_ctx.wizardVisible),
    title: "新建 AI 成员",
    width: "680px",
    closeOnClickModal: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_117));
const { default: __VLS_121 } = __VLS_119.slots;
let __VLS_122;
/** @ts-ignore @type { | typeof __VLS_components.elSteps | typeof __VLS_components.ElSteps | typeof __VLS_components['el-steps'] | typeof __VLS_components.elSteps | typeof __VLS_components.ElSteps | typeof __VLS_components['el-steps']} */
elSteps;
// @ts-ignore
const __VLS_123 = __VLS_asFunctionalComponent1(__VLS_122, new __VLS_122({
    active: (__VLS_ctx.wizardStep),
    finishStatus: "success",
    simple: true,
    ...{ style: {} },
}));
const __VLS_124 = __VLS_123({
    active: (__VLS_ctx.wizardStep),
    finishStatus: "success",
    simple: true,
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_123));
const { default: __VLS_127 } = __VLS_125.slots;
let __VLS_128;
/** @ts-ignore @type { | typeof __VLS_components.elStep | typeof __VLS_components.ElStep | typeof __VLS_components['el-step']} */
elStep;
// @ts-ignore
const __VLS_129 = __VLS_asFunctionalComponent1(__VLS_128, new __VLS_128({
    title: "基本信息",
}));
const __VLS_130 = __VLS_129({
    title: "基本信息",
}, ...__VLS_functionalComponentArgsRest(__VLS_129));
let __VLS_133;
/** @ts-ignore @type { | typeof __VLS_components.elStep | typeof __VLS_components.ElStep | typeof __VLS_components['el-step']} */
elStep;
// @ts-ignore
const __VLS_134 = __VLS_asFunctionalComponent1(__VLS_133, new __VLS_133({
    title: "选择模型",
}));
const __VLS_135 = __VLS_134({
    title: "选择模型",
}, ...__VLS_functionalComponentArgsRest(__VLS_134));
let __VLS_138;
/** @ts-ignore @type { | typeof __VLS_components.elStep | typeof __VLS_components.ElStep | typeof __VLS_components['el-step']} */
elStep;
// @ts-ignore
const __VLS_139 = __VLS_asFunctionalComponent1(__VLS_138, new __VLS_138({
    title: "消息通道",
}));
const __VLS_140 = __VLS_139({
    title: "消息通道",
}, ...__VLS_functionalComponentArgsRest(__VLS_139));
let __VLS_143;
/** @ts-ignore @type { | typeof __VLS_components.elStep | typeof __VLS_components.ElStep | typeof __VLS_components['el-step']} */
elStep;
// @ts-ignore
const __VLS_144 = __VLS_asFunctionalComponent1(__VLS_143, new __VLS_143({
    title: "开启能力",
}));
const __VLS_145 = __VLS_144({
    title: "开启能力",
}, ...__VLS_functionalComponentArgsRest(__VLS_144));
let __VLS_148;
/** @ts-ignore @type { | typeof __VLS_components.elStep | typeof __VLS_components.ElStep | typeof __VLS_components['el-step']} */
elStep;
// @ts-ignore
const __VLS_149 = __VLS_asFunctionalComponent1(__VLS_148, new __VLS_148({
    title: "安装 Skills",
}));
const __VLS_150 = __VLS_149({
    title: "安装 Skills",
}, ...__VLS_functionalComponentArgsRest(__VLS_149));
// @ts-ignore
[store, store, wizardVisible, wizardStep,];
var __VLS_125;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.wizardStep === 0) }, null, null);
let __VLS_153;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_154 = __VLS_asFunctionalComponent1(__VLS_153, new __VLS_153({
    model: (__VLS_ctx.wizardForm),
    labelWidth: "90px",
}));
const __VLS_155 = __VLS_154({
    model: (__VLS_ctx.wizardForm),
    labelWidth: "90px",
}, ...__VLS_functionalComponentArgsRest(__VLS_154));
const { default: __VLS_158 } = __VLS_156.slots;
let __VLS_159;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_160 = __VLS_asFunctionalComponent1(__VLS_159, new __VLS_159({
    label: "名称",
    required: true,
}));
const __VLS_161 = __VLS_160({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_160));
const { default: __VLS_164 } = __VLS_162.slots;
let __VLS_165;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_166 = __VLS_asFunctionalComponent1(__VLS_165, new __VLS_165({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.wizardForm.name),
    placeholder: "如：数据分析师",
}));
const __VLS_167 = __VLS_166({
    ...{ 'onInput': {} },
    modelValue: (__VLS_ctx.wizardForm.name),
    placeholder: "如：数据分析师",
}, ...__VLS_functionalComponentArgsRest(__VLS_166));
let __VLS_170;
const __VLS_171 = ({ input: {} },
    { onInput: (__VLS_ctx.autoId) });
var __VLS_168;
var __VLS_169;
// @ts-ignore
[wizardStep, wizardForm, wizardForm, autoId,];
var __VLS_162;
let __VLS_172;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_173 = __VLS_asFunctionalComponent1(__VLS_172, new __VLS_172({
    label: "ID",
}));
const __VLS_174 = __VLS_173({
    label: "ID",
}, ...__VLS_functionalComponentArgsRest(__VLS_173));
const { default: __VLS_177 } = __VLS_175.slots;
let __VLS_178;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_179 = __VLS_asFunctionalComponent1(__VLS_178, new __VLS_178({
    modelValue: (__VLS_ctx.wizardForm.id),
    placeholder: "英文标识",
}));
const __VLS_180 = __VLS_179({
    modelValue: (__VLS_ctx.wizardForm.id),
    placeholder: "英文标识",
}, ...__VLS_functionalComponentArgsRest(__VLS_179));
// @ts-ignore
[wizardForm,];
var __VLS_175;
let __VLS_183;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_184 = __VLS_asFunctionalComponent1(__VLS_183, new __VLS_183({
    label: "描述",
}));
const __VLS_185 = __VLS_184({
    label: "描述",
}, ...__VLS_functionalComponentArgsRest(__VLS_184));
const { default: __VLS_188 } = __VLS_186.slots;
let __VLS_189;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_190 = __VLS_asFunctionalComponent1(__VLS_189, new __VLS_189({
    modelValue: (__VLS_ctx.wizardForm.description),
    type: "textarea",
    rows: (2),
    placeholder: "简短描述这个 Agent 的职责",
}));
const __VLS_191 = __VLS_190({
    modelValue: (__VLS_ctx.wizardForm.description),
    type: "textarea",
    rows: (2),
    placeholder: "简短描述这个 Agent 的职责",
}, ...__VLS_functionalComponentArgsRest(__VLS_190));
// @ts-ignore
[wizardForm,];
var __VLS_186;
let __VLS_194;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_195 = __VLS_asFunctionalComponent1(__VLS_194, new __VLS_194({
    label: "头像颜色",
}));
const __VLS_196 = __VLS_195({
    label: "头像颜色",
}, ...__VLS_functionalComponentArgsRest(__VLS_195));
const { default: __VLS_199 } = __VLS_197.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [color] of __VLS_vFor((__VLS_ctx.avatarColors))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.wizardForm.avatarColor = color;
                // @ts-ignore
                [wizardForm, avatarColors,];
            } },
        key: (color),
        ...{ class: "color-swatch" },
        ...{ class: ({ active: __VLS_ctx.wizardForm.avatarColor === color }) },
        ...{ style: ({ background: color }) },
    });
    /** @type {__VLS_StyleScopedClasses['color-swatch']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    // @ts-ignore
    [wizardForm,];
}
// @ts-ignore
[];
var __VLS_197;
// @ts-ignore
[];
var __VLS_156;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.wizardStep === 1) }, null, null);
if (__VLS_ctx.modelsList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_200;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_201 = __VLS_asFunctionalComponent1(__VLS_200, new __VLS_200({
        description: "暂无已配置模型",
        imageSize: (60),
    }));
    const __VLS_202 = __VLS_201({
        description: "暂无已配置模型",
        imageSize: (60),
    }, ...__VLS_functionalComponentArgsRest(__VLS_201));
    let __VLS_205;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_206 = __VLS_asFunctionalComponent1(__VLS_205, new __VLS_205({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_207 = __VLS_206({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_206));
    let __VLS_210;
    const __VLS_211 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.modelsList.length === 0))
                    return;
                __VLS_ctx.$router.push('/config/models');
                __VLS_ctx.wizardVisible = false;
                // @ts-ignore
                [$router, wizardVisible, wizardStep, modelsList,];
            } });
    const { default: __VLS_212 } = __VLS_208.slots;
    // @ts-ignore
    [];
    var __VLS_208;
    var __VLS_209;
}
let __VLS_213;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_214 = __VLS_asFunctionalComponent1(__VLS_213, new __VLS_213({
    modelValue: (__VLS_ctx.wizardForm.modelId),
    ...{ style: {} },
}));
const __VLS_215 = __VLS_214({
    modelValue: (__VLS_ctx.wizardForm.modelId),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_214));
const { default: __VLS_218 } = __VLS_216.slots;
for (const [m] of __VLS_vFor((__VLS_ctx.modelsList))) {
    let __VLS_219;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_220 = __VLS_asFunctionalComponent1(__VLS_219, new __VLS_219({
        ...{ 'onClick': {} },
        key: (m.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.modelId === m.id }) },
    }));
    const __VLS_221 = __VLS_220({
        ...{ 'onClick': {} },
        key: (m.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.modelId === m.id }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_220));
    let __VLS_224;
    const __VLS_225 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.wizardForm.modelId = m.id;
                // @ts-ignore
                [wizardForm, wizardForm, wizardForm, modelsList,];
            } });
    /** @type {__VLS_StyleScopedClasses['select-card']} */ ;
    /** @type {__VLS_StyleScopedClasses['selected']} */ ;
    const { default: __VLS_226 } = __VLS_222.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_227;
    /** @ts-ignore @type { | typeof __VLS_components.elRadio | typeof __VLS_components.ElRadio | typeof __VLS_components['el-radio']} */
    elRadio;
    // @ts-ignore
    const __VLS_228 = __VLS_asFunctionalComponent1(__VLS_227, new __VLS_227({
        value: (m.id),
        ...{ style: {} },
    }));
    const __VLS_229 = __VLS_228({
        value: (m.id),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_228));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    (m.name);
    let __VLS_232;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_233 = __VLS_asFunctionalComponent1(__VLS_232, new __VLS_232({
        type: "info",
        size: "small",
    }));
    const __VLS_234 = __VLS_233({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_233));
    const { default: __VLS_237 } = __VLS_235.slots;
    (m.provider);
    (m.model);
    // @ts-ignore
    [];
    var __VLS_235;
    let __VLS_238;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_239 = __VLS_asFunctionalComponent1(__VLS_238, new __VLS_238({
        type: (m.status === 'ok' ? 'success' : m.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }));
    const __VLS_240 = __VLS_239({
        type: (m.status === 'ok' ? 'success' : m.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_239));
    const { default: __VLS_243 } = __VLS_241.slots;
    (m.status === 'ok' ? '✓ 已配置' : m.status === 'error' ? '✗ 错误' : '? 未测试');
    // @ts-ignore
    [];
    var __VLS_241;
    // @ts-ignore
    [];
    var __VLS_222;
    var __VLS_223;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_216;
let __VLS_244;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_245 = __VLS_asFunctionalComponent1(__VLS_244, new __VLS_244({
    ...{ 'onClick': {} },
    link: true,
    type: "primary",
    ...{ style: {} },
}));
const __VLS_246 = __VLS_245({
    ...{ 'onClick': {} },
    link: true,
    type: "primary",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_245));
let __VLS_249;
const __VLS_250 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.$router.push('/config/models');
            __VLS_ctx.wizardVisible = false;
            // @ts-ignore
            [$router, wizardVisible,];
        } });
const { default: __VLS_251 } = __VLS_247.slots;
// @ts-ignore
[];
var __VLS_247;
var __VLS_248;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.wizardStep === 2) }, null, null);
if (__VLS_ctx.channelsList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_252;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_253 = __VLS_asFunctionalComponent1(__VLS_252, new __VLS_252({
        description: "暂无消息通道（可跳过）",
        imageSize: (60),
    }));
    const __VLS_254 = __VLS_253({
        description: "暂无消息通道（可跳过）",
        imageSize: (60),
    }, ...__VLS_functionalComponentArgsRest(__VLS_253));
    let __VLS_257;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_258 = __VLS_asFunctionalComponent1(__VLS_257, new __VLS_257({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }));
    const __VLS_259 = __VLS_258({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_258));
    let __VLS_262;
    const __VLS_263 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.channelsList.length === 0))
                    return;
                __VLS_ctx.$router.push('/config/channels');
                __VLS_ctx.wizardVisible = false;
                // @ts-ignore
                [$router, wizardVisible, wizardStep, channelsList,];
            } });
    const { default: __VLS_264 } = __VLS_260.slots;
    // @ts-ignore
    [];
    var __VLS_260;
    var __VLS_261;
}
let __VLS_265;
/** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
elCheckboxGroup;
// @ts-ignore
const __VLS_266 = __VLS_asFunctionalComponent1(__VLS_265, new __VLS_265({
    modelValue: (__VLS_ctx.wizardForm.channelIds),
}));
const __VLS_267 = __VLS_266({
    modelValue: (__VLS_ctx.wizardForm.channelIds),
}, ...__VLS_functionalComponentArgsRest(__VLS_266));
const { default: __VLS_270 } = __VLS_268.slots;
for (const [ch] of __VLS_vFor((__VLS_ctx.channelsList))) {
    let __VLS_271;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_272 = __VLS_asFunctionalComponent1(__VLS_271, new __VLS_271({
        ...{ 'onClick': {} },
        key: (ch.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.channelIds.includes(ch.id) }) },
    }));
    const __VLS_273 = __VLS_272({
        ...{ 'onClick': {} },
        key: (ch.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.channelIds.includes(ch.id) }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_272));
    let __VLS_276;
    const __VLS_277 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.toggleArray(__VLS_ctx.wizardForm.channelIds, ch.id);
                // @ts-ignore
                [wizardForm, wizardForm, wizardForm, channelsList, toggleArray,];
            } });
    /** @type {__VLS_StyleScopedClasses['select-card']} */ ;
    /** @type {__VLS_StyleScopedClasses['selected']} */ ;
    const { default: __VLS_278 } = __VLS_274.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_279;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
    elCheckbox;
    // @ts-ignore
    const __VLS_280 = __VLS_asFunctionalComponent1(__VLS_279, new __VLS_279({
        ...{ 'onClick': {} },
        value: (ch.id),
    }));
    const __VLS_281 = __VLS_280({
        ...{ 'onClick': {} },
        value: (ch.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_280));
    let __VLS_284;
    const __VLS_285 = ({ click: {} },
        { onClick: () => { } });
    var __VLS_282;
    var __VLS_283;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    (ch.name);
    let __VLS_286;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_287 = __VLS_asFunctionalComponent1(__VLS_286, new __VLS_286({
        type: "info",
        size: "small",
    }));
    const __VLS_288 = __VLS_287({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_287));
    const { default: __VLS_291 } = __VLS_289.slots;
    (ch.type);
    // @ts-ignore
    [];
    var __VLS_289;
    let __VLS_292;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_293 = __VLS_asFunctionalComponent1(__VLS_292, new __VLS_292({
        type: (ch.enabled ? 'success' : 'info'),
        size: "small",
    }));
    const __VLS_294 = __VLS_293({
        type: (ch.enabled ? 'success' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_293));
    const { default: __VLS_297 } = __VLS_295.slots;
    (ch.enabled ? '启用' : '停用');
    // @ts-ignore
    [];
    var __VLS_295;
    // @ts-ignore
    [];
    var __VLS_274;
    var __VLS_275;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_268;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.wizardStep === 3) }, null, null);
if (__VLS_ctx.toolsList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_298;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_299 = __VLS_asFunctionalComponent1(__VLS_298, new __VLS_298({
        description: "暂无能力配置（可跳过）",
        imageSize: (60),
    }));
    const __VLS_300 = __VLS_299({
        description: "暂无能力配置（可跳过）",
        imageSize: (60),
    }, ...__VLS_functionalComponentArgsRest(__VLS_299));
    let __VLS_303;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_304 = __VLS_asFunctionalComponent1(__VLS_303, new __VLS_303({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }));
    const __VLS_305 = __VLS_304({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_304));
    let __VLS_308;
    const __VLS_309 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.toolsList.length === 0))
                    return;
                __VLS_ctx.$router.push('/config/tools');
                __VLS_ctx.wizardVisible = false;
                // @ts-ignore
                [$router, wizardVisible, wizardStep, toolsList,];
            } });
    const { default: __VLS_310 } = __VLS_306.slots;
    // @ts-ignore
    [];
    var __VLS_306;
    var __VLS_307;
}
let __VLS_311;
/** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
elCheckboxGroup;
// @ts-ignore
const __VLS_312 = __VLS_asFunctionalComponent1(__VLS_311, new __VLS_311({
    modelValue: (__VLS_ctx.wizardForm.toolIds),
}));
const __VLS_313 = __VLS_312({
    modelValue: (__VLS_ctx.wizardForm.toolIds),
}, ...__VLS_functionalComponentArgsRest(__VLS_312));
const { default: __VLS_316 } = __VLS_314.slots;
for (const [t] of __VLS_vFor((__VLS_ctx.toolsList))) {
    let __VLS_317;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_318 = __VLS_asFunctionalComponent1(__VLS_317, new __VLS_317({
        ...{ 'onClick': {} },
        key: (t.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.toolIds.includes(t.id) }) },
    }));
    const __VLS_319 = __VLS_318({
        ...{ 'onClick': {} },
        key: (t.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.toolIds.includes(t.id) }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_318));
    let __VLS_322;
    const __VLS_323 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.toggleArray(__VLS_ctx.wizardForm.toolIds, t.id);
                // @ts-ignore
                [wizardForm, wizardForm, wizardForm, toggleArray, toolsList,];
            } });
    /** @type {__VLS_StyleScopedClasses['select-card']} */ ;
    /** @type {__VLS_StyleScopedClasses['selected']} */ ;
    const { default: __VLS_324 } = __VLS_320.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_325;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
    elCheckbox;
    // @ts-ignore
    const __VLS_326 = __VLS_asFunctionalComponent1(__VLS_325, new __VLS_325({
        ...{ 'onClick': {} },
        value: (t.id),
    }));
    const __VLS_327 = __VLS_326({
        ...{ 'onClick': {} },
        value: (t.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_326));
    let __VLS_330;
    const __VLS_331 = ({ click: {} },
        { onClick: () => { } });
    var __VLS_328;
    var __VLS_329;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    (t.name);
    let __VLS_332;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_333 = __VLS_asFunctionalComponent1(__VLS_332, new __VLS_332({
        type: "info",
        size: "small",
    }));
    const __VLS_334 = __VLS_333({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_333));
    const { default: __VLS_337 } = __VLS_335.slots;
    (t.type);
    // @ts-ignore
    [];
    var __VLS_335;
    let __VLS_338;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_339 = __VLS_asFunctionalComponent1(__VLS_338, new __VLS_338({
        type: (t.status === 'ok' ? 'success' : 'info'),
        size: "small",
    }));
    const __VLS_340 = __VLS_339({
        type: (t.status === 'ok' ? 'success' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_339));
    const { default: __VLS_343 } = __VLS_341.slots;
    (t.status === 'ok' ? '✓' : '?');
    // @ts-ignore
    [];
    var __VLS_341;
    // @ts-ignore
    [];
    var __VLS_320;
    var __VLS_321;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_314;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.wizardStep === 4) }, null, null);
if (__VLS_ctx.skillsList.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_344;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_345 = __VLS_asFunctionalComponent1(__VLS_344, new __VLS_344({
        description: "暂无已安装 Skills（可跳过）",
        imageSize: (60),
    }));
    const __VLS_346 = __VLS_345({
        description: "暂无已安装 Skills（可跳过）",
        imageSize: (60),
    }, ...__VLS_functionalComponentArgsRest(__VLS_345));
    let __VLS_349;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_350 = __VLS_asFunctionalComponent1(__VLS_349, new __VLS_349({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }));
    const __VLS_351 = __VLS_350({
        ...{ 'onClick': {} },
        link: true,
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_350));
    let __VLS_354;
    const __VLS_355 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.skillsList.length === 0))
                    return;
                __VLS_ctx.$router.push('/config/skills');
                __VLS_ctx.wizardVisible = false;
                // @ts-ignore
                [$router, wizardVisible, wizardStep, skillsList,];
            } });
    const { default: __VLS_356 } = __VLS_352.slots;
    // @ts-ignore
    [];
    var __VLS_352;
    var __VLS_353;
}
let __VLS_357;
/** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
elCheckboxGroup;
// @ts-ignore
const __VLS_358 = __VLS_asFunctionalComponent1(__VLS_357, new __VLS_357({
    modelValue: (__VLS_ctx.wizardForm.skillIds),
}));
const __VLS_359 = __VLS_358({
    modelValue: (__VLS_ctx.wizardForm.skillIds),
}, ...__VLS_functionalComponentArgsRest(__VLS_358));
const { default: __VLS_362 } = __VLS_360.slots;
for (const [s] of __VLS_vFor((__VLS_ctx.skillsList))) {
    let __VLS_363;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_364 = __VLS_asFunctionalComponent1(__VLS_363, new __VLS_363({
        ...{ 'onClick': {} },
        key: (s.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.skillIds.includes(s.id) }) },
    }));
    const __VLS_365 = __VLS_364({
        ...{ 'onClick': {} },
        key: (s.id),
        shadow: "hover",
        ...{ class: "select-card" },
        ...{ class: ({ selected: __VLS_ctx.wizardForm.skillIds.includes(s.id) }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_364));
    let __VLS_368;
    const __VLS_369 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.toggleArray(__VLS_ctx.wizardForm.skillIds, s.id);
                // @ts-ignore
                [wizardForm, wizardForm, wizardForm, toggleArray, skillsList,];
            } });
    /** @type {__VLS_StyleScopedClasses['select-card']} */ ;
    /** @type {__VLS_StyleScopedClasses['selected']} */ ;
    const { default: __VLS_370 } = __VLS_366.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_371;
    /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
    elCheckbox;
    // @ts-ignore
    const __VLS_372 = __VLS_asFunctionalComponent1(__VLS_371, new __VLS_371({
        ...{ 'onClick': {} },
        value: (s.id),
    }));
    const __VLS_373 = __VLS_372({
        ...{ 'onClick': {} },
        value: (s.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_372));
    let __VLS_376;
    const __VLS_377 = ({ click: {} },
        { onClick: () => { } });
    var __VLS_374;
    var __VLS_375;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    (s.name);
    let __VLS_378;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_379 = __VLS_asFunctionalComponent1(__VLS_378, new __VLS_378({
        type: "info",
        size: "small",
    }));
    const __VLS_380 = __VLS_379({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_379));
    const { default: __VLS_383 } = __VLS_381.slots;
    (s.description);
    // @ts-ignore
    [];
    var __VLS_381;
    let __VLS_384;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_385 = __VLS_asFunctionalComponent1(__VLS_384, new __VLS_384({
        size: "small",
        type: "info",
    }));
    const __VLS_386 = __VLS_385({
        size: "small",
        type: "info",
    }, ...__VLS_functionalComponentArgsRest(__VLS_385));
    const { default: __VLS_389 } = __VLS_387.slots;
    (s.version);
    // @ts-ignore
    [];
    var __VLS_387;
    // @ts-ignore
    [];
    var __VLS_366;
    var __VLS_367;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_360;
{
    const { footer: __VLS_390 } = __VLS_119.slots;
    if (__VLS_ctx.wizardStep > 0) {
        let __VLS_391;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_392 = __VLS_asFunctionalComponent1(__VLS_391, new __VLS_391({
            ...{ 'onClick': {} },
        }));
        const __VLS_393 = __VLS_392({
            ...{ 'onClick': {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_392));
        let __VLS_396;
        const __VLS_397 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.wizardStep > 0))
                        return;
                    __VLS_ctx.wizardStep--;
                    // @ts-ignore
                    [wizardStep, wizardStep,];
                } });
        const { default: __VLS_398 } = __VLS_394.slots;
        // @ts-ignore
        [];
        var __VLS_394;
        var __VLS_395;
    }
    if (__VLS_ctx.wizardStep < 4) {
        let __VLS_399;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_400 = __VLS_asFunctionalComponent1(__VLS_399, new __VLS_399({
            ...{ 'onClick': {} },
            type: "primary",
        }));
        const __VLS_401 = __VLS_400({
            ...{ 'onClick': {} },
            type: "primary",
        }, ...__VLS_functionalComponentArgsRest(__VLS_400));
        let __VLS_404;
        const __VLS_405 = ({ click: {} },
            { onClick: (__VLS_ctx.nextStep) });
        const { default: __VLS_406 } = __VLS_402.slots;
        // @ts-ignore
        [wizardStep, nextStep,];
        var __VLS_402;
        var __VLS_403;
    }
    if (__VLS_ctx.wizardStep === 4) {
        let __VLS_407;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_408 = __VLS_asFunctionalComponent1(__VLS_407, new __VLS_407({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.creating),
        }));
        const __VLS_409 = __VLS_408({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.creating),
        }, ...__VLS_functionalComponentArgsRest(__VLS_408));
        let __VLS_412;
        const __VLS_413 = ({ click: {} },
            { onClick: (__VLS_ctx.createAgent) });
        const { default: __VLS_414 } = __VLS_410.slots;
        // @ts-ignore
        [wizardStep, creating, createAgent,];
        var __VLS_410;
        var __VLS_411;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_119;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
