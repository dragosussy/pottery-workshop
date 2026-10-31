// Time-zone arithmetic with Intl only (no date library). Every user-facing
// time is shown in the studio's time zone (`settings.studio_tz`).

export interface ZonedParts {
  year: number
  month: number // 1-12
  day: number
  hour: number // 0-23
  minute: number
  /** 'mon' … 'sun' */
  weekday: DayKey
}

const fmtCache = new Map<string, Intl.DateTimeFormat>()

function partsFormatter(tz: string): Intl.DateTimeFormat {
  let f = fmtCache.get(tz)
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      hourCycle: 'h23',
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: 'numeric',
      minute: 'numeric',
      weekday: 'short',
    })
    fmtCache.set(tz, f)
  }
  return f
}

export type DayKey = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun'

const WEEKDAYS: Record<string, DayKey> = {
  Mon: 'mon',
  Tue: 'tue',
  Wed: 'wed',
  Thu: 'thu',
  Fri: 'fri',
  Sat: 'sat',
  Sun: 'sun',
}

/** Whether `tz` is a time zone this browser knows. */
export function isValidTimeZone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    return true
  } catch {
    return false
  }
}

export function safeTz(tz: string | null | undefined): string {
  return tz && isValidTimeZone(tz) ? tz : 'UTC'
}

export function zonedParts(t: number | Date, tz: string): ZonedParts {
  const ms = typeof t === 'number' ? t : t.getTime()
  const parts = partsFormatter(safeTz(tz)).formatToParts(new Date(ms))
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? '0'
  return {
    year: Number(get('year')),
    month: Number(get('month')),
    day: Number(get('day')),
    hour: Number(get('hour')) % 24,
    minute: Number(get('minute')),
    weekday: WEEKDAYS[get('weekday')] ?? 'mon',
  }
}

/** Offset of `tz` from UTC at instant `ms`, in ms (e.g. +2h → 7_200_000). */
export function tzOffsetMs(ms: number, tz: string): number {
  const p = zonedParts(ms, tz)
  const asUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute)
  const floored = Math.floor(ms / 60_000) * 60_000
  return asUtc - floored
}

/**
 * The UTC instant of a wall-clock time in `tz`. For a time skipped by a DST
 * jump it returns the instant just after the jump; for a repeated time, the
 * first occurrence.
 */
export function zonedToUtc(year: number, month: number, day: number, hour: number, minute: number, tz: string): number {
  const guess = Date.UTC(year, month - 1, day, hour, minute)
  const o1 = tzOffsetMs(guess, tz)
  let t = guess - o1
  const o2 = tzOffsetMs(t, tz)
  if (o2 !== o1) {
    const t2 = guess - o2
    // Prefer the candidate that maps back to the requested wall time.
    const p = zonedParts(t2, tz)
    t = p.hour === hour && p.minute === minute ? t2 : Math.max(t, t2)
  }
  // Repeated hour (fall back): take the earlier occurrence if it matches too.
  const earlier = t - 3_600_000
  const pe = zonedParts(earlier, tz)
  if (pe.year === year && pe.month === month && pe.day === day && pe.hour === hour && pe.minute === minute) return earlier
  return t
}

/** Days since 1970-01-01 of the calendar date of `ms` in `tz`. */
export function zonedDayNumber(ms: number, tz: string): number {
  const p = zonedParts(ms, tz)
  return Math.round(Date.UTC(p.year, p.month - 1, p.day) / 86_400_000)
}

/** Calendar-day difference b − a in `tz` (0 = same day, 1 = b is the next day). */
export function dayDiff(a: number, b: number, tz: string): number {
  return zonedDayNumber(b, tz) - zonedDayNumber(a, tz)
}

export function parseTs(ts: string | null | undefined): number | null {
  if (!ts) return null
  const ms = Date.parse(ts)
  return Number.isNaN(ms) ? null : ms
}

/** RFC 3339 UTC at second precision, as the contract wants. */
export function toIso(ms: number): string {
  return new Date(Math.floor(ms / 1000) * 1000).toISOString().replace('.000Z', 'Z')
}

/** "2026-10-30T14:05" for a datetime-local input, in `tz`. */
export function toLocalInputValue(ms: number, tz: string): string {
  const p = zonedParts(ms, tz)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${p.year}-${pad(p.month)}-${pad(p.day)}T${pad(p.hour)}:${pad(p.minute)}`
}

/** Parse a datetime-local value as wall time in `tz`. */
export function fromLocalInputValue(v: string, tz: string): number | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(v)
  if (!m) return null
  return zonedToUtc(Number(m[1]), Number(m[2]), Number(m[3]), Number(m[4]), Number(m[5]), tz)
}

/** The phone's own time zone, for first-time setup. */
export function deviceTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}
