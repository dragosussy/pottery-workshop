<script setup lang="ts">
// A firing that's over: what was in it and where.
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { sayError } from '@ui/messages'
import { api, type Firing, type Piece, type Suggestions } from '@/api'
import BackLink from '@/components/BackLink.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'
import KilnShelf from '@/components/KilnShelf.vue'
import PieceRow from '@/components/PieceRow.vue'
import { LEVELS, LOADING_ORDER, firingLabel } from '@/format'
import { goBack } from '@/nav'

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)
const firing = ref<Firing | null>(null)
const pieces = ref<Piece[]>([])
const editing = ref(false)
const draft = reactive({ fired_on: '', cone: '', schedule: '', notes: '' })
const suggestions = ref<Suggestions>({ ingredients: [], schedules: [], cones: [] })

onMounted(async () => {
  try {
    ;[firing.value, pieces.value] = await Promise.all([api.firing(id), api.pieces({ firing: id })])
  } catch (e) {
    sayError(e)
  }
})

const shelves = computed(() =>
  LOADING_ORDER.map((level) => ({ level, label: LEVELS[level], pieces: pieces.value.filter((p) => p.level === level && p.x !== null) })).filter((s) => s.pieces.length),
)
const unplaced = computed(() => pieces.value.filter((p) => p.level === null || p.x === null))

function startEdit() {
  if (!firing.value) return
  const { fired_on, cone, schedule, notes } = firing.value
  Object.assign(draft, { fired_on: fired_on ?? '', cone, schedule, notes })
  editing.value = true
  void api.suggestions().then((s) => (suggestions.value = s))
}

async function saveEdit() {
  try {
    const { fired_on, cone, schedule, notes } = draft
    firing.value = await api.saveFiring(id, { fired_on: fired_on || null, cone, schedule, notes })
    editing.value = false
  } catch (e) {
    sayError(e)
  }
}

async function remove() {
  try {
    await api.deleteFiring(id)
    goBack(router, '/kiln/past')
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink fallback="/kiln/past" />
      <button v-if="firing && !editing" type="button" class="btn btn-ghost" @click="startEdit">Edit details</button>
    </header>

    <template v-if="firing">
      <h1>{{ firingLabel(firing) }}</h1>
      <p v-if="firing.notes">{{ firing.notes }}</p>

      <form v-if="editing" class="card" @submit.prevent="saveEdit">
        <label class="field">Schedule <input v-model="draft.schedule" class="input" list="schedules" maxlength="200" /></label>
        <datalist id="schedules"><option v-for="s in suggestions.schedules" :key="s" :value="s"></option></datalist>
        <label class="field">Cone <input v-model="draft.cone" class="input" list="cones" maxlength="20" /></label>
        <datalist id="cones"><option v-for="c in suggestions.cones" :key="c" :value="c"></option></datalist>
        <label class="field">Date <input v-model="draft.fired_on" class="input" type="date" /></label>
        <label class="field">Notes <textarea v-model="draft.notes" class="input" maxlength="2000"></textarea></label>
        <button type="submit" class="btn btn-primary btn-block">Save</button>
      </form>

      <RouterLink v-if="firing.waiting" :to="`/firings/${id}/results`" class="btn btn-primary btn-lg btn-block">Enter results ({{ firing.waiting }})</RouterLink>
      <KilnShelf v-for="s in shelves" :key="s.level" :label="s.label" :pieces="s.pieces" @piece="router.push(`/pieces/${$event.id}`)" />
      <section v-if="unplaced.length" class="rows">
        <h2>Position not recorded</h2>
        <PieceRow v-for="p in unplaced" :key="p.id" :piece="p" />
      </section>
      <ConfirmButton v-if="!firing.pieces" class="btn-block" label="Delete this firing" @confirmed="remove" />
    </template>
  </main>
</template>
