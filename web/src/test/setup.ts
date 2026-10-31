import { afterEach } from 'vitest'

// Node ≥ 22 ships its own (experimental) `localStorage` global, which shadows
// jsdom's and is undefined without --localstorage-file. Install a small
// in-memory Storage so tests behave like a browser.
class MemoryStorage implements Storage {
  private m = new Map<string, string>()
  get length() {
    return this.m.size
  }
  clear() {
    this.m.clear()
  }
  getItem(k: string) {
    return this.m.has(k) ? this.m.get(k)! : null
  }
  key(i: number) {
    return [...this.m.keys()][i] ?? null
  }
  removeItem(k: string) {
    this.m.delete(k)
  }
  setItem(k: string, v: string) {
    this.m.set(k, String(v))
  }
}

let ok = false
try {
  ok = typeof window.localStorage?.getItem === 'function'
} catch {
  ok = false
}
if (!ok) {
  const storage = new MemoryStorage()
  Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true, writable: true })
  Object.defineProperty(window, 'localStorage', { value: storage, configurable: true, writable: true })
}

afterEach(() => {
  try {
    localStorage.clear()
  } catch {
    // ignore
  }
})
