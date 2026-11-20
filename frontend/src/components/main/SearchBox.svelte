<script lang="ts">
  import Icon from '../Icon.svelte'
  import IconButton from '../ui/IconButton.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { ui } from '../../lib/mainui.svelte'

  let { compact = false }: { compact?: boolean } = $props()
  let input: HTMLInputElement | undefined = $state()
</script>

<div class="search nodrag" class:compact role="search">
  <Icon name="search" size={16} />
  <input bind:this={input} id="main-search" type="search" bind:value={ui.search} placeholder={t('searchShortcut')} aria-label={t('searchLabel')}
    autocomplete="off" spellcheck="false"
    onkeydown={(e) => {
      if (e.key === 'Escape' && ui.search) { e.preventDefault(); e.stopPropagation(); ui.search = '' }
      else if (e.key === 'ArrowDown' || (e.key === 'Enter' && !e.isComposing)) {
        // jump into the list
        const row = document.querySelector<HTMLElement>('[role="grid"] [role="row"][tabindex="0"]') ?? document.querySelector<HTMLElement>('[role="grid"] [role="row"][aria-rowindex]')
        if (row) { e.preventDefault(); row.focus() }
      }
    }} />
  {#if ui.search}<IconButton icon="x" size="sm" label={t('clearSearch')} tip={false} onclick={() => { ui.search = ''; input?.focus() }} />{/if}
</div>

<style>
  .search {
    flex: 1; max-width: 520px; min-width: 160px; display: flex; align-items: center; gap: 8px; height: 32px; padding: 0 4px 0 10px;
    background: var(--surface-1); border: 1px solid var(--border); border-bottom-color: var(--border-strong); border-radius: var(--r-sm); color: var(--muted);
    transition: background var(--dur-fast), border-color var(--dur-fast);
  }
  .search.compact { height: 30px; }
  :global(:root[data-glass]) .search { background: var(--surface-2); }
  .search:hover { background: var(--surface-2); }
  .search:focus-within { background: var(--surface-1); border-bottom-color: var(--accent); box-shadow: inset 0 -1px 0 var(--accent); }
  input { border: none !important; background: none !important; box-shadow: none !important; padding: 0; min-height: 0; height: 100%; flex: 1; min-width: 0; color: var(--fg); }
  input::-webkit-search-cancel-button { display: none; }
  input:focus-visible { outline: none; }
</style>
