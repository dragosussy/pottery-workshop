<script setup lang="ts">
// Settings (plan §4.9): time zone, pump, sensors (offset + salt test), backup, about.
// The studio schedule (days/hours) was removed by owner decision 2026-10-31:
// the potter can come any time, so only the display time zone remains.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import type { Health } from '@/api/types'
import { api, backupUrl } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/client'
import { useSettingsStore } from '@/stores/settings'
import { useLiveStore } from '@/stores/live'
import { showToast } from '@/composables/useToast'
import { useNow } from '@/composables/useServerClock'
import { deviceTimeZone, parseTs } from '@/utils/time'
import { formatDayTime } from '@/utils/format'
import SensorRow from '@/components/SensorRow.vue'
import PalettePicker from '@/components/PalettePicker.vue'
import PumpPanel from '@/components/PumpPanel.vue'

const settings = useSettingsStore()
const live = useLiveStore()
const route = useRoute()
const now = useNow()

const tz = ref(deviceTimeZone())
const saving = ref(false)
const formError = ref('')
/** True when the field shows the phone's zone proposed for first setup. */
const proposed = ref(false)

function fillForm() {
  const s = settings.data
  if (!s) return
  // First setup: the server falls back to UTC until a zone is saved, so
  // propose the phone's zone instead (contract §3.10).
  const device = deviceTimeZone()
  proposed.value = s.studio_tz === 'UTC' && device !== 'UTC'
  tz.value = proposed.value ? device : s.studio_tz
}
watch(() => settings.data, fillForm, { immediate: true })

const zones = computed(() => {
  let list: string[] = []
  try {
    list = Intl.supportedValuesOf('timeZone')
  } catch {
    list = []
  }
  if (!list.includes(tz.value)) list = [tz.value, ...list]
  return list
})
const dirty = computed(() => !!settings.data && tz.value !== settings.data.studio_tz)

async function saveTz() {
  saving.value = true
  formError.value = ''
  try {
    await settings.patch({ studio_tz: tz.value })
    proposed.value = false
    showToast({ message: 'Time zone saved', tone: 'ok' })
    void live.fetchStatus()
  } catch (e) {
    formError.value = errorMessage(e, "Couldn't save")
  } finally {
    saving.value = false
  }
}

// Sensors: merge live readings with stored offsets.
const sensors = computed(() => {
  const liveList = live.status?.sensors ?? []
  const stored = settings.data?.sensors ?? []
  const names = new Set([...liveList.map((s) => s.sensor), ...stored.map((s) => s.sensor)])
  return [...names].map((name) => ({
    name,
    live: liveList.find((s) => s.sensor === name) ?? null,
    stored: stored.find((s) => s.sensor === name) ?? null,
  }))
})

// About
const health = ref<Health | null>(null)
const healthError = ref(false)
onMounted(async () => {
  try {
    health.value = await api.health()
  } catch (e) {
    healthError.value = isApiError(e)
  }
  if (route.hash === '#tz') document.getElementById('tz')?.scrollIntoView({ block: 'start' })
})
const startedAt = computed(() => {
  const t = parseTs(health.value?.started_at)
  return t === null ? '—' : formatDayTime(t, now.value, settings.tz)
})
const feVersion = __APP_VERSION__
</script>

<template>
  <main class="page">
    <header class="page-header">
      <RouterLink to="/" class="btn btn-ghost" aria-label="Back to Now"><span aria-hidden="true">←</span></RouterLink>
      <h1 class="page-title">Settings</h1>
      <span style="width: 48px"></span>
    </header>

    <section id="tz" class="card stack" aria-labelledby="tz-h">
      <h2 id="tz-h" class="section-title">Time zone</h2>
      <p class="muted">Every time in the app is shown in this zone.</p>
      <label>
        <span class="sr-only">Time zone</span>
        <select v-model="tz" class="input" data-testid="tz-select">
          <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
        </select>
      </label>
      <p v-if="proposed" class="muted small">Proposed from your phone. Save to use it.</p>
      <p v-if="formError" class="error" role="alert">{{ formError }}</p>
      <button type="button" class="btn btn-primary btn-block" :disabled="saving || !dirty" data-testid="save-tz" @click="saveTz">
        {{ saving ? 'Saving…' : 'Save time zone' }}
      </button>
    </section>

    <section id="pump" class="card stack" aria-labelledby="pump-h">
      <h2 id="pump-h" class="section-title">Pump</h2>
      <PumpPanel />
    </section>

    <section class="card stack" aria-labelledby="sensors-h">
      <h2 id="sensors-h" class="section-title">Sensors</h2>
      <p v-if="!sensors.length" class="muted">No sensors reported yet.</p>
      <SensorRow v-for="s in sensors" :key="s.name" :name="s.name" :live="s.live" :stored="s.stored" />
    </section>

    <section class="card stack" aria-labelledby="backup-h">
      <h2 id="backup-h" class="section-title">Backup</h2>
      <p class="muted">Keep a copy off the controller's SD card. Photos aren't included.</p>
      <a :href="backupUrl" class="btn btn-block" download>⤓ Download backup</a>
    </section>

    <section class="card stack" aria-labelledby="colours-h">
      <h2 id="colours-h" class="section-title">Colours (trying out)</h2>
      <PalettePicker />
    </section>

    <section class="card stack" aria-labelledby="about-h">
      <h2 id="about-h" class="section-title">About</h2>
      <dl class="about">
        <dt>App</dt>
        <dd class="num">{{ feVersion }}</dd>
        <dt>Controller</dt>
        <dd class="num">{{ health ? `${health.version} (${health.commit})` : healthError ? 'unreachable' : '…' }}</dd>
        <dt>Clock</dt>
        <dd>{{ health ? (health.clock_synced ? '✓ synced' : '⚠ not synced yet') : '…' }}</dd>
        <dt>Running since</dt>
        <dd>{{ startedAt }}</dd>
      </dl>
      <p class="shutdown">
        <strong>Switching off:</strong> use the controller's power button, don't pull the plug. It protects the SD card.
      </p>
      <RouterLink to="/dev/slider" class="faint">Slider test page</RouterLink>
    </section>
  </main>
</template>

<style scoped>
.section-title {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.small {
  font-size: var(--fs-sm);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
.about {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-1) var(--sp-4);
  margin: 0;
}
.about dt {
  color: var(--text-muted);
}
.about dd {
  margin: 0;
}
.shutdown {
  padding: var(--sp-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
</style>
