<script lang="ts">
  import type { Snippet } from 'svelte'
  let {
    title,
    onclose,
    width = 640,
    children,
    footer,
  }: { title: string; onclose: () => void; width?: number; children: Snippet; footer?: Snippet } = $props()

  function key(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={key} />

<div class="backdrop" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div class="modal" style:width="min({width}px, 96vw)" role="dialog" aria-modal="true" aria-label={title}>
    <header>
      <h2>{title}</h2>
      <button class="link x" onclick={onclose} aria-label="Fermer">✕</button>
    </header>
    <div class="content">{@render children()}</div>
    {#if footer}<footer>{@render footer()}</footer>{/if}
  </div>
</div>

<style>
  .backdrop { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.35); display: flex; align-items: center; justify-content: center; z-index: 50; }
  .modal { background: var(--bg); border: 1px solid var(--border); border-radius: 10px; box-shadow: var(--shadow); max-height: 92vh; display: flex; flex-direction: column; }
  header { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid var(--border); }
  h2 { margin: 0; font-size: 15px; font-weight: 600; }
  .x { font-size: 16px; color: var(--muted); }
  .content { padding: 14px 16px; overflow: auto; min-height: 0; flex: 1; }
  footer { display: flex; justify-content: flex-end; gap: 8px; padding: 10px 16px; border-top: 1px solid var(--border); }
</style>
