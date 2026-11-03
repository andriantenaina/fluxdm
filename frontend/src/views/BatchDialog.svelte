<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { AddRequest, QueueState } from '../lib/types'

  let { token }: { token: string } = $props()
  let reqs = $state<AddRequest[]>([])
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
    } catch (e) { error = errMsg(e) }
    queues = await api.queues()
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

  function selectVisible(on: boolean) {
    const s = new Set(sel)
    for (const v of visible) on ? s.add(v.i) : s.delete(v.i)
    sel = s
  }

  async function submit(start: boolean) {
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
</script>

<div class="dialog">
  <h1>{t('batchTitle')} <span class="muted">({reqs.length})</span></h1>
  <div class="row">
    <input class="grow" placeholder={t('filter')} bind:value={filter} />
    <button onclick={() => selectVisible(true)}>{t('selectAll')}</button>
    <button onclick={() => selectVisible(false)}>{t('selectNone')}</button>
  </div>
  <div class="body card list">
    <table class="grid">
      <tbody>
        {#each visible as { r, i } (i)}
          <tr>
            <td style="width:26px"><input type="checkbox" checked={sel.has(i)} onchange={(e) => { const s = new Set(sel); (e.currentTarget as HTMLInputElement).checked ? s.add(i) : s.delete(i); sel = s }} /></td>
            <td class="ellipsis" title={r.url}>{nameOf(r)}</td>
            <td class="muted ellipsis small">{r.title ?? ''}</td>
            <td class="muted small" style="width:70px">{fmt.extOf(nameOf(r))}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if error}<div class="err">{error}</div>{/if}
  <div class="row">
    <label class="check"><input type="checkbox" bind:checked={useCategories} /> {t('categories')}</label>
    {#if !useCategories}<input class="grow" bind:value={dir} /><button onclick={async () => { const d = await api.chooseFolder(dir); if (d) dir = d }}>{t('browse')}</button>{/if}
    <span class="grow"></span>
    <span>{t('queue')}</span>
    <select bind:value={queue} style="width:140px">{#each queues as q}<option value={q.name}>{q.name === 'main' ? 'Principale' : q.name}</option>{/each}</select>
  </div>
  <div class="actions">
    <span class="muted grow">{sel.size} {t('selected')}</span>
    <button onclick={cancel}>{t('cancel')}</button>
    <button disabled={busy || !sel.size} onclick={() => submit(false)}>{t('later')}</button>
    <button class="primary" disabled={busy || !sel.size} onclick={() => submit(true)}>{t('startNow')}</button>
  </div>
</div>

<style>
  .list { padding: 0; }
  .small { font-size: 12px; }
  .grow { flex: 1; }
</style>
