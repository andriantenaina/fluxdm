// UI state of the main window shared by its components (toolbar, list,
// selection bar, context menu, status bar) and the actions they trigger.
import { api, errMsg, Unsupported } from './api'
import { confirmBox, promptBox } from './dialogs.svelte'
import { t, tf, tn } from './i18n.svelte'
import { store, refreshQueues } from './state.svelte'
import { toast } from './toast.svelte'
import { displayName } from './itemview'
import * as fmt from './format'
import type { Item } from './types'

export type Filter = { type: 'all' | 'cat' | 'status' | 'queue'; value: string }
export type SortCol = 'queue' | 'name' | 'size' | 'progress' | 'speed' | 'status' | 'date'
export type MenuAnchor = HTMLElement | { x: number; y: number }

const ls = (k: string, d: string) => { try { return localStorage.getItem(k) ?? d } catch { return d } }
const lsSet = (k: string, v: string) => { try { localStorage.setItem(k, v) } catch { /* private mode */ } }

const SORTS: SortCol[] = ['queue', 'name', 'size', 'progress', 'speed', 'status', 'date']
const savedSort = ls('fdm.sort', 'date')

export const ui = $state({
  filter: { type: 'all', value: '' } as Filter,
  search: '',
  sortCol: (SORTS.includes(savedSort as SortCol) ? savedSort : 'date') as SortCol,
  sortAsc: ls('fdm.sortAsc', '0') === '1',
  view: (ls('fdm.view', 'cards') === 'compact' ? 'compact' : 'cards') as 'cards' | 'compact',
  showDetails: ls('fdm.details', '1') === '1',
  /** details as an overlay drawer (narrow windows) */
  drawer: false,
  narrow: false,
  selected: new Set<string>(),
  anchor: null as string | null,
  /** roving focus of the list */
  focusId: null as string | null,
  menu: null as { anchor: MenuAnchor } | null,
  modal: '' as '' | 'settings' | 'scheduler' | 'grabber' | 'props',
  propsId: '',
})

export function persistUi() {
  lsSet('fdm.view', ui.view)
  lsSet('fdm.details', ui.showDetails ? '1' : '0')
  lsSet('fdm.sort', ui.sortCol)
  lsSet('fdm.sortAsc', ui.sortAsc ? '1' : '0')
}

export function setSort(c: SortCol) {
  if (ui.sortCol === c) ui.sortAsc = !ui.sortAsc
  else { ui.sortCol = c; ui.sortAsc = c === 'name' || c === 'queue' }
  persistUi()
}

export function select(ids: string[], anchor?: string | null) {
  ui.selected = new Set(ids)
  if (anchor !== undefined) ui.anchor = anchor
}

/** ids of the current selection that still exist */
export function selectedIds(): string[] {
  return store.items.filter((i) => ui.selected.has(i.id)).map((i) => i.id)
}

/** Runs an action with error toast; optional success toast. */
export async function act(fn: () => Promise<unknown>, ok?: string) {
  try {
    await fn()
    if (ok) toast(ok, 'ok')
  } catch (e) {
    toast(e instanceof Unsupported ? (t('unsupported') as string) : errMsg(e), 'err')
  }
}

export const curQueue = () => (ui.filter.type === 'queue' ? ui.filter.value : 'main')

export function openItem(it: Item) {
  if (it.status === 'completed') act(() => api.openFile(it.id))
  else act(() => api.openProgress(it.id))
}

export function toggleRun(items: Item[]) {
  if (!items.length) return
  const running = items[0].status === 'downloading' || items[0].status === 'processing' || items[0].status === 'queued'
  const ids = items.map((i) => i.id)
  return act(() => (running ? api.pause(ids) : api.start(ids)))
}

export async function removeItems(ids: string[], withFiles = false) {
  if (!ids.length) return
  const r = await confirmBox(`${t('confirmDelete')} (${ids.length})`, withFiles ? undefined : (t('confirmDeleteFiles') as string), {
    title: t('confirmDeleteTitle') as string, okLabel: t('delete') as string, danger: true,
  })
  if (!r.ok) return
  await act(async () => {
    await api.remove(ids, withFiles || r.checked)
    toast(tn('removed', ids.length), 'info')
    ui.selected = new Set([...ui.selected].filter((id) => !ids.includes(id)))
  })
}

export async function cancelItems(items: Item[]) {
  const list = items.filter((i) => i.status !== 'completed')
  if (!list.length) return
  const msg = list.length === 1 ? tf('confirmCancel', { name: displayName(list[0]) }) : tf('confirmCancelN', { n: list.length })
  const r = await confirmBox(msg, undefined, { title: t('cancelDownload') as string, okLabel: t('cancelDownload') as string, cancelLabel: t('close') as string, danger: true })
  if (r.ok) await act(() => api.cancel(list.map((i) => i.id)))
}

export function setPriority(ids: string[], p: number) {
  return act(() => api.setPriority(ids, p))
}

export async function refreshUrl(it: Item) {
  const u = await promptBox(t('newUrl') as string, it.url, { inputType: 'url' })
  if (u && u.trim() && u.trim() !== it.url) act(() => api.updateURL(it.id, u.trim()), t('urlUpdated') as string)
}

export function copyUrl(it: Item) {
  return act(() => navigator.clipboard.writeText(it.url), t('urlCopied') as string)
}

export function moveToQueue(ids: string[], q: string) {
  return act(() => api.moveToQueue(ids, q))
}

export async function setLimit(kbps: number) {
  await act(() => api.setSpeedLimit(kbps * 1024))
  toast(kbps ? tf('limitOn', { v: fmt.speed(kbps * 1024) }) : tf('limitOn', { v: t('unlimited') as string }), 'info')
}

export async function customLimit() {
  const cur = Math.round((store.settings?.speedLimit ?? 0) / 1024)
  const v = await promptBox(t('customLimitPrompt') as string, String(cur), { inputType: 'number', title: t('speedLimiter') as string })
  if (v == null) return
  const n = Math.max(0, Math.round(Number(v) || 0))
  setLimit(n)
}

export async function startQueue() {
  await act(() => api.startQueue(curQueue()).then(refreshQueues))
}
export async function stopQueue() {
  await act(() => api.stopQueue(curQueue()).then(refreshQueues))
}
export async function openTorrent() {
  const p = await api.chooseTorrent().catch(() => '')
  if (p) api.openAddDialog(p)
}
