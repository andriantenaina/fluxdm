// FluxDM — helpers shared by background, content script, popup, side panel and options.
// Works as a classic script (self.FDM) and as a CommonJS module (tests): every
// chrome.* / DOM access is guarded so the module loads in node.
;(function (root) {
  'use strict'

  // ------------------------------------------------------------ language

  /** 'fr' or 'en' from a BCP 47 tag. */
  function langOf(tag) {
    return /^fr\b/i.test(String(tag || '')) ? 'fr' : 'en'
  }

  /** Browser UI language ('fr' | 'en'); 'fr' (default_locale) outside the browser. */
  function uiLang() {
    try {
      if (typeof chrome !== 'undefined' && chrome.i18n && chrome.i18n.getUILanguage) return langOf(chrome.i18n.getUILanguage())
    } catch (e) { /* not in an extension */ }
    return 'fr'
  }

  // Small built-in strings so formatting helpers stay usable in node.
  const T = {
    fr: { units: ['o', 'Ko', 'Mo', 'Go', 'To'], calc: 'Calcul…', at: 'vers', video: 'Vidéo', s: 's', min: 'min', h: 'h' },
    en: { units: ['B', 'KB', 'MB', 'GB', 'TB'], calc: 'Calculating…', at: 'at', video: 'Video', s: 's', min: 'min', h: 'h' },
  }
  const tr = (lang) => T[lang === 'en' || lang === 'fr' ? lang : langOf(lang)] || T.fr

  // ------------------------------------------------------------ titles & names

  const SUFFIX = /\s*[-|–—•·:]\s*(youtube|vimeo|dailymotion|facebook|instagram|tiktok|twitter|x|twitch|reddit|voe|doodstream|dood|uqload|vidmoly|filemoon|streamwish|streamtape|mixdrop|watch\s+online|streaming|regarder.*|en streaming.*|vf|vostfr)\s*$/i
  const PREFIX = /^\s*(watch|regarder|ver|play|lecture)\s+/i
  const EXT = /\.(mp4|mkv|webm|avi|mov|m3u8|ts|flv)$/i
  const GENERIC = new Set(['', 'video', 'embed', 'player', 'untitled', 'watch', 'index', 'master', 'playlist', 'videoplayback', 'manifest', 'chunklist', 'stream'])

  function cleanTitle(t) {
    t = String(t || '').trim().replace(/^\(\d+\)\s*/, '')
    for (let i = 0; i < 3; i++) {
      const n = t.replace(SUFFIX, '')
      if (n === t) break
      t = n
    }
    t = t.replace(PREFIX, '').replace(EXT, '').replace(/[\u0000-\u001f​﻿]/g, '').replace(/_/g, ' ').replace(/\s+/g, ' ').trim()
    if (GENERIC.has(t.toLowerCase())) return ''
    return t.length > 120 ? t.slice(0, 120).trim() : t
  }

  const MIME_EXT = {
    'video/mp4': 'mp4', 'video/webm': 'webm', 'video/x-matroska': 'mkv', 'video/quicktime': 'mov', 'video/x-flv': 'flv',
    'video/mp2t': 'ts', 'video/3gpp': '3gp', 'audio/mpeg': 'mp3', 'audio/mp4': 'm4a', 'audio/x-m4a': 'm4a',
    'audio/ogg': 'ogg', 'audio/opus': 'opus', 'audio/aac': 'aac', 'audio/wav': 'wav', 'audio/flac': 'flac', 'audio/webm': 'weba',
  }

  function extFromMime(m) {
    return MIME_EXT[String(m || '').split(';')[0].trim().toLowerCase()] || ''
  }

  function extOfUrl(u) {
    try {
      const m = /\.([a-z0-9]{1,5})$/i.exec(new URL(u).pathname)
      return m ? m[1].toLowerCase() : ''
    } catch (e) {
      return ''
    }
  }

  /** File name from a Content-Disposition header value. */
  function dispositionName(cd) {
    if (!cd) return ''
    let m = /filename\*\s*=\s*([^']*)''([^;]+)/i.exec(cd)
    if (m) {
      try { return decodeURIComponent(m[2].trim().replace(/^"|"$/g, '')) } catch (e) { /* fall through */ }
    }
    m = /filename\s*=\s*"?([^";]+)"?/i.exec(cd)
    return m ? m[1].trim() : ''
  }

  function sanitize(name) {
    return String(name || '').replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_').replace(/\s+/g, ' ').trim().replace(/^\.+|\.+$/g, '')
  }

  /** "Title (1080p).mp4" */
  function finalName(title, label, ext) {
    let t = cleanTitle(title) || 'video'
    if (label && label !== 'Auto' && label !== 'Original' && label !== 'Audio') t += ` (${label})`
    return sanitize(`${t}.${ext || 'mp4'}`)
  }

  // ------------------------------------------------------------ formatting

  /** Size in base 1024: FR o/Ko/Mo/Go, EN B/KB/MB/GB. '' when unknown. */
  function fmtSize(n, approx, lang) {
    if (!n || n <= 0) return ''
    const u = tr(lang || uiLang()).units
    let i = 0
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
    return (approx ? '~' : '') + (i === 0 ? n.toFixed(0) : n.toFixed(n < 10 ? 1 : 0)) + ' ' + u[i]
  }

  function fmtSpeed(bps, lang) {
    return bps > 0 ? fmtSize(bps, false, lang) + '/s' : ''
  }

  function fmtDuration(s) {
    if (!s || s <= 0) return ''
    s = Math.round(s)
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60
    return (h ? h + ':' + String(m).padStart(2, '0') : m) + ':' + String(x).padStart(2, '0')
  }

  /**
   * Remaining time: "2 min 14 s" / "1 h 05 min" / "8 s"; "≈ " prefix when the
   * confidence is low (< 0.5); -2 = calculating, -1 (or unknown) = "—".
   */
  function fmtEta(eta, confidence, lang) {
    const t = tr(lang || uiLang())
    if (eta === -2) return t.calc
    if (eta == null || typeof eta !== 'number' || !isFinite(eta) || eta < 0) return '—'
    const s0 = Math.round(eta)
    const h = Math.floor(s0 / 3600), m = Math.floor((s0 % 3600) / 60), s = s0 % 60
    let out
    if (h > 0) out = `${h} ${t.h} ${String(m).padStart(2, '0')} ${t.min}`
    else if (m > 0) out = `${m} ${t.min} ${String(s).padStart(2, '0')} ${t.s}`
    else out = `${s} ${t.s}`
    return (typeof confidence === 'number' && confidence < 0.5 ? '≈ ' : '') + out
  }

  /** Finish clock: "vers 14:32" / "at 2:32 PM". '' when the ETA is unknown. */
  function fmtFinish(eta, lang, now) {
    if (typeof eta !== 'number' || !isFinite(eta) || eta <= 0) return ''
    const l = lang || uiLang()
    const d = new Date((now || Date.now()) + eta * 1000)
    let clock
    try {
      clock = new Intl.DateTimeFormat(l === 'fr' ? 'fr-FR' : 'en-US', { hour: 'numeric', minute: '2-digit' }).format(d)
    } catch (e) {
      clock = d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0')
    }
    return tr(l).at + ' ' + clock
  }

  function percent(done, size) {
    if (!size || size <= 0 || !done) return 0
    return Math.max(0, Math.min(100, (done * 100) / size))
  }

  // ------------------------------------------------------------ hosts & lists

  /** "https://www.Example.com:8080/x" → "example.com"; '' when not a host. */
  function normHost(input) {
    let s = String(input || '').trim().toLowerCase()
    if (!s) return ''
    s = s.replace(/^[a-z][a-z0-9+.-]*:\/\//, '').replace(/^\*?\./, '')
    s = s.split(/[/?#]/)[0].replace(/:\d+$/, '').replace(/\.$/, '')
    s = s.replace(/^www\./, '')
    if (s === 'localhost') return s
    if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(s) && !/^\d{1,3}(\.\d{1,3}){3}$/.test(s)) return ''
    if (s.split('.').some((p) => !p || p.startsWith('-') || p.endsWith('-') || p.length > 63)) return ''
    return s
  }

  /** Host of a URL without "www." ('' for non-web URLs). */
  function siteOf(url) {
    try {
      const u = new URL(url)
      if (!/^(https?|ftp):$/.test(u.protocol)) return ''
      return u.hostname.toLowerCase().replace(/^www\./, '')
    } catch (e) {
      return ''
    }
  }

  /** true when host equals or is a subdomain of an entry of list. */
  function hostMatches(host, list) {
    const h = String(host || '').toLowerCase()
    if (!h) return false
    return (list || []).some((s) => s && (h === s || h.endsWith('.' + s)))
  }

  /** Split free text on spaces, commas, semicolons and new lines. */
  function splitList(text) {
    return String(text || '').split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean)
  }

  /** Parse an extension list ("mp4, .zip exe") → { values, invalid }. */
  function parseExts(text) {
    const values = [], invalid = []
    for (const raw of splitList(text)) {
      const x = raw.replace(/^\*?\./, '').toLowerCase()
      if (/^[a-z0-9]{1,10}$/.test(x)) { if (!values.includes(x)) values.push(x) } else invalid.push(raw)
    }
    return { values, invalid }
  }

  /** Parse a host list → { values, invalid }. */
  function parseHosts(text) {
    const values = [], invalid = []
    for (const raw of splitList(text)) {
      const h = normHost(raw)
      if (h) { if (!values.includes(h)) values.push(h) } else invalid.push(raw)
    }
    return { values, invalid }
  }

  /** Browser-internal pages where nothing can be captured or injected. */
  function isRestrictedUrl(url) {
    const u = String(url || '')
    if (!u) return true
    if (!/^(https?|ftp|file):/i.test(u)) return true
    return /^https:\/\/(chrome\.google\.com\/webstore|chromewebstore\.google\.com|microsoftedge\.microsoft\.com\/addons|addons\.opera\.com)/i.test(u)
  }

  // ------------------------------------------------------------ media

  /**
   * Group raw detected entries into "videos": an HLS/DASH manifest absorbs the
   * other playlists of the same frame/host detected within 6 s (variants,
   * audio renditions); direct files are deduplicated by URL without range params.
   */
  function groupMedia(list) {
    const out = []
    const sorted = [...(list || [])].sort((a, b) => a.time - b.time)
    for (const m of sorted) {
      let host = ''
      try { host = new URL(m.url).host } catch (e) { /* */ }
      if (m.kind === 'hls' || m.kind === 'dash') {
        const g = out.find((x) => (x.kind === 'hls' || x.kind === 'dash') && x.frameId === m.frameId && x.host === host && Math.abs(m.time - x.time) < 6000)
        if (g) { g.extra = (g.extra || 0) + 1; continue }
      }
      if (out.some((x) => x.key === m.key)) continue
      out.push({ ...m, host })
    }
    return out.sort((a, b) => b.time - a.time)
  }

  /** Short labels for a media entry (before / after inspection). */
  function describe(m, info, lang) {
    const t = tr(lang || uiLang())
    const kindLabel = { hls: 'HLS', dash: 'DASH', page: 'yt-dlp' }[m.kind] || (extFromMime(m.mime) || extOfUrl(m.url) || t.video).toUpperCase()
    if (info && !info.error) {
      const q = (info.qualities || []).find((x) => x.id === info.defaultQuality) || (info.qualities || [])[0] || {}
      return {
        title: info.title || cleanTitle(m.title) || t.video,
        quality: q.label && q.label !== 'Original' && q.label !== 'Auto' ? q.label : '',
        format: (info.outputExt || q.ext || 'mp4').toUpperCase(),
        size: fmtSize(q.sizeEstimate || m.size, m.kind !== 'direct', lang),
        fileName: info.fileName,
        source: kindLabel,
        duration: fmtDuration(info.duration),
        live: !!info.live,
      }
    }
    const ext = m.kind === 'direct' ? (extFromMime(m.mime) || extOfUrl(m.url) || 'mp4') : 'mp4'
    return {
      title: cleanTitle(m.title) || cleanTitle(m.fileName) || t.video,
      quality: '',
      format: ext.toUpperCase(),
      size: fmtSize(m.size, false, lang),
      fileName: m.fileName || finalName(m.title, '', ext),
      source: kindLabel,
      duration: '',
      live: false,
    }
  }

  // ------------------------------------------------------------ icons (Fluent-style, 20 px grid, 1.5 stroke)

  const P = {
    download: '<path d="M10 3v10M5.5 8.5 10 13l4.5-4.5M4 16.5h12"/>',
    chevronDown: '<path d="m5.5 8 4.5 4.5L14.5 8"/>',
    close: '<path d="m5 5 10 10M15 5 5 15"/>',
    open: '<path d="M11 3.5h5.5V9M16.5 3.5 9.5 10.5M8 4.5H5.5a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V12"/>',
    video: '<rect x="2.5" y="5" width="11" height="10" rx="2"/><path d="m13.5 9 4-2.5v7l-4-2.5"/>',
    links: '<path d="M8.5 11.5a3 3 0 0 0 4.2 0l2.6-2.6a3 3 0 0 0-4.2-4.2l-.9.9M11.5 8.5a3 3 0 0 0-4.2 0l-2.6 2.6a3 3 0 0 0 4.2 4.2l.9-.9"/>',
    pause: '<path d="M7 4.5v11M13 4.5v11"/>',
    play: '<path d="M6.5 4.5v11l9-5.5z"/>',
    folder: '<path d="M2.5 6a2 2 0 0 1 2-2h3l2 2h6a2 2 0 0 1 2 2v6.5a2 2 0 0 1-2 2h-11a2 2 0 0 1-2-2z"/>',
    settings: '<circle cx="10" cy="10" r="2.5"/><path d="M10 2.5v2M10 15.5v2M2.5 10h2M15.5 10h2M4.7 4.7l1.4 1.4M13.9 13.9l1.4 1.4M4.7 15.3l1.4-1.4M13.9 6.1l1.4-1.4"/>',
    panel: '<rect x="2.5" y="3.5" width="15" height="13" rx="2"/><path d="M12.5 3.5v13"/>',
    check: '<path d="m4.5 10.5 3.5 3.5 7.5-8"/>',
    error: '<circle cx="10" cy="10" r="7.5"/><path d="M10 6v5M10 13.8v.2"/>',
    power: '<path d="M10 2.5v7M6 5a6 6 0 1 0 8 0"/>',
    eyeOff: '<path d="M3 3l14 14M8.6 8.6a2 2 0 0 0 2.8 2.8M6 6.2C4.3 7.3 3 9 2.5 10c1.2 2.5 4.2 5 7.5 5 1.4 0 2.7-.4 3.8-1.1M9 5.1c.3 0 .7-.1 1-.1 3.3 0 6.3 2.5 7.5 5-.4.8-1 1.7-1.8 2.5"/>',
    block: '<circle cx="10" cy="10" r="7.5"/><path d="m4.7 4.7 10.6 10.6"/>',
    plug: '<path d="M7 2.5v4M13 2.5v4M5 6.5h10v3a5 5 0 0 1-10 0zM10 14.5v3"/>',
    info: '<circle cx="10" cy="10" r="7.5"/><path d="M10 9v5M10 6.2v.2"/>',
  }

  /** Inline SVG markup (decorative: aria-hidden). Constant strings only. */
  function icon(name, size) {
    const s = size || 20
    return `<svg class="ic" width="${s}" height="${s}" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">${P[name] || P.info}</svg>`
  }

  // ------------------------------------------------------------ pages

  /** chrome.i18n.getMessage with a fallback ('' outside the extension). */
  function msg(key, subs, fallback) {
    try {
      if (typeof chrome !== 'undefined' && chrome.i18n) {
        const v = chrome.i18n.getMessage(key, subs)
        if (v) return v
      }
    } catch (e) { /* */ }
    return fallback != null ? fallback : key
  }

  /** Fill [data-i18n], [data-i18n-title], [data-i18n-aria], [data-i18n-ph]. */
  function localize(rootEl) {
    if (!rootEl || !rootEl.querySelectorAll) return
    for (const e of rootEl.querySelectorAll('[data-i18n]')) e.textContent = msg(e.dataset.i18n)
    for (const e of rootEl.querySelectorAll('[data-i18n-title]')) e.title = msg(e.dataset.i18nTitle)
    for (const e of rootEl.querySelectorAll('[data-i18n-aria]')) e.setAttribute('aria-label', msg(e.dataset.i18nAria))
    for (const e of rootEl.querySelectorAll('[data-i18n-ph]')) e.setAttribute('placeholder', msg(e.dataset.i18nPh))
    for (const e of rootEl.querySelectorAll('[data-icon]')) e.innerHTML = icon(e.dataset.icon, +e.dataset.iconSize || 20)
  }

  const api = {
    langOf, uiLang, cleanTitle, extFromMime, extOfUrl, dispositionName, sanitize, finalName,
    fmtSize, fmtSpeed, fmtDuration, fmtEta, fmtFinish, percent,
    normHost, siteOf, hostMatches, splitList, parseExts, parseHosts, isRestrictedUrl,
    groupMedia, describe, icon, ICON_NAMES: Object.keys(P), msg, localize,
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = api
  root.FDM = api
})(typeof self !== 'undefined' ? self : globalThis)
