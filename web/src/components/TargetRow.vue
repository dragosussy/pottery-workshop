<script setup lang="ts">
// Under the climate card: the humidity target and how far the shelf is from
// it. Tapping opens the target sheet.
import { computed } from 'vue'
import { useLiveStore } from '@/stores/live'
import { useSheet } from '@/composables/useSheet'
import { pumpShort } from '@/utils/pump'

/** How far off the target the shelf may sit before the row says so. */
const TOLERANCE_PCT = 3

const live = useLiveStore()
const sheet = useSheet()

const target = computed(() => live.status?.control.target_humidity_pct ?? null)
const room = computed(() => live.status?.room.humidity_pct ?? null)
const off = computed(() => {
  if (target.value === null || room.value === null || live.offline) return null
  const d = room.value - target.value
  if (Math.abs(d) <= TOLERANCE_PCT) return null
  return `${Math.round(Math.abs(d))}% ${d > 0 ? 'above' : 'below'}`
})
const pump = computed(() => pumpShort(live.status?.control))
</script>

<template>
  <button v-if="live.status" type="button" class="card target" data-testid="target-row" @click="sheet.open('target')">
    <span class="label">Target</span>
    <span v-if="target === null" class="value muted">none · predictions follow the last day</span>
    <span v-else class="value">
      <strong class="num">{{ target }}%</strong>&nbsp;<span v-if="off" class="off" data-testid="target-off">· shelf {{ off }}</span><span v-else class="ok">· on target</span><span v-if="pump" class="pump" data-testid="target-pump"> · {{ pump }}</span>
    </span>
    <span class="chev" aria-hidden="true">›</span>
  </button>
</template>

<style scoped>
.target {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  width: 100%;
  min-height: var(--touch);
  border: 0;
  text-align: left;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.label {
  font-size: var(--fs-sm);
  color: var(--text-muted);
  font-weight: 600;
}
.value {
  flex: 1;
}
.off {
  color: var(--warn, var(--danger));
  font-weight: 600;
}
.ok,
.pump {
  color: var(--text-muted);
}
.chev {
  font-size: var(--fs-xl);
  color: var(--text-faint);
}
</style>
