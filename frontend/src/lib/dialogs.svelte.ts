// Promise-based confirm / prompt / alert rendered by <Dialogs /> (native
// window.confirm is not available in every webview).
export type DialogOpts = { title?: string; okLabel?: string; cancelLabel?: string; danger?: boolean; placeholder?: string; inputType?: 'text' | 'number' | 'url' }

type Req = {
  kind: 'confirm' | 'prompt' | 'alert'
  message: string
  value: string
  check?: string // optional checkbox label (confirm)
  checked: boolean
  opts: DialogOpts
  resolve: (v: any) => void
}

export const dlg = $state({ current: null as Req | null })

function open(r: Req) {
  // a new request replaces (and cancels) a pending one
  const prev = dlg.current
  if (prev) prev.resolve(prev.kind === 'confirm' ? { ok: false, checked: false } : prev.kind === 'prompt' ? null : undefined)
  dlg.current = r
}

export function confirmBox(message: string, check?: string, opts: DialogOpts = {}): Promise<{ ok: boolean; checked: boolean }> {
  return new Promise((resolve) => open({ kind: 'confirm', message, value: '', check, checked: false, opts, resolve }))
}

export function promptBox(message: string, value = '', opts: DialogOpts = {}): Promise<string | null> {
  return new Promise((resolve) => open({ kind: 'prompt', message, value, checked: false, opts, resolve }))
}

export function alertBox(message: string, opts: DialogOpts = {}): Promise<void> {
  return new Promise((resolve) => open({ kind: 'alert', message, value: '', checked: false, opts, resolve }))
}
