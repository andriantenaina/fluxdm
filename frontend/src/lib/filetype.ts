import type { Category, Item, Settings } from './types'

export type FileKind = 'video' | 'audio' | 'archive' | 'doc' | 'app' | 'image' | 'torrent' | 'other'

const BY_EXT: Record<string, FileKind> = {}
const groups: Record<FileKind, string> = {
  video: 'mp4 mkv avi mov webm flv wmv ts m4v 3gp mpg mpeg ogv m3u8 mpd',
  audio: 'mp3 m4a aac flac wav ogg opus wma aif aiff weba',
  archive: 'zip rar 7z gz tgz tar bz2 xz zst iso cab',
  doc: 'pdf doc docx xls xlsx ppt pptx odt ods odp txt rtf epub csv',
  app: 'exe msi msix appx dmg pkg apk deb rpm appimage jar',
  image: 'jpg jpeg png gif webp bmp tif tiff svg heic avif',
  torrent: 'torrent',
  other: '',
}
for (const [k, list] of Object.entries(groups)) for (const e of list.split(' ')) if (e) BY_EXT[e] = k as FileKind

export const KIND_COLOR: Record<FileKind, string> = {
  video: 'var(--c-video)', audio: 'var(--c-audio)', archive: 'var(--c-archive)', doc: 'var(--c-doc)',
  app: 'var(--c-app)', image: 'var(--c-image)', torrent: 'var(--c-torrent)', other: 'var(--c-other)',
}

// Default category names (FR and EN) mapped to their stable key, for settings
// written before Category.key existed.
const NAME_KEY: Record<string, FileKind> = {
  'vidéos': 'video', videos: 'video', video: 'video', musique: 'audio', music: 'audio', audio: 'audio',
  'compressés': 'archive', compressed: 'archive', archives: 'archive', documents: 'doc', programmes: 'app', programs: 'app',
  applications: 'app', images: 'image', pictures: 'image', torrents: 'torrent',
}

/** Stable kind of a category (Category.key, else its default name). */
export function categoryKind(c: Pick<Category, 'name' | 'key'> | undefined | null): FileKind {
  if (!c) return 'other'
  if (c.key) return c.key === 'program' ? 'app' : ((c.key in KIND_COLOR ? c.key : 'other') as FileKind)
  return NAME_KEY[(c.name || '').toLowerCase()] ?? 'other'
}

/** Finds the category of a given kind ('video', 'audio'...) in the settings. */
export function categoryOfKind(s: Settings | null | undefined, kind: FileKind): Category | undefined {
  return s?.categories?.find((c) => categoryKind(c) === kind)
}

/** Kind of the category named `name` in the settings. */
export function kindOfCategoryName(s: Settings | null | undefined, name: string): FileKind {
  if (!name) return 'other'
  return categoryKind(s?.categories?.find((c) => c.name === name) ?? { name })
}

export function extOfName(name: string): string {
  const m = /\.([a-z0-9]{1,6})$/i.exec(name || '')
  return m ? m[1].toLowerCase() : ''
}

/** Extension + kind to display for an item (streams show their output format). */
export function fileTypeOf(it: Item, settings?: Settings | null): { ext: string; kind: FileKind; color: string } {
  let ext = extOfName(it.fileName || '')
  if (!ext) {
    if (it.kind === 'torrent') ext = 'torrent'
    else if (it.kind === 'hls' || it.kind === 'dash' || it.kind === 'ytdlp' || it.kind === 'hoster') ext = (it.options?.output as string) || 'mp4'
  }
  let kind: FileKind = it.kind === 'torrent' ? 'torrent' : BY_EXT[ext] ?? 'other'
  if (kind === 'other' && it.category) {
    const k = kindOfCategoryName(settings, it.category)
    if (k === 'video' || k === 'audio') kind = k
  }
  return { ext: ext || 'file', kind, color: KIND_COLOR[kind] }
}
