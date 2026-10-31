<script setup lang="ts">
// One sensor: live reading, offset, salt-test guide, "I replaced this sensor".
import { computed, ref } from 'vue'
import type { SensorSettings, SensorStatus } from '@/api/types'
import { errorMessage } from '@/api/client'
import { useSettingsStore } from '@/stores/settings'
import { useLiveStore } from '@/stores/live'
import { showToast } from '@/composables/useToast'
import { useNow } from '@/composables/useServerClock'
import { formatOffset, saltTestOffset, SALT_RH, OFFSET_MAX, OFFSET_MIN } from '@/utils/calibration'
import { dayTime } from '@/utils/format'

const props = defineProps<{ name: string; live: SensorStatus | null; stored: SensorSettings | null }>()
const settings = useSettingsStore()
const liveStore = useLiveStore()
const now = useNow()

const offset = computed(() => props.stored?.offset_pct ?? props.live?.offset_pct ?? 0)
const mode = ref<'idle' | 'salt' | 'manual' | 'replace'>('idle')
const saltReading = ref<number | null>(null)
const manualOffset = ref<number | null>(null)
const busy = ref(false)
const error = ref('')

const proposed = computed(() => (saltReading.value === null || Number.isNaN(saltReading.value) ? null : saltTestOffset(saltReading.value, offset.value)))

function openSalt() {
  mode.value = 'salt'
  saltReading.value = props.live?.humidity_pct ?? null
  error.value = ''
}
function openManual() {
  mode.value = 'manual'
  manualOffset.value = offset.value
  error.value = ''
}

async function save(newOffset: number, replaced = false) {
  busy.value = true
  error.value = ''
  try {
    await settings.patch({ sensors: [{ sensor: props.name, offset_pct: newOffset, ...(replaced ? { replaced: true } : {}) }] })
    showToast({ message: replaced ? `${props.name}: marked as replaced` : `${props.name}: offset set to ${formatOffset(newOffset)}`, tone: 'ok' })
    mode.value = 'idle'
    void liveStore.fetchStatus()
  } catch (e) {
    error.value = errorMessage(e, "Couldn't save")
  } finally {
    busy.value = false
  }
}

const manualValid = computed(() => manualOffset.value !== null && manualOffset.value >= OFFSET_MIN && manualOffset.value <= OFFSET_MAX)
const stateText = computed(() => {
  const s = props.live?.state
  if (s === 'ok') return '✓ ok'
  if (s === 'stale') return '⚠ stale'
  if (s === 'no_data') return '✕ no data'
  return '—'
})
</script>

<template>
  <div class="sensor" data-testid="sensor-row">
    <div class="sensor-head">
      <strong>{{ name }}</strong>
      <span class="sensor-state" :class="`st-${live?.state ?? 'none'}`">{{ stateText }}</span>
    </div>
    <div class="sensor-read num">
      {{ live?.humidity_pct != null ? live.humidity_pct.toFixed(1) + '%' : '—' }}
      · {{ live?.temperature_c != null ? live.temperature_c.toFixed(1) + '°C' : '—' }}
      <span class="muted">· offset {{ formatOffset(offset) }}</span>
    </div>
    <div v-if="stored?.calibrated_at" class="faint small">Calibrated {{ dayTime(stored.calibrated_at, now, settings.tz) }}</div>
    <div v-if="live?.last_error" class="small error">{{ live.last_error }}</div>

    <div v-if="mode === 'idle'" class="row-wrap">
      <button type="button" class="btn" @click="openSalt">Salt test</button>
      <button type="button" class="btn" @click="openManual">Set offset</button>
      <button type="button" class="btn btn-ghost" @click="mode = 'replace'">I replaced this sensor</button>
    </div>

    <div v-else-if="mode === 'salt'" class="panel stack">
      <ol class="guide">
        <li>Put a little damp table salt (wet sand consistency) in a small dish.</li>
        <li>Seal the dish and the sensor together in an airtight box or zip bag.</li>
        <li>Wait at least 8 hours at a steady temperature.</li>
        <li>Enter the humidity the app shows now. The box should be at {{ SALT_RH }}%.</li>
      </ol>
      <label>
        <span class="field-label">Reading in the salt box (%)</span>
        <input v-model.number="saltReading" class="input num" type="number" inputmode="decimal" step="0.1" min="0" max="100" />
      </label>
      <p v-if="proposed !== null">New offset: <strong class="num">{{ formatOffset(proposed) }}</strong></p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="row-wrap">
        <button type="button" class="btn btn-primary" :disabled="proposed === null || busy" @click="proposed !== null && save(proposed)">Use this offset</button>
        <button type="button" class="btn btn-ghost" @click="mode = 'idle'">Cancel</button>
      </div>
    </div>

    <div v-else-if="mode === 'manual'" class="panel stack">
      <label>
        <span class="field-label">Offset (% RH, {{ OFFSET_MIN }} to +{{ OFFSET_MAX }})</span>
        <input v-model.number="manualOffset" class="input num" type="number" inputmode="decimal" step="0.1" :min="OFFSET_MIN" :max="OFFSET_MAX" />
      </label>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="row-wrap">
        <button type="button" class="btn btn-primary" :disabled="!manualValid || busy" @click="manualOffset !== null && save(manualOffset)">Save offset</button>
        <button type="button" class="btn btn-ghost" @click="mode = 'idle'">Cancel</button>
      </div>
    </div>

    <div v-else class="panel stack">
      <p>This resets the offset to ±0 and notes the swap in the climate log. Do a salt test on the new sensor when you can.</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="row-wrap">
        <button type="button" class="btn btn-warn" :disabled="busy" @click="save(0, true)">Yes, I replaced it</button>
        <button type="button" class="btn btn-ghost" @click="mode = 'idle'">Cancel</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sensor {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  padding-bottom: var(--sp-3);
  border-bottom: 1px solid var(--border);
}
.sensor:last-child {
  border-bottom: 0;
}
.sensor-head {
  display: flex;
  justify-content: space-between;
}
.sensor-state.st-ok {
  color: var(--ok);
}
.sensor-state.st-stale,
.sensor-state.st-no_data {
  color: var(--warn-strong);
  font-weight: 700;
}
.sensor-read {
  font-size: var(--fs-lg);
}
.small {
  font-size: var(--fs-sm);
}
.error {
  color: var(--danger);
}
.panel {
  padding: var(--sp-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.guide {
  margin: 0;
  padding-left: 1.2em;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
</style>
