<script setup lang="ts">
// Dryness label with a clay swatch: never colour alone.
import { computed } from 'vue'
import { drynessColorVar, drynessLabel, inTrimBand } from '@/utils/dryness'

const props = withDefaults(defineProps<{ value: number | null; compact?: boolean }>(), { compact: false })
const label = computed(() => (props.value === null ? null : drynessLabel(props.value)))
</script>

<template>
  <span class="dchip" :class="{ compact }">
    <span class="swatch" :style="{ background: value === null ? 'var(--surface-3)' : drynessColorVar(value) }" aria-hidden="true"></span>
    <span v-if="label">{{ compact ? label.stageLabel : label.text }}<span v-if="value !== null && inTrimBand(value)" class="scissors"> ✂</span></span>
    <span v-else>not recorded</span>
  </span>
</template>

<style scoped>
.dchip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.swatch {
  width: 14px;
  height: 14px;
  border-radius: 4px;
  border: 1px solid rgba(0, 0, 0, 0.25);
  flex: none;
}
.scissors {
  color: var(--ok);
}
</style>
