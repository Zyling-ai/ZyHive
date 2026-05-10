/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, reactive, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api, { tools as toolsApi, config as configApi } from '../api';
const list = ref([]);
const dialogVisible = ref(false);
const editingId = ref('');
const saving = ref(false);
const form = reactive({
    id: '', name: '', type: 'brave_search', apiKey: '', baseUrl: '', enabled: true,
});
async function loadList() {
    try {
        const res = await toolsApi.list();
        list.value = res.data;
    }
    catch (e) {
        ElMessage.error('加载能力列表失败: ' + (e?.response?.data?.error || e?.message || '未知错误'));
    }
}
function openAdd() {
    editingId.value = '';
    Object.assign(form, { id: '', name: '', type: 'brave_search', apiKey: '', baseUrl: '', enabled: true });
    dialogVisible.value = true;
}
function openEdit(row) {
    editingId.value = row.id;
    Object.assign(form, { ...row });
    dialogVisible.value = true;
}
async function saveTool() {
    if (!form.name || !form.type) {
        ElMessage.warning('请填写必要字段');
        return;
    }
    if (!form.id) {
        form.id = form.type + '-' + Date.now().toString(36);
    }
    saving.value = true;
    try {
        if (editingId.value) {
            await toolsApi.update(editingId.value, { ...form });
        }
        else {
            await toolsApi.create({ ...form, status: 'untested' });
        }
        ElMessage.success('保存成功');
        dialogVisible.value = false;
        loadList();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        saving.value = false;
    }
}
async function toggleEnabled(row) {
    try {
        await toolsApi.update(row.id, { enabled: row.enabled });
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function testTool(row) {
    try {
        await toolsApi.test(row.id);
        ElMessage.success('测试成功');
        loadList();
    }
    catch {
        ElMessage.error('测试失败');
    }
}
async function deleteTool(row) {
    try {
        await ElMessageBox.confirm(`确定删除 "${row.name}"？`, '确认删除', { type: 'warning' });
        await toolsApi.delete(row.id);
        ElMessage.success('已删除');
        loadList();
    }
    catch { }
}
// ── 全局工具权限策略 ──────────────────────────────────────────────────────────
const globalPolicy = reactive({
    profile: '',
    allow: [],
    deny: [],
});
const globalPolicyAllowInput = ref('');
const globalPolicyDenyInput = ref('');
const globalPolicySaving = ref(false);
const globalPolicySaved = ref(false);
async function loadGlobalPolicy() {
    try {
        const res = await configApi.get();
        const p = res.data?.toolPolicy;
        globalPolicy.profile = p?.profile || '';
        globalPolicy.allow = p?.allow ? [...p.allow] : [];
        globalPolicy.deny = p?.deny ? [...p.deny] : [];
    }
    catch { }
}
function addGlobalTag(type) {
    const input = type === 'allow' ? globalPolicyAllowInput : globalPolicyDenyInput;
    const val = input.value.trim();
    if (!val)
        return;
    if (!globalPolicy[type].includes(val))
        globalPolicy[type].push(val);
    input.value = '';
}
async function saveGlobalPolicy() {
    globalPolicySaving.value = true;
    try {
        const policy = {};
        if (globalPolicy.profile)
            policy.profile = globalPolicy.profile;
        if (globalPolicy.allow.length)
            policy.allow = globalPolicy.allow;
        if (globalPolicy.deny.length)
            policy.deny = globalPolicy.deny;
        await configApi.patch({ toolPolicy: Object.keys(policy).length ? policy : null });
        globalPolicySaved.value = true;
        setTimeout(() => { globalPolicySaved.value = false; }, 2000);
        ElMessage.success('全局工具权限已保存，重启后生效');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        globalPolicySaving.value = false;
    }
}
const acpList = ref([]);
const acpDialogVisible = ref(false);
const editingACPId = ref('');
const acpSaving = ref(false);
const acpForm = reactive({ name: '', binary: '', workDir: '' });
const acpArgsStr = ref('');
const acpEnvStr = ref('');
async function loadACPList() {
    try {
        const res = await api.get('/acp');
        acpList.value = res.data;
    }
    catch (e) {
        ElMessage.error('加载 ACP 代理列表失败: ' + (e?.response?.data?.error || e?.message || '未知错误'));
    }
}
function openAddACP() {
    editingACPId.value = '';
    Object.assign(acpForm, { name: '', binary: '', workDir: '' });
    acpArgsStr.value = '';
    acpEnvStr.value = '';
    acpDialogVisible.value = true;
}
function openEditACP(row) {
    editingACPId.value = row.id;
    Object.assign(acpForm, { name: row.name, binary: row.binary, workDir: row.workDir || '' });
    acpArgsStr.value = row.args ? row.args.join(' ') : '';
    acpEnvStr.value = row.env ? row.env.join('\n') : '';
    acpDialogVisible.value = true;
}
async function saveACP() {
    if (!acpForm.name || !acpForm.binary) {
        ElMessage.warning('名称和可执行文件必填');
        return;
    }
    acpSaving.value = true;
    try {
        const args = acpArgsStr.value.trim() ? acpArgsStr.value.trim().split(/\s+/) : undefined;
        const env = acpEnvStr.value.trim() ? acpEnvStr.value.trim().split('\n').filter(Boolean) : undefined;
        const payload = { ...acpForm, args, env };
        if (editingACPId.value) {
            await api.patch(`/acp/${editingACPId.value}`, payload);
        }
        else {
            await api.post('/acp', payload);
        }
        ElMessage.success('已保存');
        acpDialogVisible.value = false;
        loadACPList();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        acpSaving.value = false;
    }
}
async function testACP(row) {
    try {
        const res = await api.post(`/acp/${row.id}/test`);
        if (res.data.status === 'ok') {
            ElMessage.success(`✅ ${row.binary} 存在：${res.data.path}`);
            row.status = 'ok';
        }
        else {
            ElMessage.error(`❌ 未找到：${res.data.error}`);
            row.status = 'error';
        }
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '测试失败');
    }
}
async function deleteACP(row) {
    try {
        await ElMessageBox.confirm(`确定删除「${row.name}」？`, '确认删除', { type: 'warning' });
        await api.delete(`/acp/${row.id}`);
        ElMessage.success('已删除');
        loadACPList();
    }
    catch { }
}
onMounted(() => { loadList(); loadGlobalPolicy(); loadACPList(); });
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "tools-page" },
});
/** @type {__VLS_StyleScopedClasses['tools-page']} */ ;
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
/** @ts-ignore @type { | typeof __VLS_components.SetUp} */
SetUp;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
let __VLS_11;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
    ...{ 'onClick': {} },
    type: "primary",
}));
const __VLS_13 = __VLS_12({
    ...{ 'onClick': {} },
    type: "primary",
}, ...__VLS_functionalComponentArgsRest(__VLS_12));
let __VLS_16;
const __VLS_17 = ({ click: {} },
    { onClick: (__VLS_ctx.openAdd) });
const { default: __VLS_18 } = __VLS_14.slots;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({}));
const __VLS_21 = __VLS_20({}, ...__VLS_functionalComponentArgsRest(__VLS_20));
const { default: __VLS_24 } = __VLS_22.slots;
let __VLS_25;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({}));
const __VLS_27 = __VLS_26({}, ...__VLS_functionalComponentArgsRest(__VLS_26));
// @ts-ignore
[openAdd,];
var __VLS_22;
// @ts-ignore
[];
var __VLS_14;
var __VLS_15;
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
let __VLS_30;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_31 = __VLS_asFunctionalComponent1(__VLS_30, new __VLS_30({
    shadow: "hover",
}));
const __VLS_32 = __VLS_31({
    shadow: "hover",
}, ...__VLS_functionalComponentArgsRest(__VLS_31));
const { default: __VLS_35 } = __VLS_33.slots;
let __VLS_36;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_37 = __VLS_asFunctionalComponent1(__VLS_36, new __VLS_36({
    data: (__VLS_ctx.list),
    stripe: true,
}));
const __VLS_38 = __VLS_37({
    data: (__VLS_ctx.list),
    stripe: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_37));
const { default: __VLS_41 } = __VLS_39.slots;
let __VLS_42;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_43 = __VLS_asFunctionalComponent1(__VLS_42, new __VLS_42({
    prop: "name",
    label: "名称",
    minWidth: "160",
}));
const __VLS_44 = __VLS_43({
    prop: "name",
    label: "名称",
    minWidth: "160",
}, ...__VLS_functionalComponentArgsRest(__VLS_43));
let __VLS_47;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_48 = __VLS_asFunctionalComponent1(__VLS_47, new __VLS_47({
    label: "类型",
    width: "140",
}));
const __VLS_49 = __VLS_48({
    label: "类型",
    width: "140",
}, ...__VLS_functionalComponentArgsRest(__VLS_48));
const { default: __VLS_52 } = __VLS_50.slots;
{
    const { default: __VLS_53 } = __VLS_50.slots;
    const [{ row }] = __VLS_vSlot(__VLS_53);
    let __VLS_54;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_55 = __VLS_asFunctionalComponent1(__VLS_54, new __VLS_54({
        size: "small",
    }));
    const __VLS_56 = __VLS_55({
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_55));
    const { default: __VLS_59 } = __VLS_57.slots;
    (row.type);
    // @ts-ignore
    [list,];
    var __VLS_57;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_50;
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    label: "API Key",
    minWidth: "180",
}));
const __VLS_62 = __VLS_61({
    label: "API Key",
    minWidth: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
const { default: __VLS_65 } = __VLS_63.slots;
{
    const { default: __VLS_66 } = __VLS_63.slots;
    const [{ row }] = __VLS_vSlot(__VLS_66);
    __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
        ...{ style: {} },
    });
    (row.apiKey);
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_63;
let __VLS_67;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_68 = __VLS_asFunctionalComponent1(__VLS_67, new __VLS_67({
    label: "启用",
    width: "80",
}));
const __VLS_69 = __VLS_68({
    label: "启用",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_68));
const { default: __VLS_72 } = __VLS_70.slots;
{
    const { default: __VLS_73 } = __VLS_70.slots;
    const [{ row }] = __VLS_vSlot(__VLS_73);
    let __VLS_74;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
    }));
    const __VLS_76 = __VLS_75({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_75));
    let __VLS_79;
    const __VLS_80 = ({ change: {} },
        { onChange: (...[$event]) => {
                __VLS_ctx.toggleEnabled(row);
                // @ts-ignore
                [toggleEnabled,];
            } });
    var __VLS_77;
    var __VLS_78;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_70;
let __VLS_81;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_82 = __VLS_asFunctionalComponent1(__VLS_81, new __VLS_81({
    label: "状态",
    width: "80",
}));
const __VLS_83 = __VLS_82({
    label: "状态",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_82));
const { default: __VLS_86 } = __VLS_84.slots;
{
    const { default: __VLS_87 } = __VLS_84.slots;
    const [{ row }] = __VLS_vSlot(__VLS_87);
    let __VLS_88;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_89 = __VLS_asFunctionalComponent1(__VLS_88, new __VLS_88({
        type: (row.status === 'ok' ? 'success' : 'info'),
        size: "small",
    }));
    const __VLS_90 = __VLS_89({
        type: (row.status === 'ok' ? 'success' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_89));
    const { default: __VLS_93 } = __VLS_91.slots;
    (row.status === 'ok' ? '✓' : '?');
    // @ts-ignore
    [];
    var __VLS_91;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_84;
let __VLS_94;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_95 = __VLS_asFunctionalComponent1(__VLS_94, new __VLS_94({
    label: "操作",
    width: "180",
}));
const __VLS_96 = __VLS_95({
    label: "操作",
    width: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_95));
const { default: __VLS_99 } = __VLS_97.slots;
{
    const { default: __VLS_100 } = __VLS_97.slots;
    const [{ row }] = __VLS_vSlot(__VLS_100);
    let __VLS_101;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_102 = __VLS_asFunctionalComponent1(__VLS_101, new __VLS_101({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_103 = __VLS_102({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_102));
    let __VLS_106;
    const __VLS_107 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.testTool(row);
                // @ts-ignore
                [testTool,];
            } });
    const { default: __VLS_108 } = __VLS_104.slots;
    // @ts-ignore
    [];
    var __VLS_104;
    var __VLS_105;
    let __VLS_109;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_110 = __VLS_asFunctionalComponent1(__VLS_109, new __VLS_109({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_111 = __VLS_110({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_110));
    let __VLS_114;
    const __VLS_115 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.openEdit(row);
                // @ts-ignore
                [openEdit,];
            } });
    const { default: __VLS_116 } = __VLS_112.slots;
    // @ts-ignore
    [];
    var __VLS_112;
    var __VLS_113;
    let __VLS_117;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_118 = __VLS_asFunctionalComponent1(__VLS_117, new __VLS_117({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
    }));
    const __VLS_119 = __VLS_118({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
    }, ...__VLS_functionalComponentArgsRest(__VLS_118));
    let __VLS_122;
    const __VLS_123 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.deleteTool(row);
                // @ts-ignore
                [deleteTool,];
            } });
    const { default: __VLS_124 } = __VLS_120.slots;
    // @ts-ignore
    [];
    var __VLS_120;
    var __VLS_121;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_97;
// @ts-ignore
[];
var __VLS_39;
// @ts-ignore
[];
var __VLS_33;
let __VLS_125;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_126 = __VLS_asFunctionalComponent1(__VLS_125, new __VLS_125({
    modelValue: (__VLS_ctx.dialogVisible),
    title: (__VLS_ctx.editingId ? '编辑能力' : '添加能力'),
    width: "520px",
}));
const __VLS_127 = __VLS_126({
    modelValue: (__VLS_ctx.dialogVisible),
    title: (__VLS_ctx.editingId ? '编辑能力' : '添加能力'),
    width: "520px",
}, ...__VLS_functionalComponentArgsRest(__VLS_126));
const { default: __VLS_130 } = __VLS_128.slots;
let __VLS_131;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_132 = __VLS_asFunctionalComponent1(__VLS_131, new __VLS_131({
    model: (__VLS_ctx.form),
    labelWidth: "100px",
}));
const __VLS_133 = __VLS_132({
    model: (__VLS_ctx.form),
    labelWidth: "100px",
}, ...__VLS_functionalComponentArgsRest(__VLS_132));
const { default: __VLS_136 } = __VLS_134.slots;
let __VLS_137;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_138 = __VLS_asFunctionalComponent1(__VLS_137, new __VLS_137({
    label: "类型",
    required: true,
}));
const __VLS_139 = __VLS_138({
    label: "类型",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_138));
const { default: __VLS_142 } = __VLS_140.slots;
let __VLS_143;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_144 = __VLS_asFunctionalComponent1(__VLS_143, new __VLS_143({
    modelValue: (__VLS_ctx.form.type),
    ...{ style: {} },
}));
const __VLS_145 = __VLS_144({
    modelValue: (__VLS_ctx.form.type),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_144));
const { default: __VLS_148 } = __VLS_146.slots;
let __VLS_149;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_150 = __VLS_asFunctionalComponent1(__VLS_149, new __VLS_149({
    label: "Brave Search",
    value: "brave_search",
}));
const __VLS_151 = __VLS_150({
    label: "Brave Search",
    value: "brave_search",
}, ...__VLS_functionalComponentArgsRest(__VLS_150));
let __VLS_154;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_155 = __VLS_asFunctionalComponent1(__VLS_154, new __VLS_154({
    label: "ElevenLabs",
    value: "elevenlabs",
}));
const __VLS_156 = __VLS_155({
    label: "ElevenLabs",
    value: "elevenlabs",
}, ...__VLS_functionalComponentArgsRest(__VLS_155));
let __VLS_159;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_160 = __VLS_asFunctionalComponent1(__VLS_159, new __VLS_159({
    label: "自定义",
    value: "custom",
}));
const __VLS_161 = __VLS_160({
    label: "自定义",
    value: "custom",
}, ...__VLS_functionalComponentArgsRest(__VLS_160));
// @ts-ignore
[dialogVisible, editingId, form, form,];
var __VLS_146;
// @ts-ignore
[];
var __VLS_140;
let __VLS_164;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_165 = __VLS_asFunctionalComponent1(__VLS_164, new __VLS_164({
    label: "名称",
    required: true,
}));
const __VLS_166 = __VLS_165({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_165));
const { default: __VLS_169 } = __VLS_167.slots;
let __VLS_170;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_171 = __VLS_asFunctionalComponent1(__VLS_170, new __VLS_170({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如 Brave Search",
}));
const __VLS_172 = __VLS_171({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如 Brave Search",
}, ...__VLS_functionalComponentArgsRest(__VLS_171));
// @ts-ignore
[form,];
var __VLS_167;
let __VLS_175;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_176 = __VLS_asFunctionalComponent1(__VLS_175, new __VLS_175({
    label: "ID",
}));
const __VLS_177 = __VLS_176({
    label: "ID",
}, ...__VLS_functionalComponentArgsRest(__VLS_176));
const { default: __VLS_180 } = __VLS_178.slots;
let __VLS_181;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_182 = __VLS_asFunctionalComponent1(__VLS_181, new __VLS_181({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "唯一标识",
}));
const __VLS_183 = __VLS_182({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "唯一标识",
}, ...__VLS_functionalComponentArgsRest(__VLS_182));
// @ts-ignore
[form,];
var __VLS_178;
let __VLS_186;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({
    label: "API Key",
    required: true,
}));
const __VLS_188 = __VLS_187({
    label: "API Key",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_187));
const { default: __VLS_191 } = __VLS_189.slots;
let __VLS_192;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({
    modelValue: (__VLS_ctx.form.apiKey),
    type: "password",
    showPassword: true,
}));
const __VLS_194 = __VLS_193({
    modelValue: (__VLS_ctx.form.apiKey),
    type: "password",
    showPassword: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_193));
// @ts-ignore
[form,];
var __VLS_189;
if (__VLS_ctx.form.type === 'custom') {
    let __VLS_197;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_198 = __VLS_asFunctionalComponent1(__VLS_197, new __VLS_197({
        label: "Base URL",
    }));
    const __VLS_199 = __VLS_198({
        label: "Base URL",
    }, ...__VLS_functionalComponentArgsRest(__VLS_198));
    const { default: __VLS_202 } = __VLS_200.slots;
    let __VLS_203;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_204 = __VLS_asFunctionalComponent1(__VLS_203, new __VLS_203({
        modelValue: (__VLS_ctx.form.baseUrl),
        placeholder: "https://...",
    }));
    const __VLS_205 = __VLS_204({
        modelValue: (__VLS_ctx.form.baseUrl),
        placeholder: "https://...",
    }, ...__VLS_functionalComponentArgsRest(__VLS_204));
    // @ts-ignore
    [form, form,];
    var __VLS_200;
}
let __VLS_208;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_209 = __VLS_asFunctionalComponent1(__VLS_208, new __VLS_208({
    label: "启用",
}));
const __VLS_210 = __VLS_209({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_209));
const { default: __VLS_213 } = __VLS_211.slots;
let __VLS_214;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_215 = __VLS_asFunctionalComponent1(__VLS_214, new __VLS_214({
    modelValue: (__VLS_ctx.form.enabled),
}));
const __VLS_216 = __VLS_215({
    modelValue: (__VLS_ctx.form.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_215));
// @ts-ignore
[form,];
var __VLS_211;
// @ts-ignore
[];
var __VLS_134;
{
    const { footer: __VLS_219 } = __VLS_128.slots;
    let __VLS_220;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_221 = __VLS_asFunctionalComponent1(__VLS_220, new __VLS_220({
        ...{ 'onClick': {} },
    }));
    const __VLS_222 = __VLS_221({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_221));
    let __VLS_225;
    const __VLS_226 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.dialogVisible = false;
                // @ts-ignore
                [dialogVisible,];
            } });
    const { default: __VLS_227 } = __VLS_223.slots;
    // @ts-ignore
    [];
    var __VLS_223;
    var __VLS_224;
    let __VLS_228;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_229 = __VLS_asFunctionalComponent1(__VLS_228, new __VLS_228({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.saving),
    }));
    const __VLS_230 = __VLS_229({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.saving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_229));
    let __VLS_233;
    const __VLS_234 = ({ click: {} },
        { onClick: (__VLS_ctx.saveTool) });
    const { default: __VLS_235 } = __VLS_231.slots;
    // @ts-ignore
    [saving, saveTool,];
    var __VLS_231;
    var __VLS_232;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_128;
let __VLS_236;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_237 = __VLS_asFunctionalComponent1(__VLS_236, new __VLS_236({
    shadow: "hover",
    ...{ style: {} },
}));
const __VLS_238 = __VLS_237({
    shadow: "hover",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_237));
const { default: __VLS_241 } = __VLS_239.slots;
{
    const { header: __VLS_242 } = __VLS_239.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    // @ts-ignore
    [];
}
let __VLS_243;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_244 = __VLS_asFunctionalComponent1(__VLS_243, new __VLS_243({
    labelWidth: "90px",
    size: "default",
    ...{ style: {} },
}));
const __VLS_245 = __VLS_244({
    labelWidth: "90px",
    size: "default",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_244));
const { default: __VLS_248 } = __VLS_246.slots;
let __VLS_249;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_250 = __VLS_asFunctionalComponent1(__VLS_249, new __VLS_249({
    label: "Profile",
}));
const __VLS_251 = __VLS_250({
    label: "Profile",
}, ...__VLS_functionalComponentArgsRest(__VLS_250));
const { default: __VLS_254 } = __VLS_252.slots;
let __VLS_255;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_256 = __VLS_asFunctionalComponent1(__VLS_255, new __VLS_255({
    modelValue: (__VLS_ctx.globalPolicy.profile),
    placeholder: "不限制（full）",
    ...{ style: {} },
    clearable: true,
}));
const __VLS_257 = __VLS_256({
    modelValue: (__VLS_ctx.globalPolicy.profile),
    placeholder: "不限制（full）",
    ...{ style: {} },
    clearable: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_256));
const { default: __VLS_260 } = __VLS_258.slots;
let __VLS_261;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_262 = __VLS_asFunctionalComponent1(__VLS_261, new __VLS_261({
    label: "full — 不限制（默认）",
    value: "full",
}));
const __VLS_263 = __VLS_262({
    label: "full — 不限制（默认）",
    value: "full",
}, ...__VLS_functionalComponentArgsRest(__VLS_262));
let __VLS_266;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_267 = __VLS_asFunctionalComponent1(__VLS_266, new __VLS_266({
    label: "coding — 文件+命令+Agent+记忆",
    value: "coding",
}));
const __VLS_268 = __VLS_267({
    label: "coding — 文件+命令+Agent+记忆",
    value: "coding",
}, ...__VLS_functionalComponentArgsRest(__VLS_267));
let __VLS_271;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_272 = __VLS_asFunctionalComponent1(__VLS_271, new __VLS_271({
    label: "messaging — 仅消息+Sessions",
    value: "messaging",
}));
const __VLS_273 = __VLS_272({
    label: "messaging — 仅消息+Sessions",
    value: "messaging",
}, ...__VLS_functionalComponentArgsRest(__VLS_272));
let __VLS_276;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_277 = __VLS_asFunctionalComponent1(__VLS_276, new __VLS_276({
    label: "minimal — 仅 send_message + 记忆",
    value: "minimal",
}));
const __VLS_278 = __VLS_277({
    label: "minimal — 仅 send_message + 记忆",
    value: "minimal",
}, ...__VLS_functionalComponentArgsRest(__VLS_277));
// @ts-ignore
[globalPolicy,];
var __VLS_258;
// @ts-ignore
[];
var __VLS_252;
let __VLS_281;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_282 = __VLS_asFunctionalComponent1(__VLS_281, new __VLS_281({
    label: "全局 Allow",
}));
const __VLS_283 = __VLS_282({
    label: "全局 Allow",
}, ...__VLS_functionalComponentArgsRest(__VLS_282));
const { default: __VLS_286 } = __VLS_284.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [item, idx] of __VLS_vFor((__VLS_ctx.globalPolicy.allow))) {
    let __VLS_287;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_288 = __VLS_asFunctionalComponent1(__VLS_287, new __VLS_287({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_289 = __VLS_288({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_288));
    let __VLS_292;
    const __VLS_293 = ({ close: {} },
        { onClose: (...[$event]) => {
                __VLS_ctx.globalPolicy.allow.splice(idx, 1);
                // @ts-ignore
                [globalPolicy, globalPolicy,];
            } });
    const { default: __VLS_294 } = __VLS_290.slots;
    (item);
    // @ts-ignore
    [];
    var __VLS_290;
    var __VLS_291;
    // @ts-ignore
    [];
}
let __VLS_295;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_296 = __VLS_asFunctionalComponent1(__VLS_295, new __VLS_295({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.globalPolicyAllowInput),
    size: "small",
    placeholder: "工具名或 group:xx，回车添加",
    ...{ style: {} },
}));
const __VLS_297 = __VLS_296({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.globalPolicyAllowInput),
    size: "small",
    placeholder: "工具名或 group:xx，回车添加",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_296));
let __VLS_300;
const __VLS_301 = ({ keyup: {} },
    { onKeyup: (...[$event]) => {
            __VLS_ctx.addGlobalTag('allow');
            // @ts-ignore
            [globalPolicyAllowInput, addGlobalTag,];
        } });
const { default: __VLS_302 } = __VLS_298.slots;
{
    const { append: __VLS_303 } = __VLS_298.slots;
    let __VLS_304;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_305 = __VLS_asFunctionalComponent1(__VLS_304, new __VLS_304({
        ...{ 'onClick': {} },
    }));
    const __VLS_306 = __VLS_305({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_305));
    let __VLS_309;
    const __VLS_310 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.addGlobalTag('allow');
                // @ts-ignore
                [addGlobalTag,];
            } });
    const { default: __VLS_311 } = __VLS_307.slots;
    // @ts-ignore
    [];
    var __VLS_307;
    var __VLS_308;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_298;
var __VLS_299;
// @ts-ignore
[];
var __VLS_284;
let __VLS_312;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_313 = __VLS_asFunctionalComponent1(__VLS_312, new __VLS_312({
    label: "全局 Deny",
}));
const __VLS_314 = __VLS_313({
    label: "全局 Deny",
}, ...__VLS_functionalComponentArgsRest(__VLS_313));
const { default: __VLS_317 } = __VLS_315.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [item, idx] of __VLS_vFor((__VLS_ctx.globalPolicy.deny))) {
    let __VLS_318;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_319 = __VLS_asFunctionalComponent1(__VLS_318, new __VLS_318({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        type: "danger",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_320 = __VLS_319({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        type: "danger",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_319));
    let __VLS_323;
    const __VLS_324 = ({ close: {} },
        { onClose: (...[$event]) => {
                __VLS_ctx.globalPolicy.deny.splice(idx, 1);
                // @ts-ignore
                [globalPolicy, globalPolicy,];
            } });
    const { default: __VLS_325 } = __VLS_321.slots;
    (item);
    // @ts-ignore
    [];
    var __VLS_321;
    var __VLS_322;
    // @ts-ignore
    [];
}
let __VLS_326;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_327 = __VLS_asFunctionalComponent1(__VLS_326, new __VLS_326({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.globalPolicyDenyInput),
    size: "small",
    placeholder: "工具名或 group:xx，回车拒绝",
    ...{ style: {} },
}));
const __VLS_328 = __VLS_327({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.globalPolicyDenyInput),
    size: "small",
    placeholder: "工具名或 group:xx，回车拒绝",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_327));
let __VLS_331;
const __VLS_332 = ({ keyup: {} },
    { onKeyup: (...[$event]) => {
            __VLS_ctx.addGlobalTag('deny');
            // @ts-ignore
            [addGlobalTag, globalPolicyDenyInput,];
        } });
const { default: __VLS_333 } = __VLS_329.slots;
{
    const { append: __VLS_334 } = __VLS_329.slots;
    let __VLS_335;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_336 = __VLS_asFunctionalComponent1(__VLS_335, new __VLS_335({
        ...{ 'onClick': {} },
    }));
    const __VLS_337 = __VLS_336({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_336));
    let __VLS_340;
    const __VLS_341 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.addGlobalTag('deny');
                // @ts-ignore
                [addGlobalTag,];
            } });
    const { default: __VLS_342 } = __VLS_338.slots;
    // @ts-ignore
    [];
    var __VLS_338;
    var __VLS_339;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_329;
var __VLS_330;
// @ts-ignore
[];
var __VLS_315;
let __VLS_343;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_344 = __VLS_asFunctionalComponent1(__VLS_343, new __VLS_343({
    label: "",
}));
const __VLS_345 = __VLS_344({
    label: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_344));
const { default: __VLS_348 } = __VLS_346.slots;
let __VLS_349;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_350 = __VLS_asFunctionalComponent1(__VLS_349, new __VLS_349({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.globalPolicySaving),
}));
const __VLS_351 = __VLS_350({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.globalPolicySaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_350));
let __VLS_354;
const __VLS_355 = ({ click: {} },
    { onClick: (__VLS_ctx.saveGlobalPolicy) });
const { default: __VLS_356 } = __VLS_352.slots;
// @ts-ignore
[globalPolicySaving, saveGlobalPolicy,];
var __VLS_352;
var __VLS_353;
let __VLS_357;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_358 = __VLS_asFunctionalComponent1(__VLS_357, new __VLS_357({
    ...{ 'onClick': {} },
    plain: true,
}));
const __VLS_359 = __VLS_358({
    ...{ 'onClick': {} },
    plain: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_358));
let __VLS_362;
const __VLS_363 = ({ click: {} },
    { onClick: (__VLS_ctx.loadGlobalPolicy) });
const { default: __VLS_364 } = __VLS_360.slots;
// @ts-ignore
[loadGlobalPolicy,];
var __VLS_360;
var __VLS_361;
if (__VLS_ctx.globalPolicySaved) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
// @ts-ignore
[globalPolicySaved,];
var __VLS_346;
// @ts-ignore
[];
var __VLS_246;
// @ts-ignore
[];
var __VLS_239;
let __VLS_365;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_366 = __VLS_asFunctionalComponent1(__VLS_365, new __VLS_365({
    shadow: "hover",
    ...{ style: {} },
}));
const __VLS_367 = __VLS_366({
    shadow: "hover",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_366));
const { default: __VLS_370 } = __VLS_368.slots;
{
    const { header: __VLS_371 } = __VLS_368.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_372;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_373 = __VLS_asFunctionalComponent1(__VLS_372, new __VLS_372({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }));
    const __VLS_374 = __VLS_373({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_373));
    let __VLS_377;
    const __VLS_378 = ({ click: {} },
        { onClick: (__VLS_ctx.openAddACP) });
    const { default: __VLS_379 } = __VLS_375.slots;
    // @ts-ignore
    [openAddACP,];
    var __VLS_375;
    var __VLS_376;
    // @ts-ignore
    [];
}
let __VLS_380;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_381 = __VLS_asFunctionalComponent1(__VLS_380, new __VLS_380({
    data: (__VLS_ctx.acpList),
    size: "small",
    ...{ style: {} },
    emptyText: "暂无 ACP 代理，点击「添加」配置",
}));
const __VLS_382 = __VLS_381({
    data: (__VLS_ctx.acpList),
    size: "small",
    ...{ style: {} },
    emptyText: "暂无 ACP 代理，点击「添加」配置",
}, ...__VLS_functionalComponentArgsRest(__VLS_381));
const { default: __VLS_385 } = __VLS_383.slots;
let __VLS_386;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_387 = __VLS_asFunctionalComponent1(__VLS_386, new __VLS_386({
    prop: "name",
    label: "名称",
    minWidth: "120",
}));
const __VLS_388 = __VLS_387({
    prop: "name",
    label: "名称",
    minWidth: "120",
}, ...__VLS_functionalComponentArgsRest(__VLS_387));
let __VLS_391;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_392 = __VLS_asFunctionalComponent1(__VLS_391, new __VLS_391({
    prop: "binary",
    label: "可执行文件",
    minWidth: "160",
}));
const __VLS_393 = __VLS_392({
    prop: "binary",
    label: "可执行文件",
    minWidth: "160",
}, ...__VLS_functionalComponentArgsRest(__VLS_392));
const { default: __VLS_396 } = __VLS_394.slots;
{
    const { default: __VLS_397 } = __VLS_394.slots;
    const [{ row }] = __VLS_vSlot(__VLS_397);
    __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
        ...{ style: {} },
    });
    (row.binary);
    // @ts-ignore
    [acpList,];
}
// @ts-ignore
[];
var __VLS_394;
let __VLS_398;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_399 = __VLS_asFunctionalComponent1(__VLS_398, new __VLS_398({
    label: "启动参数",
    minWidth: "180",
}));
const __VLS_400 = __VLS_399({
    label: "启动参数",
    minWidth: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_399));
const { default: __VLS_403 } = __VLS_401.slots;
{
    const { default: __VLS_404 } = __VLS_401.slots;
    const [{ row }] = __VLS_vSlot(__VLS_404);
    if (row.args?.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
            ...{ style: {} },
        });
        (row.args.join(' '));
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
var __VLS_401;
let __VLS_405;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_406 = __VLS_asFunctionalComponent1(__VLS_405, new __VLS_405({
    prop: "status",
    label: "状态",
    width: "90",
}));
const __VLS_407 = __VLS_406({
    prop: "status",
    label: "状态",
    width: "90",
}, ...__VLS_functionalComponentArgsRest(__VLS_406));
const { default: __VLS_410 } = __VLS_408.slots;
{
    const { default: __VLS_411 } = __VLS_408.slots;
    const [{ row }] = __VLS_vSlot(__VLS_411);
    let __VLS_412;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_413 = __VLS_asFunctionalComponent1(__VLS_412, new __VLS_412({
        type: (row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }));
    const __VLS_414 = __VLS_413({
        type: (row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_413));
    const { default: __VLS_417 } = __VLS_415.slots;
    (row.status || 'untested');
    // @ts-ignore
    [];
    var __VLS_415;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_408;
let __VLS_418;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_419 = __VLS_asFunctionalComponent1(__VLS_418, new __VLS_418({
    label: "操作",
    width: "140",
    fixed: "right",
}));
const __VLS_420 = __VLS_419({
    label: "操作",
    width: "140",
    fixed: "right",
}, ...__VLS_functionalComponentArgsRest(__VLS_419));
const { default: __VLS_423 } = __VLS_421.slots;
{
    const { default: __VLS_424 } = __VLS_421.slots;
    const [{ row }] = __VLS_vSlot(__VLS_424);
    let __VLS_425;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_426 = __VLS_asFunctionalComponent1(__VLS_425, new __VLS_425({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }));
    const __VLS_427 = __VLS_426({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_426));
    let __VLS_430;
    const __VLS_431 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.testACP(row);
                // @ts-ignore
                [testACP,];
            } });
    const { default: __VLS_432 } = __VLS_428.slots;
    // @ts-ignore
    [];
    var __VLS_428;
    var __VLS_429;
    let __VLS_433;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_434 = __VLS_asFunctionalComponent1(__VLS_433, new __VLS_433({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }));
    const __VLS_435 = __VLS_434({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_434));
    let __VLS_438;
    const __VLS_439 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.openEditACP(row);
                // @ts-ignore
                [openEditACP,];
            } });
    const { default: __VLS_440 } = __VLS_436.slots;
    // @ts-ignore
    [];
    var __VLS_436;
    var __VLS_437;
    let __VLS_441;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_442 = __VLS_asFunctionalComponent1(__VLS_441, new __VLS_441({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
        type: "danger",
    }));
    const __VLS_443 = __VLS_442({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
        type: "danger",
    }, ...__VLS_functionalComponentArgsRest(__VLS_442));
    let __VLS_446;
    const __VLS_447 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.deleteACP(row);
                // @ts-ignore
                [deleteACP,];
            } });
    const { default: __VLS_448 } = __VLS_444.slots;
    // @ts-ignore
    [];
    var __VLS_444;
    var __VLS_445;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_421;
// @ts-ignore
[];
var __VLS_383;
// @ts-ignore
[];
var __VLS_368;
let __VLS_449;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_450 = __VLS_asFunctionalComponent1(__VLS_449, new __VLS_449({
    modelValue: (__VLS_ctx.acpDialogVisible),
    title: (__VLS_ctx.editingACPId ? '编辑 ACP 代理' : '添加 ACP 代理'),
    width: "540px",
}));
const __VLS_451 = __VLS_450({
    modelValue: (__VLS_ctx.acpDialogVisible),
    title: (__VLS_ctx.editingACPId ? '编辑 ACP 代理' : '添加 ACP 代理'),
    width: "540px",
}, ...__VLS_functionalComponentArgsRest(__VLS_450));
const { default: __VLS_454 } = __VLS_452.slots;
let __VLS_455;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_456 = __VLS_asFunctionalComponent1(__VLS_455, new __VLS_455({
    model: (__VLS_ctx.acpForm),
    labelWidth: "100px",
    size: "default",
}));
const __VLS_457 = __VLS_456({
    model: (__VLS_ctx.acpForm),
    labelWidth: "100px",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_456));
const { default: __VLS_460 } = __VLS_458.slots;
let __VLS_461;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_462 = __VLS_asFunctionalComponent1(__VLS_461, new __VLS_461({
    label: "名称",
    required: true,
}));
const __VLS_463 = __VLS_462({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_462));
const { default: __VLS_466 } = __VLS_464.slots;
let __VLS_467;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_468 = __VLS_asFunctionalComponent1(__VLS_467, new __VLS_467({
    modelValue: (__VLS_ctx.acpForm.name),
    placeholder: "如 Claude Code",
}));
const __VLS_469 = __VLS_468({
    modelValue: (__VLS_ctx.acpForm.name),
    placeholder: "如 Claude Code",
}, ...__VLS_functionalComponentArgsRest(__VLS_468));
// @ts-ignore
[acpDialogVisible, editingACPId, acpForm, acpForm,];
var __VLS_464;
let __VLS_472;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_473 = __VLS_asFunctionalComponent1(__VLS_472, new __VLS_472({
    label: "可执行文件",
    required: true,
}));
const __VLS_474 = __VLS_473({
    label: "可执行文件",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_473));
const { default: __VLS_477 } = __VLS_475.slots;
let __VLS_478;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_479 = __VLS_asFunctionalComponent1(__VLS_478, new __VLS_478({
    modelValue: (__VLS_ctx.acpForm.binary),
    placeholder: "如 claude 或 /usr/local/bin/codex",
    ...{ style: {} },
}));
const __VLS_480 = __VLS_479({
    modelValue: (__VLS_ctx.acpForm.binary),
    placeholder: "如 claude 或 /usr/local/bin/codex",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_479));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
// @ts-ignore
[acpForm,];
var __VLS_475;
let __VLS_483;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_484 = __VLS_asFunctionalComponent1(__VLS_483, new __VLS_483({
    label: "启动参数",
}));
const __VLS_485 = __VLS_484({
    label: "启动参数",
}, ...__VLS_functionalComponentArgsRest(__VLS_484));
const { default: __VLS_488 } = __VLS_486.slots;
let __VLS_489;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_490 = __VLS_asFunctionalComponent1(__VLS_489, new __VLS_489({
    modelValue: (__VLS_ctx.acpArgsStr),
    placeholder: "如 --print  或  chat --task {{task}}",
    ...{ style: {} },
}));
const __VLS_491 = __VLS_490({
    modelValue: (__VLS_ctx.acpArgsStr),
    placeholder: "如 --print  或  chat --task {{task}}",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_490));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
// @ts-ignore
[acpArgsStr,];
var __VLS_486;
let __VLS_494;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_495 = __VLS_asFunctionalComponent1(__VLS_494, new __VLS_494({
    label: "工作目录",
}));
const __VLS_496 = __VLS_495({
    label: "工作目录",
}, ...__VLS_functionalComponentArgsRest(__VLS_495));
const { default: __VLS_499 } = __VLS_497.slots;
let __VLS_500;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_501 = __VLS_asFunctionalComponent1(__VLS_500, new __VLS_500({
    modelValue: (__VLS_ctx.acpForm.workDir),
    placeholder: "留空 = 用成员工作区",
    ...{ style: {} },
}));
const __VLS_502 = __VLS_501({
    modelValue: (__VLS_ctx.acpForm.workDir),
    placeholder: "留空 = 用成员工作区",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_501));
// @ts-ignore
[acpForm,];
var __VLS_497;
let __VLS_505;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_506 = __VLS_asFunctionalComponent1(__VLS_505, new __VLS_505({
    label: "环境变量",
}));
const __VLS_507 = __VLS_506({
    label: "环境变量",
}, ...__VLS_functionalComponentArgsRest(__VLS_506));
const { default: __VLS_510 } = __VLS_508.slots;
let __VLS_511;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_512 = __VLS_asFunctionalComponent1(__VLS_511, new __VLS_511({
    modelValue: (__VLS_ctx.acpEnvStr),
    type: "textarea",
    rows: (2),
    placeholder: "每行一个 KEY=VALUE",
    ...{ style: {} },
}));
const __VLS_513 = __VLS_512({
    modelValue: (__VLS_ctx.acpEnvStr),
    type: "textarea",
    rows: (2),
    placeholder: "每行一个 KEY=VALUE",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_512));
// @ts-ignore
[acpEnvStr,];
var __VLS_508;
// @ts-ignore
[];
var __VLS_458;
{
    const { footer: __VLS_516 } = __VLS_452.slots;
    let __VLS_517;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_518 = __VLS_asFunctionalComponent1(__VLS_517, new __VLS_517({
        ...{ 'onClick': {} },
    }));
    const __VLS_519 = __VLS_518({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_518));
    let __VLS_522;
    const __VLS_523 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.acpDialogVisible = false;
                // @ts-ignore
                [acpDialogVisible,];
            } });
    const { default: __VLS_524 } = __VLS_520.slots;
    // @ts-ignore
    [];
    var __VLS_520;
    var __VLS_521;
    let __VLS_525;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_526 = __VLS_asFunctionalComponent1(__VLS_525, new __VLS_525({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.acpSaving),
    }));
    const __VLS_527 = __VLS_526({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.acpSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_526));
    let __VLS_530;
    const __VLS_531 = ({ click: {} },
        { onClick: (__VLS_ctx.saveACP) });
    const { default: __VLS_532 } = __VLS_528.slots;
    // @ts-ignore
    [acpSaving, saveACP,];
    var __VLS_528;
    var __VLS_529;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_452;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
