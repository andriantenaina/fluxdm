<script lang="ts">
  import Dialog from './ui/Dialog.svelte'
  import Button from './ui/Button.svelte'
  import Switch from './ui/Switch.svelte'
  import EmptyState from './ui/EmptyState.svelte'
  import { api, errMsg, Unsupported } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { store } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import { kindLabel, statusLabel } from '../lib/itemview'

  let { id, onclose }: { id: string; onclose: () => void } = $props()
  let it = $derived(store.items.find((x) => x.id === id))
  // svelte-ignore state_referenced_locally
  const init = store.items.find((x) => x.id === id)
  let url = $state(init?.url ?? '')
  let conns = $state<number>(init?.options?.connections ?? store.settings?.maxConns ?? 8)
  let limitKB = $state<number>(Math.round((init?.options?.speedLimit ?? 0) / 1024))
  let sync = $state<boolean>(!!init?.options?.sync)
  let priority = $state<number>(init?.priority ?? 0)
  let error = $state('')
  let saving = $state(false)

  let headerText = $derived.by(() => {
    const h = it?.headers
    if (!h) return ''
    const keys = Object.keys(h).filter((k) => k !== 'Cookie' && k !== 'Authorization')
    return keys.map((k) => k + ': ' + h[k]).join('\n') + (h.Cookie ? '\nCookie: …' : '')
  })

  async function save() {
    if (!it) return
    saving = true
    error = ''
    try {
      if (url.trim() && url.trim() !== it.url) await api.updateURL(id, url.trim())
      if (it.kind === 'http') await api.setItemOptions(id, { connections: conns, speedLimit: Math.max(0, limitKB) * 1024, sync })
      if (priority !== (it.priority ?? 0)) {
        try {
          await api.setPriority([id], priority)
        } catch (e) {
          if (e instanceof Unsupported) { error = t('unsupported'); return }
          throw e
        }
      }
      onclose()
    } catch (e) {
      error = errMsg(e)
    } finally {
      saving = false
    }
  }
</script>

<Dialog title={t('properties')} {onclose} width={620} subtitle={it ? it.fileName || it.title || '' : ''}>
  {#if it}
    <dl class="kv">
      <dt>{t('name')}</dt><dd class="selectable">{it.fileName || it.title}</dd>
      {#if it.title && it.fileName && it.title !== it.fileName}<dt>{t('title')}</dt><dd class="selectable">{it.title}</dd>{/if}
      <dt>{t('type')}</dt><dd>{kindLabel(it.kind)}{it.category ? ' · ' + it.category : ''}</dd>
      <dt>{t('status')}</dt><dd>{statusLabel(it.status)}{#if it.error}<span class="err"> — {it.error}</span>{/if}</dd>
      <dt>{t('size')}</dt><dd class="num">{fmt.size(it.size)}{it.status !== 'completed' ? ` (${fmt.size(it.downloaded)})` : ''}</dd>
      <dt>{t('saveTo')}</dt><dd class="selectable ellipsis" title={it.path || it.dir}>{it.path || it.dir}</dd>
      {#if it.pageUrl}<dt>{t('page')}</dt><dd class="selectable ellipsis" title={it.pageUrl}>{it.pageUrl}</dd>{/if}
      {#if it.description}<dt>{t('description')}</dt><dd class="selectable">{it.description}</dd>{/if}
      <dt>{t('lastTry')}</dt><dd>{fmt.date(it.createdAt)}{it.completedAt ? ' → ' + fmt.date(it.completedAt) : ''}</dd>
      {#if headerText}<dt>{t('headers')}</dt><dd class="small muted selectable pre">{headerText}</dd>{/if}
    </dl>

    <div class="field"><label for="prop-url">{t('address')}</label><input id="prop-url" type="url" bind:value={url} /></div>
    <div class="row top">
      <div class="field grow"><label for="prop-prio">{t('priority')}</label>
        <select id="prop-prio" bind:value={priority}>
          <option value={1}>{t('prioHigh')}</option><option value={0}>{t('prioNormal')}</option><option value={-1}>{t('prioLow')}</option>
        </select></div>
      {#if it.kind === 'http'}
        <div class="field grow"><label for="prop-conns">{t('connections')}</label>
          <select id="prop-conns" bind:value={conns}>{#each [1, 2, 4, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}</select></div>
        <div class="field grow"><label for="prop-limit">{t('propLimit')}</label>
          <div class="unit"><input id="prop-limit" type="number" min="0" step="64" bind:value={limitKB} /><span>{fmt.unit(1)}/s</span></div>
          <span class="hint">{t('propLimitHint')}</span></div>
      {/if}
    </div>
    {#if it.kind === 'http'}
      <Switch bind:checked={sync} label={t('propSync')} description={t('propSyncHint')} />
    {/if}
    {#if error}<p class="err" role="alert">{error}</p>{/if}
  {:else}
    <EmptyState compact art="search" title={t('propNotFound')} />
  {/if}
  {#snippet footer()}
    <Button onclick={onclose}>{t('cancel')}</Button>
    <Button variant="primary" loading={saving} disabled={!it} onclick={save}>{t('save')}</Button>
  {/snippet}
</Dialog>

<style>
  .kv { display: grid; grid-template-columns: 130px 1fr; gap: 6px 12px; margin: 0 0 16px; font-size: var(--fs-body); }
  dt { color: var(--muted); }
  dd { margin: 0; min-width: 0; word-break: break-all; }
  .pre { white-space: pre-wrap; }
  .top { align-items: flex-start; }
  .unit { display: flex; align-items: center; gap: 6px; }
  .unit span { color: var(--muted); font-size: var(--fs-caption); white-space: nowrap; }
</style>
