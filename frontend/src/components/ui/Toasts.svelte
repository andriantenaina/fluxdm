<script lang="ts">
  import { flip } from 'svelte/animate'
  import { fly, fade } from 'svelte/transition'
  import Icon from '../Icon.svelte'
  import IconButton from './IconButton.svelte'
  import { toasts, dismiss, pauseToasts, resumeToasts } from '../../lib/toast.svelte'
  import { t } from '../../lib/i18n.svelte'
  import { dur, decel } from '../../lib/motion'

  const icons = { ok: 'checkCircle', err: 'xCircle', info: 'info' } as const
</script>

<!-- Polite region for info/success; errors use role=alert on the toast itself. -->
<section class="stack" aria-label={t('notifications')} aria-live="polite"
  onmouseenter={pauseToasts} onmouseleave={resumeToasts} onfocusin={pauseToasts}
  onfocusout={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node)) resumeToasts() }}>
  {#each toasts.list as ts (ts.id)}
    <div class="toast {ts.kind}" role={ts.kind === 'err' ? 'alert' : 'status'} animate:flip={{ duration: dur(250) }}
      in:fly={{ y: 16, duration: dur(250), easing: decel }} out:fade={{ duration: dur(167) }}>
      <span class="ic"><Icon name={icons[ts.kind]} size={18} /></span>
      <span class="txt selectable">{ts.text}</span>
      {#if ts.action}<button class="link act" onclick={() => { ts.action!.run(); dismiss(ts.id) }}>{ts.action.label}</button>{/if}
      <IconButton icon="x" size="sm" label={t('dismiss')} tip={false} onclick={() => dismiss(ts.id)} />
    </div>
  {/each}
</section>

<style>
  .stack { position: fixed; right: 16px; bottom: 44px; display: flex; flex-direction: column; gap: 8px; z-index: 300; width: min(400px, calc(100vw - 32px)); pointer-events: none; }
  .toast {
    pointer-events: auto; display: flex; align-items: center; gap: 10px; padding: 10px 8px 10px 12px; border-radius: var(--r-md);
    background: var(--surface-1); border: 1px solid var(--border); box-shadow: var(--elev-16);
  }
  :global(:root[data-glass]) .toast { -webkit-backdrop-filter: blur(30px); backdrop-filter: blur(30px); }
  .ic { display: flex; color: var(--accent-text); flex: none; }
  .ok .ic { color: var(--ok); }
  .err .ic { color: var(--err); }
  .err { border-color: var(--err-soft); }
  .txt { flex: 1; min-width: 0; word-break: break-word; }
  .act { font-weight: 600; }
</style>
