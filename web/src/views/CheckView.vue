<script setup lang="ts">
// Check-in (plan §4.3): one piece per screen, saving per piece. The slider
// starts at the LAST OBSERVED value (untouched: "same as last time?") and the
// model's estimate is never shown here. No swipe navigation: horizontal
// gestures belong to the slider; buttons move between pieces.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FollowUp, FollowUpOption, Piece } from '@/api/types'
import DrynessSlider from '@/components/DrynessSlider.vue'
import PiecePhoto from '@/components/PiecePhoto.vue'
import FollowUpPrompt from '@/components/FollowUpPrompt.vue'
import Age from '@/components/Age.vue'
import TrimCard from '@/components/TrimCard.vue'
import { parseIds, useSheet } from '@/composables/useSheet'
import { useObservationSave } from '@/composables/useObservationSave'
import { useNow } from '@/composables/useServerClock'
import { useShelfStore } from '@/stores/shelf'
import { useReferenceStore } from '@/stores/reference'
import { drynessText } from '@/utils/dryness'
import { formatAge } from '@/utils/duration'
import { numberList, pieceSpecs } from '@/utils/format'
import { parseTs } from '@/utils/time'

const route = useRoute()
const router = useRouter()
const sheet = useSheet()
const shelf = useShelfStore()
const reference = useReferenceStore()
const { save } = useObservationSave()
const now = useNow()

type Outcome = 'checked' | 'skipped' | 'left'
const order = ref<number[]>([])
/** Pieces as they were when check-in started (fallback if one leaves the shelf). */
const startSnapshot = new Map<number, Piece>()
const index = ref(0)
const outcomes = ref(new Map<number, Outcome>())
const ready = ref(false)

const dryness = ref(2)
const touched = ref(false)
const saving = ref(false)
const error = ref('')
const unconfirmed = ref(false)
const followUp = ref<FollowUp | null>(null)
/** Move on once the sheet opened from this card closes. */
let advanceAfterSheet = false

onMounted(async () => {
  await shelf.ensure()
  const limit = parseIds(route.query.ids)
  const list = shelf.checkOrder(limit.length ? limit : undefined)
  list.forEach((p) => startSnapshot.set(p.id, p))
  order.value = list.map((p) => p.id)
  ready.value = true
})

const total = computed(() => order.value.length)
const done = computed(() => ready.value && index.value >= total.value)
const currentId = computed(() => order.value[index.value] ?? null)
const piece = computed<Piece | null>(() => (currentId.value === null ? null : (shelf.byId.get(currentId.value) ?? startSnapshot.get(currentId.value) ?? null)))
const lastValue = computed(() => piece.value?.current_dryness ?? piece.value?.initial_dryness ?? 1)

// A new piece on screen: pre-fill with the last observed value, untouched.
watch(currentId, () => {
  dryness.value = lastValue.value
  touched.value = false
  error.value = ''
  unconfirmed.value = false
  followUp.value = null
}, { immediate: true })
watch(ready, () => (dryness.value = lastValue.value))

const lastAgo = computed(() => {
  const t = parseTs(piece.value?.last_observed_at)
  return t === null ? '' : `${formatAge((now.value - t) / 1000)} ago`
})
const reasons = computed(() =>
  currentId.value === null ? [] : (shelf.checkReasons.get(currentId.value) ?? []).map((r) => reference.label('check_reasons', r)),
)

function next(outcome: Outcome) {
  if (currentId.value !== null) outcomes.value.set(currentId.value, outcome)
  index.value++
  // Skip pieces that left the shelf meanwhile (trimmed on another phone).
  while (index.value < total.value) {
    const id = order.value[index.value]!
    const p = shelf.byId.get(id)
    if (shelf.loaded && !p) {
      outcomes.value.set(id, 'left')
      index.value++
      continue
    }
    break
  }
}

function skip() {
  next('skipped')
}

async function saveAndNext() {
  const p = piece.value
  if (!p) return
  saving.value = true
  error.value = ''
  unconfirmed.value = false
  // "Unchanged" is a real reading: the value is sent even if the slider wasn't moved.
  const res = await save({ items: [{ piece_id: p.id, dryness: dryness.value }] })
  saving.value = false
  if (!res.ok) {
    error.value = res.message
    unconfirmed.value = res.unconfirmed
    return
  }
  touched.value = true
  const fu = res.data.follow_ups[0]
  if (fu) followUp.value = fu
  else next('checked')
}

function onAnswered(o: FollowUpOption) {
  const fu = followUp.value
  followUp.value = null
  if (fu && o.action === 'correct_observation' && fu.previous_observation_id) {
    advanceAfterSheet = true
    void sheet.open('observe', { ids: [fu.piece_id], obs: fu.previous_observation_id })
    return
  }
  if (fu && o.action === 'end_piece') {
    advanceAfterSheet = true
    void sheet.open('trim', { ids: [fu.piece_id] })
    return
  }
  next('checked')
}
function onSkipped() {
  followUp.value = null
  next('checked')
}

function openSheet(kind: 'flip' | 'trim' | 'discard') {
  const p = piece.value
  if (!p) return
  // Flip pre-fills its dryness from the slider's current value.
  void sheet.open(kind, { ids: [p.id], ...(kind === 'flip' ? { dryness: dryness.value } : {}) })
}

// When a trim/discard (or a correction) sheet closes, move on if appropriate.
watch(
  () => route.query.sheet,
  (s, prev) => {
    if (!prev || s) return
    const p = currentId.value === null ? null : shelf.byId.get(currentId.value)
    if (advanceAfterSheet) {
      advanceAfterSheet = false
      next('checked')
    } else if (currentId.value !== null && !p) next('checked') // trimmed/discarded from this card
  },
)
// A piece trimmed from this card disappears from the shelf: move on.
watch(
  () => (currentId.value === null ? true : shelf.byId.has(currentId.value)),
  (present) => {
    if (!present && !route.query.sheet && !done.value && !advanceAfterSheet) next('left')
  },
)

const counts = computed(() => {
  let checked = 0
  let skipped = 0
  let left = 0
  outcomes.value.forEach((o) => (o === 'checked' ? checked++ : o === 'skipped' ? skipped++ : left++))
  return { checked, skipped, left }
})
const readyPieces = computed(() => shelf.pieces.filter((p) => p.prediction?.status === 'ready_now' && order.value.includes(p.id)))
watch(done, (d) => {
  if (d) void shelf.refresh()
})

function exit() {
  if (window.history.state?.back) router.back()
  else void router.push('/')
}
</script>

<template>
  <main class="check">
    <header class="check-header">
      <span v-if="!done && total" class="progress num" aria-live="polite">{{ Math.min(index + 1, total) }} of {{ total }}</span>
      <span v-else class="progress">Check-in</span>
      <button type="button" class="btn btn-ghost close" aria-label="End check-in" @click="exit"><span aria-hidden="true">✕</span></button>
    </header>

    <p v-if="!ready" class="muted pad">Loading…</p>
    <div v-else-if="!total" class="pad stack">
      <p class="big">Nothing on the shelf to check.</p>
      <button type="button" class="btn btn-primary btn-lg" @click="exit">Back</button>
    </div>

    <section v-else-if="!done && piece" :key="piece.id" class="card-piece" data-testid="check-card">
      <PiecePhoto :photo-id="piece.photo_id" :form="piece.form" size="full" :alt="`Photo of #${piece.number}`" />
      <div class="info">
        <h1 class="title"><span class="num">#{{ piece.number }}</span> <span class="specs">{{ pieceSpecs(piece, reference.data) }}</span></h1>
        <p class="muted">in <Age :since="piece.added_at" /><template v-if="piece.state === 'flipped'"> · flipped</template><template v-if="piece.label"> · {{ piece.label }}</template></p>
        <p class="last">last: {{ drynessText(piece.current_dryness) }} <span class="faint">({{ lastAgo }})</span></p>
        <p v-if="reasons.length" class="reason">{{ reasons.join(' · ') }}</p>
      </div>

      <template v-if="!followUp">
        <DrynessSlider
          v-model="dryness"
          v-model:touched="touched"
          :last-value="lastValue"
          :label="`How dry is #${piece.number} now?`"
        />
        <p class="hint">Judge the foot</p>

        <div class="row3">
          <button v-if="piece.state === 'in'" type="button" class="btn" @click="openSheet('flip')">Flip</button>
          <button type="button" class="btn" @click="openSheet('trim')">Trim</button>
          <button type="button" class="btn" @click="openSheet('discard')">Discard</button>
        </div>

        <p v-if="error" class="error" role="alert" data-testid="check-error">{{ error }}</p>
      </template>
      <FollowUpPrompt v-else :follow-up="followUp" @answered="onAnswered" @skipped="onSkipped" />

      <div v-if="!followUp" class="nav">
        <button type="button" class="btn btn-lg" :disabled="saving" data-testid="skip" @click="skip">Skip</button>
        <button type="button" class="btn btn-primary btn-lg" :disabled="saving" data-testid="save-next" @click="saveAndNext">
          {{ saving ? 'Saving…' : unconfirmed ? 'Save again' : 'Save & next' }}
        </button>
      </div>
    </section>

    <section v-else-if="done" class="pad stack" data-testid="check-done">
      <h1 class="big">Checked {{ counts.checked }} · skipped {{ counts.skipped }}</h1>
      <p v-if="counts.left" class="muted">{{ counts.left }} left the shelf during check-in.</p>
      <div v-if="readyPieces.length" class="ready stack">
        <p class="big2">{{ numberList(readyPieces.map((p) => p.number), 'and') }} {{ readyPieces.length === 1 ? 'is' : 'are' }} ready: trim {{ readyPieces.length === 1 ? 'it' : 'them' }} now?</p>
        <button type="button" class="btn btn-primary btn-lg" @click="sheet.open('trim', { ids: readyPieces.map((p) => p.id) })">Trim {{ readyPieces.length === 1 ? 'it' : 'them' }}</button>
      </div>
      <TrimCard />
      <button type="button" class="btn btn-lg" @click="router.push('/')">Done</button>
    </section>
  </main>
</template>

<style scoped>
.check {
  max-width: 640px;
  margin: 0 auto;
  padding: env(safe-area-inset-top) 0 calc(env(safe-area-inset-bottom) + var(--sp-4));
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
}
.check-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--sp-2) var(--gutter);
}
.progress {
  font-weight: 800;
  font-size: var(--fs-lg);
}
.close {
  font-size: var(--fs-xl);
}
.pad {
  padding: var(--sp-4) var(--gutter);
}
.card-piece {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  padding: 0 var(--gutter);
}
.card-piece :deep(.photo.full) {
  aspect-ratio: 16 / 9;
  max-height: 28dvh;
}
.title {
  font-size: var(--fs-xl);
  font-weight: 800;
}
.specs {
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--text-muted);
}
.last {
  font-weight: 600;
}
.reason {
  color: var(--warn-strong);
  font-weight: 700;
  font-size: var(--fs-sm);
}
.hint {
  text-align: center;
  font-style: italic;
  color: var(--text-muted);
}
.row3 {
  display: flex;
  gap: var(--sp-2);
}
.row3 .btn {
  flex: 1;
}
.nav {
  margin-top: auto;
  display: grid;
  grid-template-columns: 1fr 2fr;
  gap: var(--sp-2);
  padding-top: var(--sp-2);
}
.error {
  color: var(--danger);
  font-weight: 600;
}
.big {
  font-size: var(--fs-xl);
  font-weight: 800;
}
.big2 {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.ready {
  padding: var(--sp-3);
  border-radius: var(--radius);
  background: var(--ok-soft);
}
</style>
