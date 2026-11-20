// Theme resolution (system / light / dark) and a reactive tick that canvases
// read so they redraw when the palette changes.

export const themeState = $state({ pref: 'system', dark: false, tick: 0 })

const mq = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null
let watching = false

function lsSet(k: string, v: string) {
  try { localStorage.setItem(k, v) } catch { /* private mode */ }
}

function resolve() {
  const dark = themeState.pref === 'dark' || (themeState.pref !== 'light' && !!mq?.matches)
  themeState.dark = dark
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

function watch() {
  if (watching) return
  watching = true
  const onChange = () => { if (themeState.pref === 'system') resolve() }
  if (mq) {
    if (mq.addEventListener) mq.addEventListener('change', onChange)
    else (mq as MediaQueryList).addListener(onChange) // Safari < 14 fallback
  }
  // Any change of data-theme / data-glass bumps the tick (canvases redraw).
  new MutationObserver(() => themeState.tick++).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme', 'data-glass'],
  })
}

/** Applies the user preference ('system' | 'light' | 'dark') and remembers it for the next window. */
export function applyTheme(pref: string) {
  watch()
  themeState.pref = pref === 'light' || pref === 'dark' ? pref : 'system'
  lsSet('fdm.theme', themeState.pref)
  resolve()
}

export function applyGlass(on: boolean) {
  if (on) document.documentElement.dataset.glass = '1'
  else delete document.documentElement.dataset.glass
  lsSet('fdm.glass', on ? '1' : '0')
}

/** Reads a CSS custom property as a colour usable by canvas (any CSS colour syntax). */
export function cssColor(el: Element, name: string, fallback: string): string {
  const v = getComputedStyle(el).getPropertyValue(name).trim()
  return v || fallback
}

let probe: CanvasRenderingContext2D | null = null

/** Parses any CSS colour (hex, rgb(), hsl(), named) to [r, g, b, a]. */
export function parseColor(c: string): [number, number, number, number] {
  const hex = /^#([0-9a-f]{3,8})$/i.exec(c.trim())
  if (hex) {
    let h = hex[1]
    if (h.length === 3 || h.length === 4) h = h.split('').map((x) => x + x).join('')
    const n = (i: number) => parseInt(h.slice(i, i + 2), 16)
    return [n(0), n(2), n(4), h.length === 8 ? n(6) / 255 : 1]
  }
  const rgb = /^rgba?\(([^)]+)\)$/i.exec(c.trim())
  if (rgb) {
    const p = rgb[1].split(/[\s,/]+/).filter(Boolean).map((x) => (x.endsWith('%') ? (parseFloat(x) * 255) / 100 : parseFloat(x)))
    return [p[0] || 0, p[1] || 0, p[2] || 0, p.length > 3 ? (rgb[1].includes('%') && p[3] > 1 ? p[3] / 255 : p[3]) : 1]
  }
  // let the browser normalise anything else (named colours, hsl...)
  try {
    probe ??= document.createElement('canvas').getContext('2d')
    if (probe) {
      probe.fillStyle = '#000'
      probe.fillStyle = c
      const v = String(probe.fillStyle)
      if (v !== c) return parseColor(v)
    }
  } catch { /* ignore */ }
  return [0, 0, 0, 1]
}

/** Same colour with another alpha, as rgba(). */
export function withAlpha(c: string, a: number): string {
  const [r, g, b] = parseColor(c)
  return `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${a})`
}
