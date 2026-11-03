<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store, handoff } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { AddRequest, VideoInfo } from '../lib/types'

  let { token }: { token: string } = $props()

  let req = $state<AddRequest>({ url: '', start: true })
  let url = $state('')
  let kind = $state('')
  let info = $state<VideoInfo | null>(null)
  let loading = $state(true)
  let error = $state('')
  let busy = $state(false)

  // choices
  let choice = $state('best') // best | h:<height> | v:<variantUrl> | f:<formatId> | d:<repId> | audio
  let audioChoice = $state('') // dash audio rep / hls audio lang
  let output = $state('mp4')
  let subs = $state<Set<string>>(new Set())
  let fileName = $state('')
  let dir = $state('')
  let entrySel = $state<Set<number>>(new Set())

  onMount(async () => {
    try {
      const p = await api.pending(token)
      req = p.request
    } catch (e) { error = errMsg(e) }
    url = handoff.url || req.url
    const headers = Object.keys(handoff.headers).length ? handoff.headers : (req.headers ?? {})
    req.headers = headers
    kind = req.kind || (await api.detectKind(url))
    if (kind === 'http' && !/\.(mp4|webm|mkv|mov|m4v|flv|ts|mp3|m4a)(\?|$)/i.test(url)) kind = 'ytdlp'
    output = store.settings?.hlsOutput ?? 'mp4'
    dir = req.dir || (store.settings ? store.settings.downloadDir + (store.platform === 'windows' ? '\\' : '/') + 'Vidéos' : '')
    const cat = store.settings?.categories.find((c) => c.name === 'Vidéos')
    if (!req.dir && cat?.dir) dir = cat.dir
    await analyze(false)
  })

  async function analyze(playlist: boolean) {
    loading = true
    error = ''
    try {
      info = await api.analyzeVideo(kind, url, req.headers ?? {}, playlist)
      fileName = req.title || info.title || ''
      if (info.playlist) entrySel = new Set((info.entries ?? []).map((_, i) => i))
      const q = store.settings?.videoQuality ?? 'best'
      choice = q === 'audio' ? 'audio' : q === 'best' ? 'best' : 'h:' + q
    } catch (e) {
      error = errMsg(e)
      // fall back to what the browser extension detected
      const vs = req.options?.variants as any[] | undefined
      if (vs?.length) {
        info = { source: kind, title: req.title ?? '', duration: 0, formats: null, playlist: false, live: false,
          variants: vs.map((v) => ({ url: v.url, bandwidth: v.bandwidth, width: v.width, height: v.height, codecs: '', frameRate: 0 })) }
      }
    } finally {
      loading = false
    }
  }

  let heights = $derived.by(() => {
    const hs = new Set<number>()
    for (const f of info?.formats ?? []) if (f.height && f.vcodec !== 'none') hs.add(f.height)
    for (const v of info?.variants ?? []) if (v.height) hs.add(v.height)
    for (const r of info?.dash ?? []) if (r.kind === 'video' && r.height) hs.add(r.height)
    return [...hs].sort((a, b) => b - a)
  })

  let ytFormats = $derived((info?.formats ?? []).filter((f) => f.id !== 'direct').sort((a, b) => (b.height || 0) - (a.height || 0) || (b.tbr || 0) - (a.tbr || 0)))
  let dashAudio = $derived((info?.dash ?? []).filter((r) => r.kind === 'audio'))
  let isHoster = $derived(info?.source?.startsWith('hoster') ?? false)
  let isAudio = $derived(choice === 'audio')
  let hasPlaylistParam = $derived(/[?&]list=/.test(url) && !info?.playlist)

  function estSize(h: number): string {
    const fs = (info?.formats ?? []).filter((f) => f.height === h && f.filesize > 0)
    if (!fs.length) return ''
    const best = Math.max(...fs.map((f) => f.filesize))
    return '~' + fmt.size(best)
  }

  function buildOptions(): Record<string, any> {
    const o: Record<string, any> = {}
    o.output = isAudio ? (output === 'm4a' ? 'm4a' : 'mp3') : output
    if (choice === 'audio') { o.audioOnly = true; o.quality = 'audio' }
    else if (choice === 'best') o.quality = 'best'
    else if (choice.startsWith('h:')) o.maxHeight = Number(choice.slice(2))
    else if (choice.startsWith('v:')) o.variantUrl = choice.slice(2)
    else if (choice.startsWith('d:')) o.videoRep = choice.slice(2)
    else if (choice.startsWith('f:')) {
      const id = choice.slice(2)
      const f = info?.formats?.find((x) => x.id === id)
      o.format = f && f.acodec === 'none' ? `${id}+ba/${id}` : id
    }
    if (audioChoice) {
      if (info?.source === 'dash') o.audioRep = audioChoice
      else o.audioLang = audioChoice
    }
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

  async function submit(start: boolean) {
    if (!info) return
    busy = true
    error = ''
    try {
      const base: AddRequest = {
        url, pageUrl: req.pageUrl, title: fileName || info.title, headers: req.headers, kind: itemKind(),
        dir, category: isAudio ? 'Musique' : 'Vidéos', options: buildOptions(), start,
      }
      if (info.source === 'direct') {
        base.fileName = fileName ? (fmt.extOf(fileName) ? fileName : fileName + '.' + (info.formats?.[0]?.ext || 'mp4')) : ''
        base.options = {}
      }
      if (info.playlist && info.entries?.length) {
        const reqs = info.entries.filter((_, i) => entrySel.has(i)).map((e) => ({ ...base, url: e.url, title: e.title, fileName: '' }))
        await api.addBatch(reqs)
      } else {
        await api.add(base)
      }
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

  async function browse() {
    const d = await api.chooseFolder(dir)
    if (d) dir = d
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && cancel()} />

<div class="dialog">
  <h1>{t('videoTitle')}</h1>
  <div class="body">
    {#if loading}
      <div class="center"><div class="bar indeterminate" style="width:60%"><div></div></div><p class="muted">{t('analyzing')}</p><p class="muted ellipsis small">{url}</p></div>
    {:else}
      {#if error}<div class="card err selectable">{error}</div>{/if}
      {#if info}
        <div class="head">
          {#if info.thumbnail}<img src={info.thumbnail} alt="" />{/if}
          <div class="meta">
            <div class="field"><span>{t('fileName')}</span><input bind:value={fileName} /></div>
            <div class="row small muted">
              {#if info.duration}<span>{t('duration')} : {fmt.clock(info.duration)}</span>{/if}
              <span>{t('source')} : {info.source}{info.extractor ? ' · ' + info.extractor : ''}</span>
              {#if info.live}<span class="tag accent">{t('live')}</span>{/if}
            </div>
            {#if info.live}<div class="small warn">{t('recordLive')}</div>{/if}
          </div>
        </div>

        {#if hasPlaylistParam}
          <button class="link" onclick={() => analyze(true)}>{t('downloadPlaylist')}…</button>
        {/if}

        {#if info.playlist}
          <div class="field">
            <span>{t('playlist')} — {entrySel.size}/{info.entries?.length ?? 0} {t('entries')}</span>
            <div class="list card">
              {#each info.entries ?? [] as e, i}
                <label class="check">
                  <input type="checkbox" checked={entrySel.has(i)} onchange={(ev) => { const s = new Set(entrySel); (ev.currentTarget as HTMLInputElement).checked ? s.add(i) : s.delete(i); entrySel = s }} />
                  <span class="grow ellipsis">{e.title || e.url}</span><span class="muted">{fmt.clock(e.duration)}</span>
                </label>
              {/each}
            </div>
          </div>
        {/if}

        {#if info.source !== 'direct'}
          <div class="field">
            <span>{t('quality')}</span>
            <div class="choices">
              <label class="opt"><input type="radio" bind:group={choice} value="best" /> {t('best')}</label>
              {#each heights as h}
                <label class="opt"><input type="radio" bind:group={choice} value={'h:' + h} /> {h}p <span class="muted small">{estSize(h)}</span></label>
              {/each}
              {#if info.source === 'ytdlp' || info.source.startsWith('hoster') || (info.audio?.length ?? 0) > 0 || dashAudio.length}
                <label class="opt"><input type="radio" bind:group={choice} value="audio" /> {t('audioOnly')}</label>
              {/if}
            </div>
          </div>

          {#if !isHoster && (info.variants?.length ?? 0) > 0}
            <details>
              <summary class="muted small">Variantes HLS ({info.variants!.length})</summary>
              <table class="grid small">
                <tbody>
                  {#each [...info.variants!].sort((a, b) => b.bandwidth - a.bandwidth) as v}
                    <tr><td><label class="check"><input type="radio" bind:group={choice} value={'v:' + v.url} /> {v.width}×{v.height}</label></td>
                      <td>{fmt.size(v.bandwidth / 8)}/s</td><td class="muted ellipsis">{v.codecs}</td></tr>
                  {/each}
                </tbody>
              </table>
            </details>
          {/if}

          {#if ytFormats.length > 0 && info.source === 'ytdlp'}
            <details>
              <summary class="muted small">Formats ({ytFormats.length})</summary>
              <table class="grid small">
                <tbody>
                  {#each ytFormats as f}
                    <tr>
                      <td><label class="check"><input type="radio" bind:group={choice} value={'f:' + f.id} /> {f.id}</label></td>
                      <td>{f.ext}</td>
                      <td>{f.height ? f.height + 'p' : f.vcodec === 'none' ? 'audio' : ''}{f.fps ? ' ' + f.fps + 'fps' : ''}</td>
                      <td class="muted">{f.vcodec !== 'none' ? f.vcodec.split('.')[0] : ''} {f.acodec !== 'none' ? f.acodec.split('.')[0] : ''}</td>
                      <td>{f.filesize ? fmt.size(f.filesize) : ''}</td>
                      <td class="muted ellipsis">{f.note}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </details>
          {/if}

          {#if (info.audio?.length ?? 0) > 1 || dashAudio.length > 1}
            <div class="field">
              <span>{t('audioTrack')}</span>
              <select bind:value={audioChoice}>
                <option value="">Auto</option>
                {#each info.audio ?? [] as a}<option value={a.language}>{a.name} {a.language ? '(' + a.language + ')' : ''}</option>{/each}
                {#each dashAudio as a}<option value={a.id}>{a.lang ?? ''} {fmt.size(a.bandwidth / 8)}/s {a.codecs}</option>{/each}
              </select>
            </div>
          {/if}

          {#if (info.subtitles?.length ?? 0) > 0}
            <div class="field">
              <span>{t('subtitles')}</span>
              <div class="choices">
                {#each info.subtitles!.slice(0, 30) as s}
                  <label class="opt"><input type="checkbox" checked={subs.has(s)} onchange={(e) => { const n = new Set(subs); (e.currentTarget as HTMLInputElement).checked ? n.add(s) : n.delete(s); subs = n }} /> {s}</label>
                {/each}
              </div>
            </div>
          {/if}

          <div class="field" style="width:200px">
            <span>{t('format')}</span>
            <select bind:value={output}>
              {#if isAudio}
                <option value="mp3">MP3</option><option value="m4a">M4A</option>
              {:else}
                <option value="mp4">MP4</option><option value="mkv">MKV</option>
                {#if info.source === 'hls' || isHoster}<option value="ts">TS</option>{/if}
              {/if}
            </select>
          </div>
        {:else}
          <p>{t('size')} : <b>{fmt.size(info.formats?.[0]?.filesize ?? -1)}</b></p>
        {/if}

        <div class="field">
          <span>{t('saveTo')}</span>
          <div class="row"><input class="grow" bind:value={dir} /><button onclick={browse}>{t('browse')}</button></div>
        </div>
      {/if}
    {/if}
  </div>
  <div class="actions">
    <button onclick={cancel}>{t('cancel')}</button>
    <button disabled={busy || loading || !info} onclick={() => submit(false)}>{t('later')}</button>
    <button class="primary" disabled={busy || loading || !info} onclick={() => submit(true)}>{t('startNow')}</button>
  </div>
</div>

<style>
  .center { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; gap: 10px; }
  .head { display: flex; gap: 12px; margin-bottom: 10px; }
  .head img { width: 150px; height: 84px; object-fit: cover; border-radius: 6px; background: var(--bg3); }
  .meta { flex: 1; min-width: 0; }
  .small { font-size: 12px; }
  .warn { color: var(--warn); }
  .choices { display: flex; flex-wrap: wrap; gap: 6px; }
  .opt { display: flex; align-items: center; gap: 6px; background: var(--bg2); border: 1px solid var(--border); border-radius: 6px; padding: 5px 10px; cursor: pointer; }
  .list { max-height: 160px; overflow: auto; padding: 6px 10px; }
  details { margin-bottom: 10px; }
  details table { margin-top: 6px; }
  summary { cursor: pointer; }
</style>
