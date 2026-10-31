// Colour palette selection. The palettes themselves live in
// styles/tokens.css as [data-palette="…"] blocks; this module only picks one.
// The choice is per device (localStorage), and ?palette=name in the URL
// overrides it, which makes comparing palettes on a phone quick.

export const PALETTES = [
  { key: 'tide', label: 'Tide', hint: 'slate + teal' },
  { key: 'sage', label: 'Sage', hint: 'green-grey' },
  { key: 'cobalt', label: 'Cobalt', hint: 'porcelain + blue' },
  { key: 'kiln', label: 'Kiln', hint: 'graphite + orange' },
] as const

export type PaletteKey = (typeof PALETTES)[number]['key']

export const DEFAULT_PALETTE: PaletteKey = 'tide'
const STORAGE_KEY = 'palette'

function isPalette(v: unknown): v is PaletteKey {
  return PALETTES.some((p) => p.key === v)
}

export function currentPalette(): PaletteKey {
  const v = document.documentElement.dataset.palette
  return isPalette(v) ? v : DEFAULT_PALETTE
}

export function applyPalette(key: PaletteKey, remember = true): void {
  document.documentElement.dataset.palette = key
  if (remember) {
    try {
      localStorage.setItem(STORAGE_KEY, key)
    } catch {
      // Private mode or blocked storage: the choice lasts for this tab.
    }
  }
  syncThemeColor()
}

/** Applies the palette from ?palette=, else the saved one, else the default. */
export function initPalette(): void {
  const fromUrl = new URLSearchParams(location.search).get('palette')
  if (isPalette(fromUrl)) {
    applyPalette(fromUrl)
    return
  }
  let saved: string | null = null
  try {
    saved = localStorage.getItem(STORAGE_KEY)
  } catch {
    // ignore
  }
  applyPalette(isPalette(saved) ? saved : DEFAULT_PALETTE, false)
}

// The browser chrome colour (address bar on Android) follows the page
// background of the active palette and colour scheme.
function syncThemeColor(): void {
  const bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim()
  if (!bg) return
  for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
    meta.content = bg
  }
}

if (typeof window !== 'undefined' && window.matchMedia) {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener?.('change', syncThemeColor)
}
