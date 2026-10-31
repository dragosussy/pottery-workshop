<script setup lang="ts">
// Climate (plan §4.8): range chips, humidity/temperature/VPD toggles, shaded
// "controller off" periods, calibration markers, min/avg/max.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { ClimateLogEntry, ReadingsSeries } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import Chips from '@/components/Chips.vue'
import TimeChart, { type ChartBand, type ChartMarker, type ChartSeries } from '@/components/TimeChart.vue'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { serverNow } from '@/composables/useServerClock'
import { toIso } from '@/utils/time'
import { formatDayTime } from '@/utils/format'
import { formatSpanWords } from '@/utils/duration'

const router = useRouter()
const live = useLiveStore()
const settings = useSettingsStore()

type Range = '6h' | '24h' | '7d' | '30d'
type Metric = 'humidity' | 'temperature' | 'vpd'
const RANGES: { key: Range; label: string }[] = [
  { key: '6h', label: '6h' },
  { key: '24h', label: '24h' },
  { key: '7d', label: '7d' },
  { key: '30d', label: '30d' },
]
const METRICS: { key: Metric; label: string }[] = [
  { key: 'humidity', label: 'Humidity' },
  { key: 'temperature', label: 'Temp' },
  { key: 'vpd', label: 'VPD' },
]
const range = ref<Range>('24h')
const metrics = ref<Metric[]>(['humidity'])
const data = ref<ReadingsSeries | null>(null)
const log = ref<ClimateLogEntry[]>([])
const loading = ref(false)
const error = ref('')
let ctrl: AbortController | null = null

async function load() {
  ctrl?.abort()
  ctrl = new AbortController()
  loading.value = true
  error.value = ''
  const hours = { '6h': 6, '24h': 24, '7d': 168, '30d': 720 }[range.value]
  const from = toIso(serverNow() - hours * 3_600_000)
  try {
    const [r, l] = await Promise.all([api.readings({ from: `-${range.value}` }, ctrl.signal), api.climateLog({ from }, ctrl.signal)])
    data.value = r
    log.value = l.items
  } catch (e) {
    if ((e as Error).name !== 'AbortError') error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(range, load, { immediate: true })
const off = live.onChange((scopes) => {
  if (scopes.includes('climate_log')) void load()
})
onBeforeUnmount(() => {
  off()
  ctrl?.abort()
})

const s0 = computed(() => data.value?.series[0] ?? null)
const series = computed<ChartSeries[]>(() => {
  const s = s0.value
  if (!s) return []
  const out: ChartSeries[] = []
  if (metrics.value.includes('humidity')) out.push({ label: 'Humidity', color: 'var(--accent)', values: s.humidity_avg, scale: '%', unit: '%' })
  if (metrics.value.includes('temperature')) out.push({ label: 'Temperature', color: 'var(--ok)', values: s.temperature_avg, scale: '°C', unit: '°', dash: [6, 4] })
  if (metrics.value.includes('vpd')) out.push({ label: 'VPD', color: 'var(--warn)', values: s.vpd_avg, scale: 'kPa', unit: '' })
  return out
})
const sec = (ts: string) => Math.floor(Date.parse(ts) / 1000)
const bands = computed<ChartBand[]>(() =>
  log.value.flatMap((e): ChartBand[] => {
    if (e.kind === 'offline') return [{ from: sec(e.value.from), to: sec(e.value.to), label: 'controller off' }]
    if (e.kind === 'sensor_gap') return [{ from: sec(e.value.from), to: sec(e.value.to), label: `${e.value.sensor}: no data` }]
    return []
  }),
)
const markers = computed<ChartMarker[]>(() =>
  log.value.flatMap((e): ChartMarker[] => {
    if (e.kind === 'sensor_calibrated') return [{ at: sec(e.at), label: `${e.value.sensor} calibrated` }]
    if (e.kind === 'sensor_replaced') return [{ at: sec(e.at), label: `${e.value.sensor} replaced` }]
    return []
  }),
)
const notes = computed(() =>
  log.value.map((e) => {
    const when = formatDayTime(Date.parse(e.at), serverNow(), settings.tz)
    switch (e.kind) {
      case 'offline':
        return `⏻ Controller off for ${formatSpanWords((Date.parse(e.value.to) - Date.parse(e.value.from)) / 1000)} (until ${when})`
      case 'sensor_gap':
        return `⚠ ${e.value.sensor}: no data for ${formatSpanWords((Date.parse(e.value.to) - Date.parse(e.value.from)) / 1000)}`
      case 'sensor_calibrated':
        return `┊ ${e.value.sensor} calibrated ${when} (offset ${e.value.offset_pct > 0 ? '+' : ''}${e.value.offset_pct}%)`
      case 'sensor_replaced':
        return `┊ ${e.value.sensor} replaced ${when}`
      case 'setpoint_set':
        return e.value.target_humidity_pct === null ? `Target cleared ${when}` : `Target set to ${e.value.target_humidity_pct}% ${when}`
      case 'pump_manual':
        return `Pump run by hand for ${e.value.seconds} s ${when}`
      case 'pump_tripped':
        return `⚠ Pump stopped ${when}: humidity didn't rise after ${e.value.pulses} runs`
      case 'pump_reset':
        return `Pump resumed ${when}`
    }
  }),
)

function stats(vals: (number | null)[] | undefined, lo?: (number | null)[], hi?: (number | null)[]) {
  const v = (vals ?? []).filter((x): x is number => x !== null)
  if (!v.length) return null
  const mins = (lo ?? vals ?? []).filter((x): x is number => x !== null)
  const maxs = (hi ?? vals ?? []).filter((x): x is number => x !== null)
  return { min: Math.min(...mins), avg: v.reduce((a, b) => a + b, 0) / v.length, max: Math.max(...maxs) }
}
const rh = computed(() => stats(s0.value?.humidity_avg, s0.value?.humidity_min, s0.value?.humidity_max))
const temp = computed(() => stats(s0.value?.temperature_avg))
const vpd = computed(() => stats(s0.value?.vpd_avg))
const hasData = computed(() => (s0.value?.humidity_avg ?? []).some((v) => v !== null))
</script>

<template>
  <main class="page">
    <header class="page-header">
      <button type="button" class="btn btn-ghost" aria-label="Back" @click="router.back()"><span aria-hidden="true">←</span></button>
      <h1 class="page-title">Climate</h1>
      <span style="width: 48px"></span>
    </header>

    <Chips v-model="range" :options="RANGES" label="Range" hide-label />
    <Chips v-model="metrics" :options="METRICS" multiple label="Show" hide-label />

    <section class="card">
      <p v-if="error" class="error" role="alert">{{ error }} <button type="button" class="btn" @click="load">Try again</button></p>
      <p v-else-if="!data" class="muted">Loading…</p>
      <template v-else>
        <TimeChart
          v-if="series.length"
          :t="data.t"
          :series="series"
          :bands="bands"
          :markers="markers"
          :tz="settings.tz"
          :height="240"
          :label="`Climate over the last ${range}`"
        />
        <p v-else class="muted">Pick something to show.</p>
        <p v-if="!hasData" class="muted">No readings in this range.</p>
        <p class="faint small">Resolution {{ Math.round(data.resolution_seconds / 60) }} min · gaps are times with no readings</p>
      </template>
    </section>

    <section v-if="data" class="card">
      <table class="stats">
        <thead>
          <tr><th scope="col"></th><th scope="col">min</th><th scope="col">avg</th><th scope="col">max</th></tr>
        </thead>
        <tbody class="num">
          <tr v-if="rh"><th scope="row">Humidity</th><td>{{ rh.min.toFixed(1) }}%</td><td>{{ rh.avg.toFixed(1) }}%</td><td>{{ rh.max.toFixed(1) }}%</td></tr>
          <tr v-if="temp"><th scope="row">Temperature</th><td>{{ temp.min.toFixed(1) }}°</td><td>{{ temp.avg.toFixed(1) }}°</td><td>{{ temp.max.toFixed(1) }}°</td></tr>
          <tr v-if="vpd"><th scope="row">VPD (kPa)</th><td>{{ vpd.min.toFixed(2) }}</td><td>{{ vpd.avg.toFixed(2) }}</td><td>{{ vpd.max.toFixed(2) }}</td></tr>
        </tbody>
      </table>
    </section>

    <section v-if="notes.length" class="card stack" aria-labelledby="log-h">
      <h2 id="log-h" class="h2">Log</h2>
      <ul class="log">
        <li v-for="(n, i) in notes" :key="i">{{ n }}</li>
      </ul>
    </section>
  </main>
</template>

<style scoped>
.h2 {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.small {
  font-size: var(--fs-xs);
  margin-top: var(--sp-2);
}
.stats {
  width: 100%;
  border-collapse: collapse;
}
.stats th,
.stats td {
  padding: var(--sp-2) var(--sp-1);
  text-align: right;
}
.stats th[scope='row'] {
  text-align: left;
  font-weight: 600;
}
.stats thead th {
  color: var(--text-muted);
  font-weight: 600;
  font-size: var(--fs-sm);
}
.log {
  margin: 0;
  padding-left: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.error {
  color: var(--danger);
}
</style>
