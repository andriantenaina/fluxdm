<script lang="ts">
  /** Small area chart of speeds (bytes/s). Redraws on resize and theme change. */
  import { themeState, withAlpha } from '../../lib/theme.svelte'

  let {
    data = [], height = 40, color = 'var(--accent)', samples = 120, avgLine = false, label = '',
  }: { data?: number[]; height?: number; color?: string; samples?: number; avgLine?: boolean; label?: string } = $props()
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
    const values = data.slice(-samples)
    const w = width || c.clientWidth || 200
    const h = height
    const dpr = window.devicePixelRatio || 1
    c.width = Math.round(w * dpr)
    c.height = Math.round(h * dpr)
    const g = c.getContext('2d')
    if (!g) return
    g.setTransform(dpr, 0, 0, dpr, 0, 0)
    g.clearRect(0, 0, w, h)
    if (values.length < 2) return
    const max = Math.max(...values, 1)
    const col = getComputedStyle(c).getPropertyValue('--spark').trim() || '#0f6cbd'
    const step = w / (samples - 1)
    const x0 = w - (values.length - 1) * step
    const yOf = (v: number) => h - 2 - (v / max) * (h - 6)
    g.beginPath()
    values.forEach((v, i) => {
      const x = x0 + i * step
      i === 0 ? g.moveTo(x, yOf(v)) : g.lineTo(x, yOf(v))
    })
    g.strokeStyle = col
    g.lineWidth = 1.6
    g.lineJoin = 'round'
    g.stroke()
    g.lineTo(w, h)
    g.lineTo(x0, h)
    g.closePath()
    const grad = g.createLinearGradient(0, 0, 0, h)
    grad.addColorStop(0, withAlpha(col, 0.32))
    grad.addColorStop(1, withAlpha(col, 0))
    g.fillStyle = grad
    g.fill()
    if (avgLine) {
      const nz = values.filter((v) => v > 0)
      if (nz.length) {
        const avg = nz.reduce((a, b) => a + b, 0) / nz.length
        g.setLineDash([3, 3])
        g.strokeStyle = withAlpha(getComputedStyle(c).getPropertyValue('--muted').trim() || '#888', 0.8)
        g.lineWidth = 1
        g.beginPath()
        g.moveTo(0, Math.round(yOf(avg)) + 0.5)
        g.lineTo(w, Math.round(yOf(avg)) + 0.5)
        g.stroke()
        g.setLineDash([])
      }
    }
  })
</script>

<canvas bind:this={canvas} style:height="{height}px" style:--spark={color} role={label ? 'img' : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}></canvas>

<style>
  canvas { width: 100%; display: block; }
</style>
