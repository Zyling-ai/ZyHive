/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { computed } from 'vue';
import { ArrowRight } from '@element-plus/icons-vue';
const props = defineProps();
const __VLS_emit = defineEmits();
const typeOptions = computed(() => [
    {
        value: '上下级',
        color: '#7c3aed',
        desc: `${props.fromName} 是 ${props.toName} 的上级，箭头指向下级`,
    },
    {
        value: '平级协作',
        color: '#409eff',
        desc: `${props.fromName} 与 ${props.toName} 并列合作，地位平等`,
    },
    {
        value: '支持',
        color: '#67c23a',
        desc: `${props.fromName} 为 ${props.toName} 提供支持和辅助`,
    },
    {
        value: '其他',
        color: '#909399',
        desc: `${props.fromName} 与 ${props.toName} 之间的其他关系`,
    },
]);
const __VLS_ctx = {
    ...{},
    ...{},
    ...{},
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['type-card']} */ ;
/** @type {__VLS_StyleScopedClasses['type-card']} */ ;
/** @type {__VLS_StyleScopedClasses['type-card']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['type-desc']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "rel-pair" },
});
/** @type {__VLS_StyleScopedClasses['rel-pair']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "rel-node" },
});
/** @type {__VLS_StyleScopedClasses['rel-node']} */ ;
(__VLS_ctx.fromName);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
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
/** @ts-ignore @type { | typeof __VLS_components.ArrowRight} */
ArrowRight;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({}));
const __VLS_8 = __VLS_7({}, ...__VLS_functionalComponentArgsRest(__VLS_7));
// @ts-ignore
[fromName,];
var __VLS_3;
if (__VLS_ctx.type === '上下级') {
    let __VLS_11;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_12 = __VLS_asFunctionalComponent1(__VLS_11, new __VLS_11({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        type: "primary",
        ...{ style: {} },
        title: "交换上下级方向",
    }));
    const __VLS_13 = __VLS_12({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        type: "primary",
        ...{ style: {} },
        title: "交换上下级方向",
    }, ...__VLS_functionalComponentArgsRest(__VLS_12));
    let __VLS_16;
    const __VLS_17 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.type === '上下级'))
                    return;
                __VLS_ctx.$emit('swap');
                // @ts-ignore
                [type, $emit,];
            } });
    const { default: __VLS_18 } = __VLS_14.slots;
    // @ts-ignore
    [];
    var __VLS_14;
    var __VLS_15;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "rel-node" },
});
/** @type {__VLS_StyleScopedClasses['rel-node']} */ ;
(__VLS_ctx.toName);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "type-grid" },
});
/** @type {__VLS_StyleScopedClasses['type-grid']} */ ;
for (const [opt] of __VLS_vFor((__VLS_ctx.typeOptions))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.$emit('update:type', opt.value);
                // @ts-ignore
                [$emit, toName, typeOptions,];
            } },
        key: (opt.value),
        ...{ class: "type-card" },
        ...{ class: ({ active: __VLS_ctx.type === opt.value }) },
    });
    /** @type {__VLS_StyleScopedClasses['type-card']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "type-card-top" },
    });
    /** @type {__VLS_StyleScopedClasses['type-card-top']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "type-tag" },
        ...{ style: ({ background: opt.color + '18', color: opt.color }) },
    });
    /** @type {__VLS_StyleScopedClasses['type-tag']} */ ;
    (opt.value);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "type-desc" },
    });
    /** @type {__VLS_StyleScopedClasses['type-desc']} */ ;
    (opt.desc);
    // @ts-ignore
    [type,];
}
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    labelWidth: "72px",
    ...{ style: {} },
}));
const __VLS_21 = __VLS_20({
    labelWidth: "72px",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
const { default: __VLS_24 } = __VLS_22.slots;
let __VLS_25;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({
    label: "关系强度",
}));
const __VLS_27 = __VLS_26({
    label: "关系强度",
}, ...__VLS_functionalComponentArgsRest(__VLS_26));
const { default: __VLS_30 } = __VLS_28.slots;
let __VLS_31;
/** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
elRadioGroup;
// @ts-ignore
const __VLS_32 = __VLS_asFunctionalComponent1(__VLS_31, new __VLS_31({
    ...{ 'onUpdate:modelValue': {} },
    modelValue: (__VLS_ctx.strength),
}));
const __VLS_33 = __VLS_32({
    ...{ 'onUpdate:modelValue': {} },
    modelValue: (__VLS_ctx.strength),
}, ...__VLS_functionalComponentArgsRest(__VLS_32));
let __VLS_36;
const __VLS_37 = ({ 'update:modelValue': {} },
    { 'onUpdate:modelValue': ((v) => __VLS_ctx.$emit('update:strength', v)) });
const { default: __VLS_38 } = __VLS_34.slots;
let __VLS_39;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_40 = __VLS_asFunctionalComponent1(__VLS_39, new __VLS_39({
    value: "核心",
}));
const __VLS_41 = __VLS_40({
    value: "核心",
}, ...__VLS_functionalComponentArgsRest(__VLS_40));
const { default: __VLS_44 } = __VLS_42.slots;
// @ts-ignore
[$emit, strength,];
var __VLS_42;
let __VLS_45;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_46 = __VLS_asFunctionalComponent1(__VLS_45, new __VLS_45({
    value: "常用",
}));
const __VLS_47 = __VLS_46({
    value: "常用",
}, ...__VLS_functionalComponentArgsRest(__VLS_46));
const { default: __VLS_50 } = __VLS_48.slots;
// @ts-ignore
[];
var __VLS_48;
let __VLS_51;
/** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
elRadioButton;
// @ts-ignore
const __VLS_52 = __VLS_asFunctionalComponent1(__VLS_51, new __VLS_51({
    value: "偶尔",
}));
const __VLS_53 = __VLS_52({
    value: "偶尔",
}, ...__VLS_functionalComponentArgsRest(__VLS_52));
const { default: __VLS_56 } = __VLS_54.slots;
// @ts-ignore
[];
var __VLS_54;
// @ts-ignore
[];
var __VLS_34;
var __VLS_35;
// @ts-ignore
[];
var __VLS_28;
let __VLS_57;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_58 = __VLS_asFunctionalComponent1(__VLS_57, new __VLS_57({
    label: "说明",
}));
const __VLS_59 = __VLS_58({
    label: "说明",
}, ...__VLS_functionalComponentArgsRest(__VLS_58));
const { default: __VLS_62 } = __VLS_60.slots;
let __VLS_63;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_64 = __VLS_asFunctionalComponent1(__VLS_63, new __VLS_63({
    ...{ 'onUpdate:modelValue': {} },
    modelValue: (__VLS_ctx.desc),
    placeholder: "可选备注",
}));
const __VLS_65 = __VLS_64({
    ...{ 'onUpdate:modelValue': {} },
    modelValue: (__VLS_ctx.desc),
    placeholder: "可选备注",
}, ...__VLS_functionalComponentArgsRest(__VLS_64));
let __VLS_68;
const __VLS_69 = ({ 'update:modelValue': {} },
    { 'onUpdate:modelValue': ((v) => __VLS_ctx.$emit('update:desc', v)) });
var __VLS_66;
var __VLS_67;
// @ts-ignore
[$emit, desc,];
var __VLS_60;
// @ts-ignore
[];
var __VLS_22;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({
    __typeEmits: {},
    __typeProps: {},
});
export default {};
