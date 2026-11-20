<script lang="ts">
  import Icon from '../Icon.svelte'
  import { t, queueLabel } from '../../lib/i18n.svelte'
  import { store } from '../../lib/state.svelte'
  import { KIND_COLOR, categoryKind } from '../../lib/filetype'
  import type { Filter } from '../../lib/mainui.svelte'

  let { filter, onfilter }: { filter: Filter; onfilter: (f: Filter) => void } = $props()

  // Every count in a single pass over the list.
  let counts = $derived.by(() => {
    const c = { all: 0, downloading: 0, unfinished: 0, completed: 0, error: 0, cat: {} as Record<string, number>, queue: {} as Record<string, number> }
    for (const it of store.items) {
      c.all++
      const s = it.status
      if (s !== 'completed') c.unfinished++
      if (s === 'downloading' || s === 'processing') c.downloading++
      else if (s === 'completed') c.completed++
      else if (s === 'error') c.error++
      const cat = it.category || ''
      c.cat[cat] = (c.cat[cat] ?? 0) + 1
      c.queue[it.queue] = (c.queue[it.queue] ?? 0) + 1
    }
    return c
  })

  const isActive = (f: Filter) => filter.type === f.type && filter.value === f.value

  const statuses = [
    { v: 'downloading', icon: 'download', key: 'active' },
    { v: 'unfinished', icon: 'clock', key: 'unfinished' },
    { v: 'completed', icon: 'checkCircle', key: 'finished' },
    { v: 'error', icon: 'alert', key: 'errors' },
  ] as const
  const statusCount = (v: string) => (counts as unknown as Record<string, number>)[v] ?? 0
</script>

<nav class="side" aria-label={t('navigation')}>
  {#if !store.frameless}
    <div class="brand drag">
      <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="var(--accent)" /><path d="M12 5.5v8.5M8 10.5l4 4 4-4M7 18h10" stroke="var(--accent-fg)" stroke-width="2.2" fill="none" stroke-linecap="round" stroke-linejoin="round" /></svg>
      <b>FluxDM</b>
    </div>
  {/if}

  <ul role="list">
    <li>
      <button class="nav" aria-current={isActive({ type: 'all', value: '' }) ? 'page' : undefined} onclick={() => onfilter({ type: 'all', value: '' })}>
        <Icon name="list" /><span class="grow ellipsis">{t('allShort')}</span><span class="n">{counts.all}</span>
      </button>
    </li>
    {#each statuses as s}
      {@const f = { type: 'status' as const, value: s.v }}
      {@const n = statusCount(s.v)}
      <li>
        <button class="nav" aria-current={isActive(f) ? 'page' : undefined} onclick={() => onfilter(f)}>
          <Icon name={s.icon} /><span class="grow ellipsis">{t(s.key)}</span>
          {#if n}<span class="n" class:hot={s.v === 'downloading'} class:bad={s.v === 'error'}>{n}</span>{/if}
        </button>
      </li>
    {/each}
  </ul>

  <h2 class="h" id="side-cats">{t('categories')}</h2>
  <ul role="list" aria-labelledby="side-cats">
    {#each store.settings?.categories ?? [] as c (c.name)}
      {@const f = { type: 'cat' as const, value: c.name }}
      {@const n = counts.cat[c.name] ?? 0}
      <li>
        <button class="nav" aria-current={isActive(f) ? 'page' : undefined} onclick={() => onfilter(f)}>
          <span class="dot" style:background={KIND_COLOR[categoryKind(c)]} aria-hidden="true"></span>
          <span class="grow ellipsis">{c.name}</span>{#if n}<span class="n">{n}</span>{/if}
        </button>
      </li>
    {/each}
    <li>
      <button class="nav" aria-current={isActive({ type: 'cat', value: '' }) ? 'page' : undefined} onclick={() => onfilter({ type: 'cat', value: '' })}>
        <span class="dot" style:background="var(--c-other)" aria-hidden="true"></span><span class="grow ellipsis">{t('general')}</span>
        {#if counts.cat['']}<span class="n">{counts.cat['']}</span>{/if}
      </button>
    </li>
  </ul>

  <h2 class="h" id="side-queues">{t('queues')}</h2>
  <ul role="list" aria-labelledby="side-queues">
    {#each store.queues as q (q.name)}
      {@const f = { type: 'queue' as const, value: q.name }}
      {@const n = counts.queue[q.name] ?? 0}
      <li>
        <button class="nav" aria-current={isActive(f) ? 'page' : undefined} onclick={() => onfilter(f)}>
          <Icon name="queue" /><span class="grow ellipsis">{queueLabel(q.name)}</span>
          {#if q.running && q.name !== 'main'}<span class="live"><span class="sr-only">{t('active')}</span></span>{/if}
          {#if n}<span class="n">{n}</span>{/if}
        </button>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .side { width: 232px; flex-shrink: 0; padding: 4px 8px 14px; overflow-y: auto; display: flex; flex-direction: column; }
  :global(:root[data-mac-inset]) .brand { padding-top: 30px; }
  .brand { display: flex; align-items: center; gap: 10px; padding: 12px 10px 14px; font-size: var(--fs-body-lg); }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .h { font-size: var(--fs-caption); font-weight: 600; color: var(--muted); margin: 18px 12px 6px; }
  .nav {
    display: flex; align-items: center; gap: 12px; width: 100%; text-align: left; background: transparent; border: 1px solid transparent;
    min-height: 36px; padding: 0 10px 0 12px; border-radius: var(--r-sm); color: var(--fg); position: relative;
  }
  :global(:root[data-theme='dark']) .nav:not([aria-current]) { background: transparent; }
  .nav:hover:not([aria-current]) { background: var(--surface-3); }
  .nav[aria-current] { background: var(--surface-3); font-weight: 600; }
  :global(:root[data-theme='dark']) .nav[aria-current] { background: rgba(255, 255, 255, 0.08); }
  .nav[aria-current]::before {
    content: ''; position: absolute; left: 0; top: 50%; height: 16px; margin-top: -8px; width: 3px; border-radius: 2px; background: var(--accent);
    animation: indicator var(--dur) var(--ease-decel);
  }
  @keyframes indicator { from { transform: scaleY(0); } to { transform: scaleY(1); } }
  .nav :global(svg) { color: var(--fg2); }
  .nav[aria-current] :global(svg) { color: var(--accent-text); }
  .n { font-size: var(--fs-micro); color: var(--fg2); background: var(--surface-3); border-radius: 9px; padding: 0 7px; min-width: 22px; text-align: center; font-weight: 600; line-height: 18px; font-variant-numeric: tabular-nums; }
  .nav[aria-current] .n { background: var(--surface-1); }
  .n.hot { background: var(--accent); color: var(--accent-fg); }
  .n.bad { background: var(--err-soft); color: var(--err); }
  .dot { width: 10px; height: 10px; border-radius: 3px; flex: none; margin: 0 3px; }
  .live { width: 8px; height: 8px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px var(--ok-soft); flex: none; }
</style>
