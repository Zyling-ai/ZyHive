/// <reference types="../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from './api';
import { useUpdater } from './composables/useUpdater';
const route = useRoute();
const router = useRouter();
const collapsed = ref(false);
const starCount = ref(null);
const appVersion = ref('');
const updateInfo = ref(null);
const isMobile = ref(false);
const mobileDrawerOpen = ref(false);
// 全局升级器：顶栏按钮点击直接弹确认 → 一键升级，进度条显示在 header 下方 banner
const updater = useUpdater();
const headerUpdateBusy = ref(false); // 防重入（防止用户双击）
// UI 辅助：把 stage 翻成人话
const updateStageLabel = computed(() => {
    const s = updater.updateStatus.value?.stage;
    switch (s) {
        case 'downloading': return '下载中';
        case 'verifying': return '验证中';
        case 'applying': return '替换文件';
        case 'done': return '升级完成';
        case 'failed': return '升级失败';
        case 'rolledback': return '已回滚';
        default: return '';
    }
});
// 什么时候显示顶部 upgrade banner：
//   1. 有 updateStatus 且 stage 非 idle → 在进行中或刚结束
//   2. 或者 updateRunning=true（启动但还没来得及返回第一次 status）
const showUpgradeBanner = computed(() => {
    if (updater.updateRunning.value)
        return true;
    const st = updater.updateStatus.value;
    return !!st && st.stage !== 'idle';
});
// 顶栏点「新版本 XXX」按钮
async function handleTopUpdateClick() {
    if (headerUpdateBusy.value)
        return;
    if (!updateInfo.value)
        return;
    // 已经在升级或等重启中 → 不重复触发，跳转到 settings 让用户看详细进度
    if (updater.updateRunning.value || showUpgradeBanner.value) {
        router.push('/settings');
        return;
    }
    headerUpdateBusy.value = true;
    try {
        await ElMessageBox.confirm(`确认将 ZyHive 从 ${appVersion.value} 升级到 ${updateInfo.value.latest}？\n\n升级过程中服务将短暂重启（约 10-30 秒），成员数据和配置文件不受影响。`, '确认升级', { confirmButtonText: '立即升级', cancelButtonText: '取消', type: 'warning' });
    }
    catch {
        headerUpdateBusy.value = false;
        return; // 用户取消
    }
    try {
        await updater.startUpgrade(updateInfo.value.latest);
        ElMessage.success('升级已启动，进度见顶部横幅');
        // 升级期间清掉 localStorage 缓存，避免 restart 后还显示"有新版本"
        // (同时清旧 key 以防本机有遗留)
        localStorage.removeItem('zyhive_update_info_v2');
        localStorage.removeItem('zyhive_update_exp_v2');
        localStorage.removeItem('zyhive_update_info');
        localStorage.removeItem('zyhive_update_exp');
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '启动升级失败');
    }
    finally {
        headerUpdateBusy.value = false;
    }
}
// 升级完成后一键刷新
function reloadAfterUpgrade() {
    updater.reloadPage();
}
// 当升级成功（restartDetected）且用户还没手动刷 → 清本地缓存 + 给个 toast
watch(() => updater.restartDetected.value, (v) => {
    if (v) {
        localStorage.removeItem('zyhive_update_info_v2');
        localStorage.removeItem('zyhive_update_exp_v2');
        localStorage.removeItem('zyhive_update_info');
        localStorage.removeItem('zyhive_update_exp');
        updateInfo.value = null;
        appVersion.value = updater.currentVersion.value;
    }
});
const MOBILE_BREAKPOINT = 768;
function checkMobile() {
    isMobile.value = window.innerWidth <= MOBILE_BREAKPOINT;
    if (!isMobile.value)
        mobileDrawerOpen.value = false;
}
const isLoginPage = computed(() => route.path === '/login');
const isPublicPage = computed(() => !!route.meta.public);
const sidebarWidth = computed(() => {
    if (isMobile.value)
        return '220px';
    return collapsed.value ? '64px' : '200px';
});
const activeMenu = computed(() => {
    const path = route.path;
    if (path.startsWith('/agents/'))
        return '/agents';
    return path;
});
// 聊天页：撑满高度，不需要 padding
const isChatPage = computed(() => route.path === '/');
function onLogoClick() {
    if (isMobile.value) {
        mobileDrawerOpen.value = false;
    }
    else {
        collapsed.value = !collapsed.value;
    }
}
function onToggleSidebar() {
    if (isMobile.value) {
        mobileDrawerOpen.value = !mobileDrawerOpen.value;
    }
    else {
        collapsed.value = !collapsed.value;
    }
}
function toggleMobileDrawer() {
    mobileDrawerOpen.value = !mobileDrawerOpen.value;
}
function onMenuSelect() {
    if (isMobile.value)
        mobileDrawerOpen.value = false;
}
// Close drawer on route change
watch(() => route.path, () => {
    if (isMobile.value)
        mobileDrawerOpen.value = false;
});
function logout() {
    localStorage.removeItem('aipanel_token');
    router.push('/login');
}
// Fetch real-time GitHub star count (cached 10min in localStorage)
onMounted(async () => {
    checkMobile();
    window.addEventListener('resize', checkMobile);
    // Fetch current version
    try {
        const vRes = await api.get('/version');
        appVersion.value = vRes.data.version;
    }
    catch { /* ignore */ }
    // 初始化全局升级器 — 刷新页面时若后端已有进行中任务，顶部 banner 会自动出现
    if (localStorage.getItem('aipanel_token')) {
        updater.initFromBackend().catch(() => { });
    }
    // Check for updates (delayed 2s, cached 1h in localStorage)
    setTimeout(async () => {
        // Skip update check if not logged in — avoids 401 redirect loop on fresh installs
        if (!localStorage.getItem('aipanel_token'))
            return;
        // 26.4.23v7: bump cache key (v2) so the old-parser-era cache is invalidated
        // on first load after this deploy. Old keys ('zyhive_update_info') become
        // orphan and expire naturally.
        const uCacheKey = 'zyhive_update_info_v2';
        const uCacheExp = 'zyhive_update_exp_v2';
        const now = Date.now();
        const cached = localStorage.getItem(uCacheKey);
        const exp = parseInt(localStorage.getItem(uCacheExp) || '0');
        // semver compare: a > b (e.g. v0.9.26 > v0.9.24)
        // 对齐 internal/api/update.go::semverGt —— 支持两种格式:
        //   1. 语义版本 v0.9.26         → [0, 9, 26, 0]
        //   2. 日期版本 26.4.23v6       → [26, 4, 23, 6]  (YY.M.D + vN 修订号)
        // 原版 parse 对 "26.4.23v6" 会把最后一段 Number('23v6') → NaN,
        // 导致 "26.4.23v6 > 26.4.23v5" 判成 false, 顶栏 updateInfo 永远为空.
        const semverGt = (a, b) => {
            const parse = (s) => {
                s = s.replace(/^v/, '');
                // 剥离末尾的 vN 修订号
                let revision = 0;
                const m = s.match(/^(.+?)[vV](\d+)$/);
                if (m && m[1] && m[2]) {
                    s = m[1];
                    revision = parseInt(m[2], 10) || 0;
                }
                const p = s.split('.').map(x => parseInt(x, 10) || 0);
                return [p[0] ?? 0, p[1] ?? 0, p[2] ?? 0, revision];
            };
            const av = parse(a), bv = parse(b);
            for (let i = 0; i < 4; i++) {
                if ((av[i] ?? 0) > (bv[i] ?? 0))
                    return true;
                if ((av[i] ?? 0) < (bv[i] ?? 0))
                    return false;
            }
            return false;
        };
        const current = appVersion.value;
        if (cached && now < exp) {
            const parsed = JSON.parse(cached);
            // 验证缓存的 latest 确实大于当前运行版本，否则丢弃缓存
            if (parsed?.hasUpdate && parsed.latest && current && semverGt(parsed.latest, current)) {
                updateInfo.value = { latest: parsed.latest, releaseUrl: parsed.releaseUrl };
                return;
            }
            // 缓存无效（版本已更新），清掉并重新检查
            localStorage.removeItem(uCacheKey);
            localStorage.removeItem(uCacheExp);
        }
        try {
            const res = await api.get('/update/check');
            const d = res.data;
            localStorage.setItem(uCacheKey, JSON.stringify(d));
            localStorage.setItem(uCacheExp, String(now + 60 * 60 * 1000)); // 1h
            if (d?.hasUpdate && semverGt(d.latest, current))
                updateInfo.value = { latest: d.latest, releaseUrl: d.releaseUrl };
        }
        catch { /* ignore, non-critical */ }
    }, 2000);
    const cacheKey = 'zyhive_gh_stars';
    const cacheExp = 'zyhive_gh_stars_exp';
    const now = Date.now();
    const cached = localStorage.getItem(cacheKey);
    const exp = parseInt(localStorage.getItem(cacheExp) || '0');
    if (cached && now < exp) {
        starCount.value = parseInt(cached);
        return;
    }
    try {
        const res = await fetch('https://api.github.com/repos/Zyling-ai/ZyHive');
        if (res.ok) {
            const data = await res.json();
            starCount.value = data.stargazers_count ?? null;
            localStorage.setItem(cacheKey, String(starCount.value));
            localStorage.setItem(cacheExp, String(now + 10 * 60 * 1000));
        }
    }
    catch { /* ignore */ }
});
onUnmounted(() => {
    window.removeEventListener('resize', checkMobile);
});
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
if (__VLS_ctx.isLoginPage || __VLS_ctx.isPublicPage) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    let __VLS_0;
    /** @ts-ignore @type { | typeof __VLS_components.routerView | typeof __VLS_components.RouterView | typeof __VLS_components['router-view']} */
    routerView;
    // @ts-ignore
    const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({}));
    const __VLS_2 = __VLS_1({}, ...__VLS_functionalComponentArgsRest(__VLS_1));
}
else {
    let __VLS_5;
    /** @ts-ignore @type { | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container'] | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container']} */
    elContainer;
    // @ts-ignore
    const __VLS_6 = __VLS_asFunctionalComponent1(__VLS_5, new __VLS_5({
        ...{ class: "app-layout" },
    }));
    const __VLS_7 = __VLS_6({
        ...{ class: "app-layout" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_6));
    var __VLS_10 = {};
    /** @type {__VLS_StyleScopedClasses['app-layout']} */ ;
    const { default: __VLS_11 } = __VLS_8.slots;
    let __VLS_12;
    /** @ts-ignore @type { | typeof __VLS_components.elHeader | typeof __VLS_components.ElHeader | typeof __VLS_components['el-header'] | typeof __VLS_components.elHeader | typeof __VLS_components.ElHeader | typeof __VLS_components['el-header']} */
    elHeader;
    // @ts-ignore
    const __VLS_13 = __VLS_asFunctionalComponent1(__VLS_12, new __VLS_12({
        ...{ class: "app-header" },
        height: "44px",
    }));
    const __VLS_14 = __VLS_13({
        ...{ class: "app-header" },
        height: "44px",
    }, ...__VLS_functionalComponentArgsRest(__VLS_13));
    /** @type {__VLS_StyleScopedClasses['app-header']} */ ;
    const { default: __VLS_17 } = __VLS_15.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-left" },
    });
    /** @type {__VLS_StyleScopedClasses['header-left']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.toggleMobileDrawer) },
        ...{ class: "hamburger-btn" },
        'aria-label': "菜单",
    });
    /** @type {__VLS_StyleScopedClasses['hamburger-btn']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "hamburger-line" },
        ...{ class: ({ open: __VLS_ctx.mobileDrawerOpen }) },
    });
    /** @type {__VLS_StyleScopedClasses['hamburger-line']} */ ;
    /** @type {__VLS_StyleScopedClasses['open']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "hamburger-line" },
        ...{ class: ({ open: __VLS_ctx.mobileDrawerOpen }) },
    });
    /** @type {__VLS_StyleScopedClasses['hamburger-line']} */ ;
    /** @type {__VLS_StyleScopedClasses['open']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "hamburger-line" },
        ...{ class: ({ open: __VLS_ctx.mobileDrawerOpen }) },
    });
    /** @type {__VLS_StyleScopedClasses['hamburger-line']} */ ;
    /** @type {__VLS_StyleScopedClasses['open']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "header-title" },
    });
    /** @type {__VLS_StyleScopedClasses['header-title']} */ ;
    if (__VLS_ctx.appVersion) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "header-version" },
        });
        /** @type {__VLS_StyleScopedClasses['header-version']} */ ;
        (__VLS_ctx.appVersion);
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "header-right" },
    });
    /** @type {__VLS_StyleScopedClasses['header-right']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
        href: "https://zyling.ai",
        target: "_blank",
        ...{ class: "header-link header-website-btn header-hide-xs" },
        title: "官网",
    });
    /** @type {__VLS_StyleScopedClasses['header-link']} */ ;
    /** @type {__VLS_StyleScopedClasses['header-website-btn']} */ ;
    /** @type {__VLS_StyleScopedClasses['header-hide-xs']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "14",
        height: "14",
        viewBox: "0 0 24 24",
        fill: "currentColor",
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.a, __VLS_intrinsics.a)({
        href: "https://github.com/Zyling-ai/zyhive",
        target: "_blank",
        ...{ class: "header-link header-hide-sm" },
        title: "GitHub",
    });
    /** @type {__VLS_StyleScopedClasses['header-link']} */ ;
    /** @type {__VLS_StyleScopedClasses['header-hide-sm']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "16",
        height: "16",
        viewBox: "0 0 24 24",
        fill: "currentColor",
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "header-star-btn" },
    });
    /** @type {__VLS_StyleScopedClasses['header-star-btn']} */ ;
    if (__VLS_ctx.starCount !== null) {
        (__VLS_ctx.starCount.toLocaleString());
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "header-hide-xs" },
    });
    /** @type {__VLS_StyleScopedClasses['header-hide-xs']} */ ;
    if (__VLS_ctx.updateInfo && !__VLS_ctx.showUpgradeBanner) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ onClick: (__VLS_ctx.handleTopUpdateClick) },
            ...{ class: "header-update-btn" },
            ...{ style: {} },
            title: (`一键升级到 ${__VLS_ctx.updateInfo.latest}`),
        });
        /** @type {__VLS_StyleScopedClasses['header-update-btn']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "update-dot" },
        });
        /** @type {__VLS_StyleScopedClasses['update-dot']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "header-hide-xs" },
        });
        /** @type {__VLS_StyleScopedClasses['header-hide-xs']} */ ;
        (__VLS_ctx.updateInfo.latest);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "header-xs-only" },
            ...{ style: {} },
        });
        /** @type {__VLS_StyleScopedClasses['header-xs-only']} */ ;
    }
    else if (__VLS_ctx.showUpgradeBanner) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.isLoginPage || __VLS_ctx.isPublicPage))
                        return;
                    if (!!(__VLS_ctx.updateInfo && !__VLS_ctx.showUpgradeBanner))
                        return;
                    if (!(__VLS_ctx.showUpgradeBanner))
                        return;
                    __VLS_ctx.router.push('/settings');
                    // @ts-ignore
                    [isLoginPage, isPublicPage, toggleMobileDrawer, mobileDrawerOpen, mobileDrawerOpen, mobileDrawerOpen, appVersion, appVersion, starCount, starCount, updateInfo, updateInfo, updateInfo, showUpgradeBanner, showUpgradeBanner, handleTopUpdateClick, router,];
                } },
            ...{ class: "header-update-btn header-update-running" },
            ...{ style: {} },
            title: (`升级${__VLS_ctx.updateStageLabel}`),
        });
        /** @type {__VLS_StyleScopedClasses['header-update-btn']} */ ;
        /** @type {__VLS_StyleScopedClasses['header-update-running']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "update-dot update-dot-running" },
        });
        /** @type {__VLS_StyleScopedClasses['update-dot']} */ ;
        /** @type {__VLS_StyleScopedClasses['update-dot-running']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "header-hide-xs" },
        });
        /** @type {__VLS_StyleScopedClasses['header-hide-xs']} */ ;
        (__VLS_ctx.updater.restartDetected.value ? '重启完成' : __VLS_ctx.updateStageLabel);
        (__VLS_ctx.updater.updateStatus.value?.progress != null ? __VLS_ctx.updater.updateStatus.value.progress + '%' : '');
    }
    let __VLS_18;
    /** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
    elDivider;
    // @ts-ignore
    const __VLS_19 = __VLS_asFunctionalComponent1(__VLS_18, new __VLS_18({
        direction: "vertical",
        ...{ style: {} },
    }));
    const __VLS_20 = __VLS_19({
        direction: "vertical",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_19));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ onClick: (__VLS_ctx.logout) },
        ...{ class: "header-link" },
        ...{ style: {} },
        title: "退出登录",
    });
    /** @type {__VLS_StyleScopedClasses['header-link']} */ ;
    // @ts-ignore
    [updateStageLabel, updateStageLabel, updater, updater, updater, logout,];
    var __VLS_15;
    let __VLS_23;
    /** @ts-ignore @type { | typeof __VLS_components.transition | typeof __VLS_components.Transition | typeof __VLS_components.transition | typeof __VLS_components.Transition} */
    transition;
    // @ts-ignore
    const __VLS_24 = __VLS_asFunctionalComponent1(__VLS_23, new __VLS_23({
        name: "fade",
    }));
    const __VLS_25 = __VLS_24({
        name: "fade",
    }, ...__VLS_functionalComponentArgsRest(__VLS_24));
    const { default: __VLS_28 } = __VLS_26.slots;
    if (__VLS_ctx.showUpgradeBanner) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "upgrade-banner" },
            ...{ class: ({
                    'is-done': __VLS_ctx.updater.updateStatus.value?.stage === 'done' && __VLS_ctx.updater.restartDetected.value,
                    'is-failed': __VLS_ctx.updater.updateStatus.value?.stage === 'failed',
                    'is-rolledback': __VLS_ctx.updater.updateStatus.value?.stage === 'rolledback',
                }) },
        });
        /** @type {__VLS_StyleScopedClasses['upgrade-banner']} */ ;
        /** @type {__VLS_StyleScopedClasses['is-done']} */ ;
        /** @type {__VLS_StyleScopedClasses['is-failed']} */ ;
        /** @type {__VLS_StyleScopedClasses['is-rolledback']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "upgrade-banner-inner" },
        });
        /** @type {__VLS_StyleScopedClasses['upgrade-banner-inner']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "upgrade-banner-text" },
        });
        /** @type {__VLS_StyleScopedClasses['upgrade-banner-text']} */ ;
        if (__VLS_ctx.updater.updateStatus.value?.stage === 'done' && __VLS_ctx.updater.restartDetected.value) {
            (__VLS_ctx.updater.currentVersion.value);
        }
        else if (__VLS_ctx.updater.updateStatus.value?.stage === 'done') {
        }
        else if (__VLS_ctx.updater.updateStatus.value?.stage === 'failed') {
            (__VLS_ctx.updater.updateStatus.value?.message);
        }
        else if (__VLS_ctx.updater.updateStatus.value?.stage === 'rolledback') {
            (__VLS_ctx.updater.updateStatus.value?.message);
        }
        else {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "banner-spinner" },
            });
            /** @type {__VLS_StyleScopedClasses['banner-spinner']} */ ;
            (__VLS_ctx.updateStageLabel);
            (__VLS_ctx.updater.updateStatus.value?.message || '正在处理…');
        }
        if (!__VLS_ctx.updater.restartDetected.value && __VLS_ctx.updater.updateStatus.value?.stage !== 'failed' && __VLS_ctx.updater.updateStatus.value?.stage !== 'rolledback') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "upgrade-banner-progress" },
            });
            /** @type {__VLS_StyleScopedClasses['upgrade-banner-progress']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "upgrade-banner-progress-bar" },
                ...{ style: ({ width: (__VLS_ctx.updater.updateStatus.value?.progress ?? 0) + '%' }) },
            });
            /** @type {__VLS_StyleScopedClasses['upgrade-banner-progress-bar']} */ ;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "upgrade-banner-actions" },
        });
        /** @type {__VLS_StyleScopedClasses['upgrade-banner-actions']} */ ;
        if (__VLS_ctx.updater.restartDetected.value) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                ...{ onClick: (__VLS_ctx.reloadAfterUpgrade) },
                ...{ class: "banner-btn banner-btn-primary" },
            });
            /** @type {__VLS_StyleScopedClasses['banner-btn']} */ ;
            /** @type {__VLS_StyleScopedClasses['banner-btn-primary']} */ ;
        }
        else if (['failed', 'rolledback'].includes(__VLS_ctx.updater.updateStatus.value?.stage ?? '')) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                ...{ onClick: (...[$event]) => {
                        if (!!(__VLS_ctx.isLoginPage || __VLS_ctx.isPublicPage))
                            return;
                        if (!(__VLS_ctx.showUpgradeBanner))
                            return;
                        if (!!(__VLS_ctx.updater.restartDetected.value))
                            return;
                        if (!(['failed', 'rolledback'].includes(__VLS_ctx.updater.updateStatus.value?.stage ?? '')))
                            return;
                        __VLS_ctx.router.push('/settings');
                        // @ts-ignore
                        [showUpgradeBanner, router, updateStageLabel, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, updater, reloadAfterUpgrade,];
                    } },
                ...{ class: "banner-btn" },
            });
            /** @type {__VLS_StyleScopedClasses['banner-btn']} */ ;
        }
        else {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "upgrade-banner-pct" },
            });
            /** @type {__VLS_StyleScopedClasses['upgrade-banner-pct']} */ ;
            (__VLS_ctx.updater.updateStatus.value?.progress ?? 0);
        }
    }
    // @ts-ignore
    [updater,];
    var __VLS_26;
    let __VLS_29;
    /** @ts-ignore @type { | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container'] | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container']} */
    elContainer;
    // @ts-ignore
    const __VLS_30 = __VLS_asFunctionalComponent1(__VLS_29, new __VLS_29({
        ...{ class: "app-body" },
    }));
    const __VLS_31 = __VLS_30({
        ...{ class: "app-body" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_30));
    /** @type {__VLS_StyleScopedClasses['app-body']} */ ;
    const { default: __VLS_34 } = __VLS_32.slots;
    let __VLS_35;
    /** @ts-ignore @type { | typeof __VLS_components.transition | typeof __VLS_components.Transition | typeof __VLS_components.transition | typeof __VLS_components.Transition} */
    transition;
    // @ts-ignore
    const __VLS_36 = __VLS_asFunctionalComponent1(__VLS_35, new __VLS_35({
        name: "fade",
    }));
    const __VLS_37 = __VLS_36({
        name: "fade",
    }, ...__VLS_functionalComponentArgsRest(__VLS_36));
    const { default: __VLS_40 } = __VLS_38.slots;
    if (__VLS_ctx.mobileDrawerOpen) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.isLoginPage || __VLS_ctx.isPublicPage))
                        return;
                    if (!(__VLS_ctx.mobileDrawerOpen))
                        return;
                    __VLS_ctx.mobileDrawerOpen = false;
                    // @ts-ignore
                    [mobileDrawerOpen, mobileDrawerOpen,];
                } },
            ...{ class: "mobile-overlay" },
        });
        /** @type {__VLS_StyleScopedClasses['mobile-overlay']} */ ;
    }
    // @ts-ignore
    [];
    var __VLS_38;
    let __VLS_41;
    /** @ts-ignore @type { | typeof __VLS_components.elAside | typeof __VLS_components.ElAside | typeof __VLS_components['el-aside'] | typeof __VLS_components.elAside | typeof __VLS_components.ElAside | typeof __VLS_components['el-aside']} */
    elAside;
    // @ts-ignore
    const __VLS_42 = __VLS_asFunctionalComponent1(__VLS_41, new __VLS_41({
        width: (__VLS_ctx.sidebarWidth),
        ...{ class: "app-sidebar" },
        ...{ class: ({ 'mobile-drawer': true, 'mobile-drawer-open': __VLS_ctx.mobileDrawerOpen }) },
    }));
    const __VLS_43 = __VLS_42({
        width: (__VLS_ctx.sidebarWidth),
        ...{ class: "app-sidebar" },
        ...{ class: ({ 'mobile-drawer': true, 'mobile-drawer-open': __VLS_ctx.mobileDrawerOpen }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_42));
    /** @type {__VLS_StyleScopedClasses['app-sidebar']} */ ;
    /** @type {__VLS_StyleScopedClasses['mobile-drawer']} */ ;
    /** @type {__VLS_StyleScopedClasses['mobile-drawer-open']} */ ;
    const { default: __VLS_46 } = __VLS_44.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (__VLS_ctx.onLogoClick) },
        ...{ class: "sidebar-logo" },
    });
    /** @type {__VLS_StyleScopedClasses['sidebar-logo']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "logo-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['logo-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        width: "24",
        height: "24",
        viewBox: "0 0 24 24",
        fill: "none",
        xmlns: "http://www.w3.org/2000/svg",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z",
        fill: "#409EFF",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.text, __VLS_intrinsics.text)({
        x: "12",
        y: "16",
        'text-anchor': "middle",
        fill: "white",
        'font-size': "10",
        'font-weight': "800",
        'font-family': "sans-serif",
    });
    if (!__VLS_ctx.collapsed) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "logo-text" },
        });
        /** @type {__VLS_StyleScopedClasses['logo-text']} */ ;
    }
    let __VLS_47;
    /** @ts-ignore @type { | typeof __VLS_components.elMenu | typeof __VLS_components.ElMenu | typeof __VLS_components['el-menu'] | typeof __VLS_components.elMenu | typeof __VLS_components.ElMenu | typeof __VLS_components['el-menu']} */
    elMenu;
    // @ts-ignore
    const __VLS_48 = __VLS_asFunctionalComponent1(__VLS_47, new __VLS_47({
        ...{ 'onSelect': {} },
        defaultActive: (__VLS_ctx.activeMenu),
        collapse: (__VLS_ctx.collapsed && !__VLS_ctx.isMobile),
        collapseTransition: (false),
        router: true,
        ...{ class: "sidebar-menu" },
    }));
    const __VLS_49 = __VLS_48({
        ...{ 'onSelect': {} },
        defaultActive: (__VLS_ctx.activeMenu),
        collapse: (__VLS_ctx.collapsed && !__VLS_ctx.isMobile),
        collapseTransition: (false),
        router: true,
        ...{ class: "sidebar-menu" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_48));
    let __VLS_52;
    const __VLS_53 = ({ select: {} },
        { onSelect: (__VLS_ctx.onMenuSelect) });
    /** @type {__VLS_StyleScopedClasses['sidebar-menu']} */ ;
    const { default: __VLS_54 } = __VLS_50.slots;
    let __VLS_55;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_56 = __VLS_asFunctionalComponent1(__VLS_55, new __VLS_55({
        index: "/",
    }));
    const __VLS_57 = __VLS_56({
        index: "/",
    }, ...__VLS_functionalComponentArgsRest(__VLS_56));
    const { default: __VLS_60 } = __VLS_58.slots;
    let __VLS_61;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_62 = __VLS_asFunctionalComponent1(__VLS_61, new __VLS_61({}));
    const __VLS_63 = __VLS_62({}, ...__VLS_functionalComponentArgsRest(__VLS_62));
    const { default: __VLS_66 } = __VLS_64.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        'stroke-width': "2",
        'stroke-linecap': "round",
        'stroke-linejoin': "round",
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
        d: "M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z",
    });
    // @ts-ignore
    [mobileDrawerOpen, sidebarWidth, onLogoClick, collapsed, collapsed, activeMenu, isMobile, onMenuSelect,];
    var __VLS_64;
    {
        const { title: __VLS_67 } = __VLS_58.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_58;
    let __VLS_68;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_69 = __VLS_asFunctionalComponent1(__VLS_68, new __VLS_68({
        index: "/dashboard",
    }));
    const __VLS_70 = __VLS_69({
        index: "/dashboard",
    }, ...__VLS_functionalComponentArgsRest(__VLS_69));
    const { default: __VLS_73 } = __VLS_71.slots;
    let __VLS_74;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({}));
    const __VLS_76 = __VLS_75({}, ...__VLS_functionalComponentArgsRest(__VLS_75));
    const { default: __VLS_79 } = __VLS_77.slots;
    let __VLS_80;
    /** @ts-ignore @type { | typeof __VLS_components.HomeFilled} */
    HomeFilled;
    // @ts-ignore
    const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({}));
    const __VLS_82 = __VLS_81({}, ...__VLS_functionalComponentArgsRest(__VLS_81));
    // @ts-ignore
    [];
    var __VLS_77;
    {
        const { title: __VLS_85 } = __VLS_71.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_71;
    let __VLS_86;
    /** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
    elDivider;
    // @ts-ignore
    const __VLS_87 = __VLS_asFunctionalComponent1(__VLS_86, new __VLS_86({
        ...{ style: {} },
    }));
    const __VLS_88 = __VLS_87({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_87));
    let __VLS_91;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_92 = __VLS_asFunctionalComponent1(__VLS_91, new __VLS_91({
        index: "/agents",
    }));
    const __VLS_93 = __VLS_92({
        index: "/agents",
    }, ...__VLS_functionalComponentArgsRest(__VLS_92));
    const { default: __VLS_96 } = __VLS_94.slots;
    let __VLS_97;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_98 = __VLS_asFunctionalComponent1(__VLS_97, new __VLS_97({}));
    const __VLS_99 = __VLS_98({}, ...__VLS_functionalComponentArgsRest(__VLS_98));
    const { default: __VLS_102 } = __VLS_100.slots;
    let __VLS_103;
    /** @ts-ignore @type { | typeof __VLS_components.User} */
    User;
    // @ts-ignore
    const __VLS_104 = __VLS_asFunctionalComponent1(__VLS_103, new __VLS_103({}));
    const __VLS_105 = __VLS_104({}, ...__VLS_functionalComponentArgsRest(__VLS_104));
    // @ts-ignore
    [];
    var __VLS_100;
    {
        const { title: __VLS_108 } = __VLS_94.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_94;
    let __VLS_109;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_110 = __VLS_asFunctionalComponent1(__VLS_109, new __VLS_109({
        index: "/team",
    }));
    const __VLS_111 = __VLS_110({
        index: "/team",
    }, ...__VLS_functionalComponentArgsRest(__VLS_110));
    const { default: __VLS_114 } = __VLS_112.slots;
    let __VLS_115;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({}));
    const __VLS_117 = __VLS_116({}, ...__VLS_functionalComponentArgsRest(__VLS_116));
    const { default: __VLS_120 } = __VLS_118.slots;
    let __VLS_121;
    /** @ts-ignore @type { | typeof __VLS_components.Share} */
    Share;
    // @ts-ignore
    const __VLS_122 = __VLS_asFunctionalComponent1(__VLS_121, new __VLS_121({}));
    const __VLS_123 = __VLS_122({}, ...__VLS_functionalComponentArgsRest(__VLS_122));
    // @ts-ignore
    [];
    var __VLS_118;
    {
        const { title: __VLS_126 } = __VLS_112.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_112;
    let __VLS_127;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_128 = __VLS_asFunctionalComponent1(__VLS_127, new __VLS_127({
        index: "/goals",
    }));
    const __VLS_129 = __VLS_128({
        index: "/goals",
    }, ...__VLS_functionalComponentArgsRest(__VLS_128));
    const { default: __VLS_132 } = __VLS_130.slots;
    let __VLS_133;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_134 = __VLS_asFunctionalComponent1(__VLS_133, new __VLS_133({}));
    const __VLS_135 = __VLS_134({}, ...__VLS_functionalComponentArgsRest(__VLS_134));
    const { default: __VLS_138 } = __VLS_136.slots;
    let __VLS_139;
    /** @ts-ignore @type { | typeof __VLS_components.Flag} */
    Flag;
    // @ts-ignore
    const __VLS_140 = __VLS_asFunctionalComponent1(__VLS_139, new __VLS_139({}));
    const __VLS_141 = __VLS_140({}, ...__VLS_functionalComponentArgsRest(__VLS_140));
    // @ts-ignore
    [];
    var __VLS_136;
    {
        const { title: __VLS_144 } = __VLS_130.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_130;
    let __VLS_145;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_146 = __VLS_asFunctionalComponent1(__VLS_145, new __VLS_145({
        index: "/projects",
    }));
    const __VLS_147 = __VLS_146({
        index: "/projects",
    }, ...__VLS_functionalComponentArgsRest(__VLS_146));
    const { default: __VLS_150 } = __VLS_148.slots;
    let __VLS_151;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_152 = __VLS_asFunctionalComponent1(__VLS_151, new __VLS_151({}));
    const __VLS_153 = __VLS_152({}, ...__VLS_functionalComponentArgsRest(__VLS_152));
    const { default: __VLS_156 } = __VLS_154.slots;
    let __VLS_157;
    /** @ts-ignore @type { | typeof __VLS_components.Folder} */
    Folder;
    // @ts-ignore
    const __VLS_158 = __VLS_asFunctionalComponent1(__VLS_157, new __VLS_157({}));
    const __VLS_159 = __VLS_158({}, ...__VLS_functionalComponentArgsRest(__VLS_158));
    // @ts-ignore
    [];
    var __VLS_154;
    {
        const { title: __VLS_162 } = __VLS_148.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_148;
    let __VLS_163;
    /** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
    elDivider;
    // @ts-ignore
    const __VLS_164 = __VLS_asFunctionalComponent1(__VLS_163, new __VLS_163({
        ...{ style: {} },
    }));
    const __VLS_165 = __VLS_164({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_164));
    let __VLS_168;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_169 = __VLS_asFunctionalComponent1(__VLS_168, new __VLS_168({
        index: "/chats",
    }));
    const __VLS_170 = __VLS_169({
        index: "/chats",
    }, ...__VLS_functionalComponentArgsRest(__VLS_169));
    const { default: __VLS_173 } = __VLS_171.slots;
    let __VLS_174;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_175 = __VLS_asFunctionalComponent1(__VLS_174, new __VLS_174({}));
    const __VLS_176 = __VLS_175({}, ...__VLS_functionalComponentArgsRest(__VLS_175));
    const { default: __VLS_179 } = __VLS_177.slots;
    let __VLS_180;
    /** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
    ChatLineRound;
    // @ts-ignore
    const __VLS_181 = __VLS_asFunctionalComponent1(__VLS_180, new __VLS_180({}));
    const __VLS_182 = __VLS_181({}, ...__VLS_functionalComponentArgsRest(__VLS_181));
    // @ts-ignore
    [];
    var __VLS_177;
    {
        const { title: __VLS_185 } = __VLS_171.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_171;
    let __VLS_186;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_187 = __VLS_asFunctionalComponent1(__VLS_186, new __VLS_186({
        index: "/skills",
    }));
    const __VLS_188 = __VLS_187({
        index: "/skills",
    }, ...__VLS_functionalComponentArgsRest(__VLS_187));
    const { default: __VLS_191 } = __VLS_189.slots;
    let __VLS_192;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_193 = __VLS_asFunctionalComponent1(__VLS_192, new __VLS_192({}));
    const __VLS_194 = __VLS_193({}, ...__VLS_functionalComponentArgsRest(__VLS_193));
    const { default: __VLS_197 } = __VLS_195.slots;
    let __VLS_198;
    /** @ts-ignore @type { | typeof __VLS_components.MagicStick} */
    MagicStick;
    // @ts-ignore
    const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({}));
    const __VLS_200 = __VLS_199({}, ...__VLS_functionalComponentArgsRest(__VLS_199));
    // @ts-ignore
    [];
    var __VLS_195;
    {
        const { title: __VLS_203 } = __VLS_189.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_189;
    let __VLS_204;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_205 = __VLS_asFunctionalComponent1(__VLS_204, new __VLS_204({
        index: "/cron",
    }));
    const __VLS_206 = __VLS_205({
        index: "/cron",
    }, ...__VLS_functionalComponentArgsRest(__VLS_205));
    const { default: __VLS_209 } = __VLS_207.slots;
    let __VLS_210;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_211 = __VLS_asFunctionalComponent1(__VLS_210, new __VLS_210({}));
    const __VLS_212 = __VLS_211({}, ...__VLS_functionalComponentArgsRest(__VLS_211));
    const { default: __VLS_215 } = __VLS_213.slots;
    let __VLS_216;
    /** @ts-ignore @type { | typeof __VLS_components.Timer} */
    Timer;
    // @ts-ignore
    const __VLS_217 = __VLS_asFunctionalComponent1(__VLS_216, new __VLS_216({}));
    const __VLS_218 = __VLS_217({}, ...__VLS_functionalComponentArgsRest(__VLS_217));
    // @ts-ignore
    [];
    var __VLS_213;
    {
        const { title: __VLS_221 } = __VLS_207.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_207;
    let __VLS_222;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_223 = __VLS_asFunctionalComponent1(__VLS_222, new __VLS_222({
        index: "/tasks",
    }));
    const __VLS_224 = __VLS_223({
        index: "/tasks",
    }, ...__VLS_functionalComponentArgsRest(__VLS_223));
    const { default: __VLS_227 } = __VLS_225.slots;
    let __VLS_228;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_229 = __VLS_asFunctionalComponent1(__VLS_228, new __VLS_228({}));
    const __VLS_230 = __VLS_229({}, ...__VLS_functionalComponentArgsRest(__VLS_229));
    const { default: __VLS_233 } = __VLS_231.slots;
    let __VLS_234;
    /** @ts-ignore @type { | typeof __VLS_components.Operation} */
    Operation;
    // @ts-ignore
    const __VLS_235 = __VLS_asFunctionalComponent1(__VLS_234, new __VLS_234({}));
    const __VLS_236 = __VLS_235({}, ...__VLS_functionalComponentArgsRest(__VLS_235));
    // @ts-ignore
    [];
    var __VLS_231;
    {
        const { title: __VLS_239 } = __VLS_225.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_225;
    let __VLS_240;
    /** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
    elDivider;
    // @ts-ignore
    const __VLS_241 = __VLS_asFunctionalComponent1(__VLS_240, new __VLS_240({
        ...{ style: {} },
    }));
    const __VLS_242 = __VLS_241({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_241));
    let __VLS_245;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_246 = __VLS_asFunctionalComponent1(__VLS_245, new __VLS_245({
        index: "/config/models",
    }));
    const __VLS_247 = __VLS_246({
        index: "/config/models",
    }, ...__VLS_functionalComponentArgsRest(__VLS_246));
    const { default: __VLS_250 } = __VLS_248.slots;
    let __VLS_251;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_252 = __VLS_asFunctionalComponent1(__VLS_251, new __VLS_251({}));
    const __VLS_253 = __VLS_252({}, ...__VLS_functionalComponentArgsRest(__VLS_252));
    const { default: __VLS_256 } = __VLS_254.slots;
    let __VLS_257;
    /** @ts-ignore @type { | typeof __VLS_components.Cpu} */
    Cpu;
    // @ts-ignore
    const __VLS_258 = __VLS_asFunctionalComponent1(__VLS_257, new __VLS_257({}));
    const __VLS_259 = __VLS_258({}, ...__VLS_functionalComponentArgsRest(__VLS_258));
    // @ts-ignore
    [];
    var __VLS_254;
    {
        const { title: __VLS_262 } = __VLS_248.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_248;
    let __VLS_263;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_264 = __VLS_asFunctionalComponent1(__VLS_263, new __VLS_263({
        index: "/config/tools",
    }));
    const __VLS_265 = __VLS_264({
        index: "/config/tools",
    }, ...__VLS_functionalComponentArgsRest(__VLS_264));
    const { default: __VLS_268 } = __VLS_266.slots;
    let __VLS_269;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_270 = __VLS_asFunctionalComponent1(__VLS_269, new __VLS_269({}));
    const __VLS_271 = __VLS_270({}, ...__VLS_functionalComponentArgsRest(__VLS_270));
    const { default: __VLS_274 } = __VLS_272.slots;
    let __VLS_275;
    /** @ts-ignore @type { | typeof __VLS_components.SetUp} */
    SetUp;
    // @ts-ignore
    const __VLS_276 = __VLS_asFunctionalComponent1(__VLS_275, new __VLS_275({}));
    const __VLS_277 = __VLS_276({}, ...__VLS_functionalComponentArgsRest(__VLS_276));
    // @ts-ignore
    [];
    var __VLS_272;
    {
        const { title: __VLS_280 } = __VLS_266.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_266;
    let __VLS_281;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_282 = __VLS_asFunctionalComponent1(__VLS_281, new __VLS_281({
        index: "/logs",
    }));
    const __VLS_283 = __VLS_282({
        index: "/logs",
    }, ...__VLS_functionalComponentArgsRest(__VLS_282));
    const { default: __VLS_286 } = __VLS_284.slots;
    let __VLS_287;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_288 = __VLS_asFunctionalComponent1(__VLS_287, new __VLS_287({}));
    const __VLS_289 = __VLS_288({}, ...__VLS_functionalComponentArgsRest(__VLS_288));
    const { default: __VLS_292 } = __VLS_290.slots;
    let __VLS_293;
    /** @ts-ignore @type { | typeof __VLS_components.Document} */
    Document;
    // @ts-ignore
    const __VLS_294 = __VLS_asFunctionalComponent1(__VLS_293, new __VLS_293({}));
    const __VLS_295 = __VLS_294({}, ...__VLS_functionalComponentArgsRest(__VLS_294));
    // @ts-ignore
    [];
    var __VLS_290;
    {
        const { title: __VLS_298 } = __VLS_284.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_284;
    let __VLS_299;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_300 = __VLS_asFunctionalComponent1(__VLS_299, new __VLS_299({
        index: "/usage",
    }));
    const __VLS_301 = __VLS_300({
        index: "/usage",
    }, ...__VLS_functionalComponentArgsRest(__VLS_300));
    const { default: __VLS_304 } = __VLS_302.slots;
    let __VLS_305;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_306 = __VLS_asFunctionalComponent1(__VLS_305, new __VLS_305({}));
    const __VLS_307 = __VLS_306({}, ...__VLS_functionalComponentArgsRest(__VLS_306));
    const { default: __VLS_310 } = __VLS_308.slots;
    let __VLS_311;
    /** @ts-ignore @type { | typeof __VLS_components.TrendCharts} */
    TrendCharts;
    // @ts-ignore
    const __VLS_312 = __VLS_asFunctionalComponent1(__VLS_311, new __VLS_311({}));
    const __VLS_313 = __VLS_312({}, ...__VLS_functionalComponentArgsRest(__VLS_312));
    // @ts-ignore
    [];
    var __VLS_308;
    {
        const { title: __VLS_316 } = __VLS_302.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_302;
    let __VLS_317;
    /** @ts-ignore @type { | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item'] | typeof __VLS_components.elMenuItem | typeof __VLS_components.ElMenuItem | typeof __VLS_components['el-menu-item']} */
    elMenuItem;
    // @ts-ignore
    const __VLS_318 = __VLS_asFunctionalComponent1(__VLS_317, new __VLS_317({
        index: "/settings",
    }));
    const __VLS_319 = __VLS_318({
        index: "/settings",
    }, ...__VLS_functionalComponentArgsRest(__VLS_318));
    const { default: __VLS_322 } = __VLS_320.slots;
    let __VLS_323;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_324 = __VLS_asFunctionalComponent1(__VLS_323, new __VLS_323({}));
    const __VLS_325 = __VLS_324({}, ...__VLS_functionalComponentArgsRest(__VLS_324));
    const { default: __VLS_328 } = __VLS_326.slots;
    let __VLS_329;
    /** @ts-ignore @type { | typeof __VLS_components.Tools} */
    Tools;
    // @ts-ignore
    const __VLS_330 = __VLS_asFunctionalComponent1(__VLS_329, new __VLS_329({}));
    const __VLS_331 = __VLS_330({}, ...__VLS_functionalComponentArgsRest(__VLS_330));
    // @ts-ignore
    [];
    var __VLS_326;
    {
        const { title: __VLS_334 } = __VLS_320.slots;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_320;
    // @ts-ignore
    [];
    var __VLS_50;
    var __VLS_51;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sidebar-footer" },
    });
    /** @type {__VLS_StyleScopedClasses['sidebar-footer']} */ ;
    if (!__VLS_ctx.collapsed || __VLS_ctx.isMobile) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "sidebar-copyright" },
        });
        /** @type {__VLS_StyleScopedClasses['sidebar-copyright']} */ ;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "sidebar-copyright-mini" },
        });
        /** @type {__VLS_StyleScopedClasses['sidebar-copyright-mini']} */ ;
    }
    // @ts-ignore
    [collapsed, isMobile,];
    var __VLS_44;
    let __VLS_335;
    /** @ts-ignore @type { | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container'] | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container']} */
    elContainer;
    // @ts-ignore
    const __VLS_336 = __VLS_asFunctionalComponent1(__VLS_335, new __VLS_335({
        ...{ class: "app-right-container" },
    }));
    const __VLS_337 = __VLS_336({
        ...{ class: "app-right-container" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_336));
    /** @type {__VLS_StyleScopedClasses['app-right-container']} */ ;
    const { default: __VLS_340 } = __VLS_338.slots;
    let __VLS_341;
    /** @ts-ignore @type { | typeof __VLS_components.elMain | typeof __VLS_components.ElMain | typeof __VLS_components['el-main'] | typeof __VLS_components.elMain | typeof __VLS_components.ElMain | typeof __VLS_components['el-main']} */
    elMain;
    // @ts-ignore
    const __VLS_342 = __VLS_asFunctionalComponent1(__VLS_341, new __VLS_341({
        ...{ class: "app-main" },
        ...{ class: ({ 'is-chat-page': __VLS_ctx.isChatPage }) },
    }));
    const __VLS_343 = __VLS_342({
        ...{ class: "app-main" },
        ...{ class: ({ 'is-chat-page': __VLS_ctx.isChatPage }) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_342));
    /** @type {__VLS_StyleScopedClasses['app-main']} */ ;
    /** @type {__VLS_StyleScopedClasses['is-chat-page']} */ ;
    const { default: __VLS_346 } = __VLS_344.slots;
    let __VLS_347;
    /** @ts-ignore @type { | typeof __VLS_components.routerView | typeof __VLS_components.RouterView | typeof __VLS_components['router-view']} */
    routerView;
    // @ts-ignore
    const __VLS_348 = __VLS_asFunctionalComponent1(__VLS_347, new __VLS_347({
        ...{ 'onToggleSidebar': {} },
    }));
    const __VLS_349 = __VLS_348({
        ...{ 'onToggleSidebar': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_348));
    let __VLS_352;
    const __VLS_353 = ({ toggleSidebar: {} },
        { onToggleSidebar: (__VLS_ctx.onToggleSidebar) });
    var __VLS_350;
    var __VLS_351;
    // @ts-ignore
    [isChatPage, onToggleSidebar,];
    var __VLS_344;
    // @ts-ignore
    [];
    var __VLS_338;
    // @ts-ignore
    [];
    var __VLS_32;
    // @ts-ignore
    [];
    var __VLS_8;
}
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
