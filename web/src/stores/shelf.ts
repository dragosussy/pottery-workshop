// GET /shelf: pieces on the shelf with predictions, the visit recommendation
// and "check these". Cached in localStorage for the offline state.
import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import type { CheckReason, Piece, Shelf } from '@/api/types'
import { api } from '@/api/endpoints'
import { isApiError } from '@/api/client'
import { loadJSON, saveJSON } from '@/utils/storage'
import { useLiveStore } from './live'

export type ShelfGroupKey = 'ready' | 'check' | 'soon' | 'later' | 'past'

export const SHELF_GROUPS: { key: ShelfGroupKey; title: string }[] = [
  { key: 'ready', title: 'Ready now' },
  { key: 'check', title: 'Check these' },
  { key: 'soon', title: 'Soon' },
  { key: 'later', title: 'Later' },
  { key: 'past', title: 'Past trim' },
]

const SOON_MS = 12 * 3_600_000

/** Which shelf group a piece belongs to (frontend plan §4.4). */
export function shelfGroupOf(p: Piece, checkIds: Set<number>, now: number): ShelfGroupKey {
  const st = p.prediction?.status
  if (st === 'ready_now') return 'ready'
  if (st === 'past_trim') return 'past'
  if (checkIds.has(p.id) || st === 'probably_ready_check' || st === 'may_be_past_check') return 'check'
  const eta = p.prediction?.eta ? Date.parse(p.prediction.eta) : NaN
  if (!Number.isNaN(eta) && eta - now < SOON_MS) return 'soon'
  return 'later'
}

export const useShelfStore = defineStore('shelf', () => {
  const shelf = shallowRef<Shelf | null>(loadJSON<Shelf>('shelf'))
  const fromCache = ref(shelf.value !== null)
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref<unknown>(null)

  const pieces = computed(() => shelf.value?.items ?? [])
  const byId = computed(() => new Map(pieces.value.map((p) => [p.id, p])))
  const checkReasons = computed(() => new Map<number, CheckReason[]>((shelf.value?.check_these ?? []).map((c) => [c.piece_id, c.reasons])))
  const checkIds = computed(() => new Set(checkReasons.value.keys()))

  let inflight: Promise<void> | null = null
  function refresh(): Promise<void> {
    loading.value = true
    inflight ??= api
      .shelf()
      .then((s) => {
        shelf.value = s
        fromCache.value = false
        loaded.value = true
        error.value = null
        saveJSON('shelf', s)
      })
      .catch((e) => {
        error.value = e
        if (isApiError(e) && e.kind === 'network') useLiveStore().reachable = false
      })
      .finally(() => {
        inflight = null
        loading.value = false
      })
    return inflight
  }

  /** Load once; later calls are cheap. */
  function ensure(): Promise<void> {
    return loaded.value ? Promise.resolve() : refresh()
  }

  /** Optimistically merge updated pieces into the cached shelf. */
  function mergePieces(updated: Piece[]) {
    const s = shelf.value
    if (!s) return
    const map = new Map(updated.map((p) => [p.id, p]))
    const items = s.items
      .map((p) => map.get(p.id) ?? p)
      .filter((p) => p.state === 'in' || p.state === 'flipped')
    for (const p of updated) {
      if ((p.state === 'in' || p.state === 'flipped') && !s.items.some((x) => x.id === p.id)) items.push(p)
    }
    items.sort((a, b) => a.number - b.number)
    shelf.value = { ...s, items }
  }

  /** "check these" order first, then the rest by number (check-in order, plan §4.3). */
  function checkOrder(limitTo?: number[]): Piece[] {
    const all = pieces.value
    const pool = limitTo?.length ? all.filter((p) => limitTo.includes(p.id)) : all
    const first = (shelf.value?.check_these ?? []).map((c) => c.piece_id)
    const rank = (p: Piece) => {
      const i = first.indexOf(p.id)
      return i >= 0 ? i : first.length
    }
    return [...pool].sort((a, b) => rank(a) - rank(b) || a.number - b.number)
  }

  // Refetch on `changed` from the stream once the shelf has been loaded.
  const live = useLiveStore()
  live.onChange((scopes) => {
    if (!loaded.value && !fromCache.value) return
    if (scopes.some((s) => s === 'pieces' || s === 'observations' || s === 'predictions' || s === 'settings')) void refresh()
  })

  return { shelf, fromCache, loaded, loading, error, pieces, byId, checkReasons, checkIds, refresh, ensure, mergePieces, checkOrder }
})
