// User-facing formatting of times, windows and numbers, in studio time.

import type { Confidence, Piece, PiecePrediction, Reference } from '../api/types'
import { dayDiff, parseTs, zonedParts } from './time'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const DAY_NAMES: Record<string, string> = { mon: 'Mon', tue: 'Tue', wed: 'Wed', thu: 'Thu', fri: 'Fri', sat: 'Sat', sun: 'Sun' }

const pad = (n: number) => String(n).padStart(2, '0')

/** "14:05" in `tz`. */
export function formatTime(ms: number, tz: string): string {
  const p = zonedParts(ms, tz)
  return `${pad(p.hour)}:${pad(p.minute)}`
}

/** "Oct 29" in `tz`. */
export function formatDate(ms: number, tz: string): string {
  const p = zonedParts(ms, tz)
  return `${MONTHS[p.month - 1]} ${p.day}`
}

/** "today", "tomorrow", "yesterday", "Sat" (within a week ahead), else "Oct 29". */
export function relativeDay(ms: number, now: number, tz: string): string {
  const d = dayDiff(now, ms, tz)
  if (d === 0) return 'today'
  if (d === 1) return 'tomorrow'
  if (d === -1) return 'yesterday'
  if (d > 1 && d < 7) return DAY_NAMES[zonedParts(ms, tz).weekday] ?? ''
  return formatDate(ms, tz)
}

/** "today 14:00", "tomorrow 09:00", "Sat 14:00", "Nov 9 14:00". */
export function formatDayTime(ms: number, now: number, tz: string): string {
  return `${relativeDay(ms, now, tz)} ${formatTime(ms, tz)}`
}

/** Like formatDayTime but takes a timestamp string; '—' when missing. */
export function dayTime(ts: string | null | undefined, now: number, tz: string): string {
  const ms = parseTs(ts)
  return ms === null ? '—' : formatDayTime(ms, now, tz)
}

type DayPart = 'night' | 'morning' | 'afternoon' | 'evening'
export function dayPart(hour: number): DayPart {
  if (hour < 5) return 'night'
  if (hour < 12) return 'morning'
  if (hour < 17) return 'afternoon'
  if (hour < 22) return 'evening'
  return 'night'
}

function dayWithPart(ms: number, now: number, tz: string): string {
  const p = zonedParts(ms, tz)
  const part = dayPart(p.hour)
  const rel = relativeDay(ms, now, tz)
  if (rel === 'today' && (part === 'evening' || part === 'night')) return 'tonight'
  if (rel === 'tomorrow' && part === 'night' && p.hour < 5) return 'tonight'
  return `${rel} ${part}`
}

/**
 * An ETA window in words (frontend plan §5.2 EtaWindow):
 *   within 12 h        → "in 3–6h"
 *   same day, ≤ 6 h    → "tonight 20:00–23:00", "Sat 09:00–13:00"
 *   same day, longer   → "Sat morning" (one part of day) or "Sat 08:00–19:00"
 *   different days     → "Fri evening – Sat morning"
 */
export function formatEtaWindow(lowMs: number, highMs: number, now: number, tz: string): string {
  if (highMs <= now) return 'any time now'
  const hLow = (lowMs - now) / 3_600_000
  const hHigh = (highMs - now) / 3_600_000
  if (hHigh <= 12) {
    const a = Math.max(0, Math.floor(hLow))
    const b = Math.max(1, Math.ceil(hHigh))
    if (hLow <= 0) return `within ${b}h`
    if (a === b || a === 0) return a === 0 ? `within ${b}h` : `in ~${b}h`
    return `in ${a}–${b}h`
  }
  const sameDay = dayDiff(lowMs, highMs, tz) === 0
  if (sameDay) {
    const pl = zonedParts(lowMs, tz)
    const ph = zonedParts(highMs, tz)
    const rel = relativeDay(lowMs, now, tz)
    const evening = rel === 'today' && pl.hour >= 17
    const dayWord = evening ? 'tonight' : rel
    if (highMs - lowMs <= 6 * 3_600_000) return `${dayWord} ${formatTime(lowMs, tz)}–${formatTime(highMs, tz)}`
    if (dayPart(pl.hour) === dayPart(ph.hour)) return dayWithPart(lowMs, now, tz)
    return `${dayWord} ${formatTime(lowMs, tz)}–${formatTime(highMs, tz)}`
  }
  const a = dayWithPart(lowMs, now, tz)
  const b = dayWithPart(highMs, now, tz)
  return a === b ? a : `${a} – ${b}`
}

/** "Sat 09:00–11:30" for a visit window (both ends on the same day), else "Sat 09:00 – Sun 10:00". */
export function formatVisitWindow(startMs: number, endMs: number, now: number, tz: string): string {
  if (dayDiff(startMs, endMs, tz) === 0) {
    if (startMs === endMs) return formatDayTime(startMs, now, tz)
    return `${relativeDay(startMs, now, tz)} ${formatTime(startMs, tz)}–${formatTime(endMs, tz)}`
  }
  return `${formatDayTime(startMs, now, tz)} – ${formatDayTime(endMs, now, tz)}`
}

/** Confidence as a word, never a percentage (frontend plan §4.1). */
export function confidenceWord(c: Confidence | null | undefined): string {
  switch (c) {
    case 'good':
      return 'good guess'
    case 'decent':
      return 'decent guess'
    case 'rough':
      return 'rough guess'
    case 'learning':
      return 'rough guess'
    default:
      return ''
  }
}

// ---------------------------------------------------------------------------
// Pieces
// ---------------------------------------------------------------------------

type Ref = Pick<Reference, 'forms' | 'sizes' | 'thicknesses'>

export function optionLabel<K extends string>(opts: { key: K; label: string }[], key: K): string {
  return opts.find((o) => o.key === key)?.label ?? key
}

/** "Mug · M · medium wall" */
export function pieceSpecs(p: Pick<Piece, 'form' | 'size' | 'thickness'>, ref: Ref): string {
  return `${optionLabel(ref.forms, p.form)} · ${optionLabel(ref.sizes, p.size)} · ${p.thickness} wall`
}

export function pieceTitle(p: Pick<Piece, 'number' | 'label'>): string {
  return p.label ? `#${p.number} ${p.label}` : `#${p.number}`
}

/** "#3, #5" or "#3 and #5" */
export function numberList(nums: number[], joiner: 'comma' | 'and' = 'comma'): string {
  const parts = nums.map((n) => `#${n}`)
  if (joiner === 'and' && parts.length > 1) return `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`
  return parts.join(', ')
}

export function etaText(pred: PiecePrediction | null, now: number, tz: string): string {
  if (!pred) return ''
  switch (pred.status) {
    case 'ready_now':
      return pred.ready_until ? `ready · best before ${dayTime(pred.ready_until, now, tz)}` : 'ready now'
    case 'probably_ready_check':
      return 'probably ready · check it'
    case 'may_be_past_check':
      return 'may be past leather hard · check it'
    case 'past_trim':
      return 'past leather hard'
    case 'ended':
      return ''
    case 'drying': {
      const lo = parseTs(pred.eta_low)
      const hi = parseTs(pred.eta_high)
      if (lo === null || hi === null) return 'drying'
      return formatEtaWindow(lo, hi, now, tz)
    }
  }
}

export function formatPct(v: number | null | undefined, digits = 0): string {
  return v === null || v === undefined ? '—' : `${v.toFixed(digits)}%`
}

export function formatTemp(v: number | null | undefined): string {
  return v === null || v === undefined ? '—' : `${v.toFixed(1)}°C`
}
