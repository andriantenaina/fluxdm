// Toast queue: at most 4 visible, timers paused while hovered or focused.
export type Toast = {
  id: number
  text: string
  kind: 'info' | 'ok' | 'err'
  action?: { label: string; run: () => void }
  ms: number
  /** remaining time when paused */
  left: number
  since: number
}

export const toasts = $state({ list: [] as Toast[], paused: false })
let seq = 0
const timers = new Map<number, ReturnType<typeof setTimeout>>()
const waiting: Toast[] = []

function arm(t: Toast) {
  clearTimeout(timers.get(t.id))
  t.since = Date.now()
  timers.set(t.id, setTimeout(() => dismiss(t.id), t.left))
}

export function toast(text: string, kind: Toast['kind'] = 'info', action?: Toast['action'], ms = 4200) {
  if (kind === 'err') ms = Math.max(ms, 7000)
  const t: Toast = { id: ++seq, text, kind, action, ms, left: ms, since: Date.now() }
  if (toasts.list.length >= 4) waiting.push(t)
  else show(t)
}

function show(t: Toast) {
  toasts.list.push(t)
  if (!toasts.paused) arm(toasts.list[toasts.list.length - 1])
}

export function dismiss(id: number) {
  clearTimeout(timers.get(id))
  timers.delete(id)
  const i = toasts.list.findIndex((x) => x.id === id)
  if (i >= 0) toasts.list.splice(i, 1)
  const next = waiting.shift()
  if (next) show(next)
}

/** Pause every timer (hover / focus inside the stack). */
export function pauseToasts() {
  if (toasts.paused) return
  toasts.paused = true
  const now = Date.now()
  for (const t of toasts.list) {
    clearTimeout(timers.get(t.id))
    t.left = Math.max(1200, t.left - (now - t.since))
  }
}

export function resumeToasts() {
  if (!toasts.paused) return
  toasts.paused = false
  for (const t of toasts.list) arm(t)
}
