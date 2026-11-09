import { describe, expect, it } from 'vitest'
import { drynessWords, inTrimBand, nearestStage, qualifier } from './dryness'

describe('nearestStage', () => {
  it('picks the nearest whole landmark, x.5 towards dry', () => {
    expect(nearestStage(0)).toBe(0)
    expect(nearestStage(2.4)).toBe(2)
    expect(nearestStage(2.5)).toBe(3)
    expect(nearestStage(2.8)).toBe(3)
    expect(nearestStage(5)).toBe(5)
  })
})

describe('qualifier around leather hard', () => {
  it.each([
    [3.0, ''],
    [2.9, ''],
    [3.1, ''],
    [2.8, 'a touch soft'],
    [2.7, 'a touch soft'],
    [2.6, 'soft side'],
    [2.5, 'soft side'],
    [3.2, 'a touch firm'],
    [3.3, 'a touch firm'],
    [3.4, 'firm side'],
  ] as const)('%s → "%s"', (v, q) => {
    expect(qualifier(v)).toBe(q)
  })

  it('is immune to floating point noise', () => {
    expect(qualifier(0.1 + 0.2 + 2.5)).toBe('a touch soft') // 2.8000000000000003
    expect(qualifier(3.2 - 1e-9)).toBe('a touch firm')
  })
})

describe('drynessWords', () => {
  it('names the landmark alone when close to it', () => {
    expect(drynessWords(3)).toBe('Leather hard')
    expect(drynessWords(0)).toBe('Very wet')
    expect(drynessWords(5)).toBe('Bone dry')
  })
  it('adds the qualifier after a comma', () => {
    expect(drynessWords(2.8)).toBe('Leather hard, a touch soft')
    expect(drynessWords(3.4)).toBe('Leather hard, firm side')
    expect(drynessWords(3.6)).toBe('Hard leather hard, soft side')
    expect(drynessWords(4.2)).toBe('Hard leather hard, a touch firm')
  })
  it('talks about wetness up to soft leather hard', () => {
    expect(drynessWords(2.4)).toBe('Soft leather hard, firming up')
    expect(drynessWords(2.3)).toBe('Soft leather hard, a touch firmer')
    expect(drynessWords(1.8)).toBe('Soft leather hard, a touch wetter')
    expect(drynessWords(1.6)).toBe('Soft leather hard, wetter')
    expect(drynessWords(0.3)).toBe('Very wet, a touch firmer')
  })
})

describe('inTrimBand', () => {
  it('includes 2.8 and 3.6, excludes just outside', () => {
    expect(inTrimBand(2.8)).toBe(true)
    expect(inTrimBand(3.6)).toBe(true)
    expect(inTrimBand(2.7)).toBe(false)
    expect(inTrimBand(3.7)).toBe(false)
    expect(inTrimBand(0.1 + 0.2 + 2.5)).toBe(true)
  })
})
