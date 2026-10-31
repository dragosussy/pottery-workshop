<script setup lang="ts">
// Add pieces (plan §4.2). Photo uploads in the background; Add waits for an
// upload in progress and never blocks on a failed one. POST /pieces is never
// retried: with no response, refetch the shelf and ask the user to check.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/client'
import type { AddPieces } from '@/api/types'
import BottomSheet from '@/components/BottomSheet.vue'
import Chips from '@/components/Chips.vue'
import DrynessSlider from '@/components/DrynessSlider.vue'
import PhotoPicker from '@/components/PhotoPicker.vue'
import QuantityStepper from '@/components/QuantityStepper.vue'
import WhenChips from '@/components/WhenChips.vue'
import { usePhotoUpload } from '@/composables/usePhotoUpload'
import { useStickyDefaults } from '@/composables/useStickyDefaults'
import { serverNow } from '@/composables/useServerClock'
import { showToast } from '@/composables/useToast'
import { useReferenceStore } from '@/stores/reference'
import { useSettingsStore } from '@/stores/settings'
import { useShelfStore } from '@/stores/shelf'
import { useLiveStore } from '@/stores/live'
import { inTrimBand, TRIM_BAND } from '@/utils/dryness'
import { resolveWhen, type WhenChoice } from '@/utils/when'

const props = withDefaults(defineProps<{ photoDeps?: Parameters<typeof usePhotoUpload>[0] }>(), { photoDeps: undefined })
const emit = defineEmits<{ close: []; done: [] }>()
const router = useRouter()
const reference = useReferenceStore()
const settings = useSettingsStore()
const shelf = useShelfStore()
const live = useLiveStore()

const defaults = useStickyDefaults()
const photo = usePhotoUpload(props.photoDeps)
const touched = ref(true)
const quantity = ref(1)
const when = ref<WhenChoice>({ kind: 'now' })
const label = ref('')
const notes = ref('')
const busy = ref<'' | 'waiting' | 'saving'>('')
const error = ref('')
const unconfirmed = ref(false)

const warning = computed(() => {
  if (defaults.dryness > TRIM_BAND[1]) return 'Already past leather hard: it will show as past trim.'
  if (inTrimBand(defaults.dryness)) return 'Already in the trim band: it will show as ready now.'
  return ''
})

async function submit(next: boolean) {
  const at = resolveWhen(when.value, serverNow(), settings.tz)
  if (at === null) {
    error.value = 'Pick a time that is not in the future.'
    return
  }
  error.value = ''
  unconfirmed.value = false
  if (photo.busy.value) {
    busy.value = 'waiting'
    await photo.settled()
  }
  busy.value = 'saving'
  const photoFailed = photo.state.value === 'error'
  const body: AddPieces = {
    quantity: quantity.value,
    form: defaults.form,
    initial_dryness: defaults.dryness,
    size: defaults.size,
    thickness: defaults.thickness,
    ...(photo.photoId.value !== null ? { photo_id: photo.photoId.value } : {}),
    ...(label.value.trim() ? { label: label.value.trim() } : {}),
    ...(notes.value.trim() ? { notes: notes.value.trim() } : {}),
    ...(at ? { added_at: at } : {}),
  }
  try {
    const res = await api.addPieces(body)
    shelf.mergePieces(res.items)
    void shelf.refresh()
    void live.fetchStatus()
    showToast({
      big: res.items.map((p) => `#${p.number}`).join(' '),
      message: `Write ${res.items.length === 1 ? 'it on the bat' : 'them on the bats'}${photoFailed ? ' · added without the photo' : ''}`,
      tone: 'ok',
      timeoutMs: 10_000,
    })
    if (next) {
      photo.clear()
      quantity.value = 1
      label.value = ''
      notes.value = ''
    } else emit('done')
  } catch (e) {
    if (isApiError(e) && e.unconfirmed) {
      unconfirmed.value = true
      void shelf.refresh()
    } else error.value = errorMessage(e, "Couldn't add. Try again.")
  } finally {
    busy.value = ''
  }
}

function checkShelf() {
  emit('close')
  setTimeout(() => void router.push('/shelf'), 0)
}
</script>

<template>
  <BottomSheet title="Add pieces" @close="emit('close')">
    <PhotoPicker :photo="photo" />
    <Chips v-model="defaults.form" :options="reference.data.forms" label="Form" />
    <div>
      <p class="field-label">How wet is it?</p>
      <DrynessSlider v-model="defaults.dryness" v-model:touched="touched" label="Dryness when added" untouched-hint="" />
      <p v-if="warning" class="warn" role="note">⚠ {{ warning }}</p>
    </div>
    <Chips v-model="defaults.size" :options="reference.data.sizes" label="Size" size="lg" />
    <Chips v-model="defaults.thickness" :options="reference.data.thicknesses" label="Wall" />
    <QuantityStepper v-model="quantity" label="How many identical pieces?" />
    <WhenChips v-model="when" label="Thrown" />
    <details>
      <summary class="more">More: label, notes</summary>
      <div class="stack">
        <label>
          <span class="field-label">Label</span>
          <input v-model="label" class="input" maxlength="80" placeholder="e.g. Anna's mugs" />
        </label>
        <label>
          <span class="field-label">Notes</span>
          <textarea v-model="notes" class="input" maxlength="2000"></textarea>
        </label>
      </div>
    </details>
    <div v-if="unconfirmed" class="unconfirmed" role="alert" data-testid="add-unconfirmed">
      <p><strong>Couldn't confirm.</strong> Check the shelf before adding again: the pieces may already be there.</p>
      <button type="button" class="btn" @click="checkShelf">Check the shelf</button>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <template #footer>
      <button type="button" class="btn" :disabled="!!busy" data-testid="add-next" @click="submit(true)">Add &amp; next</button>
      <button type="button" class="btn btn-primary btn-lg" :disabled="!!busy" data-testid="add-submit" @click="submit(false)">
        {{ busy === 'waiting' ? 'Waiting for photo…' : busy === 'saving' ? 'Adding…' : quantity > 1 ? `Add ${quantity}` : 'Add' }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.warn {
  margin-top: var(--sp-2);
  color: var(--warn-strong);
  font-weight: 600;
}
.more {
  min-height: var(--touch);
  display: flex;
  align-items: center;
  font-weight: 600;
  cursor: pointer;
}
.unconfirmed {
  padding: var(--sp-3);
  border-radius: var(--radius);
  background: var(--warn-soft);
  border: 2px solid var(--warn-border);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
</style>
