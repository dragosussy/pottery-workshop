<script setup lang="ts">
// What needs you: find the tile in your hand, log what you just did, or carry
// on with whatever is in progress.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { sayError } from '@ui/messages'
import { api, type Firing, type Piece } from '@/api'
import PieceRow from '@/components/PieceRow.vue'
import { firingLabel } from '@/format'

const router = useRouter()
const number = ref('')
const missing = ref<string | null>(null)
const toGlaze = ref<Piece[]>([])
const firings = ref<Firing[]>([])

const active = computed(() => firings.value.find((f) => f.state !== 'unloaded'))
const toEnter = computed(() => firings.value.filter((f) => f.state === 'unloaded' && f.waiting > 0))
const kilnLine = computed(() => {
  const f = active.value
  if (!f) return 'The kiln is empty'
  const count = `${f.pieces} piece${f.pieces === 1 ? '' : 's'}`
  return f.state === 'firing' ? `The kiln is firing: ${count}` : `Being loaded: ${count}`
})

onMounted(async () => {
  try {
    ;[toGlaze.value, firings.value] = await Promise.all([api.pieces({ state: 'bench' }), api.firings()])
  } catch (e) {
    sayError(e)
  }
})

async function find() {
  const n = number.value.trim()
  if (!n) return
  missing.value = null
  try {
    // Newest first, so a re-fired tile opens on its latest record.
    const [piece] = await api.pieces({ number: n })
    if (piece) void router.push(`/pieces/${piece.id}`)
    else missing.value = n
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <h1>Glaze notebook</h1>
      <RouterLink to="/settings" class="btn btn-ghost" aria-label="Settings">⚙︎</RouterLink>
    </header>

    <form class="card" @submit.prevent="find">
      <label class="field">Find a tile by its number
        <span class="find">
          <input v-model="number" class="input num" type="text" inputmode="numeric" placeholder="142" />
          <button type="submit" class="btn btn-primary">Find</button>
        </span>
      </label>
      <p v-if="missing" class="small">
        Tile {{ missing }} isn't in the notebook yet.
        <RouterLink :to="{ path: '/new', query: { fired: 1, number: missing } }">Add it</RouterLink>
      </p>
    </form>

    <RouterLink to="/new" class="btn btn-primary btn-lg btn-block">Log a piece</RouterLink>

    <RouterLink v-for="f in toEnter" :key="f.id" :to="`/firings/${f.id}/results`" class="btn btn-block">Enter {{ f.waiting }} result{{ f.waiting === 1 ? '' : 's' }}: {{ firingLabel(f) }}</RouterLink>

    <RouterLink to="/kiln" class="card kiln-line" :class="{ fire: active?.state === 'firing' }">
      <strong>{{ kilnLine }}</strong>
      <span aria-hidden="true">›</span>
    </RouterLink>

    <section v-if="toGlaze.length" class="rows">
      <h2>To be glazed</h2>
      <PieceRow v-for="p in toGlaze" :key="p.id" :piece="p" />
    </section>
  </main>
</template>

<style scoped>
.find {
  display: flex;
  gap: var(--sp-2);
}
.kiln-line {
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
  color: inherit;
  text-decoration: none;
}
</style>
