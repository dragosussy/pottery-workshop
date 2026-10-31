// Sheets are addressed by a route query param (?sheet=add, ?sheet=trim&ids=…)
// so the back button closes them (frontend plan §3).
import { computed } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'

export type SheetName = 'add' | 'observe' | 'flip' | 'trim' | 'discard' | 'edit' | 'target'

let pushedBySheet = false

/** Called by the router after navigation: once no sheet is open, history is ours again. */
export function resetSheetPush(): void {
  pushedBySheet = false
}

export function parseIds(v: unknown): number[] {
  const s = Array.isArray(v) ? v.join(',') : typeof v === 'string' ? v : ''
  return s
    .split(',')
    .map((x) => Number(x))
    .filter((n) => Number.isInteger(n) && n > 0)
}

export function useSheet() {
  const route = useRoute()
  const router = useRouter()

  const current = computed(() => (typeof route.query.sheet === 'string' ? (route.query.sheet as SheetName) : null))
  const ids = computed(() => parseIds(route.query.ids))

  function withoutSheet(): LocationQueryRaw {
    const q: LocationQueryRaw = { ...route.query }
    delete q.sheet
    delete q.ids
    delete q.obs
    delete q.focus
    delete q.d
    return q
  }

  function open(name: SheetName, params: { ids?: number[]; obs?: number; focus?: string; dryness?: number } = {}) {
    const query: LocationQueryRaw = { ...withoutSheet(), sheet: name }
    if (params.ids?.length) query.ids = params.ids.join(',')
    if (params.obs) query.obs = String(params.obs)
    if (params.focus) query.focus = params.focus
    if (params.dryness !== undefined) query.d = String(params.dryness)
    if (current.value) {
      // Switching from one sheet to another: don't stack history entries.
      return router.replace({ query })
    }
    pushedBySheet = true
    return router.push({ query })
  }

  function close() {
    if (!current.value) return
    if (pushedBySheet) {
      pushedBySheet = false
      router.back()
    } else {
      router.replace({ query: withoutSheet() })
    }
  }

  return { current, ids, open, close, query: computed(() => route.query) }
}
