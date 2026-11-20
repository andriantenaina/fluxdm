<script lang="ts" module>
  export type SideItem = { id: string; label: string; icon?: string; mark?: { icon: string; label: string; tone?: 'accent' | 'ok' } }
</script>

<script lang="ts">
  // Side list shared by Settings-like modals (Scheduler, Grabber): a listbox
  // with roving focus (arrows, Home/End) and an optional footer (add form).
  import type { Snippet } from 'svelte'
  import Icon from '../Icon.svelte'

  let {
    items, value, onselect, label, heading = '', footer,
  }: {
    items: SideItem[]
    value: string
    onselect: (id: string) => void
    label: string
    heading?: string
    footer?: Snippet
  } = $props()

  let list: HTMLElement | undefined = $state()

  function key(e: KeyboardEvent) {
    const i = items.findIndex((x) => x.id === value)
    let j = -1
    if (e.key === 'ArrowDown') j = Math.min(items.length - 1, i + 1)
    else if (e.key === 'ArrowUp') j = Math.max(0, i - 1)
    else if (e.key === 'Home') j = 0
    else if (e.key === 'End') j = items.length - 1
    if (j < 0 || !items[j]) return
    e.preventDefault()
    onselect(items[j].id)
    queueMicrotask(() => list?.querySelector<HTMLElement>(`[data-id="${CSS.escape(items[j].id)}"]`)?.focus())
  }
</script>

<aside class="side">
  {#if heading}<div class="h" aria-hidden="true">{heading}</div>{/if}
  <div bind:this={list} class="list" role="listbox" aria-label={label} tabindex="-1" onkeydown={key}>
    {#each items as it (it.id)}
      <button type="button" role="option" data-id={it.id} aria-selected={value === it.id} tabindex={value === it.id || (!items.some((x) => x.id === value) && it === items[0]) ? 0 : -1}
        class:on={value === it.id} onclick={() => onselect(it.id)}>
        {#if it.icon}<Icon name={it.icon} size={16} />{/if}
        <span class="lbl">{it.label}</span>
        {#if it.mark}<span class="mark {it.mark.tone ?? 'accent'}" title={it.mark.label}><Icon name={it.mark.icon} size={14} /><span class="sr-only">{it.mark.label}</span></span>{/if}
      </button>
    {/each}
  </div>
  {#if footer}<div class="foot">{@render footer()}</div>{/if}
</aside>

<style>
  .side { width: 190px; flex-shrink: 0; display: flex; flex-direction: column; gap: 4px; min-height: 0; }
  .h { font-size: var(--fs-caption); color: var(--muted); font-weight: 600; padding: 0 8px 4px; }
  .list { display: flex; flex-direction: column; gap: 2px; overflow-y: auto; min-height: 0; }
  .list:focus { outline: none; }
  button { display: flex; align-items: center; gap: 8px; text-align: left; background: none; border: 1px solid transparent; color: var(--fg2); min-height: 32px; padding: 4px 10px; position: relative; }
  :global(:root[data-theme='dark']) button:not(.on) { background: none; }
  button:hover:not(.on) { background: var(--surface-3); }
  button.on { background: var(--surface-3); color: var(--fg); font-weight: 600; }
  button.on::before { content: ''; position: absolute; left: 0; top: 50%; height: 16px; margin-top: -8px; width: 3px; border-radius: 2px; background: var(--accent); }
  .lbl { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mark { display: flex; color: var(--accent-text); }
  .mark.ok { color: var(--ok); }
  .foot { margin-top: 8px; }
</style>
