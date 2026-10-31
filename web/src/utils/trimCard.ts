// Which trim-card state to show (plan §4.1, mid-batch §7.3, as amended by the
// owner on 2026-10-31: no studio schedule, so no "at risk" and no "come by").
// Pure, so every state is unit-testable from fixtures.
import type { LiveStatus, Piece, Visit } from '../api/types'
import { confidenceWord, formatDayTime, formatVisitWindow } from './format'
import { parseTs } from './time'

export type TrimCardKind = 'empty' | 'ready_now' | 'next_visit' | 'nothing_soon'

export interface TrimCardModel {
  kind: TrimCardKind
  /** Main line, e.g. "3 ready now" / "Come Sat 09:00–11:30". */
  headline: string
  /** Second line, e.g. "best before Sat 14:00" / "5 ready, 2 maybe". */
  detail: string
  /** Pieces shown as thumbnails (ready now). */
  readyIds: number[]
  visit: Visit | null
  /** A later visit for the other pieces, when something is ready now. */
  then: string
  /** "rough guess · learning your shelf (2 of 5)", or a confidence word. */
  confidence: string
  learning: boolean
}

export function trimCardModel(status: LiveStatus, pieces: Piece[], tz: string, now: number): TrimCardModel {
  const t = status.trim
  const learning = status.learning.state === 'learning'
  const visit = t.next_visit
  const confidence = learning
    ? `rough guess · learning your shelf (${status.learning.samples} of ${status.learning.needed})`
    : confidenceWord(visit?.confidence ?? null)
  const base = { visit, confidence, learning, readyIds: [] as number[], then: '' }

  if (status.shelf.on_shelf === 0) return { ...base, kind: 'empty', headline: 'The shelf is empty', detail: '', confidence: '' }

  if (t.ready_now_count > 0) {
    const readyIds = pieces.filter((p) => p.prediction?.status === 'ready_now').map((p) => p.id)
    const until = parseTs(t.ready_now_until)
    const detail = until === null ? '' : `best before ${formatDayTime(until, now, tz)}`
    // Pieces ready at the recommended visit that aren't ready now.
    let then = ''
    if (visit) {
      const more = visit.ready_piece_ids.filter((id) => !readyIds.includes(id))
      const start = parseTs(visit.start)
      const end = parseTs(visit.end)
      if (more.length && start !== null && end !== null && start > now) then = `Then come ${formatVisitWindow(start, end, now, tz)} for ${more.length} more`
    }
    return { ...base, kind: 'ready_now', headline: `${t.ready_now_count} ready now`, detail, readyIds, then }
  }

  if (visit) {
    const start = parseTs(visit.start)!
    const end = parseTs(visit.end)!
    const parts = [`${visit.ready_piece_ids.length} ready`]
    if (visit.maybe_piece_ids.length) parts.push(`${visit.maybe_piece_ids.length} maybe`)
    return {
      ...base,
      kind: 'next_visit',
      headline: start <= now ? `Come now, until ${formatDayTime(end, now, tz)}` : `Come ${formatVisitWindow(start, end, now, tz)}`,
      detail: parts.join(', '),
    }
  }

  return { ...base, kind: 'nothing_soon', headline: 'Nothing to trim in the next few days', detail: '' }
}
