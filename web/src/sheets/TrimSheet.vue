<script setup lang="ts">
// Trim (plan §4.6): "How dry is it now?" pre-filled at 3.0, WhenChips, and the
// covering hint. One tap for the usual case.
import { computed, ref, toRef } from 'vue'
import BottomSheet from '@/components/BottomSheet.vue'
import DrynessSlider from '@/components/DrynessSlider.vue'
import WhenChips from '@/components/WhenChips.vue'
import { useSheetPieces } from '@/composables/useSheetPieces'
import { transitionErrorMessage, usePieceActions } from '@/composables/usePieceActions'
import { serverNow } from '@/composables/useServerClock'
import { useSettingsStore } from '@/stores/settings'
import { numberList } from '@/utils/format'
import { resolveWhen, type WhenChoice } from '@/utils/when'

const props = defineProps<{ ids: number[]; initialDryness?: number | null }>()
const emit = defineEmits<{ close: []; done: [] }>()
const { pieces, loading } = useSheetPieces(toRef(props, 'ids'))
const actions = usePieceActions()
const settings = useSettingsStore()

const trimmable = computed(() => pieces.value.filter((p) => p.state === 'in' || p.state === 'flipped'))
const dryness = ref(props.initialDryness ?? 3.0)
const touched = ref(true)
const when = ref<WhenChoice>({ kind: 'now' })
const note = ref('')
const busy = ref(false)
const error = ref('')

const title = computed(() => (trimmable.value.length > 1 ? `Trim ${trimmable.value.length} pieces` : `Trim ${numberList(trimmable.value.map((p) => p.number))}`))

async function submit() {
  const at = resolveWhen(when.value, serverNow(), settings.tz)
  if (at === null) {
    error.value = 'Pick a time that is not in the future.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await actions.transition(trimmable.value, {
      state: 'trimmed',
      end_dryness: dryness.value,
      ...(at ? { ended_at: at } : {}),
      ...(note.value.trim() ? { end_note: note.value.trim() } : {}),
    })
    emit('done')
  } catch (e) {
    error.value = transitionErrorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BottomSheet :title="title" @close="emit('close')">
    <p v-if="loading" class="muted">Loading…</p>
    <template v-else>
      <p v-if="!trimmable.length" class="muted">Nothing to trim: {{ pieces.length ? 'already off the shelf.' : 'piece not found.' }}</p>
      <template v-else>
        <div>
          <p class="field-label">How dry is it now?</p>
          <DrynessSlider
            v-model="dryness"
            v-model:touched="touched"
            :last-value="trimmable.length === 1 ? (trimmable[0]?.current_dryness ?? null) : null"
            label="Dryness when trimmed"
            untouched-hint=""
          />
          <p class="hint">Covering it to trim later? Set how dry it is now.</p>
        </div>
        <WhenChips v-model="when" />
        <details>
          <summary class="more">Add a note</summary>
          <textarea v-model="note" class="input" maxlength="2000" aria-label="Note"></textarea>
        </details>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </template>
    </template>
    <template #footer>
      <button type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="busy || !trimmable.length" data-testid="confirm-trim" @click="submit">
        {{ busy ? 'Saving…' : 'Trim' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.hint {
  margin-top: var(--sp-2);
  font-style: italic;
  color: var(--text-muted);
}
.more {
  min-height: var(--touch);
  display: flex;
  align-items: center;
  font-weight: 600;
  cursor: pointer;
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
