// Hand-written mirror of docs/design/api-contract.md (revision 3), §8.
// Field names are snake_case exactly as on the wire. Every documented field is
// always present; optional values are `null` (contract §1 "Null policy").

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export type FormKey = 'bowl' | 'mug' | 'plate' | 'vase' | 'other'
export type SizeKey = 'small' | 'medium' | 'large' | 'xlarge'
export type ThicknessKey = 'thin' | 'medium' | 'thick'
export type StageKey =
  | 'very_wet'
  | 'wet'
  | 'soft_leather_hard'
  | 'leather_hard'
  | 'hard_leather_hard'
  | 'bone_dry'
export type PieceState = 'in' | 'flipped' | 'trimmed' | 'discarded'
export type ObservationSource = 'add' | 'user' | 'flip' | 'end'
export type ObservationFlag = 'inconsistent'
export type Confidence = 'learning' | 'rough' | 'decent' | 'good'
export type PredictionStatus =
  | 'drying'
  | 'probably_ready_check'
  | 'ready_now'
  | 'may_be_past_check'
  | 'past_trim'
  | 'ended'
export type UncertaintyReason =
  | 'learning'
  | 'stale_observation'
  | 'device_was_off'
  | 'sensor_gap'
  | 'few_checks'
export type CheckReason =
  | 'probably_ready'
  | 'may_be_past'
  | 'not_checked_today'
  | 'not_checked_since_restart'
  | 'stale_observation'
export type SensorState = 'ok' | 'stale' | 'no_data'
export type RoomState = 'ok' | 'stale' | 'no_data'
export type ClimateLogKind = 'offline' | 'sensor_gap' | 'sensor_calibrated' | 'sensor_replaced' | 'setpoint_set'
export type LearningState = 'learning' | 'learned'
export type ForecastBasis = 'recent' | 'setpoint'
export type BasisMethod = 'prior' | 'learned'
export type FollowUpKind = 'much_slower' | 'went_wetter'
export type FollowUpAction = 'none' | 'correct_observation' | 'end_piece'

/** Continuous dryness, 0.0 (very wet) to 5.0 (bone dry), one decimal place. */
export type Dryness = number
/** RFC 3339 UTC timestamp, e.g. "2026-10-30T14:05:03Z". */
export type Timestamp = string

// ---------------------------------------------------------------------------
// §1 Errors and pagination
// ---------------------------------------------------------------------------

export interface FieldError {
  field: string
  code: string
  message: string
}

export interface ApiErrorBody {
  error: {
    code: string
    message: string
    fields?: FieldError[]
  }
}

export interface Page<T> {
  items: T[]
  next_cursor: string | null
}

export interface Items<T> {
  items: T[]
}

// ---------------------------------------------------------------------------
// §3.1 Health
// ---------------------------------------------------------------------------

export interface Health {
  status: 'ok' | 'degraded'
  version: string
  commit: string
  built_at: Timestamp
  api_version: number
  server_time: Timestamp
  clock_synced: boolean
  started_at: Timestamp
  checks: { database: string; sensors: string }
  features: { control: boolean }
}

// ---------------------------------------------------------------------------
// §3.2 Live status
// ---------------------------------------------------------------------------

export interface Room {
  humidity_pct: number | null
  temperature_c: number | null
  vpd_kpa: number | null
  state: RoomState
  read_at: Timestamp | null
}

export interface SensorStatus {
  sensor: string
  state: SensorState
  humidity_pct: number | null
  temperature_c: number | null
  offset_pct: number
  read_at: Timestamp | null
  age_seconds: number | null
  failing_since: Timestamp | null
  last_error: string | null
}

export interface ShelfSummary {
  on_shelf: number
  oldest_added_at: Timestamp | null
  by_stage: Record<StageKey, number>
  last_check_at: Timestamp | null
}

export interface TrimSummary {
  ready_now_count: number
  ready_now_until: Timestamp | null
  check_count: number
  late_count: number
  next_visit: Visit | null
}

export interface Learning {
  state: LearningState
  samples: number
  needed: number
}

export interface OfflinePeriod {
  from: Timestamp
  to: Timestamp
  pieces_to_check: number
}

export interface LiveStatus {
  server_time: Timestamp
  poll_interval_seconds: number
  stale_after_seconds: number
  room: Room
  sensors: SensorStatus[]
  shelf: ShelfSummary
  trim: TrimSummary
  learning: Learning
  last_offline: OfflinePeriod | null
  control: ControlState | null
}

// ---------------------------------------------------------------------------
// §3.3 Pieces
// ---------------------------------------------------------------------------

export interface PredictionBasis {
  method: BasisMethod
  past_pieces: number
  summary: string
}

export interface PiecePrediction {
  status: PredictionStatus
  eta: Timestamp | null
  eta_low: Timestamp | null
  eta_high: Timestamp | null
  ready_until: Timestamp | null
  dryness_now: Dryness | null
  confidence: Confidence
  uncertainty_reasons: UncertaintyReason[]
  forecast_basis: ForecastBasis
  basis: PredictionBasis
  anchor: { dryness: Dryness; observed_at: Timestamp } | null
  computed_at: Timestamp
}

export interface Piece {
  id: number
  number: number
  group_id: number | null
  group_size: number
  form: FormKey
  label: string
  size: SizeKey
  thickness: ThicknessKey
  photo_id: number | null
  notes: string
  state: PieceState
  added_at: Timestamp
  flipped_at: Timestamp | null
  flip_dryness: Dryness | null
  ended_at: Timestamp | null
  end_dryness: Dryness | null
  end_note: string
  mistake: boolean
  initial_dryness: Dryness | null
  current_dryness: Dryness | null
  last_observed_at: Timestamp | null
  observation_count: number
  prediction: PiecePrediction | null
}

export interface AddPieces {
  quantity?: number
  form: FormKey
  initial_dryness: Dryness
  size: SizeKey
  thickness: ThicknessKey
  photo_id?: number | null
  label?: string
  notes?: string
  added_at?: Timestamp
}

export interface PiecePatch {
  state?: PieceState
  flipped_at?: Timestamp | null
  flip_dryness?: Dryness | null
  ended_at?: Timestamp | null
  end_dryness?: Dryness | null
  end_note?: string
  mistake?: boolean
  form?: FormKey
  label?: string
  size?: SizeKey
  thickness?: ThicknessKey
  notes?: string
  photo_id?: number | null
  added_at?: Timestamp
  initial_dryness?: Dryness
}

export interface PiecesPatch extends PiecePatch {
  ids: number[]
}

// ---------------------------------------------------------------------------
// §3.4 Readings and climate log
// ---------------------------------------------------------------------------

export interface SensorSeries {
  sensor: string
  humidity_avg: (number | null)[]
  humidity_min: (number | null)[]
  humidity_max: (number | null)[]
  temperature_avg: (number | null)[]
  vpd_avg: (number | null)[]
  samples: number[]
}

export interface ReadingsSeries {
  from: Timestamp
  to: Timestamp
  resolution_seconds: number
  /** Unix seconds. */
  t: number[]
  series: SensorSeries[]
}

export type ReadingsResolution = 'auto' | '1m' | '5m' | '15m' | '1h' | '6h' | '1d'

interface ClimateLogBase<K extends ClimateLogKind, V> {
  id: number
  kind: K
  value: V
  at: Timestamp
}

export type ClimateLogEntry =
  | ClimateLogBase<'offline', { from: Timestamp; to: Timestamp }>
  | ClimateLogBase<'sensor_gap', { sensor: string; from: Timestamp; to: Timestamp }>
  | ClimateLogBase<'sensor_calibrated', { sensor: string; offset_pct: number; previous_offset_pct: number }>
  | ClimateLogBase<'sensor_replaced', { sensor: string }>
  | ClimateLogBase<'setpoint_set', { target_humidity_pct: number }>

// ---------------------------------------------------------------------------
// §3.6 Photos
// ---------------------------------------------------------------------------

export interface PhotoCreated {
  id: number
  width: number
  height: number
  created_at: Timestamp
}

// ---------------------------------------------------------------------------
// §3.7 Shelf
// ---------------------------------------------------------------------------

export interface Visit {
  recommended_at: Timestamp
  start: Timestamp
  end: Timestamp
  deadline: Timestamp | null
  ready_piece_ids: number[]
  maybe_piece_ids: number[]
  not_yet_piece_ids: number[]
  score: number
  confidence: Confidence
}

export interface CheckItem {
  piece_id: number
  reasons: CheckReason[]
}

export interface Shelf {
  computed_at: Timestamp
  items: Piece[]
  next_visit: Visit | null
  check_these: CheckItem[]
}

// ---------------------------------------------------------------------------
// §3.8 Reference
// ---------------------------------------------------------------------------

export interface DrynessScale {
  min: number
  max: number
  step: number
  target: number
  trim_band: [number, number]
}

export interface StageLandmark {
  key: StageKey
  label: string
  value: number
}

export interface Option<K extends string = string> {
  key: K
  label: string
}

export interface Reference {
  dryness: DrynessScale
  stages: StageLandmark[]
  forms: Option<FormKey>[]
  sizes: Option<SizeKey>[]
  thicknesses: Option<ThicknessKey>[]
  piece_states: Option<PieceState>[]
  prediction_statuses: Option<PredictionStatus>[]
  uncertainty_reasons: Option<UncertaintyReason>[]
  check_reasons: Option<CheckReason>[]
}

// ---------------------------------------------------------------------------
// §3.9 Observations
// ---------------------------------------------------------------------------

export interface Observation {
  id: number
  piece_id: number
  dryness: Dryness
  observed_at: Timestamp
  source: ObservationSource
  flag: ObservationFlag | null
  note: string
  created_at: Timestamp
}

export interface NewObservations {
  observed_at?: Timestamp
  items: { piece_id: number; dryness: Dryness }[]
  note?: string
}

export interface ObservationPatch {
  dryness?: Dryness
  observed_at?: Timestamp
  note?: string
}

export interface FollowUpOption {
  key: string
  label: string
  action: FollowUpAction
}

export interface FollowUp {
  kind: FollowUpKind
  piece_id: number
  observation_id: number
  previous_observation_id: number | null
  question: string
  options: FollowUpOption[]
}

export interface ObservationsCreated {
  items: Observation[]
  follow_ups: FollowUp[]
}

// ---------------------------------------------------------------------------
// §3.10 Settings
// ---------------------------------------------------------------------------

export interface SensorSettings {
  sensor: string
  offset_pct: number
  calibrated_at: Timestamp | null
}

export interface Settings {
  /** IANA zone used to display every time (the studio schedule was removed, owner decision 2026-10-31). */
  studio_tz: string
  sensors: SensorSettings[]
}

export interface SettingsPatch {
  studio_tz?: string
  sensors?: { sensor: string; offset_pct: number; replaced?: boolean }[]
}

// ---------------------------------------------------------------------------
// §4 Stream
// ---------------------------------------------------------------------------

export interface StreamHello {
  server_time: Timestamp
  version: string
  api_version: number
  poll_interval_seconds: number
  stale_after_seconds: number
  clock_synced: boolean
}

export interface StreamReadings {
  server_time: Timestamp
  room: Room
  sensors: SensorStatus[]
}

export type ChangeScope = 'pieces' | 'observations' | 'predictions' | 'settings' | 'climate_log'

export interface StreamChanged {
  scopes: ChangeScope[]
  piece_ids: number[]
}

// ---------------------------------------------------------------------------
// §6 Control (FUTURE; declared, not used in v1)
// ---------------------------------------------------------------------------

export interface Actuator {
  id: string
  kind: string
  state: 'on' | 'off'
  since: Timestamp
  override_until: Timestamp | null
}

export interface ControlState {
  mode: string
  target_humidity_pct: number
  hysteresis_pct: number
  actuators: Actuator[]
  holding: boolean
  safety: { tripped: boolean; reason: string | null }
}
