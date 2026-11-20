<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t, tf } from '../lib/i18n.svelte'
  import { store, handoff } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { categoryOfKind } from '../lib/filetype'
  import { layers } from '../lib/layers.svelte'
  import type { AddRequest, Inspection, Quality, VideoInfo } from '../lib/types'
  import WindowShell from '../components/windows/WindowShell.svelte'
  import Button from '../components/ui/Button.svelte'
  import IconButton from '../components/ui/IconButton.svelte'
  import ProgressRing from '../components/ui/ProgressRing.svelte'
  import Skeleton from '../components/ui/Skeleton.svelte'
  import EmptyState from '../components/ui/EmptyState.svelte'
  import Tabs, { tabId, panelId, type TabDef } from '../components/ui/Tabs.svelte'
  import Icon from '../components/Icon.svelte'

  let { token }: { token: string } = $props()

  let req = $state<AddRequest>({ url: '', start: true })
  let url = $state('')
  let kind = $state('')
  let info = $state<VideoInfo | null>(null)
  let insp = $state<Inspection | null>(null)
  let loading = $state(true)
  let error = $state('')
  let submitErr = $state('')
  let busy = $state(false)
  let tab = $state('video')
  let lastPlaylist = false

  let choice = $state('') // Quality.id or "f:<ytdlp format id>" / variant url
  let output = $state('mp4')
  let audioFmt = $state('mp3')
  let subs = $state<Set<string>>(new Set())
  let title = $state('')
  let nameTouched = false
  let dir = $state('')
  let dirTouched = false
  let entrySel = $state<Set<number>>(new Set())

  const sep = () => (store.platform === 'windows' ? '\\' : '/')
  function catFor(audio: boolean) {
    const c = categoryOfKind(store.settings, audio ? 'audio' : 'video')
    const name = c?.name ?? (audio ? t('vidCatAudio') : t('vidCatVideo'))
    const d = c?.dir || (store.settings ? store.settings.downloadDir + sep() + name : '')
    return { name, dir: d }
  }

  onMount(async () => {
    try {
      const p = await api.pending(token)
      req = p.request
    } catch (e) { error = errMsg(e) }
    url = handoff.url || req.url
    req.headers = Object.keys(handoff.headers).length ? handoff.headers : (req.headers ?? {})
    kind = req.kind || (await api.detectKind(url).catch(() => ''))
    if (kind === 'http' && !/\.(mp4|webm|mkv|mov|m4v|flv|ts|mp3|m4a)(\?|$)/i.test(url)) kind = 'ytdlp'
    output = store.settings?.hlsOutput === 'mkv' ? 'mkv' : store.settings?.hlsOutput === 'ts' ? 'ts' : 'mp4'
    if (req.dir) { dir = req.dir; dirTouched = true } else dir = catFor(false).dir
    if (req.fileName) { title = req.fileName.replace(/\.[a-z0-9]{2,4}$/i, ''); nameTouched = true }
    await analyze(false)
  })

  async function analyze(playlist: boolean) {
    lastPlaylist = playlist
    loading = true
    error = ''
    try {
      const d = await api.analyzeDetails(kind, url, req.headers ?? {}, req.title ?? '', playlist)
      info = d.info
      insp = d.inspection
      if (!nameTouched) title = (insp.fileName || '').replace(/\.[a-z0-9]{2,4}$/i, '') || insp.title
      choice = insp.defaultQuality
      if (info.playlist) entrySel = new Set((info.entries ?? []).map((_, i) => i))
    } catch (e) {
      info = null
      error = errMsg(e)
    } finally {
      loading = false
    }
  }

  let videoQs = $derived((insp?.qualities ?? []).filter((q) => q.id !== 'audio'))
  let hasAudio = $derived((insp?.qualities ?? []).some((q) => q.id === 'audio'))
  let isHoster = $derived(info?.source?.startsWith('hoster') ?? false)
  let isAudio = $derived(tab === 'audio')
  let chosen = $derived(videoQs.find((q) => q.id === choice))
  let ytFormats = $derived((info?.formats ?? []).filter((f) => f.id !== 'direct').sort((a, b) => (b.height || 0) - (a.height || 0) || (b.tbr || 0) - (a.tbr || 0)))
  let hasPlaylistParam = $derived(/[?&]list=/.test(url) && !info?.playlist)
  /** best recent total speed, for an indicative download time */
  let refSpeed = $derived(store.totalHistory.length ? Math.max(...store.totalHistory) : 0)

  let tabs = $derived<TabDef[]>([
    { id: 'video', label: t('vidTabVideo'), icon: 'video' },
    { id: 'audio', label: t('audioOnly'), icon: 'music', hidden: !hasAudio },
    { id: 'subs', label: t('subtitles'), badge: subs.size || '', hidden: !(info?.subtitles?.length) },
    { id: 'adv', label: t('vidTabAdvanced'), icon: 'tune', hidden: !(ytFormats.length || info?.variants?.length) },
  ])

  // the default directory follows the chosen tab (video / audio category)
  $effect(() => {
    const a = isAudio
    if (!dirTouched) dir = catFor(a).dir
  })

  function pick(q: Quality) {
    choice = q.id
    if (!nameTouched && insp) title = `${insp.title || 'video'}${q.label && q.label !== 'Auto' && q.label !== 'Original' ? ` (${q.label})` : ''}`
  }

  function radioKeys(e: KeyboardEvent, list: readonly { id: string }[], current: string, set: (id: string) => void) {
    const i = list.findIndex((x) => x.id === current)
    let j = -1
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') j = (i + 1) % list.length
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') j = (i - 1 + list.length) % list.length
    if (j < 0) return
    e.preventDefault()
    set(list[j].id)
    const group = e.currentTarget as HTMLElement
    queueMicrotask(() => group.querySelector<HTMLElement>('[aria-checked="true"]')?.focus())
  }

  function finalExt() {
    if (isAudio) return audioFmt
    if (info?.source === 'direct') return insp?.outputExt || 'mp4'
    return output
  }

  function buildOptions(): Record<string, any> {
    const o: Record<string, any> = { output: finalExt() }
    if (isAudio) { o.audioOnly = true; o.quality = 'audio'; return o }
    const c = choice
    if (c.startsWith('f:')) {
      const id = c.slice(2)
      const f = info?.formats?.find((x) => x.id === id)
      o.format = f && f.acodec === 'none' ? `${id}+ba/${id}` : id
    } else if (c.startsWith('rep:')) o.videoRep = c.slice(4)
    else if (c.startsWith('h:')) o.maxHeight = Number(c.slice(2))
    else if (/^https?:/.test(c)) {
      if (isHoster) o.maxHeight = chosen?.height || 0 // hoster links expire: re-resolve with a height
      else o.variantUrl = c
    } else o.quality = 'best'
    if (subs.size) o.subtitles = [...subs].join(',')
    return o
  }

  function itemKind(): string {
    if (isHoster) return 'hoster'
    switch (info?.source) {
      case 'hls': return 'hls'
      case 'dash': return 'dash'
      case 'direct': return 'http'
    }
    return 'ytdlp'
  }

  async function finish() {
    await api.donePending(token)
    Window.Close()
  }

  async function submit(start: boolean) {
    if (!info || busy) return
    busy = true
    submitErr = ''
    try {
      const name = `${(title || insp?.title || 'video').replace(/[<>:"/\\|?*]/g, '_')}.${finalExt()}`
      const base: AddRequest = {
        url, pageUrl: req.pageUrl, title: insp?.title || title, headers: req.headers, kind: itemKind(), dir,
        category: catFor(isAudio).name, options: buildOptions(), start, fileName: name,
      }
      if (info.source === 'direct') base.options = {}
      if (info.playlist && info.entries?.length) {
        const reqs = info.entries.filter((_, i) => entrySel.has(i)).map((e) => ({ ...base, url: e.url, title: e.title, fileName: '' }))
        await api.addBatch(reqs)
      } else {
        await api.add(base)
      }
      await finish()
    } catch (e) {
      submitErr = errMsg(e)
      busy = false
    }
  }

  /** Analysis failed: download the address as a plain file. */
  async function addPlain() {
    busy = true
    try {
      await api.add({ url, pageUrl: req.pageUrl, title: req.title, headers: req.headers, kind: 'http', start: true, dir: dirTouched ? dir : undefined })
      await finish()
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

  const estTime = (q: Quality) => (q.sizeEstimate && refSpeed > 0 ? fmt.duration(q.sizeEstimate / refSpeed) : '')
  const audioChoices = [{ id: 'mp3', l: 'MP3', d: 'vidMp3Desc' }, { id: 'm4a', l: 'M4A', d: 'vidM4aDesc' }] as const
</script>

<svelte:window onkeydown={onKey} />

<WindowShell title={t('videoTitle')} onclose={cancel}>
  <div class="dialog">
    {#if loading}
      <div class="center" aria-busy="true">
        <ProgressRing size={40} indeterminate label={t('analyzing')} />
        <p>{t('analyzing')}</p>
        <p class="muted small ellipsis u">{url}</p>
        <div class="skel" aria-hidden="true">
          <Skeleton width="168px" height="95px" radius="var(--r-md)" />
          <div class="sk-lines"><Skeleton height="14px" /><Skeleton width="60%" height="14px" /><Skeleton width="40%" height="14px" /></div>
        </div>
      </div>
    {:else if !info}
      <div class="center">
        <EmptyState art="search" title={t('vidErrTitle')} text={t('vidErrHint')}>
          <Button variant="primary" icon="refresh" onclick={() => analyze(lastPlaylist)}>{t('retry')}</Button>
          <Button icon="download" loading={busy} onclick={addPlain}>{t('vidAddPlain')}</Button>
        </EmptyState>
        {#if error}<p class="card err small selectable errbox" role="alert">{error}</p>{/if}
        <p class="muted small ellipsis u">{url}</p>
      </div>
    {:else}
      <div class="hero">
        <div class="thumb" style:background-image={info.thumbnail ? `url("${info.thumbnail}")` : ''}>
          {#if !info.thumbnail}<Icon name="video" size={28} />{/if}
          {#if insp?.duration}<span class="dur">{fmt.clock(insp.duration)}</span>{/if}
        </div>
        <div class="meta">
          <div class="field"><label for="vid-name">{t('fileName')}</label>
            <div class="namebox"><input id="vid-name" class="grow" bind:value={title} oninput={() => (nameTouched = true)} /><span class="ext">.{finalExt()}</span></div>
          </div>
          <div class="row small muted wrap">
            <span class="tag">{info.source === 'ytdlp' ? 'yt-dlp' : info.source ?? ''}</span>
            {#if info.extractor}<span>{info.extractor}</span>{/if}
            {#if insp?.live}<span class="tag err">{t('live')}</span><span>{t('recordLive')}</span>{/if}
          </div>
        </div>
      </div>

      {#if submitErr}<div class="card err selectable small" role="alert">{submitErr}</div>{/if}

      <div class="body">
        {#if hasPlaylistParam}<button class="link" onclick={() => analyze(true)}>{t('downloadPlaylist')}…</button>{/if}

        {#if info.playlist}
          <fieldset class="field">
            <legend>{t('playlist')} — {entrySel.size}/{info.entries?.length ?? 0} {t('entries')}</legend>
            <div class="list card">
              {#each info.entries ?? [] as e, i}
                <label class="check">
                  <input type="checkbox" checked={entrySel.has(i)} onchange={(ev) => { const s = new Set(entrySel); (ev.currentTarget as HTMLInputElement).checked ? s.add(i) : s.delete(i); entrySel = s }} />
                  <span class="grow ellipsis">{e.title || e.url}</span><span class="muted num">{fmt.clock(e.duration)}</span>
                </label>
              {/each}
            </div>
          </fieldset>
        {/if}

        {#if info.source !== 'direct'}
          <Tabs {tabs} bind:value={tab} prefix="vid" label={t('quality')} />
          <div class="panel" role="tabpanel" id={panelId('vid', tab)} aria-labelledby={tabId('vid', tab)}>
            {#if tab === 'video'}
              <div class="qgrid" role="radiogroup" aria-label={t('quality')} tabindex="-1" onkeydown={(e) => radioKeys(e, videoQs, choice, (id) => { const q = videoQs.find((x) => x.id === id); if (q) pick(q) })}>
                {#each videoQs as q}
                  {@const est = estTime(q)}
                  <button class="qcard" role="radio" aria-checked={choice === q.id} tabindex={choice === q.id || (!chosen && q === videoQs[0]) ? 0 : -1} onclick={() => pick(q)}>
                    <b>{q.label}</b>
                    <span>{output.toUpperCase()}{q.sizeEstimate ? ' · ~' + fmt.size(q.sizeEstimate) : ''}</span>
                    {#if est}<span class="muted">{tf('vidEst', { d: est })}</span>{:else if q.bandwidth}<span class="muted">{(q.bandwidth / 1e6).toFixed(1)} Mbit/s</span>{/if}
                  </button>
                {/each}
              </div>
              <div class="field fmt"><label for="vid-out">{t('format')}</label>
                <select id="vid-out" bind:value={output}>
                  <option value="mp4">{t('vidMp4')}</option><option value="mkv">MKV</option>
                  {#if info.source === 'hls' || isHoster}<option value="ts">{t('vidTs')}</option>{/if}
                </select>
              </div>
            {:else if tab === 'audio'}
              <div class="qgrid" role="radiogroup" aria-label={t('format')} tabindex="-1" onkeydown={(e) => radioKeys(e, audioChoices, audioFmt, (id) => (audioFmt = id))}>
                {#each audioChoices as a}
                  <button class="qcard" role="radio" aria-checked={audioFmt === a.id} tabindex={audioFmt === a.id ? 0 : -1} onclick={() => (audioFmt = a.id)}><b>{a.l}</b><span>{t(a.d)}</span></button>
                {/each}
              </div>
            {:else if tab === 'subs'}
              <div class="chips">
                {#each info.subtitles!.slice(0, 40) as s}
                  <label class="opt"><input type="checkbox" checked={subs.has(s)} onchange={(e) => { const n = new Set(subs); (e.currentTarget as HTMLInputElement).checked ? n.add(s) : n.delete(s); subs = n }} /> {s}</label>
                {/each}
              </div>
            {:else if tab === 'adv'}
              <table class="grid small">
                <caption class="sr-only">{t('vidTabAdvanced')}</caption>
                <tbody>
                  {#if !isHoster}
                    {#each [...(info.variants ?? [])].sort((a, b) => b.bandwidth - a.bandwidth) as v}
                      <tr><td><label class="check"><input type="radio" name="vfmt" bind:group={choice} value={v.url} /> {v.width}×{v.height}</label></td>
                        <td>{(v.bandwidth / 1e6).toFixed(2)} Mbit/s</td><td class="muted ellipsis">{v.codecs}</td></tr>
                    {/each}
                  {/if}
                  {#each ytFormats as f}
                    <tr>
                      <td><label class="check"><input type="radio" name="vfmt" bind:group={choice} value={'f:' + f.id} /> {f.id}</label></td>
                      <td>{f.ext}</td>
                      <td>{f.height ? f.height + 'p' : f.vcodec === 'none' ? 'audio' : ''}{f.fps ? ' ' + f.fps + 'fps' : ''}</td>
                      <td class="muted">{f.vcodec !== 'none' ? f.vcodec.split('.')[0] : ''} {f.acodec !== 'none' ? f.acodec.split('.')[0] : ''}</td>
                      <td class="num">{f.filesize ? fmt.size(f.filesize) : ''}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {/if}
          </div>
        {:else}
          <div class="card"><b>{(insp?.outputExt ?? '').toUpperCase()}</b> · {fmt.size(info.formats?.[0]?.filesize ?? -1)}</div>
        {/if}

        <div class="field dirf"><label for="vid-dir">{t('saveTo')}</label>
          <div class="row"><input id="vid-dir" class="grow" bind:value={dir} oninput={() => (dirTouched = true)} />
            <IconButton icon="folderOpen" variant="default" label={t('browse')} onclick={async () => { const d = await api.chooseFolder(dir); if (d) { dir = d; dirTouched = true } }} /></div>
        </div>
      </div>
    {/if}
    <div class="actions">
      <Button onclick={cancel}>{t('cancel')}</Button>
      {#if info}
        <Button disabled={busy || loading} onclick={() => submit(false)}>{t('later')}</Button>
        <Button variant="primary" icon="download" loading={busy} disabled={loading} onclick={() => submit(true)}>{t('startNow')}</Button>
      {/if}
    </div>
  </div>
</WindowShell>

<style>
  .center { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; gap: 6px; min-height: 0; }
  .center p { margin: 0; }
  .u { max-width: 90%; }
  .skel { display: flex; gap: 14px; width: 100%; max-width: 520px; margin-top: 18px; opacity: 0.7; }
  .sk-lines { flex: 1; display: flex; flex-direction: column; gap: 10px; padding-top: 6px; }
  .errbox { max-width: 90%; max-height: 90px; overflow: auto; margin: 0; }
  .hero { display: flex; gap: 16px; }
  .thumb { width: 168px; height: 95px; border-radius: var(--r-md); background: var(--surface-3) center/cover no-repeat; flex: none; position: relative;
    display: flex; align-items: center; justify-content: center; color: var(--muted); box-shadow: var(--elev-2); }
  .dur { position: absolute; right: 6px; bottom: 6px; background: rgba(0, 0, 0, 0.72); color: #fff; font-size: var(--fs-micro); padding: 1px 5px; border-radius: var(--r-sm); }
  .meta { flex: 1; min-width: 0; }
  .namebox { display: flex; align-items: center; gap: 6px; }
  .ext { color: var(--muted); font-weight: 600; }
  .wrap { flex-wrap: wrap; }
  .panel { margin-top: 12px; }
  .qgrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 8px; }
  .qcard { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; text-align: left; padding: 10px 12px; border-radius: var(--r-md);
    border: 1px solid var(--border); background: var(--surface-1); min-height: 0; white-space: normal; }
  .qcard b { font-size: var(--fs-subtitle); }
  .qcard span { font-size: var(--fs-caption); color: var(--fg2); }
  .qcard span.muted { color: var(--muted); }
  .qcard[aria-checked='true'] { border-color: var(--accent); background: var(--accent-soft); box-shadow: inset 0 0 0 1px var(--accent); }
  .qcard[aria-checked='true'] b { color: var(--accent-text); }
  .fmt { width: 240px; margin-top: 12px; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .opt { display: flex; align-items: center; gap: 6px; background: var(--surface-1); border: 1px solid var(--border); border-radius: var(--r-md); padding: 5px 9px; }
  fieldset { border: none; padding: 0; margin: 0 0 12px; }
  legend { color: var(--fg2); font-size: var(--fs-caption); font-weight: 600; padding: 0; margin-bottom: 6px; }
  .list { max-height: 150px; overflow: auto; padding: 4px 10px; }
  .dirf { margin-top: 12px; }
</style>
