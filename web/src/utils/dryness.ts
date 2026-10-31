// Dryness scale helpers (frontend plan §5.1, mid-batch §2).
//
// Dryness is a continuous value 0.0–5.0 in steps of 0.1. The six named
// stages are landmarks at the whole numbers. A label is the nearest landmark
// plus a qualifier for the distance from it.
//
// All arithmetic is done in integer tenths so that 2.8 - 3 is exactly -2
// tenths and the qualifier boundaries never wobble with floating point.

import type { Dryness, StageKey, StageLandmark } from '../api/types'

export const DRYNESS_MIN = 0
export const DRYNESS_MAX = 5
export const DRYNESS_STEP = 0.1
export const DRYNESS_TARGET = 3
export const TRIM_BAND: readonly [number, number] = [2.8, 3.6]

/** Default landmarks; identical to `GET /reference` stages (contract §3.8). */
export const DEFAULT_STAGES: readonly StageLandmark[] = [
  { key: 'very_wet', label: 'Very wet', value: 0 },
  { key: 'wet', label: 'Wet', value: 1 },
  { key: 'soft_leather_hard', label: 'Soft leather hard', value: 2 },
  { key: 'leather_hard', label: 'Leather hard', value: 3 },
  { key: 'hard_leather_hard', label: 'Hard leather hard', value: 4 },
  { key: 'bone_dry', label: 'Bone dry', value: 5 },
]

/** Short labels drawn under the slider's tick marks. */
export const SHORT_STAGE_LABELS: Record<StageKey, string> = {
  very_wet: 'very wet',
  wet: 'wet',
  soft_leather_hard: 'soft LH',
  leather_hard: 'leather hard',
  hard_leather_hard: 'hard LH',
  bone_dry: 'bone dry',
}

export type Qualifier = 'touch_soft' | 'soft_side' | 'touch_firm' | 'firm_side' | null

/** Clamp to 0–5 and round to the nearest 0.1. */
export function roundDryness(v: number): Dryness {
  if (!Number.isFinite(v)) return DRYNESS_MIN
  const tenths = Math.round(v * 10)
  return Math.min(DRYNESS_MAX * 10, Math.max(DRYNESS_MIN * 10, tenths)) / 10
}

export function toTenths(v: number): number {
  return Math.round(v * 10)
}

/** The nearest landmark (x.5 rounds up, towards dry). */
export function nearestStage(v: number, stages: readonly StageLandmark[] = DEFAULT_STAGES): StageLandmark {
  const t = toTenths(v)
  let best = stages[0]!
  let bestDist = Infinity
  for (const s of stages) {
    const d = Math.abs(t - s.value * 10)
    // `<=` so that a tie at x.5 goes to the drier landmark.
    if (d <= bestDist) {
      best = s
      bestDist = d
    }
  }
  return best
}

/**
 * Qualifier from the distance d = value − landmark (frontend plan §5.1 table):
 *   |d| < 0.15            → none
 *   −0.35 < d ≤ −0.15     → touch_soft
 *   d ≤ −0.35             → soft_side
 *   0.15 ≤ d < 0.35       → touch_firm
 *   d ≥ 0.35              → firm_side
 */
export function qualifierFor(v: number, stages: readonly StageLandmark[] = DEFAULT_STAGES): Qualifier {
  const s = nearestStage(v, stages)
  const d = toTenths(v) - s.value * 10 // tenths
  if (Math.abs(d) <= 1) return null
  if (d < 0) return d >= -3 ? 'touch_soft' : 'soft_side'
  return d <= 3 ? 'touch_firm' : 'firm_side'
}

// Wording. Wet landmarks (very wet, wet, soft leather hard) say
// "wetter"/"firming up" instead of "soft"/"firm" (plan §5.1).
const WORDS_DRY: Record<Exclude<Qualifier, null>, string> = {
  touch_soft: 'a touch soft',
  soft_side: 'soft side',
  touch_firm: 'a touch firm',
  firm_side: 'firm side',
}
const WORDS_WET: Record<Exclude<Qualifier, null>, string> = {
  touch_soft: 'a touch wetter',
  soft_side: 'wetter',
  touch_firm: 'a touch firmer',
  firm_side: 'firming up',
}

export interface DrynessLabel {
  stage: StageLandmark
  qualifier: Qualifier
  /** e.g. "Leather hard" */
  stageLabel: string
  /** e.g. "a touch soft", or "" */
  qualifierText: string
  /** e.g. "Leather hard · a touch soft" */
  text: string
}

export function drynessLabel(v: number, stages: readonly StageLandmark[] = DEFAULT_STAGES): DrynessLabel {
  const stage = nearestStage(v, stages)
  const qualifier = qualifierFor(v, stages)
  const words = stage.value <= 2 ? WORDS_WET : WORDS_DRY
  const qualifierText = qualifier ? words[qualifier] : ''
  return {
    stage,
    qualifier,
    stageLabel: stage.label,
    qualifierText,
    text: qualifierText ? `${stage.label} · ${qualifierText}` : stage.label,
  }
}

export function drynessText(v: number | null | undefined, stages?: readonly StageLandmark[]): string {
  if (v === null || v === undefined) return '—'
  return drynessLabel(v, stages).text
}

export function inTrimBand(v: number, band: readonly [number, number] = TRIM_BAND): boolean {
  const t = toTenths(v)
  return t >= toTenths(band[0]) && t <= toTenths(band[1])
}

/**
 * Whether moving from `prev` to `next` should give a haptic tick: when the
 * value lands on or crosses a landmark (a whole number), or enters or leaves
 * the trim band.
 */
export function crossesTickPoint(prev: number, next: number, band: readonly [number, number] = TRIM_BAND): boolean {
  const a = toTenths(prev)
  const b = toTenths(next)
  if (a === b) return false
  if (inTrimBand(prev, band) !== inTrimBand(next, band)) return true
  const lo = Math.min(a, b)
  const hi = Math.max(a, b)
  // Any whole number in (lo, hi] going up, or [lo, hi) going down.
  for (let t = lo; t <= hi; t++) {
    if (t % 10 !== 0) continue
    if (b > a ? t > lo : t < hi) return true
  }
  return false
}

/** Clay colour for a dryness value (CSS custom property from tokens.css). */
export function drynessColorVar(v: number): string {
  const idx = Math.min(5, Math.max(0, Math.round(v)))
  return `var(--dry-${idx})`
}

export function formatDrynessNumber(v: number): string {
  return roundDryness(v).toFixed(1)
}
