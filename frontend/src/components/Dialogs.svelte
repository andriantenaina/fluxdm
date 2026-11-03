<script lang="ts">
  import { dlg } from '../lib/dialogs.svelte'
  import { t } from '../lib/i18n.svelte'

  function done(ok: boolean) {
    const c = dlg.current
    if (!c) return
    dlg.current = null
    if (c.kind === 'confirm') c.resolve({ ok, checked: c.checked })
    else if (c.kind === 'prompt') c.resolve(ok ? c.value : null)
    else c.resolve(undefined)
  }
  function key(e: KeyboardEvent) {
    if (!dlg.current) return
    if (e.key === 'Escape') done(false)
    if (e.key === 'Enter') done(true)
  }
</script>

<svelte:window onkeydown={key} />

{#if dlg.current}
  <div class="backdrop">
    <div class="box" role="alertdialog" aria-modal="true">
      <p class="selectable">{dlg.current.message}</p>
      {#if dlg.current.kind === 'prompt'}
        <!-- svelte-ignore a11y_autofocus -->
        <input bind:value={dlg.current.value} autofocus />
      {/if}
      {#if dlg.current.kind === 'confirm' && dlg.current.check}
        <label class="check"><input type="checkbox" bind:checked={dlg.current.checked} /> {dlg.current.check}</label>
      {/if}
      <div class="actions">
        {#if dlg.current.kind !== 'alert'}<button onclick={() => done(false)}>{t('cancel')}</button>{/if}
        <button class="primary" onclick={() => done(true)}>{t('ok')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.35); display: flex; align-items: center; justify-content: center; z-index: 100; }
  .box { background: var(--bg); border: 1px solid var(--border); border-radius: 10px; box-shadow: var(--shadow); padding: 16px; width: min(440px, 92vw); display: flex; flex-direction: column; gap: 12px; }
  p { margin: 0; white-space: pre-wrap; word-break: break-word; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
