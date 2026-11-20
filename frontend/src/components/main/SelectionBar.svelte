<script lang="ts">
  import { fade } from 'svelte/transition'
  import Button from '../ui/Button.svelte'
  import IconButton from '../ui/IconButton.svelte'
  import Menu, { type MenuEntry } from '../ui/Menu.svelte'
  import { api } from '../../lib/api'
  import { t, tn, queueLabel } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import { dur } from '../../lib/motion'
  import { ui, act, removeItems, cancelItems, setPriority, moveToQueue } from '../../lib/mainui.svelte'
  import type { Item } from '../../lib/types'

  let { items }: { items: Item[] } = $props()
  let ids = $derived(items.map((i) => i.id))
  let anyRunning = $derived(items.some((i) => i.status === 'downloading' || i.status === 'processing' || i.status === 'queued'))
  let anyStoppable = $derived(items.some((i) => i.status !== 'completed' && i.status !== 'downloading' && i.status !== 'processing'))
  let anyUnfinished = $derived(items.some((i) => i.status !== 'completed'))

  let open = $state<'' | 'prio' | 'queue'>('')
  let prioBtn: HTMLButtonElement | undefined = $state()
  let queueBtn: HTMLButtonElement | undefined = $state()

  let prioItems = $derived<MenuEntry[]>([
    { label: t('prioHigh'), icon: 'arrowUp', onselect: () => setPriority(ids, 1) },
    { label: t('prioNormal'), icon: 'minus', onselect: () => setPriority(ids, 0) },
    { label: t('prioLow'), icon: 'arrowDown', onselect: () => setPriority(ids, -1) },
  ])
  let queueItems = $derived<MenuEntry[]>(store.queues.map((q) => ({ label: queueLabel(q.name), icon: 'queue', onselect: () => moveToQueue(ids, q.name) })))
</script>

{#if items.length > 0}
  <div class="selbar" role="toolbar" aria-label={tn('selectedCount', items.length)} in:fade={{ duration: dur(167) }}>
    <span class="count num" aria-live="polite">{tn('selectedCount', items.length)}</span>
    {#if anyStoppable || !anyRunning}<Button size="sm" variant="subtle" icon="play" onclick={() => act(() => api.start(ids))}>{t('resume')}</Button>{/if}
    {#if anyRunning}<Button size="sm" variant="subtle" icon="pause" onclick={() => act(() => api.pause(ids))}>{t('pause')}</Button>{/if}
    <Button size="sm" variant="subtle" icon="flag" iconEnd="chevronDown" bind:el={prioBtn} aria-haspopup="menu" aria-expanded={open === 'prio'}
      onclick={() => (open = open === 'prio' ? '' : 'prio')}>{t('priority')}</Button>
    {#if store.queues.length > 1}
      <Button size="sm" variant="subtle" icon="queue" iconEnd="chevronDown" bind:el={queueBtn} aria-haspopup="menu" aria-expanded={open === 'queue'}
        onclick={() => (open = open === 'queue' ? '' : 'queue')}>{t('moveToQueue')}</Button>
    {/if}
    {#if anyUnfinished}<Button size="sm" variant="subtle" icon="xCircle" onclick={() => cancelItems(items)}>{t('cancel')}</Button>{/if}
    <Button size="sm" variant="subtle" icon="trash" class="danger" onclick={() => removeItems(ids)}>{t('delete')}</Button>
    <span class="grow"></span>
    <IconButton icon="x" size="sm" label={t('clearSelection')} shortcut="Esc" onclick={() => (ui.selected = new Set())} />
  </div>
{:else}
  <div class="selbar quiet">
    <Button size="sm" variant="subtle" icon="play" onclick={() => act(() => api.startAll())}>{t('resumeAll')}</Button>
    <Button size="sm" variant="subtle" icon="pause" onclick={() => act(() => api.pauseAll())}>{t('stopAll')}</Button>
  </div>
{/if}

{#if open === 'prio' && prioBtn}<Menu items={prioItems} anchor={prioBtn} label={t('priority')} minWidth={160} onclose={() => (open = '')} />{/if}
{#if open === 'queue' && queueBtn}<Menu items={queueItems} anchor={queueBtn} label={t('moveToQueue')} minWidth={180} onclose={() => (open = '')} />{/if}

<style>
  .selbar { display: flex; align-items: center; gap: 4px; padding: 0 12px 8px 16px; min-height: 40px; }
  .selbar:not(.quiet) { margin: 0 8px 6px; padding: 4px 6px 4px 12px; border-radius: var(--r-md); background: var(--accent-soft); }
  .count { font-weight: 600; margin-right: 8px; color: var(--fg); }
  .selbar :global(.danger) { color: var(--err); }
</style>
