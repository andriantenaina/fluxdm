<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../Icon.svelte'
  import Sparkline from '../ui/Sparkline.svelte'
  import AnimNum from '../ui/AnimNum.svelte'
  import Menu, { type MenuEntry } from '../ui/Menu.svelte'
  import { api, hasMethod } from '../../lib/api'
  import { t, tn, tf } from '../../lib/i18n.svelte'
  import { store, liveSize } from '../../lib/state.svelte'
  import * as fmt from '../../lib/format'
  import { setLimit, customLimit } from '../../lib/mainui.svelte'

  let totalSpeed = $derived(store.items.reduce((s, it) => s + ((it.status === 'downloading' || it.status === 'processing') ? (store.progress[it.id]?.speed || 0) : 0), 0))

  // Client-side totals, used when the backend has no Totals() method.
  let local = $derived.by(() => {
    let active = 0, queued = 0, remaining = 0, unknown = false
    for (const it of store.items) {
      const run = it.status === 'downloading' || it.status === 'processing'
      if (run) active++
      else if (it.status === 'queued') queued++
      else continue
      const { done, size } = liveSize(it)
      if (size > 0) remaining += Math.max(0, size - done)
      else if (run) unknown = true
    }
    const eta = !unknown && totalSpeed > 0 && remaining > 0 ? remaining / totalSpeed : -1
    return { speed: totalSpeed, active, queued, remaining, eta }
  })
  let totals = $derived(store.totals ?? local)

  onMount(() => {
    let alive = true
    const poll = async () => {
      if (!alive || !hasMethod('Totals') || document.hidden) return
      try { store.totals = await api.totals() } catch { store.totals = null }
    }
    poll()
    const iv = setInterval(poll, 1000)
    return () => { alive = false; clearInterval(iv) }
  })

  let limit = $derived(store.settings?.speedLimit ?? 0)
  let limBtn: HTMLButtonElement | undefined = $state()
  let open = $state(false)
  const LIMITS = [0, 256, 512, 1024, 2048, 5120, 10240, 25600]
  let items = $derived<MenuEntry[]>([
    ...LIMITS.map((l): MenuEntry => ({ label: l === 0 ? t('unlimited') : fmt.speed(l * 1024), checked: limit === l * 1024, onselect: () => setLimit(l) })),
    { kind: 'separator' },
    { label: t('customLimit'), icon: 'tune', onselect: customLimit },
  ])
</script>

<footer class="status">
  <span class="spd" title={t('totalSpeed')}>
    <Icon name="download" size={14} />
    <b><AnimNum value={totalSpeed} format={(v) => fmt.speed(v)} /></b>
  </span>
  {#if totalSpeed > 0}<div class="spark"><Sparkline data={store.totalHistory} height={20} label={t('totalSpeed')} /></div>{/if}
  {#if totals.active > 0 || totals.queued > 0}
    <span class="sep"></span>
    <span class="num">{tn('activeCount', totals.active)}</span>
    {#if totals.queued > 0}<span class="num">{tn('queuedCount', totals.queued)}</span>{/if}
    {#if totals.eta === -2 || totals.eta > 0}
      <span class="eta num" title={t('queueEta')}><Icon name="clock" size={14} />
        {t('queueEta')} : {fmt.fmtEta({ eta: totals.eta, speed: totals.speed })}
        {#if totals.eta > 0}<span class="muted">· {fmt.fmtClock(totals.eta)}</span>{/if}
      </span>
    {/if}
  {/if}
  <span class="grow"></span>
  <button bind:this={limBtn} class="ghost lim" class:warn={limit > 0} aria-haspopup="menu" aria-expanded={open} onclick={() => (open = !open)}>
    <Icon name="gauge" size={14} />{limit > 0 ? tf('limitOn', { v: fmt.speed(limit) }) : `${t('speedLimiter')} : ${t('unlimited')}`}
  </button>
</footer>

{#if open && limBtn}<Menu items={items} anchor={limBtn} placement="top-end" label={t('speedLimiter')} minWidth={180} onclose={() => (open = false)} />{/if}

<style>
  .status { display: flex; align-items: center; gap: 14px; padding: 0 8px 0 16px; height: 32px; font-size: var(--fs-caption); color: var(--fg2); border-top: 1px solid var(--border); flex: none; }
  .spd { display: flex; align-items: center; gap: 6px; color: var(--fg2); }
  .spd b { color: var(--fg); min-width: 72px; font-weight: 600; }
  .spark { width: 120px; }
  .sep { width: 1px; height: 14px; background: var(--border); }
  .eta { display: flex; align-items: center; gap: 6px; }
  .lim { display: flex; align-items: center; gap: 6px; min-height: 24px; padding: 2px 8px; font-size: var(--fs-caption); color: var(--fg2); }
  .lim.warn { color: var(--warn); }
  @media (max-width: 760px) { .spark, .eta { display: none; } }
</style>
