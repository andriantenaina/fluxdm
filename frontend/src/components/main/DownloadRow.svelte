<script lang="ts" module>
  export type RowAction = 'start' | 'pause' | 'open' | 'folder' | 'retry' | 'progress'
</script>

<script lang="ts">
  import Icon from '../Icon.svelte'
  import IconButton from '../ui/IconButton.svelte'
  import Progress from '../ui/Progress.svelte'
  import FileBadge from '../ui/FileBadge.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import * as fmt from '../../lib/format'
  import { displayName, statusText, statusLabel, etaOf, isRunning, priorityLabel } from '../../lib/itemview'
  import type { Item } from '../../lib/types'

  let {
    item, index, compact = false, selected = false, focused = false, canDrag = false, armed = false, drop = null, dragging = false,
    onpointer, onopen, onmenu, onaction, onarm,
  }: {
    item: Item
    /** 1-based, for aria-rowindex */
    index: number
    compact?: boolean
    selected?: boolean
    /** roving tabindex owner */
    focused?: boolean
    canDrag?: boolean
    /** the drag handle is pressed: the row is draggable */
    armed?: boolean
    drop?: 'before' | 'after' | null
    dragging?: boolean
    onpointer: (e: MouseEvent) => void
    onopen: () => void
    onmenu: (e: MouseEvent | HTMLElement) => void
    onaction: (a: RowAction) => void
    /** pointer down on the drag handle */
    onarm: () => void
  } = $props()

  let p = $derived(store.progress[item.id])
  let size = $derived(p?.size && p.size > 0 ? p.size : item.size)
  let done = $derived(p?.downloaded ?? item.downloaded)
  let pct = $derived(item.status === 'completed' ? 100 : fmt.percent(done, size))
  let running = $derived(isRunning(item.status))
  let name = $derived(displayName(item))
  let host = $derived(fmt.hostOf(item.pageUrl || item.url))
  let prio = $derived(item.priority ?? 0)
  let eta = $derived(etaOf(item, p))
  let etaText = $derived(eta ? fmt.fmtEta(eta) : '')
  let clock = $derived(eta && eta.eta && eta.eta > 0 ? fmt.fmtClock(eta.eta) : '')
  let status = $derived(statusText(item, p))
  let tone = $derived<'accent' | 'err' | 'paused' | 'ok' | 'warn'>(
    item.status === 'error' ? 'err' : item.status === 'processing' ? 'warn' : running ? 'accent' : item.status === 'completed' ? 'ok' : 'paused',
  )
  let kindTag = $derived(item.kind !== 'http' ? ({ hls: 'HLS', dash: 'DASH', ytdlp: 'yt-dlp', torrent: 'Torrent', hoster: t('hoster'), ftp: 'FTP' } as Record<string, string>)[item.kind] ?? '' : '')

  function quality() {
    const h = item.options?.maxHeight
    if (h) return h + 'p'
    if (item.options?.audioOnly) return 'Audio'
    return ''
  }

  let meta = $derived.by(() => {
    const parts: string[] = []
    if (item.status === 'completed') {
      parts.push(fmt.size(item.size))
      if (host) parts.push(host)
      parts.push(fmt.relative(item.completedAt))
    } else {
      parts.push(size > 0 ? `${fmt.size(done)} / ${fmt.size(size)}` : done > 0 ? fmt.size(done) : '')
      if (host) parts.push(host)
      const q = quality()
      if (q) parts.push(q)
    }
    return parts.filter(Boolean).join(' · ')
  })

  let tab = $derived(focused ? 0 : -1)
  const stop = (e: Event) => e.stopPropagation()
</script>

<!-- Keyboard interaction (arrows, Space, Enter, Delete, menu key) is handled by the parent grid. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="row" class:compact class:sel={selected} class:err={item.status === 'error'} class:dragging
  class:drop-before={drop === 'before'} class:drop-after={drop === 'after'}
  role="row" aria-rowindex={index} aria-selected={selected} tabindex={tab} data-id={item.id} draggable={armed}
  onclick={onpointer} ondblclick={onopen}
  oncontextmenu={(e) => { e.preventDefault(); e.stopPropagation(); onmenu(e) }}>
  {#if compact}
    <div role="gridcell" class="c-name">
      {#if canDrag}<span class="grip" role="presentation" title={t('dragHandle')} onpointerdown={onarm}><Icon name="grip" size={14} /></span>{/if}
      <FileBadge {item} size={20} />
      <span class="ellipsis nm" title={item.url}>{name}</span>
      {#if prio !== 0}<span class="prio" class:low={prio < 0} title={priorityLabel(prio)}><Icon name={prio > 0 ? 'arrowUp' : 'arrowDown'} size={12} /><span class="sr-only">{priorityLabel(prio)}</span></span>{/if}
    </div>
    <div role="gridcell" class="c-r num">{size > 0 ? fmt.size(size) : ''}</div>
    <div role="gridcell" class="c-prog">
      <Progress value={pct} tone={tone} size="xs" indeterminate={running && !(size > 0)} label={name} valueText={size > 0 ? fmt.pct(pct) : undefined} />
      <span class="pc num">{size > 0 || item.status === 'completed' ? fmt.pct(pct) : ''}</span>
    </div>
    <div role="gridcell" class="c-r num">{item.status === 'downloading' ? fmt.speed(p?.speed) : ''}</div>
    <div role="gridcell" class="num ellipsis" title={clock}>{etaText}</div>
    <div role="gridcell" class="ellipsis st-{item.status}" title={item.error || status}>{item.status === 'downloading' ? statusLabel(item.status) : status}</div>
    <div role="gridcell" class="muted ellipsis num">{fmt.date(item.completedAt || item.createdAt)}</div>
  {:else}
    <div role="gridcell" class="c-main">
      {#if canDrag}<span class="grip" role="presentation" title={t('dragHandle')} onpointerdown={onarm}><Icon name="grip" size={16} /></span>{/if}
      <FileBadge {item} size={36} />
      <div class="main">
        <div class="line1">
          <span class="name ellipsis" title={item.url}>{name}</span>
          {#if prio !== 0}<span class="tag" class:accent={prio > 0}><Icon name={prio > 0 ? 'arrowUp' : 'arrowDown'} size={11} />{prio > 0 ? t('prioHigh') : t('prioLow')}</span>{/if}
          {#if kindTag}<span class="tag">{kindTag}</span>{/if}
        </div>
        <div class="line2 ellipsis">{meta}</div>
        {#if item.status !== 'completed'}
          <div class="prog">
            <Progress value={pct} {tone} size="sm" indeterminate={running && !(size > 0)} label={name} valueText={size > 0 ? fmt.pct(pct) : undefined} />
            <span class="pc num">{size > 0 ? fmt.pct(pct) : ''}</span>
          </div>
        {/if}
      </div>
    </div>
    <div role="gridcell" class="c-right">
      <div class="info">
        <span class="status st-{item.status} ellipsis num" title={item.error || status}>{status}</span>
        {#if etaText}<span class="eta num ellipsis" title={t('timeLeft')}>{etaText}{#if clock}<span class="clk">&nbsp;· {clock}</span>{/if}</span>{/if}
      </div>
      <div class="acts" role="group" aria-label={name}>
        {#if item.status === 'completed'}
          <IconButton icon="openFile" size="sm" label={t('open')} tabindex={tab} onclick={(e) => { stop(e); onaction('open') }} ondblclick={stop} />
        {:else if running || item.status === 'queued'}
          <IconButton icon="pause" size="sm" label={t('pause')} shortcut={t('space')} tabindex={tab} onclick={(e) => { stop(e); onaction('pause') }} ondblclick={stop} />
        {:else if item.status === 'error'}
          <IconButton icon="refresh" size="sm" label={t('retry')} tabindex={tab} onclick={(e) => { stop(e); onaction('retry') }} ondblclick={stop} />
        {:else}
          <IconButton icon="play" size="sm" label={t('resume')} shortcut={t('space')} tabindex={tab} onclick={(e) => { stop(e); onaction('start') }} ondblclick={stop} />
        {/if}
        <IconButton icon="folderOpen" size="sm" label={t('openFolder')} tabindex={tab} onclick={(e) => { stop(e); onaction('folder') }} ondblclick={stop} />
        <IconButton icon="more" size="sm" label={t('more')} tabindex={tab} aria-haspopup="menu" onclick={(e) => { stop(e); onmenu(e.currentTarget) }} ondblclick={stop} />
      </div>
    </div>
  {/if}
</div>

<style>
  .row {
    display: flex; align-items: center; gap: 12px; height: 68px; padding: 0 8px 0 12px; margin: 0 8px; border-radius: var(--r-md);
    border: 1px solid transparent; position: relative; outline: none;
    transition: background var(--dur-fast), border-color var(--dur-fast), opacity var(--dur-fast);
  }
  .row:hover { background: var(--row-hover); }
  .row.sel { background: var(--sel); border-color: transparent; }
  .row.sel:hover { background: var(--sel-hover); }
  .row.sel::before { content: ''; position: absolute; left: 0; top: 50%; width: 3px; height: 18px; margin-top: -9px; border-radius: 2px; background: var(--accent); }
  .row:focus-visible { box-shadow: inset 0 0 0 2px var(--focus); }
  .row.dragging { opacity: 0.45; }
  .row.drop-before::after, .row.drop-after::after { content: ''; position: absolute; left: 8px; right: 8px; height: 2px; border-radius: 1px; background: var(--accent); }
  .row.drop-before::after { top: -2px; }
  .row.drop-after::after { bottom: -2px; }

  .grip { display: flex; color: var(--muted); cursor: grab; margin-left: -6px; opacity: 0; transition: opacity var(--dur-fast); }
  .row:hover .grip, .row:focus-within .grip, .row.sel .grip { opacity: 1; }

  .c-main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .line1 { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .name { font-weight: 600; font-size: var(--fs-body-lg); }
  .row.err .name { color: var(--err); }
  .line2 { font-size: var(--fs-caption); color: var(--muted); }
  .prog { display: flex; align-items: center; gap: 8px; margin-top: 3px; }
  .pc { font-size: var(--fs-micro); color: var(--muted); width: 38px; text-align: right; flex: none; }

  .c-right { display: flex; align-items: center; gap: 8px; flex: none; width: 236px; justify-content: flex-end; }
  .info { display: flex; flex-direction: column; align-items: flex-end; min-width: 0; flex: 1; gap: 1px; }
  .status { font-size: var(--fs-caption); max-width: 100%; }
  .eta { font-size: var(--fs-caption); color: var(--fg2); max-width: 100%; }
  .clk { color: var(--muted); }
  .st-downloading { color: var(--accent-text); font-weight: 600; }
  .st-completed { color: var(--ok); }
  .st-error { color: var(--err); }
  .st-processing { color: var(--warn); }
  .st-paused, .st-queued { color: var(--muted); }
  /* actions: visible on hover, keyboard focus and selection */
  .acts { display: flex; gap: 2px; opacity: 0; transition: opacity var(--dur-fast); }
  .row:hover .acts, .row:focus-within .acts, .row.sel .acts, .row:focus .acts { opacity: 1; }

  /* compact table row: columns shared with the header in DownloadList */
  .row.compact {
    display: grid; grid-template-columns: var(--cols); height: 36px; gap: 0; padding: 0 4px; margin: 0 8px; border-radius: var(--r-sm);
  }
  .row.compact > div { padding: 0 8px; min-width: 0; font-size: var(--fs-caption); }
  .c-name { display: flex; align-items: center; gap: 8px; }
  .nm { font-size: var(--fs-body); }
  .c-r { text-align: right; }
  .c-prog { display: flex; align-items: center; gap: 6px; }
  .prio { display: inline-flex; color: var(--accent-text); flex: none; }
  .prio.low { color: var(--muted); }
  .compact .grip { margin-left: 0; }
</style>
