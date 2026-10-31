// Compact durations: "3d 4h", "5h 10m", "12m", "just now".

export function formatAge(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 60) return 'just now'
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) {
    const rm = m % 60
    return rm ? `${h}h ${rm}m` : `${h}h`
  }
  const d = Math.floor(h / 24)
  const rh = h % 24
  return rh ? `${d}d ${rh}h` : `${d}d`
}

/** "9 days", "5 hours", "40 minutes" — for sentences. */
export function formatSpanWords(seconds: number): string {
  const m = Math.round(seconds / 60)
  if (m < 60) return m === 1 ? '1 minute' : `${Math.max(m, 0)} minutes`
  const h = Math.round(m / 60)
  if (h < 48) return h === 1 ? '1 hour' : `${h} hours`
  const d = Math.round(h / 24)
  return `${d} days`
}

export function secondsBetween(fromMs: number, toMs: number): number {
  return Math.max(0, (toMs - fromMs) / 1000)
}
