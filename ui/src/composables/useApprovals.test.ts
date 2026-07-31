import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useApprovals } from './useApprovals'

describe('useApprovals stream authentication', () => {
  beforeEach(() => {
    localStorage.setItem('aipanel_token', 'admin-secret')
  })

  it('exchanges the admin token for a short-lived EventSource ticket', async () => {
    const openedURLs: string[] = []
    let stream: FakeEventSource | null = null
    class FakeEventSource {
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null

      constructor(url: string) {
        openedURLs.push(url)
        stream = this
      }

      close() {}
    }
    const currentStream = (): FakeEventSource | null => stream

    vi.stubGlobal('EventSource', FakeEventSource)
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/stream-ticket')) {
        expect(init?.headers).toEqual({ Authorization: 'Bearer admin-secret' })
        return new Response(JSON.stringify({ ticket: 'short-ticket', expiresIn: 60 }), { status: 200 })
      }
      return new Response(JSON.stringify({ pending: [] }), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    const approvals = useApprovals()

    await vi.waitFor(() => {
      expect(openedURLs).toEqual(['/api/approvals/stream?ticket=short-ticket'])
    })
    expect(openedURLs[0]).not.toContain('admin-secret')

    currentStream()?.onopen?.()
    await vi.waitFor(() => {
      const pendingCalls = fetchMock.mock.calls.filter(([input]) =>
        String(input).endsWith('/api/approvals/pending'))
      expect(pendingCalls).toHaveLength(2)
    })
    currentStream()?.onmessage?.({
      data: JSON.stringify({
        type: 'hello',
        pending: [{ id: 'apv-1', agentId: 'agent-1', toolName: 'exec' }],
      }),
    } as MessageEvent)
    expect(approvals.pending.value.map(item => item.id)).toEqual(['apv-1'])
  })
})
