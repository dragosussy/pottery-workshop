<script setup lang="ts">
// Find anything: a search box, the common filters as chips, the rest folded.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { sayError } from '@ui/messages'
import { PAGE_SIZE, api, type ClayBody, type Material, type Piece, type Suggestions } from '@/api'
import PieceTile from '@/components/PieceTile.vue'
import { DEFECTS, LEVELS, SPOTS, positionText } from '@/format'

const blank = { q: '', kind: '', material: '', clay: '', schedule: '', level: '', spot: '', rating: '', defect: '' }
const filters = reactive({ ...blank })
const pieces = ref<Piece[]>([])
const more = ref(false)
const clayBodies = ref<ClayBody[]>([])
const materials = ref<Material[]>([])
const suggestions = ref<Suggestions>({ ingredients: [], schedules: [], cones: [] })

const sortedMaterials = computed(() => [...materials.value].sort((a, b) => a.name.localeCompare(b.name)))
const caption = (p: Piece) => [p.firing?.schedule, positionText(p)].filter(Boolean).join(' · ')
const filtered = computed(() => Object.values(filters).some(Boolean))

async function search(offset = 0) {
  try {
    const page = await api.pieces({ ...filters, offset })
    pieces.value = offset ? [...pieces.value, ...page] : page
    more.value = page.length === PAGE_SIZE
  } catch (e) {
    sayError(e)
  }
}

onMounted(async () => {
  void search()
  try {
    ;[clayBodies.value, materials.value, suggestions.value] = await Promise.all([api.clayBodies(), api.materials(), api.suggestions()])
  } catch (e) {
    sayError(e)
  }
})
let typing: ReturnType<typeof setTimeout> | undefined
watch(filters, () => {
  clearTimeout(typing)
  typing = setTimeout(() => void search(), 250) // don't search on every keystroke
})
</script>

<template>
  <main class="page">
    <header class="bar"><h1>Pieces</h1></header>

    <input v-model="filters.q" class="input" type="search" placeholder="Search notes, materials or a number" aria-label="Search" />
    <div class="chips" role="group" aria-label="Quick filters">
      <button type="button" :aria-pressed="filters.rating === 'great'" @click="filters.rating = filters.rating === 'great' ? '' : 'great'">Keepers</button>
      <button type="button" :aria-pressed="filters.kind === 'tile'" @click="filters.kind = filters.kind === 'tile' ? '' : 'tile'">Tiles</button>
      <button type="button" :aria-pressed="filters.kind === 'pot'" @click="filters.kind = filters.kind === 'pot' ? '' : 'pot'">Pots</button>
    </div>
    <details class="more">
      <summary>More filters</summary>
      <div class="filters">
        <select v-model="filters.material" class="input" aria-label="Material">
          <option value="">Any material</option>
          <option v-for="m in sortedMaterials" :key="m.id" :value="m.id">{{ m.name }}</option>
        </select>
        <select v-model="filters.clay" class="input" aria-label="Clay body">
          <option value="">Any clay</option>
          <option v-for="c in clayBodies" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
        <select v-model="filters.schedule" class="input" aria-label="Schedule">
          <option value="">Any schedule</option>
          <option v-for="s in suggestions.schedules" :key="s" :value="s">{{ s }}</option>
        </select>
        <select v-model="filters.level" class="input" aria-label="Shelf">
          <option value="">Any shelf</option>
          <option v-for="(text, key) in LEVELS" :key="key" :value="key">{{ text }}</option>
        </select>
        <select v-model="filters.spot" class="input" aria-label="Edge or middle">
          <option value="">Edge or middle</option>
          <option v-for="(text, key) in SPOTS" :key="key" :value="key">{{ text }}</option>
        </select>
        <select v-model="filters.defect" class="input" aria-label="Defect">
          <option value="">Any defect</option>
          <option v-for="(text, key) in DEFECTS" :key="key" :value="key">{{ text }}</option>
        </select>
      </div>
    </details>
    <button v-if="filtered" type="button" class="btn btn-ghost btn-small" @click="Object.assign(filters, blank)">Clear filters</button>

    <p v-if="!pieces.length" class="muted">Nothing found.</p>
    <div class="tiles">
      <PieceTile v-for="p in pieces" :key="p.id" :piece="p" :caption="caption(p)" />
    </div>
    <button v-if="more" type="button" class="btn btn-block" @click="search(pieces.length)">Show more</button>
  </main>
</template>

<style scoped>
.filters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-2);
}
</style>
