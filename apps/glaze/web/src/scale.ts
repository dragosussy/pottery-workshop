// Scaling a recipe to a batch. Amounts are parts by weight; the ingredients
// that aren't additions are the base, and the batch is the weight of the base.
import type { RecipeLine } from './api'

export interface ScaledLine extends RecipeLine {
  percent: number // of the base
  grams: number
}

export const baseTotal = (lines: RecipeLine[]) => lines.filter((l) => !l.addition).reduce((sum, l) => sum + l.amount, 0)

/** The water to go with a batch of the base. */
export function waterFor(lines: RecipeLine[], water: number, batch: number): number {
  const base = baseTotal(lines)
  return base > 0 ? (water / base) * batch : 0
}

/** The batch of the base that goes with an amount of water, for a wash. */
export const baseForWater = (lines: RecipeLine[], water: number, ml: number) => (ml / water) * baseTotal(lines)

export function scaleRecipe(lines: RecipeLine[], batch: number): ScaledLine[] {
  const base = baseTotal(lines)
  return lines.map((l) => {
    const share = base > 0 ? l.amount / base : 0
    return { ...l, percent: share * 100, grams: share * batch }
  })
}

export const formatMl = (ml: number) => `${Math.round(ml)} ml`

export function formatPercent(p: number): string {
  return `${Math.round(p * 10) / 10} %`
}

/** Whole grams once it's enough that a tenth doesn't matter. */
export function formatGrams(g: number): string {
  return g >= 100 ? `${Math.round(g)} g` : `${(Math.round(g * 10) / 10).toFixed(1)} g`
}
