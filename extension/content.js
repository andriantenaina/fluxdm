// FluxDM content script: floating "Download video" button over playing videos
// (in every frame, so it also works inside embedded players such as Voe,
// Doodstream, Uqload or Vidmoly iframes).
;(() => {
  'use strict'
  if (window.__fluxdmInjected) return
  window.__fluxdmInjected = true

  const MSG = (k) => (chrome.i18n && chrome.i18n.getMessage(k)) || k
  let media = []
  let enabled = true
  let ytdlp = false
  let host = null // shadow host element
  let root = null
  let currentVideo = null
  let hideTimer = 0
  let menuOpen = false

  function send(msg) {
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendMessage(msg, (r) => {
          void chrome.runtime.lastError
          resolve(r || {})
        })
      } catch (e) {
        resolve({}) // extension reloaded: context invalidated
      }
    })
  }

  async function refresh() {
    const r = await send({ type: 'getMedia', pageUrl: top === window ? location.href : '' })
    if (r.items) media = r.items
    if (r.settings) enabled = r.settings.videoButton !== false
    ytdlp = !!r.ytdlp
  }

  chrome.runtime.onMessage.addListener((msg) => {
    if (msg && msg.type === 'fluxdm:media') {
      media = msg.items || []
      if (host && host.style.display !== 'none') renderMenu()
    }
  })

  function fmtSize(n) {
    if (!n || n < 0) return ''
    const u = ['o', 'Ko', 'Mo', 'Go']
    let i = 0
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
    return n.toFixed(i ? 1 : 0) + ' ' + u[i]
  }

  function label(m) {
    const kind = { hls: 'HLS', dash: 'DASH', direct: (m.mime || 'vidéo').replace(/^video\//, '').toUpperCase() }[m.kind] || m.kind
    let name = ''
    try { name = decodeURIComponent(new URL(m.url).pathname.split('/').pop()) } catch (e) { /* */ }
    return { kind, name: name.slice(0, 48), size: fmtSize(m.size) }
  }

  function ensureHost() {
    if (host) return
    host = document.createElement('div')
    host.style.cssText = 'position:fixed;z-index:2147483647;top:0;left:0;display:none;'
    root = host.attachShadow({ mode: 'closed' })
    root.innerHTML = `
      <style>
        :host{all:initial}
        .wrap{font:13px/1.3 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;color:#fff}
        .btn{display:flex;align-items:center;gap:6px;background:linear-gradient(135deg,#258cff,#0e54c8);border:0;border-radius:18px;
          padding:7px 12px 7px 9px;color:#fff;cursor:pointer;box-shadow:0 4px 14px rgba(0,0,0,.35);font:inherit;font-weight:600;opacity:.95}
        .btn:hover{opacity:1;filter:brightness(1.08)}
        .btn svg{width:16px;height:16px}
        .x{margin-left:4px;opacity:.7;font-weight:400;padding:0 2px}
        .x:hover{opacity:1}
        .menu{margin-top:6px;background:#1f2229;border:1px solid #333845;border-radius:10px;box-shadow:0 10px 30px rgba(0,0,0,.5);
          padding:4px;min-width:260px;max-width:420px;max-height:320px;overflow:auto}
        .it{display:flex;gap:8px;align-items:center;padding:7px 9px;border-radius:7px;cursor:pointer;color:#e7e9ee}
        .it:hover{background:#2a4a7a}
        .k{font-size:10.5px;font-weight:700;background:#3b8cff33;color:#7fb4ff;border-radius:4px;padding:1px 5px}
        .n{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
        .s{color:#959cae;font-size:11.5px}
        .hint{color:#959cae;padding:8px 10px;font-size:12px}
        .sep{border-top:1px solid #333845;margin:4px 0}
      </style>
      <div class="wrap">
        <button class="btn" part="btn"><svg viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 10l5 5 5-5M5 20h14"/></svg><span class="t"></span><span class="x" title="Masquer">✕</span></button>
        <div class="menu" style="display:none"></div>
      </div>`
    root.querySelector('.t').textContent = MSG('downloadVideo')
    root.querySelector('.btn').addEventListener('click', (e) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.target.classList.contains('x')) {
        host.style.display = 'none'
        currentVideo && (currentVideo.__fluxdmHidden = true)
        return
      }
      menuOpen = !menuOpen
      renderMenu()
    })
    host.addEventListener('mouseenter', () => clearTimeout(hideTimer))
    host.addEventListener('mouseleave', scheduleHide)
    ;(document.body || document.documentElement).appendChild(host)
  }

  function candidates() {
    // prefer streams detected in this frame, else all of the tab
    const frameItems = media.filter((m) => m.frameId === window.__fluxdmFrameId)
    const list = frameItems.length ? frameItems : media
    const out = list.map((m) => ({ type: 'media', m }))
    const v = currentVideo
    if (v) {
      const src = v.currentSrc || v.src
      if (src && /^https?:/i.test(src) && !list.some((m) => m.url === src)) out.unshift({ type: 'direct', url: src })
    }
    return out
  }

  function renderMenu() {
    if (!root) return
    const box = root.querySelector('.menu')
    box.style.display = menuOpen ? 'block' : 'none'
    if (!menuOpen) return
    box.textContent = ''
    const items = candidates()
    const add = (k, n, s, fn) => {
      const d = document.createElement('div')
      d.className = 'it'
      d.innerHTML = '<span class="k"></span><span class="n"></span><span class="s"></span>'
      d.querySelector('.k').textContent = k
      d.querySelector('.n').textContent = n
      d.querySelector('.s').textContent = s || ''
      d.addEventListener('click', (e) => {
        e.stopPropagation()
        menuOpen = false
        renderMenu()
        fn()
      })
      box.appendChild(d)
    }
    const title = (top === window ? document.title : '') || ''
    for (const c of items) {
      if (c.type === 'direct') {
        add('MP4', decodeURIComponent(c.url.split('?')[0].split('/').pop()).slice(0, 48), '', () =>
          send({ type: 'downloadDirect', url: c.url, title }))
      } else {
        const l = label(c.m)
        add(l.kind, l.name || c.m.url, l.size, () => send({ type: 'downloadMedia', item: c.m, title }))
      }
    }
    if (ytdlp || !items.length) {
      if (items.length) box.appendChild(Object.assign(document.createElement('div'), { className: 'sep' }))
      add('yt-dlp', MSG('downloadPage'), '', () => send({ type: 'downloadPage', pageUrl: top === window ? location.href : document.referrer || location.href, title }))
    }
    if (!items.length && !ytdlp) {
      const h = document.createElement('div')
      h.className = 'hint'
      h.textContent = MSG('noMedia')
      box.appendChild(h)
    }
  }

  function place(v) {
    const r = v.getBoundingClientRect()
    if (r.width < 200 || r.height < 110 || r.bottom < 0 || r.top > innerHeight) {
      host.style.display = 'none'
      return
    }
    host.style.display = 'block'
    const w = host.offsetWidth || 180
    host.style.left = Math.max(4, Math.min(innerWidth - w - 4, r.right - w - 10)) + 'px'
    host.style.top = Math.max(4, r.top + 10) + 'px'
  }

  function scheduleHide() {
    clearTimeout(hideTimer)
    hideTimer = setTimeout(() => {
      if (!menuOpen && host) host.style.display = 'none'
    }, 3500)
  }

  async function showFor(v) {
    if (!v || v.__fluxdmHidden) return
    await refresh()
    if (!enabled) return
    ensureHost()
    currentVideo = v
    place(v)
    scheduleHide()
  }

  // a video starts playing anywhere in the frame
  document.addEventListener('play', (e) => {
    if (e.target instanceof HTMLVideoElement) showFor(e.target)
  }, true)

  // hover over a video
  document.addEventListener('mouseover', (e) => {
    const t = e.target
    if (!(t instanceof Element)) return
    let v = t instanceof HTMLVideoElement ? t : null
    if (!v) {
      // players put overlays above the <video>: look under the pointer
      const els = document.elementsFromPoint ? document.elementsFromPoint(e.clientX, e.clientY) : []
      v = els.find((x) => x instanceof HTMLVideoElement) || null
    }
    if (v && v !== currentVideo) showFor(v)
    else if (v && host) { place(v); scheduleHide() }
  }, { passive: true, capture: true })

  const reposition = () => { if (currentVideo && host && host.style.display !== 'none') place(currentVideo) }
  addEventListener('scroll', reposition, { passive: true, capture: true })
  addEventListener('resize', reposition, { passive: true })
  document.addEventListener('fullscreenchange', () => {
    // move the button inside the fullscreen element so it stays visible
    if (!host) return
    const fs = document.fullscreenElement
    ;(fs && fs.tagName !== 'VIDEO' ? fs : document.body || document.documentElement).appendChild(host)
    reposition()
  })
  document.addEventListener('click', () => { if (menuOpen) { menuOpen = false; renderMenu() } }, true)

  // frame id is needed to match network requests to this frame
  send({ type: 'getMedia' }).then((r) => {
    if (r && typeof r.frameId === 'number') window.__fluxdmFrameId = r.frameId
    if (r.items) media = r.items
    if (r.settings) enabled = r.settings.videoButton !== false
    ytdlp = !!r.ytdlp
  })
})()
