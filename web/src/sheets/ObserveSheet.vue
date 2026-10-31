<script setup lang="ts">
// Record dryness (piece detail / selection), or correct an observation (`obs`).
// Outside check-in, the model's estimate IS shown as a faint marker (plan §4.5).
import { computed, onMounted, ref, toRef, watch } from 'vue'
import type { FollowUp, FollowUpOption, Observation } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import BottomSheet from '@/components/BottomSheet.vue'
import DrynessSlider from '@/components/DrynessSlider.vue'
import WhenChips from '@/components/WhenChips.vue'
import FollowUpPrompt from '@/components/FollowUpPrompt.vue'
import { useSheetPieces } from '@/composables/useSheetPieces'
import { useObservationSave } from '@/composables/useObservationSave'
import { serverNow } from '@/composables/useServerClock'
import { showToast } from '@/composables/useToast'
import { useSheet } from '@/composables/useSheet'
import { useShelfStore } from '@/stores/shelf'
import { useSettingsStore } from '@/stores/settings'
import { numberList } from '@/utils/format'
import { fromLocalInputValue, toIso, toLocalInputValue } from '@/utils/time'
import { resolveWhen, type WhenChoice } from '@/utils/when'

const props = defineProps<{ ids: number[]; obsId?: number | null }>()
const emit = defineEmits<{ close: []; done: [] }>()
const { pieces, loading } = useSheetPieces(toRef(props, 'ids'))
const { save } = useObservationSave()
const sheet = useSheet()
const shelf = useShelfStore()
const settings = useSettingsStore()

const editing = ref<Observation | null>(null)
const dryness = ref(2.0)
const touched = ref(false)
const when = ref<WhenChoice>({ kind: 'now' })
const editTime = ref('')
const busy = ref(false)
const error = ref('')
const followUps = ref<FollowUp[]>([])

const live = computed(() => pieces.value.filter((p) => p.state === 'in' || p.state === 'flipped'))
const single = computed(() => (pieces.value.length === 1 ? pieces.value[0]! : null))
const title = computed(() => (props.obsId ? 'Correct a reading' : `Record dryness · ${numberList(pieces.value.map((p) => p.number))}`))

watch(pieces, (ps) => {
  if (!props.obsId) dryness.value = ps[0]?.current_dryness ?? 2.0
})

onMounted(async () => {
  if (!props.obsId || !props.ids[0]) return
  try {
    const list = await api.pieceObservations(props.ids[0])
    editing.value = list.items.find((o) => o.id === props.obsId) ?? null
    if (editing.value) {
      dryness.value = editing.value.dryness
      editTime.value = toLocalInputValue(Date.parse(editing.value.observed_at), settings.tz)
    }
  } catch (e) {
    error.value = errorMessage(e)
  }
})

async function submit() {
  error.value = ''
  if (editing.value) {
    const t = fromLocalInputValue(editTime.value, settings.tz)
    if (t === null || t > serverNow() + 60_000) {
      error.value = 'Pick a time that is not in the future.'
      return
    }
    busy.value = true
    try {
      const body: { dryness?: number; observed_at?: string } = {}
      if (dryness.value !== editing.value.dryness) body.dryness = dryness.value
      if (toLocalInputValue(Date.parse(editing.value.observed_at), settings.tz) !== editTime.value) body.observed_at = toIso(t)
      if (Object.keys(body).length) await api.patchObservation(editing.value.id, body)
      void shelf.refresh()
      showToast({ message: 'Reading corrected', tone: 'ok' })
      emit('done')
    } catch (e) {
      error.value = errorMessage(e, "Couldn't save")
    } finally {
      busy.value = false
    }
    return
  }
  const at = resolveWhen(when.value, serverNow(), settings.tz)
  if (at === null) {
    error.value = 'Pick a time that is not in the future.'
    return
  }
  busy.value = true
  const res = await save({ items: live.value.map((p) => ({ piece_id: p.id, dryness: dryness.value })), ...(at ? { observed_at: at } : {}) })
  busy.value = false
  if (!res.ok) {
    error.value = res.message
    return
  }
  showToast({ message: `Saved for ${numberList(live.value.map((p) => p.number))}`, tone: 'ok' })
  if (res.data.follow_ups.length) followUps.value = res.data.follow_ups
  else emit('done')
}

function onAnswered(fu: FollowUp, o: FollowUpOption) {
  followUps.value = followUps.value.filter((f) => f !== fu)
  if (o.action === 'correct_observation' && fu.previous_observation_id) {
    void sheet.open('observe', { ids: [fu.piece_id], obs: fu.previous_observation_id })
    return
  }
  if (o.action === 'end_piece') {
    void sheet.open('trim', { ids: [fu.piece_id] })
    return
  }
  if (!followUps.value.length) emit('done')
}
function onSkipped(fu: FollowUp) {
  followUps.value = followUps.value.filter((f) => f !== fu)
  if (!followUps.value.length) emit('done')
}
</script>

<template>
  <BottomSheet :title="title" @close="emit('close')">
    <p v-if="loading" class="muted">Loading…</p>
    <template v-else-if="followUps.length">
      <FollowUpPrompt v-for="fu in followUps.slice(0, 1)" :key="fu.observation_id" :follow-up="fu" @answered="onAnswered(fu, $event)" @skipped="onSkipped(fu)" />
    </template>
    <template v-else>
      <p v-if="!obsId && !live.length" class="muted">These pieces have left the shelf.</p>
      <template v-else>
        <DrynessSlider
          v-model="dryness"
          v-model:touched="touched"
          :last-value="obsId ? null : (single?.current_dryness ?? null)"
          :estimate="obsId ? null : (single?.prediction?.dryness_now ?? null)"
          label="How dry is it?"
          :untouched-hint="obsId ? '' : 'same as last time?'"
        />
        <p v-if="!obsId && single?.prediction?.dryness_now != null" class="faint legend">○ last reading · ┊ the app's estimate</p>
        <label v-if="obsId">
          <span class="field-label">When ({{ settings.tz }})</span>
          <input v-model="editTime" class="input" type="datetime-local" />
        </label>
        <WhenChips v-else v-model="when" />
      </template>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </template>
    <template v-if="!followUps.length" #footer>
      <button type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="busy || (!obsId && !live.length) || (!!obsId && !editing)" data-testid="confirm-observe" @click="submit">
        {{ busy ? 'Saving…' : 'Save' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.legend {
  font-size: var(--fs-sm);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
