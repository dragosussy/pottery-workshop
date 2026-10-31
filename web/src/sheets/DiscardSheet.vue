<script setup lang="ts">
// Discard (plan §4.6): an optional slider (off until touched), a note, an
// "Entered by mistake" checkbox, and WhenChips.
import { computed, ref, toRef, watch } from 'vue'
import BottomSheet from '@/components/BottomSheet.vue'
import DrynessSlider from '@/components/DrynessSlider.vue'
import WhenChips from '@/components/WhenChips.vue'
import { useSheetPieces } from '@/composables/useSheetPieces'
import { transitionErrorMessage, usePieceActions } from '@/composables/usePieceActions'
import { serverNow } from '@/composables/useServerClock'
import { useSettingsStore } from '@/stores/settings'
import { numberList } from '@/utils/format'
import { resolveWhen, type WhenChoice } from '@/utils/when'

const props = defineProps<{ ids: number[] }>()
const emit = defineEmits<{ close: []; done: [] }>()
const { pieces, loading } = useSheetPieces(toRef(props, 'ids'))
const actions = usePieceActions()
const settings = useSettingsStore()

const discardable = computed(() => pieces.value.filter((p) => p.state === 'in' || p.state === 'flipped'))
const dryness = ref(2.0)
const touched = ref(false)
const note = ref('')
const mistake = ref(false)
const when = ref<WhenChoice>({ kind: 'now' })
const busy = ref(false)
const error = ref('')

watch(pieces, (ps) => (dryness.value = ps[0]?.current_dryness ?? 2.0), { immediate: true })

const title = computed(() => (discardable.value.length > 1 ? `Discard ${discardable.value.length} pieces` : `Discard ${numberList(discardable.value.map((p) => p.number))}`))

async function submit() {
  const at = resolveWhen(when.value, serverNow(), settings.tz)
  if (at === null) {
    error.value = 'Pick a time that is not in the future.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await actions.transition(discardable.value, {
      state: 'discarded',
      ...(at ? { ended_at: at } : {}),
      ...(touched.value && !mistake.value ? { end_dryness: dryness.value } : {}),
      ...(note.value.trim() ? { end_note: note.value.trim() } : {}),
      ...(mistake.value ? { mistake: true } : {}),
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
      <p v-if="!discardable.length" class="muted">Nothing to discard: {{ pieces.length ? 'already off the shelf.' : 'piece not found.' }}</p>
      <template v-else>
        <label class="check">
          <input v-model="mistake" type="checkbox" />
          <span><strong>Entered by mistake</strong><br /><span class="muted">It never existed: hide it everywhere.</span></span>
        </label>
        <div v-if="!mistake">
          <p class="field-label">How dry was it? <span class="faint">(optional)</span></p>
          <DrynessSlider
            v-model="dryness"
            v-model:touched="touched"
            :last-value="discardable[0]?.current_dryness ?? null"
            label="Dryness when discarded"
            untouched-hint="not recorded: move it to record dryness"
          />
        </div>
        <label>
          <span class="field-label">Note <span class="faint">(optional)</span></span>
          <textarea v-model="note" class="input" maxlength="2000" placeholder="Cracked, reclaimed, gave up…"></textarea>
        </label>
        <WhenChips v-if="!mistake" v-model="when" />
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </template>
    </template>
    <template #footer>
      <button type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-danger btn-lg" :disabled="busy || !discardable.length" data-testid="confirm-discard" @click="submit">
        {{ busy ? 'Saving…' : 'Discard' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.check {
  display: flex;
  gap: var(--sp-3);
  align-items: flex-start;
  min-height: var(--touch);
  padding: var(--sp-2) 0;
}
.check input {
  width: 28px;
  height: 28px;
  flex: none;
  accent-color: var(--accent);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
