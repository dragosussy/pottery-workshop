// Thin typed fetch wrapper (frontend plan §6, contract §1).
//
// Retry rules (contract §1.2):
// - GET, PATCH and DELETE are idempotent: retried once on a network error.
// - POST is NEVER retried automatically. A POST that gets no response is
//   reported as `kind: 'network'`, and the caller refetches the shelf and
//   tells the user to check before trying again.

import type { ApiErrorBody, FieldError } from './types'

export const API_BASE = '/api/v1'
const TIMEOUT_MS = 15_000

export type ApiErrorKind = 'http' | 'network'

export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly status: number
  readonly code: string
  readonly fields: FieldError[]
  readonly method: string

  constructor(init: { kind: ApiErrorKind; status?: number; code?: string; message: string; fields?: FieldError[]; method: string }) {
    super(init.message)
    this.name = 'ApiError'
    this.kind = init.kind
    this.status = init.status ?? 0
    this.code = init.code ?? (init.kind === 'network' ? 'network' : 'unknown')
    this.fields = init.fields ?? []
    this.method = init.method
  }

  /** No response arrived: the write may or may not have happened. */
  get unconfirmed(): boolean {
    return this.kind === 'network'
  }

  fieldMessage(field: string): string | undefined {
    return this.fields.find((f) => f.field === field)?.message
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError
}

export type Query = Record<string, string | number | boolean | null | undefined>

export interface RequestOptions {
  query?: Query
  body?: unknown
  /** Raw body (e.g. a JPEG Blob) sent as-is with `contentType`. */
  rawBody?: Blob | ArrayBuffer
  contentType?: string
  signal?: AbortSignal
  timeoutMs?: number
}

export function buildUrl(path: string, query?: Query): string {
  let url = API_BASE + path
  if (query) {
    const params = new URLSearchParams()
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null || v === '') continue
      params.set(k, String(v))
    }
    const qs = params.toString()
    if (qs) url += '?' + qs
  }
  return url
}

async function parseError(res: Response, method: string): Promise<ApiError> {
  let body: ApiErrorBody | null = null
  try {
    body = (await res.json()) as ApiErrorBody
  } catch {
    body = null
  }
  return new ApiError({
    kind: 'http',
    status: res.status,
    code: body?.error?.code ?? `http_${res.status}`,
    message: body?.error?.message ?? res.statusText ?? `HTTP ${res.status}`,
    fields: body?.error?.fields ?? [],
    method,
  })
}

async function once<T>(method: string, path: string, opts: RequestOptions): Promise<T> {
  const controller = new AbortController()
  const onAbort = () => controller.abort()
  opts.signal?.addEventListener('abort', onAbort)
  const timer = setTimeout(() => controller.abort(), opts.timeoutMs ?? TIMEOUT_MS)
  const headers: Record<string, string> = { Accept: 'application/json' }
  let body: BodyInit | undefined
  if (opts.rawBody !== undefined) {
    body = opts.rawBody
    headers['Content-Type'] = opts.contentType ?? 'application/octet-stream'
  } else if (opts.body !== undefined) {
    body = JSON.stringify(opts.body)
    headers['Content-Type'] = 'application/json; charset=utf-8'
  }
  let res: Response
  try {
    res = await fetch(buildUrl(path, opts.query), {
      method,
      headers,
      body,
      signal: controller.signal,
      cache: 'no-store',
      credentials: 'same-origin',
    })
  } catch (e) {
    if (opts.signal?.aborted) throw e
    throw new ApiError({ kind: 'network', message: 'No response from the controller', method })
  } finally {
    clearTimeout(timer)
    opts.signal?.removeEventListener('abort', onAbort)
  }
  if (!res.ok) throw await parseError(res, method)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

/** Whether a failed request of this method may be retried automatically. */
export function mayRetry(method: string): boolean {
  return method === 'GET' || method === 'PATCH' || method === 'DELETE'
}

export async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  try {
    return await once<T>(method, path, opts)
  } catch (e) {
    if (isApiError(e) && e.kind === 'network' && mayRetry(method) && !opts.signal?.aborted) {
      return once<T>(method, path, opts)
    }
    throw e
  }
}

export const http = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => request<T>('GET', path, { query, signal }),
  post: <T>(path: string, body: unknown) => request<T>('POST', path, { body }),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, { body }),
  del: <T = void>(path: string) => request<T>('DELETE', path),
}

/** A user-facing sentence for an error. */
export function errorMessage(e: unknown, fallback = 'Something went wrong'): string {
  if (isApiError(e)) {
    if (e.kind === 'network') return "Couldn't reach the controller"
    if (e.fields.length) return e.fields.map((f) => f.message).join('; ')
    return e.message || fallback
  }
  return fallback
}
