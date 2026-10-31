<script setup lang="ts">
// uPlot behind a small wrapper (plan §2): time series with null gaps, shaded
// periods (e.g. "controller off") and vertical markers (calibrations).
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { formatTime, formatDate } from '@/utils/format'

export interface ChartSeries {
  label: string
  /** CSS colour or var(--token). */
  color: string
  values: (number | null)[]
  scale?: string
  unit?: string
  dash?: number[]
  points?: boolean
  width?: number
  spanGaps?: boolean
}
export interface ChartBand {
  from: number // unix seconds
  to: number
  label: string
}
export interface ChartMarker {
  at: number
  label: string
}

const props = withDefaults(
  defineProps<{
    t: number[]
    series: ChartSeries[]
    bands?: ChartBand[]
    markers?: ChartMarker[]
    height?: number
    tz: string
    label: string
    /** Fixed y range per scale, e.g. { dryness: [0, 5] } */
    ranges?: Record<string, [number, number]>
  }>(),
  { bands: () => [], markers: () => [], height: 220, ranges: () => ({}) },
)

const el = ref<HTMLDivElement | null>(null)
let chart: uPlot | null = null
let ro: ResizeObserver | null = null

function cssColor(c: string): string {
  const m = /^var\((--[\w-]+)\)$/.exec(c)
  if (!m || !el.value) return c
  return getComputedStyle(el.value).getPropertyValue(m[1]!).trim() || '#888'
}

function build() {
  if (!el.value) return
  chart?.destroy()
  const width = el.value.clientWidth || 320
  const text = cssColor('var(--text-muted)')
  const grid = cssColor('var(--border)')
  const bandFill = cssColor('var(--surface-3)')
  const markerColor = cssColor('var(--accent)')
  const scales: uPlot.Scales = { x: { time: true } }
  for (const s of props.series) {
    const key = s.scale ?? 'y'
    const r = props.ranges[key]
    scales[key] = r ? { range: () => r } : { auto: true }
  }
  const usedScales = [...new Set(props.series.map((s) => s.scale ?? 'y'))]
  const axes: uPlot.Axis[] = [
    {
      stroke: text,
      font: '12px system-ui, sans-serif',
      grid: { stroke: grid, width: 1 },
      ticks: { stroke: grid },
      values: (_u, splits) => splits.map((v) => {
        const ms = v * 1000
        const span = (props.t[props.t.length - 1] ?? 0) - (props.t[0] ?? 0)
        return span > 2 * 86400 ? formatDate(ms, props.tz) : formatTime(ms, props.tz)
      }),
    },
    ...usedScales.map((sc, i) => {
      const s = props.series.find((x) => (x.scale ?? 'y') === sc)!
      return {
        scale: sc,
        side: i === 0 ? 3 : 1,
        stroke: text,
        grid: { show: i === 0, stroke: grid, width: 1 },
        size: 60,
        font: '12px system-ui, sans-serif',
        values: (_u: uPlot, splits: number[]) => splits.map((v) => `${Math.round(v * 10) / 10}${s.unit ?? ''}`),
      } as uPlot.Axis
    }),
  ]
  const opts: uPlot.Options = {
    width,
    height: props.height,
    legend: { show: false },
    cursor: { drag: { x: false, y: false }, points: { size: 8 } },
    scales,
    axes,
    series: [
      {},
      ...props.series.map((s) => ({
        label: s.label,
        scale: s.scale ?? 'y',
        stroke: cssColor(s.color),
        width: s.width ?? 2,
        dash: s.dash,
        spanGaps: s.spanGaps ?? false,
        points: { show: s.points ?? false, size: 7, fill: cssColor(s.color) },
      })),
    ],
    hooks: {
      drawClear: [
        (u) => {
          const ctx = u.ctx
          ctx.save()
          ctx.fillStyle = bandFill
          for (const b of props.bands) {
            const x0 = u.valToPos(b.from, 'x', true)
            const x1 = u.valToPos(b.to, 'x', true)
            const l = Math.max(u.bbox.left, x0)
            const r = Math.min(u.bbox.left + u.bbox.width, x1)
            if (r <= l) continue
            ctx.fillRect(l, u.bbox.top, r - l, u.bbox.height)
            ctx.fillStyle = text
            ctx.font = `${12 * devicePixelRatio}px system-ui, sans-serif`
            ctx.fillText(b.label, l + 4 * devicePixelRatio, u.bbox.top + 14 * devicePixelRatio)
            ctx.fillStyle = bandFill
          }
          ctx.restore()
        },
      ],
      draw: [
        (u) => {
          const ctx = u.ctx
          ctx.save()
          ctx.strokeStyle = markerColor
          ctx.setLineDash([4 * devicePixelRatio, 4 * devicePixelRatio])
          for (const m of props.markers) {
            const x = u.valToPos(m.at, 'x', true)
            if (x < u.bbox.left || x > u.bbox.left + u.bbox.width) continue
            ctx.beginPath()
            ctx.moveTo(x, u.bbox.top)
            ctx.lineTo(x, u.bbox.top + u.bbox.height)
            ctx.stroke()
          }
          ctx.restore()
        },
      ],
    },
  }
  const data = [props.t, ...props.series.map((s) => s.values)] as uPlot.AlignedData
  chart = new uPlot(opts, data, el.value)
}

onMounted(() => {
  build()
  if (typeof ResizeObserver !== 'undefined' && el.value) {
    let w = el.value.clientWidth
    ro = new ResizeObserver(() => {
      if (!el.value || !chart) return
      if (el.value.clientWidth !== w) {
        w = el.value.clientWidth
        chart.setSize({ width: w, height: props.height })
      }
    })
    ro.observe(el.value)
  }
})
watch(() => [props.t, props.series, props.bands, props.markers], build, { deep: false })
onBeforeUnmount(() => {
  ro?.disconnect()
  chart?.destroy()
})
</script>

<template>
  <div ref="el" class="tchart" role="img" :aria-label="label"></div>
</template>

<style scoped>
.tchart {
  width: 100%;
  min-height: 120px;
}
</style>
