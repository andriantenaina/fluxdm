<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { slide } from 'svelte/transition'
  import { Window } from '@wailsio/runtime'
  import { api, errMsg } from '../lib/api'
  import { t, tf } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { displayName, elapsedOf, etaOf, isRunning, kindLabel, statusLabel } from '../lib/itemview'
  import { dur } from '../lib/motion'
  import { confirmBox } from '../lib/dialogs.svelte'
  import { toast } from '../lib/toast.svelte'
  import SegmentMap from '../components/SegmentMap.svelte'
  import Sparkline from '../components/ui/Sparkline.svelte'
  import FileBadge from '../components/ui/FileBadge.svelte'
  import ProgressRing from '../components/ui/ProgressRing.svelte'
  import Progress from '../components/ui/Progress.svelte'
  import AnimNum from '../components/ui/AnimNum.svelte'
  import Button from '../components/ui/Button.svelte'
  import IconButton from '../components/ui/IconButton.svelte'
  import EmptyState from '../components/ui/EmptyState.svelte'
  import Icon from '../components/Icon.svelte'
  import Dialogs from '../components/Dialogs.svelte'
  import Toasts from '../components/ui/Toasts.svelte'
  import WindowShell from '../components/windows/WindowShell.svelte'
  import SuccessCheck from '../components/windows/SuccessCheck.svelte'

  let { id }: { id: string } = $props()
  let details = $state(false)
  let mini = $state(false)
  let openWhenDone = $state(false)
  let notFound = $state(false)
  let limitKB = $state(0)
  let limitInit = false
  let wasDone = false
  let detailsEl: HTMLElement | undefined = $state()
  let detailsExtra = 0
  let announce = $state('')

  let it = $derived(store.items.find((x) => x.id === id))
  let p = $derived(store.progress[id])
  let size = $derived(p?.size && p.size > 0 ? p.size : (it?.size ?? 0))
  let done = $derived(p?.downloaded ?? it?.downloaded ?? 0)
  let pct = $derived(it?.status === 'completed' ? 100 : fmt.percent(done, size))
  let running = $derived(!!it && isRunning(it.status))
  let completed = $derived(it?.status === 'completed')
  let speed = $derived(running ? (p?.speed ?? 0) : 0)
  let hist = $derived(store.history[id] ?? [])
  let peak = $derived(hist.length ? Math.max(...hist) : 0)
  let avg = $derived.by(() => {
    if (p?.avgSpeed && p.avgSpeed > 0) return p.avgSpeed
    const nz = hist.filter((v) => v > 0)
    return nz.length ? nz.reduce((a, b) => a + b, 0) / nz.length : 0
  })
  let eta = $derived(it ? etaOf(it, p) : null)
  let elapsed = $derived(it ? elapsedOf(it, p) : 0)
  let name = $derived(it ? displayName(it) : '')
  let tone = $derived(it?.status === 'error' ? 'err' : it?.status === 'paused' || it?.status === 'queued' ? 'paused' : 'accent') as 'accent' | 'err' | 'paused'
  let tagTone = $derived(({ downloading: 'accent', completed: 'ok', error: 'err', processing: 'warn' } as Record<string, string>)[it?.status ?? ''] ?? '')
  let resumable = $derived(!it ? '' : it.kind === 'http' ? (p?.segments && p.segments.length > 1 ? t('yes') : it.size > 0 ? t('yes') : '—') : t('yes'))

  // real limit: progress (effective) else the item option, in KB/s
  $effect(() => {
    if (limitInit || !it) return
    const bps = p?.limit ?? (it.options?.speedLimit as number | undefined) ?? 0
    limitKB = Math.round((bps || 0) / 1024)
    limitInit = true
  })

  /** 1 / 2 / 5 × 10^n ceiling */
  function nice(v: number): number {
    const e = Math.pow(10, Math.floor(Math.log10(Math.max(1, v))))
    for (const m of [1, 2, 5, 10]) if (m * e >= v) return m * e
    return 10 * e
  }
  let capKB = $derived(nice(Math.max(10240, (2 * peak) / 1024, limitKB)))
  let stepKB = $derived(Math.max(16, capKB / 200))

  let limitTimer: ReturnType<typeof setTimeout> | undefined
  function applyLimit(delay = 0) {
    clearTimeout(limitTimer)
    limitTimer = setTimeout(async () => {
      try { await api.setItemOptions(id, { speedLimit: limitKB > 0 ? Math.round(limitKB * 1024) : 0 }) } catch (e) { toast(errMsg(e), 'err') }
    }, delay)
  }

  $effect(() => {
    if (!it) return
    document.title = completed ? `${t('pwDone')} · ${name}` : `${fmt.pct(pct)} · ${name}`
  })

  // announce state changes only (not every tick)
  $effect(() => {
    if (it) announce = completed ? t('doneTitle') : statusLabel(it.status)
  })

  $effect(() => {
    if (completed && !wasDone) {
      wasDone = true
      if (openWhenDone) api.openFolder(id)
      if (mini) toggleMini()
    }
  })

  onMount(() => {
    // the item may arrive a bit later by event; ask the backend before giving up
    const tm = setTimeout(async () => {
      if (store.items.some((x) => x.id === id)) return
      try {
        const x = await api.get(id)
        if (x && !store.items.some((y) => y.id === id)) store.items.push(x)
        else if (!x) notFound = true
      } catch { notFound = true }
    }, 1500)
    return () => clearTimeout(tm)
  })

  async function pause() {
    try { await api.pause([id]) } catch (e) { toast(errMsg(e), 'err') }
  }
  async function resume() {
    try { await api.start([id]) } catch (e) { toast(errMsg(e), 'err') }
  }

  async function cancel() {
    const r = await confirmBox(tf('confirmCancel', { name }), undefined, { title: t('cancelDownload'), danger: true, okLabel: t('pwCancelYes'), cancelLabel: t('pwKeep') })
    if (!r.ok) return
    try {
      await api.cancel([id])
      Window.Close()
    } catch (e) { toast(errMsg(e), 'err') }
  }

  async function toggleMini() {
    mini = !mini
    try {
      if (mini) {
        await Window.SetMinSize(320, 84)
        await Window.SetSize(400, store.frameless ? 112 : 100)
        await Window.SetAlwaysOnTop(true)
      } else {
        await Window.SetAlwaysOnTop(false)
        await Window.SetSize(580, (store.frameless ? 492 : 460) + (details ? detailsExtra : 0))
        await Window.SetMinSize(460, 380)
      }
    } catch { /* runtime unavailable */ }
  }

  async function toggleDetails() {
    try {
      const cur = await Window.Size()
      if (!details) {
        details = true
        await tick()
        detailsExtra = (detailsEl?.scrollHeight ?? 260) + 12
        if (!mini) await Window.SetSize(cur.width, cur.height + detailsExtra)
      } else {
        details = false
        if (!mini) setTimeout(() => Window.SetSize(cur.width, Math.max(380, cur.height - detailsExtra)), dur(250))
      }
    } catch {
      details = !details
    }
  }

  function close() { Window.Close() }
</script>

<WindowShell title={name || 'FluxDM'} maximisable={false}>
  <div class="sr-only" role="status" aria-live="polite">{announce}</div>
  {#if !it}
    <div class="dialog center">
      {#if notFound}
        <EmptyState art="search" title={t('pwNotFound')} text={t('pwNotFoundHint')}>
          <Button onclick={close}>{t('close')}</Button>
        </EmptyState>
      {:else}
        <ProgressRing size={40} indeterminate label={t('loading')} />
        <p class="muted">{t('loading')}</p>
      {/if}
    </div>
  {:else if mini}
    <div class="mini drag">
      <FileBadge item={it} size={30} />
      <div class="grow">
        <div class="ellipsis mnm">{name}</div>
        <Progress value={pct} {tone} indeterminate={running && !(size > 0)} label={name} valueText={fmt.pct(pct)} />
        <div class="muted small num">{size > 0 ? fmt.pct(pct) : ''} · {fmt.speed(speed)} · {fmt.fmtEta(eta)}</div>
      </div>
      <div class="nodrag row">
        {#if running}
          <IconButton icon="pause" label={t('pause')} onclick={pause} />
        {:else if !completed}
          <IconButton icon="play" label={t('resume')} onclick={resume} />
        {/if}
        <IconButton icon="expand" label={t('pwExpand')} onclick={toggleMini} />
      </div>
    </div>
  {:else}
    <div class="dialog">
      <header class="hd">
        <FileBadge item={it} size={40} />
        <div class="grow">
          <div class="nm ellipsis selectable" title={name}>{name}</div>
          <div class="muted small ellipsis">{kindLabel(it.kind)} · {fmt.hostOf(it.pageUrl || it.url)}</div>
        </div>
        <span class="tag {tagTone}">{completed ? t('s_completed') : statusLabel(it.status)}</span>
        <IconButton icon="pip" label={t('pwMini')} onclick={toggleMini} />
      </header>

      <div class="body">
        {#if completed}
          <div class="donebox">
            <SuccessCheck size={64} label={t('doneTitle')} />
            <div class="grow">
              <b class="dt">{t('doneTitle')}</b>
              <div class="muted small">{fmt.size(it.size)}{elapsed > 0 ? ' · ' + tf('pwIn', { d: fmt.duration(elapsed) }) : ''}</div>
              <div class="muted small ellipsis selectable" title={it.path}>{it.path || it.dir}</div>
            </div>
          </div>
        {:else}
          <div class="top">
            <ProgressRing size={124} stroke={8} value={pct} {tone} indeterminate={running && !(size > 0)} label={t('progress')}>
              <span class="ringpct num">{size > 0 ? Math.floor(pct) : '—'}<small>%</small></span>
              <span class="ringsub muted">{fmt.size(done)}</span>
            </ProgressRing>
            <div class="stats">
              <div class="spd" aria-label={t('currentSpeed')}>
                {#if running}<AnimNum value={speed} format={(v) => fmt.speed(v)} />{:else}<span class="muted">{statusLabel(it.status)}</span>{/if}
              </div>
              {#if it.status === 'error'}
                <p class="err small selectable errtxt">{it.error}</p>
              {:else if it.status === 'processing'}
                <p class="muted small">{t('assembling')}</p>
              {:else}
                <div class="eta">{running ? fmt.fmtEtaLong(eta) : '—'}</div>
                {#if running && eta?.eta && eta.eta > 0}<div class="muted small">{fmt.fmtClock(eta.eta)}</div>{/if}
              {/if}
              <dl class="kv">
                <dt>{t('downloaded')}</dt><dd class="num">{fmt.size(done)}{size > 0 ? ' / ' + fmt.size(size) : ''}</dd>
                <dt>{t('elapsed')}</dt><dd class="num">{elapsed > 0 ? fmt.duration(elapsed) : '—'}</dd>
                <dt>{t('avgSpeed')}</dt><dd class="num">{fmt.speed(avg)}</dd>
              </dl>
            </div>
          </div>
          {#if it.kind === 'http' && p?.segments?.length}
            <SegmentMap segments={p.segments} {size} label={tf('pwSegmentsLabel', { n: p.segments.length })} />
          {/if}
        {/if}

        <button class="link more" aria-expanded={details} aria-controls="pw-details" onclick={toggleDetails}>
          <Icon name={details ? 'chevronDown' : 'chevronRight'} size={14} />{details ? t('hideDetails') : t('showDetails')}
        </button>
        {#if details}
          <div id="pw-details" class="details" bind:this={detailsEl} transition:slide={{ duration: dur(250) }}>
            <div class="chart card">
              <div class="row small">
                <span class="muted">{t('pwLast30')}</span><span class="grow"></span>
                <span class="muted">{t('peak')}</span><b class="num">{fmt.speed(peak)}</b>
                <span class="muted">{t('average')}</span><b class="num">{fmt.speed(avg)}</b>
              </div>
              <Sparkline data={hist} height={56} samples={120} avgLine label={t('pwSpeedChart')} />
            </div>

            <div class="tiles">
              {#if it.kind === 'torrent'}
                <div><span>{t('peers')}</span><b>{p?.extra?.peers ?? 0} ({p?.extra?.seeds ?? 0} {t('seeds')})</b></div>
              {:else if p?.extra?.segmentsTotal}
                <div><span>{t('segments')}</span><b>{p.extra.segments} / {p.extra.segmentsTotal}{p.extra.live ? ' · ' + t('live') : ''}</b></div>
              {:else}
                <div><span>{t('pwActiveConns')}</span><b>{p?.conns ?? 0}</b></div>
              {/if}
              <div><span>{t('avgSpeed')}</span><b>{fmt.speed(avg)}</b></div>
              <div><span>{t('size')}</span><b>{size > 0 ? fmt.size(size) : t('unknownSize')}</b></div>
              <div><span>{t('resumeCap')}</span><b>{resumable}</b></div>
            </div>

            {#if p?.segments?.length && !completed}
              <table class="grid small segs">
                <caption class="sr-only">{t('pwConnections')}</caption>
                <thead><tr><th>#</th><th>{t('downloaded')}</th><th class="w">{t('progress')}</th></tr></thead>
                <tbody>
                  {#each p.segments as s, i}
                    {@const len = s.e > 0 ? s.e - s.s : 0}
                    {@const sp = len ? Math.min(100, (100 * (s.p - s.s)) / len) : 0}
                    <tr>
                      <td>{i + 1}</td>
                      <td class="num">{fmt.size(s.p - s.s)}{len ? ' / ' + fmt.size(len) : ''}</td>
                      <td><div class="row"><Progress value={sp} size="xs" label={tf('pwSegment', { n: i + 1 })} /><span class="num pc">{len ? fmt.pct(sp) : ''}</span></div></td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            {/if}

            <div class="kv2">
              <span class="muted">{t('address')}</span><span class="selectable ellipsis" title={it.url}>{it.url}</span>
              <span class="muted">{t('saveTo')}</span><span class="selectable ellipsis" title={it.path || it.dir}>{it.path || it.dir}</span>
            </div>

            {#if !completed}
              <div class="limit card">
                <label class="lbl" for="pw-limit">{t('limitThis')}</label>
                <div class="row">
                  <input id="pw-limit" type="range" min="0" max={capKB} step={stepKB} bind:value={limitKB} onchange={() => applyLimit()}
                    aria-valuetext={limitKB > 0 ? fmt.speed(limitKB * 1024) : t('unlimited')} />
                  <input class="kb" type="number" min="0" step="64" bind:value={limitKB} oninput={() => applyLimit(500)} aria-label={t('pwLimitKB')} />
                  <span class="muted small">{fmt.unit(1)}/s</span>
                  <Button size="sm" variant={limitKB > 0 ? 'default' : 'subtle'} onclick={() => { limitKB = 0; applyLimit() }}>{t('unlimited')}</Button>
                </div>
                <div class="muted small">{limitKB > 0 ? fmt.speed(limitKB * 1024) : t('unlimited')}</div>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <div class="actions">
        {#if completed}
          <Button onclick={close}>{t('close')}</Button>
          <Button icon="folder" onclick={() => api.openFolder(id)}>{t('openFolder')}</Button>
          <Button variant="primary" icon="openFile" onclick={() => { api.openFile(id); close() }}>{t('open')}</Button>
        {:else}
          <label class="check small"><input type="checkbox" bind:checked={openWhenDone} /> {t('pwOpenWhenDone')}</label>
          <span class="grow"></span>
          <Button variant="danger" onclick={cancel}>{t('cancel')}</Button>
          {#if running}
            <Button variant="primary" icon="pause" onclick={pause}>{t('pause')}</Button>
          {:else}
            <Button variant="primary" icon="play" onclick={resume}>{it.status === 'error' ? t('retry') : t('resume')}</Button>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</WindowShell>
<Dialogs />
<Toasts />

<style>
  .center { align-items: center; justify-content: center; }
  .hd { display: flex; align-items: center; gap: 12px; }
  .nm { font-weight: 600; font-size: var(--fs-body-lg); }
  .body { display: flex; flex-direction: column; gap: 12px; }
  .top { display: flex; align-items: center; gap: 24px; padding: 4px 4px 0; }
  .ringpct { font-family: var(--font-display); font-size: var(--fs-title-lg); font-weight: 600; line-height: 1; }
  .ringpct small { font-size: var(--fs-body); color: var(--muted); margin-left: 1px; }
  .ringsub { font-size: var(--fs-caption); margin-top: 2px; }
  .stats { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .spd { font-family: var(--font-display); font-size: var(--fs-title); font-weight: 600; color: var(--accent-text); }
  .eta { font-size: var(--fs-body-lg); font-weight: 600; }
  .errtxt { margin: 4px 0; max-height: 3.6em; overflow: auto; }
  .kv { display: grid; grid-template-columns: auto 1fr; gap: 2px 12px; margin: 8px 0 0; font-size: var(--fs-caption); }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; }
  .more { align-self: flex-start; display: inline-flex; align-items: center; gap: 4px; }
  .details { display: flex; flex-direction: column; gap: 10px; }
  .chart { padding: 8px 12px 6px; }
  .chart .row { gap: 6px; margin-bottom: 4px; }
  .tiles { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
  .tiles div { background: var(--surface-1); border: 1px solid var(--border); border-radius: var(--r-md); padding: 8px 10px; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .tiles span { font-size: var(--fs-caption); color: var(--muted); }
  .tiles b { font-size: var(--fs-body); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-variant-numeric: tabular-nums; }
  .segs th, .segs td { padding: 4px 8px; }
  .segs .w { width: 45%; }
  .pc { width: 44px; text-align: right; }
  .kv2 { display: grid; grid-template-columns: 110px 1fr; gap: 4px 8px; font-size: var(--fs-caption); }
  .limit { display: flex; flex-direction: column; gap: 6px; }
  .lbl { font-size: var(--fs-caption); font-weight: 600; color: var(--fg2); }
  .kb { width: 96px; flex: none; }
  .donebox { display: flex; align-items: center; gap: 16px; background: var(--ok-soft); border-radius: var(--r-lg); padding: 16px; }
  .dt { font-size: var(--fs-subtitle); }
  .mini { display: flex; align-items: center; gap: 10px; flex: 1; padding: 10px 12px; }
  .mnm { font-size: var(--fs-caption); font-weight: 600; margin-bottom: 4px; }
  .mini .small { margin-top: 3px; }
</style>
