<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    value = 0, size = 32, stroke = 3, indeterminate = false, tone = 'accent', label = '', children,
  }: {
    value?: number
    size?: number
    stroke?: number
    indeterminate?: boolean
    tone?: 'accent' | 'ok' | 'err' | 'paused'
    /** Accessible name; empty = decorative. */
    label?: string
    children?: Snippet
  } = $props()

  let r = $derived((size - stroke) / 2)
  let c = $derived(2 * Math.PI * r)
  let v = $derived(Math.max(0, Math.min(100, value || 0)))
  let off = $derived(indeterminate ? c * 0.72 : c * (1 - v / 100))
</script>

<span class="ring {tone}" class:indet={indeterminate} style:width="{size}px" style:height="{size}px"
  role={label ? 'progressbar' : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}
  aria-valuemin={label && !indeterminate ? 0 : undefined} aria-valuemax={label && !indeterminate ? 100 : undefined}
  aria-valuenow={label && !indeterminate ? Math.round(v) : undefined}>
  <svg width={size} height={size} viewBox="0 0 {size} {size}">
    {#if !indeterminate}<circle class="track" cx={size / 2} cy={size / 2} {r} stroke-width={stroke} fill="none" />{/if}
    <circle class="val" cx={size / 2} cy={size / 2} {r} stroke-width={stroke} fill="none" stroke-linecap="round"
      stroke-dasharray={c} stroke-dashoffset={off} transform="rotate(-90 {size / 2} {size / 2})" />
  </svg>
  {#if children}<span class="center">{@render children()}</span>{/if}
</span>

<style>
  .ring { position: relative; display: inline-flex; align-items: center; justify-content: center; flex: none; }
  svg { position: absolute; inset: 0; overflow: visible; }
  .track { stroke: var(--bar); }
  .val { stroke: var(--accent); transition: stroke-dashoffset var(--dur-slow) var(--ease-decel), stroke var(--dur); }
  .ok .val { stroke: var(--ok); }
  .err .val { stroke: var(--err); }
  .paused .val { stroke: var(--muted); }
  .indet svg { animation: rot 1s linear infinite; }
  .center { position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }
</style>
