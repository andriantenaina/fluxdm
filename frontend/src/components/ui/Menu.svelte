<script lang="ts" module>
  export type MenuEntry =
    | {
        kind?: 'item'
        id?: string
        label: string
        icon?: string
        shortcut?: string
        danger?: boolean
        /** checkable item (menuitemcheckbox / radio look) */
        checked?: boolean
        disabled?: boolean
        hidden?: boolean
        onselect?: () => void
        children?: MenuEntry[]
      }
    | { kind: 'separator'; hidden?: boolean }
    | { kind: 'header'; label: string; hidden?: boolean }

  export type Anchor = HTMLElement | { x: number; y: number }
  export type Placement = 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end' | 'right-start'

  /** Moves a node to <body> so `position: fixed` is never trapped by a transformed ancestor. */
  export function portal(node: HTMLElement) {
    document.body.appendChild(node)
    return { destroy() { node.remove() } }
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import MenuItem from './MenuItem.svelte'
  import Menu from './Menu.svelte'
  import { pushLayer } from '../../lib/layers.svelte'

  let {
    items, anchor, onclose, placement = 'bottom-start', label = '', minWidth = 200, rootId, parentFocus, autofocus = true,
  }: {
    items: MenuEntry[]
    anchor: Anchor
    onclose: () => void
    placement?: Placement
    label?: string
    minWidth?: number
    /** internal: shared id of a menu and its submenus */
    rootId?: string
    /** internal: element to focus when a submenu closes with ArrowLeft */
    parentFocus?: HTMLElement
    /** focus the first item when opened (false for submenus opened by hover) */
    autofocus?: boolean
  } = $props()

  // svelte-ignore state_referenced_locally
  const root = rootId ?? 'm' + Math.random().toString(36).slice(2, 8)
  let el: HTMLDivElement | undefined = $state()
  let pos = $state({ x: -9999, y: -9999, ready: false, origin: 'top left' })
  let sub = $state<{ index: number; el: HTMLElement; kbd: boolean } | null>(null)
  // hidden entries removed, separators never leading, trailing or doubled
  let entries = $derived.by(() => {
    const out: MenuEntry[] = []
    for (const e of items) {
      if (e.hidden) continue
      if (e.kind === 'separator' && (!out.length || out[out.length - 1].kind === 'separator')) continue
      out.push(e)
    }
    while (out.length && out[out.length - 1].kind === 'separator') out.pop()
    return out
  })
  let typed = ''
  let typedT: ReturnType<typeof setTimeout> | undefined

  const isItem = (e: MenuEntry): e is Extract<MenuEntry, { label: string; kind?: 'item' }> => !e.kind || e.kind === 'item'

  function buttons(): HTMLElement[] {
    return el ? [...el.querySelectorAll<HTMLElement>(':scope > [role^="menuitem"]:not([aria-disabled="true"])')] : []
  }

  function place() {
    if (!el) return
    const m = el.getBoundingClientRect()
    const vw = window.innerWidth, vh = window.innerHeight, pad = 4
    let x: number, y: number
    let origin = 'top left'
    if (anchor instanceof HTMLElement) {
      const a = anchor.getBoundingClientRect()
      if (placement === 'right-start') {
        x = a.right - 2; y = a.top - 5
        if (x + m.width > vw - pad) x = a.left - m.width + 2
      } else {
        x = placement.endsWith('end') ? a.right - m.width : a.left
        y = placement.startsWith('top') ? a.top - m.height - 4 : a.bottom + 4
        if (y + m.height > vh - pad && a.top - m.height - 4 > pad) { y = a.top - m.height - 4; origin = 'bottom' }
        if (y < pad && a.bottom + 4 + m.height < vh) y = a.bottom + 4
      }
    } else {
      x = anchor.x; y = anchor.y
      if (x + m.width > vw - pad) x = Math.max(pad, anchor.x - m.width)
      if (y + m.height > vh - pad) { y = Math.max(pad, anchor.y - m.height); origin = 'bottom left' }
    }
    x = Math.max(pad, Math.min(x, vw - m.width - pad))
    y = Math.max(pad, Math.min(y, vh - m.height - pad))
    pos = { x, y, ready: true, origin }
  }

  onMount(() => {
    const prev = document.activeElement as HTMLElement | null
    place()
    if (autofocus) buttons()[0]?.focus({ preventScroll: true })
    else el?.focus({ preventScroll: true })
    const pop = pushLayer(onclose)
    const outside = (e: PointerEvent) => {
      const tgt = e.target as Element | null
      if (tgt?.closest(`[data-menu-root="${root}"]`)) return
      if (anchor instanceof HTMLElement && anchor.contains(tgt)) return // the trigger toggles itself
      onclose()
    }
    const blur = () => onclose()
    if (!rootId) {
      window.addEventListener('pointerdown', outside, true)
      window.addEventListener('resize', blur)
      window.addEventListener('blur', blur)
    }
    return () => {
      pop()
      window.removeEventListener('pointerdown', outside, true)
      window.removeEventListener('resize', blur)
      window.removeEventListener('blur', blur)
      if (!rootId && prev && document.contains(prev)) {
        const a = document.activeElement
        if (!a || a === document.body || !document.contains(a) || el?.contains(a)) prev.focus({ preventScroll: true })
      }
    }
  })

  function activate(e: MenuEntry, i: number, target: HTMLElement) {
    if (!isItem(e) || e.disabled) return
    if (e.children?.length) { sub = { index: i, el: target, kbd: true }; return }
    onclose()
    // run after the menu is gone and focus is restored (the action may open a dialog)
    if (e.onselect) setTimeout(e.onselect, 0)
  }

  function key(ev: KeyboardEvent) {
    const list = buttons()
    const i = list.indexOf(document.activeElement as HTMLElement)
    const move = (j: number) => { ev.preventDefault(); list[(j + list.length) % list.length]?.focus() }
    switch (ev.key) {
      case 'ArrowDown': return move(i + 1)
      case 'ArrowUp': return move(i < 0 ? list.length - 1 : i - 1)
      case 'Home': return move(0)
      case 'End': return move(list.length - 1)
      case 'ArrowRight': {
        const idx = Number((document.activeElement as HTMLElement)?.dataset.idx)
        const e = entries[idx]
        if (e && isItem(e) && e.children?.length) { ev.preventDefault(); sub = { index: idx, el: document.activeElement as HTMLElement, kbd: true } }
        return
      }
      case 'ArrowLeft':
        if (rootId) { ev.preventDefault(); ev.stopPropagation(); onclose(); parentFocus?.focus() }
        return
      case 'Tab': ev.preventDefault(); onclose(); return
    }
    if (ev.key.length === 1 && /\S/.test(ev.key)) {
      typed += ev.key.toLowerCase()
      clearTimeout(typedT)
      typedT = setTimeout(() => (typed = ''), 600)
      const start = Math.max(0, i)
      for (let k = 1; k <= list.length; k++) {
        const b = list[(start + k) % list.length]
        if ((b.textContent ?? '').trim().toLowerCase().startsWith(typed)) { b.focus(); break }
      }
    }
  }
</script>

<div bind:this={el} use:portal class="menu" role="menu" aria-label={label || undefined} tabindex="-1" data-menu-root={root}
  style:left="{pos.x}px" style:top="{pos.y}px" style:min-width="{minWidth}px" style:visibility={pos.ready ? 'visible' : 'hidden'}
  style:transform-origin={pos.origin} onkeydown={key} oncontextmenu={(e) => e.preventDefault()}>
  {#each entries as e, i}
    {#if e.kind === 'separator'}
      <div class="sep" role="separator"></div>
    {:else if e.kind === 'header'}
      <div class="hdr" role="presentation">{e.label}</div>
    {:else}
      <MenuItem entry={e} index={i} expanded={sub?.index === i}
        onactivate={(target) => activate(e, i, target)}
        onhover={(target) => { if (sub?.index !== i) sub = e.children?.length ? { index: i, el: target, kbd: false } : null }} />
    {/if}
  {/each}
</div>

{#if sub}
  {@const se = entries[sub.index]}
  {#if se && isItem(se) && se.children}
    <Menu items={se.children} anchor={sub.el} placement="right-start" rootId={root} parentFocus={sub.el}
      onclose={() => { sub = null }} label={se.label} minWidth={170} autofocus={sub.kbd} />
  {/if}
{/if}

<style>
  .menu {
    position: fixed; z-index: 500; padding: 4px; border-radius: var(--r-md);
    background: var(--surface-1); border: 1px solid var(--border); box-shadow: var(--elev-16);
    display: flex; flex-direction: column; gap: 1px; max-height: calc(100vh - 8px); overflow-y: auto; outline: none;
    animation: menuIn var(--dur-fast) var(--ease-decel);
  }
  :global(:root[data-glass]) .menu { -webkit-backdrop-filter: blur(30px) saturate(1.5); backdrop-filter: blur(30px) saturate(1.5); }
  :global(:root[data-glass]:not([data-theme='dark'])) .menu { background: rgba(252, 252, 252, 0.85); }
  :global(:root[data-glass][data-theme='dark']) .menu { background: rgba(44, 44, 44, 0.88); }
  .sep { height: 1px; background: var(--border); margin: 4px -4px; }
  .hdr { font-size: var(--fs-caption); color: var(--muted); font-weight: 600; padding: 6px 12px 4px; }
  @keyframes menuIn { from { opacity: 0; transform: translateY(-6px); } to { opacity: 1; transform: none; } }
</style>
