<script setup lang="ts">
// Edit details / correct times (plan §4.5: "tapping a time corrects it").
// A PATCH without `state` is a correction (contract §3.3).
import { computed, nextTick, reactive, ref, toRef, watch } from 'vue'
import type { FormKey, PiecePatch, SizeKey, ThicknessKey } from '@/api/types'
import { errorMessage } from '@/api/client'
import BottomSheet from '@/components/BottomSheet.vue'
import Chips from '@/components/Chips.vue'
import DrynessSlider from '@/components/DrynessSlider.vue'
import { useSheetPieces } from '@/composables/useSheetPieces'
import { usePieceActions } from '@/composables/usePieceActions'
import { serverNow } from '@/composables/useServerClock'
import { showToast } from '@/composables/useToast'
import { useReferenceStore } from '@/stores/reference'
import { useSettingsStore } from '@/stores/settings'
import { fromLocalInputValue, toIso, toLocalInputValue } from '@/utils/time'

const props = defineProps<{ ids: number[]; focus?: string | null }>()
const emit = defineEmits<{ close: []; done: [] }>()
const { pieces, loading } = useSheetPieces(toRef(props, 'ids'))
const reference = useReferenceStore()
const settings = useSettingsStore()
const actions = usePieceActions()
const piece = computed(() => pieces.value[0] ?? null)

const f = reactive({
  form: 'mug' as FormKey,
  size: 'medium' as SizeKey,
  thickness: 'medium' as ThicknessKey,
  label: '',
  notes: '',
  added: '',
  initial: 1,
  flipped: '',
  ended: '',
  end: 3,
})
const touchedInitial = ref(true)
const touchedEnd = ref(true)
const busy = ref(false)
const error = ref('')
const local = (ts: string | null) => (ts ? toLocalInputValue(Date.parse(ts), settings.tz) : '')

watch(piece, async (p) => {
  if (!p) return
  Object.assign(f, {
    form: p.form,
    size: p.size,
    thickness: p.thickness,
    label: p.label,
    notes: p.notes,
    added: local(p.added_at),
    initial: p.initial_dryness ?? 1,
    flipped: local(p.flipped_at),
    ended: local(p.ended_at),
    end: p.end_dryness ?? 3,
  })
  await nextTick()
  if (props.focus) document.getElementById(`edit-${props.focus}`)?.scrollIntoView({ block: 'center' })
})

async function submit() {
  const p = piece.value
  if (!p) return
  const body: PiecePatch = {}
  if (f.form !== p.form) body.form = f.form
  if (f.size !== p.size) body.size = f.size
  if (f.thickness !== p.thickness) body.thickness = f.thickness
  if (f.label !== p.label) body.label = f.label
  if (f.notes !== p.notes) body.notes = f.notes
  if (p.initial_dryness !== null && f.initial !== p.initial_dryness) body.initial_dryness = f.initial
  const times: [string, string | null, 'added_at' | 'flipped_at' | 'ended_at'][] = [
    [f.added, p.added_at, 'added_at'],
    [f.flipped, p.flipped_at, 'flipped_at'],
    [f.ended, p.ended_at, 'ended_at'],
  ]
  for (const [v, orig, key] of times) {
    if (!orig || v === local(orig)) continue
    const t = fromLocalInputValue(v, settings.tz)
    if (t === null || t > serverNow() + 60_000) {
      error.value = 'Times must not be in the future.'
      return
    }
    body[key] = toIso(t)
  }
  if (p.ended_at && p.end_dryness !== null && f.end !== p.end_dryness) body.end_dryness = f.end
  if (!Object.keys(body).length) return emit('done')
  busy.value = true
  error.value = ''
  try {
    await actions.correct(p.id, body)
    showToast({ message: `#${p.number} updated`, tone: 'ok' })
    emit('done')
  } catch (e) {
    error.value = errorMessage(e, "Couldn't save")
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BottomSheet :title="piece ? `Edit #${piece.number}` : 'Edit'" @close="emit('close')">
    <p v-if="loading" class="muted">Loading…</p>
    <template v-else-if="piece">
      <Chips v-model="f.form" :options="reference.data.forms" label="Form" />
      <Chips v-model="f.size" :options="reference.data.sizes" label="Size" />
      <Chips v-model="f.thickness" :options="reference.data.thicknesses" label="Wall" />
      <label>
        <span class="field-label">Label</span>
        <input v-model="f.label" class="input" maxlength="80" />
      </label>
      <label>
        <span class="field-label">Notes</span>
        <textarea v-model="f.notes" class="input" maxlength="2000"></textarea>
      </label>
      <label id="edit-added_at">
        <span class="field-label">Added ({{ settings.tz }})</span>
        <input v-model="f.added" class="input" type="datetime-local" />
      </label>
      <div v-if="piece.initial_dryness !== null">
        <p class="field-label">Dryness when added</p>
        <DrynessSlider v-model="f.initial" v-model:touched="touchedInitial" label="Dryness when added" untouched-hint="" />
      </div>
      <label v-if="piece.flipped_at" id="edit-flipped_at">
        <span class="field-label">Flipped</span>
        <input v-model="f.flipped" class="input" type="datetime-local" />
      </label>
      <label v-if="piece.ended_at" id="edit-ended_at">
        <span class="field-label">{{ piece.state === 'trimmed' ? 'Trimmed' : 'Discarded' }}</span>
        <input v-model="f.ended" class="input" type="datetime-local" />
      </label>
      <div v-if="piece.ended_at && piece.end_dryness !== null">
        <p class="field-label">Dryness when it left the shelf</p>
        <DrynessSlider v-model="f.end" v-model:touched="touchedEnd" label="End dryness" untouched-hint="" />
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </template>
    <template #footer>
      <button type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="busy || !piece" @click="submit">{{ busy ? 'Saving…' : 'Save' }}</button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
