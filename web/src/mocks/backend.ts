// An in-memory implementation of the v1 API over the fixtures. Pure request →
// response, so it serves both the dev mock (mock/mockApi.ts) and tests
// (src/test/mockFetch.ts). It enforces the contract rules the UI depends on:
// the error envelope, lifecycle transitions (§3.5), dryness range, follow-ups.
//
// Only relative imports here: this file is also compiled for the Vite config.

import type {
  AddPieces,
  ChangeScope,
  FieldError,
  NewObservations,
  Observation,
  Piece,
  PiecePatch,
  SettingsPatch,
} from '../api/types'
import { isValidTimeZone, toIso } from '../utils/time'
import {
  addSpec,
  buildScenario,
  healthOf,
  isLearning,
  lowestFreeNumber,
  onShelf,
  refreshPieceObservations,
  controlOf,
  shelfOf,
  statusOf,
  withPrediction,
  type MockWorld,
  type ScenarioName,
} from './fixtures'
import { DEFAULT_REFERENCE_JSON } from './reference'

export interface MockRequest {
  method: string
  /** Path after /api/v1, e.g. "/pieces/12". */
  path: string
  query: URLSearchParams
  /** Parsed JSON body (or undefined). */
  json?: unknown
  /** Raw body bytes (photo upload). */
  bytes?: Uint8Array
  contentType?: string
}

export interface MockResponse {
  status: number
  headers: Record<string, string>
  body: string | Uint8Array | null
}

type Listener = (scopes: ChangeScope[], pieceIds: number[]) => void

class HttpError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly fields: FieldError[] = [],
  ) {
    super(message)
  }
}

const H = 3_600_000
const json = (status: number, data: unknown): MockResponse => ({
  status,
  headers: { 'Content-Type': 'application/json; charset=utf-8' },
  body: data === undefined ? null : JSON.stringify(data),
})

const invalid = (fields: FieldError[]) => new HttpError(422, 'validation_failed', `${fields.length} field${fields.length === 1 ? ' is' : 's are'} invalid`, fields)

function checkKeys(body: Record<string, unknown>, allowed: string[], prefix = ''): void {
  const bad = Object.keys(body).filter((k) => !allowed.includes(k))
  if (bad.length) throw invalid(bad.map((k) => ({ field: prefix + k, code: 'unknown_field', message: 'unknown field' })))
}

function asObject(v: unknown): Record<string, unknown> {
  if (!v || typeof v !== 'object' || Array.isArray(v)) throw new HttpError(400, 'bad_request', 'Expected a JSON object')
  return v as Record<string, unknown>
}

function checkDryness(v: unknown, field: string, errs: FieldError[]): number | undefined {
  if (v === undefined || v === null) return undefined
  if (typeof v !== 'number' || !Number.isFinite(v)) {
    errs.push({ field, code: 'invalid_value', message: 'must be a number' })
    return undefined
  }
  if (v < 0 || v > 5) {
    errs.push({ field, code: 'out_of_range', message: 'must be between 0 and 5' })
    return undefined
  }
  return Math.round(v * 10) / 10
}

function checkTime(v: unknown, field: string, now: number, errs: FieldError[]): number | undefined {
  if (v === undefined || v === null) return undefined
  const t = typeof v === 'string' ? Date.parse(v) : NaN
  if (Number.isNaN(t)) {
    errs.push({ field, code: 'invalid_time', message: 'must be an RFC 3339 time' })
    return undefined
  }
  if (t > now + 2 * 60_000) {
    errs.push({ field, code: 'in_future', message: 'must not be in the future' })
    return undefined
  }
  return t
}

// ---------------------------------------------------------------------------

export function createMockBackend(scenario: ScenarioName = 'calibrated', nowFn: () => number = Date.now) {
  let world: MockWorld = buildScenario(scenario, nowFn())
  let current: ScenarioName = scenario
  const listeners = new Set<Listener>()

  function reset(name: ScenarioName) {
    current = name
    world = buildScenario(name, nowFn())
    emit(['pieces', 'observations', 'predictions', 'settings', 'climate_log'], [])
  }

  function emit(scopes: ChangeScope[], ids: number[]) {
    listeners.forEach((l) => l(scopes, ids))
  }

  function subscribe(l: Listener) {
    listeners.add(l)
    return () => listeners.delete(l)
  }

  /** Keep ok sensors fresh and gently moving. */
  function tickSensors() {
    const now = nowFn()
    for (const s of world.sensors) {
      if (s.state !== 'ok') {
        s.age_seconds = s.read_at ? Math.round((now - Date.parse(s.read_at)) / 1000) : null
        continue
      }
      const base = 81 + 2 * Math.sin(now / (3 * H)) + s.offset_pct - 1.5
      s.humidity_pct = Math.round((base + Math.sin(now / 47_000) * 0.3) * 10) / 10
      s.temperature_c = Math.round((18.5 + Math.sin(now / (12 * H))) * 10) / 10
      s.read_at = toIso(now - 1000)
      s.age_seconds = 1
    }
  }

  function piece(id: number): Piece {
    const p = world.pieces.find((x) => x.id === id && !x.mistake)
    // Mistakes stay reachable by id so Undo works (contract §3.3).
    const any = p ?? world.pieces.find((x) => x.id === id)
    if (!any) throw new HttpError(404, 'not_found', 'Piece not found')
    return any
  }

  function addObservation(p: Piece, dryness: number, at: number, source: Observation['source']): Observation & { deleted: boolean } {
    const now = nowFn()
    const prev = world.observations
      .filter((o) => o.piece_id === p.id && !o.deleted)
      .sort((a, b) => a.observed_at.localeCompare(b.observed_at))
      .at(-1)
    const o = {
      id: ++world.seq.obs,
      piece_id: p.id,
      dryness,
      observed_at: toIso(at),
      source,
      flag: prev && dryness < prev.dryness - 0.5 ? ('inconsistent' as const) : null,
      note: '',
      created_at: toIso(now),
      deleted: false,
    }
    world.observations.push(o)
    refreshPieceObservations(world, p)
    return o
  }

  const publicObs = (o: Observation & { deleted: boolean }): Observation => {
    const { deleted: _deleted, ...rest } = o
    return rest
  }

  // ---- handlers -----------------------------------------------------------

  function patchOne(p: Piece, body: PiecePatch, now: number, errs: FieldError[], prefix: string): void {
    const target = body.state
    const t = (k: 'flipped_at' | 'ended_at' | 'added_at') => checkTime(body[k], prefix + k, now, errs)
    if (target === undefined || target === p.state) {
      // Correction (or a no-op state change, which makes retries safe).
      if (body.form) p.form = body.form
      if (body.size) p.size = body.size
      if (body.thickness) p.thickness = body.thickness
      if (body.label !== undefined) p.label = body.label
      if (body.notes !== undefined) p.notes = body.notes
      if (body.photo_id !== undefined) p.photo_id = body.photo_id
      if (body.end_note !== undefined) p.end_note = body.end_note
      if (body.mistake !== undefined) p.mistake = body.mistake
      const added = t('added_at')
      if (added !== undefined) p.added_at = toIso(added)
      const init = checkDryness(body.initial_dryness, prefix + 'initial_dryness', errs)
      if (init !== undefined) {
        p.initial_dryness = init
        const addObs = world.observations.find((o) => o.piece_id === p.id && o.source === 'add')
        if (addObs) addObs.dryness = init
      }
      const flipped = t('flipped_at')
      if (flipped !== undefined && p.state === 'flipped') p.flipped_at = toIso(flipped)
      const ended = t('ended_at')
      if (ended !== undefined && p.ended_at) p.ended_at = toIso(ended)
      const ed = checkDryness(body.end_dryness, prefix + 'end_dryness', errs)
      if (ed !== undefined && p.ended_at) {
        p.end_dryness = ed
        const endObs = world.observations.find((o) => o.piece_id === p.id && o.source === 'end' && !o.deleted)
        if (endObs) endObs.dryness = ed
      }
      refreshPieceObservations(world, p)
      return
    }
    const from = p.state
    if (from === 'in' && target === 'flipped') {
      const at = t('flipped_at') ?? now
      const d = checkDryness(body.flip_dryness, prefix + 'flip_dryness', errs)
      if (errs.length) return
      p.state = 'flipped'
      p.flipped_at = toIso(at)
      p.flip_dryness = d ?? null
      if (d !== undefined) addObservation(p, d, at, 'flip')
      return
    }
    if ((from === 'in' || from === 'flipped') && (target === 'trimmed' || target === 'discarded')) {
      const at = t('ended_at') ?? now
      let d = checkDryness(body.end_dryness, prefix + 'end_dryness', errs)
      if (target === 'trimmed' && d === undefined) d = 3.0
      if (errs.length) return
      p.state = target
      p.ended_at = toIso(at)
      p.end_dryness = d ?? null
      p.end_note = body.end_note ?? ''
      p.mistake = body.mistake ?? false
      if (d !== undefined) addObservation(p, d, at, 'end')
      return
    }
    if (from === 'flipped' && target === 'in') {
      p.state = 'in'
      p.flipped_at = null
      p.flip_dryness = null
      world.observations.filter((o) => o.piece_id === p.id && o.source === 'flip').forEach((o) => (o.deleted = true))
      refreshPieceObservations(world, p)
      return
    }
    if ((from === 'trimmed' || from === 'discarded') && (target === 'in' || target === 'flipped')) {
      const before = p.flipped_at ? 'flipped' : 'in'
      if (target !== before) throw new HttpError(409, 'invalid_transition', `Can't go from ${from} to ${target}`)
      // Back on the shelf: reuse the number unless it's taken now.
      const taken = onShelf(world).some((x) => x.number === p.number && x.id !== p.id)
      p.state = before
      p.ended_at = null
      p.end_dryness = null
      p.end_note = ''
      p.mistake = false
      if (taken) p.number = lowestFreeNumber(world.pieces)
      world.observations.filter((o) => o.piece_id === p.id && o.source === 'end').forEach((o) => (o.deleted = true))
      refreshPieceObservations(world, p)
      return
    }
    if ((from === 'trimmed' && target === 'discarded') || (from === 'discarded' && target === 'trimmed')) {
      p.state = target
      const d = checkDryness(body.end_dryness, prefix + 'end_dryness', errs)
      if (d !== undefined) p.end_dryness = d
      if (body.end_note !== undefined) p.end_note = body.end_note
      if (body.mistake !== undefined) p.mistake = body.mistake
      return
    }
    throw new HttpError(409, 'invalid_transition', `Can't go from ${from} to ${target}`)
  }

  const PATCH_KEYS = ['state', 'flipped_at', 'flip_dryness', 'ended_at', 'end_dryness', 'end_note', 'mistake', 'form', 'label', 'size', 'thickness', 'notes', 'photo_id', 'added_at', 'initial_dryness']

  function readings(q: URLSearchParams, now: number) {
    const parseWhen = (v: string | null, def: number) => {
      if (!v) return def
      const m = /^-(\d+)([hd])$/.exec(v)
      if (m) return now - Number(m[1]) * (m[2] === 'h' ? H : 24 * H)
      const t = Date.parse(v)
      if (Number.isNaN(t)) throw invalid([{ field: 'from', code: 'invalid_time', message: 'bad time' }])
      return t
    }
    const from = parseWhen(q.get('from'), now - 6 * H)
    const to = parseWhen(q.get('to'), now)
    const steps = [60, 300, 900, 3600, 21600, 86400]
    const named: Record<string, number> = { '1m': 60, '5m': 300, '15m': 900, '1h': 3600, '6h': 21600, '1d': 86400 }
    const resQ = q.get('resolution') ?? 'auto'
    let res = resQ === 'auto' ? (steps.find((s) => (to - from) / 1000 / s <= 360) ?? 86400) : named[resQ]
    if (!res) throw invalid([{ field: 'resolution', code: 'invalid_enum', message: 'unknown resolution' }])
    if ((to - from) / 1000 / res > 2000) throw invalid([{ field: 'resolution', code: 'too_many_points', message: 'too many points' }])
    res = Math.max(60, res)
    const t: number[] = []
    const hum: (number | null)[] = []
    const hmin: (number | null)[] = []
    const hmax: (number | null)[] = []
    const temp: (number | null)[] = []
    const vpd: (number | null)[] = []
    const samples: number[] = []
    const start = Math.floor(from / 1000 / res) * res
    const sensorDead = world.sensors.every((s) => s.state !== 'ok')
    for (let s = start; s * 1000 <= to; s += res) {
      const ms = s * 1000
      t.push(s)
      const off = world.offline && ms >= world.offline.from && ms < world.offline.to
      const dead = sensorDead && ms > now - 5 * 60_000
      if (off || dead || ms > now) {
        for (const arr of [hum, hmin, hmax, temp, vpd]) arr.push(null)
        samples.push(0)
        continue
      }
      const rh = Math.round((81 + 2 * Math.sin(ms / (3 * H)) + 1.5 * Math.sin(ms / (29 * H))) * 10) / 10
      const tc = Math.round((18.5 + Math.sin(ms / (12 * H))) * 10) / 10
      const svp = 0.6108 * Math.exp((17.27 * tc) / (tc + 237.3))
      hum.push(rh)
      hmin.push(Math.round((rh - 0.4) * 10) / 10)
      hmax.push(Math.round((rh + 0.4) * 10) / 10)
      temp.push(tc)
      vpd.push(Math.round(svp * (1 - rh / 100) * 100) / 100)
      samples.push(Math.round(res / 60))
    }
    return {
      from: toIso(from),
      to: toIso(to),
      resolution_seconds: res,
      t,
      series: [{ sensor: q.get('sensors') ?? 'room', humidity_avg: hum, humidity_min: hmin, humidity_max: hmax, temperature_avg: temp, vpd_avg: vpd, samples }],
    }
  }

  function photoSvg(hue: number, id: number): string {
    return `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="240" viewBox="0 0 320 240"><rect width="320" height="240" fill="hsl(${hue} 25% 80%)"/><ellipse cx="160" cy="200" rx="90" ry="14" fill="hsl(${hue} 20% 55%)"/><path d="M90 80 Q160 60 230 80 L215 195 Q160 210 105 195 Z" fill="hsl(${hue} 30% 45%)"/><text x="12" y="28" font-family="sans-serif" font-size="18" fill="#222">photo ${id}</text></svg>`
  }

  function route(req: MockRequest): MockResponse {
    const now = nowFn()
    const { method, path, query: q } = req
    const seg = path.split('/').filter(Boolean)
    const id = seg[1] !== undefined ? Number(seg[1]) : NaN

    if (method === 'GET' && path === '/health') return json(200, healthOf(world, now))
    if (method === 'GET' && path === '/reference') return { status: 200, headers: { 'Content-Type': 'application/json; charset=utf-8' }, body: DEFAULT_REFERENCE_JSON }
    if (method === 'GET' && path === '/status') {
      tickSensors()
      return json(200, statusOf(world, now))
    }
    if (method === 'GET' && path === '/shelf') return json(200, shelfOf(world, now))
    if (method === 'GET' && path === '/settings') return json(200, world.settings)
    if (method === 'GET' && path === '/readings') return json(200, readings(q, now))
    if (method === 'GET' && path === '/climate-log') {
      const from = q.get('from') ? Date.parse(q.get('from')!) : -Infinity
      const to = q.get('to') ? Date.parse(q.get('to')!) : Infinity
      return json(200, { items: world.climateLog.filter((e) => Date.parse(e.at) >= from && Date.parse(e.at) <= to) })
    }
    if (method === 'GET' && path === '/backup') {
      return {
        status: 200,
        headers: { 'Content-Type': 'application/vnd.sqlite3', 'Content-Disposition': `attachment; filename="humidity-mock.db"` },
        body: new TextEncoder().encode('SQLite format 3\u0000 (mock backup)'),
      }
    }

    // ---- pieces
    if (seg[0] === 'pieces' && seg.length === 1 && method === 'GET') {
      const state = q.get('state') ?? 'on_shelf'
      if (state === 'on_shelf') return json(200, { items: onShelf(world).map((p) => withPrediction(world, p, now)) })
      const all = world.pieces.filter((p) => !p.mistake && (state === 'all' || p.state === 'trimmed' || p.state === 'discarded'))
      all.sort((a, b) => (b.ended_at ?? b.added_at).localeCompare(a.ended_at ?? a.added_at))
      const limit = Math.min(100, Number(q.get('limit') ?? 20) || 20)
      const offset = q.get('cursor') ? Number(atob(q.get('cursor')!)) : 0
      const items = all.slice(offset, offset + limit).map((p) => withPrediction(world, p, now))
      const next = offset + limit < all.length ? btoa(String(offset + limit)) : null
      return json(200, { items, next_cursor: next })
    }
    if (seg[0] === 'pieces' && seg.length === 1 && method === 'POST') {
      const body = asObject(req.json)
      checkKeys(body, ['quantity', 'form', 'initial_dryness', 'size', 'thickness', 'photo_id', 'label', 'notes', 'added_at'])
      const b = body as unknown as AddPieces
      const errs: FieldError[] = []
      for (const k of ['form', 'initial_dryness', 'size', 'thickness'] as const) if (b[k] === undefined) errs.push({ field: k, code: 'required', message: 'required' })
      const qty = b.quantity ?? 1
      if (!Number.isInteger(qty) || qty < 1 || qty > 20) errs.push({ field: 'quantity', code: 'out_of_range', message: 'must be 1–20' })
      const d = checkDryness(b.initial_dryness, 'initial_dryness', errs)
      const at = checkTime(b.added_at, 'added_at', now, errs) ?? now
      if (b.photo_id != null && !world.photos.has(b.photo_id)) errs.push({ field: 'photo_id', code: 'photo_not_found', message: 'photo not found' })
      if ((b.label ?? '').length > 80) errs.push({ field: 'label', code: 'too_long', message: 'at most 80 characters' })
      if (errs.length) throw invalid(errs)
      const created: Piece[] = []
      for (let i = 0; i < qty; i++) {
        const p = addSpec(world, now, {
          form: b.form, size: b.size, thickness: b.thickness, label: b.label ?? '',
          added: (now - at) / H, initial: d ?? 1,
        })
        p.notes = b.notes ?? ''
        p.photo_id = b.photo_id ?? null
        if (qty > 1) {
          p.group_id = created[0]?.id ?? p.id
          p.group_size = qty
        }
        created.push(p)
      }
      emit(['pieces', 'predictions'], created.map((p) => p.id))
      return json(201, { items: created.map((p) => withPrediction(world, p, now)) })
    }
    if (seg[0] === 'pieces' && seg.length === 1 && method === 'PATCH') {
      const body = asObject(req.json)
      checkKeys(body, [...PATCH_KEYS, 'ids'])
      const ids = body.ids
      if (!Array.isArray(ids) || ids.length < 1 || ids.length > 50) throw invalid([{ field: 'ids', code: 'invalid_value', message: '1–50 ids' }])
      const { ids: _ids, ...rest } = body
      // All or nothing: work on copies first.
      const snapshotPieces = structuredClone(world.pieces)
      const snapshotObs = structuredClone(world.observations)
      try {
        const errs: FieldError[] = []
        const out = (ids as number[]).map((pid, i) => {
          const p = piece(pid)
          patchOne(p, rest as PiecePatch, now, errs, '')
          if (errs.length) throw invalid(errs.map((e) => ({ ...e, field: `ids[${i}].${e.field}` })))
          return p
        })
        emit(['pieces', 'observations', 'predictions'], ids as number[])
        return json(200, { items: out.map((p) => withPrediction(world, p, now)) })
      } catch (e) {
        world.pieces = snapshotPieces
        world.observations = snapshotObs
        throw e
      }
    }
    if (seg[0] === 'pieces' && seg.length === 2 && !Number.isNaN(id)) {
      if (method === 'GET') return json(200, withPrediction(world, piece(id), now))
      if (method === 'PATCH') {
        const body = asObject(req.json)
        checkKeys(body, PATCH_KEYS)
        const p = piece(id)
        const errs: FieldError[] = []
        const before = structuredClone(p)
        patchOne(p, body as PiecePatch, now, errs, '')
        if (errs.length) {
          Object.assign(p, before)
          throw invalid(errs)
        }
        emit(['pieces', 'observations', 'predictions'], [id])
        return json(200, withPrediction(world, p, now))
      }
    }
    if (seg[0] === 'pieces' && seg[2] === 'observations' && method === 'GET') {
      piece(id)
      const items = world.observations.filter((o) => o.piece_id === id && !o.deleted).sort((a, b) => a.observed_at.localeCompare(b.observed_at)).map(publicObs)
      return json(200, { items })
    }

    // ---- photos
    if (seg[0] === 'photos' && seg.length === 1 && method === 'POST') {
      if (!(req.contentType ?? '').startsWith('image/jpeg')) throw new HttpError(415, 'unsupported_media_type', 'Photo must be image/jpeg')
      const bytes = req.bytes ?? new Uint8Array()
      if (bytes.length > 3 * 1024 * 1024) throw new HttpError(413, 'too_large', 'Photo over 3 MB')
      if (!bytes.length) throw invalid([{ field: 'body', code: 'required', message: 'empty body' }])
      const pid = ++world.seq.photo
      world.photos.set(pid, { type: 'image/jpeg', bytes, hue: 0 })
      return json(201, { id: pid, width: 1600, height: 1200, created_at: toIso(now) })
    }
    if (seg[0] === 'photos' && seg.length === 2 && method === 'GET') {
      const ph = world.photos.get(id)
      if (!ph) throw new HttpError(404, 'not_found', 'Photo not found')
      const headers = { 'Content-Type': ph.type, 'Cache-Control': 'private, max-age=31536000, immutable' }
      return { status: 200, headers, body: ph.bytes ?? photoSvg(ph.hue, id) }
    }

    // ---- observations
    if (seg[0] === 'observations' && seg.length === 1 && method === 'POST') {
      const body = asObject(req.json)
      checkKeys(body, ['observed_at', 'items', 'note'])
      const b = body as unknown as NewObservations
      const errs: FieldError[] = []
      if (!Array.isArray(b.items) || b.items.length < 1 || b.items.length > 50) throw invalid([{ field: 'items', code: 'invalid_value', message: '1–50 items' }])
      const at = checkTime(b.observed_at, 'observed_at', now, errs) ?? now
      const seen = new Set<number>()
      b.items.forEach((it, i) => {
        checkKeys(it as unknown as Record<string, unknown>, ['piece_id', 'dryness'], `items[${i}].`)
        checkDryness(it.dryness, `items[${i}].dryness`, errs)
        if (it.dryness === undefined) errs.push({ field: `items[${i}].dryness`, code: 'required', message: 'required' })
        if (seen.has(it.piece_id)) errs.push({ field: `items[${i}].piece_id`, code: 'duplicate', message: 'duplicate piece' })
        seen.add(it.piece_id)
      })
      if (errs.length) throw invalid(errs)
      const pieces = b.items.map((it) => piece(it.piece_id))
      if (pieces.some((p) => p.state === 'trimmed' || p.state === 'discarded')) throw new HttpError(409, 'piece_ended', 'That piece has left the shelf')
      const created: Observation[] = []
      const followUps = []
      for (const [i, it] of b.items.entries()) {
        const p = pieces[i]!
        const prevObs = world.observations.filter((o) => o.piece_id === p.id && !o.deleted).sort((a, c) => a.observed_at.localeCompare(c.observed_at)).at(-1)
        const predicted = withPrediction(world, p, now).prediction?.dryness_now ?? null
        const d = Math.round(it.dryness * 10) / 10
        const o = addObservation(p, d, at, 'user')
        created.push(publicObs(o))
        if (world.followUpsAsked.has(p.id)) continue
        if (prevObs && d <= prevObs.dryness - 1.5) {
          world.followUpsAsked.add(p.id)
          followUps.push({
            kind: 'went_wetter' as const, piece_id: p.id, observation_id: o.id, previous_observation_id: prevObs.id,
            question: 'Wetter than last time. What happened?',
            options: [
              { key: 'last_entry_wrong', label: 'Last entry was wrong', action: 'correct_observation' as const },
              { key: 'its_fine', label: "It's fine", action: 'none' as const },
            ],
          })
        } else if (!isLearning(world) && predicted !== null && predicted - d >= 1.5) {
          world.followUpsAsked.add(p.id)
          followUps.push({
            kind: 'much_slower' as const, piece_id: p.id, observation_id: o.id, previous_observation_id: prevObs?.id ?? null,
            question: 'Much wetter than expected. Is it still drying on the shelf?',
            options: [
              { key: 'still_drying', label: 'Still drying', action: 'none' as const },
              { key: 'covered_or_removed', label: 'I covered it / took it off', action: 'end_piece' as const },
            ],
          })
        }
      }
      emit(['observations', 'predictions'], pieces.map((p) => p.id))
      return json(201, { items: created, follow_ups: followUps })
    }
    if (seg[0] === 'observations' && seg.length === 1 && method === 'GET') {
      const all = world.observations.filter((o) => !o.deleted).sort((a, b) => b.observed_at.localeCompare(a.observed_at))
      const limit = Math.min(100, Number(q.get('limit') ?? 20) || 20)
      const offset = q.get('cursor') ? Number(atob(q.get('cursor')!)) : 0
      return json(200, { items: all.slice(offset, offset + limit).map(publicObs), next_cursor: offset + limit < all.length ? btoa(String(offset + limit)) : null })
    }
    if (seg[0] === 'observations' && seg.length >= 2 && !Number.isNaN(id)) {
      const o = world.observations.find((x) => x.id === id)
      if (!o) throw new HttpError(404, 'not_found', 'Observation not found')
      if (seg[2] === 'undelete' && method === 'POST') {
        o.deleted = false
        refreshPieceObservations(world, piece(o.piece_id))
        emit(['observations', 'predictions'], [o.piece_id])
        return json(200, publicObs(o))
      }
      if (seg[2] === 'follow-up' && method === 'POST') {
        const body = asObject(req.json)
        checkKeys(body, ['answer'])
        if (typeof body.answer !== 'string' || !body.answer) throw invalid([{ field: 'answer', code: 'required', message: 'required' }])
        return json(200, {})
      }
      if (o.deleted) throw new HttpError(404, 'not_found', 'Observation not found')
      if (method === 'PATCH') {
        const body = asObject(req.json)
        checkKeys(body, ['dryness', 'observed_at', 'note'])
        const errs: FieldError[] = []
        const d = checkDryness(body.dryness, 'dryness', errs)
        const t = checkTime(body.observed_at, 'observed_at', now, errs)
        if (errs.length) throw invalid(errs)
        if (d !== undefined) o.dryness = d
        if (t !== undefined) o.observed_at = toIso(t)
        if (typeof body.note === 'string') o.note = body.note
        refreshPieceObservations(world, piece(o.piece_id))
        emit(['observations', 'predictions'], [o.piece_id])
        return json(200, publicObs(o))
      }
      if (method === 'DELETE') {
        const live = world.observations.filter((x) => x.piece_id === o.piece_id && !x.deleted)
        if (live.length <= 1) throw new HttpError(409, 'last_observation', "Can't delete a piece's only observation")
        o.deleted = true
        refreshPieceObservations(world, piece(o.piece_id))
        emit(['observations', 'predictions'], [o.piece_id])
        return { status: 204, headers: {}, body: null }
      }
    }

    // ---- settings
    if (path === '/settings' && method === 'PATCH') {
      const body = asObject(req.json)
      checkKeys(body, ['studio_tz', 'sensors'])
      const b = body as SettingsPatch
      const errs: FieldError[] = []
      if (b.studio_tz !== undefined && !isValidTimeZone(b.studio_tz)) errs.push({ field: 'studio_tz', code: 'invalid_value', message: 'unknown time zone' })
      b.sensors?.forEach((s, i) => {
        if (s.offset_pct < -10 || s.offset_pct > 10) errs.push({ field: `sensors[${i}].offset_pct`, code: 'out_of_range', message: 'must be between −10 and +10' })
      })
      if (errs.length) throw invalid(errs)
      const st = world.settings
      if (b.studio_tz !== undefined) st.studio_tz = b.studio_tz
      for (const s of b.sensors ?? []) {
        const stored = st.sensors.find((x) => x.sensor === s.sensor)
        const prev = stored?.offset_pct ?? 0
        if (stored) {
          stored.offset_pct = s.offset_pct
          stored.calibrated_at = toIso(now)
        }
        const live = world.sensors.find((x) => x.sensor === s.sensor)
        if (live) {
          if (live.humidity_pct !== null) live.humidity_pct = Math.round((live.humidity_pct + s.offset_pct - prev) * 10) / 10
          live.offset_pct = s.offset_pct
        }
        world.climateLog.push(
          s.replaced
            ? { id: ++world.seq.log, kind: 'sensor_replaced', value: { sensor: s.sensor }, at: toIso(now) }
            : { id: ++world.seq.log, kind: 'sensor_calibrated', value: { sensor: s.sensor, offset_pct: s.offset_pct, previous_offset_pct: prev }, at: toIso(now) },
        )
      }
      emit(['settings', 'predictions', 'climate_log'], [])
      return json(200, st)
    }

    if (method === 'GET' && path === '/control') return json(200, controlOf(world, now))
    if (method === 'PUT' && path === '/control/setpoint') {
      const pct = (asObject(req.json) as { target_humidity_pct?: number | null }).target_humidity_pct
      if (pct === undefined || (pct !== null && (typeof pct !== 'number' || pct < 50 || pct > 98))) {
        throw new HttpError(422, 'validation_failed', 'Invalid target', [{ field: 'target_humidity_pct', code: 'out_of_range', message: 'must be between 50 and 98' }])
      }
      const prev = world.target?.pct ?? null
      const next = pct === null ? null : Math.round(pct * 10) / 10
      if (prev !== next) {
        world.target = next === null ? null : { pct: next, setAt: now }
        world.climateLog.push({ id: ++world.seq.log, kind: 'setpoint_set', value: { target_humidity_pct: next, previous_pct: prev }, at: toIso(now) })
        emit(['control', 'predictions', 'climate_log'], [])
      }
      return json(200, controlOf(world, now))
    }
    if (method === 'GET' && path === '/control/preview') {
      // The mock has no climate model: a damper target than the room only
      // shifts the visit later by a rough amount.
      const pct = q.get('target_humidity_pct') ? Number(q.get('target_humidity_pct')) : null
      const visit = shelfOf(world, now).next_visit
      const room = world.sensors[0]?.humidity_pct ?? 80
      const shiftH = pct === null ? 0 : Math.max(-12, (pct - room) * 1.5)
      const shift = (iso: string) => toIso(Date.parse(iso) + shiftH * 3_600_000)
      return json(200, {
        target_humidity_pct: pct,
        forecast_temp_c: 18.6,
        next_visit: visit && { ...visit, start: shift(visit.start), end: shift(visit.end), recommended_at: shift(visit.recommended_at) },
        ready_now_count: statusOf(world, now).trim.ready_now_count,
      })
    }
    if (method === 'POST' && path === '/control/override') {
      const seconds = (asObject(req.json) as { seconds?: number }).seconds
      if (typeof seconds !== 'number' || seconds < 1 || seconds > 60) {
        throw new HttpError(422, 'validation_failed', 'Invalid manual run', [{ field: 'seconds', code: 'out_of_range', message: 'must be between 1 and 60' }])
      }
      world.pump.manualUntil = now + seconds * 1000
      world.pump.lastRunAt = now
      world.pump.runtime += seconds
      world.climateLog.push({ id: ++world.seq.log, kind: 'pump_manual', value: { seconds }, at: toIso(now) })
      emit(['control', 'climate_log'], [])
      return json(200, controlOf(world, now))
    }
    if (method === 'DELETE' && path === '/control/override') {
      world.pump.manualUntil = 0
      emit(['control'], [])
      return json(200, controlOf(world, now))
    }
    if (method === 'POST' && path === '/control/reset') {
      if (world.pump.tripped) world.climateLog.push({ id: ++world.seq.log, kind: 'pump_reset', value: { reason: 'not_rising' }, at: toIso(now) })
      world.pump.tripped = false
      emit(['control', 'climate_log'], [])
      return json(200, controlOf(world, now))
    }
    throw new HttpError(404, 'not_found', 'Unknown route')
  }

  function handle(req: MockRequest): MockResponse {
    try {
      return route(req)
    } catch (e) {
      if (e instanceof HttpError) {
        return json(e.status, { error: { code: e.code, message: e.message, ...(e.fields.length ? { fields: e.fields } : {}) } })
      }
      return json(500, { error: { code: 'internal', message: String(e) } })
    }
  }

  return {
    handle,
    reset,
    subscribe,
    tickSensors,
    get world() {
      return world
    },
    get scenario() {
      return current
    },
    status: () => statusOf(world, nowFn()),
    hello: () => ({
      server_time: toIso(nowFn()),
      version: world.version,
      api_version: 1,
      poll_interval_seconds: 5,
      stale_after_seconds: 30,
      clock_synced: world.clockSynced,
    }),
    readingsEvent: () => {
      tickSensors()
      const s = statusOf(world, nowFn())
      return { server_time: s.server_time, room: s.room, sensors: s.sensors }
    },
  }
}

export type MockBackend = ReturnType<typeof createMockBackend>
