export type Status = 'queued' | 'downloading' | 'paused' | 'completed' | 'error' | 'processing'
export type Kind = 'http' | 'ftp' | 'hls' | 'dash' | 'ytdlp' | 'torrent' | 'hoster'

export interface Item {
  id: string
  kind: Kind
  url: string
  pageUrl?: string
  title?: string
  fileName: string
  dir: string
  path?: string
  category: string
  queue: string
  status: Status
  size: number
  downloaded: number
  error?: string
  description?: string
  headers?: Record<string, string>
  options?: Record<string, any>
  createdAt: string
  completedAt?: string
}

export interface Segment { s: number; e: number; p: number }

export interface Progress {
  id: string
  size: number
  downloaded: number
  speed: number
  eta: number
  conns: number
  segments?: Segment[]
  extra?: Record<string, any>
}

export interface Category { name: string; exts: string[]; dir: string }
export interface Cred { host: string; user: string; password: string }

export interface Settings {
  downloadDir: string
  maxConns: number
  maxActive: number
  speedLimit: number
  proxy: string
  allowHttp2: boolean
  insecureTls: boolean
  userAgent: string
  categories: Category[]
  captureExts: string[]
  captureMinSize: number
  skipSites: string[]
  clipboardMonitor: boolean
  showAddDialog: boolean
  showDoneDialog: boolean
  antivirusPath: string
  antivirusArgs: string
  language: string
  theme: string
  hlsOutput: string
  hlsConcurrency: number
  videoQuality: string
  autoUpdateTools: boolean
  ytdlpCookiesBrowser: string
  credentials: Cred[]
  torrentPort: number
  torrentSeed: boolean
  startMinimized: boolean
  launchAtStartup: boolean
  showProgressWin: boolean
  closeToTray: boolean
  videoButton: boolean
  settingsVersion: number
  rulesUrl: string
  updateRepo: string
}

export interface UpdateInfo { current: string; latest: string; newer: boolean; url: string; asset: string; notes: string }

export interface AddRequest {
  url: string
  pageUrl?: string
  title?: string
  fileName?: string
  dir?: string
  category?: string
  queue?: string
  kind?: string
  headers?: Record<string, string>
  options?: Record<string, any>
  start: boolean
  size?: number
  description?: string
}

export interface PendingAdd {
  token: string
  request: AddRequest
  batch?: AddRequest[]
  video: boolean
}

export interface ProbeResult {
  kind: string
  fileName: string
  size: number
  resumable: boolean
  contentType: string
  category: string
  dir: string
  finalUrl: string
}

export interface QueueState { name: string; running: boolean; maxActive: number }

export interface Format {
  id: string; ext: string; width: number; height: number; fps: number
  vcodec: string; acodec: string; filesize: number; tbr: number; note: string; protocol: string
}
export interface Variant { url: string; bandwidth: number; width: number; height: number; codecs: string; frameRate: number; audioGroup?: string; name?: string }
export interface DashRep { id: string; kind: string; bandwidth: number; width: number; height: number; codecs: string; lang?: string }
export interface Rendition { type: string; groupId: string; name: string; language: string; uri: string; default: boolean }
export interface VideoInfo {
  source: string
  title: string
  thumbnail?: string
  duration: number
  extractor?: string
  formats: Format[] | null
  subtitles?: string[] | null
  playlist: boolean
  entries?: { url: string; title: string; duration: number }[] | null
  variants?: Variant[] | null
  dash?: DashRep[] | null
  audio?: Rendition[] | null
  live: boolean
}

export interface ToolStatus { name: string; installed: boolean; path: string; version: string; managed: boolean }

export interface Schedule {
  queue: string; enabled: boolean; startTime: string; stopTime: string
  days: number[]; once: boolean; onDone: string
}
export interface SchedulerConfig {
  schedules: Schedule[]
  quota: { enabled: boolean; mb: number; hours: number }
  syncEveryMinutes: number
}
export interface SchedulerStatus { config: SchedulerConfig; quotaUsedMb: number; quotaPaused: boolean }

export interface GrabberProject {
  id: string; name: string; startUrl: string; depth: number; sameHost: boolean; pathPrefix: boolean
  maxPages: number; include: string[]; exclude: string[]; minSize: number; dir: string
  savePages: boolean; autoAdd: boolean; queue: string; cookie: string; periodicHours: number; lastRun?: string
}
export interface GrabberFound { url: string; page: string; text?: string; type: string; ext: string; new: boolean }
export interface GrabberStatus {
  projectId: string; running: boolean; pages: number; queued: number
  found?: GrabberFound[]; foundCount: number; added: number; error?: string; current?: string
}

export interface TorrentInfo { name: string; infoHash: string; size: number; files?: { index: number; path: string; size: number }[] }

export interface AppEvent {
  type: 'added' | 'updated' | 'removed' | 'progress' | 'completed' | 'failed' | 'settings'
  item?: Item
  id?: string
  progress?: Progress
}
