// useApprovals — global approval store (F-01, 26.5.12v1).
//
// Single SSE connection to /api/approvals/stream maintained for the lifetime
// of the app (started on first import). All views share this state:
//   - pending: list of unresolved ApprovalRequest
//   - lastEvent: last received ApprovalEvent (for animation triggers)
//
// REST helpers wrap POST /approvals/:id/{approve,deny}.
import { ref } from 'vue'

export interface ApprovalRequest {
  id: string
  agentId: string
  sessionId?: string
  toolName: string
  input: any
  createdAt: string
  expiresAt: string
}

export interface ApprovalDecision {
  approved: boolean
  reason?: string
  by?: string
}

export interface ApprovalEvent {
  type: 'hello' | 'approval_request' | 'approval_resolved' | 'approval_expired'
  request?: ApprovalRequest
  id?: string
  decision?: ApprovalDecision
}

const pending = ref<ApprovalRequest[]>([])
const lastEvent = ref<ApprovalEvent | null>(null)
const connected = ref(false)

let es: EventSource | null = null
let started = false

function token(): string {
  return localStorage.getItem('aipanel_token') || ''
}

function refresh() {
  fetch('/api/approvals/pending', {
    headers: { 'Authorization': `Bearer ${token()}` },
  })
    .then(r => r.ok ? r.json() : { pending: [] })
    .then(j => { pending.value = j.pending || [] })
    .catch(() => {})
}

function open() {
  // EventSource doesn't support custom headers; encode token as query param.
  const url = `/api/approvals/stream?token=${encodeURIComponent(token())}`
  es = new EventSource(url, { withCredentials: false } as any)
  es.onopen = () => { connected.value = true }
  es.onerror = () => {
    connected.value = false
    es?.close()
    es = null
    // Reconnect after 5s; gives backend time to recover.
    setTimeout(() => { if (started) open() }, 5000)
  }
  es.onmessage = (msg) => {
    try {
      const ev: ApprovalEvent = JSON.parse(msg.data)
      lastEvent.value = ev
      if (ev.type === 'approval_request' && ev.request) {
        // Append; dedupe by id.
        const exists = pending.value.find(p => p.id === ev.request!.id)
        if (!exists) pending.value.push(ev.request)
      } else if ((ev.type === 'approval_resolved' || ev.type === 'approval_expired') && ev.id) {
        pending.value = pending.value.filter(p => p.id !== ev.id)
      }
    } catch {}
  }
}

export function useApprovals() {
  if (!started) {
    started = true
    refresh()
    open()
  }
  return { pending, lastEvent, connected, refresh }
}

export async function approve(id: string, reason = '') {
  const resp = await fetch(`/api/approvals/${encodeURIComponent(id)}/approve`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token()}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ reason }),
  })
  return resp.ok
}

export async function deny(id: string, reason = '') {
  const resp = await fetch(`/api/approvals/${encodeURIComponent(id)}/deny`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token()}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ reason }),
  })
  return resp.ok
}
