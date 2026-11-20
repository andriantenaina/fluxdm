<script lang="ts">
  // Map of the file: one colour per connection/segment, drawn from theme tokens
  // and redrawn on resize and theme change.
  import type { Segment } from '../lib/types'
  import { themeState, withAlpha } from '../lib/theme.svelte'

  let {
    segments = [], size = 0, done = false, height = 14, label = '',
  }: { segments?: Segment[]; size?: number; done?: boolean; height?: number; label?: string } = $props()

  const TOKENS = ['--accent', '--c-video', '--c-image', '--c-app', '--c-archive', '--c-audio', '--c-torrent', '--c-doc']

  let canvas: HTMLCanvasElement | undefined = $state()
  let width = $state(0)

  $effect(() => {
    const c = canvas
    if (!c) return
    const ro = new ResizeObserver(() => (width = c.clientWidth))
    ro.observe(c)
    return () => ro.disconnect()
  })

  $effect(() => {
    const c = canvas
    if (!c) return
    void themeState.tick
    const segs = segments
    const w = width || c.clientWidth || 300
    const h = height
    const dpr = window.devicePixelRatio || 1
    c.width = Math.round(w * dpr)
    c.height = Math.round(h * dpr)
    const g = c.getContext('2d')
    if (!g) return
    g.setTransform(dpr, 0, 0, dpr, 0, 0)
    const css = getComputedStyle(c)
    const tok = (n: string, f: string) => css.getPropertyValue(n).trim() || f
    g.fillStyle = tok('--bar', '#e0e0e0')
    g.fillRect(0, 0, w, h)
    if (done) {
      g.fillStyle = tok('--ok', '#107c10')
      g.fillRect(0, 0, w, h)
      return
    }
    if (!size || size <= 0) return
    const palette = TOKENS.map((n) => tok(n, '#0f6cbd'))
    const sep = tok('--surface-1', '#ffffff')
    segs.forEach((s, i) => {
      const col = palette[i % palette.length]
      const x0 = (s.s / size) * w
      const x1 = (s.p / size) * w
      const end = s.e > 0 ? (s.e / size) * w : w
      g.fillStyle = withAlpha(col, 0.22)
      g.fillRect(x0, 0, end - x0, h)
      g.fillStyle = col
      g.fillRect(x0, 0, Math.max(1, x1 - x0), h)
      if (i > 0) {
        g.fillStyle = withAlpha(sep, 0.85)
        g.fillRect(x0, 0, 1, h)
      }
    })
  })
</script>

<div class="map" role="img" aria-label={label || `${segments.length} segments`}>
  <canvas bind:this={canvas} style:height="{height}px" aria-hidden="true"></canvas>
</div>

<style>
  .map { width: 100%; }
  canvas { width: 100%; border-radius: var(--r-sm); display: block; }
</style>
