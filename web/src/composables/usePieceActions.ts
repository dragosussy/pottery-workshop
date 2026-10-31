// Lifecycle changes (flip, trim, discard, back on the shelf) as PATCHes on
// `state` (contract §3.5), each with an Undo toast that sends the same PATCH
// with the previous state. PATCH is idempotent, so the client may retry it.
import type { Piece, PiecePatch, PieceState } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/client'
import { useShelfStore } from '@/stores/shelf'
import { useLiveStore } from '@/stores/live'
import { showToast } from './useToast'
import { numberList } from '@/utils/format'

const VERB: Record<PieceState, string> = { in: 'back on the shelf', flipped: 'flipped', trimmed: 'trimmed', discarded: 'discarded' }

/** The state an ended piece returns to on Undo / "Back on the shelf". */
export function stateBeforeEnd(p: Pick<Piece, 'flipped_at'>): PieceState {
  return p.flipped_at ? 'flipped' : 'in'
}

export function usePieceActions() {
  const shelf = useShelfStore()
  const live = useLiveStore()

  async function send(ids: number[], body: PiecePatch): Promise<Piece[]> {
    if (ids.length === 1) return [await api.patchPiece(ids[0]!, body)]
    return (await api.patchPieces({ ids, ...body })).items
  }

  function afterWrite(updated: Piece[]) {
    shelf.mergePieces(updated)
    void shelf.refresh()
    void live.fetchStatus()
  }

  async function undo(previous: Map<number, PieceState>) {
    // One PATCH per previous state (a group may mix "in" and "flipped").
    const groups = new Map<PieceState, number[]>()
    previous.forEach((st, id) => groups.set(st, [...(groups.get(st) ?? []), id]))
    try {
      const all: Piece[] = []
      for (const [state, ids] of groups) all.push(...(await send(ids, { state })))
      afterWrite(all)
      showToast({ message: 'Undone', tone: 'ok' })
    } catch (e) {
      showToast({ message: `Couldn't undo: ${errorMessage(e)}`, tone: 'error' })
      void shelf.refresh()
    }
  }

  /**
   * Apply a lifecycle change to `pieces` and offer Undo. Throws on failure so
   * the calling sheet can stay open and show the error.
   */
  async function transition(pieces: Pick<Piece, 'id' | 'number' | 'state'>[], body: PiecePatch & { state: PieceState }): Promise<Piece[]> {
    const ids = pieces.map((p) => p.id)
    const previous = new Map(pieces.map((p) => [p.id, p.state]))
    try {
      const updated = await send(ids, body)
      afterWrite(updated)
      const nums = updated.map((p) => p.number)
      showToast({
        message: `${numberList(nums)} ${VERB[body.state]}`,
        tone: 'ok',
        action: { label: 'Undo', run: () => undo(previous) },
      })
      return updated
    } catch (e) {
      if (isApiError(e) && e.code === 'invalid_transition') void shelf.refresh()
      throw e
    }
  }

  /** A correction (PATCH without `state`). */
  async function correct(id: number, body: PiecePatch): Promise<Piece> {
    const p = await api.patchPiece(id, body)
    afterWrite([p])
    return p
  }

  return { transition, correct, undo }
}

export function transitionErrorMessage(e: unknown): string {
  if (isApiError(e) && e.code === 'invalid_transition') return 'That piece was changed elsewhere. The shelf has been refreshed.'
  if (isApiError(e) && e.kind === 'network') return "Couldn't reach the controller. Try again."
  return errorMessage(e, "Couldn't save")
}
