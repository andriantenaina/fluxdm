<script lang="ts">
  // Drawn success check: circle then check mark (stroke-dashoffset), static under reduced motion.
  import { onMount } from 'svelte'
  let { size = 56, label = '' }: { size?: number; label?: string } = $props()
  let drawn = $state(false)
  onMount(() => {
    const r = requestAnimationFrame(() => requestAnimationFrame(() => (drawn = true)))
    return () => cancelAnimationFrame(r)
  })
</script>

<svg class="sc" class:drawn width={size} height={size} viewBox="0 0 52 52" role={label ? 'img' : undefined}
  aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}>
  <circle class="bg" cx="26" cy="26" r="24" />
  <circle class="ring" cx="26" cy="26" r="24" />
  <path class="tick" d="M15 27l7.5 7.5L38 19" />
</svg>

<style>
  .sc { flex: none; display: block; }
  .bg { fill: var(--ok-soft); }
  .ring { fill: none; stroke: var(--ok); stroke-width: 3; stroke-dasharray: 151; stroke-dashoffset: 151; transform: rotate(-90deg); transform-origin: center;
    transition: stroke-dashoffset var(--dur-slow) var(--ease-decel); }
  .tick { fill: none; stroke: var(--ok); stroke-width: 4; stroke-linecap: round; stroke-linejoin: round; stroke-dasharray: 36; stroke-dashoffset: 36;
    transition: stroke-dashoffset var(--dur) var(--ease-decel) var(--dur-slow); }
  .drawn .ring, .drawn .tick { stroke-dashoffset: 0; }
  @media (prefers-reduced-motion: reduce) { .ring, .tick { stroke-dashoffset: 0; transition: none; } }
</style>
