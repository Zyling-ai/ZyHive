import type { AxiosResponse } from 'axios'
import { beforeEach, describe, expect, it } from 'vitest'

import api from './index'

describe('API 客户端远程服务器切换', () => {
  beforeEach(() => {
    localStorage.clear()
    delete api.defaults.adapter
  })

  it('模块加载后切换服务器仍作用于下一次请求', async () => {
    const bases: Array<string | undefined> = []
    api.defaults.adapter = async config => {
      bases.push(config.baseURL)
      return {
        data: {},
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      } as AxiosResponse
    }

    await api.get('/health')
    localStorage.setItem('aipanel_url', 'https://remote.example.com/')
    await api.get('/health')

    expect(bases).toEqual(['/api', 'https://remote.example.com/api'])
  })
})
