<script setup lang="ts">
// Shelf (plan §4.4): grouped by status; Select (a button) for group actions.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Piece } from '@/api/types'
import PieceRow from '@/components/PieceRow.vue'
import { useShelfStore, SHELF_GROUPS, shelfGroupOf } from '@/stores/shelf'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { useReferenceStore } from '@/stores/reference'
import { useNow } from '@/composables/useServerClock'
import { useSheet } from '@/composables/useSheet'
import { formatDate, optionLabel } from '@/utils/format'
import { chrome } from '@/composables/useChrome'

const shelf = useShelfStore()
const live = useLiveStore()
const settings = useSettingsStore()
const reference = useReferenceStore()
const sheet = useSheet()
const router = useRouter()
const now = useNow()

onMounted(() => void shelf.refresh())

const groups = computed(() => {
  const by = new Map<string, Piece[]>()
  for (const p of shelf.pieces) {
    const g = shelfGroupOf(p, shelf.checkIds, now.value)
    by.set(g, [...(by.get(g) ?? []), p])
  }
  return SHELF_GROUPS.map((g) => ({ ...g, items: by.get(g.key) ?? [] })).filter((g) => g.items.length)
})

const selecting = ref(false)
const selected = ref<Set<number>>(new Set())
watch(selecting, (v) => (chrome.hideFab = v))
onBeforeUnmount(() => (chrome.hideFab = false))
function toggle(id: number) {
  const s = new Set(selected.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selected.value = s
}
function stopSelecting() {
  selecting.value = false
  selected.value = new Set()
}

// "Select group": pieces added together ("Tue · 4 mugs").
const addGroups = computed(() => {
  const by = new Map<number, Piece[]>()
  for (const p of shelf.pieces) if (p.group_id !== null) by.set(p.group_id, [...(by.get(p.group_id) ?? []), p])
  return [...by.values()]
    .filter((ps) => ps.length > 1)
    .map((ps) => {
      const p = ps[0]!
      const day = formatDate(Date.parse(p.added_at), settings.tz)
      const form = optionLabel(reference.data.forms, p.form).toLowerCase()
      return { ids: ps.map((x) => x.id), label: `${day} · ${ps.length} ${form}s` }
    })
})
function selectGroup(ids: number[]) {
  selected.value = new Set([...selected.value, ...ids])
}

const ids = computed(() => [...selected.value])
function act(kind: 'check' | 'flip' | 'trim' | 'discard') {
  const list = ids.value
  if (!list.length) return
  stopSelecting()
  if (kind === 'check') void router.push({ path: '/check', query: { ids: list.join(',') } })
  else void sheet.open(kind, { ids: list })
}
</script>

<template>
  <main class="page" :class="{ 'with-actions': selecting }">
    <header class="page-header">
      <h1 class="page-title">Shelf <span class="muted count">· {{ shelf.pieces.length }}</span></h1>
      <button v-if="shelf.pieces.length" type="button" class="btn" :aria-pressed="selecting" @click="selecting ? stopSelecting() : (selecting = true)">
        {{ selecting ? 'Done' : 'Select' }}
      </button>
    </header>

    <div v-if="selecting && addGroups.length" class="row-wrap" aria-label="Select a group">
      <button v-for="g in addGroups" :key="g.label" type="button" class="btn" @click="selectGroup(g.ids)">+ {{ g.label }}</button>
    </div>

    <p v-if="!shelf.pieces.length && (shelf.loaded || shelf.fromCache)" class="empty">The shelf is empty.</p>
    <p v-else-if="!shelf.pieces.length" class="muted">Loading…</p>

    <div :class="{ 'offline-dim': live.offline }" class="stack">
      <section v-for="g in groups" :key="g.key" class="stack" :aria-labelledby="`g-${g.key}`">
        <h2 :id="`g-${g.key}`" class="group-title">{{ g.title }} <span class="muted">{{ g.items.length }}</span></h2>
        <PieceRow
          v-for="p in g.items"
          :key="p.id"
          :piece="p"
          :selectable="selecting"
          :selected="selected.has(p.id)"
          :reasons="g.key === 'check' ? (shelf.checkReasons.get(p.id) ?? []) : []"
          @toggle="toggle"
        />
      </section>
    </div>

    <div v-if="selecting" class="actionbar" role="toolbar" aria-label="Actions for the selection">
      <span class="sel num">{{ ids.length }} selected</span>
      <button type="button" class="btn btn-primary" :disabled="!ids.length" @click="act('check')">Check</button>
      <button type="button" class="btn" :disabled="!ids.length" @click="act('flip')">Flip</button>
      <button type="button" class="btn" :disabled="!ids.length" @click="act('trim')">Trim</button>
      <button type="button" class="btn" :disabled="!ids.length" @click="act('discard')">Discard</button>
    </div>
  </main>
</template>

<style scoped>
.count {
  font-weight: 600;
}
.group-title {
  font-size: var(--fs-md);
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin-top: var(--sp-2);
}
.empty {
  padding: var(--sp-6) 0;
  text-align: center;
  font-size: var(--fs-lg);
  color: var(--text-muted);
}
.actionbar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: calc(var(--bottom-bar-h) + env(safe-area-inset-bottom));
  z-index: 45;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-2) var(--gutter);
  background: var(--surface);
  border-top: 1px solid var(--border);
  box-shadow: var(--shadow-lg);
  overflow-x: auto;
}
.actionbar .btn {
  flex: 1;
  padding: 0 var(--sp-2);
}
.sel {
  font-size: var(--fs-sm);
  font-weight: 700;
  white-space: nowrap;
}
</style>
