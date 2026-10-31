<script setup lang="ts">
// Piece detail (plan §4.5): photo, specs, lifecycle line (tap a time to
// correct it), prediction card, dryness history (edit/delete with Undo),
// actions. Pieces that left the shelf stay correctable and can go back.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import type { Observation, Piece } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/client'
import PiecePhoto from '@/components/PiecePhoto.vue'
import DrynessChip from '@/components/DrynessChip.vue'
import EtaWindow from '@/components/EtaWindow.vue'
import Age from '@/components/Age.vue'
import PieceChart from '@/components/PieceChart.vue'
import { useSheet } from '@/composables/useSheet'
import { useNow } from '@/composables/useServerClock'
import { showToast } from '@/composables/useToast'
import { stateBeforeEnd, transitionErrorMessage, usePieceActions } from '@/composables/usePieceActions'
import { useLiveStore } from '@/stores/live'
import { useReferenceStore } from '@/stores/reference'
import { useSettingsStore } from '@/stores/settings'
import { useShelfStore } from '@/stores/shelf'
import { confidenceWord, dayTime, pieceSpecs } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const sheet = useSheet()
const now = useNow()
const live = useLiveStore()
const reference = useReferenceStore()
const settings = useSettingsStore()
const shelf = useShelfStore()
const actions = usePieceActions()

const id = computed(() => Number(route.params.id))
const piece = ref<Piece | null>(null)
const observations = ref<Observation[]>([])
const error = ref('')
const notFound = ref(false)

async function load() {
  error.value = ''
  try {
    const [p, obs] = await Promise.all([api.piece(id.value), api.pieceObservations(id.value)])
    piece.value = p
    observations.value = obs.items
    notFound.value = false
  } catch (e) {
    if (isApiError(e) && e.status === 404) notFound.value = true
    else {
      // Offline: fall back to the cached shelf.
      piece.value = shelf.byId.get(id.value) ?? piece.value
      error.value = errorMessage(e)
    }
  }
}
watch(id, load, { immediate: true })
// Refetch after any change (sheets closing, other devices).
const off = live.onChange((_s, ids) => {
  if (!ids.length || ids.includes(id.value)) void load()
})
onBeforeUnmount(off)
watch(() => route.query.sheet, (s, prev) => {
  if (prev && !s) void load()
})

const onShelf = computed(() => piece.value?.state === 'in' || piece.value?.state === 'flipped')
const pred = computed(() => piece.value?.prediction ?? null)
const reasons = computed(() => (pred.value?.uncertainty_reasons ?? []).map((r) => reference.label('uncertainty_reasons', r)))
const tz = computed(() => settings.tz)
const lifecycle = computed(() => {
  const p = piece.value
  if (!p) return []
  const steps: { label: string; at: string | null; field: string }[] = [{ label: 'Added', at: p.added_at, field: 'added_at' }]
  if (p.flipped_at) steps.push({ label: 'Flipped', at: p.flipped_at, field: 'flipped_at' })
  if (p.ended_at) steps.push({ label: p.state === 'trimmed' ? 'Trimmed' : 'Discarded', at: p.ended_at, field: 'ended_at' })
  return steps
})
const SOURCE: Record<Observation['source'], string> = { add: 'when added', user: 'check', flip: 'at flip', end: 'at the end' }

async function deleteObs(o: Observation) {
  try {
    await api.deleteObservation(o.id)
    observations.value = observations.value.filter((x) => x.id !== o.id)
    void shelf.refresh()
    showToast({
      message: 'Reading deleted',
      action: {
        label: 'Undo',
        run: async () => {
          try {
            await api.undeleteObservation(o.id)
            void load()
            void shelf.refresh()
          } catch (e) {
            showToast({ message: `Couldn't undo: ${errorMessage(e)}`, tone: 'error' })
          }
        },
      },
    })
  } catch (e) {
    showToast({ message: isApiError(e) && e.code === 'last_observation' ? "Can't delete the only reading." : errorMessage(e), tone: 'error' })
  }
}

const backBusy = ref(false)
async function backOnShelf() {
  const p = piece.value
  if (!p) return
  backBusy.value = true
  try {
    const [updated] = await actions.transition([p], { state: stateBeforeEnd(p) })
    if (updated) piece.value = updated
    void load()
  } catch (e) {
    showToast({ message: transitionErrorMessage(e), tone: 'error' })
  } finally {
    backBusy.value = false
  }
}

function back() {
  if (window.history.state?.back) router.back()
  else void router.push(onShelf.value ? '/shelf' : '/history')
}
</script>

<template>
  <main class="page">
    <header class="page-header">
      <button type="button" class="btn btn-ghost" aria-label="Back" @click="back"><span aria-hidden="true">←</span></button>
      <h1 class="page-title">{{ piece ? `#${piece.number}` : 'Piece' }}<span v-if="piece?.label" class="muted"> · {{ piece.label }}</span></h1>
      <button v-if="piece" type="button" class="btn btn-ghost" @click="sheet.open('edit', { ids: [piece.id] })">Edit</button>
      <span v-else style="width: 48px"></span>
    </header>

    <p v-if="notFound" class="muted">This piece doesn't exist (or was entered by mistake). <RouterLink to="/shelf">Back to the shelf</RouterLink></p>
    <p v-if="error && !piece" class="error">{{ error }}</p>

    <template v-if="piece">
      <PiecePhoto :photo-id="piece.photo_id" :form="piece.form" size="full" :alt="`Photo of #${piece.number}`" />
      <section class="card stack">
        <div class="specs">{{ pieceSpecs(piece, reference.data) }}</div>
        <div class="row-wrap">
          <DrynessChip :value="piece.current_dryness" />
          <span class="muted">· <Age :since="piece.added_at" :until="piece.ended_at" /> {{ onShelf ? 'on the shelf' : 'on the shelf in total' }}</span>
        </div>
        <ol class="lifecycle" aria-label="Lifecycle">
          <li v-for="(s, i) in lifecycle" :key="s.field">
            <span class="lc-label"><span v-if="i > 0" aria-hidden="true">→ </span>{{ s.label }}</span>
            <button type="button" class="lc-time" :aria-label="`${s.label} ${dayTime(s.at, now, tz)}: correct the time`" @click="sheet.open('edit', { ids: [piece.id], focus: s.field })">
              {{ dayTime(s.at, now, tz) }} <span aria-hidden="true">✎</span>
            </button>
          </li>
        </ol>
        <p v-if="piece.end_note" class="muted">“{{ piece.end_note }}”</p>
        <p v-if="piece.notes" class="muted">{{ piece.notes }}</p>
      </section>

      <section v-if="onShelf && pred" class="card stack" :class="{ 'offline-dim': live.offline }" aria-labelledby="pred-h">
        <h2 id="pred-h" class="h2">Prediction</h2>
        <p class="pred-main"><EtaWindow :prediction="pred" /></p>
        <dl class="kv">
          <template v-if="pred.ready_until">
            <dt>Best before</dt>
            <dd>{{ dayTime(pred.ready_until, now, tz) }}</dd>
          </template>
          <dt>Confidence</dt>
          <dd>{{ confidenceWord(pred.confidence) }}</dd>
          <template v-if="reasons.length">
            <dt>Wider because</dt>
            <dd>{{ reasons[0] }}<span v-if="reasons.length > 1" class="faint"> (also {{ reasons.slice(1).join(', ') }})</span></dd>
          </template>
          <dt>Based on</dt>
          <dd>{{ pred.basis.summary }}</dd>
        </dl>
      </section>

      <section class="card stack" aria-labelledby="hist-h">
        <h2 id="hist-h" class="h2">Dryness history</h2>
        <PieceChart v-if="observations.length > 1" :observations="observations" :added-at="piece.added_at" :ended-at="piece.ended_at" />
        <ul class="obs">
          <li v-for="o in [...observations].reverse()" :key="o.id" class="obs-row" data-testid="obs-row">
            <div class="obs-main">
              <DrynessChip :value="o.dryness" />
              <span class="faint small">{{ dayTime(o.observed_at, now, tz) }} · {{ SOURCE[o.source] }}<span v-if="o.flag === 'inconsistent'"> · ⚠ wetter than before</span></span>
            </div>
            <div v-if="o.source === 'user'" class="obs-actions">
              <button type="button" class="btn btn-ghost" :aria-label="`Correct the reading from ${dayTime(o.observed_at, now, tz)}`" @click="sheet.open('observe', { ids: [piece.id], obs: o.id })">✎</button>
              <button type="button" class="btn btn-ghost" :aria-label="`Delete the reading from ${dayTime(o.observed_at, now, tz)}`" @click="deleteObs(o)">🗑</button>
            </div>
          </li>
        </ul>
      </section>

      <div v-if="onShelf" class="actions">
        <button type="button" class="btn btn-primary btn-lg btn-block" @click="sheet.open('observe', { ids: [piece.id] })">Record dryness</button>
        <div class="row3">
          <button v-if="piece.state === 'in'" type="button" class="btn" @click="sheet.open('flip', { ids: [piece.id] })">Flip</button>
          <button type="button" class="btn" @click="sheet.open('trim', { ids: [piece.id] })">Trim</button>
          <button type="button" class="btn" @click="sheet.open('discard', { ids: [piece.id] })">Discard</button>
        </div>
      </div>
      <div v-else class="actions">
        <button type="button" class="btn btn-block" :disabled="backBusy" @click="backOnShelf">↩ Back on the shelf</button>
      </div>
    </template>
  </main>
</template>

<style scoped>
.h2 {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.specs {
  font-size: var(--fs-lg);
  font-weight: 600;
}
.lifecycle {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1) var(--sp-3);
}
.lifecycle li {
  display: flex;
  flex-direction: column;
}
.lc-label {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  font-weight: 700;
  text-transform: uppercase;
}
.lc-time {
  min-height: var(--touch);
  padding: 0;
  border: 0;
  background: none;
  font-weight: 600;
  text-align: left;
  text-decoration: underline dotted;
  cursor: pointer;
}
.pred-main {
  font-size: var(--fs-xl);
  font-weight: 800;
}
.kv {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-1) var(--sp-3);
  margin: 0;
}
.kv dt {
  color: var(--text-muted);
}
.kv dd {
  margin: 0;
}
.obs {
  list-style: none;
  margin: 0;
  padding: 0;
}
.obs-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
  padding: var(--sp-2) 0;
  border-bottom: 1px solid var(--border);
}
.obs-row:last-child {
  border-bottom: 0;
}
.obs-main {
  display: flex;
  flex-direction: column;
}
.small {
  font-size: var(--fs-sm);
}
.obs-actions {
  display: flex;
}
.actions {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.row3 {
  display: flex;
  gap: var(--sp-2);
}
.row3 .btn {
  flex: 1;
}
.error {
  color: var(--danger);
}
</style>
