<script setup lang="ts">
// Flip (plan §4.6): WhenChips plus an optional slider pre-filled with the last
// observed value. Skipping it records the flip alone.
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

const props = defineProps<{ ids: number[]; initialDryness?: number | null }>()
const emit = defineEmits<{ close: []; done: [] }>()
const { pieces, loading } = useSheetPieces(toRef(props, 'ids'))
const actions = usePieceActions()
const settings = useSettingsStore()

const flippable = computed(() => pieces.value.filter((p) => p.state === 'in'))
const dryness = ref(2.0)
const touched = ref(false)
const when = ref<WhenChoice>({ kind: 'now' })
const busy = ref(false)
const error = ref('')

watch(
  pieces,
  (ps) => {
    // Pre-fill: the value passed from check-in, else the last observed one.
    dryness.value = props.initialDryness ?? ps[0]?.current_dryness ?? 2.0
    touched.value = props.initialDryness !== undefined && props.initialDryness !== null
  },
  { immediate: true },
)

const title = computed(() => (flippable.value.length > 1 ? `Flip ${flippable.value.length} pieces` : `Flip ${numberList(flippable.value.map((p) => p.number))}`))

async function submit() {
  const at = resolveWhen(when.value, serverNow(), settings.tz)
  if (at === null) {
    error.value = 'Pick a time that is not in the future.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await actions.transition(flippable.value, {
      state: 'flipped',
      ...(at ? { flipped_at: at } : {}),
      ...(touched.value ? { flip_dryness: dryness.value } : {}),
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
      <p v-if="!flippable.length" class="muted">Nothing to flip: {{ pieces.length ? 'already flipped or off the shelf.' : 'piece not found.' }}</p>
      <template v-else>
        <div>
          <p class="field-label">How dry is it? <span class="faint">(optional)</span></p>
          <DrynessSlider
            v-model="dryness"
            v-model:touched="touched"
            :last-value="flippable[0]?.current_dryness ?? null"
            label="Dryness at the flip"
            untouched-hint="not recorded: move it to record dryness"
          />
        </div>
        <WhenChips v-model="when" label="When did you flip it?" />
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </template>
    </template>
    <template #footer>
      <button type="button" class="btn btn-ghost" @click="emit('close')">Cancel</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="busy || !flippable.length" data-testid="confirm-flip" @click="submit">
        {{ busy ? 'Saving…' : 'Flip' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
