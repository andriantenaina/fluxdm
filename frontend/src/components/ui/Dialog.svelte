<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fade, scale } from 'svelte/transition'
  import IconButton from './IconButton.svelte'
  import { layer, trapFocus } from '../../lib/layers.svelte'
  import { dur, decel } from '../../lib/motion'
  import { t } from '../../lib/i18n.svelte'

  let {
    title, onclose, width = 560, role = 'dialog', closeable = true, dismissOnBackdrop = true, initial, subtitle = '',
    children, footer, header, padded = true, height,
  }: {
    title: string
    onclose: () => void
    width?: number
    role?: 'dialog' | 'alertdialog'
    closeable?: boolean
    dismissOnBackdrop?: boolean
    /** CSS selector of the element focused first (default: [data-autofocus], else first focusable). */
    initial?: string
    subtitle?: string
    children: Snippet
    footer?: Snippet
    /** Extra header content (right of the title). */
    header?: Snippet
    padded?: boolean
    height?: number
  } = $props()

  const uid = 'dlg-' + Math.random().toString(36).slice(2, 8)

  // Keys pressed inside a dialog never reach the window-level shortcuts of the view below.
  function stopKeys(e: KeyboardEvent) {
    if (e.key !== 'Tab' && e.key !== 'Escape') e.stopPropagation()
  }
</script>

<div class="backdrop" role="presentation" transition:fade|global={{ duration: dur(167) }}
  onmousedown={(e) => { if (dismissOnBackdrop && e.target === e.currentTarget) onclose() }}>
  <div class="dlg" style:width="min({width}px, calc(100vw - 32px))" style:height={height ? `min(${height}px, calc(100vh - 32px))` : undefined}
    {role} aria-modal="true" aria-labelledby="{uid}-t" aria-describedby={subtitle ? `${uid}-s` : undefined} tabindex="-1"
    use:layer={onclose} use:trapFocus={{ initial }} onkeydown={stopKeys}
    transition:scale|global={{ duration: dur(250), start: 0.96, opacity: 0, easing: decel }}>
    <header>
      <div class="titles">
        <h2 id="{uid}-t">{title}</h2>
        {#if subtitle}<p id="{uid}-s" class="sub">{subtitle}</p>{/if}
      </div>
      {#if header}{@render header()}{/if}
      {#if closeable}<IconButton icon="x" label={t('close')} onclick={onclose} tip={false} />{/if}
    </header>
    <div class="content" class:padded>{@render children()}</div>
    {#if footer}<footer>{@render footer()}</footer>{/if}
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; background: var(--scrim); display: flex; align-items: center; justify-content: center; z-index: 100; padding: 16px; }
  .dlg {
    background: var(--surface-1); border: 1px solid var(--border); border-radius: var(--r-lg); box-shadow: var(--elev-28);
    max-height: calc(100vh - 32px); display: flex; flex-direction: column; overflow: hidden; outline: none;
  }
  :global(:root[data-glass]) .dlg { background: var(--surface-1); -webkit-backdrop-filter: blur(40px) saturate(1.4); backdrop-filter: blur(40px) saturate(1.4); }
  :global(:root[data-glass][data-theme='dark']) .dlg { background: rgba(44, 44, 44, 0.92); }
  :global(:root[data-glass]:not([data-theme='dark'])) .dlg { background: rgba(252, 252, 252, 0.94); }
  header { display: flex; align-items: flex-start; gap: 8px; padding: 18px 16px 6px 24px; }
  .titles { flex: 1; min-width: 0; }
  h2 { margin: 0; font-size: var(--fs-title); font-weight: 600; line-height: 1.3; }
  .sub { margin: 4px 0 0; color: var(--muted); }
  .content { overflow: auto; min-height: 0; flex: 1; }
  .content.padded { padding: 10px 24px 20px; }
  footer { display: flex; justify-content: flex-end; align-items: center; gap: 8px; padding: 16px 24px; background: var(--surface-2); border-top: 1px solid var(--border); }
  :global(:root[data-glass]) footer { background: rgba(127, 127, 127, 0.06); }
</style>
