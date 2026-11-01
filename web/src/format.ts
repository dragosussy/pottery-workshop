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

/** "Sat 14:00" within a week either side of now, else "Sat 14 Nov 14:00". */
export function when(iso: string, now = Date.now()): string {
  const d = new Date(iso)
  const day = DAYS[d.getDay()]
  if (Math.abs(d.getTime() - now) < 6.5 * 86400e3) return `${day} ${clockTime(d)}`
  return `${day} ${d.getDate()} ${MONTHS[d.getMonth()]} ${clockTime(d)}`
}

/** "12 s ago", "5 min ago", "3 h ago", "2 d ago". */
export function ago(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s} s ago`
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 2 * 86400) return `${Math.floor(s / 3600)} h ago`
  return `${Math.floor(s / 86400)} d ago`
}

export function estimateText(e: Estimate | null): string {
  if (!e) return ''
  if (e.status === 'past') return 'probably past leather hard'
  if (e.status === 'ready') return e.ready_until ? `ready now, until ${when(e.ready_until)}` : 'ready now'
  return e.ready_at ? `ready around ${when(e.ready_at)}` : 'drying'
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
