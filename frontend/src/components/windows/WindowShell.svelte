<script lang="ts">
  // Frame of a separate window: custom title bar when the window is frameless,
  // then the content filling the rest of the window.
  import type { Snippet } from 'svelte'
  import TitleBar from '../TitleBar.svelte'
  import { store } from '../../lib/state.svelte'

  let {
    title, subtitle = '', onclose, minimisable = true, maximisable = false, children,
  }: {
    title: string
    subtitle?: string
    onclose?: () => void
    minimisable?: boolean
    maximisable?: boolean
    children: Snippet
  } = $props()
</script>

<div class="window">
  {#if store.frameless}<TitleBar compact {title} {subtitle} {onclose} {minimisable} {maximisable} />{/if}
  {@render children()}
</div>
