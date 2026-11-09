// Glaze thickness, judged by feel: 0.0–5.0, the whole numbers are landmarks.

export const THICKNESS_WORDS = ['Trace', 'Very thin', 'Thin', 'Medium', 'Thick', 'Very thick'] as const
export const THICKNESS_GRADIENT = 'linear-gradient(to right, var(--surface-3), var(--accent))'

/** The nearest landmark; x.5 goes to the thicker one. */
export function thicknessWords(v: number): string {
  const i = Math.min(5, Math.max(0, Math.floor(Math.round(v * 10 + 5) / 10)))
  return THICKNESS_WORDS[i]!
}
