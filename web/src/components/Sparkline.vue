<script setup lang="ts">
// A tiny SVG line with gaps where values are null (device off, sensor gap).
import { computed } from 'vue'

const props = withDefaults(defineProps<{ values: (number | null)[]; height?: number; label?: string }>(), {
  height: 36,
  label: 'Humidity over the last 6 hours',
})

const paths = computed(() => {
  const vals = props.values
  const nums = vals.filter((v): v is number => v !== null)
  if (nums.length < 2) return [] as string[]
  let lo = Math.min(...nums)
  let hi = Math.max(...nums)
  if (hi - lo < 2) {
    const mid = (hi + lo) / 2
    lo = mid - 1
    hi = mid + 1
  }
  const n = vals.length
  const out: string[] = []
  let cur = ''
  vals.forEach((v, i) => {
    if (v === null) {
      if (cur) out.push(cur)
      cur = ''
      return
    }
    const x = (i / Math.max(1, n - 1)) * 100
    const y = 2 + (1 - (v - lo) / (hi - lo)) * 26
    cur += `${cur ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`
  })
  if (cur) out.push(cur)
  return out
})
</script>

<template>
  <svg class="spark" viewBox="0 0 100 30" preserveAspectRatio="none" :style="{ height: height + 'px' }" role="img" :aria-label="label">
    <path v-for="(d, i) in paths" :key="i" :d="d" vector-effect="non-scaling-stroke" />
  </svg>
</template>

<style scoped>
.spark {
  width: 100%;
  display: block;
}
path {
  fill: none;
  stroke: var(--accent);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}
</style>
