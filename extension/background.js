// FluxDM browser integration — service worker (Manifest V3, Chromium/Opera).
// - finds the FluxDM app (native messaging host, else default local port)
// - captures downloads and hands them to FluxDM (browser fallback + notification)
// - sniffs network responses to detect videos (HLS .m3u8, DASH .mpd, MP4/WebM...)
// - context menus: download link / media / all links / page video / side panel
// - badge: media count (blue), "!" when FluxDM is unreachable (orange), grey when capture is off
//
// Firefox: manifest.json carries browser_specific_settings (gecko id) but no
// background.scripts fallback: Chrome only tolerates it next to service_worker
// from Chrome 121, and minimum_chrome_version is 102 (older Chromium/Opera
// would refuse to load the extension).
'use strict'

try { importScripts('common.js') } catch (e) { /* Firefox/Safari: loaded via manifest */ }
const FDM = self.FDM
const M = (k, subs) => FDM.msg(k, subs)

// Safari (macOS) lacks some Chromium APIs: inert stand-ins keep the worker
// alive; videos are then detected by the content script only.
;(() => {
  const ev = { addListener() {}, removeListener() {}, hasListener: () => false }
  const stub = (name, obj) => { try { if (!chrome[name]) chrome[name] = obj } catch (e) { /* frozen namespace */ } }
  stub('webRequest', { onSendHeaders: ev, onHeadersReceived: ev, onErrorOccurred: ev })
  stub('webNavigation', { onCommitted: ev, getFrame: async () => null })
  stub('cookies', { getAll: async () => [] })
  stub('contextMenus', { removeAll: (cb) => cb && cb(), create() {}, onClicked: ev })
})()

const HOST = 'com.fluxdm.host'
const DEFAULT_PORT = 52775
const MAX_MEDIA_PER_TAB = 30
const MIN_DIRECT_SIZE = 300 * 1024
const COLOR = { media: '#0f6cbd', off: '#616161', warn: '#bc4b09' }

// ---------------------------------------------------------------- state

let conn = null // { port, token }
/** Settings as last returned by the app (/v1/ping). */
let appSettings = {
  captureExts: [], captureMinSize: 0, skipSites: [], videoButton: true, enabled: true, ytdlpSites: [],
  captureEnabled: true, videoSkipSites: [],
}
/** Changes saved while the app was unreachable or lacked POST /v1/settings; pushed on the next successful ping. */
let overrides = {}
/** Effective settings = appSettings + overrides. */
let settings = { ...appSettings }
let captureEnabled = true // extension-side toggle (popup / options)
let prefs = { theme: 'system', notifications: true }
let connState = null // true / false / null (unknown)
let lastConnectError = '' // hostMissing | appMissing | noResponse
let lastConnectFail = 0
let lastPing = 0

function recompute() {
  settings = { ...appSettings, ...overrides }
}

const ready = (async () => {
  const s = await chrome.storage.session.get(['conn', 'connState', 'lastPing'])
  if (s.conn) conn = s.conn
  if (typeof s.connState === 'boolean') connState = s.connState
  lastPing = s.lastPing || 0
  const l = await chrome.storage.local.get(['settings', 'overrides', 'captureEnabled', 'prefs'])
  if (l.settings) appSettings = { ...appSettings, ...l.settings }
  if (l.overrides) overrides = l.overrides
  if (typeof l.captureEnabled === 'boolean') captureEnabled = l.captureEnabled
  if (l.prefs) prefs = { ...prefs, ...l.prefs }
  recompute()
})()

chrome.storage.onChanged.addListener((ch, area) => {
  if (area !== 'local') return
  if (ch.prefs) prefs = { theme: 'system', notifications: true, ...(ch.prefs.newValue || {}) }
  if (ch.captureEnabled) {
    captureEnabled = ch.captureEnabled.newValue !== false
    scheduleBadges()
  }
})

class FdmError extends Error {
  constructor(message, code, status) {
    super(message)
    this.code = code // unreachable | unsupported | http | app
    this.status = status || 0
  }
}

function setConnected(v) {
  if (connState === v) return
  connState = v
  chrome.storage.session.set({ connState: v }).catch(() => {})
  scheduleBadges()
}

async function connect(launch) {
  lastConnectError = ''
  try {
    const r = await chrome.runtime.sendNativeMessage(HOST, { type: launch ? 'launch' : 'hello' })
    if (r && r.ok) {
      conn = { port: r.port, token: r.token }
      await chrome.storage.session.set({ conn })
      return conn
    }
    lastConnectError = r && /introuvable|not found/i.test(r.error || '') ? 'appMissing' : 'noResponse'
  } catch (e) {
    // host not installed / not allowed: try the default port
    lastConnectError = 'hostMissing'
  }
  try {
    const res = await fetch(`http://127.0.0.1:${DEFAULT_PORT}/v1/hello`, { cache: 'no-store' })
    if (res.ok) {
      const j = await res.json()
      conn = { port: DEFAULT_PORT, token: j.token }
      lastConnectError = ''
      await chrome.storage.session.set({ conn })
      return conn
    }
  } catch (e) { /* app not running */ }
  conn = null
  lastConnectFail = Date.now()
  await chrome.storage.session.remove('conn')
  setConnected(false)
  return null
}

/**
 * Call the bridge. opts.launch: start FluxDM if needed (user actions);
 * opts.quiet: polling call, does not retry the connection more than every 5 s.
 * Throws FdmError (code unreachable | unsupported | http | app).
 */
async function request(path, body, opts) {
  const o = typeof opts === 'boolean' ? { launch: opts } : { launch: true, ...(opts || {}) }
  await ready
  if (!conn && o.quiet && Date.now() - lastConnectFail < 5000) throw new FdmError(M('errUnreachable'), 'unreachable')
  for (let attempt = 0; attempt < 3; attempt++) {
    if (!conn) await connect(attempt > 0 && o.launch)
    if (!conn) {
      if (o.quiet || !o.launch) break // retrying without launching would only fail again
      continue
    }
    let res
    try {
      res = await fetch(`http://127.0.0.1:${conn.port}${path}`, {
        method: body ? 'POST' : 'GET',
        headers: { 'Content-Type': 'application/json', 'X-FluxDM-Token': conn.token },
        body: body ? JSON.stringify(body) : undefined,
        cache: 'no-store',
      })
    } catch (e) {
      conn = null // network error: app closed or port changed
      continue
    }
    if (res.status === 401) { conn = null; continue }
    setConnected(true)
    const j = await res.json().catch(() => ({}))
    if (res.status === 404 || res.status === 405) throw new FdmError(M('errUnsupported'), 'unsupported', res.status)
    if (!res.ok) throw new FdmError(j.error || res.statusText || 'HTTP ' + res.status, j.error ? 'app' : 'http', res.status)
    if (j && j.ok === false && j.error) throw new FdmError(j.error, 'app', res.status)
    return j
  }
  setConnected(false)
  throw new FdmError(M('errUnreachable'), 'unreachable')
}

function errPayload(e) {
  return { error: String((e && e.message) || e), code: (e && e.code) || 'app', hint: e && e.code === 'unreachable' ? lastConnectError : '' }
}

async function persistSettings() {
  await chrome.storage.local.set({ settings: appSettings, overrides })
}

async function refreshSettings() {
  try {
    const s = await request('/v1/ping', null, { launch: false })
    appSettings = { ...appSettings, ...s }
    lastPing = Date.now()
    chrome.storage.session.set({ lastPing }).catch(() => {})
    if (Object.keys(overrides).length) {
      try {
        const r = await request('/v1/settings', overrides, { launch: false })
        appSettings = { ...appSettings, ...overrides, ...(r && r.captureExts ? r : {}) }
        overrides = {}
      } catch (e) { /* old app without POST /v1/settings: keep the local overrides */ }
    }
    recompute()
    await persistSettings()
    scheduleBadges()
    return true
  } catch (e) {
    return false
  }
}

/** Save a partial ExtSettings in the app (POST /v1/settings), locally when not possible. */
async function saveSettings(partial) {
  await ready
  try {
    const r = await request('/v1/settings', partial, { launch: false })
    appSettings = { ...appSettings, ...partial, ...(r && r.captureExts ? r : {}) }
    for (const k of Object.keys(partial)) delete overrides[k]
    recompute()
    await persistSettings()
    scheduleBadges()
    return { ok: true, settings }
  } catch (e) {
    if (e.code !== 'unsupported' && e.code !== 'unreachable') throw e
    overrides = { ...overrides, ...partial }
    recompute()
    await persistSettings()
    scheduleBadges()
    return { ok: true, local: true, reason: e.code, settings }
  }
}

/** Add (on) or remove (off) a host in skipSites / videoSkipSites. */
async function siteRule(list, host, on) {
  if (list !== 'skipSites' && list !== 'videoSkipSites') throw new FdmError('bad list', 'app')
  const h = FDM.normHost(host)
  if (!h) throw new FdmError(M('errBadHost'), 'app')
  const cur = (settings[list] || []).slice()
  let next
  if (on) next = FDM.hostMatches(h, cur) ? cur : [...cur, h]
  else next = cur.filter((s) => !(h === s || h.endsWith('.' + s)))
  const r = await saveSettings({ [list]: next })
  return { ...r, host: h, on: FDM.hostMatches(h, settings[list] || []) }
}

chrome.runtime.onStartup.addListener(() => {
  refreshSettings()
  createMenus()
})
chrome.runtime.onInstalled.addListener(() => {
  refreshSettings()
  createMenus()
  initSidePanel()
})
chrome.alarms.create('fluxdm-refresh', { periodInMinutes: 2 })
chrome.alarms.onAlarm.addListener((a) => { if (a.name === 'fluxdm-refresh') refreshSettings() })
// worker woken up: refresh the connection state at most once a minute
ready.then(() => { if (Date.now() - lastPing > 60 * 1000) refreshSettings() })

function initSidePanel() {
  try {
    if (chrome.sidePanel && chrome.sidePanel.setPanelBehavior) chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: false }).catch(() => {})
  } catch (e) { /* no side panel (Opera, Firefox) */ }
}

// ---------------------------------------------------------------- notifications

const notifTargets = new Map() // notification id -> { id?: item id }

function notify(title, message, target) {
  if (!chrome.notifications || prefs.notifications === false) return
  const id = 'fdm-' + Date.now() + '-' + Math.random().toString(36).slice(2, 7)
  notifTargets.set(id, target || {})
  if (notifTargets.size > 30) notifTargets.delete(notifTargets.keys().next().value)
  try {
    chrome.notifications.create(id, {
      type: 'basic', iconUrl: 'icons/icon-128.png', title, message: String(message || '').slice(0, 300), priority: 0,
    }, () => void chrome.runtime.lastError)
  } catch (e) { /* notifications unavailable */ }
}

async function showApp(id) {
  try {
    await request('/v1/show', id ? { id } : {}, { launch: true })
  } catch (e) {
    // old app without /v1/show, or not running: the native host brings it up
    await connect(true)
  }
}

if (chrome.notifications) {
  chrome.notifications.onClicked.addListener((nid) => {
    const t = notifTargets.get(nid) || {}
    notifTargets.delete(nid)
    chrome.notifications.clear(nid)
    showApp(t.id)
  })
  chrome.notifications.onClosed.addListener((nid) => notifTargets.delete(nid))
}

// ---------------------------------------------------------------- badge

function captureOn() {
  return captureEnabled && settings.enabled !== false && settings.captureEnabled !== false
}

async function tabCount(tabId) {
  return FDM.groupMedia(await tabMedia(tabId)).length
}

function badgeFor(n) {
  if (connState === false) return { text: '!', color: COLOR.warn, title: M('tipDisconnected') }
  const text = n ? String(n) : ''
  if (!captureOn()) return { text, color: COLOR.off, title: n ? M('tipCaptureOffMedia', [String(n)]) : M('tipCaptureOff') }
  return { text, color: COLOR.media, title: n ? M('tipMedia', [String(n)]) : M('tipConnected') }
}

function applyBadge(tabId, b) {
  const t = tabId != null ? { tabId } : {}
  const a = chrome.action
  if (!a) return
  a.setBadgeText({ ...t, text: b.text }).catch(() => {})
  a.setBadgeBackgroundColor({ ...t, color: b.color }).catch(() => {})
  if (a.setBadgeTextColor) a.setBadgeTextColor({ ...t, color: '#ffffff' }).catch(() => {})
  a.setTitle({ ...t, title: 'FluxDM — ' + b.title }).catch(() => {})
}

function updateBadge(tabId, n) {
  if (tabId == null || tabId < 0) return
  applyBadge(tabId, badgeFor(n))
}

let badgeTimer = 0
function scheduleBadges() {
  clearTimeout(badgeTimer)
  badgeTimer = setTimeout(async () => {
    applyBadge(null, badgeFor(0))
    try {
      for (const t of await chrome.tabs.query({})) updateBadge(t.id, await tabCount(t.id))
    } catch (e) { /* */ }
  }, 150)
}

// ---------------------------------------------------------------- helpers

async function cookiesFor(url) {
  try {
    const list = await chrome.cookies.getAll({ url })
    return list.map((c) => `${c.name}=${c.value}`).join('; ')
  } catch (e) {
    return ''
  }
}

function hostOf(u) {
  try { return new URL(u).hostname.toLowerCase() } catch (e) { return '' }
}

function extOf(s) {
  try {
    const p = new URL(s).pathname
    const m = /\.([a-z0-9]{1,6})$/i.exec(p)
    return m ? m[1].toLowerCase() : ''
  } catch (e) {
    const m = /\.([a-z0-9]{1,6})$/i.exec(s || '')
    return m ? m[1].toLowerCase() : ''
  }
}

function isSkipped(url) {
  return FDM.hostMatches(hostOf(url), settings.skipSites)
}

function isYtdlpPage(url) {
  const h = hostOf(url).replace(/^(www|m)\./, '')
  return (settings.ytdlpSites || []).some((s) => h === s || h.endsWith('.' + s))
}

function shortName(url, fileName) {
  if (fileName) return fileName
  try {
    const u = new URL(url)
    return decodeURIComponent(u.pathname.split('/').pop() || '') || u.hostname
  } catch (e) {
    return String(url || '').slice(0, 80)
  }
}

// ---------------------------------------------------------------- download capture

const ownDownloads = new Set() // downloads re-issued by us (fallback) must not be captured again
const recentlyCaptured = new Map()

function shouldCapture(item, url, filename) {
  if (!captureOn()) return false
  if (!/^(https?|ftp):/i.test(url)) return false // blob:, data:, file: cannot be handled outside the browser
  if (ownDownloads.has(item.id) || ownDownloads.has(url)) return false
  if (item.byExtensionId && item.byExtensionId === chrome.runtime.id) return false
  if (isSkipped(url) || isSkipped(item.referrer || '')) return false
  const size = item.fileSize > 0 ? item.fileSize : item.totalBytes
  if (settings.captureMinSize > 0 && size > 0 && size < settings.captureMinSize) return false
  const ext = extOf(filename || '') || extOf(url)
  const exts = settings.captureExts || []
  if (ext && exts.includes(ext)) return true
  const mime = (item.mime || '').toLowerCase()
  if (!ext && (mime.startsWith('application/') && !mime.includes('json') && !mime.includes('xml') && !mime.includes('javascript'))) return true
  return false
}

async function capture(item, filename) {
  const url = item.finalUrl || item.url
  if (recentlyCaptured.has(url) && Date.now() - recentlyCaptured.get(url) < 3000) return
  recentlyCaptured.set(url, Date.now())
  try { await chrome.downloads.cancel(item.id) } catch (e) { /* already gone */ }
  try { await chrome.downloads.erase({ id: item.id }) } catch (e) { /* ignore */ }
  const name = (filename || '').split(/[\\/]/).pop()
  const payload = {
    url,
    fileName: name || '',
    size: item.fileSize > 0 ? item.fileSize : item.totalBytes > 0 ? item.totalBytes : 0,
    mime: item.mime || '',
    referer: item.referrer || '',
    pageUrl: item.referrer || '',
    cookies: await cookiesFor(url),
    userAgent: navigator.userAgent,
  }
  try {
    await request('/v1/download', payload)
  } catch (e) {
    // FluxDM unavailable: give the download back to the browser, and say so
    const id = await chrome.downloads.download({ url, filename: name || undefined }).catch(() => null)
    if (id != null) ownDownloads.add(id)
    ownDownloads.add(url)
    setTimeout(() => ownDownloads.delete(url), 10000)
    notify(
      e.code === 'unreachable' ? M('notifFallbackTitle') : M('notifFailTitle'),
      M('notifFallbackBody', [shortName(url, name), String(e.message || e)]),
      {},
    )
  }
}

// Safari has no downloads API: capture is then done by the content script / popup only.
if (!chrome.downloads) {
  /* nothing to hook */
} else if (chrome.downloads.onDeterminingFilename) {
  chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
    const url = item.finalUrl || item.url
    if (shouldCapture(item, url, item.filename)) {
      suggest()
      capture(item, item.filename)
      return
    }
    suggest()
  })
} else {
  chrome.downloads.onCreated.addListener((item) => {
    const url = item.finalUrl || item.url
    if (shouldCapture(item, url, item.filename)) capture(item, item.filename)
  })
}

// ---------------------------------------------------------------- media sniffing

// request headers of recent requests (Referer/Origin/Cookie as really sent)
const reqHeaders = new Map()
const REQ_LIMIT = 400

chrome.webRequest.onSendHeaders.addListener(
  (d) => {
    if (d.tabId < 0) return
    const h = {}
    for (const x of d.requestHeaders || []) {
      const k = x.name.toLowerCase()
      if (k === 'referer' || k === 'origin' || k === 'cookie' || k === 'user-agent' || k === 'authorization' || k.startsWith('x-')) h[x.name] = x.value
    }
    reqHeaders.set(d.requestId, h)
    if (reqHeaders.size > REQ_LIMIT) reqHeaders.delete(reqHeaders.keys().next().value)
  },
  { urls: ['<all_urls>'], types: ['xmlhttprequest', 'media', 'other', 'object', 'sub_frame'] },
  ['requestHeaders', 'extraHeaders'],
)

function header(d, name) {
  for (const h of d.responseHeaders || []) if (h.name.toLowerCase() === name) return h.value || ''
  return ''
}

function classify(d) {
  const ct = header(d, 'content-type').toLowerCase().split(';')[0].trim()
  const ext = extOf(d.url)
  const u = d.url.toLowerCase()
  if (ct.includes('mpegurl') || ext === 'm3u8') return 'hls'
  if (ct === 'application/dash+xml' || ext === 'mpd') return 'dash'
  if (ct === 'video/mp2t' || ct === 'video/iso.segment' || ext === 'ts' || ext === 'm4s' || ext === 'aac' && u.includes('seg')) return ''
  if (u.includes('googlevideo.com/videoplayback') || u.includes('.fbcdn.net') && u.includes('bytestart')) return '' // split A/V, page-level only
  if (ct.startsWith('video/') || ct.startsWith('audio/') || ['mp4', 'webm', 'mkv', 'flv', 'mov', 'm4v', 'mp3', 'm4a', 'ogg', 'opus'].includes(ext)) {
    let size = parseInt(header(d, 'content-length'), 10) || 0
    const cr = /\/(\d+)\s*$/.exec(header(d, 'content-range'))
    if (cr) size = parseInt(cr[1], 10)
    if (size && size < MIN_DIRECT_SIZE) return ''
    if (ct === 'application/octet-stream' && !ext) return ''
    return 'direct'
  }
  return ''
}

async function tabMedia(tabId) {
  const k = 'media_' + tabId
  const s = await chrome.storage.session.get(k)
  return s[k] || []
}

async function setTabMedia(tabId, list) {
  await chrome.storage.session.set({ ['media_' + tabId]: list })
  updateBadge(tabId, FDM.groupMedia(list).length)
}

const pending = new Map() // tabId -> promise chain (serialize storage updates)

function addMedia(tabId, entry) {
  const prev = pending.get(tabId) || Promise.resolve()
  const next = prev.then(async () => {
    const list = await tabMedia(tabId)
    const key = entry.url.split('#')[0]
    const baseKey = key.replace(/([?&])(range|bytestart|byteend|start|end|rn|rbuf)=[^&]*/gi, '$1')
    if (list.some((m) => m.key === baseKey)) return
    entry.key = baseKey
    list.unshift(entry)
    if (list.length > MAX_MEDIA_PER_TAB) list.length = MAX_MEDIA_PER_TAB
    await setTabMedia(tabId, list)
    broadcast(tabId)
    inspectSoon(tabId, entry)
  })
  pending.set(tabId, next.catch(() => {}))
}

// ---------------------------------------------------------------- titles & inspection

/** Best title known for a tab/frame: top page (og:title / document.title) > frame title. */
async function titleFor(tabId, frameId) {
  const s = await chrome.storage.session.get('titles_' + tabId)
  const t = s['titles_' + tabId] || {}
  const top = t[0] || {}
  const fr = t[frameId] || {}
  return FDM.cleanTitle(top.og) || FDM.cleanTitle(top.title) || FDM.cleanTitle(fr.player) || FDM.cleanTitle(fr.og) || FDM.cleanTitle(fr.title) || ''
}

async function setFrameTitle(tabId, frameId, info) {
  const k = 'titles_' + tabId
  const s = await chrome.storage.session.get(k)
  const t = s[k] || {}
  t[frameId] = info
  await chrome.storage.session.set({ [k]: t })
}

async function inspectionOf(key) {
  const s = await chrome.storage.session.get('insp_' + key)
  const v = s['insp_' + key]
  return v && Date.now() - v.at < 10 * 60 * 1000 ? v.info : null
}

const inflightInspect = new Set()

function inspectSoon(tabId, m) {
  if (inflightInspect.has(m.key)) return
  inflightInspect.add(m.key)
  setTimeout(async () => {
    try {
      if (await inspectionOf(m.key)) return
      const info = await inspect(tabId, m)
      if (info) {
        await chrome.storage.session.set({ ['insp_' + m.key]: { at: Date.now(), info } })
        broadcast(tabId)
      }
    } finally {
      inflightInspect.delete(m.key)
    }
  }, 600) // let the page title / master playlist settle first
}

async function mediaHeaders(tabId, m) {
  const tab = await tabInfo(tabId)
  const page = tab ? tab.url : ''
  const h = m.headers || {}
  return {
    page,
    referer: h.Referer || h.referer || (await frameUrl(tabId, m.frameId)) || page,
    origin: h.Origin || h.origin || m.initiator || '',
    cookies: h.Cookie || h.cookie || (await cookiesFor(m.url)),
    extra: Object.fromEntries(Object.entries(h).filter(([k]) => !['referer', 'origin', 'cookie', 'user-agent'].includes(k.toLowerCase()))),
  }
}

async function inspect(tabId, m) {
  const hh = await mediaHeaders(tabId, m)
  try {
    return await request('/v1/inspect', {
      kind: m.kind, url: m.url, pageUrl: hh.page, title: await titleFor(tabId, m.frameId),
      fileName: m.fileName || '', mime: m.mime || '', size: m.size || 0,
      referer: hh.referer, origin: hh.origin, cookies: hh.cookies, userAgent: navigator.userAgent, headers: hh.extra,
    }, { launch: false })
  } catch (e) {
    return null
  }
}

/** Grouped media of a tab, enriched with title + inspection. */
async function mediaView(tabId) {
  const groups = FDM.groupMedia(await tabMedia(tabId))
  const out = []
  for (const m of groups) {
    const info = await inspectionOf(m.key)
    if (!m.title) m.title = await titleFor(tabId, m.frameId)
    out.push({ ...m, info, view: FDM.describe(m, info) })
  }
  return out
}

async function broadcast(tabId) {
  const items = await mediaView(tabId)
  updateBadge(tabId, items.length)
  chrome.tabs.sendMessage(tabId, { type: 'fluxdm:media', items }).catch(() => {})
  chrome.runtime.sendMessage({ type: 'fluxdm:popup', tabId, items }).catch(() => {})
}

chrome.webRequest.onHeadersReceived.addListener(
  (d) => {
    if (d.tabId < 0 || d.statusCode >= 400) {
      reqHeaders.delete(d.requestId)
      return
    }
    const kind = classify(d)
    const sent = reqHeaders.get(d.requestId) || {}
    reqHeaders.delete(d.requestId)
    if (!kind) return
    let size = parseInt(header(d, 'content-length'), 10) || 0
    const cr = /\/(\d+)\s*$/.exec(header(d, 'content-range'))
    if (cr) size = parseInt(cr[1], 10)
    addMedia(d.tabId, {
      kind,
      url: d.url,
      frameId: d.frameId,
      initiator: d.initiator || '',
      headers: sent,
      size: kind === 'direct' ? size : 0,
      mime: header(d, 'content-type').split(';')[0],
      fileName: FDM.dispositionName(header(d, 'content-disposition')),
      time: Date.now(),
    })
  },
  { urls: ['<all_urls>'], types: ['xmlhttprequest', 'media', 'other', 'object'] },
  ['responseHeaders'],
)

chrome.webRequest.onErrorOccurred.addListener((d) => reqHeaders.delete(d.requestId), { urls: ['<all_urls>'] })

// reset the list when the tab navigates to another page
chrome.webNavigation.onCommitted.addListener((d) => {
  if (d.frameId === 0 && d.transitionType !== 'auto_subframe') {
    setTabMedia(d.tabId, []).then(() => chrome.runtime.sendMessage({ type: 'fluxdm:popup', tabId: d.tabId, items: [] }).catch(() => {}))
    chrome.storage.session.remove('titles_' + d.tabId)
  }
})
chrome.tabs.onRemoved.addListener((tabId) => chrome.storage.session.remove(['media_' + tabId, 'titles_' + tabId]))

async function frameUrl(tabId, frameId) {
  try {
    const f = await chrome.webNavigation.getFrame({ tabId, frameId: frameId || 0 })
    return f ? f.url : ''
  } catch (e) {
    return ''
  }
}

async function tabInfo(tabId) {
  try { return await chrome.tabs.get(tabId) } catch (e) { return null }
}

/**
 * Send a detected media entry to FluxDM. opts.quality = Quality.id chosen in
 * the UI (default: inspected default); the final file name matches what the
 * extension displayed ("Title (1080p).mp4").
 */
async function downloadMedia(tabId, m, opts = {}) {
  const hh = await mediaHeaders(tabId, m)
  const info = m.info || (m.key ? await inspectionOf(m.key) : null)
  const title = (await titleFor(tabId, m.frameId)) || FDM.cleanTitle(opts.title) || (info && info.title) || ''
  let q = null
  if (info && info.qualities) {
    q = info.qualities.find((x) => x.id === (opts.quality || info.defaultQuality)) || info.qualities[0]
  }
  const fileName = q ? FDM.finalName(info.title || title, q.label, q.ext) : (info && info.fileName) || ''
  return request('/v1/video', {
    kind: m.kind,
    url: m.url,
    pageUrl: hh.page,
    title: (info && info.title) || title,
    referer: hh.referer,
    origin: hh.origin,
    cookies: hh.cookies,
    userAgent: navigator.userAgent,
    headers: hh.extra,
    quick: !!opts.quick,
    fileName,
    quality: q ? q.id : '',
    height: q && q.height ? q.height : 0,
    audioOnly: !!(q && q.id === 'audio'),
  })
}

async function downloadPage(tabId, pageUrl, title) {
  if (!/^https?:/i.test(pageUrl || '')) throw new FdmError(M('errRestricted'), 'app')
  return request('/v1/video', {
    kind: 'page',
    url: pageUrl,
    pageUrl,
    title: title || '',
    cookies: await cookiesFor(pageUrl),
    userAgent: navigator.userAgent,
  })
}

async function downloadAllLinks(tabId) {
  let results
  try {
    results = await chrome.scripting.executeScript({
      target: { tabId, allFrames: true },
      func: () => {
        const out = []
        const seen = new Set()
        const push = (url, text, type) => {
          if (!url || seen.has(url) || !/^(https?|ftp):/i.test(url)) return
          seen.add(url)
          out.push({ url, text: (text || '').trim().slice(0, 200), type })
        }
        document.querySelectorAll('a[href]').forEach((a) => push(a.href, a.textContent, 'link'))
        document.querySelectorAll('img[src]').forEach((i) => push(i.currentSrc || i.src, i.alt, 'image'))
        document.querySelectorAll('video[src], video source[src], audio[src], audio source[src]').forEach((v) => push(v.src, '', 'video'))
        return out
      },
    })
  } catch (e) {
    throw new FdmError(M('errRestricted'), 'app')
  }
  const links = []
  for (const r of results || []) for (const l of r.result || []) links.push(l)
  if (!links.length) throw new FdmError(M('errNoLinks'), 'app')
  const tab = await tabInfo(tabId)
  const page = tab ? tab.url : ''
  const r = await request('/v1/batch', { pageUrl: page, cookies: await cookiesFor(page), userAgent: navigator.userAgent, links })
  return { ...r, count: links.length }
}

async function downloadLink(url, pageUrl) {
  return request('/v1/download', {
    url, referer: pageUrl || '', pageUrl: pageUrl || '', cookies: await cookiesFor(url), userAgent: navigator.userAgent,
  })
}

// ---------------------------------------------------------------- context menus

function createMenus() {
  chrome.contextMenus.removeAll(() => {
    void chrome.runtime.lastError
    const add = (o) => chrome.contextMenus.create(o, () => void chrome.runtime.lastError)
    add({ id: 'fdm-link', title: M('downloadLink'), contexts: ['link'] })
    add({ id: 'fdm-media', title: M('downloadMedia'), contexts: ['video', 'audio'] })
    add({ id: 'fdm-image', title: M('downloadImage'), contexts: ['image'] })
    add({ id: 'fdm-page', title: M('downloadPage'), contexts: ['page', 'frame'] })
    add({ id: 'fdm-all', title: M('downloadAll'), contexts: ['page', 'frame', 'selection'] })
    if (chrome.sidePanel && chrome.sidePanel.open) add({ id: 'fdm-sidepanel', title: M('openSidePanel'), contexts: ['action'] })
    add({ id: 'fdm-show', title: M('openApp'), contexts: ['action'] })
  })
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const tabId = tab ? tab.id : -1
  if (info.menuItemId === 'fdm-sidepanel') {
    // must stay synchronous with the click (user gesture)
    try { chrome.sidePanel.open({ windowId: tab.windowId }).catch(() => {}) } catch (e) { /* */ }
    return
  }
  if (info.menuItemId === 'fdm-show') { showApp(); return }
  let label = ''
  try {
    let r = null
    switch (info.menuItemId) {
      case 'fdm-link':
        label = shortName(info.linkUrl)
        r = await downloadLink(info.linkUrl, info.pageUrl)
        break
      case 'fdm-image':
        label = shortName(info.srcUrl)
        r = await downloadLink(info.srcUrl, info.pageUrl)
        break
      case 'fdm-media': {
        const src = info.srcUrl || ''
        label = tab ? tab.title : shortName(src)
        if (/^https?:/i.test(src)) {
          r = await downloadMedia(tabId, { kind: extOf(src) === 'm3u8' ? 'hls' : 'direct', url: src, frameId: info.frameId, headers: {} })
        } else {
          // blob: (MSE) video: use the detected stream of the tab, else yt-dlp on the page
          const list = await tabMedia(tabId)
          if (list.length) r = await downloadMedia(tabId, list[0])
          else r = await downloadPage(tabId, info.pageUrl, tab ? tab.title : '')
        }
        break
      }
      case 'fdm-page':
        label = tab ? tab.title : ''
        r = await downloadPage(tabId, info.frameUrl || info.pageUrl, tab ? tab.title : '')
        break
      case 'fdm-all': {
        r = await downloadAllLinks(tabId)
        label = M('linksSent', [String(r.count || 0)])
        break
      }
      default:
        return
    }
    notify(M('notifSentTitle'), label || M('sent'), { id: r && r.id })
  } catch (e) {
    console.warn('FluxDM:', e)
    notify(M('notifFailTitle'), (label ? label + ' — ' : '') + (e.message || e), {})
  }
})

// ---------------------------------------------------------------- messages (content script / popup / side panel / options)

function senderHost(msg, sender) {
  return msg.host || (sender.tab && sender.tab.url) || ''
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const tabId = msg.tabId != null ? msg.tabId : sender.tab ? sender.tab.id : -1
  ;(async () => {
    await ready
    switch (msg.type) {
      case 'getMedia': {
        const top = (sender.tab && sender.tab.url) || msg.pageUrl || ''
        return {
          items: await mediaView(tabId), settings, prefs, frameId: sender.frameId, connected: connState !== false,
          ytdlp: isYtdlpPage(msg.pageUrl || top),
          videoHidden: FDM.hostMatches(FDM.siteOf(top), settings.videoSkipSites),
        }
      }
      case 'pageInfo':
        if (sender.tab) await setFrameTitle(tabId, sender.frameId || 0, msg.info || {})
        return { ok: true }
      case 'inspectNow': {
        const item = msg.item
        let info = item.key ? await inspectionOf(item.key) : null
        if (!info) {
          info = await inspect(tabId, item)
          if (info && item.key) await chrome.storage.session.set({ ['insp_' + item.key]: { at: Date.now(), info } })
        }
        return { info }
      }
      case 'downloadMedia':
        return { ok: true, ...(await downloadMedia(tabId, msg.item, { title: msg.title, quick: msg.quick, quality: msg.quality })) }
      case 'downloadPage':
        return { ok: true, ...(await downloadPage(tabId, msg.pageUrl, msg.title)) }
      case 'downloadDirect':
        return { ok: true, ...(await downloadMedia(tabId, { kind: msg.kind || 'direct', url: msg.url, frameId: sender.frameId, headers: {} }, { title: msg.title })) }
      case 'downloadAll':
        return { ok: true, ...(await downloadAllLinks(tabId)) }
      case 'status': {
        const ok = msg.cached && connState !== null ? connState : await refreshSettings()
        return {
          connected: ok, captureEnabled, captureOn: captureOn(), settings, prefs, hint: ok ? '' : lastConnectError,
          overridden: Object.keys(overrides), version: settings.version || '',
        }
      }
      case 'setCapture':
        captureEnabled = !!msg.value
        await chrome.storage.local.set({ captureEnabled })
        scheduleBadges()
        return { ok: true }
      case 'launch': {
        const c = await connect(true)
        if (c) { setConnected(true); refreshSettings() }
        return { connected: !!c, hint: c ? '' : lastConnectError }
      }
      case 'clear':
        await setTabMedia(tabId, [])
        return { ok: true }
      case 'setVideoButton':
        return saveSettings({ videoButton: !!msg.value })
      case 'saveSettings':
        return saveSettings(msg.settings || {})
      case 'siteRule':
        return siteRule(msg.list, FDM.siteOf(senderHost(msg, sender)) || msg.host, msg.on !== false)
      case 'getActive':
        return await request('/v1/active', null, { launch: false, quiet: true })
      case 'control':
        return await request('/v1/control', { id: msg.id, action: msg.action }, { launch: false })
      case 'show':
        await showApp(msg.id)
        return { ok: true }
    }
    return { error: 'unknown', code: 'app' }
  })()
    .then(sendResponse)
    .catch((e) => sendResponse(errPayload(e)))
  return true // async response
})
