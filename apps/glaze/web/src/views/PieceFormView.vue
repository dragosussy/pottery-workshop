<script setup lang="ts">
// Logging a piece, in four variations: new, editing, "do this again" (?from=)
// and re-firing (?refire=). A new piece that has already been fired, such as
// an old tile, gets a rating straight away and "Save and add another".
// ?back=1 returns where it came from.
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Choice from '@ui/Choice.vue'
import { ApiError } from '@ui/http'
import { say, sayError } from '@ui/messages'
import { api, toLayerInput, type ClayBody, type Defect, type Level, type Material, type Piece, type PieceInput, type PieceKind, type Rating } from '@/api'
import BackLink from '@/components/BackLink.vue'
import LayerEditor, { type LayerDraft } from '@/components/LayerEditor.vue'
import OptionalChoice from '@/components/OptionalChoice.vue'
import PhotoPicker from '@/components/PhotoPicker.vue'
import { DEFECTS, KINDS, RATINGS, firingLabel, pieceTitle, positionText } from '@/format'
import { goBack } from '@/nav'

const LAST_CLAY = 'glaze.lastClay'

const route = useRoute()
const router = useRouter()
const editId = route.params.id ? Number(route.params.id) : null
const fromId = Number(route.query.from) || null
const refireId = Number(route.query.refire) || null
const fresh = !editId && !fromId && !refireId // a brand new record
const alreadyFired = ref(route.query.fired === '1')
const firedChoice = computed({
  get: () => (alreadyFired.value ? 'fired' : 'not'),
  set: (v) => (alreadyFired.value = v === 'fired'),
})

const f = reactive({
  kind: 'tile' as PieceKind,
  number: String(route.query.number ?? ''),
  clay_body_id: null as number | null,
  notes: '',
  from_memory: false,
  rating: null as Rating | null,
  result_notes: '',
  defects: [] as Defect[],
  firing_id: null as number | null,
  level: null as Level | null,
  x: null as number | null,
  y: null as number | null,
  refire_of: null as number | null,
})
const layers = ref<LayerDraft[]>([])
const clayBodies = ref<ClayBody[]>([])
const materials = ref<Material[]>([])
const source = ref<Piece | null>(null) // what this was started from
const existingPhotos = ref(0)
const photo = ref<Blob | null>(null)
const duplicateOf = ref<number | null>(null)
const busy = ref(false)
const loaded = ref(false)

const locked = computed(() => editId !== null || refireId !== null) // the kind can't change
const numberLocked = computed(() => refireId !== null || f.refire_of !== null)
const heading = computed(() =>
  editId ? 'Edit' : refireId ? 'Re-fire' : fromId ? 'Do this again' : 'Log a piece',
)
const activeClay = computed(() => clayBodies.value.filter((c) => !c.archived || c.id === f.clay_body_id))
const aim = computed(() => {
  const p = source.value
  if (!fromId || !p?.firing) return ''
  return [firingLabel({ ...p.firing, fired_on: null }).replace('Date unknown · ', ''), positionText(p)].filter(Boolean).join(' · ')
})
const hasNumber = computed(() => numberLocked.value || /^[1-9]\d*$/.test(f.number.trim()))
const canSave = computed(() => {
  if (f.kind === 'tile' && !hasNumber.value) return false
  if (f.kind === 'pot' && !photo.value && existingPhotos.value === 0) return false // a photo is how a pot is found
  if (alreadyFired.value && !f.rating) return false
  return true
})
const missingHint = computed(() => {
  if (canSave.value) return ''
  if (alreadyFired.value && !f.rating && (f.kind === 'pot' || hasNumber.value)) return 'Rate it to save it.'
  return f.kind === 'pot' ? "A pot needs a photo: it's how you'll find it again." : 'Enter the number written on the tile.'
})

const showResult = computed(() => alreadyFired.value || !!f.rating || source.value?.firing?.state === 'unloaded')

function rememberedClay(): number | null {
  try {
    return Number(localStorage.getItem(LAST_CLAY)) || null
  } catch {
    return null
  }
}

function rememberClay() {
  try {
    if (f.clay_body_id) localStorage.setItem(LAST_CLAY, String(f.clay_body_id))
  } catch {
    // private mode: nothing to remember
  }
}

function fromPiece(p: Piece): LayerDraft[] {
  return p.layers.map((l, i) => ({ ...toLayerInput(l), key: i + 1 }))
}

onMounted(async () => {
  try {
    ;[clayBodies.value, materials.value] = await Promise.all([api.clayBodies(), api.materials()])
    const id = editId ?? fromId ?? refireId
    if (id) {
      const p = await api.piece(id)
      source.value = p
      f.clay_body_id = p.clay_body_id
      layers.value = fromPiece(p)
      if (editId) {
        Object.assign(f, {
          kind: p.kind, number: p.number === null ? '' : String(p.number), notes: p.notes, from_memory: p.from_memory,
          rating: p.rating, result_notes: p.result_notes, defects: p.defects, firing_id: p.firing_id, level: p.level,
          x: p.x, y: p.y, refire_of: p.refire_of,
        })
        existingPhotos.value = p.photos.length
      } else if (refireId) {
        f.kind = p.kind
        f.number = p.number === null ? '' : String(p.number)
        f.refire_of = p.id
      } else {
        f.kind = p.kind
      }
    } else {
      const last = rememberedClay()
      if (last && clayBodies.value.some((c) => c.id === last && !c.archived)) f.clay_body_id = last
    }
  } catch (e) {
    sayError(e)
  }
  loaded.value = true
})

function toggleDefect(d: Defect) {
  f.defects = f.defects.includes(d) ? f.defects.filter((x) => x !== d) : [...f.defects, d]
}

function input(): PieceInput {
  return {
    kind: f.kind,
    number: f.kind === 'tile' ? Number(f.number.trim()) : null,
    clay_body_id: f.clay_body_id, firing_id: f.firing_id, level: f.level, x: f.x, y: f.y, refire_of: f.refire_of,
    notes: f.notes, from_memory: fresh ? alreadyFired.value : f.from_memory, rating: f.rating, result_notes: f.result_notes, defects: f.defects,
    layers: layers.value.map(({ key: _key, ...layer }) => layer),
  }
}

/** Saves, and returns the piece's id; null if it couldn't be saved. */
async function save(): Promise<number | null> {
  busy.value = true
  duplicateOf.value = null
  try {
    const saved = editId ? await api.savePiece(editId, input()) : await api.addPiece(input())
    rememberClay()
    if (photo.value) {
      try {
        await api.addPhoto(saved.id, alreadyFired.value ? 'after' : 'before', photo.value)
      } catch (err) {
        say(`Saved, but the photo didn't upload: ${(err as Error).message}`, true)
      }
    }
    return saved.id
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && f.kind === 'tile') {
      const [existing] = await api.pieces({ number: f.number }).catch(() => [])
      duplicateOf.value = existing?.id ?? null
    }
    sayError(e)
    return null
  } finally {
    busy.value = false
  }
}

async function submit() {
  const id = await save()
  if (!id) return
  if (route.query.back) goBack(router, '/kiln')
  else void router.replace(`/pieces/${id}`)
}

// The backlog is typed in one tile after another.
async function saveAndAnother() {
  const id = await save()
  if (!id) return
  say(f.kind === 'tile' ? `Tile ${f.number} added` : 'Pot added')
  Object.assign(f, { number: '', notes: '', rating: null, result_notes: '', defects: [] })
  layers.value = []
  photo.value = null
  window.scrollTo({ top: 0 })
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink :fallback="source ? `/pieces/${source.id}` : '/'" />
      <h1>{{ heading }}</h1>
    </header>

    <form v-if="loaded" class="card" @submit.prevent="submit">
      <p v-if="aim" class="notice small"><strong>To match:</strong> {{ aim }}</p>
      <p v-if="refireId && source" class="notice small">This keeps {{ pieceTitle(source) }}'s stack. Add what's going on for this firing.</p>

      <Choice v-if="!locked" v-model="f.kind" label="What is it?" :options="KINDS" />

      <div v-if="fresh">
        <Choice v-model="firedChoice" label="Has it been fired yet?" :options="{ not: 'Not yet', fired: 'Already fired' }" />
        <p v-if="alreadyFired" class="small muted hint">For old tiles and anything already out of the kiln. Rate it now and leave blank what you can't remember.</p>
      </div>

      <label v-if="f.kind === 'tile'" class="field">Number on the tile
        <input v-model="f.number" class="input num" type="text" inputmode="numeric" :disabled="numberLocked" />
      </label>
      <p v-if="duplicateOf" class="small bad">
        That number is taken. <RouterLink :to="`/pieces/${duplicateOf}`">Open that tile</RouterLink>
      </p>

      <PhotoPicker v-model="photo" :label="f.kind === 'pot' ? 'Take a photo of the pot' : 'Add a photo'" />

      <label class="field">Clay body
        <select v-model="f.clay_body_id" class="input">
          <option :value="null">Not sure</option>
          <option v-for="c in activeClay" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </label>
      <p v-if="!clayBodies.length" class="small muted">Add your clay bodies in <RouterLink to="/settings">Settings</RouterLink> first.</p>

      <LayerEditor v-model="layers" :materials="materials" :refire="refireId !== null" :blank="alreadyFired" @created="materials.unshift($event)" />

      <label class="field">Notes
        <textarea v-model="f.notes" class="input" maxlength="2000"></textarea>
      </label>

      <template v-if="showResult">
        <OptionalChoice v-model="f.rating" label="How did it come out?" :options="RATINGS" />
        <div class="field-group">
          <span class="section-title">Defects</span>
          <div class="chips">
            <button v-for="(text, d) in DEFECTS" :key="d" type="button" :aria-pressed="f.defects.includes(d)" @click="toggleDefect(d)">{{ text }}</button>
          </div>
        </div>
        <label class="field">Result notes
          <textarea v-model="f.result_notes" class="input" maxlength="2000"></textarea>
        </label>
      </template>

      <p v-if="missingHint" class="small muted">{{ missingHint }}</p>
      <button v-if="alreadyFired && fresh" type="button" class="btn btn-primary btn-block" :disabled="busy || !canSave" @click="saveAndAnother">Save and add another</button>
      <button type="submit" class="btn btn-block" :class="{ 'btn-primary': !(alreadyFired && fresh) }" :disabled="busy || !canSave">{{ busy ? 'Saving…' : 'Save' }}</button>
    </form>
  </main>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  font-weight: 600;
}
.hint {
  margin-top: var(--sp-2);
}
</style>
