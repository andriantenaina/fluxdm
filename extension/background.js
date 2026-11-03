// FluxDM browser integration — service worker (Manifest V3, Chromium/Opera).
// - finds the FluxDM app (native messaging host, else default local port)
// - captures downloads and hands them to FluxDM
// - sniffs network responses to detect videos (HLS .m3u8, DASH .mpd, MP4/WebM...)
// - context menus: download link / media / all links / page video
'use strict'

const HOST = 'com.fluxdm.host'
const DEFAULT_PORT = 52775
const MAX_MEDIA_PER_TAB = 30
const MIN_DIRECT_SIZE = 300 * 1024

// ---------------------------------------------------------------- connection

let conn = null // { port, token }
let settings = {
  captureExts: [], captureMinSize: 0, skipSites: [], videoButton: true, enabled: true, ytdlpSites: [],
}
let captureEnabled = true

const ready = (async () => {
  const s = await chrome.storage.session.get(['conn'])
  if (s.conn) conn = s.conn
  const l = await chrome.storage.local.get(['settings', 'captureEnabled'])
  if (l.settings) settings = { ...settings, ...l.settings }
  if (typeof l.captureEnabled === 'boolean') captureEnabled = l.captureEnabled
})()

async function connect(launch) {
  try {
    const r = await chrome.runtime.sendNativeMessage(HOST, { type: launch ? 'launch' : 'hello' })
    if (r && r.ok) {
      conn = { port: r.port, token: r.token }
      await chrome.storage.session.set({ conn })
      return conn
    }
  } catch (e) {
    // host not installed / not allowed: try the default port
  }
  try {
    const res = await fetch(`http://127.0.0.1:${DEFAULT_PORT}/v1/hello`, { cache: 'no-store' })
    if (res.ok) {
      const j = await res.json()
      conn = { port: DEFAULT_PORT, token: j.token }
      await chrome.storage.session.set({ conn })
      return conn
    }
  } catch (e) { /* app not running */ }
  conn = null
  await chrome.storage.session.remove('conn')
  return null
}

async function request(path, body, allowLaunch = true) {
  await ready
  for (let attempt = 0; attempt < 3; attempt++) {
    if (!conn) await connect(attempt > 0 && allowLaunch)
    if (!conn) continue
    try {
      const res = await fetch(`http://127.0.0.1:${conn.port}${path}`, {
        method: body ? 'POST' : 'GET',
        headers: { 'Content-Type': 'application/json', 'X-FluxDM-Token': conn.token },
        body: body ? JSON.stringify(body) : undefined,
        cache: 'no-store',
      })
      if (res.status === 401) { conn = null; continue }
      const j = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(j.error || res.statusText)
      return j
    } catch (e) {
      if (e && e.message && !/fetch|network|Failed/i.test(e.message)) throw e
      conn = null
    }
  }
  throw new Error('FluxDM injoignable')
}

async function refreshSettings() {
  try {
    const s = await request('/v1/ping', null, false)
    settings = { ...settings, ...s }
    await chrome.storage.local.set({ settings })
    return true
  } catch (e) {
    return false
  }
}

chrome.runtime.onStartup.addListener(() => refreshSettings())
chrome.runtime.onInstalled.addListener(() => {
  refreshSettings()
  createMenus()
})
chrome.alarms.create('fluxdm-refresh', { periodInMinutes: 5 })
chrome.alarms.onAlarm.addListener((a) => { if (a.name === 'fluxdm-refresh') refreshSettings() })

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
  const h = hostOf(url)
  return (settings.skipSites || []).some((s) => s && (h === s || h.endsWith('.' + s)))
}

function isYtdlpPage(url) {
  const h = hostOf(url).replace(/^(www|m)\./, '')
  return (settings.ytdlpSites || []).some((s) => h === s || h.endsWith('.' + s))
}

// ---------------------------------------------------------------- download capture

const ownDownloads = new Set() // downloads re-issued by us (fallback) must not be captured again
const recentlyCaptured = new Map()

function shouldCapture(item, url, filename) {
  if (!captureEnabled || settings.enabled === false) return false
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
    // FluxDM unavailable: give the download back to the browser
    const id = await chrome.downloads.download({ url, filename: name || undefined }).catch(() => null)
    if (id != null) ownDownloads.add(id)
    ownDownloads.add(url)
    setTimeout(() => ownDownloads.delete(url), 10000)
  }
}

if (chrome.downloads.onDeterminingFilename) {
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
  const n = list.length
  chrome.action.setBadgeText({ tabId, text: n ? String(n) : '' }).catch(() => {})
  chrome.action.setBadgeBackgroundColor({ tabId, color: '#1f7ae0' }).catch(() => {})
}

const pending = new Map() // tabId -> promise chain (serialize storage updates)

function addMedia(tabId, entry) {
  const prev = pending.get(tabId) || Promise.resolve()
  const next = prev.then(async () => {
    const list = await tabMedia(tabId)
    const key = entry.url.split('#')[0]
    const baseKey = key.replace(/([?&])(range|bytestart|byteend|start|end)=[^&]*/gi, '$1')
    if (list.some((m) => m.key === baseKey)) return
    // a variant playlist of an already listed HLS master: skip
    if (entry.kind === 'hls' && list.some((m) => m.kind === 'hls' && m.frameId === entry.frameId && Date.now() - m.time < 4000)) return
    entry.key = baseKey
    list.unshift(entry)
    if (list.length > MAX_MEDIA_PER_TAB) list.length = MAX_MEDIA_PER_TAB
    await setTabMedia(tabId, list)
    chrome.tabs.sendMessage(tabId, { type: 'fluxdm:media', items: list }).catch(() => {})
  })
  pending.set(tabId, next.catch(() => {}))
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
    setTabMedia(d.tabId, [])
  }
})
chrome.tabs.onRemoved.addListener((tabId) => chrome.storage.session.remove('media_' + tabId))

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

/** Send a detected media entry to FluxDM. */
async function downloadMedia(tabId, m, opts = {}) {
  const tab = await tabInfo(tabId)
  const page = tab ? tab.url : ''
  const ref = m.headers && (m.headers.Referer || m.headers.referer) || (await frameUrl(tabId, m.frameId)) || page
  const origin = m.headers && (m.headers.Origin || m.headers.origin) || m.initiator || ''
  const cookies = m.headers && (m.headers.Cookie || m.headers.cookie) || (await cookiesFor(m.url))
  const extra = {}
  for (const [k, v] of Object.entries(m.headers || {})) {
    const lk = k.toLowerCase()
    if (lk !== 'referer' && lk !== 'origin' && lk !== 'cookie' && lk !== 'user-agent') extra[k] = v
  }
  return request('/v1/video', {
    kind: m.kind,
    url: m.url,
    pageUrl: page,
    title: (opts.title || (tab && tab.title) || '').slice(0, 200),
    referer: ref,
    origin,
    cookies,
    userAgent: navigator.userAgent,
    headers: extra,
    quick: !!opts.quick,
  })
}

async function downloadPage(tabId, pageUrl, title) {
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
  const results = await chrome.scripting.executeScript({
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
  const links = []
  for (const r of results || []) for (const l of r.result || []) links.push(l)
  const tab = await tabInfo(tabId)
  const page = tab ? tab.url : ''
  return request('/v1/batch', { pageUrl: page, cookies: await cookiesFor(page), userAgent: navigator.userAgent, links })
}

async function downloadLink(url, pageUrl) {
  return request('/v1/download', {
    url, referer: pageUrl || '', pageUrl: pageUrl || '', cookies: await cookiesFor(url), userAgent: navigator.userAgent,
  })
}

// ---------------------------------------------------------------- context menus

function createMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'fdm-link', title: chrome.i18n.getMessage('downloadLink'), contexts: ['link'] })
    chrome.contextMenus.create({ id: 'fdm-media', title: chrome.i18n.getMessage('downloadMedia'), contexts: ['video', 'audio'] })
    chrome.contextMenus.create({ id: 'fdm-image', title: chrome.i18n.getMessage('downloadImage'), contexts: ['image'] })
    chrome.contextMenus.create({ id: 'fdm-page', title: chrome.i18n.getMessage('downloadPage'), contexts: ['page', 'frame'] })
    chrome.contextMenus.create({ id: 'fdm-all', title: chrome.i18n.getMessage('downloadAll'), contexts: ['page', 'frame', 'selection'] })
  })
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const tabId = tab ? tab.id : -1
  try {
    switch (info.menuItemId) {
      case 'fdm-link':
        await downloadLink(info.linkUrl, info.pageUrl)
        break
      case 'fdm-image':
        await downloadLink(info.srcUrl, info.pageUrl)
        break
      case 'fdm-media': {
        const src = info.srcUrl || ''
        if (/^https?:/i.test(src)) {
          await downloadMedia(tabId, { kind: extOf(src) === 'm3u8' ? 'hls' : 'direct', url: src, frameId: info.frameId, headers: {} })
        } else {
          // blob: (MSE) video: use the detected stream of the tab, else yt-dlp on the page
          const list = await tabMedia(tabId)
          if (list.length) await downloadMedia(tabId, list[0])
          else await downloadPage(tabId, info.pageUrl, tab ? tab.title : '')
        }
        break
      }
      case 'fdm-page':
        await downloadPage(tabId, info.frameUrl || info.pageUrl, tab ? tab.title : '')
        break
      case 'fdm-all':
        await downloadAllLinks(tabId)
        break
    }
  } catch (e) {
    console.warn('FluxDM:', e)
  }
})

// ---------------------------------------------------------------- messages (content script / popup)

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  const tabId = msg.tabId != null ? msg.tabId : sender.tab ? sender.tab.id : -1
  ;(async () => {
    await ready
    switch (msg.type) {
      case 'getMedia':
        return { items: await tabMedia(tabId), settings, frameId: sender.frameId, ytdlp: isYtdlpPage(msg.pageUrl || (sender.tab && sender.tab.url) || '') }
      case 'downloadMedia':
        await downloadMedia(tabId, msg.item, { title: msg.title, quick: msg.quick })
        return { ok: true }
      case 'downloadPage':
        await downloadPage(tabId, msg.pageUrl, msg.title)
        return { ok: true }
      case 'downloadDirect':
        await downloadMedia(tabId, { kind: msg.kind || 'direct', url: msg.url, frameId: sender.frameId, headers: {} }, { title: msg.title })
        return { ok: true }
      case 'downloadAll':
        await downloadAllLinks(tabId)
        return { ok: true }
      case 'status': {
        const ok = await refreshSettings()
        return { connected: ok, captureEnabled, settings }
      }
      case 'setCapture':
        captureEnabled = !!msg.value
        await chrome.storage.local.set({ captureEnabled })
        return { ok: true }
      case 'launch':
        await connect(true)
        return { connected: !!conn }
      case 'clear':
        await setTabMedia(tabId, [])
        return { ok: true }
    }
    return { error: 'unknown' }
  })()
    .then(sendResponse)
    .catch((e) => sendResponse({ error: String(e && e.message || e) }))
  return true // async response
})
