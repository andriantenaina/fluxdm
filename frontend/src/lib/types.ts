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
  /** -1 low, 0 normal (default), 1 high */
  priority?: number
  /** manual position in its queue (smaller first; 0 = use createdAt) */
  order?: number
  /** cumulative active seconds (excluding pauses) */
  elapsed?: number
}

export interface Segment { s: number; e: number; p: number }

export interface Progress {
  id: string
  size: number
  downloaded: number
  speed: number
  /** seconds; -1 = unknown (size unknown), -2 = calculating */
  eta: number
  conns: number
  segments?: Segment[]
  extra?: Record<string, any>
  /** 0..1, low means the ETA is approximate */
  etaConfidence?: number
  /** active seconds, cumulative over sessions */
  elapsed?: number
  /** bytes/s averaged over the current session */
  avgSpeed?: number
  /** unix ms of the current session start */
  startedAt?: number
  /** effective per-item limit in bytes/s (0 = none) */
  limit?: number
}

export interface Totals { speed: number; active: number; queued: number; remaining: number; eta: number }

export interface UIInfo { glass: boolean; platform: string; macInset: boolean; frameless?: boolean; maximised?: boolean }

export type CategoryKey = 'video' | 'audio' | 'archive' | 'doc' | 'app' | 'image' | 'torrent' | 'program' | 'other' | ''

export interface Category { name: string; exts: string[]; dir: string; key?: CategoryKey }
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
  autoConns: boolean
  accelerateYoutube: boolean
  hostConns?: Record<string, number>
  remote?: { enabled: boolean; port: number; devices: RemoteDevice[] }
  maxConnsPerHost?: number
  maxActivePerHost?: number
  autoActive?: boolean
  maxActiveGlobal?: number
  fairShare?: boolean
  nativeTitleBar?: boolean
  extVideoSkipSites?: string[]
}

export interface RemoteDevice { id: string; name: string; createdAt: number; lastSeen: number }
export interface RemoteAddress { label: string; url: string; qr?: string }
export interface RemoteStatus { enabled: boolean; running: boolean; port: number; addresses: RemoteAddress[] | null; devices: RemoteDevice[] | null }

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
  priority?: number
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

export interface Quality { id: string; label: string; height?: number; bandwidth?: number; sizeEstimate?: number; ext: string }
export interface Inspection {
  title: string; duration: number; qualities: Quality[]; defaultQuality: string
  outputExt: string; fileName: string; live: boolean; thumbnail?: string
}
export interface VideoDetails { info: VideoInfo; inspection: Inspection }

export interface TorrentInfo { name: string; infoHash: string; size: number; files?: { index: number; path: string; size: number }[] }

export interface AppEvent {
  type: 'added' | 'updated' | 'removed' | 'progress' | 'completed' | 'failed' | 'settings'
  item?: Item
  id?: string
  progress?: Progress
}
