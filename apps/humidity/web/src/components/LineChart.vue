<script setup lang="ts">
// One series over the last `hours`, hand-drawn in SVG. The line breaks where
// buckets are missing (more than 2 × step apart). Touch or hover shows the
// value at that time.
import { computed, ref } from 'vue'
import { clockTime } from '@/format'

const props = defineProps<{
  title: string
  t: number[] // unix seconds, oldest first
  v: number[]
  step: number
  hours: number
  unit: string
  /** Seconds since epoch at the right edge. */
  end: number
  /** An optional dashed reference line (the target). */
  line?: number | null
}>()

const W = 1000
const H = 100
const start = computed(() => props.end - props.hours * 3600)

const yRange = computed(() => {
  const vals = props.line != null ? [...props.v, props.line] : props.v
  if (!vals.length) return { lo: 0, hi: 1, ticks: [] as number[] }
  let lo = Math.min(...vals)
  let hi = Math.max(...vals)
  if (hi - lo < 2) [lo, hi] = [lo - 1, hi + 1]
  const tick = [1, 2, 5, 10, 20].find((s) => (hi - lo) / s <= 4) ?? 50
  lo = Math.floor(lo / tick) * tick
  hi = Math.ceil(hi / tick) * tick
  const ticks: number[] = []
  for (let y = lo; y <= hi + 1e-9; y += tick) ticks.push(y)
  return { lo, hi, ticks }
})

const xOf = (t: number) => ((t - start.value) / (props.end - start.value)) * W
const yOf = (v: number) => H - ((v - yRange.value.lo) / (yRange.value.hi - yRange.value.lo)) * H

const path = computed(() => {
  let d = ''
  props.t.forEach((t, i) => {
    const gap = i === 0 || t - props.t[i - 1]! > 2 * props.step
    d += `${gap ? 'M' : 'L'}${xOf(t).toFixed(1)} ${yOf(props.v[i]!).toFixed(1)}`
  })
  return d
})

// Local-time gridlines: every 15 min for an hour, every 6 h for a day,
// midnights for a week.
const xTicks = computed(() => {
  if (props.hours <= 1) {
    const quarter = 15 * 60
    const out: { x: number; label: string }[] = []
    for (let t = Math.ceil(start.value / quarter) * quarter; t < props.end; t += quarter) {
      out.push({ x: xOf(t) / W, label: clockTime(new Date(t * 1000)) })
    }
    return out
  }
  const week = props.hours > 48
  const d = new Date(start.value * 1000)
  if (week) d.setHours(24, 0, 0, 0)
  else d.setHours(d.getHours() + 1, 0, 0, 0)
  const out: { x: number; label: string }[] = []
  while (d.getTime() / 1000 < props.end) {
    if (week) out.push({ x: xOf(d.getTime() / 1000) / W, label: d.toLocaleDateString('en-GB', { weekday: 'short' }) })
    else if (d.getHours() % 6 === 0) out.push({ x: xOf(d.getTime() / 1000) / W, label: clockTime(d) })
    if (week) d.setDate(d.getDate() + 1)
    else d.setHours(d.getHours() + 1)
  }
  return out
})

const hover = ref<number | null>(null)
function onPointer(e: PointerEvent) {
  if (!props.t.length) return
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const t = start.value + ((e.clientX - r.left) / r.width) * (props.end - start.value)
  let best = 0
  props.t.forEach((ti, i) => {
    if (Math.abs(ti - t) < Math.abs(props.t[best]! - t)) best = i
  })
  hover.value = best
}
const shown = computed(() => {
  const i = hover.value ?? props.t.length - 1
  if (i < 0 || props.v[i] === undefined) return ''
  const at = hover.value === null ? 'latest' : clockTime(new Date(props.t[i]! * 1000))
  return `${props.v[i]!.toFixed(1)} ${props.unit} · ${at}`
})
</script>

<template>
  <figure class="chart">
    <figcaption class="row small">
      <strong>{{ title }}</strong>
      <span class="muted num">{{ shown }}</span>
    </figcaption>
    <div class="plot" @pointermove="onPointer" @pointerdown="onPointer" @pointerleave="hover = null">
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" role="img" :aria-label="`${title} chart`">
        <line v-for="y in yRange.ticks" :key="'y' + y" class="grid" x1="0" :x2="W" :y1="yOf(y)" :y2="yOf(y)" />
        <line v-for="x in xTicks" :key="'x' + x.x" class="grid" :x1="x.x * W" :x2="x.x * W" y1="0" :y2="H" />
        <line v-if="line != null" class="target" x1="0" :x2="W" :y1="yOf(line)" :y2="yOf(line)" />
        <path :d="path" class="series" />
        <line v-if="hover !== null" class="cursor" :x1="xOf(t[hover]!)" :x2="xOf(t[hover]!)" y1="0" :y2="H" />
      </svg>
      <span v-for="y in yRange.ticks" :key="'yl' + y" class="ylabel num" :style="{ top: (yOf(y) / H) * 100 + '%' }">{{ y }}</span>
      <p v-if="!t.length" class="empty muted small">No readings in this period</p>
    </div>
    <div class="xlabels small muted">
      <span v-for="x in xTicks" :key="'xl' + x.x" :style="{ left: x.x * 100 + '%' }">{{ x.label }}</span>
    </div>
  </figure>
</template>

<style scoped>
.chart {
  margin: 0;
  --series: var(--accent);
}
.plot {
  position: relative;
  height: 110px;
  margin: var(--sp-2) 0 0 28px;
  touch-action: pan-y;
}
svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}
svg * {
  vector-effect: non-scaling-stroke;
  fill: none;
}
.grid {
  stroke: var(--surface-3);
  stroke-width: 1;
}
.target {
  stroke: var(--text-muted);
  stroke-width: 1.5;
  stroke-dasharray: 6 4;
}
.series {
  stroke: var(--series);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.cursor {
  stroke: var(--text-faint);
  stroke-width: 1;
}
.ylabel {
  position: absolute;
  left: -28px;
  width: 24px;
  text-align: right;
  transform: translateY(-50%);
  font-size: 0.75rem;
  color: var(--text-faint);
}
.empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
}
.xlabels {
  position: relative;
  height: 1.4em;
  margin-left: 28px;
}
.xlabels span {
  position: absolute;
  transform: translateX(-50%);
  white-space: nowrap;
  font-size: 0.75rem;
}
</style>
