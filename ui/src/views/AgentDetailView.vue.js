/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, onMounted, computed, watch, nextTick } from 'vue';
import { useRoute } from 'vue-router';
import { ArrowLeft, Plus, EditPen, Refresh, FolderOpened, Document, ArrowDown, Loading } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import SkillStudio from '../components/SkillStudio.vue';
import api, { agents as agentsApi, files as filesApi, memoryApi, cron as cronApi, sessions as sessionsApi, relationsApi, memoryConfigApi, agentChannels as agentChannelsApi, agentConversations, models as modelsApi } from '../api';
import AiChat from '../components/AiChat.vue';
import WorkspaceChatLayout from '../components/WorkspaceChatLayout.vue';
const route = useRoute();
const agentId = route.params.id;
const agent = ref(null);
const activeTab = ref('chat');
// 切到关系 tab 时自动重拉 —— 确保看到"其他成员给自己加的反向关系"
watch(activeTab, (val) => {
    if (val === 'relations')
        loadRelations();
});
const mobileSessionOpen = ref(false);
// 规范化 source：把后端返回的 source 字段（或从 session ID 前缀推断）映射为标准值
function normalizeSource(raw, sessionId) {
    const s = (raw || '').toLowerCase();
    if (s === 'feishu' || s === 'telegram' || s === 'web')
        return s;
    // 兜底：从 ID 前缀推断（后端老数据可能没 source）
    if (sessionId.startsWith('feishu-'))
        return 'feishu';
    if (sessionId.startsWith('tg-'))
        return 'telegram';
    if (sessionId.startsWith('web-'))
        return 'web';
    return 'panel';
}
// 当 session 没有 title 时，根据 ID 前缀生成友好标签
function sessionLabelFromId(id) {
    if (id.startsWith('feishu-')) {
        const rest = id.slice(7);
        return '飞书聊天 · ' + (rest.length > 10 ? rest.slice(0, 8) + '…' : rest);
    }
    if (id.startsWith('tg-'))
        return 'Telegram · ' + id.slice(3, 11);
    if (id.startsWith('web-'))
        return '网页聊天 · ' + id.slice(4, 12);
    return '新对话';
}
// 去掉群聊消息里自动附加的发送者 ID 前缀（形如 "[ou_xxxxx]: 正文" 或 "[userName]: 正文"）
// 这样在侧边栏标题里显示更干净的正文
function cleanTitleForSidebar(raw, source) {
    if (!raw)
        return '';
    const trimmed = raw.trim();
    // 只对渠道来源的 session 做前缀剥离（面板对话一般不会带 [xxx]:）
    if (source !== 'feishu' && source !== 'telegram')
        return trimmed;
    const m = trimmed.match(/^\[([^\]]{1,60})\]\s*[:：]\s*(.+)$/);
    if (m && m[2])
        return m[2].trim();
    return trimmed;
}
const SOURCE_TAG_PANEL = { label: '面板', type: 'info', className: 'tag-panel' };
const SOURCE_TAG_FEISHU = { label: '飞书', type: 'primary', className: 'tag-feishu' };
const SOURCE_TAG_TELEGRAM = { label: 'TG', type: 'success', className: 'tag-telegram' };
const SOURCE_TAG_WEB = { label: 'Web', type: 'warning', className: 'tag-web' };
function sourceTag(source) {
    switch (source) {
        case 'feishu': return SOURCE_TAG_FEISHU;
        case 'telegram': return SOURCE_TAG_TELEGRAM;
        case 'web': return SOURCE_TAG_WEB;
        default: return SOURCE_TAG_PANEL;
    }
}
// 渠道主色（用于侧边栏左侧色条与小圆点）
function sourceColor(source) {
    switch (source) {
        case 'feishu': return '#6366f1';
        case 'telegram': return '#10b981';
        case 'web': return '#f59e0b';
        default: return '#94a3b8';
    }
}
const aiChatRef = ref();
const agentSessions = ref([]);
const allSidebarItems = ref([]);
const selectedItem = ref(null);
const viewMode = ref(null);
const historyMessages = ref([]);
const historyLoading = ref(false);
const sessionsLoading = ref(false);
const activeSessionId = ref();
// 判断当前选中的 session 是否应只读。
// 只读条件:
//   1. type === 'channel' (convlog 存储的访客对话, 没有可续接的 session 上下文)
//   2. source 是飞书/Telegram 等外部客户端渠道 (面板这边发消息也回不到对方客户端)
// 不只读 (可继续对话):
//   - type === 'panel' 且 source === 'panel' 或 'web': 面板自建会话
//     (web 指 'ses-xxx' 前缀的面板内新建会话, 完全可继续)
const EXTERNAL_CHANNEL_SOURCES = ['feishu', 'telegram'];
const isReadOnlySession = computed(() => {
    const it = selectedItem.value;
    if (!it)
        return false;
    if (it.type === 'channel')
        return true; // convlog 型访客对话, 只读
    return EXTERNAL_CHANNEL_SOURCES.includes(it.source);
});
const readOnlyReason = computed(() => {
    const it = selectedItem.value;
    if (!it)
        return '';
    if (it.type === 'channel') {
        const src = sourceTag(it.source).label;
        return `此对话来自${src}访客链接，仅可查看历史 · 无法在面板内直接回复访客`;
    }
    const src = sourceTag(it.source).label;
    return `此对话来自${src}客户端，仅可查看历史 · 要回复请前往 ${src} 客户端操作`;
});
async function loadSidebarItems() {
    sessionsLoading.value = true;
    try {
        const [sesRes, chRes] = await Promise.all([
            sessionsApi.list({ agentId, limit: 50 }),
            agentConversations.list(agentId).catch(() => ({ data: [] })),
        ]);
        const panelItems = (sesRes.data.sessions || [])
            .filter(s => !s.id.startsWith('subagent-') && !s.id.startsWith('goal-') && !s.id.startsWith('__'))
            .map(s => {
            const src = normalizeSource(s.source, s.id);
            const cleanTitle = cleanTitleForSidebar(s.title, src);
            return {
                id: s.id,
                type: 'panel',
                source: src,
                label: cleanTitle || sessionLabelFromId(s.id),
                messageCount: s.messageCount,
                lastAt: typeof s.lastAt === 'string' ? new Date(s.lastAt).getTime() : (s.lastAt || 0),
                tokenEstimate: s.tokenEstimate,
                _panel: s,
            };
        });
        const channelItems = (chRes.data || []).map(ch => ({
            id: ch.channelId,
            type: 'channel',
            source: (ch.channelType || 'web').toLowerCase(),
            channelType: ch.channelType,
            label: ch.channelId,
            messageCount: ch.messageCount,
            lastAt: typeof ch.lastAt === 'string' ? new Date(ch.lastAt).getTime() : (ch.lastAt || 0),
            _channel: ch,
        }));
        allSidebarItems.value = [...panelItems, ...channelItems].sort((a, b) => b.lastAt - a.lastAt);
        agentSessions.value = sesRes.data.sessions || []; // backward compat
    }
    catch { }
    finally {
        sessionsLoading.value = false;
    }
}
function isSelectedItem(item) {
    if (!selectedItem.value)
        return false;
    return selectedItem.value.type === item.type && selectedItem.value.id === item.id;
}
async function selectSidebarItem(item) {
    selectedItem.value = item;
    if (item.type === 'panel') {
        // 面板类 session（含 source=feishu/telegram 的 channel-originated session）
        // 统一走 resumeSession：读取完整 session JSONL（含 toolCalls）
        viewMode.value = 'chat';
        activeSessionId.value = item.id;
        await nextTick();
        aiChatRef.value?.resumeSession(item.id);
    }
    else {
        // type === 'channel' 类对话（convlog 存储）—— 把 convlog 转成 ChatMsg 注入到 AiChat 只读展示
        viewMode.value = 'chat';
        historyMessages.value = [];
        historyLoading.value = true;
        try {
            const res = await agentConversations.messages(agentId, item.id, { limit: 500, offset: 0 });
            const raw = (res.data.messages || []).filter(m => !(m.content || '').trim().startsWith('<task-notification>'));
            historyMessages.value = raw;
            // 转成 AiChat 识别的 ChatMsg 结构
            const msgs = raw.map(m => ({
                role: m.role,
                text: m.role === 'user' && m.sender ? `[${m.sender}] ${m.content}` : m.content,
            }));
            await nextTick();
            aiChatRef.value?.loadHistoryMessages(msgs);
        }
        catch { }
        finally {
            historyLoading.value = false;
        }
    }
}
function newSession() {
    selectedItem.value = null;
    viewMode.value = null;
    activeSessionId.value = undefined;
    aiChatRef.value?.startNewSession();
}
function onSessionChange(sessionId) {
    activeSessionId.value = sessionId;
    localStorage.setItem(`zyhive_session_${agentId}`, sessionId);
    setTimeout(loadSidebarItems, 500);
}
function formatRelative(ms) {
    if (!ms)
        return '';
    const diff = Date.now() - ms;
    if (diff < 60_000)
        return '刚刚';
    if (diff < 3_600_000)
        return `${Math.floor(diff / 60_000)}分前`;
    if (diff < 86_400_000)
        return `${Math.floor(diff / 3_600_000)}小时前`;
    return `${Math.floor(diff / 86_400_000)}天前`;
}
// ── @ 其他成员 ─────────────────────────────────────────────────────────────
const showAtPanel = ref(false);
const atTargetId = ref('');
const atMessage = ref('');
const atSending = ref(false);
const otherAgents = ref([]);
function toggleAtPanel() {
    showAtPanel.value = !showAtPanel.value;
    if (showAtPanel.value && !otherAgents.value.length)
        loadOtherAgents();
}
async function loadOtherAgents() {
    try {
        const res = await agentsApi.list();
        otherAgents.value = res.data.filter(a => a.id !== agentId);
    }
    catch {
        otherAgents.value = [];
    }
}
function onAtAgentSelect(id) {
    // 同步在 AiChat 输入框填入 @AgentName: 前缀（方便用户知道当前 @ 模式）
    const target = otherAgents.value.find(a => a.id === id);
    if (target) {
        aiChatRef.value?.fillInput(`@${target.name}: `);
    }
}
async function sendAtMessage() {
    const targetId = atTargetId.value;
    const msg = atMessage.value.trim();
    if (!targetId || !msg)
        return;
    const targetAgent = otherAgents.value.find(a => a.id === targetId);
    const targetName = targetAgent?.name ?? targetId;
    atSending.value = true;
    // 在对话区显示「转发」提示气泡
    const forwardBubble = {
        role: 'user',
        text: `→ 转发给 ${targetName}：\n${msg}`,
    };
    aiChatRef.value?.appendMessage(forwardBubble);
    try {
        const res = await agentsApi.message(targetId, msg, agentId);
        const reply = res.data.response;
        // 显示「回复」气泡
        const replyBubble = {
            role: 'assistant',
            text: `← **${targetName}** 回复：\n\n${reply}`,
        };
        aiChatRef.value?.appendMessage(replyBubble);
        // 清空输入
        atMessage.value = '';
        atTargetId.value = '';
        showAtPanel.value = false;
        ElMessage.success(`${targetName} 已回复`);
    }
    catch (e) {
        const errMsg = {
            role: 'system',
            text: `[失败] 转发失败：${e.response?.data?.error ?? e.message ?? '网络错误'}`,
        };
        aiChatRef.value?.appendMessage(errMsg);
        ElMessage.error('转发失败');
    }
    finally {
        atSending.value = false;
    }
}
// Identity/Soul
const identityContent = ref('');
const soulContent = ref('');
const userProfileContent = ref('');
const userProfilePlaceholder = `# 用户档案

> 这份档案写给服务你的 AI。它会在每次对话开始时被读取。
> 你可以随时修改，只写你愿意让 AI 知道的部分。留空也完全可以。

## 基本
- 称呼：
- 所在地 / 时区：
- 主要语言：

## 沟通偏好
- 回答长度：简洁 / 中等 / 详尽
- 风格：正式 / 轻松 / 直给不啰嗦
- emoji：要 / 不要 / 少量
- 不确定时：直说"不知道" / 给出最佳猜测并标记

## 在做的事 / 长期关心
-

## 禁忌
-`;
const wishlist = ref(null);
const wishlistLoading = ref(false);
async function loadWishlist() {
    wishlistLoading.value = true;
    try {
        const res = await api.get(`/agents/${agentId}/wishlist`);
        wishlist.value = res.data;
    }
    catch (e) {
        if (e?.response?.status !== 404) {
            ElMessage.error('加载愿望清单失败');
        }
        else {
            wishlist.value = { total: 0, wishes: [] };
        }
    }
    finally {
        wishlistLoading.value = false;
    }
}
function wishPriorityType(p) {
    if (p === 'P0')
        return 'danger';
    if (p === 'P1')
        return 'warning';
    return 'info';
}
const toolHealth = ref(null);
const toolHealthLoading = ref(false);
const blockedTools = computed(() => toolHealth.value?.tools.filter(t => !t.ready) ?? []);
const readyTools = computed(() => toolHealth.value?.tools.filter(t => t.ready) ?? []);
async function runToolHealth(force = false) {
    toolHealthLoading.value = true;
    try {
        const url = force
            ? `/agents/${agentId}/tool-health?refresh=1`
            : `/agents/${agentId}/tool-health`;
        const res = await api.get(url);
        toolHealth.value = res.data;
    }
    catch (e) {
        ElMessage.error('工具体检失败: ' + (e?.message || '未知错误'));
    }
    finally {
        toolHealthLoading.value = false;
    }
}
// P0.5 — re-ping provider bypassing the 30s server cache.
async function refreshProviderHealth() {
    await runToolHealth(true);
}
// P0.5 — user-facing hint based on status code.
function providerHealthTip(ph) {
    if (!ph || ph.ok)
        return '';
    const code = ph.statusCode;
    if (code === 401 || code === 403)
        return '认证失败 — 请在「模型」页检查 API Key 是否正确。';
    if (code === 429)
        return 'Provider 侧限流中，稍后重试即可。';
    if (code >= 500)
        return 'Provider 服务暂时不可用（5xx）— 可能是厂商故障，稍后重试。';
    if (code === 0)
        return '网络不可达 — 请检查 baseURL 配置和服务器网络。';
    return '未知错误 — 点击「重新检测」尝试刷新。';
}
// Model selector
const modelList = ref([]);
const modelsLoaded = ref(false);
const agentModelId = ref('');
const agentModelSaving = ref(false);
// 当前选中的模型是否不支持工具调用
const selectedModelNoTools = computed(() => {
    const m = modelList.value.find(m => m.id === agentModelId.value);
    return m ? m.supportsTools === false : false;
});
// ── Env Vars ──────────────────────────────────────────────────────────────────
const envVarsList = ref([]);
const newEnvKey = ref('');
const newEnvValue = ref('');
const envSaving = ref(false);
function loadEnvVars() {
    const env = agent.value?.env || {};
    envVarsList.value = Object.entries(env).map(([key, value]) => ({ key, value }));
}
function addEnvVar() {
    const key = newEnvKey.value.trim();
    if (!key)
        return;
    const existing = envVarsList.value.findIndex(e => e.key === key);
    if (existing >= 0) {
        envVarsList.value[existing].value = newEnvValue.value;
    }
    else {
        envVarsList.value.push({ key, value: newEnvValue.value });
    }
    newEnvKey.value = '';
    newEnvValue.value = '';
}
function removeEnvVar(index) {
    envVarsList.value.splice(index, 1);
}
async function saveEnvVars() {
    envSaving.value = true;
    try {
        const env = {};
        for (const { key, value } of envVarsList.value) {
            if (key.trim())
                env[key.trim()] = value;
        }
        const res = await agentsApi.update(agentId, { env });
        agent.value = res.data;
        ElMessage.success('环境变量已保存');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        envSaving.value = false;
    }
}
// ── Heartbeat ─────────────────────────────────────────────────────────────────
const heartbeatForm = ref({ enabled: false, intervalMin: 30, prompt: '' });
const heartbeatSaving = ref(false);
const heartbeatSaved = ref(false);
function loadHeartbeat() {
    const hb = agent.value?.heartbeat;
    heartbeatForm.value = {
        enabled: hb?.enabled ?? false,
        intervalMin: hb?.intervalMin || 30,
        prompt: hb?.prompt || '',
    };
}
async function saveHeartbeat() {
    heartbeatSaving.value = true;
    try {
        const hb = heartbeatForm.value.enabled
            ? {
                enabled: true,
                intervalMin: heartbeatForm.value.intervalMin || 30,
                prompt: heartbeatForm.value.prompt || undefined,
            }
            : null;
        const res = await agentsApi.update(agentId, { heartbeat: hb });
        agent.value = res.data;
        heartbeatSaved.value = true;
        setTimeout(() => { heartbeatSaved.value = false; }, 2500);
        ElMessage.success(heartbeatForm.value.enabled ? '心跳已启动' : '心跳已停止');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        heartbeatSaving.value = false;
    }
}
// ── Tool Policy ──────────────────────────────────────────────────────────────
const toolPolicyForm = ref({
    profile: '',
    allow: [],
    deny: [],
});
const toolPolicyAllowInput = ref('');
const toolPolicyDenyInput = ref('');
const toolPolicySaving = ref(false);
const toolPolicySaved = ref(false);
// All built-in tool names for preview
const ALL_TOOL_NAMES = [
    'read', 'write', 'edit', 'grep', 'glob',
    'exec', 'process',
    'web_fetch', 'web_search',
    'memory_search',
    'browser', 'show_image', 'image',
    'agent_list', 'agent_spawn', 'agent_tasks', 'agent_kill', 'agent_result',
    'sessions_list', 'sessions_history', 'sessions_send',
    'cron_list', 'cron_add', 'cron_remove',
    'send_message', 'send_file',
    'self_list_skills', 'self_install_skill', 'self_uninstall_skill', 'self_rename', 'self_update_soul', 'self_set_env', 'self_delete_env',
    'project_list', 'project_read', 'project_write', 'project_create', 'project_glob',
    'report_result',
];
const TOOL_GROUPS = {
    'group:fs': ['read', 'write', 'edit', 'grep', 'glob'],
    'group:runtime': ['exec', 'process'],
    'group:web': ['web_fetch', 'web_search'],
    'group:memory': ['memory_search'],
    'group:ui': ['browser', 'show_image', 'image'],
    'group:agent': ['agent_list', 'agent_spawn', 'agent_tasks', 'agent_kill', 'agent_result'],
    'group:sessions': ['sessions_list', 'sessions_history', 'sessions_send'],
    'group:cron': ['cron_list', 'cron_add', 'cron_remove'],
    'group:messaging': ['send_message', 'send_file'],
    'group:self': ['self_list_skills', 'self_install_skill', 'self_uninstall_skill', 'self_rename', 'self_update_soul', 'self_set_env', 'self_delete_env'],
    'group:project': ['project_list', 'project_read', 'project_write', 'project_create', 'project_glob'],
};
const PROFILE_ALLOWLISTS = {
    'full': null,
    'coding': ['read', 'write', 'edit', 'grep', 'glob', 'exec', 'process', 'agent_list', 'agent_spawn', 'agent_tasks', 'agent_kill', 'agent_result', 'memory_search', 'image', 'web_fetch', 'web_search'],
    'messaging': ['send_message', 'send_file', 'sessions_list', 'sessions_history', 'sessions_send', 'memory_search'],
    'minimal': ['send_message', 'memory_search'],
};
function expandPatterns(patterns) {
    const result = new Set();
    for (const p of patterns) {
        if (p === '*') {
            ALL_TOOL_NAMES.forEach(n => result.add(n));
            continue;
        }
        if (TOOL_GROUPS[p]) {
            TOOL_GROUPS[p].forEach(n => result.add(n));
            continue;
        }
        result.add(p.toLowerCase());
    }
    return result;
}
const toolPolicyPreview = computed(() => {
    const profile = toolPolicyForm.value.profile;
    const allow = toolPolicyForm.value.allow;
    const deny = toolPolicyForm.value.deny;
    if (!profile && !allow.length && !deny.length)
        return [];
    const baseAllow = profile && profile !== 'full' && PROFILE_ALLOWLISTS[profile]
        ? new Set(PROFILE_ALLOWLISTS[profile])
        : null;
    const extraAllow = expandPatterns(allow);
    const denySet = expandPatterns(deny);
    return ALL_TOOL_NAMES.map(name => {
        const denied = denySet.has('*') || denySet.has(name);
        if (denied)
            return { name, denied: true };
        let allowed = baseAllow === null ? true : baseAllow.has(name);
        if (extraAllow.has('*') || extraAllow.has(name))
            allowed = true;
        return { name, denied: !allowed };
    });
});
function loadToolPolicy() {
    const p = agent.value?.toolPolicy;
    toolPolicyForm.value = {
        profile: p?.profile || '',
        allow: p?.allow ? [...p.allow] : [],
        deny: p?.deny ? [...p.deny] : [],
    };
}
function addToolPolicyTag(type) {
    const input = type === 'allow' ? toolPolicyAllowInput : toolPolicyDenyInput;
    const val = input.value.trim();
    if (!val)
        return;
    if (!toolPolicyForm.value[type].includes(val)) {
        toolPolicyForm.value[type].push(val);
    }
    input.value = '';
}
function quickDeny(pattern) {
    if (!toolPolicyForm.value.deny.includes(pattern)) {
        toolPolicyForm.value.deny.push(pattern);
    }
}
function clearToolPolicy() {
    toolPolicyForm.value = { profile: '', allow: [], deny: [] };
}
async function saveToolPolicy() {
    toolPolicySaving.value = true;
    try {
        const policy = {};
        if (toolPolicyForm.value.profile)
            policy.profile = toolPolicyForm.value.profile;
        if (toolPolicyForm.value.allow.length)
            policy.allow = toolPolicyForm.value.allow;
        if (toolPolicyForm.value.deny.length)
            policy.deny = toolPolicyForm.value.deny;
        const payload = Object.keys(policy).length ? policy : null;
        const res = await agentsApi.update(agentId, { toolPolicy: payload });
        agent.value = res.data;
        toolPolicySaved.value = true;
        setTimeout(() => { toolPolicySaved.value = false; }, 2000);
        ElMessage.success('工具权限已保存');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        toolPolicySaving.value = false;
    }
}
// Memory config (automatic consolidation)
const memCfg = ref({
    enabled: false,
    schedule: 'daily',
    keepTurns: 3,
    focusHint: '',
    cronJobId: '',
});
const memCfgSaving = ref(false);
const memConsolidating = ref(false);
async function loadMemConfig() {
    try {
        const res = await memoryConfigApi.getConfig(agentId);
        memCfg.value = res.data;
        loadMemLogs();
    }
    catch {
        // use defaults
    }
}
async function saveMemConfig() {
    memCfgSaving.value = true;
    try {
        const res = await memoryConfigApi.setConfig(agentId, memCfg.value);
        memCfg.value = res.data;
        ElMessage.success(memCfg.value.enabled ? '自动记忆已开启' : '自动记忆已关闭');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        memCfgSaving.value = false;
    }
}
async function consolidateNow() {
    memConsolidating.value = true;
    try {
        await memoryConfigApi.consolidate(agentId);
        ElMessage.success('记忆整理已在后台启动（约需10~30秒），稍后自动刷新日志');
        setTimeout(loadMemLogs, 10000); // 10秒后刷新日志
    }
    catch {
        ElMessage.error('整理失败');
    }
    finally {
        memConsolidating.value = false;
    }
}
// Consolidation run log
const memLogs = ref([]);
const memLogsLoading = ref(false);
async function loadMemLogs() {
    memLogsLoading.value = true;
    try {
        const res = await memoryConfigApi.runLog(agentId);
        memLogs.value = res.data || [];
    }
    catch {
        memLogs.value = [];
    }
    finally {
        memLogsLoading.value = false;
    }
}
// Memory tree
const memoryTreeData = ref([]);
const memoryEditPath = ref('');
const memoryEditContent = ref('');
const memorySaving = ref(false);
const memoryFileBreadcrumb = ref([]);
const showNewMemoryFile = ref(false);
const newMemoryPath = ref('');
const showDailyEntry = ref(false);
const dailyEntryContent = ref('');
// (Workspace tab now uses WorkspaceChatLayout component)
// Relations
const parsedRelations = ref([]);
const relationsSaving = ref(false);
const newRelation = ref({ agentId: '', agentName: '', relationType: '平级协作', strength: '常用', desc: '' });
async function loadRelations() {
    try {
        const res = await relationsApi.get(agentId);
        parsedRelations.value = res.data.parsed || [];
    }
    catch {
        parsedRelations.value = [];
    }
}
function onRelationAgentChange(id) {
    const a = otherAgents.value.find(x => x.id === id);
    newRelation.value.agentName = a ? a.name : id;
}
async function addRelation() {
    if (!newRelation.value.agentId)
        return;
    // Avoid duplicate
    const exists = parsedRelations.value.find(r => r.agentId === newRelation.value.agentId);
    if (exists) {
        ElMessage.warning('该成员关系已存在，请先删除再重新添加');
        return;
    }
    parsedRelations.value.push({ ...newRelation.value });
    newRelation.value = { agentId: '', agentName: '', relationType: '平级协作', strength: '常用', desc: '' };
    await saveRelations();
}
async function deleteRelation(index) {
    parsedRelations.value.splice(index, 1);
    await saveRelations();
}
function serializeRelations() {
    if (parsedRelations.value.length === 0)
        return '';
    const header = '| 成员ID | 成员名称 | 关系类型 | 关系程度 | 说明 |\n|--------|--------|--------|--------|------|';
    const rows = parsedRelations.value
        .map(r => `| ${r.agentId} | ${r.agentName} | ${r.relationType} | ${r.strength} | ${r.desc || ''} |`)
        .join('\n');
    return header + '\n' + rows;
}
async function saveRelations() {
    relationsSaving.value = true;
    try {
        await relationsApi.put(agentId, serializeRelations());
        // 保存成功后重拉, 同步后端做过的规范化/双向补全等副作用
        await loadRelations();
        ElMessage.success('关系已保存');
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        relationsSaving.value = false;
    }
}
function avatarColor(id) {
    const colors = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#909399', '#B45309', '#7C3AED', '#0891B2'];
    let hash = 0;
    for (let i = 0; i < id.length; i++)
        hash = id.charCodeAt(i) + ((hash << 5) - hash);
    return colors[Math.abs(hash) % colors.length] ?? '#409EFF';
}
function relationTypeColor(type) {
    if (type === '上级')
        return 'danger';
    if (type === '下级')
        return ''; // blue = default primary
    if (type === '平级协作')
        return 'success';
    return 'info'; // 支持
}
function strengthColor(s) {
    if (s === '核心')
        return 'danger';
    if (s === '常用')
        return 'warning';
    return 'info';
}
// Cron
const cronJobs = ref([]);
const showCronCreate = ref(false);
const showCronLogs = ref(false);
const cronLogsJob = ref(null);
const cronLogs = ref([]);
const loadingCronLogs = ref(false);
const cronForm = ref({ name: '', expr: '0 9 * * *', tz: 'Asia/Shanghai', message: '', enabled: true });
function statusType(s) {
    return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info';
}
function statusLabel(s) {
    return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲';
}
function formatSize(bytes) {
    if (!bytes)
        return '0 B';
    if (bytes < 1024)
        return bytes + ' B';
    if (bytes < 1048576)
        return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1048576).toFixed(1) + ' MB';
}
function formatTimestamp(ms) {
    if (!ms)
        return '';
    return new Date(ms).toLocaleString();
}
// Load agent
// ── Per-agent Channel management ──────────────────────────────────────────
const agentChannelList = ref([]);
const channelsLoading = ref(false);
const channelDialogVisible = ref(false);
const channelEditingId = ref('');
const pendingChannelId = ref(''); // pre-generated id for new web channel
const channelSaving = ref(false);
const testingChannelId = ref('');
const channelForm = ref({
    type: 'telegram',
    name: '',
    enabled: true,
    botToken: '',
    allowedFrom: '',
    webPassword: '',
    webWelcome: '',
    webTitle: '',
    appId: '',
    appSecret: '',
});
// ── Token inline validation ────────────────────────────────────────────────
const tokenCheckState = ref({ loading: false, status: '' });
let tokenDebounceTimer = null;
function ismaskedToken(v) {
    return /^\*+$/.test(v);
}
async function doCheckToken() {
    const token = channelForm.value.botToken;
    if (!token || ismaskedToken(token))
        return;
    tokenCheckState.value = { loading: true, status: '' };
    try {
        const res = await agentChannelsApi.checkToken(agentId, token);
        const d = res.data;
        if (d.duplicate) {
            tokenCheckState.value = { loading: false, status: 'duplicate', usedBy: d.usedBy, usedByCh: d.usedByCh };
        }
        else if (d.valid) {
            tokenCheckState.value = { loading: false, status: 'ok', botName: d.botName };
            // Auto-fill name if empty
            if (!channelForm.value.name && d.botName)
                channelForm.value.name = d.botName;
        }
        else {
            tokenCheckState.value = { loading: false, status: 'error', error: d.error || 'Token 无效' };
        }
    }
    catch {
        tokenCheckState.value = { loading: false, status: 'error', error: '网络错误，请重试' };
    }
}
// Auto-check when token input stabilises (800ms debounce, min length ~20)
watch(() => channelForm.value.type, (val) => {
    if (!channelEditingId.value) {
        pendingChannelId.value = genChannelId(val);
    }
});
watch(() => channelForm.value.botToken, (val) => {
    // Reset state on change
    tokenCheckState.value = { loading: false, status: '' };
    if (tokenDebounceTimer)
        clearTimeout(tokenDebounceTimer);
    // Telegram tokens are "botId:hash" — typically 40+ chars; skip short/masked values
    if (!val || ismaskedToken(val) || val.length < 20 || !val.includes(':'))
        return;
    tokenDebounceTimer = setTimeout(doCheckToken, 800);
});
function webChatUrl(aid, chId) {
    return chId
        ? `${window.location.origin}/chat/${aid}/${chId}`
        : `${window.location.origin}/chat/${aid}`;
}
function copyUrl(url) {
    navigator.clipboard.writeText(url).then(() => ElMessage.success('链接已复制'));
}
async function loadAgentChannels() {
    channelsLoading.value = true;
    try {
        const res = await agentChannelsApi.list(agentId);
        agentChannelList.value = res.data || [];
    }
    catch {
        agentChannelList.value = [];
    }
    finally {
        channelsLoading.value = false;
    }
}
function genChannelId(type) {
    return type + '-' + Date.now().toString(36);
}
function openAddChannel() {
    channelEditingId.value = '';
    const defaultName = agent.value?.name || '';
    pendingChannelId.value = genChannelId('telegram'); // default, updated on type change
    channelForm.value = { type: 'telegram', name: defaultName, enabled: true, botToken: '', allowedFrom: '', webPassword: '', webWelcome: '', webTitle: '', appId: '', appSecret: '' };
    tokenCheckState.value = { loading: false, status: '' };
    channelDialogVisible.value = true;
}
function openEditChannel(row) {
    channelEditingId.value = row.id;
    channelForm.value = {
        type: row.type,
        name: row.name,
        enabled: row.enabled,
        botToken: row.config?.botToken || '',
        allowedFrom: row.config?.allowedFrom || '',
        webPassword: '', // password always cleared on edit for security
        webWelcome: row.config?.welcomeMsg || '',
        webTitle: row.config?.title || '',
        appId: row.config?.appId || '',
        appSecret: '', // secret always cleared on edit for security
    };
    tokenCheckState.value = { loading: false, status: '' };
    channelDialogVisible.value = true;
}
async function saveChannelDialog() {
    if (!channelForm.value.name || !channelForm.value.type) {
        ElMessage.warning('请填写名称和类型');
        return;
    }
    if (tokenCheckState.value.status === 'duplicate') {
        ElMessage.error(`Bot Token 已被成员「${tokenCheckState.value.usedBy}」使用，请更换`);
        return;
    }
    channelSaving.value = true;
    try {
        const newConfig = {};
        if (channelForm.value.type === 'telegram') {
            if (channelForm.value.botToken)
                newConfig.botToken = channelForm.value.botToken;
            if (channelForm.value.allowedFrom)
                newConfig.allowedFrom = channelForm.value.allowedFrom;
        }
        else if (channelForm.value.type === 'web') {
            if (channelForm.value.webPassword)
                newConfig.password = channelForm.value.webPassword;
            if (channelForm.value.webWelcome)
                newConfig.welcomeMsg = channelForm.value.webWelcome;
            if (channelForm.value.webTitle)
                newConfig.title = channelForm.value.webTitle;
        }
        else if (channelForm.value.type === 'feishu') {
            if (channelForm.value.appId)
                newConfig.appId = channelForm.value.appId;
            if (channelForm.value.appSecret)
                newConfig.appSecret = channelForm.value.appSecret;
            if (channelForm.value.allowedFrom)
                newConfig.allowedFrom = channelForm.value.allowedFrom;
        }
        if (channelEditingId.value) {
            // Update existing
            const list = agentChannelList.value.map(ch => {
                if (ch.id !== channelEditingId.value)
                    return ch;
                return { ...ch, name: channelForm.value.name, type: channelForm.value.type, enabled: channelForm.value.enabled, config: { ...ch.config, ...newConfig } };
            });
            await agentChannelsApi.set(agentId, list);
        }
        else {
            // Add new
            const newEntry = {
                id: pendingChannelId.value || genChannelId(channelForm.value.type),
                name: channelForm.value.name,
                type: channelForm.value.type,
                enabled: channelForm.value.enabled,
                config: newConfig,
                status: 'untested',
            };
            await agentChannelsApi.set(agentId, [...agentChannelList.value, newEntry]);
        }
        ElMessage.success(channelForm.value.type === 'web' ? '保存成功，Web 聊天页立即生效' : '保存成功，重启后新渠道生效');
        channelDialogVisible.value = false;
        await loadAgentChannels();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        channelSaving.value = false;
    }
}
async function saveChannels() {
    try {
        await agentChannelsApi.set(agentId, agentChannelList.value);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
        await loadAgentChannels(); // revert UI state on error
    }
}
async function deleteAgentChannel(row) {
    const updated = agentChannelList.value.filter(ch => ch.id !== row.id);
    try {
        await agentChannelsApi.set(agentId, updated);
        agentChannelList.value = updated;
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
async function testAgentChannel(row) {
    testingChannelId.value = row.id;
    try {
        const res = await agentChannelsApi.test(agentId, row.id);
        if (res.data.valid) {
            ElMessage.success(res.data.botName ? `测试成功 (@${res.data.botName})` : '测试成功');
        }
        else {
            ElMessage.error(res.data.error || '测试失败');
        }
        await loadAgentChannels();
    }
    catch {
        ElMessage.error('测试请求失败');
    }
    finally {
        testingChannelId.value = '';
    }
}
// ── Pending users (待审核用户) ────────────────────────────────────────────
const pendingUsers = ref({});
const pendingLoading = ref({});
const expandedPending = ref('');
const allowingUserId = ref('');
async function loadPendingUsers(chId) {
    pendingLoading.value[chId] = true;
    try {
        const res = await agentChannelsApi.listPending(agentId, chId);
        pendingUsers.value[chId] = res.data || [];
    }
    catch {
        pendingUsers.value[chId] = [];
    }
    finally {
        pendingLoading.value[chId] = false;
    }
}
function togglePending(chId) {
    if (expandedPending.value === chId) {
        expandedPending.value = '';
    }
    else {
        expandedPending.value = chId;
        loadPendingUsers(chId);
    }
}
async function allowUser(chId, userId) {
    allowingUserId.value = `${chId}-${userId}`;
    try {
        await agentChannelsApi.allowUser(agentId, chId, userId);
        ElMessage.success(`用户 ${userId} 已加入白名单`);
        await loadPendingUsers(chId);
        await loadAgentChannels(); // refresh allowedFrom display
    }
    catch {
        ElMessage.error('操作失败');
    }
    finally {
        allowingUserId.value = '';
    }
}
async function dismissUser(chId, userId) {
    try {
        await agentChannelsApi.dismissUser(agentId, chId, userId);
        ElMessage.success('已忽略');
        await loadPendingUsers(chId);
    }
    catch {
        ElMessage.error('操作失败');
    }
}
async function removeAllowed(chId, userId) {
    try {
        await ElMessageBox.confirm(`确定将用户 ${userId} 从白名单中移除？移除后该用户将无法使用此 Bot。`, '移除白名单', { confirmButtonText: '确认移除', cancelButtonText: '取消', type: 'warning' });
    }
    catch {
        return; // user cancelled
    }
    try {
        await agentChannelsApi.removeAllowed(agentId, chId, userId);
        ElMessage.success(`用户 ${userId} 已从白名单移除`);
        await loadAgentChannels();
    }
    catch {
        ElMessage.error('操作失败');
    }
}
onMounted(async () => {
    try {
        const res = await agentsApi.get(agentId);
        agent.value = res.data;
    }
    catch {
        ElMessage.error('加载 Agent 失败');
    }
    loadIdentityFiles();
    loadModels();
    loadWishlist();
    loadRelations();
    loadOtherAgents();
    loadMemConfig();
    loadCron();
    loadAgentChannels();
    loadEnvVars();
    loadHeartbeat();
    loadToolPolicy();
    await loadSidebarItems();
    // Handle ?tab=<name> query param (e.g. from CronView "查看" button)
    const tabParam = route.query.tab;
    if (tabParam)
        activeTab.value = tabParam;
    // Handle ?resumeSession=<id> query param (from ChatsView 继续对话 button)
    const resumeId = route.query.resumeSession;
    const savedSessionId = !resumeId ? localStorage.getItem(`zyhive_session_${agentId}`) : null;
    const sessionToLoad = resumeId || savedSessionId || null;
    if (sessionToLoad) {
        const panelItem = allSidebarItems.value.find(item => item.type === 'panel' && item.id === sessionToLoad);
        if (panelItem) {
            await selectSidebarItem(panelItem);
        }
        else {
            // Session not in list yet (new), still resume it
            activeSessionId.value = sessionToLoad;
            viewMode.value = 'chat';
            await new Promise(r => setTimeout(r, 100));
            aiChatRef.value?.resumeSession(sessionToLoad);
        }
    }
});
// Identity files
async function loadIdentityFiles() {
    try {
        const [id, soul] = await Promise.all([
            filesApi.read(agentId, 'IDENTITY.md'),
            filesApi.read(agentId, 'SOUL.md'),
        ]);
        identityContent.value = id.data?.content || '';
        soulContent.value = soul.data?.content || '';
    }
    catch { }
    // user-profile.md 可能不存在，单独 try 不阻塞 identity/soul 加载
    try {
        const up = await filesApi.read(agentId, 'memory/core/user-profile.md');
        userProfileContent.value = up.data?.content || '';
    }
    catch {
        userProfileContent.value = '';
    }
    loadMemoryTree();
}
async function saveFile(name, content) {
    try {
        await filesApi.write(agentId, name, content);
        ElMessage.success(`${name} 已保存`);
    }
    catch {
        ElMessage.error(`保存 ${name} 失败`);
    }
}
// 保存用户档案到 memory/core/user-profile.md
// 空白 = 不创建文件（saveFile 空串也会写空文件，这里不额外优化，保持一致）
async function saveUserProfile() {
    try {
        await filesApi.write(agentId, 'memory/core/user-profile.md', userProfileContent.value || '');
        // 空白保存不弹提示（用户可能只是 blur 聚焦切换）
        if (userProfileContent.value) {
            ElMessage.success('用户档案已保存');
        }
    }
    catch {
        ElMessage.error('保存用户档案失败');
    }
}
async function loadModels() {
    try {
        const res = await modelsApi.list();
        // 过滤掉 provider API Key 已测试失败的模型
        modelList.value = (res.data || []).filter((m) => m.providerStatus !== 'error');
        if (agent.value?.modelId) {
            agentModelId.value = agent.value.modelId;
        }
        else {
            const matched = modelList.value.find(m => m.provider + '/' + m.model === agent.value?.model || m.id === agent.value?.model);
            agentModelId.value = matched?.id || '';
        }
    }
    catch {
        modelList.value = [];
    }
    finally {
        modelsLoaded.value = true;
    }
}
// 当前 agent 绑定的模型是否指向了 error 状态的 provider（用于在对话页顶部显示警告条）
const currentModelUnavailable = computed(() => {
    if (!agent.value?.modelId)
        return null;
    // modelList 已经过滤掉了 error provider，所以 modelList 找不到 = error
    const inList = modelList.value.find(m => m.id === agent.value.modelId);
    if (inList)
        return null;
    // 检查是否只是"没有模型配置"（未设置 modelId）—— 这种 AiChat 已有 no-model 引导
    if (modelList.value.length === 0)
        return null;
    return {
        modelId: agent.value.modelId,
        reason: '当前成员绑定的 AI 模型 API Key 已失效，请在「模型配置」页重新测试或更换 Key 后再继续对话',
    };
});
async function saveAgentModel() {
    if (!agentModelId.value)
        return;
    agentModelSaving.value = true;
    try {
        const res = await agentsApi.update(agentId, { modelId: agentModelId.value });
        agent.value = res.data;
        ElMessage.success('模型已更新');
    }
    catch {
        ElMessage.error('更新失败');
    }
    finally {
        agentModelSaving.value = false;
    }
}
// Memory tree functions
async function loadMemoryTree() {
    try {
        const res = await memoryApi.tree(agentId);
        memoryTreeData.value = res.data || [];
    }
    catch {
        memoryTreeData.value = [];
    }
}
async function handleMemoryNodeClick(data) {
    if (data.isDir)
        return;
    memoryEditPath.value = data.path;
    memoryFileBreadcrumb.value = data.path.split('/');
    try {
        const res = await memoryApi.readFile(agentId, data.path);
        memoryEditContent.value = res.data?.content || '';
    }
    catch {
        memoryEditContent.value = '(无法读取)';
    }
}
async function saveMemoryFile() {
    if (!memoryEditPath.value)
        return;
    memorySaving.value = true;
    try {
        await memoryApi.writeFile(agentId, memoryEditPath.value, memoryEditContent.value);
        ElMessage.success('记忆文件已保存');
        loadMemoryTree();
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        memorySaving.value = false;
    }
}
async function createMemoryFile() {
    const p = newMemoryPath.value.trim();
    if (!p) {
        ElMessage.warning('请输入路径');
        return;
    }
    try {
        await memoryApi.writeFile(agentId, p, `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`);
        ElMessage.success('文件已创建');
        showNewMemoryFile.value = false;
        newMemoryPath.value = '';
        loadMemoryTree();
        // Open the new file
        memoryEditPath.value = p;
        memoryFileBreadcrumb.value = p.split('/');
        memoryEditContent.value = `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`;
    }
    catch {
        ElMessage.error('创建失败');
    }
}
async function submitDailyEntry() {
    const content = dailyEntryContent.value.trim();
    if (!content) {
        ElMessage.warning('请输入内容');
        return;
    }
    try {
        await memoryApi.dailyLog(agentId, content);
        ElMessage.success('日志已添加');
        showDailyEntry.value = false;
        dailyEntryContent.value = '';
        loadMemoryTree();
    }
    catch {
        ElMessage.error('添加失败');
    }
}
// Cron
async function loadCron() {
    try {
        // Only load this agent's own cron jobs
        const res = await cronApi.list(agentId);
        cronJobs.value = res.data || [];
    }
    catch { }
}
async function createCron() {
    try {
        await cronApi.create({
            name: cronForm.value.name,
            enabled: cronForm.value.enabled,
            agentId: agentId, // bind to this agent
            schedule: { kind: 'cron', expr: cronForm.value.expr, tz: cronForm.value.tz },
            payload: { kind: 'agentTurn', message: cronForm.value.message },
            delivery: { mode: 'announce' },
        });
        ElMessage.success('任务创建成功');
        showCronCreate.value = false;
        loadCron();
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
}
async function toggleCron(job) {
    try {
        await cronApi.update(job.id, job);
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function runCronNow(job) {
    try {
        await cronApi.run(job.id);
        ElMessage.success('已触发运行');
        setTimeout(loadCron, 2000);
    }
    catch {
        ElMessage.error('运行失败');
    }
}
async function deleteCron(job) {
    try {
        await cronApi.delete(job.id);
        ElMessage.success('已删除');
        loadCron();
    }
    catch {
        ElMessage.error('删除失败');
    }
}
async function openCronLogs(job) {
    cronLogsJob.value = job;
    showCronLogs.value = true;
    loadingCronLogs.value = true;
    try {
        const res = await cronApi.runs(job.id);
        cronLogs.value = (res.data || []).slice().reverse();
    }
    catch {
        ElMessage.error('获取日志失败');
        cronLogs.value = [];
    }
    finally {
        loadingCronLogs.value = false;
    }
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['wish-item']} */ ;
/** @type {__VLS_StyleScopedClasses['th-provider-health']} */ ;
/** @type {__VLS_StyleScopedClasses['th-provider-health']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-detail']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-detail']} */ ;
/** @type {__VLS_StyleScopedClasses['el-card']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-detail']} */ ;
/** @type {__VLS_StyleScopedClasses['header-left']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['user']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-msg']} */ ;
/** @type {__VLS_StyleScopedClasses['tool']} */ ;
/** @type {__VLS_StyleScopedClasses['msg-bubble']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-indicator']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-indicator']} */ ;
/** @type {__VLS_StyleScopedClasses['typing-indicator']} */ ;
/** @type {__VLS_StyleScopedClasses['memory-card']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item']} */ ;
/** @type {__VLS_StyleScopedClasses['active']} */ ;
/** @type {__VLS_StyleScopedClasses['session-item-title']} */ ;
/** @type {__VLS_StyleScopedClasses['at-toggle-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['pending-section-header']} */ ;
/** @type {__VLS_StyleScopedClasses['pending-user-row']} */ ;
/** @type {__VLS_StyleScopedClasses['conv-msg-user']} */ ;
/** @type {__VLS_StyleScopedClasses['conv-msg-content']} */ ;
/** @type {__VLS_StyleScopedClasses['conv-msg-assistant']} */ ;
/** @type {__VLS_StyleScopedClasses['conv-msg-content']} */ ;
/** @type {__VLS_StyleScopedClasses['detail-header']} */ ;
/** @type {__VLS_StyleScopedClasses['header-left']} */ ;
/** @type {__VLS_StyleScopedClasses['detail-title']} */ ;
/** @type {__VLS_StyleScopedClasses['detail-model-mobile']} */ ;
/** @type {__VLS_StyleScopedClasses['detail-model-desktop']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__nav-scroll']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__nav-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__header']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__nav-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__item']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__item']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tabs__item']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tab-pane']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['el-tab-pane']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
/** @type {__VLS_StyleScopedClasses['agent-detail']} */ ;
/** @type {__VLS_StyleScopedClasses['el-main']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-layout']} */ ;
/** @type {__VLS_StyleScopedClasses['mobile-session-toggle']} */ ;
/** @type {__VLS_StyleScopedClasses['mobile-session-toggle']} */ ;
/** @type {__VLS_StyleScopedClasses['session-sidebar']} */ ;
/** @type {__VLS_StyleScopedClasses['session-sidebar']} */ ;
/** @type {__VLS_StyleScopedClasses['chat-area']} */ ;
/** @type {__VLS_StyleScopedClasses['channel-card-header']} */ ;
/** @type {__VLS_StyleScopedClasses['channel-card-actions']} */ ;
/** @type {__VLS_StyleScopedClasses['channel-card-actions']} */ ;
/** @type {__VLS_StyleScopedClasses['channel-info-row']} */ ;
/** @type {__VLS_StyleScopedClasses['channel-info-value']} */ ;
/** @type {__VLS_StyleScopedClasses['el-table']} */ ;
/** @type {__VLS_StyleScopedClasses['el-table']} */ ;
/** @type {__VLS_StyleScopedClasses['env-add-row']} */ ;
/** @type {__VLS_StyleScopedClasses['el-input']} */ ;
/** @type {__VLS_StyleScopedClasses['el-card']} */ ;
/** @type {__VLS_StyleScopedClasses['el-card__header']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container'] | typeof __VLS_components.elContainer | typeof __VLS_components.ElContainer | typeof __VLS_components['el-container']} */
elContainer;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ class: "agent-detail" },
}));
const __VLS_2 = __VLS_1({
    ...{ class: "agent-detail" },
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
var __VLS_5 = {};
/** @type {__VLS_StyleScopedClasses['agent-detail']} */ ;
const { default: __VLS_6 } = __VLS_3.slots;
let __VLS_7;
/** @ts-ignore @type { | typeof __VLS_components.elHeader | typeof __VLS_components.ElHeader | typeof __VLS_components['el-header'] | typeof __VLS_components.elHeader | typeof __VLS_components.ElHeader | typeof __VLS_components['el-header']} */
elHeader;
// @ts-ignore
const __VLS_8 = __VLS_asFunctionalComponent1(__VLS_7, new __VLS_7({
    ...{ class: "detail-header" },
}));
const __VLS_9 = __VLS_8({
    ...{ class: "detail-header" },
}, ...__VLS_functionalComponentArgsRest(__VLS_8));
/** @type {__VLS_StyleScopedClasses['detail-header']} */ ;
const { default: __VLS_12 } = __VLS_10.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "header-left" },
});
/** @type {__VLS_StyleScopedClasses['header-left']} */ ;
let __VLS_13;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_14 = __VLS_asFunctionalComponent1(__VLS_13, new __VLS_13({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.ArrowLeft),
    circle: true,
    size: "small",
}));
const __VLS_15 = __VLS_14({
    ...{ 'onClick': {} },
    icon: (__VLS_ctx.ArrowLeft),
    circle: true,
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_14));
let __VLS_18;
const __VLS_19 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.$router.push('/agents');
            // @ts-ignore
            [ArrowLeft, $router,];
        } });
var __VLS_16;
var __VLS_17;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "detail-title-block" },
});
/** @type {__VLS_StyleScopedClasses['detail-title-block']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.h2, __VLS_intrinsics.h2)({
    ...{ class: "detail-title" },
});
/** @type {__VLS_StyleScopedClasses['detail-title']} */ ;
(__VLS_ctx.agent?.name || '...');
let __VLS_20;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_21 = __VLS_asFunctionalComponent1(__VLS_20, new __VLS_20({
    type: "info",
    ...{ class: "detail-model-mobile" },
}));
const __VLS_22 = __VLS_21({
    type: "info",
    ...{ class: "detail-model-mobile" },
}, ...__VLS_functionalComponentArgsRest(__VLS_21));
/** @type {__VLS_StyleScopedClasses['detail-model-mobile']} */ ;
const { default: __VLS_25 } = __VLS_23.slots;
(__VLS_ctx.agent?.model);
// @ts-ignore
[agent, agent,];
var __VLS_23;
let __VLS_26;
/** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
elTag;
// @ts-ignore
const __VLS_27 = __VLS_asFunctionalComponent1(__VLS_26, new __VLS_26({
    type: (__VLS_ctx.statusType(__VLS_ctx.agent?.status)),
    size: "small",
}));
const __VLS_28 = __VLS_27({
    type: (__VLS_ctx.statusType(__VLS_ctx.agent?.status)),
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_27));
const { default: __VLS_31 } = __VLS_29.slots;
(__VLS_ctx.statusLabel(__VLS_ctx.agent?.status));
// @ts-ignore
[agent, agent, statusType, statusLabel,];
var __VLS_29;
let __VLS_32;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_33 = __VLS_asFunctionalComponent1(__VLS_32, new __VLS_32({
    type: "info",
    ...{ class: "detail-model-desktop" },
}));
const __VLS_34 = __VLS_33({
    type: "info",
    ...{ class: "detail-model-desktop" },
}, ...__VLS_functionalComponentArgsRest(__VLS_33));
/** @type {__VLS_StyleScopedClasses['detail-model-desktop']} */ ;
const { default: __VLS_37 } = __VLS_35.slots;
(__VLS_ctx.agent?.model);
// @ts-ignore
[agent,];
var __VLS_35;
// @ts-ignore
[];
var __VLS_10;
let __VLS_38;
/** @ts-ignore @type { | typeof __VLS_components.elMain | typeof __VLS_components.ElMain | typeof __VLS_components['el-main'] | typeof __VLS_components.elMain | typeof __VLS_components.ElMain | typeof __VLS_components['el-main']} */
elMain;
// @ts-ignore
const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({}));
const __VLS_40 = __VLS_39({}, ...__VLS_functionalComponentArgsRest(__VLS_39));
const { default: __VLS_43 } = __VLS_41.slots;
let __VLS_44;
/** @ts-ignore @type { | typeof __VLS_components.elTabs | typeof __VLS_components.ElTabs | typeof __VLS_components['el-tabs'] | typeof __VLS_components.elTabs | typeof __VLS_components.ElTabs | typeof __VLS_components['el-tabs']} */
elTabs;
// @ts-ignore
const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({
    modelValue: (__VLS_ctx.activeTab),
    ...{ class: "agent-tabs" },
}));
const __VLS_46 = __VLS_45({
    modelValue: (__VLS_ctx.activeTab),
    ...{ class: "agent-tabs" },
}, ...__VLS_functionalComponentArgsRest(__VLS_45));
/** @type {__VLS_StyleScopedClasses['agent-tabs']} */ ;
const { default: __VLS_49 } = __VLS_47.slots;
let __VLS_50;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_51 = __VLS_asFunctionalComponent1(__VLS_50, new __VLS_50({
    label: "对话",
    name: "chat",
}));
const __VLS_52 = __VLS_51({
    label: "对话",
    name: "chat",
}, ...__VLS_functionalComponentArgsRest(__VLS_51));
const { default: __VLS_55 } = __VLS_53.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-layout" },
});
/** @type {__VLS_StyleScopedClasses['chat-layout']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.mobileSessionOpen = !__VLS_ctx.mobileSessionOpen;
            // @ts-ignore
            [activeTab, mobileSessionOpen, mobileSessionOpen,];
        } },
    ...{ class: "mobile-session-toggle" },
});
/** @type {__VLS_StyleScopedClasses['mobile-session-toggle']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "14",
    height: "14",
    viewBox: "0 0 24 24",
    fill: "currentColor",
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.path)({
    d: "M3 18h18v-2H3v2zm0-5h18v-2H3v2zm0-7v2h18V6H3z",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "session-count-badge" },
});
/** @type {__VLS_StyleScopedClasses['session-count-badge']} */ ;
(__VLS_ctx.agentSessions.length);
__VLS_asFunctionalElement1(__VLS_intrinsics.svg, __VLS_intrinsics.svg)({
    width: "12",
    height: "12",
    viewBox: "0 0 24 24",
    fill: "currentColor",
    ...{ style: {} },
    ...{ style: ({ transform: __VLS_ctx.mobileSessionOpen ? 'rotate(180deg)' : '' }) },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.path)({
    d: "M7 10l5 5 5-5z",
});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "session-sidebar" },
    ...{ class: ({ 'mobile-session-open': __VLS_ctx.mobileSessionOpen }) },
});
/** @type {__VLS_StyleScopedClasses['session-sidebar']} */ ;
/** @type {__VLS_StyleScopedClasses['mobile-session-open']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "session-sidebar-header" },
});
/** @type {__VLS_StyleScopedClasses['session-sidebar-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sidebar-title" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-title']} */ ;
let __VLS_56;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_57 = __VLS_asFunctionalComponent1(__VLS_56, new __VLS_56({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    plain: true,
    icon: (__VLS_ctx.Plus),
}));
const __VLS_58 = __VLS_57({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    plain: true,
    icon: (__VLS_ctx.Plus),
}, ...__VLS_functionalComponentArgsRest(__VLS_57));
let __VLS_61;
const __VLS_62 = ({ click: {} },
    { onClick: (__VLS_ctx.newSession) });
const { default: __VLS_63 } = __VLS_59.slots;
// @ts-ignore
[mobileSessionOpen, mobileSessionOpen, agentSessions, Plus, newSession,];
var __VLS_59;
var __VLS_60;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "session-list" },
});
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.sessionsLoading) }, null, null);
/** @type {__VLS_StyleScopedClasses['session-list']} */ ;
for (const [item] of __VLS_vFor((__VLS_ctx.allSidebarItems))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectSidebarItem(item);
                // @ts-ignore
                [vLoading, sessionsLoading, allSidebarItems, selectSidebarItem,];
            } },
        key: (item.type + ':' + item.id),
        ...{ class: (['session-item', { active: __VLS_ctx.isSelectedItem(item) }]) },
        ...{ style: ({ '--src-color': __VLS_ctx.sourceColor(item.source) }) },
        title: (__VLS_ctx.sourceTag(item.source).label + ' · ' + item.label),
    });
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    /** @type {__VLS_StyleScopedClasses['session-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "session-item-title" },
    });
    /** @type {__VLS_StyleScopedClasses['session-item-title']} */ ;
    (item.label);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "session-item-meta" },
    });
    /** @type {__VLS_StyleScopedClasses['session-item-meta']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "session-src-dot" },
        ...{ style: ({ background: __VLS_ctx.sourceColor(item.source) }) },
    });
    /** @type {__VLS_StyleScopedClasses['session-src-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-item-src-text" },
    });
    /** @type {__VLS_StyleScopedClasses['session-item-src-text']} */ ;
    (__VLS_ctx.sourceTag(item.source).label);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-meta-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['session-meta-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (item.messageCount);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-meta-dot" },
    });
    /** @type {__VLS_StyleScopedClasses['session-meta-dot']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "session-item-time" },
    });
    /** @type {__VLS_StyleScopedClasses['session-item-time']} */ ;
    (__VLS_ctx.formatRelative(item.lastAt));
    // @ts-ignore
    [isSelectedItem, sourceColor, sourceColor, sourceTag, sourceTag, formatRelative,];
}
if (!__VLS_ctx.sessionsLoading && !__VLS_ctx.allSidebarItems.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "session-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['session-empty']} */ ;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "at-panel" },
});
/** @type {__VLS_StyleScopedClasses['at-panel']} */ ;
let __VLS_64;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
    ...{ class: "at-toggle-btn" },
}));
const __VLS_66 = __VLS_65({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
    ...{ class: "at-toggle-btn" },
}, ...__VLS_functionalComponentArgsRest(__VLS_65));
let __VLS_69;
const __VLS_70 = ({ click: {} },
    { onClick: (__VLS_ctx.toggleAtPanel) });
/** @type {__VLS_StyleScopedClasses['at-toggle-btn']} */ ;
const { default: __VLS_71 } = __VLS_67.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "at-icon" },
});
/** @type {__VLS_StyleScopedClasses['at-icon']} */ ;
// @ts-ignore
[sessionsLoading, allSidebarItems, toggleAtPanel,];
var __VLS_67;
var __VLS_68;
if (__VLS_ctx.showAtPanel) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "at-form" },
    });
    /** @type {__VLS_StyleScopedClasses['at-form']} */ ;
    let __VLS_72;
    /** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
    elSelect;
    // @ts-ignore
    const __VLS_73 = __VLS_asFunctionalComponent1(__VLS_72, new __VLS_72({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.atTargetId),
        placeholder: "选择成员",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_74 = __VLS_73({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.atTargetId),
        placeholder: "选择成员",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_73));
    let __VLS_77;
    const __VLS_78 = ({ change: {} },
        { onChange: (__VLS_ctx.onAtAgentSelect) });
    const { default: __VLS_79 } = __VLS_75.slots;
    for (const [a] of __VLS_vFor((__VLS_ctx.otherAgents))) {
        let __VLS_80;
        /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
        elOption;
        // @ts-ignore
        const __VLS_81 = __VLS_asFunctionalComponent1(__VLS_80, new __VLS_80({
            key: (a.id),
            label: (a.name),
            value: (a.id),
        }));
        const __VLS_82 = __VLS_81({
            key: (a.id),
            label: (a.name),
            value: (a.id),
        }, ...__VLS_functionalComponentArgsRest(__VLS_81));
        // @ts-ignore
        [showAtPanel, atTargetId, onAtAgentSelect, otherAgents,];
    }
    // @ts-ignore
    [];
    var __VLS_75;
    var __VLS_76;
    let __VLS_85;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_86 = __VLS_asFunctionalComponent1(__VLS_85, new __VLS_85({
        modelValue: (__VLS_ctx.atMessage),
        type: "textarea",
        rows: (3),
        placeholder: "输入要转发的消息…",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_87 = __VLS_86({
        modelValue: (__VLS_ctx.atMessage),
        type: "textarea",
        rows: (3),
        placeholder: "输入要转发的消息…",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_86));
    let __VLS_90;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_91 = __VLS_asFunctionalComponent1(__VLS_90, new __VLS_90({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
        ...{ style: {} },
        loading: (__VLS_ctx.atSending),
        disabled: (!__VLS_ctx.atTargetId || !__VLS_ctx.atMessage.trim()),
    }));
    const __VLS_92 = __VLS_91({
        ...{ 'onClick': {} },
        type: "primary",
        size: "small",
        ...{ style: {} },
        loading: (__VLS_ctx.atSending),
        disabled: (!__VLS_ctx.atTargetId || !__VLS_ctx.atMessage.trim()),
    }, ...__VLS_functionalComponentArgsRest(__VLS_91));
    let __VLS_95;
    const __VLS_96 = ({ click: {} },
        { onClick: (__VLS_ctx.sendAtMessage) });
    const { default: __VLS_97 } = __VLS_93.slots;
    // @ts-ignore
    [atTargetId, atMessage, atMessage, atSending, sendAtMessage,];
    var __VLS_93;
    var __VLS_94;
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-area" },
});
/** @type {__VLS_StyleScopedClasses['chat-area']} */ ;
const __VLS_98 = AiChat;
// @ts-ignore
const __VLS_99 = __VLS_asFunctionalComponent1(__VLS_98, new __VLS_98({
    ...{ 'onSessionChange': {} },
    ref: "aiChatRef",
    agentId: (__VLS_ctx.agentId),
    scenario: ('agent-detail'),
    welcomeMessage: (`你好！我是 **${__VLS_ctx.agent?.name || 'AI'}**，有什么可以帮你的？`),
    height: "calc(100vh - 145px)",
    showThinking: (true),
    noModel: (__VLS_ctx.modelsLoaded && __VLS_ctx.modelList.length === 0),
    readOnly: (__VLS_ctx.isReadOnlySession),
    readOnlyReason: (__VLS_ctx.readOnlyReason),
    modelUnavailable: (__VLS_ctx.currentModelUnavailable?.reason),
}));
const __VLS_100 = __VLS_99({
    ...{ 'onSessionChange': {} },
    ref: "aiChatRef",
    agentId: (__VLS_ctx.agentId),
    scenario: ('agent-detail'),
    welcomeMessage: (`你好！我是 **${__VLS_ctx.agent?.name || 'AI'}**，有什么可以帮你的？`),
    height: "calc(100vh - 145px)",
    showThinking: (true),
    noModel: (__VLS_ctx.modelsLoaded && __VLS_ctx.modelList.length === 0),
    readOnly: (__VLS_ctx.isReadOnlySession),
    readOnlyReason: (__VLS_ctx.readOnlyReason),
    modelUnavailable: (__VLS_ctx.currentModelUnavailable?.reason),
}, ...__VLS_functionalComponentArgsRest(__VLS_99));
let __VLS_103;
const __VLS_104 = ({ sessionChange: {} },
    { onSessionChange: (__VLS_ctx.onSessionChange) });
var __VLS_105 = {};
var __VLS_101;
var __VLS_102;
if (__VLS_ctx.historyLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "history-loading-overlay" },
    });
    /** @type {__VLS_StyleScopedClasses['history-loading-overlay']} */ ;
    let __VLS_107;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_108 = __VLS_asFunctionalComponent1(__VLS_107, new __VLS_107({
        ...{ class: "is-loading" },
        size: (22),
        color: "#94a3b8",
    }));
    const __VLS_109 = __VLS_108({
        ...{ class: "is-loading" },
        size: (22),
        color: "#94a3b8",
    }, ...__VLS_functionalComponentArgsRest(__VLS_108));
    /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
    const { default: __VLS_112 } = __VLS_110.slots;
    let __VLS_113;
    /** @ts-ignore @type { | typeof __VLS_components.Loading} */
    Loading;
    // @ts-ignore
    const __VLS_114 = __VLS_asFunctionalComponent1(__VLS_113, new __VLS_113({}));
    const __VLS_115 = __VLS_114({}, ...__VLS_functionalComponentArgsRest(__VLS_114));
    // @ts-ignore
    [agent, agentId, modelsLoaded, modelList, isReadOnlySession, readOnlyReason, currentModelUnavailable, onSessionChange, historyLoading,];
    var __VLS_110;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
}
// @ts-ignore
[];
var __VLS_53;
let __VLS_118;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_119 = __VLS_asFunctionalComponent1(__VLS_118, new __VLS_118({
    label: "工作区",
    name: "workspace",
}));
const __VLS_120 = __VLS_119({
    label: "工作区",
    name: "workspace",
}, ...__VLS_functionalComponentArgsRest(__VLS_119));
const { default: __VLS_123 } = __VLS_121.slots;
const __VLS_124 = WorkspaceChatLayout;
// @ts-ignore
const __VLS_125 = __VLS_asFunctionalComponent1(__VLS_124, new __VLS_124({
    agentId: (__VLS_ctx.agentId),
    ...{ style: {} },
}));
const __VLS_126 = __VLS_125({
    agentId: (__VLS_ctx.agentId),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_125));
// @ts-ignore
[agentId,];
var __VLS_121;
let __VLS_129;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_130 = __VLS_asFunctionalComponent1(__VLS_129, new __VLS_129({
    label: "身份 & 灵魂",
    name: "identity",
}));
const __VLS_131 = __VLS_130({
    label: "身份 & 灵魂",
    name: "identity",
}, ...__VLS_functionalComponentArgsRest(__VLS_130));
const { default: __VLS_134 } = __VLS_132.slots;
let __VLS_135;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_136 = __VLS_asFunctionalComponent1(__VLS_135, new __VLS_135({
    ...{ style: {} },
}));
const __VLS_137 = __VLS_136({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_136));
const { default: __VLS_140 } = __VLS_138.slots;
{
    const { header: __VLS_141 } = __VLS_138.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    // @ts-ignore
    [];
}
let __VLS_142;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_143 = __VLS_asFunctionalComponent1(__VLS_142, new __VLS_142({
    labelWidth: "80px",
    size: "default",
}));
const __VLS_144 = __VLS_143({
    labelWidth: "80px",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_143));
const { default: __VLS_147 } = __VLS_145.slots;
let __VLS_148;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_149 = __VLS_asFunctionalComponent1(__VLS_148, new __VLS_148({
    label: "使用模型",
}));
const __VLS_150 = __VLS_149({
    label: "使用模型",
}, ...__VLS_functionalComponentArgsRest(__VLS_149));
const { default: __VLS_153 } = __VLS_151.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_154;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_155 = __VLS_asFunctionalComponent1(__VLS_154, new __VLS_154({
    modelValue: (__VLS_ctx.agentModelId),
    placeholder: "选择模型",
    ...{ style: {} },
}));
const __VLS_156 = __VLS_155({
    modelValue: (__VLS_ctx.agentModelId),
    placeholder: "选择模型",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_155));
const { default: __VLS_159 } = __VLS_157.slots;
for (const [m] of __VLS_vFor((__VLS_ctx.modelList))) {
    let __VLS_160;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_161 = __VLS_asFunctionalComponent1(__VLS_160, new __VLS_160({
        key: (m.id),
        label: (m.name || m.model),
        value: (m.id),
    }));
    const __VLS_162 = __VLS_161({
        key: (m.id),
        label: (m.name || m.model),
        value: (m.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_161));
    const { default: __VLS_165 } = __VLS_163.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: (m.supportsTools === false ? 'color:#999' : '') },
    });
    (m.name || m.model);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    if (m.supportsTools === false) {
        let __VLS_166;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_167 = __VLS_asFunctionalComponent1(__VLS_166, new __VLS_166({
            size: "small",
            type: "warning",
            ...{ style: {} },
        }));
        const __VLS_168 = __VLS_167({
            size: "small",
            type: "warning",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_167));
        const { default: __VLS_171 } = __VLS_169.slots;
        // @ts-ignore
        [modelList, agentModelId,];
        var __VLS_169;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (m.provider);
    // @ts-ignore
    [];
    var __VLS_163;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_157;
let __VLS_172;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_173 = __VLS_asFunctionalComponent1(__VLS_172, new __VLS_172({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.agentModelSaving),
    disabled: (!__VLS_ctx.agentModelId),
}));
const __VLS_174 = __VLS_173({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.agentModelSaving),
    disabled: (!__VLS_ctx.agentModelId),
}, ...__VLS_functionalComponentArgsRest(__VLS_173));
let __VLS_177;
const __VLS_178 = ({ click: {} },
    { onClick: (__VLS_ctx.saveAgentModel) });
const { default: __VLS_179 } = __VLS_175.slots;
// @ts-ignore
[agentModelId, agentModelSaving, saveAgentModel,];
var __VLS_175;
var __VLS_176;
if (__VLS_ctx.selectedModelNoTools) {
    let __VLS_180;
    /** @ts-ignore @type { | typeof __VLS_components.elAlert | typeof __VLS_components.ElAlert | typeof __VLS_components['el-alert'] | typeof __VLS_components.elAlert | typeof __VLS_components.ElAlert | typeof __VLS_components['el-alert']} */
    elAlert;
    // @ts-ignore
    const __VLS_181 = __VLS_asFunctionalComponent1(__VLS_180, new __VLS_180({
        type: "warning",
        closable: (false),
        showIcon: true,
        ...{ style: {} },
    }));
    const __VLS_182 = __VLS_181({
        type: "warning",
        closable: (false),
        showIcon: true,
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_181));
    const { default: __VLS_185 } = __VLS_183.slots;
    {
        const { title: __VLS_186 } = __VLS_183.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
        // @ts-ignore
        [selectedModelNoTools,];
    }
    // @ts-ignore
    [];
    var __VLS_183;
}
// @ts-ignore
[];
var __VLS_151;
// @ts-ignore
[];
var __VLS_145;
// @ts-ignore
[];
var __VLS_138;
let __VLS_187;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_188 = __VLS_asFunctionalComponent1(__VLS_187, new __VLS_187({
    gutter: (20),
}));
const __VLS_189 = __VLS_188({
    gutter: (20),
}, ...__VLS_functionalComponentArgsRest(__VLS_188));
const { default: __VLS_192 } = __VLS_190.slots;
let __VLS_193;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_194 = __VLS_asFunctionalComponent1(__VLS_193, new __VLS_193({
    xs: (24),
    sm: (12),
}));
const __VLS_195 = __VLS_194({
    xs: (24),
    sm: (12),
}, ...__VLS_functionalComponentArgsRest(__VLS_194));
const { default: __VLS_198 } = __VLS_196.slots;
let __VLS_199;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_200 = __VLS_asFunctionalComponent1(__VLS_199, new __VLS_199({
    header: "IDENTITY.md",
    ...{ style: {} },
}));
const __VLS_201 = __VLS_200({
    header: "IDENTITY.md",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_200));
const { default: __VLS_204 } = __VLS_202.slots;
let __VLS_205;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_206 = __VLS_asFunctionalComponent1(__VLS_205, new __VLS_205({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.identityContent),
    type: "textarea",
    rows: (15),
}));
const __VLS_207 = __VLS_206({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.identityContent),
    type: "textarea",
    rows: (15),
}, ...__VLS_functionalComponentArgsRest(__VLS_206));
let __VLS_210;
const __VLS_211 = ({ blur: {} },
    { onBlur: (...[$event]) => {
            __VLS_ctx.saveFile('IDENTITY.md', __VLS_ctx.identityContent);
            // @ts-ignore
            [identityContent, identityContent, saveFile,];
        } });
var __VLS_208;
var __VLS_209;
// @ts-ignore
[];
var __VLS_202;
// @ts-ignore
[];
var __VLS_196;
let __VLS_212;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_213 = __VLS_asFunctionalComponent1(__VLS_212, new __VLS_212({
    xs: (24),
    sm: (12),
}));
const __VLS_214 = __VLS_213({
    xs: (24),
    sm: (12),
}, ...__VLS_functionalComponentArgsRest(__VLS_213));
const { default: __VLS_217 } = __VLS_215.slots;
let __VLS_218;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_219 = __VLS_asFunctionalComponent1(__VLS_218, new __VLS_218({
    header: "SOUL.md",
}));
const __VLS_220 = __VLS_219({
    header: "SOUL.md",
}, ...__VLS_functionalComponentArgsRest(__VLS_219));
const { default: __VLS_223 } = __VLS_221.slots;
let __VLS_224;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_225 = __VLS_asFunctionalComponent1(__VLS_224, new __VLS_224({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.soulContent),
    type: "textarea",
    rows: (15),
}));
const __VLS_226 = __VLS_225({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.soulContent),
    type: "textarea",
    rows: (15),
}, ...__VLS_functionalComponentArgsRest(__VLS_225));
let __VLS_229;
const __VLS_230 = ({ blur: {} },
    { onBlur: (...[$event]) => {
            __VLS_ctx.saveFile('SOUL.md', __VLS_ctx.soulContent);
            // @ts-ignore
            [saveFile, soulContent, soulContent,];
        } });
var __VLS_227;
var __VLS_228;
// @ts-ignore
[];
var __VLS_221;
// @ts-ignore
[];
var __VLS_215;
// @ts-ignore
[];
var __VLS_190;
let __VLS_231;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_232 = __VLS_asFunctionalComponent1(__VLS_231, new __VLS_231({
    ...{ style: {} },
}));
const __VLS_233 = __VLS_232({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_232));
const { default: __VLS_236 } = __VLS_234.slots;
{
    const { header: __VLS_237 } = __VLS_234.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_238;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_239 = __VLS_asFunctionalComponent1(__VLS_238, new __VLS_238({
        size: "small",
        type: "info",
    }));
    const __VLS_240 = __VLS_239({
        size: "small",
        type: "info",
    }, ...__VLS_functionalComponentArgsRest(__VLS_239));
    const { default: __VLS_243 } = __VLS_241.slots;
    // @ts-ignore
    [];
    var __VLS_241;
    // @ts-ignore
    [];
}
let __VLS_244;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_245 = __VLS_asFunctionalComponent1(__VLS_244, new __VLS_244({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.userProfileContent),
    type: "textarea",
    rows: (14),
    placeholder: (__VLS_ctx.userProfilePlaceholder),
}));
const __VLS_246 = __VLS_245({
    ...{ 'onBlur': {} },
    modelValue: (__VLS_ctx.userProfileContent),
    type: "textarea",
    rows: (14),
    placeholder: (__VLS_ctx.userProfilePlaceholder),
}, ...__VLS_functionalComponentArgsRest(__VLS_245));
let __VLS_249;
const __VLS_250 = ({ blur: {} },
    { onBlur: (__VLS_ctx.saveUserProfile) });
var __VLS_247;
var __VLS_248;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
// @ts-ignore
[userProfileContent, userProfilePlaceholder, saveUserProfile,];
var __VLS_234;
if (__VLS_ctx.wishlist && __VLS_ctx.wishlist.total > 0) {
    let __VLS_251;
    /** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
    elCard;
    // @ts-ignore
    const __VLS_252 = __VLS_asFunctionalComponent1(__VLS_251, new __VLS_251({
        ...{ style: {} },
    }));
    const __VLS_253 = __VLS_252({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_252));
    const { default: __VLS_256 } = __VLS_254.slots;
    {
        const { header: __VLS_257 } = __VLS_254.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.wishlist.total);
        let __VLS_258;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_259 = __VLS_asFunctionalComponent1(__VLS_258, new __VLS_258({
            ...{ 'onClick': {} },
            size: "small",
            loading: (__VLS_ctx.wishlistLoading),
        }));
        const __VLS_260 = __VLS_259({
            ...{ 'onClick': {} },
            size: "small",
            loading: (__VLS_ctx.wishlistLoading),
        }, ...__VLS_functionalComponentArgsRest(__VLS_259));
        let __VLS_263;
        const __VLS_264 = ({ click: {} },
            { onClick: (__VLS_ctx.loadWishlist) });
        const { default: __VLS_265 } = __VLS_261.slots;
        let __VLS_266;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_267 = __VLS_asFunctionalComponent1(__VLS_266, new __VLS_266({}));
        const __VLS_268 = __VLS_267({}, ...__VLS_functionalComponentArgsRest(__VLS_267));
        const { default: __VLS_271 } = __VLS_269.slots;
        let __VLS_272;
        /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
        Refresh;
        // @ts-ignore
        const __VLS_273 = __VLS_asFunctionalComponent1(__VLS_272, new __VLS_272({}));
        const __VLS_274 = __VLS_273({}, ...__VLS_functionalComponentArgsRest(__VLS_273));
        // @ts-ignore
        [wishlist, wishlist, wishlist, wishlistLoading, loadWishlist,];
        var __VLS_269;
        // @ts-ignore
        [];
        var __VLS_261;
        var __VLS_262;
        // @ts-ignore
        [];
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    for (const [w, idx] of __VLS_vFor((__VLS_ctx.wishlist.wishes))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (idx),
            ...{ class: "wish-item" },
        });
        /** @type {__VLS_StyleScopedClasses['wish-item']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "wish-head" },
        });
        /** @type {__VLS_StyleScopedClasses['wish-head']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "wish-title" },
        });
        /** @type {__VLS_StyleScopedClasses['wish-title']} */ ;
        (w.title);
        if (w.priority) {
            let __VLS_277;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_278 = __VLS_asFunctionalComponent1(__VLS_277, new __VLS_277({
                size: "small",
                type: (__VLS_ctx.wishPriorityType(w.priority)),
                effect: "plain",
                ...{ class: "wish-pri" },
            }));
            const __VLS_279 = __VLS_278({
                size: "small",
                type: (__VLS_ctx.wishPriorityType(w.priority)),
                effect: "plain",
                ...{ class: "wish-pri" },
            }, ...__VLS_functionalComponentArgsRest(__VLS_278));
            /** @type {__VLS_StyleScopedClasses['wish-pri']} */ ;
            const { default: __VLS_282 } = __VLS_280.slots;
            (w.priority);
            // @ts-ignore
            [wishlist, wishPriorityType,];
            var __VLS_280;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "wish-time" },
        });
        /** @type {__VLS_StyleScopedClasses['wish-time']} */ ;
        (w.createdAt);
        if (w.reason) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "wish-reason" },
            });
            /** @type {__VLS_StyleScopedClasses['wish-reason']} */ ;
            (w.reason);
        }
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_254;
}
// @ts-ignore
[];
var __VLS_132;
let __VLS_283;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_284 = __VLS_asFunctionalComponent1(__VLS_283, new __VLS_283({
    label: "关系",
    name: "relations",
}));
const __VLS_285 = __VLS_284({
    label: "关系",
    name: "relations",
}, ...__VLS_functionalComponentArgsRest(__VLS_284));
const { default: __VLS_288 } = __VLS_286.slots;
let __VLS_289;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_290 = __VLS_asFunctionalComponent1(__VLS_289, new __VLS_289({
    gutter: (20),
}));
const __VLS_291 = __VLS_290({
    gutter: (20),
}, ...__VLS_functionalComponentArgsRest(__VLS_290));
const { default: __VLS_294 } = __VLS_292.slots;
let __VLS_295;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_296 = __VLS_asFunctionalComponent1(__VLS_295, new __VLS_295({
    span: (14),
}));
const __VLS_297 = __VLS_296({
    span: (14),
}, ...__VLS_functionalComponentArgsRest(__VLS_296));
const { default: __VLS_300 } = __VLS_298.slots;
let __VLS_301;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_302 = __VLS_asFunctionalComponent1(__VLS_301, new __VLS_301({
    ...{ style: {} },
}));
const __VLS_303 = __VLS_302({
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_302));
const { default: __VLS_306 } = __VLS_304.slots;
{
    const { header: __VLS_307 } = __VLS_304.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    // @ts-ignore
    [];
}
let __VLS_308;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_309 = __VLS_asFunctionalComponent1(__VLS_308, new __VLS_308({
    model: (__VLS_ctx.newRelation),
    labelPosition: "top",
    size: "default",
}));
const __VLS_310 = __VLS_309({
    model: (__VLS_ctx.newRelation),
    labelPosition: "top",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_309));
const { default: __VLS_313 } = __VLS_311.slots;
let __VLS_314;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_315 = __VLS_asFunctionalComponent1(__VLS_314, new __VLS_314({
    gutter: (12),
}));
const __VLS_316 = __VLS_315({
    gutter: (12),
}, ...__VLS_functionalComponentArgsRest(__VLS_315));
const { default: __VLS_319 } = __VLS_317.slots;
let __VLS_320;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_321 = __VLS_asFunctionalComponent1(__VLS_320, new __VLS_320({
    span: (10),
}));
const __VLS_322 = __VLS_321({
    span: (10),
}, ...__VLS_functionalComponentArgsRest(__VLS_321));
const { default: __VLS_325 } = __VLS_323.slots;
let __VLS_326;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_327 = __VLS_asFunctionalComponent1(__VLS_326, new __VLS_326({
    label: "关联成员",
}));
const __VLS_328 = __VLS_327({
    label: "关联成员",
}, ...__VLS_functionalComponentArgsRest(__VLS_327));
const { default: __VLS_331 } = __VLS_329.slots;
let __VLS_332;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_333 = __VLS_asFunctionalComponent1(__VLS_332, new __VLS_332({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.newRelation.agentId),
    placeholder: "选择系统成员",
    filterable: true,
    ...{ style: {} },
}));
const __VLS_334 = __VLS_333({
    ...{ 'onChange': {} },
    modelValue: (__VLS_ctx.newRelation.agentId),
    placeholder: "选择系统成员",
    filterable: true,
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_333));
let __VLS_337;
const __VLS_338 = ({ change: {} },
    { onChange: (__VLS_ctx.onRelationAgentChange) });
const { default: __VLS_339 } = __VLS_335.slots;
for (const [a] of __VLS_vFor((__VLS_ctx.otherAgents))) {
    let __VLS_340;
    /** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option'] | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
    elOption;
    // @ts-ignore
    const __VLS_341 = __VLS_asFunctionalComponent1(__VLS_340, new __VLS_340({
        key: (a.id),
        label: (a.name),
        value: (a.id),
    }));
    const __VLS_342 = __VLS_341({
        key: (a.id),
        label: (a.name),
        value: (a.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_341));
    const { default: __VLS_345 } = __VLS_343.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
        ...{ style: ({ background: __VLS_ctx.avatarColor(a.id) }) },
    });
    (a.name.charAt(0));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (a.name);
    // @ts-ignore
    [otherAgents, newRelation, newRelation, onRelationAgentChange, avatarColor,];
    var __VLS_343;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_335;
var __VLS_336;
// @ts-ignore
[];
var __VLS_329;
// @ts-ignore
[];
var __VLS_323;
let __VLS_346;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_347 = __VLS_asFunctionalComponent1(__VLS_346, new __VLS_346({
    span: (7),
}));
const __VLS_348 = __VLS_347({
    span: (7),
}, ...__VLS_functionalComponentArgsRest(__VLS_347));
const { default: __VLS_351 } = __VLS_349.slots;
let __VLS_352;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_353 = __VLS_asFunctionalComponent1(__VLS_352, new __VLS_352({
    label: "关系类型",
}));
const __VLS_354 = __VLS_353({
    label: "关系类型",
}, ...__VLS_functionalComponentArgsRest(__VLS_353));
const { default: __VLS_357 } = __VLS_355.slots;
let __VLS_358;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_359 = __VLS_asFunctionalComponent1(__VLS_358, new __VLS_358({
    modelValue: (__VLS_ctx.newRelation.relationType),
    ...{ style: {} },
}));
const __VLS_360 = __VLS_359({
    modelValue: (__VLS_ctx.newRelation.relationType),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_359));
const { default: __VLS_363 } = __VLS_361.slots;
let __VLS_364;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_365 = __VLS_asFunctionalComponent1(__VLS_364, new __VLS_364({
    label: "上级",
    value: "上级",
}));
const __VLS_366 = __VLS_365({
    label: "上级",
    value: "上级",
}, ...__VLS_functionalComponentArgsRest(__VLS_365));
let __VLS_369;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_370 = __VLS_asFunctionalComponent1(__VLS_369, new __VLS_369({
    label: "下级",
    value: "下级",
}));
const __VLS_371 = __VLS_370({
    label: "下级",
    value: "下级",
}, ...__VLS_functionalComponentArgsRest(__VLS_370));
let __VLS_374;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_375 = __VLS_asFunctionalComponent1(__VLS_374, new __VLS_374({
    label: "平级协作",
    value: "平级协作",
}));
const __VLS_376 = __VLS_375({
    label: "平级协作",
    value: "平级协作",
}, ...__VLS_functionalComponentArgsRest(__VLS_375));
let __VLS_379;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_380 = __VLS_asFunctionalComponent1(__VLS_379, new __VLS_379({
    label: "支持",
    value: "支持",
}));
const __VLS_381 = __VLS_380({
    label: "支持",
    value: "支持",
}, ...__VLS_functionalComponentArgsRest(__VLS_380));
// @ts-ignore
[newRelation,];
var __VLS_361;
// @ts-ignore
[];
var __VLS_355;
// @ts-ignore
[];
var __VLS_349;
let __VLS_384;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_385 = __VLS_asFunctionalComponent1(__VLS_384, new __VLS_384({
    span: (7),
}));
const __VLS_386 = __VLS_385({
    span: (7),
}, ...__VLS_functionalComponentArgsRest(__VLS_385));
const { default: __VLS_389 } = __VLS_387.slots;
let __VLS_390;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_391 = __VLS_asFunctionalComponent1(__VLS_390, new __VLS_390({
    label: "协作程度",
}));
const __VLS_392 = __VLS_391({
    label: "协作程度",
}, ...__VLS_functionalComponentArgsRest(__VLS_391));
const { default: __VLS_395 } = __VLS_393.slots;
let __VLS_396;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_397 = __VLS_asFunctionalComponent1(__VLS_396, new __VLS_396({
    modelValue: (__VLS_ctx.newRelation.strength),
    ...{ style: {} },
}));
const __VLS_398 = __VLS_397({
    modelValue: (__VLS_ctx.newRelation.strength),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_397));
const { default: __VLS_401 } = __VLS_399.slots;
let __VLS_402;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_403 = __VLS_asFunctionalComponent1(__VLS_402, new __VLS_402({
    label: "核心",
    value: "核心",
}));
const __VLS_404 = __VLS_403({
    label: "核心",
    value: "核心",
}, ...__VLS_functionalComponentArgsRest(__VLS_403));
let __VLS_407;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_408 = __VLS_asFunctionalComponent1(__VLS_407, new __VLS_407({
    label: "常用",
    value: "常用",
}));
const __VLS_409 = __VLS_408({
    label: "常用",
    value: "常用",
}, ...__VLS_functionalComponentArgsRest(__VLS_408));
let __VLS_412;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_413 = __VLS_asFunctionalComponent1(__VLS_412, new __VLS_412({
    label: "偶尔",
    value: "偶尔",
}));
const __VLS_414 = __VLS_413({
    label: "偶尔",
    value: "偶尔",
}, ...__VLS_functionalComponentArgsRest(__VLS_413));
// @ts-ignore
[newRelation,];
var __VLS_399;
// @ts-ignore
[];
var __VLS_393;
// @ts-ignore
[];
var __VLS_387;
// @ts-ignore
[];
var __VLS_317;
let __VLS_417;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_418 = __VLS_asFunctionalComponent1(__VLS_417, new __VLS_417({
    gutter: (12),
}));
const __VLS_419 = __VLS_418({
    gutter: (12),
}, ...__VLS_functionalComponentArgsRest(__VLS_418));
const { default: __VLS_422 } = __VLS_420.slots;
let __VLS_423;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_424 = __VLS_asFunctionalComponent1(__VLS_423, new __VLS_423({
    span: (18),
}));
const __VLS_425 = __VLS_424({
    span: (18),
}, ...__VLS_functionalComponentArgsRest(__VLS_424));
const { default: __VLS_428 } = __VLS_426.slots;
let __VLS_429;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_430 = __VLS_asFunctionalComponent1(__VLS_429, new __VLS_429({
    label: "说明（选填）",
}));
const __VLS_431 = __VLS_430({
    label: "说明（选填）",
}, ...__VLS_functionalComponentArgsRest(__VLS_430));
const { default: __VLS_434 } = __VLS_432.slots;
let __VLS_435;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_436 = __VLS_asFunctionalComponent1(__VLS_435, new __VLS_435({
    modelValue: (__VLS_ctx.newRelation.desc),
    placeholder: "简要描述这段关系...",
}));
const __VLS_437 = __VLS_436({
    modelValue: (__VLS_ctx.newRelation.desc),
    placeholder: "简要描述这段关系...",
}, ...__VLS_functionalComponentArgsRest(__VLS_436));
// @ts-ignore
[newRelation,];
var __VLS_432;
// @ts-ignore
[];
var __VLS_426;
let __VLS_440;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_441 = __VLS_asFunctionalComponent1(__VLS_440, new __VLS_440({
    span: (6),
}));
const __VLS_442 = __VLS_441({
    span: (6),
}, ...__VLS_functionalComponentArgsRest(__VLS_441));
const { default: __VLS_445 } = __VLS_443.slots;
let __VLS_446;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_447 = __VLS_asFunctionalComponent1(__VLS_446, new __VLS_446({
    label: " ",
}));
const __VLS_448 = __VLS_447({
    label: " ",
}, ...__VLS_functionalComponentArgsRest(__VLS_447));
const { default: __VLS_451 } = __VLS_449.slots;
let __VLS_452;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_453 = __VLS_asFunctionalComponent1(__VLS_452, new __VLS_452({
    ...{ 'onClick': {} },
    type: "primary",
    ...{ style: {} },
    disabled: (!__VLS_ctx.newRelation.agentId || !__VLS_ctx.newRelation.relationType || !__VLS_ctx.newRelation.strength),
    loading: (__VLS_ctx.relationsSaving),
}));
const __VLS_454 = __VLS_453({
    ...{ 'onClick': {} },
    type: "primary",
    ...{ style: {} },
    disabled: (!__VLS_ctx.newRelation.agentId || !__VLS_ctx.newRelation.relationType || !__VLS_ctx.newRelation.strength),
    loading: (__VLS_ctx.relationsSaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_453));
let __VLS_457;
const __VLS_458 = ({ click: {} },
    { onClick: (__VLS_ctx.addRelation) });
const { default: __VLS_459 } = __VLS_455.slots;
// @ts-ignore
[newRelation, newRelation, newRelation, relationsSaving, addRelation,];
var __VLS_455;
var __VLS_456;
// @ts-ignore
[];
var __VLS_449;
// @ts-ignore
[];
var __VLS_443;
// @ts-ignore
[];
var __VLS_420;
// @ts-ignore
[];
var __VLS_311;
// @ts-ignore
[];
var __VLS_304;
let __VLS_460;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_461 = __VLS_asFunctionalComponent1(__VLS_460, new __VLS_460({}));
const __VLS_462 = __VLS_461({}, ...__VLS_functionalComponentArgsRest(__VLS_461));
const { default: __VLS_465 } = __VLS_463.slots;
{
    const { header: __VLS_466 } = __VLS_463.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_467;
    /** @ts-ignore @type { | typeof __VLS_components.elBadge | typeof __VLS_components.ElBadge | typeof __VLS_components['el-badge']} */
    elBadge;
    // @ts-ignore
    const __VLS_468 = __VLS_asFunctionalComponent1(__VLS_467, new __VLS_467({
        value: (__VLS_ctx.parsedRelations.length),
        type: "info",
        ...{ style: {} },
    }));
    const __VLS_469 = __VLS_468({
        value: (__VLS_ctx.parsedRelations.length),
        type: "info",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_468));
    // @ts-ignore
    [parsedRelations,];
}
if (__VLS_ctx.parsedRelations.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
}
else {
    let __VLS_472;
    /** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
    elTable;
    // @ts-ignore
    const __VLS_473 = __VLS_asFunctionalComponent1(__VLS_472, new __VLS_472({
        data: (__VLS_ctx.parsedRelations),
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_474 = __VLS_473({
        data: (__VLS_ctx.parsedRelations),
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_473));
    const { default: __VLS_477 } = __VLS_475.slots;
    let __VLS_478;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_479 = __VLS_asFunctionalComponent1(__VLS_478, new __VLS_478({
        label: "成员",
        minWidth: "120",
    }));
    const __VLS_480 = __VLS_479({
        label: "成员",
        minWidth: "120",
    }, ...__VLS_functionalComponentArgsRest(__VLS_479));
    const { default: __VLS_483 } = __VLS_481.slots;
    {
        const { default: __VLS_484 } = __VLS_481.slots;
        const [{ row }] = __VLS_vSlot(__VLS_484);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
            ...{ style: ({ background: __VLS_ctx.avatarColor(row.agentId) }) },
        });
        (row.agentName.charAt(0));
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (row.agentName);
        // @ts-ignore
        [avatarColor, parsedRelations, parsedRelations,];
    }
    // @ts-ignore
    [];
    var __VLS_481;
    let __VLS_485;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_486 = __VLS_asFunctionalComponent1(__VLS_485, new __VLS_485({
        label: "类型",
        width: "100",
    }));
    const __VLS_487 = __VLS_486({
        label: "类型",
        width: "100",
    }, ...__VLS_functionalComponentArgsRest(__VLS_486));
    const { default: __VLS_490 } = __VLS_488.slots;
    {
        const { default: __VLS_491 } = __VLS_488.slots;
        const [{ row }] = __VLS_vSlot(__VLS_491);
        let __VLS_492;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_493 = __VLS_asFunctionalComponent1(__VLS_492, new __VLS_492({
            type: (__VLS_ctx.relationTypeColor(row.relationType)),
            size: "small",
        }));
        const __VLS_494 = __VLS_493({
            type: (__VLS_ctx.relationTypeColor(row.relationType)),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_493));
        const { default: __VLS_497 } = __VLS_495.slots;
        (row.relationType);
        // @ts-ignore
        [relationTypeColor,];
        var __VLS_495;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_488;
    let __VLS_498;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_499 = __VLS_asFunctionalComponent1(__VLS_498, new __VLS_498({
        label: "程度",
        width: "80",
    }));
    const __VLS_500 = __VLS_499({
        label: "程度",
        width: "80",
    }, ...__VLS_functionalComponentArgsRest(__VLS_499));
    const { default: __VLS_503 } = __VLS_501.slots;
    {
        const { default: __VLS_504 } = __VLS_501.slots;
        const [{ row }] = __VLS_vSlot(__VLS_504);
        let __VLS_505;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_506 = __VLS_asFunctionalComponent1(__VLS_505, new __VLS_505({
            type: (__VLS_ctx.strengthColor(row.strength)),
            size: "small",
            effect: "plain",
        }));
        const __VLS_507 = __VLS_506({
            type: (__VLS_ctx.strengthColor(row.strength)),
            size: "small",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_506));
        const { default: __VLS_510 } = __VLS_508.slots;
        (row.strength);
        // @ts-ignore
        [strengthColor,];
        var __VLS_508;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_501;
    let __VLS_511;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_512 = __VLS_asFunctionalComponent1(__VLS_511, new __VLS_511({
        label: "说明",
        minWidth: "120",
        showOverflowTooltip: true,
    }));
    const __VLS_513 = __VLS_512({
        label: "说明",
        minWidth: "120",
        showOverflowTooltip: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_512));
    const { default: __VLS_516 } = __VLS_514.slots;
    {
        const { default: __VLS_517 } = __VLS_514.slots;
        const [{ row }] = __VLS_vSlot(__VLS_517);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (row.desc || '—');
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_514;
    let __VLS_518;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_519 = __VLS_asFunctionalComponent1(__VLS_518, new __VLS_518({
        label: "操作",
        width: "70",
        fixed: "right",
    }));
    const __VLS_520 = __VLS_519({
        label: "操作",
        width: "70",
        fixed: "right",
    }, ...__VLS_functionalComponentArgsRest(__VLS_519));
    const { default: __VLS_523 } = __VLS_521.slots;
    {
        const { default: __VLS_524 } = __VLS_521.slots;
        const [{ $index }] = __VLS_vSlot(__VLS_524);
        let __VLS_525;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_526 = __VLS_asFunctionalComponent1(__VLS_525, new __VLS_525({
            ...{ 'onClick': {} },
            type: "danger",
            link: true,
            size: "small",
        }));
        const __VLS_527 = __VLS_526({
            ...{ 'onClick': {} },
            type: "danger",
            link: true,
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_526));
        let __VLS_530;
        const __VLS_531 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(__VLS_ctx.parsedRelations.length === 0))
                        return;
                    __VLS_ctx.deleteRelation($index);
                    // @ts-ignore
                    [deleteRelation,];
                } });
        const { default: __VLS_532 } = __VLS_528.slots;
        // @ts-ignore
        [];
        var __VLS_528;
        var __VLS_529;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_521;
    // @ts-ignore
    [];
    var __VLS_475;
}
// @ts-ignore
[];
var __VLS_463;
// @ts-ignore
[];
var __VLS_298;
let __VLS_533;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_534 = __VLS_asFunctionalComponent1(__VLS_533, new __VLS_533({
    span: (10),
}));
const __VLS_535 = __VLS_534({
    span: (10),
}, ...__VLS_functionalComponentArgsRest(__VLS_534));
const { default: __VLS_538 } = __VLS_536.slots;
let __VLS_539;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_540 = __VLS_asFunctionalComponent1(__VLS_539, new __VLS_539({
    header: "关系预览",
}));
const __VLS_541 = __VLS_540({
    header: "关系预览",
}, ...__VLS_functionalComponentArgsRest(__VLS_540));
const { default: __VLS_544 } = __VLS_542.slots;
if (__VLS_ctx.parsedRelations.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "relations-list" },
    });
    /** @type {__VLS_StyleScopedClasses['relations-list']} */ ;
    for (const [row] of __VLS_vFor((__VLS_ctx.parsedRelations))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (row.agentId),
            ...{ class: "relation-card" },
        });
        /** @type {__VLS_StyleScopedClasses['relation-card']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "relation-avatar" },
            ...{ style: ({ background: __VLS_ctx.avatarColor(row.agentId) }) },
        });
        /** @type {__VLS_StyleScopedClasses['relation-avatar']} */ ;
        (row.agentName.charAt(0).toUpperCase());
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "relation-info" },
        });
        /** @type {__VLS_StyleScopedClasses['relation-info']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "relation-name" },
        });
        /** @type {__VLS_StyleScopedClasses['relation-name']} */ ;
        (row.agentName);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "relation-tags" },
        });
        /** @type {__VLS_StyleScopedClasses['relation-tags']} */ ;
        let __VLS_545;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_546 = __VLS_asFunctionalComponent1(__VLS_545, new __VLS_545({
            type: (__VLS_ctx.relationTypeColor(row.relationType)),
            size: "small",
        }));
        const __VLS_547 = __VLS_546({
            type: (__VLS_ctx.relationTypeColor(row.relationType)),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_546));
        const { default: __VLS_550 } = __VLS_548.slots;
        (row.relationType);
        // @ts-ignore
        [avatarColor, parsedRelations, parsedRelations, relationTypeColor,];
        var __VLS_548;
        let __VLS_551;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_552 = __VLS_asFunctionalComponent1(__VLS_551, new __VLS_551({
            type: (__VLS_ctx.strengthColor(row.strength)),
            size: "small",
            effect: "plain",
        }));
        const __VLS_553 = __VLS_552({
            type: (__VLS_ctx.strengthColor(row.strength)),
            size: "small",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_552));
        const { default: __VLS_556 } = __VLS_554.slots;
        (row.strength);
        // @ts-ignore
        [strengthColor,];
        var __VLS_554;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "relation-desc" },
        });
        /** @type {__VLS_StyleScopedClasses['relation-desc']} */ ;
        (row.desc);
        // @ts-ignore
        [];
    }
}
// @ts-ignore
[];
var __VLS_542;
// @ts-ignore
[];
var __VLS_536;
// @ts-ignore
[];
var __VLS_292;
// @ts-ignore
[];
var __VLS_286;
let __VLS_557;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_558 = __VLS_asFunctionalComponent1(__VLS_557, new __VLS_557({
    label: "记忆",
    name: "memory",
}));
const __VLS_559 = __VLS_558({
    label: "记忆",
    name: "memory",
}, ...__VLS_functionalComponentArgsRest(__VLS_558));
const { default: __VLS_562 } = __VLS_560.slots;
let __VLS_563;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_564 = __VLS_asFunctionalComponent1(__VLS_563, new __VLS_563({
    ...{ style: {} },
    shadow: "never",
}));
const __VLS_565 = __VLS_564({
    ...{ style: {} },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_564));
const { default: __VLS_568 } = __VLS_566.slots;
{
    const { header: __VLS_569 } = __VLS_566.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_570;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_571 = __VLS_asFunctionalComponent1(__VLS_570, new __VLS_570({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.memCfg.enabled),
        activeText: "已开启",
        inactiveText: "已关闭",
    }));
    const __VLS_572 = __VLS_571({
        ...{ 'onChange': {} },
        modelValue: (__VLS_ctx.memCfg.enabled),
        activeText: "已开启",
        inactiveText: "已关闭",
    }, ...__VLS_functionalComponentArgsRest(__VLS_571));
    let __VLS_575;
    const __VLS_576 = ({ change: {} },
        { onChange: (__VLS_ctx.saveMemConfig) });
    var __VLS_573;
    var __VLS_574;
    // @ts-ignore
    [memCfg, saveMemConfig,];
}
let __VLS_577;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_578 = __VLS_asFunctionalComponent1(__VLS_577, new __VLS_577({
    model: (__VLS_ctx.memCfg),
    labelPosition: "top",
    size: "small",
    disabled: (!__VLS_ctx.memCfg.enabled),
}));
const __VLS_579 = __VLS_578({
    model: (__VLS_ctx.memCfg),
    labelPosition: "top",
    size: "small",
    disabled: (!__VLS_ctx.memCfg.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_578));
const { default: __VLS_582 } = __VLS_580.slots;
let __VLS_583;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_584 = __VLS_asFunctionalComponent1(__VLS_583, new __VLS_583({
    gutter: (16),
}));
const __VLS_585 = __VLS_584({
    gutter: (16),
}, ...__VLS_functionalComponentArgsRest(__VLS_584));
const { default: __VLS_588 } = __VLS_586.slots;
let __VLS_589;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_590 = __VLS_asFunctionalComponent1(__VLS_589, new __VLS_589({
    span: (6),
}));
const __VLS_591 = __VLS_590({
    span: (6),
}, ...__VLS_functionalComponentArgsRest(__VLS_590));
const { default: __VLS_594 } = __VLS_592.slots;
let __VLS_595;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_596 = __VLS_asFunctionalComponent1(__VLS_595, new __VLS_595({
    label: "整理频率",
}));
const __VLS_597 = __VLS_596({
    label: "整理频率",
}, ...__VLS_functionalComponentArgsRest(__VLS_596));
const { default: __VLS_600 } = __VLS_598.slots;
let __VLS_601;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_602 = __VLS_asFunctionalComponent1(__VLS_601, new __VLS_601({
    modelValue: (__VLS_ctx.memCfg.schedule),
    ...{ style: {} },
}));
const __VLS_603 = __VLS_602({
    modelValue: (__VLS_ctx.memCfg.schedule),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_602));
const { default: __VLS_606 } = __VLS_604.slots;
let __VLS_607;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_608 = __VLS_asFunctionalComponent1(__VLS_607, new __VLS_607({
    label: "每小时",
    value: "hourly",
}));
const __VLS_609 = __VLS_608({
    label: "每小时",
    value: "hourly",
}, ...__VLS_functionalComponentArgsRest(__VLS_608));
let __VLS_612;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_613 = __VLS_asFunctionalComponent1(__VLS_612, new __VLS_612({
    label: "每6小时",
    value: "every6h",
}));
const __VLS_614 = __VLS_613({
    label: "每6小时",
    value: "every6h",
}, ...__VLS_functionalComponentArgsRest(__VLS_613));
let __VLS_617;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_618 = __VLS_asFunctionalComponent1(__VLS_617, new __VLS_617({
    label: "每天",
    value: "daily",
}));
const __VLS_619 = __VLS_618({
    label: "每天",
    value: "daily",
}, ...__VLS_functionalComponentArgsRest(__VLS_618));
let __VLS_622;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_623 = __VLS_asFunctionalComponent1(__VLS_622, new __VLS_622({
    label: "每周",
    value: "weekly",
}));
const __VLS_624 = __VLS_623({
    label: "每周",
    value: "weekly",
}, ...__VLS_functionalComponentArgsRest(__VLS_623));
// @ts-ignore
[memCfg, memCfg, memCfg,];
var __VLS_604;
// @ts-ignore
[];
var __VLS_598;
// @ts-ignore
[];
var __VLS_592;
let __VLS_627;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_628 = __VLS_asFunctionalComponent1(__VLS_627, new __VLS_627({
    span: (5),
}));
const __VLS_629 = __VLS_628({
    span: (5),
}, ...__VLS_functionalComponentArgsRest(__VLS_628));
const { default: __VLS_632 } = __VLS_630.slots;
let __VLS_633;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_634 = __VLS_asFunctionalComponent1(__VLS_633, new __VLS_633({
    label: "每个会话保留轮数",
}));
const __VLS_635 = __VLS_634({
    label: "每个会话保留轮数",
}, ...__VLS_functionalComponentArgsRest(__VLS_634));
const { default: __VLS_638 } = __VLS_636.slots;
let __VLS_639;
/** @ts-ignore @type { | typeof __VLS_components.elInputNumber | typeof __VLS_components.ElInputNumber | typeof __VLS_components['el-input-number']} */
elInputNumber;
// @ts-ignore
const __VLS_640 = __VLS_asFunctionalComponent1(__VLS_639, new __VLS_639({
    modelValue: (__VLS_ctx.memCfg.keepTurns),
    min: (1),
    max: (20),
    ...{ style: {} },
}));
const __VLS_641 = __VLS_640({
    modelValue: (__VLS_ctx.memCfg.keepTurns),
    min: (1),
    max: (20),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_640));
// @ts-ignore
[memCfg,];
var __VLS_636;
// @ts-ignore
[];
var __VLS_630;
let __VLS_644;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_645 = __VLS_asFunctionalComponent1(__VLS_644, new __VLS_644({
    span: (13),
}));
const __VLS_646 = __VLS_645({
    span: (13),
}, ...__VLS_functionalComponentArgsRest(__VLS_645));
const { default: __VLS_649 } = __VLS_647.slots;
let __VLS_650;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_651 = __VLS_asFunctionalComponent1(__VLS_650, new __VLS_650({
    label: "记录重点（留空则自动）",
}));
const __VLS_652 = __VLS_651({
    label: "记录重点（留空则自动）",
}, ...__VLS_functionalComponentArgsRest(__VLS_651));
const { default: __VLS_655 } = __VLS_653.slots;
let __VLS_656;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_657 = __VLS_asFunctionalComponent1(__VLS_656, new __VLS_656({
    modelValue: (__VLS_ctx.memCfg.focusHint),
    placeholder: "例如：记录数学解题步骤和用户常见错误",
}));
const __VLS_658 = __VLS_657({
    modelValue: (__VLS_ctx.memCfg.focusHint),
    placeholder: "例如：记录数学解题步骤和用户常见错误",
}, ...__VLS_functionalComponentArgsRest(__VLS_657));
// @ts-ignore
[memCfg,];
var __VLS_653;
// @ts-ignore
[];
var __VLS_647;
// @ts-ignore
[];
var __VLS_586;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_661;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_662 = __VLS_asFunctionalComponent1(__VLS_661, new __VLS_661({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
    loading: (__VLS_ctx.memCfgSaving),
}));
const __VLS_663 = __VLS_662({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
    loading: (__VLS_ctx.memCfgSaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_662));
let __VLS_666;
const __VLS_667 = ({ click: {} },
    { onClick: (__VLS_ctx.saveMemConfig) });
const { default: __VLS_668 } = __VLS_664.slots;
// @ts-ignore
[saveMemConfig, memCfgSaving,];
var __VLS_664;
var __VLS_665;
let __VLS_669;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_670 = __VLS_asFunctionalComponent1(__VLS_669, new __VLS_669({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.memConsolidating),
}));
const __VLS_671 = __VLS_670({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.memConsolidating),
}, ...__VLS_functionalComponentArgsRest(__VLS_670));
let __VLS_674;
const __VLS_675 = ({ click: {} },
    { onClick: (__VLS_ctx.consolidateNow) });
const { default: __VLS_676 } = __VLS_672.slots;
// @ts-ignore
[memConsolidating, consolidateNow,];
var __VLS_672;
var __VLS_673;
let __VLS_677;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_678 = __VLS_asFunctionalComponent1(__VLS_677, new __VLS_677({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_679 = __VLS_678({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_678));
const { default: __VLS_682 } = __VLS_680.slots;
(__VLS_ctx.memCfg.keepTurns);
// @ts-ignore
[memCfg,];
var __VLS_680;
// @ts-ignore
[];
var __VLS_580;
// @ts-ignore
[];
var __VLS_566;
let __VLS_683;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_684 = __VLS_asFunctionalComponent1(__VLS_683, new __VLS_683({
    ...{ style: {} },
    shadow: "never",
}));
const __VLS_685 = __VLS_684({
    ...{ style: {} },
    shadow: "never",
}, ...__VLS_functionalComponentArgsRest(__VLS_684));
const { default: __VLS_688 } = __VLS_686.slots;
{
    const { header: __VLS_689 } = __VLS_686.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_690;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_691 = __VLS_asFunctionalComponent1(__VLS_690, new __VLS_690({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        loading: (__VLS_ctx.memLogsLoading),
    }));
    const __VLS_692 = __VLS_691({
        ...{ 'onClick': {} },
        size: "small",
        text: true,
        loading: (__VLS_ctx.memLogsLoading),
    }, ...__VLS_functionalComponentArgsRest(__VLS_691));
    let __VLS_695;
    const __VLS_696 = ({ click: {} },
        { onClick: (__VLS_ctx.loadMemLogs) });
    const { default: __VLS_697 } = __VLS_693.slots;
    // @ts-ignore
    [memLogsLoading, loadMemLogs,];
    var __VLS_693;
    var __VLS_694;
    // @ts-ignore
    [];
}
if (__VLS_ctx.memLogs.length === 0 && !__VLS_ctx.memLogsLoading) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
}
else {
    let __VLS_698;
    /** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
    elTable;
    // @ts-ignore
    const __VLS_699 = __VLS_asFunctionalComponent1(__VLS_698, new __VLS_698({
        data: (__VLS_ctx.memLogs.slice(0, 20)),
        size: "small",
    }));
    const __VLS_700 = __VLS_699({
        data: (__VLS_ctx.memLogs.slice(0, 20)),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_699));
    const { default: __VLS_703 } = __VLS_701.slots;
    let __VLS_704;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_705 = __VLS_asFunctionalComponent1(__VLS_704, new __VLS_704({
        label: "时间",
        width: "160",
    }));
    const __VLS_706 = __VLS_705({
        label: "时间",
        width: "160",
    }, ...__VLS_functionalComponentArgsRest(__VLS_705));
    const { default: __VLS_709 } = __VLS_707.slots;
    {
        const { default: __VLS_710 } = __VLS_707.slots;
        const [{ row }] = __VLS_vSlot(__VLS_710);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.formatTimestamp(row.timestamp));
        // @ts-ignore
        [memLogsLoading, memLogs, memLogs, formatTimestamp,];
    }
    // @ts-ignore
    [];
    var __VLS_707;
    let __VLS_711;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_712 = __VLS_asFunctionalComponent1(__VLS_711, new __VLS_711({
        label: "状态",
        width: "72",
    }));
    const __VLS_713 = __VLS_712({
        label: "状态",
        width: "72",
    }, ...__VLS_functionalComponentArgsRest(__VLS_712));
    const { default: __VLS_716 } = __VLS_714.slots;
    {
        const { default: __VLS_717 } = __VLS_714.slots;
        const [{ row }] = __VLS_vSlot(__VLS_717);
        let __VLS_718;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_719 = __VLS_asFunctionalComponent1(__VLS_718, new __VLS_718({
            type: (row.status === 'ok' ? 'success' : 'danger'),
            size: "small",
        }));
        const __VLS_720 = __VLS_719({
            type: (row.status === 'ok' ? 'success' : 'danger'),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_719));
        const { default: __VLS_723 } = __VLS_721.slots;
        (row.status);
        // @ts-ignore
        [];
        var __VLS_721;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_714;
    let __VLS_724;
    /** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
    elTableColumn;
    // @ts-ignore
    const __VLS_725 = __VLS_asFunctionalComponent1(__VLS_724, new __VLS_724({
        label: "结果",
        minWidth: "200",
        showOverflowTooltip: true,
    }));
    const __VLS_726 = __VLS_725({
        label: "结果",
        minWidth: "200",
        showOverflowTooltip: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_725));
    const { default: __VLS_729 } = __VLS_727.slots;
    {
        const { default: __VLS_730 } = __VLS_727.slots;
        const [{ row }] = __VLS_vSlot(__VLS_730);
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (row.message || '—');
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_727;
    // @ts-ignore
    [];
    var __VLS_701;
}
// @ts-ignore
[];
var __VLS_686;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "memory-toolbar" },
    ...{ style: {} },
});
/** @type {__VLS_StyleScopedClasses['memory-toolbar']} */ ;
let __VLS_731;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_732 = __VLS_asFunctionalComponent1(__VLS_731, new __VLS_731({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
}));
const __VLS_733 = __VLS_732({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_732));
let __VLS_736;
const __VLS_737 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.showNewMemoryFile = true;
            // @ts-ignore
            [showNewMemoryFile,];
        } });
const { default: __VLS_738 } = __VLS_734.slots;
let __VLS_739;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_740 = __VLS_asFunctionalComponent1(__VLS_739, new __VLS_739({}));
const __VLS_741 = __VLS_740({}, ...__VLS_functionalComponentArgsRest(__VLS_740));
const { default: __VLS_744 } = __VLS_742.slots;
let __VLS_745;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_746 = __VLS_asFunctionalComponent1(__VLS_745, new __VLS_745({}));
const __VLS_747 = __VLS_746({}, ...__VLS_functionalComponentArgsRest(__VLS_746));
// @ts-ignore
[];
var __VLS_742;
// @ts-ignore
[];
var __VLS_734;
var __VLS_735;
let __VLS_750;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_751 = __VLS_asFunctionalComponent1(__VLS_750, new __VLS_750({
    ...{ 'onClick': {} },
    size: "small",
}));
const __VLS_752 = __VLS_751({
    ...{ 'onClick': {} },
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_751));
let __VLS_755;
const __VLS_756 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.showDailyEntry = true;
            // @ts-ignore
            [showDailyEntry,];
        } });
const { default: __VLS_757 } = __VLS_753.slots;
let __VLS_758;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_759 = __VLS_asFunctionalComponent1(__VLS_758, new __VLS_758({}));
const __VLS_760 = __VLS_759({}, ...__VLS_functionalComponentArgsRest(__VLS_759));
const { default: __VLS_763 } = __VLS_761.slots;
let __VLS_764;
/** @ts-ignore @type { | typeof __VLS_components.EditPen} */
EditPen;
// @ts-ignore
const __VLS_765 = __VLS_asFunctionalComponent1(__VLS_764, new __VLS_764({}));
const __VLS_766 = __VLS_765({}, ...__VLS_functionalComponentArgsRest(__VLS_765));
// @ts-ignore
[];
var __VLS_761;
// @ts-ignore
[];
var __VLS_753;
var __VLS_754;
let __VLS_769;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_770 = __VLS_asFunctionalComponent1(__VLS_769, new __VLS_769({
    ...{ 'onClick': {} },
    size: "small",
}));
const __VLS_771 = __VLS_770({
    ...{ 'onClick': {} },
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_770));
let __VLS_774;
const __VLS_775 = ({ click: {} },
    { onClick: (__VLS_ctx.loadMemoryTree) });
const { default: __VLS_776 } = __VLS_772.slots;
let __VLS_777;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_778 = __VLS_asFunctionalComponent1(__VLS_777, new __VLS_777({}));
const __VLS_779 = __VLS_778({}, ...__VLS_functionalComponentArgsRest(__VLS_778));
const { default: __VLS_782 } = __VLS_780.slots;
let __VLS_783;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_784 = __VLS_asFunctionalComponent1(__VLS_783, new __VLS_783({}));
const __VLS_785 = __VLS_784({}, ...__VLS_functionalComponentArgsRest(__VLS_784));
// @ts-ignore
[loadMemoryTree,];
var __VLS_780;
// @ts-ignore
[];
var __VLS_772;
var __VLS_773;
let __VLS_788;
/** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
elRow;
// @ts-ignore
const __VLS_789 = __VLS_asFunctionalComponent1(__VLS_788, new __VLS_788({
    gutter: (16),
}));
const __VLS_790 = __VLS_789({
    gutter: (16),
}, ...__VLS_functionalComponentArgsRest(__VLS_789));
const { default: __VLS_793 } = __VLS_791.slots;
let __VLS_794;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_795 = __VLS_asFunctionalComponent1(__VLS_794, new __VLS_794({
    span: (7),
}));
const __VLS_796 = __VLS_795({
    span: (7),
}, ...__VLS_functionalComponentArgsRest(__VLS_795));
const { default: __VLS_799 } = __VLS_797.slots;
let __VLS_800;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_801 = __VLS_asFunctionalComponent1(__VLS_800, new __VLS_800({
    header: "记忆目录",
    shadow: "hover",
}));
const __VLS_802 = __VLS_801({
    header: "记忆目录",
    shadow: "hover",
}, ...__VLS_functionalComponentArgsRest(__VLS_801));
const { default: __VLS_805 } = __VLS_803.slots;
let __VLS_806;
/** @ts-ignore @type { | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree'] | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree']} */
elTree;
// @ts-ignore
const __VLS_807 = __VLS_asFunctionalComponent1(__VLS_806, new __VLS_806({
    ...{ 'onNodeClick': {} },
    data: (__VLS_ctx.memoryTreeData),
    props: ({ label: 'name', children: 'children', isLeaf: (d) => !d.isDir }),
    highlightCurrent: true,
    defaultExpandAll: true,
    expandOnClickNode: (false),
}));
const __VLS_808 = __VLS_807({
    ...{ 'onNodeClick': {} },
    data: (__VLS_ctx.memoryTreeData),
    props: ({ label: 'name', children: 'children', isLeaf: (d) => !d.isDir }),
    highlightCurrent: true,
    defaultExpandAll: true,
    expandOnClickNode: (false),
}, ...__VLS_functionalComponentArgsRest(__VLS_807));
let __VLS_811;
const __VLS_812 = ({ nodeClick: {} },
    { onNodeClick: (__VLS_ctx.handleMemoryNodeClick) });
const { default: __VLS_813 } = __VLS_809.slots;
{
    const { default: __VLS_814 } = __VLS_809.slots;
    const [{ data }] = __VLS_vSlot(__VLS_814);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    if (data.isDir) {
        let __VLS_815;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_816 = __VLS_asFunctionalComponent1(__VLS_815, new __VLS_815({
            ...{ style: {} },
        }));
        const __VLS_817 = __VLS_816({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_816));
        const { default: __VLS_820 } = __VLS_818.slots;
        let __VLS_821;
        /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
        FolderOpened;
        // @ts-ignore
        const __VLS_822 = __VLS_asFunctionalComponent1(__VLS_821, new __VLS_821({}));
        const __VLS_823 = __VLS_822({}, ...__VLS_functionalComponentArgsRest(__VLS_822));
        // @ts-ignore
        [memoryTreeData, handleMemoryNodeClick,];
        var __VLS_818;
    }
    else {
        let __VLS_826;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_827 = __VLS_asFunctionalComponent1(__VLS_826, new __VLS_826({
            ...{ style: {} },
        }));
        const __VLS_828 = __VLS_827({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_827));
        const { default: __VLS_831 } = __VLS_829.slots;
        let __VLS_832;
        /** @ts-ignore @type { | typeof __VLS_components.Document} */
        Document;
        // @ts-ignore
        const __VLS_833 = __VLS_asFunctionalComponent1(__VLS_832, new __VLS_832({}));
        const __VLS_834 = __VLS_833({}, ...__VLS_functionalComponentArgsRest(__VLS_833));
        // @ts-ignore
        [];
        var __VLS_829;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (data.name);
    if (!data.isDir && data.size) {
        let __VLS_837;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_838 = __VLS_asFunctionalComponent1(__VLS_837, new __VLS_837({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_839 = __VLS_838({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_838));
        const { default: __VLS_842 } = __VLS_840.slots;
        (__VLS_ctx.formatSize(data.size));
        // @ts-ignore
        [formatSize,];
        var __VLS_840;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_809;
var __VLS_810;
if (__VLS_ctx.memoryTreeData.length === 0) {
    let __VLS_843;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_844 = __VLS_asFunctionalComponent1(__VLS_843, new __VLS_843({
        description: "记忆树为空",
        imageSize: (40),
    }));
    const __VLS_845 = __VLS_844({
        description: "记忆树为空",
        imageSize: (40),
    }, ...__VLS_functionalComponentArgsRest(__VLS_844));
}
// @ts-ignore
[memoryTreeData,];
var __VLS_803;
// @ts-ignore
[];
var __VLS_797;
let __VLS_848;
/** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
elCol;
// @ts-ignore
const __VLS_849 = __VLS_asFunctionalComponent1(__VLS_848, new __VLS_848({
    span: (17),
}));
const __VLS_850 = __VLS_849({
    span: (17),
}, ...__VLS_functionalComponentArgsRest(__VLS_849));
const { default: __VLS_853 } = __VLS_851.slots;
let __VLS_854;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_855 = __VLS_asFunctionalComponent1(__VLS_854, new __VLS_854({
    shadow: "hover",
}));
const __VLS_856 = __VLS_855({
    shadow: "hover",
}, ...__VLS_functionalComponentArgsRest(__VLS_855));
const { default: __VLS_859 } = __VLS_857.slots;
{
    const { header: __VLS_860 } = __VLS_857.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_861;
    /** @ts-ignore @type { | typeof __VLS_components.elBreadcrumb | typeof __VLS_components.ElBreadcrumb | typeof __VLS_components['el-breadcrumb'] | typeof __VLS_components.elBreadcrumb | typeof __VLS_components.ElBreadcrumb | typeof __VLS_components['el-breadcrumb']} */
    elBreadcrumb;
    // @ts-ignore
    const __VLS_862 = __VLS_asFunctionalComponent1(__VLS_861, new __VLS_861({
        separator: "/",
    }));
    const __VLS_863 = __VLS_862({
        separator: "/",
    }, ...__VLS_functionalComponentArgsRest(__VLS_862));
    const { default: __VLS_866 } = __VLS_864.slots;
    let __VLS_867;
    /** @ts-ignore @type { | typeof __VLS_components.elBreadcrumbItem | typeof __VLS_components.ElBreadcrumbItem | typeof __VLS_components['el-breadcrumb-item'] | typeof __VLS_components.elBreadcrumbItem | typeof __VLS_components.ElBreadcrumbItem | typeof __VLS_components['el-breadcrumb-item']} */
    elBreadcrumbItem;
    // @ts-ignore
    const __VLS_868 = __VLS_asFunctionalComponent1(__VLS_867, new __VLS_867({}));
    const __VLS_869 = __VLS_868({}, ...__VLS_functionalComponentArgsRest(__VLS_868));
    const { default: __VLS_872 } = __VLS_870.slots;
    // @ts-ignore
    [];
    var __VLS_870;
    for (const [seg, i] of __VLS_vFor((__VLS_ctx.memoryFileBreadcrumb))) {
        let __VLS_873;
        /** @ts-ignore @type { | typeof __VLS_components.elBreadcrumbItem | typeof __VLS_components.ElBreadcrumbItem | typeof __VLS_components['el-breadcrumb-item'] | typeof __VLS_components.elBreadcrumbItem | typeof __VLS_components.ElBreadcrumbItem | typeof __VLS_components['el-breadcrumb-item']} */
        elBreadcrumbItem;
        // @ts-ignore
        const __VLS_874 = __VLS_asFunctionalComponent1(__VLS_873, new __VLS_873({
            key: (i),
        }));
        const __VLS_875 = __VLS_874({
            key: (i),
        }, ...__VLS_functionalComponentArgsRest(__VLS_874));
        const { default: __VLS_878 } = __VLS_876.slots;
        (seg);
        // @ts-ignore
        [memoryFileBreadcrumb,];
        var __VLS_876;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_864;
    if (__VLS_ctx.memoryEditPath) {
        let __VLS_879;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_880 = __VLS_asFunctionalComponent1(__VLS_879, new __VLS_879({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
            loading: (__VLS_ctx.memorySaving),
        }));
        const __VLS_881 = __VLS_880({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
            loading: (__VLS_ctx.memorySaving),
        }, ...__VLS_functionalComponentArgsRest(__VLS_880));
        let __VLS_884;
        const __VLS_885 = ({ click: {} },
            { onClick: (__VLS_ctx.saveMemoryFile) });
        const { default: __VLS_886 } = __VLS_882.slots;
        // @ts-ignore
        [memoryEditPath, memorySaving, saveMemoryFile,];
        var __VLS_882;
        var __VLS_883;
    }
    // @ts-ignore
    [];
}
if (__VLS_ctx.memoryEditPath) {
    let __VLS_887;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_888 = __VLS_asFunctionalComponent1(__VLS_887, new __VLS_887({
        modelValue: (__VLS_ctx.memoryEditContent),
        type: "textarea",
        rows: (22),
        ...{ style: {} },
    }));
    const __VLS_889 = __VLS_888({
        modelValue: (__VLS_ctx.memoryEditContent),
        type: "textarea",
        rows: (22),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_888));
}
else {
    let __VLS_892;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_893 = __VLS_asFunctionalComponent1(__VLS_892, new __VLS_892({
        description: "点击左侧文件查看和编辑",
        imageSize: (60),
    }));
    const __VLS_894 = __VLS_893({
        description: "点击左侧文件查看和编辑",
        imageSize: (60),
    }, ...__VLS_functionalComponentArgsRest(__VLS_893));
}
// @ts-ignore
[memoryEditPath, memoryEditContent,];
var __VLS_857;
// @ts-ignore
[];
var __VLS_851;
// @ts-ignore
[];
var __VLS_791;
let __VLS_897;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_898 = __VLS_asFunctionalComponent1(__VLS_897, new __VLS_897({
    modelValue: (__VLS_ctx.showNewMemoryFile),
    title: "新建记忆文件",
    width: "480px",
}));
const __VLS_899 = __VLS_898({
    modelValue: (__VLS_ctx.showNewMemoryFile),
    title: "新建记忆文件",
    width: "480px",
}, ...__VLS_functionalComponentArgsRest(__VLS_898));
const { default: __VLS_902 } = __VLS_900.slots;
let __VLS_903;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_904 = __VLS_asFunctionalComponent1(__VLS_903, new __VLS_903({
    labelWidth: "80px",
}));
const __VLS_905 = __VLS_904({
    labelWidth: "80px",
}, ...__VLS_functionalComponentArgsRest(__VLS_904));
const { default: __VLS_908 } = __VLS_906.slots;
let __VLS_909;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_910 = __VLS_asFunctionalComponent1(__VLS_909, new __VLS_909({
    label: "路径",
}));
const __VLS_911 = __VLS_910({
    label: "路径",
}, ...__VLS_functionalComponentArgsRest(__VLS_910));
const { default: __VLS_914 } = __VLS_912.slots;
let __VLS_915;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_916 = __VLS_asFunctionalComponent1(__VLS_915, new __VLS_915({
    modelValue: (__VLS_ctx.newMemoryPath),
    placeholder: "例如: projects/my-project.md 或 topics/cooking.md",
}));
const __VLS_917 = __VLS_916({
    modelValue: (__VLS_ctx.newMemoryPath),
    placeholder: "例如: projects/my-project.md 或 topics/cooking.md",
}, ...__VLS_functionalComponentArgsRest(__VLS_916));
let __VLS_920;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_921 = __VLS_asFunctionalComponent1(__VLS_920, new __VLS_920({
    type: "info",
    size: "small",
    ...{ style: {} },
}));
const __VLS_922 = __VLS_921({
    type: "info",
    size: "small",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_921));
const { default: __VLS_925 } = __VLS_923.slots;
// @ts-ignore
[showNewMemoryFile, newMemoryPath,];
var __VLS_923;
// @ts-ignore
[];
var __VLS_912;
// @ts-ignore
[];
var __VLS_906;
{
    const { footer: __VLS_926 } = __VLS_900.slots;
    let __VLS_927;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_928 = __VLS_asFunctionalComponent1(__VLS_927, new __VLS_927({
        ...{ 'onClick': {} },
    }));
    const __VLS_929 = __VLS_928({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_928));
    let __VLS_932;
    const __VLS_933 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showNewMemoryFile = false;
                // @ts-ignore
                [showNewMemoryFile,];
            } });
    const { default: __VLS_934 } = __VLS_930.slots;
    // @ts-ignore
    [];
    var __VLS_930;
    var __VLS_931;
    let __VLS_935;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_936 = __VLS_asFunctionalComponent1(__VLS_935, new __VLS_935({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_937 = __VLS_936({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_936));
    let __VLS_940;
    const __VLS_941 = ({ click: {} },
        { onClick: (__VLS_ctx.createMemoryFile) });
    const { default: __VLS_942 } = __VLS_938.slots;
    // @ts-ignore
    [createMemoryFile,];
    var __VLS_938;
    var __VLS_939;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_900;
let __VLS_943;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_944 = __VLS_asFunctionalComponent1(__VLS_943, new __VLS_943({
    modelValue: (__VLS_ctx.showDailyEntry),
    title: "添加今日日志",
    width: "600px",
}));
const __VLS_945 = __VLS_944({
    modelValue: (__VLS_ctx.showDailyEntry),
    title: "添加今日日志",
    width: "600px",
}, ...__VLS_functionalComponentArgsRest(__VLS_944));
const { default: __VLS_948 } = __VLS_946.slots;
let __VLS_949;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_950 = __VLS_asFunctionalComponent1(__VLS_949, new __VLS_949({
    modelValue: (__VLS_ctx.dailyEntryContent),
    type: "textarea",
    rows: (10),
    placeholder: "记录今天的重要事项、学习心得、待办...",
}));
const __VLS_951 = __VLS_950({
    modelValue: (__VLS_ctx.dailyEntryContent),
    type: "textarea",
    rows: (10),
    placeholder: "记录今天的重要事项、学习心得、待办...",
}, ...__VLS_functionalComponentArgsRest(__VLS_950));
{
    const { footer: __VLS_954 } = __VLS_946.slots;
    let __VLS_955;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_956 = __VLS_asFunctionalComponent1(__VLS_955, new __VLS_955({
        ...{ 'onClick': {} },
    }));
    const __VLS_957 = __VLS_956({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_956));
    let __VLS_960;
    const __VLS_961 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showDailyEntry = false;
                // @ts-ignore
                [showDailyEntry, showDailyEntry, dailyEntryContent,];
            } });
    const { default: __VLS_962 } = __VLS_958.slots;
    // @ts-ignore
    [];
    var __VLS_958;
    var __VLS_959;
    let __VLS_963;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_964 = __VLS_asFunctionalComponent1(__VLS_963, new __VLS_963({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_965 = __VLS_964({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_964));
    let __VLS_968;
    const __VLS_969 = ({ click: {} },
        { onClick: (__VLS_ctx.submitDailyEntry) });
    const { default: __VLS_970 } = __VLS_966.slots;
    // @ts-ignore
    [submitDailyEntry,];
    var __VLS_966;
    var __VLS_967;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_946;
// @ts-ignore
[];
var __VLS_560;
let __VLS_971;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_972 = __VLS_asFunctionalComponent1(__VLS_971, new __VLS_971({
    label: "技能",
    name: "skills",
}));
const __VLS_973 = __VLS_972({
    label: "技能",
    name: "skills",
}, ...__VLS_functionalComponentArgsRest(__VLS_972));
const { default: __VLS_976 } = __VLS_974.slots;
const __VLS_977 = SkillStudio;
// @ts-ignore
const __VLS_978 = __VLS_asFunctionalComponent1(__VLS_977, new __VLS_977({
    agentId: (__VLS_ctx.agentId),
    ...{ style: {} },
}));
const __VLS_979 = __VLS_978({
    agentId: (__VLS_ctx.agentId),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_978));
// @ts-ignore
[agentId,];
var __VLS_974;
let __VLS_982;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_983 = __VLS_asFunctionalComponent1(__VLS_982, new __VLS_982({
    label: "定时任务",
    name: "cron",
}));
const __VLS_984 = __VLS_983({
    label: "定时任务",
    name: "cron",
}, ...__VLS_functionalComponentArgsRest(__VLS_983));
const { default: __VLS_987 } = __VLS_985.slots;
let __VLS_988;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_989 = __VLS_asFunctionalComponent1(__VLS_988, new __VLS_988({
    ...{ 'onClick': {} },
    type: "primary",
    ...{ style: {} },
}));
const __VLS_990 = __VLS_989({
    ...{ 'onClick': {} },
    type: "primary",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_989));
let __VLS_993;
const __VLS_994 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.showCronCreate = true;
            // @ts-ignore
            [showCronCreate,];
        } });
const { default: __VLS_995 } = __VLS_991.slots;
let __VLS_996;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_997 = __VLS_asFunctionalComponent1(__VLS_996, new __VLS_996({}));
const __VLS_998 = __VLS_997({}, ...__VLS_functionalComponentArgsRest(__VLS_997));
const { default: __VLS_1001 } = __VLS_999.slots;
let __VLS_1002;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_1003 = __VLS_asFunctionalComponent1(__VLS_1002, new __VLS_1002({}));
const __VLS_1004 = __VLS_1003({}, ...__VLS_functionalComponentArgsRest(__VLS_1003));
// @ts-ignore
[];
var __VLS_999;
// @ts-ignore
[];
var __VLS_991;
var __VLS_992;
let __VLS_1007;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_1008 = __VLS_asFunctionalComponent1(__VLS_1007, new __VLS_1007({
    data: (__VLS_ctx.cronJobs),
    stripe: true,
}));
const __VLS_1009 = __VLS_1008({
    data: (__VLS_ctx.cronJobs),
    stripe: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1008));
const { default: __VLS_1012 } = __VLS_1010.slots;
let __VLS_1013;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1014 = __VLS_asFunctionalComponent1(__VLS_1013, new __VLS_1013({
    prop: "name",
    label: "名称",
}));
const __VLS_1015 = __VLS_1014({
    prop: "name",
    label: "名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_1014));
let __VLS_1018;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1019 = __VLS_asFunctionalComponent1(__VLS_1018, new __VLS_1018({
    label: "调度",
}));
const __VLS_1020 = __VLS_1019({
    label: "调度",
}, ...__VLS_functionalComponentArgsRest(__VLS_1019));
const { default: __VLS_1023 } = __VLS_1021.slots;
{
    const { default: __VLS_1024 } = __VLS_1021.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1024);
    (row.schedule?.expr);
    (row.schedule?.tz);
    // @ts-ignore
    [cronJobs,];
}
// @ts-ignore
[];
var __VLS_1021;
let __VLS_1025;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1026 = __VLS_asFunctionalComponent1(__VLS_1025, new __VLS_1025({
    label: "最近运行",
    width: "180",
}));
const __VLS_1027 = __VLS_1026({
    label: "最近运行",
    width: "180",
}, ...__VLS_functionalComponentArgsRest(__VLS_1026));
const { default: __VLS_1030 } = __VLS_1028.slots;
{
    const { default: __VLS_1031 } = __VLS_1028.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1031);
    if (row.state?.lastRunAtMs) {
        let __VLS_1032;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_1033 = __VLS_asFunctionalComponent1(__VLS_1032, new __VLS_1032({
            type: (row.state?.lastStatus === 'ok' ? 'success' : 'danger'),
            size: "small",
        }));
        const __VLS_1034 = __VLS_1033({
            type: (row.state?.lastStatus === 'ok' ? 'success' : 'danger'),
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1033));
        const { default: __VLS_1037 } = __VLS_1035.slots;
        (row.state?.lastStatus);
        // @ts-ignore
        [];
        var __VLS_1035;
        let __VLS_1038;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_1039 = __VLS_asFunctionalComponent1(__VLS_1038, new __VLS_1038({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_1040 = __VLS_1039({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1039));
        const { default: __VLS_1043 } = __VLS_1041.slots;
        (__VLS_ctx.formatTimestamp(row.state?.lastRunAtMs));
        // @ts-ignore
        [formatTimestamp,];
        var __VLS_1041;
    }
    else {
        let __VLS_1044;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_1045 = __VLS_asFunctionalComponent1(__VLS_1044, new __VLS_1044({
            type: "info",
            size: "small",
        }));
        const __VLS_1046 = __VLS_1045({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1045));
        const { default: __VLS_1049 } = __VLS_1047.slots;
        // @ts-ignore
        [];
        var __VLS_1047;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1028;
let __VLS_1050;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1051 = __VLS_asFunctionalComponent1(__VLS_1050, new __VLS_1050({
    label: "启用",
    width: "80",
}));
const __VLS_1052 = __VLS_1051({
    label: "启用",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_1051));
const { default: __VLS_1055 } = __VLS_1053.slots;
{
    const { default: __VLS_1056 } = __VLS_1053.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1056);
    let __VLS_1057;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_1058 = __VLS_asFunctionalComponent1(__VLS_1057, new __VLS_1057({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
    }));
    const __VLS_1059 = __VLS_1058({
        ...{ 'onChange': {} },
        modelValue: (row.enabled),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1058));
    let __VLS_1062;
    const __VLS_1063 = ({ change: {} },
        { onChange: (...[$event]) => {
                __VLS_ctx.toggleCron(row);
                // @ts-ignore
                [toggleCron,];
            } });
    var __VLS_1060;
    var __VLS_1061;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1053;
let __VLS_1064;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1065 = __VLS_asFunctionalComponent1(__VLS_1064, new __VLS_1064({
    label: "操作",
    width: "270",
}));
const __VLS_1066 = __VLS_1065({
    label: "操作",
    width: "270",
}, ...__VLS_functionalComponentArgsRest(__VLS_1065));
const { default: __VLS_1069 } = __VLS_1067.slots;
{
    const { default: __VLS_1070 } = __VLS_1067.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1070);
    if (row.payload?.message === '__MEMORY_CONSOLIDATE__') {
        let __VLS_1071;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_1072 = __VLS_asFunctionalComponent1(__VLS_1071, new __VLS_1071({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_1073 = __VLS_1072({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1072));
        const { default: __VLS_1076 } = __VLS_1074.slots;
        // @ts-ignore
        [];
        var __VLS_1074;
        let __VLS_1077;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1078 = __VLS_asFunctionalComponent1(__VLS_1077, new __VLS_1077({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_1079 = __VLS_1078({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1078));
        let __VLS_1082;
        const __VLS_1083 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(row.payload?.message === '__MEMORY_CONSOLIDATE__'))
                        return;
                    __VLS_ctx.runCronNow(row);
                    // @ts-ignore
                    [runCronNow,];
                } });
        const { default: __VLS_1084 } = __VLS_1080.slots;
        // @ts-ignore
        [];
        var __VLS_1080;
        var __VLS_1081;
    }
    else {
        let __VLS_1085;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1086 = __VLS_asFunctionalComponent1(__VLS_1085, new __VLS_1085({
            ...{ 'onClick': {} },
            size: "small",
        }));
        const __VLS_1087 = __VLS_1086({
            ...{ 'onClick': {} },
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1086));
        let __VLS_1090;
        const __VLS_1091 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(row.payload?.message === '__MEMORY_CONSOLIDATE__'))
                        return;
                    __VLS_ctx.runCronNow(row);
                    // @ts-ignore
                    [runCronNow,];
                } });
        const { default: __VLS_1092 } = __VLS_1088.slots;
        // @ts-ignore
        [];
        var __VLS_1088;
        var __VLS_1089;
        let __VLS_1093;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1094 = __VLS_asFunctionalComponent1(__VLS_1093, new __VLS_1093({
            ...{ 'onClick': {} },
            size: "small",
            type: "info",
        }));
        const __VLS_1095 = __VLS_1094({
            ...{ 'onClick': {} },
            size: "small",
            type: "info",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1094));
        let __VLS_1098;
        const __VLS_1099 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(row.payload?.message === '__MEMORY_CONSOLIDATE__'))
                        return;
                    __VLS_ctx.openCronLogs(row);
                    // @ts-ignore
                    [openCronLogs,];
                } });
        const { default: __VLS_1100 } = __VLS_1096.slots;
        // @ts-ignore
        [];
        var __VLS_1096;
        var __VLS_1097;
        let __VLS_1101;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1102 = __VLS_asFunctionalComponent1(__VLS_1101, new __VLS_1101({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
        }));
        const __VLS_1103 = __VLS_1102({
            ...{ 'onClick': {} },
            size: "small",
            type: "danger",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1102));
        let __VLS_1106;
        const __VLS_1107 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(row.payload?.message === '__MEMORY_CONSOLIDATE__'))
                        return;
                    __VLS_ctx.deleteCron(row);
                    // @ts-ignore
                    [deleteCron,];
                } });
        const { default: __VLS_1108 } = __VLS_1104.slots;
        // @ts-ignore
        [];
        var __VLS_1104;
        var __VLS_1105;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1067;
// @ts-ignore
[];
var __VLS_1010;
let __VLS_1109;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_1110 = __VLS_asFunctionalComponent1(__VLS_1109, new __VLS_1109({
    modelValue: (__VLS_ctx.showCronCreate),
    title: "新建定时任务",
    width: "520px",
}));
const __VLS_1111 = __VLS_1110({
    modelValue: (__VLS_ctx.showCronCreate),
    title: "新建定时任务",
    width: "520px",
}, ...__VLS_functionalComponentArgsRest(__VLS_1110));
const { default: __VLS_1114 } = __VLS_1112.slots;
let __VLS_1115;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_1116 = __VLS_asFunctionalComponent1(__VLS_1115, new __VLS_1115({
    model: (__VLS_ctx.cronForm),
    labelWidth: "100px",
}));
const __VLS_1117 = __VLS_1116({
    model: (__VLS_ctx.cronForm),
    labelWidth: "100px",
}, ...__VLS_functionalComponentArgsRest(__VLS_1116));
const { default: __VLS_1120 } = __VLS_1118.slots;
let __VLS_1121;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1122 = __VLS_asFunctionalComponent1(__VLS_1121, new __VLS_1121({
    label: "名称",
}));
const __VLS_1123 = __VLS_1122({
    label: "名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_1122));
const { default: __VLS_1126 } = __VLS_1124.slots;
let __VLS_1127;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1128 = __VLS_asFunctionalComponent1(__VLS_1127, new __VLS_1127({
    modelValue: (__VLS_ctx.cronForm.name),
}));
const __VLS_1129 = __VLS_1128({
    modelValue: (__VLS_ctx.cronForm.name),
}, ...__VLS_functionalComponentArgsRest(__VLS_1128));
// @ts-ignore
[showCronCreate, cronForm, cronForm,];
var __VLS_1124;
let __VLS_1132;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1133 = __VLS_asFunctionalComponent1(__VLS_1132, new __VLS_1132({
    label: "Cron 表达式",
}));
const __VLS_1134 = __VLS_1133({
    label: "Cron 表达式",
}, ...__VLS_functionalComponentArgsRest(__VLS_1133));
const { default: __VLS_1137 } = __VLS_1135.slots;
let __VLS_1138;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1139 = __VLS_asFunctionalComponent1(__VLS_1138, new __VLS_1138({
    modelValue: (__VLS_ctx.cronForm.expr),
    placeholder: "30 3 * * *",
}));
const __VLS_1140 = __VLS_1139({
    modelValue: (__VLS_ctx.cronForm.expr),
    placeholder: "30 3 * * *",
}, ...__VLS_functionalComponentArgsRest(__VLS_1139));
// @ts-ignore
[cronForm,];
var __VLS_1135;
let __VLS_1143;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1144 = __VLS_asFunctionalComponent1(__VLS_1143, new __VLS_1143({
    label: "时区",
}));
const __VLS_1145 = __VLS_1144({
    label: "时区",
}, ...__VLS_functionalComponentArgsRest(__VLS_1144));
const { default: __VLS_1148 } = __VLS_1146.slots;
let __VLS_1149;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_1150 = __VLS_asFunctionalComponent1(__VLS_1149, new __VLS_1149({
    modelValue: (__VLS_ctx.cronForm.tz),
}));
const __VLS_1151 = __VLS_1150({
    modelValue: (__VLS_ctx.cronForm.tz),
}, ...__VLS_functionalComponentArgsRest(__VLS_1150));
const { default: __VLS_1154 } = __VLS_1152.slots;
let __VLS_1155;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1156 = __VLS_asFunctionalComponent1(__VLS_1155, new __VLS_1155({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}));
const __VLS_1157 = __VLS_1156({
    label: "Asia/Shanghai",
    value: "Asia/Shanghai",
}, ...__VLS_functionalComponentArgsRest(__VLS_1156));
let __VLS_1160;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1161 = __VLS_asFunctionalComponent1(__VLS_1160, new __VLS_1160({
    label: "UTC",
    value: "UTC",
}));
const __VLS_1162 = __VLS_1161({
    label: "UTC",
    value: "UTC",
}, ...__VLS_functionalComponentArgsRest(__VLS_1161));
let __VLS_1165;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1166 = __VLS_asFunctionalComponent1(__VLS_1165, new __VLS_1165({
    label: "America/New_York",
    value: "America/New_York",
}));
const __VLS_1167 = __VLS_1166({
    label: "America/New_York",
    value: "America/New_York",
}, ...__VLS_functionalComponentArgsRest(__VLS_1166));
// @ts-ignore
[cronForm,];
var __VLS_1152;
// @ts-ignore
[];
var __VLS_1146;
let __VLS_1170;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1171 = __VLS_asFunctionalComponent1(__VLS_1170, new __VLS_1170({
    label: "消息",
}));
const __VLS_1172 = __VLS_1171({
    label: "消息",
}, ...__VLS_functionalComponentArgsRest(__VLS_1171));
const { default: __VLS_1175 } = __VLS_1173.slots;
let __VLS_1176;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1177 = __VLS_asFunctionalComponent1(__VLS_1176, new __VLS_1176({
    modelValue: (__VLS_ctx.cronForm.message),
    type: "textarea",
    rows: (3),
}));
const __VLS_1178 = __VLS_1177({
    modelValue: (__VLS_ctx.cronForm.message),
    type: "textarea",
    rows: (3),
}, ...__VLS_functionalComponentArgsRest(__VLS_1177));
// @ts-ignore
[cronForm,];
var __VLS_1173;
let __VLS_1181;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1182 = __VLS_asFunctionalComponent1(__VLS_1181, new __VLS_1181({
    label: "启用",
}));
const __VLS_1183 = __VLS_1182({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_1182));
const { default: __VLS_1186 } = __VLS_1184.slots;
let __VLS_1187;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_1188 = __VLS_asFunctionalComponent1(__VLS_1187, new __VLS_1187({
    modelValue: (__VLS_ctx.cronForm.enabled),
}));
const __VLS_1189 = __VLS_1188({
    modelValue: (__VLS_ctx.cronForm.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_1188));
// @ts-ignore
[cronForm,];
var __VLS_1184;
// @ts-ignore
[];
var __VLS_1118;
{
    const { footer: __VLS_1192 } = __VLS_1112.slots;
    let __VLS_1193;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1194 = __VLS_asFunctionalComponent1(__VLS_1193, new __VLS_1193({
        ...{ 'onClick': {} },
    }));
    const __VLS_1195 = __VLS_1194({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1194));
    let __VLS_1198;
    const __VLS_1199 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showCronCreate = false;
                // @ts-ignore
                [showCronCreate,];
            } });
    const { default: __VLS_1200 } = __VLS_1196.slots;
    // @ts-ignore
    [];
    var __VLS_1196;
    var __VLS_1197;
    let __VLS_1201;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1202 = __VLS_asFunctionalComponent1(__VLS_1201, new __VLS_1201({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_1203 = __VLS_1202({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1202));
    let __VLS_1206;
    const __VLS_1207 = ({ click: {} },
        { onClick: (__VLS_ctx.createCron) });
    const { default: __VLS_1208 } = __VLS_1204.slots;
    // @ts-ignore
    [createCron,];
    var __VLS_1204;
    var __VLS_1205;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1112;
let __VLS_1209;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_1210 = __VLS_asFunctionalComponent1(__VLS_1209, new __VLS_1209({
    modelValue: (__VLS_ctx.showCronLogs),
    title: (`执行日志 — ${__VLS_ctx.cronLogsJob?.name}`),
    width: "780px",
}));
const __VLS_1211 = __VLS_1210({
    modelValue: (__VLS_ctx.showCronLogs),
    title: (`执行日志 — ${__VLS_ctx.cronLogsJob?.name}`),
    width: "780px",
}, ...__VLS_functionalComponentArgsRest(__VLS_1210));
const { default: __VLS_1214 } = __VLS_1212.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_1215;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_1216 = __VLS_asFunctionalComponent1(__VLS_1215, new __VLS_1215({
    type: "info",
    size: "small",
}));
const __VLS_1217 = __VLS_1216({
    type: "info",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_1216));
const { default: __VLS_1220 } = __VLS_1218.slots;
// @ts-ignore
[showCronLogs, cronLogsJob,];
var __VLS_1218;
let __VLS_1221;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1222 = __VLS_asFunctionalComponent1(__VLS_1221, new __VLS_1221({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loadingCronLogs),
}));
const __VLS_1223 = __VLS_1222({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.loadingCronLogs),
}, ...__VLS_functionalComponentArgsRest(__VLS_1222));
let __VLS_1226;
const __VLS_1227 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.openCronLogs(__VLS_ctx.cronLogsJob);
            // @ts-ignore
            [openCronLogs, cronLogsJob, loadingCronLogs,];
        } });
const { default: __VLS_1228 } = __VLS_1224.slots;
// @ts-ignore
[];
var __VLS_1224;
var __VLS_1225;
let __VLS_1229;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_1230 = __VLS_asFunctionalComponent1(__VLS_1229, new __VLS_1229({
    data: (__VLS_ctx.cronLogs),
    stripe: true,
    size: "small",
    maxHeight: "460",
}));
const __VLS_1231 = __VLS_1230({
    data: (__VLS_ctx.cronLogs),
    stripe: true,
    size: "small",
    maxHeight: "460",
}, ...__VLS_functionalComponentArgsRest(__VLS_1230));
__VLS_asFunctionalDirective(__VLS_directives.vLoading, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.loadingCronLogs) }, null, null);
const { default: __VLS_1234 } = __VLS_1232.slots;
let __VLS_1235;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1236 = __VLS_asFunctionalComponent1(__VLS_1235, new __VLS_1235({
    label: "运行时间",
    width: "170",
}));
const __VLS_1237 = __VLS_1236({
    label: "运行时间",
    width: "170",
}, ...__VLS_functionalComponentArgsRest(__VLS_1236));
const { default: __VLS_1240 } = __VLS_1238.slots;
{
    const { default: __VLS_1241 } = __VLS_1238.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1241);
    (new Date(row.startedAt).toLocaleString('zh-CN'));
    // @ts-ignore
    [vLoading, loadingCronLogs, cronLogs,];
}
// @ts-ignore
[];
var __VLS_1238;
let __VLS_1242;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1243 = __VLS_asFunctionalComponent1(__VLS_1242, new __VLS_1242({
    label: "耗时",
    width: "80",
}));
const __VLS_1244 = __VLS_1243({
    label: "耗时",
    width: "80",
}, ...__VLS_functionalComponentArgsRest(__VLS_1243));
const { default: __VLS_1247 } = __VLS_1245.slots;
{
    const { default: __VLS_1248 } = __VLS_1245.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1248);
    let __VLS_1249;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_1250 = __VLS_asFunctionalComponent1(__VLS_1249, new __VLS_1249({
        size: "small",
    }));
    const __VLS_1251 = __VLS_1250({
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1250));
    const { default: __VLS_1254 } = __VLS_1252.slots;
    (((row.endedAt - row.startedAt) / 1000).toFixed(1));
    // @ts-ignore
    [];
    var __VLS_1252;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1245;
let __VLS_1255;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1256 = __VLS_asFunctionalComponent1(__VLS_1255, new __VLS_1255({
    label: "状态",
    width: "75",
}));
const __VLS_1257 = __VLS_1256({
    label: "状态",
    width: "75",
}, ...__VLS_functionalComponentArgsRest(__VLS_1256));
const { default: __VLS_1260 } = __VLS_1258.slots;
{
    const { default: __VLS_1261 } = __VLS_1258.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1261);
    let __VLS_1262;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_1263 = __VLS_asFunctionalComponent1(__VLS_1262, new __VLS_1262({
        type: (row.status === 'ok' ? 'success' : 'danger'),
        size: "small",
    }));
    const __VLS_1264 = __VLS_1263({
        type: (row.status === 'ok' ? 'success' : 'danger'),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1263));
    const { default: __VLS_1267 } = __VLS_1265.slots;
    (row.status);
    // @ts-ignore
    [];
    var __VLS_1265;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1258;
let __VLS_1268;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1269 = __VLS_asFunctionalComponent1(__VLS_1268, new __VLS_1268({
    label: "推送",
    width: "60",
}));
const __VLS_1270 = __VLS_1269({
    label: "推送",
    width: "60",
}, ...__VLS_functionalComponentArgsRest(__VLS_1269));
const { default: __VLS_1273 } = __VLS_1271.slots;
{
    const { default: __VLS_1274 } = __VLS_1271.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1274);
    if (row.announced) {
        let __VLS_1275;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_1276 = __VLS_asFunctionalComponent1(__VLS_1275, new __VLS_1275({
            type: "success",
            size: "small",
            effect: "plain",
        }));
        const __VLS_1277 = __VLS_1276({
            type: "success",
            size: "small",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1276));
        const { default: __VLS_1280 } = __VLS_1278.slots;
        // @ts-ignore
        [];
        var __VLS_1278;
    }
    else {
        let __VLS_1281;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_1282 = __VLS_asFunctionalComponent1(__VLS_1281, new __VLS_1281({
            type: "info",
            size: "small",
        }));
        const __VLS_1283 = __VLS_1282({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1282));
        const { default: __VLS_1286 } = __VLS_1284.slots;
        // @ts-ignore
        [];
        var __VLS_1284;
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1271;
let __VLS_1287;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1288 = __VLS_asFunctionalComponent1(__VLS_1287, new __VLS_1287({
    label: "输出 / 错误",
    minWidth: "200",
}));
const __VLS_1289 = __VLS_1288({
    label: "输出 / 错误",
    minWidth: "200",
}, ...__VLS_functionalComponentArgsRest(__VLS_1288));
const { default: __VLS_1292 } = __VLS_1290.slots;
{
    const { default: __VLS_1293 } = __VLS_1290.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1293);
    if (row.status === 'error') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        (row.error);
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        (row.output || '—');
    }
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1290;
// @ts-ignore
[];
var __VLS_1232;
if (!__VLS_ctx.loadingCronLogs && __VLS_ctx.cronLogs.length === 0) {
    let __VLS_1294;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_1295 = __VLS_asFunctionalComponent1(__VLS_1294, new __VLS_1294({
        description: "暂无执行记录",
    }));
    const __VLS_1296 = __VLS_1295({
        description: "暂无执行记录",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1295));
}
{
    const { footer: __VLS_1299 } = __VLS_1212.slots;
    let __VLS_1300;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1301 = __VLS_asFunctionalComponent1(__VLS_1300, new __VLS_1300({
        ...{ 'onClick': {} },
    }));
    const __VLS_1302 = __VLS_1301({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1301));
    let __VLS_1305;
    const __VLS_1306 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showCronLogs = false;
                // @ts-ignore
                [showCronLogs, loadingCronLogs, cronLogs,];
            } });
    const { default: __VLS_1307 } = __VLS_1303.slots;
    // @ts-ignore
    [];
    var __VLS_1303;
    var __VLS_1304;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1212;
let __VLS_1308;
/** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
elDivider;
// @ts-ignore
const __VLS_1309 = __VLS_asFunctionalComponent1(__VLS_1308, new __VLS_1308({}));
const __VLS_1310 = __VLS_1309({}, ...__VLS_functionalComponentArgsRest(__VLS_1309));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h3, __VLS_intrinsics.h3)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
let __VLS_1313;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_1314 = __VLS_asFunctionalComponent1(__VLS_1313, new __VLS_1313({
    labelWidth: "90px",
    size: "default",
}));
const __VLS_1315 = __VLS_1314({
    labelWidth: "90px",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_1314));
const { default: __VLS_1318 } = __VLS_1316.slots;
let __VLS_1319;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1320 = __VLS_asFunctionalComponent1(__VLS_1319, new __VLS_1319({
    label: "启用心跳",
}));
const __VLS_1321 = __VLS_1320({
    label: "启用心跳",
}, ...__VLS_functionalComponentArgsRest(__VLS_1320));
const { default: __VLS_1324 } = __VLS_1322.slots;
let __VLS_1325;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_1326 = __VLS_asFunctionalComponent1(__VLS_1325, new __VLS_1325({
    modelValue: (__VLS_ctx.heartbeatForm.enabled),
}));
const __VLS_1327 = __VLS_1326({
    modelValue: (__VLS_ctx.heartbeatForm.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_1326));
// @ts-ignore
[heartbeatForm,];
var __VLS_1322;
if (__VLS_ctx.heartbeatForm.enabled) {
    let __VLS_1330;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1331 = __VLS_asFunctionalComponent1(__VLS_1330, new __VLS_1330({
        label: "间隔（分钟）",
    }));
    const __VLS_1332 = __VLS_1331({
        label: "间隔（分钟）",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1331));
    const { default: __VLS_1335 } = __VLS_1333.slots;
    let __VLS_1336;
    /** @ts-ignore @type { | typeof __VLS_components.elInputNumber | typeof __VLS_components.ElInputNumber | typeof __VLS_components['el-input-number']} */
    elInputNumber;
    // @ts-ignore
    const __VLS_1337 = __VLS_asFunctionalComponent1(__VLS_1336, new __VLS_1336({
        modelValue: (__VLS_ctx.heartbeatForm.intervalMin),
        min: (1),
        max: (1440),
        ...{ style: {} },
    }));
    const __VLS_1338 = __VLS_1337({
        modelValue: (__VLS_ctx.heartbeatForm.intervalMin),
        min: (1),
        max: (1440),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1337));
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    // @ts-ignore
    [heartbeatForm, heartbeatForm,];
    var __VLS_1333;
    let __VLS_1341;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1342 = __VLS_asFunctionalComponent1(__VLS_1341, new __VLS_1341({
        label: "Prompt",
    }));
    const __VLS_1343 = __VLS_1342({
        label: "Prompt",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1342));
    const { default: __VLS_1346 } = __VLS_1344.slots;
    let __VLS_1347;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1348 = __VLS_asFunctionalComponent1(__VLS_1347, new __VLS_1347({
        modelValue: (__VLS_ctx.heartbeatForm.prompt),
        type: "textarea",
        rows: (3),
        placeholder: "留空使用默认 prompt（读取 HEARTBEAT.md，无事则 HEARTBEAT_OK）",
        ...{ style: {} },
    }));
    const __VLS_1349 = __VLS_1348({
        modelValue: (__VLS_ctx.heartbeatForm.prompt),
        type: "textarea",
        rows: (3),
        placeholder: "留空使用默认 prompt（读取 HEARTBEAT.md，无事则 HEARTBEAT_OK）",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1348));
    // @ts-ignore
    [heartbeatForm,];
    var __VLS_1344;
}
let __VLS_1352;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1353 = __VLS_asFunctionalComponent1(__VLS_1352, new __VLS_1352({
    label: "",
}));
const __VLS_1354 = __VLS_1353({
    label: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_1353));
const { default: __VLS_1357 } = __VLS_1355.slots;
let __VLS_1358;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1359 = __VLS_asFunctionalComponent1(__VLS_1358, new __VLS_1358({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.heartbeatSaving),
}));
const __VLS_1360 = __VLS_1359({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.heartbeatSaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_1359));
let __VLS_1363;
const __VLS_1364 = ({ click: {} },
    { onClick: (__VLS_ctx.saveHeartbeat) });
const { default: __VLS_1365 } = __VLS_1361.slots;
// @ts-ignore
[heartbeatSaving, saveHeartbeat,];
var __VLS_1361;
var __VLS_1362;
if (__VLS_ctx.heartbeatSaved) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
// @ts-ignore
[heartbeatSaved,];
var __VLS_1355;
// @ts-ignore
[];
var __VLS_1316;
// @ts-ignore
[];
var __VLS_985;
let __VLS_1366;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_1367 = __VLS_asFunctionalComponent1(__VLS_1366, new __VLS_1366({
    label: "渠道",
    name: "channels",
}));
const __VLS_1368 = __VLS_1367({
    label: "渠道",
    name: "channels",
}, ...__VLS_functionalComponentArgsRest(__VLS_1367));
const { default: __VLS_1371 } = __VLS_1369.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
let __VLS_1372;
/** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
elText;
// @ts-ignore
const __VLS_1373 = __VLS_asFunctionalComponent1(__VLS_1372, new __VLS_1372({
    type: "info",
    size: "small",
}));
const __VLS_1374 = __VLS_1373({
    type: "info",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_1373));
const { default: __VLS_1377 } = __VLS_1375.slots;
// @ts-ignore
[];
var __VLS_1375;
let __VLS_1378;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1379 = __VLS_asFunctionalComponent1(__VLS_1378, new __VLS_1378({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
}));
const __VLS_1380 = __VLS_1379({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_1379));
let __VLS_1383;
const __VLS_1384 = ({ click: {} },
    { onClick: (__VLS_ctx.openAddChannel) });
const { default: __VLS_1385 } = __VLS_1381.slots;
let __VLS_1386;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_1387 = __VLS_asFunctionalComponent1(__VLS_1386, new __VLS_1386({}));
const __VLS_1388 = __VLS_1387({}, ...__VLS_functionalComponentArgsRest(__VLS_1387));
const { default: __VLS_1391 } = __VLS_1389.slots;
let __VLS_1392;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_1393 = __VLS_asFunctionalComponent1(__VLS_1392, new __VLS_1392({}));
const __VLS_1394 = __VLS_1393({}, ...__VLS_functionalComponentArgsRest(__VLS_1393));
// @ts-ignore
[openAddChannel,];
var __VLS_1389;
// @ts-ignore
[];
var __VLS_1381;
var __VLS_1382;
for (const [ch] of __VLS_vFor((__VLS_ctx.agentChannelList))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        key: (ch.id),
        ...{ class: "channel-card" },
    });
    /** @type {__VLS_StyleScopedClasses['channel-card']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "channel-card-header" },
    });
    /** @type {__VLS_StyleScopedClasses['channel-card-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "channel-card-left" },
    });
    /** @type {__VLS_StyleScopedClasses['channel-card-left']} */ ;
    let __VLS_1397;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_1398 = __VLS_asFunctionalComponent1(__VLS_1397, new __VLS_1397({
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_1399 = __VLS_1398({
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1398));
    const { default: __VLS_1402 } = __VLS_1400.slots;
    (ch.type);
    // @ts-ignore
    [agentChannelList,];
    var __VLS_1400;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "channel-card-name" },
    });
    /** @type {__VLS_StyleScopedClasses['channel-card-name']} */ ;
    (ch.name);
    if (ch.config?.botName) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-bot-username" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-bot-username']} */ ;
        (ch.config.botName);
    }
    let __VLS_1403;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_1404 = __VLS_asFunctionalComponent1(__VLS_1403, new __VLS_1403({
        type: (ch.status === 'ok' ? 'success' : ch.status === 'error' ? 'danger' : 'info'),
        size: "small",
        effect: "plain",
        ...{ style: {} },
    }));
    const __VLS_1405 = __VLS_1404({
        type: (ch.status === 'ok' ? 'success' : ch.status === 'error' ? 'danger' : 'info'),
        size: "small",
        effect: "plain",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1404));
    const { default: __VLS_1408 } = __VLS_1406.slots;
    (ch.status === 'ok' ? '✓ 正常' : ch.status === 'error' ? '✗ 错误' : '未测试');
    // @ts-ignore
    [];
    var __VLS_1406;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "channel-card-actions" },
    });
    /** @type {__VLS_StyleScopedClasses['channel-card-actions']} */ ;
    let __VLS_1409;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_1410 = __VLS_asFunctionalComponent1(__VLS_1409, new __VLS_1409({
        ...{ 'onChange': {} },
        modelValue: (ch.enabled),
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_1411 = __VLS_1410({
        ...{ 'onChange': {} },
        modelValue: (ch.enabled),
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1410));
    let __VLS_1414;
    const __VLS_1415 = ({ change: {} },
        { onChange: (__VLS_ctx.saveChannels) });
    var __VLS_1412;
    var __VLS_1413;
    let __VLS_1416;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1417 = __VLS_asFunctionalComponent1(__VLS_1416, new __VLS_1416({
        ...{ 'onClick': {} },
        size: "small",
        loading: (__VLS_ctx.testingChannelId === ch.id),
    }));
    const __VLS_1418 = __VLS_1417({
        ...{ 'onClick': {} },
        size: "small",
        loading: (__VLS_ctx.testingChannelId === ch.id),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1417));
    let __VLS_1421;
    const __VLS_1422 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.testAgentChannel(ch);
                // @ts-ignore
                [saveChannels, testingChannelId, testAgentChannel,];
            } });
    const { default: __VLS_1423 } = __VLS_1419.slots;
    // @ts-ignore
    [];
    var __VLS_1419;
    var __VLS_1420;
    let __VLS_1424;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1425 = __VLS_asFunctionalComponent1(__VLS_1424, new __VLS_1424({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_1426 = __VLS_1425({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1425));
    let __VLS_1429;
    const __VLS_1430 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.openEditChannel(ch);
                // @ts-ignore
                [openEditChannel,];
            } });
    const { default: __VLS_1431 } = __VLS_1427.slots;
    // @ts-ignore
    [];
    var __VLS_1427;
    var __VLS_1428;
    let __VLS_1432;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1433 = __VLS_asFunctionalComponent1(__VLS_1432, new __VLS_1432({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        plain: true,
    }));
    const __VLS_1434 = __VLS_1433({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        plain: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_1433));
    let __VLS_1437;
    const __VLS_1438 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.deleteAgentChannel(ch);
                // @ts-ignore
                [deleteAgentChannel,];
            } });
    const { default: __VLS_1439 } = __VLS_1435.slots;
    // @ts-ignore
    [];
    var __VLS_1435;
    var __VLS_1436;
    if (ch.type === 'web') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-card-body" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-card-body']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-info-row" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-label" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-value" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-value']} */ ;
        let __VLS_1440;
        /** @ts-ignore @type { | typeof __VLS_components.elLink | typeof __VLS_components.ElLink | typeof __VLS_components['el-link'] | typeof __VLS_components.elLink | typeof __VLS_components.ElLink | typeof __VLS_components['el-link']} */
        elLink;
        // @ts-ignore
        const __VLS_1441 = __VLS_asFunctionalComponent1(__VLS_1440, new __VLS_1440({
            href: (__VLS_ctx.webChatUrl(__VLS_ctx.agentId, ch.id)),
            target: "_blank",
            type: "primary",
            ...{ style: {} },
        }));
        const __VLS_1442 = __VLS_1441({
            href: (__VLS_ctx.webChatUrl(__VLS_ctx.agentId, ch.id)),
            target: "_blank",
            type: "primary",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1441));
        const { default: __VLS_1445 } = __VLS_1443.slots;
        (__VLS_ctx.webChatUrl(__VLS_ctx.agentId, ch.id));
        // @ts-ignore
        [agentId, agentId, webChatUrl, webChatUrl,];
        var __VLS_1443;
        let __VLS_1446;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1447 = __VLS_asFunctionalComponent1(__VLS_1446, new __VLS_1446({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }));
        const __VLS_1448 = __VLS_1447({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1447));
        let __VLS_1451;
        const __VLS_1452 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(ch.type === 'web'))
                        return;
                    __VLS_ctx.copyUrl(__VLS_ctx.webChatUrl(__VLS_ctx.agentId, ch.id));
                    // @ts-ignore
                    [agentId, webChatUrl, copyUrl,];
                } });
        const { default: __VLS_1453 } = __VLS_1449.slots;
        // @ts-ignore
        [];
        var __VLS_1449;
        var __VLS_1450;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-info-row" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-label" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-value" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-value']} */ ;
        let __VLS_1454;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_1455 = __VLS_asFunctionalComponent1(__VLS_1454, new __VLS_1454({
            size: "small",
            type: (ch.config?.password ? 'warning' : 'info'),
            effect: "plain",
        }));
        const __VLS_1456 = __VLS_1455({
            size: "small",
            type: (ch.config?.password ? 'warning' : 'info'),
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1455));
        const { default: __VLS_1459 } = __VLS_1457.slots;
        (ch.config?.password ? '已设置' : '无密码');
        // @ts-ignore
        [];
        var __VLS_1457;
    }
    if (ch.type === 'telegram' || ch.type === 'feishu') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-card-body" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-card-body']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-info-row" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-label" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-label']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-info-value" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-info-value']} */ ;
        if (ch.allowedFromUsers?.length) {
            for (const [u] of __VLS_vFor((ch.allowedFromUsers))) {
                let __VLS_1460;
                /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
                elTag;
                // @ts-ignore
                const __VLS_1461 = __VLS_asFunctionalComponent1(__VLS_1460, new __VLS_1460({
                    ...{ 'onClose': {} },
                    key: (u.id),
                    size: "small",
                    closable: true,
                    disableTransitions: (true),
                    ...{ style: {} },
                }));
                const __VLS_1462 = __VLS_1461({
                    ...{ 'onClose': {} },
                    key: (u.id),
                    size: "small",
                    closable: true,
                    disableTransitions: (true),
                    ...{ style: {} },
                }, ...__VLS_functionalComponentArgsRest(__VLS_1461));
                let __VLS_1465;
                const __VLS_1466 = ({ close: {} },
                    { onClose: (...[$event]) => {
                            if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                                return;
                            if (!(ch.allowedFromUsers?.length))
                                return;
                            __VLS_ctx.removeAllowed(ch.id, u.id);
                            // @ts-ignore
                            [removeAllowed,];
                        } });
                const { default: __VLS_1467 } = __VLS_1463.slots;
                (u.username ? '@' + u.username : u.firstName || String(u.id));
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ style: {} },
                });
                (u.id);
                // @ts-ignore
                [];
                var __VLS_1463;
                var __VLS_1464;
                // @ts-ignore
                [];
            }
        }
        else if (ch.config?.allowedFrom) {
            for (const [uid] of __VLS_vFor((ch.config.allowedFrom.split(',')))) {
                let __VLS_1468;
                /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
                elTag;
                // @ts-ignore
                const __VLS_1469 = __VLS_asFunctionalComponent1(__VLS_1468, new __VLS_1468({
                    ...{ 'onClose': {} },
                    key: (uid),
                    size: "small",
                    closable: true,
                    disableTransitions: (true),
                    ...{ style: {} },
                }));
                const __VLS_1470 = __VLS_1469({
                    ...{ 'onClose': {} },
                    key: (uid),
                    size: "small",
                    closable: true,
                    disableTransitions: (true),
                    ...{ style: {} },
                }, ...__VLS_functionalComponentArgsRest(__VLS_1469));
                let __VLS_1473;
                const __VLS_1474 = ({ close: {} },
                    { onClose: (...[$event]) => {
                            if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                                return;
                            if (!!(ch.allowedFromUsers?.length))
                                return;
                            if (!(ch.config?.allowedFrom))
                                return;
                            __VLS_ctx.removeAllowed(ch.id, ch.type === 'feishu' ? uid.trim() : Number(uid.trim()));
                            // @ts-ignore
                            [removeAllowed,];
                        } });
                const { default: __VLS_1475 } = __VLS_1471.slots;
                (uid.trim());
                // @ts-ignore
                [];
                var __VLS_1471;
                var __VLS_1472;
                // @ts-ignore
                [];
            }
        }
        else {
            let __VLS_1476;
            /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
            elText;
            // @ts-ignore
            const __VLS_1477 = __VLS_asFunctionalComponent1(__VLS_1476, new __VLS_1476({
                type: "warning",
                size: "small",
            }));
            const __VLS_1478 = __VLS_1477({
                type: "warning",
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_1477));
            const { default: __VLS_1481 } = __VLS_1479.slots;
            (ch.type === 'feishu' ? '未设置（配对模式，向用户返回其 Open ID）' : '未设置（配对模式，向用户返回其 ID）');
            // @ts-ignore
            [];
            var __VLS_1479;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "pending-section" },
        });
        /** @type {__VLS_StyleScopedClasses['pending-section']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (...[$event]) => {
                    if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                        return;
                    __VLS_ctx.togglePending(ch.id);
                    // @ts-ignore
                    [togglePending,];
                } },
            ...{ class: "pending-section-header" },
        });
        /** @type {__VLS_StyleScopedClasses['pending-section-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        let __VLS_1482;
        /** @ts-ignore @type { | typeof __VLS_components.elBadge | typeof __VLS_components.ElBadge | typeof __VLS_components['el-badge']} */
        elBadge;
        // @ts-ignore
        const __VLS_1483 = __VLS_asFunctionalComponent1(__VLS_1482, new __VLS_1482({
            value: ((__VLS_ctx.pendingUsers[ch.id] || []).length),
            hidden: (!(__VLS_ctx.pendingUsers[ch.id] || []).length),
            type: "warning",
            ...{ style: {} },
        }));
        const __VLS_1484 = __VLS_1483({
            value: ((__VLS_ctx.pendingUsers[ch.id] || []).length),
            hidden: (!(__VLS_ctx.pendingUsers[ch.id] || []).length),
            type: "warning",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1483));
        let __VLS_1487;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1488 = __VLS_asFunctionalComponent1(__VLS_1487, new __VLS_1487({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }));
        const __VLS_1489 = __VLS_1488({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1488));
        let __VLS_1492;
        const __VLS_1493 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                        return;
                    __VLS_ctx.loadPendingUsers(ch.id);
                    // @ts-ignore
                    [pendingUsers, pendingUsers, loadPendingUsers,];
                } });
        const { default: __VLS_1494 } = __VLS_1490.slots;
        // @ts-ignore
        [];
        var __VLS_1490;
        var __VLS_1491;
        let __VLS_1495;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1496 = __VLS_asFunctionalComponent1(__VLS_1495, new __VLS_1495({
            ...{ style: {} },
            ...{ style: ({ transform: __VLS_ctx.expandedPending === ch.id ? 'rotate(180deg)' : '' }) },
        }));
        const __VLS_1497 = __VLS_1496({
            ...{ style: {} },
            ...{ style: ({ transform: __VLS_ctx.expandedPending === ch.id ? 'rotate(180deg)' : '' }) },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1496));
        const { default: __VLS_1500 } = __VLS_1498.slots;
        let __VLS_1501;
        /** @ts-ignore @type { | typeof __VLS_components.ArrowDown} */
        ArrowDown;
        // @ts-ignore
        const __VLS_1502 = __VLS_asFunctionalComponent1(__VLS_1501, new __VLS_1501({}));
        const __VLS_1503 = __VLS_1502({}, ...__VLS_functionalComponentArgsRest(__VLS_1502));
        // @ts-ignore
        [expandedPending,];
        var __VLS_1498;
        if (__VLS_ctx.expandedPending === ch.id) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "pending-list" },
            });
            /** @type {__VLS_StyleScopedClasses['pending-list']} */ ;
            if (__VLS_ctx.pendingLoading[ch.id]) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ style: {} },
                });
                let __VLS_1506;
                /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
                elText;
                // @ts-ignore
                const __VLS_1507 = __VLS_asFunctionalComponent1(__VLS_1506, new __VLS_1506({
                    type: "info",
                    size: "small",
                }));
                const __VLS_1508 = __VLS_1507({
                    type: "info",
                    size: "small",
                }, ...__VLS_functionalComponentArgsRest(__VLS_1507));
                const { default: __VLS_1511 } = __VLS_1509.slots;
                // @ts-ignore
                [expandedPending, pendingLoading,];
                var __VLS_1509;
            }
            else if ((__VLS_ctx.pendingUsers[ch.id] || []).length) {
                for (const [user] of __VLS_vFor((__VLS_ctx.pendingUsers[ch.id]))) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        key: (user.id),
                        ...{ class: "pending-user-row" },
                    });
                    /** @type {__VLS_StyleScopedClasses['pending-user-row']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "pending-user-info" },
                    });
                    /** @type {__VLS_StyleScopedClasses['pending-user-info']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "pending-user-name" },
                    });
                    /** @type {__VLS_StyleScopedClasses['pending-user-name']} */ ;
                    (user.firstName || '未知');
                    if (user.username) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                            ...{ class: "pending-user-username" },
                        });
                        /** @type {__VLS_StyleScopedClasses['pending-user-username']} */ ;
                        (user.username);
                    }
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "pending-user-id" },
                    });
                    /** @type {__VLS_StyleScopedClasses['pending-user-id']} */ ;
                    (user.id);
                    let __VLS_1512;
                    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
                    elText;
                    // @ts-ignore
                    const __VLS_1513 = __VLS_asFunctionalComponent1(__VLS_1512, new __VLS_1512({
                        type: "info",
                        size: "small",
                        ...{ style: {} },
                    }));
                    const __VLS_1514 = __VLS_1513({
                        type: "info",
                        size: "small",
                        ...{ style: {} },
                    }, ...__VLS_functionalComponentArgsRest(__VLS_1513));
                    const { default: __VLS_1517 } = __VLS_1515.slots;
                    (__VLS_ctx.formatRelative(user.lastSeen));
                    // @ts-ignore
                    [formatRelative, pendingUsers, pendingUsers,];
                    var __VLS_1515;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                        ...{ class: "pending-user-actions" },
                    });
                    /** @type {__VLS_StyleScopedClasses['pending-user-actions']} */ ;
                    let __VLS_1518;
                    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
                    elButton;
                    // @ts-ignore
                    const __VLS_1519 = __VLS_asFunctionalComponent1(__VLS_1518, new __VLS_1518({
                        ...{ 'onClick': {} },
                        size: "small",
                        type: "success",
                        plain: true,
                        loading: (__VLS_ctx.allowingUserId === `${ch.id}-${user.id}`),
                    }));
                    const __VLS_1520 = __VLS_1519({
                        ...{ 'onClick': {} },
                        size: "small",
                        type: "success",
                        plain: true,
                        loading: (__VLS_ctx.allowingUserId === `${ch.id}-${user.id}`),
                    }, ...__VLS_functionalComponentArgsRest(__VLS_1519));
                    let __VLS_1523;
                    const __VLS_1524 = ({ click: {} },
                        { onClick: (...[$event]) => {
                                if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                                    return;
                                if (!(__VLS_ctx.expandedPending === ch.id))
                                    return;
                                if (!!(__VLS_ctx.pendingLoading[ch.id]))
                                    return;
                                if (!((__VLS_ctx.pendingUsers[ch.id] || []).length))
                                    return;
                                __VLS_ctx.allowUser(ch.id, user.id);
                                // @ts-ignore
                                [allowingUserId, allowUser,];
                            } });
                    const { default: __VLS_1525 } = __VLS_1521.slots;
                    // @ts-ignore
                    [];
                    var __VLS_1521;
                    var __VLS_1522;
                    let __VLS_1526;
                    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
                    elButton;
                    // @ts-ignore
                    const __VLS_1527 = __VLS_asFunctionalComponent1(__VLS_1526, new __VLS_1526({
                        ...{ 'onClick': {} },
                        size: "small",
                        type: "danger",
                        plain: true,
                    }));
                    const __VLS_1528 = __VLS_1527({
                        ...{ 'onClick': {} },
                        size: "small",
                        type: "danger",
                        plain: true,
                    }, ...__VLS_functionalComponentArgsRest(__VLS_1527));
                    let __VLS_1531;
                    const __VLS_1532 = ({ click: {} },
                        { onClick: (...[$event]) => {
                                if (!(ch.type === 'telegram' || ch.type === 'feishu'))
                                    return;
                                if (!(__VLS_ctx.expandedPending === ch.id))
                                    return;
                                if (!!(__VLS_ctx.pendingLoading[ch.id]))
                                    return;
                                if (!((__VLS_ctx.pendingUsers[ch.id] || []).length))
                                    return;
                                __VLS_ctx.dismissUser(ch.id, user.id);
                                // @ts-ignore
                                [dismissUser,];
                            } });
                    const { default: __VLS_1533 } = __VLS_1529.slots;
                    // @ts-ignore
                    [];
                    var __VLS_1529;
                    var __VLS_1530;
                    // @ts-ignore
                    [];
                }
            }
            else {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "pending-empty" },
                });
                /** @type {__VLS_StyleScopedClasses['pending-empty']} */ ;
                if (ch.type === 'feishu') {
                }
                else {
                }
            }
        }
    }
    // @ts-ignore
    [];
}
if (!__VLS_ctx.channelsLoading && !__VLS_ctx.agentChannelList.length) {
    let __VLS_1534;
    /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
    elEmpty;
    // @ts-ignore
    const __VLS_1535 = __VLS_asFunctionalComponent1(__VLS_1534, new __VLS_1534({
        description: "暂无消息渠道，点击「添加消息渠道」开始配置",
        imageSize: (80),
        ...{ style: {} },
    }));
    const __VLS_1536 = __VLS_1535({
        description: "暂无消息渠道，点击「添加消息渠道」开始配置",
        imageSize: (80),
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1535));
}
let __VLS_1539;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_1540 = __VLS_asFunctionalComponent1(__VLS_1539, new __VLS_1539({
    modelValue: (__VLS_ctx.channelDialogVisible),
    title: (__VLS_ctx.channelEditingId ? '编辑消息渠道' : '添加消息渠道'),
    width: "540px",
}));
const __VLS_1541 = __VLS_1540({
    modelValue: (__VLS_ctx.channelDialogVisible),
    title: (__VLS_ctx.channelEditingId ? '编辑消息渠道' : '添加消息渠道'),
    width: "540px",
}, ...__VLS_functionalComponentArgsRest(__VLS_1540));
const { default: __VLS_1544 } = __VLS_1542.slots;
let __VLS_1545;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_1546 = __VLS_asFunctionalComponent1(__VLS_1545, new __VLS_1545({
    model: (__VLS_ctx.channelForm),
    labelWidth: "120px",
}));
const __VLS_1547 = __VLS_1546({
    model: (__VLS_ctx.channelForm),
    labelWidth: "120px",
}, ...__VLS_functionalComponentArgsRest(__VLS_1546));
const { default: __VLS_1550 } = __VLS_1548.slots;
let __VLS_1551;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1552 = __VLS_asFunctionalComponent1(__VLS_1551, new __VLS_1551({
    label: "类型",
    required: true,
}));
const __VLS_1553 = __VLS_1552({
    label: "类型",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1552));
const { default: __VLS_1556 } = __VLS_1554.slots;
let __VLS_1557;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_1558 = __VLS_asFunctionalComponent1(__VLS_1557, new __VLS_1557({
    modelValue: (__VLS_ctx.channelForm.type),
    ...{ style: {} },
}));
const __VLS_1559 = __VLS_1558({
    modelValue: (__VLS_ctx.channelForm.type),
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_1558));
const { default: __VLS_1562 } = __VLS_1560.slots;
let __VLS_1563;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1564 = __VLS_asFunctionalComponent1(__VLS_1563, new __VLS_1563({
    label: "Telegram",
    value: "telegram",
}));
const __VLS_1565 = __VLS_1564({
    label: "Telegram",
    value: "telegram",
}, ...__VLS_functionalComponentArgsRest(__VLS_1564));
let __VLS_1568;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1569 = __VLS_asFunctionalComponent1(__VLS_1568, new __VLS_1568({
    label: "飞书 / Lark",
    value: "feishu",
}));
const __VLS_1570 = __VLS_1569({
    label: "飞书 / Lark",
    value: "feishu",
}, ...__VLS_functionalComponentArgsRest(__VLS_1569));
let __VLS_1573;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1574 = __VLS_asFunctionalComponent1(__VLS_1573, new __VLS_1573({
    label: "Web 聊天页",
    value: "web",
}));
const __VLS_1575 = __VLS_1574({
    label: "Web 聊天页",
    value: "web",
}, ...__VLS_functionalComponentArgsRest(__VLS_1574));
let __VLS_1578;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1579 = __VLS_asFunctionalComponent1(__VLS_1578, new __VLS_1578({
    label: "iMessage",
    value: "imessage",
}));
const __VLS_1580 = __VLS_1579({
    label: "iMessage",
    value: "imessage",
}, ...__VLS_functionalComponentArgsRest(__VLS_1579));
let __VLS_1583;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_1584 = __VLS_asFunctionalComponent1(__VLS_1583, new __VLS_1583({
    label: "WhatsApp",
    value: "whatsapp",
}));
const __VLS_1585 = __VLS_1584({
    label: "WhatsApp",
    value: "whatsapp",
}, ...__VLS_functionalComponentArgsRest(__VLS_1584));
// @ts-ignore
[agentChannelList, channelsLoading, channelDialogVisible, channelEditingId, channelForm, channelForm,];
var __VLS_1560;
// @ts-ignore
[];
var __VLS_1554;
let __VLS_1588;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1589 = __VLS_asFunctionalComponent1(__VLS_1588, new __VLS_1588({
    label: "名称",
    required: true,
}));
const __VLS_1590 = __VLS_1589({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1589));
const { default: __VLS_1593 } = __VLS_1591.slots;
let __VLS_1594;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1595 = __VLS_asFunctionalComponent1(__VLS_1594, new __VLS_1594({
    modelValue: (__VLS_ctx.channelForm.name),
    placeholder: "如：客服 Bot",
}));
const __VLS_1596 = __VLS_1595({
    modelValue: (__VLS_ctx.channelForm.name),
    placeholder: "如：客服 Bot",
}, ...__VLS_functionalComponentArgsRest(__VLS_1595));
// @ts-ignore
[channelForm,];
var __VLS_1591;
if (__VLS_ctx.channelForm.type === 'telegram') {
    let __VLS_1599;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1600 = __VLS_asFunctionalComponent1(__VLS_1599, new __VLS_1599({
        label: "Bot Token",
        required: true,
    }));
    const __VLS_1601 = __VLS_1600({
        label: "Bot Token",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_1600));
    const { default: __VLS_1604 } = __VLS_1602.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_1605;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1606 = __VLS_asFunctionalComponent1(__VLS_1605, new __VLS_1605({
        modelValue: (__VLS_ctx.channelForm.botToken),
        type: "password",
        showPassword: true,
        placeholder: "从 @BotFather 获取",
        ...{ style: {} },
        status: (__VLS_ctx.tokenCheckState.status === 'error' ? 'error' : __VLS_ctx.tokenCheckState.status === 'ok' ? 'success' : ''),
    }));
    const __VLS_1607 = __VLS_1606({
        modelValue: (__VLS_ctx.channelForm.botToken),
        type: "password",
        showPassword: true,
        placeholder: "从 @BotFather 获取",
        ...{ style: {} },
        status: (__VLS_ctx.tokenCheckState.status === 'error' ? 'error' : __VLS_ctx.tokenCheckState.status === 'ok' ? 'success' : ''),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1606));
    let __VLS_1610;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1611 = __VLS_asFunctionalComponent1(__VLS_1610, new __VLS_1610({
        ...{ 'onClick': {} },
        size: "default",
        loading: (__VLS_ctx.tokenCheckState.loading),
        type: (__VLS_ctx.tokenCheckState.status === 'ok' ? 'success' : __VLS_ctx.tokenCheckState.status === 'error' ? 'danger' : 'default'),
        disabled: (!__VLS_ctx.channelForm.botToken || __VLS_ctx.ismaskedToken(__VLS_ctx.channelForm.botToken)),
    }));
    const __VLS_1612 = __VLS_1611({
        ...{ 'onClick': {} },
        size: "default",
        loading: (__VLS_ctx.tokenCheckState.loading),
        type: (__VLS_ctx.tokenCheckState.status === 'ok' ? 'success' : __VLS_ctx.tokenCheckState.status === 'error' ? 'danger' : 'default'),
        disabled: (!__VLS_ctx.channelForm.botToken || __VLS_ctx.ismaskedToken(__VLS_ctx.channelForm.botToken)),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1611));
    let __VLS_1615;
    const __VLS_1616 = ({ click: {} },
        { onClick: (__VLS_ctx.doCheckToken) });
    const { default: __VLS_1617 } = __VLS_1613.slots;
    // @ts-ignore
    [channelForm, channelForm, channelForm, channelForm, tokenCheckState, tokenCheckState, tokenCheckState, tokenCheckState, tokenCheckState, ismaskedToken, doCheckToken,];
    var __VLS_1613;
    var __VLS_1614;
    if (__VLS_ctx.tokenCheckState.loading) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_1618;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1619 = __VLS_asFunctionalComponent1(__VLS_1618, new __VLS_1618({
            ...{ class: "is-loading" },
        }));
        const __VLS_1620 = __VLS_1619({
            ...{ class: "is-loading" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1619));
        /** @type {__VLS_StyleScopedClasses['is-loading']} */ ;
        const { default: __VLS_1623 } = __VLS_1621.slots;
        let __VLS_1624;
        /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
        Refresh;
        // @ts-ignore
        const __VLS_1625 = __VLS_asFunctionalComponent1(__VLS_1624, new __VLS_1624({}));
        const __VLS_1626 = __VLS_1625({}, ...__VLS_functionalComponentArgsRest(__VLS_1625));
        // @ts-ignore
        [tokenCheckState,];
        var __VLS_1621;
    }
    else if (__VLS_ctx.tokenCheckState.status === 'ok') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_1629;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1630 = __VLS_asFunctionalComponent1(__VLS_1629, new __VLS_1629({
            ...{ style: {} },
        }));
        const __VLS_1631 = __VLS_1630({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1630));
        const { default: __VLS_1634 } = __VLS_1632.slots;
        let __VLS_1635;
        /** @ts-ignore @type { | typeof __VLS_components.CircleCheck} */
        CircleCheck;
        // @ts-ignore
        const __VLS_1636 = __VLS_asFunctionalComponent1(__VLS_1635, new __VLS_1635({}));
        const __VLS_1637 = __VLS_1636({}, ...__VLS_functionalComponentArgsRest(__VLS_1636));
        // @ts-ignore
        [tokenCheckState,];
        var __VLS_1632;
        __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
        (__VLS_ctx.tokenCheckState.botName);
    }
    else if (__VLS_ctx.tokenCheckState.status === 'duplicate') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_1640;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1641 = __VLS_asFunctionalComponent1(__VLS_1640, new __VLS_1640({
            ...{ style: {} },
        }));
        const __VLS_1642 = __VLS_1641({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1641));
        const { default: __VLS_1645 } = __VLS_1643.slots;
        let __VLS_1646;
        /** @ts-ignore @type { | typeof __VLS_components.Warning} */
        Warning;
        // @ts-ignore
        const __VLS_1647 = __VLS_asFunctionalComponent1(__VLS_1646, new __VLS_1646({}));
        const __VLS_1648 = __VLS_1647({}, ...__VLS_functionalComponentArgsRest(__VLS_1647));
        // @ts-ignore
        [tokenCheckState, tokenCheckState,];
        var __VLS_1643;
        __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
        (__VLS_ctx.tokenCheckState.usedBy);
        (__VLS_ctx.tokenCheckState.usedByCh);
    }
    else if (__VLS_ctx.tokenCheckState.status === 'error') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_1651;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1652 = __VLS_asFunctionalComponent1(__VLS_1651, new __VLS_1651({
            ...{ style: {} },
        }));
        const __VLS_1653 = __VLS_1652({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1652));
        const { default: __VLS_1656 } = __VLS_1654.slots;
        let __VLS_1657;
        /** @ts-ignore @type { | typeof __VLS_components.CircleClose} */
        CircleClose;
        // @ts-ignore
        const __VLS_1658 = __VLS_asFunctionalComponent1(__VLS_1657, new __VLS_1657({}));
        const __VLS_1659 = __VLS_1658({}, ...__VLS_functionalComponentArgsRest(__VLS_1658));
        // @ts-ignore
        [tokenCheckState, tokenCheckState, tokenCheckState,];
        var __VLS_1654;
        (__VLS_ctx.tokenCheckState.error);
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_1662;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_1663 = __VLS_asFunctionalComponent1(__VLS_1662, new __VLS_1662({
            type: "info",
            size: "small",
        }));
        const __VLS_1664 = __VLS_1663({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1663));
        const { default: __VLS_1667 } = __VLS_1665.slots;
        let __VLS_1668;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1669 = __VLS_asFunctionalComponent1(__VLS_1668, new __VLS_1668({
            ...{ style: {} },
        }));
        const __VLS_1670 = __VLS_1669({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1669));
        const { default: __VLS_1673 } = __VLS_1671.slots;
        let __VLS_1674;
        /** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
        InfoFilled;
        // @ts-ignore
        const __VLS_1675 = __VLS_asFunctionalComponent1(__VLS_1674, new __VLS_1674({}));
        const __VLS_1676 = __VLS_1675({}, ...__VLS_functionalComponentArgsRest(__VLS_1675));
        // @ts-ignore
        [tokenCheckState,];
        var __VLS_1671;
        // @ts-ignore
        [];
        var __VLS_1665;
    }
    // @ts-ignore
    [];
    var __VLS_1602;
    let __VLS_1679;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1680 = __VLS_asFunctionalComponent1(__VLS_1679, new __VLS_1679({
        label: "白名单用户",
    }));
    const __VLS_1681 = __VLS_1680({
        label: "白名单用户",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1680));
    const { default: __VLS_1684 } = __VLS_1682.slots;
    let __VLS_1685;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1686 = __VLS_asFunctionalComponent1(__VLS_1685, new __VLS_1685({
        modelValue: (__VLS_ctx.channelForm.allowedFrom),
        placeholder: "填入 Telegram 用户 ID，多个用逗号分隔",
    }));
    const __VLS_1687 = __VLS_1686({
        modelValue: (__VLS_ctx.channelForm.allowedFrom),
        placeholder: "填入 Telegram 用户 ID，多个用逗号分隔",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1686));
    let __VLS_1690;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_1691 = __VLS_asFunctionalComponent1(__VLS_1690, new __VLS_1690({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_1692 = __VLS_1691({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1691));
    const { default: __VLS_1695 } = __VLS_1693.slots;
    let __VLS_1696;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_1697 = __VLS_asFunctionalComponent1(__VLS_1696, new __VLS_1696({
        ...{ style: {} },
    }));
    const __VLS_1698 = __VLS_1697({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1697));
    const { default: __VLS_1701 } = __VLS_1699.slots;
    let __VLS_1702;
    /** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
    InfoFilled;
    // @ts-ignore
    const __VLS_1703 = __VLS_asFunctionalComponent1(__VLS_1702, new __VLS_1702({}));
    const __VLS_1704 = __VLS_1703({}, ...__VLS_functionalComponentArgsRest(__VLS_1703));
    // @ts-ignore
    [channelForm,];
    var __VLS_1699;
    // @ts-ignore
    [];
    var __VLS_1693;
    // @ts-ignore
    [];
    var __VLS_1682;
}
if (__VLS_ctx.channelForm.type === 'web') {
    if (__VLS_ctx.channelEditingId) {
        let __VLS_1707;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_1708 = __VLS_asFunctionalComponent1(__VLS_1707, new __VLS_1707({
            label: "访问链接",
        }));
        const __VLS_1709 = __VLS_1708({
            label: "访问链接",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1708));
        const { default: __VLS_1712 } = __VLS_1710.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-url-preview" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-url-preview']} */ ;
        let __VLS_1713;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1714 = __VLS_asFunctionalComponent1(__VLS_1713, new __VLS_1713({
            ...{ style: {} },
        }));
        const __VLS_1715 = __VLS_1714({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1714));
        const { default: __VLS_1718 } = __VLS_1716.slots;
        let __VLS_1719;
        /** @ts-ignore @type { | typeof __VLS_components.Link} */
        Link;
        // @ts-ignore
        const __VLS_1720 = __VLS_asFunctionalComponent1(__VLS_1719, new __VLS_1719({}));
        const __VLS_1721 = __VLS_1720({}, ...__VLS_functionalComponentArgsRest(__VLS_1720));
        // @ts-ignore
        [channelEditingId, channelForm,];
        var __VLS_1716;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-url-text" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-url-text']} */ ;
        (__VLS_ctx.webChatUrl(__VLS_ctx.agentId, __VLS_ctx.channelEditingId));
        let __VLS_1724;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1725 = __VLS_asFunctionalComponent1(__VLS_1724, new __VLS_1724({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
        }));
        const __VLS_1726 = __VLS_1725({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_1725));
        let __VLS_1729;
        const __VLS_1730 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.channelForm.type === 'web'))
                        return;
                    if (!(__VLS_ctx.channelEditingId))
                        return;
                    __VLS_ctx.copyUrl(__VLS_ctx.webChatUrl(__VLS_ctx.agentId, __VLS_ctx.channelEditingId));
                    // @ts-ignore
                    [agentId, agentId, webChatUrl, webChatUrl, copyUrl, channelEditingId, channelEditingId,];
                } });
        const { default: __VLS_1731 } = __VLS_1727.slots;
        // @ts-ignore
        [];
        var __VLS_1727;
        var __VLS_1728;
        // @ts-ignore
        [];
        var __VLS_1710;
    }
    if (!__VLS_ctx.channelEditingId) {
        let __VLS_1732;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_1733 = __VLS_asFunctionalComponent1(__VLS_1732, new __VLS_1732({
            label: "访问链接",
        }));
        const __VLS_1734 = __VLS_1733({
            label: "访问链接",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1733));
        const { default: __VLS_1737 } = __VLS_1735.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "channel-url-preview" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-url-preview']} */ ;
        let __VLS_1738;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_1739 = __VLS_asFunctionalComponent1(__VLS_1738, new __VLS_1738({
            ...{ style: {} },
        }));
        const __VLS_1740 = __VLS_1739({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1739));
        const { default: __VLS_1743 } = __VLS_1741.slots;
        let __VLS_1744;
        /** @ts-ignore @type { | typeof __VLS_components.Link} */
        Link;
        // @ts-ignore
        const __VLS_1745 = __VLS_asFunctionalComponent1(__VLS_1744, new __VLS_1744({}));
        const __VLS_1746 = __VLS_1745({}, ...__VLS_functionalComponentArgsRest(__VLS_1745));
        // @ts-ignore
        [channelEditingId,];
        var __VLS_1741;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "channel-url-text" },
        });
        /** @type {__VLS_StyleScopedClasses['channel-url-text']} */ ;
        (__VLS_ctx.webChatUrl(__VLS_ctx.agentId, __VLS_ctx.pendingChannelId));
        let __VLS_1749;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_1750 = __VLS_asFunctionalComponent1(__VLS_1749, new __VLS_1749({
            size: "small",
            type: "info",
            effect: "plain",
        }));
        const __VLS_1751 = __VLS_1750({
            size: "small",
            type: "info",
            effect: "plain",
        }, ...__VLS_functionalComponentArgsRest(__VLS_1750));
        const { default: __VLS_1754 } = __VLS_1752.slots;
        // @ts-ignore
        [agentId, webChatUrl, pendingChannelId,];
        var __VLS_1752;
        let __VLS_1755;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_1756 = __VLS_asFunctionalComponent1(__VLS_1755, new __VLS_1755({
            type: "info",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_1757 = __VLS_1756({
            type: "info",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1756));
        const { default: __VLS_1760 } = __VLS_1758.slots;
        // @ts-ignore
        [];
        var __VLS_1758;
        // @ts-ignore
        [];
        var __VLS_1735;
    }
    let __VLS_1761;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1762 = __VLS_asFunctionalComponent1(__VLS_1761, new __VLS_1761({
        label: "访问密码",
    }));
    const __VLS_1763 = __VLS_1762({
        label: "访问密码",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1762));
    const { default: __VLS_1766 } = __VLS_1764.slots;
    let __VLS_1767;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1768 = __VLS_asFunctionalComponent1(__VLS_1767, new __VLS_1767({
        modelValue: (__VLS_ctx.channelForm.webPassword),
        type: "password",
        showPassword: true,
        placeholder: "留空则无需密码",
    }));
    const __VLS_1769 = __VLS_1768({
        modelValue: (__VLS_ctx.channelForm.webPassword),
        type: "password",
        showPassword: true,
        placeholder: "留空则无需密码",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1768));
    // @ts-ignore
    [channelForm,];
    var __VLS_1764;
    let __VLS_1772;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1773 = __VLS_asFunctionalComponent1(__VLS_1772, new __VLS_1772({
        label: "欢迎语",
    }));
    const __VLS_1774 = __VLS_1773({
        label: "欢迎语",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1773));
    const { default: __VLS_1777 } = __VLS_1775.slots;
    let __VLS_1778;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1779 = __VLS_asFunctionalComponent1(__VLS_1778, new __VLS_1778({
        modelValue: (__VLS_ctx.channelForm.webWelcome),
        placeholder: "你好！有什么可以帮你的？",
    }));
    const __VLS_1780 = __VLS_1779({
        modelValue: (__VLS_ctx.channelForm.webWelcome),
        placeholder: "你好！有什么可以帮你的？",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1779));
    // @ts-ignore
    [channelForm,];
    var __VLS_1775;
    let __VLS_1783;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1784 = __VLS_asFunctionalComponent1(__VLS_1783, new __VLS_1783({
        label: "页面标题",
    }));
    const __VLS_1785 = __VLS_1784({
        label: "页面标题",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1784));
    const { default: __VLS_1788 } = __VLS_1786.slots;
    let __VLS_1789;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1790 = __VLS_asFunctionalComponent1(__VLS_1789, new __VLS_1789({
        modelValue: (__VLS_ctx.channelForm.webTitle),
        placeholder: "AI 助手",
    }));
    const __VLS_1791 = __VLS_1790({
        modelValue: (__VLS_ctx.channelForm.webTitle),
        placeholder: "AI 助手",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1790));
    // @ts-ignore
    [channelForm,];
    var __VLS_1786;
}
if (__VLS_ctx.channelForm.type === 'feishu') {
    let __VLS_1794;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1795 = __VLS_asFunctionalComponent1(__VLS_1794, new __VLS_1794({
        label: "App ID",
        required: true,
    }));
    const __VLS_1796 = __VLS_1795({
        label: "App ID",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_1795));
    const { default: __VLS_1799 } = __VLS_1797.slots;
    let __VLS_1800;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1801 = __VLS_asFunctionalComponent1(__VLS_1800, new __VLS_1800({
        modelValue: (__VLS_ctx.channelForm.appId),
        placeholder: "cli_xxxxxxxxxxxxxxxx",
    }));
    const __VLS_1802 = __VLS_1801({
        modelValue: (__VLS_ctx.channelForm.appId),
        placeholder: "cli_xxxxxxxxxxxxxxxx",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1801));
    // @ts-ignore
    [channelForm, channelForm,];
    var __VLS_1797;
    let __VLS_1805;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1806 = __VLS_asFunctionalComponent1(__VLS_1805, new __VLS_1805({
        label: "App Secret",
        required: true,
    }));
    const __VLS_1807 = __VLS_1806({
        label: "App Secret",
        required: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_1806));
    const { default: __VLS_1810 } = __VLS_1808.slots;
    let __VLS_1811;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1812 = __VLS_asFunctionalComponent1(__VLS_1811, new __VLS_1811({
        modelValue: (__VLS_ctx.channelForm.appSecret),
        type: "password",
        showPassword: true,
        placeholder: "从飞书开放平台获取",
    }));
    const __VLS_1813 = __VLS_1812({
        modelValue: (__VLS_ctx.channelForm.appSecret),
        type: "password",
        showPassword: true,
        placeholder: "从飞书开放平台获取",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1812));
    // @ts-ignore
    [channelForm,];
    var __VLS_1808;
    let __VLS_1816;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1817 = __VLS_asFunctionalComponent1(__VLS_1816, new __VLS_1816({
        label: "白名单用户",
    }));
    const __VLS_1818 = __VLS_1817({
        label: "白名单用户",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1817));
    const { default: __VLS_1821 } = __VLS_1819.slots;
    let __VLS_1822;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1823 = __VLS_asFunctionalComponent1(__VLS_1822, new __VLS_1822({
        modelValue: (__VLS_ctx.channelForm.allowedFrom),
        placeholder: "填入用户 Open ID，多个用逗号分隔",
    }));
    const __VLS_1824 = __VLS_1823({
        modelValue: (__VLS_ctx.channelForm.allowedFrom),
        placeholder: "填入用户 Open ID，多个用逗号分隔",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1823));
    let __VLS_1827;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_1828 = __VLS_asFunctionalComponent1(__VLS_1827, new __VLS_1827({
        type: "info",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_1829 = __VLS_1828({
        type: "info",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1828));
    const { default: __VLS_1832 } = __VLS_1830.slots;
    let __VLS_1833;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_1834 = __VLS_asFunctionalComponent1(__VLS_1833, new __VLS_1833({
        ...{ style: {} },
    }));
    const __VLS_1835 = __VLS_1834({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1834));
    const { default: __VLS_1838 } = __VLS_1836.slots;
    let __VLS_1839;
    /** @ts-ignore @type { | typeof __VLS_components.InfoFilled} */
    InfoFilled;
    // @ts-ignore
    const __VLS_1840 = __VLS_asFunctionalComponent1(__VLS_1839, new __VLS_1839({}));
    const __VLS_1841 = __VLS_1840({}, ...__VLS_functionalComponentArgsRest(__VLS_1840));
    // @ts-ignore
    [channelForm,];
    var __VLS_1836;
    // @ts-ignore
    [];
    var __VLS_1830;
    // @ts-ignore
    [];
    var __VLS_1819;
    let __VLS_1844;
    /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
    elFormItem;
    // @ts-ignore
    const __VLS_1845 = __VLS_asFunctionalComponent1(__VLS_1844, new __VLS_1844({
        label: "配置说明",
    }));
    const __VLS_1846 = __VLS_1845({
        label: "配置说明",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1845));
    const { default: __VLS_1849 } = __VLS_1847.slots;
    let __VLS_1850;
    /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
    elText;
    // @ts-ignore
    const __VLS_1851 = __VLS_asFunctionalComponent1(__VLS_1850, new __VLS_1850({
        type: "info",
        size: "small",
    }));
    const __VLS_1852 = __VLS_1851({
        type: "info",
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1851));
    const { default: __VLS_1855 } = __VLS_1853.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
    // @ts-ignore
    [];
    var __VLS_1853;
    // @ts-ignore
    [];
    var __VLS_1847;
}
let __VLS_1856;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_1857 = __VLS_asFunctionalComponent1(__VLS_1856, new __VLS_1856({
    label: "启用",
}));
const __VLS_1858 = __VLS_1857({
    label: "启用",
}, ...__VLS_functionalComponentArgsRest(__VLS_1857));
const { default: __VLS_1861 } = __VLS_1859.slots;
let __VLS_1862;
/** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
elSwitch;
// @ts-ignore
const __VLS_1863 = __VLS_asFunctionalComponent1(__VLS_1862, new __VLS_1862({
    modelValue: (__VLS_ctx.channelForm.enabled),
}));
const __VLS_1864 = __VLS_1863({
    modelValue: (__VLS_ctx.channelForm.enabled),
}, ...__VLS_functionalComponentArgsRest(__VLS_1863));
// @ts-ignore
[channelForm,];
var __VLS_1859;
// @ts-ignore
[];
var __VLS_1548;
{
    const { footer: __VLS_1867 } = __VLS_1542.slots;
    let __VLS_1868;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1869 = __VLS_asFunctionalComponent1(__VLS_1868, new __VLS_1868({
        ...{ 'onClick': {} },
    }));
    const __VLS_1870 = __VLS_1869({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_1869));
    let __VLS_1873;
    const __VLS_1874 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.channelDialogVisible = false;
                // @ts-ignore
                [channelDialogVisible,];
            } });
    const { default: __VLS_1875 } = __VLS_1871.slots;
    // @ts-ignore
    [];
    var __VLS_1871;
    var __VLS_1872;
    let __VLS_1876;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1877 = __VLS_asFunctionalComponent1(__VLS_1876, new __VLS_1876({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.channelSaving),
    }));
    const __VLS_1878 = __VLS_1877({
        ...{ 'onClick': {} },
        type: "primary",
        loading: (__VLS_ctx.channelSaving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1877));
    let __VLS_1881;
    const __VLS_1882 = ({ click: {} },
        { onClick: (__VLS_ctx.saveChannelDialog) });
    const { default: __VLS_1883 } = __VLS_1879.slots;
    // @ts-ignore
    [channelSaving, saveChannelDialog,];
    var __VLS_1879;
    var __VLS_1880;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1542;
// @ts-ignore
[];
var __VLS_1369;
let __VLS_1884;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_1885 = __VLS_asFunctionalComponent1(__VLS_1884, new __VLS_1884({
    label: "环境变量",
    name: "env",
}));
const __VLS_1886 = __VLS_1885({
    label: "环境变量",
    name: "env",
}, ...__VLS_functionalComponentArgsRest(__VLS_1885));
const { default: __VLS_1889 } = __VLS_1887.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h3, __VLS_intrinsics.h3)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "env-add-row" },
    ...{ style: {} },
});
/** @type {__VLS_StyleScopedClasses['env-add-row']} */ ;
let __VLS_1890;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1891 = __VLS_asFunctionalComponent1(__VLS_1890, new __VLS_1890({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newEnvKey),
    placeholder: "KEY（如 GITHUB_TOKEN）",
    ...{ style: {} },
    size: "small",
}));
const __VLS_1892 = __VLS_1891({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newEnvKey),
    placeholder: "KEY（如 GITHUB_TOKEN）",
    ...{ style: {} },
    size: "small",
}, ...__VLS_functionalComponentArgsRest(__VLS_1891));
let __VLS_1895;
const __VLS_1896 = ({ keyup: {} },
    { onKeyup: (__VLS_ctx.addEnvVar) });
var __VLS_1893;
var __VLS_1894;
let __VLS_1897;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_1898 = __VLS_asFunctionalComponent1(__VLS_1897, new __VLS_1897({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newEnvValue),
    placeholder: "VALUE",
    ...{ style: {} },
    size: "small",
    type: "password",
    showPassword: true,
}));
const __VLS_1899 = __VLS_1898({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newEnvValue),
    placeholder: "VALUE",
    ...{ style: {} },
    size: "small",
    type: "password",
    showPassword: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1898));
let __VLS_1902;
const __VLS_1903 = ({ keyup: {} },
    { onKeyup: (__VLS_ctx.addEnvVar) });
var __VLS_1900;
var __VLS_1901;
let __VLS_1904;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1905 = __VLS_asFunctionalComponent1(__VLS_1904, new __VLS_1904({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    disabled: (!__VLS_ctx.newEnvKey.trim()),
}));
const __VLS_1906 = __VLS_1905({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    disabled: (!__VLS_ctx.newEnvKey.trim()),
}, ...__VLS_functionalComponentArgsRest(__VLS_1905));
let __VLS_1909;
const __VLS_1910 = ({ click: {} },
    { onClick: (__VLS_ctx.addEnvVar) });
const { default: __VLS_1911 } = __VLS_1907.slots;
// @ts-ignore
[newEnvKey, newEnvKey, addEnvVar, addEnvVar, addEnvVar, newEnvValue,];
var __VLS_1907;
var __VLS_1908;
let __VLS_1912;
/** @ts-ignore @type { | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table'] | typeof __VLS_components.elTable | typeof __VLS_components.ElTable | typeof __VLS_components['el-table']} */
elTable;
// @ts-ignore
const __VLS_1913 = __VLS_asFunctionalComponent1(__VLS_1912, new __VLS_1912({
    data: (__VLS_ctx.envVarsList),
    size: "small",
    ...{ style: {} },
    emptyText: "暂无环境变量",
}));
const __VLS_1914 = __VLS_1913({
    data: (__VLS_ctx.envVarsList),
    size: "small",
    ...{ style: {} },
    emptyText: "暂无环境变量",
}, ...__VLS_functionalComponentArgsRest(__VLS_1913));
const { default: __VLS_1917 } = __VLS_1915.slots;
let __VLS_1918;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1919 = __VLS_asFunctionalComponent1(__VLS_1918, new __VLS_1918({
    label: "KEY",
    minWidth: "200",
}));
const __VLS_1920 = __VLS_1919({
    label: "KEY",
    minWidth: "200",
}, ...__VLS_functionalComponentArgsRest(__VLS_1919));
const { default: __VLS_1923 } = __VLS_1921.slots;
{
    const { default: __VLS_1924 } = __VLS_1921.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1924);
    __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
        ...{ style: {} },
    });
    (row.key);
    // @ts-ignore
    [envVarsList,];
}
// @ts-ignore
[];
var __VLS_1921;
let __VLS_1925;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1926 = __VLS_asFunctionalComponent1(__VLS_1925, new __VLS_1925({
    label: "VALUE",
    minWidth: "200",
}));
const __VLS_1927 = __VLS_1926({
    label: "VALUE",
    minWidth: "200",
}, ...__VLS_functionalComponentArgsRest(__VLS_1926));
const { default: __VLS_1930 } = __VLS_1928.slots;
{
    const { default: __VLS_1931 } = __VLS_1928.slots;
    const [{ row }] = __VLS_vSlot(__VLS_1931);
    let __VLS_1932;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_1933 = __VLS_asFunctionalComponent1(__VLS_1932, new __VLS_1932({
        modelValue: (row.value),
        type: "password",
        showPassword: true,
        size: "small",
        ...{ style: {} },
        placeholder: "（未设置）",
    }));
    const __VLS_1934 = __VLS_1933({
        modelValue: (row.value),
        type: "password",
        showPassword: true,
        size: "small",
        ...{ style: {} },
        placeholder: "（未设置）",
    }, ...__VLS_functionalComponentArgsRest(__VLS_1933));
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1928;
let __VLS_1937;
/** @ts-ignore @type { | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column'] | typeof __VLS_components.elTableColumn | typeof __VLS_components.ElTableColumn | typeof __VLS_components['el-table-column']} */
elTableColumn;
// @ts-ignore
const __VLS_1938 = __VLS_asFunctionalComponent1(__VLS_1937, new __VLS_1937({
    label: "操作",
    width: "80",
    fixed: "right",
}));
const __VLS_1939 = __VLS_1938({
    label: "操作",
    width: "80",
    fixed: "right",
}, ...__VLS_functionalComponentArgsRest(__VLS_1938));
const { default: __VLS_1942 } = __VLS_1940.slots;
{
    const { default: __VLS_1943 } = __VLS_1940.slots;
    const [{ $index }] = __VLS_vSlot(__VLS_1943);
    let __VLS_1944;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1945 = __VLS_asFunctionalComponent1(__VLS_1944, new __VLS_1944({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        link: true,
    }));
    const __VLS_1946 = __VLS_1945({
        ...{ 'onClick': {} },
        size: "small",
        type: "danger",
        link: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_1945));
    let __VLS_1949;
    const __VLS_1950 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.removeEnvVar($index);
                // @ts-ignore
                [removeEnvVar,];
            } });
    const { default: __VLS_1951 } = __VLS_1947.slots;
    // @ts-ignore
    [];
    var __VLS_1947;
    var __VLS_1948;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_1940;
// @ts-ignore
[];
var __VLS_1915;
let __VLS_1952;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1953 = __VLS_asFunctionalComponent1(__VLS_1952, new __VLS_1952({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
    loading: (__VLS_ctx.envSaving),
}));
const __VLS_1954 = __VLS_1953({
    ...{ 'onClick': {} },
    type: "primary",
    size: "small",
    loading: (__VLS_ctx.envSaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_1953));
let __VLS_1957;
const __VLS_1958 = ({ click: {} },
    { onClick: (__VLS_ctx.saveEnvVars) });
const { default: __VLS_1959 } = __VLS_1955.slots;
// @ts-ignore
[envSaving, saveEnvVars,];
var __VLS_1955;
var __VLS_1956;
// @ts-ignore
[];
var __VLS_1887;
let __VLS_1960;
/** @ts-ignore @type { | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane'] | typeof __VLS_components.elTabPane | typeof __VLS_components.ElTabPane | typeof __VLS_components['el-tab-pane']} */
elTabPane;
// @ts-ignore
const __VLS_1961 = __VLS_asFunctionalComponent1(__VLS_1960, new __VLS_1960({
    label: "工具权限",
    name: "toolpolicy",
}));
const __VLS_1962 = __VLS_1961({
    label: "工具权限",
    name: "toolpolicy",
}, ...__VLS_functionalComponentArgsRest(__VLS_1961));
const { default: __VLS_1965 } = __VLS_1963.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.h3, __VLS_intrinsics.h3)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
__VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({});
let __VLS_1966;
/** @ts-ignore @type { | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card'] | typeof __VLS_components.elCard | typeof __VLS_components.ElCard | typeof __VLS_components['el-card']} */
elCard;
// @ts-ignore
const __VLS_1967 = __VLS_asFunctionalComponent1(__VLS_1966, new __VLS_1966({
    shadow: "never",
    ...{ style: {} },
}));
const __VLS_1968 = __VLS_1967({
    shadow: "never",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_1967));
const { default: __VLS_1971 } = __VLS_1969.slots;
{
    const { header: __VLS_1972 } = __VLS_1969.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    let __VLS_1973;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_1974 = __VLS_asFunctionalComponent1(__VLS_1973, new __VLS_1973({
        ...{ 'onClick': {} },
        size: "small",
        loading: (__VLS_ctx.toolHealthLoading),
    }));
    const __VLS_1975 = __VLS_1974({
        ...{ 'onClick': {} },
        size: "small",
        loading: (__VLS_ctx.toolHealthLoading),
    }, ...__VLS_functionalComponentArgsRest(__VLS_1974));
    let __VLS_1978;
    const __VLS_1979 = ({ click: {} },
        { onClick: (__VLS_ctx.runToolHealth) });
    const { default: __VLS_1980 } = __VLS_1976.slots;
    let __VLS_1981;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_1982 = __VLS_asFunctionalComponent1(__VLS_1981, new __VLS_1981({}));
    const __VLS_1983 = __VLS_1982({}, ...__VLS_functionalComponentArgsRest(__VLS_1982));
    const { default: __VLS_1986 } = __VLS_1984.slots;
    let __VLS_1987;
    /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
    Refresh;
    // @ts-ignore
    const __VLS_1988 = __VLS_asFunctionalComponent1(__VLS_1987, new __VLS_1987({}));
    const __VLS_1989 = __VLS_1988({}, ...__VLS_functionalComponentArgsRest(__VLS_1988));
    // @ts-ignore
    [toolHealthLoading, runToolHealth,];
    var __VLS_1984;
    // @ts-ignore
    [];
    var __VLS_1976;
    var __VLS_1977;
    // @ts-ignore
    [];
}
if (!__VLS_ctx.toolHealth) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.b, __VLS_intrinsics.b)({});
    (__VLS_ctx.toolHealth.summary.total);
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (__VLS_ctx.toolHealth.summary.ready);
    if (__VLS_ctx.toolHealth.summary.blocked > 0) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.toolHealth.summary.blocked);
    }
    if (__VLS_ctx.toolHealth.providerHealth) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "th-provider-health" },
            ...{ class: (__VLS_ctx.toolHealth.providerHealth.ok ? 'is-ok' : 'is-down') },
        });
        /** @type {__VLS_StyleScopedClasses['th-provider-health']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "th-provider-row" },
        });
        /** @type {__VLS_StyleScopedClasses['th-provider-row']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "th-provider-dot" },
        });
        /** @type {__VLS_StyleScopedClasses['th-provider-dot']} */ ;
        (__VLS_ctx.toolHealth.providerHealth.ok ? '🟢' : '🔴');
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "th-provider-name" },
        });
        /** @type {__VLS_StyleScopedClasses['th-provider-name']} */ ;
        (__VLS_ctx.toolHealth.providerHealth.provider);
        (__VLS_ctx.toolHealth.providerHealth.model);
        if (__VLS_ctx.toolHealth.providerHealth.ok) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "th-provider-latency" },
            });
            /** @type {__VLS_StyleScopedClasses['th-provider-latency']} */ ;
            (__VLS_ctx.toolHealth.providerHealth.latencyMs);
        }
        else if (__VLS_ctx.toolHealth.providerHealth.error) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "th-provider-err" },
            });
            /** @type {__VLS_StyleScopedClasses['th-provider-err']} */ ;
            (__VLS_ctx.toolHealth.providerHealth.error);
        }
        let __VLS_1992;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_1993 = __VLS_asFunctionalComponent1(__VLS_1992, new __VLS_1992({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }));
        const __VLS_1994 = __VLS_1993({
            ...{ 'onClick': {} },
            size: "small",
            link: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_1993));
        let __VLS_1997;
        const __VLS_1998 = ({ click: {} },
            { onClick: (__VLS_ctx.refreshProviderHealth) });
        const { default: __VLS_1999 } = __VLS_1995.slots;
        let __VLS_2000;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_2001 = __VLS_asFunctionalComponent1(__VLS_2000, new __VLS_2000({}));
        const __VLS_2002 = __VLS_2001({}, ...__VLS_functionalComponentArgsRest(__VLS_2001));
        const { default: __VLS_2005 } = __VLS_2003.slots;
        let __VLS_2006;
        /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
        Refresh;
        // @ts-ignore
        const __VLS_2007 = __VLS_asFunctionalComponent1(__VLS_2006, new __VLS_2006({}));
        const __VLS_2008 = __VLS_2007({}, ...__VLS_functionalComponentArgsRest(__VLS_2007));
        // @ts-ignore
        [toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, toolHealth, refreshProviderHealth,];
        var __VLS_2003;
        // @ts-ignore
        [];
        var __VLS_1995;
        var __VLS_1996;
        if (!__VLS_ctx.toolHealth.providerHealth.ok) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "th-provider-tip" },
            });
            /** @type {__VLS_StyleScopedClasses['th-provider-tip']} */ ;
            (__VLS_ctx.providerHealthTip(__VLS_ctx.toolHealth.providerHealth));
        }
    }
    if (__VLS_ctx.blockedTools.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        for (const [t] of __VLS_vFor((__VLS_ctx.blockedTools))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (t.name),
                ...{ class: "th-row th-blocked" },
            });
            /** @type {__VLS_StyleScopedClasses['th-row']} */ ;
            /** @type {__VLS_StyleScopedClasses['th-blocked']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({});
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
                ...{ class: "th-name" },
            });
            /** @type {__VLS_StyleScopedClasses['th-name']} */ ;
            (t.name);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "th-group" },
            });
            /** @type {__VLS_StyleScopedClasses['th-group']} */ ;
            (t.group);
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "th-reason" },
            });
            /** @type {__VLS_StyleScopedClasses['th-reason']} */ ;
            (t.reason);
            if (t.hint) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "th-hint" },
                });
                /** @type {__VLS_StyleScopedClasses['th-hint']} */ ;
                (t.hint);
            }
            // @ts-ignore
            [toolHealth, toolHealth, providerHealthTip, blockedTools, blockedTools,];
        }
    }
    if (__VLS_ctx.readyTools.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.details, __VLS_intrinsics.details)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.summary, __VLS_intrinsics.summary)({
            ...{ style: {} },
        });
        (__VLS_ctx.readyTools.length);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        for (const [t] of __VLS_vFor((__VLS_ctx.readyTools))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.code, __VLS_intrinsics.code)({
                key: (t.name),
                ...{ class: "th-tag" },
            });
            /** @type {__VLS_StyleScopedClasses['th-tag']} */ ;
            (t.name);
            // @ts-ignore
            [readyTools, readyTools, readyTools,];
        }
    }
}
// @ts-ignore
[];
var __VLS_1969;
let __VLS_2011;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_2012 = __VLS_asFunctionalComponent1(__VLS_2011, new __VLS_2011({
    labelWidth: "90px",
    size: "default",
}));
const __VLS_2013 = __VLS_2012({
    labelWidth: "90px",
    size: "default",
}, ...__VLS_functionalComponentArgsRest(__VLS_2012));
const { default: __VLS_2016 } = __VLS_2014.slots;
let __VLS_2017;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_2018 = __VLS_asFunctionalComponent1(__VLS_2017, new __VLS_2017({
    label: "Profile",
}));
const __VLS_2019 = __VLS_2018({
    label: "Profile",
}, ...__VLS_functionalComponentArgsRest(__VLS_2018));
const { default: __VLS_2022 } = __VLS_2020.slots;
let __VLS_2023;
/** @ts-ignore @type { | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select'] | typeof __VLS_components.elSelect | typeof __VLS_components.ElSelect | typeof __VLS_components['el-select']} */
elSelect;
// @ts-ignore
const __VLS_2024 = __VLS_asFunctionalComponent1(__VLS_2023, new __VLS_2023({
    modelValue: (__VLS_ctx.toolPolicyForm.profile),
    placeholder: "继承全局（不限制）",
    ...{ style: {} },
    clearable: true,
}));
const __VLS_2025 = __VLS_2024({
    modelValue: (__VLS_ctx.toolPolicyForm.profile),
    placeholder: "继承全局（不限制）",
    ...{ style: {} },
    clearable: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_2024));
const { default: __VLS_2028 } = __VLS_2026.slots;
let __VLS_2029;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_2030 = __VLS_asFunctionalComponent1(__VLS_2029, new __VLS_2029({
    label: "full — 不限制（默认）",
    value: "full",
}));
const __VLS_2031 = __VLS_2030({
    label: "full — 不限制（默认）",
    value: "full",
}, ...__VLS_functionalComponentArgsRest(__VLS_2030));
let __VLS_2034;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_2035 = __VLS_asFunctionalComponent1(__VLS_2034, new __VLS_2034({
    label: "coding — 文件+命令+Agent+记忆",
    value: "coding",
}));
const __VLS_2036 = __VLS_2035({
    label: "coding — 文件+命令+Agent+记忆",
    value: "coding",
}, ...__VLS_functionalComponentArgsRest(__VLS_2035));
let __VLS_2039;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_2040 = __VLS_asFunctionalComponent1(__VLS_2039, new __VLS_2039({
    label: "messaging — 仅消息+Sessions",
    value: "messaging",
}));
const __VLS_2041 = __VLS_2040({
    label: "messaging — 仅消息+Sessions",
    value: "messaging",
}, ...__VLS_functionalComponentArgsRest(__VLS_2040));
let __VLS_2044;
/** @ts-ignore @type { | typeof __VLS_components.elOption | typeof __VLS_components.ElOption | typeof __VLS_components['el-option']} */
elOption;
// @ts-ignore
const __VLS_2045 = __VLS_asFunctionalComponent1(__VLS_2044, new __VLS_2044({
    label: "minimal — 仅 send_message + 记忆",
    value: "minimal",
}));
const __VLS_2046 = __VLS_2045({
    label: "minimal — 仅 send_message + 记忆",
    value: "minimal",
}, ...__VLS_functionalComponentArgsRest(__VLS_2045));
// @ts-ignore
[toolPolicyForm,];
var __VLS_2026;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
// @ts-ignore
[];
var __VLS_2020;
let __VLS_2049;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_2050 = __VLS_asFunctionalComponent1(__VLS_2049, new __VLS_2049({
    label: "Allow",
}));
const __VLS_2051 = __VLS_2050({
    label: "Allow",
}, ...__VLS_functionalComponentArgsRest(__VLS_2050));
const { default: __VLS_2054 } = __VLS_2052.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [item, idx] of __VLS_vFor((__VLS_ctx.toolPolicyForm.allow))) {
    let __VLS_2055;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_2056 = __VLS_asFunctionalComponent1(__VLS_2055, new __VLS_2055({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_2057 = __VLS_2056({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_2056));
    let __VLS_2060;
    const __VLS_2061 = ({ close: {} },
        { onClose: (...[$event]) => {
                __VLS_ctx.toolPolicyForm.allow.splice(idx, 1);
                // @ts-ignore
                [toolPolicyForm, toolPolicyForm,];
            } });
    const { default: __VLS_2062 } = __VLS_2058.slots;
    (item);
    // @ts-ignore
    [];
    var __VLS_2058;
    var __VLS_2059;
    // @ts-ignore
    [];
}
let __VLS_2063;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_2064 = __VLS_asFunctionalComponent1(__VLS_2063, new __VLS_2063({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.toolPolicyAllowInput),
    size: "small",
    placeholder: "输入工具名或 group:xx，回车添加",
    ...{ style: {} },
}));
const __VLS_2065 = __VLS_2064({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.toolPolicyAllowInput),
    size: "small",
    placeholder: "输入工具名或 group:xx，回车添加",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_2064));
let __VLS_2068;
const __VLS_2069 = ({ keyup: {} },
    { onKeyup: (...[$event]) => {
            __VLS_ctx.addToolPolicyTag('allow');
            // @ts-ignore
            [toolPolicyAllowInput, addToolPolicyTag,];
        } });
const { default: __VLS_2070 } = __VLS_2066.slots;
{
    const { append: __VLS_2071 } = __VLS_2066.slots;
    let __VLS_2072;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_2073 = __VLS_asFunctionalComponent1(__VLS_2072, new __VLS_2072({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_2074 = __VLS_2073({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_2073));
    let __VLS_2077;
    const __VLS_2078 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.addToolPolicyTag('allow');
                // @ts-ignore
                [addToolPolicyTag,];
            } });
    const { default: __VLS_2079 } = __VLS_2075.slots;
    // @ts-ignore
    [];
    var __VLS_2075;
    var __VLS_2076;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_2066;
var __VLS_2067;
// @ts-ignore
[];
var __VLS_2052;
let __VLS_2080;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_2081 = __VLS_asFunctionalComponent1(__VLS_2080, new __VLS_2080({
    label: "Deny",
}));
const __VLS_2082 = __VLS_2081({
    label: "Deny",
}, ...__VLS_functionalComponentArgsRest(__VLS_2081));
const { default: __VLS_2085 } = __VLS_2083.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [item, idx] of __VLS_vFor((__VLS_ctx.toolPolicyForm.deny))) {
    let __VLS_2086;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_2087 = __VLS_asFunctionalComponent1(__VLS_2086, new __VLS_2086({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        type: "danger",
        size: "small",
        ...{ style: {} },
    }));
    const __VLS_2088 = __VLS_2087({
        ...{ 'onClose': {} },
        key: (idx),
        closable: true,
        type: "danger",
        size: "small",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_2087));
    let __VLS_2091;
    const __VLS_2092 = ({ close: {} },
        { onClose: (...[$event]) => {
                __VLS_ctx.toolPolicyForm.deny.splice(idx, 1);
                // @ts-ignore
                [toolPolicyForm, toolPolicyForm,];
            } });
    const { default: __VLS_2093 } = __VLS_2089.slots;
    (item);
    // @ts-ignore
    [];
    var __VLS_2089;
    var __VLS_2090;
    // @ts-ignore
    [];
}
let __VLS_2094;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input'] | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_2095 = __VLS_asFunctionalComponent1(__VLS_2094, new __VLS_2094({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.toolPolicyDenyInput),
    size: "small",
    placeholder: "输入工具名或 group:xx，回车拒绝",
    ...{ style: {} },
}));
const __VLS_2096 = __VLS_2095({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.toolPolicyDenyInput),
    size: "small",
    placeholder: "输入工具名或 group:xx，回车拒绝",
    ...{ style: {} },
}, ...__VLS_functionalComponentArgsRest(__VLS_2095));
let __VLS_2099;
const __VLS_2100 = ({ keyup: {} },
    { onKeyup: (...[$event]) => {
            __VLS_ctx.addToolPolicyTag('deny');
            // @ts-ignore
            [addToolPolicyTag, toolPolicyDenyInput,];
        } });
const { default: __VLS_2101 } = __VLS_2097.slots;
{
    const { append: __VLS_2102 } = __VLS_2097.slots;
    let __VLS_2103;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_2104 = __VLS_asFunctionalComponent1(__VLS_2103, new __VLS_2103({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_2105 = __VLS_2104({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_2104));
    let __VLS_2108;
    const __VLS_2109 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.addToolPolicyTag('deny');
                // @ts-ignore
                [addToolPolicyTag,];
            } });
    const { default: __VLS_2110 } = __VLS_2106.slots;
    // @ts-ignore
    [];
    var __VLS_2106;
    var __VLS_2107;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_2097;
var __VLS_2098;
// @ts-ignore
[];
var __VLS_2083;
let __VLS_2111;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_2112 = __VLS_asFunctionalComponent1(__VLS_2111, new __VLS_2111({
    label: "",
}));
const __VLS_2113 = __VLS_2112({
    label: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_2112));
const { default: __VLS_2116 } = __VLS_2114.slots;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ style: {} },
});
let __VLS_2117;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_2118 = __VLS_asFunctionalComponent1(__VLS_2117, new __VLS_2117({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}));
const __VLS_2119 = __VLS_2118({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_2118));
let __VLS_2122;
const __VLS_2123 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.quickDeny('group:runtime');
            // @ts-ignore
            [quickDeny,];
        } });
const { default: __VLS_2124 } = __VLS_2120.slots;
// @ts-ignore
[];
var __VLS_2120;
var __VLS_2121;
let __VLS_2125;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_2126 = __VLS_asFunctionalComponent1(__VLS_2125, new __VLS_2125({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}));
const __VLS_2127 = __VLS_2126({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_2126));
let __VLS_2130;
const __VLS_2131 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.quickDeny('group:fs');
            // @ts-ignore
            [quickDeny,];
        } });
const { default: __VLS_2132 } = __VLS_2128.slots;
// @ts-ignore
[];
var __VLS_2128;
var __VLS_2129;
let __VLS_2133;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_2134 = __VLS_asFunctionalComponent1(__VLS_2133, new __VLS_2133({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}));
const __VLS_2135 = __VLS_2134({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_2134));
let __VLS_2138;
const __VLS_2139 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.quickDeny('exec');
            // @ts-ignore
            [quickDeny,];
        } });
const { default: __VLS_2140 } = __VLS_2136.slots;
// @ts-ignore
[];
var __VLS_2136;
var __VLS_2137;
let __VLS_2141;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_2142 = __VLS_asFunctionalComponent1(__VLS_2141, new __VLS_2141({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
    type: "danger",
}));
const __VLS_2143 = __VLS_2142({
    ...{ 'onClick': {} },
    size: "small",
    plain: true,
    type: "danger",
}, ...__VLS_functionalComponentArgsRest(__VLS_2142));
let __VLS_2146;
const __VLS_2147 = ({ click: {} },
    { onClick: (__VLS_ctx.clearToolPolicy) });
const { default: __VLS_2148 } = __VLS_2144.slots;
// @ts-ignore
[clearToolPolicy,];
var __VLS_2144;
var __VLS_2145;
// @ts-ignore
[];
var __VLS_2114;
let __VLS_2149;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_2150 = __VLS_asFunctionalComponent1(__VLS_2149, new __VLS_2149({
    label: "",
}));
const __VLS_2151 = __VLS_2150({
    label: "",
}, ...__VLS_functionalComponentArgsRest(__VLS_2150));
const { default: __VLS_2154 } = __VLS_2152.slots;
let __VLS_2155;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_2156 = __VLS_asFunctionalComponent1(__VLS_2155, new __VLS_2155({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.toolPolicySaving),
}));
const __VLS_2157 = __VLS_2156({
    ...{ 'onClick': {} },
    type: "primary",
    loading: (__VLS_ctx.toolPolicySaving),
}, ...__VLS_functionalComponentArgsRest(__VLS_2156));
let __VLS_2160;
const __VLS_2161 = ({ click: {} },
    { onClick: (__VLS_ctx.saveToolPolicy) });
const { default: __VLS_2162 } = __VLS_2158.slots;
// @ts-ignore
[toolPolicySaving, saveToolPolicy,];
var __VLS_2158;
var __VLS_2159;
if (__VLS_ctx.toolPolicySaved) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
// @ts-ignore
[toolPolicySaved,];
var __VLS_2152;
// @ts-ignore
[];
var __VLS_2014;
let __VLS_2163;
/** @ts-ignore @type { | typeof __VLS_components.elDivider | typeof __VLS_components.ElDivider | typeof __VLS_components['el-divider']} */
elDivider;
// @ts-ignore
const __VLS_2164 = __VLS_asFunctionalComponent1(__VLS_2163, new __VLS_2163({}));
const __VLS_2165 = __VLS_2164({}, ...__VLS_functionalComponentArgsRest(__VLS_2164));
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ style: {} },
});
for (const [t] of __VLS_vFor((__VLS_ctx.toolPolicyPreview))) {
    let __VLS_2168;
    /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
    elTag;
    // @ts-ignore
    const __VLS_2169 = __VLS_asFunctionalComponent1(__VLS_2168, new __VLS_2168({
        key: (t.name),
        type: (t.denied ? 'danger' : 'success'),
        size: "small",
        title: (t.denied ? '被 deny 屏蔽' : '可用'),
    }));
    const __VLS_2170 = __VLS_2169({
        key: (t.name),
        type: (t.denied ? 'danger' : 'success'),
        size: "small",
        title: (t.denied ? '被 deny 屏蔽' : '可用'),
    }, ...__VLS_functionalComponentArgsRest(__VLS_2169));
    const { default: __VLS_2173 } = __VLS_2171.slots;
    (t.name);
    // @ts-ignore
    [toolPolicyPreview,];
    var __VLS_2171;
    // @ts-ignore
    [];
}
if (!__VLS_ctx.toolPolicyPreview.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
}
// @ts-ignore
[toolPolicyPreview,];
var __VLS_1963;
// @ts-ignore
[];
var __VLS_47;
// @ts-ignore
[];
var __VLS_41;
// @ts-ignore
[];
var __VLS_3;
// @ts-ignore
var __VLS_106 = __VLS_105;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
