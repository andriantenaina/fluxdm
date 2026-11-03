<script lang="ts">
  import { onMount } from 'svelte'
  import Modal from './Modal.svelte'
  import { api, errMsg, on } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import type { GrabberProject, GrabberStatus, QueueState } from '../lib/types'

  let { onclose }: { onclose: () => void } = $props()
  let projects = $state<GrabberProject[]>([])
  let cur = $state<GrabberProject | null>(null)
  let includeText = $state('')
  let excludeText = $state('')
  let status = $state<Record<string, GrabberStatus>>({})
  let sel = $state<Set<string>>(new Set())
  let queues = $state<QueueState[]>([])
  let error = $state('')

  function blank(): GrabberProject {
    return { id: '', name: '', startUrl: '', depth: 1, sameHost: true, pathPrefix: false, maxPages: 300, include: [], exclude: [],
      minSize: 0, dir: '', savePages: false, autoAdd: false, queue: 'main', cookie: '', periodicHours: 0 }
  }

  onMount(() => {
    load()
    api.queues().then((q) => (queues = q))
    return on<GrabberStatus>('fdm:grabber', (s) => {
      status[s.projectId] = s
      if (!s.running) load()
    })
  })

  async function load() {
    projects = await api.grabberProjects()
    if (!cur && projects.length) pick(projects[0])
    if (!projects.length && !cur) pick(blank())
  }

  function pick(p: GrabberProject) {
    cur = { ...p }
    includeText = (p.include ?? []).join(' ')
    excludeText = (p.exclude ?? []).join(' ')
    sel = new Set()
  }

  async function saveCur(): Promise<GrabberProject | null> {
    if (!cur) return null
    error = ''
    cur.include = includeText.split(/[\s,;]+/).filter(Boolean)
    cur.exclude = excludeText.split(/[\s,;]+/).filter(Boolean)
    try {
      const p = await api.saveGrabber($state.snapshot(cur) as GrabberProject)
      cur = p
      projects = await api.grabberProjects()
      return p
    } catch (e) {
      error = errMsg(e)
      return null
    }
  }

  async function run() {
    const p = await saveCur()
    if (p) await api.startGrabber(p.id)
  }

  async function del() {
    if (!cur?.id) return
    await api.deleteGrabber(cur.id)
    cur = null
    await load()
  }

  let st = $derived(cur?.id ? status[cur.id] : undefined)
  let found = $derived(st?.found ?? [])

  async function addSelected() {
    const list = found.filter((f) => sel.has(f.url)).map((f) => ({
      url: f.url, pageUrl: f.page, dir: cur?.dir || '', queue: cur?.queue || 'main', start: true,
      headers: { Referer: f.page, ...(cur?.cookie ? { Cookie: cur.cookie } : {}) },
    }))
    if (list.length) await api.addBatch(list)
    sel = new Set()
  }
</script>

<Modal title={t('grabTitle')} {onclose} width={900}>
  <div class="wrap">
    <aside>
      <div class="h">{t('projects')}</div>
      {#each projects as p}
        <button class:active={cur?.id === p.id} onclick={() => pick(p)}>{p.name}{#if status[p.id]?.running} ●{/if}</button>
      {/each}
      <button class="new" onclick={() => pick(blank())}>+ {t('newProject')}</button>
    </aside>
    {#if cur}
      <section>
        <div class="field"><span>{t('startUrl')}</span><input bind:value={cur.startUrl} placeholder="https://exemple.com/galerie/" /></div>
        <div class="row">
          <div class="field grow"><span>{t('name')}</span><input bind:value={cur.name} /></div>
          <div class="field" style="width:120px"><span>{t('depth')}</span><input type="number" min="0" max="10" bind:value={cur.depth} /></div>
          <div class="field" style="width:120px"><span>{t('maxPages')}</span><input type="number" min="1" bind:value={cur.maxPages} /></div>
        </div>
        <div class="field"><span>{t('include')}</span><input bind:value={includeText} placeholder="jpg png pdf zip" /></div>
        <div class="field"><span>{t('exclude')}</span><input bind:value={excludeText} placeholder="*thumb* re:/ads/" /></div>
        <div class="row">
          <label class="check"><input type="checkbox" bind:checked={cur.sameHost} /> {t('sameHost')}</label>
          <label class="check"><input type="checkbox" bind:checked={cur.pathPrefix} /> {t('pathPrefix')}</label>
        </div>
        <div class="row">
          <label class="check"><input type="checkbox" bind:checked={cur.savePages} /> {t('savePages')}</label>
          <label class="check"><input type="checkbox" bind:checked={cur.autoAdd} /> {t('autoAdd')}</label>
        </div>
        <div class="field"><span>{t('saveTo')}</span>
          <div class="row"><input class="grow" bind:value={cur.dir} placeholder={store.settings?.downloadDir + '/Grabber/…'} />
            <button onclick={async () => { const d = await api.chooseFolder(cur!.dir); if (d) cur!.dir = d }}>{t('browse')}</button></div></div>
        <div class="row">
          <div class="field grow"><span>{t('cookie')}</span><input bind:value={cur.cookie} /></div>
          <div class="field" style="width:150px"><span>{t('periodic')}</span><input type="number" min="0" bind:value={cur.periodicHours} /></div>
          <div class="field" style="width:140px"><span>{t('queue')}</span>
            <select bind:value={cur.queue}>{#each queues as q}<option value={q.name}>{q.name === 'main' ? 'Principale' : q.name}</option>{/each}</select></div>
        </div>

        {#if st}
          <div class="status">
            {#if st.running}<div class="bar indeterminate"><div></div></div>{/if}
            <span>{st.pages} {t('pages')} · {st.foundCount} {t('found')} · {st.added} {t('added')}</span>
            {#if st.current}<span class="muted ellipsis small">{st.current}</span>{/if}
            {#if st.error}<span class="err">{st.error}</span>{/if}
          </div>
          {#if found.length && !cur.autoAdd}
            <div class="row"><button onclick={() => (sel = new Set(found.map((f) => f.url)))}>{t('selectAll')}</button>
              <button onclick={() => (sel = new Set())}>{t('selectNone')}</button>
              <span class="grow"></span><button class="primary" disabled={!sel.size} onclick={addSelected}>{t('addFound')} ({sel.size})</button></div>
            <div class="found card">
              <table class="grid">
                <tbody>
                  {#each found as f (f.url)}
                    <tr>
                      <td style="width:24px"><input type="checkbox" checked={sel.has(f.url)} onchange={(e) => { const s = new Set(sel); (e.currentTarget as HTMLInputElement).checked ? s.add(f.url) : s.delete(f.url); sel = s }} /></td>
                      <td class="ellipsis" title={f.url}>{decodeURIComponent(f.url.split('/').pop() || f.url)}</td>
                      <td class="muted small" style="width:80px">{f.type}</td>
                      <td style="width:40px">{#if f.new}<span class="tag accent">new</span>{/if}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        {/if}
      </section>
    {/if}
  </div>
  {#snippet footer()}
    {#if error}<span class="err grow">{error}</span>{/if}
    {#if cur?.id}<button class="danger" onclick={del}>{t('delete')}</button>{/if}
    <button onclick={saveCur}>{t('save')}</button>
    {#if st?.running}
      <button onclick={() => api.stopGrabber(cur!.id)}>{t('stop')}</button>
    {:else}
      <button class="primary" disabled={!cur?.startUrl} onclick={run}>{t('run')}</button>
    {/if}
  {/snippet}
</Modal>

<style>
  .wrap { display: flex; gap: 14px; min-height: 440px; }
  aside { width: 180px; flex-shrink: 0; display: flex; flex-direction: column; gap: 2px; }
  aside .h { font-size: 11px; color: var(--muted); text-transform: uppercase; margin-bottom: 4px; }
  aside button { text-align: left; background: none; border: none; padding: 6px 10px; border-radius: 6px; overflow: hidden; text-overflow: ellipsis; }
  aside button.active { background: var(--sel); color: var(--accent); }
  aside .new { color: var(--accent); }
  section { flex: 1; min-width: 0; }
  .status { display: flex; flex-direction: column; gap: 4px; margin: 8px 0; font-size: 12.5px; }
  .found { max-height: 200px; overflow: auto; padding: 0; margin-top: 6px; }
  .small { font-size: 12px; }
  .danger { color: var(--err); }
  .grow { flex: 1; }
</style>
