// Promise-based confirm / prompt / alert rendered by <Dialogs /> (native
// window.confirm is not available in every webview).
type Req = {
  kind: 'confirm' | 'prompt' | 'alert'
  message: string
  value: string
  check?: string // optional checkbox label (confirm)
  checked: boolean
  resolve: (v: any) => void
}

export const dlg = $state({ current: null as Req | null })

export function confirmBox(message: string, check?: string): Promise<{ ok: boolean; checked: boolean }> {
  return new Promise((resolve) => {
    dlg.current = { kind: 'confirm', message, value: '', check, checked: false, resolve }
  })
}

export function promptBox(message: string, value = ''): Promise<string | null> {
  return new Promise((resolve) => {
    dlg.current = { kind: 'prompt', message, value, checked: false, resolve }
  })
}

export function alertBox(message: string): Promise<void> {
  return new Promise((resolve) => {
    dlg.current = { kind: 'alert', message, value: '', checked: false, resolve }
  })
}
