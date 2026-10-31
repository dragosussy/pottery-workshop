import { describe, expect, it } from 'vitest'
import { trimCardModel } from './trimCard'
import { FIXED_NOW, snapshot, type ScenarioName } from '@/mocks/fixtures'

function model(name: ScenarioName, now = FIXED_NOW) {
  const s = snapshot(name, now)
  return trimCardModel(s.status, s.shelf.items, s.settings.studio_tz, now)
}

describe('trim card states (plan §4.1, mid-batch §7.3)', () => {
  it('empty shelf', () => {
    expect(model('empty')).toMatchObject({ kind: 'empty', headline: 'The shelf is empty' })
  })

  it('ready now: "N ready now · best before X"', () => {
    const m = model('ready_now')
    expect(m.kind).toBe('ready_now')
    expect(m.headline).toBe('3 ready now')
    expect(m.detail).toMatch(/^best before (today|tomorrow|\w{3}) \d\d:\d\d$/)
    expect(m.readyIds).toHaveLength(3)
  })

  it('next visit with ready/maybe counts', () => {
    const m = model('calibrated')
    expect(m.kind).toBe('next_visit')
    expect(m.headline).toMatch(/^Come /)
    expect(m.detail).toMatch(/^\d+ ready/)
    expect(m.learning).toBe(false)
    expect(m.confidence).toBe('decent guess')
  })

  it('learning adds the learning line', () => {
    expect(model('learning').confidence).toBe('rough guess · learning your shelf (2 of 5)')
  })

  it('back from offline still recommends (with wide windows)', () => {
    const m = model('back_from_offline')
    expect(['next_visit', 'nothing_soon', 'ready_now']).toContain(m.kind)
  })

  it('a visit can be at any hour, e.g. in the night', () => {
    const s = snapshot('calibrated')
    const night = Date.parse('2026-10-30T23:30:00Z') // 01:30 Sat in Bucharest; "now" is 00:30
    const visit = { ...s.status.trim.next_visit!, start: '2026-10-30T23:30:00Z', end: '2026-10-31T00:30:00Z' }
    const status = { ...s.status, trim: { ...s.status.trim, next_visit: visit, ready_now_count: 0 } }
    const m = trimCardModel(status, s.shelf.items, 'Europe/Bucharest', night - 3_600_000)
    expect(m.headline).toBe('Come today 01:30–02:30')
  })

  it('nothing soon when no slot scores', () => {
    const s = snapshot('calibrated')
    const status = { ...s.status, trim: { ...s.status.trim, next_visit: null, ready_now_count: 0 } }
    expect(trimCardModel(status, s.shelf.items, s.settings.studio_tz, FIXED_NOW).kind).toBe('nothing_soon')
  })
})
