<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store, handoff } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { AddRequest, ProbeResult, QueueState, TorrentInfo } from '../lib/types'

  let { token }: { token: string } = $props()

  let req = $state<AddRequest>({ url: '', start: true })
  let url = $state('')
  let fileName = $state('')
  let dir = $state('')
  let category = $state('')
  let queue = $state('main')
  let conns = $state(0)
  let description = $state('')
  let user = $state('')
  let password = $state('')
  let showAuth = $state(false)
  let probe = $state<ProbeResult | null>(null)
  let probing = $state(false)
  let error = $state('')
  let free = $state(-1)
  let queues = $state<QueueState[]>([])
  let torrent = $state<TorrentInfo | null>(null)
  let torrentSel = $state<Set<number>>(new Set())
  let busy = $state(false)
  let nameTouched = false
  let dirTouched = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const videoKinds = ['ytdlp', 'hoster', 'hls', 'dash']

  onMount(async () => {
    try {
      const p = await api.pending(token)
      req = p.request
      url = req.url
      fileName = req.fileName ?? ''
      if (fileName) nameTouched = true
      dir = req.dir ?? ''
      if (dir) dirTouched = true
      category = req.category ?? ''
      queue = req.queue || 'main'
      conns = store.settings?.maxConns ?? 8
    } catch (e) {
      error = errMsg(e)
    }
    queues = await api.queues()
    if (!url) url = await api.clipboardText()
    if (url) runProbe()
    else dir = store.settings?.downloadDir ?? ''
  })

  function onUrlInput() {
    clearTimeout(timer)
    timer = setTimeout(runProbe, 600)
  }

  async function runProbe() {
    const u = url.trim()
    if (!u) return
    error = ''
    probing = true
    torrent = null
    try {
      const r = await api.probe(u, req.headers ?? {})
      probe = r
      if (videoKinds.includes(r.kind)) {
        goVideo()
        return
      }
      if (!nameTouched && r.fileName) fileName = r.fileName
      if (!category || !dirTouched) category = r.category
      if (!dirTouched) dir = r.dir
      if (r.kind === 'torrent') {
        try {
          torrent = await api.inspectTorrent(u)
          torrentSel = new Set((torrent.files ?? []).map((f) => f.index))
          if (!nameTouched) fileName = torrent.name
        } catch (e) { error = errMsg(e) }
      }
      free = await api.diskFree(dir)
    } catch (e) {
      probe = null
      error = errMsg(e)
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
    dir = c ? (c.dir || store.settings.downloadDir + (store.platform === 'windows' ? '\\' : '/') + c.name) : store.settings.downloadDir
  }

  async function browse() {
    const d = await api.chooseFolder(dir)
    if (d) { dir = d; dirTouched = true; free = await api.diskFree(dir) }
  }

  async function submit(start: boolean) {
    error = ''
    busy = true
    const headers = { ...(req.headers ?? {}) }
    if (user) headers['Authorization'] = 'Basic ' + btoa(unescape(encodeURIComponent(user + ':' + password)))
    const options: Record<string, any> = { ...(req.options ?? {}) }
    if (conns && conns !== store.settings?.maxConns) options.connections = conns
    if (torrent?.files && torrentSel.size !== torrent.files.length) options.files = [...torrentSel]
    try {
      await api.add({
        ...req,
        url: url.trim(),
        fileName: probe?.kind === 'torrent' ? '' : fileName,
        dir,
        category,
        queue,
        headers,
        options,
        start,
        size: probe?.size ?? req.size,
        description,
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
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') cancel(); if (e.key === 'Enter' && !busy && url) submit(true) }} />

<div class="dialog">
  <h1>{t('addTitle')}</h1>
  <div class="body">
    <div class="field">
      <span>{t('url')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input bind:value={url} oninput={onUrlInput} placeholder="https://… / magnet:?…" autofocus />
    </div>

    <div class="info">
      {#if probing}
        <span class="muted">{t('probing')}</span>
      {:else if probe}
        <span><b>{probe.size >= 0 ? fmt.size(probe.size) : t('unknownSize')}</b></span>
        {#if probe.kind === 'http'}
          <span class={probe.resumable ? 'ok' : 'muted'}>{probe.resumable ? '✓ ' + t('resumable') : t('notResumable')}</span>
        {/if}
        {#if probe.contentType}<span class="muted ellipsis">{probe.contentType}</span>{/if}
      {/if}
      {#if error}<span class="err selectable">{error}</span>{/if}
    </div>

    {#if probe?.kind === 'page'}
      <div class="card note">{t('pageDetected')} <button class="link" onclick={goVideo}>{t('analyzeVideo')}</button></div>
    {/if}

    {#if probe?.kind !== 'torrent'}
      <div class="field">
        <span>{t('fileName')}</span>
        <input bind:value={fileName} oninput={() => (nameTouched = true)} />
      </div>
    {/if}

    <div class="row">
      <div class="field grow">
        <span>{t('category')}</span>
        <select bind:value={category} onchange={onCategory}>
          <option value="">{t('general')}</option>
          {#each store.settings?.categories ?? [] as c}<option value={c.name}>{c.name}</option>{/each}
        </select>
      </div>
      <div class="field" style="width:150px">
        <span>{t('queue')}</span>
        <select bind:value={queue}>
          {#each queues as q}<option value={q.name}>{q.name === 'main' ? 'Principale' : q.name}</option>{/each}
        </select>
      </div>
      {#if !probe || probe.kind === 'http'}
        <div class="field" style="width:110px">
          <span>{t('connections')}</span>
          <select bind:value={conns}>
            {#each [1, 2, 4, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}
          </select>
        </div>
      {/if}
    </div>

    <div class="field">
      <span>{t('saveTo')} {#if free >= 0}<em class="muted">— {t('freeSpace')} : {fmt.size(free)}</em>{/if}</span>
      <div class="row">
        <input class="grow" bind:value={dir} oninput={() => (dirTouched = true)} />
        <button onclick={browse}>{t('browse')}</button>
      </div>
    </div>

    {#if torrent?.files?.length}
      <div class="field">
        <span>{t('files')} ({torrentSel.size}/{torrent.files.length}) — {fmt.size(torrent.size)}</span>
        <div class="files card">
          {#each torrent.files as f}
            <label class="check">
              <input type="checkbox" checked={torrentSel.has(f.index)}
                onchange={(e) => { const s = new Set(torrentSel); (e.currentTarget as HTMLInputElement).checked ? s.add(f.index) : s.delete(f.index); torrentSel = s }} />
              <span class="grow ellipsis">{f.path}</span><span class="muted">{fmt.size(f.size)}</span>
            </label>
          {/each}
        </div>
      </div>
    {/if}

    <div class="field">
      <span>{t('description')}</span>
      <input bind:value={description} />
    </div>

    <button class="link" onclick={() => (showAuth = !showAuth)}>{showAuth ? '▾' : '▸'} {t('login')}</button>
    {#if showAuth}
      <div class="row">
        <div class="field grow"><span>{t('user')}</span><input bind:value={user} autocomplete="off" /></div>
        <div class="field grow"><span>{t('password')}</span><input type="password" bind:value={password} autocomplete="off" /></div>
      </div>
    {/if}
  </div>
  <div class="actions">
    <button onclick={cancel}>{t('cancel')}</button>
    <button disabled={busy || !url} onclick={() => submit(false)}>{t('later')}</button>
    <button class="primary" disabled={busy || !url} onclick={() => submit(true)}>{t('startNow')}</button>
  </div>
</div>

<style>
  .info { display: flex; gap: 12px; align-items: center; min-height: 22px; margin: -4px 0 8px; font-size: 12px; flex-wrap: wrap; }
  .note { margin-bottom: 10px; font-size: 12px; }
  .files { max-height: 150px; overflow: auto; padding: 6px 10px; }
  .files label { font-size: 12px; }
  em { font-style: normal; }
</style>
