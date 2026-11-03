<script lang="ts">
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import SegmentMap from '../components/SegmentMap.svelte'

  let { id }: { id: string } = $props()
  let details = $state(true)
  let limitOn = $state(false)
  let limitKB = $state(500)
  let error = $state('')

  let it = $derived(store.items.find((x) => x.id === id))
  let p = $derived(store.progress[id])
  let size = $derived(p?.size && p.size > 0 ? p.size : (it?.size ?? 0))
  let done = $derived(p?.downloaded ?? it?.downloaded ?? 0)
  let pct = $derived(it?.status === 'completed' ? 100 : fmt.percent(done, size))
  let running = $derived(it?.status === 'downloading' || it?.status === 'processing')

  $effect(() => {
    if (it) document.title = `${pct.toFixed(0)}% ${it.fileName || it.title || ''}`
  })

  async function toggle() {
    error = ''
    try {
      if (running) await api.pause([id])
      else await api.start([id])
    } catch (e) { error = errMsg(e) }
  }

  async function applyLimit() {
    await api.setItemOptions(id, { speedLimit: limitOn ? limitKB * 1024 : 0 })
  }

  async function cancel() {
    if (running) await api.pause([id])
    Window.Close()
  }
</script>

<div class="dialog">
  {#if it}
    <div class="title ellipsis" title={it.url}><b>{it.fileName || it.title || it.url}</b></div>
    <table class="kv">
      <tbody>
        <tr><td>{t('address')}</td><td class="ellipsis selectable muted">{it.url}</td></tr>
        <tr><td>{t('status')}</td><td><b class="st-{it.status}">{t(('s_' + it.status) as 's_queued')}</b>{#if it.error}<span class="err"> — {it.error}</span>{/if}</td></tr>
        <tr><td>{t('size')}</td><td>{size > 0 ? fmt.size(size) : t('unknownSize')}</td></tr>
        <tr><td>{t('downloaded')}</td><td>{fmt.size(done)} {size > 0 ? `(${pct.toFixed(2)} %)` : ''}</td></tr>
        <tr><td>{t('transferRate')}</td><td>{running ? fmt.speed(p?.speed) : '—'}</td></tr>
        <tr><td>{t('timeLeft')}</td><td>{running ? fmt.duration(p?.eta) : '—'}</td></tr>
        {#if it.kind === 'http'}
          <tr><td>{t('connections')}</td><td>{p?.conns ?? 0}</td></tr>
        {/if}
        {#if p?.extra?.segmentsTotal}
          <tr><td>{t('segments')}</td><td>{p.extra.segments} / {p.extra.segmentsTotal}{p.extra.live ? ' — ' + t('live') : ''}</td></tr>
        {/if}
        {#if it.kind === 'torrent' && p?.extra}
          <tr><td>{t('peers')}</td><td>{p.extra.peers ?? 0} ({t('seeds')} : {p.extra.seeds ?? 0})</td></tr>
        {/if}
      </tbody>
    </table>

    <div class="bar big" class:done={it.status === 'completed'} class:error={it.status === 'error'} class:indeterminate={running && !(size > 0)}>
      <div style:width="{pct}%"></div>
    </div>

    <button class="link" onclick={() => (details = !details)}>{details ? '▾ ' + t('hideDetails') : '▸ ' + t('showDetails')}</button>
    {#if details}
      {#if it.kind === 'http' && p?.segments?.length}
        <SegmentMap segments={p.segments} {size} done={it.status === 'completed'} />
        <div class="segs">
          <table class="grid">
            <thead><tr><th>#</th><th>{t('downloaded')}</th><th>{t('progress')}</th></tr></thead>
            <tbody>
              {#each p.segments as s, i}
                {@const len = s.e > 0 ? s.e - s.s : 0}
                <tr><td>{i + 1}</td><td>{fmt.size(s.p - s.s)}{len ? ' / ' + fmt.size(len) : ''}</td>
                  <td>{len ? (len <= s.p - s.s ? '✓' : ((100 * (s.p - s.s)) / len).toFixed(0) + ' %') : ''}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      <div class="row limit">
        <label class="check"><input type="checkbox" bind:checked={limitOn} onchange={applyLimit} /> {t('limitThis')}</label>
        <input type="number" min="8" bind:value={limitKB} onchange={applyLimit} style="width:90px" disabled={!limitOn} /> <span class="muted">Ko/s</span>
      </div>
    {/if}
    {#if error}<div class="err">{error}</div>{/if}
    <div class="actions">
      {#if it.status === 'completed'}
        <button onclick={() => api.openFolder(id)}>{t('openFolder')}</button>
        <button class="primary" onclick={() => api.openFile(id)}>{t('open')}</button>
      {:else}
        <button onclick={toggle}>{running ? t('pause') : t('resume')}</button>
        <button onclick={cancel}>{t('cancel')}</button>
      {/if}
      <button onclick={() => Window.Close()}>{t('close')}</button>
    </div>
  {:else}
    <p class="muted">…</p>
  {/if}
</div>

<style>
  .title { font-size: 14px; }
  .kv { width: 100%; border-collapse: collapse; font-size: 12.5px; table-layout: fixed; }
  .kv td { padding: 3px 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .kv td:first-child { width: 120px; color: var(--muted); }
  .big { height: 14px; border-radius: 7px; }
  .segs { max-height: 120px; overflow: auto; font-size: 12px; border: 1px solid var(--border); border-radius: 6px; }
  .limit { font-size: 12.5px; }
  .st-downloading { color: var(--accent); }
  .st-completed { color: var(--ok); }
  .st-error { color: var(--err); }
</style>
