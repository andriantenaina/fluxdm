// A single stack of overlay layers (dialogs, menus, drawers). Escape closes
// only the topmost layer, and views can check `layers.depth` to ignore global
// shortcuts while an overlay is open (e.g. Enter on a confirm dialog).

export const layers = $state({ depth: 0 })

type Layer = { id: number; close: () => void }
const stack: Layer[] = []
let seq = 0
let bound = false

function onKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !stack.length || e.defaultPrevented) return
  e.preventDefault()
  e.stopImmediatePropagation()
  stack[stack.length - 1].close()
}

/** Registers a layer; returns its removal function. */
export function pushLayer(close: () => void): () => void {
  if (!bound) {
    window.addEventListener('keydown', onKey, true)
    bound = true
  }
  const l = { id: ++seq, close }
  stack.push(l)
  layers.depth = stack.length
  return () => {
    const i = stack.findIndex((x) => x.id === l.id)
    if (i >= 0) stack.splice(i, 1)
    layers.depth = stack.length
  }
}

/** Svelte action form: `use:layer={onclose}`. */
export function layer(_node: HTMLElement, close: () => void) {
  let fn = close
  const pop = pushLayer(() => fn())
  return {
    update(c: () => void) { fn = c },
    destroy: pop,
  }
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable="true"]'

export function focusables(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.offsetParent !== null || el === document.activeElement)
}

/**
 * Focus trap action: keeps Tab inside the node, focuses `[data-autofocus]` or
 * the first focusable on mount and restores the previous focus on destroy.
 */
export function trapFocus(node: HTMLElement, opts: { initial?: string } = {}) {
  const prev = document.activeElement as HTMLElement | null
  queueMicrotask(() => {
    const target = (opts.initial && node.querySelector<HTMLElement>(opts.initial)) || node.querySelector<HTMLElement>('[data-autofocus]') || focusables(node)[0] || node
    target.focus({ preventScroll: true })
    if (target instanceof HTMLInputElement && (target.type === 'text' || target.type === 'url' || !target.type)) target.select()
  })
  function key(e: KeyboardEvent) {
    if (e.key !== 'Tab') return
    const f = focusables(node)
    if (!f.length) { e.preventDefault(); return }
    const first = f[0], last = f[f.length - 1]
    const a = document.activeElement
    if (e.shiftKey && (a === first || !node.contains(a))) { e.preventDefault(); last.focus() }
    else if (!e.shiftKey && (a === last || !node.contains(a))) { e.preventDefault(); first.focus() }
  }
  node.addEventListener('keydown', key)
  return {
    destroy() {
      node.removeEventListener('keydown', key)
      if (prev && document.contains(prev)) queueMicrotask(() => prev.focus({ preventScroll: true }))
    },
  }
}
