// localStorage can throw (private mode, quota, disabled storage). Every access
// goes through these helpers so a storage failure never breaks the app.

const PREFIX = 'hc.'

export function loadJSON<T>(key: string): T | null {
  try {
    const raw = globalThis.localStorage?.getItem(PREFIX + key)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    return null
  }
}

export function saveJSON(key: string, value: unknown): void {
  try {
    globalThis.localStorage?.setItem(PREFIX + key, JSON.stringify(value))
  } catch {
    // ignore: quota or disabled storage
  }
}

export function removeKey(key: string): void {
  try {
    globalThis.localStorage?.removeItem(PREFIX + key)
  } catch {
    // ignore
  }
}
