import { api, on } from './api'
import { setLang } from './i18n.svelte'
import { applyGlass, applyTheme } from './theme.svelte'
import type { AppEvent, Item, Progress, QueueState, Settings, Totals } from './types'

export { applyTheme }

export const store = $state({
  items: [] as Item[],
  /** true once the list has been loaded (skeletons before) */
  loaded: false,
  progress: {} as Record<string, Progress>,
  settings: null as Settings | null,
  queues: [] as QueueState[],
  platform: 'windows',
  version: '',
  glass: false,
  macInset: false,
  /** the window has no native frame: draw the custom title bar */
  frameless: false,
  maximised: false,
  /** window name passed to UIInfo / WindowAction */
  win: 'main',
  /** last samples (4 Hz) of speed per item and globally, for sparklines */
  history: {} as Record<string, number[]>,
  totalHistory: [] as number[],
  /** backend queue totals (null when the backend has no Totals method) */
  totals: null as Totals | null,
})

const HISTORY = 120

function pushHistory(arr: number[], v: number) {
  arr.push(v)
  if (arr.length > HISTORY) arr.splice(0, arr.length - HISTORY)
}

function upsert(it: Item) {
  const i = store.items.findIndex((x) => x.id === it.id)
  if (i >= 0) store.items[i] = it
  else store.items.push(it)
}

function forget(id: string) {
  delete store.progress[id]
  delete store.history[id]
}

let started = false

/** Window name for UIInfo / WindowAction, derived from the route. */
export function windowName(): string {
  const r = route()
  if (r.name === 'progress') return 'progress:' + r.arg
  return r.name || 'main'
}

/** Loads data and subscribes to backend events (call once per window). */
export async function initStore(opts: { items?: boolean } = { items: true }) {
  if (started) return
  started = true
  store.win = windowName()
  const [s, platform, version, ui] = await Promise.all([
    api.settings(), api.platform(), api.version(),
    api.uiInfo(store.win === 'main' ? 'main' : store.win).catch(() => null),
  ])
  store.settings = s
  store.platform = platform
  store.version = version
  document.documentElement.dataset.platform = platform
  if (ui) {
    store.glass = ui.glass
    store.macInset = ui.macInset
    store.frameless = !!ui.frameless
    store.maximised = !!ui.maximised
    applyGlass(ui.glass)
    if (ui.macInset) document.documentElement.dataset.macInset = '1'
    if (store.frameless) document.documentElement.dataset.frameless = '1'
  }
  setLang(s.language)
  applyTheme(s.theme)
  if (opts.items !== false) {
    const [items, queues] = await Promise.all([api.list(), api.queues()])
    store.items = items
    store.queues = queues
  }
  store.loaded = true
  on<AppEvent>('fdm:event', async (ev) => {
    switch (ev.type) {
      case 'added':
      case 'updated':
      case 'completed':
      case 'failed':
        if (ev.item) upsert(ev.item)
        if (ev.item && (ev.item.status === 'completed' || ev.item.status === 'error' || ev.item.status === 'paused' || ev.item.status === 'queued')) {
          delete store.progress[ev.item.id]
        }
        if (ev.item?.status === 'completed') delete store.history[ev.item.id]
        break
      case 'removed':
        store.items = store.items.filter((x) => x.id !== ev.id)
        if (ev.id) forget(ev.id)
        break
      case 'settings':
        store.settings = await api.settings()
        setLang(store.settings.language)
        applyTheme(store.settings.theme)
        store.queues = await api.queues()
        break
    }
  })
  // Progress ticks only touch store.progress / history: the item list (and its
  // filter/sort) is not invalidated 4 times per second.
  on<Progress[]>('fdm:progress', (batch) => {
    let total = 0
    for (const p of batch ?? []) {
      store.progress[p.id] = p
      total += p.speed || 0
      const h = (store.history[p.id] ??= [])
      pushHistory(h, p.speed || 0)
    }
    pushHistory(store.totalHistory, total)
  })
  on<{ win: string; maximised: boolean }>('fdm:window', (w) => {
    if (w && (w.win === store.win || (w.win === 'main' && store.win === 'main'))) store.maximised = !!w.maximised
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

/** Item currently downloaded size / total size, preferring live progress. */
export function liveSize(it: Item): { done: number; size: number } {
  const p = store.progress[it.id]
  return { done: p?.downloaded ?? it.downloaded, size: p?.size && p.size > 0 ? p.size : it.size }
}
