import { t as tr, tf } from '../lib/i18n.svelte'

// HTTP client of FluxDM Remote (phone side). Auth: device token kept in
// localStorage (the home-screen app on iOS has its own storage) + cookie.

export interface LiteItem {
  id: string
  kind: string
  name: string
  status: string
  size: number
  downloaded: number
  category: string
  error?: string
  host: string
  createdAt: string
  completedAt?: string
  ext: string
}

export interface LiteProgress { id: string; size: number; downloaded: number; speed: number; eta: number; conns: number; etaConfidence?: number }

export interface Quality { id: string; label: string; height?: number; bandwidth?: number; sizeEstimate?: number; ext: string }
export interface Inspection {
  title: string
  duration: number
  qualities: Quality[]
  defaultQuality: string
  outputExt: string
  fileName: string
  live: boolean
  thumbnail?: string
}

export interface AddReq {
  url: string
  pageUrl?: string
  title?: string
  kind?: string
  quality?: string
  output?: string
  fileName?: string
  referer?: string
}

const KEY = 'fdm_token'

function storedToken(): string {
  try { return localStorage.getItem(KEY) || '' } catch { return '' }
}

/** Picks up ?k=<token> from the pairing redirect and remembers it. */
export function captureToken(): string {
  const k = new URLSearchParams(location.search).get('k')
  if (k) {
    try { localStorage.setItem(KEY, k) } catch { /* private mode */ }
  }
  return k || storedToken()
}

export const token = () => storedToken() || new URLSearchParams(location.search).get('k') || ''

export class Unauthorized extends Error {}

async function req<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const t = token()
  if (t) headers.Authorization = 'Bearer ' + t
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const r = await fetch(path, {
    method: body === undefined ? 'GET' : 'POST',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  if (r.status === 401) throw new Unauthorized(tr('rmErrUnpaired'))
  const data = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error((data && data.error) || tf('rmErrHttp', { code: r.status }))
  return data as T
}

const withToken = (path: string) => {
  const t = token()
  return t ? path + (path.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(t) : path
}

export const remote = {
  info: () => req<{ version: string; device: string; base: string }>('/api/info'),
  items: () => req<LiteItem[]>('/api/items'),
  add: (a: AddReq) => req<LiteItem>('/api/add', a),
  analyze: (url: string, kind = '', title = '', referer = '') =>
    req<{ kind: string; inspection: Inspection }>('/api/analyze', { url, kind, title, referer }),
  pause: (ids: string[]) => req('/api/pause', { ids }),
  resume: (ids: string[]) => req('/api/resume', { ids }),
  remove: (ids: string[], deleteFile = false) => req('/api/remove', { ids, deleteFile }),
  bookmarklet: () => req<{ code: string }>('/api/bookmarklet'),
  fileURL: (id: string) => withToken('/files/' + encodeURIComponent(id)),
  events: () => new EventSource(withToken('/api/events')),
}

/** What the bookmarklet / Share shortcut sent: /add?d=<base64 json> or ?u=&t=&p= */
export interface Incoming { page: string; title: string; media: string[]; frames: string[] }

export function parseIncoming(query: string): Incoming | null {
  const q = new URLSearchParams(query)
  const d = q.get('d')
  if (d) {
    try {
      const bin = atob(d.replace(/-/g, '+').replace(/_/g, '/'))
      const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0))
      const j = JSON.parse(new TextDecoder().decode(bytes))
      return {
        page: String(j.p || ''),
        title: String(j.t || ''),
        media: Array.isArray(j.m) ? j.m.map(String) : [],
        frames: Array.isArray(j.f) ? j.f.map(String) : [],
      }
    } catch {
      return null
    }
  }
  const u = q.get('u') || q.get('url')
  if (!u) return null
  return { page: q.get('p') || u, title: q.get('t') || '', media: u !== (q.get('p') || u) ? [u] : [], frames: [] }
}

/** Kind sent to /api/analyze for a candidate URL. */
export function kindOf(u: string, isPage: boolean): string {
  const p = u.split('?')[0].toLowerCase()
  if (p.endsWith('.m3u8')) return 'hls'
  if (p.endsWith('.mpd')) return 'dash'
  if (/\.(mp4|webm|m4a|mov|mkv|mp3)$/.test(p)) return 'http'
  return isPage ? 'ytdlp' : ''
}
