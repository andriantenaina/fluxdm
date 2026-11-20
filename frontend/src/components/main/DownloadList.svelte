<script lang="ts">
  import { tick } from 'svelte'
  import { flip } from 'svelte/animate'
  import { fade } from 'svelte/transition'
  import DownloadRow, { type RowAction } from './DownloadRow.svelte'
  import Icon from '../Icon.svelte'
  import Button from '../ui/Button.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import { api } from '../../lib/api'
  import { t, tf } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import { toast } from '../../lib/toast.svelte'
  import { dur } from '../../lib/motion'
  import { ui, act, openItem, toggleRun, removeItems, setSort, type SortCol } from '../../lib/mainui.svelte'
  import type { Item } from '../../lib/types'

  let { rows, onmenu }: {
    rows: Item[]
    /** open the context menu for the current selection */
    onmenu: (anchor: HTMLElement | { x: number; y: number }) => void
  } = $props()

  const VIRTUAL_AT = 200
  let compact = $derived(ui.view === 'compact')
  let rowH = $derived(compact ? 36 : 68)
  let headH = $derived(compact ? 32 : 0)
  let canReorder = $derived(ui.sortCol === 'queue' && ui.sortAsc)

  // ---------- virtualization ----------
  let scroller: HTMLDivElement | undefined = $state()
  let scrollTop = $state(0)
  let viewH = $state(600)
  let virtual = $derived(rows.length > VIRTUAL_AT)
  let start = $derived(virtual ? Math.max(0, Math.floor((scrollTop - headH) / rowH) - 8) : 0)
  let end = $derived(virtual ? Math.min(rows.length, Math.ceil((scrollTop + viewH) / rowH) + 8) : rows.length)
  let slice = $derived(virtual ? rows.slice(start, end) : rows)

  $effect(() => {
    const el = scroller
    if (!el) return
    const ro = new ResizeObserver(() => (viewH = el.clientHeight))
    ro.observe(el)
    return () => ro.disconnect()
  })

  // ---------- roving focus ----------
  let focusIdx = $derived.by(() => {
    const i = ui.focusId ? rows.findIndex((r) => r.id === ui.focusId) : -1
    return i >= 0 ? i : 0
  })

  async function focusRow(i: number, opts: { select?: 'single' | 'range' | 'none' } = {}) {
    if (!rows.length) return
    i = Math.max(0, Math.min(rows.length - 1, i))
    const it = rows[i]
    ui.focusId = it.id
    if (opts.select === 'single') { ui.selected = new Set([it.id]); ui.anchor = it.id }
    else if (opts.select === 'range') selectRange(it.id)
    // bring into view (virtual rows may not be rendered yet)
    const el = scroller
    if (el) {
      const top = headH + i * rowH
      if (top < el.scrollTop + headH) el.scrollTop = top - headH
      else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight
      scrollTop = el.scrollTop
    }
    await tick()
    el?.querySelector<HTMLElement>(`[role="row"][data-id="${CSS.escape(it.id)}"]`)?.focus({ preventScroll: true })
  }

  function selectRange(toId: string) {
    const ids = rows.map((r) => r.id)
    const a = ui.anchor ? ids.indexOf(ui.anchor) : -1
    const b = ids.indexOf(toId)
    if (a < 0 || b < 0) { ui.selected = new Set([toId]); ui.anchor = toId; return }
    const [lo, hi] = a < b ? [a, b] : [b, a]
    ui.selected = new Set(ids.slice(lo, hi + 1))
  }

  function pointer(e: MouseEvent, it: Item) {
    ui.focusId = it.id
    if (e.shiftKey && ui.anchor) selectRange(it.id)
    else if (e.ctrlKey || e.metaKey) {
      const s = new Set(ui.selected)
      s.has(it.id) ? s.delete(it.id) : s.add(it.id)
      ui.selected = s
      ui.anchor = it.id
    } else {
      ui.selected = new Set([it.id])
      ui.anchor = it.id
    }
  }

  function rowMenu(e: MouseEvent | HTMLElement, it: Item) {
    if (!ui.selected.has(it.id)) { ui.selected = new Set([it.id]); ui.anchor = it.id }
    ui.focusId = it.id
    onmenu(e instanceof HTMLElement ? e : { x: e.clientX, y: e.clientY })
  }

  function rowAction(it: Item, a: RowAction) {
    switch (a) {
      case 'start': case 'retry': return act(() => api.start([it.id]))
      case 'pause': return act(() => api.pause([it.id]))
      case 'open': return act(() => api.openFile(it.id))
      case 'folder': return act(() => api.openFolder(it.id))
      case 'progress': return act(() => api.openProgress(it.id))
    }
  }

  const selectedItems = () => {
    const sel = rows.filter((r) => ui.selected.has(r.id))
    return sel.length ? sel : rows[focusIdx] ? [rows[focusIdx]] : []
  }

  // ---------- reorder ----------
  export function move(id: string, dir: -1 | 1) {
    if (!canReorder) { toast(t('reorderHint') as string, 'info'); return }
    const i = rows.findIndex((r) => r.id === id)
    if (i < 0) return
    const j = i + dir
    if (j < 0 || j >= rows.length) return
    const before = dir < 0 ? rows[j].id : (rows[i + 2]?.id ?? '')
    act(() => api.reorder(id, before)).then(() => focusRow(j))
  }

  let armed = $state<string | null>(null)
  let dragId = $state<string | null>(null)
  let dropAt = $state<{ id: string; pos: 'before' | 'after' } | null>(null)
  const ROW_MIME = 'application/x-fdm-row'

  function onDragStart(e: DragEvent) {
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-id]')
    if (!armed || !el || el.dataset.id !== armed) { e.preventDefault(); return }
    dragId = armed
    e.dataTransfer?.setData(ROW_MIME, armed)
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
  }
  function onDragOver(e: DragEvent) {
    if (!dragId || !e.dataTransfer?.types.includes(ROW_MIME)) return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-id]')
    if (!el || el.dataset.id === dragId) { dropAt = null; return }
    const r = el.getBoundingClientRect()
    dropAt = { id: el.dataset.id!, pos: e.clientY < r.top + r.height / 2 ? 'before' : 'after' }
  }
  function onDrop(e: DragEvent) {
    if (!dragId) return
    e.preventDefault()
    e.stopPropagation()
    const id = dragId, at = dropAt
    endDrag()
    if (!at) return
    const ids = rows.map((r) => r.id)
    const ti = ids.indexOf(at.id)
    let before = at.pos === 'before' ? at.id : (ids[ti + 1] ?? '')
    if (before === id) return
    const cur = ids.indexOf(id)
    if (ids[cur + 1] === before || (before === '' && cur === ids.length - 1)) return // no-op
    act(() => api.reorder(id, before))
  }
  function endDrag() { dragId = null; dropAt = null; armed = null }

  // ---------- keyboard ----------
  function key(e: KeyboardEvent) {
    const target = e.target as HTMLElement
    if (target.closest('button') && (e.key === 'Enter' || e.key === ' ')) return // let row buttons work
    const it = rows[focusIdx]
    const page = Math.max(1, Math.floor(viewH / rowH) - 1)
    const sel: 'single' | 'range' | 'none' = e.shiftKey ? 'range' : e.ctrlKey || e.metaKey ? 'none' : 'single'
    let handled = true
    switch (e.key) {
      case 'ArrowDown':
        if (e.altKey && it) move(it.id, 1)
        else focusRow(focusIdx + 1, { select: sel })
        break
      case 'ArrowUp':
        if (e.altKey && it) move(it.id, -1)
        else focusRow(focusIdx - 1, { select: sel })
        break
      case 'Home': focusRow(0, { select: sel }); break
      case 'End': focusRow(rows.length - 1, { select: sel }); break
      case 'PageDown': focusRow(focusIdx + page, { select: sel }); break
      case 'PageUp': focusRow(focusIdx - page, { select: sel }); break
      case ' ':
        if (!it) break
        if (e.ctrlKey || e.metaKey) {
          const s = new Set(ui.selected)
          s.has(it.id) ? s.delete(it.id) : s.add(it.id)
          ui.selected = s
          ui.anchor = it.id
        } else toggleRun(ui.selected.has(it.id) ? selectedItems() : [it])
        break
      case 'Enter': if (it) openItem(it); break
      case 'Delete': removeItems(selectedItems().map((x) => x.id), e.shiftKey); break
      case 'ContextMenu': {
        const el = target.closest<HTMLElement>('[role="row"]')
        if (it && el) rowMenu(el, it)
        break
      }
      case 'F10':
        if (e.shiftKey && it) { const el = target.closest<HTMLElement>('[role="row"]'); if (el) rowMenu(el, it) } else handled = false
        break
      case 'a': case 'A':
        if (e.ctrlKey || e.metaKey) { ui.selected = new Set(rows.map((r) => r.id)) } else handled = false
        break
      default: handled = false
    }
    if (handled) { e.preventDefault(); e.stopPropagation() }
  }

  // ---------- compact header ----------
  const cols: { key: SortCol | ''; label: string; right?: boolean }[] = [
    { key: 'name', label: 'name' }, { key: 'size', label: 'size', right: true }, { key: 'progress', label: 'progress' },
    { key: 'speed', label: 'speed', right: true }, { key: '', label: 'timeLeft' }, { key: 'status', label: 'status' }, { key: 'date', label: 'lastTry' },
  ]
  const ariaSort = (k: SortCol | '') => (k && ui.sortCol === k ? (ui.sortAsc ? 'ascending' : 'descending') : k ? 'none' : undefined)

  // ---------- empty states ----------
  let empty = $derived.by(() => {
    if (ui.search.trim()) return { art: 'search' as const, title: t('noResults'), text: tf('noResultsHint', { q: ui.search.trim() }), add: false }
    const f = ui.filter
    if (f.type === 'status') {
      if (f.value === 'downloading') return { art: 'active' as const, title: t('emptyActive'), text: t('emptyActiveHint'), add: true }
      if (f.value === 'unfinished') return { art: 'done' as const, title: t('emptyUnfinished'), text: t('emptyUnfinishedHint'), add: true }
      if (f.value === 'completed') return { art: 'done' as const, title: t('emptyDone'), text: t('emptyDoneHint'), add: false }
      return { art: 'error' as const, title: t('emptyErrors'), text: t('emptyErrorsHint'), add: false }
    }
    if (f.type === 'cat') return { art: 'folder' as const, title: t('emptyCat'), text: t('emptyCatHint'), add: true }
    if (f.type === 'queue') return { art: 'queue' as const, title: t('emptyQueue'), text: t('emptyQueueHint'), add: true }
    return { art: 'downloads' as const, title: t('noDownloads'), text: `${t('dropHere')}. ${t('emptyHint')}.`, add: true }
  })
</script>

<svelte:window onpointerup={() => { if (!dragId) armed = null }} />

<div class="listwrap" bind:this={scroller} onscroll={() => (scrollTop = scroller?.scrollTop ?? 0)} style:--cols="minmax(220px, 1fr) 88px 150px 92px 104px 110px 132px">
  {#if !store.loaded}
    <div class="skel" aria-busy="true" aria-label={t('loading')}>
      {#each Array(7) as _, i (i)}
        <div class="skrow" class:compact><Skeleton width={compact ? '20px' : '36px'} height={compact ? '20px' : '36px'} radius="var(--r-md)" />
          <div class="skmain"><Skeleton width="{50 + ((i * 17) % 35)}%" height="12px" />{#if !compact}<Skeleton width="30%" height="10px" /><Skeleton width="100%" height="4px" />{/if}</div>
        </div>
      {/each}
    </div>
  {:else if rows.length === 0}
    <EmptyState art={empty.art} title={empty.title} text={empty.text}>
      {#if empty.add}<Button variant="primary" icon="plus" onclick={() => api.openAddDialog('')}>{t('add')}</Button>{/if}
      {#if ui.search.trim()}<Button onclick={() => (ui.search = '')}>{t('clearSearch')}</Button>{/if}
    </EmptyState>
  {:else}
    <div class="grid" class:compact role="grid" aria-label={t('downloadsList')} aria-multiselectable="true" aria-rowcount={rows.length + (compact ? 1 : 0)}
      tabindex="-1" onkeydown={key} ondragstart={onDragStart} ondragover={onDragOver} ondrop={onDrop} ondragend={endDrag}
      ondragleave={(e) => { if (e.target === e.currentTarget) dropAt = null }}>
      {#if compact}
        <div class="head" role="row" aria-rowindex={1}>
          {#each cols as c}
            <div role="columnheader" aria-sort={ariaSort(c.key)} class:r={c.right}>
              {#if c.key}
                {@const k = c.key}
                <button class="sortbtn" onclick={() => setSort(k)}>
                  <span class="ellipsis">{t(c.label as 'name')}</span>
                  {#if ui.sortCol === k}<Icon name={ui.sortAsc ? 'chevronUp' : 'chevronDown'} size={12} />{/if}
                </button>
              {:else}<span class="nosort ellipsis">{t(c.label as 'name')}</span>{/if}
            </div>
          {/each}
        </div>
      {/if}
      <div class="rows" style:padding-top="{start * rowH}px" style:padding-bottom="{(rows.length - end) * rowH}px">
        {#each slice as it, k (it.id)}
          <div class="slot" animate:flip={{ duration: virtual ? 0 : dur(250) }} in:fade={{ duration: virtual ? 0 : dur(167) }}>
            <DownloadRow item={it} index={start + k + 1 + (compact ? 1 : 0)} {compact} selected={ui.selected.has(it.id)}
              focused={start + k === focusIdx} canDrag={canReorder} armed={armed === it.id} dragging={dragId === it.id}
              drop={dropAt?.id === it.id ? dropAt.pos : null}
              onpointer={(e) => pointer(e, it)} onopen={() => openItem(it)} onmenu={(e) => rowMenu(e, it)}
              onaction={(a) => rowAction(it, a)} onarm={() => (armed = it.id)} />
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .listwrap { flex: 1; overflow: auto; position: relative; padding-bottom: 8px; min-height: 0; }
  .grid { outline: none; min-width: 0; }
  .grid.compact { min-width: 860px; }
  .head {
    display: grid; grid-template-columns: var(--cols); position: sticky; top: 0; z-index: 2; height: 32px; align-items: stretch;
    margin: 0 8px; padding: 0 4px; background: var(--surface-1); border-bottom: 1px solid var(--border);
  }
  :global(:root[data-glass]) .head { background: transparent; -webkit-backdrop-filter: blur(20px); backdrop-filter: blur(20px); }
  .head > div { min-width: 0; display: flex; align-items: center; }
  .head .r { justify-content: flex-end; }
  .sortbtn {
    display: flex; align-items: center; gap: 4px; width: 100%; height: 100%; min-height: 0; padding: 0 8px; border: none; border-radius: var(--r-sm);
    background: transparent !important; font-size: var(--fs-caption); font-weight: 600; color: var(--fg2); text-align: left;
  }
  .head .r .sortbtn { justify-content: flex-end; }
  .sortbtn:hover { background: var(--surface-3) !important; }
  .nosort { padding: 0 8px; font-size: var(--fs-caption); font-weight: 600; color: var(--fg2); }
  .rows { display: flex; flex-direction: column; }
  .slot { display: block; }
  .skel { display: flex; flex-direction: column; gap: 4px; padding: 4px 16px; }
  .skrow { display: flex; align-items: center; gap: 12px; height: 64px; }
  .skrow.compact { height: 32px; }
  .skmain { flex: 1; display: flex; flex-direction: column; gap: 8px; }
</style>
