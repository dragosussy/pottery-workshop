// A single global toast queue. Toasts can carry an action (Undo, Retry).
import { reactive } from 'vue'

export interface ToastAction {
  label: string
  run: () => void | Promise<void>
}

export interface Toast {
  id: number
  message: string
  /** Large-type line, e.g. "#3 #4 #5" for new shelf numbers. */
  big?: string
  tone: 'info' | 'ok' | 'warn' | 'error'
  action?: ToastAction
  timeoutMs: number
}

const state = reactive<{ toasts: Toast[] }>({ toasts: [] })
let nextId = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()

export function showToast(t: Omit<Toast, 'id' | 'tone' | 'timeoutMs'> & Partial<Pick<Toast, 'tone' | 'timeoutMs'>>): number {
  const id = nextId++
  const toast: Toast = { tone: 'info', timeoutMs: t.action ? 8000 : 4000, ...t, id }
  // Keep at most two on screen; the newest wins.
  if (state.toasts.length >= 2) dismissToast(state.toasts[0]!.id)
  state.toasts.push(toast)
  if (toast.timeoutMs > 0) timers.set(id, setTimeout(() => dismissToast(id), toast.timeoutMs))
  return id
}

export function dismissToast(id: number): void {
  const i = state.toasts.findIndex((t) => t.id === id)
  if (i >= 0) state.toasts.splice(i, 1)
  const timer = timers.get(id)
  if (timer) clearTimeout(timer)
  timers.delete(id)
}

export function clearToasts(): void {
  ;[...state.toasts].forEach((t) => dismissToast(t.id))
}

export function useToasts() {
  return state
}
