// "When" choices (plan §5.2 WhenChips): Now / 1h ago / 3h ago / this morning / pick.
// "Now" sends no time (the server fills it in). The others are computed from
// server time, in the studio time zone.
import { fromLocalInputValue, toIso, zonedParts, zonedToUtc } from './time'

export type WhenChoice =
  | { kind: 'now' }
  | { kind: 'ago'; hours: number }
  | { kind: 'morning' }
  | { kind: 'pick'; local: string }

export const MORNING_HOUR = 8

/** 08:00 studio time today, or null when that is still in the future. */
export function thisMorning(serverNow: number, tz: string): number | null {
  const p = zonedParts(serverNow, tz)
  const t = zonedToUtc(p.year, p.month, p.day, MORNING_HOUR, 0, tz)
  return t <= serverNow ? t : null
}

/**
 * The time to send for a choice: undefined for "now" (omit the field), an
 * RFC 3339 string otherwise, or null if the choice is invalid (e.g. a picked
 * time in the future).
 */
export function resolveWhen(c: WhenChoice, serverNow: number, tz: string): string | undefined | null {
  switch (c.kind) {
    case 'now':
      return undefined
    case 'ago':
      return toIso(serverNow - c.hours * 3_600_000)
    case 'morning': {
      const t = thisMorning(serverNow, tz)
      return t === null ? null : toIso(t)
    }
    case 'pick': {
      const t = fromLocalInputValue(c.local, tz)
      if (t === null || t > serverNow + 60_000) return null
      return toIso(t)
    }
  }
}
