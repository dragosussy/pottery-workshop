// The mock follows the contract rules the UI depends on; these tests pin them
// so the fixtures stay a faithful stand-in for the real backend.
import { describe, expect, it } from 'vitest'
import { createMockBackend } from './backend'
import { FIXED_NOW, SCENARIOS, snapshot } from './fixtures'

const now = () => FIXED_NOW
function call(b: ReturnType<typeof createMockBackend>, method: string, path: string, json?: unknown) {
  const out = b.handle({ method, path: path.split('?')[0]!, query: new URLSearchParams(path.split('?')[1] ?? ''), json })
  return { status: out.status, body: out.body ? JSON.parse(out.body as string) : null }
}

describe('fixtures', () => {
  it.each(SCENARIOS)('%s: every documented status field is present', (name) => {
    const { status, shelf } = snapshot(name)
    for (const k of ['server_time', 'poll_interval_seconds', 'stale_after_seconds', 'room', 'sensors', 'shelf', 'trim', 'learning', 'last_offline', 'control'])
      expect(status).toHaveProperty(k)
    expect(Object.keys(status.shelf.by_stage)).toHaveLength(6)
    for (const p of shelf.items) expect(p.prediction).not.toBeNull()
  })

  it('visits use an around-the-clock 30-minute grid (no studio schedule)', () => {
    const seen = new Set<number>()
    for (let h = 0; h < 24; h += 1) {
      const s = snapshot('calibrated', FIXED_NOW + h * 3_600_000)
      const v = s.status.trim.next_visit
      expect(v, `at +${h}h`).not.toBeNull()
      const start = Date.parse(v!.start)
      // Every slot is "now" or on a half hour.
      expect(start === Date.parse(s.status.server_time) || start % 1_800_000 === 0, `at +${h}h`).toBe(true)
      seen.add(new Date(Date.parse(v!.recommended_at)).getUTCHours())
      expect(s.status.trim).not.toHaveProperty('at_risk')
      expect(s.shelf).not.toHaveProperty('at_risk')
    }
    expect(seen.size).toBeGreaterThan(1)
  })

  it('ready_now scenarios have ready pieces whatever time it is', () => {
    for (let h = 0; h < 24; h += 1) {
      const t = FIXED_NOW + h * 3_600_000
      expect(snapshot('ready_now', t).status.trim.ready_now_count, `+${h}h`).toBe(3)
    }
  })

  it('covers the states the trim card must show', () => {
    expect(snapshot('empty').status.shelf.on_shelf).toBe(0)
    expect(snapshot('learning').status.learning).toMatchObject({ state: 'learning', samples: 2, needed: 5 })
    expect(snapshot('ready_now').status.trim.ready_now_count).toBeGreaterThan(0)
    expect(snapshot('back_from_offline').status.last_offline?.pieces_to_check).toBe(3)
    expect(snapshot('stale_sensor').status.room.state).toBe('stale')
    expect(snapshot('calibrated').settings).toEqual({ studio_tz: 'Europe/Bucharest', sensors: expect.any(Array) })
  })
})

describe('mock backend: lifecycle (contract §3.5)', () => {
  it('flip → trim → undo → back on the shelf', () => {
    const b = createMockBackend('calibrated', now)
    const p = b.world.pieces.find((x) => x.state === 'in')!
    expect(call(b, 'PATCH', `/pieces/${p.id}`, { state: 'flipped', flip_dryness: 2.5 }).body.state).toBe('flipped')
    const trimmed = call(b, 'PATCH', `/pieces/${p.id}`, { state: 'trimmed' }).body
    expect(trimmed).toMatchObject({ state: 'trimmed', end_dryness: 3 })
    // Same state again is a no-op (safe retry).
    expect(call(b, 'PATCH', `/pieces/${p.id}`, { state: 'trimmed' }).status).toBe(200)
    // Undo goes back to the state before the end.
    expect(call(b, 'PATCH', `/pieces/${p.id}`, { state: 'in' }).status).toBe(409)
    expect(call(b, 'PATCH', `/pieces/${p.id}`, { state: 'flipped' }).body.state).toBe('flipped')
  })

  it('rejects invalid transitions and unknown fields with the envelope', () => {
    const b = createMockBackend('calibrated', now)
    const p = b.world.pieces.find((x) => x.state === 'in')!
    const r = call(b, 'PATCH', `/pieces/${p.id}`, { state: 'in', nope: 1 })
    expect(r.status).toBe(422)
    expect(r.body.error.fields[0]).toMatchObject({ field: 'nope', code: 'unknown_field' })
    const bad = call(b, 'PATCH', `/pieces/${p.id}`, { state: 'trimmed', end_dryness: 7 })
    expect(bad.status).toBe(422)
    expect(bad.body.error.fields[0].code).toBe('out_of_range')
  })

  it('a group PATCH is all or nothing', () => {
    const b = createMockBackend('calibrated', now)
    const inIds = b.world.pieces.filter((x) => x.state === 'in').map((x) => x.id)
    const ended = b.world.pieces.find((x) => x.state === 'trimmed')!
    const r = call(b, 'PATCH', '/pieces', { ids: [inIds[0], ended.id], state: 'flipped' })
    expect(r.status).toBe(409)
    expect(b.world.pieces.find((x) => x.id === inIds[0])!.state).toBe('in')
  })

  it('adds N pieces with the lowest free shelf numbers, as a group', () => {
    const b = createMockBackend('ready_now', now) // numbers 1–4 in use
    const r = call(b, 'POST', '/pieces', { quantity: 3, form: 'mug', initial_dryness: 1.0, size: 'medium', thickness: 'thin' })
    expect(r.status).toBe(201)
    expect(r.body.items.map((p: { number: number }) => p.number)).toEqual([5, 6, 7])
    expect(new Set(r.body.items.map((p: { group_id: number }) => p.group_id)).size).toBe(1)
  })

  it('asks "went wetter" once, and refuses observations on ended pieces', () => {
    const b = createMockBackend('calibrated', now)
    const p = b.world.pieces.find((x) => x.state === 'in' && (x.current_dryness ?? 0) >= 2)!
    const r = call(b, 'POST', '/observations', { items: [{ piece_id: p.id, dryness: 0.2 }] })
    expect(r.status).toBe(201)
    expect(r.body.follow_ups[0].kind).toBe('went_wetter')
    expect(r.body.items[0].flag).toBe('inconsistent')
    const again = call(b, 'POST', '/observations', { items: [{ piece_id: p.id, dryness: 0.1 }] })
    expect(again.body.follow_ups).toEqual([])
    const ended = b.world.pieces.find((x) => x.state === 'trimmed')!
    expect(call(b, 'POST', '/observations', { items: [{ piece_id: ended.id, dryness: 3 }] }).body.error.code).toBe('piece_ended')
  })

  it("won't delete a piece's only observation", () => {
    const b = createMockBackend('calibrated', now)
    const p = b.world.pieces.find((x) => x.observation_count === 1)!
    const only = b.world.observations.find((o) => o.piece_id === p.id)!
    expect(call(b, 'DELETE', `/observations/${only.id}`).body.error.code).toBe('last_observation')
  })

  it('settings only take the time zone and sensors', () => {
    const b = createMockBackend('calibrated', now)
    expect(call(b, 'PATCH', '/settings', { studio_days: ['mon'] }).body.error.fields[0]).toMatchObject({ field: 'studio_days', code: 'unknown_field' })
    expect(call(b, 'PATCH', '/settings', { studio_tz: 'Europe/London' }).body.studio_tz).toBe('Europe/London')
  })

  it('the humidity target can be set, previewed and cleared', () => {
    const b = createMockBackend('calibrated', now)
    expect(call(b, 'GET', '/control').body).toMatchObject({ target_humidity_pct: null, controller: true })
    expect(call(b, 'PUT', '/control/setpoint', { target_humidity_pct: 30 }).status).toBe(422)
    expect(call(b, 'PUT', '/control/setpoint', { target_humidity_pct: 84 }).body).toMatchObject({ target_humidity_pct: 84 })
    expect(call(b, 'GET', '/status').body.control.target_humidity_pct).toBe(84)
    expect(call(b, 'GET', '/control/preview?target_humidity_pct=90').body).toHaveProperty('next_visit')
    expect(call(b, 'PUT', '/control/setpoint', { target_humidity_pct: null }).body.target_humidity_pct).toBeNull()
  })

  it('the pump runs by hand and stops', () => {
    const b = createMockBackend('calibrated', now)
    expect(call(b, 'POST', '/control/override', { seconds: 600 }).status).toBe(422)
    expect(call(b, 'POST', '/control/override', { seconds: 5 }).body.pump).toMatchObject({ on: true, reason: 'manual' })
    expect(call(b, 'DELETE', '/control/override').body.pump.on).toBe(false)
    expect(call(b, 'POST', '/control/reset', {}).body.safety.tripped).toBe(false)
  })

  it('unknown routes return the JSON 404 envelope', () => {
    const b = createMockBackend('calibrated', now)
    expect(call(b, 'GET', '/nope')).toMatchObject({ status: 404, body: { error: { code: 'not_found' } } })
  })
})
