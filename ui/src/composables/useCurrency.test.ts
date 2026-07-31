import { beforeEach, describe, expect, it } from 'vitest'
import {
  _resetCurrencyForTests,
  formatMoney,
  SUPPORTED_CURRENCIES,
} from './useCurrency'

describe('formatMoney', () => {
  beforeEach(() => {
    _resetCurrencyForTests()
  })

  it('formats the accounting currency with two decimals', () => {
    expect(formatMoney('12.345')).toBe('12.35 USDT')
  })

  it('uses the selected fallback exchange rate', () => {
    expect(formatMoney(10, { currency: 'CNY' })).toBe('¥71.80')
    expect(formatMoney(10, { currency: 'JPY' })).toBe('¥1550')
  })

  it('renders invalid numeric input safely', () => {
    expect(formatMoney('not-a-number')).toBe('—')
  })

  it('keeps the supported currency list unique', () => {
    expect(new Set(SUPPORTED_CURRENCIES).size).toBe(SUPPORTED_CURRENCIES.length)
  })
})
