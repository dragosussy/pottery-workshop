import { describe, expect, it } from 'vitest'
import { thicknessWords } from './thickness'

describe('thicknessWords', () => {
  it('names the landmarks', () => {
    expect([0, 1, 2, 3, 4, 5].map(thicknessWords)).toEqual(['Trace', 'Very thin', 'Thin', 'Medium', 'Thick', 'Very thick'])
  })
  it('uses the nearest one, with halves going thicker', () => {
    expect(thicknessWords(2.4)).toBe('Thin')
    expect(thicknessWords(2.5)).toBe('Medium')
    expect(thicknessWords(0.2)).toBe('Trace')
    expect(thicknessWords(4.9)).toBe('Very thick')
  })
})
