<script setup lang="ts">
// Unloading the kiln, one piece at a time.
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { say, sayError } from '@ui/messages'
import { api, photoUrl, toInput, type Defect, type Piece, type Rating } from '@/api'
import BackLink from '@/components/BackLink.vue'
import KilnShelf from '@/components/KilnShelf.vue'
import OptionalChoice from '@/components/OptionalChoice.vue'
import PhotoPicker from '@/components/PhotoPicker.vue'
import { DEFECTS, RATINGS, layerDetail, pieceTitle, positionText } from '@/format'

const route = useRoute()
const firingId = Number(route.params.id)
const pieces = ref<Piece[]>([])
const index = ref(0)
const loaded = ref(false)
const rating = ref<Rating | null>(null)
const defects = ref<Defect[]>([])
const notes = ref('')
const photo = ref<Blob | null>(null)
const busy = ref(false)

const current = computed(() => pieces.value[index.value])
const lastPhoto = computed(() => current.value?.photos.at(-1))

onMounted(async () => {
  try {
    // The order is fixed here, so finishing one piece doesn't reshuffle the rest.
    pieces.value = (await api.pieces({ firing: firingId })).filter((p) => !p.rating)
    // Opened from one piece's page, it starts on that piece.
    index.value = Math.max(0, pieces.value.findIndex((p) => p.id === Number(route.query.piece)))
  } catch (e) {
    sayError(e)
  }
  loaded.value = true
})

function next() {
  index.value++
  rating.value = null
  defects.value = []
  notes.value = ''
  photo.value = null
  window.scrollTo({ top: 0 })
}

function toggle(d: Defect) {
  defects.value = defects.value.includes(d) ? defects.value.filter((x) => x !== d) : [...defects.value, d]
}

async function save() {
  const p = current.value
  if (!p || !rating.value) return
  busy.value = true
  try {
    await api.savePiece(p.id, { ...toInput(p), rating: rating.value, defects: defects.value, result_notes: notes.value })
    if (photo.value) {
      try {
        await api.addPhoto(p.id, 'after', photo.value)
      } catch (err) {
        say(`Saved, but the photo didn't upload: ${(err as Error).message}`, true)
      }
    }
    next()
  } catch (e) {
    sayError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink :fallback="`/firings/${firingId}`" />
      <span v-if="current" class="muted num">{{ index + 1 }} of {{ pieces.length }}</span>
    </header>

    <section v-if="loaded && !current" class="card">
      <h1>All done</h1>
      <p class="muted">Every piece from this firing has a result.</p>
      <RouterLink to="/" class="btn btn-primary btn-block">Back to the notebook</RouterLink>
    </section>

    <template v-if="current">
      <h1>{{ pieceTitle(current) }}</h1>
      <p v-if="positionText(current)" class="muted">{{ positionText(current) }}</p>
      <KilnShelf v-if="current.x !== null && current.y !== null" :marked="{ x: current.x, y: current.y, level: current.level }" />
      <img v-if="lastPhoto" :src="photoUrl(lastPhoto.id)" class="identify" :alt="`${pieceTitle(current)} before firing`" />

      <section class="card">
        <ol v-if="current.layers.length" class="stack">
          <li v-for="(l, i) in current.layers" :key="i">
            <strong>{{ l.material }}</strong>
            <div v-if="layerDetail(l)" class="small muted">{{ layerDetail(l) }}</div>
          </li>
        </ol>
        <p v-else class="muted">No layers recorded.</p>
      </section>

      <form class="card" @submit.prevent="save">
        <PhotoPicker v-model="photo" label="Take a photo of the result" />
        <OptionalChoice v-model="rating" label="How did it come out?" :options="RATINGS" />
        <div class="field-group">
          <span class="section-title">Defects</span>
          <div class="chips">
            <button v-for="(text, d) in DEFECTS" :key="d" type="button" :aria-pressed="defects.includes(d)" @click="toggle(d)">{{ text }}</button>
          </div>
        </div>
        <label class="field">Notes
          <textarea v-model="notes" class="input" maxlength="2000"></textarea>
        </label>
        <button type="submit" class="btn btn-primary btn-block" :disabled="busy || !rating">{{ busy ? 'Saving…' : 'Save and next' }}</button>
        <button type="button" class="btn btn-ghost btn-block" @click="next">Skip for now</button>
      </form>
    </template>
  </main>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  font-weight: 600;
}
.identify {
  width: 100%;
  border-radius: var(--radius);
}
</style>
