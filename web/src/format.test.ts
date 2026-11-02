import { describe, expect, it } from 'vitest'
import { fromNow, when } from './format'

// Monday 2 Nov 2026, 13:00 local time.
const now = new Date(2026, 10, 2, 13, 0).getTime()
const at = (day: number, h: number, m = 0) => new Date(2026, 10, day, h, m).toISOString()

describe('when', () => {
  it('names today, tomorrow and yesterday', () => {
    expect(when(at(2, 18), now)).toBe('today 18:00')
    expect(when(at(3, 9, 5), now)).toBe('tomorrow 09:05')
    expect(when(at(1, 18, 30), now)).toBe('yesterday 18:30')
  })
  it('gives the date for anything further, so next Tuesday is not tomorrow', () => {
    expect(when(at(4, 13), now)).toBe('Wed 4 Nov 13:00')
    expect(when(at(10, 13), now)).toBe('Tue 10 Nov 13:00')
  })
})

describe('fromNow', () => {
  it('uses minutes, hours, then days', () => {
    expect(fromNow(at(2, 13, 40), now)).toBe('in 40 min')
    expect(fromNow(at(3, 9), now)).toBe('in 20 h')
    expect(fromNow(at(10, 13), now)).toBe('in 8 days')
    // Thursday early morning is 3 calendar days from Monday, though only ~2.6 days away.
    expect(fromNow(at(5, 3, 50), now)).toBe('in 3 days')
    expect(fromNow(at(2, 12), now)).toBe('now')
    // Late tomorrow from just after midnight: 47 h but one calendar day.
    expect(fromNow(at(3, 23, 30), new Date(2026, 10, 2, 0, 30).getTime())).toBe('in 47 h')
  })
})
