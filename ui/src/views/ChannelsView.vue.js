/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, reactive, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { channels as channelsApi } from '../api';
const list = ref([]);
const dialogVisible = ref(false);
const editingId = ref('');
const saving = ref(false);
const form = reactive({
    id: '', name: '', type: 'telegram', enabled: true,
    config: { botToken: '', defaultAgent: 'main', allowedFrom: '' },
});
onMounted(loadList);
async function loadList() {
    try {
        const res = await channelsApi.list();
        list.value = res.data;
    }
    catch { }
}
function openAdd() {
    editingId.value = '';
    Object.assign(form, {
        id: '', name: '', type: 'telegram', enabled: true,
        config: { botToken: '', defaultAgent: 'main', allowedFrom: '' },
    });
    dialogVisible.value = true;
}
function openEdit(row) {
    editingId.value = row.id;
    Object.assign(form, { ...row, config: { ...row.config } });
    dialogVisible.value = true;
}
async function saveChannel() {
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
            await channelsApi.update(editingId.value, { ...form });
        }
        else {
            await channelsApi.create({ ...form, status: 'untested' });
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
        await channelsApi.update(row.id, { enabled: row.enabled });
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function testChannel(row) {
    try {
        await channelsApi.test(row.id);
        ElMessage.success('测试成功');
        loadList();
    }
    catch {
        ElMessage.error('测试失败');
    }
}
async function deleteChannel(row) {
    try {
        await ElMessageBox.confirm(`确定删除通道 "${row.name}"？`, '确认删除', { type: 'warning' });
        await channelsApi.delete(row.id);
        ElMessage.success('已删除');
        loadList();
    }
    catch { }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "channels-page" },
});
/** @type {__VLS_StyleScopedClasses['channels-page']} */ ;
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
/** @ts-ignore @type { | typeof __VLS_components.Connection} */
Connection;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
var __VLS_3;
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
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_43 = __VLS_asFunctionalComponent1(__VLS_42, new __VLS_42({
    label: "类型",
    width: "120",
}));
const __VLS_44 = __VLS_43({
    label: "类型",
    width: "120",
}, ...__VLS_functionalComponentArgsRest(__VLS_43));
const { default: __VLS_47 } = __VLS_45.slots;
{
    const { default: __VLS_48 } = __VLS_45.slots;
    const [{ row }] = __VLS_vSlot(__VLS_48);
    let __VLS_49;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
        size: "small",
    }));
    const __VLS_51 = __VLS_50({
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_50));
    const { default: __VLS_54 } = __VLS_52.slots;
    (row.type);
    // @ts-ignore
    [list,];
    var __VLS_52;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_45;
let __VLS_55;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_56 = __VLS_asFunctionalComponent1(__VLS_55, new __VLS_55({
    prop: "name",
    label: "名称",
    minWidth: "160",
}));
const __VLS_57 = __VLS_56({
    prop: "name",
    label: "名称",
    minWidth: "160",
}, ...__VLS_functionalComponentArgsRest(__VLS_56));
let __VLS_60;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_61 = __VLS_asFunctionalComponent1(__VLS_60, new __VLS_60({
    label: "配置",
    minWidth: "200",
}));
const __VLS_62 = __VLS_61({
    label: "配置",
    minWidth: "200",
}, ...__VLS_functionalComponentArgsRest(__VLS_61));
const { default: __VLS_65 } = __VLS_63.slots;
{
    const { default: __VLS_66 } = __VLS_63.slots;
    const [{ row }] = __VLS_vSlot(__VLS_66);
    let __VLS_67;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_68 = __VLS_asFunctionalComponent1(__VLS_67, new __VLS_67({
        type: "info",
        size: "small",
    }));
    const __VLS_69 = __VLS_68({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_68));
    const { default: __VLS_72 } = __VLS_70.slots;
    for (const [v, k] of __VLS_vFor((row.config))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            key: (k),
            ...{ style: {} },
        });
        (k);
        (v);
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_70;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_63;
let __VLS_73;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_74 = __VLS_asFunctionalComponent1(__VLS_73, new __VLS_73({
    label: "启用",
    width: "80",
}));
const __VLS_75 = __VLS_74({
    label: "启用",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_74));
const { default: __VLS_78 } = __VLS_76.slots;
{
    const { default: __VLS_79 } = __VLS_76.slots;
    const [{ row }] = __VLS_vSlot(__VLS_79);
    let __VLS_80;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
    }));
    const __VLS_82 = __VLS_81({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_81));
    let __VLS_85;
    const __VLS_86 = ({ change: {} },
        { onChange: (...[$event]) => {
                __VLS_ctx.toggleEnabled(row);
                // @ts-ignore
                [toggleEnabled,];
            } });
    var __VLS_83;
    var __VLS_84;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_76;
let __VLS_87;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_88 = __VLS_asFunctionalComponent1(__VLS_87, new __VLS_87({
    label: "状态",
    width: "100",
}));
const __VLS_89 = __VLS_88({
    label: "状态",
    width: "100",
}, ...__VLS_functionalComponentArgsRest(__VLS_88));
const { default: __VLS_92 } = __VLS_90.slots;
{
    const { default: __VLS_93 } = __VLS_90.slots;
    const [{ row }] = __VLS_vSlot(__VLS_93);
    let __VLS_94;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_95 = __VLS_asFunctionalComponent1(__VLS_94, new __VLS_94({
        type: (row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }));
    const __VLS_96 = __VLS_95({
        type: (row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_95));
    const { default: __VLS_99 } = __VLS_97.slots;
    (row.status === 'ok' ? '✓' : row.status === 'error' ? '✗' : '?');
    // @ts-ignore
    [];
    var __VLS_97;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_90;
let __VLS_100;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_101 = __VLS_asFunctionalComponent1(__VLS_100, new __VLS_100({
    label: "操作",
    width: "180",
}));
const __VLS_102 = __VLS_101({
    label: "操作",
    width: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_101));
const { default: __VLS_105 } = __VLS_103.slots;
{
    const { default: __VLS_106 } = __VLS_103.slots;
    const [{ row }] = __VLS_vSlot(__VLS_106);
    let __VLS_107;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_108 = __VLS_asFunctionalComponent1(__VLS_107, new __VLS_107({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_109 = __VLS_108({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_108));
    let __VLS_112;
    const __VLS_113 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.testChannel(row);
                // @ts-ignore
                [testChannel,];
            } });
    const { default: __VLS_114 } = __VLS_110.slots;
    // @ts-ignore
    [];
    var __VLS_110;
    var __VLS_111;
    let __VLS_115;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_117 = __VLS_116({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_116));
    let __VLS_120;
    const __VLS_121 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.openEdit(row);
                // @ts-ignore
                [openEdit,];
            } });
    const { default: __VLS_122 } = __VLS_118.slots;
    // @ts-ignore
    [];
    var __VLS_118;
    var __VLS_119;
    let __VLS_123;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
    }));
    const __VLS_125 = __VLS_124({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
    }, ...__VLS_functionalComponentArgsRest(__VLS_124));
    let __VLS_128;
    const __VLS_129 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.deleteChannel(row);
                // @ts-ignore
                [deleteChannel,];
            } });
    const { default: __VLS_130 } = __VLS_126.slots;
    // @ts-ignore
    [];
    var __VLS_126;
    var __VLS_127;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_103;
// @ts-ignore
[];
var __VLS_39;
// @ts-ignore
[];
var __VLS_33;
let __VLS_131;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_132 = __VLS_asFunctionalComponent1(__VLS_131, new __VLS_131({
    modelValue: (__VLS_ctx.dialogVisible),
    title: (__VLS_ctx.editingId ? '编辑通道' : '添加通道'),
    width: "520px",
}));
const __VLS_133 = __VLS_132({
    modelValue: (__VLS_ctx.dialogVisible),
    title: (__VLS_ctx.editingId ? '编辑通道' : '添加通道'),
    width: "520px",
}, ...__VLS_functionalComponentArgsRest(__VLS_132));
const { default: __VLS_136 } = __VLS_134.slots;
let __VLS_137;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_138 = __VLS_asFunctionalComponent1(__VLS_137, new __VLS_137({
    model: (__VLS_ctx.form),
    labelWidth: "120px",
}));
const __VLS_139 = __VLS_138({
    model: (__VLS_ctx.form),
    labelWidth: "120px",
}, ...__VLS_functionalComponentArgsRest(__VLS_138));
const { default: __VLS_142 } = __VLS_140.slots;
let __VLS_143;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_144 = __VLS_asFunctionalComponent1(__VLS_143, new __VLS_143({
    label: "类型",
    required: true,
}));
const __VLS_145 = __VLS_144({
    label: "类型",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_144));
const { default: __VLS_148 } = __VLS_146.slots;
let __VLS_149;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_150 = __VLS_asFunctionalComponent1(__VLS_149, new __VLS_149({
    modelValue: (__VLS_ctx.form.type),
    ...{ style: {} },
}));
const __VLS_151 = __VLS_150({
    modelValue: (__VLS_ctx.form.type),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_150));
const { default: __VLS_154 } = __VLS_152.slots;
let __VLS_155;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_156 = __VLS_asFunctionalComponent1(__VLS_155, new __VLS_155({
    label: "Telegram",
    value: "telegram",
}));
const __VLS_157 = __VLS_156({
    label: "Telegram",
    value: "telegram",
}, ...__VLS_functionalComponentArgsRest(__VLS_156));
let __VLS_160;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_161 = __VLS_asFunctionalComponent1(__VLS_160, new __VLS_160({
    label: "iMessage",
    value: "imessage",
}));
const __VLS_162 = __VLS_161({
    label: "iMessage",
    value: "imessage",
}, ...__VLS_functionalComponentArgsRest(__VLS_161));
let __VLS_165;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_166 = __VLS_asFunctionalComponent1(__VLS_165, new __VLS_165({
    label: "WhatsApp",
    value: "whatsapp",
}));
const __VLS_167 = __VLS_166({
    label: "WhatsApp",
    value: "whatsapp",
}, ...__VLS_functionalComponentArgsRest(__VLS_166));
// @ts-ignore
[dialogVisible, editingId, form, form,];
var __VLS_152;
// @ts-ignore
[];
var __VLS_146;
let __VLS_170;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_171 = __VLS_asFunctionalComponent1(__VLS_170, new __VLS_170({
    label: "名称",
    required: true,
}));
const __VLS_172 = __VLS_171({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_171));
const { default: __VLS_175 } = __VLS_173.slots;
let __VLS_176;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_177 = __VLS_asFunctionalComponent1(__VLS_176, new __VLS_176({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如 Telegram Bot",
}));
const __VLS_178 = __VLS_177({
    modelValue: (__VLS_ctx.form.name),
    placeholder: "如 Telegram Bot",
}, ...__VLS_functionalComponentArgsRest(__VLS_177));
// @ts-ignore
[form,];
var __VLS_173;
let __VLS_181;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_182 = __VLS_asFunctionalComponent1(__VLS_181, new __VLS_181({
    label: "ID",
}));
const __VLS_183 = __VLS_182({
    label: "ID",
}, ...__VLS_functionalComponentArgsRest(__VLS_182));
const { default: __VLS_186 } = __VLS_184.slots;
let __VLS_187;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_188 = __VLS_asFunctionalComponent1(__VLS_187, new __VLS_187({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "唯一标识",
}));
const __VLS_189 = __VLS_188({
    modelValue: (__VLS_ctx.form.id),
    placeholder: "唯一标识",
}, ...__VLS_functionalComponentArgsRest(__VLS_188));
// @ts-ignore
[form,];
var __VLS_184;
if (__VLS_ctx.form.type === 'telegram') {
    let __VLS_192;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({
        label: "Bot Token",
        required: true,
    }));
    const __VLS_194 = __VLS_193({
        label: "Bot Token",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_193));
    const { default: __VLS_197 } = __VLS_195.slots;
    let __VLS_198;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({
        modelValue: (__VLS_ctx.form.config.botToken),
        type: "password",
        showPassword: true,
    }));
    const __VLS_200 = __VLS_199({
        modelValue: (__VLS_ctx.form.config.botToken),
        type: "password",
        showPassword: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_199));
    // @ts-ignore
    [form, form,];
    var __VLS_195;
    let __VLS_203;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_204 = __VLS_asFunctionalComponent1(__VLS_203, new __VLS_203({
        label: "默认 Agent",
    }));
    const __VLS_205 = __VLS_204({
        label: "默认 Agent",
    }, ...__VLS_functionalComponentArgsRest(__VLS_204));
    const { default: __VLS_208 } = __VLS_206.slots;
    let __VLS_209;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_210 = __VLS_asFunctionalComponent1(__VLS_209, new __VLS_209({
        modelValue: (__VLS_ctx.form.config.defaultAgent),
        placeholder: "main",
    }));
    const __VLS_211 = __VLS_210({
        modelValue: (__VLS_ctx.form.config.defaultAgent),
        placeholder: "main",
    }, ...__VLS_functionalComponentArgsRest(__VLS_210));
    // @ts-ignore
    [form,];
    var __VLS_206;
    let __VLS_214;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_215 = __VLS_asFunctionalComponent1(__VLS_214, new __VLS_214({
        label: "允许的发送者",
    }));
    const __VLS_216 = __VLS_215({
        label: "允许的发送者",
    }, ...__VLS_functionalComponentArgsRest(__VLS_215));
    const { default: __VLS_219 } = __VLS_217.slots;
    let __VLS_220;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_221 = __VLS_asFunctionalComponent1(__VLS_220, new __VLS_220({
        modelValue: (__VLS_ctx.form.config.allowedFrom),
        placeholder: "逗号分隔的 Telegram 用户 ID",
    }));
    const __VLS_222 = __VLS_221({
        modelValue: (__VLS_ctx.form.config.allowedFrom),
        placeholder: "逗号分隔的 Telegram 用户 ID",
    }, ...__VLS_functionalComponentArgsRest(__VLS_221));
    // @ts-ignore
    [form,];
    var __VLS_217;
}
let __VLS_225;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_226 = __VLS_asFunctionalComponent1(__VLS_225, new __VLS_225({
    label: "启用",
}));
const __VLS_227 = __VLS_226({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_226));
const { default: __VLS_230 } = __VLS_228.slots;
let __VLS_231;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_232 = __VLS_asFunctionalComponent1(__VLS_231, new __VLS_231({
    modelValue: (__VLS_ctx.form.enabled),
}));
const __VLS_233 = __VLS_232({
    modelValue: (__VLS_ctx.form.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_232));
// @ts-ignore
[form,];
var __VLS_228;
// @ts-ignore
[];
var __VLS_140;
{
    const { footer: __VLS_236 } = __VLS_134.slots;
    let __VLS_237;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_238 = __VLS_asFunctionalComponent1(__VLS_237, new __VLS_237({
        ...{ 'onClick': {} },
    }));
    const __VLS_239 = __VLS_238({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_238));
    let __VLS_242;
    const __VLS_243 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.dialogVisible = false;
                // @ts-ignore
                [dialogVisible,];
            } });
    const { default: __VLS_244 } = __VLS_240.slots;
    // @ts-ignore
    [];
    var __VLS_240;
    var __VLS_241;
    let __VLS_245;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_246 = __VLS_asFunctionalComponent1(__VLS_245, new __VLS_245({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.saving),
    }));
    const __VLS_247 = __VLS_246({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.saving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_246));
    let __VLS_250;
    const __VLS_251 = ({ click: {} },
        { onClick: (__VLS_ctx.saveChannel) });
    const { default: __VLS_252 } = __VLS_248.slots;
    // @ts-ignore
    [saving, saveChannel,];
    var __VLS_248;
    var __VLS_249;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_134;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
