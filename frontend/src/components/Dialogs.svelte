<script lang="ts">
  import Dialog from './ui/Dialog.svelte'
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
</script>

{#if dlg.current}
  {@const c = dlg.current}
  <Dialog title={c.opts.title || (c.kind === 'alert' ? t('error') : c.kind === 'prompt' ? c.message : t('confirm'))} role="alertdialog" width={440}
    onclose={() => done(false)} dismissOnBackdrop={false} initial={c.kind === 'prompt' ? 'input' : c.opts.danger ? '[data-cancel]' : '[data-ok]'}>
    <form id="dlg-form" onsubmit={(e) => { e.preventDefault(); done(true) }}>
      {#if c.kind !== 'prompt' || c.opts.title}<p class="selectable msg">{c.message}</p>{/if}
      {#if c.kind === 'prompt'}
        <input type={c.opts.inputType ?? 'text'} bind:value={c.value} placeholder={c.opts.placeholder} aria-label={c.message} />
      {/if}
      {#if c.kind === 'confirm' && c.check}
        <label class="check"><input type="checkbox" bind:checked={c.checked} /> {c.check}</label>
      {/if}
    </form>
    {#snippet footer()}
      {#if c.kind !== 'alert'}<button type="button" data-cancel onclick={() => done(false)}>{c.opts.cancelLabel || t('cancel')}</button>{/if}
      <button type="submit" form="dlg-form" data-ok class:primary={!c.opts.danger} class:danger-fill={c.opts.danger}>{c.opts.okLabel || t('ok')}</button>
    {/snippet}
  </Dialog>
{/if}

<style>
  form { display: flex; flex-direction: column; gap: 12px; }
  .msg { margin: 0; white-space: pre-wrap; word-break: break-word; }
  button[data-ok], button[data-cancel] { min-width: 96px; }
  .danger-fill { background: var(--err) !important; color: #fff !important; border-color: transparent !important; font-weight: 600; }
  :global(:root[data-theme='dark']) .danger-fill { color: #000 !important; }
</style>
