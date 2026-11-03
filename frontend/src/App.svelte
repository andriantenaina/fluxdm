<script lang="ts">
  import { onMount } from 'svelte'
  import { initStore, route } from './lib/state.svelte'
  import Main from './views/Main.svelte'
  import AddDialog from './views/AddDialog.svelte'
  import BatchDialog from './views/BatchDialog.svelte'
  import VideoDialog from './views/VideoDialog.svelte'
  import ProgressWindow from './views/ProgressWindow.svelte'
  import DoneDialog from './views/DoneDialog.svelte'

  let r = $state(route())
  let ready = $state(false)

  onMount(() => {
    const onHash = () => (r = route())
    window.addEventListener('hashchange', onHash)
    // dialogs do not need the full list
    initStore({ items: r.name === 'main' || r.name === 'progress' || r.name === 'done' }).then(() => (ready = true))
    // block the default browser context menu except in text fields
    const ctx = (e: MouseEvent) => {
      const t = e.target as HTMLElement
      if (!t.closest('input, textarea, [data-native-menu]')) e.preventDefault()
    }
    window.addEventListener('contextmenu', ctx)
    return () => {
      window.removeEventListener('hashchange', onHash)
      window.removeEventListener('contextmenu', ctx)
    }
  })
</script>

{#if ready}
  {#if r.name === 'add'}
    <AddDialog token={r.arg} />
  {:else if r.name === 'batch'}
    <BatchDialog token={r.arg} />
  {:else if r.name === 'video'}
    <VideoDialog token={r.arg} />
  {:else if r.name === 'progress'}
    <ProgressWindow id={r.arg} />
  {:else if r.name === 'done'}
    <DoneDialog id={r.arg} />
  {:else}
    <Main />
  {/if}
{/if}
