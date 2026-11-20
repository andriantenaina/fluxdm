<script lang="ts">
  import Icon from '../Icon.svelte'

  type Item = { label: string; icon?: string; shortcut?: string; danger?: boolean; checked?: boolean; disabled?: boolean; children?: unknown[] }
  let {
    entry, index, expanded = false, onactivate, onhover,
  }: {
    entry: Item
    index: number
    expanded?: boolean
    onactivate: (target: HTMLElement) => void
    onhover: (target: HTMLElement) => void
  } = $props()

  let hasSub = $derived(!!entry.children?.length)
  let role = $derived(entry.checked !== undefined ? 'menuitemcheckbox' : 'menuitem')
</script>

<button type="button" {role} class="mi" class:danger={entry.danger} data-idx={index} tabindex="-1"
  aria-checked={entry.checked !== undefined ? entry.checked : undefined}
  aria-disabled={entry.disabled || undefined} aria-haspopup={hasSub ? 'menu' : undefined} aria-expanded={hasSub ? expanded : undefined}
  onclick={(e) => onactivate(e.currentTarget)}
  onpointerenter={(e) => { e.currentTarget.focus({ preventScroll: true }); onhover(e.currentTarget) }}>
  <span class="ic">
    {#if entry.checked}<Icon name="check" size={16} />{:else if entry.icon}<Icon name={entry.icon} size={16} />{/if}
  </span>
  <span class="lbl">{entry.label}</span>
  {#if entry.shortcut}<span class="sc">{entry.shortcut}</span>{/if}
  {#if hasSub}<Icon name="chevronRight" size={14} />{/if}
</button>

<style>
  .mi {
    display: flex; align-items: center; gap: 10px; width: 100%; min-height: 32px; padding: 0 12px 0 8px; text-align: left;
    background: none !important; border: none; border-radius: var(--r-sm); color: var(--fg); position: relative;
  }
  .mi:focus { outline: none; }
  .mi:focus, .mi[aria-expanded='true'] { background: var(--surface-3) !important; }
  .mi:focus-visible { outline: 2px solid var(--focus); outline-offset: -2px; }
  .mi[aria-disabled='true'] { opacity: 0.45; }
  .ic { width: 16px; display: flex; color: var(--fg2); }
  .lbl { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .sc { color: var(--muted); font-size: var(--fs-caption); }
  .danger, .danger .ic { color: var(--err); }
</style>
