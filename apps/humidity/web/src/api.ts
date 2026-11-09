// Types and calls for the HTTP API (docs/apps/humidity/design.md §6). Times are RFC 3339 UTC
// strings; history arrays use unix seconds.

export type Form = 'bowl' | 'mug' | 'plate' | 'vase' | 'other'
export type Size = 'small' | 'medium' | 'large' | 'xlarge'
export type Thickness = 'thin' | 'medium' | 'thick'
export type PieceState = 'in' | 'flipped' | 'trimmed' | 'discarded'
export type PumpStatus = 'running' | 'resting' | 'idle' | 'stopped'

export interface Sensor {
  addr: string
  rh: number | null
  raw_rh: number | null
  temp: number | null
  offset: number
  at: string | null
  error: string | null
}

export interface Now {
  room: { rh: number; temp: number; at: string } | null
  sensors: Sensor[]
  target: number | null
  max_rh: number
  pump: { enabled: boolean; on: boolean; status: PumpStatus; reason: string | null; last_run_at: string | null }
  learning: boolean
}

export interface History {
  step: number
  t: number[]
  rh: number[]
  temp: number[]
}

export interface Check {
  id: number
  dryness: number
  at: string
}

export interface Estimate {
  status: 'drying' | 'ready' | 'past'
  ready_at: string | null
  ready_until: string | null
}

export interface Piece {
  id: number
  number: number
  form: Form
  size: Size
  thickness: Thickness
  label: string
  notes: string
  photo: boolean
  state: PieceState
  added_at: string
  flipped_at: string | null
  ended_at: string | null
  dryness: number
  last_check: { dryness: number; at: string }
  estimate: Estimate | null
  checks?: Check[]
}

export interface NewPieces {
  form: Form
  size: Size
  thickness: Thickness
  dryness: number
  label?: string
  notes?: string
  quantity?: number
}

export interface PiecePatch {
  label?: string
  notes?: string
  form?: Form
  size?: Size
  thickness?: Thickness
  state?: PieceState
  dryness?: number
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number, // 0: no response
  ) {
    super(message)
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} }
  if (body instanceof Blob) {
    init.body = body
    init.headers = { 'Content-Type': body.type || 'application/octet-stream' }
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    init.headers = { 'Content-Type': 'application/json' }
  }
  const abort = new AbortController()
  const timer = setTimeout(() => abort.abort(), 20_000)
  init.signal = abort.signal
  let res: Response
  try {
    res = await fetch('/api' + path, init)
  } catch {
    throw new ApiError("Can't reach the shelf. Check the Wi-Fi or Tailscale.", 0)
  } finally {
    clearTimeout(timer)
  }
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const j = (await res.json()) as { error?: string }
      if (j.error) message = j.error
    } catch {
      // not JSON; keep the status line
    }
    throw new ApiError(message, res.status)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  now: () => call<Now>('GET', '/now'),
  history: (hours: 1 | 24 | 168) => call<History>('GET', `/history?hours=${hours}`),
  setTarget: (pct: number | null) => call<Now>('PUT', '/target', { pct }),
  pumpRun: (seconds: number) => call<Now>('POST', '/pump/run', { seconds }),
  pumpStop: () => call<Now>('POST', '/pump/stop'),
  pumpResume: () => call<Now>('POST', '/pump/resume'),
  setOffset: (addr: string, offset: number) => call<Now>('PUT', `/sensors/${addr}/offset`, { offset }),
  shelf: () => call<Piece[]>('GET', '/pieces'),
  finished: () => call<Piece[]>('GET', '/pieces?ended=1'),
  piece: (id: number) => call<Piece>('GET', `/pieces/${id}`),
  addPieces: (p: NewPieces) => call<Piece[]>('POST', '/pieces', p),
  patchPiece: (id: number, p: PiecePatch) => call<Piece>('PATCH', `/pieces/${id}`, p),
  deletePiece: (id: number) => call<void>('DELETE', `/pieces/${id}`),
  /** The camera's original file; the server turns it upright and shrinks it. */
  putPhoto: (id: number, photo: Blob) => call<void>('PUT', `/pieces/${id}/photo`, photo),
  addCheck: (id: number, dryness: number) => call<Piece>('POST', `/pieces/${id}/checks`, { dryness }),
  deleteCheck: (checkId: number) => call<Piece>('DELETE', `/checks/${checkId}`),
}
