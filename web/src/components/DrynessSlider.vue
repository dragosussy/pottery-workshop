<script setup lang="ts">
// The instrument (frontend plan §5.1): a custom pointer-events slider for
// continuous dryness 0.0–5.0, built for a phone and muddy fingers.
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import type { StageLandmark } from '@/api/types'
import { usePointerSlider, type ChangeSource } from '@/composables/usePointerSlider'
import {
  DEFAULT_STAGES,
  DRYNESS_MAX,
  DRYNESS_MIN,
  DRYNESS_STEP,
  SHORT_STAGE_LABELS,
  TRIM_BAND,
  crossesTickPoint,
  drynessLabel,
  inTrimBand,
} from '@/utils/dryness'
import { hapticTick } from '@/utils/haptics'

const props = withDefaults(
  defineProps<{
    /** Accessible name, e.g. "How dry is #6 now?" */
    label?: string
    /** Hollow ring at the last observed value. */
    lastValue?: number | null
    /** Faint tick at the model's estimate (piece detail only, never in check-in). */
    estimate?: number | null
    /** Shown under the readout while untouched. */
    untouchedHint?: string
    disabled?: boolean
    stages?: readonly StageLandmark[]
    band?: readonly [number, number]
  }>(),
  {
    label: 'Dryness',
    lastValue: null,
    estimate: null,
    untouchedHint: 'same as last time?',
    disabled: false,
    stages: () => DEFAULT_STAGES,
    band: () => TRIM_BAND,
  },
)

const model = defineModel<number>({ required: true })
/** False until the user moves the value (drag, tap, nudge or key). */
const touched = defineModel<boolean>('touched', { default: false })

const emit = defineEmits<{
  /** A drag or tap ended (useful for committing). */
  commit: [value: number]
}>()

const scale = { min: DRYNESS_MIN, max: DRYNESS_MAX, step: DRYNESS_STEP }
const INSET = 18 // thumb radius: 0 and 5 sit this far inside the track ends

const track = ref<HTMLElement | null>(null)
const uid = useId()

// The value as last set here. The model prop only catches up after the
// parent re-renders, and a burst of pointer events (or nudge repeats) must
// build on the newest value, not a stale prop.
const current = ref(model.value)
watch(model, (v) => (current.value = v))

function setValue(v: number, _source: ChangeSource) {
  const prev = current.value
  if (v !== prev && crossesTickPoint(prev, v, props.band)) hapticTick()
  current.value = v
  model.value = v
  touched.value = true
}

const slider = usePointerSlider({
  track,
  inset: INSET,
  scale,
  getValue: () => current.value,
  setValue,
  onCommit: () => emit('commit', current.value),
  isDisabled: () => props.disabled,
})
const { dragging, factor } = slider

onMounted(() => {
  // Non-passive so the slider can stop a vertical scroll once it owns the gesture.
  track.value?.addEventListener('touchmove', slider.onTouchmove, { passive: false })
})
onBeforeUnmount(() => {
  track.value?.removeEventListener('touchmove', slider.onTouchmove)
})

const fraction = (v: number) => (v - scale.min) / (scale.max - scale.min)
const reading = computed(() => drynessLabel(current.value, props.stages))
const inBand = computed(() => inTrimBand(current.value, props.band))
const ariaText = computed(() => {
  const parts = [reading.value.text]
  if (inBand.value) parts.push('in the trim band')
  if (!touched.value && props.untouchedHint) parts.push(props.untouchedHint)
  return parts.join(', ')
})

const landmarks = computed(() =>
  props.stages.map((s, i) => ({
    ...s,
    short: SHORT_STAGE_LABELS[s.key] ?? s.label,
    f: fraction(s.value),
    row: i % 2,
    edge: i === 0 ? 'start' : i === props.stages.length - 1 ? 'end' : 'mid',
    target: s.key === 'leather_hard',
  })),
)

// Nudge buttons: pointerdown starts hold-repeat, click covers keyboard activation.
function nudgeDown(dir: 1 | -1, e: PointerEvent) {
  if (props.disabled) return
  if (e.pointerType === 'mouse' && e.button !== 0) return
  slider.startRepeat(dir)
}
function nudgeClick(dir: 1 | -1, e: MouseEvent) {
  // Pointer presses were handled on pointerdown; only keyboard clicks (detail 0) step here.
  if (e.detail === 0) slider.nudge(dir)
}
function nudgeStop() {
  slider.stopRepeat()
}
const fineText = computed(() => (factor.value === 0.25 ? 'fine ¼×' : factor.value === 0.5 ? 'fine ½×' : ''))
</script>

<template>
  <div
    class="ds"
    :class="{ 'is-untouched': !touched, 'is-dragging': dragging, 'is-disabled': disabled, 'is-in-band': inBand }"
    :style="{ '--f': fraction(current), '--inset': INSET + 'px' }"
  >
    <div class="ds-wrap">
      <div v-if="dragging" class="ds-bubble" aria-hidden="true">
        <span class="ds-bubble-box">
          <strong>{{ reading.stageLabel }}</strong>
          <span v-if="reading.qualifierText"> · {{ reading.qualifierText }}</span>
          <span v-if="fineText" class="ds-fine">{{ fineText }}</span>
        </span>
        <span class="ds-bubble-arrow"></span>
      </div>

      <div
        :id="uid"
        ref="track"
        class="ds-track"
        role="slider"
        :tabindex="disabled ? -1 : 0"
        :aria-label="label"
        :aria-valuemin="scale.min"
        :aria-valuemax="scale.max"
        :aria-valuenow="current"
        :aria-valuetext="ariaText"
        :aria-disabled="disabled || undefined"
        aria-orientation="horizontal"
        data-testid="slider-track"
        @pointerdown="slider.onPointerdown"
        @pointermove="slider.onPointermove"
        @pointerup="slider.onPointerup"
        @pointercancel="slider.onPointercancel"
        @keydown="slider.onKeydown"
      >
        <div
          class="ds-band"
          :style="{ '--b0': fraction(band[0]), '--b1': fraction(band[1]) }"
          data-testid="trim-band"
        ></div>
        <div class="ds-bar"></div>
        <div
          v-for="m in landmarks"
          :key="m.key"
          class="ds-tick"
          :class="{ 'is-target': m.target }"
          :style="{ '--p': m.f }"
        ></div>
        <div
          v-if="lastValue !== null && lastValue !== undefined"
          class="ds-last"
          :style="{ '--p': fraction(lastValue) }"
          data-testid="last-marker"
          title="Last observed"
        ></div>
        <div
          v-if="estimate !== null && estimate !== undefined"
          class="ds-estimate"
          :style="{ '--p': fraction(estimate) }"
          data-testid="estimate-marker"
          title="Model estimate"
        ></div>
        <div class="ds-thumb" data-testid="thumb"></div>
      </div>
    </div>

    <div class="ds-landmarks" aria-hidden="true">
      <span
        v-for="m in landmarks"
        :key="m.key"
        class="ds-landmark"
        :class="[`row-${m.row}`, `edge-${m.edge}`, { 'is-target': m.target }]"
        :style="{ '--p': m.f }"
      >{{ m.short }}<template v-if="m.target"> ✂</template></span>
    </div>

    <div class="ds-readout-row">
      <button
        type="button"
        class="ds-nudge"
        :disabled="disabled || current <= scale.min"
        aria-label="A touch wetter (−0.1)"
        :aria-controls="uid"
        data-testid="nudge-minus"
        @pointerdown="nudgeDown(-1, $event)"
        @pointerup="nudgeStop"
        @pointerleave="nudgeStop"
        @pointercancel="nudgeStop"
        @click="nudgeClick(-1, $event)"
        @contextmenu.prevent
      >
        <span aria-hidden="true">−</span>
      </button>

      <div class="ds-readout" data-testid="readout">
        <div class="ds-readout-main">{{ reading.stageLabel }}</div>
        <div class="ds-readout-sub">
          <span v-if="reading.qualifierText">{{ reading.qualifierText }}</span>
          <span v-if="inBand" class="ds-band-badge">✂ trim band</span>
        </div>
        <div v-if="!touched && untouchedHint" class="ds-untouched" data-testid="untouched-hint">{{ untouchedHint }}</div>
      </div>

      <button
        type="button"
        class="ds-nudge"
        :disabled="disabled || current >= scale.max"
        aria-label="A touch drier (+0.1)"
        :aria-controls="uid"
        data-testid="nudge-plus"
        @pointerdown="nudgeDown(1, $event)"
        @pointerup="nudgeStop"
        @pointerleave="nudgeStop"
        @pointercancel="nudgeStop"
        @click="nudgeClick(1, $event)"
        @contextmenu.prevent
      >
        <span aria-hidden="true">+</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.ds {
  --track-h: 56px;
  --bar-h: 12px;
  --thumb: 36px;
  container-type: inline-size;
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.ds-wrap {
  position: relative;
}

/* Track: 56px touch area. pan-y lets vertical scrolling through. */
.ds-track {
  position: relative;
  height: var(--track-h);
  touch-action: pan-y;
  cursor: pointer;
  border-radius: var(--radius-sm);
  outline-offset: 4px;
}
.is-disabled .ds-track {
  cursor: default;
  opacity: 0.5;
}
.ds-track > * {
  pointer-events: none;
}
.ds-bar {
  position: absolute;
  left: calc(var(--inset) - 6px);
  right: calc(var(--inset) - 6px);
  top: 50%;
  height: var(--bar-h);
  transform: translateY(-50%);
  border-radius: 999px;
  background: linear-gradient(
    to right,
    var(--dry-0),
    var(--dry-1) 20%,
    var(--dry-2) 40%,
    var(--dry-3) 60%,
    var(--dry-4) 80%,
    var(--dry-5)
  );
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.25);
}

/* Trim band 2.8–3.6: a subtle band behind the bar. */
.ds-band {
  position: absolute;
  top: 6px;
  bottom: 6px;
  left: calc(var(--inset) + var(--b0) * (100% - 2 * var(--inset)));
  width: calc((var(--b1) - var(--b0)) * (100% - 2 * var(--inset)));
  background: var(--trim-band);
  border-left: 2px solid var(--trim-band-edge);
  border-right: 2px solid var(--trim-band-edge);
  border-radius: 6px;
}

.ds-tick {
  position: absolute;
  top: calc(50% + var(--bar-h) / 2 + 2px);
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  width: 2px;
  height: 8px;
  margin-left: -1px;
  background: var(--text-muted);
}
.ds-tick.is-target {
  height: 12px;
  width: 3px;
  background: var(--text);
}

/* Last observed value: hollow ring, always visible. */
.ds-last {
  position: absolute;
  top: 50%;
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  width: 18px;
  height: 18px;
  transform: translate(-50%, -50%);
  border-radius: 50%;
  border: 3px solid var(--text);
  background: transparent;
  opacity: 0.75;
}

/* Model estimate: a faint dashed tick above the bar. */
.ds-estimate {
  position: absolute;
  top: 2px;
  height: calc(50% - var(--bar-h) / 2 - 2px);
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  border-left: 3px dotted var(--text-faint);
  margin-left: -1.5px;
}

.ds-thumb {
  position: absolute;
  top: 50%;
  left: calc(var(--inset) + var(--f) * (100% - 2 * var(--inset)));
  width: var(--thumb);
  height: var(--thumb);
  transform: translate(-50%, -50%);
  border-radius: 50%;
  background: var(--surface);
  border: 4px solid var(--text);
  box-shadow: var(--shadow);
  transition: transform 80ms ease-out;
}
.ds:not(.is-untouched) .ds-thumb {
  background: var(--accent);
  border-color: var(--surface);
  box-shadow: 0 0 0 2px var(--text), var(--shadow);
}
/* Untouched: hollow thumb (a dashed ring, so it reads without colour). */
.is-untouched .ds-thumb {
  background: transparent;
  border-style: dashed;
}
.is-dragging .ds-thumb {
  transform: translate(-50%, -50%) scale(1.12);
}

/* Floating readout above the finger while dragging. */
.ds-bubble {
  position: absolute;
  bottom: calc(100% + 18px);
  left: 0;
  right: 0;
  height: 0;
  z-index: 5;
  pointer-events: none;
}
.ds-bubble-box {
  position: absolute;
  bottom: 0;
  left: clamp(0px, calc(var(--inset) + var(--f) * (100% - 2 * var(--inset)) - 110px), calc(100% - 220px));
  width: 220px;
  padding: 10px 12px;
  border-radius: var(--radius);
  background: var(--text);
  color: var(--bg);
  font-size: var(--fs-md);
  text-align: center;
  box-shadow: var(--shadow-lg);
}
.ds-bubble-arrow {
  position: absolute;
  bottom: -8px;
  left: calc(var(--inset) + var(--f) * (100% - 2 * var(--inset)) - 8px);
  border: 8px solid transparent;
  border-top-color: var(--text);
  border-bottom: 0;
}
.ds-fine {
  display: block;
  margin-top: 2px;
  font-size: var(--fs-xs);
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  opacity: 0.85;
}

/* Landmark labels: two alternating rows on narrow screens. */
.ds-landmarks {
  position: relative;
  height: 2.6em;
  font-size: 0.8125rem;
  color: var(--text-muted);
}
.ds-landmark {
  position: absolute;
  left: calc(var(--inset) + var(--p) * (100% - 2 * var(--inset)));
  transform: translateX(-50%);
  white-space: nowrap;
  line-height: 1.3em;
}
.ds-landmark.row-0 {
  top: 0;
}
.ds-landmark.row-1 {
  top: 1.3em;
}
.ds-landmark.edge-start {
  transform: translateX(calc(-1 * var(--inset)));
}
.ds-landmark.edge-end {
  transform: translateX(calc(-100% + var(--inset)));
}
.ds-landmark.is-target {
  color: var(--text);
  font-weight: 700;
}
@container (min-width: 560px) {
  .ds-landmarks {
    height: 1.4em;
  }
  .ds-landmark.row-1 {
    top: 0;
  }
}

/* Readout row: [−] big label [+] */
.ds-readout-row {
  display: grid;
  grid-template-columns: 56px 1fr 56px;
  align-items: center;
  gap: var(--sp-2);
  margin-top: var(--sp-2);
}
.ds-nudge {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  border: 2px solid var(--border);
  background: var(--surface-2);
  color: var(--text);
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
  touch-action: manipulation;
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}
.ds-nudge:active:not(:disabled) {
  background: var(--surface-3);
}
.ds-nudge:disabled {
  opacity: 0.4;
}
.ds-readout {
  text-align: center;
  min-height: 56px;
}
.ds-readout-main {
  font-size: var(--fs-xl);
  font-weight: 800;
  line-height: 1.15;
}
.ds-readout-sub {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: 0 var(--sp-2);
  font-size: var(--fs-lg);
  color: var(--text-muted);
  min-height: 1.4em;
}
.ds-band-badge {
  color: var(--ok);
  font-weight: 700;
}
.ds-untouched {
  font-size: var(--fs-sm);
  font-style: italic;
  color: var(--text-faint);
}
</style>
