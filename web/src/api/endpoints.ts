// One function per endpoint (contract §2).
import { API_BASE, http, request } from './client'
import type {
  AddPieces,
  ClimateLogEntry,
  ControlPreview,
  ControlState,
  Health,
  Items,
  LiveStatus,
  NewObservations,
  Observation,
  ObservationPatch,
  ObservationsCreated,
  Page,
  PhotoCreated,
  Piece,
  PiecePatch,
  PiecesPatch,
  ReadingsResolution,
  ReadingsSeries,
  Reference,
  Settings,
  SettingsPatch,
  Shelf,
} from './types'

export const api = {
  health: () => http.get<Health>('/health'),
  reference: () => http.get<Reference>('/reference'),
  status: (signal?: AbortSignal) => http.get<LiveStatus>('/status', undefined, signal),
  shelf: (signal?: AbortSignal) => http.get<Shelf>('/shelf', undefined, signal),

  readings: (q: { from: string; to?: string; sensors?: string; resolution?: ReadingsResolution }, signal?: AbortSignal) =>
    http.get<ReadingsSeries>('/readings', q, signal),
  climateLog: (q: { from?: string; to?: string }, signal?: AbortSignal) =>
    http.get<Items<ClimateLogEntry>>('/climate-log', q, signal),

  addPieces: (body: AddPieces) => http.post<Items<Piece>>('/pieces', body),
  piecesOnShelf: () => http.get<Items<Piece>>('/pieces', { state: 'on_shelf' }),
  piecesEnded: (q: { limit?: number; cursor?: string | null } = {}) =>
    http.get<Page<Piece>>('/pieces', { state: 'ended', limit: q.limit, cursor: q.cursor }),
  piece: (id: number) => http.get<Piece>(`/pieces/${id}`),
  patchPiece: (id: number, body: PiecePatch) => http.patch<Piece>(`/pieces/${id}`, body),
  patchPieces: (body: PiecesPatch) => http.patch<Items<Piece>>('/pieces', body),

  uploadPhoto: (jpeg: Blob) => request<PhotoCreated>('POST', '/photos', { rawBody: jpeg, contentType: 'image/jpeg', timeoutMs: 60_000 }),

  addObservations: (body: NewObservations) => http.post<ObservationsCreated>('/observations', body),
  pieceObservations: (id: number) => http.get<Items<Observation>>(`/pieces/${id}/observations`),
  patchObservation: (id: number, body: ObservationPatch) => http.patch<Observation>(`/observations/${id}`, body),
  deleteObservation: (id: number) => http.del(`/observations/${id}`),
  undeleteObservation: (id: number) => http.post<Observation>(`/observations/${id}/undelete`, {}),
  answerFollowUp: (observationId: number, answer: string) =>
    http.post<Record<string, never>>(`/observations/${observationId}/follow-up`, { answer }),

  settings: () => http.get<Settings>('/settings'),
  patchSettings: (body: SettingsPatch) => http.patch<Settings>('/settings', body),

  control: () => http.get<ControlState>('/control'),
  /** null clears the target. */
  setTarget: (pct: number | null) => http.put<ControlState>('/control/setpoint', { target_humidity_pct: pct }),
  previewTarget: (pct: number | null, signal?: AbortSignal) =>
    http.get<ControlPreview>('/control/preview', pct === null ? {} : { target_humidity_pct: pct }, signal),
  /** Run the pump by hand (testing the wiring, priming after a refill). */
  runPump: (seconds: number) => http.post<ControlState>('/control/override', { seconds }),
  stopPump: () => http.del<ControlState>('/control/override'),
  /** Clear a safety trip. */
  resetPump: () => http.post<ControlState>('/control/reset', {}),
}

export const photoUrl = (id: number, size: 'thumb' | 'full' = 'thumb') => `${API_BASE}/photos/${id}?size=${size}`
export const backupUrl = `${API_BASE}/backup`
export const streamUrl = `${API_BASE}/stream`
