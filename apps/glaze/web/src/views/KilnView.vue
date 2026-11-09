<script setup lang="ts">
// The one kiln, in whichever state it's in: empty, being loaded, or firing.
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { say, sayError } from '@ui/messages'
import { api, thumbUrl, toInput, type Firing, type Level, type Piece, type Suggestions } from '@/api'
import ConfirmButton from '@/components/ConfirmButton.vue'
import KilnShelf from '@/components/KilnShelf.vue'
import { kilnFiring } from '@/kilnStatus'
import { LEVELS, LOADING_ORDER, firingLabel, pieceTitle, positionText, stackText } from '@/format'

const route = useRoute()
const router = useRouter()
const load = ref<Firing | null>(null)
const inKiln = ref<Piece[]>([])
const bench = ref<Piece[]>([])
const selectedId = ref<number | null>(null)
const starting = ref(false)
const loaded = ref(false)
const details = reactive({ fired_on: '', cone: '', schedule: '', notes: '' })
const suggestions = ref<Suggestions>({ ingredients: [], schedules: [], cones: [] })

async function refresh() {
  try {
    const firings = await api.firings()
    load.value = firings.find((f) => f.state !== 'unloaded') ?? null
    kilnFiring.value = load.value?.state === 'firing'
    ;[inKiln.value, bench.value] = await Promise.all([
      load.value ? api.pieces({ firing: load.value.id }) : Promise.resolve([]),
      api.pieces({ state: 'bench' }),
    ])
  } catch (e) {
    sayError(e)
  }
  loaded.value = true
}

onMounted(async () => {
  await refresh()
  selectedId.value = Number(route.query.select) || toPlace.value[0]?.id || null
})

const placed = (p: Piece) => p.level !== null && p.x !== null
const selected = computed(() => [...bench.value, ...inKiln.value].find((p) => p.id === selectedId.value) ?? null)
const toPlace = computed(() => [...inKiln.value.filter((p) => !placed(p)), ...bench.value])
const shelves = computed(() => LOADING_ORDER.map((level) => ({ level, label: LEVELS[level], pieces: inKiln.value.filter((p) => p.level === level) })))
const canStart = computed(() => details.schedule.trim() !== '')

async function startLoading() {
  try {
    await api.addFiring({ fired_on: null, cone: '', schedule: '', notes: '' })
    await refresh()
    selectedId.value = selectedId.value ?? toPlace.value[0]?.id ?? null
  } catch (e) {
    sayError(e)
  }
}

async function place(level: Level, x: number, y: number) {
  const p = selected.value
  if (!load.value || !p || load.value.state !== 'loading') return
  const moving = placed(p)
  try {
    await api.savePiece(p.id, { ...toInput(p), firing_id: load.value.id, level, x, y })
    await refresh()
    // Loading goes piece after piece; moving one doesn't start that again.
    selectedId.value = moving ? null : (toPlace.value[0]?.id ?? null)
  } catch (e) {
    sayError(e)
  }
}

function tapPiece(p: Piece) {
  if (load.value?.state === 'firing') void router.push(`/pieces/${p.id}`)
  else selectedId.value = p.id // picked up, to move it or take it out
}

async function takeOut(p: Piece) {
  try {
    await api.savePiece(p.id, { ...toInput(p), firing_id: null, level: null, x: null, y: null })
    selectedId.value = null
    await refresh()
  } catch (e) {
    sayError(e)
  }
}

function askToStart() {
  const f = load.value
  if (!f) return
  Object.assign(details, { fired_on: f.fired_on ?? new Date().toLocaleDateString('en-CA'), cone: f.cone, schedule: f.schedule, notes: f.notes })
  starting.value = true
  void api.suggestions().then((s) => (suggestions.value = s))
}

async function startFiring() {
  if (!load.value) return
  try {
    const { fired_on, cone, schedule, notes } = details
    await api.saveFiring(load.value.id, { fired_on: fired_on || null, cone, schedule, notes })
    await api.setFiringState(load.value.id, 'firing')
    starting.value = false
    selectedId.value = null
    await refresh()
    // The button is at the bottom of a long page, and the news is at the top.
    window.scrollTo({ top: 0, behavior: 'smooth' })
  } catch (e) {
    sayError(e)
  }
}

async function backToLoading() {
  if (!load.value) return
  try {
    await api.setFiringState(load.value.id, 'loading')
    await refresh()
  } catch (e) {
    sayError(e)
  }
}

async function unload() {
  if (!load.value) return
  const id = load.value.id
  try {
    await api.setFiringState(id, 'unloaded')
    say('Kiln unloaded', false, async () => {
      await api.setFiringState(id, 'firing')
      void router.replace('/kiln')
    })
    void router.push(`/firings/${id}/results`)
  } catch (e) {
    sayError(e)
  }
}

async function cancelLoad() {
  if (!load.value) return
  try {
    await api.deleteFiring(load.value.id)
    await refresh()
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <h1>Kiln</h1>
      <RouterLink to="/kiln/past" class="btn btn-ghost">Past firings</RouterLink>
    </header>

    <section v-if="loaded && !load" class="card">
      <p>The kiln is empty.</p>
      <button type="button" class="btn btn-primary btn-lg btn-block" @click="startLoading">Start loading</button>
    </section>

    <template v-if="load?.state === 'loading'">
      <p class="muted">Being loaded. Choose a piece, then tap where it sits.</p>
      <div v-if="selected" class="notice sticky">
        <strong>{{ pieceTitle(selected) }}</strong>:
        <template v-if="placed(selected)">{{ positionText(selected) }}. Tap a new spot to move it.</template>
        <template v-else>tap where it goes, on any shelf.</template>
        <div class="row actions">
          <button v-if="selected.firing_id === load.id" type="button" class="btn btn-small" @click="takeOut(selected)">Take out of the kiln</button>
          <button type="button" class="btn btn-small btn-ghost" @click="selectedId = null">Cancel</button>
        </div>
      </div>
      <p v-else-if="inKiln.length" class="small muted">Tap a piece on a shelf to move it.</p>

      <KilnShelf
        v-for="s in shelves" :key="s.level" :label="s.label" :pieces="s.pieces" :selected="selectedId" :placing="!!selected"
        @place="(x, y) => place(s.level, x, y)" @piece="tapPiece"
      />

      <section class="rows">
        <h2>To place</h2>
        <button v-for="p in toPlace" :key="p.id" type="button" class="pick" :aria-pressed="p.id === selectedId" @click="selectedId = p.id">
          <img v-if="p.photos.at(-1)" :src="thumbUrl(p.photos.at(-1)!.id)" alt="" />
          <span class="pick-text">
            <strong>{{ pieceTitle(p) }}</strong>
            <span v-if="p.layers.length" class="small muted">{{ stackText(p.layers) }}</span>
          </span>
        </button>
        <p v-if="!toPlace.length" class="small muted">Everything is placed.</p>
        <RouterLink :to="{ path: '/new', query: { back: 1 } }" class="btn btn-block">Log another piece</RouterLink>
      </section>

      <button v-if="!starting" type="button" class="btn btn-primary btn-lg btn-block" :disabled="!load.pieces" @click="askToStart">Start firing</button>
      <form v-else class="card" @submit.prevent="startFiring">
        <h2>Start firing</h2>
        <label class="field">Schedule
          <input v-model="details.schedule" class="input" list="schedules" maxlength="200" placeholder="e.g. 1240 °C, 100°/h, slow cool" />
          <datalist id="schedules"><option v-for="s in suggestions.schedules" :key="s" :value="s"></option></datalist>
        </label>
        <label class="field">Cone
          <input v-model="details.cone" class="input" list="cones" maxlength="20" placeholder="e.g. 6" />
          <datalist id="cones"><option v-for="c in suggestions.cones" :key="c" :value="c"></option></datalist>
        </label>
        <label class="field">Date
          <input v-model="details.fired_on" class="input" type="date" />
        </label>
        <label class="field">Notes
          <textarea v-model="details.notes" class="input" maxlength="2000"></textarea>
        </label>
        <button type="submit" class="btn btn-primary btn-block" :disabled="!canStart">Start firing</button>
        <button type="button" class="btn btn-ghost btn-block" @click="starting = false">Not yet</button>
      </form>
      <ConfirmButton v-if="!load.pieces" class="btn-block" label="Cancel this load" @confirmed="cancelLoad" />
    </template>

    <template v-if="load?.state === 'firing'">
      <section class="card fire firing-banner" role="status">
        <strong class="headline">The kiln is firing</strong>
        <p>{{ firingLabel(load) }}</p>
        <p v-if="load.notes" class="small">{{ load.notes }}</p>
      </section>
      <button type="button" class="btn btn-primary btn-lg btn-block" @click="unload">Unload</button>
      <KilnShelf v-for="s in shelves" :key="s.level" :label="s.label" :pieces="s.pieces" firing @piece="tapPiece" />
      <button type="button" class="btn btn-block" @click="backToLoading">Back to loading</button>
    </template>
  </main>
</template>

<style scoped>
.firing-banner {
  gap: var(--sp-1);
}
.headline {
  font-size: var(--fs-xl);
}
.sticky {
  position: sticky;
  top: var(--sp-2);
  z-index: 2;
  box-shadow: var(--shadow);
}
.actions {
  margin-top: var(--sp-2);
}
.pick {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  width: 100%;
  min-height: var(--touch);
  padding: var(--sp-2);
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  text-align: left;
  cursor: pointer;
}
.pick[aria-pressed='true'] {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.pick img {
  width: 48px;
  height: 48px;
  flex: none;
  border-radius: 4px;
  object-fit: cover;
}
.pick-text {
  display: flex;
  flex-direction: column;
}
</style>
