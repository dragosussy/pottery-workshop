<script setup lang="ts">
// Where a piece is on its way through the studio.
import { computed } from 'vue'
import type { Piece } from '@/api'
import { JOURNEY } from '@/format'

const props = defineProps<{ state: Piece['state'] }>()
const current = computed(() => ({ bench: 0, kiln: 1, waiting: 2, done: 2 })[props.state])
</script>

<template>
  <ol class="journey">
    <li v-for="(step, i) in JOURNEY" :key="step" :class="{ past: i < current, now: i === current }" :aria-current="i === current ? 'step' : undefined">{{ step }}</li>
  </ol>
</template>

<style scoped>
.journey {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 3px;
}
li {
  padding: var(--sp-2) var(--sp-1);
  background: var(--surface-2);
  color: var(--text-muted);
  font-size: var(--fs-sm);
  font-weight: 600;
  text-align: center;
}
li:first-child {
  border-radius: var(--radius-sm) 0 0 var(--radius-sm);
}
li:last-child {
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}
li.past {
  background: var(--accent-soft);
  color: var(--accent);
}
li.now {
  background: var(--accent);
  color: var(--accent-text);
}
</style>
