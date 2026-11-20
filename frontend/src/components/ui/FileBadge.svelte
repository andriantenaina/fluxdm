<script lang="ts">
  import type { Item } from '../../lib/types'
  import { fileTypeOf } from '../../lib/filetype'
  import { store } from '../../lib/state.svelte'

  let { item, size = 36 }: { item: Item; size?: number } = $props()
  let ft = $derived(fileTypeOf(item, store.settings))
  let label = $derived(ft.ext.length > 4 ? ft.ext.slice(0, 4) : ft.ext)
</script>

<div class="badge" style:width="{size}px" style:height="{size}px" style:--c={ft.color} style:font-size="{Math.max(9, Math.round(size * 0.26))}px"
  style:border-radius="{Math.max(4, Math.round(size * 0.22))}px" aria-hidden="true">
  <span>{label.toUpperCase()}</span>
</div>

<style>
  .badge {
    flex: none;
    background: var(--c);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    letter-spacing: 0.02em;
    box-shadow: inset 0 -2px 0 rgba(0, 0, 0, 0.14);
    position: relative;
    overflow: hidden;
  }
  .badge::after {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    width: 30%;
    height: 30%;
    background: rgba(255, 255, 255, 0.22);
    border-bottom-left-radius: 4px;
  }
</style>
