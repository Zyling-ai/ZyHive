import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use(cfg => {
  const token = localStorage.getItem('aipanel_token')
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401) {
      localStorage.removeItem('aipanel_token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

// ── Types ────────────────────────────────────────────────────────────────

export interface AgentInfo {
  id: string
  name: string
  description?: string
  model: string
  modelId?: string
  channelIds?: string[]
  toolIds?: string[]
  skillIds?: string[]
  avatarColor?: string
  status: string
  workspaceDir: string
}

export interface ModelEntry {
  id: string
  name: string
  provider: string
  model: string
  apiKey: string
  baseUrl?: string
  isDefault: boolean
  status: string // "ok" | "error" | "untested"
}

export interface ProbeModelInfo {
  id: string
  name: string
}

export interface AllowedUserInfo {
  id: number
  username?: string
  firstName?: string
}

export interface ChannelEntry {
  id: string
  name: string
  type: string
  config: Record<string, string>
  enabled: boolean
  status: string
  allowedFromUsers?: AllowedUserInfo[]
}

export interface ToolEntry {
  id: string
  name: string
  type: string
  apiKey: string
  baseUrl?: string
  enabled: boolean
  status: string
}

export interface SkillEntry {
  id: string
  name: string
  description: string
  version: string
  path: string
  enabled: boolean
}

export interface FileEntry {
  name: string
  isDir: boolean
  size: number
  modTime: string
}

export interface CronJob {
  id: string
  name: string
  remark?: string
  enabled: boolean
  schedule: { kind: string; expr: string; tz: string }
  payload: { kind: string; message: string; model?: string }
  delivery: { mode: string }
  agentId?: string
  createdAtMs: number
  state?: {
    nextRunAtMs?: number
    lastRunAtMs?: number
    lastStatus?: string
  }
}

// ── API Calls ────────────────────────────────────────────────────────────

export const agents = {
  list: () => api.get<AgentInfo[]>('/agents'),
  get: (id: string) => api.get<AgentInfo>(`/agents/${id}`),
  create: (data: Partial<AgentInfo> & { id: string; name: string }) => api.post<AgentInfo>('/agents', data),
  /** Agent 间通信：向目标 Agent 发消息，同步等待回复 */
  message: (targetId: string, message: string, fromAgentId?: string) =>
    api.post<{ response: string }>(`/agents/${targetId}/message`, { message, fromAgentId }),
}

export const models = {
  list: () => api.get<ModelEntry[]>('/models'),
  create: (data: Partial<ModelEntry>) => api.post<ModelEntry>('/models', data),
  update: (id: string, data: Partial<ModelEntry>) => api.patch<ModelEntry>(`/models/${id}`, data),
  delete: (id: string) => api.delete(`/models/${id}`),
  test: (id: string) => api.post<{ valid: boolean; error?: string }>(`/models/${id}/test`),
  probe: (baseUrl: string, apiKey?: string, provider?: string) =>
    api.get<{ models: ProbeModelInfo[]; count: number }>('/models/probe', {
      params: { baseUrl, apiKey: apiKey || undefined, provider: provider || undefined },
    }),
  envKeys: () =>
    api.get<{ envKeys: { provider: string; envVar: string; masked: string; baseUrl: string }[] }>('/models/env-keys'),
}

// Global channel registry (deprecated — kept for backward compat)
export const channels = {
  list: () => api.get<ChannelEntry[]>('/channels'),
  create: (data: Partial<ChannelEntry>) => api.post<ChannelEntry>('/channels', data),
  update: (id: string, data: Partial<ChannelEntry>) => api.patch<ChannelEntry>(`/channels/${id}`, data),
  delete: (id: string) => api.delete(`/channels/${id}`),
  test: (id: string) => api.post<{ valid: boolean }>(`/channels/${id}/test`),
}

// Per-agent channel config — each member manages its own bot tokens
export const agentChannels = {
  list: (agentId: string) => api.get<ChannelEntry[]>(`/agents/${agentId}/channels`),
  set: (agentId: string, channels: ChannelEntry[]) => api.put(`/agents/${agentId}/channels`, channels),
  test: (agentId: string, chId: string) => api.post<{ valid: boolean; botName?: string; error?: string }>(`/agents/${agentId}/channels/${chId}/test`),
  // Pending users
  checkToken: (agentId: string, token: string) => api.post<{ valid: boolean; botName?: string; duplicate?: boolean; usedBy?: string; usedByCh?: string; error?: string }>(`/agents/${agentId}/channels/check-token`, { token }),
  listPending: (agentId: string, chId: string) => api.get<PendingUser[]>(`/agents/${agentId}/channels/${chId}/pending`),
  allowUser: (agentId: string, chId: string, userId: number) => api.post(`/agents/${agentId}/channels/${chId}/pending/${userId}/allow`),
  dismissUser: (agentId: string, chId: string, userId: number) => api.delete(`/agents/${agentId}/channels/${chId}/pending/${userId}`),
  removeAllowed: (agentId: string, chId: string, userId: number) => api.delete(`/agents/${agentId}/channels/${chId}/allowed/${userId}`),
}

export interface PendingUser {
  id: number
  username?: string
  firstName?: string
  lastSeen: number
}

export const tools = {
  list: () => api.get<ToolEntry[]>('/tools'),
  create: (data: Partial<ToolEntry>) => api.post<ToolEntry>('/tools', data),
  update: (id: string, data: Partial<ToolEntry>) => api.patch<ToolEntry>(`/tools/${id}`, data),
  delete: (id: string) => api.delete(`/tools/${id}`),
  test: (id: string) => api.post<{ valid: boolean }>(`/tools/${id}/test`),
}

export const skills = {
  list: () => api.get<SkillEntry[]>('/skills'),
  install: (data: Partial<SkillEntry>) => api.post<SkillEntry>('/skills/install', data),
  delete: (id: string) => api.delete(`/skills/${id}`),
}

// Per-agent skill metadata (skill.json based, stored in agent workspace)
export interface AgentSkillMeta {
  id: string
  name: string
  version: string
  icon: string
  category: string
  description: string
  enabled: boolean
  installedAt: string
  source: string
}

export const agentSkills = {
  list: (agentId: string) => api.get<AgentSkillMeta[]>(`/agents/${agentId}/skills`),
  create: (agentId: string, data: { meta: Partial<AgentSkillMeta>; promptContent?: string }) =>
    api.post<AgentSkillMeta>(`/agents/${agentId}/skills`, data),
  update: (agentId: string, skillId: string, data: Partial<AgentSkillMeta>) =>
    api.patch<AgentSkillMeta>(`/agents/${agentId}/skills/${skillId}`, data),
  remove: (agentId: string, skillId: string) => api.delete(`/agents/${agentId}/skills/${skillId}`),
}

export const files = {
  read: (agentId: string, path: string) => api.get(`/agents/${agentId}/files/${path}`),
  write: (agentId: string, path: string, content: string) =>
    api.put(`/agents/${agentId}/files/${path}`, content, { headers: { 'Content-Type': 'text/plain' } }),
  delete: (agentId: string, path: string) => api.delete(`/agents/${agentId}/files/${path}`),
}

export const config = {
  get: () => api.get('/config'),
  patch: (data: any) => api.patch('/config', data),
  testKey: (provider: string, key: string) => api.post('/config/test-key', { provider, key }),
}

export const memoryApi = {
  tree: (agentId: string) => api.get(`/agents/${agentId}/memory/tree`),
  readFile: (agentId: string, path: string) => api.get(`/agents/${agentId}/memory/file/${path}`),
  writeFile: (agentId: string, path: string, content: string) =>
    api.put(`/agents/${agentId}/memory/file/${path}`, content, { headers: { 'Content-Type': 'text/plain' } }),
  dailyLog: (agentId: string, content: string) =>
    api.post(`/agents/${agentId}/memory/daily`, content, { headers: { 'Content-Type': 'text/plain' } }),
}

export const cron = {
  /** List all jobs. Pass agentId to filter by owner; '__global__' for jobs with no owner. */
  list: (agentId?: string) => api.get<CronJob[]>('/cron', { params: agentId ? { agentId } : undefined }),
  create: (job: Partial<CronJob>) => api.post<CronJob>('/cron', job),
  update: (jobId: string, job: Partial<CronJob>) => api.patch<CronJob>(`/cron/${jobId}`, job),
  delete: (jobId: string) => api.delete(`/cron/${jobId}`),
  run: (jobId: string) => api.post(`/cron/${jobId}/run`),
  runs: (jobId: string) => api.get(`/cron/${jobId}/runs`),
}

// ChatParams are optional extra parameters passed through to the model.
export interface ChatParams {
  sessionId?: string // resume existing session; if set, server loads history, no need to send history[]
  context?: string   // extra system context (scenario background, page state)
  scenario?: string  // label e.g. "agent-creation", "general"
  images?: string[]  // base64 data URIs
  history?: { role: 'user' | 'assistant'; content: string }[]  // prior turns for multi-turn context
}

// SSE chat helper
export function chatSSE(agentId: string, message: string, onEvent: (ev: any) => void, params?: ChatParams): AbortController {
  const ctrl = new AbortController()
  const token = localStorage.getItem('aipanel_token')

  fetch(`/api/agents/${agentId}/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body: JSON.stringify({ message, ...params }),
    signal: ctrl.signal
  }).then(async res => {
    if (!res.ok) {
      const text = await res.text()
      try {
        const err = JSON.parse(text)
        onEvent({ type: 'error', error: err.error || `HTTP ${res.status}` })
      } catch {
        onEvent({ type: 'error', error: `HTTP ${res.status}: ${text}` })
      }
      return
    }
    const reader = res.body?.getReader()
    if (!reader) return
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        onEvent({ type: 'done' })
        break
      }
      buffer += decoder.decode(value, { stream: true })
      const parts = buffer.split('\n')
      buffer = parts.pop() || ''
      for (const line of parts) {
        const trimmed = line.trim()
        if (trimmed.startsWith('data: ')) {
          try {
            const data = JSON.parse(trimmed.slice(6))
            onEvent(data)
            if (data.type === 'done') return
          } catch {}
        }
      }
    }
  }).catch((err) => {
    if (err.name !== 'AbortError') {
      onEvent({ type: 'error', error: err.message || 'Network error' })
    }
  })
  return ctrl
}

// ── Session types ────────────────────────────────────────────────────────

export interface SessionSummary {
  id: string
  agentId: string
  agentName: string
  filePath: string
  createdAt: number
  title?: string
  messageCount: number
  lastAt: number
  tokenEstimate: number
}

export interface ParsedMessage {
  role: 'user' | 'assistant' | 'compaction'
  text: string
  timestamp: number
  isCompact?: boolean
}

export interface SessionDetail {
  session: SessionSummary
  messages: ParsedMessage[]
  agent: { id: string; name: string }
}

// ── Sessions API ─────────────────────────────────────────────────────────

export const sessions = {
  list: (params?: { agentId?: string; limit?: number }) =>
    api.get<{ sessions: SessionSummary[]; total: number }>('/sessions', { params }),
  get: (agentId: string, sid: string) =>
    api.get<SessionDetail>(`/sessions/${agentId}/${sid}`),
  delete: (agentId: string, sid: string) =>
    api.delete(`/sessions/${agentId}/${sid}`),
  rename: (agentId: string, sid: string, title: string) =>
    api.patch(`/sessions/${agentId}/${sid}`, { title }),
}

// ── Stats API ─────────────────────────────────────────────────────────────

export interface StatsResult {
  agents: { total: number; running: number }
  sessions: { total: number; totalMessages: number; totalTokens: number }
  topAgents: { id: string; name: string; sessions: number; messages: number; tokens: number }[]
}

export const statsApi = {
  get: () => api.get<StatsResult>('/stats'),
}

// ── Logs API ──────────────────────────────────────────────────────────────

export const logsApi = {
  get: (limit = 200) => api.get<{ lines: string[] }>('/logs', { params: { limit } }),
}

// ── Relations API ─────────────────────────────────────────────────────────

export interface RelationRow {
  agentId: string
  agentName: string
  relationType: string
  strength: string
  desc: string
}

export interface RelationsResponse {
  content: string
  parsed: RelationRow[]
}

export interface TeamGraphNode {
  id: string
  name: string
  status: string
}

export interface TeamGraphEdge {
  from: string
  to: string
  type: string
  strength: string
  label: string
}

export interface TeamGraph {
  nodes: TeamGraphNode[]
  edges: TeamGraphEdge[]
}

export const relationsApi = {
  get: (agentId: string) => api.get<RelationsResponse>(`/agents/${agentId}/relations`),
  put: (agentId: string, content: string) =>
    api.put(`/agents/${agentId}/relations`, content, { headers: { 'Content-Type': 'text/plain' } }),
  graph: () => api.get<TeamGraph>('/team/graph'),
}

export interface MemConfig {
  enabled: boolean
  schedule: 'hourly' | 'every6h' | 'daily' | 'weekly'
  keepTurns: number
  focusHint: string
  cronJobId: string
}

export interface MemRunLog {
  timestamp: number // unix ms
  status: 'ok' | 'error'
  message: string
}

export const memoryConfigApi = {
  getConfig: (agentId: string) => api.get<MemConfig>(`/agents/${agentId}/memory/config`),
  setConfig: (agentId: string, cfg: Partial<MemConfig>) =>
    api.put<MemConfig>(`/agents/${agentId}/memory/config`, cfg),
  consolidate: (agentId: string) =>
    api.post<{ ok: boolean; message: string }>(`/agents/${agentId}/memory/consolidate`),
  runLog: (agentId: string) =>
    api.get<MemRunLog[]>(`/agents/${agentId}/memory/run-log`),
}

// ── Conversation Log API ──────────────────────────────────────────────────

export interface ConvEntry {
  ts: string
  role: 'user' | 'assistant'
  content: string
  channelId: string
  channelType: string
  sender?: string
}

export interface ChannelSummary {
  channelId: string
  channelType: string
  messageCount: number
  lastAt: string
  firstAt: string
}

export interface GlobalConvRow {
  agentId: string
  agentName: string
  channelId: string
  channelType: string
  messageCount: number
  lastAt: string
  firstAt: string
}

export const agentConversations = {
  list: (agentId: string) =>
    api.get<ChannelSummary[]>(`/agents/${agentId}/conversations`),
  messages: (agentId: string, channelId: string, params?: { limit?: number; offset?: number }) =>
    api.get<{ total: number; messages: ConvEntry[] }>(
      `/agents/${agentId}/conversations/${channelId}`,
      { params }
    ),
  globalList: (params?: { agentId?: string; channelType?: string }) =>
    api.get<GlobalConvRow[]>('/conversations', { params }),
}

export default api
