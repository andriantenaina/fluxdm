<script lang="ts">
  // Light "odometer": tweens a number and renders it through a formatter.
  // Under prefers-reduced-motion the value is shown as is.
  import { Tween, prefersReducedMotion } from 'svelte/motion'
  import { cubicOut } from 'svelte/easing'

  let { value, format = (n: number) => String(Math.round(n)), duration = 400 }: { value: number; format?: (n: number) => string; duration?: number } = $props()

  // svelte-ignore state_referenced_locally
  const tw = new Tween(value || 0, { duration, easing: cubicOut })
  $effect(() => {
    const v = value || 0
    // big jumps (resume after pause, new item) are not animated
    const jump = tw.current > 0 && (v > tw.current * 4 || v < tw.current / 4)
    tw.set(v, { duration: prefersReducedMotion.current || jump ? 0 : duration })
  })
</script>

<span class="num">{format(tw.current)}</span>
