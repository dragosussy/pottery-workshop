// Labels for the API's enums and time formatting (the phone's local time).
import type { Estimate, Form, Now, Piece, PieceState, Size, Thickness } from './api'

export const FORMS: Record<Form, string> = { bowl: 'Bowl', mug: 'Mug', plate: 'Plate', vase: 'Vase', other: 'Other' }
export const SIZES: Record<Size, string> = { small: 'Small', medium: 'Medium', large: 'Large', xlarge: 'Extra large' }
export const THICKNESSES: Record<Thickness, string> = { thin: 'Thin', medium: 'Medium', thick: 'Thick' }
export const STATES: Record<PieceState, string> = { in: 'On the shelf', flipped: 'Flipped', trimmed: 'Trimmed', discarded: 'Discarded' }

export const keys = <K extends string>(o: Record<K, string>) => Object.keys(o) as K[]

export function pieceTitle(p: Piece): string {
  return `#${p.number} ${FORMS[p.form]}${p.label ? ' · ' + p.label : ''}`
}

export function specs(p: Piece): string {
  return `${SIZES[p.size]}, ${THICKNESSES[p.thickness].toLowerCase()} walls`
}

const two = (n: number) => String(n).padStart(2, '0')
const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export function clockTime(d: Date): string {
  return `${two(d.getHours())}:${two(d.getMinutes())}`
}

/** Whole calendar days from `now` to `d`, in the phone's time zone. */
function dayDiff(d: Date, now: Date): number {
  const a = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const b = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  return Math.round((a.getTime() - b.getTime()) / 86400e3)
}

/**
 * "today 14:00", "tomorrow 09:00", "yesterday 18:30", else always with the
 * date ("Tue 10 Nov 13:00"), so next Tuesday is never mistaken for tomorrow.
 */
export function when(iso: string, now = Date.now()): string {
  const d = new Date(iso)
  const diff = dayDiff(d, new Date(now))
  const words: Record<number, string> = { [-1]: 'yesterday', 0: 'today', 1: 'tomorrow' }
  if (diff in words) return `${words[diff]} ${clockTime(d)}`
  return `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]} ${clockTime(d)}`
}

/**
 * "in 40 min", "in 5 h", then calendar days ("in 3 days" for Thursday when
 * it's Monday), so it agrees with the date shown next to it.
 */
export function fromNow(iso: string, now = Date.now()): string {
  const ms = Date.parse(iso) - now
  if (ms <= 0) return 'now'
  const min = Math.round(ms / 60e3)
  if (min < 60) return `in ${min} min`
  const h = Math.round(ms / 3600e3)
  const days = dayDiff(new Date(iso), new Date(now))
  if (h < 36 || days < 2) return `in ${h} h`
  return `in ${days} days`
}

/** "12 s ago", "5 min ago", "3 h ago", "2 d ago". */
export function ago(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s} s ago`
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 2 * 86400) return `${Math.floor(s / 3600)} h ago`
  return `${Math.floor(s / 86400)} d ago`
}

export function estimateText(e: Estimate | null, now = Date.now()): string {
  if (!e) return ''
  if (e.status === 'past') return 'probably past leather hard'
  if (e.status === 'ready') return e.ready_until ? `ready now, until ${when(e.ready_until, now)}` : 'ready now'
  return e.ready_at ? `ready around ${when(e.ready_at, now)} (${fromNow(e.ready_at, now)})` : 'drying'
}

/** The estimate as a headline and a detail line, for the piece page. */
export function estimateParts(e: Estimate, now = Date.now()): { label: string; headline: string; detail: string } {
  if (e.status === 'past') {
    return { label: 'Probably too dry', headline: 'Past leather hard', detail: 'Check it: it may be too hard to trim.' }
  }
  if (e.status === 'ready') {
    return {
      label: 'Ready to trim',
      headline: 'Now',
      detail: e.ready_until ? `until ${when(e.ready_until, now)} (${fromNow(e.ready_until, now)})` : '',
    }
  }
  if (!e.ready_at) return { label: 'Drying', headline: '–', detail: '' }
  return {
    label: 'Ready to trim around',
    headline: when(e.ready_at, now),
    detail: fromNow(e.ready_at, now) + (e.ready_until ? ` · trimmable until ${when(e.ready_until, now)}` : ''),
  }
}

export function endedText(p: Piece): string {
  return p.ended_at ? `${STATES[p.state]} ${when(p.ended_at)}` : STATES[p.state]
}

const PUMP_STATUS = { running: 'Misting', resting: 'Resting after a pulse', idle: 'Idle', stopped: 'Stopped' }
const PUMP_REASONS: Record<string, string> = {
  no_target: 'no target set',
  no_data: 'no humidity reading',
  at_target: 'humidity is at the target',
  above_max: 'a sensor reads at or above the safety limit',
  user: 'stopped by hand; press Resume',
  no_rise: "the humidity didn't rise after several pulses. Check the water tank and the nozzle, then Resume",
  switch_error: "couldn't switch the pump. Check the wiring, then Resume",
}

export function pumpText(pump: Now['pump']): string {
  const status = PUMP_STATUS[pump.status]
  const reason = pump.reason ? (PUMP_REASONS[pump.reason] ?? pump.reason) : ''
  return reason ? `${status}: ${reason}` : status
}
