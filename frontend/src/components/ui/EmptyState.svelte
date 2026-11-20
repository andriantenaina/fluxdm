<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    art = 'downloads', title, text = '', compact = false, children,
  }: {
    art?: 'downloads' | 'search' | 'done' | 'active' | 'error' | 'folder' | 'queue' | 'generic'
    title: string
    text?: string
    compact?: boolean
    children?: Snippet
  } = $props()
</script>

<div class="empty" class:compact>
  <svg class="art" viewBox="0 0 120 96" width={compact ? 84 : 120} height={compact ? 67 : 96} aria-hidden="true">
    <ellipse cx="60" cy="86" rx="38" ry="5" class="shadow" />
    {#if art === 'search'}
      <rect x="22" y="14" width="64" height="58" rx="8" class="paper" />
      <rect x="32" y="26" width="36" height="5" rx="2.5" class="line" />
      <rect x="32" y="37" width="44" height="5" rx="2.5" class="line" />
      <rect x="32" y="48" width="28" height="5" rx="2.5" class="line" />
      <circle cx="78" cy="56" r="15" class="glass" />
      <path d="M89 67l10 10" class="stroke-accent" />
    {:else if art === 'done'}
      <rect x="24" y="16" width="72" height="56" rx="10" class="paper" />
      <circle cx="60" cy="44" r="17" class="ok-fill" />
      <path d="M52 44.5l5.5 5.5L69 38.5" class="stroke-ok" />
    {:else if art === 'active'}
      <rect x="18" y="22" width="84" height="16" rx="8" class="paper" />
      <rect x="18" y="22" width="52" height="16" rx="8" class="accent-fill" />
      <rect x="18" y="46" width="84" height="16" rx="8" class="paper" />
      <rect x="18" y="46" width="30" height="16" rx="8" class="accent-soft" />
      <path d="M96 66v12M90 72l6 6 6-6" class="stroke-accent" />
    {:else if art === 'error'}
      <rect x="24" y="16" width="72" height="56" rx="10" class="paper" />
      <path d="M60 30l16 28H44z" class="ok-soft" />
      <path d="M50 44.5l7 7L71 37" class="stroke-ok" />
    {:else if art === 'folder' || art === 'queue'}
      <path d="M20 28a6 6 0 016-6h20l8 8h40a6 6 0 016 6v34a6 6 0 01-6 6H26a6 6 0 01-6-6z" class="paper" />
      <path d="M20 40h80" class="stroke-line" />
      {#if art === 'queue'}
        <rect x="36" y="50" width="48" height="5" rx="2.5" class="line" />
        <rect x="36" y="60" width="30" height="5" rx="2.5" class="line" />
      {/if}
    {:else}
      <rect x="30" y="10" width="60" height="66" rx="10" class="paper" />
      <circle cx="60" cy="40" r="18" class="accent-soft" />
      <path d="M60 30v18M52.5 41.5L60 49l7.5-7.5M50 56h20" class="stroke-accent" />
    {/if}
  </svg>
  <h3>{title}</h3>
  {#if text}<p>{text}</p>{/if}
  {#if children}<div class="acts">{@render children()}</div>{/if}
</div>

<style>
  .empty { display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; gap: 6px; padding: 32px 24px; height: 100%; animation: fade var(--dur) var(--ease-decel); }
  .empty.compact { padding: 16px; height: auto; }
  .art { margin-bottom: 10px; }
  h3 { margin: 0; font-size: var(--fs-subtitle); font-weight: 600; }
  p { margin: 0; color: var(--muted); max-width: 380px; }
  .acts { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; justify-content: center; }
  .shadow { fill: var(--fg); opacity: 0.06; }
  .paper { fill: var(--surface-1); stroke: var(--border-strong); stroke-width: 1.5; }
  .line { fill: var(--surface-3); }
  .glass { fill: var(--surface-1); stroke: var(--accent); stroke-width: 3; }
  .accent-fill { fill: var(--accent); }
  .accent-soft { fill: var(--accent-soft); }
  .ok-fill { fill: var(--ok-soft); }
  .ok-soft { fill: var(--ok-soft); }
  .stroke-accent, .stroke-ok, .stroke-line { fill: none; stroke-width: 3.5; stroke-linecap: round; stroke-linejoin: round; }
  .stroke-accent { stroke: var(--accent); }
  .stroke-ok { stroke: var(--ok); }
  .stroke-line { stroke: var(--border-strong); stroke-width: 1.5; }
</style>
