<script lang="ts">
  import { onMount } from 'svelte'
  import { slide } from 'svelte/transition'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t, queueLabel } from '../lib/i18n.svelte'
  import { store, handoff } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { extOfName, KIND_COLOR, fileTypeOf } from '../lib/filetype'
  import { layers } from '../lib/layers.svelte'
  import { dur } from '../lib/motion'
  import type { AddRequest, ProbeResult, QueueState, TorrentInfo } from '../lib/types'
  import WindowShell from '../components/windows/WindowShell.svelte'
  import Button from '../components/ui/Button.svelte'
  import IconButton from '../components/ui/IconButton.svelte'
  import Skeleton from '../components/ui/Skeleton.svelte'
  import Menu, { type MenuEntry } from '../components/ui/Menu.svelte'
  import Icon from '../components/Icon.svelte'

  let { token }: { token: string } = $props()

  let req = $state<AddRequest>({ url: '', start: true })
  let url = $state('')
  let urlTouched = $state(false)
  let base = $state('') // file name without extension
  let ext = $state('')
  let dir = $state('')
  let category = $state('')
  let queue = $state('main')
  let priority = $state(0)
  let conns = $state(0)
  let description = $state('')
  let mirrors = $state('')
  let user = $state('')
  let password = $state('')
  let advanced = $state(false)
  let probe = $state<ProbeResult | null>(null)
  let probing = $state(false)
  let error = $state('')
  let free = $state(-1)
  let queues = $state<QueueState[]>([])
  let torrent = $state<TorrentInfo | null>(null)
  let torrentSel = $state<Set<number>>(new Set())
  let busy = $state(false)
  let splitOpen = $state(false)
  let caretEl: HTMLButtonElement | undefined = $state()
  let nameTouched = false
  let dirTouched = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const videoKinds = ['ytdlp', 'hoster', 'hls', 'dash']

  function setName(n: string) {
    const e = extOfName(n)
    ext = e
    base = e ? n.slice(0, -(e.length + 1)) : n
  }
  let fileName = $derived(base ? (ext ? `${base}.${ext}` : base) : '')
  let problem = $derived(fmt.urlProblem(url))
  let showProblem = $derived(urlTouched && !!url.trim() && problem !== '')
  let problemText = $derived(problem === 'scheme' ? t('addErrScheme') : problem === 'invalid' ? t('addErrInvalid') : '')
  let canSubmit = $derived(!busy && !probing && problem === '')

  onMount(async () => {
    try {
      const p = await api.pending(token)
      req = p.request
      url = req.url
      if (req.fileName) { setName(req.fileName); nameTouched = true }
      dir = req.dir ?? ''
      if (dir) dirTouched = true
      category = req.category ?? ''
      queue = req.queue || 'main'
      description = req.description ?? ''
      priority = req.priority ?? 0
    } catch (e) {
      error = errMsg(e)
    }
    conns = store.settings?.maxConns ?? 8
    queues = await api.queues().catch(() => [])
    if (!url) {
      const clip = (await api.clipboardText().catch(() => '')).trim()
      if (clip && fmt.urlProblem(clip) === '') url = clip
    }
    if (url) { urlTouched = true; runProbe() }
    else dir = store.settings?.downloadDir ?? ''
  })

  function onUrlInput() {
    urlTouched = true
    clearTimeout(timer)
    timer = setTimeout(runProbe, 500)
  }

  async function runProbe() {
    const u = url.trim()
    if (!u || fmt.urlProblem(u) !== '') { probe = null; return }
    error = ''
    probing = true
    torrent = null
    try {
      const r = await api.probe(u, req.headers ?? {})
      if (u !== url.trim()) return // the address changed meanwhile
      probe = r
      if (videoKinds.includes(r.kind)) { goVideo(); return }
      if (!nameTouched && r.fileName) setName(r.fileName)
      if (!category || !dirTouched) category = r.category
      if (!dirTouched) dir = r.dir
      if (r.kind === 'torrent') {
        try {
          torrent = await api.inspectTorrent(u)
          torrentSel = new Set((torrent.files ?? []).map((f) => f.index))
        } catch (e) { error = errMsg(e) }
      }
      free = await api.diskFree(dir).catch(() => -1)
    } catch (e) {
      probe = null
      error = errMsg(e)
      if (!base) { try { const seg = decodeURIComponent(new URL(url.trim()).pathname.split("/").pop() || ""); if (seg) setName(seg) } catch {} }
      if (!dirTouched) dir = store.settings?.downloadDir ?? ''
    } finally {
      probing = false
    }
  }

  function goVideo() {
    handoff.url = url.trim()
    handoff.headers = req.headers ?? {}
    location.hash = '#/video/' + token
  }

  function onCategory() {
    if (dirTouched || !store.settings) return
    const c = store.settings.categories.find((x) => x.name === category)
    const sep = store.platform === 'windows' ? '\\' : '/'
    dir = c ? (c.dir || store.settings.downloadDir + sep + c.name) : store.settings.downloadDir
  }

  async function browse() {
    const d = await api.chooseFolder(dir)
    if (d) { dir = d; dirTouched = true; free = await api.diskFree(dir).catch(() => -1) }
  }

  async function submit(start: boolean, toQueue = '') {
    urlTouched = true
    if (!canSubmit) return
    error = ''
    busy = true
    splitOpen = false
    const headers = { ...(req.headers ?? {}) }
    if (user) headers['Authorization'] = 'Basic ' + btoa(unescape(encodeURIComponent(user + ':' + password)))
    const options: Record<string, any> = { ...(req.options ?? {}) }
    if (conns && conns !== store.settings?.maxConns) options.connections = conns
    const ms = mirrors.split(/\s+/).map((x) => x.trim()).filter((x) => /^https?:\/\//i.test(x))
    if (ms.length) options.mirrors = ms
    if (torrent?.files && torrentSel.size !== torrent.files.length) options.files = [...torrentSel]
    try {
      await api.add({
        ...req,
        url: url.trim(),
        fileName: probe?.kind === 'torrent' ? '' : fileName,
        dir, category, queue: toQueue || queue, headers, options, start, description, priority,
        size: probe?.size ?? req.size,
        kind: probe?.kind === 'page' ? 'http' : probe?.kind === 'torrent' ? 'torrent' : req.kind,
      })
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
    if (e.key === 'Escape') { cancel(); return }
    const tg = e.target as HTMLElement
    if (e.key === 'Enter' && !tg.closest('textarea, button, select, [role="menu"]')) {
      e.preventDefault()
      submit(true)
    }
  }

  let typeChip = $derived.by(() => {
    if (!probe) return null
    const k = probe.kind
    if (k === 'torrent') return { label: 'Torrent', color: KIND_COLOR.torrent }
    if (k === 'page') return { label: t('addWebPage'), color: KIND_COLOR.other }
    const ft = fileTypeOf({ fileName, kind: 'http', category } as any, store.settings)
    return { label: ft.ext.toUpperCase(), color: ft.color }
  })

  let splitItems = $derived<MenuEntry[]>([
    { label: t('later'), icon: 'clock', onselect: () => submit(false) },
    ...queues.filter((q) => q.name !== 'main').map((q) => ({ label: `${t('addToQueue')} « ${queueLabel(q.name)} »`, icon: 'queue', onselect: () => submit(false, q.name) })),
  ])
</script>

<svelte:window onkeydown={onKey} />

<WindowShell title={t('addTitle')} onclose={cancel}>
  <div class="dialog">
    {#if !store.frameless}<h1>{t('addTitle')}</h1>{/if}
    <div class="body">
      <div class="field">
        <label for="add-url">{t('url')}</label>
        <div class="urlbox" class:bad={showProblem}>
          <Icon name="link" size={16} />
          <!-- svelte-ignore a11y_autofocus -->
          <input id="add-url" type="url" bind:value={url} oninput={onUrlInput} onblur={() => (urlTouched = true)}
            placeholder="https://…  ·  magnet:?…  ·  ftp://…" autofocus spellcheck="false"
            aria-invalid={showProblem} aria-describedby={showProblem ? 'add-url-err' : undefined} />
          {#if typeChip}<span class="chip" style:background={typeChip.color}>{typeChip.label}</span>{/if}
        </div>
        {#if showProblem}<span id="add-url-err" class="error-text"><Icon name="alert" size={14} />{problemText}</span>{/if}
      </div>

      <div class="info" aria-live="polite">
        {#if probing}
          <span class="sr-only">{t('probing')}</span>
          <Skeleton width="84px" height="22px" radius="11px" /><Skeleton width="130px" height="22px" radius="11px" /><Skeleton width="110px" height="22px" radius="11px" />
        {:else if probe}
          <span class="pill"><b>{probe.size >= 0 ? fmt.size(probe.size) : t('unknownSize')}</b></span>
          {#if probe.kind === 'http'}
            <span class="pill" class:ok={probe.resumable}><Icon name={probe.resumable ? 'check' : 'info'} size={12} />{probe.resumable ? t('resumable') : t('notResumable')}</span>
          {/if}
          {#if free >= 0}<span class="pill">{t('freeSpace')} : {fmt.size(free)}</span>{/if}
        {/if}
        {#if error}<span class="err selectable" role="alert">{error}</span>{/if}
      </div>

      {#if probe?.kind === 'page'}
        <div class="card note">
          <Icon name="video" size={20} />
          <span class="grow">{t('pageDetected')}</span>
          <Button variant="primary" icon="search" onclick={goVideo}>{t('analyzeVideo')}</Button>
        </div>
      {/if}

      {#if probe?.kind !== 'torrent'}
        <div class="field">
          <label for="add-name">{t('fileName')}</label>
          <div class="namebox">
            <input id="add-name" class="grow" bind:value={base} oninput={() => (nameTouched = true)} />
            <span class="dot" aria-hidden="true">.</span>
            <input class="ext" bind:value={ext} oninput={() => (nameTouched = true)} aria-label={t('addExt')} />
          </div>
        </div>
      {/if}

      <div class="row top">
        <div class="field grow">
          <label for="add-cat">{t('category')}</label>
          <select id="add-cat" bind:value={category} onchange={onCategory}>
            <option value="">{t('general')}</option>
            {#each store.settings?.categories ?? [] as c}<option value={c.name}>{c.name}</option>{/each}
          </select>
        </div>
        <div class="field" style="flex:2">
          <label for="add-dir">{t('saveTo')}</label>
          <div class="row"><input id="add-dir" class="grow" bind:value={dir} oninput={() => (dirTouched = true)} />
            <IconButton icon="folderOpen" variant="default" label={t('browse')} onclick={browse} /></div>
        </div>
        <div class="field prio">
          <label for="add-prio">{t('priority')}</label>
          <select id="add-prio" bind:value={priority}>
            <option value={1}>{t('prioHigh')}</option><option value={0}>{t('prioNormal')}</option><option value={-1}>{t('prioLow')}</option>
          </select>
        </div>
      </div>

      {#if torrent?.files?.length}
        <fieldset class="field">
          <legend>{t('files')} ({torrentSel.size}/{torrent.files.length}) — {fmt.size(torrent.size)}</legend>
          <div class="files card">
            {#each torrent.files as f}
              <label class="check">
                <input type="checkbox" checked={torrentSel.has(f.index)}
                  onchange={(e) => { const s = new Set(torrentSel); (e.currentTarget as HTMLInputElement).checked ? s.add(f.index) : s.delete(f.index); torrentSel = s }} />
                <span class="grow ellipsis">{f.path}</span><span class="muted num">{fmt.size(f.size)}</span>
              </label>
            {/each}
          </div>
        </fieldset>
      {/if}

      <button class="link adv" aria-expanded={advanced} aria-controls="add-adv" onclick={() => (advanced = !advanced)}>
        <Icon name={advanced ? 'chevronDown' : 'chevronRight'} size={14} />{t('addAdvanced')}
      </button>
      {#if advanced}
        <div id="add-adv" class="card advbox" transition:slide={{ duration: dur(250) }}>
          <div class="row">
            <div class="field grow">
              <label for="add-queue">{t('queue')}</label>
              <select id="add-queue" bind:value={queue}>{#each queues as q}<option value={q.name}>{queueLabel(q.name)}</option>{/each}</select>
            </div>
            {#if !probe || probe.kind === 'http'}
              <div class="field grow">
                <label for="add-conns">{t('connections')}</label>
                <select id="add-conns" bind:value={conns}>{#each [1, 2, 4, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}</select>
              </div>
            {/if}
          </div>
          {#if !probe || probe.kind === 'http'}
            <div class="field"><label for="add-mirrors">{t('addMirrors')}</label><textarea id="add-mirrors" rows="2" bind:value={mirrors} placeholder="https://mirror1/file.zip"></textarea></div>
          {/if}
          <div class="field"><label for="add-desc">{t('description')}</label><input id="add-desc" bind:value={description} /></div>
          <div class="row">
            <div class="field grow"><label for="add-user">{t('user')}</label><input id="add-user" bind:value={user} autocomplete="off" /></div>
            <div class="field grow"><label for="add-pass">{t('password')}</label><input id="add-pass" type="password" bind:value={password} autocomplete="off" /></div>
          </div>
        </div>
      {/if}
    </div>

    <div class="actions">
      <Button onclick={cancel}>{t('cancel')}</Button>
      <div class="split">
        <Button class="main" variant="primary" icon="download" loading={busy} disabled={!canSubmit} onclick={() => submit(true)}>{t('startNow')}</Button>
        <IconButton bind:el={caretEl} class="caret" variant="primary" icon="chevronDown" label={t('addMoreOptions')} tip={false}
          aria-haspopup="menu" aria-expanded={splitOpen} disabled={!canSubmit} onclick={() => (splitOpen = !splitOpen)} />
      </div>
    </div>
  </div>
</WindowShell>

{#if splitOpen && caretEl}
  <Menu items={splitItems} anchor={caretEl} placement="top-end" label={t('addMoreOptions')} onclose={() => (splitOpen = false)} />
{/if}

<style>
  .urlbox { display: flex; align-items: center; gap: 8px; background: var(--surface-1); border: 1px solid var(--border); border-bottom-color: var(--border-strong);
    border-radius: var(--r-sm); padding: 2px 6px 2px 10px; color: var(--muted); }
  .urlbox:focus-within { border-bottom-color: var(--accent); box-shadow: inset 0 -1px 0 var(--accent); }
  .urlbox.bad { border-bottom-color: var(--err); box-shadow: inset 0 -1px 0 var(--err); }
  .urlbox input { border: none !important; background: none !important; box-shadow: none !important; padding-left: 0; font-size: var(--fs-body-lg); color: var(--fg); }
  .chip { color: #fff; font-size: var(--fs-micro); font-weight: 700; padding: 2px 8px; border-radius: var(--r-sm); flex: none; }
  .info { display: flex; gap: 6px; align-items: center; min-height: 24px; margin: -4px 0 12px; font-size: var(--fs-caption); flex-wrap: wrap; }
  .pill { display: inline-flex; align-items: center; gap: 4px; background: var(--surface-3); border-radius: 11px; padding: 2px 10px; line-height: 18px; }
  .pill.ok { color: var(--ok); background: var(--ok-soft); }
  .note { display: flex; gap: 12px; align-items: center; margin-bottom: 12px; color: var(--accent-text); }
  .note span { color: var(--fg); }
  .namebox { display: flex; align-items: center; gap: 4px; }
  .namebox .dot { color: var(--muted); font-weight: 700; }
  .namebox .ext { width: 80px; flex: none; text-align: center; font-weight: 600; }
  .top { align-items: flex-start; }
  .prio { width: 120px; flex: none; }
  fieldset { border: none; padding: 0; margin: 0 0 12px; }
  legend { color: var(--fg2); font-size: var(--fs-caption); font-weight: 600; padding: 0; margin-bottom: 6px; }
  .files { max-height: 150px; overflow: auto; padding: 4px 10px; }
  .files label { font-size: var(--fs-caption); }
  .adv { margin: 2px 0 8px; display: inline-flex; align-items: center; gap: 4px; }
  .advbox { padding: 12px 12px 0; }
  .split { display: flex; }
  .split :global(.main) { border-top-right-radius: 0; border-bottom-right-radius: 0; }
  .split :global(.caret) { border-top-left-radius: 0; border-bottom-left-radius: 0; border-left: 1px solid rgba(255, 255, 255, 0.3); height: 32px; width: 32px; }
</style>
