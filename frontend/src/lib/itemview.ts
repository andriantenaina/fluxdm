// Presentation helpers shared by the list, the details pane and the progress window.
import * as fmt from './format'
import { t } from './i18n.svelte'
import type { Item, Progress, Status } from './types'

export const displayName = (it: Item) => it.fileName || it.title || it.url

export const isRunning = (s: Status) => s === 'downloading' || s === 'processing'

export function statusLabel(s: Status): string {
  return t(('s_' + s) as 's_queued') as string
}

export function kindLabel(k: string): string {
  return ({ hls: 'HLS', dash: 'DASH', ytdlp: 'yt-dlp', torrent: 'BitTorrent', hoster: t('hoster') as string, ftp: 'FTP', http: 'HTTP' } as Record<string, string>)[k] ?? k
}

export function priorityLabel(p: number | undefined): string {
  return (p ?? 0) > 0 ? (t('prioHigh') as string) : (p ?? 0) < 0 ? (t('prioLow') as string) : (t('prioNormal') as string)
}

/** Short status text of a row: speed for running items, else the state. */
export function statusText(it: Item, p: Progress | undefined): string {
  if (it.status === 'downloading') {
    if (p?.extra?.phase === 'metadata') return t('metadata') as string
    return fmt.speed(p?.speed)
  }
  if (it.status === 'processing') return t('assembling') as string
  if (it.status === 'error') return it.error || statusLabel('error')
  return statusLabel(it.status)
}

/** ETA input for fmtEta, tolerant of older backends. */
export function etaOf(it: Item, p: Progress | undefined): fmt.EtaInput | null {
  if (it.status !== 'downloading' || !p) return null
  return { eta: p.eta, etaConfidence: p.etaConfidence, speed: p.speed }
}

/** Effective queue position: manual order, else creation time. */
export function queueKey(it: Item): number {
  return it.order && it.order > 0 ? it.order : Date.parse(it.createdAt) || 0
}

/** Elapsed active seconds (live while running). */
export function elapsedOf(it: Item, p: Progress | undefined): number {
  if (p?.elapsed != null && p.elapsed >= 0) return p.elapsed
  return it.elapsed ?? 0
}
