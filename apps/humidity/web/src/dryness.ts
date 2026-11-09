// Dryness words (plan §2): 0.0–5.0, the whole numbers are named landmarks.
// A readout is the nearest landmark plus a qualifier for the distance from it,
// e.g. "Leather hard, a touch soft". Arithmetic is in integer tenths so the
// boundaries never wobble with floating point.

export const STAGES = ['Very wet', 'Wet', 'Soft leather hard', 'Leather hard', 'Hard leather hard', 'Bone dry'] as const
export const TRIM_BAND: [number, number] = [2.8, 3.6]

const tenths = (v: number) => Math.round(v * 10)

/** Index of the nearest landmark; x.5 goes to the drier one. */
export function nearestStage(v: number): number {
  return Math.min(5, Math.max(0, Math.floor((tenths(v) + 5) / 10)))
}

// Distance from the landmark in tenths: ±1 is "at" it, ±2..3 "a touch",
// further "… side". Up to soft leather hard the clay is wet, so the words are
// about wetness rather than softness.
const DRY_WORDS = { touchSoft: 'a touch soft', soft: 'soft side', touchFirm: 'a touch firm', firm: 'firm side' }
const WET_WORDS = { touchSoft: 'a touch wetter', soft: 'wetter', touchFirm: 'a touch firmer', firm: 'firming up' }

export function qualifier(v: number): string {
  const stage = nearestStage(v)
  const d = tenths(v) - stage * 10
  const w = stage <= 2 ? WET_WORDS : DRY_WORDS
  if (Math.abs(d) <= 1) return ''
  if (d < 0) return d >= -3 ? w.touchSoft : w.soft
  return d <= 3 ? w.touchFirm : w.firm
}

/** "Leather hard, a touch soft" */
export function drynessWords(v: number): string {
  const q = qualifier(v)
  const stage = STAGES[nearestStage(v)]!
  return q ? `${stage}, ${q}` : stage
}

export function inTrimBand(v: number): boolean {
  const t = tenths(v)
  return t >= tenths(TRIM_BAND[0]) && t <= tenths(TRIM_BAND[1])
}
