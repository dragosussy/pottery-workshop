<script setup lang="ts">
// History (plan §4.7): ended pieces, newest first, paginated, with
// Trimmed/Discarded filters (client-side: the API pages both together).
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import type { Piece } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import PiecePhoto from '@/components/PiecePhoto.vue'
import Chips from '@/components/Chips.vue'
import { useReferenceStore } from '@/stores/reference'
import { useSettingsStore } from '@/stores/settings'
import { formatAge } from '@/utils/duration'
import { drynessText } from '@/utils/dryness'
import { formatDate, optionLabel } from '@/utils/format'

const reference = useReferenceStore()
const settings = useSettingsStore()
const items = ref<Piece[]>([])
const cursor = ref<string | null>(null)
const done = ref(false)
const loading = ref(false)
const error = ref('')
type Filter = 'all' | 'trimmed' | 'discarded'
const filter = ref<Filter>('all')
const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'trimmed', label: 'Trimmed' },
  { key: 'discarded', label: 'Discarded' },
]

async function more() {
  if (loading.value || done.value) return
  loading.value = true
  error.value = ''
  try {
    const page = await api.piecesEnded({ limit: 20, cursor: cursor.value })
    items.value = [...items.value, ...page.items]
    cursor.value = page.next_cursor
    done.value = page.next_cursor === null
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(more)

const shown = computed(() => items.value.filter((p) => filter.value === 'all' || p.state === filter.value))
function summary(p: Piece): string {
  const span = p.ended_at ? formatAge((Date.parse(p.ended_at) - Date.parse(p.added_at)) / 1000) : ''
  const verb = p.state === 'trimmed' ? 'trimmed' : 'discarded'
  const at = p.end_dryness !== null ? ` at ${drynessText(p.end_dryness)}` : ''
  return `${verb} after ${span}${at}`
}
</script>

<template>
  <main class="page">
    <header class="page-header">
      <h1 class="page-title">History</h1>
    </header>
    <Chips v-model="filter" :options="FILTERS" label="Show" hide-label />
    <ul class="list">
      <li v-for="p in shown" :key="p.id">
        <RouterLink :to="`/pieces/${p.id}`" class="hrow" data-testid="history-row">
          <PiecePhoto :photo-id="p.photo_id" :form="p.form" :size="56" />
          <span class="body">
            <strong>#{{ p.number }} · {{ p.ended_at ? formatDate(Date.parse(p.ended_at), settings.tz) : '' }}</strong>
            <span class="muted">{{ optionLabel(reference.data.forms, p.form) }}<template v-if="p.label"> · {{ p.label }}</template></span>
            <span class="small">{{ summary(p) }}</span>
            <span v-if="p.end_note" class="small faint">“{{ p.end_note }}”</span>
          </span>
        </RouterLink>
      </li>
    </ul>
    <p v-if="!loading && !shown.length && !error" class="muted empty">Nothing here yet.</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <button v-if="!done && !error" type="button" class="btn btn-block" :disabled="loading" @click="more">{{ loading ? 'Loading…' : 'Load more' }}</button>
    <button v-if="error" type="button" class="btn btn-block" @click="more">Try again</button>
  </main>
</template>

<style scoped>
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.hrow {
  display: flex;
  gap: var(--sp-3);
  align-items: center;
  padding: var(--sp-2) var(--sp-3);
  min-height: 72px;
  border-radius: var(--radius);
  background: var(--surface);
  color: inherit;
  text-decoration: none;
}
.body {
  display: flex;
  flex-direction: column;
}
.small {
  font-size: var(--fs-sm);
}
.empty {
  text-align: center;
  padding: var(--sp-5);
}
.error {
  color: var(--danger);
}
</style>
