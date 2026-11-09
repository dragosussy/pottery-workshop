// Labels and wording. The keys are the API's values.
import type { Defect, Firing, Level, Layer, MaterialKind, Method, Piece, PieceKind, Rating, Spot, Stage } from './api'
import { thicknessWords } from './thickness'

export const KINDS: Record<PieceKind, string> = { tile: 'Test tile', pot: 'Pot' }
export const LEVELS: Record<Level, string> = { bottom: 'Bottom shelf', middle: 'Middle shelf', top: 'Top shelf' }
export const SPOTS: Record<Spot, string> = { edge: 'Edge', middle: 'Middle' }
export const RATINGS: Record<Rating, string> = { great: 'Great', fine: 'Fine', failed: 'Failed' }
export const STAGES: Record<Stage, string> = { greenware: 'Greenware', bisque: 'Bisque', refire: 'Before re-fire' }
export const EVERYDAY_STAGES = { greenware: STAGES.greenware, bisque: STAGES.bisque }
export const METHODS: Record<Method, string> = { brush: 'Brush', dip: 'Dip', pour: 'Pour', spray: 'Spray', sponge: 'Sponge' }
export const MATERIAL_KINDS: Record<MaterialKind, string> = { oxide: 'Oxide', slip: 'Slip', commercial: 'Commercial glaze', own: 'Our own glaze' }
export const DEFECTS: Record<Defect, string> = {
  crawling: 'Crawling', pinholes: 'Pinholes', blisters: 'Blisters', running: 'Running',
  crazing: 'Crazing', shivering: 'Shivering', underfired: 'Underfired',
}
// The kiln's heights in the order it's loaded, from the bottom up.
export const LOADING_ORDER: Level[] = ['bottom', 'middle', 'top']

// The steps of a piece's journey, as shown on its page.
export const JOURNEY = ['To be glazed', 'In the kiln', 'Fired'] as const

export const AREAS = ['Inside', 'Outside', 'Rim']

export function pieceTitle(p: Pick<Piece, 'kind' | 'number' | 'id'>): string {
  return p.kind === 'tile' ? `Tile ${p.number}` : `Pot #${p.id}`
}

export function formatDate(iso: string | null): string {
  if (!iso) return 'Date unknown'
  // A plain date, so it must not shift with the time zone.
  return new Date(`${iso.slice(0, 10)}T12:00:00`).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

export function firingLabel(f: Pick<Firing, 'fired_on' | 'cone' | 'schedule'>): string {
  return [formatDate(f.fired_on), f.cone && `cone ${f.cone}`, f.schedule].filter(Boolean).join(' · ')
}

export function positionText(p: Pick<Piece, 'level' | 'spot'>): string {
  const parts = [p.level && LEVELS[p.level], p.spot && SPOTS[p.spot].toLowerCase()].filter(Boolean)
  return parts.join(', ')
}

export function stackText(layers: Pick<Layer, 'material'>[]): string {
  return layers.map((l) => l.material).join(' → ')
}

/** "Brush, 3 coats · Medium (3.0) · rim" */
export function layerDetail(l: Pick<Layer, 'method' | 'coats' | 'thickness' | 'area' | 'stage'>): string {
  const how = l.method ? METHODS[l.method] + (l.coats ? `, ${l.coats} coat${l.coats === 1 ? '' : 's'}` : '') : ''
  const thick = l.thickness === null ? '' : `${thicknessWords(l.thickness)} (${l.thickness.toFixed(1)})`
  return [l.stage && STAGES[l.stage], how, thick, l.area].filter(Boolean).join(' · ')
}
