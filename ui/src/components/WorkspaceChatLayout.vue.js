/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Loading, ChatDotRound } from '@element-plus/icons-vue';
import { files as filesApi, sessions as sessionsApi } from '../api';
import AiChat from './AiChat.vue';
const props = defineProps();
const emit = defineEmits();
// el-tree props mapping
const treeProps = { label: 'name', children: 'children', isLeaf: (d) => !d.isDir };
// ── Panel sizes (px) ──────────────────────────────────────────────────────
const leftW = ref(200);
const midW = ref(460);
const MIN_W = 140;
const MAX_LEFT = 380;
const MAX_MID = 900;
// ── Refs ──────────────────────────────────────────────────────────────────
const treeRef = ref();
const editorRef = ref();
const lineNumRef = ref();
const newFileInput = ref();
const renameInputRef = ref();
const chatRef = ref();
// ── File tree state ────────────────────────────────────────────────────────
const treeData = ref([]);
const treeLoading = ref(false);
const openFilePath = ref('');
const fileContent = ref('');
const fileDirty = ref(false);
const fileBinary = ref(false);
const fileInfo = ref(null);
const showNewFile = ref(false);
const newFilePath = ref('');
// Rename
const renaming = ref('');
const renameValue = ref('');
// Context menu
const ctxMenu = ref({ visible: false, x: 0, y: 0, node: null });
// ── Session state ──────────────────────────────────────────────────────────
const sessionList = ref([]);
const currentSessionId = ref();
// ── Resize ────────────────────────────────────────────────────────────────
let resStartX = 0, resStartW = 0, resSide = null;
const draggingLeft = ref(false);
const draggingRight = ref(false);
function startResizeLeft(e) { startResize(e, 'left'); }
function startResizeRight(e) { startResize(e, 'right'); }
function startResize(e, side) {
    resStartX = e.clientX;
    resStartW = side === 'left' ? leftW.value : midW.value;
    resSide = side;
    side === 'left' ? draggingLeft.value = true : draggingRight.value = true;
    window.addEventListener('mousemove', onResize);
    window.addEventListener('mouseup', stopResize);
    document.body.style.cssText += 'cursor:col-resize;user-select:none;';
}
function onResize(e) {
    const d = e.clientX - resStartX;
    if (resSide === 'left')
        leftW.value = Math.max(MIN_W, Math.min(MAX_LEFT, resStartW + d));
    else if (resSide === 'right')
        midW.value = Math.max(MIN_W, Math.min(MAX_MID, resStartW + d));
}
function stopResize() {
    draggingLeft.value = false;
    draggingRight.value = false;
    resSide = null;
    window.removeEventListener('mousemove', onResize);
    window.removeEventListener('mouseup', stopResize);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
}
// ── File tree ─────────────────────────────────────────────────────────────
async function loadTree() {
    treeLoading.value = true;
    try {
        const res = await filesApi.readTree(props.agentId);
        treeData.value = buildTree(res.data);
    }
    catch {
        treeData.value = [];
    }
    finally {
        treeLoading.value = false;
    }
}
function buildTree(data) {
    const arr = Array.isArray(data) ? data : data?.children ?? [];
    return arr.map((item) => ({
        name: item.name,
        path: item.path ?? item.name,
        isDir: !!(item.isDir ?? item.type === 'dir'),
        size: item.size,
        children: item.children?.length ? buildTree(item.children) : undefined,
    })).sort((a, b) => (+b.isDir - +a.isDir) || a.name.localeCompare(b.name));
}
function onNodeClick(data) {
    ctxMenu.value.visible = false;
    if (!data.isDir)
        openFile(data.path);
}
function onNodeContextmenu(e, data) {
    e.preventDefault();
    ctxMenu.value = { visible: true, x: e.clientX, y: e.clientY, node: data };
}
async function openFile(path) {
    if (fileDirty.value && openFilePath.value) {
        const ok = await ElMessageBox.confirm('有未保存更改，继续切换？', '提示', {
            confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning',
        }).then(() => true).catch(() => false);
        if (!ok)
            return;
    }
    openFilePath.value = path;
    fileDirty.value = false;
    nextTick(() => treeRef.value?.setCurrentKey(path));
    await refreshFile();
}
async function refreshFile() {
    if (!openFilePath.value)
        return;
    try {
        const res = await filesApi.read(props.agentId, openFilePath.value);
        const d = res.data;
        fileBinary.value = d.binary ?? d.encoding === 'base64';
        if (!fileBinary.value) {
            fileContent.value = d.content ?? '';
            fileInfo.value = d.size != null ? { size: d.size, modTime: d.modTime } : null;
        }
        fileDirty.value = false;
    }
    catch {
        fileContent.value = '';
    }
}
async function saveFile() {
    if (!openFilePath.value || fileBinary.value)
        return;
    try {
        await filesApi.write(props.agentId, openFilePath.value, fileContent.value);
        fileDirty.value = false;
        ElMessage.success('已保存');
    }
    catch {
        ElMessage.error('保存失败');
    }
}
async function deleteFile() {
    if (!openFilePath.value)
        return;
    const ok = await ElMessageBox.confirm(`删除 ${openFilePath.value}？`, '确认', {
        confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    }).then(() => true).catch(() => false);
    if (!ok)
        return;
    try {
        await filesApi.delete(props.agentId, openFilePath.value);
        openFilePath.value = '';
        fileContent.value = '';
        fileDirty.value = false;
        await loadTree();
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
async function createFile() {
    const p = newFilePath.value.trim();
    if (!p)
        return;
    try {
        await filesApi.write(props.agentId, p, '');
        showNewFile.value = false;
        newFilePath.value = '';
        await loadTree();
        await openFile(p);
    }
    catch {
        ElMessage.error('创建失败');
    }
}
// ── Delete node (from hover button) ──────────────────────────────────────
async function deleteNode(data) {
    const ok = await ElMessageBox.confirm(`删除 ${data.path}？`, '确认', {
        confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    }).then(() => true).catch(() => false);
    if (!ok)
        return;
    try {
        await filesApi.delete(props.agentId, data.path);
        if (openFilePath.value === data.path) {
            openFilePath.value = '';
            fileContent.value = '';
        }
        await loadTree();
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
// ── Rename ────────────────────────────────────────────────────────────────
function startRename(data) {
    renaming.value = data.path;
    renameValue.value = data.name;
    nextTick(() => renameInputRef.value?.focus());
}
async function commitRename(data) {
    if (!renaming.value || renameValue.value === data.name) {
        renaming.value = '';
        return;
    }
    const dir = data.path.includes('/') ? data.path.substring(0, data.path.lastIndexOf('/') + 1) : '';
    const newPath = dir + renameValue.value;
    try {
        // Read content → write to new path → delete old
        const res = await filesApi.read(props.agentId, data.path);
        await filesApi.write(props.agentId, newPath, res.data?.content ?? '');
        await filesApi.delete(props.agentId, data.path);
        if (openFilePath.value === data.path)
            openFilePath.value = newPath;
        await loadTree();
        ElMessage.success('已重命名');
    }
    catch {
        ElMessage.error('重命名失败');
    }
    renaming.value = '';
}
// ── Context menu actions ──────────────────────────────────────────────────
function ctxNewFile() { ctxMenu.value.visible = false; showNewFile.value = true; }
function ctxNewFolder() { ctxMenu.value.visible = false; showNewFile.value = true; }
function ctxRename() { const n = ctxMenu.value.node; ctxMenu.value.visible = false; if (n)
    startRename(n); }
async function ctxDelete() {
    const node = ctxMenu.value.node;
    ctxMenu.value.visible = false;
    if (!node)
        return;
    const ok = await ElMessageBox.confirm(`删除 ${node.path}？`, '确认', {
        confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    }).then(() => true).catch(() => false);
    if (!ok)
        return;
    try {
        await filesApi.delete(props.agentId, node.path);
        if (openFilePath.value === node.path) {
            openFilePath.value = '';
            fileContent.value = '';
        }
        await loadTree();
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
// Close ctx menu on any click
function onDocClick() { if (ctxMenu.value.visible)
    ctxMenu.value.visible = false; }
// ── Session history ────────────────────────────────────────────────────────
async function loadSessions() {
    try {
        const res = await sessionsApi.list({ agentId: props.agentId, limit: 50 });
        sessionList.value = (res.data?.sessions || []).sort((a, b) => b.lastAt - a.lastAt);
    }
    catch {
        sessionList.value = [];
    }
}
function onSessionSelect(sid) {
    currentSessionId.value = sid || undefined;
    if (sid) {
        chatRef.value?.resumeSession(sid);
    }
    else {
        chatRef.value?.startNewSession();
    }
}
function newSession() {
    currentSessionId.value = undefined;
    chatRef.value?.startNewSession();
}
function onSessionCreated(sid) {
    currentSessionId.value = sid;
    emit('session-change', sid);
    // Reload session list so the new session appears
    loadSessions();
}
// ── Chat → Editor sync ────────────────────────────────────────────────────
async function onChatResponse() {
    await loadTree();
    if (openFilePath.value) {
        await new Promise(r => setTimeout(r, 300));
        await refreshFile();
    }
}
// ── Editor helpers ─────────────────────────────────────────────────────────
const lineCount = computed(() => (fileContent.value.match(/\n/g) ?? []).length + 1);
const chatContext = computed(() => openFilePath.value ? `用户当前打开的文件: ${openFilePath.value}` : undefined);
function insertTab(_e) {
    const ta = editorRef.value;
    const s = ta.selectionStart;
    fileContent.value = ta.value.slice(0, s) + '  ' + ta.value.slice(ta.selectionEnd);
    nextTick(() => { ta.selectionStart = ta.selectionEnd = s + 2; });
}
function syncScroll() {
    if (lineNumRef.value && editorRef.value)
        lineNumRef.value.scrollTop = editorRef.value.scrollTop;
}
// ── Watch showNewFile ──────────────────────────────────────────────────────
watch(showNewFile, async (v) => { if (v) {
    await nextTick();
    newFileInput.value?.focus();
} });
// ── Utils ──────────────────────────────────────────────────────────────────
function fileExt(path) {
    const ext = path.split('.').pop()?.toLowerCase();
    return ext ? `.${ext}` : 'txt';
}
function fmtSize(bytes) {
    if (!bytes)
        return '';
    if (bytes < 1024)
        return `${bytes}B`;
    if (bytes < 1048576)
        return `${(bytes / 1024).toFixed(0)}K`;
    return `${(bytes / 1048576).toFixed(1)}M`;
}
function formatSize(bytes) { return fmtSize(bytes) ?? ''; }
// SVG file icon color classes
function fileColorClass(name) {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    const colors = {
        ts: 'fc-blue', tsx: 'fc-blue', js: 'fc-yellow', jsx: 'fc-yellow',
        vue: 'fc-green', py: 'fc-blue', go: 'fc-teal', rs: 'fc-orange',
        md: 'fc-gray', json: 'fc-yellow', yaml: 'fc-red', yml: 'fc-red',
        toml: 'fc-gray', html: 'fc-orange', css: 'fc-blue', scss: 'fc-pink',
        sh: 'fc-green', bash: 'fc-green', sql: 'fc-orange',
        png: 'fc-purple', jpg: 'fc-purple', jpeg: 'fc-purple', gif: 'fc-purple', svg: 'fc-purple', webp: 'fc-purple',
        env: 'fc-yellow', gitignore: 'fc-gray', dockerfile: 'fc-teal',
    };
    return colors[ext] ?? 'fc-default';
}
function fmtTs(ms) {
    if (!ms)
        return '';
    const d = new Date(ms);
    const now = Date.now();
    const diff = now - ms;
    if (diff < 60000)
        return '刚刚';
    if (diff < 3600000)
        return `${Math.floor(diff / 60000)}分钟前`;
    if (diff < 86400000)
        return `${Math.floor(diff / 3600000)}小时前`;
    return d.toLocaleDateString();
}
// ── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(() => {
    loadTree();
    loadSessions();
    document.addEventListener('click', onDocClick);
});
onUnmounted(() => {
    stopResize();
    document.removeEventListener('click', onDocClick);
});
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
/** @type {__VLS_StyleScopedClasses['wc-layout']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-title']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-save-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tree-node__content']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tree-node__content']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tree-node__expand-icon']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tree-node']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-node']} */ ;
/** @type {__VLS_StyleScopedClasses['icon-folder-open']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-node-name']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-node']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-node-actions']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-act-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-act-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['danger']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-act-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-header']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-left']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-title']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-left']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-left']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-left']} */ ;
/** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
/** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
/** @type {__VLS_StyleScopedClasses['danger']} */ ;
/** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
/** @type {__VLS_StyleScopedClasses['danger']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['session-select']} */ ;
/** @type {__VLS_StyleScopedClasses['session-new-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-modal-input']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-layout" },
});
/** @type {__VLS_StyleScopedClasses['wc-layout']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel wc-panel-left" },
    ...{ style: ({ width: __VLS_ctx.leftW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['wc-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-left']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel-header" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "wc-panel-title" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-header-actions" },
});
/** @type {__VLS_StyleScopedClasses['wc-header-actions']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
elTooltip;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    content: "新建文件",
    placement: "top",
    showAfter: (500),
}));
const __VLS_2 = __VLS_1({
    content: "新建文件",
    placement: "top",
    showAfter: (500),
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.showNewFile = true;
            // @ts-ignore
            [leftW, showNewFile,];
        } },
    ...{ class: "wc-icon-btn" },
});
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
// @ts-ignore
[];
var __VLS_3;
let __VLS_6;
/** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
elTooltip;
// @ts-ignore
const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
    content: "刷新",
    placement: "top",
    showAfter: (500),
}));
const __VLS_8 = __VLS_7({
    content: "刷新",
    placement: "top",
    showAfter: (500),
}, ...__VLS_functionalComponentArgsRest(__VLS_7));
const { default: __VLS_11 } = __VLS_9.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (__VLS_ctx.loadTree) },
    ...{ class: "wc-icon-btn" },
});
/** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
// @ts-ignore
[loadTree,];
var __VLS_9;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel-body file-tree-body" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel-body']} */ ;
/** @type {__VLS_StyleScopedClasses['file-tree-body']} */ ;
if (__VLS_ctx.treeLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-loading" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-loading']} */ ;
    let __VLS_12;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_13 = __VLS_asFunctionalComponent1(__VLS_12, new __VLS_12({
        ...{ class: "rotating" },
    }));
    const __VLS_14 = __VLS_13({
        ...{ class: "rotating" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_13));
    /** @type {__VLS_StyleScopedClasses['rotating']} */ ;
    const { default: __VLS_17 } = __VLS_15.slots;
    let __VLS_18;
    /** @ts-ignore @type { | typeof __VLS_components.Loading} */
    Loading;
    // @ts-ignore
    const __VLS_19 = __VLS_asFunctionalComponent1(__VLS_18, new __VLS_18({}));
    const __VLS_20 = __VLS_19({}, ...__VLS_functionalComponentArgsRest(__VLS_19));
    // @ts-ignore
    [treeLoading,];
    var __VLS_15;
}
else if (!__VLS_ctx.treeData.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-empty']} */ ;
}
else {
    let __VLS_23;
    /** @ts-ignore @type { | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree'] | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree']} */
    elTree;
    // @ts-ignore
    const __VLS_24 = __VLS_asFunctionalComponent1(__VLS_23, new __VLS_23({
        ...{ 'onNodeClick': {} },
        ...{ 'onNodeContextmenu': {} },
        ref: "treeRef",
        data: (__VLS_ctx.treeData),
        props: (__VLS_ctx.treeProps),
        highlightCurrent: (true),
        expandOnClickNode: (true),
        defaultExpandAll: (false),
        nodeKey: "path",
        ...{ class: "wc-file-tree" },
    }));
    const __VLS_25 = __VLS_24({
        ...{ 'onNodeClick': {} },
        ...{ 'onNodeContextmenu': {} },
        ref: "treeRef",
        data: (__VLS_ctx.treeData),
        props: (__VLS_ctx.treeProps),
        highlightCurrent: (true),
        expandOnClickNode: (true),
        defaultExpandAll: (false),
        nodeKey: "path",
        ...{ class: "wc-file-tree" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_24));
    let __VLS_28;
    const __VLS_29 = ({ nodeClick: {} },
        { onNodeClick: (__VLS_ctx.onNodeClick) });
    const __VLS_30 = ({ nodeContextmenu: {} },
        { onNodeContextmenu: (__VLS_ctx.onNodeContextmenu) });
    var __VLS_31 = {};
    /** @type {__VLS_StyleScopedClasses['wc-file-tree']} */ ;
    const { default: __VLS_33 } = __VLS_26.slots;
    {
        const { default: __VLS_34 } = __VLS_26.slots;
        const [{ node, data }] = __VLS_vSlot(__VLS_34);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "tree-node" },
            ...{ class: ({ 'active': data.path === __VLS_ctx.openFilePath }) },
        });
        /** @type {__VLS_StyleScopedClasses['tree-node']} */ ;
        /** @type {__VLS_StyleScopedClasses['active']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "tree-icon-wrap" },
        });
        /** @type {__VLS_StyleScopedClasses['tree-icon-wrap']} */ ;
        if (data.isDir && node.expanded) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
                ...{ class: "icon-folder-open" },
                viewBox: "0 0 16 16",
            });
            /** @type {__VLS_StyleScopedClasses['icon-folder-open']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
                d: "M1.5 3A1.5 1.5 0 0 0 0 4.5v8A1.5 1.5 0 0 0 1.5 14h13a1.5 1.5 0 0 0 1.5-1.5V6a1.5 1.5 0 0 0-1.5-1.5H7.914a.5.5 0 0 1-.354-.146L6.146 2.94A1.5 1.5 0 0 0 5.086 2.5H1.5z",
            });
        }
        else if (data.isDir) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
                ...{ class: "icon-folder" },
                viewBox: "0 0 16 16",
            });
            /** @type {__VLS_StyleScopedClasses['icon-folder']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
                d: "M.54 3.87.5 3a2 2 0 0 1 2-2h3.672a2 2 0 0 1 1.414.586l.828.828A2 2 0 0 0 9.828 3h3.982a2 2 0 0 1 1.992 2.181l-.637 7A2 2 0 0 1 13.174 14H2.826a2 2 0 0 1-1.991-1.819l-.637-7a2 2 0 0 1 .342-1.31z",
            });
        }
        else {
            __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
                ...{ class: "icon-file" },
                ...{ class: (__VLS_ctx.fileColorClass(data.name)) },
                viewBox: "0 0 16 16",
            });
            /** @type {__VLS_StyleScopedClasses['icon-file']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
                'fill-rule': "evenodd",
                d: "M4 0h5.293A1 1 0 0 1 10 .293L13.707 4a1 1 0 0 1 .293.707V14a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V2a2 2 0 0 1 2-2zm5.5 1.5v2a1 1 0 0 0 1 1h2l-3-3z",
            });
        }
        if (__VLS_ctx.renaming === data.path) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.input)({
                ...{ onKeyup: (...[$event]) => {
                        if (!!(__VLS_ctx.treeLoading))
                            return;
                        if (!!(!__VLS_ctx.treeData.length))
                            return;
                        if (!(__VLS_ctx.renaming === data.path))
                            return;
                        __VLS_ctx.commitRename(data);
                        // @ts-ignore
                        [treeData, treeData, treeProps, onNodeClick, onNodeContextmenu, openFilePath, fileColorClass, renaming, commitRename,];
                    } },
                ...{ onKeyup: (...[$event]) => {
                        if (!!(__VLS_ctx.treeLoading))
                            return;
                        if (!!(!__VLS_ctx.treeData.length))
                            return;
                        if (!(__VLS_ctx.renaming === data.path))
                            return;
                        __VLS_ctx.renaming = '';
                        // @ts-ignore
                        [renaming,];
                    } },
                ...{ onBlur: (...[$event]) => {
                        if (!!(__VLS_ctx.treeLoading))
                            return;
                        if (!!(!__VLS_ctx.treeData.length))
                            return;
                        if (!(__VLS_ctx.renaming === data.path))
                            return;
                        __VLS_ctx.commitRename(data);
                        // @ts-ignore
                        [commitRename,];
                    } },
                ...{ onClick: () => { } },
                ...{ class: "tree-rename-input" },
                ref: "renameInputRef",
            });
            (__VLS_ctx.renameValue);
            /** @type {__VLS_StyleScopedClasses['tree-rename-input']} */ ;
        }
        else {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tree-node-name" },
            });
            /** @type {__VLS_StyleScopedClasses['tree-node-name']} */ ;
            (data.name);
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ onClick: () => { } },
            ...{ class: "tree-node-actions" },
        });
        /** @type {__VLS_StyleScopedClasses['tree-node-actions']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
            ...{ onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.treeLoading))
                        return;
                    if (!!(!__VLS_ctx.treeData.length))
                        return;
                    __VLS_ctx.startRename(data);
                    // @ts-ignore
                    [renameValue, startRename,];
                } },
            ...{ class: "tree-act-btn" },
            title: "重命名",
        });
        /** @type {__VLS_StyleScopedClasses['tree-act-btn']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
            viewBox: "0 0 16 16",
            width: "11",
            height: "11",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
            d: "M15.502 1.94a.5.5 0 0 1 0 .706L14.459 3.69l-2-2L13.502.646a.5.5 0 0 1 .707 0l1.293 1.293zm-1.75 2.456-2-2L4.939 9.21a.5.5 0 0 0-.121.196l-.805 2.414a.25.25 0 0 0 .316.316l2.414-.805a.5.5 0 0 0 .196-.12l6.813-6.814z",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
            ...{ onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.treeLoading))
                        return;
                    if (!!(!__VLS_ctx.treeData.length))
                        return;
                    __VLS_ctx.deleteNode(data);
                    // @ts-ignore
                    [deleteNode,];
                } },
            ...{ class: "tree-act-btn danger" },
            title: "删除",
        });
        /** @type {__VLS_StyleScopedClasses['tree-act-btn']} */ ;
        /** @type {__VLS_StyleScopedClasses['danger']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
            viewBox: "0 0 16 16",
            width: "11",
            height: "11",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
            d: "M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5zm2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5zm3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0V6z",
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.path)({
            'fill-rule': "evenodd",
            d: "M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1v1zM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4H4.118zM2.5 3V2h11v1h-11z",
        });
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_26;
    var __VLS_27;
}
let __VLS_35;
/** @ts-ignore @type { | typeof __VLS_components.Teleport | typeof __VLS_components.Teleport} */
Teleport;
// @ts-ignore
const __VLS_36 = __VLS_asFunctionalComponent1(__VLS_35, new __VLS_35({
    to: "body",
}));
const __VLS_37 = __VLS_36({
    to: "body",
}, ...__VLS_functionalComponentArgsRest(__VLS_36));
const { default: __VLS_40 } = __VLS_38.slots;
if (__VLS_ctx.ctxMenu.visible) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onMouseleave: (...[$event]) => {
                if (!(__VLS_ctx.ctxMenu.visible))
                    return;
                __VLS_ctx.ctxMenu.visible = false;
                // @ts-ignore
                [ctxMenu, ctxMenu,];
            } },
        ...{ class: "ctx-menu" },
        ...{ style: ({ left: __VLS_ctx.ctxMenu.x + 'px', top: __VLS_ctx.ctxMenu.y + 'px' }) },
    });
    /** @type {__VLS_StyleScopedClasses['ctx-menu']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (__VLS_ctx.ctxNewFile) },
        ...{ class: "ctx-item" },
    });
    /** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (__VLS_ctx.ctxNewFolder) },
        ...{ class: "ctx-item" },
    });
    /** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
    if (__VLS_ctx.ctxMenu.node && !__VLS_ctx.ctxMenu.node.isDir) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (__VLS_ctx.ctxRename) },
            ...{ class: "ctx-item" },
        });
        /** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
        ...{ class: "ctx-divider" },
    });
    /** @type {__VLS_StyleScopedClasses['ctx-divider']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (__VLS_ctx.ctxDelete) },
        ...{ class: "ctx-item danger" },
    });
    /** @type {__VLS_StyleScopedClasses['ctx-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['danger']} */ ;
}
// @ts-ignore
[ctxMenu, ctxMenu, ctxMenu, ctxMenu, ctxNewFile, ctxNewFolder, ctxRename, ctxDelete,];
var __VLS_38;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (__VLS_ctx.startResizeLeft) },
    ...{ class: "wc-handle" },
    ...{ class: ({ dragging: __VLS_ctx.draggingLeft }) },
});
/** @type {__VLS_StyleScopedClasses['wc-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "wc-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['wc-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel wc-panel-mid" },
    ...{ style: ({ width: __VLS_ctx.midW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['wc-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-mid']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel-header" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel-header']} */ ;
if (__VLS_ctx.openFilePath) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "wc-panel-title file-path-title" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-panel-title']} */ ;
    /** @type {__VLS_StyleScopedClasses['file-path-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "file-ext-badge" },
    });
    /** @type {__VLS_StyleScopedClasses['file-ext-badge']} */ ;
    (__VLS_ctx.fileExt(__VLS_ctx.openFilePath));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "file-path-text" },
        title: (__VLS_ctx.openFilePath),
    });
    /** @type {__VLS_StyleScopedClasses['file-path-text']} */ ;
    (__VLS_ctx.openFilePath);
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "wc-panel-title muted" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-panel-title']} */ ;
    /** @type {__VLS_StyleScopedClasses['muted']} */ ;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-header-actions" },
});
/** @type {__VLS_StyleScopedClasses['wc-header-actions']} */ ;
if (__VLS_ctx.fileDirty) {
    let __VLS_41;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_42 = __VLS_asFunctionalComponent1(__VLS_41, new __VLS_41({
        size: "small",
        type: "warning",
        ...{ style: {} },
    }));
    const __VLS_43 = __VLS_42({
        size: "small",
        type: "warning",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_42));
    const { default: __VLS_46 } = __VLS_44.slots;
    // @ts-ignore
    [openFilePath, openFilePath, openFilePath, openFilePath, startResizeLeft, draggingLeft, midW, fileExt, fileDirty,];
    var __VLS_44;
}
if (__VLS_ctx.openFilePath && __VLS_ctx.fileDirty) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.saveFile) },
        ...{ class: "wc-save-btn" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-save-btn']} */ ;
}
if (__VLS_ctx.openFilePath) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.refreshFile) },
        ...{ class: "wc-icon-btn" },
        title: "刷新",
    });
    /** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
}
if (__VLS_ctx.openFilePath) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.deleteFile) },
        ...{ class: "wc-icon-btn danger" },
        title: "删除文件",
    });
    /** @type {__VLS_StyleScopedClasses['wc-icon-btn']} */ ;
    /** @type {__VLS_StyleScopedClasses['danger']} */ ;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel-body editor-body" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel-body']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-body']} */ ;
if (!__VLS_ctx.openFilePath) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-empty-editor" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-empty-editor']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-empty-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-empty-icon']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-empty-hint" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-empty-hint']} */ ;
}
else if (__VLS_ctx.fileBinary) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-binary-notice" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-binary-notice']} */ ;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-wrap" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-wrap']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "line-numbers" },
        ref: "lineNumRef",
    });
    /** @type {__VLS_StyleScopedClasses['line-numbers']} */ ;
    for (const [n] of __VLS_vFor((__VLS_ctx.lineCount))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (n),
            ...{ class: "line-num" },
        });
        /** @type {__VLS_StyleScopedClasses['line-num']} */ ;
        (n);
        // @ts-ignore
        [openFilePath, openFilePath, openFilePath, openFilePath, fileDirty, saveFile, refreshFile, deleteFile, fileBinary, lineCount,];
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.textarea)({
        ...{ onInput: (...[$event]) => {
                if (!!(!__VLS_ctx.openFilePath))
                    return;
                if (!!(__VLS_ctx.fileBinary))
                    return;
                __VLS_ctx.fileDirty = true;
                __VLS_ctx.syncScroll();
                // @ts-ignore
                [fileDirty, syncScroll,];
            } },
        ...{ onScroll: (__VLS_ctx.syncScroll) },
        ...{ onKeydown: (__VLS_ctx.insertTab) },
        ...{ onKeydown: (__VLS_ctx.saveFile) },
        ...{ onKeydown: (__VLS_ctx.saveFile) },
        ref: "editorRef",
        value: (__VLS_ctx.fileContent),
        ...{ class: "code-editor" },
        spellcheck: "false",
        autocorrect: "off",
        autocapitalize: "off",
    });
    /** @type {__VLS_StyleScopedClasses['code-editor']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-statusbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-statusbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "stat-chip" },
    });
    /** @type {__VLS_StyleScopedClasses['stat-chip']} */ ;
    (__VLS_ctx.fileExt(__VLS_ctx.openFilePath));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.lineCount);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.fileContent.length);
    if (__VLS_ctx.fileInfo) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        (__VLS_ctx.formatSize(__VLS_ctx.fileInfo.size));
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "stat-flex" },
    });
    /** @type {__VLS_StyleScopedClasses['stat-flex']} */ ;
    if (__VLS_ctx.fileDirty) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "status-dirty" },
        });
        /** @type {__VLS_StyleScopedClasses['status-dirty']} */ ;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "status-saved" },
        });
        /** @type {__VLS_StyleScopedClasses['status-saved']} */ ;
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (__VLS_ctx.startResizeRight) },
    ...{ class: "wc-handle" },
    ...{ class: ({ dragging: __VLS_ctx.draggingRight }) },
});
/** @type {__VLS_StyleScopedClasses['wc-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "wc-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['wc-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "wc-panel wc-panel-right" },
});
/** @type {__VLS_StyleScopedClasses['wc-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['wc-panel-right']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "session-bar" },
});
/** @type {__VLS_StyleScopedClasses['session-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "session-bar-left" },
});
/** @type {__VLS_StyleScopedClasses['session-bar-left']} */ ;
let __VLS_47;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_48 = __VLS_asFunctionalComponent1(__VLS_47, new __VLS_47({
    ...{ style: {} },
}));
const __VLS_49 = __VLS_48({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_48));
const { default: __VLS_52 } = __VLS_50.slots;
let __VLS_53;
/** @ts-ignore @type { | typeof __VLS_components.ChatDotRound} */
ChatDotRound;
// @ts-ignore
const __VLS_54 = __VLS_asFunctionalComponent1(__VLS_53, new __VLS_53({}));
const __VLS_55 = __VLS_54({}, ...__VLS_functionalComponentArgsRest(__VLS_54));
// @ts-ignore
[openFilePath, fileExt, fileDirty, saveFile, saveFile, lineCount, syncScroll, insertTab, fileContent, fileContent, fileInfo, fileInfo, formatSize, startResizeRight, draggingRight,];
var __VLS_50;
let __VLS_58;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_59 = __VLS_asFunctionalComponent1(__VLS_58, new __VLS_58({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentSessionId),
    placeholder: "新对话",
    size: "small",
    clearable: true,
    ...{ class: "session-select" },
}));
const __VLS_60 = __VLS_59({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.currentSessionId),
    placeholder: "新对话",
    size: "small",
    clearable: true,
    ...{ class: "session-select" },
}, ...__VLS_functionalComponentArgsRest(__VLS_59));
let __VLS_63;
const __VLS_64 = ({ change: {} },
    { onChange: (__VLS_ctx.onSessionSelect) });
/** @type {__VLS_StyleScopedClasses['session-select']} */ ;
const { default: __VLS_65 } = __VLS_61.slots;
for (const [s] of __VLS_vFor((__VLS_ctx.sessionList))) {
    let __VLS_66;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_67 = __VLS_asFunctionalComponent1(__VLS_66, new __VLS_66({
        key: (s.id),
        value: (s.id),
        label: (s.title || ('对话 ' + s.id.slice(0, 8))),
    }));
    const __VLS_68 = __VLS_67({
        key: (s.id),
        value: (s.id),
        label: (s.title || ('对话 ' + s.id.slice(0, 8))),
    }, ...__VLS_functionalComponentArgsRest(__VLS_67));
    const { default: __VLS_71 } = __VLS_69.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "session-opt" },
    });
    /** @type {__VLS_StyleScopedClasses['session-opt']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-opt-title" },
    });
    /** @type {__VLS_StyleScopedClasses['session-opt-title']} */ ;
    (s.title || '无标题');
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-opt-time" },
    });
    /** @type {__VLS_StyleScopedClasses['session-opt-time']} */ ;
    (__VLS_ctx.fmtTs(s.lastAt));
    // @ts-ignore
    [currentSessionId, onSessionSelect, sessionList, fmtTs,];
    var __VLS_69;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_61;
var __VLS_62;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (__VLS_ctx.newSession) },
    ...{ class: "session-new-btn" },
    title: "新建对话",
});
/** @type {__VLS_StyleScopedClasses['session-new-btn']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-area" },
});
/** @type {__VLS_StyleScopedClasses['chat-area']} */ ;
const __VLS_72 = AiChat;
// @ts-ignore
const __VLS_73 = __VLS_asFunctionalComponent1(__VLS_72, new __VLS_72({
    ...{ 'onResponse': {} },
    ...{ 'onSessionChange': {} },
    agentId: (__VLS_ctx.agentId),
    sessionId: (__VLS_ctx.currentSessionId || undefined),
    context: (__VLS_ctx.chatContext),
    height: "100%",
    ref: "chatRef",
}));
const __VLS_74 = __VLS_73({
    ...{ 'onResponse': {} },
    ...{ 'onSessionChange': {} },
    agentId: (__VLS_ctx.agentId),
    sessionId: (__VLS_ctx.currentSessionId || undefined),
    context: (__VLS_ctx.chatContext),
    height: "100%",
    ref: "chatRef",
}, ...__VLS_functionalComponentArgsRest(__VLS_73));
let __VLS_77;
const __VLS_78 = ({ response: {} },
    { onResponse: (__VLS_ctx.onChatResponse) });
const __VLS_79 = ({ sessionChange: {} },
    { onSessionChange: (__VLS_ctx.onSessionCreated) });
var __VLS_80 = {};
var __VLS_75;
var __VLS_76;
let __VLS_82;
/** @ts-ignore @type { | typeof __VLS_components.Teleport | typeof __VLS_components.Teleport} */
Teleport;
// @ts-ignore
const __VLS_83 = __VLS_asFunctionalComponent1(__VLS_82, new __VLS_82({
    to: "body",
}));
const __VLS_84 = __VLS_83({
    to: "body",
}, ...__VLS_functionalComponentArgsRest(__VLS_83));
const { default: __VLS_87 } = __VLS_85.slots;
if (__VLS_ctx.showNewFile) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.showNewFile))
                    return;
                __VLS_ctx.showNewFile = false;
                // @ts-ignore
                [showNewFile, showNewFile, currentSessionId, newSession, agentId, chatContext, onChatResponse, onSessionCreated,];
            } },
        ...{ class: "wc-modal-mask" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-modal-mask']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-modal" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-modal']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-modal-title" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-modal-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.input)({
        ...{ onKeyup: (__VLS_ctx.createFile) },
        ...{ class: "wc-modal-input" },
        placeholder: "如 notes.md 或 scripts/run.sh",
        ref: "newFileInput",
    });
    (__VLS_ctx.newFilePath);
    /** @type {__VLS_StyleScopedClasses['wc-modal-input']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "wc-modal-footer" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-modal-footer']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.showNewFile))
                    return;
                __VLS_ctx.showNewFile = false;
                // @ts-ignore
                [showNewFile, createFile, newFilePath,];
            } },
        ...{ class: "wc-btn" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-btn']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (__VLS_ctx.createFile) },
        ...{ class: "wc-btn primary" },
    });
    /** @type {__VLS_StyleScopedClasses['wc-btn']} */ ;
    /** @type {__VLS_StyleScopedClasses['primary']} */ ;
}
// @ts-ignore
[createFile,];
var __VLS_85;
// @ts-ignore
var __VLS_32 = __VLS_31, __VLS_81 = __VLS_80;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({
    __typeEmits: {},
    __typeProps: {},
});
export default {};
