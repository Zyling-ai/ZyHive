import { beforeEach, describe, expect, it } from 'vitest'

import { apiURL, getAPIBaseURL, getServerBaseURL } from './base'

describe('远程服务器 API 地址', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('未配置远程服务器时使用当前站点', () => {
    expect(getServerBaseURL()).toBe('')
    expect(getAPIBaseURL()).toBe('/api')
    expect(apiURL('/version')).toBe('/api/version')
  })

  it('统一清理远程地址尾部斜杠', () => {
    localStorage.setItem('aipanel_url', ' https://remote.example.com/base/// ')

    expect(getServerBaseURL()).toBe('https://remote.example.com/base')
    expect(getAPIBaseURL()).toBe('https://remote.example.com/base/api')
    expect(apiURL('agents')).toBe('https://remote.example.com/base/api/agents')
  })

  it('每次调用都读取最新远程地址', () => {
    expect(apiURL('/health')).toBe('/api/health')
    localStorage.setItem('aipanel_url', 'https://remote.example.com')
    expect(apiURL('/health')).toBe('https://remote.example.com/api/health')
  })
})
