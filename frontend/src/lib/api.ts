import { Call, Events } from '@wailsio/runtime'
import type {
  AddRequest, GrabberProject, Item, PendingAdd, ProbeResult, QueueState, SchedulerConfig, Totals, UIInfo, VideoDetails,
  RemoteAddress, RemoteStatus, SchedulerStatus, Settings, ToolStatus, TorrentInfo, UpdateInfo, VideoInfo,
} from './types'

const call = <T = any>(name: string, ...args: any[]): Promise<T> => Call.ByName('main.API.' + name, ...args) as Promise<T>

/** Methods the running backend does not expose (older build): remembered to avoid repeated failing calls. */
const missing = new Set<string>()

/** True when the error says the bound method does not exist. */
export function isMissingMethod(e: any): boolean {
  return /unknown|not found|no such|cannot find|unbound|not bound|is not a function/i.test(errMsg(e))
}

/** Calls a method that may not exist in older backends; resolves to `fallback()` when missing. */
async function optional<T>(name: string, fallback: () => T | Promise<T>, ...args: any[]): Promise<T> {
  if (missing.has(name)) return fallback()
  try {
    return await call<T>(name, ...args)
  } catch (e) {
    if (isMissingMethod(e)) {
      missing.add(name)
      return fallback()
    }
    throw e
  }
}

export class Unsupported extends Error {}
const unsupported = (name: string) => () => { throw new Unsupported(name) }

/** Whether a backend method is known to be available (false after a "missing method" failure). */
export const hasMethod = (name: string) => !missing.has(name)

export const api = {
  version: () => call<string>('Version'),
  uiInfo: (win: string) => call<UIInfo>('UIInfo', win),
  platform: () => call<string>('Platform'),
  list: () => call<Item[] | null>('List').then((l) => l ?? []),
  get: (id: string) => call<Item>('Get', id),
  settings: () => call<Settings>('Settings'),
  saveSettings: (s: Settings) => call<void>('SaveSettings', s),
  defaultSettings: () => call<Settings>('DefaultSettings'),
  probe: (url: string, headers: Record<string, string> = {}) => call<ProbeResult>('Probe', url, headers),
  add: (r: AddRequest) => call<Item>('Add', r),
  addBatch: (r: AddRequest[]) => call<number>('AddBatch', r),
  pending: (t: string) => call<PendingAdd>('Pending', t),
  donePending: (t: string) => call<void>('DonePending', t),
  openAddDialog: (url = '') => call<void>('OpenAddDialog', url),
  openProgress: (id: string) => call<void>('OpenProgress', id),
  showMain: () => call<void>('ShowMain'),
  start: (ids: string[]) => call<void>('Start', ids),
  pause: (ids: string[]) => call<void>('Pause', ids),
  enqueue: (ids: string[]) => call<void>('Enqueue', ids),
  remove: (ids: string[], deleteFiles: boolean) => call<void>('Remove', ids, deleteFiles),
  /** Stops, removes and deletes partial files (never completed files). Falls back to Remove+delete. */
  cancel: (ids: string[]) => optional<void>('Cancel', () => call<void>('Remove', ids, true), ids),
  /** -1 low, 0 normal, 1 high. Throws Unsupported on older backends. */
  setPriority: (ids: string[], p: number) => optional<void>('SetPriority', unsupported('SetPriority'), ids, p),
  /** Moves `id` just before `beforeId` in its queue ('' = end). Throws Unsupported on older backends. */
  reorder: (id: string, beforeId: string) => optional<void>('Reorder', unsupported('Reorder'), id, beforeId),
  /** minimise | toggleMaximise | close | startDrag. Resolves false when unavailable. */
  windowAction: (win: string, action: 'minimise' | 'toggleMaximise' | 'close' | 'startDrag') =>
    optional<boolean>('WindowAction', () => false, win, action).then((r) => r !== false),
  /** Queue totals; null when the backend has no Totals method (the UI computes them). */
  totals: () => optional<Totals | null>('Totals', () => null),
  removeCompleted: () => call<number>('RemoveCompleted'),
  redownload: (id: string) => call<void>('Redownload', id),
  startAll: () => call<void>('StartAll'),
  pauseAll: () => call<void>('PauseAll'),
  updateURL: (id: string, url: string) => call<void>('UpdateURL', id, url),
  setItemOptions: (id: string, o: Record<string, any>) => call<void>('SetItemOptions', id, o),
  setSpeedLimit: (bps: number) => call<void>('SetSpeedLimit', bps),
  queues: () => call<QueueState[] | null>('Queues').then((q) => q ?? []),
  saveQueue: (q: QueueState) => call<void>('SaveQueue', q),
  deleteQueue: (n: string) => call<void>('DeleteQueue', n),
  startQueue: (n: string) => call<void>('StartQueue', n),
  stopQueue: (n: string) => call<void>('StopQueue', n),
  moveToQueue: (ids: string[], q: string) => call<void>('MoveToQueue', ids, q),
  openFile: (id: string) => call<void>('OpenFile', id),
  openFolder: (id: string) => call<void>('OpenFolder', id),
  openDir: (d: string) => call<void>('OpenDir', d),
  openURL: (u: string) => call<void>('OpenURL', u),
  chooseFolder: (initial = '') => call<string>('ChooseFolder', initial),
  chooseFile: (title: string, filterName: string, pattern: string) => call<string>('ChooseFile', title, filterName, pattern),
  exportList: () => call<string>('ExportList'),
  importList: () => call<number>('ImportList'),
  clipboardText: () => call<string>('ClipboardText'),
  diskFree: (d: string) => call<number>('DiskFree', d),
  stats: () => call<Record<string, any>>('Stats'),
  analyzeVideo: (kind: string, url: string, headers: Record<string, string> = {}, playlist = false) =>
    call<VideoInfo>('AnalyzeVideo', kind, url, headers, playlist),
  analyzeDetails: (kind: string, url: string, headers: Record<string, string> = {}, title = '', playlist = false) =>
    call<VideoDetails>('AnalyzeDetails', kind, url, headers, title, playlist),
  detectKind: (url: string) => call<string>('DetectKind', url),
  tools: () => call<ToolStatus[] | null>('Tools').then((t) => t ?? []),
  installTool: (n: string) => call<void>('InstallTool', n),
  hosters: () => call<{ names: string[]; version: number; ytdlp: string[] }>('Hosters'),
  updateRules: () => call<boolean>('UpdateRules'),
  scheduler: () => call<SchedulerStatus>('Scheduler'),
  saveScheduler: (c: SchedulerConfig) => call<void>('SaveScheduler', c),
  grabberProjects: () => call<GrabberProject[] | null>('GrabberProjects').then((p) => p ?? []),
  saveGrabber: (p: GrabberProject) => call<GrabberProject>('SaveGrabber', p),
  deleteGrabber: (id: string) => call<void>('DeleteGrabber', id),
  startGrabber: (id: string) => call<void>('StartGrabber', id),
  stopGrabber: (id: string) => call<void>('StopGrabber', id),
  inspectTorrent: (src: string) => call<TorrentInfo>('InspectTorrent', src),
  chooseTorrent: () => call<string>('ChooseTorrent'),
  remoteStatus: () => call<RemoteStatus>('RemoteStatus'),
  remoteEnable: (on: boolean, port: number) => call<RemoteStatus>('RemoteEnable', on, port),
  remotePair: () => call<{ code: string; addresses: RemoteAddress[] | null }>('RemotePair'),
  remoteRevoke: (id: string) => call<void>('RemoteRevoke', id),
  checkUpdate: () => call<UpdateInfo>('CheckUpdate'),
  installUpdate: () => call<void>('InstallUpdate'),
}

/** Subscribe to a backend event; returns an unsubscribe function. */
export function on<T = any>(name: string, cb: (data: T) => void): () => void {
  return Events.On(name, (ev: any) => cb(ev.data as T))
}

/** Extract a readable message from a rejected call. */
export function errMsg(e: any): string {
  if (!e) return ''
  if (typeof e === 'string') return e
  if (e.message) {
    try {
      const j = JSON.parse(e.message)
      if (j && j.message) return j.message
    } catch { /* plain text */ }
    return e.message
  }
  return String(e)
}
