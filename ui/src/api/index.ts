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

export interface AgentInfo {
  id: string
  name: string
  description?: string
  model: string
  status: string
  workspaceDir: string
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

export const agents = {
  list: () => api.get<AgentInfo[]>('/agents'),
  get: (id: string) => api.get<AgentInfo>(`/agents/${id}`),
  create: (data: { id: string; name: string; model: string }) => api.post<AgentInfo>('/agents', data),
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
  list: () => api.get<CronJob[]>('/cron'),
  create: (job: Partial<CronJob>) => api.post<CronJob>('/cron', job),
  update: (jobId: string, job: Partial<CronJob>) => api.patch<CronJob>(`/cron/${jobId}`, job),
  delete: (jobId: string) => api.delete(`/cron/${jobId}`),
  run: (jobId: string) => api.post(`/cron/${jobId}/run`),
  runs: (jobId: string) => api.get(`/cron/${jobId}/runs`),
}

// SSE chat helper — uses fetch + ReadableStream for auth header support
export function chatSSE(agentId: string, message: string, onEvent: (ev: any) => void): AbortController {
  const ctrl = new AbortController()
  const token = localStorage.getItem('aipanel_token')
  
  fetch(`/api/agents/${agentId}/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body: JSON.stringify({ message }),
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
        // If we didn't get a 'done' event, send one
        onEvent({ type: 'done' })
        break
      }
      buffer += decoder.decode(value, { stream: true })
      
      // Parse newline-delimited SSE: data: {...}\n\n
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

export default api
