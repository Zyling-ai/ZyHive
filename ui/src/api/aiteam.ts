/**
 * aiteam REST API client wrappers.
 *
 * Every call hits the bearer-auth-protected /api/aiteam/* surface.
 * When the relevant flag is off the endpoint returns 404; the caller
 * decides whether to treat that as "feature disabled" UI state.
 *
 * See docs/aiteam-architecture.md for the full subsystem map.
 */

import api from './index'

// ── Flags ────────────────────────────────────────────────────────────────────

export interface AITeamFlagsResponse {
  flags: Record<string, boolean>
  any: boolean
}

export async function getFlags(): Promise<AITeamFlagsResponse> {
  const { data } = await api.get('/aiteam/flags')
  return data
}

// ── Wallet ───────────────────────────────────────────────────────────────────

export interface WalletLedgerEntry {
  ts: number
  type: string
  amount_usdt: string
  balance_after_usdt: string
  reason?: string
  counterparty?: string
  fx_snapshot?: Record<string, number>
}

export interface WalletResponse {
  agentId: string
  balance_usdt: string
  recent_ledger: WalletLedgerEntry[]
}

export async function getWallet(agentId: string): Promise<WalletResponse> {
  const { data } = await api.get(`/aiteam/wallet/${encodeURIComponent(agentId)}`)
  return data
}

export async function creditWallet(agentId: string, amount_usdt: string, reason: string) {
  const { data } = await api.post(`/aiteam/wallet/${encodeURIComponent(agentId)}/credit`, {
    amount_usdt, reason,
  })
  return data
}

export async function transferWallet(from: string, to: string, amount_usdt: string, reason: string) {
  const { data } = await api.post(`/aiteam/wallet/${encodeURIComponent(from)}/transfer`, {
    to, amount_usdt, reason,
  })
  return data
}

export async function getWalletLedger(agentId: string): Promise<{ agentId: string; entries: WalletLedgerEntry[] }> {
  const { data } = await api.get(`/aiteam/wallet/${encodeURIComponent(agentId)}/ledger`)
  return data
}

// ── FX ───────────────────────────────────────────────────────────────────────

export interface FXSnapshot {
  base: string
  rates: Record<string, number>
  source: string
  fetched_at: string
  overrides?: Record<string, number>
}

export async function getFxRates(): Promise<FXSnapshot> {
  const { data } = await api.get('/aiteam/fx/rates')
  return data
}

export async function refreshFx(): Promise<{ source: string; snap: FXSnapshot }> {
  const { data } = await api.post('/aiteam/fx/refresh')
  return data
}

export async function overrideFx(currency: string, rate: number) {
  const { data } = await api.post('/aiteam/fx/override', { currency, rate })
  return data
}

export async function clearFxOverride(currency: string) {
  const { data } = await api.delete(`/aiteam/fx/override/${encodeURIComponent(currency)}`)
  return data
}

// ── BudgetGuard ──────────────────────────────────────────────────────────────

export interface GuardSnapshot {
  enabled: boolean
  day_key: string
  tz: string
  global_used_usdt: string
  limits: {
    per_agent_daily_usdt: string
    global_daily_usdt: string
    per_session_usdt: string
    cooldown_ns: number
    tz: string
  }
  agents: Record<string, {
    used_daily_usdt: string
    effective_limit_usdt: string
    panicked: boolean
    panic_reason?: string
    cooldown_until?: string
  }>
  sessions?: Record<string, { used_usdt: string; panicked?: boolean }>
}

export async function getGuard(): Promise<GuardSnapshot> {
  const { data } = await api.get('/aiteam/guard')
  return data
}

export async function releaseGuard(agentId: string, operator: string, reason: string) {
  const { data } = await api.post(`/aiteam/guard/${encodeURIComponent(agentId)}/release`, {
    operator, reason,
  })
  return data
}

export async function setGuardLimit(agentId: string, limit_usdt: string) {
  const { data } = await api.patch(`/aiteam/guard/${encodeURIComponent(agentId)}/limit`, {
    limit_usdt,
  })
  return data
}

// ── Judge ────────────────────────────────────────────────────────────────────

export interface JudgeScore {
  agent_id: string
  period: string
  completion: number
  quality: number
  communication: number
  creativity: number
  cost: number
  average: number
  rationale?: string
  source: string
  operator?: string
  ts: number
}

export async function runJudge(agentId: string, usage_cost_usd: number, call_count: number) {
  const { data } = await api.post('/aiteam/judge/run', {
    agent_id: agentId, usage_cost_usd, call_count,
  })
  return data as JudgeScore
}

export async function getJudgeScores(agentId: string, period?: string) {
  const url = `/aiteam/judge/scores/${encodeURIComponent(agentId)}`
  const { data } = await api.get(url, { params: period ? { period } : {} })
  return data as {
    agentId: string
    history?: JudgeScore[]
    average_30d?: number
    rows?: JudgeScore[]
    period?: string
  }
}

export async function overrideJudge(payload: {
  agent_id: string
  period?: string
  operator?: string
  rationale?: string
  completion: number
  quality: number
  communication: number
  creativity: number
  cost: number
}) {
  const { data } = await api.post('/aiteam/judge/override', payload)
  return data as JudgeScore
}

export async function listJudgeAgents() {
  const { data } = await api.get('/aiteam/judge/agents')
  return data as { agents: string[] }
}

// ── Payroll ──────────────────────────────────────────────────────────────────

export interface PayslipEntry {
  period: string
  agent_id: string
  base_usdt: string
  bonus_usdt: string
  bonus_factor: number
  offset_usdt: string
  net_usdt: string
  skipped: boolean
  skipped_note?: string
  ts: number
}

export async function getPayroll(agentId: string) {
  const { data } = await api.get(`/aiteam/payroll/${encodeURIComponent(agentId)}`)
  return data as { agentId: string; history: PayslipEntry[] }
}

export async function runPayroll(agent_ids?: string[], period?: string) {
  const { data } = await api.post('/aiteam/payroll/run', { agent_ids, period })
  return data as { period: string; entries: PayslipEntry[] }
}

// ── Dashboard ────────────────────────────────────────────────────────────────

export interface OverviewResponse {
  flags: Record<string, boolean>
  any: boolean
  wallet?: { total_balance_usdt: string; agents: Record<string, string>; count: number }
  fx?: FXSnapshot
  guard?: GuardSnapshot
  judge?: { agents: string[]; avg_7d_by_agent: Record<string, number> }
  payroll?: { enabled: boolean }
  revenue?: { enabled: boolean }
}

export async function getOverview(): Promise<OverviewResponse> {
  const { data } = await api.get('/aiteam/overview')
  return data
}

export interface AuditEntry {
  type: string
  subsystem: string
  agentId?: string
  sessionId?: string
  ts: number
  detail?: Record<string, any>
}

export async function getAuditTail(limit = 200): Promise<{ entries: AuditEntry[]; count: number; limit: number }> {
  const { data } = await api.get('/aiteam/audit', { params: { limit } })
  return data
}
