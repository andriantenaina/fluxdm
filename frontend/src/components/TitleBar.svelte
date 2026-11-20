<script lang="ts">
  // Custom title bar for frameless windows (Windows 11 look): app icon, title,
  // optional centre content (search), caption buttons. Double-click maximises.
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import Icon from './Icon.svelte'
  import { api } from '../lib/api'
  import { store } from '../lib/state.svelte'
  import { t } from '../lib/i18n.svelte'
  import { tooltip } from '../lib/tooltip'

  let {
    title = 'FluxDM', subtitle = '', compact = false, minimisable = true, maximisable = true, onclose, center, actions,
  }: {
    title?: string
    subtitle?: string
    /** 32 px dialog bar instead of the 40 px main bar */
    compact?: boolean
    minimisable?: boolean
    maximisable?: boolean
    /** custom close (e.g. release a pending request first) */
    onclose?: () => void
    center?: Snippet
    actions?: Snippet
  } = $props()

  type Action = 'minimise' | 'toggleMaximise' | 'close'

  async function syncMax() {
    try { store.maximised = await Window.IsMaximised() } catch { /* runtime unavailable */ }
  }

  export async function windowAction(a: Action) {
    if (a === 'close' && onclose) { onclose(); return }
    try {
      if (a === 'minimise') await Window.Minimise()
      else if (a === 'toggleMaximise') { await Window.ToggleMaximise(); await syncMax() }
      else await Window.Close()
    } catch {
      await api.windowAction(store.win, a)
      if (a === 'toggleMaximise') store.maximised = !store.maximised
    }
  }

  onMount(() => {
    syncMax()
    let tm: ReturnType<typeof setTimeout> | undefined
    const onResize = () => { clearTimeout(tm); tm = setTimeout(syncMax, 120) }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  })

  function dbl(e: MouseEvent) {
    if (!maximisable || store.platform === 'darwin') return // macOS: handled by the runtime
    if ((e.target as HTMLElement).closest('button, input, select, a, [data-nodrag]')) return
    windowAction('toggleMaximise')
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions (double-click is a mouse shortcut; the caption buttons cover the keyboard) -->
<header class="tb drag" class:compact class:max={store.maximised} ondblclick={dbl}>
  <div class="brand">
    <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="var(--accent)" /><path d="M12 5.5v8.5M8 10.5l4 4 4-4M7 18h10" stroke="var(--accent-fg)" stroke-width="2.2" fill="none" stroke-linecap="round" stroke-linejoin="round" /></svg>
    <span class="title ellipsis">{title}</span>
    {#if subtitle}<span class="sub ellipsis">{subtitle}</span>{/if}
  </div>
  <div class="center">{#if center}{@render center()}{/if}</div>
  <div class="right nodrag">
    {#if actions}{@render actions()}{/if}
    <div class="caps" role="group" aria-label="Window">
      {#if minimisable}
        <button class="cap" aria-label={t('minimize')} use:tooltip={t('minimize')} onclick={() => windowAction('minimise')}><Icon name="winMin" size={16} stroke={1.2} /></button>
      {/if}
      {#if maximisable}
        <button class="cap" aria-label={store.maximised ? t('restore') : t('maximize')} use:tooltip={store.maximised ? t('restore') : t('maximize')}
          onclick={() => windowAction('toggleMaximise')}><Icon name={store.maximised ? 'winRestore' : 'winMax'} size={16} stroke={1.2} /></button>
      {/if}
      <button class="cap close" aria-label={t('close')} use:tooltip={t('close')} onclick={() => windowAction('close')}><Icon name="winClose" size={16} stroke={1.2} /></button>
    </div>
  </div>
</header>

<style>
  .tb { display: flex; align-items: center; height: 40px; flex: none; position: relative; z-index: 20; padding-left: 14px; gap: 12px; }
  .tb.compact { height: 32px; padding-left: 12px; }
  .brand { display: flex; align-items: center; gap: 10px; min-width: 0; flex: 1 1 0; }
  .title { font-size: var(--fs-caption); color: var(--fg); }
  .sub { font-size: var(--fs-caption); color: var(--muted); }
  .center { flex: 0 1 520px; display: flex; justify-content: center; min-width: 0; }
  .center:empty { flex: 0; }
  .right { display: flex; align-items: center; justify-content: flex-end; flex: 1 1 0; align-self: stretch; gap: 4px; }
  .caps { display: flex; align-self: stretch; }
  .cap {
    width: 46px; height: 100%; min-height: 0; padding: 0; border: none; border-radius: 0; background: transparent !important;
    display: flex; align-items: center; justify-content: center; color: var(--fg);
  }
  .compact .cap { width: 42px; }
  .cap:hover { background: var(--surface-3) !important; }
  .cap:active { background: var(--surface-4) !important; }
  .cap.close:hover { background: #c42b1c !important; color: #fff; }
  .cap.close:active { background: #c84031 !important; color: rgba(255, 255, 255, 0.8); }
  .cap:focus-visible { outline: 2px solid var(--focus); outline-offset: -2px; }
  :global(:root:not([data-glass])) .tb { background: var(--surface-0); }
</style>
