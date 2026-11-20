// Locale-aware formatting. The language is pushed by i18n.setLang (no import
// cycle): FR uses o/Ko/Mo/Go and a decimal comma, EN uses B/KB/MB/GB.

const UNITS: Record<string, string[]> = {
  fr: ['o', 'Ko', 'Mo', 'Go', 'To'],
  en: ['B', 'KB', 'MB', 'GB', 'TB'],
}
const WORDS: Record<string, Record<string, string>> = {
  fr: { calc: 'Calcul…', left: 'restante', leftPl: 'restantes', at: 'vers', d: 'j', h: 'h', min: 'min', s: 's' },
  en: { calc: 'Calculating…', left: 'left', leftPl: 'left', at: 'at', d: 'd', h: 'h', min: 'min', s: 's' },
}

let lang = 'fr'
let nf: Record<number, Intl.NumberFormat> = {}

export function setFormatLang(l: string) {
  lang = UNITS[l] ? l : 'fr'
  nf = {}
}
export const formatLang = () => lang

function num(n: number, digits: number): string {
  nf[digits] ??= new Intl.NumberFormat(lang, { minimumFractionDigits: digits, maximumFractionDigits: digits })
  return nf[digits].format(n)
}

/** Narrow no-break space between number and unit (as Windows Explorer does). */
const NB = ' '

export function size(n: number | undefined | null): string {
  if (n == null || n < 0) return '?'
  const units = UNITS[lang]
  if (n === 0) return '0' + NB + units[0]
  let i = 0
  let f = n
  while (f >= 1024 && i < units.length - 1) { f /= 1024; i++ }
  return (i === 0 ? num(f, 0) : num(f, f < 10 ? 2 : f < 100 ? 1 : 0)) + NB + units[i]
}

export function speed(bps: number | undefined | null): string {
  if (!bps || bps < 1) return '—'
  return size(bps) + '/s'
}

/** Short unit for a size slider ("Mo/s" / "MB/s"). */
export function unit(i: number): string {
  return UNITS[lang][i]
}

/** "2 min 14 s" / "1 h 05 min" / "8 s" / "2 j 03 h". */
export function duration(sec: number | undefined | null): string {
  if (sec == null || sec < 0 || !isFinite(sec)) return '—'
  const w = WORDS[lang]
  sec = Math.round(sec)
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (d > 0) return `${d}${NB}${w.d} ${String(h).padStart(2, '0')}${NB}${w.h}`
  if (h > 0) return `${h}${NB}${w.h} ${String(m).padStart(2, '0')}${NB}${w.min}`
  if (m > 0) return `${m}${NB}${w.min} ${String(s).padStart(2, '0')}${NB}${w.s}`
  return `${s}${NB}${w.s}`
}

export interface EtaInput { eta?: number | null; etaConfidence?: number | null; speed?: number | null }

/**
 * ETA following the shared contract: -2 → "Calcul…", -1/unknown → "—",
 * otherwise a duration, prefixed by "≈" when the confidence is low.
 */
export function fmtEta(p: EtaInput | null | undefined): string {
  if (!p || p.eta == null) return '—'
  if (p.eta === -2) return WORDS[lang].calc
  if (p.eta < 0 || !isFinite(p.eta)) return '—'
  // older backends report 0 when the speed is 0
  if (p.eta === 0 && !(p.speed && p.speed > 0)) return '—'
  const approx = p.etaConfidence != null && p.etaConfidence < 0.5 ? '≈' + NB : ''
  return approx + duration(p.eta)
}

/** "2 min 14 s restantes" / "2 min 14 s left" (or the bare state text). */
export function fmtEtaLong(p: EtaInput | null | undefined): string {
  const s = fmtEta(p)
  if (!p || p.eta == null || p.eta < 0 || s === '—') return s
  return lang === 'fr' ? `${s} ${p.eta >= 2 ? WORDS.fr.leftPl : WORDS.fr.left}` : `${s} ${WORDS.en.left}`
}

/** Clock time when a download finishing in `etaSec` seconds will end: "vers 14:32" / "at 2:32 PM". */
export function fmtClock(etaSec: number | null | undefined, now = Date.now()): string {
  if (etaSec == null || etaSec <= 0 || !isFinite(etaSec)) return ''
  const end = new Date(now + etaSec * 1000)
  const sameDay = new Date(now).toDateString() === end.toDateString()
  const opts: Intl.DateTimeFormatOptions = sameDay ? { hour: 'numeric', minute: '2-digit' } : { weekday: 'short', hour: 'numeric', minute: '2-digit' }
  return `${WORDS[lang].at} ${new Intl.DateTimeFormat(lang, opts).format(end)}`
}

/** Media duration "1:02:03" / "4:05". */
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

/** Percentage text "42 %" (FR) / "42%" (EN). */
export function pct(v: number, digits = 0): string {
  return lang === 'fr' ? num(v, digits) + NB + '%' : num(v, digits) + '%'
}

export function date(iso?: string | number | Date): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat(lang, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }).format(d)
}

/** "il y a 5 min" / "5 min ago" for recent dates, otherwise a date. */
export function relative(iso?: string | number | Date): string {
  if (!iso) return ''
  const d = new Date(iso)
  const diff = (Date.now() - d.getTime()) / 1000
  if (isNaN(diff)) return ''
  if (diff < 0 || diff > 7 * 86400 || typeof Intl.RelativeTimeFormat !== 'function') return date(d)
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto' })
  if (diff < 60) return rtf.format(-Math.round(diff), 'second')
  if (diff < 3600) return rtf.format(-Math.round(diff / 60), 'minute')
  if (diff < 86400) return rtf.format(-Math.round(diff / 3600), 'hour')
  return rtf.format(-Math.round(diff / 86400), 'day')
}

export function hostOf(u: string): string {
  try { return new URL(u).hostname } catch { return '' }
}

export function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

/** Validates a download address; returns '' when valid or an error key. */
export function urlProblem(u: string): '' | 'empty' | 'scheme' | 'invalid' {
  const s = u.trim()
  if (!s) return 'empty'
  if (/^magnet:\?/i.test(s)) return ''
  // local .torrent files dropped or chosen in a dialog
  if (/^([a-z]:[\\/]|\/|\\\\).+\.torrent$/i.test(s)) return ''
  const m = /^([a-z][a-z0-9+.-]*):/i.exec(s)
  if (!m) return 'scheme'
  if (!/^(https?|ftp|ftps|sftp)$/i.test(m[1])) return 'scheme'
  try {
    const x = new URL(s)
    if (!x.hostname) return 'invalid'
  } catch { return 'invalid' }
  return ''
}
