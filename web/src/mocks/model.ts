// A crude stand-in for the backend's prediction and visit planner, so that the
// dev mock stays self-consistent while pieces are added, observed and trimmed.
// It follows the contract's *shapes* and rules (§3.3 statuses, §3.7 / mid-batch
// §7.2 visit planning), not the real model's numbers.
//
// Only relative imports here: this file is also compiled for the Vite config.

import type {
  CheckItem,
  Confidence,
  Piece,
  PiecePrediction,
  PredictionStatus,
  UncertaintyReason,
  Visit,
} from '../api/types'
import { toIso } from '../utils/time'

const H = 3_600_000
const SLOT = 30 * 60_000
const EFF_H_PER_UNIT = { thin: 3, medium: 6, thick: 12 } as const
const SIZE_MULT = { small: 0.75, medium: 1, large: 1.4, xlarge: 1.9 } as const
/** Effective hours per wall-clock hour in the damp tent. */
const TENT = 0.4

export function ratePerHour(p: Pick<Piece, 'thickness' | 'size'>, factor = 1): number {
  return (TENT * factor) / (EFF_H_PER_UNIT[p.thickness] * SIZE_MULT[p.size])
}

export interface PredictContext {
  now: number
  learning: boolean
  /** Phone-free "offline ended at" — pieces anchored before it get device_was_off. */
  offlineEndedAt: number | null
  /** Per-piece speed tweak (1 = prior). */
  factor?: number
  pastPieces: number
}

export function predict(p: Piece, ctx: PredictContext): PiecePrediction {
  const computed_at = toIso(ctx.now)
  const confidence: Confidence = ctx.learning ? 'learning' : 'decent'
  const basis = ctx.learning
    ? { method: 'prior' as const, past_pieces: ctx.pastPieces, summary: `Default rates for a ${p.size} ${p.form}, adjusted by ${ctx.pastPieces} finished pieces` }
    : { method: 'learned' as const, past_pieces: ctx.pastPieces, summary: `Learned from ${ctx.pastPieces} finished pieces on this shelf` }
  if (p.state === 'trimmed' || p.state === 'discarded') {
    return {
      status: 'ended', eta: null, eta_low: null, eta_high: null, ready_until: null, dryness_now: null,
      confidence, uncertainty_reasons: [], forecast_basis: 'recent', basis, anchor: null, computed_at,
    }
  }
  const d0 = p.current_dryness ?? p.initial_dryness ?? 1
  const t0 = Date.parse(p.last_observed_at ?? p.added_at)
  const rate = ratePerHour(p, ctx.factor ?? 1)
  const reasons: UncertaintyReason[] = []
  if (ctx.learning) reasons.push('learning')
  const deviceWasOff = ctx.offlineEndedAt !== null && t0 < ctx.offlineEndedAt
  if (deviceWasOff) reasons.push('device_was_off')
  if (ctx.now - t0 > 24 * H) reasons.push('stale_observation')
  if (p.observation_count < 2) reasons.push('few_checks')

  let spread = ctx.learning ? 0.35 : 0.2
  if (deviceWasOff) spread += 0.4
  const at = (x: number, k = 1) => t0 + ((x - d0) / (rate * k)) * H
  const drynessNow = Math.min(5, Math.round((d0 + (rate * (ctx.now - t0)) / H) * 10) / 10)

  let status: PredictionStatus
  let eta: number | null = at(3)
  let etaLow: number | null = t0 + Math.max(0, at(3) - t0) / (1 + spread)
  let etaHigh: number | null = t0 + Math.max(0, at(3) - t0) * (1 + spread)
  let readyUntil: number | null = at(3.6, 1 + spread)

  if (d0 > 3.6) {
    status = 'past_trim'
    eta = etaLow = etaHigh = readyUntil = null
  } else if (d0 >= 2.8 && drynessNow < 3.6) {
    status = 'ready_now'
    eta = etaLow = etaHigh = t0
  } else if (drynessNow > 3.6) {
    status = 'may_be_past_check'
    eta = etaLow = etaHigh = null
  } else if (drynessNow >= 2.8) {
    status = 'probably_ready_check'
  } else {
    status = 'drying'
  }
  const iso = (v: number | null) => (v === null ? null : toIso(v))
  return {
    status,
    eta: iso(eta),
    eta_low: iso(etaLow),
    eta_high: iso(etaHigh),
    ready_until: iso(readyUntil),
    dryness_now: drynessNow,
    confidence,
    uncertainty_reasons: reasons,
    forecast_basis: 'recent',
    basis,
    anchor: { dryness: d0, observed_at: toIso(t0) },
    computed_at,
  }
}

/**
 * Visit slots: every 30 minutes around the clock over the next 7 days, with
 * now as the first slot (owner decision 2026-10-31: no studio schedule).
 */
export function visitSlots(now: number): number[] {
  const slots: number[] = [now]
  for (let t = Math.floor(now / SLOT) * SLOT + SLOT; t <= now + 7 * 24 * H; t += SLOT) slots.push(t)
  return slots
}

/** The deadline-aware visit planner (mid-batch §7.2, without the studio schedule). */
export function planVisit(pieces: Piece[], now: number, confidence: Confidence): Visit | null {
  const slots = visitSlots(now)
  type C = { id: number; eta: number; low: number; until: number }
  const cands: C[] = []
  for (const p of pieces) {
    const pr = p.prediction
    if (!pr || !['drying', 'probably_ready_check', 'ready_now'].includes(pr.status)) continue
    if (!pr.eta || !pr.eta_low || !pr.ready_until) continue
    const c = { id: p.id, eta: Date.parse(pr.eta), low: Date.parse(pr.eta_low), until: Date.parse(pr.ready_until) }
    if (c.until >= now) cands.push(c)
  }
  if (!cands.length) return null
  const deadline = Math.min(...cands.map((c) => c.until))
  const score = (t: number) => cands.reduce((acc, c) => acc + (t >= c.eta && t <= c.until ? 1 : t >= c.low && t < c.eta ? 0.5 : 0), 0)
  const usable = slots.filter((t) => t <= deadline)
  let best = -1
  let bestScore = 0
  usable.forEach((t, i) => {
    const sc = score(t)
    if (sc > bestScore) {
      bestScore = sc
      best = i
    }
  })
  if (best < 0) return null
  let a = best
  let b = best
  while (a > 0 && score(usable[a - 1]!) === bestScore) a--
  while (b < usable.length - 1 && score(usable[b + 1]!) === bestScore) b++
  const t = usable[best]!
  const ready: number[] = []
  const maybe: number[] = []
  const notYet: number[] = []
  for (const c of cands) {
    if (t >= c.eta && t <= c.until) ready.push(c.id)
    else if (t >= c.low && t < c.eta) maybe.push(c.id)
    else notYet.push(c.id)
  }
  return {
    recommended_at: toIso(t),
    start: toIso(usable[a]!),
    end: toIso(usable[b]!),
    deadline: toIso(deadline),
    ready_piece_ids: ready,
    maybe_piece_ids: maybe,
    not_yet_piece_ids: notYet,
    score: bestScore,
    confidence,
  }
}

export function checkThese(pieces: Piece[], now: number, learning: boolean, offlineEndedAt: number | null): CheckItem[] {
  const out: CheckItem[] = []
  for (const p of pieces) {
    const reasons: CheckItem['reasons'] = []
    const st = p.prediction?.status
    const last = Date.parse(p.last_observed_at ?? p.added_at)
    if (offlineEndedAt !== null && last < offlineEndedAt) reasons.push('not_checked_since_restart')
    if (st === 'probably_ready_check') reasons.push('probably_ready')
    if (st === 'may_be_past_check') reasons.push('may_be_past')
    if (learning && now - last > 24 * H && !reasons.includes('not_checked_since_restart')) reasons.push('not_checked_today')
    if (reasons.length) out.push({ piece_id: p.id, reasons })
  }
  // Most urgent first: may be past, then probably ready, then the rest.
  const rank = (c: CheckItem) => (c.reasons.includes('may_be_past') ? 0 : c.reasons.includes('probably_ready') ? 1 : 2)
  return out.sort((a, b) => rank(a) - rank(b))
}
