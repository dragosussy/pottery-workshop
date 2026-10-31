// Words for the pump's state (contract §6).
import type { ControlState, PumpReason } from '@/api/types'
import { formatSpanWords } from './duration'

const REASONS: Record<PumpReason, string> = {
  below_target: 'the shelf is below the target',
  manual: 'by hand',
  resting: 'waiting for the mist to settle',
  at_target: 'at the target',
  no_target: 'no target set',
  no_sensor_data: 'no sensor data',
  above_max: 'humidity at the safety limit',
  tripped: 'stopped: check the water',
  switch_error: "the switch didn't respond",
}

export function pumpReasonText(reason: PumpReason): string {
  return REASONS[reason] ?? reason
}

/** "ran 4 minutes in the last 24 h". */
export function pumpRuntimeText(seconds: number): string {
  if (seconds <= 0) return 'not run in the last 24 h'
  const span = seconds < 90 ? `${Math.round(seconds)} s` : formatSpanWords(seconds)
  return `ran ${span} in the last 24 h`
}

/** Short state for the home screen's target row, or '' without a pump. */
export function pumpShort(c: ControlState | null | undefined): string {
  const p = c?.pump
  if (!p) return ''
  if (c?.safety?.tripped) return 'pump stopped'
  if (p.on) return 'pump running'
  if (p.reason === 'no_sensor_data' || p.reason === 'switch_error') return 'pump off'
  return ''
}
