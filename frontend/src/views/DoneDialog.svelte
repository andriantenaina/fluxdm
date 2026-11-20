<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { api } from '../lib/api'
  import { t, tf } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { displayName } from '../lib/itemview'
  import { layers } from '../lib/layers.svelte'
  import WindowShell from '../components/windows/WindowShell.svelte'
  import SuccessCheck from '../components/windows/SuccessCheck.svelte'
  import Button from '../components/ui/Button.svelte'
  import Skeleton from '../components/ui/Skeleton.svelte'
  import EmptyState from '../components/ui/EmptyState.svelte'
  import FileBadge from '../components/ui/FileBadge.svelte'

  let { id }: { id: string } = $props()
  let it = $derived(store.items.find((x) => x.id === id))
  let notFound = $state(false)

  onMount(() => {
    const tm = setTimeout(async () => {
      if (store.items.some((x) => x.id === id)) return
      try {
        const x = await api.get(id)
        if (x) store.items.push(x)
        else notFound = true
      } catch { notFound = true }
    }, 1200)
    return () => clearTimeout(tm)
  })

  function close() { Window.Close() }
  function onKey(e: KeyboardEvent) {
    if (layers.depth > 0) return
    if (e.key === 'Escape') close()
  }
</script>

<svelte:window onkeydown={onKey} />

<WindowShell title={t('doneTitle')}>
  <div class="dialog">
    {#if it}
      <div class="body hero">
        <SuccessCheck size={56} label={t('doneTitle')} />
        <div class="grow txt">
          <h1>{t('doneTitle')}</h1>
          <div class="file">
            <FileBadge item={it} size={28} />
            <b class="ellipsis selectable" title={it.path}>{displayName(it)}</b>
          </div>
          <p class="muted small ellipsis selectable" title={it.path}>{it.path || it.dir}</p>
          <p class="small muted">{fmt.size(it.size)}{it.elapsed ? ' · ' + tf('pwIn', { d: fmt.duration(it.elapsed) }) : ''}</p>
        </div>
      </div>
      <div class="actions">
        <Button onclick={close}>{t('closeAfter')}</Button>
        <Button icon="folder" onclick={() => { api.openFolder(id); close() }}>{t('openFolder')}</Button>
        <!-- svelte-ignore a11y_autofocus -->
        <Button variant="primary" icon="openFile" autofocus onclick={() => { api.openFile(id); close() }}>{t('openWith')}</Button>
      </div>
    {:else if notFound}
      <div class="body center">
        <EmptyState compact art="search" title={t('pwNotFound')} text={t('pwNotFoundHint')} />
      </div>
      <div class="actions"><Button onclick={close}>{t('close')}</Button></div>
    {:else}
      <div class="body hero" aria-busy="true" aria-label={t('loading')}>
        <Skeleton width="56px" height="56px" circle />
        <div class="grow txt"><Skeleton width="60%" height="18px" /><Skeleton height="12px" /><Skeleton width="40%" height="12px" /></div>
      </div>
    {/if}
  </div>
</WindowShell>

<style>
  .hero { display: flex; gap: 16px; align-items: flex-start; padding-top: 4px; }
  .txt { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  h1 { color: var(--ok); font-size: var(--fs-subtitle) !important; }
  .file { display: flex; align-items: center; gap: 8px; min-width: 0; }
  p { margin: 0; }
  .center { display: flex; align-items: center; justify-content: center; }
</style>
