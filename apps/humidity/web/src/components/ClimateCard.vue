<script setup lang="ts">
// The shelf's climate in one card: humidity now next to its target (the
// target chip opens the slider; no target means humidity control is off),
// a scale bar showing both, temperature, how old the reading is, the pump, and the
// charts.
import { computed, ref, watch } from 'vue'
import { api, type History, type Now } from '@/api'
import { ago, pumpText } from '@/format'
import { live, refreshShelf, sayError, setNow } from '@/live'
import LineChart from './LineChart.vue'
import PreciseSlider from '@ui/PreciseSlider.vue'

/** A short test run, enough to hear the pump and see the mist. */
const TEST_SECONDS = 1

const now = computed(() => live.now)
const room = computed(() => live.now?.room ?? null)

const status = computed(() => {
  if (live.unreachable) return live.nowAt ? `Can't reach the shelf · last update ${ago(live.clock - live.nowAt)}` : "Can't reach the shelf"
  if (!live.now) return 'Loading…'
  if (room.value) return ago(live.clock - Date.parse(room.value.at))
  const sensors = live.now.sensors
  const newest = Math.max(0, ...sensors.map((s) => (s.at ? Date.parse(s.at) : 0)))
  const last = newest ? ` · last reading ${ago(live.clock - newest)}` : ''
  // Every sensor failing to answer: usually not connected or miswired.
  if (sensors.length && sensors.every((s) => s.error)) {
    return newest ? `Sensor not responding${last}` : 'No sensor connected'
  }
  return newest ? `No fresh reading${last}` : 'No reading yet'
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
// Estimates depend on the target, so the piece list refreshes with it.
function targetSaved() {
  editing.value = false
  void refreshShelf()
}
const close = () => (editing.value = false)

// The scale bar under the reading: humidity and target on 40–100 %.
const SCALE_MIN = 40
const along = (pct: number) => `${Math.min(100, Math.max(0, ((pct - SCALE_MIN) / (100 - SCALE_MIN)) * 100))}%`
const shownTarget = computed(() => (editing.value ? draft.value : (now.value?.target ?? null)))

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
    <!-- Humidity and where it should be -->
    <div class="reading">
      <div>
        <div class="label">Humidity</div>
        <div v-if="room" class="big num">{{ room.rh.toFixed(1) }}<small> %</small></div>
        <div v-else class="big none">–</div>
      </div>
      <button
        v-if="now"
        type="button"
        class="target"
        :class="{ off: shownTarget === null, editing }"
        :aria-label="now.target === null ? 'Turn humidity control on' : 'Change the target'"
        :disabled="editing"
        @click="startEdit"
      >
        <span class="label">Target</span>
        <span v-if="shownTarget === null" class="mid">Off</span>
        <span v-else class="mid num">{{ shownTarget }}<small> %</small></span>
        <span class="action" :class="{ hidden: editing }">{{ now.target === null ? 'Turn on' : 'Change' }}</span>
      </button>
    </div>

    <div v-if="now" class="scale" aria-hidden="true">
      <div v-if="room" class="fill" :style="{ width: along(room.rh) }" />
      <div v-if="shownTarget !== null" class="tick" :style="{ left: along(shownTarget) }" />
    </div>

    <p class="small status">
      <span v-if="room" class="num temp">{{ room.temp.toFixed(1) }} °C</span>
      <span v-if="room" class="muted"> · </span>
      <span :class="bad ? 'bad' : 'muted'">{{ status }}</span>
      <span v-if="now && now.target === null && !editing" class="muted"> · not controlling humidity</span>
    </p>

    <template v-if="now && editing">
      <PreciseSlider v-model="draft" :min="50" :max="now.max_rh - 1" :step="1" unit=" %" label="Humidity target" :last="now.target" />
      <div class="row">
        <button type="button" class="btn btn-primary" :disabled="busy" @click="run(() => api.setTarget(draft), targetSaved)">Save</button>
        <button v-if="now.target !== null" type="button" class="btn" :disabled="busy" @click="run(() => api.setTarget(null), targetSaved)">Turn off</button>
        <button type="button" class="btn btn-ghost" @click="close">Cancel</button>
      </div>
    </template>

    <!-- The pump that holds the target -->
    <div v-if="now" class="pump">
      <template v-if="now.pump.enabled">
        <p :class="{ bad: now.pump.status === 'stopped' }">
          <span class="label">Pump</span> {{ pumpText(now.pump) }}
          <span v-if="now.pump.last_run_at" class="muted small"> · last ran {{ ago(live.clock - Date.parse(now.pump.last_run_at)) }}</span>
        </p>
        <div class="row">
          <button type="button" class="btn btn-small" :disabled="busy" @click="run(() => api.pumpRun(TEST_SECONDS))">Run {{ TEST_SECONDS }} s</button>
          <button v-if="now.pump.status === 'stopped'" type="button" class="btn btn-small btn-primary" :disabled="busy" @click="run(api.pumpResume)">Resume</button>
          <button v-else-if="now.target !== null || now.pump.on" type="button" class="btn btn-small btn-danger" :disabled="busy" @click="run(api.pumpStop)">Stop</button>
        </div>
      </template>
      <p v-else class="muted small"><span class="label">Pump</span> not connected</p>
    </div>

    <!-- The past -->
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
.mid {
  font-size: 2rem;
  font-weight: 800;
  line-height: 1;
}
.big small,
.mid small {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.none {
  color: var(--text-faint);
  font-weight: 400;
}

.reading {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: var(--sp-4);
}

/* The target: a tappable figure answering the big number. */
.target {
  display: grid;
  justify-items: end;
  gap: 2px;
  padding: var(--sp-1) var(--sp-2);
  margin: calc(-1 * var(--sp-1)) calc(-1 * var(--sp-2)) 0 0;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--accent);
  font: inherit;
  text-align: right;
  cursor: pointer;
}
.target:hover:not(:disabled) {
  background: var(--surface-2);
}
.target.editing {
  border-color: var(--accent);
  background: var(--accent-soft);
  cursor: default;
}
.target.off {
  color: var(--text-muted);
}
.target .action {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.target .action.hidden {
  visibility: hidden;
}

/* Humidity (fill) and target (tick) on one 40–100 % scale. */
.scale {
  position: relative;
  height: 10px;
  border-radius: 5px;
  background: var(--surface-2);
  margin-top: var(--sp-1);
}
.fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 5px;
  background: var(--dry-2);
  transition: width 0.4s;
}
.tick {
  position: absolute;
  top: -5px;
  bottom: -5px;
  width: 4px;
  margin-left: -2px;
  border-radius: 2px;
  background: var(--accent);
  box-shadow: 0 0 0 2px var(--surface);
  transition: left 0.15s;
}

.status {
  margin: 0;
}
.temp {
  color: var(--temp);
  font-weight: 700;
}

.pump {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: var(--sp-2) var(--sp-3);
  padding-top: var(--sp-3);
  border-top: 1px solid var(--surface-3);
}
.pump p {
  margin: 0;
}
.pump .label {
  margin-right: var(--sp-1);
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
