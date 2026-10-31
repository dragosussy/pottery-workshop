<script setup lang="ts">
// "8 pieces · oldest 3d 4h", a stacked bar by stage (with a text legend, never
// colour alone) and the first three "check these".
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { StageKey } from '@/api/types'
import { useLiveStore } from '@/stores/live'
import { useShelfStore } from '@/stores/shelf'
import { useReferenceStore } from '@/stores/reference'
import Age from './Age.vue'
import PieceRow from './PieceRow.vue'

const live = useLiveStore()
const shelf = useShelfStore()
const reference = useReferenceStore()

const summary = computed(() => live.status?.shelf ?? null)
const stages = computed(() => {
  const by = summary.value?.by_stage
  if (!by) return []
  return reference.data.stages
    .map((s, i) => ({ key: s.key as StageKey, label: s.label, count: by[s.key] ?? 0, color: `var(--dry-${i})` }))
    .filter((s) => s.count > 0)
})
const total = computed(() => stages.value.reduce((a, s) => a + s.count, 0))
const checkFirst = computed(() =>
  (shelf.shelf?.check_these ?? [])
    .slice(0, 3)
    .map((c) => ({ piece: shelf.byId.get(c.piece_id), reasons: c.reasons }))
    .filter((x) => x.piece),
)
</script>

<template>
  <section v-if="summary && summary.on_shelf > 0" class="card stack" aria-labelledby="sum-h" data-testid="shelf-summary">
    <h2 id="sum-h" class="sum-title">
      <RouterLink to="/shelf" class="plain"><strong>{{ summary.on_shelf }} {{ summary.on_shelf === 1 ? 'piece' : 'pieces' }}</strong></RouterLink>
      <span v-if="summary.oldest_added_at" class="muted"> · oldest <Age :since="summary.oldest_added_at" /></span>
    </h2>
    <div class="bar" role="img" :aria-label="stages.map((s) => `${s.count} ${s.label}`).join(', ')">
      <span v-for="s in stages" :key="s.key" class="seg" :style="{ flexGrow: s.count, background: s.color }"></span>
    </div>
    <ul class="legend" aria-hidden="true">
      <li v-for="s in stages" :key="s.key"><span class="sw" :style="{ background: s.color }"></span>{{ s.count }} {{ s.label.toLowerCase() }}</li>
    </ul>
    <p v-if="total !== summary.on_shelf" class="faint small">Counts by the nearest stage to the last reading.</p>
    <template v-if="checkFirst.length">
      <h3 class="check-h">Check these</h3>
      <PieceRow v-for="c in checkFirst" :key="c.piece!.id" :piece="c.piece!" :reasons="c.reasons" class="bordered" />
    </template>
  </section>
</template>

<style scoped>
.sum-title {
  font-size: var(--fs-lg);
  font-weight: 400;
}
.plain {
  color: inherit;
  text-decoration: none;
}
.bar {
  display: flex;
  height: 16px;
  border-radius: 8px;
  overflow: hidden;
  gap: 2px;
  background: var(--surface-2);
}
.seg {
  flex-basis: 0;
}
.legend {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1) var(--sp-3);
  font-size: var(--fs-sm);
}
.legend li {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.sw {
  width: 12px;
  height: 12px;
  border-radius: 3px;
  border: 1px solid rgba(0, 0, 0, 0.2);
}
.small {
  font-size: var(--fs-xs);
}
.check-h {
  font-size: var(--fs-sm);
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--warn-strong);
}
.bordered {
  border: 1px solid var(--border);
}
</style>
