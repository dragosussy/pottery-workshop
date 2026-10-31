// Gesture engine for the DrynessSlider (frontend plan §5.1).
//
// The maths lives in pure functions and a small DOM-free state machine
// (`SliderGesture`) so it can be unit-tested with plain numbers. The
// `usePointerSlider` composable wires DOM pointer, touch and key events to it.

import { onBeforeUnmount, ref, type Ref } from 'vue'

export const GESTURE = {
  /** Horizontal travel that captures the gesture as a drag. */
  captureX: 6,
  /** Vertical travel that gives the gesture away to scrolling. */
  releaseY: 10,
  /** Below the track centre: ½× after this many px … */
  fineHalfAt: 60,
  /** … and ¼× after this many. */
  fineQuarterAt: 120,
  /** A press within this many px of the thumb centre grabs the thumb (no jump). */
  thumbGrabRadius: 26,
  /** Nudge hold-repeat: first repeat after `repeatDelay`, then every `repeatInterval`. */
  repeatDelay: 400,
  repeatInterval: 90,
  /** After this many repeats the interval halves. */
  repeatAccelerateAfter: 10,
} as const

export type GesturePhase = 'idle' | 'pending' | 'drag'
export type Classification = 'pending' | 'drag' | 'scroll'

/**
 * Decide what a touch that moved (dx, dy) from its start is. The slider only
 * takes the gesture once horizontal movement passes 6px before vertical
 * movement passes 10px; otherwise it belongs to the page scroll.
 */
export function classifyGesture(dx: number, dy: number): Classification {
  const ax = Math.abs(dx)
  const ay = Math.abs(dy)
  const overX = ax > GESTURE.captureX
  const overY = ay > GESTURE.releaseY
  if (overX && overY) return ax > ay ? 'drag' : 'scroll' // one big jump: dominant axis wins
  if (overY) return 'scroll'
  if (overX) return 'drag'
  return 'pending'
}

/**
 * Sensitivity while dragging, from how far the finger is *below* the track
 * centre: 1× at the track, ½× after 60px, ¼× after 120px. Moving up never
 * changes sensitivity.
 */
export function fineFactor(distanceBelow: number): number {
  if (distanceBelow >= GESTURE.fineQuarterAt) return 0.25
  if (distanceBelow >= GESTURE.fineHalfAt) return 0.5
  return 1
}

export interface TrackGeometry {
  /** Client x of the value 0 position. */
  left: number
  /** Width in px of the 0..max span. */
  width: number
  /** Client y of the track centre line. */
  centerY: number
}

export interface Scale {
  min: number
  max: number
  step: number
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

/** Snap to the step grid (works in integer steps to avoid FP drift). */
export function snap(v: number, s: Scale): number {
  const steps = Math.round((clamp(v, s.min, s.max) - s.min) / s.step)
  const decimals = Math.max(0, -Math.floor(Math.log10(s.step)))
  return Number((s.min + steps * s.step).toFixed(decimals))
}

/** Unsnapped value under client x. */
export function positionToRaw(x: number, geo: TrackGeometry, s: Scale): number {
  if (geo.width <= 0) return s.min
  const f = clamp((x - geo.left) / geo.width, 0, 1)
  return s.min + f * (s.max - s.min)
}

export function valueToFraction(v: number, s: Scale): number {
  return clamp((v - s.min) / (s.max - s.min), 0, 1)
}

export function valueToX(v: number, geo: TrackGeometry, s: Scale): number {
  return geo.left + valueToFraction(v, s) * geo.width
}

/** Keyboard: arrows ±step, PageUp/PageDown ±1.0, Home/End to the ends. */
export function keyToValue(key: string, value: number, s: Scale): number | null {
  switch (key) {
    case 'ArrowRight':
    case 'ArrowUp':
      return snap(value + s.step, s)
    case 'ArrowLeft':
    case 'ArrowDown':
      return snap(value - s.step, s)
    case 'PageUp':
      return snap(value + 1, s)
    case 'PageDown':
      return snap(value - 1, s)
    case 'Home':
      return s.min
    case 'End':
      return s.max
    default:
      return null
  }
}

export interface MoveResult {
  phase: GesturePhase
  /** The snapped value to emit, if it should change. */
  value?: number
  /** True on the move that turned the gesture into a drag (capture the pointer). */
  captured?: boolean
  /** True when the gesture was handed to the page scroll. */
  released?: boolean
  /** Current sensitivity (1, 0.5 or 0.25). */
  factor: number
}

/**
 * DOM-free state machine for one pointer gesture on the track.
 *
 * - `down`: remember the start. Nothing changes yet.
 * - `move` while pending: classify. A drag captures; a vertical move releases.
 *   On capture, a press that grabbed the thumb moves it relatively (no jump);
 *   a press elsewhere jumps the thumb to the finger, then moves relatively.
 * - `move` while dragging: value += Δx / pxPerUnit × fineFactor(distance below).
 * - `up` while still pending (no real movement): a tap moves the thumb to the
 *   tapped point, unless the tap was on the thumb itself.
 */
export class SliderGesture {
  phase: GesturePhase = 'idle'
  factor = 1
  private startX = 0
  private startY = 0
  private lastX = 0
  private raw = 0
  private startValue = 0
  private grabbedThumb = false

  constructor(private readonly scale: Scale) {}

  down(x: number, y: number, value: number, geo: TrackGeometry): void {
    this.phase = 'pending'
    this.startX = x
    this.startY = y
    this.lastX = x
    this.startValue = value
    this.raw = value
    this.factor = 1
    this.grabbedThumb = Math.abs(x - valueToX(value, geo, this.scale)) <= GESTURE.thumbGrabRadius
  }

  move(x: number, y: number, geo: TrackGeometry): MoveResult {
    if (this.phase === 'idle') return { phase: 'idle', factor: 1 }
    if (this.phase === 'pending') {
      const c = classifyGesture(x - this.startX, y - this.startY)
      if (c === 'pending') return { phase: 'pending', factor: 1 }
      if (c === 'scroll') {
        this.phase = 'idle'
        return { phase: 'idle', released: true, factor: 1 }
      }
      // Captured: start the drag, applying the movement so far at 1×.
      this.phase = 'drag'
      this.raw = this.grabbedThumb
        ? this.startValue + this.dxToUnits(x - this.startX, geo)
        : positionToRaw(x, geo, this.scale)
      this.raw = clamp(this.raw, this.scale.min, this.scale.max)
      this.lastX = x
      this.factor = 1
      return { phase: 'drag', captured: true, value: snap(this.raw, this.scale), factor: 1 }
    }
    // Dragging.
    this.factor = fineFactor(y - geo.centerY)
    this.raw = clamp(this.raw + this.dxToUnits(x - this.lastX, geo) * this.factor, this.scale.min, this.scale.max)
    this.lastX = x
    return { phase: 'drag', value: snap(this.raw, this.scale), factor: this.factor }
  }

  /** Returns the value for a tap, or undefined if nothing should change. */
  up(x: number, geo: TrackGeometry): number | undefined {
    const wasPending = this.phase === 'pending'
    this.phase = 'idle'
    this.factor = 1
    if (wasPending && !this.grabbedThumb) return snap(positionToRaw(x, geo, this.scale), this.scale)
    return undefined
  }

  cancel(): void {
    this.phase = 'idle'
    this.factor = 1
  }

  private dxToUnits(dx: number, geo: TrackGeometry): number {
    if (geo.width <= 0) return 0
    return (dx / geo.width) * (this.scale.max - this.scale.min)
  }
}

/**
 * Hold-to-repeat for the nudge buttons: fires once immediately, then after
 * `repeatDelay` repeats every `repeatInterval` (twice as fast after a while).
 */
export function createRepeater(fire: () => boolean | void) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let count = 0
  const stop = () => {
    if (timer !== null) clearTimeout(timer)
    timer = null
    count = 0
  }
  const tick = () => {
    count++
    // `fire` returns false when the value can't move any further.
    if (fire() === false) return stop()
    const interval = count > GESTURE.repeatAccelerateAfter ? GESTURE.repeatInterval / 2 : GESTURE.repeatInterval
    timer = setTimeout(tick, interval)
  }
  const start = () => {
    stop()
    if (fire() === false) return
    timer = setTimeout(tick, GESTURE.repeatDelay)
  }
  return { start, stop, get active() { return timer !== null } }
}

// ---------------------------------------------------------------------------
// DOM wiring
// ---------------------------------------------------------------------------

export type ChangeSource = 'drag' | 'tap' | 'key' | 'nudge'

export interface PointerSliderOptions {
  /** The element with the full touch area. */
  track: Ref<HTMLElement | null>
  /** Horizontal inset (px) of the 0 and max positions inside the track (thumb radius). */
  inset: number
  scale: Scale
  getValue: () => number
  setValue: (v: number, source: ChangeSource) => void
  /** Called when a drag ends (pointer up). */
  onCommit?: () => void
  isDisabled?: () => boolean
}

export function usePointerSlider(opts: PointerSliderOptions) {
  const gesture = new SliderGesture(opts.scale)
  const dragging = ref(false)
  const factor = ref(1)
  let pointerId: number | null = null
  let startX = 0
  let startY = 0

  const geometry = (): TrackGeometry => {
    const el = opts.track.value
    if (!el) return { left: 0, width: 0, centerY: 0 }
    const r = el.getBoundingClientRect()
    return { left: r.left + opts.inset, width: Math.max(0, r.width - 2 * opts.inset), centerY: r.top + r.height / 2 }
  }

  const reset = () => {
    gesture.cancel()
    dragging.value = false
    factor.value = 1
    pointerId = null
  }

  function onPointerdown(e: PointerEvent) {
    if (opts.isDisabled?.()) return
    if (e.pointerType === 'mouse' && e.button !== 0) return
    if (pointerId !== null) return // ignore a second finger
    pointerId = e.pointerId
    startX = e.clientX
    startY = e.clientY
    gesture.down(e.clientX, e.clientY, opts.getValue(), geometry())
    // A mouse never scrolls the page by dragging, so capture straight away.
    if (e.pointerType === 'mouse') {
      e.preventDefault()
      ;(opts.track.value as HTMLElement | null)?.focus?.({ preventScroll: true })
    }
  }

  function onPointermove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return
    const r = gesture.move(e.clientX, e.clientY, geometry())
    if (r.released) return reset()
    if (r.captured) {
      dragging.value = true
      try {
        opts.track.value?.setPointerCapture(e.pointerId)
      } catch {
        // Capturing can fail if the pointer is already gone; harmless.
      }
    }
    factor.value = r.factor
    if (r.value !== undefined && r.value !== opts.getValue()) opts.setValue(r.value, 'drag')
  }

  function onPointerup(e: PointerEvent) {
    if (e.pointerId !== pointerId) return
    const wasDragging = dragging.value
    const v = gesture.up(e.clientX, geometry())
    if (v !== undefined) opts.setValue(v, 'tap')
    reset()
    if (wasDragging || v !== undefined) opts.onCommit?.()
  }

  function onPointercancel(e: PointerEvent) {
    if (e.pointerId !== pointerId) return
    const wasDragging = dragging.value
    reset()
    if (wasDragging) opts.onCommit?.()
  }

  /**
   * Non-passive touchmove: once the slider owns the gesture (or the movement
   * so far is mostly horizontal), stop the browser from starting a vertical
   * scroll, so moving the finger *down* for fine scrubbing keeps working.
   */
  function onTouchmove(e: TouchEvent) {
    if (pointerId === null || !e.cancelable) return
    const t = e.touches[0]
    if (!t) return
    const dx = Math.abs(t.clientX - startX)
    const dy = Math.abs(t.clientY - startY)
    if (gesture.phase === 'drag' || (gesture.phase === 'pending' && dx > dy)) e.preventDefault()
  }

  function onKeydown(e: KeyboardEvent) {
    if (opts.isDisabled?.()) return
    const v = keyToValue(e.key, opts.getValue(), opts.scale)
    if (v === null) return
    e.preventDefault()
    // Emitted even when unchanged (at an end): a key press is a deliberate touch.
    opts.setValue(v, 'key')
  }

  /** Move by `dir` steps; returns false when already at the end. */
  function nudge(dir: 1 | -1): boolean {
    if (opts.isDisabled?.()) return false
    const cur = opts.getValue()
    const v = snap(cur + dir * opts.scale.step, opts.scale)
    if (v === cur) return false
    opts.setValue(v, 'nudge')
    return true
  }

  const repeaters = {
    [-1]: createRepeater(() => nudge(-1)),
    [1]: createRepeater(() => nudge(1)),
  } as const

  function startRepeat(dir: 1 | -1) {
    repeaters[dir].start()
  }
  function stopRepeat() {
    repeaters[-1].stop()
    repeaters[1].stop()
  }

  onBeforeUnmount(stopRepeat)

  return {
    dragging,
    factor,
    geometry,
    onPointerdown,
    onPointermove,
    onPointerup,
    onPointercancel,
    onTouchmove,
    onKeydown,
    nudge,
    startRepeat,
    stopRepeat,
  }
}
