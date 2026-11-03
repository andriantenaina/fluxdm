<script lang="ts">
  import { Window } from '@wailsio/runtime'
  import { api } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'

  let { id }: { id: string } = $props()
  let it = $derived(store.items.find((x) => x.id === id))
</script>

<div class="dialog">
  <h1>✓ {t('doneTitle')}</h1>
  {#if it}
    <div class="body">
      <p class="name ellipsis" title={it.path}><b>{it.fileName || it.title}</b></p>
      <p class="muted small ellipsis selectable">{it.path}</p>
      <p class="small">{t('size')} : {fmt.size(it.size)}</p>
    </div>
    <div class="actions">
      <button onclick={() => { api.openFolder(id); Window.Close() }}>{t('openFolder')}</button>
      <button class="primary" onclick={() => { api.openFile(id); Window.Close() }}>{t('openWith')}</button>
      <button onclick={() => Window.Close()}>{t('closeAfter')}</button>
    </div>
  {/if}
</div>

<style>
  h1 { color: var(--ok); }
  .small { font-size: 12px; }
  p { margin: 4px 0; }
</style>
