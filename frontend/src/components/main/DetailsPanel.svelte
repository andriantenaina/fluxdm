<script lang="ts">
  import { fly, fade } from 'svelte/transition'
  import FileBadge from '../ui/FileBadge.svelte'
  import Sparkline from '../ui/Sparkline.svelte'
  import AnimNum from '../ui/AnimNum.svelte'
  import Button from '../ui/Button.svelte'
  import IconButton from '../ui/IconButton.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import Progress from '../ui/Progress.svelte'
  import SegmentMap from '../SegmentMap.svelte'
  import { api } from '../../lib/api'
  import { t, tn, queueLabel } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import { layer, trapFocus } from '../../lib/layers.svelte'
  import { dur, decel } from '../../lib/motion'
  import * as fmt from '../../lib/format'
  import { displayName, statusLabel, kindLabel, priorityLabel, etaOf, elapsedOf, isRunning } from '../../lib/itemview'
  import { act, openItem } from '../../lib/mainui.svelte'
  import type { Item } from '../../lib/types'

  let { item, drawer = false, onclose }: { item: Item | undefined; drawer?: boolean; onclose: () => void } = $props()

  let p = $derived(item ? store.progress[item.id] : undefined)
  let size = $derived(p?.size && p.size > 0 ? p.size : (item?.size ?? 0))
  let done = $derived(p?.downloaded ?? item?.downloaded ?? 0)
  let pct = $derived(item?.status === 'completed' ? 100 : fmt.percent(done, size))
  let running = $derived(item ? isRunning(item.status) : false)
  let eta = $derived(item ? etaOf(item, p) : null)
  let elapsed = $derived(item ? elapsedOf(item, p) : 0)
  let hist = $derived(item ? (store.history[item.id] ?? []) : [])
  let avg = $derived.by(() => {
    if (p?.avgSpeed && p.avgSpeed > 0) return p.avgSpeed
    const nz = hist.filter((v) => v > 0)
    return nz.length ? nz.reduce((a, b) => a + b, 0) / nz.length : 0
  })

</script>

{#snippet body()}
  <div class="head">
    <h2 class="h">{t('details')}</h2>
    <IconButton icon={drawer ? 'x' : 'panel'} label={drawer ? t('closeDrawer') : t('detailsToggle')} shortcut="Ctrl+I" onclick={onclose} />
  </div>
  {#if item}
    <div class="hero">
      <FileBadge {item} size={52} />
      <div class="nm selectable">{displayName(item)}</div>
      {#if item.title && item.fileName && item.title !== item.fileName}<div class="muted small selectable">{item.title}</div>{/if}
      <span class="tag st-{item.status}">{statusLabel(item.status)}</span>
    </div>

    {#if item.status !== 'completed'}
      <section class="card live" aria-label={t('progress')}>
        <div class="big">
          <b class="spd"><AnimNum value={running ? (p?.speed ?? 0) : 0} format={(v) => fmt.speed(v)} /></b>
          <span class="pc num">{size > 0 ? fmt.pct(pct, pct < 10 ? 1 : 0) : ''}</span>
        </div>
        <Progress value={pct} indeterminate={running && !(size > 0)} tone={item.status === 'error' ? 'err' : running ? 'accent' : 'paused'} size="md" label={displayName(item)} />
        {#if running}
          <div class="etaline">
            <span class="num">{fmt.fmtEtaLong(eta)}</span>
            {#if eta?.eta && eta.eta > 0}<span class="muted num">{fmt.fmtClock(eta.eta)}</span>{/if}
          </div>
          <Sparkline data={hist} height={44} avgLine label={t('speed')} />
        {/if}
        <dl class="mini">
          <div><dt>{t('elapsed')}</dt><dd class="num">{elapsed > 0 ? fmt.duration(elapsed) : '—'}</dd></div>
          <div><dt>{t('avgSpeed')}</dt><dd class="num">{fmt.speed(avg)}</dd></div>
          <div><dt>{t('downloaded')}</dt><dd class="num">{fmt.size(done)}</dd></div>
        </dl>
      </section>
    {/if}

    {#if item.kind === 'http' && p?.segments?.length}
      <section class="sec">
        <div class="lbl">{t('segments')} · {tn('connCount', p.conns)}</div>
        <SegmentMap segments={p.segments} {size} done={item.status === 'completed'} label={tn('connCount', p.conns)} />
      </section>
    {/if}

    <dl class="kv">
      {#if item.error}<dt>{t('error')}</dt><dd class="err selectable">{item.error}</dd>{/if}
      <dt>{t('size')}</dt><dd class="num">{size > 0 ? fmt.size(size) : '—'}</dd>
      <dt>{t('type')}</dt><dd>{kindLabel(item.kind)}{item.category ? ' · ' + item.category : ''}</dd>
      <dt>{t('priority')}</dt><dd>{priorityLabel(item.priority)}</dd>
      <dt>{t('queue')}</dt><dd>{queueLabel(item.queue)}</dd>
      <dt>{t('source')}</dt><dd class="selectable ellipsis" title={item.pageUrl || item.url}>{fmt.hostOf(item.pageUrl || item.url)}</dd>
      <dt>{t('saveTo')}</dt><dd class="selectable ellipsis" title={item.path || item.dir}>{item.path || item.dir}</dd>
      <dt>{t('lastTry')}</dt><dd class="num">{fmt.date(item.createdAt)}</dd>
      {#if item.completedAt}<dt>{t('s_completed')}</dt><dd class="num">{fmt.date(item.completedAt)}</dd>{/if}
      {#if item.status === 'completed' && elapsed > 0}<dt>{t('elapsed')}</dt><dd class="num">{fmt.duration(elapsed)}</dd>{/if}
    </dl>

    <div class="acts">
      {#if item.status === 'completed'}
        <Button variant="primary" icon="openFile" onclick={() => openItem(item)}>{t('open')}</Button>
      {:else}
        <Button variant="primary" icon="gauge" onclick={() => openItem(item)}>{t('showProgress')}</Button>
      {/if}
      <Button icon="folderOpen" onclick={() => act(() => api.openFolder(item.id))}>{t('openFolder')}</Button>
    </div>
  {:else}
    <EmptyState art="generic" compact title={t('details')} text={t('selectHint')} />
  {/if}
{/snippet}

{#if drawer}
  <!-- svelte-ignore a11y_click_events_have_key_events (Escape closes the drawer) -->
  <div class="scrim" role="presentation" onclick={onclose} transition:fade|global={{ duration: dur(167) }}></div>
  <div class="panel drawer" role="dialog" aria-modal="true" tabindex="-1" aria-label={t('details')} use:layer={onclose} use:trapFocus={{}} onkeydown={(e) => { if (e.key !== 'Tab') e.stopPropagation() }}
    transition:fly|global={{ x: 320, duration: dur(250), easing: decel, opacity: 1 }}>
    {@render body()}
  </div>
{:else}
  <aside class="panel" aria-label={t('details')} in:fly={{ x: 24, duration: dur(167), easing: decel }}>
    {@render body()}
  </aside>
{/if}

<style>
  .panel { width: 304px; flex-shrink: 0; border-left: 1px solid var(--border); background: var(--surface-0); padding: 8px 16px 16px; overflow-y: auto; display: flex; flex-direction: column; gap: 14px; }
  :global(:root[data-glass]) .panel:not(.drawer) { background: transparent; }
  .drawer { position: absolute; top: 0; right: 0; bottom: 0; z-index: 60; background: var(--surface-1); box-shadow: var(--elev-28); border-left-color: var(--border); }
  :global(:root[data-glass]) .drawer { -webkit-backdrop-filter: blur(40px); backdrop-filter: blur(40px); }
  .scrim { position: absolute; inset: 0; z-index: 59; background: var(--scrim); }
  .head { display: flex; justify-content: space-between; align-items: center; min-height: 36px; }
  .h { margin: 0; font-size: var(--fs-subtitle); font-weight: 600; }
  .hero { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 8px; padding: 4px 0; }
  .nm { font-weight: 600; font-size: var(--fs-body-lg); word-break: break-word; }
  .st-downloading { background: var(--accent-soft); color: var(--accent-text); }
  .st-completed { background: var(--ok-soft); color: var(--ok); }
  .st-error { background: var(--err-soft); color: var(--err); }
  .st-processing { background: var(--warn-soft); color: var(--warn); }
  .live { display: flex; flex-direction: column; gap: 8px; }
  .big { display: flex; align-items: baseline; justify-content: space-between; }
  .spd { color: var(--accent-text); font-size: var(--fs-subtitle); font-weight: 600; }
  .pc { color: var(--fg2); font-weight: 600; }
  .etaline { display: flex; justify-content: space-between; gap: 8px; font-size: var(--fs-caption); }
  .mini { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px; margin: 2px 0 0; }
  .mini div { display: flex; flex-direction: column; min-width: 0; }
  .mini dt { font-size: var(--fs-micro); color: var(--muted); }
  .mini dd { margin: 0; font-size: var(--fs-caption); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .sec .lbl { font-size: var(--fs-caption); color: var(--muted); margin-bottom: 6px; }
  .kv { display: grid; grid-template-columns: 96px 1fr; gap: 8px 10px; margin: 0; font-size: var(--fs-caption); }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; min-width: 0; }
  .acts { display: flex; gap: 8px; flex-wrap: wrap; }
  .acts :global(.btn) { flex: 1; }
</style>
