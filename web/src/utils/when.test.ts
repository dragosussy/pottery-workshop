import { describe, expect, it } from 'vitest'
import { resolveWhen, thisMorning } from './when'

const TZ = 'Europe/Bucharest'

describe('resolveWhen (studio time zone)', () => {
  const now = Date.parse('2026-10-30T14:05:00Z') // 16:05 local
  it('"Now" sends no time at all', () => {
    expect(resolveWhen({ kind: 'now' }, now, TZ)).toBeUndefined()
  })
  it('"1h ago" is relative to server time', () => {
    expect(resolveWhen({ kind: 'ago', hours: 1 }, now, TZ)).toBe('2026-10-30T13:05:00Z')
    expect(resolveWhen({ kind: 'ago', hours: 3 }, now, TZ)).toBe('2026-10-30T11:05:00Z')
  })
  it('"This morning" is 08:00 studio time', () => {
    expect(resolveWhen({ kind: 'morning' }, now, TZ)).toBe('2026-10-30T06:00:00Z')
    expect(resolveWhen({ kind: 'morning' }, now, 'UTC')).toBe('2026-10-30T08:00:00Z')
  })
  it('"This morning" respects DST (Sunday 25 Oct: +3 → +2)', () => {
    const sun = Date.parse('2026-10-25T12:00:00Z')
    expect(resolveWhen({ kind: 'morning' }, sun, TZ)).toBe('2026-10-25T06:00:00Z')
    const sat = Date.parse('2026-10-24T12:00:00Z')
    expect(resolveWhen({ kind: 'morning' }, sat, TZ)).toBe('2026-10-24T05:00:00Z')
  })
  it('"This morning" is unavailable before 08:00', () => {
    const early = Date.parse('2026-10-30T04:30:00Z') // 06:30 local
    expect(thisMorning(early, TZ)).toBeNull()
    expect(resolveWhen({ kind: 'morning' }, early, TZ)).toBeNull()
  })
  it('a picked time is wall time in the studio zone, never in the future', () => {
    expect(resolveWhen({ kind: 'pick', local: '2026-10-30T09:15' }, now, TZ)).toBe('2026-10-30T07:15:00Z')
    expect(resolveWhen({ kind: 'pick', local: '2026-10-30T19:00' }, now, TZ)).toBeNull()
    expect(resolveWhen({ kind: 'pick', local: 'garbage' }, now, TZ)).toBeNull()
  })
})
