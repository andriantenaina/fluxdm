<script lang="ts">
  import { onMount } from 'svelte'
  import { size, speed, duration, percent, fmtEta, fmtClock } from '../lib/format'
  import { KIND_COLOR, extOfName, categoryKind, type FileKind } from '../lib/filetype'
  import { t, tf, tn, setLang, guessLang } from '../lib/i18n.svelte'
  import { applyTheme } from '../lib/theme.svelte'
  import Icon from '../components/Icon.svelte'
  import Progress from '../components/ui/Progress.svelte'
  import RemoteConfirm from './RemoteConfirm.svelte'
  import type { CategoryKey } from '../lib/types'
  import {
    remote, captureToken, parseIncoming, kindOf, Unauthorized,
    type LiteItem, type LiteProgress, type Inspection, type Quality, type Incoming,
  } from './client'

  type Tab = 'active' | 'add' | 'done' | 'settings'

  let tab = $state<Tab>('active')
  let paired = $state(true)
  let info = $state<{ version: string; device: string; base: string } | null>(null)
  let items = $state<LiteItem[]>([])
  let prog = $state<Record<string, LiteProgress>>({})
  let toast = $state('')
  let toastT: ReturnType<typeof setTimeout> | undefined

  // Add tab
  let url = $state('')
  let incoming = $state<Incoming | null>(null)
  let candidate = $state('')
  let analyzing = $state(false)
  let analyzed = $state<{ kind: string; insp: Inspection; url: string } | null>(null)
  let quality = $state('')
  let fileName = $state('')
  let nameEdited = $state(false)
  let addErr = $state('')
  let busy = $state(false)

  // Settings tab
  let bm = $state('')

  const active = $derived(items.filter((i) => i.status !== 'completed'))
  const done = $derived(items.filter((i) => i.status === 'completed'))

  function say(msg: string) {
    toast = msg
    clearTimeout(toastT)
    toastT = setTimeout(() => (toast = ''), 2600)
  }

  function handle(e: unknown) {
    if (e instanceof Unauthorized) paired = false
    else say(e instanceof Error ? e.message : String(e))
  }

  async function load() {
    try {
      items = await remote.items()
    } catch (e) { handle(e) }
  }

  let es: EventSource | null = null
  function listen() {
    es?.close()
    es = remote.events()
    es.addEventListener('progress', (ev) => {
      for (const p of JSON.parse((ev as MessageEvent).data) as LiteProgress[]) {
        prog[p.id] = p
        const it = items.find((x) => x.id === p.id)
        if (it) {
          it.downloaded = p.downloaded
          if (p.size > 0) it.size = p.size
        }
      }
    })
    es.addEventListener('items', () => load())
    es.onerror = () => {
      // Safari closes SSE when the app goes to the background: reconnect later
      es?.close()
      es = null
      setTimeout(() => { if (!es && document.visibilityState === 'visible') { load(); listen() } }, 3000)
    }
  }

  function readRoute() {
    const h = location.hash
    const qi = h.indexOf('?')
    if (h.startsWith('#/remote/add') && qi >= 0) {
      const inc = parseIncoming(h.slice(qi + 1))
      if (inc) {
        incoming = inc
        tab = 'add'
        const first = inc.media.find((u) => /\.m3u8|\.mpd/i.test(u)) || inc.media[inc.media.length - 1] || inc.frames[0] || inc.page
        pick(first)
      }
      history.replaceState(null, '', location.pathname + location.search + '#/remote')
    }
  }

  setLang(guessLang())

  onMount(() => {
    applyTheme('system')
    captureToken()
    document.documentElement.classList.add('remote')
    remote.info().then((i) => { info = i; paired = true; load(); listen(); readRoute() }).catch(handle)
    const vis = () => { if (document.visibilityState === 'visible' && paired) { load(); if (!es) listen() } }
    document.addEventListener('visibilitychange', vis)
    window.addEventListener('hashchange', readRoute)
    return () => {
      es?.close()
      document.removeEventListener('visibilitychange', vis)
      window.removeEventListener('hashchange', readRoute)
    }
  })

  // ---------- add ----------
  function isPageUrl(u: string) {
    return !!incoming && (u === incoming.page || incoming.frames.includes(u))
  }

  async function pick(u: string) {
    candidate = u
    url = u
    await analyze()
  }

  async function paste() {
    try {
      const t = await navigator.clipboard.readText()
      if (t) url = t.trim()
    } catch { say(t('rmPasteFail')) }
  }

  async function analyze() {
    const u = url.trim()
    if (!/^https?:\/\//i.test(u) && !u.startsWith('magnet:')) { addErr = t('rmInvalidUrl'); return }
    addErr = ''
    analyzed = null
    analyzing = true
    try {
      const page = isPageUrl(u)
      const k = kindOf(u, page)
      const ref = incoming && u !== incoming.page ? incoming.page : ''
      const title = incoming?.title || ''
      const r = await remote.analyze(u, k, title, ref)
      analyzed = { kind: r.kind, insp: r.inspection, url: u }
      quality = r.inspection.defaultQuality
      fileName = r.inspection.fileName
      nameEdited = false
    } catch (e) {
      if (e instanceof Unauthorized) return handle(e)
      // not analyzable (plain file, unknown page): it can still be added as is
      addErr = (e instanceof Error ? e.message : String(e))
    } finally {
      analyzing = false
    }
  }

  function chooseQuality(q: Quality) {
    quality = q.id
    if (!nameEdited && analyzed) {
      const base = (analyzed.insp.title || 'video').replace(/[\\/:*?"<>|]+/g, ' ').trim()
      fileName = base + (q.label && q.label !== 'Auto' && q.label !== 'Original' && q.label !== 'Audio' ? ` (${q.label})` : '') + '.' + q.ext
    }
  }

  async function download(direct = false) {
    const u = url.trim()
    if (!u) return
    busy = true
    try {
      const ref = incoming && u !== incoming.page ? incoming.page : ''
      if (direct || !analyzed) {
        await remote.add({ url: u, pageUrl: incoming?.page, title: incoming?.title, referer: ref })
      } else {
        const q = analyzed.insp.qualities.find((x) => x.id === quality)
        let qid = quality
        // variant URLs are only valid for a plain HLS master; other kinds re-resolve by height
        if (qid.startsWith('http') && analyzed.kind !== 'hls') qid = q?.height ? 'h:' + q.height : ''
        await remote.add({
          url: u, pageUrl: incoming?.page, title: analyzed.insp.title, kind: analyzed.kind, quality: qid,
          output: q?.ext, fileName: fileName.trim() || undefined, referer: ref,
        })
      }
      say(t('rmAdded'))
      url = ''
      analyzed = null
      incoming = null
      tab = 'active'
      load()
    } catch (e) { handle(e) } finally { busy = false }
  }

  // ---------- list ----------
  async function toggle(it: LiteItem) {
    try {
      if (it.status === 'downloading' || it.status === 'queued' || it.status === 'waiting') await remote.pause([it.id])
      else await remote.resume([it.id])
      setTimeout(load, 400)
    } catch (e) { handle(e) }
  }

  let confirming = $state<LiteItem | null>(null)

  function del(it: LiteItem) {
    confirming = it
  }

  async function confirmed(ok: boolean) {
    const it = confirming
    confirming = null
    if (!ok || !it) return
    try { await remote.remove([it.id]); items = items.filter((x) => x.id !== it.id) } catch (e) { handle(e) }
  }

  const isRunning = (it: LiteItem) => it.status === 'downloading' || it.status === 'queued' || it.status === 'waiting'

  const EXT_KIND: [FileKind, string][] = [
    ['video', 'mp4 mkv webm mov avi ts m4v flv'], ['audio', 'mp3 m4a aac flac wav ogg opus'], ['archive', 'zip rar 7z gz tar iso'],
    ['doc', 'pdf doc docx epub txt'], ['image', 'jpg jpeg png gif webp heic'], ['app', 'exe msi dmg pkg apk'],
  ]

  function color(it: LiteItem) {
    if (it.kind === 'torrent') return KIND_COLOR.torrent
    const ext = it.ext || extOfName(it.name)
    const byExt = EXT_KIND.find(([, l]) => l.split(' ').includes(ext))
    if (byExt) return KIND_COLOR[byExt[0]]
    // the category may be a stable key or a (localised) name
    const k = categoryKind({ name: it.category, key: (it.category in KIND_COLOR ? it.category : '') as CategoryKey })
    return KIND_COLOR[k]
  }

  function statusLabel(s: string): string {
    const m: Record<string, string> = {
      downloading: t('rmStDownloading'), queued: t('rmStQueued'), waiting: t('rmStWaiting'), paused: t('rmStPaused'),
      error: t('rmStError'), completed: t('rmStCompleted'), processing: t('rmStProcessing'),
    }
    return m[s] || s
  }

  const TABS: { id: Tab; icon: string; label: () => string }[] = [
    { id: 'active', icon: 'download', label: () => t('rmTabActive') },
    { id: 'add', icon: 'plus', label: () => t('rmTabAdd') },
    { id: 'done', icon: 'checkCircle', label: () => t('rmTabDone') },
    { id: 'settings', icon: 'gear', label: () => t('rmTabSettings') },
  ]

  function tabKey(e: KeyboardEvent) {
    const i = TABS.findIndex((x) => x.id === tab)
    let j = -1
    if (e.key === 'ArrowRight') j = (i + 1) % TABS.length
    else if (e.key === 'ArrowLeft') j = (i - 1 + TABS.length) % TABS.length
    else if (e.key === 'Home') j = 0
    else if (e.key === 'End') j = TABS.length - 1
    if (j < 0) return
    e.preventDefault()
    tab = TABS[j].id
    queueMicrotask(() => document.getElementById('rtab-' + TABS[j].id)?.focus())
  }

  const titles: Record<Tab, () => string> = {
    active: () => t('rmTitleActive'), add: () => t('rmTitleAdd'), done: () => t('rmTitleDone'), settings: () => t('rmTitleSettings'),
  }

  // ---------- settings ----------
  async function loadBookmarklet() {
    try { bm = (await remote.bookmarklet()).code } catch (e) { handle(e) }
  }
  async function copy(text: string, what: string) {
    try { await navigator.clipboard.writeText(text); say(tf('rmCopied', { what })) } catch {
      say(t('rmCopyFail'))
    }
  }
  $effect(() => { if (tab === 'settings' && !bm && paired) loadBookmarklet() })

  const base = $derived(info?.base || location.origin)
</script>

<div class="app">
  {#if !paired}
    <div class="pair">
      <div class="logo" aria-hidden="true"><Icon name="download" size={36} stroke={2} /></div>
      <h1>FluxDM Remote</h1>
      <p>{t('rmNotPaired')}</p>
      <ol>
        <li>{t('rmPairStep1')}</li>
        <li>{t('rmPairStep2')}</li>
      </ol>
    </div>
  {:else}
    <header>
      <h1>{titles[tab]()}</h1>
    </header>

    <div class="main" id="rpanel-{tab}" role="tabpanel" aria-labelledby="rtab-{tab}" tabindex="-1">
      {#if tab === 'active'}
        {#if active.length === 0}
          <div class="empty">
            <div class="eic" aria-hidden="true"><Icon name="download" size={32} /></div>
            <h2>{t('rmNoActive')}</h2>
            <p>{t('rmNoActiveHint')}</p>
            <button class="primary" onclick={() => (tab = 'add')}><Icon name="plus" size={18} />{t('rmAddLink')}</button>
          </div>
        {/if}
        {#each active as it (it.id)}
          {@const p = prog[it.id]}
          {@const pct = percent(it.downloaded, it.size)}
          {@const running = it.status === 'downloading'}
          {@const end = p ? fmtClock(p.eta) : ''}
          <div class="card">
            <div class="badge" style:background={color(it)} aria-hidden="true">{(it.ext || 'file').slice(0, 4).toUpperCase()}</div>
            <div class="body">
              <div class="name">{it.name}</div>
              <div class="meta">
                {#if running && p}
                  <span class="num">{speed(p.speed)}</span> · <span class="num">{fmtEta(p)}</span>{#if end}<span class="clock"> · {end}</span>{/if}
                {:else}
                  <span class:err={it.status === 'error'}>{statusLabel(it.status)}</span>
                {/if}
                · <span class="num">{size(it.downloaded)}{it.size > 0 ? ' / ' + size(it.size) : ''}</span>
              </div>
              <div class="pbar">
                <Progress value={it.size > 0 ? pct : 0} indeterminate={it.size <= 0 && running} size="md"
                  tone={it.status === 'error' ? 'err' : running ? 'accent' : 'paused'} label={tf('rmProgress', { name: it.name })} />
              </div>
              {#if it.error}<div class="small err">{it.error}</div>{/if}
            </div>
            <div class="acts">
              <button class="icon" aria-label={isRunning(it) ? t('rmPause') : t('rmResume')} onclick={() => toggle(it)}>
                <Icon name={isRunning(it) ? 'pause' : 'play'} size={20} />
              </button>
              <button class="icon" aria-label={t('rmRemove')} onclick={() => del(it)}><Icon name="x" size={20} /></button>
            </div>
          </div>
        {/each}
      {:else if tab === 'add'}
        <div class="field">
          <input type="url" placeholder={t('rmUrlPlaceholder')} aria-label={t('rmUrlLabel')} bind:value={url} autocapitalize="off" autocorrect="off" spellcheck="false"
            aria-invalid={addErr ? 'true' : undefined} aria-describedby={addErr ? 'add-err' : undefined}
            onkeydown={(e) => e.key === 'Enter' && analyze()} />
          <button onclick={paste}><Icon name="copy" size={18} />{t('rmPaste')}</button>
        </div>
        <div class="row">
          <button class="primary grow" disabled={!url.trim() || analyzing} onclick={analyze}>{analyzing ? t('rmAnalyzing') : t('rmAnalyze')}</button>
          <button class="grow" disabled={!url.trim() || busy} onclick={() => download(true)}>{t('rmDirect')}</button>
        </div>

        {#if incoming && (incoming.media.length + incoming.frames.length) > 0}
          <h2>{t('rmFoundOnPage')}</h2>
          <div class="cands" role="radiogroup" aria-label={t('rmFoundOnPage')}>
            {#each [...new Set([...incoming.media, ...incoming.frames, incoming.page])] as u}
              <button class="cand" class:sel={candidate === u} role="radio" aria-checked={candidate === u} onclick={() => pick(u)}>
                <b>{u === incoming.page ? t('rmWholePage') : incoming.frames.includes(u) ? t('rmEmbedded') : (u.split('?')[0].split('.').pop() || '').toUpperCase()}</b>
                <span>{u}</span>
              </button>
            {/each}
          </div>
        {/if}

        {#if addErr}<p class="err small" id="add-err" role="alert">{addErr}</p>{/if}

        {#if analyzing}
          <div class="skel" aria-hidden="true"></div><div class="skel" aria-hidden="true"></div>
        {:else if analyzed}
          <h2>{analyzed.insp.title || t('rmVideo')}</h2>
          {#if analyzed.insp.thumbnail}<img class="thumb" src={analyzed.insp.thumbnail} alt="" referrerpolicy="no-referrer" />{/if}
          {#if analyzed.insp.duration > 0}<p class="small muted">{tf('rmDuration', { d: duration(analyzed.insp.duration) })}{analyzed.insp.live ? ' · ' + t('rmLive') : ''}</p>{/if}
          <div class="quals" role="radiogroup" aria-label={t('rmQuality')}>
            {#each analyzed.insp.qualities as q (q.id)}
              <button class="qual" class:sel={quality === q.id} role="radio" aria-checked={quality === q.id} onclick={() => chooseQuality(q)}>
                <b>{q.label}</b>
                <span>{q.ext.toUpperCase()}{q.sizeEstimate ? ' · ~' + size(q.sizeEstimate) : ''}</span>
              </button>
            {/each}
          </div>
          <label class="lbl" for="fname">{t('rmFileName')}</label>
          <input id="fname" class="full" bind:value={fileName} oninput={() => (nameEdited = true)} />
          <button class="primary full big" disabled={busy} onclick={() => download()}>
            <Icon name="download" size={20} />{busy ? t('rmSending') : t('rmSend')}
          </button>
        {/if}
      {:else if tab === 'done'}
        {#if done.length === 0}
          <div class="empty">
            <div class="eic" aria-hidden="true"><Icon name="checkCircle" size={32} /></div>
            <h2>{t('rmNoDone')}</h2>
            <p>{t('rmNoDoneHint')}</p>
          </div>
        {/if}
        {#each done as it (it.id)}
          <div class="card">
            <div class="badge" style:background={color(it)} aria-hidden="true">{(it.ext || 'file').slice(0, 4).toUpperCase()}</div>
            <div class="body">
              <div class="name">{it.name}</div>
              <div class="meta">{size(it.size)} · {it.host}</div>
            </div>
            <div class="acts">
              <a class="btn primary" href={remote.fileURL(it.id)} download={it.name}><Icon name="download" size={18} />{t('rmGet')}</a>
            </div>
          </div>
        {/each}
        {#if done.length > 0}
          <p class="small muted help">{t('rmGetHelp')}</p>
        {/if}
      {:else}
        <section class="box">
          <h2>{t('rmConnection')}</h2>
          <p class="small">{t('rmDevice')} : <b>{info?.device}</b> · FluxDM {info?.version}</p>
          <p class="small">{t('rmAddress')} : <code>{base}</code></p>
        </section>

        <section class="box">
          <h2>{t('rmBookmarklet')}</h2>
          <p class="small">{t('rmBookmarkletHelp')}</p>
          <ol class="small">
            <li>{t('rmBm1')}</li>
            <li>{t('rmBm2')}</li>
            <li>{t('rmBm3')}</li>
            <li>{t('rmBm4')}</li>
          </ol>
          <button class="primary full" disabled={!bm} onclick={() => copy(bm, t('rmBmCode'))}><Icon name="copy" size={18} />{t('rmCopyCode')}</button>
        </section>

        <section class="box">
          <h2>{t('rmShortcut')}</h2>
          <p class="small">{t('rmShortcutHelp')}</p>
          <ol class="small">
            <li>{t('rmSc1')}</li>
            <li>{t('rmSc2')}</li>
            <li>{t('rmSc3')}</li>
          </ol>
          <code class="blk">{base}/add?u=</code>
          <button class="full" onclick={() => copy(base + '/add?u=', t('rmAddressWord'))}><Icon name="link" size={18} />{t('rmCopyAddress')}</button>
        </section>

        <section class="box">
          <h2>{t('rmHome')}</h2>
          <p class="small">{t('rmHomeHelp')}</p>
        </section>
      {/if}
    </div>

    <div class="nav" role="tablist" aria-label={t('rmNav')} tabindex="-1" onkeydown={tabKey}>
      {#each TABS as tb (tb.id)}
        <button role="tab" id="rtab-{tb.id}" aria-controls="rpanel-{tb.id}" aria-selected={tab === tb.id} tabindex={tab === tb.id ? 0 : -1}
          class:on={tab === tb.id} onclick={() => (tab = tb.id)}>
          <span class="ic"><Icon name={tb.icon} size={22} /></span>
          <span>{tb.label()}</span>
          {#if tb.id === 'active' && active.length}<i aria-hidden="true">{active.length}</i><span class="sr-only">{tn('rmBadge', active.length)}</span>{/if}
        </button>
      {/each}
    </div>
  {/if}

  <div class="toast-region" role="status" aria-live="polite">
    {#if toast}<div class="toast">{toast}</div>{/if}
  </div>

  {#if confirming}
    <RemoteConfirm title={t('rmConfirmRemoveTitle')} message={tf('rmConfirmRemove', { name: confirming.name })}
      okLabel={t('rmRemoveOk')} cancelLabel={t('rmCancel')} danger onresult={confirmed} />
  {/if}
</div>

<style>
  :global(html.remote), :global(html.remote body) { font-size: 16px; -webkit-user-select: auto; user-select: auto; background: var(--surface-0); }
  :global(html.remote [role='alertdialog'] header button) { width: 44px; height: 44px; }
  .app { height: 100%; display: flex; flex-direction: column; -webkit-text-size-adjust: 100%; }
  header { padding: calc(env(safe-area-inset-top) + 12px) calc(env(safe-area-inset-right) + 16px) 8px calc(env(safe-area-inset-left) + 16px); }
  h1 { margin: 0; font-size: var(--fs-title-lg); font-weight: 700; }
  h2 { font-size: var(--fs-subtitle); margin: 18px 0 8px; }
  .main { flex: 1; overflow-y: auto; -webkit-overflow-scrolling: touch; padding: 4px calc(env(safe-area-inset-right) + 16px) 16px calc(env(safe-area-inset-left) + 16px); outline: none; }
  .nav {
    display: flex; border-top: 1px solid var(--border); background: var(--surface-1);
    padding-bottom: env(safe-area-inset-bottom);
  }
  .nav button {
    flex: 1; border: 0; border-radius: 0; background: none; padding: 6px 0 4px; font-size: var(--fs-micro); color: var(--muted);
    display: flex; flex-direction: column; align-items: center; gap: 2px; min-height: 52px; position: relative;
  }
  .nav button.on { color: var(--accent-text); font-weight: 600; }
  .nav .ic { display: flex; }
  .nav i { position: absolute; top: 4px; left: calc(50% + 8px); background: var(--err); color: #fff; font-style: normal; font-size: 10px; border-radius: 9px; padding: 1px 5px; font-weight: 700; }
  :global(:root[data-theme='dark']) .nav i { color: #000; }

  button, .btn {
    min-height: 44px; border-radius: var(--r-lg); border: 1px solid var(--border); background: var(--surface-1);
    padding: 0 14px; font-size: 16px; color: var(--fg); text-decoration: none; display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  }
  button:disabled { opacity: 0.5; }
  .primary { background: var(--accent); border-color: var(--accent); color: var(--accent-fg); font-weight: 600; }
  .full { width: 100%; margin-top: 10px; }
  .big { min-height: 52px; font-size: 17px; }
  .grow { flex: 1; }
  .row { display: flex; gap: 8px; margin-top: 8px; }
  .field { display: flex; gap: 8px; }
  input {
    flex: 1; min-height: 44px; border-radius: var(--r-lg); border: 1px solid var(--border); background: var(--surface-1);
    padding: 0 12px; font-size: 16px; min-width: 0;
  }
  input.full { display: block; }
  .lbl { display: block; margin-top: 14px; font-size: 13px; color: var(--muted); }

  .card {
    display: flex; gap: 12px; align-items: center; background: var(--surface-1); border: 1px solid var(--border); border-radius: var(--r-xl);
    padding: 12px; margin-bottom: 10px; box-shadow: var(--elev-2);
  }
  .badge {
    flex: none; width: 44px; height: 44px; border-radius: var(--r-lg); color: #fff; font-weight: 700; font-size: 11px;
    display: flex; align-items: center; justify-content: center;
  }
  .body { flex: 1; min-width: 0; }
  .name { font-weight: 600; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: 13px; color: var(--muted); margin-top: 2px; }
  .num { font-variant-numeric: tabular-nums; }
  .pbar { margin-top: 8px; }
  .acts { display: flex; gap: 6px; }
  .icon { width: 44px; padding: 0; }

  .cands { display: flex; flex-direction: column; gap: 6px; }
  .cand { flex-direction: column; align-items: flex-start; text-align: left; padding: 8px 12px; gap: 2px; }
  .cand span { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
  .cand.sel, .qual.sel { border-color: var(--accent); background: var(--accent-soft); box-shadow: inset 0 0 0 1px var(--accent); }
  .quals { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px; }
  .qual { flex-direction: column; min-height: 60px; gap: 2px; }
  .qual b { font-size: 17px; }
  .qual span { font-size: 12px; color: var(--muted); }
  .thumb { width: 100%; border-radius: var(--r-lg); margin-bottom: 8px; }

  .box { background: var(--surface-1); border: 1px solid var(--border); border-radius: var(--r-xl); padding: 4px 14px 14px; margin-bottom: 12px; }
  .box h2 { margin-top: 12px; }
  ol { padding-left: 20px; }
  ol li { margin: 4px 0; }
  code { font-size: 13px; word-break: break-all; }
  code.blk { display: block; background: var(--surface-2); padding: 8px 10px; border-radius: var(--r-md); margin-top: 6px; }

  .empty { text-align: center; color: var(--muted); padding: 48px 0; display: flex; flex-direction: column; align-items: center; gap: 6px; }
  .empty h2 { color: var(--fg); margin: 6px 0 0; }
  .empty p { margin: 0 0 12px; }
  .eic { width: 64px; height: 64px; border-radius: 50%; background: var(--accent-soft); color: var(--accent-text); display: flex; align-items: center; justify-content: center; }
  .skel {
    height: 60px; border-radius: var(--r-lg); margin-top: 10px;
    background: linear-gradient(90deg, var(--surface-3) 0%, var(--surface-2) 40%, var(--surface-3) 80%); background-size: 200% 100%;
    animation: shimmer 1.6s linear infinite;
  }
  .small { font-size: 13px; }
  .muted { color: var(--muted); }
  .err { color: var(--err); }
  .help { margin-top: 8px; }
  .clock { color: var(--muted); }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }

  .pair { padding: calc(env(safe-area-inset-top) + 48px) 24px; text-align: center; }
  .pair ol { text-align: left; }
  .logo { width: 72px; height: 72px; margin: 0 auto 12px; border-radius: 18px; background: var(--accent); color: var(--accent-fg); display: flex; align-items: center; justify-content: center; }

  .toast-region {
    position: fixed; left: 50%; transform: translateX(-50%); bottom: calc(env(safe-area-inset-bottom) + 70px); max-width: 90%; z-index: 50; pointer-events: none;
  }
  .toast { background: var(--fg); color: var(--surface-0); padding: 10px 16px; border-radius: var(--r-lg); font-size: 14px; box-shadow: var(--elev-16); animation: pop var(--dur) var(--ease-decel); }
  @media (prefers-reduced-motion: reduce) { .skel, .toast { animation: none; } }
</style>
