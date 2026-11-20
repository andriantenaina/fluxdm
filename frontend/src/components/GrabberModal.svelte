<script lang="ts">
  import { onMount } from 'svelte'
  import Dialog from './ui/Dialog.svelte'
  import Button from './ui/Button.svelte'
  import Switch from './ui/Switch.svelte'
  import Progress from './ui/Progress.svelte'
  import Skeleton from './ui/Skeleton.svelte'
  import EmptyState from './ui/EmptyState.svelte'
  import SideList, { type SideItem } from './settings/SideList.svelte'
  import { api, errMsg, on } from '../lib/api'
  import { t, tf, queueLabel } from '../lib/i18n.svelte'
  import { confirmBox } from '../lib/dialogs.svelte'
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
  let loaded = $state(false)
  let busy = $state(false)

  const NEW = '__new__'

  function blank(): GrabberProject {
    return { id: '', name: '', startUrl: '', depth: 1, sameHost: true, pathPrefix: false, maxPages: 300, include: [], exclude: [],
      minSize: 0, dir: '', savePages: false, autoAdd: false, queue: 'main', cookie: '', periodicHours: 0 }
  }

  onMount(() => {
    load()
    api.queues().then((q) => (queues = q)).catch(() => {})
    return on<GrabberStatus>('fdm:grabber', (s) => {
      status[s.projectId] = s
      if (!s.running) load()
    })
  })

  async function load() {
    try {
      projects = await api.grabberProjects()
      if (!cur && projects.length) pick(projects[0])
      if (!projects.length && !cur) pick(blank())
    } catch (e) { error = errMsg(e) } finally { loaded = true }
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
    if (!cur.name.trim()) {
      try { cur.name = new URL(cur.startUrl).hostname } catch { /* keep empty */ }
    }
    busy = true
    try {
      const p = await api.saveGrabber($state.snapshot(cur) as GrabberProject)
      cur = p
      projects = await api.grabberProjects()
      return p
    } catch (e) {
      error = errMsg(e)
      return null
    } finally { busy = false }
  }

  async function run() {
    const p = await saveCur()
    if (p) {
      try { await api.startGrabber(p.id) } catch (e) { error = errMsg(e) }
    }
  }

  async function del() {
    if (!cur?.id) return
    const r = await confirmBox(tf('grabDeleteConfirm', { name: cur.name || cur.startUrl }), undefined, { title: t('delete'), okLabel: t('delete'), danger: true })
    if (!r.ok) return
    try {
      await api.deleteGrabber(cur.id)
      cur = null
      await load()
    } catch (e) { error = errMsg(e) }
  }

  let st = $derived(cur?.id ? status[cur.id] : undefined)
  let found = $derived(st?.found ?? [])
  let urlOk = $derived(!cur?.startUrl || /^https?:\/\/\S+/i.test(cur.startUrl.trim()))
  let sideItems = $derived<SideItem[]>([
    ...projects.map((p) => ({
      id: p.id, label: p.name || p.startUrl, icon: 'spider',
      mark: status[p.id]?.running ? { icon: 'dot', label: t('grabRunning'), tone: 'ok' as const } : undefined,
    })),
    ...(cur && !cur.id ? [{ id: NEW, label: t('newProject'), icon: 'plus' }] : []),
  ])

  async function addSelected() {
    const list = found.filter((f) => sel.has(f.url)).map((f) => ({
      url: f.url, pageUrl: f.page, dir: cur?.dir || '', queue: cur?.queue || 'main', start: true,
      headers: { Referer: f.page, ...(cur?.cookie ? { Cookie: cur.cookie } : {}) },
    }))
    try {
      if (list.length) await api.addBatch(list)
      sel = new Set()
    } catch (e) { error = errMsg(e) }
  }

  function toggleFound(url: string, on: boolean) {
    const s = new Set(sel)
    on ? s.add(url) : s.delete(url)
    sel = s
  }
</script>

<Dialog title={t('grabTitle')} {onclose} width={920}>
  {#if !loaded}
    <div class="wrap" aria-busy="true">
      <div class="skside"><Skeleton height="28px" /><Skeleton height="28px" /></div>
      <div class="grow col"><Skeleton height="32px" /><Skeleton height="32px" /><Skeleton height="120px" radius="var(--r-md)" /></div>
    </div>
  {:else}
    <div class="wrap">
      <SideList items={sideItems} value={cur ? cur.id || NEW : ''} label={t('projects')} heading={t('projects')}
        onselect={(id) => { const p = projects.find((x) => x.id === id); if (p) pick(p) }}>
        {#snippet footer()}
          <Button size="sm" variant="subtle" icon="plus" onclick={() => pick(blank())}>{t('newProject')}</Button>
        {/snippet}
      </SideList>
      {#if cur}
        <section class="grow">
          <div class="field"><label for="gr-url">{t('startUrl')}</label>
            <input id="gr-url" type="url" bind:value={cur.startUrl} placeholder="https://example.com/gallery/" aria-invalid={!urlOk}
              aria-describedby={urlOk ? undefined : 'gr-url-err'} data-autofocus />
            {#if !urlOk}<span id="gr-url-err" class="error-text"><span class="sr-only">{t('error')} : </span>{t('grabUrlInvalid')}</span>{/if}
          </div>
          <div class="row">
            <div class="field grow"><label for="gr-name">{t('name')}</label><input id="gr-name" bind:value={cur.name} /></div>
            <div class="field w"><label for="gr-depth">{t('depth')}</label><input id="gr-depth" type="number" min="0" max="10" bind:value={cur.depth} /></div>
            <div class="field w"><label for="gr-max">{t('maxPages')}</label><input id="gr-max" type="number" min="1" bind:value={cur.maxPages} /></div>
          </div>
          <div class="field"><label for="gr-inc">{t('include')}</label><input id="gr-inc" bind:value={includeText} placeholder="jpg png pdf zip" /></div>
          <div class="field"><label for="gr-exc">{t('exclude')}</label><input id="gr-exc" bind:value={excludeText} placeholder="*thumb* re:/ads/" /></div>
          <div class="switches">
            <Switch bind:checked={cur.sameHost} label={t('sameHost')} />
            <Switch bind:checked={cur.pathPrefix} label={t('pathPrefix')} />
            <Switch bind:checked={cur.savePages} label={t('savePages')} />
            <Switch bind:checked={cur.autoAdd} label={t('autoAdd')} />
          </div>
          <div class="field"><label for="gr-dir">{t('saveTo')}</label>
            <div class="row"><input id="gr-dir" class="grow" bind:value={cur.dir} placeholder={(store.settings?.downloadDir ?? '') + '/Grabber/…'} />
              <Button icon="folder" onclick={async () => { const d = await api.chooseFolder(cur!.dir); if (d) cur!.dir = d }}>{t('browse')}</Button></div></div>
          <div class="row">
            <div class="field grow"><label for="gr-cookie">{t('cookie')}</label><input id="gr-cookie" bind:value={cur.cookie} autocomplete="off" /></div>
            <div class="field w2"><label for="gr-per">{t('periodic')}</label><input id="gr-per" type="number" min="0" bind:value={cur.periodicHours} /></div>
            <div class="field w2"><label for="gr-q">{t('queue')}</label>
              <select id="gr-q" bind:value={cur.queue}>{#each queues as q}<option value={q.name}>{queueLabel(q.name)}</option>{/each}</select></div>
          </div>

          {#if st}
            <div class="status card" aria-live="polite">
              {#if st.running}<Progress indeterminate label={t('grabRunning')} />{/if}
              <span class="num">{st.pages} {t('pages')} · {st.foundCount} {t('found')} · {st.added} {t('added')}</span>
              {#if st.current}<span class="muted ellipsis small">{st.current}</span>{/if}
              {#if st.error}<span class="err" role="alert">{st.error}</span>{/if}
            </div>
            {#if !cur.autoAdd}
              {#if found.length}
                <div class="row foundbar">
                  <Button size="sm" onclick={() => (sel = new Set(found.map((f) => f.url)))}>{t('selectAll')}</Button>
                  <Button size="sm" onclick={() => (sel = new Set())}>{t('selectNone')}</Button>
                  <span class="grow"></span>
                  <Button size="sm" variant="primary" icon="download" disabled={!sel.size} onclick={addSelected}>{t('addFound')} ({sel.size})</Button>
                </div>
                <div class="found card">
                  <table class="grid">
                    <tbody>
                      {#each found as f (f.url)}
                        <tr>
                          <td class="cb"><input type="checkbox" aria-label={f.url} checked={sel.has(f.url)} onchange={(e) => toggleFound(f.url, e.currentTarget.checked)} /></td>
                          <td class="ellipsis" title={f.url}>{decodeURIComponent(f.url.split('/').pop() || f.url)}</td>
                          <td class="muted small ty">{f.type}</td>
                          <td class="nw">{#if f.new}<span class="tag accent">{t('grabNew')}</span>{/if}</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {:else if !st.running}
                <EmptyState compact art="search" title={t('grabNoFiles')} text={t('grabNoFilesHint')} />
              {/if}
            {/if}
          {/if}
        </section>
      {:else}
        <section class="grow">
          <EmptyState art="folder" title={t('grabNoProject')} text={t('grabNoProjectHint')}>
            <Button variant="primary" icon="plus" onclick={() => pick(blank())}>{t('newProject')}</Button>
          </EmptyState>
        </section>
      {/if}
    </div>
  {/if}
  {#snippet footer()}
    {#if error}<span class="err grow" role="alert">{error}</span>{/if}
    {#if cur?.id}<Button variant="subtle" icon="trash" class="danger" onclick={del}>{t('delete')}</Button>{/if}
    <Button disabled={!cur || busy} onclick={saveCur}>{t('save')}</Button>
    {#if st?.running}
      <Button icon="stop" onclick={() => api.stopGrabber(cur!.id)}>{t('stop')}</Button>
    {:else}
      <Button variant="primary" icon="play" loading={busy} disabled={!cur?.startUrl || !urlOk} onclick={run}>{t('run')}</Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .wrap { display: flex; gap: 20px; min-height: 460px; }
  .skside { width: 190px; display: flex; flex-direction: column; gap: 6px; }
  .col { display: flex; flex-direction: column; gap: 12px; }
  section { min-width: 0; }
  .w { width: 130px; }
  .w2 { width: 160px; }
  .switches { display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; margin-bottom: 8px; }
  .status { display: flex; flex-direction: column; gap: 6px; margin: 8px 0; font-size: var(--fs-body); }
  .foundbar { margin-top: 4px; }
  .found { max-height: 220px; overflow: auto; padding: 0; margin-top: 6px; }
  .cb { width: 30px; }
  .ty { width: 80px; }
  .nw { width: 56px; }
</style>
