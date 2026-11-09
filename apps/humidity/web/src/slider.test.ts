import { describe, expect, it } from 'vitest'
import { classify, fineFactor, fraction, keyValue, snap, valueAt } from './slider'

const dryness = { min: 0, max: 5, step: 0.1 }
const target = { min: 50, max: 94, step: 1 }

describe('snap', () => {
  it('rounds to the step without float drift', () => {
    expect(snap(2.84, dryness)).toBe(2.8)
    expect(snap(2.85, dryness)).toBe(2.9)
    expect(snap(0.1 + 0.2, dryness)).toBe(0.3)
    expect(snap(81.6, target)).toBe(82)
  })
  it('clamps to the ends', () => {
    expect(snap(-1, dryness)).toBe(0)
    expect(snap(7, dryness)).toBe(5)
    expect(snap(120, target)).toBe(94)
  })
})

describe('positions', () => {
  it('maps values to 0..1 and back', () => {
    expect(fraction(2.5, dryness)).toBe(0.5)
    expect(fraction(50, target)).toBe(0)
    expect(valueAt(150, 100, 200, dryness)).toBe(1.25)
    expect(valueAt(0, 100, 200, dryness)).toBe(0)
    expect(valueAt(999, 100, 200, target)).toBe(94)
  })
})

describe('fineFactor', () => {
  it('is 1× near the track, ½× beyond 60 px below, ¼× beyond 120 px', () => {
    expect(fineFactor(-200)).toBe(1)
    expect(fineFactor(0)).toBe(1)
    expect(fineFactor(59)).toBe(1)
    expect(fineFactor(60)).toBe(0.5)
    expect(fineFactor(119)).toBe(0.5)
    expect(fineFactor(120)).toBe(0.25)
  })
})

describe('classify', () => {
  it('waits for real movement', () => {
    expect(classify(3, 4)).toBe('pending')
  })
  it('takes horizontal movement as a drag', () => {
    expect(classify(7, 2)).toBe('drag')
    expect(classify(-7, 9)).toBe('drag')
  })
  it('gives vertical movement to the page scroll', () => {
    expect(classify(2, 11)).toBe('scroll')
    expect(classify(12, -20)).toBe('scroll')
    expect(classify(25, 12)).toBe('drag')
  })
})

describe('keyValue', () => {
  it('steps with arrows and pages, jumps with Home/End', () => {
    expect(keyValue('ArrowRight', 2.8, dryness)).toBe(2.9)
    expect(keyValue('ArrowDown', 2.8, dryness)).toBe(2.7)
    expect(keyValue('PageUp', 2.8, dryness)).toBe(3.8)
    expect(keyValue('PageDown', 55, target)).toBe(50)
    expect(keyValue('Home', 80, target)).toBe(50)
    expect(keyValue('End', 80, target)).toBe(94)
    expect(keyValue('ArrowRight', 5, dryness)).toBe(5)
    expect(keyValue('a', 1, dryness)).toBeNull()
  })
})
