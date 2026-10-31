// GET /reference, fetched once. Built-in defaults (identical to contract §3.8)
// keep every label working before the fetch completes or while offline.
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Reference } from '@/api/types'
import { api } from '@/api/endpoints'
import { DEFAULT_REFERENCE } from '@/api/defaults'
import { loadJSON, saveJSON } from '@/utils/storage'

export { DEFAULT_REFERENCE }

export const useReferenceStore = defineStore('reference', () => {
  const data = ref<Reference>(loadJSON<Reference>('reference') ?? DEFAULT_REFERENCE)
  const loaded = ref(false)
  let inflight: Promise<void> | null = null

  function load(): Promise<void> {
    if (loaded.value) return Promise.resolve()
    inflight ??= api
      .reference()
      .then((r) => {
        // Fill anything the server leaves out with the defaults.
        data.value = { ...DEFAULT_REFERENCE, ...r }
        loaded.value = true
        saveJSON('reference', data.value)
      })
      .catch(() => {
        // Keep the defaults; try again next time.
      })
      .finally(() => {
        inflight = null
      })
    return inflight
  }

  const label = (list: keyof Pick<Reference, 'forms' | 'sizes' | 'thicknesses' | 'piece_states' | 'prediction_statuses' | 'uncertainty_reasons' | 'check_reasons'>, key: string) =>
    (data.value[list] as { key: string; label: string }[]).find((o) => o.key === key)?.label ?? key

  return { data, loaded, load, label }
})
