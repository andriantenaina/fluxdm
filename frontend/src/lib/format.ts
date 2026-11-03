const UNITS_FR = ['o', 'Ko', 'Mo', 'Go', 'To']
const UNITS_EN = ['B', 'KB', 'MB', 'GB', 'TB']
let units = UNITS_FR

export function setFormatLang(lang: string) {
  units = lang === 'en' ? UNITS_EN : UNITS_FR
}

export function size(n: number | undefined | null): string {
  if (n == null || n < 0) return '?'
  if (n === 0) return '0 ' + units[0]
  let i = 0
  let f = n
  while (f >= 1024 && i < units.length - 1) { f /= 1024; i++ }
  return (i === 0 ? f.toFixed(0) : f.toFixed(f < 10 ? 2 : 1)) + ' ' + units[i]
}

export function speed(bps: number | undefined): string {
  if (!bps || bps < 1) return '—'
  return size(bps) + '/s'
}

export function duration(sec: number | undefined | null): string {
  if (sec == null || sec < 0 || !isFinite(sec)) return '—'
  sec = Math.round(sec)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (h > 0) return `${h} h ${String(m).padStart(2, '0')} min`
  if (m > 0) return `${m} min ${String(s).padStart(2, '0')} s`
  return `${s} s`
}

export function clock(sec: number): string {
  if (!sec || sec < 0) return ''
  sec = Math.round(sec)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(s).padStart(2, '0')
}

export function percent(done: number, total: number): number {
  if (!total || total <= 0) return 0
  return Math.min(100, (done * 100) / total)
}

export function date(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function hostOf(u: string): string {
  try { return new URL(u).hostname } catch { return '' }
}

export function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}
