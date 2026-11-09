// The maths behind PreciseSlider, kept pure so it can be unit-tested.

export interface Scale {
  min: number
  max: number
  step: number
}

/** Horizontal travel (px) before a press becomes a drag. */
export const CAPTURE_X = 6
/** Vertical travel (px) that hands the press to page scrolling instead. */
export const RELEASE_Y = 10
/** A press this close (px) to the thumb centre grabs it without jumping. */
export const GRAB_RADIUS = 26

export const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

/** Clamp and round to the step grid, counting whole steps so 0.1s don't drift. */
export function snap(v: number, s: Scale): number {
  const steps = Math.round((clamp(v, s.min, s.max) - s.min) / s.step)
  const decimals = Math.max(0, -Math.floor(Math.log10(s.step)))
  return Number((s.min + steps * s.step).toFixed(decimals))
}

/** 0..1 position of a value along the track. */
export function fraction(v: number, s: Scale): number {
  return s.max > s.min ? clamp((v - s.min) / (s.max - s.min), 0, 1) : 0
}

/** Unsnapped value under x, for a track whose 0..1 span is [left, left + width]. */
export function valueAt(x: number, left: number, width: number, s: Scale): number {
  return width > 0 ? s.min + clamp((x - left) / width, 0, 1) * (s.max - s.min) : s.min
}

/**
 * Sensitivity from how far (px) the finger is below the track: 1× near it,
 * ½× beyond 60 px, ¼× beyond 120 px. Above the track is always 1×.
 */
export function fineFactor(below: number): number {
  if (below >= 120) return 0.25
  if (below >= 60) return 0.5
  return 1
}

/** A press that has moved (dx, dy): still undecided, a drag, or a scroll. */
export function classify(dx: number, dy: number): 'pending' | 'drag' | 'scroll' {
  const ax = Math.abs(dx)
  const ay = Math.abs(dy)
  if (ax > CAPTURE_X && ay > RELEASE_Y) return ax > ay ? 'drag' : 'scroll'
  if (ay > RELEASE_Y) return 'scroll'
  if (ax > CAPTURE_X) return 'drag'
  return 'pending'
}

/** Keyboard: arrows ±1 step, PageUp/PageDown ±10 steps, Home/End. */
export function keyValue(key: string, v: number, s: Scale): number | null {
  const by: Record<string, number> = {
    ArrowRight: 1, ArrowUp: 1, ArrowLeft: -1, ArrowDown: -1, PageUp: 10, PageDown: -10,
  }
  if (key === 'Home') return s.min
  if (key === 'End') return s.max
  const n = by[key]
  return n === undefined ? null : snap(v + n * s.step, s)
}
