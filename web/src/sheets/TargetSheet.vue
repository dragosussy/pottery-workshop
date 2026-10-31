<script setup lang="ts">
// The humidity target: a precise slider, and a live preview of when to come
// with that target (GET /control/preview), so the potter can raise it until
// the visit lands on a day that suits them.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import BottomSheet from '@/components/BottomSheet.vue'
import HumiditySlider from '@/components/HumiditySlider.vue'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import type { ControlPreview } from '@/api/types'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { useNow } from '@/composables/useServerClock'
import { formatDayTime, formatVisitWindow } from '@/utils/format'
import { parseTs } from '@/utils/time'

const emit = defineEmits<{ close: []; done: [] }>()
const live = useLiveStore()
const settings = useSettingsStore()
const now = useNow()

const control = computed(() => live.status?.control ?? null)
const room = computed(() => live.status?.room.humidity_pct ?? null)
const saved = computed(() => control.value?.target_humidity_pct ?? null)
const min = computed(() => control.value?.min_pct ?? 50)
const max = computed(() => control.value?.max_pct ?? 98)

const clampPct = (v: number) => Math.min(max.value, Math.max(min.value, Math.round(v)))
const target = ref(clampPct(saved.value ?? room.value ?? 80))

const busy = ref(false)
const error = ref('')

// Preview, debounced while dragging; a newer request aborts the older one.
const preview = ref<ControlPreview | null>(null)
const previewFor = ref<number | null>(null)
const previewError = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null
let inflight: AbortController | null = null

async function loadPreview(pct: number) {
  inflight?.abort()
  const ac = new AbortController()
  inflight = ac
  try {
    preview.value = await api.previewTarget(pct, ac.signal)
    previewFor.value = pct
    previewError.value = false
  } catch {
    if (!ac.signal.aborted) previewError.value = true
  }
}
watch(
  target,
  (pct) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => void loadPreview(pct), 250)
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
  inflight?.abort()
})

const visitText = computed(() => {
  const p = preview.value
  if (!p) return null
  const v = p.next_visit
  if (p.ready_now_count > 0 && (!v || (parseTs(v.start) ?? 0) <= now.value)) {
    return { headline: `${p.ready_now_count} ready now`, detail: 'the target only changes pieces still drying' }
  }
  if (!v) return { headline: 'Nothing to trim in the next week', detail: '' }
  const start = parseTs(v.start)!
  const end = parseTs(v.end)!
  const parts = [`${v.ready_piece_ids.length} ready`]
  if (v.maybe_piece_ids.length) parts.push(`${v.maybe_piece_ids.length} maybe`)
  return {
    headline: start <= now.value ? `Come now, until ${formatDayTime(end, now.value, settings.tz)}` : `Come ${formatVisitWindow(start, end, now.value, settings.tz)}`,
    detail: parts.join(', '),
  }
})
const stale = computed(() => previewFor.value !== target.value)

async function save(pct: number | null) {
  busy.value = true
  error.value = ''
  try {
    await api.setTarget(pct)
    void live.fetchStatus()
    emit('done')
  } catch (e) {
    error.value = errorMessage(e, "Couldn't save the target")
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BottomSheet title="Humidity target" @close="emit('close')">
    <HumiditySlider v-model="target" :min="min" :max="max" :room-value="room" />

    <section class="preview" :class="{ 'is-stale': stale }" aria-live="polite" data-testid="target-preview">
      <p class="field-label">With {{ target }}%</p>
      <p v-if="previewError" class="muted">Couldn't work out the visit.</p>
      <p v-else-if="!visitText" class="muted">Working it out…</p>
      <template v-else>
        <p class="headline"><strong>{{ visitText.headline }}</strong></p>
        <p v-if="visitText.detail" class="muted">{{ visitText.detail }}</p>
      </template>
      <p class="muted small">Higher humidity dries slower, so the visit moves later.</p>
    </section>

    <p v-if="control?.controller && control.limits" class="note muted" data-testid="pump-note">
      The pump holds the shelf at the target: it runs {{ control.limits.pulse_seconds }} s when the shelf is
      {{ control.limits.hysteresis_pct }}% below, then waits {{ Math.round(control.limits.rest_seconds / 60) }} min for
      the mist to spread. It can only add moisture: if the shelf is above the target, open the plastic a little.
    </p>
    <p v-else-if="control" class="note muted">
      No pump is connected. Keep the shelf near the target with the plastic and water; the predictions assume it stays
      there.
    </p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>

    <template #footer>
      <button v-if="saved !== null" type="button" class="btn btn-ghost" :disabled="busy" data-testid="clear-target" @click="save(null)">
        No target
      </button>
      <button v-else type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="busy" data-testid="save-target" @click="save(target)">
        {{ busy ? 'Saving…' : saved === target ? 'Keep ' + target + '%' : 'Set ' + target + '%' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.preview {
  padding: var(--sp-3);
  border-radius: var(--radius);
  background: var(--surface-2);
  transition: opacity 120ms;
}
.preview.is-stale {
  opacity: 0.6;
}
.preview p {
  margin: 0;
}
.headline {
  font-size: var(--fs-lg);
}
.small {
  margin-top: var(--sp-2) !important;
  font-size: var(--fs-sm);
}
.note {
  font-size: var(--fs-sm);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
