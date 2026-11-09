// Types and calls for the HTTP API. Times are RFC 3339 UTC strings.
import { apiCaller, apiUrl } from '@ui/http'

export type PieceKind = 'tile' | 'pot'
export type Level = 'bottom' | 'middle' | 'top'
export type Spot = 'edge' | 'middle'
export type Rating = 'great' | 'fine' | 'failed'
export type Defect = 'crawling' | 'pinholes' | 'blisters' | 'running' | 'crazing' | 'shivering' | 'underfired'
export type Stage = 'greenware' | 'bisque' | 'refire'
export type Method = 'brush' | 'dip' | 'pour' | 'spray' | 'sponge'
export type MaterialKind = 'oxide' | 'slip' | 'commercial' | 'own'
export type PhotoStage = 'before' | 'after'

export interface ClayBody {
  id: number
  name: string
  notes: string
  archived: boolean
}

export interface LastUse {
  stage: Stage | null
  method: Method | null
  thickness: number | null
  coats: number | null
}

export interface RecipeRef {
  id: number
  version: number
}

export interface Material {
  id: number
  name: string
  kind: MaterialKind
  maker: string
  notes: string
  archived: boolean
  uses: number
  last_use: LastUse | null
  versions: RecipeRef[]
}

export interface RecipeLine {
  ingredient: string
  amount: number
  addition: boolean
}

export interface Recipe {
  id: number
  version: number
  change_note: string
  water: number | null // to start with, in the same parts as the lines
  consistency: string // what to aim for once mixed
  created_at: string
  used: boolean
  lines: RecipeLine[]
}

export interface MaterialDetail extends Material {
  recipes: Recipe[]
}

export interface MaterialInput {
  name: string
  kind: MaterialKind
  maker: string
  notes: string
  archived: boolean
}

export type FiringState = 'loading' | 'firing' | 'unloaded'

export interface Firing {
  id: number
  state: FiringState
  fired_on: string | null
  cone: string
  schedule: string
  notes: string
  pieces: number
  waiting: number
}

export type FiringInput = Pick<Firing, 'fired_on' | 'cone' | 'schedule' | 'notes'>

export interface Layer {
  material_id: number
  material: string
  kind: MaterialKind
  recipe_id: number | null
  recipe_version: number | null
  stage: Stage | null
  method: Method | null
  thickness: number | null
  coats: number | null
  area: string
  notes: string
}

export interface Photo {
  id: number
  stage: PhotoStage
}

export interface Piece {
  id: number
  kind: PieceKind
  number: number | null
  clay_body_id: number | null
  clay_body: string | null
  firing_id: number | null
  firing: { id: number; state: FiringState; fired_on: string | null; cone: string; schedule: string } | null
  level: Level | null
  x: number | null
  y: number | null
  spot: Spot | null
  refire_of: number | null
  refired_by: number | null
  notes: string
  from_memory: boolean
  rating: Rating | null
  result_notes: string
  defects: Defect[]
  state: 'bench' | 'kiln' | 'waiting' | 'done'
  created_at: string
  layers: Layer[]
  photos: Photo[]
}

export type LayerInput = Omit<Layer, 'material' | 'kind' | 'recipe_version'>

export type PieceInput = Pick<
  Piece,
  | 'kind' | 'number' | 'clay_body_id' | 'firing_id' | 'level' | 'x' | 'y' | 'refire_of'
  | 'notes' | 'from_memory' | 'rating' | 'result_notes' | 'defects'
> & { layers: LayerInput[] }

export interface Suggestions {
  ingredients: string[]
  schedules: string[]
  cones: string[]
}

export const PAGE_SIZE = 60

// Long enough to send a full-size photo from a phone.
const call = apiCaller("Can't reach the notebook. Check the Wi-Fi or Tailscale.", 60_000)

type Query = Record<string, string | number | undefined>

function queryString(q: Query): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(q)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  return params.size ? `?${params}` : ''
}

export const api = {
  suggestions: () => call<Suggestions>('GET', '/suggestions'),

  clayBodies: () => call<ClayBody[]>('GET', '/clay-bodies'),
  addClayBody: (name: string) => call<ClayBody>('POST', '/clay-bodies', { name, notes: '' }),
  saveClayBody: (c: ClayBody) => call<ClayBody>('PUT', `/clay-bodies/${c.id}`, { name: c.name, notes: c.notes, archived: c.archived }),
  deleteClayBody: (id: number) => call<void>('DELETE', `/clay-bodies/${id}`),

  materials: () => call<Material[]>('GET', '/materials'),
  material: (id: number) => call<MaterialDetail>('GET', `/materials/${id}`),
  addMaterial: (m: MaterialInput) => call<MaterialDetail>('POST', '/materials', m),
  saveMaterial: (id: number, m: MaterialInput) => call<MaterialDetail>('PUT', `/materials/${id}`, m),
  deleteMaterial: (id: number) => call<void>('DELETE', `/materials/${id}`),
  saveRecipe: (id: number, recipe: Pick<Recipe, 'change_note' | 'water' | 'consistency' | 'lines'>) =>
    call<MaterialDetail>('PUT', `/materials/${id}/recipe`, recipe),

  firings: () => call<Firing[]>('GET', '/firings'),
  firing: (id: number) => call<Firing>('GET', `/firings/${id}`),
  addFiring: (f: FiringInput) => call<Firing>('POST', '/firings', f),
  saveFiring: (id: number, f: FiringInput) => call<Firing>('PUT', `/firings/${id}`, f),
  setFiringState: (id: number, state: FiringState) => call<Firing>('PUT', `/firings/${id}/state`, { state }),
  deleteFiring: (id: number) => call<void>('DELETE', `/firings/${id}`),

  pieces: (q: Query = {}) => call<Piece[]>('GET', `/pieces${queryString(q)}`),
  piece: (id: number) => call<Piece>('GET', `/pieces/${id}`),
  addPiece: (p: PieceInput) => call<Piece>('POST', '/pieces', p),
  savePiece: (id: number, p: PieceInput) => call<Piece>('PUT', `/pieces/${id}`, p),
  deletePiece: (id: number) => call<void>('DELETE', `/pieces/${id}`),

  addPhoto: (pieceId: number, stage: PhotoStage, photo: Blob) => call<Photo>('POST', `/pieces/${pieceId}/photos?stage=${stage}`, photo),
  deletePhoto: (id: number) => call<void>('DELETE', `/photos/${id}`),
}

export const photoUrl = (id: number) => apiUrl(`/photos/${id}`)
export const thumbUrl = (id: number) => apiUrl(`/photos/${id}/thumb`)
export const backupUrl = apiUrl('/backup')

/** A piece as the API wants it back, to change a few fields and save. */
export function toInput(p: Piece): PieceInput {
  return {
    kind: p.kind, number: p.number, clay_body_id: p.clay_body_id, firing_id: p.firing_id, level: p.level, x: p.x, y: p.y,
    refire_of: p.refire_of, notes: p.notes, from_memory: p.from_memory, rating: p.rating, result_notes: p.result_notes,
    defects: p.defects,
    layers: p.layers.map(toLayerInput),
  }
}

export function toLayerInput(l: Layer): LayerInput {
  return {
    material_id: l.material_id, recipe_id: l.recipe_id, stage: l.stage, method: l.method,
    thickness: l.thickness, coats: l.coats, area: l.area, notes: l.notes,
  }
}
