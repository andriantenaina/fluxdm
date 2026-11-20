<script lang="ts">
  // Accessible confirm dialog for the phone (replaces window.confirm).
  import Dialog from '../components/ui/Dialog.svelte'

  let {
    title, message, okLabel, cancelLabel, danger = false, onresult,
  }: {
    title: string
    message: string
    okLabel: string
    cancelLabel: string
    danger?: boolean
    onresult: (ok: boolean) => void
  } = $props()
</script>

<Dialog {title} role="alertdialog" width={420} onclose={() => onresult(false)} initial="[data-cancel]">
  <p class="msg selectable">{message}</p>
  {#snippet footer()}
    <button type="button" class="rbtn" data-cancel onclick={() => onresult(false)}>{cancelLabel}</button>
    <button type="button" class="rbtn" class:danger class:primary={!danger} onclick={() => onresult(true)}>{okLabel}</button>
  {/snippet}
</Dialog>

<style>
  .msg { margin: 0; font-size: var(--fs-body-lg); word-break: break-word; }
  .rbtn { flex: 1; min-height: 44px; border-radius: var(--r-lg); font-size: 16px; }
  .rbtn.danger { background: var(--err); color: #fff; border-color: transparent; font-weight: 600; }
  :global(:root[data-theme='dark']) .rbtn.danger { color: #000; }
</style>
