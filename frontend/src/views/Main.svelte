<script lang="ts">
  import { onMount } from 'svelte'
  import { api, on } from '../lib/api'
  import { t, queueLabel } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import { layers } from '../lib/layers.svelte'
  import { toast } from '../lib/toast.svelte'
  import { displayName, queueKey } from '../lib/itemview'
  import { ui, persistUi, select } from '../lib/mainui.svelte'
  import type { AppEvent, Item } from '../lib/types'
  import Icon from '../components/Icon.svelte'
  import TitleBar from '../components/TitleBar.svelte'
  import Dialogs from '../components/Dialogs.svelte'
  import Toasts from '../components/ui/Toasts.svelte'
  import Sidebar from '../components/main/Sidebar.svelte'
  import SearchBox from '../components/main/SearchBox.svelte'
  import Toolbar from '../components/main/Toolbar.svelte'
  import SelectionBar from '../components/main/SelectionBar.svelte'
  import DownloadList from '../components/main/DownloadList.svelte'
  import StatusBar from '../components/main/StatusBar.svelte'
  import ContextMenu from '../components/main/ContextMenu.svelte'
  import DetailsPanel from '../components/main/DetailsPanel.svelte'
  import SettingsModal from '../components/SettingsModal.svelte'
  import SchedulerModal from '../components/SchedulerModal.svelte'
  import GrabberModal from '../components/GrabberModal.svelte'
  import PropertiesModal from '../components/PropertiesModal.svelte'
  import { refreshQueues } from '../lib/state.svelte'

  let list: DownloadList | undefined = $state()
  let dragOver = $state(false)
  let dragTimer: ReturnType<typeof setTimeout> | undefined

  onMount(() => {
    const offEv = on<AppEvent>('fdm:event', (ev) => {
      if (ev.type === 'completed' && ev.item) {
        const it = ev.item
        toast(`${t('s_completed')} : ${displayName(it)}`, 'ok', { label: t('open') as string, run: () => api.openFile(it.id) })
      } else if (ev.type === 'failed' && ev.item) {
        const it = ev.item
        toast(`${displayName(it)} : ${it.error}`, 'err', { label: t('retry') as string, run: () => api.start([it.id]) })
      }
    })
    // the native (Wails) file drop never reaches our drop handler: reset the overlay
    const offDrop = on('common:WindowFilesDropped', () => (dragOver = false))
    const mq = window.matchMedia('(max-width: 1079px)')
    const onMq = () => { ui.narrow = mq.matches; if (!mq.matches) ui.drawer = false }
    onMq()
    mq.addEventListener?.('change', onMq)
    return () => { offEv(); offDrop(); mq.removeEventListener?.('change', onMq) }
  })

  function matches(it: Item, q: string) {
    const f = ui.filter
    switch (f.type) {
      case 'cat': if ((it.category || '') !== f.value) return false; break
      case 'queue': if (it.queue !== f.value) return false; break
      case 'status':
        if (f.value === 'unfinished' && it.status === 'completed') return false
        if (f.value === 'downloading' && it.status !== 'downloading' && it.status !== 'processing') return false
        if (f.value === 'completed' && it.status !== 'completed') return false
        if (f.value === 'error' && it.status !== 'error') return false
    }
    return !q || displayName(it).toLowerCase().includes(q) || it.url.toLowerCase().includes(q) || (it.title ?? '').toLowerCase().includes(q)
  }

  // Filtering only reads stable item fields; live progress is only read when
  // sorting by progress or speed, so the 4 Hz ticks don't re-sort the list otherwise.
  let filtered = $derived.by(() => {
    const q = ui.search.trim().toLowerCase()
    return store.items.filter((it) => matches(it, q))
  })

  let rows = $derived.by(() => {
    const dir = ui.sortAsc ? 1 : -1
    const col = ui.sortCol
    const live = (it: Item) => store.progress[it.id]
    const key = (it: Item): number | string => {
      switch (col) {
        case 'name': return displayName(it).toLowerCase()
        case 'size': return it.size
        case 'progress': {
          if (it.status === 'completed') return 101
          const p = live(it)
          const size = p?.size && p.size > 0 ? p.size : it.size
          return size > 0 ? ((p?.downloaded ?? it.downloaded) * 100) / size : 0
        }
        case 'speed': return it.status === 'downloading' ? (live(it)?.speed ?? 0) : 0
        case 'status': return it.status
        case 'queue': return 0
        default: return it.createdAt
      }
    }
    const arr = [...filtered]
    if (col === 'queue') {
      // priority first, then manual order / creation time
      return arr.sort((a, b) => ((b.priority ?? 0) - (a.priority ?? 0) || queueKey(a) - queueKey(b)) * dir)
    }
    return arr.sort((a, b) => {
      const ka = key(a), kb = key(b)
      return (ka < kb ? -1 : ka > kb ? 1 : 0) * dir
    })
  })

  let selItems = $derived(store.items.filter((i) => ui.selected.has(i.id)))
  let first = $derived(selItems.find((i) => i.id === ui.anchor) ?? selItems[0])
  let menuItems = $derived(first ? [first, ...selItems.filter((i) => i !== first)] : [])
  let detailsOpen = $derived(ui.narrow ? ui.drawer : ui.showDetails)

  let title = $derived.by(() => {
    const f = ui.filter
    if (f.type === 'all') return t('all') as string
    if (f.type === 'cat') return f.value || (t('general') as string)
    if (f.type === 'queue') return queueLabel(f.value)
    return ({ downloading: t('active'), unfinished: t('unfinished'), completed: t('finished'), error: t('errors') } as Record<string, string>)[f.value] ?? ''
  })

  function toggleDetails() {
    if (ui.narrow) ui.drawer = !ui.drawer
    else { ui.showDetails = !ui.showDetails; persistUi() }
  }

  function focusSearch() {
    const el = document.getElementById('main-search') as HTMLInputElement | null
    el?.focus()
    el?.select()
  }

  function key(e: KeyboardEvent) {
    // overlays (dialogs, menus, drawer) own the keyboard
    if (layers.depth > 0 || ui.modal) return
    const mod = e.ctrlKey || e.metaKey
    const k = e.key.toLowerCase()
    if (mod && (k === 'k' || k === 'f')) { e.preventDefault(); focusSearch(); return }
    if (mod && k === 'n') { e.preventDefault(); api.openAddDialog(''); return }
    if (mod && k === 'i') { e.preventDefault(); toggleDetails(); return }
    if ((e.target as HTMLElement).closest('input, textarea, select, [contenteditable]')) return
    if (mod && k === 'v') api.clipboardText().then((u) => { if (u) api.openAddDialog(u) })
    else if (mod && k === 'a') { e.preventDefault(); select(rows.map((r) => r.id)) }
    else if (e.key === 'Escape') {
      if (ui.selected.size) ui.selected = new Set()
      else if (ui.search) ui.search = ''
    }
  }

  // ---------- drop of links (files go through the native Wails drop) ----------
  const isRowDrag = (e: DragEvent) => !!e.dataTransfer?.types.includes('application/x-fdm-row')
  function onDragOver(e: DragEvent) {
    if (isRowDrag(e)) return
    e.preventDefault()
    dragOver = true
    // dragleave is unreliable (children, native drops): hide when dragover stops
    clearTimeout(dragTimer)
    dragTimer = setTimeout(() => (dragOver = false), 300)
  }
  function onDrop(e: DragEvent) {
    clearTimeout(dragTimer)
    dragOver = false
    if (isRowDrag(e)) return
    const txt = e.dataTransfer?.getData('text/uri-list') || e.dataTransfer?.getData('text/plain')
    if (txt) {
      e.preventDefault()
      txt.split(/\s+/).filter((u) => /^(https?|ftp|magnet):/i.test(u)).forEach((u) => api.openAddDialog(u))
    }
  }
</script>

<svelte:window onkeydown={key} ondragend={() => (dragOver = false)} ondrop={() => { clearTimeout(dragTimer); dragOver = false }} />

<div class="shell">
  {#if store.frameless}
    <TitleBar title="FluxDM">
      {#snippet center()}<SearchBox compact />{/snippet}
    </TitleBar>
  {/if}
  <div class="layout" data-file-drop-target role="presentation" ondragover={onDragOver} ondrop={onDrop}>
    <Sidebar filter={ui.filter} onfilter={(f) => { ui.filter = f; ui.selected = new Set(); ui.focusId = null }} />

    <main class="content" aria-label={title}>
      <Toolbar {title} count={rows.length} {detailsOpen} ontoggledetails={toggleDetails} />
      <SelectionBar items={selItems} />
      <DownloadList bind:this={list} {rows} onmenu={(a) => (ui.menu = { anchor: a })} />
      <StatusBar />
    </main>

    {#if !ui.narrow && ui.showDetails}<DetailsPanel item={first} onclose={toggleDetails} />{/if}
    {#if ui.narrow && ui.drawer}<DetailsPanel item={first} drawer onclose={() => (ui.drawer = false)} />{/if}

    {#if dragOver}
      <div class="dropzone" aria-hidden="true"><Icon name="download" size={36} /><span>{t('dropHere')}</span></div>
    {/if}
  </div>
</div>

{#if ui.menu && menuItems.length}
  <ContextMenu items={menuItems} anchor={ui.menu.anchor} canReorder={ui.sortCol === 'queue' && ui.sortAsc}
    onmove={(id, d) => list?.move(id, d)} onclose={() => (ui.menu = null)} />
{/if}

{#if ui.modal === 'settings'}<SettingsModal onclose={() => (ui.modal = '')} />{/if}
{#if ui.modal === 'scheduler'}<SchedulerModal onclose={() => { ui.modal = ''; refreshQueues() }} />{/if}
{#if ui.modal === 'grabber'}<GrabberModal onclose={() => (ui.modal = '')} />{/if}
{#if ui.modal === 'props'}<PropertiesModal id={ui.propsId} onclose={() => (ui.modal = '')} />{/if}
<Dialogs />
<Toasts />

<style>
  .shell { display: flex; flex-direction: column; height: 100%; }
  .layout { display: flex; flex: 1; min-height: 0; position: relative; }
  .content { flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--surface-1); border-top-left-radius: var(--r-lg); border: 1px solid var(--border); border-right: none; border-bottom: none; }
  :global(:root:not([data-frameless])) .content { border-top-left-radius: 0; border-top: none; }
  :global(:root[data-glass]) .content { background: var(--surface-0); }
  :global(:root[data-glass][data-theme='dark']) .content { background: rgba(32, 32, 32, 0.55); }
  .dropzone {
    position: absolute; inset: 10px; border: 2px dashed var(--accent); border-radius: var(--r-xl); background: var(--accent-soft);
    display: flex; flex-direction: column; gap: 10px; align-items: center; justify-content: center; font-size: var(--fs-subtitle);
    color: var(--accent-text); pointer-events: none; z-index: 80; animation: fade var(--dur-fast) var(--ease-decel);
  }
</style>
