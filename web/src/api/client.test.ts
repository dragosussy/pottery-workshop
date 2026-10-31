import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, buildUrl, errorMessage, request } from './client'

function jsonResponse(status: number, body: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('request retry rules (contract §1.2)', () => {
  it('never retries a POST that got no response', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)
    const err = await request('POST', '/observations', { body: { items: [] } }).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).kind).toBe('network')
    expect((err as ApiError).unconfirmed).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it.each(['PATCH', 'DELETE', 'GET'])('retries %s once on a network error', async (method) => {
    const fetchMock = vi.fn().mockRejectedValueOnce(new TypeError('Failed to fetch')).mockResolvedValueOnce(jsonResponse(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(request(method, '/pieces/1', { body: method === 'GET' ? undefined : {} })).resolves.toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('retries a PATCH only once', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(request('PATCH', '/pieces/1', { body: {} })).rejects.toMatchObject({ kind: 'network' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('never retries an HTTP error response, and parses the envelope', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(422, {
        error: { code: 'validation_failed', message: '1 field is invalid', fields: [{ field: 'items[0].dryness', code: 'out_of_range', message: 'must be between 0 and 5' }] },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const err = (await request('PATCH', '/pieces/1', { body: {} }).catch((e) => e)) as ApiError
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(err.kind).toBe('http')
    expect(err.status).toBe(422)
    expect(err.code).toBe('validation_failed')
    expect(err.unconfirmed).toBe(false)
    expect(err.fieldMessage('items[0].dryness')).toBe('must be between 0 and 5')
    expect(errorMessage(err)).toBe('must be between 0 and 5')
  })

  it('returns undefined for 204 and sends JSON bodies', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(request('DELETE', '/observations/3')).resolves.toBeUndefined()
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init.method).toBe('DELETE')
  })

  it('builds URLs under /api/v1 and skips empty params', () => {
    expect(buildUrl('/pieces', { state: 'ended', cursor: null, limit: 20 })).toBe('/api/v1/pieces?state=ended&limit=20')
  })
})
