// Resolve the pieces a sheet acts on: from the shelf, or fetched by id (a
// piece that has left the shelf, opened from History).
import { ref, watch, type Ref } from 'vue'
import type { Piece } from '@/api/types'
import { api } from '@/api/endpoints'
import { useShelfStore } from '@/stores/shelf'

export function useSheetPieces(ids: Ref<number[]>) {
  const shelf = useShelfStore()
  const pieces = ref<Piece[]>([])
  const loading = ref(true)
  const missing = ref(false)

  async function load() {
    loading.value = true
    if (!shelf.loaded) await shelf.ensure()
    const out: Piece[] = []
    for (const id of ids.value) {
      const p = shelf.byId.get(id)
      if (p) out.push(p)
      else {
        try {
          out.push(await api.piece(id))
        } catch {
          // skip unknown ids
        }
      }
    }
    pieces.value = out
    missing.value = out.length === 0
    loading.value = false
  }
  watch(ids, load, { immediate: true })
  return { pieces, loading, missing, reload: load }
}
