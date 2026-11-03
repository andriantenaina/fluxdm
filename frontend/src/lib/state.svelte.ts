import { api, on } from './api'
import { setLang } from './i18n.svelte'
import type { AppEvent, Item, Progress, QueueState, Settings } from './types'

export const store = $state({
  items: [] as Item[],
  progress: {} as Record<string, Progress>,
  settings: null as Settings | null,
  queues: [] as QueueState[],
  platform: 'windows',
  version: '',
})

export function applyTheme(theme: string) {
  const el = document.documentElement
  if (theme === 'light' || theme === 'dark') el.dataset.theme = theme
  else delete el.dataset.theme
}

function upsert(it: Item) {
  const i = store.items.findIndex((x) => x.id === it.id)
  if (i >= 0) store.items[i] = it
  else store.items.push(it)
}

let started = false

/** Loads data and subscribes to backend events (call once per window). */
export async function initStore(opts: { items?: boolean } = { items: true }) {
  if (started) return
  started = true
  const [s, platform, version] = await Promise.all([api.settings(), api.platform(), api.version()])
  store.settings = s
  store.platform = platform
  store.version = version
  setLang(s.language)
  applyTheme(s.theme)
  if (opts.items !== false) {
    store.items = await api.list()
    store.queues = await api.queues()
  }
  on<AppEvent>('fdm:event', async (ev) => {
    switch (ev.type) {
      case 'added':
      case 'updated':
      case 'completed':
      case 'failed':
        if (ev.item) upsert(ev.item)
        if (ev.item && (ev.item.status === 'completed' || ev.item.status === 'error' || ev.item.status === 'paused')) {
          delete store.progress[ev.item.id]
        }
        break
      case 'removed':
        store.items = store.items.filter((x) => x.id !== ev.id)
        if (ev.id) delete store.progress[ev.id]
        break
      case 'settings':
        store.settings = await api.settings()
        setLang(store.settings.language)
        applyTheme(store.settings.theme)
        store.queues = await api.queues()
        break
    }
  })
  on<Progress[]>('fdm:progress', (batch) => {
    for (const p of batch ?? []) {
      store.progress[p.id] = p
      const it = store.items.find((x) => x.id === p.id)
      if (it) {
        it.downloaded = p.downloaded
        if (p.size > 0) it.size = p.size
      }
    }
  })
}

/** Hand-off between dialog routes of the same window (add -> video). */
export const handoff = $state({ url: '', headers: {} as Record<string, string> })

export async function refreshQueues() {
  store.queues = await api.queues()
}

/** Route from the hash: #/main, #/add/<token>, #/progress/<id> ... */
export function route(): { name: string; arg: string } {
  const h = location.hash.replace(/^#\/?/, '')
  const [name, ...rest] = h.split('/')
  return { name: name || 'main', arg: decodeURIComponent(rest.join('/')) }
}
