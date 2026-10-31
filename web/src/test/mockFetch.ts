// Route `fetch('/api/v1/…')` to the in-memory mock backend, so component and
// store tests run against the same contract-conformant fixtures as `dev:mock`.
import { vi } from 'vitest'
import { createMockBackend, type MockBackend } from '@/mocks/backend'
import { FIXED_NOW, type ScenarioName } from '@/mocks/fixtures'

export interface FetchLog {
  method: string
  path: string
  body: unknown
}

export function installMockFetch(scenario: ScenarioName = 'calibrated', now: () => number = () => FIXED_NOW) {
  const backend: MockBackend = createMockBackend(scenario, now)
  const calls: FetchLog[] = []
  let failNext: 'network' | 'network-after' | null = null
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(typeof input === 'string' ? input : input.toString(), 'http://test.local')
    const method = (init.method ?? 'GET').toUpperCase()
    const path = url.pathname.replace(/^\/api\/v1/, '')
    let json: unknown
    let bytes: Uint8Array | undefined
    if (typeof init.body === 'string') json = JSON.parse(init.body)
    else if (init.body instanceof Blob) bytes = new Uint8Array(await init.body.arrayBuffer())
    calls.push({ method, path, body: json })
    const headers = (init.headers ?? {}) as Record<string, string>
    const mode = method !== 'GET' ? failNext : null
    if (mode) failNext = null
    if (mode === 'network') throw new TypeError('Failed to fetch')
    const out = backend.handle({ method, path, query: url.searchParams, json, bytes, contentType: headers['Content-Type'] })
    if (mode === 'network-after') throw new TypeError('Failed to fetch')
    return new Response(out.body as BodyInit | null, { status: out.status, headers: out.headers })
  })
  vi.stubGlobal('fetch', fetchMock)
  return {
    backend,
    calls,
    fetchMock,
    /** Make the next write fail with no response: before or after it was applied. */
    failNextWrite(mode: 'network' | 'network-after') {
      failNext = mode
    },
    writes: () => calls.filter((c) => c.method !== 'GET'),
  }
}
