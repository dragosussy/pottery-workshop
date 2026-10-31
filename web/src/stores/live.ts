// Live state (frontend plan §6, contract §4.3): GET /status and the
// EventSource stream.
//
// - `hello` → refetch; `readings` → climate card; `changed` → debounced refetch.
// - After 3 stream failures within 60 s: poll /status every 10 s and retry
//   the stream every 60 s.
// - The stream is closed while the page is hidden and reopened (with a
//   refetch) when it becomes visible. /status is refetched every 5 min.
// - The last /status is cached in localStorage for the "controller offline"
//   state.

import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import type { ChangeScope, LiveStatus, StreamChanged, StreamHello, StreamReadings } from '@/api/types'
import { api, streamUrl } from '@/api/endpoints'
import { isApiError } from '@/api/client'
import { sharedNow, updateServerClock } from '@/composables/useServerClock'
import { loadJSON, saveJSON } from '@/utils/storage'
import { parseTs } from '@/utils/time'

export const STREAM = {
  failureWindowMs: 60_000,
  failuresBeforeFallback: 3,
  pollMs: 10_000,
  retryStreamMs: 60_000,
  reconnectMs: 5_000,
  refreshMs: 5 * 60_000,
  changedDebounceMs: 400,
} as const

interface CachedStatus {
  status: LiveStatus
  /** Phone time when it was stored. */
  saved_at: number
}

type Listener = (scopes: ChangeScope[], pieceIds: number[]) => void

// Injectable for tests.
let EventSourceCtor: (new (url: string) => EventSource) | null = typeof EventSource !== 'undefined' ? EventSource : null
export function setEventSourceCtor(ctor: (new (url: string) => EventSource) | null) {
  EventSourceCtor = ctor
}

export const useLiveStore = defineStore('live', () => {
  const cached = loadJSON<CachedStatus>('status')
  const status = shallowRef<LiveStatus | null>(cached?.status ?? null)
  /** True while what we show comes from the cache, not a live response. */
  const fromCache = ref(cached !== null)
  /** False once a request to the controller fails with no response. */
  const reachable = ref(true)
  const loaded = ref(false)
  const streamState = ref<'idle' | 'connecting' | 'open' | 'polling'>('idle')
  const clockSynced = ref<boolean | null>(null)
  const serverVersion = ref<string | null>(null)
  const newVersionAvailable = ref(false)

  let es: EventSource | null = null
  let failures: number[] = []
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let refreshTimer: ReturnType<typeof setInterval> | null = null
  let changedTimer: ReturnType<typeof setTimeout> | null = null
  let pendingScopes = new Set<ChangeScope>()
  let pendingIds = new Set<number>()
  let running = false
  const listeners = new Set<Listener>()

  /** Server time of the data shown (for "as of Thu 18:05"). */
  const asOf = computed(() => parseTs(status.value?.server_time))
  const offline = computed(() => !reachable.value)

  /** Room state, also flipping to stale client-side when readings stop arriving. */
  const roomState = computed(() => {
    const s = status.value
    if (!s) return 'no_data'
    if (s.room.state !== 'ok') return s.room.state
    const readAt = parseTs(s.room.read_at)
    if (readAt !== null && sharedNow.value - readAt > s.stale_after_seconds * 1000) return 'stale'
    return 'ok'
  })

  function setStatus(s: LiveStatus) {
    status.value = s
    fromCache.value = false
    reachable.value = true
    loaded.value = true
    updateServerClock(s.server_time)
    saveJSON('status', { status: s, saved_at: Date.now() } satisfies CachedStatus)
  }

  let statusInflight: Promise<void> | null = null
  function fetchStatus(): Promise<void> {
    statusInflight ??= api
      .status()
      .then(setStatus)
      .catch((e) => {
        if (isApiError(e) && e.kind === 'network') reachable.value = false
      })
      .finally(() => {
        statusInflight = null
      })
    return statusInflight
  }

  function applyReadings(r: StreamReadings) {
    updateServerClock(r.server_time)
    const s = status.value
    if (!s) return
    status.value = { ...s, server_time: r.server_time, room: r.room, sensors: r.sensors }
    reachable.value = true
  }

  function onHello(h: StreamHello) {
    updateServerClock(h.server_time)
    clockSynced.value = h.clock_synced
    if (serverVersion.value && serverVersion.value !== h.version) newVersionAvailable.value = true
    serverVersion.value = h.version
    failures = []
    streamState.value = 'open'
    reachable.value = true
    // No replay on (re)connect: refetch everything that might have changed.
    void fetchStatus()
    emitChanged(['pieces', 'observations', 'predictions', 'settings'], [])
  }

  function onChanged(c: StreamChanged) {
    c.scopes.forEach((s) => pendingScopes.add(s))
    c.piece_ids.forEach((id) => pendingIds.add(id))
    if (changedTimer) clearTimeout(changedTimer)
    changedTimer = setTimeout(() => {
      changedTimer = null
      const scopes = [...pendingScopes]
      const ids = [...pendingIds]
      pendingScopes = new Set()
      pendingIds = new Set()
      void fetchStatus()
      emitChanged(scopes, ids)
    }, STREAM.changedDebounceMs)
  }

  function emitChanged(scopes: ChangeScope[], ids: number[]) {
    listeners.forEach((l) => l(scopes, ids))
  }

  /** Subscribe to (debounced) `changed` signals; returns an unsubscribe function. */
  function onChange(l: Listener): () => void {
    listeners.add(l)
    return () => listeners.delete(l)
  }

  function parse<T>(e: Event): T | null {
    try {
      return JSON.parse((e as MessageEvent<string>).data) as T
    } catch {
      return null
    }
  }

  function closeStream() {
    if (es) {
      es.close()
      es = null
    }
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  function openStream() {
    closeStream()
    if (!EventSourceCtor || !running) return
    streamState.value = 'connecting'
    const source = new EventSourceCtor(streamUrl)
    es = source
    source.addEventListener('hello', (e) => {
      const h = parse<StreamHello>(e)
      if (h) onHello(h)
    })
    source.addEventListener('readings', (e) => {
      const r = parse<StreamReadings>(e)
      if (r) applyReadings(r)
    })
    source.addEventListener('changed', (e) => {
      const c = parse<StreamChanged>(e)
      if (c) onChanged(c)
    })
    source.onerror = () => {
      if (es !== source) return
      const now = Date.now()
      failures = [...failures.filter((t) => now - t < STREAM.failureWindowMs), now]
      // Find out whether the controller itself is reachable.
      void fetchStatus()
      if (failures.length >= STREAM.failuresBeforeFallback) {
        startFallback()
        return
      }
      if (source.readyState === 2 /* CLOSED */) {
        // The browser gave up (e.g. a non-200 response); retry ourselves.
        reconnectTimer = setTimeout(openStream, STREAM.reconnectMs)
      }
    }
  }

  function startFallback() {
    closeStream()
    streamState.value = 'polling'
    if (!pollTimer) pollTimer = setInterval(() => void fetchStatus(), STREAM.pollMs)
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = setTimeout(() => {
      retryTimer = null
      stopPolling()
      failures = []
      openStream()
    }, STREAM.retryStreamMs)
  }

  function stopPolling() {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
  }

  function clearTimers() {
    stopPolling()
    if (retryTimer) clearTimeout(retryTimer)
    retryTimer = null
    if (refreshTimer) clearInterval(refreshTimer)
    refreshTimer = null
    if (changedTimer) clearTimeout(changedTimer)
    changedTimer = null
  }

  /** Start live updates (called while the page is visible). */
  function start() {
    if (running) return
    running = true
    failures = []
    void fetchStatus()
    openStream()
    refreshTimer = setInterval(() => void fetchStatus(), STREAM.refreshMs)
  }

  /** Stop live updates (page hidden). */
  function stop() {
    running = false
    closeStream()
    clearTimers()
    streamState.value = 'idle'
  }

  let visibilityBound = false
  /** Start now and follow page visibility from here on. */
  function connect() {
    if (typeof document !== 'undefined' && !visibilityBound) {
      visibilityBound = true
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') stop()
        else start()
      })
    }
    if (typeof document === 'undefined' || document.visibilityState !== 'hidden') start()
  }

  return {
    status,
    fromCache,
    reachable,
    offline,
    loaded,
    asOf,
    roomState,
    streamState,
    clockSynced,
    serverVersion,
    newVersionAvailable,
    fetchStatus,
    applyReadings,
    onChange,
    start,
    stop,
    connect,
    // exposed for tests
    _onHello: onHello,
    _onChanged: onChanged,
  }
})
