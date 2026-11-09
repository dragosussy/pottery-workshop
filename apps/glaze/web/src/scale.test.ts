import { describe, expect, it } from 'vitest'
import { baseForWater, formatGrams, formatMl, formatPercent, scaleRecipe, waterFor } from './scale'

const line = (ingredient: string, amount: number, addition = false) => ({ ingredient, amount, addition })

describe('scaleRecipe', () => {
  it('scales the base to the batch and additions on top of it', () => {
    const scaled = scaleRecipe([line('Feldspar', 40), line('Silica', 60), line('Copper', 2, true)], 3000)
    expect(scaled.map((l) => l.grams)).toEqual([1200, 1800, 60])
    expect(scaled.map((l) => l.percent)).toEqual([40, 60, 2])
  })
  it('works when the base does not add up to 100', () => {
    const scaled = scaleRecipe([line('A', 1), line('B', 3)], 1000)
    expect(scaled.map((l) => l.grams)).toEqual([250, 750])
  })
  it('gives nothing rather than dividing by zero', () => {
    expect(scaleRecipe([line('Copper', 2, true)], 1000)[0]!.grams).toBe(0)
  })
})

describe('water', () => {
  const glaze = [line('Feldspar', 40), line('Silica', 60), line('Copper', 2, true)]
  it('goes by the weight of the base, not the additions', () => {
    expect(waterFor(glaze, 80, 3000)).toBe(2400)
  })
  it('works a wash out from the water', () => {
    const wash = [line('Copper carbonate', 10)]
    const oxide = baseForWater(wash, 100, 300)
    expect(oxide).toBe(30)
    expect(waterFor(wash, 100, oxide)).toBe(300)
  })
  it('reads in whole millilitres', () => {
    expect(formatMl(2400.4)).toBe('2400 ml')
  })
})

describe('formatPercent', () => {
  it('drops needless decimals', () => {
    expect(formatPercent(40)).toBe('40 %')
    expect(formatPercent(33.333)).toBe('33.3 %')
  })
})

describe('formatGrams', () => {
  it('rounds to what a scale can weigh', () => {
    expect(formatGrams(1234.4)).toBe('1234 g')
    expect(formatGrams(12.34)).toBe('12.3 g')
    expect(formatGrams(60)).toBe('60.0 g')
  })
})
