/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import axios from 'axios';
const router = useRouter();
const token = ref('');
const loading = ref(false);
const version = ref('');
const showToken = ref(false);
const errorMsg = ref('');
const showTokenHint = ref(false);
// 默认服务器地址 = 当前页面地址
const serverUrl = ref(window.location.origin);
onMounted(async () => {
    // 尝试获取版本号（不需要鉴权）
    try {
        const base = serverUrl.value || window.location.origin;
        const res = await axios.get(`${base}/api/version`, { timeout: 3000 });
        version.value = res.data.version || '';
    }
    catch { }
    // 如果已有 token，直接尝试静默连接
    const saved = localStorage.getItem('aipanel_token');
    const savedUrl = localStorage.getItem('aipanel_url');
    if (saved) {
        token.value = saved;
        if (savedUrl)
            serverUrl.value = savedUrl;
    }
});
async function handleLogin() {
    errorMsg.value = '';
    showTokenHint.value = false;
    if (!token.value.trim()) {
        errorMsg.value = '请输入访问令牌';
        return;
    }
    loading.value = true;
    try {
        const base = (serverUrl.value || window.location.origin).replace(/\/$/, '');
        // 更新 axios 基础 URL
        axios.defaults.baseURL = base;
        // 测试连接
        await axios.get(`${base}/api/health`, {
            headers: { Authorization: `Bearer ${token.value.trim()}` },
            timeout: 5000,
        });
        localStorage.setItem('aipanel_token', token.value.trim());
        localStorage.setItem('aipanel_url', base);
        // 跳转主页
        router.push('/');
    }
    catch (e) {
        const status = e?.response?.status;
        if (status === 401 || status === 403) {
            errorMsg.value = '令牌无效或已过期，请重新获取';
            showTokenHint.value = true;
        }
        else if (!e?.response) {
            errorMsg.value = `无法连接到服务器：${serverUrl.value}`;
            showTokenHint.value = false;
        }
        else {
            errorMsg.value = `连接失败（${status}）`;
            showTokenHint.value = false;
        }
        localStorage.removeItem('aipanel_token');
    }
    finally {
        loading.value = false;
    }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['field-input']} */ ;
/** @type {__VLS_StyleScopedClasses['field-input']} */ ;
/** @type {__VLS_StyleScopedClasses['secret-row']} */ ;
/** @type {__VLS_StyleScopedClasses['field-input']} */ ;
/** @type {__VLS_StyleScopedClasses['icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['error-hint']} */ ;
/** @type {__VLS_StyleScopedClasses['connect-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['connect-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['connect-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['guide-steps']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "login-wrapper" },
});
/** @type {__VLS_StyleScopedClasses['login-wrapper']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "login-card" },
});
/** @type {__VLS_StyleScopedClasses['login-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "login-header" },
});
/** @type {__VLS_StyleScopedClasses['login-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "login-logo" },
});
/** @type {__VLS_StyleScopedClasses['login-logo']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "44",
    height: "44",
    viewBox: "0 0 24 24",
    fill: "none",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.path)({
    d: "M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z",
    fill: "url(#zg)",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.defs, __VLS_intrinsics.defs)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.linearGradient, __VLS_intrinsics.linearGradient)({
    id: "zg",
    x1: "2.5",
    y1: "2",
    x2: "21.5",
    y2: "22",
    gradientUnits: "userSpaceOnUse",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.stop)({
    offset: "0%",
    'stop-color': "#6366f1",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.stop)({
    offset: "100%",
    'stop-color': "#0ea5e9",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
    x: "12",
    y: "16.5",
    'text-anchor': "middle",
    fill: "white",
    'font-size': "9.5",
    'font-weight': "900",
    'font-family': "sans-serif",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h1, __VLS_intrinsics.h1)({
    ...{ class: "login-title" },
});
/** @type {__VLS_StyleScopedClasses['login-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ class: "login-sub" },
});
/** @type {__VLS_StyleScopedClasses['login-sub']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.form, __VLS_intrinsics.form)({
    ...{ onSubmit: (__VLS_ctx.handleLogin) },
    ...{ class: "login-form" },
});
/** @type {__VLS_StyleScopedClasses['login-form']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "field-group" },
});
/** @type {__VLS_StyleScopedClasses['field-group']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
    ...{ class: "field-label" },
});
/** @type {__VLS_StyleScopedClasses['field-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.input)({
    value: (__VLS_ctx.serverUrl),
    ...{ class: "field-input" },
    type: "text",
    placeholder: "http://localhost:8080",
    autocomplete: "off",
});
/** @type {__VLS_StyleScopedClasses['field-input']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "field-group" },
});
/** @type {__VLS_StyleScopedClasses['field-group']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.label, __VLS_intrinsics.label)({
    ...{ class: "field-label" },
});
/** @type {__VLS_StyleScopedClasses['field-label']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "secret-row" },
});
/** @type {__VLS_StyleScopedClasses['secret-row']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.input)({
    ...{ onKeydown: (__VLS_ctx.handleLogin) },
    ...{ class: "field-input" },
    type: (__VLS_ctx.showToken ? 'text' : 'password'),
    placeholder: "粘贴访问令牌",
    autocomplete: "off",
    spellcheck: "false",
});
(__VLS_ctx.token);
/** @type {__VLS_StyleScopedClasses['field-input']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.showToken = !__VLS_ctx.showToken;
            // @ts-ignore
            [handleLogin, handleLogin, serverUrl, showToken, showToken, showToken, token,];
        } },
    type: "button",
    ...{ class: "icon-btn" },
    title: (__VLS_ctx.showToken ? '隐藏' : '显示'),
});
/** @type {__VLS_StyleScopedClasses['icon-btn']} */ ;
if (__VLS_ctx.showToken) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "18",
        height: "18",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
        cx: "12",
        cy: "12",
        r: "3",
    });
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "18",
        height: "18",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
        x1: "1",
        y1: "1",
        x2: "23",
        y2: "23",
    });
}
if (__VLS_ctx.errorMsg) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "error-callout" },
    });
    /** @type {__VLS_StyleScopedClasses['error-callout']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "error-callout__icon" },
    });
    /** @type {__VLS_StyleScopedClasses['error-callout__icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "16",
        height: "16",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.circle)({
        cx: "12",
        cy: "12",
        r: "10",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
        x1: "12",
        y1: "8",
        x2: "12",
        y2: "12",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.line)({
        x1: "12",
        y1: "16",
        x2: "12.01",
        y2: "16",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    (__VLS_ctx.errorMsg);
    if (__VLS_ctx.showTokenHint) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "error-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['error-hint']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    type: "submit",
    ...{ class: "connect-btn" },
    disabled: (__VLS_ctx.loading),
});
/** @type {__VLS_StyleScopedClasses['connect-btn']} */ ;
if (__VLS_ctx.loading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "spin" },
    });
    /** @type {__VLS_StyleScopedClasses['spin']} */ ;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
(__VLS_ctx.loading ? '连接中...' : '连接');
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "guide-section" },
});
/** @type {__VLS_StyleScopedClasses['guide-section']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "guide-title" },
});
/** @type {__VLS_StyleScopedClasses['guide-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.ol, __VLS_intrinsics.ol)({
    ...{ class: "guide-steps" },
});
/** @type {__VLS_StyleScopedClasses['guide-steps']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.li, __VLS_intrinsics.li)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
    ...{ class: "code-block" },
});
/** @type {__VLS_StyleScopedClasses['code-block']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.li, __VLS_intrinsics.li)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
    ...{ class: "code-block" },
});
/** @type {__VLS_StyleScopedClasses['code-block']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.li, __VLS_intrinsics.li)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "guide-tip" },
});
/** @type {__VLS_StyleScopedClasses['guide-tip']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ class: "login-footer" },
});
/** @type {__VLS_StyleScopedClasses['login-footer']} */ ;
if (__VLS_ctx.version) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "ver" },
    });
    /** @type {__VLS_StyleScopedClasses['ver']} */ ;
    (__VLS_ctx.version);
}
// @ts-ignore
[showToken, showToken, errorMsg, errorMsg, showTokenHint, loading, loading, loading, version, version,];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
