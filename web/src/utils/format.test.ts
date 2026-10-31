import { describe, expect, it } from 'vitest'
import { formatDayTime, formatEtaWindow, formatVisitWindow, relativeDay } from './format'

const TZ = 'Europe/Bucharest'
// Friday 2026-10-30 16:05 local (14:05Z)
const NOW = Date.parse('2026-10-30T14:05:00Z')
const at = (iso: string) => Date.parse(iso)

describe('relativeDay / formatDayTime (studio time)', () => {
  it('names today, tomorrow and weekdays', () => {
    expect(relativeDay(at('2026-10-30T18:00:00Z'), NOW, TZ)).toBe('today')
    expect(relativeDay(at('2026-10-30T22:30:00Z'), NOW, TZ)).toBe('tomorrow') // 00:30 Sat local
    expect(relativeDay(at('2026-11-01T10:00:00Z'), NOW, TZ)).toBe('Sun')
    expect(relativeDay(at('2026-11-09T10:00:00Z'), NOW, TZ)).toBe('Nov 9')
  })
  it('formats times in the studio zone', () => {
    expect(formatDayTime(at('2026-10-31T07:00:00Z'), NOW, TZ)).toBe('tomorrow 09:00')
    expect(formatDayTime(at('2026-10-31T07:00:00Z'), NOW, 'UTC')).toBe('tomorrow 07:00')
  })
})

describe('formatEtaWindow', () => {
  it('uses hours when close', () => {
    expect(formatEtaWindow(NOW + 3 * 3.6e6, NOW + 6 * 3.6e6, NOW, TZ)).toBe('in 3–6h')
    expect(formatEtaWindow(NOW - 3.6e6, NOW + 2 * 3.6e6, NOW, TZ)).toBe('within 2h')
  })
  it('says "tonight" for an evening window today', () => {
    // 20:00–23:00 local, more than 12 h? No: make "now" early morning.
    const early = Date.parse('2026-10-30T04:00:00Z') // 06:00 local
    expect(formatEtaWindow(at('2026-10-30T18:00:00Z'), at('2026-10-30T21:00:00Z'), early, TZ)).toBe('tonight 20:00–23:00')
  })
  it('uses a part of day for a long window', () => {
    expect(formatEtaWindow(at('2026-10-31T05:00:00Z'), at('2026-10-31T09:30:00Z'), NOW, TZ)).toBe('tomorrow 07:00–11:30')
    expect(formatEtaWindow(at('2026-11-01T03:30:00Z'), at('2026-11-01T09:45:00Z'), NOW, TZ)).toBe('Sun morning')
    expect(formatEtaWindow(at('2026-10-31T17:00:00Z'), at('2026-11-01T07:00:00Z'), NOW, TZ)).toBe('tomorrow evening – Sun morning')
  })
  it('keeps local hours across the DST change', () => {
    const sat = Date.parse('2026-10-24T08:00:00Z') // Sat 11:00 EEST
    // Sunday after the change: 09:00–13:00 local = 07:00–11:00Z
    expect(formatEtaWindow(at('2026-10-25T07:00:00Z'), at('2026-10-25T11:00:00Z'), sat, TZ)).toBe('tomorrow 09:00–13:00')
  })
})

describe('visit window', () => {
  it('formats a same-day window', () => {
    expect(formatVisitWindow(at('2026-10-31T07:00:00Z'), at('2026-10-31T09:30:00Z'), NOW, TZ)).toBe('tomorrow 09:00–11:30')
  })
})
