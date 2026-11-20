// Lightweight tooltip action: `use:tooltip={'Label'}` or `use:tooltip={{ text, shortcut }}`.
// One shared floating element, shown on hover (after a delay) and on keyboard focus.
type Opts = string | { text: string; shortcut?: string; placement?: 'top' | 'bottom' } | null | undefined

let tip: HTMLDivElement | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let owner: HTMLElement | null = null

function ensure(): HTMLDivElement {
  if (tip) return tip
  tip = document.createElement('div')
  tip.className = 'fdm-tooltip'
  tip.setAttribute('role', 'tooltip')
  tip.id = 'fdm-tooltip'
  document.body.appendChild(tip)
  return tip
}

function norm(o: Opts) {
  if (!o) return null
  return typeof o === 'string' ? { text: o, shortcut: '', placement: 'bottom' as const } : { shortcut: '', placement: 'bottom' as const, ...o }
}

function show(node: HTMLElement, o: NonNullable<ReturnType<typeof norm>>) {
  const el = ensure()
  owner = node
  el.textContent = ''
  el.append(o.text)
  if (o.shortcut) {
    const k = document.createElement('kbd')
    k.textContent = o.shortcut
    el.append(k)
  }
  el.dataset.show = '1'
  const r = node.getBoundingClientRect()
  const tr = el.getBoundingClientRect()
  let top = o.placement === 'top' ? r.top - tr.height - 6 : r.bottom + 6
  if (top + tr.height > window.innerHeight - 4) top = r.top - tr.height - 6
  if (top < 4) top = r.bottom + 6
  const left = Math.max(4, Math.min(window.innerWidth - tr.width - 4, r.left + r.width / 2 - tr.width / 2))
  el.style.transform = `translate(${Math.round(left)}px, ${Math.round(top)}px)`
}

function hide(node?: HTMLElement) {
  clearTimeout(timer)
  if (node && owner !== node) return
  owner = null
  if (tip) delete tip.dataset.show
}

export function tooltip(node: HTMLElement, opts: Opts) {
  let o = norm(opts)
  const enter = () => { if (!o) return; clearTimeout(timer); timer = setTimeout(() => o && show(node, o), 500) }
  const focus = () => { if (o && node.matches(':focus-visible')) show(node, o) }
  const leave = () => hide(node)
  node.addEventListener('mouseenter', enter)
  node.addEventListener('mouseleave', leave)
  node.addEventListener('focus', focus)
  node.addEventListener('blur', leave)
  node.addEventListener('pointerdown', leave)
  return {
    update(n: Opts) { o = norm(n); if (owner === node) { if (o) show(node, o); else hide(node) } },
    destroy() {
      hide(node)
      node.removeEventListener('mouseenter', enter)
      node.removeEventListener('mouseleave', leave)
      node.removeEventListener('focus', focus)
      node.removeEventListener('blur', leave)
      node.removeEventListener('pointerdown', leave)
    },
  }
}
