/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Setting, Refresh, Upload, CircleCheckFilled, CircleCloseFilled, InfoFilled, WarningFilled, Loading, Lock } from '@element-plus/icons-vue';
import { config as configApi } from '../api';
import { useUpdater } from '../composables/useUpdater';
// ── 基本设置 ─────────────────────────────────────────────────────────────────
const port = ref(8080);
const token = ref('');
const lang = ref('zh');
const theme = ref('light');
const saving = ref(false);
onMounted(async () => {
    try {
        const res = await configApi.get();
        port.value = res.data.gateway?.port || 8080;
    }
    catch { }
    // 复用全局升级 composable（第一次调用自动 fetchVersion + 接管进行中任务，
    // 之后每次 onMounted 都立即返回，避免重复初始化）
    await updater.initFromBackend();
});
async function save() {
    saving.value = true;
    try {
        const patch = { gateway: { port: port.value } };
        if (token.value)
            patch.auth = { mode: 'token', token: token.value };
        await configApi.patch(patch);
        ElMessage.success('设置已保存');
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        saving.value = false;
    }
}
// ── 版本与更新 ────────────────────────────────────────────────────────────────
// 复用全局升级 composable，保证顶栏按钮和 Settings 页共享同一份状态机
// （不会出现"顶栏在跑 polling，Settings 看到另一份状态"）
const updater = useUpdater();
const currentVersion = updater.currentVersion;
const updateStatus = updater.updateStatus;
const updateRunning = updater.updateRunning;
const restartDetected = updater.restartDetected;
const checkResult = ref(null);
const checking = ref(false);
async function checkUpdate() {
    checking.value = true;
    checkResult.value = null;
    try {
        checkResult.value = await updater.checkForUpdate();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '检查更新失败，请检查网络');
    }
    finally {
        checking.value = false;
    }
}
async function applyUpdate() {
    if (!checkResult.value?.hasUpdate)
        return;
    try {
        await ElMessageBox.confirm(`确认将 ZyHive 从 ${currentVersion.value} 升级到 ${checkResult.value.latest}？\n\n升级过程中服务将短暂重启（约 10-30 秒），成员数据和配置文件不受影响。`, '确认升级', { confirmButtonText: '立即升级', cancelButtonText: '取消', type: 'warning' });
    }
    catch {
        return; // 用户取消
    }
    try {
        await updater.startUpgrade(checkResult.value.latest);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '启动升级失败');
    }
}
function reloadPage() { updater.reloadPage(); }
// 注：不再在 onBeforeUnmount 停 polling —— 升级状态是全局单例
// （composables/useUpdater.ts），顶栏在跑的任务切页面时不能被销毁。
// ── computed ──────────────────────────────────────────────────────────────────
const stageLabel = computed(() => {
    const map = {
        idle: '空闲',
        downloading: '下载中',
        verifying: '验证中',
        applying: '替换文件',
        done: '升级完成',
        failed: '升级失败',
        rolledback: '已回滚',
    };
    return map[updateStatus.value?.stage ?? 'idle'] ?? '';
});
const progressStatus = computed(() => {
    const s = updateStatus.value?.stage;
    if (s === 'done')
        return 'success';
    if (s === 'failed')
        return 'exception';
    if (s === 'rolledback')
        return 'warning';
    return undefined;
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['check-tip']} */ ;
/** @type {__VLS_StyleScopedClasses['check-tip']} */ ;
/** @type {__VLS_StyleScopedClasses['restart-tip']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "settings-page" },
});
/** @type {__VLS_StyleScopedClasses['settings-page']} */ ;
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
/** @ts-ignore @type { | typeof __VLS_components.Setting} */
Setting;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
let __VLS_11;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
    shadow: "hover",
    ...{ style: {} },
}));
const __VLS_13 = __VLS_12({
    shadow: "hover",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_12));
const { default: __VLS_16 } = __VLS_14.slots;
{
    const { header: __VLS_17 } = __VLS_14.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
let __VLS_18;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_19 = __VLS_asFunctionalComponent1(__VLS_18, new __VLS_18({
    labelWidth: "120px",
}));
const __VLS_20 = __VLS_19({
    labelWidth: "120px",
}, ...__VLS_functionalComponentArgsRest(__VLS_19));
const { default: __VLS_23 } = __VLS_21.slots;
let __VLS_24;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_25 = __VLS_asFunctionalComponent1(__VLS_24, new __VLS_24({
    label: "面板端口",
}));
const __VLS_26 = __VLS_25({
    label: "面板端口",
}, ...__VLS_functionalComponentArgsRest(__VLS_25));
const { default: __VLS_29 } = __VLS_27.slots;
let __VLS_30;
/** @ts-ignore @type { | typeof __VLS_components.elInputNumber | typeof __VLS_components.ElInputNumber | typeof __VLS_components['el-input-number']} */
elInputNumber;
// @ts-ignore
const __VLS_31 = __VLS_asFunctionalComponent1(__VLS_30, new __VLS_30({
    modelValue: (__VLS_ctx.port),
    min: (1024),
    max: (65535),
}));
const __VLS_32 = __VLS_31({
    modelValue: (__VLS_ctx.port),
    min: (1024),
    max: (65535),
}, ...__VLS_functionalComponentArgsRest(__VLS_31));
// @ts-ignore
[port,];
var __VLS_27;
let __VLS_35;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_36 = __VLS_asFunctionalComponent1(__VLS_35, new __VLS_35({
    label: "访问令牌",
}));
const __VLS_37 = __VLS_36({
    label: "访问令牌",
}, ...__VLS_functionalComponentArgsRest(__VLS_36));
const { default: __VLS_40 } = __VLS_38.slots;
let __VLS_41;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_42 = __VLS_asFunctionalComponent1(__VLS_41, new __VLS_41({
    modelValue: (__VLS_ctx.token),
    type: "password",
    showPassword: true,
    placeholder: "留空保持不变",
    ...{ style: {} },
}));
const __VLS_43 = __VLS_42({
    modelValue: (__VLS_ctx.token),
    type: "password",
    showPassword: true,
    placeholder: "留空保持不变",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_42));
// @ts-ignore
[token,];
var __VLS_38;
let __VLS_46;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_47 = __VLS_asFunctionalComponent1(__VLS_46, new __VLS_46({
    label: "语言",
}));
const __VLS_48 = __VLS_47({
    label: "语言",
}, ...__VLS_functionalComponentArgsRest(__VLS_47));
const { default: __VLS_51 } = __VLS_49.slots;
let __VLS_52;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_53 = __VLS_asFunctionalComponent1(__VLS_52, new __VLS_52({
    modelValue: (__VLS_ctx.lang),
    ...{ style: {} },
}));
const __VLS_54 = __VLS_53({
    modelValue: (__VLS_ctx.lang),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_53));
const { default: __VLS_57 } = __VLS_55.slots;
let __VLS_58;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_59 = __VLS_asFunctionalComponent1(__VLS_58, new __VLS_58({
    label: "中文",
    value: "zh",
}));
const __VLS_60 = __VLS_59({
    label: "中文",
    value: "zh",
}, ...__VLS_functionalComponentArgsRest(__VLS_59));
let __VLS_63;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_64 = __VLS_asFunctionalComponent1(__VLS_63, new __VLS_63({
    label: "English",
    value: "en",
    disabled: true,
}));
const __VLS_65 = __VLS_64({
    label: "English",
    value: "en",
    disabled: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_64));
// @ts-ignore
[lang,];
var __VLS_55;
// @ts-ignore
[];
var __VLS_49;
let __VLS_68;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_69 = __VLS_asFunctionalComponent1(__VLS_68, new __VLS_68({
    label: "主题",
}));
const __VLS_70 = __VLS_69({
    label: "主题",
}, ...__VLS_functionalComponentArgsRest(__VLS_69));
const { default: __VLS_73 } = __VLS_71.slots;
let __VLS_74;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({
    modelValue: (__VLS_ctx.theme),
    ...{ style: {} },
}));
const __VLS_76 = __VLS_75({
    modelValue: (__VLS_ctx.theme),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_75));
const { default: __VLS_79 } = __VLS_77.slots;
let __VLS_80;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
    label: "浅色",
    value: "light",
}));
const __VLS_82 = __VLS_81({
    label: "浅色",
    value: "light",
}, ...__VLS_functionalComponentArgsRest(__VLS_81));
let __VLS_85;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_86 = __VLS_asFunctionalComponent1(__VLS_85, new __VLS_85({
    label: "深色",
    value: "dark",
    disabled: true,
}));
const __VLS_87 = __VLS_86({
    label: "深色",
    value: "dark",
    disabled: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_86));
// @ts-ignore
[theme,];
var __VLS_77;
// @ts-ignore
[];
var __VLS_71;
let __VLS_90;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_91 = __VLS_asFunctionalComponent1(__VLS_90, new __VLS_90({}));
const __VLS_92 = __VLS_91({}, ...__VLS_functionalComponentArgsRest(__VLS_91));
const { default: __VLS_95 } = __VLS_93.slots;
let __VLS_96;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_97 = __VLS_asFunctionalComponent1(__VLS_96, new __VLS_96({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.saving),
}));
const __VLS_98 = __VLS_97({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.saving),
}, ...__VLS_functionalComponentArgsRest(__VLS_97));
let __VLS_101;
const __VLS_102 = ({ click: {} },
    { onClick: (__VLS_ctx.save) });
const { default: __VLS_103 } = __VLS_99.slots;
// @ts-ignore
[saving, save,];
var __VLS_99;
var __VLS_100;
// @ts-ignore
[];
var __VLS_93;
// @ts-ignore
[];
var __VLS_21;
// @ts-ignore
[];
var __VLS_14;
let __VLS_104;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_105 = __VLS_asFunctionalComponent1(__VLS_104, new __VLS_104({
    shadow: "hover",
    ...{ style: {} },
}));
const __VLS_106 = __VLS_105({
    shadow: "hover",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_105));
const { default: __VLS_109 } = __VLS_107.slots;
{
    const { header: __VLS_110 } = __VLS_107.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    // @ts-ignore
    [];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "version-section" },
});
/** @type {__VLS_StyleScopedClasses['version-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "version-row" },
});
/** @type {__VLS_StyleScopedClasses['version-row']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "version-info" },
});
/** @type {__VLS_StyleScopedClasses['version-info']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "version-label" },
});
/** @type {__VLS_StyleScopedClasses['version-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "version-value" },
});
/** @type {__VLS_StyleScopedClasses['version-value']} */ ;
let __VLS_111;
/** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
elTag;
// @ts-ignore
const __VLS_112 = __VLS_asFunctionalComponent1(__VLS_111, new __VLS_111({
    type: "info",
    size: "small",
}));
const __VLS_113 = __VLS_112({
    type: "info",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_112));
const { default: __VLS_116 } = __VLS_114.slots;
(__VLS_ctx.currentVersion || '…');
// @ts-ignore
[currentVersion,];
var __VLS_114;
if (__VLS_ctx.checkResult) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "version-info" },
    });
    /** @type {__VLS_StyleScopedClasses['version-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "version-label" },
    });
    /** @type {__VLS_StyleScopedClasses['version-label']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "version-value" },
    });
    /** @type {__VLS_StyleScopedClasses['version-value']} */ ;
    let __VLS_117;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_118 = __VLS_asFunctionalComponent1(__VLS_117, new __VLS_117({
        type: (__VLS_ctx.checkResult.hasUpdate ? 'success' : 'info'),
        size: "small",
    }));
    const __VLS_119 = __VLS_118({
        type: (__VLS_ctx.checkResult.hasUpdate ? 'success' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_118));
    const { default: __VLS_122 } = __VLS_120.slots;
    (__VLS_ctx.checkResult.latest);
    // @ts-ignore
    [checkResult, checkResult, checkResult,];
    var __VLS_120;
}
if (__VLS_ctx.checkResult && !__VLS_ctx.checkResult.hasUpdate && !__VLS_ctx.updateRunning) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "check-tip ok" },
    });
    /** @type {__VLS_StyleScopedClasses['check-tip']} */ ;
    /** @type {__VLS_StyleScopedClasses['ok']} */ ;
    let __VLS_123;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({}));
    const __VLS_125 = __VLS_124({}, ...__VLS_functionalComponentArgsRest(__VLS_124));
    const { default: __VLS_128 } = __VLS_126.slots;
    let __VLS_129;
    /** @ts-ignore @type { | typeof __VLS_components.CircleCheckFilled} */
    CircleCheckFilled;
    // @ts-ignore
    const __VLS_130 = __VLS_asFunctionalComponent1(__VLS_129, new __VLS_129({}));
    const __VLS_131 = __VLS_130({}, ...__VLS_functionalComponentArgsRest(__VLS_130));
    // @ts-ignore
    [checkResult, checkResult, updateRunning,];
    var __VLS_126;
}
if (__VLS_ctx.checkResult && __VLS_ctx.checkResult.hasUpdate && !__VLS_ctx.updateRunning) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "check-tip new" },
    });
    /** @type {__VLS_StyleScopedClasses['check-tip']} */ ;
    /** @type {__VLS_StyleScopedClasses['new']} */ ;
    let __VLS_134;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_135 = __VLS_asFunctionalComponent1(__VLS_134, new __VLS_134({}));
    const __VLS_136 = __VLS_135({}, ...__VLS_functionalComponentArgsRest(__VLS_135));
    const { default: __VLS_139 } = __VLS_137.slots;
    let __VLS_140;
    /** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
    InfoFilled;
    // @ts-ignore
    const __VLS_141 = __VLS_asFunctionalComponent1(__VLS_140, new __VLS_140({}));
    const __VLS_142 = __VLS_141({}, ...__VLS_functionalComponentArgsRest(__VLS_141));
    // @ts-ignore
    [checkResult, checkResult, updateRunning,];
    var __VLS_137;
    (__VLS_ctx.checkResult.latest);
    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
        href: (__VLS_ctx.checkResult.releaseUrl),
        target: "_blank",
        ...{ style: {} },
    });
}
if (__VLS_ctx.updateRunning || __VLS_ctx.updateStatus) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "update-progress" },
    });
    /** @type {__VLS_StyleScopedClasses['update-progress']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "progress-header" },
    });
    /** @type {__VLS_StyleScopedClasses['progress-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "stage-label" },
    });
    /** @type {__VLS_StyleScopedClasses['stage-label']} */ ;
    (__VLS_ctx.stageLabel);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "progress-pct" },
    });
    /** @type {__VLS_StyleScopedClasses['progress-pct']} */ ;
    (__VLS_ctx.updateStatus?.progress ?? 0);
    let __VLS_145;
    /** @ts-ignore @type { | typeof __VLS_components.elProgress | typeof __VLS_components.ElProgress | typeof __VLS_components['el-progress']} */
    elProgress;
    // @ts-ignore
    const __VLS_146 = __VLS_asFunctionalComponent1(__VLS_145, new __VLS_145({
        percentage: (__VLS_ctx.updateStatus?.progress ?? 0),
        status: (__VLS_ctx.progressStatus),
        striped: (__VLS_ctx.updateRunning),
        stripedFlow: (__VLS_ctx.updateRunning),
        duration: (1),
    }));
    const __VLS_147 = __VLS_146({
        percentage: (__VLS_ctx.updateStatus?.progress ?? 0),
        status: (__VLS_ctx.progressStatus),
        striped: (__VLS_ctx.updateRunning),
        stripedFlow: (__VLS_ctx.updateRunning),
        duration: (1),
    }, ...__VLS_functionalComponentArgsRest(__VLS_146));
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "progress-msg" },
    });
    /** @type {__VLS_StyleScopedClasses['progress-msg']} */ ;
    (__VLS_ctx.updateStatus?.message);
    if (__VLS_ctx.updateStatus?.stage === 'done' && __VLS_ctx.restartDetected) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "restart-tip" },
        });
        /** @type {__VLS_StyleScopedClasses['restart-tip']} */ ;
        let __VLS_150;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_151 = __VLS_asFunctionalComponent1(__VLS_150, new __VLS_150({}));
        const __VLS_152 = __VLS_151({}, ...__VLS_functionalComponentArgsRest(__VLS_151));
        const { default: __VLS_155 } = __VLS_153.slots;
        let __VLS_156;
        /** @ts-ignore @type { | typeof __VLS_components.CircleCheckFilled} */
        CircleCheckFilled;
        // @ts-ignore
        const __VLS_157 = __VLS_asFunctionalComponent1(__VLS_156, new __VLS_156({}));
        const __VLS_158 = __VLS_157({}, ...__VLS_functionalComponentArgsRest(__VLS_157));
        // @ts-ignore
        [checkResult, checkResult, updateRunning, updateRunning, updateRunning, updateStatus, updateStatus, updateStatus, updateStatus, updateStatus, stageLabel, progressStatus, restartDetected,];
        var __VLS_153;
        (__VLS_ctx.updateStatus.newVersion);
        let __VLS_161;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_162 = __VLS_asFunctionalComponent1(__VLS_161, new __VLS_161({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_163 = __VLS_162({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_162));
        let __VLS_166;
        const __VLS_167 = ({ click: {} },
            { onClick: (__VLS_ctx.reloadPage) });
        const { default: __VLS_168 } = __VLS_164.slots;
        // @ts-ignore
        [updateStatus, reloadPage,];
        var __VLS_164;
        var __VLS_165;
    }
    else if (__VLS_ctx.updateStatus?.stage === 'done') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "restart-tip waiting" },
        });
        /** @type {__VLS_StyleScopedClasses['restart-tip']} */ ;
        /** @type {__VLS_StyleScopedClasses['waiting']} */ ;
        let __VLS_169;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_170 = __VLS_asFunctionalComponent1(__VLS_169, new __VLS_169({
            ...{ class: "spin" },
        }));
        const __VLS_171 = __VLS_170({
            ...{ class: "spin" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_170));
        /** @type {__VLS_StyleScopedClasses['spin']} */ ;
        const { default: __VLS_174 } = __VLS_172.slots;
        let __VLS_175;
        /** @ts-ignore @type { | typeof __VLS_components.Loading} */
        Loading;
        // @ts-ignore
        const __VLS_176 = __VLS_asFunctionalComponent1(__VLS_175, new __VLS_175({}));
        const __VLS_177 = __VLS_176({}, ...__VLS_functionalComponentArgsRest(__VLS_176));
        // @ts-ignore
        [updateStatus,];
        var __VLS_172;
    }
    if (__VLS_ctx.updateStatus?.stage === 'failed') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "fail-tip" },
        });
        /** @type {__VLS_StyleScopedClasses['fail-tip']} */ ;
        let __VLS_180;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_181 = __VLS_asFunctionalComponent1(__VLS_180, new __VLS_180({}));
        const __VLS_182 = __VLS_181({}, ...__VLS_functionalComponentArgsRest(__VLS_181));
        const { default: __VLS_185 } = __VLS_183.slots;
        let __VLS_186;
        /** @ts-ignore @type { | typeof __VLS_components.CircleCloseFilled} */
        CircleCloseFilled;
        // @ts-ignore
        const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({}));
        const __VLS_188 = __VLS_187({}, ...__VLS_functionalComponentArgsRest(__VLS_187));
        // @ts-ignore
        [updateStatus,];
        var __VLS_183;
        (__VLS_ctx.updateStatus.message);
    }
    if (__VLS_ctx.updateStatus?.stage === 'rolledback') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "rollback-tip" },
        });
        /** @type {__VLS_StyleScopedClasses['rollback-tip']} */ ;
        let __VLS_191;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_192 = __VLS_asFunctionalComponent1(__VLS_191, new __VLS_191({}));
        const __VLS_193 = __VLS_192({}, ...__VLS_functionalComponentArgsRest(__VLS_192));
        const { default: __VLS_196 } = __VLS_194.slots;
        let __VLS_197;
        /** @ts-ignore @type { | typeof __VLS_components.WarningFilled} */
        WarningFilled;
        // @ts-ignore
        const __VLS_198 = __VLS_asFunctionalComponent1(__VLS_197, new __VLS_197({}));
        const __VLS_199 = __VLS_198({}, ...__VLS_functionalComponentArgsRest(__VLS_198));
        // @ts-ignore
        [updateStatus, updateStatus,];
        var __VLS_194;
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "update-actions" },
});
/** @type {__VLS_StyleScopedClasses['update-actions']} */ ;
let __VLS_202;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_203 = __VLS_asFunctionalComponent1(__VLS_202, new __VLS_202({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.checking),
    disabled: (__VLS_ctx.updateRunning),
}));
const __VLS_204 = __VLS_203({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.checking),
    disabled: (__VLS_ctx.updateRunning),
}, ...__VLS_functionalComponentArgsRest(__VLS_203));
let __VLS_207;
const __VLS_208 = ({ click: {} },
    { onClick: (__VLS_ctx.checkUpdate) });
const { default: __VLS_209 } = __VLS_205.slots;
let __VLS_210;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({
    ...{ style: {} },
}));
const __VLS_212 = __VLS_211({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_211));
const { default: __VLS_215 } = __VLS_213.slots;
let __VLS_216;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({}));
const __VLS_218 = __VLS_217({}, ...__VLS_functionalComponentArgsRest(__VLS_217));
// @ts-ignore
[updateRunning, checking, checkUpdate,];
var __VLS_213;
// @ts-ignore
[];
var __VLS_205;
var __VLS_206;
if (__VLS_ctx.checkResult?.hasUpdate) {
    let __VLS_221;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_222 = __VLS_asFunctionalComponent1(__VLS_221, new __VLS_221({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.updateRunning),
        disabled: (__VLS_ctx.updateRunning),
    }));
    const __VLS_223 = __VLS_222({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.updateRunning),
        disabled: (__VLS_ctx.updateRunning),
    }, ...__VLS_functionalComponentArgsRest(__VLS_222));
    let __VLS_226;
    const __VLS_227 = ({ click: {} },
        { onClick: (__VLS_ctx.applyUpdate) });
    const { default: __VLS_228 } = __VLS_224.slots;
    let __VLS_229;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_230 = __VLS_asFunctionalComponent1(__VLS_229, new __VLS_229({
        ...{ style: {} },
    }));
    const __VLS_231 = __VLS_230({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_230));
    const { default: __VLS_234 } = __VLS_232.slots;
    let __VLS_235;
    /** @ts-ignore @type { | typeof __VLS_components.Upload} */
    Upload;
    // @ts-ignore
    const __VLS_236 = __VLS_asFunctionalComponent1(__VLS_235, new __VLS_235({}));
    const __VLS_237 = __VLS_236({}, ...__VLS_functionalComponentArgsRest(__VLS_236));
    // @ts-ignore
    [checkResult, updateRunning, updateRunning, applyUpdate,];
    var __VLS_232;
    (__VLS_ctx.checkResult.latest);
    // @ts-ignore
    [checkResult,];
    var __VLS_224;
    var __VLS_225;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "data-safe-tip" },
});
/** @type {__VLS_StyleScopedClasses['data-safe-tip']} */ ;
let __VLS_240;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_241 = __VLS_asFunctionalComponent1(__VLS_240, new __VLS_240({}));
const __VLS_242 = __VLS_241({}, ...__VLS_functionalComponentArgsRest(__VLS_241));
const { default: __VLS_245 } = __VLS_243.slots;
let __VLS_246;
/** @ts-ignore @type { | typeof __VLS_components.Lock} */
Lock;
// @ts-ignore
const __VLS_247 = __VLS_asFunctionalComponent1(__VLS_246, new __VLS_246({}));
const __VLS_248 = __VLS_247({}, ...__VLS_functionalComponentArgsRest(__VLS_247));
// @ts-ignore
[];
var __VLS_243;
// @ts-ignore
[];
var __VLS_107;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
