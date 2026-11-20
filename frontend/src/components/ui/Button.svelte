<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'
  import Icon from '../Icon.svelte'
  import ProgressRing from './ProgressRing.svelte'

  type Props = Omit<HTMLButtonAttributes, 'class'> & {
    variant?: 'default' | 'primary' | 'subtle' | 'danger' | 'link'
    size?: 'sm' | 'md' | 'lg'
    icon?: string
    iconEnd?: string
    loading?: boolean
    class?: string
    el?: HTMLButtonElement
    children?: Snippet
  }
  let {
    variant = 'default', size = 'md', icon, iconEnd, loading = false, class: cls = '', type = 'button',
    disabled, el = $bindable(), children, ...rest
  }: Props = $props()
</script>

<button bind:this={el} {type} class="btn {variant} {size} {cls}" class:primary={variant === 'primary'} class:ghost={variant === 'subtle'}
  class:link={variant === 'link'} class:danger={variant === 'danger'} disabled={disabled || loading} aria-busy={loading || undefined} {...rest}>
  {#if loading}<ProgressRing size={14} stroke={2} indeterminate label="" />{:else if icon}<Icon name={icon} size={size === 'sm' ? 14 : 16} />{/if}
  {#if children}<span class="lbl">{@render children()}</span>{/if}
  {#if iconEnd}<Icon name={iconEnd} size={14} />{/if}
</button>

<style>
  .btn { display: inline-flex; align-items: center; justify-content: center; gap: 8px; }
  .btn.sm { min-height: 28px; padding: 3px 10px; font-size: var(--fs-caption); gap: 6px; }
  .btn.lg { min-height: 40px; padding: 8px 18px; font-size: var(--fs-body-lg); }
  .lbl { overflow: hidden; text-overflow: ellipsis; }
</style>
