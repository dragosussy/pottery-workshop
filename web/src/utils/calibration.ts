// Salt test: damp table salt in a sealed box holds about 75 % RH.
export const SALT_RH = 75
export const OFFSET_MIN = -10
export const OFFSET_MAX = 10

/**
 * The offset that makes the sensor read SALT_RH in the salt box.
 * `shownReading` is what the app showed (it already includes `currentOffset`).
 */
export function saltTestOffset(shownReading: number, currentOffset: number): number {
  const raw = shownReading - currentOffset
  const offset = SALT_RH - raw
  return Math.round(Math.min(OFFSET_MAX, Math.max(OFFSET_MIN, offset)) * 10) / 10
}

export function formatOffset(v: number): string {
  return `${v > 0 ? '+' : v < 0 ? '−' : '±'}${Math.abs(v).toFixed(1)}%`
}
