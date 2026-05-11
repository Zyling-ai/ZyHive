/**
 * useCurrency — global reactive store for aiteam multi-currency display.
 *
 * Internal accounting is always USDT (see docs/aiteam-fx-and-currency.md).
 * This composable picks the user's preferred display currency from
 * localStorage, fetches FX rates via /api/aiteam/fx/rates (when the
 * wallet flag is on), and exposes a `formatMoney(usdt)` helper for
 * every aiteam view.
 *
 * Updates rates every 5 minutes. Falls back to last known rate when
 * the network request fails. Falls back to 1.0 when the backend
 * returns 404 (wallet flag off) — display layer still renders the
 * USDT number.
 */

import { ref, computed, readonly } from 'vue'
import api from '../api'

export type Currency =
  | 'USDT' | 'USD' | 'CNY' | 'EUR' | 'JPY'
  | 'GBP' | 'KRW' | 'HKD' | 'TWD'

export const SUPPORTED_CURRENCIES: Currency[] = [
  'USDT', 'USD', 'CNY', 'EUR', 'JPY',
  'GBP', 'KRW', 'HKD', 'TWD',
]

interface FxSnapshot {
  base: string
  rates: Record<string, number>
  source: string
  fetched_at: string
  overrides?: Record<string, number>
}

// Hardcoded fallback rates matching pkg/aiteam/fx Go side.
// These are spring-2026 approximations; the live API overrides them
// when the backend is reachable.
const FALLBACK_RATES: Record<Currency, number> = {
  USDT: 1.0, USD: 1.0, CNY: 7.18, EUR: 0.93, JPY: 155,
  GBP: 0.79, KRW: 1380, HKD: 7.81, TWD: 32.4,
}

const LS_KEY = 'aiteam.currency'
const POLL_INTERVAL_MS = 5 * 60 * 1000  // 5 minutes

// Module-level state so every component shares the same currency / rates.
const _currency = ref<Currency>(loadCurrencyPref())
const _rates = ref<Record<string, number>>({ ...FALLBACK_RATES })
const _source = ref<string>('fallback')
const _fetchedAt = ref<string>('')
let _pollTimer: ReturnType<typeof setInterval> | null = null

function loadCurrencyPref(): Currency {
  if (typeof localStorage === 'undefined') return 'USDT'
  const v = localStorage.getItem(LS_KEY)
  if (v && SUPPORTED_CURRENCIES.includes(v as Currency)) {
    return v as Currency
  }
  return 'USDT'
}

function saveCurrencyPref(c: Currency) {
  try { localStorage.setItem(LS_KEY, c) } catch { /* private mode */ }
}

/**
 * Format a USDT amount in the user's chosen display currency.
 *
 * Acceptable input: number or numeric string (decimal precision
 * preserved up to 6 places — the backend always sends strings to
 * avoid float64 loss; this helper uses parseFloat which is fine for
 * UI rendering up to ~9 significant figures).
 */
export function formatMoney(usdt: number | string, opts: { currency?: Currency } = {}): string {
  const amt = (typeof usdt === 'string') ? parseFloat(usdt) : usdt
  if (!Number.isFinite(amt)) return '—'

  const cur: Currency = opts.currency ?? _currency.value
  const rate = _rates.value[cur] ?? FALLBACK_RATES[cur] ?? 1.0
  const display = amt * rate

  switch (cur) {
    case 'USDT': return `${display.toFixed(2)} USDT`
    case 'USD':  return `$${display.toFixed(2)}`
    case 'CNY':  return `¥${display.toFixed(2)}`
    case 'EUR':  return `€${display.toFixed(2)}`
    case 'JPY':  return `¥${display.toFixed(0)}`     // JPY has no decimals
    case 'GBP':  return `£${display.toFixed(2)}`
    case 'KRW':  return `₩${display.toFixed(0)}`     // KRW has no decimals
    case 'HKD':  return `HK$${display.toFixed(2)}`
    case 'TWD':  return `NT$${display.toFixed(2)}`
    default:     return `${display.toFixed(2)} ${cur}`
  }
}

async function refreshRates() {
  try {
    const { data } = await api.get<FxSnapshot>('/aiteam/fx/rates')
    if (data && data.rates) {
      _rates.value = data.rates
      _source.value = data.source || 'unknown'
      _fetchedAt.value = data.fetched_at || ''
    }
  } catch (e: any) {
    // 404 means flag is off; quietly keep fallback rates.
    // Any other error is also non-fatal — UI keeps rendering.
    if (e?.response?.status && e.response.status !== 404) {
      console.warn('[aiteam/fx] rate refresh failed:', e.response.status)
    }
  }
}

/**
 * Hook: returns the reactive currency state plus a formatter.
 *
 * Caller mounts this once (e.g. in App.vue) to kick off polling; all
 * other component callers just read the values.
 */
export function useCurrency() {
  // Kick off polling on first call.
  if (!_pollTimer && typeof setInterval !== 'undefined') {
    refreshRates()
    _pollTimer = setInterval(refreshRates, POLL_INTERVAL_MS)
  }

  function setCurrency(c: Currency) {
    if (!SUPPORTED_CURRENCIES.includes(c)) return
    _currency.value = c
    saveCurrencyPref(c)
  }

  function refresh() {
    return refreshRates()
  }

  return {
    currency: readonly(_currency),
    setCurrency,
    refresh,
    rate: computed(() => _rates.value[_currency.value] ?? 1.0),
    source: readonly(_source),
    fetchedAt: readonly(_fetchedAt),
    rates: readonly(_rates),
    formatMoney,
    supported: SUPPORTED_CURRENCIES,
  }
}

/**
 * Resets the module state (for tests).
 */
export function _resetCurrencyForTests() {
  if (_pollTimer) {
    clearInterval(_pollTimer)
    _pollTimer = null
  }
  _currency.value = 'USDT'
  _rates.value = { ...FALLBACK_RATES }
  _source.value = 'fallback'
  _fetchedAt.value = ''
}
