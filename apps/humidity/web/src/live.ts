// Shared state, refreshed by polling (plan §8): /api/now every 5 s and the
// shelf list every 30 s while the page is visible, and the shelf after
// every write. One 1 s timer also drives the "12 s ago" clocks.
import { reactive } from 'vue'
import { apiUrl } from '@ui/http'
import { bar, say, sayError } from '@ui/messages'
import { api, type Now, type Piece, type PiecePatch } from './api'

export { say, sayError }

export const live = reactive({
  now: null as Now | null,
  /** Phone time (ms) of the last good /api/now. */
  nowAt: 0,
  /** True when the last /api/now poll failed. */
  unreachable: false,
  shelf: [] as Piece[],
  shelfLoaded: false,
  /** Phone time (ms), ticking every second. */
  clock: Date.now(),
  /** Changes after a photo upload or delete, so photo URLs refetch. */
  photoVersion: Number(read('humidity.photoVersion')) || 1,
  /** Piece ids in shelf order when the current walk started ("Next piece →"). */
  walk: null as number[] | null,
  /** Bumped after an Undo, so an open piece page reloads. */
  undone: 0,
})

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/** Call after uploading or deleting a photo: photo URLs change, so they reload. */
export function bumpPhotos() {
  live.photoVersion = Date.now()
  try {
    localStorage.setItem('humidity.photoVersion', String(live.photoVersion))
  } catch {
    // private mode: the version just lasts this session
  }
}

/** Runs the message's undo, then refreshes the shelf. */
export async function undoLast() {
  const undo = bar.message?.undo
  bar.message = null
  if (!undo) return
  try {
    await undo()
    live.undone++
    say('Undone')
  } catch (e) {
    sayError(e)
  }
  void refreshShelf()
}

export function setNow(n: Now) {
  live.now = n
  live.nowAt = Date.now()
  live.unreachable = false
}

let lastNowPoll = 0
let lastShelfPoll = 0

export async function refreshNow() {
  lastNowPoll = Date.now()
  try {
    setNow(await api.now())
  } catch {
    live.unreachable = true
  }
}

export async function refreshShelf() {
  lastShelfPoll = Date.now()
  try {
    live.shelf = await api.shelf()
    live.shelfLoaded = true
  } catch (e) {
    if (!live.shelfLoaded) sayError(e)
  }
}

export function photoUrl(id: number) {
  return apiUrl(`/pieces/${id}/photo?v=${live.photoVersion}`)
}

/** Starts polling; returns a function that stops it. */
export function startPolling(): () => void {
  const tick = () => {
    live.clock = Date.now()
    if (document.visibilityState !== 'visible') return
    if (live.clock - lastNowPoll >= 5000) void refreshNow()
    if (live.clock - lastShelfPoll >= 30000) void refreshShelf()
  }
  const timer = setInterval(tick, 1000)
  document.addEventListener('visibilitychange', tick)
  tick()
  return () => {
    clearInterval(timer)
    document.removeEventListener('visibilitychange', tick)
  }
}

/**
 * Flips, trims or discards a piece and offers Undo, which sends it back to
 * the shelf (undo flip, or put back). Returns the updated piece.
 */
export async function markPiece(p: Piece, patch: PiecePatch & { state: 'flipped' | 'trimmed' | 'discarded' }): Promise<Piece> {
  const updated = await api.patchPiece(p.id, patch)
  say(`#${p.number} ${patch.state}`, false, () => api.patchPiece(p.id, { state: 'in' }))
  void refreshShelf()
  return updated
}
