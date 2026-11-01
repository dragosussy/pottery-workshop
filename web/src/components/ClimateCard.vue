<script setup lang="ts">
// The shelf's climate in one card: humidity now next to its target (with a
// Change control), temperature, how old the reading is, the pump, and the
// charts.
import { computed, ref, watch } from 'vue'
import { api, type History, type Now } from '@/api'
import { ago, pumpText } from '@/format'
import { live, sayError, setNow } from '@/live'
import LineChart from './LineChart.vue'
import PreciseSlider from './PreciseSlider.vue'

/** A short test run, enough to hear the pump and see the mist. */
const TEST_SECONDS = 1

const now = computed(() => live.now)
const room = computed(() => live.now?.room ?? null)

const status = computed(() => {
  if (live.unreachable) return live.nowAt ? `Can't reach the shelf · last update ${ago(live.clock - live.nowAt)}` : "Can't reach the shelf"
  if (!live.now) return 'Loading…'
  if (room.value) return ago(live.clock - Date.parse(room.value.at))
  const times = live.now.sensors.map((s) => (s.at ? Date.parse(s.at) : 0))
  const newest = Math.max(0, ...times)
  return newest ? `No fresh reading · last ${ago(live.clock - newest)}` : 'No reading yet'
})
const bad = computed(() => live.unreachable || (!!live.now && !room.value))

// Target editing.
const editing = ref(false)
const draft = ref(80)
const busy = ref(false)

function startEdit() {
  const n = now.value!
  const guess = n.target ?? Math.round(n.room?.rh ?? 80)
  draft.value = Math.min(n.max_rh - 1, Math.max(50, guess))
  editing.value = true
}
const close = () => (editing.value = false)

async function run(write: () => Promise<Now>, after?: () => void) {
  busy.value = true
  try {
    setNow(await write())
    after?.()
  } catch (e) {
    sayError(e)
  } finally {
    busy.value = false
  }
}

// Charts.
const hours = ref<1 | 24 | 168>(24)
const history = ref<History | null>(null)
const end = ref(0)
let fetchedAt = 0

async function load() {
  fetchedAt = Date.now()
  try {
    const h = await api.history(hours.value)
    end.value = Math.floor(Date.now() / 1000)
    history.value = h
  } catch {
    // the status line already says when the shelf is unreachable
  }
}
watch(hours, load, { immediate: true })
// Refresh the chart once a minute while the page is open (the clock ticks every second).
watch(
  () => live.clock,
  (c) => {
    if (c - fetchedAt > 60_000 && document.visibilityState === 'visible') void load()
  },
)
</script>

<template>
  <section class="card">
    <div class="top">
      <div>
        <div class="label">Humidity</div>
        <div class="big num">{{ room ? room.rh.toFixed(1) : '–' }}<small> %</small></div>
      </div>
      <div v-if="now" class="target">
        <div class="label">Target</div>
        <div class="big num">{{ now.target === null ? '–' : now.target }}<small v-if="now.target !== null"> %</small></div>
        <button v-if="!editing" type="button" class="btn btn-small" @click="startEdit">Change</button>
      </div>
    </div>
    <p class="small" :class="bad ? 'bad' : 'muted'">
      <span v-if="room" class="num">{{ room.temp.toFixed(1) }} °C · </span>{{ status }}
    </p>

    <template v-if="now && editing">
      <PreciseSlider v-model="draft" :min="50" :max="now.max_rh - 1" :step="1" unit=" %" label="Humidity target" :last="now.target" />
      <div class="row">
        <button type="button" class="btn btn-primary" :disabled="busy" @click="run(() => api.setTarget(draft), close)">Save</button>
        <button type="button" class="btn" :disabled="busy" @click="run(() => api.setTarget(null), close)">Clear</button>
        <button type="button" class="btn btn-ghost" @click="close">Cancel</button>
      </div>
    </template>

    <div v-if="now?.pump.enabled" class="pump">
      <p :class="{ bad: now.pump.status === 'stopped' }">
        <strong>Pump:</strong> {{ pumpText(now.pump) }}
        <span v-if="now.pump.last_run_at" class="muted small"> · last ran {{ ago(live.clock - Date.parse(now.pump.last_run_at)) }}</span>
      </p>
      <div class="row">
        <button type="button" class="btn btn-small" :disabled="busy" @click="run(() => api.pumpRun(TEST_SECONDS))">Run {{ TEST_SECONDS }} s</button>
        <button v-if="now.pump.status === 'stopped'" type="button" class="btn btn-small btn-primary" :disabled="busy" @click="run(api.pumpResume)">Resume</button>
        <button v-else type="button" class="btn btn-small btn-danger" :disabled="busy" @click="run(api.pumpStop)">Stop</button>
      </div>
    </div>
    <p v-else-if="now" class="muted small">No pump connected.</p>

    <div class="history-head">
      <span class="label">History</span>
      <div class="chips small-chips" role="group" aria-label="Chart period">
        <button type="button" :aria-pressed="hours === 1" @click="hours = 1">1 h</button>
        <button type="button" :aria-pressed="hours === 24" @click="hours = 24">24 h</button>
        <button type="button" :aria-pressed="hours === 168" @click="hours = 168">7 d</button>
      </div>
    </div>
    <template v-if="history">
      <LineChart title="Humidity" unit="%" :t="history.t" :v="history.rh" :step="history.step" :hours="hours" :end="end" :line="now?.target ?? null" />
      <LineChart title="Temperature" unit="°C" :t="history.t" :v="history.temp" :step="history.step" :hours="hours" :end="end" style="--series: var(--temp)" />
    </template>
  </section>
</template>

<style scoped>
.top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--sp-4);
}
.label {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--text-muted);
}
.big {
  font-size: 3.5rem;
  font-weight: 800;
  line-height: 1;
}
.big small {
  font-size: var(--fs-xl);
}
.target {
  text-align: right;
  display: grid;
  justify-items: end;
  gap: var(--sp-1);
}
.target .big {
  color: var(--accent);
}
.pump {
  display: grid;
  gap: var(--sp-2);
  padding-top: var(--sp-3);
  border-top: 1px solid var(--surface-3);
}
.pump p {
  margin: 0;
}
.history-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: var(--sp-3);
  border-top: 1px solid var(--surface-3);
}
.small-chips button {
  min-height: 36px;
  padding: 0 var(--sp-3);
  font-size: var(--fs-sm);
}
</style>
