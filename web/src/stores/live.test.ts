import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { setEventSourceCtor, STREAM, useLiveStore } from './live'
import { installMockFetch } from '@/test/mockFetch'
import { snapshot } from '@/mocks/fixtures'

class FakeES {
  static instances: FakeES[] = []
  readyState = 0
  onerror: (() => void) | null = null
  listeners = new Map<string, ((e: MessageEvent) => void)[]>()
  closed = false
  constructor(public url: string) {
    FakeES.instances.push(this)
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  emit(type: string, data: unknown) {
    this.listeners.get(type)?.forEach((fn) => fn(new MessageEvent(type, { data: JSON.stringify(data) })))
  }
  fail() {
    this.onerror?.()
  }
  close() {
    this.closed = true
    this.readyState = 2
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  FakeES.instances = []
  setEventSourceCtor(FakeES as unknown as typeof EventSource)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('live store', () => {
  it('fetches /status, caches it and opens the stream', async () => {
    installMockFetch('calibrated')
    const live = useLiveStore()
    live.start()
    await flush()
    expect(live.status?.shelf.on_shelf).toBe(8)
    expect(live.reachable).toBe(true)
    expect(FakeES.instances).toHaveLength(1)
    expect(JSON.parse(localStorage.getItem('hc.status')!).status.shelf.on_shelf).toBe(8)
    live.stop()
  })

  it('applies readings events to the climate', async () => {
    installMockFetch('calibrated')
    const live = useLiveStore()
    live.start()
    await flush()
    const es = FakeES.instances[0]!
    es.emit('readings', { server_time: '2026-10-30T14:06:00Z', room: { humidity_pct: 77.7, temperature_c: 19, vpd_kpa: 0.5, state: 'ok', read_at: '2026-10-30T14:05:59Z' }, sensors: [] })
    expect(live.status?.room.humidity_pct).toBe(77.7)
    live.stop()
  })

  it('shows the cached status, marked offline, when the controller is unreachable', async () => {
    const snap = snapshot('calibrated')
    localStorage.setItem('hc.status', JSON.stringify({ status: snap.status, saved_at: 0 }))
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    setActivePinia(createPinia())
    const live = useLiveStore()
    expect(live.fromCache).toBe(true)
    expect(live.status?.shelf.on_shelf).toBe(8)
    await live.fetchStatus()
    expect(live.offline).toBe(true)
    expect(live.asOf).toBe(Date.parse(snap.status.server_time))
  })

  it('falls back to polling after 3 stream failures within 60 s, then retries the stream', async () => {
    const m = installMockFetch('calibrated')
    vi.useFakeTimers()
    const live = useLiveStore()
    live.start()
    const es = FakeES.instances[0]!
    es.fail()
    es.fail()
    expect(live.streamState).not.toBe('polling')
    es.fail()
    expect(live.streamState).toBe('polling')
    expect(es.closed).toBe(true)
    const before = m.calls.filter((c) => c.path === '/status').length
    await vi.advanceTimersByTimeAsync(STREAM.pollMs * 3)
    expect(m.calls.filter((c) => c.path === '/status').length).toBeGreaterThanOrEqual(before + 3)
    await vi.advanceTimersByTimeAsync(STREAM.retryStreamMs)
    expect(FakeES.instances.length).toBe(2)
    live.stop()
  })

  it('debounces changed events into one refetch and notifies listeners', async () => {
    const m = installMockFetch('calibrated')
    vi.useFakeTimers()
    const live = useLiveStore()
    const seen: string[][] = []
    live.onChange((s) => seen.push(s))
    live.start()
    await vi.advanceTimersByTimeAsync(0)
    const es = FakeES.instances[0]!
    const before = m.calls.filter((c) => c.path === '/status').length
    es.emit('changed', { scopes: ['pieces'], piece_ids: [1] })
    es.emit('changed', { scopes: ['observations'], piece_ids: [2] })
    await vi.advanceTimersByTimeAsync(STREAM.changedDebounceMs + 10)
    expect(m.calls.filter((c) => c.path === '/status').length).toBe(before + 1)
    expect(seen).toEqual([['pieces', 'observations']])
    live.stop()
  })

  it('offers a reload when the server version changes', async () => {
    installMockFetch('calibrated')
    const live = useLiveStore()
    live.start()
    const es = FakeES.instances[0]!
    const hello = { server_time: '2026-10-30T14:05:03Z', version: '0.4.0', api_version: 1, poll_interval_seconds: 5, stale_after_seconds: 30, clock_synced: false }
    es.emit('hello', hello)
    expect(live.newVersionAvailable).toBe(false)
    expect(live.clockSynced).toBe(false)
    es.emit('hello', { ...hello, version: '0.5.0' })
    expect(live.newVersionAvailable).toBe(true)
    live.stop()
  })

  it('closes the stream while the page is hidden and reconnects when visible', async () => {
    installMockFetch('calibrated')
    const live = useLiveStore()
    live.connect()
    expect(FakeES.instances).toHaveLength(1)
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(FakeES.instances[0]!.closed).toBe(true)
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(FakeES.instances).toHaveLength(2)
    live.stop()
  })
})
