<script lang="ts">
  let {
    value = 0, indeterminate = false, tone = 'accent', size = 'sm', label, valueText,
  }: {
    /** 0..100 */
    value?: number
    indeterminate?: boolean
    tone?: 'accent' | 'ok' | 'err' | 'paused' | 'warn'
    size?: 'xs' | 'sm' | 'md' | 'lg'
    /** Accessible name (required for a meaningful progressbar). */
    label: string
    valueText?: string
  } = $props()
  let v = $derived(Math.max(0, Math.min(100, value || 0)))
</script>

<div class="progress {tone} {size}" class:indet={indeterminate} role="progressbar" aria-label={label}
  aria-valuemin={indeterminate ? undefined : 0} aria-valuemax={indeterminate ? undefined : 100}
  aria-valuenow={indeterminate ? undefined : Math.round(v)} aria-valuetext={valueText}>
  <div class="fill" style:transform={indeterminate ? undefined : `scaleX(${v / 100})`}></div>
</div>

<style>
  .progress { position: relative; height: 4px; border-radius: 2px; background: var(--bar); overflow: hidden; width: 100%; }
  .xs { height: 3px; }
  .md { height: 6px; border-radius: 3px; }
  .lg { height: 8px; border-radius: 4px; }
  .fill { position: absolute; inset: 0; transform-origin: left center; background: var(--accent); border-radius: inherit; transition: transform var(--dur) linear, background var(--dur); }
  .ok .fill { background: var(--ok); }
  .err .fill { background: var(--err); }
  .warn .fill { background: var(--warn); }
  .paused .fill { background: var(--muted); }
  .indet .fill { right: auto; width: 40%; animation: indet 1.4s var(--ease-p2p) infinite; }
</style>
