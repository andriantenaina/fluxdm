import { Call, Events } from '@wailsio/runtime'
import type {
  AddRequest, GrabberProject, Item, PendingAdd, ProbeResult, QueueState, SchedulerConfig,
  SchedulerStatus, Settings, ToolStatus, TorrentInfo, UpdateInfo, VideoInfo,
} from './types'

const call = <T = any>(name: string, ...args: any[]): Promise<T> => Call.ByName('main.API.' + name, ...args) as Promise<T>

export const api = {
  version: () => call<string>('Version'),
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
