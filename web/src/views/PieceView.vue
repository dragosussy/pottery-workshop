<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, type Piece, type PiecePatch } from '@/api'
import Choice from '@/components/Choice.vue'
import PreciseSlider from '@/components/PreciseSlider.vue'
import { TRIM_BAND, drynessWords } from '@/dryness'
import { FORMS, SIZES, STATES, THICKNESSES, endedText, estimateText, pieceTitle, specs, when } from '@/format'
import { bumpPhotos, live, markPiece, photoUrl, refreshShelf, say, sayError } from '@/live'

const route = useRoute()
const router = useRouter()
const id = computed(() => Number(route.params.id))
const piece = ref<Piece | null>(null)
const busy = ref(false)
const editing = ref(false)

// The slider starts at the last check; `touched` says whether the potter moved it.
const dryness = ref(0)
const touched = ref(false)
function show(p: Piece, keepSlider = false) {
  piece.value = p
  if (keepSlider) return
  dryness.value = p.last_check.dryness
  touched.value = false
}

watch(
  id,
  async (pid) => {
    piece.value = null
    editing.value = false
    // "Next piece →" walks the shelf in the order it had when the walk started.
    if (!live.walk?.includes(pid)) live.walk = live.shelf.some((p) => p.id === pid) ? live.shelf.map((p) => p.id) : null
    try {
      show(await api.piece(pid))
    } catch (e) {
      sayError(e)
    }
  },
  { immediate: true },
)

// An Undo from the message bar changed this piece: reload it.
watch(
  () => live.undone,
  async () => {
    try {
      show(await api.piece(id.value))
    } catch (e) {
      sayError(e)
    }
  },
)

const next = computed(() => {
  const order = live.walk ?? live.shelf.map((p) => p.id)
  const i = order.indexOf(id.value)
  return i >= 0 ? (order[i + 1] ?? null) : null
})
const ended = computed(() => piece.value?.state === 'trimmed' || piece.value?.state === 'discarded')
const checks = computed(() => [...(piece.value?.checks ?? [])].reverse())

async function write(fn: () => Promise<Piece>, done?: string, keepSlider = false) {
  busy.value = true
  try {
    show(await fn(), keepSlider)
    if (done) say(done)
    void refreshShelf()
  } catch (e) {
    sayError(e)
  } finally {
    busy.value = false
  }
}

// After a check the slider stays where it is (still "moved"), so a Trimmed
// straight after records that same value.
const saveCheck = () => write(() => api.addCheck(id.value, dryness.value), `Saved ${dryness.value.toFixed(1)}`, true)
const setState = (state: PiecePatch['state']) => write(() => api.patchPiece(id.value, { state }))
// Ending sends the slider only if it was moved: otherwise trim records 3.0
// (so the model never learns from its own estimate) and discard records
// nothing. The buttons show what they'll record.
function end(state: 'trimmed' | 'discarded') {
  const patch = touched.value ? { state, dryness: dryness.value } : { state }
  return write(() => markPiece(piece.value!, patch))
}
const mark = (state: 'flipped') => write(() => markPiece(piece.value!, { state }), undefined, true)

// Two taps to delete: the first arms the button for a few seconds.
const armed = ref('')
let armTimer: ReturnType<typeof setTimeout> | undefined
function confirm(key: string, action: () => unknown) {
  clearTimeout(armTimer)
  if (armed.value === key) {
    armed.value = ''
    return action()
  }
  armed.value = key
  armTimer = setTimeout(() => (armed.value = ''), 3000)
}

async function deletePiece() {
  busy.value = true
  try {
    await api.deletePiece(id.value)
    bumpPhotos() // the id may be reused by a new piece
    say(`Deleted #${piece.value?.number}`)
    void refreshShelf()
    void router.push('/')
  } catch (e) {
    sayError(e)
  } finally {
    busy.value = false
  }
}

const draft = reactive({ label: '', notes: '', form: 'bowl', size: 'medium', thickness: 'medium' } as Pick<Piece, 'label' | 'notes' | 'form' | 'size' | 'thickness'>)
function startEdit() {
  const p = piece.value!
  Object.assign(draft, { label: p.label, notes: p.notes, form: p.form, size: p.size, thickness: p.thickness })
  editing.value = true
}
async function saveEdit() {
  await write(() => api.patchPiece(id.value, { ...draft, label: draft.label.trim() }))
  editing.value = false
}

async function changePhoto(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  busy.value = true
  try {
    await api.putPhoto(id.value, file)
    bumpPhotos()
    show(await api.piece(id.value))
  } catch (err) {
    sayError(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="page">
    <header class="bar sticky">
      <RouterLink to="/" class="btn btn-ghost">← Shelf</RouterLink>
      <RouterLink v-if="next" :to="`/pieces/${next}`" class="btn">Next piece →</RouterLink>
    </header>

    <p v-if="!piece" class="muted">Loading…</p>
    <template v-else>
      <section class="card">
        <img v-if="piece.photo" :src="photoUrl(piece.id)" alt="" class="photo" />
        <h1>{{ pieceTitle(piece) }}</h1>
        <p class="muted">{{ specs(piece) }} · {{ ended ? endedText(piece) : STATES[piece.state] }} · added {{ when(piece.added_at) }}</p>
        <p>
          <strong>{{ ended ? 'Last check' : 'Now about' }}:</strong> {{ drynessWords(piece.dryness) }}
          <span class="muted num">{{ piece.dryness.toFixed(1) }}</span>
        </p>
        <p v-if="piece.estimate">
          <strong>{{ estimateText(piece.estimate) }}</strong>
          <span v-if="live.now?.learning" class="muted small"> (rough guess)</span>
        </p>
        <p v-if="piece.notes" class="notes">{{ piece.notes }}</p>
      </section>

      <section v-if="!ended" class="card events">
        <h2>How dry is it now?</h2>
        <PreciseSlider
          v-model="dryness" v-model:touched="touched" :min="0" :max="5" :step="0.1" :band="TRIM_BAND"
          :last="piece.last_check.dryness" :words="drynessWords" :label="`Dryness of #${piece.number}`"
        />
        <p class="muted small">Ring: last check, {{ piece.last_check.dryness.toFixed(1) }} on {{ when(piece.last_check.at) }}</p>
        <button type="button" class="btn btn-primary btn-block" :disabled="busy" @click="saveCheck">Save check</button>
        <div class="event-buttons">
          <button v-if="piece.state === 'in'" type="button" class="btn btn-lg event" :disabled="busy" @click="mark('flipped')">
            <span class="icon" aria-hidden="true">↻</span> Flipped
          </button>
          <button v-else type="button" class="btn btn-lg event" :disabled="busy" @click="setState('in')">
            <span class="icon" aria-hidden="true">↺</span> Undo flip
          </button>
          <button type="button" class="btn btn-lg event" :disabled="busy" @click="end('trimmed')">
            <span class="icon" aria-hidden="true">✂</span> Trimmed
            <small class="num">at {{ (touched ? dryness : 3).toFixed(1) }}</small>
          </button>
          <button type="button" class="btn btn-lg event" :disabled="busy" @click="end('discarded')">
            <span class="icon" aria-hidden="true">✕</span> Discarded
            <small v-if="touched" class="num">at {{ dryness.toFixed(1) }}</small>
          </button>
        </div>
        <p v-if="piece.flipped_at" class="muted small">Flipped {{ when(piece.flipped_at) }}</p>
      </section>
      <section v-else class="card events">
        <p><strong>{{ endedText(piece) }}</strong></p>
        <button type="button" class="btn btn-lg btn-block" :disabled="busy" @click="setState('in')">Put back on the shelf</button>
      </section>

      <section class="card">
        <h2>Checks</h2>
        <ul class="checks">
          <li v-for="c in checks" :key="c.id">
            <span class="num">{{ c.dryness.toFixed(1) }}</span>
            <span class="grow">{{ drynessWords(c.dryness) }}<br /><span class="muted small">{{ when(c.at) }}</span></span>
            <button
              type="button" class="btn btn-small" :class="{ 'btn-danger': armed === 'c' + c.id }"
              :disabled="busy || checks.length < 2" :aria-label="`Delete check ${c.dryness.toFixed(1)}`"
              @click="confirm('c' + c.id, () => write(() => api.deleteCheck(c.id)))"
            >{{ armed === 'c' + c.id ? 'Delete?' : '✕' }}</button>
          </li>
        </ul>
      </section>

      <section class="card">
        <template v-if="editing">
          <label class="field">Label <input v-model="draft.label" class="input" maxlength="100" /></label>
          <label class="field">Notes <textarea v-model="draft.notes" class="input" maxlength="2000"></textarea></label>
          <Choice v-model="draft.form" label="Form" :options="FORMS" />
          <Choice v-model="draft.size" label="Size" :options="SIZES" />
          <Choice v-model="draft.thickness" label="Walls" :options="THICKNESSES" />
          <div class="row">
            <button type="button" class="btn btn-primary" :disabled="busy" @click="saveEdit">Save</button>
            <button type="button" class="btn btn-ghost" @click="editing = false">Cancel</button>
          </div>
        </template>
        <div v-else class="row">
          <button type="button" class="btn" @click="startEdit">Edit details</button>
          <label class="btn">
            {{ piece.photo ? 'Change photo' : 'Add photo' }}
            <input type="file" accept="image/*" hidden @change="changePhoto" />
          </label>
          <button type="button" class="btn btn-danger" :disabled="busy" @click="confirm('piece', deletePiece)">
            {{ armed === 'piece' ? 'Really delete?' : 'Delete piece' }}
          </button>
        </div>
      </section>
    </template>
  </main>
</template>

<style scoped>
.sticky {
  position: sticky;
  top: env(safe-area-inset-top);
  z-index: 2;
  background: var(--bg);
  margin: calc(-1 * var(--sp-2)) 0;
  padding: var(--sp-2) 0;
}
.event-buttons {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--sp-2);
}
.event {
  flex-direction: column;
  gap: 2px;
  padding-inline: var(--sp-1);
}
.event .icon {
  font-size: 1.4rem;
  line-height: 1;
}
.event small {
  font-size: var(--fs-sm);
  font-weight: 600;
  opacity: 0.75;
}
.events p {
  margin: 0;
}
.photo {
  border-radius: var(--radius-sm);
  max-height: 50vh;
  object-fit: cover;
}
.notes {
  white-space: pre-wrap;
}
.checks {
  list-style: none;
  margin: 0;
  padding: 0;
}
.checks li {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-2) 0;
  border-bottom: 1px solid var(--surface-3);
}
.checks .num {
  font-weight: 700;
  width: 2.5em;
}
.grow {
  flex: 1;
}
</style>
