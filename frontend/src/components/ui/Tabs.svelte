<script lang="ts" module>
  export type TabDef = { id: string; label: string; icon?: string; badge?: string | number; hidden?: boolean }
  /** ids shared between a tab and its panel */
  export const tabId = (prefix: string, id: string) => `${prefix}-tab-${id}`
  export const panelId = (prefix: string, id: string) => `${prefix}-panel-${id}`
</script>

<script lang="ts">
  import Icon from '../Icon.svelte'

  let {
    tabs, value = $bindable(), prefix, label, orientation = 'horizontal', variant = 'pill', onchange,
  }: {
    tabs: TabDef[]
    value: string
    /** unique prefix for tab/panel ids */
    prefix: string
    label: string
    orientation?: 'horizontal' | 'vertical'
    variant?: 'pill' | 'underline' | 'nav'
    onchange?: (id: string) => void
  } = $props()

  let list: HTMLElement | undefined = $state()
  let visible = $derived(tabs.filter((t) => !t.hidden))

  function select(id: string, focus = false) {
    value = id
    onchange?.(id)
    if (focus) queueMicrotask(() => list?.querySelector<HTMLElement>(`#${CSS.escape(tabId(prefix, id))}`)?.focus())
  }

  function key(e: KeyboardEvent) {
    const i = visible.findIndex((t) => t.id === value)
    const prev = orientation === 'vertical' ? 'ArrowUp' : 'ArrowLeft'
    const next = orientation === 'vertical' ? 'ArrowDown' : 'ArrowRight'
    let j = -1
    if (e.key === next) j = (i + 1) % visible.length
    else if (e.key === prev) j = (i - 1 + visible.length) % visible.length
    else if (e.key === 'Home') j = 0
    else if (e.key === 'End') j = visible.length - 1
    if (j < 0) return
    e.preventDefault()
    select(visible[j].id, true)
  }
</script>

<div bind:this={list} class="tabs {variant} {orientation}" role="tablist" aria-label={label} aria-orientation={orientation} tabindex="-1" onkeydown={key}>
  {#each visible as tb (tb.id)}
    <button type="button" role="tab" id={tabId(prefix, tb.id)} aria-controls={panelId(prefix, tb.id)} aria-selected={value === tb.id}
      tabindex={value === tb.id ? 0 : -1} class:on={value === tb.id} onclick={() => select(tb.id)}>
      {#if tb.icon}<Icon name={tb.icon} size={16} />{/if}
      <span class="lbl">{tb.label}</span>
      {#if tb.badge !== undefined && tb.badge !== ''}<span class="badge">{tb.badge}</span>{/if}
    </button>
  {/each}
</div>

<style>
  .tabs { display: flex; gap: 2px; }
  .tabs:focus { outline: none; }
  .vertical { flex-direction: column; }
  button { display: flex; align-items: center; gap: 8px; background: none; border: 1px solid transparent; color: var(--fg2); min-height: 32px; padding: 5px 12px; text-align: left; position: relative; }
  :global(:root[data-theme='dark']) button:not(.on) { background: none; }
  button:hover:not(.on) { background: var(--surface-3); }
  .lbl { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .badge { font-size: var(--fs-micro); font-weight: 600; background: var(--accent-soft); color: var(--accent-text); border-radius: 8px; padding: 0 6px; }

  .pill { background: var(--surface-3); padding: 3px; border-radius: var(--r-md); width: fit-content; }
  .pill button { border-radius: 6px; }
  .pill button.on { background: var(--surface-1); color: var(--fg); font-weight: 600; box-shadow: var(--elev-2); }

  .underline { border-bottom: 1px solid var(--border); gap: 4px; }
  .underline button { border-radius: var(--r-sm) var(--r-sm) 0 0; }
  .underline button.on { color: var(--fg); font-weight: 600; }
  .underline button.on::after { content: ''; position: absolute; left: 12px; right: 12px; bottom: -1px; height: 3px; border-radius: 2px; background: var(--accent); }

  .nav button { border-radius: var(--r-sm); width: 100%; }
  .nav button.on { background: var(--surface-3); color: var(--fg); font-weight: 600; }
  .nav button.on::before { content: ''; position: absolute; left: 0; top: 50%; height: 16px; margin-top: -8px; width: 3px; border-radius: 2px; background: var(--accent); }
  .nav :global(svg) { color: var(--muted); }
  .nav .on :global(svg) { color: var(--accent-text); }
</style>
