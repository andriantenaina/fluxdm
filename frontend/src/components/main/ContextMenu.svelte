<script lang="ts">
  import Menu, { type MenuEntry } from '../ui/Menu.svelte'
  import { api } from '../../lib/api'
  import { t, queueLabel } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import {
    ui, act, openItem, removeItems, cancelItems, setPriority, refreshUrl, copyUrl, moveToQueue, type MenuAnchor,
  } from '../../lib/mainui.svelte'
  import type { Item } from '../../lib/types'

  let { items, anchor, canReorder, onmove, onclose }: {
    /** selected items, the first one is the item the menu was opened on */
    items: Item[]
    anchor: MenuAnchor
    canReorder: boolean
    onmove: (id: string, dir: -1 | 1) => void
    onclose: () => void
  } = $props()

  let first = $derived(items[0])
  let ids = $derived(items.map((i) => i.id))

  let entries = $derived.by<MenuEntry[]>(() => {
    const it = first
    const st = it.status
    const running = st === 'downloading' || st === 'processing' || st === 'queued'
    const prio = it.priority ?? 0
    return [
      st === 'completed'
        ? { label: t('open'), icon: 'openFile', shortcut: 'Enter', onselect: () => openItem(it) }
        : { label: t('showProgress'), icon: 'gauge', shortcut: 'Enter', onselect: () => openItem(it) },
      { label: t('openFolder'), icon: 'folderOpen', onselect: () => act(() => api.openFolder(it.id)) },
      { kind: 'separator' },
      // one of Start / Pause, never both
      running
        ? { label: t('pause'), icon: 'pause', shortcut: 'Space', onselect: () => act(() => api.pause(ids)) }
        : st === 'error'
          ? { label: t('retry'), icon: 'refresh', shortcut: 'Space', onselect: () => act(() => api.start(ids)) }
          : { label: st === 'paused' ? t('resume') : t('start'), icon: 'play', shortcut: 'Space', hidden: st === 'completed', onselect: () => act(() => api.start(ids)) },
      { label: t('redownload'), icon: 'refresh', hidden: st !== 'completed', onselect: () => act(() => api.redownload(it.id)) },
      {
        label: t('priority'), icon: 'flag', hidden: st === 'completed',
        children: [
          { label: t('prioHigh'), checked: prio > 0, onselect: () => setPriority(ids, 1) },
          { label: t('prioNormal'), checked: prio === 0, onselect: () => setPriority(ids, 0) },
          { label: t('prioLow'), checked: prio < 0, onselect: () => setPriority(ids, -1) },
        ],
      },
      { label: t('moveUp'), icon: 'arrowUp', shortcut: 'Alt+↑', hidden: !canReorder || items.length !== 1, onselect: () => onmove(it.id, -1) },
      { label: t('moveDown'), icon: 'arrowDown', shortcut: 'Alt+↓', hidden: !canReorder || items.length !== 1, onselect: () => onmove(it.id, 1) },
      {
        label: t('moveTo'), icon: 'queue', hidden: store.queues.length < 2,
        children: store.queues.map((q) => ({ label: queueLabel(q.name), checked: items.every((i) => i.queue === q.name), onselect: () => moveToQueue(ids, q.name) })),
      },
      { kind: 'separator' },
      { label: t('refreshUrl'), icon: 'link', hidden: items.length !== 1, onselect: () => refreshUrl(it) },
      { label: t('copyUrl'), icon: 'copy', hidden: items.length !== 1, onselect: () => copyUrl(it) },
      { label: t('properties'), icon: 'info', hidden: items.length !== 1, onselect: () => { ui.propsId = it.id; ui.modal = 'props' } },
      { kind: 'separator' },
      { label: t('cancelDownload'), icon: 'xCircle', danger: true, hidden: items.every((i) => i.status === 'completed'), onselect: () => cancelItems(items) },
      { label: t('delete'), icon: 'trash', danger: true, shortcut: t('kbdDelete'), onselect: () => removeItems(ids, false) },
      { label: t('deleteWithFile'), icon: 'trash', danger: true, onselect: () => removeItems(ids, true) },
    ]
  })
</script>

{#if first}<Menu items={entries} {anchor} label={first.fileName || first.title || first.url} minWidth={240} {onclose} />{/if}
