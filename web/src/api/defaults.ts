// Reference data defaults: identical to `GET /reference` (contract §3.8).
// Used before the fetch completes, while offline, and by the dev mock.
import type { Reference } from './types'
import { DEFAULT_STAGES } from '../utils/dryness'

export const DEFAULT_REFERENCE: Reference = {
  dryness: { min: 0, max: 5, step: 0.1, target: 3, trim_band: [2.8, 3.6] },
  stages: [...DEFAULT_STAGES],
  forms: [
    { key: 'bowl', label: 'Bowl' },
    { key: 'mug', label: 'Mug' },
    { key: 'plate', label: 'Plate' },
    { key: 'vase', label: 'Vase' },
    { key: 'other', label: 'Other' },
  ],
  sizes: [
    { key: 'small', label: 'S' },
    { key: 'medium', label: 'M' },
    { key: 'large', label: 'L' },
    { key: 'xlarge', label: 'XL' },
  ],
  thicknesses: [
    { key: 'thin', label: 'Thin' },
    { key: 'medium', label: 'Medium' },
    { key: 'thick', label: 'Thick' },
  ],
  piece_states: [
    { key: 'in', label: 'On shelf' },
    { key: 'flipped', label: 'Flipped' },
    { key: 'trimmed', label: 'Trimmed' },
    { key: 'discarded', label: 'Discarded' },
  ],
  prediction_statuses: [
    { key: 'drying', label: 'Drying' },
    { key: 'probably_ready_check', label: 'Probably ready, check it' },
    { key: 'ready_now', label: 'Ready now' },
    { key: 'may_be_past_check', label: 'May be past leather hard, check it' },
    { key: 'past_trim', label: 'Past leather hard' },
    { key: 'ended', label: 'Off the shelf' },
  ],
  uncertainty_reasons: [
    { key: 'learning', label: 'still learning your shelf' },
    { key: 'stale_observation', label: 'not checked for a while' },
    { key: 'device_was_off', label: 'the controller was off' },
    { key: 'sensor_gap', label: 'the sensor was down' },
    { key: 'few_checks', label: 'only checked when added' },
  ],
  check_reasons: [
    { key: 'probably_ready', label: 'Probably ready' },
    { key: 'may_be_past', label: 'May be past leather hard' },
    { key: 'not_checked_today', label: 'Not checked in the last day' },
    { key: 'not_checked_since_restart', label: 'Not checked since the controller came back on' },
    { key: 'stale_observation', label: 'Not checked for a while' },
  ],
}
