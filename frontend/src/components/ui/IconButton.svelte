<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements'
  import Icon from '../Icon.svelte'
  import { tooltip } from '../../lib/tooltip'

  type Props = Omit<HTMLButtonAttributes, 'class' | 'aria-label'> & {
    /** Accessible name, also shown as tooltip (required). */
    label: string
    icon: string
    shortcut?: string
    size?: 'sm' | 'md' | 'lg'
    iconSize?: number
    variant?: 'subtle' | 'default' | 'primary'
    /** Toggle buttons: sets aria-pressed and the "on" look. */
    pressed?: boolean
    tone?: '' | 'warn' | 'accent' | 'danger'
    tip?: boolean
    class?: string
    el?: HTMLButtonElement
  }
  let {
    label, icon, shortcut = '', size = 'md', iconSize, variant = 'subtle', pressed, tone = '', tip = true,
    class: cls = '', type = 'button', el = $bindable(), ...rest
  }: Props = $props()
  let px = $derived(iconSize ?? (size === 'sm' ? 14 : size === 'lg' ? 20 : 16))
</script>

<button bind:this={el} {type} class="ib {size} {tone} {cls}" class:ghost={variant === 'subtle'} class:primary={variant === 'primary'}
  class:on={pressed} aria-label={label} aria-pressed={pressed} use:tooltip={tip ? { text: label, shortcut } : null} {...rest}>
  <Icon name={icon} size={px} />
</button>

<style>
  .ib { display: inline-flex; align-items: center; justify-content: center; padding: 0; width: 32px; height: 32px; min-height: 0; color: var(--fg2); flex: none; }
  .ib.sm { width: 26px; height: 26px; }
  .ib.lg { width: 40px; height: 40px; }
  .ib.primary { color: var(--accent-fg); }
  .ib.on { color: var(--accent-text); background: var(--accent-soft); }
  .ib.warn { color: var(--warn); }
  .ib.accent { color: var(--accent-text); }
  .ib.danger { color: var(--err); }
</style>
