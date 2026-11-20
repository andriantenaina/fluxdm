<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import Button from './ui/Button.svelte'
  import Switch from './ui/Switch.svelte'
  import Skeleton from './ui/Skeleton.svelte'
  import EmptyState from './ui/EmptyState.svelte'
  import { api, errMsg } from '../lib/api'
  import { t, tf } from '../lib/i18n.svelte'
  import { confirmBox } from '../lib/dialogs.svelte'
  import * as fmt from '../lib/format'
  import type { RemoteAddress, RemoteStatus } from '../lib/types'

  let st = $state<RemoteStatus | null>(null)
  let pairing = $state<{ code: string; addresses: RemoteAddress[] | null } | null>(null)
  let port = $state(52780)
  let error = $state('')
  let busy = $state(false)
  let pairingBusy = $state(false)

  onMount(load)

  async function load() {
    try {
      st = await api.remoteStatus()
      port = st.port || 52780
    } catch (e) { error = errMsg(e) }
  }

  async function toggle(on: boolean) {
    busy = true
    error = ''
    try {
      st = await api.remoteEnable(on, port)
      if (!on) pairing = null
    } catch (e) {
      error = errMsg(e)
      await load()
    } finally {
      busy = false
    }
  }

  async function pair() {
    error = ''
    pairingBusy = true
    try {
      pairing = await api.remotePair()
      if (!pairing.addresses?.length) error = t('rpNoAddress')
    } catch (e) { error = errMsg(e) } finally { pairingBusy = false }
  }

  async function revoke(id: string, name: string) {
    const r = await confirmBox(tf('rpRevokeConfirm', { name }), undefined, { title: t('rpRevoke'), okLabel: t('rpRevoke'), danger: true })
    if (!r.ok) return
    try {
      await api.remoteRevoke(id)
      await load()
    } catch (e) { error = errMsg(e) }
  }
</script>

{#if st}
  <div class="intro card">
    <Icon name="phone" size={24} />
    <div>
      <b>FluxDM Remote</b>
      <p class="muted small">{t('rpIntro')}</p>
    </div>
  </div>

  <div class="row top">
    <div class="grow">
      <Switch checked={st.enabled} disabled={busy} label={t('rpEnable')} onchange={toggle} />
    </div>
    <div class="field port">
      <label for="rp-port">{t('rpPort')}</label>
      <input id="rp-port" type="number" min="1024" max="65535" bind:value={port} disabled={st.enabled} />
    </div>
  </div>
  {#if st.enabled}
    <p class="state small" class:ok={st.running} class:err={!st.running} role="status">
      <Icon name={st.running ? 'checkCircle' : 'alert'} size={16} />
      {st.running ? t('rpRunning') : t('rpStopped')}
    </p>
  {/if}
  {#if error}<p class="err small selectable" role="alert">{error}</p>{/if}

  {#if st.enabled && st.running}
    <h4>{t('rpAddPhone')}</h4>
    <ol class="small muted steps">
      <li>{t('rpStep1')}</li>
      <li>{t('rpStep2')}</li>
      <li>{t('rpStep3')}</li>
    </ol>
    <Button variant="primary" icon="phone" loading={pairingBusy} onclick={pair}>{t('rpGenerate')}</Button>
    {#if pairing}
      <div class="qrs">
        {#each pairing.addresses ?? [] as a}
          <div class="qr">
            {#if a.qr}<img src={a.qr} alt={tf('rpQrAlt', { label: a.label })} width="190" height="190" />{/if}
            <b class="small">{a.label}</b>
            <span class="small selectable url">{a.url}</span>
          </div>
        {/each}
      </div>
      <p class="small muted">{t('rpCode')} <b class="selectable code">{pairing.code}</b>. {t('rpFirewall')}</p>
    {/if}

    <h4>{t('rpDevices')}</h4>
    {#if st.devices?.length}
      <table class="grid small">
        <thead><tr><th>{t('rpDevice')}</th><th>{t('rpAdded')}</th><th>{t('rpLastSeen')}</th><th><span class="sr-only">{t('rpRevoke')}</span></th></tr></thead>
        <tbody>
          {#each st.devices as d (d.id)}
            <tr>
              <td>{d.name}</td>
              <td>{fmt.date(d.createdAt * 1000)}</td>
              <td>{fmt.relative(d.lastSeen * 1000)}</td>
              <td class="r"><Button size="sm" variant="subtle" icon="trash" class="danger" onclick={() => revoke(d.id, d.name)}>{t('rpRevoke')}</Button></td>
            </tr>
          {/each}
        </tbody>
      </table>
    {:else}
      <EmptyState compact art="generic" title={t('rpNoDevices')} />
    {/if}
    <p class="small muted">{t('rpGuide')}</p>
  {/if}
{:else if error}
  <p class="err" role="alert">{error}</p>
{:else}
  <div class="sk" aria-busy="true"><Skeleton height="64px" radius="var(--r-md)" /><Skeleton width="60%" height="20px" /><Skeleton width="40%" height="20px" /></div>
{/if}

<style>
  .intro { display: flex; gap: 12px; align-items: flex-start; margin-bottom: 14px; color: var(--accent-text); }
  .intro b { color: var(--fg); }
  .intro p { margin: 4px 0 0; }
  .top { align-items: flex-end; }
  .port { width: 110px; margin-bottom: 0; }
  .state { display: flex; align-items: center; gap: 6px; margin: 8px 0; }
  h4 { margin: 20px 0 8px; font-size: var(--fs-body-lg); }
  .steps { margin: 0 0 10px; padding-left: 20px; }
  .steps li { margin: 2px 0; }
  .qrs { display: flex; gap: 12px; flex-wrap: wrap; margin: 12px 0 6px; }
  /* QR codes keep a white quiet zone in both themes */
  .qr { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 10px; background: #fff; color: #111; border-radius: var(--r-md); border: 1px solid var(--border); }
  .qr .url { color: #444; }
  .code { font-size: var(--fs-body-lg); letter-spacing: 0.05em; color: var(--fg); }
  .r { text-align: right; }
  .sk { display: flex; flex-direction: column; gap: 10px; }
</style>
