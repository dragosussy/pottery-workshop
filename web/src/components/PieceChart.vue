<script setup lang="ts">
// A piece's dryness over time (points), with room humidity behind it.
import { computed, onMounted, ref } from 'vue'
import type { Observation } from '@/api/types'
import { api } from '@/api/endpoints'
import { serverNow } from '@/composables/useServerClock'
import { useSettingsStore } from '@/stores/settings'
import { toIso } from '@/utils/time'
import TimeChart from './TimeChart.vue'

const props = defineProps<{ observations: Observation[]; addedAt: string; endedAt: string | null }>()
const settings = useSettingsStore()
const room = ref<{ t: number[]; rh: (number | null)[] } | null>(null)

onMounted(async () => {
  try {
    const from = Date.parse(props.addedAt)
    const to = props.endedAt ? Date.parse(props.endedAt) : serverNow()
    const r = await api.readings({ from: toIso(from), to: toIso(Math.max(to, from + 3_600_000)) })
    room.value = { t: r.t, rh: r.series[0]?.humidity_avg ?? [] }
  } catch {
    room.value = null
  }
})

// Merge the observation times into the humidity time axis (uPlot needs one x array).
const merged = computed(() => {
  const obs = props.observations.map((o) => ({ t: Math.floor(Date.parse(o.observed_at) / 1000), d: o.dryness }))
  const roomT = room.value?.t ?? []
  const all = [...new Set([...roomT, ...obs.map((o) => o.t)])].sort((a, b) => a - b)
  const rhAt = new Map(roomT.map((t, i) => [t, room.value!.rh[i] ?? null]))
  const dAt = new Map(obs.map((o) => [o.t, o.d]))
  return {
    t: all,
    dryness: all.map((t) => dAt.get(t) ?? null),
    rh: all.map((t) => rhAt.get(t) ?? null),
  }
})
const series = computed(() => [
  { label: 'Room humidity', color: 'var(--surface-3)', values: merged.value.rh, scale: 'rh', unit: '%', width: 2 },
  { label: 'Dryness', color: 'var(--accent)', values: merged.value.dryness, scale: 'dryness', points: true, width: 2, spanGaps: true, unit: '' },
])
</script>

<template>
  <TimeChart
    :t="merged.t"
    :series="series"
    :ranges="{ dryness: [0, 5] }"
    :height="160"
    :tz="settings.tz"
    label="Dryness readings over time, with room humidity behind"
  />
</template>
