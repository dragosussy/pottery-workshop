// Contract-conformant fixtures (api-contract.md rev. 3) for every state the UI
// must handle. Used by the dev mock server (`npm run dev:mock`) and by tests.
// Every scenario is built relative to a `now`, so the dev mock always looks
// current and tests can pin a fixed instant.
//
// Only relative imports here: this file is also compiled for the Vite config.

import type {
  ClimateLogEntry,
  FormKey,
  Health,
  LiveStatus,
  Observation,
  Piece,
  PieceState,
  Room,
  SensorStatus,
  Settings,
  Shelf,
  SizeKey,
  StageKey,
  ThicknessKey,
} from '../api/types'
import { nearestStage } from '../utils/dryness'
import { toIso } from '../utils/time'
import { checkThese, planVisit, predict } from './model'

const H = 3_600_000
const D = 24 * H

export const SCENARIOS = [
  'calibrated',
  'learning',
  'empty',
  'ready_now',
  'back_from_offline',
  'stale_sensor',
] as const
export type ScenarioName = (typeof SCENARIOS)[number]

export const SCENARIO_NOTES: Record<ScenarioName, string> = {
  calibrated: 'Learned model, 8 pieces in mixed states',
  learning: 'Learning period (2 of 5), 6 pieces',
  empty: 'Nothing on the shelf; history only',
  ready_now: '3 pieces ready now',
  back_from_offline: 'Back on after 9 days, 3 pieces to check',
  stale_sensor: 'Sensor failing: stale climate',
}

export interface StoredPhoto {
  type: string
  bytes: Uint8Array | null
  hue: number
}

export interface MockWorld {
  version: string
  startedAt: number
  settings: Settings
  pieces: Piece[]
  observations: (Observation & { deleted: boolean })[]
  climateLog: ClimateLogEntry[]
  sensors: SensorStatus[]
  learningSamples: number
  offline: { from: number; to: number } | null
  clockSynced: boolean
  photos: Map<number, StoredPhoto>
  /** Per-piece speed factor for the mock model (1 = prior). */
  speed: Map<number, number>
  followUpsAsked: Set<number>
  seq: { piece: number; obs: number; photo: number; log: number }
}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

export function baseSettings(over: Partial<Settings> = {}): Settings {
  return {
    studio_tz: 'Europe/Bucharest',
    sensors: [{ sensor: 'main', offset_pct: 1.5, calibrated_at: '2026-10-20T12:00:00Z' }],
    ...over,
  }
}

function emptyWorld(now: number, settings: Settings): MockWorld {
  return {
    version: '0.4.0-mock',
    startedAt: now - 6 * H,
    settings,
    pieces: [],
    observations: [],
    climateLog: [
      { id: 1, kind: 'sensor_calibrated', value: { sensor: 'main', offset_pct: 1.5, previous_offset_pct: 0 }, at: '2026-10-20T12:00:00Z' },
    ],
    sensors: [
      {
        sensor: 'main', state: 'ok', humidity_pct: 82.4, temperature_c: 18.6, offset_pct: 1.5,
        read_at: toIso(now - 2000), age_seconds: 2, failing_since: null, last_error: null,
      },
    ],
    learningSamples: 12,
    offline: null,
    clockSynced: true,
    photos: new Map(),
    speed: new Map(),
    followUpsAsked: new Set(),
    seq: { piece: 100, obs: 1000, photo: 10, log: 10 },
  }
}

export interface PieceSpec {
  number?: number
  form?: FormKey
  size?: SizeKey
  thickness?: ThicknessKey
  label?: string
  /** Hours before now it was added. */
  added: number
  initial: number
  /** [hoursAgo, dryness] user observations. */
  obs?: [number, number][]
  flipped?: [number, number | null]
  state?: PieceState
  ended?: [number, number | null]
  endNote?: string
  photo?: boolean
  group?: number
  speed?: number
}

/** The lowest number not used by anything currently on the shelf. */
export function lowestFreeNumber(pieces: Piece[]): number {
  const used = new Set(pieces.filter((p) => p.state === 'in' || p.state === 'flipped').map((p) => p.number))
  let n = 1
  while (used.has(n)) n++
  return n
}

export function addSpec(w: MockWorld, now: number, spec: PieceSpec): Piece {
  const id = ++w.seq.piece
  const addedAt = now - spec.added * H
  let photoId: number | null = null
  if (spec.photo) {
    photoId = ++w.seq.photo
    w.photos.set(photoId, { type: 'image/svg+xml', bytes: null, hue: (id * 47) % 360 })
  }
  const state: PieceState = spec.state ?? (spec.flipped ? 'flipped' : 'in')
  const p: Piece = {
    id,
    number: spec.number ?? lowestFreeNumber(w.pieces),
    group_id: spec.group ?? null,
    group_size: 1,
    form: spec.form ?? 'mug',
    label: spec.label ?? '',
    size: spec.size ?? 'medium',
    thickness: spec.thickness ?? 'medium',
    photo_id: photoId,
    notes: '',
    state,
    added_at: toIso(addedAt),
    flipped_at: spec.flipped ? toIso(now - spec.flipped[0] * H) : null,
    flip_dryness: spec.flipped ? spec.flipped[1] : null,
    ended_at: spec.ended ? toIso(now - spec.ended[0] * H) : null,
    end_dryness: spec.ended ? spec.ended[1] : null,
    end_note: spec.endNote ?? '',
    mistake: false,
    initial_dryness: spec.initial,
    current_dryness: spec.initial,
    last_observed_at: toIso(addedAt),
    observation_count: 0,
    prediction: null,
  }
  const obs: Observation[] = []
  const push = (t: number, d: number, source: Observation['source']) =>
    obs.push({ id: ++w.seq.obs, piece_id: id, dryness: d, observed_at: toIso(t), source, flag: null, note: '', created_at: toIso(t + 4000) })
  push(addedAt, spec.initial, 'add')
  for (const [h, d] of spec.obs ?? []) push(now - h * H, d, 'user')
  if (spec.flipped && spec.flipped[1] !== null) push(now - spec.flipped[0] * H, spec.flipped[1], 'flip')
  if (spec.ended && spec.ended[1] !== null) push(now - spec.ended[0] * H, spec.ended[1], 'end')
  obs.sort((a, b) => a.observed_at.localeCompare(b.observed_at))
  w.observations.push(...obs.map((o) => ({ ...o, deleted: false })))
  w.pieces.push(p)
  if (spec.speed) w.speed.set(id, spec.speed)
  refreshPieceObservations(w, p)
  return p
}

/** Recompute the observation-derived fields of a piece. */
export function refreshPieceObservations(w: MockWorld, p: Piece): void {
  const obs = w.observations.filter((o) => o.piece_id === p.id && !o.deleted).sort((a, b) => a.observed_at.localeCompare(b.observed_at))
  const last = obs[obs.length - 1]
  p.observation_count = obs.length
  p.current_dryness = last ? last.dryness : p.initial_dryness
  p.last_observed_at = last ? last.observed_at : p.added_at
}

function addGroup(w: MockWorld, now: number, n: number, spec: PieceSpec): Piece[] {
  const first = addSpec(w, now, { ...spec, photo: spec.photo })
  first.group_id = first.id
  first.group_size = n
  const out = [first]
  for (let i = 1; i < n; i++) {
    const p = addSpec(w, now, { ...spec, photo: false })
    p.group_id = first.id
    p.group_size = n
    p.photo_id = first.photo_id
    out.push(p)
  }
  return out
}

function addHistory(w: MockWorld, now: number): void {
  // Finished pieces for /history (numbers were reused since).
  addSpec(w, now, { number: 1, form: 'bowl', size: 'medium', thickness: 'medium', added: 200, initial: 1.0, obs: [[170, 2.2], [150, 2.9]], state: 'trimmed', ended: [149, 3.0], photo: true })
  addSpec(w, now, { number: 2, form: 'mug', size: 'small', thickness: 'thin', added: 196, initial: 1.1, obs: [[180, 2.5]], state: 'trimmed', ended: [176, 3.1] })
  addSpec(w, now, { number: 3, form: 'plate', size: 'large', thickness: 'thick', added: 190, initial: 0.8, obs: [[160, 1.6]], state: 'discarded', ended: [158, null], endNote: 'Cracked at the rim' })
  addSpec(w, now, { number: 4, form: 'vase', size: 'large', thickness: 'medium', added: 120, initial: 1.0, obs: [[90, 2.4]], state: 'trimmed', ended: [80, 2.4], endNote: 'Covered to trim later' })
  addSpec(w, now, { number: 5, form: 'bowl', size: 'small', thickness: 'thin', added: 100, initial: 1.2, obs: [[85, 3.4]], state: 'trimmed', ended: [84, 3.4] })
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

function standardShelf(w: MockWorld, now: number): void {
  addGroup(w, now, 4, { form: 'mug', size: 'medium', thickness: 'medium', added: 30, initial: 1.0, obs: [[14, 2.0]], flipped: [10, 2.3], photo: true })
  addSpec(w, now, { form: 'bowl', size: 'large', thickness: 'thick', added: 50, initial: 0.9, obs: [[26, 1.6], [6, 2.0]], photo: true })
  addSpec(w, now, { form: 'vase', size: 'medium', thickness: 'thin', added: 20, initial: 1.0, obs: [[8, 2.4]], label: 'tall one' })
  addSpec(w, now, { form: 'plate', size: 'xlarge', thickness: 'thick', added: 8, initial: 0.6 })
  addSpec(w, now, { form: 'bowl', size: 'small', thickness: 'thin', added: 40, initial: 1.1, obs: [[20, 2.6], [2, 3.8]] })
}

export function buildScenario(name: ScenarioName, now: number): MockWorld {
  switch (name) {
    case 'calibrated': {
      const w = emptyWorld(now, baseSettings())
      addHistory(w, now)
      standardShelf(w, now)
      return w
    }
    case 'learning': {
      const w = emptyWorld(now, baseSettings())
      w.learningSamples = 2
      addHistory(w, now)
      addGroup(w, now, 3, { form: 'bowl', size: 'medium', thickness: 'medium', added: 26, initial: 1.0, obs: [[12, 1.8]], photo: true })
      addSpec(w, now, { form: 'mug', size: 'small', thickness: 'thin', added: 18, initial: 1.2, obs: [[3, 2.3]] })
      addSpec(w, now, { form: 'vase', size: 'large', thickness: 'thick', added: 30, initial: 0.8 })
      addSpec(w, now, { form: 'plate', size: 'medium', thickness: 'medium', added: 6, initial: 1.0, photo: true })
      return w
    }
    case 'empty': {
      const w = emptyWorld(now, baseSettings())
      addHistory(w, now)
      return w
    }
    case 'ready_now': {
      const w = emptyWorld(now, baseSettings())
      addHistory(w, now)
      addSpec(w, now, { form: 'mug', size: 'medium', thickness: 'thick', added: 40, initial: 1.0, obs: [[16, 2.2], [1, 2.9]], photo: true })
      addSpec(w, now, { form: 'mug', size: 'medium', thickness: 'thick', added: 40, initial: 1.0, obs: [[16, 2.3], [1, 3.0]], photo: true })
      addSpec(w, now, { form: 'bowl', size: 'large', thickness: 'medium', added: 44, initial: 1.0, obs: [[2, 3.1]] })
      addSpec(w, now, { form: 'vase', size: 'medium', thickness: 'medium', added: 10, initial: 1.0, obs: [[4, 1.8]] })
      return w
    }
    case 'back_from_offline': {
      const w = emptyWorld(now, baseSettings())
      w.offline = { from: now - 9 * D - 2 * H, to: now - 15 * 60_000 }
      w.startedAt = w.offline.to
      w.climateLog.push({ id: ++w.seq.log, kind: 'offline', value: { from: toIso(w.offline.from), to: toIso(w.offline.to) }, at: toIso(w.offline.to) })
      addHistory(w, now)
      addSpec(w, now, { form: 'mug', size: 'medium', thickness: 'medium', added: 9 * 24 + 30, initial: 1.0, obs: [[9 * 24 + 8, 2.1]], photo: true })
      addSpec(w, now, { form: 'bowl', size: 'large', thickness: 'thick', added: 9 * 24 + 30, initial: 0.9, obs: [[9 * 24 + 6, 1.5]] })
      addSpec(w, now, { form: 'vase', size: 'medium', thickness: 'thin', added: 9 * 24 + 20, initial: 1.0 })
      return w
    }
    case 'stale_sensor': {
      const w = emptyWorld(now, baseSettings())
      addHistory(w, now)
      standardShelf(w, now)
      w.sensors = [
        {
          sensor: 'main', state: 'stale', humidity_pct: 81.9, temperature_c: 18.4, offset_pct: 1.5,
          read_at: toIso(now - 4 * 60_000), age_seconds: 240, failing_since: toIso(now - 4 * 60_000), last_error: 'i2c: no ACK from 0x44',
        },
      ]
      return w
    }
  }
}

// ---------------------------------------------------------------------------
// Derived views (what the API returns)
// ---------------------------------------------------------------------------

export function isLearning(w: MockWorld): boolean {
  return w.learningSamples < 5
}

export function onShelf(w: MockWorld): Piece[] {
  return w.pieces.filter((p) => !p.mistake && (p.state === 'in' || p.state === 'flipped')).sort((a, b) => a.number - b.number)
}

/** A piece with a fresh prediction. */
export function withPrediction(w: MockWorld, p: Piece, now: number): Piece {
  return {
    ...p,
    prediction: predict(p, {
      now,
      learning: isLearning(w),
      offlineEndedAt: w.offline?.to ?? null,
      factor: w.speed.get(p.id),
      pastPieces: w.learningSamples,
    }),
  }
}

export function shelfOf(w: MockWorld, now: number): Shelf {
  const items = onShelf(w).map((p) => withPrediction(w, p, now))
  const next_visit = planVisit(items, now, isLearning(w) ? 'learning' : 'decent')
  return {
    computed_at: toIso(now),
    items,
    next_visit,
    check_these: checkThese(items, now, isLearning(w), w.offline?.to ?? null),
  }
}

export function roomOf(w: MockWorld): Room {
  const ok = w.sensors.filter((s) => s.state === 'ok')
  if (!ok.length) {
    const any = w.sensors[0]
    return {
      humidity_pct: any?.humidity_pct ?? null,
      temperature_c: any?.temperature_c ?? null,
      vpd_kpa: null,
      state: any ? 'stale' : 'no_data',
      read_at: any?.read_at ?? null,
    }
  }
  const rh = ok.reduce((a, s) => a + (s.humidity_pct ?? 0), 0) / ok.length
  const t = ok.reduce((a, s) => a + (s.temperature_c ?? 0), 0) / ok.length
  const svp = 0.6108 * Math.exp((17.27 * t) / (t + 237.3))
  return {
    humidity_pct: Math.round(rh * 10) / 10,
    temperature_c: Math.round(t * 10) / 10,
    vpd_kpa: Math.round(Math.max(0.05, svp * (1 - rh / 100)) * 100) / 100,
    state: 'ok',
    read_at: ok[0]!.read_at,
  }
}

export function statusOf(w: MockWorld, now: number): LiveStatus {
  const shelf = shelfOf(w, now)
  const by_stage = { very_wet: 0, wet: 0, soft_leather_hard: 0, leather_hard: 0, hard_leather_hard: 0, bone_dry: 0 } as Record<StageKey, number>
  for (const p of shelf.items) by_stage[nearestStage(p.current_dryness ?? 0).key]++
  const ready = shelf.items.filter((p) => p.prediction?.status === 'ready_now')
  const readyUntil = ready.map((p) => p.prediction?.ready_until).filter((x): x is string => !!x).sort()[0] ?? null
  const userObs = w.observations.filter((o) => !o.deleted && o.source === 'user').map((o) => o.observed_at).sort()
  const oldest = shelf.items.map((p) => p.added_at).sort()[0] ?? null
  const lastOffline =
    w.offline && now - w.offline.to < 7 * D
      ? {
          from: toIso(w.offline.from),
          to: toIso(w.offline.to),
          pieces_to_check: shelf.items.filter((p) => Date.parse(p.last_observed_at ?? p.added_at) < w.offline!.to).length,
        }
      : null
  return {
    server_time: toIso(now),
    poll_interval_seconds: 5,
    stale_after_seconds: 30,
    room: roomOf(w),
    sensors: w.sensors.map((s) => ({ ...s })),
    shelf: { on_shelf: shelf.items.length, oldest_added_at: oldest, by_stage, last_check_at: userObs[userObs.length - 1] ?? null },
    trim: {
      ready_now_count: ready.length,
      ready_now_until: readyUntil,
      check_count: shelf.check_these.length,
      late_count: shelf.items.filter((p) => p.prediction?.status === 'may_be_past_check' || p.prediction?.status === 'past_trim').length,
      next_visit: shelf.next_visit,
    },
    learning: { state: isLearning(w) ? 'learning' : 'learned', samples: Math.min(w.learningSamples, 5), needed: 5 },
    last_offline: lastOffline,
    control: null,
  }
}

export function healthOf(w: MockWorld, now: number): Health {
  const anyOk = w.sensors.some((s) => s.state === 'ok')
  return {
    status: anyOk ? 'ok' : 'degraded',
    version: w.version,
    commit: 'mock',
    built_at: toIso(w.startedAt),
    api_version: 1,
    server_time: toIso(now),
    clock_synced: w.clockSynced,
    started_at: toIso(w.startedAt),
    checks: { database: 'ok', sensors: anyOk ? 'ok' : 'degraded' },
    features: { control: false },
  }
}

/** A fixed instant for deterministic tests: Friday 2026-10-30 14:05:03 UTC (16:05 in Bucharest). */
export const FIXED_NOW = Date.parse('2026-10-30T14:05:03Z')

/** Convenience for tests: status + shelf + settings of a scenario at `now`. */
export function snapshot(name: ScenarioName, now = FIXED_NOW) {
  const w = buildScenario(name, now)
  return { world: w, status: statusOf(w, now), shelf: shelfOf(w, now), settings: w.settings, health: healthOf(w, now) }
}
