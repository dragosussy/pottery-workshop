import { describe, expect, it } from 'vitest'
import { crossesTickPoint, drynessLabel, drynessText, inTrimBand, nearestStage, qualifierFor, roundDryness } from './dryness'

describe('nearestStage', () => {
  it('picks the nearest whole landmark', () => {
    expect(nearestStage(2.8).key).toBe('leather_hard')
    expect(nearestStage(2.4).key).toBe('soft_leather_hard')
    expect(nearestStage(0).key).toBe('very_wet')
    expect(nearestStage(5).key).toBe('bone_dry')
  })
  it('breaks an x.5 tie towards the drier landmark', () => {
    expect(nearestStage(2.5).key).toBe('leather_hard')
    expect(nearestStage(0.5).key).toBe('wet')
  })
})

describe('qualifierFor (plan §5.1 table)', () => {
  // d = value − 3 around leather hard
  it.each([
    [3.0, null],
    [2.9, null], // |d| = 0.1 < 0.15
    [3.1, null],
    [2.8, 'touch_soft'], // −0.35 < −0.2 ≤ −0.15
    [2.7, 'touch_soft'], // −0.3
    [2.6, 'soft_side'], // −0.4 ≤ −0.35
    [2.5, 'soft_side'],
    [3.2, 'touch_firm'], // 0.15 ≤ 0.2 < 0.35
    [3.3, 'touch_firm'],
    [3.4, 'firm_side'], // ≥ 0.35
    [3.5, 'soft_side'], // x.5 tie → nearest is 4: "Hard leather hard · soft side"
    [3.6, 'soft_side'], // nearest is 4
  ] as const)('%s → %s', (v, q) => {
    expect(qualifierFor(v)).toBe(q)
  })

  it('is immune to floating point noise', () => {
    expect(qualifierFor(0.1 + 0.2 + 2.5)).toBe('touch_soft') // 2.8000000000000003
    expect(qualifierFor(3.2 - 1e-9)).toBe("touch_firm")
  })
})

describe('drynessLabel', () => {
  it('builds "Leather hard · a touch soft" for 2.8', () => {
    expect(drynessLabel(2.8).text).toBe('Leather hard · a touch soft')
  })
  it('has no qualifier at a landmark', () => {
    expect(drynessLabel(3).text).toBe('Leather hard')
    expect(drynessLabel(5).text).toBe('Bone dry')
  })
  it('uses firm/soft words from leather hard upwards', () => {
    expect(drynessLabel(3.4).text).toBe('Leather hard · firm side')
    expect(drynessLabel(4.2).text).toBe('Hard leather hard · a touch firm')
    expect(drynessLabel(3.7).text).toBe('Hard leather hard · a touch soft')
  })
  it('uses wetter/firming up at soft leather hard and wetter landmarks', () => {
    expect(drynessLabel(2.4).text).toBe('Soft leather hard · firming up')
    expect(drynessLabel(2.3).text).toBe('Soft leather hard · a touch firmer')
    expect(drynessLabel(1.8).text).toBe('Soft leather hard · a touch wetter')
    expect(drynessLabel(1.6).text).toBe('Soft leather hard · wetter')
    expect(drynessLabel(0.3).text).toBe('Very wet · a touch firmer')
  })
  it('drynessText handles null', () => {
    expect(drynessText(null)).toBe('—')
    expect(drynessText(2.3)).toBe('Soft leather hard · a touch firmer')
  })
})

describe('trim band and haptic ticks', () => {
  it('trim band is 2.8–3.6 inclusive', () => {
    expect(inTrimBand(2.7)).toBe(false)
    expect(inTrimBand(2.8)).toBe(true)
    expect(inTrimBand(3.6)).toBe(true)
    expect(inTrimBand(3.7)).toBe(false)
  })
  it('ticks when landing on or crossing a landmark', () => {
    expect(crossesTickPoint(1.9, 2.0)).toBe(true)
    expect(crossesTickPoint(2.0, 2.1)).toBe(false)
    expect(crossesTickPoint(1.8, 2.3)).toBe(true)
    expect(crossesTickPoint(2.1, 2.0)).toBe(true)
    expect(crossesTickPoint(2.2, 2.1)).toBe(false)
    expect(crossesTickPoint(4.3, 4.4)).toBe(false)
  })
  it('ticks when entering or leaving the trim band', () => {
    expect(crossesTickPoint(2.7, 2.8)).toBe(true)
    expect(crossesTickPoint(3.6, 3.7)).toBe(true)
    expect(crossesTickPoint(3.1, 3.2)).toBe(false)
  })
  it('no tick without a change', () => {
    expect(crossesTickPoint(3, 3)).toBe(false)
  })
})

describe('roundDryness', () => {
  it('clamps and rounds to 0.1', () => {
    expect(roundDryness(2.84)).toBe(2.8)
    expect(roundDryness(2.85)).toBe(2.9)
    expect(roundDryness(-1)).toBe(0)
    expect(roundDryness(7)).toBe(5)
    expect(roundDryness(Number.NaN)).toBe(0)
  })
})
