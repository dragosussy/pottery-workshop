<script setup lang="ts">
// The humidity target slider: the DrynessSlider's gesture engine (fine
// scrubbing below the track, hold-to-repeat nudges) on a %RH scale, with a
// ring at the room's current humidity.
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { usePointerSlider } from '@/composables/usePointerSlider'

const props = withDefaults(
  defineProps<{
    min?: number
    max?: number
    /** Hollow ring at the room's humidity now. */
    roomValue?: number | null
    label?: string
  }>(),
  { min: 50, max: 98, roomValue: null, label: 'Humidity target' },
)

const model = defineModel<number>({ required: true })

const scale = computed(() => ({ min: props.min, max: props.max, step: 1 }))
const INSET = 18

const track = ref<HTMLElement | null>(null)
const uid = useId()
const current = ref(model.value)
watch(model, (v) => (current.value = v))

const slider = usePointerSlider({
  track,
  inset: INSET,
  // The composable reads the scale once; min and max don't change while open.
  scale: scale.value,
  getValue: () => current.value,
  setValue: (v) => {
    current.value = v
    model.value = v
  },
})
const { dragging, factor } = slider

onMounted(() => track.value?.addEventListener('touchmove', slider.onTouchmove, { passive: false }))
onBeforeUnmount(() => track.value?.removeEventListener('touchmove', slider.onTouchmove))

const fraction = (v: number) => (Math.min(props.max, Math.max(props.min, v)) - props.min) / (props.max - props.min)
const ticks = computed(() => {
  const out: { v: number; f: number; major: boolean }[] = []
  for (let v = Math.ceil(props.min / 5) * 5; v <= props.max; v += 5) out.push({ v, f: fraction(v), major: v % 10 === 0 })
  return out
})
const fineText = computed(() => (factor.value === 0.25 ? 'fine ¼×' : factor.value === 0.5 ? 'fine ½×' : ''))
const diff = computed(() => (props.roomValue === null ? null : Math.round(current.value - props.roomValue)))

function nudgeDown(dir: 1 | -1, e: PointerEvent) {
  if (e.pointerType === 'mouse' && e.button !== 0) return
  slider.startRepeat(dir)
}
function nudgeClick(dir: 1 | -1, e: MouseEvent) {
  if (e.detail === 0) slider.nudge(dir)
}
</script>

<template>
  <div class="hs" :class="{ 'is-dragging': dragging }" :style="{ '--f': fraction(current), '--inset': INSET + 'px' }">
    <div class="hs-readout-row">
      <button
        type="button"
        class="hs-nudge"
        :disabled="current <= min"
        aria-label="1% lower"
        :aria-controls="uid"
        data-testid="target-minus"
        @pointerdown="nudgeDown(-1, $event)"
        @pointerup="slider.stopRepeat"
        @pointerleave="slider.stopRepeat"
        @pointercancel="slider.stopRepeat"
        @click="nudgeClick(-1, $event)"
        @contextmenu.prevent
      >
        <span aria-hidden="true">−</span>
      </button>
      <div class="hs-readout">
        <div class="hs-value num" data-testid="target-value">{{ current }}<span class="unit">%</span></div>
        <div class="hs-sub">
          <template v-if="fineText">{{ fineText }}</template>
          <template v-else-if="diff === null">&nbsp;</template>
          <template v-else-if="diff === 0">same as the shelf now</template>
          <template v-else>{{ Math.abs(diff) }}% {{ diff > 0 ? 'above' : 'below' }} the shelf now</template>
        </div>
      </div>
      <button
        type="button"
        class="hs-nudge"
        :disabled="current >= max"
        aria-label="1% higher"
        :aria-controls="uid"
        data-testid="target-plus"
        @pointerdown="nudgeDown(1, $event)"
        @pointerup="slider.stopRepeat"
        @pointerleave="slider.stopRepeat"
        @pointercancel="slider.stopRepeat"
        @click="nudgeClick(1, $event)"
        @contextmenu.prevent
      >
        <span aria-hidden="true">+</span>
      </button>
    </div>

    <div
      :id="uid"
      ref="track"
      class="hs-track"
      role="slider"
      tabindex="0"
      :aria-label="label"
      :aria-valuemin="min"
      :aria-valuemax="max"
      :aria-valuenow="current"
      :aria-valuetext="`${current}%`"
      data-testid="target-track"
      @pointerdown="slider.onPointerdown"
      @pointermove="slider.onPointermove"
      @pointerup="slider.onPointerup"
      @pointercancel="slider.onPointercancel"
      @keydown="slider.onKeydown"
    >
      <div class="hs-bar"></div>
      <div v-for="t in ticks" :key="t.v" class="hs-tick" :class="{ major: t.major }" :style="{ '--p': t.f }"></div>
      <div v-if="roomValue !== null" class="hs-room" :style="{ '--p': fraction(roomValue) }" title="The shelf now"></div>
      <div class="hs-thumb"></div>
    </div>
    <div class="hs-labels" aria-hidden="true">
      <span v-for="t in ticks.filter((t) => t.major)" :key="t.v" :style="{ '--p': t.f }">{{ t.v }}</span>
    </div>
    <p class="hs-legend faint">
      <span class="ring" aria-hidden="true"></span> the shelf now · drag below the bar for finer steps
    </p>
  </div>
</template>

<style scoped>
.hs {
  --track-h: 56px;
  --bar-h: 12px;
  --thumb: 36px;
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.hs-readout-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.hs-readout {
  text-align: center;
  flex: 1;
}
.hs-value {
  font-size: var(--fs-3xl);
  font-weight: 800;
  line-height: 1;
}
.unit {
  font-size: var(--fs-xl);
  margin-left: 2px;
}
.hs-sub {
  font-size: var(--fs-sm);
  color: var(--text-muted);
  min-height: 1.4em;
}
.hs-nudge {
  width: var(--touch);
  height: var(--touch);
  flex: none;
  border-radius: 50%;
  border: 2px solid var(--border);
  background: var(--surface);
  color: var(--text);
  font-size: 1.6rem;
  font-weight: 700;
  touch-action: manipulation;
}
.hs-nudge:disabled {
  opacity: 0.4;
}
.hs-track {
  position: relative;
  height: var(--track-h);
  margin-top: var(--sp-2);
  touch-action: pan-y;
  cursor: pointer;
  border-radius: var(--radius-sm);
  outline-offset: 4px;
}
.hs-track > * {
  pointer-events: none;
}
.hs-bar {
  position: absolute;
  left: calc(var(--inset) - 6px);
  right: calc(var(--inset) - 6px);
  top: 50%;
  height: var(--bar-h);
  transform: translateY(-50%);
  border-radius: 999px;
  background: linear-gradient(to right, var(--dry-4), var(--dry-0));
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.25);
}
.hs-tick {
  position: absolute;
  top: calc(50% + var(--bar-h) / 2 + 2px);
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  width: 2px;
  height: 6px;
  margin-left: -1px;
  background: var(--text-faint);
}
.hs-tick.major {
  height: 10px;
  background: var(--text-muted);
}
.hs-room {
  position: absolute;
  top: 50%;
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  width: 18px;
  height: 18px;
  transform: translate(-50%, -50%);
  border-radius: 50%;
  border: 3px solid var(--text);
  opacity: 0.75;
}
.hs-thumb {
  position: absolute;
  top: 50%;
  left: calc(var(--inset) + var(--f) * (100% - 2 * var(--inset)));
  width: var(--thumb);
  height: var(--thumb);
  transform: translate(-50%, -50%);
  border-radius: 50%;
  background: var(--accent);
  border: 4px solid var(--surface);
  box-shadow: 0 0 0 2px var(--text), var(--shadow);
  transition: transform 80ms ease-out;
}
.is-dragging .hs-thumb {
  transform: translate(-50%, -50%) scale(1.12);
}
.hs-labels {
  position: relative;
  height: 1.4em;
  margin: 0 var(--inset);
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.hs-labels span {
  position: absolute;
  left: calc(var(--p) * 100%);
  transform: translateX(-50%);
}
.hs-legend {
  margin-top: var(--sp-2);
  font-size: var(--fs-sm);
  display: flex;
  align-items: center;
  gap: 6px;
}
.ring {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid var(--text);
  opacity: 0.75;
}
</style>
