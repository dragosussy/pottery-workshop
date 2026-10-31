<script setup lang="ts">
// Settings → Pump: what the pump is doing, a manual run for checking the
// wiring (or priming after a refill), and Resume after a safety stop.
import { computed, ref } from 'vue'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { useNow } from '@/composables/useServerClock'
import { showToast } from '@/composables/useToast'
import { dayTime } from '@/utils/format'
import { pumpReasonText, pumpRuntimeText } from '@/utils/pump'

const TEST_SECONDS = 5

const live = useLiveStore()
const settings = useSettingsStore()
const now = useNow()
const busy = ref(false)

const control = computed(() => live.status?.control ?? null)
const pump = computed(() => control.value?.pump ?? null)
const safety = computed(() => control.value?.safety ?? null)
const limits = computed(() => control.value?.limits ?? null)

async function act(fn: () => Promise<unknown>, ok: string) {
  busy.value = true
  try {
    await fn()
    showToast({ message: ok, tone: 'ok' })
    void live.fetchStatus()
  } catch (e) {
    showToast({ message: errorMessage(e, "Couldn't reach the pump"), tone: 'error' })
  } finally {
    busy.value = false
  }
}
const run = () => act(() => api.runPump(TEST_SECONDS), `Pump running for ${TEST_SECONDS} s`)
const stop = () => act(() => api.stopPump(), 'Pump stopped')
const resume = () => act(() => api.resetPump(), 'Pump resumed')
</script>

<template>
  <p v-if="!control" class="muted">Loading…</p>
  <p v-else-if="!control.controller || !pump" class="muted" data-testid="no-pump">
    No pump connected. To add one, wire the relay and set <code>PUMP_DRIVER=gpio</code>,
    <code>PUMP_GPIO_LINE</code> and <code>PUMP_ACTIVE_LOW</code> in docker-compose.yml.
  </p>
  <template v-else>
    <p class="state" data-testid="pump-state">
      <span class="dot" :class="{ on: pump.on }" aria-hidden="true"></span>
      <strong>{{ pump.on ? 'Running' : 'Off' }}</strong>
      <span class="muted"> · {{ pumpReasonText(pump.reason) }}</span>
    </p>
    <p class="muted">
      {{ pumpRuntimeText(pump.runtime_24h_seconds) }}<template v-if="pump.last_run_at">
        · last run {{ dayTime(pump.last_run_at, now, settings.tz) }}</template
      >
    </p>
    <p v-if="pump.error" class="error" role="alert">Switch error: {{ pump.error }}</p>
    <div v-if="safety?.tripped" class="tripped" role="alert">
      <p>
        <strong>Stopped for safety.</strong> The humidity didn't rise after several runs. Check the water tank and the
        nozzle, then resume.
      </p>
      <button type="button" class="btn btn-primary" :disabled="busy" data-testid="pump-resume" @click="resume">Resume</button>
    </div>
    <div class="row">
      <button v-if="!pump.on" type="button" class="btn" :disabled="busy" data-testid="pump-run" @click="run">
        Run {{ TEST_SECONDS }} s
      </button>
      <button v-else type="button" class="btn" :disabled="busy" data-testid="pump-stop" @click="stop">Stop</button>
      <span class="faint small">to check the wiring, or prime it after a refill</span>
    </div>
    <p v-if="limits" class="faint small">
      Runs {{ limits.pulse_seconds }} s at a time, then waits {{ Math.round(limits.rest_seconds / 60) }} min. Never above
      {{ limits.max_rh_pct }}%.
    </p>
  </template>
</template>

<style scoped>
p {
  margin: 0;
}
.state {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--text-faint);
}
.dot.on {
  background: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.row {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  flex-wrap: wrap;
}
.small {
  font-size: var(--fs-sm);
}
.tripped {
  display: grid;
  gap: var(--sp-2);
  padding: var(--sp-3);
  border-radius: var(--radius);
  background: var(--danger-soft);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
