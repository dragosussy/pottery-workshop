<script setup lang="ts">
// The one control that gets care: the potter's feel is the
// measurement. Drag moves the value relative to where it was (no jump when
// grabbing the thumb), a tap jumps, holding the finger below the track while
// dragging gives ½× / ¼× steps, ± buttons repeat while held, arrows work.
// The readout sits above the track so a finger never covers it.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { GRAB_RADIUS, clamp, classify, fineFactor, fraction, keyValue, snap, valueAt, type Scale } from './slider'

const props = withDefaults(
  defineProps<{
    min: number
    max: number
    step: number
    label: string
    /** Readout in words; without it the number (plus unit) is the readout. */
    words?: ((v: number) => string) | null
    unit?: string
    /** A ring at the last recorded value. */
    last?: number | null
    /** A marked band, e.g. the trim band [2.8, 3.6]. */
    band?: [number, number] | null
    /** Says so under the readout while the value is in the band. */
    bandLabel?: string
    /** A CSS background for the track, when the scale has its own colours. */
    gradient?: string
    disabled?: boolean
  }>(),
  { words: null, unit: '', last: null, band: null, bandLabel: '', gradient: undefined, disabled: false },
)
const model = defineModel<number>({ required: true })
/** Set true by any change the user makes; the parent resets it. */
const touched = defineModel<boolean>('touched', { default: false })

const INSET = 18 // thumb radius: min and max sit this far inside the track
const scale = computed<Scale>(() => ({ min: props.min, max: props.max, step: props.step }))
const decimals = computed(() => Math.max(0, -Math.floor(Math.log10(props.step))))
const track = ref<HTMLElement | null>(null)

// The newest value: pointer events and repeats build on this, not on a prop
// that only catches up after the parent re-renders.
const current = ref(model.value)
watch(model, (v) => (current.value = v))

function set(v: number) {
  current.value = v
  model.value = v
  touched.value = true
}

const pct = (v: number) => fraction(v, scale.value)
const number = computed(() => current.value.toFixed(decimals.value))
const inBand = computed(() => !!props.band && current.value >= props.band[0] - 1e-9 && current.value <= props.band[1] + 1e-9)
const readout = computed(() => (props.words ? props.words(current.value) : number.value + props.unit))

// ---- Pointer gesture on the track ----
let pointer: number | null = null
let phase: 'pending' | 'drag' = 'pending'
let startX = 0
let startY = 0
let lastX = 0
let raw = 0 // unsnapped value, so slow fine moves add up
let grabbed = false
const dragging = ref(false)
const factor = ref(1)

function geometry() {
  const r = track.value!.getBoundingClientRect()
  return { left: r.left + INSET, width: Math.max(1, r.width - 2 * INSET), centerY: r.top + r.height / 2 }
}
const range = () => props.max - props.min

function onDown(e: PointerEvent) {
  if (props.disabled || pointer !== null || (e.pointerType === 'mouse' && e.button !== 0)) return
  pointer = e.pointerId
  phase = 'pending'
  startX = lastX = e.clientX
  startY = e.clientY
  raw = current.value
  const g = geometry()
  grabbed = Math.abs(e.clientX - (g.left + pct(current.value) * g.width)) <= GRAB_RADIUS
  if (e.pointerType === 'mouse') {
    // A mouse can't scroll by dragging: own the gesture now so a release
    // outside the track still ends it.
    e.preventDefault()
    track.value?.setPointerCapture(e.pointerId)
    track.value?.focus({ preventScroll: true })
  }
}

function onMove(e: PointerEvent) {
  if (e.pointerId !== pointer) return
  const g = geometry()
  if (phase === 'pending') {
    const c = classify(e.clientX - startX, e.clientY - startY)
    if (c === 'scroll' && e.pointerType !== 'mouse') return end()
    if (c !== 'drag') return
    phase = 'drag'
    dragging.value = true
    track.value?.setPointerCapture(e.pointerId)
    // Grabbed the thumb: carry on from its value. Elsewhere: start under the finger.
    raw = grabbed ? current.value + ((e.clientX - startX) / g.width) * range() : valueAt(e.clientX, g.left, g.width, scale.value)
  } else {
    factor.value = fineFactor(e.clientY - g.centerY)
    raw += ((e.clientX - lastX) / g.width) * range() * factor.value
  }
  raw = clamp(raw, props.min, props.max)
  lastX = e.clientX
  const v = snap(raw, scale.value)
  if (v !== current.value) set(v)
}

function onUp(e: PointerEvent) {
  if (e.pointerId !== pointer) return
  // A tap (no drag) away from the thumb jumps there.
  if (phase === 'pending' && !grabbed) {
    const g = geometry()
    set(snap(valueAt(e.clientX, g.left, g.width, scale.value), scale.value))
  }
  end()
}

function end() {
  pointer = null
  dragging.value = false
  factor.value = 1
}

// Once the slider owns a touch, stop the browser turning it into a scroll,
// so moving the finger down for fine steps keeps working.
function onTouchMove(e: TouchEvent) {
  const t = e.touches[0]
  if (pointer === null || !e.cancelable || !t) return
  if (phase === 'drag' || Math.abs(t.clientX - startX) > Math.abs(t.clientY - startY)) e.preventDefault()
}
onMounted(() => track.value?.addEventListener('touchmove', onTouchMove, { passive: false }))
onBeforeUnmount(() => track.value?.removeEventListener('touchmove', onTouchMove))

function onKey(e: KeyboardEvent) {
  if (props.disabled) return
  const v = keyValue(e.key, current.value, scale.value)
  if (v === null) return
  e.preventDefault()
  set(v)
}

// ---- ± buttons: one step now, then repeat while held (faster after 10) ----
let timer: ReturnType<typeof setTimeout> | undefined
function nudge(dir: number): boolean {
  const v = snap(current.value + dir * props.step, scale.value)
  if (v === current.value) return false
  set(v)
  return true
}
function holdStart(dir: number, e: PointerEvent) {
  if (props.disabled || (e.pointerType === 'mouse' && e.button !== 0)) return
  holdStop()
  if (!nudge(dir)) return
  let count = 0
  const repeat = () => {
    if (!nudge(dir)) return holdStop()
    timer = setTimeout(repeat, ++count > 10 ? 45 : 90)
  }
  timer = setTimeout(repeat, 400)
}
function holdStop() {
  clearTimeout(timer)
}
onBeforeUnmount(holdStop)
// Keyboard activation of a ± button (pointer presses were handled on pointerdown).
function nudgeClick(dir: number, e: MouseEvent) {
  if (e.detail === 0 && !props.disabled) nudge(dir)
}
</script>

<template>
  <div class="ps" :class="{ dragging, disabled }" :style="{ '--f': pct(current), '--inset': INSET + 'px' }">
    <div class="ps-readout">
      <button
        v-for="dir in [-1, 1]" :key="dir" type="button" class="ps-nudge" :style="{ order: dir }"
        :disabled="disabled || (dir < 0 ? current <= min : current >= max)"
        :aria-label="(dir < 0 ? 'Less' : 'More') + ` (${dir < 0 ? '−' : '+'}${step})`"
        @pointerdown="holdStart(dir, $event)" @pointerup="holdStop" @pointerleave="holdStop" @pointercancel="holdStop"
        @click="nudgeClick(dir, $event)" @contextmenu.prevent
      >{{ dir < 0 ? '−' : '+' }}</button>
      <div class="ps-text" aria-hidden="true">
        <div class="ps-main">{{ readout }}</div>
        <div class="ps-sub muted">
          <span v-if="words" class="num">{{ number }}</span>
          <span v-if="inBand && bandLabel" class="ps-band-word">{{ bandLabel }}</span>
          <span v-if="factor < 1" class="ps-fine">fine {{ factor === 0.5 ? '½' : '¼' }}×</span>
        </div>
      </div>
    </div>
    <div
      ref="track" class="ps-track" role="slider" :tabindex="disabled ? -1 : 0" :aria-label="label"
      :aria-valuemin="min" :aria-valuemax="max" :aria-valuenow="current"
      :aria-valuetext="readout + (words ? ` (${number})` : '')" :aria-disabled="disabled || undefined"
      @pointerdown="onDown" @pointermove="onMove" @pointerup="onUp" @pointercancel="end" @keydown="onKey"
    >
      <div v-if="band" class="ps-band" :style="{ '--b0': pct(band[0]), '--b1': pct(band[1]) }"></div>
      <div class="ps-bar" :style="gradient ? { background: gradient } : undefined"></div>
      <div v-if="last !== null" class="ps-last" :style="{ '--p': pct(last) }" title="Last recorded"></div>
      <div class="ps-thumb" :class="{ untouched: !touched }"></div>
    </div>
  </div>
</template>

<style scoped>
.ps {
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.ps-readout {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.ps-text {
  flex: 1;
  text-align: center;
  min-height: 64px;
}
.ps-main {
  font-size: var(--fs-xl);
  font-weight: 800;
  line-height: 1.2;
}
.ps-sub {
  display: flex;
  justify-content: center;
  gap: var(--sp-3);
  min-height: 1.4em;
}
.ps-band-word {
  color: var(--ok);
  font-weight: 700;
}
.ps-fine {
  font-weight: 700;
  color: var(--accent);
}
.ps-nudge {
  width: 60px;
  height: 60px;
  flex: none;
  border-radius: 50%;
  border: 2px solid var(--border);
  background: var(--surface-2);
  font-size: 2rem;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  touch-action: manipulation;
}
.ps-nudge:active:not(:disabled) { background: var(--surface-3); }
.ps-nudge:disabled { opacity: 0.4; }
/* 64px touch area; pan-y lets a vertical swipe scroll the page. */
.ps-track {
  position: relative;
  height: 64px;
  touch-action: pan-y;
  cursor: pointer;
  border-radius: var(--radius-sm);
}
.disabled .ps-track {
  opacity: 0.5;
  cursor: default;
}
.ps-track > * {
  position: absolute;
  pointer-events: none;
}
.ps-bar {
  left: calc(var(--inset) - 6px);
  right: calc(var(--inset) - 6px);
  top: calc(50% - 6px);
  height: 12px;
  border-radius: 999px;
  background: var(--surface-3);
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.2);
}
.ps-band {
  top: 8px;
  bottom: 8px;
  left: calc(var(--inset) + var(--b0) * (100% - 2 * var(--inset)));
  width: calc((var(--b1) - var(--b0)) * (100% - 2 * var(--inset)));
  background: var(--band);
  border: 2px solid var(--band-edge);
  border-radius: 6px;
}
.ps-last,
.ps-thumb {
  top: 50%;
  border-radius: 50%;
  transform: translate(-50%, -50%);
}
.ps-last {
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  width: 20px;
  height: 20px;
  border: 3px solid var(--text);
  opacity: 0.7;
}
.ps-thumb {
  left: calc(var(--inset) + var(--f) * (100% - 2 * var(--inset)));
  width: 36px;
  height: 36px;
  background: var(--accent);
  border: 4px solid var(--surface);
  box-shadow: 0 0 0 2px var(--text), var(--shadow);
}
.ps-thumb.untouched {
  background: transparent;
  border: 4px dashed var(--text);
  box-shadow: none;
}
.dragging .ps-thumb { transform: translate(-50%, -50%) scale(1.15); }
</style>
