import axios from 'axios';
// 支持跨服务器连接：从 localStorage 读取服务器地址
function getBaseURL() {
    const saved = localStorage.getItem('aipanel_url');
    if (saved)
        return `${saved}/api`;
    return '/api';
}
const api = axios.create({ baseURL: getBaseURL() });
api.interceptors.request.use(cfg => {
    const token = localStorage.getItem('aipanel_token');
    if (token)
        cfg.headers.Authorization = `Bearer ${token}`;
    return cfg;
});
api.interceptors.response.use(res => res, err => {
    if (err.response?.status === 401) {
        localStorage.removeItem('aipanel_token');
        // Avoid redirect loop: only navigate away if not already on /login
        if (!window.location.pathname.startsWith('/login')) {
            window.location.href = '/login';
        }
    }
    return Promise.reject(err);
});
// ── API Calls ────────────────────────────────────────────────────────────
export const agents = {
    list: () => api.get('/agents'),
    get: (id) => api.get(`/agents/${id}`),
    create: (data) => api.post('/agents', data),
    update: (id, data) => api.patch(`/agents/${id}`, data),
    delete: (id) => api.delete(`/agents/${id}`),
    /** Agent 间通信：向目标 Agent 发消息，同步等待回复 */
    message: (targetId, message, fromAgentId) => api.post(`/agents/${targetId}/message`, { message, fromAgentId }),
    /** 获取成员的会话历史列表 */
    listSessions: (agentId, limit = 20) => api.get(`/sessions?agentId=${encodeURIComponent(agentId)}&limit=${limit}`),
};
export const providers = {
    list: () => api.get('/providers'),
    create: (data) => api.post('/providers', data),
    update: (id, data) => api.put(`/providers/${id}`, data),
    delete: (id) => api.delete(`/providers/${id}`),
    test: (id) => api.post(`/providers/${id}/test`),
};
export const models = {
    list: () => api.get('/models'),
    create: (data) => api.post('/models', data),
    update: (id, data) => api.patch(`/models/${id}`, data),
    delete: (id) => api.delete(`/models/${id}`),
    test: (id) => api.post(`/models/${id}/test`),
    probe: (baseUrl, apiKey, provider, providerId) => api.get('/models/probe', {
        params: { baseUrl, apiKey: apiKey || undefined, provider: provider || undefined, providerId: providerId || undefined },
    }),
    envKeys: () => api.get('/models/env-keys'),
};
// Global channel registry (deprecated — kept for backward compat)
export const channels = {
    list: () => api.get('/channels'),
    create: (data) => api.post('/channels', data),
    update: (id, data) => api.patch(`/channels/${id}`, data),
    delete: (id) => api.delete(`/channels/${id}`),
    test: (id) => api.post(`/channels/${id}/test`),
};
// Per-agent channel config — each member manages its own bot tokens
export const agentChannels = {
    list: (agentId) => api.get(`/agents/${agentId}/channels`),
    set: (agentId, channels) => api.put(`/agents/${agentId}/channels`, channels),
    test: (agentId, chId) => api.post(`/agents/${agentId}/channels/${chId}/test`),
    // Pending users
    checkToken: (agentId, token) => api.post(`/agents/${agentId}/channels/check-token`, { token }),
    listPending: (agentId, chId) => api.get(`/agents/${agentId}/channels/${chId}/pending`),
    allowUser: (agentId, chId, userId) => api.post(`/agents/${agentId}/channels/${chId}/pending/${userId}/allow`),
    dismissUser: (agentId, chId, userId) => api.delete(`/agents/${agentId}/channels/${chId}/pending/${userId}`),
    removeAllowed: (agentId, chId, userId) => api.delete(`/agents/${agentId}/channels/${chId}/allowed/${userId}`),
};
export const tools = {
    list: () => api.get('/tools'),
    create: (data) => api.post('/tools', data),
    update: (id, data) => api.patch(`/tools/${id}`, data),
    delete: (id) => api.delete(`/tools/${id}`),
    test: (id) => api.post(`/tools/${id}/test`),
};
export const skills = {
    list: () => api.get('/skills'),
    install: (data) => api.post('/skills/install', data),
    delete: (id) => api.delete(`/skills/${id}`),
};
export const agentSkills = {
    list: (agentId) => api.get(`/agents/${agentId}/skills`),
    create: (agentId, data) => api.post(`/agents/${agentId}/skills`, data),
    update: (agentId, skillId, data) => api.patch(`/agents/${agentId}/skills/${skillId}`, data),
    remove: (agentId, skillId) => api.delete(`/agents/${agentId}/skills/${skillId}`),
};
export const files = {
    read: (agentId, path) => api.get(`/agents/${agentId}/files/${path}`),
    readTree: (agentId) => api.get(`/agents/${agentId}/files/?tree=true`),
    write: (agentId, path, content) => api.put(`/agents/${agentId}/files/${path}`, content, { headers: { 'Content-Type': 'text/plain' } }),
    delete: (agentId, path) => api.delete(`/agents/${agentId}/files/${path}`),
};
export const networkApi = {
    list: (agentId) => api.get(`/agents/${agentId}/network/contacts`),
    get: (agentId, contactId) => api.get(`/agents/${agentId}/network/contacts/${encodeURIComponent(contactId)}`),
    update: (agentId, contactId, patch) => api.patch(`/agents/${agentId}/network/contacts/${encodeURIComponent(contactId)}`, patch),
    delete: (agentId, contactId) => api.delete(`/agents/${agentId}/network/contacts/${encodeURIComponent(contactId)}`),
    merge: (agentId, primaryId, aliasId) => api.post(`/agents/${agentId}/network/contacts/${encodeURIComponent(primaryId)}/merge`, { aliasId }),
    refresh: (agentId) => api.post(`/agents/${agentId}/network/refresh`),
    // Chat profile (26.4.24v1)
    listChats: (agentId) => api.get(`/agents/${agentId}/network/chats`),
    getChat: (agentId, chatId) => api.get(`/agents/${agentId}/network/chats/${encodeURIComponent(chatId)}`),
    updateChat: (agentId, chatId, patch) => api.patch(`/agents/${agentId}/network/chats/${encodeURIComponent(chatId)}`, patch),
    deleteChat: (agentId, chatId) => api.delete(`/agents/${agentId}/network/chats/${encodeURIComponent(chatId)}`),
};
export const config = {
    get: () => api.get('/config'),
    patch: (data) => api.patch('/config', data),
    testKey: (provider, key) => api.post('/config/test-key', { provider, key }),
};
export const memoryApi = {
    tree: (agentId) => api.get(`/agents/${agentId}/memory/tree`),
    readFile: (agentId, path) => api.get(`/agents/${agentId}/memory/file/${path}`),
    writeFile: (agentId, path, content) => api.put(`/agents/${agentId}/memory/file/${path}`, content, { headers: { 'Content-Type': 'text/plain' } }),
    dailyLog: (agentId, content) => api.post(`/agents/${agentId}/memory/daily`, content, { headers: { 'Content-Type': 'text/plain' } }),
};
export const cron = {
    /** List all jobs. Pass agentId to filter by owner; '__global__' for jobs with no owner. */
    list: (agentId) => api.get('/cron', { params: agentId ? { agentId } : undefined }),
    create: (job) => api.post('/cron', job),
    update: (jobId, job) => api.patch(`/cron/${jobId}`, job),
    delete: (jobId) => api.delete(`/cron/${jobId}`),
    run: (jobId) => api.post(`/cron/${jobId}/run`),
    runs: (jobId) => api.get(`/cron/${jobId}/runs`),
};
// SSE chat helper
export function chatSSE(agentId, message, onEvent, params) {
    const ctrl = new AbortController();
    const token = localStorage.getItem('aipanel_token');
    fetch(`/api/agents/${agentId}/chat`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {})
        },
        body: JSON.stringify({ message, ...params }),
        signal: ctrl.signal
    }).then(async (res) => {
        if (!res.ok) {
            const text = await res.text();
            try {
                const err = JSON.parse(text);
                onEvent({ type: 'error', error: err.error || `HTTP ${res.status}` });
            }
            catch {
                onEvent({ type: 'error', error: `HTTP ${res.status}: ${text}` });
            }
            return;
        }
        const reader = res.body?.getReader();
        if (!reader)
            return;
        const decoder = new TextDecoder();
        let buffer = '';
        while (true) {
            const { done, value } = await reader.read();
            if (done) {
                onEvent({ type: 'done' });
                break;
            }
            buffer += decoder.decode(value, { stream: true });
            const parts = buffer.split('\n');
            buffer = parts.pop() || '';
            for (const line of parts) {
                const trimmed = line.trim();
                if (trimmed.startsWith('data: ')) {
                    try {
                        const data = JSON.parse(trimmed.slice(6));
                        onEvent(data);
                        if (data.type === 'done' || data.type === 'error')
                            return;
                    }
                    catch { }
                }
            }
        }
    }).catch((err) => {
        if (err.name !== 'AbortError') {
            onEvent({ type: 'error', error: err.message || 'Network error' });
        }
    });
    return ctrl;
}
// Resume an existing generation by subscribing to the session's broadcaster.
// Returns buffered events first (replay), then live events.
// If the worker no longer exists, receives {type:"idle"} immediately.
export function resumeSSE(agentId, sessionId, onEvent) {
    const ctrl = new AbortController();
    const token = localStorage.getItem('aipanel_token');
    fetch(`/api/agents/${agentId}/chat/stream?sessionId=${encodeURIComponent(sessionId)}`, {
        method: 'GET',
        headers: {
            ...(token ? { Authorization: `Bearer ${token}` } : {})
        },
        signal: ctrl.signal
    }).then(async (res) => {
        if (!res.ok) {
            onEvent({ type: 'idle' });
            return;
        }
        const reader = res.body?.getReader();
        if (!reader) {
            onEvent({ type: 'idle' });
            return;
        }
        const decoder = new TextDecoder();
        let buffer = '';
        while (true) {
            const { done, value } = await reader.read();
            if (done)
                break;
            buffer += decoder.decode(value, { stream: true });
            const parts = buffer.split('\n');
            buffer = parts.pop() || '';
            for (const line of parts) {
                const trimmed = line.trim();
                if (trimmed.startsWith('data: ')) {
                    try {
                        const data = JSON.parse(trimmed.slice(6));
                        onEvent(data);
                        if (data.type === 'done' || data.type === 'error' || data.type === 'idle')
                            return;
                    }
                    catch { }
                }
            }
        }
    }).catch((err) => {
        if (err.name !== 'AbortError') {
            onEvent({ type: 'idle' });
        }
    });
    return ctrl;
}
// Check if a session has an active background worker.
export async function getSessionStatus(agentId, sessionId) {
    const token = localStorage.getItem('aipanel_token');
    try {
        const res = await fetch(`/api/agents/${agentId}/chat/status?sessionId=${encodeURIComponent(sessionId)}`, {
            headers: token ? { Authorization: `Bearer ${token}` } : {}
        });
        if (res.ok)
            return await res.json();
    }
    catch { }
    return { status: 'idle', hasWorker: false, bufferedEvents: 0 };
}
// ── Sessions API ─────────────────────────────────────────────────────────
export const sessions = {
    list: (params) => api.get('/sessions', { params }),
    get: (agentId, sid) => api.get(`/agents/${agentId}/sessions/${sid}`),
    delete: (agentId, sid) => api.delete(`/sessions/${agentId}/${sid}`),
    rename: (agentId, sid, title) => api.patch(`/sessions/${agentId}/${sid}`, { title }),
};
export const statsApi = {
    get: () => api.get('/stats'),
};
// ── Logs API ──────────────────────────────────────────────────────────────
export const logsApi = {
    get: (limit = 200) => api.get('/logs', { params: { limit } }),
};
export const relationsApi = {
    get: (agentId) => api.get(`/agents/${agentId}/relations`),
    put: (agentId, content) => api.put(`/agents/${agentId}/relations`, content, { headers: { 'Content-Type': 'text/plain' } }),
    graph: () => api.get('/team/graph'),
    clearAll: () => api.delete('/team/relations'),
    putEdge: (from, to, type, strength, desc) => api.put('/team/relations/edge', { from, to, type, strength, desc }),
    deleteEdge: (from, to) => api.delete(`/team/relations/edge?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
};
export const memoryConfigApi = {
    getConfig: (agentId) => api.get(`/agents/${agentId}/memory/config`),
    setConfig: (agentId, cfg) => api.put(`/agents/${agentId}/memory/config`, cfg),
    consolidate: (agentId) => api.post(`/agents/${agentId}/memory/consolidate`),
    runLog: (agentId) => api.get(`/agents/${agentId}/memory/run-log`),
};
export const agentConversations = {
    list: (agentId) => api.get(`/agents/${agentId}/conversations`),
    messages: (agentId, channelId, params) => api.get(`/agents/${agentId}/conversations/${channelId}`, { params }),
    globalList: (params) => api.get('/conversations', { params }),
};
export const projects = {
    list: () => api.get('/projects'),
    get: (id) => api.get(`/projects/${id}`),
    create: (data) => api.post('/projects', data),
    update: (id, data) => api.patch(`/projects/${id}`, data),
    delete: (id) => api.delete(`/projects/${id}`),
    // Permissions
    setPermissions: (id, editors) => api.put(`/projects/${id}/permissions`, { editors }),
    // File management
    readTree: (id, path = '/') => api.get(`/projects/${id}/files${path}?tree=true`),
    readFile: (id, path) => api.get(`/projects/${id}/files/${path}`),
    writeFile: (id, path, content) => api.put(`/projects/${id}/files/${path}`, content, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } }),
    deleteFile: (id, path) => api.delete(`/projects/${id}/files/${path}`),
};
export const tasks = {
    list: (params) => api.get('/tasks', { params }),
    get: (id) => api.get(`/tasks/${id}`),
    spawn: (data) => api.post('/tasks', data),
    kill: (id) => api.delete(`/tasks/${id}`),
    eligibleTargets: (from, mode) => api.get('/tasks/eligible', { params: { from, mode } }),
};
export const goalsApi = {
    list: (agentId) => api.get('/goals', { params: agentId ? { agentId } : undefined }),
    create: (data) => api.post('/goals', data),
    get: (id) => api.get(`/goals/${id}`),
    update: (id, data) => api.patch(`/goals/${id}`, data),
    delete: (id) => api.delete(`/goals/${id}`),
    updateProgress: (id, progress) => api.patch(`/goals/${id}/progress`, { progress }),
    setMilestoneDone: (goalId, milestoneId, done) => api.patch(`/goals/${goalId}/milestones/${milestoneId}`, { done }),
    // Checks
    listChecks: (goalId) => api.get(`/goals/${goalId}/checks`),
    addCheck: (goalId, data) => api.post(`/goals/${goalId}/checks`, data),
    updateCheck: (goalId, checkId, data) => api.patch(`/goals/${goalId}/checks/${checkId}`, data),
    removeCheck: (goalId, checkId) => api.delete(`/goals/${goalId}/checks/${checkId}`),
    runCheck: (goalId, checkId) => api.post(`/goals/${goalId}/checks/${checkId}/run`),
    listCheckRecords: (goalId) => api.get(`/goals/${goalId}/check-records`),
};
/**
 * GET /api/subagent-events?sessionId=xxx
 * Returns stored subagent lifecycle events for a parent session.
 * Used by DispatchPanel to restore state after a page reload.
 */
export const getSubagentEvents = (sessionId) => api.get('/subagent-events', { params: { sessionId } });
export const updateApi = {
    check: () => api.get('/update/check'),
    apply: (version) => api.post('/update/apply', version ? { version } : {}),
    // status 是 public endpoint，无需 auth token（服务重启后前端仍可轮询）
    status: () => axios.get('/api/update/status'),
};
export const usageApi = {
    summary: (params) => api.get('/usage/summary', { params }),
    timeline: (params) => api.get('/usage/timeline', { params }),
    records: (params) => api.get('/usage/records', { params }),
};
export default api;
