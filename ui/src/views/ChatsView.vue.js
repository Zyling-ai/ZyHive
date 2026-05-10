/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, nextTick } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import { Refresh, Search, EditPen, Loading, ChatLineRound, Close } from '@element-plus/icons-vue';
import { sessions as sessionsApi, agents as agentsApi, agentConversations } from '../api';
import AiChat from '../components/AiChat.vue';
const router = useRouter();
function tagFor(source) {
    switch ((source || '').toLowerCase()) {
        case 'feishu': return { label: '飞书', type: 'primary' };
        case 'telegram': return { label: 'TG', type: 'success' };
        case 'web': return { label: 'Web', type: 'warning' };
        default: return { label: '面板', type: 'info' };
    }
}
function normalizeSessionSource(raw, sessionId) {
    const s = (raw || '').toLowerCase();
    if (s === 'feishu' || s === 'telegram' || s === 'web')
        return s;
    if (sessionId.startsWith('feishu-'))
        return 'feishu';
    if (sessionId.startsWith('tg-'))
        return 'telegram';
    if (sessionId.startsWith('web-'))
        return 'web';
    return 'panel';
}
function sessionLabelFromId(id) {
    if (id.startsWith('feishu-')) {
        const rest = id.slice(7);
        return '飞书 · ' + (rest.length > 14 ? rest.slice(0, 12) + '…' : rest);
    }
    if (id.startsWith('tg-'))
        return 'Telegram · ' + id.slice(3, 11);
    if (id.startsWith('web-'))
        return '网页 · ' + id.slice(4, 12);
    return '';
}
// ── 状态 ─────────────────────────────────────────────────────────────────
const agentList = ref([]);
const loading = ref(false);
const allRows = ref([]);
// 筛选
const filterType = ref('');
const filterAgent = ref('');
const filterChannel = ref('');
const searchKw = ref('');
const sortBy = ref('lastAt');
// ── 计算：筛选+排序 ───────────────────────────────────────────────────────
const filteredRows = computed(() => {
    let list = allRows.value;
    if (filterType.value)
        list = list.filter(r => r.kind === filterType.value);
    if (filterAgent.value)
        list = list.filter(r => r.agentId === filterAgent.value);
    if (filterChannel.value)
        list = list.filter(r => r.source === filterChannel.value);
    if (searchKw.value) {
        const kw = searchKw.value.toLowerCase();
        list = list.filter(r => (r.title || '').toLowerCase().includes(kw) ||
            (r.channelId || '').toLowerCase().includes(kw) ||
            r.id.toLowerCase().includes(kw) ||
            r.agentName.toLowerCase().includes(kw));
    }
    const sorted = [...list];
    if (sortBy.value === 'messageCount')
        sorted.sort((a, b) => b.messageCount - a.messageCount);
    else if (sortBy.value === 'tokenEstimate')
        sorted.sort((a, b) => (b.tokenEstimate || 0) - (a.tokenEstimate || 0));
    else
        sorted.sort((a, b) => b.lastAt - a.lastAt);
    return sorted;
});
const agentColorMap = computed(() => {
    const m = {};
    agentList.value.forEach(a => { m[a.id] = a.avatarColor || '#6366f1'; });
    return m;
});
// ── 数据加载 ─────────────────────────────────────────────────────────────
async function loadAll() {
    loading.value = true;
    try {
        const [agRes, chRes, sesRes] = await Promise.all([
            agentsApi.list().catch(() => ({ data: [] })),
            // globalList 的 channelType 只认 telegram/web；feishu 和 panel 走 sessions 分支，前端统一过滤
            agentConversations.globalList({ agentId: filterAgent.value || undefined, channelType: (filterChannel.value === 'telegram' || filterChannel.value === 'web') ? filterChannel.value : undefined })
                .catch(() => ({ data: [] })),
            sessionsApi.list({ agentId: filterAgent.value || undefined, limit: 300 })
                .catch(() => ({ data: { sessions: [], total: 0 } })),
        ]);
        agentList.value = agRes.data || [];
        const agentNameMap = {};
        agentList.value.forEach(a => { agentNameMap[a.id] = a.name; });
        // 渠道对话（channelType 就是 source）
        const chRows = (chRes.data || []).map(r => ({
            kind: 'channel',
            source: (r.channelType || 'web').toLowerCase(),
            id: r.channelId,
            agentId: r.agentId,
            agentName: r.agentName || agentNameMap[r.agentId] || r.agentId,
            messageCount: r.messageCount,
            lastAt: typeof r.lastAt === 'string' ? new Date(r.lastAt).getTime() : r.lastAt,
            firstAt: typeof r.firstAt === 'string' ? new Date(r.firstAt).getTime() : r.firstAt,
            channelType: r.channelType,
            channelId: r.channelId,
            _channel: r,
        }));
        // 面板会话（过滤掉内部系统 session）
        // 注意：source 从 session.source 取（后端已写入），缺失时用 ID 前缀兜底
        const sesRows = (sesRes.data?.sessions || [])
            .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
            .map(s => ({
            kind: 'session',
            source: normalizeSessionSource(s.source, s.id),
            id: s.id,
            agentId: s.agentId,
            agentName: s.agentName || agentNameMap[s.agentId] || s.agentId,
            messageCount: s.messageCount,
            lastAt: typeof s.lastAt === 'string' ? new Date(s.lastAt).getTime() : (s.lastAt || 0),
            createdAt: typeof s.createdAt === 'string' ? new Date(s.createdAt).getTime() : (s.createdAt || 0),
            title: s.title || sessionLabelFromId(s.id),
            tokenEstimate: s.tokenEstimate,
            _session: s,
        }));
        allRows.value = [...chRows, ...sesRows];
    }
    catch (e) {
        ElMessage.error('加载失败');
    }
    finally {
        loading.value = false;
    }
}
// ── 打开详情 ─────────────────────────────────────────────────────────────
function openDetail(row) {
    if (row.kind === 'channel' && row._channel) {
        openChannelDetail(row._channel);
    }
    else if (row.kind === 'session' && row._session) {
        openSessionDetail(row._session);
    }
}
function rowClassName({ row }) {
    if (row.kind === 'session' && row._session && drawerSession.value?.id === row._session.id)
        return 'active-row';
    return '';
}
// ── 渠道对话详情 ─────────────────────────────────────────────────────────
const channelDrawer = ref(false);
const drawerChannelRow = ref(null);
const channelMessages = ref([]);
const channelDetailLoading = ref(false);
const channelTotal = ref(0);
const channelPage = ref(1);
const channelLimit = 50;
// 把 ConvEntry / ParsedMessage 转成 AiChat 接受的 ChatMsg 结构
// 过滤掉空消息（只有 avatar 没正文 + 没工具）
function toChatMsgs(raws) {
    const out = [];
    for (const m of raws) {
        if (m.isCompact) {
            out.push({ role: 'system', text: '— 以上内容已压缩 —' });
            continue;
        }
        const text = (m.text ?? m.content ?? '').trim();
        const tools = (m.toolCalls || []).map((tc) => ({
            id: tc.id, name: tc.name, input: tc.input, result: tc.result,
            status: 'done', _expanded: false,
        }));
        // 真·空消息（没文字 + 没工具 + 没图）→ 跳过, 避免空气泡
        if (!text && tools.length === 0)
            continue;
        const role = (m.role === 'user' || m.role === 'assistant') ? m.role : 'assistant';
        // 渠道消息 (convlog) 有 sender 时前缀标明
        const prefixedText = role === 'user' && m.sender ? `[${m.sender}] ${text}` : text;
        out.push({ role, text: prefixedText, toolCalls: tools.length ? tools : undefined });
    }
    return out;
}
const channelAiChatRef = ref(null);
async function openChannelDetail(row) {
    drawerChannelRow.value = row;
    channelDrawer.value = true;
    channelPage.value = 1;
    await fetchChannelMessages(row, 1);
}
async function fetchChannelMessages(row, page) {
    channelDetailLoading.value = true;
    let loadedMsgs = null;
    try {
        const offset = (page - 1) * channelLimit;
        const res = await agentConversations.messages(row.agentId, row.channelId, { limit: channelLimit, offset });
        const raw = (res.data.messages || []).filter(m => !isSystemSignalMsg(m.content || ''));
        channelMessages.value = raw;
        channelTotal.value = res.data.total;
        loadedMsgs = toChatMsgs(raw);
    }
    catch {
        ElMessage.error('加载消息失败');
    }
    finally {
        channelDetailLoading.value = false;
    }
    if (loadedMsgs) {
        await nextTick();
        await nextTick();
        if (channelAiChatRef.value?.loadHistoryMessages) {
            channelAiChatRef.value.loadHistoryMessages(loadedMsgs);
        }
        else {
            const msgs = loadedMsgs;
            const started = Date.now();
            const timer = setInterval(() => {
                if (channelAiChatRef.value?.loadHistoryMessages) {
                    channelAiChatRef.value.loadHistoryMessages(msgs);
                    clearInterval(timer);
                }
                else if (Date.now() - started > 500) {
                    clearInterval(timer);
                }
            }, 50);
        }
    }
}
async function onChannelPageChange(page) {
    channelPage.value = page;
    if (drawerChannelRow.value)
        await fetchChannelMessages(drawerChannelRow.value, page);
}
// ── 面板会话详情 ─────────────────────────────────────────────────────────
const sessionDrawer = ref(false);
const drawerSession = ref(null);
const detailMessages = ref([]);
const detailLoading = ref(false);
const editingTitle = ref(false);
const editTitle = ref('');
const sessionAiChatRef = ref(null);
// 当前抽屉中 session 的来源（飞书/TG/Web/面板）
const drawerSessionSource = computed(() => {
    if (!drawerSession.value)
        return 'panel';
    return normalizeSessionSource(drawerSession.value.source, drawerSession.value.id);
});
async function openSessionDetail(row) {
    drawerSession.value = row;
    sessionDrawer.value = true;
    editingTitle.value = false;
    detailMessages.value = [];
    detailLoading.value = true;
    let loadedMsgs = null;
    try {
        const res = await sessionsApi.get(row.agentId, row.id);
        const raw = (res.data.messages || []).filter(m => !isSystemSignalMsg(m.text || ''));
        detailMessages.value = raw;
        loadedMsgs = toChatMsgs(raw);
    }
    catch (e) {
        ElMessage.error('加载对话失败');
    }
    finally {
        detailLoading.value = false;
    }
    // drawerLoading 变 false 之后 AiChat 可能才 mount, 必须再等两次 tick 确保 ref 就位.
    // 若 ref 仍为 null 再轮询一次作兜底 (el-drawer 动画期间 mount 时机略慢).
    if (loadedMsgs) {
        await nextTick();
        await nextTick();
        if (sessionAiChatRef.value?.loadHistoryMessages) {
            sessionAiChatRef.value.loadHistoryMessages(loadedMsgs);
        }
        else {
            // 兜底: 轮询 500ms 内 ref 出现
            const msgs = loadedMsgs;
            const started = Date.now();
            const timer = setInterval(() => {
                if (sessionAiChatRef.value?.loadHistoryMessages) {
                    sessionAiChatRef.value.loadHistoryMessages(msgs);
                    clearInterval(timer);
                }
                else if (Date.now() - started > 500) {
                    clearInterval(timer);
                }
            }, 50);
        }
    }
}
function isSystemSignalMsg(text) {
    const t = (text || '').trim();
    return t.startsWith('<task-notification>');
}
function continueSession(row) {
    if (!row)
        return;
    router.push(`/agents/${row.agentId}?resumeSession=${row.id}`);
}
async function deleteSession(row) {
    if (!row._session)
        return;
    try {
        await sessionsApi.delete(row._session.agentId, row._session.id);
        ElMessage.success('已删除');
        if (drawerSession.value?.id === row._session.id)
            sessionDrawer.value = false;
        allRows.value = allRows.value.filter(r => r.id !== row.id || r.kind !== 'session');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
function startEditTitle() {
    editTitle.value = drawerSession.value?.title || '';
    editingTitle.value = true;
}
async function saveTitle() {
    if (!drawerSession.value)
        return;
    try {
        await sessionsApi.rename(drawerSession.value.agentId, drawerSession.value.id, editTitle.value);
        drawerSession.value.title = editTitle.value;
        const row = allRows.value.find(r => r.kind === 'session' && r.id === drawerSession.value.id);
        if (row)
            row.title = editTitle.value;
        editingTitle.value = false;
        ElMessage.success('已重命名');
    }
    catch {
        ElMessage.error('保存失败');
    }
}
// ── 辅助 ─────────────────────────────────────────────────────────────────
function formatDate(ms) {
    if (!ms)
        return '—';
    const d = typeof ms === 'string' ? new Date(ms) : new Date(ms);
    return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}
function formatRelative(ms) {
    if (!ms)
        return '—';
    const t = typeof ms === 'string' ? new Date(ms).getTime() : ms;
    const diff = Date.now() - t;
    if (diff < 60_000)
        return '刚刚';
    if (diff < 3_600_000)
        return `${Math.floor(diff / 60_000)} 分钟前`;
    if (diff < 86_400_000)
        return `${Math.floor(diff / 3_600_000)} 小时前`;
    if (diff < 7 * 86_400_000)
        return `${Math.floor(diff / 86_400_000)} 天前`;
    return formatDate(t);
}
function formatTokens(n) {
    if (!n)
        return '0';
    return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
}
onMounted(() => loadAll());
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['list-card']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chats-page" },
});
/** @type {__VLS_StyleScopedClasses['chats-page']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "page-header" },
});
/** @type {__VLS_StyleScopedClasses['page-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    loading: (__VLS_ctx.loading),
    icon: (__VLS_ctx.Refresh),
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (__VLS_ctx.loadAll) });
const { default: __VLS_7 } = __VLS_3.slots;
// @ts-ignore
[loading, Refresh, loadAll,];
var __VLS_3;
var __VLS_4;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "filter-bar" },
});
/** @type {__VLS_StyleScopedClasses['filter-bar']} */ ;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({
    modelValue: (__VLS_ctx.filterType),
    placeholder: "全部类型",
    clearable: true,
    size: "small",
    ...{ style: {} },
}));
const __VLS_10 = __VLS_9({
    modelValue: (__VLS_ctx.filterType),
    placeholder: "全部类型",
    clearable: true,
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({
    label: "渠道对话",
    value: "channel",
}));
const __VLS_16 = __VLS_15({
    label: "渠道对话",
    value: "channel",
}, ...__VLS_functionalComponentArgsRest(__VLS_15));
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    label: "面板会话",
    value: "session",
}));
const __VLS_21 = __VLS_20({
    label: "面板会话",
    value: "session",
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
// @ts-ignore
[filterType,];
var __VLS_11;
let __VLS_24;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_25 = __VLS_asFunctionalComponent1(__VLS_24, new __VLS_24({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgent),
    placeholder: "全部成员",
    clearable: true,
    size: "small",
    ...{ style: {} },
}));
const __VLS_26 = __VLS_25({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.filterAgent),
    placeholder: "全部成员",
    clearable: true,
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_25));
let __VLS_29;
const __VLS_30 = ({ change: {} },
    { onChange: (__VLS_ctx.loadAll) });
const { default: __VLS_31 } = __VLS_27.slots;
for (const [ag] of __VLS_vFor((__VLS_ctx.agentList))) {
    let __VLS_32;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_33 = __VLS_asFunctionalComponent1(__VLS_32, new __VLS_32({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }));
    const __VLS_34 = __VLS_33({
        key: (ag.id),
        label: (ag.name),
        value: (ag.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_33));
    // @ts-ignore
    [loadAll, filterAgent, agentList,];
}
// @ts-ignore
[];
var __VLS_27;
var __VLS_28;
let __VLS_37;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_38 = __VLS_asFunctionalComponent1(__VLS_37, new __VLS_37({
    modelValue: (__VLS_ctx.filterChannel),
    placeholder: "全部渠道",
    clearable: true,
    size: "small",
    ...{ style: {} },
}));
const __VLS_39 = __VLS_38({
    modelValue: (__VLS_ctx.filterChannel),
    placeholder: "全部渠道",
    clearable: true,
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_38));
const { default: __VLS_42 } = __VLS_40.slots;
let __VLS_43;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_44 = __VLS_asFunctionalComponent1(__VLS_43, new __VLS_43({
    label: "飞书",
    value: "feishu",
}));
const __VLS_45 = __VLS_44({
    label: "飞书",
    value: "feishu",
}, ...__VLS_functionalComponentArgsRest(__VLS_44));
let __VLS_48;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_49 = __VLS_asFunctionalComponent1(__VLS_48, new __VLS_48({
    label: "Telegram",
    value: "telegram",
}));
const __VLS_50 = __VLS_49({
    label: "Telegram",
    value: "telegram",
}, ...__VLS_functionalComponentArgsRest(__VLS_49));
let __VLS_53;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_54 = __VLS_asFunctionalComponent1(__VLS_53, new __VLS_53({
    label: "Web 聊天",
    value: "web",
}));
const __VLS_55 = __VLS_54({
    label: "Web 聊天",
    value: "web",
}, ...__VLS_functionalComponentArgsRest(__VLS_54));
let __VLS_58;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_59 = __VLS_asFunctionalComponent1(__VLS_58, new __VLS_58({
    label: "面板（本地）",
    value: "panel",
}));
const __VLS_60 = __VLS_59({
    label: "面板（本地）",
    value: "panel",
}, ...__VLS_functionalComponentArgsRest(__VLS_59));
// @ts-ignore
[filterChannel,];
var __VLS_40;
let __VLS_63;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_64 = __VLS_asFunctionalComponent1(__VLS_63, new __VLS_63({
    modelValue: (__VLS_ctx.searchKw),
    placeholder: "搜索标题 / ID / 成员…",
    clearable: true,
    size: "small",
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}));
const __VLS_65 = __VLS_64({
    modelValue: (__VLS_ctx.searchKw),
    placeholder: "搜索标题 / ID / 成员…",
    clearable: true,
    size: "small",
    ...{ style: {} },
    prefixIcon: (__VLS_ctx.Search),
}, ...__VLS_functionalComponentArgsRest(__VLS_64));
let __VLS_68;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_69 = __VLS_asFunctionalComponent1(__VLS_68, new __VLS_68({
    modelValue: (__VLS_ctx.sortBy),
    size: "small",
    ...{ style: {} },
}));
const __VLS_70 = __VLS_69({
    modelValue: (__VLS_ctx.sortBy),
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_69));
const { default: __VLS_73 } = __VLS_71.slots;
let __VLS_74;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({
    label: "最近活跃",
    value: "lastAt",
}));
const __VLS_76 = __VLS_75({
    label: "最近活跃",
    value: "lastAt",
}, ...__VLS_functionalComponentArgsRest(__VLS_75));
let __VLS_79;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_80 = __VLS_asFunctionalComponent1(__VLS_79, new __VLS_79({
    label: "消息最多",
    value: "messageCount",
}));
const __VLS_81 = __VLS_80({
    label: "消息最多",
    value: "messageCount",
}, ...__VLS_functionalComponentArgsRest(__VLS_80));
let __VLS_84;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_85 = __VLS_asFunctionalComponent1(__VLS_84, new __VLS_84({
    label: "Token 最多",
    value: "tokenEstimate",
}));
const __VLS_86 = __VLS_85({
    label: "Token 最多",
    value: "tokenEstimate",
}, ...__VLS_functionalComponentArgsRest(__VLS_85));
// @ts-ignore
[searchKw, Search, sortBy,];
var __VLS_71;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "filter-count" },
});
/** @type {__VLS_StyleScopedClasses['filter-count']} */ ;
(__VLS_ctx.filteredRows.length);
let __VLS_89;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_90 = __VLS_asFunctionalComponent1(__VLS_89, new __VLS_89({
    shadow: "never",
    ...{ class: "list-card" },
}));
const __VLS_91 = __VLS_90({
    shadow: "never",
    ...{ class: "list-card" },
}, ...__VLS_functionalComponentArgsRest(__VLS_90));
/** @type {__VLS_StyleScopedClasses['list-card']} */ ;
const { default: __VLS_94 } = __VLS_92.slots;
let __VLS_95;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_96 = __VLS_asFunctionalComponent1(__VLS_95, new __VLS_95({
    ...{ 'onRowClick': {} },
    data: (__VLS_ctx.filteredRows),
    stripe: true,
    ...{ style: {} },
    rowClassName: (__VLS_ctx.rowClassName),
    size: "default",
}));
const __VLS_97 = __VLS_96({
    ...{ 'onRowClick': {} },
    data: (__VLS_ctx.filteredRows),
    stripe: true,
    ...{ style: {} },
    rowClassName: (__VLS_ctx.rowClassName),
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_96));
let __VLS_100;
const __VLS_101 = ({ rowClick: {} },
    { onRowClick: (__VLS_ctx.openDetail) });
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.loading) }, null, null);
const { default: __VLS_102 } = __VLS_98.slots;
let __VLS_103;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_104 = __VLS_asFunctionalComponent1(__VLS_103, new __VLS_103({
    label: "渠道",
    width: "90",
    align: "center",
}));
const __VLS_105 = __VLS_104({
    label: "渠道",
    width: "90",
    align: "center",
}, ...__VLS_functionalComponentArgsRest(__VLS_104));
const { default: __VLS_108 } = __VLS_106.slots;
{
    const { default: __VLS_109 } = __VLS_106.slots;
    const [{ row }] = __VLS_vSlot(__VLS_109);
    let __VLS_110;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_111 = __VLS_asFunctionalComponent1(__VLS_110, new __VLS_110({
        type: (__VLS_ctx.tagFor(row.source).type),
        ...{ class: (['src-tag', 'src-' + row.source]) },
        size: "small",
        effect: "plain",
    }));
    const __VLS_112 = __VLS_111({
        type: (__VLS_ctx.tagFor(row.source).type),
        ...{ class: (['src-tag', 'src-' + row.source]) },
        size: "small",
        effect: "plain",
    }, ...__VLS_functionalComponentArgsRest(__VLS_111));
    /** @type {__VLS_StyleScopedClasses['src-tag']} */ ;
    const { default: __VLS_115 } = __VLS_113.slots;
    (__VLS_ctx.tagFor(row.source).label);
    // @ts-ignore
    [loading, filteredRows, filteredRows, rowClassName, openDetail, vLoading, tagFor, tagFor,];
    var __VLS_113;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_106;
let __VLS_116;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_117 = __VLS_asFunctionalComponent1(__VLS_116, new __VLS_116({
    label: "标题 / 渠道",
    minWidth: "240",
}));
const __VLS_118 = __VLS_117({
    label: "标题 / 渠道",
    minWidth: "240",
}, ...__VLS_functionalComponentArgsRest(__VLS_117));
const { default: __VLS_121 } = __VLS_119.slots;
{
    const { default: __VLS_122 } = __VLS_119.slots;
    const [{ row }] = __VLS_vSlot(__VLS_122);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "row-title-cell" },
    });
    /** @type {__VLS_StyleScopedClasses['row-title-cell']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "row-title" },
    });
    /** @type {__VLS_StyleScopedClasses['row-title']} */ ;
    (row.title || row.channelId || '（无标题）');
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "row-id" },
    });
    /** @type {__VLS_StyleScopedClasses['row-id']} */ ;
    (row.id);
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_119;
let __VLS_123;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({
    label: "成员",
    width: "110",
}));
const __VLS_125 = __VLS_124({
    label: "成员",
    width: "110",
}, ...__VLS_functionalComponentArgsRest(__VLS_124));
const { default: __VLS_128 } = __VLS_126.slots;
{
    const { default: __VLS_129 } = __VLS_126.slots;
    const [{ row }] = __VLS_vSlot(__VLS_129);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "agent-cell" },
    });
    /** @type {__VLS_StyleScopedClasses['agent-cell']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "agent-dot" },
        ...{ style: ({ background: __VLS_ctx.agentColorMap[row.agentId] || '#6366f1' }) },
    });
    /** @type {__VLS_StyleScopedClasses['agent-dot']} */ ;
    (row.agentName);
    // @ts-ignore
    [agentColorMap,];
}
// @ts-ignore
[];
var __VLS_126;
let __VLS_130;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_131 = __VLS_asFunctionalComponent1(__VLS_130, new __VLS_130({
    label: "消息",
    width: "70",
    align: "center",
}));
const __VLS_132 = __VLS_131({
    label: "消息",
    width: "70",
    align: "center",
}, ...__VLS_functionalComponentArgsRest(__VLS_131));
const { default: __VLS_135 } = __VLS_133.slots;
{
    const { default: __VLS_136 } = __VLS_133.slots;
    const [{ row }] = __VLS_vSlot(__VLS_136);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (row.messageCount);
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_133;
let __VLS_137;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_138 = __VLS_asFunctionalComponent1(__VLS_137, new __VLS_137({
    label: "Token",
    width: "100",
    align: "center",
}));
const __VLS_139 = __VLS_138({
    label: "Token",
    width: "100",
    align: "center",
}, ...__VLS_functionalComponentArgsRest(__VLS_138));
const { default: __VLS_142 } = __VLS_140.slots;
{
    const { default: __VLS_143 } = __VLS_140.slots;
    const [{ row }] = __VLS_vSlot(__VLS_143);
    if (row.kind === 'session' && row.tokenEstimate) {
        let __VLS_144;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_145 = __VLS_asFunctionalComponent1(__VLS_144, new __VLS_144({
            type: (row.tokenEstimate > 60000 ? 'danger' : row.tokenEstimate > 30000 ? 'warning' : 'info'),
            size: "small",
            effect: "plain",
        }));
        const __VLS_146 = __VLS_145({
            type: (row.tokenEstimate > 60000 ? 'danger' : row.tokenEstimate > 30000 ? 'warning' : 'info'),
            size: "small",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_145));
        const { default: __VLS_149 } = __VLS_147.slots;
        (__VLS_ctx.formatTokens(row.tokenEstimate));
        // @ts-ignore
        [formatTokens,];
        var __VLS_147;
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
var __VLS_140;
let __VLS_150;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_151 = __VLS_asFunctionalComponent1(__VLS_150, new __VLS_150({
    label: "最后活跃",
    width: "130",
}));
const __VLS_152 = __VLS_151({
    label: "最后活跃",
    width: "130",
}, ...__VLS_functionalComponentArgsRest(__VLS_151));
const { default: __VLS_155 } = __VLS_153.slots;
{
    const { default: __VLS_156 } = __VLS_153.slots;
    const [{ row }] = __VLS_vSlot(__VLS_156);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (__VLS_ctx.formatRelative(row.lastAt));
    // @ts-ignore
    [formatRelative,];
}
// @ts-ignore
[];
var __VLS_153;
let __VLS_157;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_158 = __VLS_asFunctionalComponent1(__VLS_157, new __VLS_157({
    label: "创建时间",
    width: "120",
}));
const __VLS_159 = __VLS_158({
    label: "创建时间",
    width: "120",
}, ...__VLS_functionalComponentArgsRest(__VLS_158));
const { default: __VLS_162 } = __VLS_160.slots;
{
    const { default: __VLS_163 } = __VLS_160.slots;
    const [{ row }] = __VLS_vSlot(__VLS_163);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (__VLS_ctx.formatDate(row.firstAt || row.createdAt));
    // @ts-ignore
    [formatDate,];
}
// @ts-ignore
[];
var __VLS_160;
let __VLS_164;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_165 = __VLS_asFunctionalComponent1(__VLS_164, new __VLS_164({
    ...{ 'onClick': {} },
    label: "操作",
    width: "160",
}));
const __VLS_166 = __VLS_165({
    ...{ 'onClick': {} },
    label: "操作",
    width: "160",
}, ...__VLS_functionalComponentArgsRest(__VLS_165));
let __VLS_169;
const __VLS_170 = ({ click: {} },
    { onClick: () => { } });
const { default: __VLS_171 } = __VLS_167.slots;
{
    const { default: __VLS_172 } = __VLS_167.slots;
    const [{ row }] = __VLS_vSlot(__VLS_172);
    let __VLS_173;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_174 = __VLS_asFunctionalComponent1(__VLS_173, new __VLS_173({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }));
    const __VLS_175 = __VLS_174({
        ...{ 'onClick': {} },
        size: "small",
        link: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_174));
    let __VLS_178;
    const __VLS_179 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.openDetail(row);
                // @ts-ignore
                [openDetail,];
            } });
    const { default: __VLS_180 } = __VLS_176.slots;
    // @ts-ignore
    [];
    var __VLS_176;
    var __VLS_177;
    if (row.kind === 'session') {
        let __VLS_181;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_182 = __VLS_asFunctionalComponent1(__VLS_181, new __VLS_181({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }));
        const __VLS_183 = __VLS_182({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            type: "primary",
        }, ...__VLS_functionalComponentArgsRest(__VLS_182));
        let __VLS_186;
        const __VLS_187 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(row.kind === 'session'))
                        return;
                    __VLS_ctx.continueSession(row);
                    // @ts-ignore
                    [continueSession,];
                } });
        const { default: __VLS_188 } = __VLS_184.slots;
        // @ts-ignore
        [];
        var __VLS_184;
        var __VLS_185;
        let __VLS_189;
        /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
        elPopconfirm;
        // @ts-ignore
        const __VLS_190 = __VLS_asFunctionalComponent1(__VLS_189, new __VLS_189({
            ...{ 'onConfirm': {} },
            title: "确认删除此对话？",
            width: "180",
        }));
        const __VLS_191 = __VLS_190({
            ...{ 'onConfirm': {} },
            title: "确认删除此对话？",
            width: "180",
        }, ...__VLS_functionalComponentArgsRest(__VLS_190));
        let __VLS_194;
        const __VLS_195 = ({ confirm: {} },
            { onConfirm: (...[$event]) => {
                    if (!(row.kind === 'session'))
                        return;
                    __VLS_ctx.deleteSession(row);
                    // @ts-ignore
                    [deleteSession,];
                } });
        const { default: __VLS_196 } = __VLS_192.slots;
        {
            const { reference: __VLS_197 } = __VLS_192.slots;
            let __VLS_198;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_199 = __VLS_asFunctionalComponent1(__VLS_198, new __VLS_198({
                ...{ 'onClick': {} },
                size: "small",
                link: true,
                type: "danger",
            }));
            const __VLS_200 = __VLS_199({
                ...{ 'onClick': {} },
                size: "small",
                link: true,
                type: "danger",
            }, ...__VLS_functionalComponentArgsRest(__VLS_199));
            let __VLS_203;
            const __VLS_204 = ({ click: {} },
                { onClick: () => { } });
            const { default: __VLS_205 } = __VLS_201.slots;
            // @ts-ignore
            [];
            var __VLS_201;
            var __VLS_202;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_192;
        var __VLS_193;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_167;
var __VLS_168;
{
    const { empty: __VLS_206 } = __VLS_98.slots;
    let __VLS_207;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_208 = __VLS_asFunctionalComponent1(__VLS_207, new __VLS_207({
        description: "暂无对话记录",
    }));
    const __VLS_209 = __VLS_208({
        description: "暂无对话记录",
    }, ...__VLS_functionalComponentArgsRest(__VLS_208));
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_98;
var __VLS_99;
// @ts-ignore
[];
var __VLS_92;
let __VLS_212;
/** @ts-ignore @type { | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer'] | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer']} */
elDrawer;
// @ts-ignore
const __VLS_213 = __VLS_asFunctionalComponent1(__VLS_212, new __VLS_212({
    modelValue: (__VLS_ctx.channelDrawer),
    size: "55%",
    direction: "rtl",
    withHeader: (false),
}));
const __VLS_214 = __VLS_213({
    modelValue: (__VLS_ctx.channelDrawer),
    size: "55%",
    direction: "rtl",
    withHeader: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_213));
const { default: __VLS_217 } = __VLS_215.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-wrap" },
});
/** @type {__VLS_StyleScopedClasses['drawer-wrap']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd-main" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-main']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd-title" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-title']} */ ;
let __VLS_218;
/** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
elTag;
// @ts-ignore
const __VLS_219 = __VLS_asFunctionalComponent1(__VLS_218, new __VLS_218({
    type: (__VLS_ctx.tagFor((__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()).type),
    size: "small",
    effect: "plain",
    ...{ class: (['src-tag', 'src-' + (__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()]) },
}));
const __VLS_220 = __VLS_219({
    type: (__VLS_ctx.tagFor((__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()).type),
    size: "small",
    effect: "plain",
    ...{ class: (['src-tag', 'src-' + (__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()]) },
}, ...__VLS_functionalComponentArgsRest(__VLS_219));
/** @type {__VLS_StyleScopedClasses['src-tag']} */ ;
const { default: __VLS_223 } = __VLS_221.slots;
(__VLS_ctx.tagFor((__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()).label);
// @ts-ignore
[tagFor, tagFor, channelDrawer, drawerChannelRow, drawerChannelRow, drawerChannelRow,];
var __VLS_221;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "drawer-hd-id" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-id']} */ ;
(__VLS_ctx.drawerChannelRow?.channelId);
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd-sub" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-sub']} */ ;
(__VLS_ctx.drawerChannelRow?.agentName);
(__VLS_ctx.drawerChannelRow?.messageCount);
let __VLS_224;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_225 = __VLS_asFunctionalComponent1(__VLS_224, new __VLS_224({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.Close),
    circle: true,
    size: "small",
}));
const __VLS_226 = __VLS_225({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.Close),
    circle: true,
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_225));
let __VLS_229;
const __VLS_230 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.channelDrawer = false;
            // @ts-ignore
            [channelDrawer, drawerChannelRow, drawerChannelRow, drawerChannelRow, Close,];
        } });
var __VLS_227;
var __VLS_228;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-chat-body" },
    ...{ style: {} },
});
/** @type {__VLS_StyleScopedClasses['drawer-chat-body']} */ ;
if (__VLS_ctx.channelDrawer && __VLS_ctx.drawerChannelRow) {
    const __VLS_231 = AiChat;
    // @ts-ignore
    const __VLS_232 = __VLS_asFunctionalComponent1(__VLS_231, new __VLS_231({
        key: ('ch-' + __VLS_ctx.drawerChannelRow.channelId),
        agentId: (__VLS_ctx.drawerChannelRow.agentId),
        readOnly: (true),
        readOnlyReason: (`${__VLS_ctx.tagFor((__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()).label} 渠道 · ${__VLS_ctx.drawerChannelRow.channelId} · 只读`),
        ref: "channelAiChatRef",
    }));
    const __VLS_233 = __VLS_232({
        key: ('ch-' + __VLS_ctx.drawerChannelRow.channelId),
        agentId: (__VLS_ctx.drawerChannelRow.agentId),
        readOnly: (true),
        readOnlyReason: (`${__VLS_ctx.tagFor((__VLS_ctx.drawerChannelRow?.channelType || 'web').toLowerCase()).label} 渠道 · ${__VLS_ctx.drawerChannelRow.channelId} · 只读`),
        ref: "channelAiChatRef",
    }, ...__VLS_functionalComponentArgsRest(__VLS_232));
    var __VLS_236 = {};
    var __VLS_234;
}
if (__VLS_ctx.channelDetailLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drawer-loading-overlay" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-loading-overlay']} */ ;
    let __VLS_238;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_239 = __VLS_asFunctionalComponent1(__VLS_238, new __VLS_238({
        ...{ class: "is-loading" },
        size: (28),
        color: "#94a3b8",
    }));
    const __VLS_240 = __VLS_239({
        ...{ class: "is-loading" },
        size: (28),
        color: "#94a3b8",
    }, ...__VLS_functionalComponentArgsRest(__VLS_239));
    /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
    const { default: __VLS_243 } = __VLS_241.slots;
    let __VLS_244;
    /** @ts-ignore @type { | typeof __VLS_components.Loading} */
    Loading;
    // @ts-ignore
    const __VLS_245 = __VLS_asFunctionalComponent1(__VLS_244, new __VLS_244({}));
    const __VLS_246 = __VLS_245({}, ...__VLS_functionalComponentArgsRest(__VLS_245));
    // @ts-ignore
    [tagFor, channelDrawer, drawerChannelRow, drawerChannelRow, drawerChannelRow, drawerChannelRow, drawerChannelRow, channelDetailLoading,];
    var __VLS_241;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
if (__VLS_ctx.channelTotal > __VLS_ctx.channelLimit) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drawer-ft" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-ft']} */ ;
    let __VLS_249;
    /** @ts-ignore @type { | typeof __VLS_components.elPagination | typeof __VLS_components.ElPagination | typeof __VLS_components['el-pagination']} */
    elPagination;
    // @ts-ignore
    const __VLS_250 = __VLS_asFunctionalComponent1(__VLS_249, new __VLS_249({
        ...{ 'onCurrentChange': {} },
        currentPage: (__VLS_ctx.channelPage),
        pageSize: (__VLS_ctx.channelLimit),
        total: (__VLS_ctx.channelTotal),
        layout: "prev, pager, next",
        small: true,
    }));
    const __VLS_251 = __VLS_250({
        ...{ 'onCurrentChange': {} },
        currentPage: (__VLS_ctx.channelPage),
        pageSize: (__VLS_ctx.channelLimit),
        total: (__VLS_ctx.channelTotal),
        layout: "prev, pager, next",
        small: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_250));
    let __VLS_254;
    const __VLS_255 = ({ currentChange: {} },
        { onCurrentChange: (__VLS_ctx.onChannelPageChange) });
    var __VLS_252;
    var __VLS_253;
}
// @ts-ignore
[channelTotal, channelTotal, channelLimit, channelLimit, channelPage, onChannelPageChange,];
var __VLS_215;
let __VLS_256;
/** @ts-ignore @type { | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer'] | typeof __VLS_components.elDrawer | typeof __VLS_components.ElDrawer | typeof __VLS_components['el-drawer']} */
elDrawer;
// @ts-ignore
const __VLS_257 = __VLS_asFunctionalComponent1(__VLS_256, new __VLS_256({
    modelValue: (__VLS_ctx.sessionDrawer),
    size: "55%",
    direction: "rtl",
    withHeader: (false),
}));
const __VLS_258 = __VLS_257({
    modelValue: (__VLS_ctx.sessionDrawer),
    size: "55%",
    direction: "rtl",
    withHeader: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_257));
const { default: __VLS_261 } = __VLS_259.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-wrap" },
});
/** @type {__VLS_StyleScopedClasses['drawer-wrap']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd-main" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-main']} */ ;
if (!__VLS_ctx.editingTitle) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drawer-hd-title" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-hd-title']} */ ;
    let __VLS_262;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_263 = __VLS_asFunctionalComponent1(__VLS_262, new __VLS_262({
        type: (__VLS_ctx.tagFor(__VLS_ctx.drawerSessionSource).type),
        size: "small",
        effect: "plain",
        ...{ class: (['src-tag', 'src-' + __VLS_ctx.drawerSessionSource]) },
    }));
    const __VLS_264 = __VLS_263({
        type: (__VLS_ctx.tagFor(__VLS_ctx.drawerSessionSource).type),
        size: "small",
        effect: "plain",
        ...{ class: (['src-tag', 'src-' + __VLS_ctx.drawerSessionSource]) },
    }, ...__VLS_functionalComponentArgsRest(__VLS_263));
    /** @type {__VLS_StyleScopedClasses['src-tag']} */ ;
    const { default: __VLS_267 } = __VLS_265.slots;
    (__VLS_ctx.tagFor(__VLS_ctx.drawerSessionSource).label);
    // @ts-ignore
    [tagFor, tagFor, sessionDrawer, editingTitle, drawerSessionSource, drawerSessionSource, drawerSessionSource,];
    var __VLS_265;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "drawer-hd-title-text" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-hd-title-text']} */ ;
    (__VLS_ctx.drawerSession?.title || '（无标题）');
    let __VLS_268;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_269 = __VLS_asFunctionalComponent1(__VLS_268, new __VLS_268({
        ...{ 'onClick': {} },
        icon: (__VLS_ctx.EditPen),
        circle: true,
        size: "small",
    }));
    const __VLS_270 = __VLS_269({
        ...{ 'onClick': {} },
        icon: (__VLS_ctx.EditPen),
        circle: true,
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_269));
    let __VLS_273;
    const __VLS_274 = ({ click: {} },
        { onClick: (__VLS_ctx.startEditTitle) });
    var __VLS_271;
    var __VLS_272;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drawer-hd-title" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-hd-title']} */ ;
    let __VLS_275;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_276 = __VLS_asFunctionalComponent1(__VLS_275, new __VLS_275({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.editTitle),
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_277 = __VLS_276({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.editTitle),
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_276));
    let __VLS_280;
    const __VLS_281 = ({ keyup: {} },
        { onKeyup: (__VLS_ctx.saveTitle) });
    var __VLS_278;
    var __VLS_279;
    let __VLS_282;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_283 = __VLS_asFunctionalComponent1(__VLS_282, new __VLS_282({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }));
    const __VLS_284 = __VLS_283({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_283));
    let __VLS_287;
    const __VLS_288 = ({ click: {} },
        { onClick: (__VLS_ctx.saveTitle) });
    const { default: __VLS_289 } = __VLS_285.slots;
    // @ts-ignore
    [drawerSession, EditPen, startEditTitle, editTitle, saveTitle, saveTitle,];
    var __VLS_285;
    var __VLS_286;
    let __VLS_290;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_291 = __VLS_asFunctionalComponent1(__VLS_290, new __VLS_290({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_292 = __VLS_291({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_291));
    let __VLS_295;
    const __VLS_296 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.editingTitle))
                    return;
                __VLS_ctx.editingTitle = false;
                // @ts-ignore
                [editingTitle,];
            } });
    const { default: __VLS_297 } = __VLS_293.slots;
    // @ts-ignore
    [];
    var __VLS_293;
    var __VLS_294;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-hd-sub" },
});
/** @type {__VLS_StyleScopedClasses['drawer-hd-sub']} */ ;
(__VLS_ctx.drawerSession?.agentName);
(__VLS_ctx.drawerSession?.messageCount ?? 0);
(__VLS_ctx.formatTokens(__VLS_ctx.drawerSession?.tokenEstimate ?? 0));
let __VLS_298;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_299 = __VLS_asFunctionalComponent1(__VLS_298, new __VLS_298({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.Close),
    circle: true,
    size: "small",
}));
const __VLS_300 = __VLS_299({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.Close),
    circle: true,
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_299));
let __VLS_303;
const __VLS_304 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.sessionDrawer = false;
            // @ts-ignore
            [formatTokens, Close, sessionDrawer, drawerSession, drawerSession, drawerSession,];
        } });
var __VLS_301;
var __VLS_302;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-chat-body" },
    ...{ style: {} },
});
/** @type {__VLS_StyleScopedClasses['drawer-chat-body']} */ ;
if (__VLS_ctx.sessionDrawer && __VLS_ctx.drawerSession) {
    const __VLS_305 = AiChat;
    // @ts-ignore
    const __VLS_306 = __VLS_asFunctionalComponent1(__VLS_305, new __VLS_305({
        key: ('sess-' + __VLS_ctx.drawerSession.id),
        agentId: (__VLS_ctx.drawerSession.agentId),
        readOnly: (true),
        readOnlyReason: (`${__VLS_ctx.tagFor(__VLS_ctx.drawerSessionSource).label} · ${__VLS_ctx.drawerSession.id} · 只读（如需继续请点右下角「继续对话」）`),
        ref: "sessionAiChatRef",
    }));
    const __VLS_307 = __VLS_306({
        key: ('sess-' + __VLS_ctx.drawerSession.id),
        agentId: (__VLS_ctx.drawerSession.agentId),
        readOnly: (true),
        readOnlyReason: (`${__VLS_ctx.tagFor(__VLS_ctx.drawerSessionSource).label} · ${__VLS_ctx.drawerSession.id} · 只读（如需继续请点右下角「继续对话」）`),
        ref: "sessionAiChatRef",
    }, ...__VLS_functionalComponentArgsRest(__VLS_306));
    var __VLS_310 = {};
    var __VLS_308;
}
if (__VLS_ctx.detailLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "drawer-loading-overlay" },
    });
    /** @type {__VLS_StyleScopedClasses['drawer-loading-overlay']} */ ;
    let __VLS_312;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_313 = __VLS_asFunctionalComponent1(__VLS_312, new __VLS_312({
        ...{ class: "is-loading" },
        size: (28),
        color: "#94a3b8",
    }));
    const __VLS_314 = __VLS_313({
        ...{ class: "is-loading" },
        size: (28),
        color: "#94a3b8",
    }, ...__VLS_functionalComponentArgsRest(__VLS_313));
    /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
    const { default: __VLS_317 } = __VLS_315.slots;
    let __VLS_318;
    /** @ts-ignore @type { | typeof __VLS_components.Loading} */
    Loading;
    // @ts-ignore
    const __VLS_319 = __VLS_asFunctionalComponent1(__VLS_318, new __VLS_318({}));
    const __VLS_320 = __VLS_319({}, ...__VLS_functionalComponentArgsRest(__VLS_319));
    // @ts-ignore
    [tagFor, sessionDrawer, drawerSessionSource, drawerSession, drawerSession, drawerSession, drawerSession, detailLoading,];
    var __VLS_315;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "drawer-ft" },
});
/** @type {__VLS_StyleScopedClasses['drawer-ft']} */ ;
let __VLS_323;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_324 = __VLS_asFunctionalComponent1(__VLS_323, new __VLS_323({
    ...{ 'onClick': {} },
}));
const __VLS_325 = __VLS_324({
    ...{ 'onClick': {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_324));
let __VLS_328;
const __VLS_329 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.sessionDrawer = false;
            // @ts-ignore
            [sessionDrawer,];
        } });
const { default: __VLS_330 } = __VLS_326.slots;
// @ts-ignore
[];
var __VLS_326;
var __VLS_327;
let __VLS_331;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_332 = __VLS_asFunctionalComponent1(__VLS_331, new __VLS_331({
    ...{ 'onClick': {} },
    type: "primary",
    icon: (__VLS_ctx.ChatLineRound),
    disabled: (!__VLS_ctx.drawerSession),
}));
const __VLS_333 = __VLS_332({
    ...{ 'onClick': {} },
    type: "primary",
    icon: (__VLS_ctx.ChatLineRound),
    disabled: (!__VLS_ctx.drawerSession),
}, ...__VLS_functionalComponentArgsRest(__VLS_332));
let __VLS_336;
const __VLS_337 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.continueSession(__VLS_ctx.drawerSession);
            // @ts-ignore
            [continueSession, drawerSession, drawerSession, ChatLineRound,];
        } });
const { default: __VLS_338 } = __VLS_334.slots;
// @ts-ignore
[];
var __VLS_334;
var __VLS_335;
// @ts-ignore
[];
var __VLS_259;
// @ts-ignore
var __VLS_237 = __VLS_236, __VLS_311 = __VLS_310;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
