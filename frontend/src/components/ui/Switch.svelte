<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    checked = $bindable(false), disabled = false, label = '', description = '', onchange, children, id,
  }: {
    checked?: boolean
    disabled?: boolean
    label?: string
    description?: string
    onchange?: (v: boolean) => void
    children?: Snippet
    id?: string
  } = $props()
  const uid = 'sw-' + Math.random().toString(36).slice(2, 8)
  let inputId = $derived(id ?? uid)
</script>

<label class="switch" class:disabled for={inputId}>
  <span class="ctl">
    <input id={inputId} type="checkbox" role="switch" bind:checked {disabled} aria-checked={checked}
      aria-describedby={description ? inputId + '-d' : undefined} onchange={() => onchange?.(checked)} />
    <span class="track" aria-hidden="true"><span class="thumb"></span></span>
  </span>
  <span class="txt">
    {#if children}{@render children()}{:else}{label}{/if}
    {#if description}<span class="desc" id={inputId + '-d'}>{description}</span>{/if}
  </span>
</label>

<style>
  .switch { display: flex; align-items: flex-start; gap: 12px; padding: 6px 0; }
  .switch.disabled { opacity: 0.55; }
  .ctl { position: relative; flex: none; width: 40px; height: 20px; margin-top: 1px; }
  /* the real input stays focusable and clickable, just invisible */
  input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; z-index: 1; }
  .track {
    position: absolute; inset: 0; border-radius: 10px; border: 1px solid var(--muted); background: transparent;
    transition: background var(--dur-fast), border-color var(--dur-fast);
  }
  .thumb {
    position: absolute; top: 50%; left: 4px; width: 12px; height: 12px; margin-top: -6px; border-radius: 50%; background: var(--fg2);
    transition: transform var(--dur-fast) var(--ease-spring), width var(--dur-fast), background var(--dur-fast);
  }
  input:hover:not(:disabled) + .track .thumb { width: 14px; height: 14px; margin-top: -7px; }
  input:checked + .track { background: var(--accent); border-color: var(--accent); }
  input:checked + .track .thumb { transform: translateX(19px); background: var(--accent-fg); }
  input:checked:hover:not(:disabled) + .track { background: var(--accent-hover); }
  input:focus-visible + .track { outline: 2px solid var(--focus); outline-offset: 2px; }
  .txt { display: flex; flex-direction: column; gap: 2px; line-height: 1.4; min-width: 0; }
  .desc { color: var(--muted); font-size: var(--fs-caption); }
</style>
