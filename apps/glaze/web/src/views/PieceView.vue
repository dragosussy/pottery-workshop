<script setup lang="ts">
// One piece, and what to do with it next.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Choice from '@ui/Choice.vue'
import { say, sayError } from '@ui/messages'
import { api, photoUrl, type Photo, type PhotoStage, type Piece } from '@/api'
import BackLink from '@/components/BackLink.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import JourneyStrip from '@/components/JourneyStrip.vue'
import KilnShelf from '@/components/KilnShelf.vue'
import PhotoPicker from '@/components/PhotoPicker.vue'
import { DEFECTS, RATINGS, firingLabel, layerDetail, pieceTitle, positionText } from '@/format'
import { goBack } from '@/nav'

const route = useRoute()
const router = useRouter()
const piece = ref<Piece | null>(null)
const notFound = ref(false)
const newPhoto = ref<Blob | null>(null)
const newPhotoStage = ref<PhotoStage>('after')
const uploading = ref(false)

async function load() {
  try {
    piece.value = await api.piece(Number(route.params.id))
    notFound.value = false
    newPhotoStage.value = piece.value.state === 'bench' ? 'before' : 'after'
  } catch (e) {
    notFound.value = true
    sayError(e)
  }
}
watch(() => route.params.id, () => route.params.id && load(), { immediate: true })

const photos = computed(() => {
  // The fired result first, since that's what people came to see.
  const all = piece.value?.photos ?? []
  return [...all.filter((p) => p.stage === 'after').reverse(), ...all.filter((p) => p.stage === 'before')]
})
const fired = computed(() => piece.value?.state === 'waiting' || piece.value?.state === 'done')
const stackHeading = computed(() => (fired.value ? 'How to repeat it' : "What's on it"))

async function upload() {
  if (!newPhoto.value || !piece.value) return
  uploading.value = true
  try {
    await api.addPhoto(piece.value.id, newPhotoStage.value, newPhoto.value)
    newPhoto.value = null
    await load()
  } catch (e) {
    sayError(e)
  } finally {
    uploading.value = false
  }
}

async function removePhoto(p: Photo) {
  try {
    await api.deletePhoto(p.id)
    await load()
  } catch (e) {
    sayError(e)
  }
}

async function remove() {
  if (!piece.value) return
  const title = pieceTitle(piece.value)
  try {
    await api.deletePiece(piece.value.id)
    say(`${title} deleted`)
    goBack(router, '/pieces')
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink fallback="/pieces" />
    </header>
    <p v-if="notFound" class="card">That piece isn't in the notebook.</p>

    <template v-if="piece">
      <div class="row title">
        <h1>{{ pieceTitle(piece) }}</h1>
        <span v-if="piece.rating" class="badge" :class="piece.rating">{{ RATINGS[piece.rating] }}</span>
        <span v-if="piece.from_memory" class="badge warn">Logged after firing</span>
      </div>
      <JourneyStrip :state="piece.state" />

      <template v-if="piece.state === 'bench'">
        <RouterLink :to="`/pieces/${piece.id}/edit`" class="btn btn-primary btn-lg btn-block">Add what went on</RouterLink>
        <RouterLink :to="{ path: '/kiln', query: { select: piece.id } }" class="btn btn-block">Put in the kiln</RouterLink>
      </template>
      <RouterLink v-else-if="piece.state === 'kiln'" to="/kiln" class="btn btn-lg btn-block">See it in the kiln</RouterLink>
      <RouterLink v-else-if="piece.state === 'waiting'" :to="{ path: `/firings/${piece.firing_id}/results`, query: { piece: piece.id } }" class="btn btn-primary btn-lg btn-block">Enter the result</RouterLink>
      <RouterLink v-else :to="{ path: '/new', query: { from: piece.id } }" class="btn btn-primary btn-lg btn-block">Do this again</RouterLink>
      <RouterLink v-if="piece.state !== 'bench'" :to="`/pieces/${piece.id}/edit`" class="btn btn-block">Edit this record</RouterLink>

      <section v-for="p in photos" :key="p.id" class="photo">
        <img :src="photoUrl(p.id)" :alt="`${pieceTitle(piece)}, ${p.stage} firing`" />
        <div class="row caption">
          <span class="small muted">{{ p.stage === 'after' ? 'After firing' : 'Before firing' }}</span>
          <ConfirmButton class="btn-small" label="Delete" @confirmed="removePhoto(p)" />
        </div>
      </section>

      <section class="card">
        <h2>{{ stackHeading }}</h2>
        <p><strong>Clay:</strong> {{ piece.clay_body ?? 'Not recorded' }}</p>
        <ol v-if="piece.layers.length" class="stack">
          <li v-for="(l, i) in piece.layers" :key="i">
            <RouterLink :to="{ path: '/glazes', query: { open: l.material_id } }"><strong>{{ l.material }}</strong></RouterLink>
            <span v-if="l.recipe_version" class="small muted"> version {{ l.recipe_version }}</span>
            <div v-if="layerDetail(l)" class="small muted">{{ layerDetail(l) }}</div>
            <div v-if="l.notes" class="small">{{ l.notes }}</div>
          </li>
        </ol>
        <p v-else class="muted">No layers recorded.</p>
        <p v-if="piece.firing">
          <strong>Firing: </strong>
          <RouterLink :to="piece.state === 'kiln' ? '/kiln' : `/firings/${piece.firing.id}`">{{ firingLabel(piece.firing) }}</RouterLink>
        </p>
        <div v-if="positionText(piece)" class="where">
          <p><strong>In the kiln:</strong> {{ positionText(piece) }}</p>
          <KilnShelf v-if="piece.x !== null && piece.y !== null" :marked="{ x: piece.x, y: piece.y, level: piece.level }" />
        </div>
        <p v-if="piece.notes"><strong>Notes:</strong> {{ piece.notes }}</p>
      </section>

      <section v-if="piece.defects.length || piece.result_notes" class="card">
        <h2>How it came out</h2>
        <div v-if="piece.defects.length" class="row">
          <span v-for="d in piece.defects" :key="d" class="badge warn">{{ DEFECTS[d] }}</span>
        </div>
        <p v-if="piece.result_notes"><strong>Notes:</strong> {{ piece.result_notes }}</p>
      </section>

      <p v-if="piece.refire_of"><RouterLink :to="`/pieces/${piece.refire_of}`">See it before this re-fire</RouterLink></p>
      <p v-if="piece.refired_by"><RouterLink :to="`/pieces/${piece.refired_by}`">See it after it was re-fired</RouterLink></p>

      <details class="more">
        <summary>More</summary>
        <div class="more-body">
          <div class="field-group">
            <span class="section-title">Add a photo</span>
            <PhotoPicker v-model="newPhoto" label="Choose a photo" />
            <template v-if="newPhoto">
              <Choice v-model="newPhotoStage" label="Taken" :options="{ before: 'Before firing', after: 'After firing' }" />
              <button type="button" class="btn btn-primary btn-block" :disabled="uploading" @click="upload">{{ uploading ? 'Uploading…' : 'Save photo' }}</button>
            </template>
          </div>
          <RouterLink v-if="fired && !piece.refired_by" :to="{ path: '/new', query: { refire: piece.id } }" class="btn btn-block">Re-fire it</RouterLink>
          <ConfirmButton class="btn-block" label="Delete this record" @confirmed="remove" />
        </div>
      </details>
    </template>
  </main>
</template>

<style scoped>
.title {
  gap: var(--sp-2);
}
.photo img {
  width: 100%;
  border-radius: var(--radius);
}
.caption {
  justify-content: space-between;
  margin-top: var(--sp-1);
}
.where {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
</style>
