// "Defaulting to the last one used" for the Add sheet (plan §4.2), kept in
// localStorage (wrapped in try/catch by utils/storage).
import { reactive, watch } from 'vue'
import type { FormKey, SizeKey, ThicknessKey } from '@/api/types'
import { loadJSON, saveJSON } from '@/utils/storage'

export interface AddDefaults {
  form: FormKey
  dryness: number
  size: SizeKey
  thickness: ThicknessKey
}

export const FIRST_DEFAULTS: AddDefaults = { form: 'mug', dryness: 1.0, size: 'medium', thickness: 'medium' }

export function useStickyDefaults() {
  const stored = loadJSON<Partial<AddDefaults>>('add-defaults') ?? {}
  const state = reactive<AddDefaults>({ ...FIRST_DEFAULTS, ...stored })
  function remember() {
    saveJSON('add-defaults', { ...state })
  }
  watch(state, remember, { deep: true })
  return state
}
