<script lang="ts">
  import Button from '../ui/Button.svelte'
  import IconButton from '../ui/IconButton.svelte'
  import Menu, { type MenuEntry } from '../ui/Menu.svelte'
  import SearchBox from './SearchBox.svelte'
  import { api } from '../../lib/api'
  import { t, tn } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import * as fmt from '../../lib/format'
  import {
    ui, setSort, persistUi, act, setLimit, customLimit, startQueue, stopQueue, openTorrent, type SortCol,
  } from '../../lib/mainui.svelte'

  let { title, count, detailsOpen, ontoggledetails }: { title: string; count: number; detailsOpen: boolean; ontoggledetails: () => void } = $props()

  let open = $state<'' | 'sort' | 'limit' | 'more'>('')
  let sortBtn: HTMLButtonElement | undefined = $state()
  let limitBtn: HTMLButtonElement | undefined = $state()
  let moreBtn: HTMLButtonElement | undefined = $state()

  let limit = $derived(store.settings?.speedLimit ?? 0)
  const LIMITS = [0, 256, 512, 1024, 2048, 5120, 10240, 25600]

  const sortLabels: [SortCol, string][] = [
    ['queue', 'sortQueue'], ['date', 'sortDate'], ['name', 'name'], ['size', 'size'], ['progress', 'progress'], ['speed', 'speed'], ['status', 'status'],
  ]

  let sortItems = $derived<MenuEntry[]>([
    { kind: 'header', label: t('sortBy') },
    ...sortLabels.map(([k, l]): MenuEntry => ({
      label: t(l as 'name') + (ui.sortCol === k ? ` (${ui.sortAsc ? t('sortAsc') : t('sortDesc')})` : ''),
      checked: ui.sortCol === k, icon: undefined, onselect: () => setSort(k),
    })),
    { kind: 'separator' },
    { kind: 'header', label: t('view') },
    { label: t('viewCards'), checked: ui.view === 'cards', onselect: () => { ui.view = 'cards'; persistUi() } },
    { label: t('viewCompact'), checked: ui.view === 'compact', onselect: () => { ui.view = 'compact'; persistUi() } },
  ])

  let limitItems = $derived<MenuEntry[]>([
    { kind: 'header', label: t('speedLimiter') },
    ...LIMITS.map((l): MenuEntry => ({ label: l === 0 ? t('unlimited') : fmt.speed(l * 1024), checked: limit === l * 1024, onselect: () => setLimit(l) })),
    { kind: 'separator' },
    { label: t('customLimit'), icon: 'tune', onselect: customLimit },
  ])

  let moreItems = $derived<MenuEntry[]>([
    { label: t('scheduler'), icon: 'alarm', onselect: () => (ui.modal = 'scheduler') },
    { label: t('startQueue'), icon: 'play', onselect: startQueue },
    { label: t('stopQueue'), icon: 'stop', onselect: stopQueue },
    { kind: 'separator' },
    { label: t('grabber'), icon: 'spider', onselect: () => (ui.modal = 'grabber') },
    { label: t('openTorrent'), icon: 'magnet', onselect: openTorrent },
    { label: t('importList'), icon: 'import', onselect: () => act(() => api.importList()) },
    { label: t('exportList'), icon: 'export', onselect: () => act(() => api.exportList()) },
    { kind: 'separator' },
    { label: t('deleteDone'), icon: 'broom', onselect: () => act(() => api.removeCompleted(), t('completedRemoved') as string) },
  ])
</script>

<div class="toolbar" class:drag={!store.frameless}>
  <div class="titlebox">
    <h1 class="ellipsis">{title}</h1>
    <span class="muted small num">{tn('items', count)}</span>
  </div>
  {#if !store.frameless}<SearchBox />{:else}<span class="grow"></span>{/if}
  <div class="tools nodrag" role="toolbar" aria-label={t('options')}>
    <IconButton bind:el={sortBtn} icon="sort" label={t('sortBy')} aria-haspopup="menu" aria-expanded={open === 'sort'}
      onclick={() => (open = open === 'sort' ? '' : 'sort')} />
    <IconButton bind:el={limitBtn} icon="gauge" label={limit > 0 ? `${t('speedLimiter')} : ${fmt.speed(limit)}` : t('speedLimiter')} tone={limit > 0 ? 'warn' : ''}
      aria-haspopup="menu" aria-expanded={open === 'limit'} onclick={() => (open = open === 'limit' ? '' : 'limit')} />
    <IconButton icon="panel" label={t('detailsToggle')} shortcut="Ctrl+I" pressed={detailsOpen} onclick={ontoggledetails} />
    <IconButton bind:el={moreBtn} icon="more" label={t('more')} aria-haspopup="menu" aria-expanded={open === 'more'}
      onclick={() => (open = open === 'more' ? '' : 'more')} />
    <IconButton icon="gear" label={t('options')} onclick={() => (ui.modal = 'settings')} />
    <Button variant="primary" icon="plus" class="add" title={t('addShortcut')} onclick={() => api.openAddDialog('')}>{t('add')}</Button>
  </div>
</div>

{#if open === 'sort' && sortBtn}<Menu items={sortItems} anchor={sortBtn} placement="bottom-end" label={t('sortBy')} onclose={() => (open = '')} />{/if}
{#if open === 'limit' && limitBtn}<Menu items={limitItems} anchor={limitBtn} placement="bottom-end" label={t('speedLimiter')} minWidth={180} onclose={() => (open = '')} />{/if}
{#if open === 'more' && moreBtn}<Menu items={moreItems} anchor={moreBtn} placement="bottom-end" label={t('more')} onclose={() => (open = '')} />{/if}

<style>
  .toolbar { display: flex; align-items: center; gap: 16px; padding: 10px 12px 8px 20px; min-height: 52px; }
  .titlebox { display: flex; align-items: baseline; gap: 10px; min-width: 0; flex: 0 1 auto; }
  h1 { margin: 0; font-size: var(--fs-title); font-weight: 600; white-space: nowrap; }
  .tools { display: flex; align-items: center; gap: 2px; margin-left: auto; }
  .tools :global(.add) { margin-left: 8px; }
</style>
