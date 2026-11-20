<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t, tn, queueLabel } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { layers } from '../lib/layers.svelte'
  import type { AddRequest, QueueState } from '../lib/types'
  import WindowShell from '../components/windows/WindowShell.svelte'
  import Button from '../components/ui/Button.svelte'
  import IconButton from '../components/ui/IconButton.svelte'
  import Skeleton from '../components/ui/Skeleton.svelte'
  import EmptyState from '../components/ui/EmptyState.svelte'
  import Switch from '../components/ui/Switch.svelte'
  import Icon from '../components/Icon.svelte'

  let { token }: { token: string } = $props()
  let reqs = $state<AddRequest[]>([])
  let loading = $state(true)
  let sel = $state<Set<number>>(new Set())
  let filter = $state('')
  let dir = $state('')
  let useCategories = $state(true)
  let queue = $state('main')
  let queues = $state<QueueState[]>([])
  let error = $state('')
  let busy = $state(false)

  onMount(async () => {
    try {
      const p = await api.pending(token)
      reqs = p.batch ?? []
      sel = new Set(reqs.map((_, i) => i))
    } catch (e) { error = errMsg(e) }
    loading = false
    queues = await api.queues().catch(() => [])
    dir = store.settings?.downloadDir ?? ''
  })

  function nameOf(r: AddRequest) {
    try {
      const u = new URL(r.url)
      return decodeURIComponent(u.pathname.split('/').pop() || u.hostname)
    } catch { return r.url }
  }

  let visible = $derived.by(() => {
    const f = filter.trim().toLowerCase().split(/[\s,;]+/).filter(Boolean)
    return reqs.map((r, i) => ({ r, i })).filter(({ r }) => {
      if (!f.length) return true
      const n = (nameOf(r) + ' ' + r.url + ' ' + (r.title ?? '')).toLowerCase()
      return f.some((x) => n.includes(x.replace(/^\*?\.?/, '')))
    })
  })
  let allVisibleOn = $derived(visible.length > 0 && visible.every((v) => sel.has(v.i)))

  function selectVisible(on: boolean) {
    const s = new Set(sel)
    for (const v of visible) on ? s.add(v.i) : s.delete(v.i)
    sel = s
  }

  function toggle(i: number, on: boolean) {
    const s = new Set(sel)
    on ? s.add(i) : s.delete(i)
    sel = s
  }

  async function submit(start: boolean) {
    if (busy || !sel.size) return
    busy = true
    try {
      const list = reqs.filter((_, i) => sel.has(i)).map((r) => ({ ...r, dir: useCategories ? '' : dir, queue, start }))
      await api.addBatch(list)
      await api.donePending(token)
      Window.Close()
    } catch (e) {
      error = errMsg(e)
      busy = false
    }
  }

  function cancel() {
    api.donePending(token)
    Window.Close()
  }

  function onKey(e: KeyboardEvent) {
    if (layers.depth > 0 || e.defaultPrevented) return
    if (e.key === 'Escape') cancel()
  }
</script>

<svelte:window onkeydown={onKey} />

<WindowShell title={t('batchTitle')} onclose={cancel}>
  <div class="dialog">
    {#if !store.frameless}<h1>{t('batchTitle')} <span class="muted">({reqs.length})</span></h1>{/if}
    <div class="row">
      <div class="search grow">
        <Icon name="filter" size={14} />
        <input type="search" placeholder={t('filter')} aria-label={t('filter')} bind:value={filter} disabled={loading || !reqs.length} />
      </div>
      <Button size="sm" disabled={!visible.length} onclick={() => selectVisible(true)}>{t('selectAll')}</Button>
      <Button size="sm" disabled={!visible.length} onclick={() => selectVisible(false)}>{t('selectNone')}</Button>
    </div>

    <div class="body card list">
      {#if loading}
        <div class="sk" aria-busy="true" aria-label={t('loading')}>
          {#each Array(6) as _}
            <div class="skrow"><Skeleton width="16px" height="16px" /><Skeleton height="12px" /><Skeleton width="48px" height="12px" /></div>
          {/each}
        </div>
      {:else if !reqs.length}
        <EmptyState compact art="search" title={t('batchEmpty')} text={t('batchEmptyHint')} />
      {:else if !visible.length}
        <EmptyState compact art="search" title={t('noResults')} />
      {:else}
        <table class="grid">
          <caption class="sr-only">{t('batchTitle')}</caption>
          <thead>
            <tr>
              <th class="cb"><input type="checkbox" checked={allVisibleOn} aria-label={t('selectAll')} onchange={(e) => selectVisible((e.currentTarget as HTMLInputElement).checked)} /></th>
              <th>{t('name')}</th><th>{t('title')}</th><th class="ext">{t('type')}</th>
            </tr>
          </thead>
          <tbody>
            {#each visible as { r, i } (i)}
              {@const n = nameOf(r)}
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions (the row checkbox is the keyboard path) -->
              <tr class:on={sel.has(i)} onclick={(e) => { if (!(e.target as HTMLElement).closest('input')) toggle(i, !sel.has(i)) }}>
                <td class="cb"><input type="checkbox" checked={sel.has(i)} aria-label={n} onchange={(e) => toggle(i, (e.currentTarget as HTMLInputElement).checked)} /></td>
                <td class="ellipsis" title={r.url}>{n}</td>
                <td class="muted ellipsis small">{r.title ?? ''}</td>
                <td class="muted small ext">{fmt.extOf(n)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
    {#if error}<div class="err small" role="alert">{error}</div>{/if}
    <div class="row opts">
      <Switch bind:checked={useCategories} label={t('batchUseCats')} />
      {#if !useCategories}
        <input class="grow" bind:value={dir} aria-label={t('saveTo')} />
        <IconButton icon="folderOpen" variant="default" label={t('browse')} onclick={async () => { const d = await api.chooseFolder(dir); if (d) dir = d }} />
      {:else}<span class="grow"></span>{/if}
      <label class="small" for="batch-queue">{t('queue')}</label>
      <select id="batch-queue" bind:value={queue} class="q">{#each queues as q}<option value={q.name}>{queueLabel(q.name)}</option>{/each}</select>
    </div>
    <div class="actions">
      <span class="muted grow" aria-live="polite">{tn('selectedCount', sel.size)}</span>
      <Button onclick={cancel}>{t('cancel')}</Button>
      <Button disabled={busy || !sel.size} onclick={() => submit(false)}>{t('later')}</Button>
      <Button variant="primary" icon="download" loading={busy} disabled={!sel.size} onclick={() => submit(true)}>{t('startNow')}</Button>
    </div>
  </div>
</WindowShell>

<style>
  .search { display: flex; align-items: center; gap: 6px; background: var(--surface-1); border: 1px solid var(--border); border-bottom-color: var(--border-strong);
    border-radius: var(--r-sm); padding: 0 8px; color: var(--muted); }
  .search:focus-within { border-bottom-color: var(--accent); box-shadow: inset 0 -1px 0 var(--accent); }
  .search input { border: none !important; background: none !important; box-shadow: none !important; padding-left: 2px; color: var(--fg); }
  .list { padding: 0; }
  .grid { table-layout: fixed; }
  .grid td, .grid th { padding: 6px 10px; }
  .grid tbody tr:hover td { background: var(--surface-2); }
  .grid tbody tr.on td { background: var(--sel); }
  .cb { width: 36px; }
  .ext { width: 70px; }
  .sk { padding: 8px 10px; display: flex; flex-direction: column; gap: 12px; }
  .skrow { display: flex; gap: 12px; align-items: center; }
  .opts { flex-wrap: wrap; }
  .q { width: 160px; }
</style>
