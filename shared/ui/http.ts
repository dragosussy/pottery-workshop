// Talking to an app's API from the browser, under its URL prefix.

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number, // 0: no response
  ) {
    super(message)
  }
}

/** An API path as the browser must request it. */
export const apiUrl = (path: string) => `${import.meta.env.BASE_URL}api${path}`

/**
 * Makes the function that calls an app's API. A Blob body is sent as is and
 * anything else as JSON. `unreachable` is the message when there's no answer.
 */
export function apiCaller(unreachable: string, timeoutMs = 20_000) {
  return async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const init: RequestInit = { method, headers: {} }
    if (body instanceof Blob) {
      init.body = body
      init.headers = { 'Content-Type': body.type || 'application/octet-stream' }
    } else if (body !== undefined) {
      init.body = JSON.stringify(body)
      init.headers = { 'Content-Type': 'application/json' }
    }
    const abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), timeoutMs)
    init.signal = abort.signal
    let res: Response
    try {
      res = await fetch(apiUrl(path), init)
    } catch {
      throw new ApiError(unreachable, 0)
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
}
