// FluxDM content script (every frame): reports page titles and shows the
// floating "download 1080p · MP4 · 245 MB" pill over playing videos, with an
// accessible quality menu (menu/menuitem, arrows, Esc) and a clickable toast.
;(() => {
  'use strict'
  if (window.__fluxdmInjected) return
  window.__fluxdmInjected = true

  const F = self.FDM
  const MSG = (k, subs) => F.msg(k, subs)
  const LANG = F.uiLang()
  const isTop = window === window.top

  let items = []
  let enabled = true
  let ytdlp = false
  let frameId = -1
  let theme = 'system'
  let host = null
  let root = null
  let currentVideo = null
  let hideTimer = 0
  let toastTimer = 0
  let menuOpen = false
  let busy = false

  function send(msg) {
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendMessage(msg, (r) => { void chrome.runtime.lastError; resolve(r || {}) })
      } catch (e) {
        resolve({}) // extension reloaded
      }
    })
  }

  // ------------------------------------------------------------ titles

  function meta(name) {
    const el = document.querySelector(`meta[property="${name}"],meta[name="${name}"]`)
    return el ? el.getAttribute('content') || '' : ''
  }

  function playerTitle() {
    const v = currentVideo || document.querySelector('video')
    if (!v) return ''
    const t = v.getAttribute('title') || v.getAttribute('aria-label') || ''
    if (t) return t
    // closest visible heading above the player
    let el = v
    for (let i = 0; i < 6 && el; i++, el = el.parentElement) {
      const h = el.querySelector && el.querySelector('h1, h2, .title, [class*="title"]')
      if (h && h.textContent && h.textContent.trim().length > 2) return h.textContent.trim().slice(0, 200)
    }
    return ''
  }

  let titleTimer = 0
  function reportTitle() {
    clearTimeout(titleTimer)
    titleTimer = setTimeout(() => {
      send({ type: 'pageInfo', info: { title: document.title || '', og: meta('og:title') || meta('twitter:title') || '', player: playerTitle() } })
    }, 300)
  }
  reportTitle()
  const titleEl = document.querySelector('title')
  if (titleEl) new MutationObserver(reportTitle).observe(titleEl, { childList: true, characterData: true, subtree: true })
  addEventListener('yt-navigate-finish', reportTitle) // YouTube SPA

  // ------------------------------------------------------------ data

  async function refresh() {
    const r = await send({ type: 'getMedia', pageUrl: isTop ? location.href : '' })
    if (Array.isArray(r.items)) items = r.items
    if (r.settings) enabled = r.settings.videoButton !== false && !r.videoHidden
    if (r.prefs && r.prefs.theme) theme = r.prefs.theme
    if (typeof r.frameId === 'number') frameId = r.frameId
    ytdlp = !!r.ytdlp
  }

  chrome.runtime.onMessage.addListener((msg) => {
    if (msg && msg.type === 'fluxdm:media') {
      items = msg.items || []
      if (host && host.style.display !== 'none' && !menuHasFocus()) render()
    }
  })

  /** Media relevant for this frame, plus the page itself for yt-dlp sites. */
  function candidates() {
    const mine = items.filter((m) => m.frameId === frameId)
    const list = (mine.length ? mine : items).slice()
    const v = currentVideo
    const src = v && (v.currentSrc || v.src)
    if (src && /^https?:/i.test(src) && !list.some((m) => m.url === src)) {
      const m = { kind: 'direct', url: src, key: 'v:' + src, frameId, mime: '', title: '' }
      list.unshift({ ...m, view: F.describe(m, null, LANG) })
    }
    if (ytdlp && isTop) {
      const m = { kind: 'page', url: location.href, key: 'page:' + location.href, frameId, title: document.title }
      list.unshift({ ...m, view: F.describe(m, null, LANG) })
    }
    return list
  }

  // ------------------------------------------------------------ UI

  const CSS = `
  :host{all:initial}
  .wrap{--bg:rgba(255,255,255,.86);--menu:rgba(255,255,255,.96);--fg:#242424;--muted:#616161;--stroke:rgba(0,0,0,.12);
    --hover:rgba(0,0,0,.06);--chip:rgba(0,0,0,.04);--accent:#0f6cbd;--accent-fg:#fff;--ok:#107c10;--err:#c42b1c;--focus:#000;
    --shadow:0 8px 16px rgba(0,0,0,.14),0 0 2px rgba(0,0,0,.12);
    font:600 13px/1.25 "Segoe UI Variable Text","Segoe UI Variable","Segoe UI",system-ui,-apple-system,Roboto,Arial,sans-serif;color:var(--fg)}
  .wrap[data-theme=dark]{--bg:rgba(32,32,32,.82);--menu:rgba(41,41,41,.96);--fg:#fff;--muted:#adadad;--stroke:rgba(255,255,255,.14);
    --hover:rgba(255,255,255,.1);--chip:rgba(255,255,255,.06);--accent:#479ef5;--accent-fg:#000;--ok:#54b054;--err:#ff99a4;--focus:#fff;
    --shadow:0 8px 16px rgba(0,0,0,.4),0 0 2px rgba(0,0,0,.3)}
  .pill{display:flex;align-items:stretch;border-radius:999px;overflow:hidden;background:var(--bg);
    -webkit-backdrop-filter:blur(20px) saturate(160%);backdrop-filter:blur(20px) saturate(160%);
    border:1px solid var(--stroke);box-shadow:var(--shadow);opacity:0;transform:translateY(-4px);
    transition:opacity 167ms cubic-bezier(0,0,0,1),transform 167ms cubic-bezier(0,0,0,1)}
  .pill.show{opacity:1;transform:none}
  button{font:inherit;color:inherit;background:none;border:0;margin:0;cursor:pointer;-webkit-appearance:none;appearance:none}
  button:focus{outline:none}
  button:focus-visible{outline:2px solid var(--focus);outline-offset:-3px;border-radius:8px}
  .pill button:focus-visible{border-radius:999px}
  .main,.more,.x{display:flex;align-items:center;gap:7px;padding:7px 12px}
  .main:hover,.more:hover,.x:hover{background:var(--hover)}
  .ic{flex:none;display:block}
  .dot{opacity:.55}
  .q{background:var(--accent);color:var(--accent-fg);border-radius:4px;padding:1px 6px;font-size:11.5px}
  .more{padding:7px 8px;border-left:1px solid var(--stroke)}
  .more[aria-expanded=true] .ic{transform:rotate(180deg)}
  .x{padding:7px 8px;color:var(--muted);border-left:1px solid var(--stroke)}
  .menu{margin-top:8px;width:340px;max-height:380px;overflow:auto;border-radius:12px;padding:6px;
    background:var(--menu);-webkit-backdrop-filter:blur(24px);backdrop-filter:blur(24px);
    border:1px solid var(--stroke);box-shadow:var(--shadow);display:none}
  .menu.open{display:block}
  .card{padding:8px 10px;border-radius:8px}
  .card+.card{border-top:1px solid var(--stroke)}
  .t{font-weight:600;font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .sub{font-weight:400;color:var(--muted);font-size:12px;margin:3px 0 7px;display:flex;gap:6px;flex-wrap:wrap}
  .chips{display:flex;flex-wrap:wrap;gap:6px}
  .chip{display:flex;flex-direction:column;gap:2px;padding:6px 9px;border-radius:8px;text-align:left;
    background:var(--chip);border:1px solid var(--stroke);min-width:64px}
  .chip:hover,.chip:focus-visible{background:var(--accent);border-color:var(--accent);color:var(--accent-fg)}
  .chip b{font-size:13px}.chip span{font-weight:400;font-size:11.5px;color:var(--muted)}
  .chip:hover span,.chip:focus-visible span{color:var(--accent-fg)}
  .chip:focus-visible{outline-offset:2px}
  .item{display:flex;align-items:center;gap:8px;width:100%;padding:8px 10px;border-radius:8px;font-weight:400;color:var(--muted);text-align:left}
  .item:hover,.item:focus-visible{background:var(--hover);color:var(--fg)}
  .sep{height:1px;background:var(--stroke);margin:4px 0}
  .skel{height:30px;border-radius:8px;background:linear-gradient(90deg,var(--chip),var(--hover),var(--chip));
    background-size:200% 100%;animation:sh 1.2s linear infinite;flex:1}
  @keyframes sh{to{background-position:-200% 0}}
  .hint{color:var(--muted);font-weight:400;padding:10px;font-size:12.5px}
  .toast{margin-top:8px;display:none;align-items:center;gap:8px;padding:8px 12px;border-radius:8px;width:max-content;max-width:340px;
    background:var(--menu);border:1px solid var(--stroke);box-shadow:var(--shadow);text-align:left}
  .toast.show{display:flex}
  .toast .ic{color:var(--ok)}
  .toast.err .ic{color:var(--err)}
  .toast u{color:var(--accent);text-decoration:none;font-weight:600;margin-left:4px}
  .toast:hover u{text-decoration:underline}
  @media (prefers-reduced-motion: reduce){.pill{transition:none}.skel{animation:none}}
  `

  function el(tag, cls, text) {
    const e = document.createElement(tag)
    if (cls) e.className = cls
    if (text != null) e.textContent = text
    return e
  }

  function resolvedTheme() {
    if (theme === 'dark' || theme === 'light') return theme
    return matchMedia && matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }

  function ensureHost() {
    if (host) return
    host = document.createElement('div')
    host.style.cssText = 'position:fixed;z-index:2147483647;top:0;left:0;display:none;'
    root = host.attachShadow({ mode: 'closed' })
    root.innerHTML = `<style>${CSS}</style><div class="wrap">
      <div class="pill"><button type="button" class="main"></button><button type="button" class="more" aria-haspopup="menu" aria-expanded="false" aria-controls="fdm-menu">${F.icon('chevronDown', 16)}</button><button type="button" class="x">${F.icon('close', 14)}</button></div>
      <div class="menu" id="fdm-menu" role="menu"></div><div class="live" role="status" aria-live="polite"><button type="button" class="toast"></button></div></div>`
    const more = root.querySelector('.more')
    more.title = MSG('qualities')
    more.setAttribute('aria-label', MSG('qualities'))
    const x = root.querySelector('.x')
    x.title = MSG('hideButton')
    x.setAttribute('aria-label', MSG('hideButton'))
    root.querySelector('.menu').setAttribute('aria-label', MSG('qualities'))
    root.querySelector('.main').addEventListener('click', (e) => { e.stopPropagation(); quickDownload() })
    more.addEventListener('click', (e) => { e.stopPropagation(); toggleMenu(!menuOpen, e.detail === 0) })
    more.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); toggleMenu(true, true, e.key === 'ArrowUp') }
    })
    x.addEventListener('click', (e) => {
      e.stopPropagation()
      hideNow()
      if (currentVideo) currentVideo.__fluxdmHidden = true
    })
    root.querySelector('.menu').addEventListener('keydown', menuKeys)
    root.querySelector('.toast').addEventListener('click', (e) => {
      e.stopPropagation()
      send({ type: 'show', id: root.querySelector('.toast').dataset.id || undefined })
      root.querySelector('.toast').classList.remove('show')
    })
    host.addEventListener('mouseenter', () => clearTimeout(hideTimer))
    host.addEventListener('mouseleave', scheduleHide)
    host.addEventListener('focusin', () => clearTimeout(hideTimer))
    host.addEventListener('focusout', () => { if (!menuHasFocus()) scheduleHide() })
    host.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        if (menuOpen) { toggleMenu(false); more.focus() }
      }
    })
    ;(document.body || document.documentElement).appendChild(host)
  }

  function menuHasFocus() {
    return !!(root && root.activeElement && root.querySelector('.menu').contains(root.activeElement))
  }

  function menuItems() {
    return [...root.querySelectorAll('.menu [role=menuitem]')]
  }

  function menuKeys(e) {
    const list = menuItems()
    if (!list.length) return
    const i = list.indexOf(root.activeElement)
    let n = -1
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight') n = (i + 1) % list.length
    else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') n = (i - 1 + list.length) % list.length
    else if (e.key === 'Home') n = 0
    else if (e.key === 'End') n = list.length - 1
    else if (e.key === 'Tab') { toggleMenu(false); return }
    if (n >= 0) { e.preventDefault(); e.stopPropagation(); list[n].focus() }
  }

  function toggleMenu(open, focusFirst, focusLast) {
    menuOpen = open
    root.querySelector('.more').setAttribute('aria-expanded', String(open))
    render()
    if (open) {
      clearTimeout(hideTimer)
      inspectAll()
      if (focusFirst) {
        const list = menuItems()
        const t = focusLast ? list[list.length - 1] : list[0]
        if (t) t.focus()
      }
    }
  }

  function pillLabel() {
    const c = candidates()[0]
    const main = root.querySelector('.main')
    main.innerHTML = F.icon('download', 16)
    const label = []
    if (!c || c.kind === 'page') {
      const t = MSG('downloadVideo')
      main.appendChild(el('span', '', t))
      label.push(t)
    } else {
      const v = c.view || F.describe(c, c.info, LANG)
      if (v.quality) { main.appendChild(el('span', 'q', v.quality)); label.push(v.quality) }
      main.appendChild(el('span', '', v.format))
      label.push(v.format)
      if (v.size) { main.appendChild(el('span', 'dot', '·')); main.appendChild(el('span', '', v.size)); label.push(v.size) }
    }
    main.setAttribute('aria-label', MSG('downloadWith', [label.join(', ')]))
    main.title = MSG('downloadWith', [label.join(', ')])
  }

  function menuItem(cls, onClick) {
    const b = el('button', cls)
    b.type = 'button'
    b.setAttribute('role', 'menuitem')
    b.tabIndex = -1
    b.addEventListener('click', (e) => { e.stopPropagation(); onClick() })
    return b
  }

  function render() {
    if (!root) return
    root.querySelector('.wrap').dataset.theme = resolvedTheme()
    pillLabel()
    const box = root.querySelector('.menu')
    box.classList.toggle('open', menuOpen)
    if (!menuOpen) return
    const focusedKey = root.activeElement && root.activeElement.dataset ? root.activeElement.dataset.k : ''
    box.textContent = ''
    const list = candidates()
    if (!list.length) box.appendChild(el('div', 'hint', MSG('noMedia')))
    for (const c of list.slice(0, 6)) {
      const v = c.view || F.describe(c, c.info, LANG)
      const card = el('div', 'card')
      card.setAttribute('role', 'group')
      card.setAttribute('aria-label', v.title)
      card.appendChild(el('div', 't', v.title))
      const sub = el('div', 'sub')
      for (const s of [v.source, v.duration, v.live ? MSG('live') : '']) if (s) sub.appendChild(el('span', '', s))
      card.appendChild(sub)
      const chips = el('div', 'chips')
      const quals = c.info && c.info.qualities
      if (quals && quals.length) {
        for (const q of quals.slice(0, 8)) {
          const b = menuItem('chip', () => download(c, q.id))
          const name = q.label === 'Original' ? (q.ext || '').toUpperCase() : q.label
          const detail = [q.ext && q.label !== 'Original' ? q.ext.toUpperCase() : '', F.fmtSize(q.sizeEstimate, c.kind !== 'direct', LANG)].filter(Boolean).join(' · ')
          b.dataset.k = c.key + '|' + q.id
          b.appendChild(el('b', '', name))
          b.appendChild(el('span', '', detail))
          b.setAttribute('aria-label', MSG('downloadQuality', [name + (detail ? ', ' + detail : '')]))
          chips.appendChild(b)
        }
      } else {
        if (c.kind !== 'page' && !c.info) chips.appendChild(el('div', 'skel'))
        const b = menuItem('chip', () => download(c, ''))
        b.dataset.k = c.key + '|'
        b.appendChild(el('b', '', c.kind === 'page' ? MSG('chooseQualityShort') : v.format))
        b.appendChild(el('span', '', c.kind === 'page' ? 'yt-dlp' : v.size || ''))
        chips.appendChild(b)
      }
      card.appendChild(chips)
      box.appendChild(card)
    }
    box.appendChild(el('div', 'sep'))
    const hideSite = menuItem('item', hideOnSite)
    hideSite.dataset.k = 'hide-site'
    hideSite.innerHTML = F.icon('eyeOff', 16)
    hideSite.appendChild(el('span', '', MSG('hideOnSite')))
    box.appendChild(hideSite)
    // keep the keyboard position across re-renders (inspection results arriving)
    if (focusedKey) {
      const t = menuItems().find((x) => x.dataset.k === focusedKey)
      if (t) t.focus()
    }
  }

  async function inspectAll() {
    for (const c of candidates().slice(0, 4)) {
      if (c.info || c.kind === 'page') continue
      const r = await send({ type: 'inspectNow', item: c })
      if (r && r.info) {
        c.info = r.info
        c.view = F.describe(c, r.info, LANG)
        const i = items.findIndex((x) => x.key === c.key)
        if (i >= 0) items[i] = c
        render()
      }
    }
  }

  function toast(text, err, id) {
    const t = root.querySelector('.toast')
    t.textContent = ''
    t.className = 'toast show' + (err ? ' err' : '')
    t.insertAdjacentHTML('beforeend', F.icon(err ? 'error' : 'check', 16))
    t.appendChild(el('span', '', text))
    t.appendChild(el('u', '', MSG('openApp')))
    t.dataset.id = id || ''
    t.title = MSG('openApp')
    clearTimeout(toastTimer)
    clearTimeout(hideTimer)
    toastTimer = setTimeout(() => { t.classList.remove('show'); scheduleHide() }, err ? 6000 : 4000)
  }

  async function download(c, quality) {
    if (busy) return // double click guard
    busy = true
    toggleMenu(false)
    root.querySelector('.main').focus()
    const title = isTop ? document.title : ''
    let r
    try {
      if (c.kind === 'page') r = await send({ type: 'downloadPage', pageUrl: c.url, title })
      else r = await send({ type: 'downloadMedia', item: c, title, quality, quick: !!quality }) // a chosen quality downloads directly
    } finally {
      busy = false
    }
    toast(r && r.error ? r.error : MSG('sent'), !!(r && r.error), r && r.id)
  }

  async function hideOnSite() {
    toggleMenu(false)
    const r = await send({ type: 'siteRule', list: 'videoSkipSites', on: true })
    if (r && r.error) { toast(r.error, true); return }
    enabled = false
    hideNow()
  }

  function quickDownload() {
    const c = candidates()[0]
    if (!c) { toggleMenu(true, true); return }
    // pages (yt-dlp) and un-inspected streams open the quality dialog in FluxDM
    download(c, c.info ? c.info.defaultQuality : '')
  }

  function hideNow() {
    if (!host) return
    menuOpen = false
    root.querySelector('.more').setAttribute('aria-expanded', 'false')
    host.style.display = 'none'
  }

  function place(v) {
    const r = v.getBoundingClientRect()
    if (r.width < 220 || r.height < 120 || r.bottom < 0 || r.top > innerHeight) {
      if (!menuOpen) host.style.display = 'none'
      return
    }
    host.style.display = 'block'
    const w = host.offsetWidth || 220
    host.style.left = Math.max(6, Math.min(innerWidth - w - 6, r.right - w - 12)) + 'px'
    host.style.top = Math.max(6, r.top + 12) + 'px'
    requestAnimationFrame(() => root.querySelector('.pill').classList.add('show'))
  }

  function scheduleHide() {
    clearTimeout(hideTimer)
    hideTimer = setTimeout(() => {
      if (menuOpen || !host) return
      const a = root.activeElement
      if (a && a.matches && a.matches(':focus-visible')) return // keyboard user inside the pill
      if (root.querySelector('.toast').classList.contains('show')) return
      root.querySelector('.pill').classList.remove('show')
      setTimeout(() => { if (!menuOpen) host.style.display = 'none' }, 180)
    }, 3000)
  }

  async function showFor(v) {
    if (!v || v.__fluxdmHidden) return
    currentVideo = v
    await refresh()
    if (!enabled) { hideNow(); return }
    ensureHost()
    render()
    place(v)
    scheduleHide()
    reportTitle()
  }

  document.addEventListener('play', (e) => { if (e.target instanceof HTMLVideoElement) showFor(e.target) }, true)
  document.addEventListener('mouseover', (e) => {
    if (!(e.target instanceof Element) || e.target === host) return
    let v = e.target instanceof HTMLVideoElement ? e.target : null
    if (!v && document.elementsFromPoint) v = document.elementsFromPoint(e.clientX, e.clientY).find((x) => x instanceof HTMLVideoElement) || null
    if (v && v !== currentVideo) showFor(v)
    else if (v && host && enabled && !v.__fluxdmHidden) { place(v); scheduleHide() }
  }, { passive: true, capture: true })

  const reposition = () => { if (currentVideo && host && host.style.display !== 'none') place(currentVideo) }
  addEventListener('scroll', reposition, { passive: true, capture: true })
  addEventListener('resize', reposition, { passive: true })
  document.addEventListener('fullscreenchange', () => {
    if (!host) return
    const fs = document.fullscreenElement
    ;(fs && fs.tagName !== 'VIDEO' ? fs : document.body || document.documentElement).appendChild(host)
    reposition()
  })
  document.addEventListener('click', (e) => {
    if (!menuOpen || (host && e.composedPath && e.composedPath().includes(host))) return
    toggleMenu(false)
  }, true)
  if (matchMedia) {
    const mq = matchMedia('(prefers-color-scheme: dark)')
    if (mq.addEventListener) mq.addEventListener('change', () => { if (root) render() })
  }

  refresh()
})()
