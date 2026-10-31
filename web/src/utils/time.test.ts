import { describe, expect, it } from 'vitest'
import { dayDiff, fromLocalInputValue, toIso, toLocalInputValue, zonedParts, zonedToUtc } from './time'

const TZ = 'Europe/Bucharest' // EEST +3 in summer, EET +2 in winter; DST ends 2026-10-25 04:00 → 03:00

describe('zonedToUtc', () => {
  it('converts wall time in summer and winter', () => {
    expect(toIso(zonedToUtc(2026, 7, 1, 9, 0, TZ))).toBe('2026-07-01T06:00:00Z')
    expect(toIso(zonedToUtc(2026, 11, 2, 9, 0, TZ))).toBe('2026-11-02T07:00:00Z')
  })
  it('handles the autumn DST change day', () => {
    // After the change the offset is +2.
    expect(toIso(zonedToUtc(2026, 10, 25, 8, 0, TZ))).toBe('2026-10-25T06:00:00Z')
    // Before it, +3.
    expect(toIso(zonedToUtc(2026, 10, 25, 1, 0, TZ))).toBe('2026-10-24T22:00:00Z')
    // The repeated hour resolves to its first occurrence.
    expect(toIso(zonedToUtc(2026, 10, 25, 3, 30, TZ))).toBe('2026-10-25T00:30:00Z')
  })
  it('handles the spring gap by moving past it', () => {
    // 2026-03-29 03:00 → 04:00 in Bucharest; 03:30 doesn't exist.
    expect(toIso(zonedToUtc(2026, 3, 29, 9, 0, TZ))).toBe('2026-03-29T06:00:00Z')
    const t = zonedToUtc(2026, 3, 29, 3, 30, TZ)
    expect(zonedParts(t, TZ).hour).toBe(4)
  })
})

describe('dayDiff in the studio time zone', () => {
  it('counts calendar days locally, not UTC days', () => {
    const late = Date.parse('2026-10-30T21:30:00Z') // 23:30 local Fri
    const after = Date.parse('2026-10-30T22:30:00Z') // 00:30 local Sat
    expect(dayDiff(late, after, TZ)).toBe(1)
    expect(dayDiff(late, after, 'UTC')).toBe(0)
  })
  it('works across the DST change (a 25-hour day)', () => {
    const sat = Date.parse('2026-10-24T09:00:00Z')
    const sun = Date.parse('2026-10-25T21:30:00Z') // 23:30 local Sunday
    expect(dayDiff(sat, sun, TZ)).toBe(1)
  })
})

describe('datetime-local round trip', () => {
  it('formats and parses in the studio zone', () => {
    const ms = Date.parse('2026-10-30T14:05:00Z')
    expect(toLocalInputValue(ms, TZ)).toBe('2026-10-30T16:05')
    expect(fromLocalInputValue('2026-10-30T16:05', TZ)).toBe(ms)
  })
})
