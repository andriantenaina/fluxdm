<script lang="ts">
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store, refreshQueues } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { Item } from '../lib/types'
  import Icon from '../components/Icon.svelte'
  import Dialogs from '../components/Dialogs.svelte'
  import { alertBox, confirmBox, promptBox } from '../lib/dialogs.svelte'
  import SettingsModal from '../components/SettingsModal.svelte'
  import SchedulerModal from '../components/SchedulerModal.svelte'
  import GrabberModal from '../components/GrabberModal.svelte'
  import PropertiesModal from '../components/PropertiesModal.svelte'

  type Filter = { type: 'all' | 'cat' | 'status' | 'queue'; value: string }
  let filter = $state<Filter>({ type: 'all', value: '' })
  let search = $state('')
  let sortCol = $state<'name' | 'size' | 'progress' | 'speed' | 'eta' | 'status' | 'date'>('date')
  let sortAsc = $state(false)
  let selected = $state<Set<string>>(new Set())
  let anchor = $state<string | null>(null)
  let menu = $state<{ x: number; y: number } | null>(null)
  let limiterMenu = $state(false)
  let modal = $state<'' | 'settings' | 'scheduler' | 'grabber' | 'props'>('')
  let propsId = $state('')
  let dragOver = $state(false)

  const catIcon: Record<string, string> = {
    'Compressés': 'archive', Documents: 'doc', Musique: 'music', Programmes: 'app', 'Vidéos': 'video', Images: 'image', Torrents: 'magnet',
  }
  const statusFilters = ['downloading', 'unfinished', 'completed', 'error'] as const

  function displayName(it: Item) {
    return it.fileName || it.title || it.url
  }
  function prog(it: Item) {
    return store.progress[it.id]
  }
  function pct(it: Item) {
    if (it.status === 'completed') return 100
    const p = prog(it)
    const size = p?.size && p.size > 0 ? p.size : it.size
    return fmt.percent(p?.downloaded ?? it.downloaded, size)
  }
  function etaOf(it: Item) {
    return it.status === 'downloading' ? (prog(it)?.eta ?? -1) : -1
  }
  function speedOf(it: Item) {
    return it.status === 'downloading' ? (prog(it)?.speed ?? 0) : 0
  }

  let rows = $derived.by(() => {
    const q = search.trim().toLowerCase()
    let list = store.items.filter((it) => {
      switch (filter.type) {
        case 'cat':
          if ((it.category || '') !== filter.value) return false
          break
        case 'status':
          if (filter.value === 'unfinished' && it.status === 'completed') return false
          if (filter.value === 'downloading' && it.status !== 'downloading' && it.status !== 'processing') return false
          if (filter.value === 'completed' && it.status !== 'completed') return false
          if (filter.value === 'error' && it.status !== 'error') return false
          break
        case 'queue':
          if (it.queue !== filter.value) return false
          break
      }
      if (q && !(displayName(it).toLowerCase().includes(q) || it.url.toLowerCase().includes(q) || (it.title ?? '').toLowerCase().includes(q))) return false
      return true
    })
    const dir = sortAsc ? 1 : -1
    const key = (it: Item): number | string => {
      switch (sortCol) {
        case 'name': return displayName(it).toLowerCase()
        case 'size': return it.size
        case 'progress': return pct(it)
        case 'speed': return speedOf(it)
        case 'eta': return etaOf(it)
        case 'status': return it.status
        default: return it.createdAt
      }
    }
    return [...list].sort((a, b) => {
      const ka = key(a), kb = key(b)
      return (ka < kb ? -1 : ka > kb ? 1 : 0) * dir
    })
  })

  let totalSpeed = $derived(Object.values(store.progress).reduce((s, p) => s + (p.speed || 0), 0))
  let activeCount = $derived(store.items.filter((i) => i.status === 'downloading' || i.status === 'processing').length)
  let categories = $derived(store.settings?.categories.map((c) => c.name) ?? [])

  function count(f: Filter) {
    return store.items.filter((it) => {
      if (f.type === 'cat') return (it.category || '') === f.value
      if (f.type === 'queue') return it.queue === f.value
      if (f.type === 'status') {
        if (f.value === 'unfinished') return it.status !== 'completed'
        if (f.value === 'downloading') return it.status === 'downloading' || it.status === 'processing'
        return it.status === f.value
      }
      return true
    }).length
  }

  function setSort(c: typeof sortCol) {
    if (sortCol === c) sortAsc = !sortAsc
    else { sortCol = c; sortAsc = c === 'name' }
  }

  function rowClick(e: MouseEvent, it: Item) {
    const ids = rows.map((r) => r.id)
    if (e.shiftKey && anchor) {
      const a = ids.indexOf(anchor), b = ids.indexOf(it.id)
      const [lo, hi] = a < b ? [a, b] : [b, a]
      selected = new Set(ids.slice(lo, hi + 1))
    } else if (e.ctrlKey || e.metaKey) {
      const s = new Set(selected)
      s.has(it.id) ? s.delete(it.id) : s.add(it.id)
      selected = s
      anchor = it.id
    } else {
      selected = new Set([it.id])
      anchor = it.id
    }
  }

  function rowMenu(e: MouseEvent, it: Item) {
    if (!selected.has(it.id)) {
      selected = new Set([it.id])
      anchor = it.id
    }
    menu = { x: Math.min(e.clientX, window.innerWidth - 240), y: Math.min(e.clientY, window.innerHeight - 380) }
  }

  function dbl(it: Item) {
    if (it.status === 'completed') api.openFile(it.id).catch((e) => alertBox(errMsg(e)))
    else api.openProgress(it.id)
  }

  let sel = $derived([...selected].filter((id) => store.items.some((i) => i.id === id)))
  let selItems = $derived(store.items.filter((i) => selected.has(i.id)))
  let first = $derived(selItems[0])

  async function act(fn: () => Promise<any>) {
    menu = null
    try { await fn() } catch (e) { alertBox(errMsg(e)) }
  }

  async function removeSel(withFiles = false) {
    if (!sel.length) return
    menu = null
    const r = await confirmBox(`${t('confirmDelete')} (${sel.length})`, withFiles ? undefined : t('confirmDeleteFiles'))
    if (!r.ok) return
    await api.remove(sel, withFiles || r.checked)
    selected = new Set()
  }

  async function refreshUrl() {
    menu = null
    if (!first) return
    const u = await promptBox(t('newUrl'), first.url)
    if (u && u !== first.url) act(() => api.updateURL(first.id, u))
  }

  async function removeCompleted() {
    await api.removeCompleted()
  }

  async function setLimit(kbps: number) {
    limiterMenu = false
    await api.setSpeedLimit(kbps * 1024)
  }

  async function openTorrent() {
    const p = await api.chooseTorrent()
    if (p) api.openAddDialog(p)
  }

  async function startQueueSel() {
    const q = filter.type === 'queue' ? filter.value : 'main'
    await api.startQueue(q)
    refreshQueues()
  }
  async function stopQueueSel() {
    const q = filter.type === 'queue' ? filter.value : 'main'
    await api.stopQueue(q)
    refreshQueues()
  }

  function key(e: KeyboardEvent) {
    if (modal || (e.target as HTMLElement).closest('input, textarea, select')) return
    if (e.key === 'Delete') removeSel()
    else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') {
      e.preventDefault()
      selected = new Set(rows.map((r) => r.id))
    } else if (e.key === 'Enter' && first) dbl(first)
    else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
      e.preventDefault()
      api.openAddDialog('')
    } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'v') {
      api.clipboardText().then((u) => { if (u) api.openAddDialog(u) })
    }
  }

  function onDrop(e: DragEvent) {
    dragOver = false
    const dt = e.dataTransfer
    if (!dt) return
    const txt = dt.getData('text/uri-list') || dt.getData('text/plain')
    if (txt) {
      e.preventDefault()
      const urls = txt.split(/\s+/).filter((u) => /^(https?|ftp|magnet):/i.test(u))
      urls.forEach((u) => api.openAddDialog(u))
    }
  }

  function statusLabel(it: Item) {
    const k = ('s_' + it.status) as 's_queued'
    let s = t(k) ?? it.status
    const p = prog(it)
    if (it.status === 'downloading' && p?.extra?.phase === 'metadata') s = '…'
    return s
  }

  function kindTag(it: Item) {
    return { hls: 'HLS', dash: 'DASH', ytdlp: 'yt-dlp', torrent: 'Torrent', hoster: 'Hébergeur', ftp: 'FTP', http: '' }[it.kind] ?? ''
  }

  const limits = [0, 64, 128, 256, 512, 1024, 2048, 5120, 10240]
</script>

<svelte:window onkeydown={key} onclick={() => { menu = null; limiterMenu = false }} />

<div
  class="layout"
  role="application"
  ondragover={(e) => { e.preventDefault(); dragOver = true }}
  ondragleave={(e) => { if (e.target === e.currentTarget) dragOver = false }}
  ondrop={onDrop}
>
  <div class="toolbar">
    <button class="tb primary" onclick={() => api.openAddDialog('')} title="Ctrl+N"><Icon name="plus" size={18} /><span>{t('add')}</span></button>
    <button class="tb" disabled={!sel.length} onclick={() => act(() => api.start(sel))}><Icon name="play" size={18} /><span>{t('resume')}</span></button>
    <button class="tb" disabled={!sel.length} onclick={() => act(() => api.pause(sel))}><Icon name="pause" size={18} /><span>{t('stop')}</span></button>
    <button class="tb" onclick={() => act(() => api.pauseAll())}><Icon name="stopAll" size={18} /><span>{t('stopAll')}</span></button>
    <button class="tb" disabled={!sel.length} onclick={() => removeSel()}><Icon name="trash" size={18} /><span>{t('delete')}</span></button>
    <button class="tb" onclick={removeCompleted}><Icon name="broom" size={18} /><span>{t('deleteDone')}</span></button>
    <div class="sep"></div>
    <button class="tb" onclick={() => (modal = 'settings')}><Icon name="gear" size={18} /><span>{t('options')}</span></button>
    <button class="tb" onclick={() => (modal = 'scheduler')}><Icon name="clock" size={18} /><span>{t('scheduler')}</span></button>
    <button class="tb" onclick={startQueueSel}><Icon name="queue" size={18} /><span>{t('startQueue')}</span></button>
    <button class="tb" onclick={stopQueueSel}><Icon name="stop" size={18} /><span>{t('stopQueue')}</span></button>
    <div class="sep"></div>
    <button class="tb" onclick={() => (modal = 'grabber')}><Icon name="spider" size={18} /><span>{t('grabber')}</span></button>
    <button class="tb" onclick={openTorrent}><Icon name="magnet" size={18} /><span>{t('torrent')}</span></button>
    <button class="tb" onclick={() => act(() => api.importList())} title="Importer une liste d'adresses (.txt)"><Icon name="import" size={18} /><span>Importer</span></button>
    <button class="tb" onclick={() => act(() => api.exportList())} title="Exporter la liste des adresses (.txt)"><Icon name="export" size={18} /><span>Exporter</span></button>
    <div class="limiter">
      <button class="tb" class:on={(store.settings?.speedLimit ?? 0) > 0} onclick={(e) => { e.stopPropagation(); limiterMenu = !limiterMenu }}>
        <Icon name="gauge" size={18} /><span>{(store.settings?.speedLimit ?? 0) > 0 ? fmt.speed(store.settings!.speedLimit) : t('speedLimiter')}</span>
      </button>
      {#if limiterMenu}
        <div class="menu limits" role="menu">
          {#each limits as l}
            <button role="menuitem" onclick={() => setLimit(l)}>{l === 0 ? t('unlimited') : fmt.speed(l * 1024)}</button>
          {/each}
        </div>
      {/if}
    </div>
    <div class="grow"></div>
    <div class="searchbox">
      <Icon name="search" size={14} />
      <input placeholder={t('search')} bind:value={search} />
    </div>
  </div>

  <div class="main">
    <nav class="side">
      <button class="nav" class:active={filter.type === 'all'} onclick={() => (filter = { type: 'all', value: '' })}>
        <Icon name="list" /><span class="grow">{t('all')}</span><span class="n">{store.items.length}</span>
      </button>
      <div class="h">{t('categories')}</div>
      {#each categories as c}
        <button class="nav" class:active={filter.type === 'cat' && filter.value === c} onclick={() => (filter = { type: 'cat', value: c })}>
          <Icon name={catIcon[c] ?? 'folder'} /><span class="grow ellipsis">{c}</span><span class="n">{count({ type: 'cat', value: c })}</span>
        </button>
      {/each}
      <button class="nav" class:active={filter.type === 'cat' && filter.value === ''} onclick={() => (filter = { type: 'cat', value: '' })}>
        <Icon name="file" /><span class="grow">{t('general')}</span><span class="n">{count({ type: 'cat', value: '' })}</span>
      </button>
      <div class="h">{t('status')}</div>
      {#each statusFilters as s}
        <button class="nav" class:active={filter.type === 'status' && filter.value === s} onclick={() => (filter = { type: 'status', value: s })}>
          <Icon name={s === 'completed' ? 'check' : s === 'error' ? 'alert' : s === 'downloading' ? 'download' : 'clock'} />
          <span class="grow">{s === 'downloading' ? t('active') : s === 'unfinished' ? t('unfinished') : s === 'completed' ? t('finished') : t('errors')}</span>
          <span class="n">{count({ type: 'status', value: s })}</span>
        </button>
      {/each}
      <div class="h">{t('queues')}</div>
      {#each store.queues as q}
        <button class="nav" class:active={filter.type === 'queue' && filter.value === q.name} onclick={() => (filter = { type: 'queue', value: q.name })}>
          <Icon name="queue" /><span class="grow ellipsis">{q.name === 'main' ? 'Principale' : q.name}</span>
          {#if q.running && q.name !== 'main'}<span class="dot"></span>{/if}
          <span class="n">{count({ type: 'queue', value: q.name })}</span>
        </button>
      {/each}
    </nav>

    <div class="tablewrap">
      <table class="list">
        <thead>
          <tr>
            <th class="c-name" onclick={() => setSort('name')}>{t('name')} {sortCol === 'name' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-size" onclick={() => setSort('size')}>{t('size')} {sortCol === 'size' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-prog" onclick={() => setSort('progress')}>{t('progress')} {sortCol === 'progress' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-speed" onclick={() => setSort('speed')}>{t('speed')} {sortCol === 'speed' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-eta" onclick={() => setSort('eta')}>{t('timeLeft')} {sortCol === 'eta' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-status" onclick={() => setSort('status')}>{t('status')} {sortCol === 'status' ? (sortAsc ? '▲' : '▼') : ''}</th>
            <th class="c-date" onclick={() => setSort('date')}>{t('lastTry')} {sortCol === 'date' ? (sortAsc ? '▲' : '▼') : ''}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as it (it.id)}
            {@const p = pct(it)}
            <tr
              class:sel={selected.has(it.id)}
              class:err={it.status === 'error'}
              onclick={(e) => rowClick(e, it)}
              ondblclick={() => dbl(it)}
              oncontextmenu={(e) => { e.preventDefault(); e.stopPropagation(); rowMenu(e, it) }}
              title={it.error || it.url}
            >
              <td class="c-name">
                <span class="nm">
                  <Icon name={it.kind === 'torrent' ? 'magnet' : catIcon[it.category] ?? 'file'} size={15} />
                  <span class="ellipsis">{displayName(it)}</span>
                  {#if kindTag(it)}<span class="tag">{kindTag(it)}</span>{/if}
                </span>
              </td>
              <td class="c-size">{it.size > 0 ? fmt.size(it.size) : it.downloaded > 0 ? fmt.size(it.downloaded) : ''}</td>
              <td class="c-prog">
                <div class="pc">
                  <div class="bar" class:done={it.status === 'completed'} class:error={it.status === 'error'}
                    class:indeterminate={it.status === 'downloading' && !(it.size > 0)}>
                    <div style:width="{p}%"></div>
                  </div>
                  <span>{it.size > 0 || it.status === 'completed' ? p.toFixed(1) + '%' : ''}</span>
                </div>
              </td>
              <td class="c-speed">{it.status === 'downloading' ? fmt.speed(speedOf(it)) : ''}</td>
              <td class="c-eta">{it.status === 'downloading' ? fmt.duration(etaOf(it)) : ''}</td>
              <td class="c-status"><span class="st st-{it.status}">{statusLabel(it)}</span></td>
              <td class="c-date muted">{fmt.date(it.completedAt || it.createdAt)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if rows.length === 0}
        <div class="empty">
          <Icon name="download" size={40} />
          <p>{t('dropHere')}</p>
          <button class="primary" onclick={() => api.openAddDialog('')}>{t('add')}</button>
        </div>
      {/if}
    </div>
  </div>

  <div class="statusbar">
    <span>{store.items.length} {t('items')}</span>
    <span>{activeCount} {t('activeN')}</span>
    <span class="grow"></span>
    {#if (store.settings?.speedLimit ?? 0) > 0}<span class="warn">⚠ {t('speedLimiter')} : {fmt.speed(store.settings!.speedLimit)}</span>{/if}
    <span><Icon name="download" size={13} /> {fmt.speed(totalSpeed)}</span>
  </div>

  {#if dragOver}<div class="dropzone">{t('dropHere')}</div>{/if}
</div>

{#if menu && first}
  <div class="menu ctx" style:left="{menu.x}px" style:top="{menu.y}px" role="menu">
    {#if first.status === 'completed'}
      <button onclick={() => act(() => api.openFile(first.id))}><Icon name="file" />{t('open')}</button>
    {/if}
    <button onclick={() => act(() => api.openFolder(first.id))}><Icon name="folder" />{t('openFolder')}</button>
    <hr />
    <button onclick={() => act(() => api.start(sel))}><Icon name="play" />{t('start')}</button>
    <button onclick={() => act(() => api.pause(sel))}><Icon name="pause" />{t('pause')}</button>
    <button onclick={() => act(() => api.openProgress(first.id))}><Icon name="gauge" />{t('showProgress')}</button>
    <button onclick={() => act(() => api.redownload(first.id))}><Icon name="refresh" />{t('redownload')}</button>
    <button onclick={refreshUrl}><Icon name="link" />{t('refreshUrl')}</button>
    <button onclick={() => act(() => navigator.clipboard.writeText(first.url))}><Icon name="link" />{t('copyUrl')}</button>
    {#if store.queues.length > 1}
      <hr />
      <div class="sub">{t('moveTo')}</div>
      {#each store.queues as q}
        <button onclick={() => act(() => api.moveToQueue(sel, q.name))}><Icon name="queue" />{q.name === 'main' ? 'Principale' : q.name}</button>
      {/each}
    {/if}
    <hr />
    <button onclick={() => { menu = null; propsId = first.id; modal = 'props' }}><Icon name="gear" />{t('properties')}</button>
    <button class="danger" onclick={() => removeSel(false)}><Icon name="trash" />{t('delete')}</button>
    <button class="danger" onclick={() => removeSel(true)}><Icon name="trash" />{t('deleteWithFile')}</button>
  </div>
{/if}

{#if modal === 'settings'}<SettingsModal onclose={() => (modal = '')} />{/if}
{#if modal === 'scheduler'}<SchedulerModal onclose={() => { modal = ''; refreshQueues() }} />{/if}
{#if modal === 'grabber'}<GrabberModal onclose={() => (modal = '')} />{/if}
{#if modal === 'props'}<PropertiesModal id={propsId} onclose={() => (modal = '')} />{/if}
<Dialogs />

<style>
  .layout { display: flex; flex-direction: column; height: 100%; position: relative; }
  .toolbar { display: flex; align-items: center; gap: 2px; padding: 6px 8px; background: var(--bg2); border-bottom: 1px solid var(--border); flex-wrap: nowrap; overflow-x: auto; }
  .tb { display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 5px 8px; background: none; border: 1px solid transparent; font-size: 11.5px; min-width: 62px; }
  .tb:hover:not(:disabled) { background: var(--bg3); border-color: transparent; filter: none; }
  .tb.primary { color: var(--accent); background: none; }
  .tb.on { color: var(--warn); }
  .sep { width: 1px; height: 34px; background: var(--border); margin: 0 4px; }
  .grow { flex: 1; }
  .limiter { position: relative; }
  .searchbox { display: flex; align-items: center; gap: 6px; background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 0 8px; width: 220px; color: var(--muted); }
  .searchbox input { border: none; background: none; padding: 6px 0; }
  .main { flex: 1; display: flex; min-height: 0; }
  .side { width: 210px; flex-shrink: 0; background: var(--bg2); border-right: 1px solid var(--border); overflow-y: auto; padding: 6px; }
  .side .h { font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); margin: 12px 8px 4px; }
  .nav { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; background: none; border: none; padding: 6px 8px; border-radius: 6px; }
  .nav:hover { background: var(--bg3); filter: none; }
  .nav.active { background: var(--sel); color: var(--accent); }
  .nav .n { color: var(--muted); font-size: 11px; }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
  .tablewrap { flex: 1; overflow: auto; position: relative; background: var(--bg2); }
  table.list { width: 100%; border-collapse: collapse; table-layout: fixed; }
  table.list th { position: sticky; top: 0; background: var(--bg2); text-align: left; font-weight: 600; font-size: 12px; color: var(--muted); padding: 7px 8px; border-bottom: 1px solid var(--border); cursor: pointer; white-space: nowrap; overflow: hidden; z-index: 1; }
  table.list td { padding: 5px 8px; border-bottom: 1px solid var(--border); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  table.list tr:hover td { background: var(--bg3); }
  table.list tr.sel td { background: var(--sel); }
  table.list tr.err .c-name { color: var(--err); }
  .c-name { width: auto; }
  .c-size { width: 90px; text-align: right; }
  .c-prog { width: 170px; }
  .c-speed { width: 100px; text-align: right; }
  .c-eta { width: 110px; }
  .c-status { width: 105px; }
  .c-date { width: 130px; font-size: 12px; }
  th.c-size, th.c-speed { text-align: right; }
  .nm { display: flex; align-items: center; gap: 7px; min-width: 0; }
  .nm :global(svg) { flex-shrink: 0; color: var(--muted); }
  .pc { display: flex; align-items: center; gap: 6px; }
  .pc .bar { flex: 1; }
  .pc span { width: 44px; text-align: right; font-size: 11.5px; color: var(--muted); }
  .st { font-size: 12px; }
  .st-downloading { color: var(--accent); }
  .st-completed { color: var(--ok); }
  .st-error { color: var(--err); }
  .st-processing { color: var(--warn); }
  .st-paused, .st-queued { color: var(--muted); }
  .empty { position: absolute; inset: 60px 0 0 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; color: var(--muted); pointer-events: none; }
  .empty button { pointer-events: auto; }
  .statusbar { display: flex; gap: 18px; padding: 4px 12px; font-size: 12px; color: var(--muted); background: var(--bg2); border-top: 1px solid var(--border); align-items: center; }
  .statusbar span { display: flex; align-items: center; gap: 4px; }
  .warn { color: var(--warn); }
  .menu { position: fixed; z-index: 40; background: var(--bg2); border: 1px solid var(--border); border-radius: 8px; box-shadow: var(--shadow); padding: 4px; min-width: 220px; }
  .menu.limits { position: absolute; top: 100%; left: 0; min-width: 140px; }
  .menu button { display: flex; align-items: center; gap: 8px; width: 100%; background: none; border: none; text-align: left; padding: 6px 10px; border-radius: 5px; }
  .menu button:hover { background: var(--sel); filter: none; }
  .menu hr { border: none; border-top: 1px solid var(--border); margin: 4px 0; }
  .menu .sub { font-size: 11px; color: var(--muted); padding: 2px 10px; }
  .menu .danger { color: var(--err); }
  .dropzone { position: absolute; inset: 8px; border: 2px dashed var(--accent); border-radius: 12px; background: rgba(59, 140, 255, 0.1); display: flex; align-items: center; justify-content: center; font-size: 16px; color: var(--accent); pointer-events: none; z-index: 60; }
  @media (max-width: 980px) { .tb span { display: none; } .tb { min-width: 36px; } .c-date, .c-eta { display: none; } }
</style>
