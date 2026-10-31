// POST /observations is never retried automatically (contract §1.2). With an
// HTTP error nothing was saved ("Try again"); with no response the write may
// or may not have happened: refetch the shelf and ask the user to check.
import type { NewObservations, ObservationsCreated } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/client'
import { useShelfStore } from '@/stores/shelf'
import { useLiveStore } from '@/stores/live'

export type SaveResult =
  | { ok: true; data: ObservationsCreated }
  | { ok: false; unconfirmed: boolean; message: string }

export const UNCONFIRMED_SAVE = "Couldn't confirm the save. Check the piece's history before saving again."

export function useObservationSave() {
  const shelf = useShelfStore()
  const live = useLiveStore()
  async function save(body: NewObservations): Promise<SaveResult> {
    try {
      const data = await api.addObservations(body)
      void shelf.refresh()
      void live.fetchStatus()
      return { ok: true, data }
    } catch (e) {
      if (isApiError(e) && e.unconfirmed) {
        void shelf.refresh()
        return { ok: false, unconfirmed: true, message: UNCONFIRMED_SAVE }
      }
      if (isApiError(e) && e.code === 'piece_ended') {
        void shelf.refresh()
        return { ok: false, unconfirmed: false, message: 'That piece has already left the shelf.' }
      }
      return { ok: false, unconfirmed: false, message: errorMessage(e, "Couldn't save. Try again.") }
    }
  }
  return { save }
}
