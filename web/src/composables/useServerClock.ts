// Keeps the offset between the phone's clock and the controller's clock
// (contract §1 "Clock"). "Now" choices send no time at all; relative choices
// ("1h ago") are computed from server time so a wrong phone clock can't
// produce a time in the future.

import { onBeforeUnmount, onMounted, readonly, ref } from 'vue'
import { parseTs } from '@/utils/time'

const offsetMs = ref(0)

export function updateServerClock(serverTime: string | null | undefined): void {
  const ms = parseTs(serverTime)
  if (ms === null) return
  offsetMs.value = ms - Date.now()
  tickNow.value = serverNow()
}

/** Current server time in ms. */
export function serverNow(): number {
  return Date.now() + offsetMs.value
}

export function useServerClock() {
  return { offsetMs: readonly(offsetMs), serverNow, updateServerClock }
}

// One shared ticking "now" so ages and "x ago" labels update client-side.
const tickNow = ref(serverNow())
/** The shared ticking now (read-only). Ticks while any component calls useNow(). */
export const sharedNow = readonly(tickNow)
let subscribers = 0
let timer: ReturnType<typeof setInterval> | null = null

/** A reactive server-time "now" that ticks every 15 s while any component uses it. */
export function useNow() {
  onMounted(() => {
    subscribers++
    tickNow.value = serverNow()
    if (!timer) timer = setInterval(() => (tickNow.value = serverNow()), 15_000)
  })
  onBeforeUnmount(() => {
    subscribers--
    if (subscribers <= 0 && timer) {
      clearInterval(timer)
      timer = null
      subscribers = 0
    }
  })
  return tickNow
}

/** Force the shared clock to refresh (e.g. after new server_time arrives). */
export function refreshNow(): void {
  tickNow.value = serverNow()
}
