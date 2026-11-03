<script lang="ts">
  import Modal from './Modal.svelte'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'

  let { id, onclose }: { id: string; onclose: () => void } = $props()
  let it = $derived(store.items.find((x) => x.id === id))
  // svelte-ignore state_referenced_locally
  let url = $state(store.items.find((x) => x.id === id)?.url ?? '')
  // svelte-ignore state_referenced_locally
  let conns = $state<number>(store.items.find((x) => x.id === id)?.options?.connections ?? store.settings?.maxConns ?? 8)
  // svelte-ignore state_referenced_locally
  let limitKB = $state<number>(Math.round((store.items.find((x) => x.id === id)?.options?.speedLimit ?? 0) / 1024))
  // svelte-ignore state_referenced_locally
  let sync = $state<boolean>(!!store.items.find((x) => x.id === id)?.options?.sync)
  let error = $state('')

  async function save() {
    if (!it) return
    try {
      if (url.trim() && url.trim() !== it.url) await api.updateURL(id, url.trim())
      await api.setItemOptions(id, { connections: conns, speedLimit: limitKB * 1024, sync })
      onclose()
    } catch (e) { error = errMsg(e) }
  }
</script>

<Modal title={t('properties')} {onclose} width={620}>
  {#if it}
    <table class="kv">
      <tbody>
        <tr><td>{t('name')}</td><td class="selectable">{it.fileName || it.title}</td></tr>
        {#if it.title && it.fileName}<tr><td>Titre</td><td class="selectable">{it.title}</td></tr>{/if}
        <tr><td>{t('type')}</td><td>{it.kind}{it.category ? ' · ' + it.category : ''}</td></tr>
        <tr><td>{t('status')}</td><td>{t(('s_' + it.status) as 's_queued')}{it.error ? ' — ' + it.error : ''}</td></tr>
        <tr><td>{t('size')}</td><td>{fmt.size(it.size)} ({fmt.size(it.downloaded)})</td></tr>
        <tr><td>{t('saveTo')}</td><td class="selectable ellipsis" title={it.path || it.dir}>{it.path || it.dir}</td></tr>
        {#if it.pageUrl}<tr><td>Page</td><td class="selectable ellipsis">{it.pageUrl}</td></tr>{/if}
        {#if it.description}<tr><td>{t('description')}</td><td class="selectable">{it.description}</td></tr>{/if}
        <tr><td>{t('lastTry')}</td><td>{fmt.date(it.createdAt)}{it.completedAt ? ' → ' + fmt.date(it.completedAt) : ''}</td></tr>
        {#if it.headers && Object.keys(it.headers).length}
          <tr><td>En-têtes</td><td class="small muted selectable">{Object.keys(it.headers).filter((k) => k !== 'Cookie' && k !== 'Authorization').map((k) => k + ': ' + it!.headers![k]).join('\n')}{it.headers.Cookie ? '\nCookie: …' : ''}</td></tr>
        {/if}
      </tbody>
    </table>
    <div class="field"><span>{t('address')}</span><input bind:value={url} /></div>
    {#if it.kind === 'http'}
      <div class="row">
        <div class="field grow"><span>{t('connections')}</span>
          <select bind:value={conns}>{#each [1, 2, 4, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}</select></div>
        <div class="field grow"><span>{t('speedLimit')}</span><input type="number" min="0" bind:value={limitKB} /></div>
      </div>
      <label class="check"><input type="checkbox" bind:checked={sync} /> Synchroniser périodiquement (retélécharger si le fichier distant change)</label>
    {/if}
    {#if error}<p class="err">{error}</p>{/if}
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{t('cancel')}</button>
    <button class="primary" onclick={save}>{t('save')}</button>
  {/snippet}
</Modal>

<style>
  .kv { width: 100%; border-collapse: collapse; margin-bottom: 12px; table-layout: fixed; }
  .kv td { padding: 4px 0; vertical-align: top; font-size: 12.5px; white-space: pre-wrap; word-break: break-all; }
  .kv td:first-child { width: 130px; color: var(--muted); white-space: nowrap; }
  .small { font-size: 11.5px; }
  .grow { flex: 1; }
</style>
