<script lang="ts">
  import type { Segment } from '../lib/types'
  let { segments = [], size = 0, done = false }: { segments?: Segment[]; size?: number; done?: boolean } = $props()

  let canvas: HTMLCanvasElement | undefined = $state()

  $effect(() => {
    const c = canvas
    if (!c) return
    const segs = segments
    const w = c.clientWidth, h = c.clientHeight
    const dpr = window.devicePixelRatio || 1
    c.width = w * dpr
    c.height = h * dpr
    const g = c.getContext('2d')!
    g.scale(dpr, dpr)
    const css = getComputedStyle(c)
    g.fillStyle = css.getPropertyValue('--bar') || '#ddd'
    g.fillRect(0, 0, w, h)
    const accent = css.getPropertyValue('--accent') || '#1f7ae0'
    const ok = css.getPropertyValue('--ok') || '#1f9d5c'
    if (done) {
      g.fillStyle = ok
      g.fillRect(0, 0, w, h)
      return
    }
    if (!size || size <= 0) return
    for (const s of segs) {
      const x0 = (s.s / size) * w
      const x1 = (s.p / size) * w
      g.fillStyle = accent
      g.fillRect(x0, 0, Math.max(1, x1 - x0), h)
      // segment boundary
      g.fillStyle = 'rgba(0,0,0,0.25)'
      g.fillRect(x0, 0, 1, h)
    }
  })
</script>

<canvas bind:this={canvas}></canvas>

<style>
  canvas { width: 100%; height: 16px; border-radius: 4px; display: block; }
</style>
