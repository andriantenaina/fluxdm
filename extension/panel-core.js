// FluxDM — shared UI of the popup and the side panel: connection header,
// detected media cards, live downloads (/v1/active), page actions, switches.
// FDMPanel.mount({ mode: 'popup' | 'side' }) builds the page into <body>.
;(function (root) {
  'use strict'
  const F = root.FDM
  const M = (k, subs) => F.msg(k, subs)
  const LANG = F.uiLang()
  const POLL_MS = 1000

  function send(msg) {
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendMessage(msg, (r) => { void chrome.runtime.lastError; resolve(r || { error: M('errUnreachable'), code: 'unreachable' }) })
      } catch (e) {
        resolve({ error: String(e.message || e), code: 'app' })
      }
    })
  }

  function el(tag, cls, text) {
    const e = document.createElement(tag)
    if (cls) e.className = cls
    if (text != null) e.textContent = text
    return e
  }

  function iconBtn(name, label, cls) {
    const b = el('button', 'icon-btn' + (cls ? ' ' + cls : ''))
    b.type = 'button'
    b.innerHTML = F.icon(name, 18)
    b.title = label
    b.setAttribute('aria-label', label)
    return b
  }

  /** Set a status line: kind = ok | err | busy | ''. */
  function setStatus(node, kind, text, extra) {
    node.className = 'status-line' + (kind ? ' ' + kind : '')
    node.textContent = ''
    if (!text) return
    if (kind === 'ok') node.insertAdjacentHTML('beforeend', F.icon('check', 14))
    if (kind === 'err') node.insertAdjacentHTML('beforeend', F.icon('error', 14))
    node.appendChild(el('span', '', text))
    if (extra) node.appendChild(extra)
  }

  const SHELL = `
  <header class="top">
    <img src="icons/icon-32.png" alt="" width="24" height="24" />
    <div class="brand">
      <h1 class="name">FluxDM</h1>
      <div class="conn" id="conn"><span class="dot wait" id="connDot"></span><span id="connText" data-i18n="checking"></span></div>
    </div>
    <button type="button" class="icon-btn" id="openPanel" data-icon="panel" data-i18n-title="openSidePanel" data-i18n-aria="openSidePanel" hidden></button>
    <button type="button" class="icon-btn" id="openApp" data-icon="open" data-i18n-title="openApp" data-i18n-aria="openApp"></button>
    <button type="button" class="icon-btn" id="openOptions" data-icon="settings" data-i18n-title="options" data-i18n-aria="options"></button>
  </header>
  <div class="content">
    <div class="banner" id="offline" role="region" aria-labelledby="offlineTitle" hidden>
      <span data-icon="plug"></span>
      <div class="txt">
        <b id="offlineTitle" data-i18n="notConnected"></b>
        <p id="offlineHelp"></p>
        <button type="button" class="btn primary small" id="launch"><span data-icon="power" data-icon-size="16"></span><span data-i18n="launchApp"></span></button>
        <p class="status-line" id="launchStatus" role="status" aria-live="polite"></p>
      </div>
    </div>

    <section id="activeSec" aria-labelledby="activeH" hidden>
      <h2 class="sec-h"><span id="activeH" data-i18n="inProgress"></span><span id="totalSpeed" class="total"></span></h2>
      <ul class="dl-list" id="active"></ul>
      <p class="status-line" id="activeStatus" role="status" aria-live="polite"></p>
    </section>

    <section aria-labelledby="mediaH">
      <h2 class="sec-h"><span id="mediaH" data-i18n="videos"></span><span id="count" class="count" hidden></span></h2>
      <div class="list" id="list"></div>
    </section>

    <section class="actions" aria-labelledby="actionsH">
      <h2 class="sec-h" id="actionsH" data-i18n="pageActions"></h2>
      <button type="button" class="action" id="page">
        <span class="ico" data-icon="video" data-icon-size="18"></span>
        <span class="lbl"><b data-i18n="downloadPage"></b><small data-i18n="downloadPageHint"></small></span>
      </button>
      <button type="button" class="action" id="all">
        <span class="ico" data-icon="links" data-icon-size="18"></span>
        <span class="lbl"><b data-i18n="downloadAll"></b><small data-i18n="downloadAllHint"></small></span>
      </button>
      <button type="button" class="action subtle" id="skipSite">
        <span class="ico" data-icon="block" data-icon-size="18"></span>
        <span class="lbl"><b id="skipLabel" data-i18n="disableOnSite"></b><small id="skipHost"></small></span>
      </button>
      <p class="status-line" id="actionStatus" role="status" aria-live="polite"></p>
    </section>

    <section id="recentSec" aria-labelledby="recentH" hidden>
      <h2 class="sec-h" id="recentH" data-i18n="recent"></h2>
      <ul class="dl-list" id="recent"></ul>
    </section>
  </div>
  <footer class="bottom">
    <label class="switch"><input type="checkbox" role="switch" id="capture" /><span class="track" aria-hidden="true"></span><span data-i18n="captureOn"></span></label>
    <label class="switch"><input type="checkbox" role="switch" id="button" /><span class="track" aria-hidden="true"></span><span data-i18n="videoButton"></span></label>
  </footer>`

  function mount(opts) {
    const mode = (opts && opts.mode) || 'popup'
    const side = mode === 'side'
    document.body.classList.add(mode)
    document.body.innerHTML = SHELL
    F.localize(document.body)
    document.title = M(side ? 'sidePanelTitle' : 'popupTitle')
    const $ = (id) => document.getElementById(id)

    let tab = null
    let windowId = null
    let items = []
    let status = { connected: null, settings: {} }
    const cardState = new Map() // media key -> { kind, text }
    const inspecting = new Set()
    let activeSupported = true
    let controlSupported = true
    let pollTimer = 0

    // ------------------------------------------------------------ header / connection

    function renderConn() {
      const c = status.connected
      $('connDot').className = 'dot ' + (c === true ? 'ok' : c === false ? 'ko' : 'wait')
      $('connText').textContent = c === true ? M('connected') : c === false ? M('notConnected') : M('checking')
      $('offline').hidden = c !== false
      if (c === false) {
        const h = status.hint
        $('offlineHelp').textContent = M(h === 'hostMissing' ? 'helpHostMissing' : h === 'appMissing' ? 'helpAppMissing' : 'helpNotRunning')
      }
      $('capture').checked = status.captureEnabled !== false
      $('button').checked = !status.settings || status.settings.videoButton !== false
      renderSkip()
    }

    async function loadStatus(cached) {
      status = await send({ type: 'status', cached })
      if (status.error) status = { connected: false, settings: {} }
      renderConn()
    }

    $('launch').onclick = async () => {
      const b = $('launch')
      b.disabled = true
      setStatus($('launchStatus'), 'busy', M('launching'))
      const r = await send({ type: 'launch' })
      b.disabled = false
      if (r.connected) {
        setStatus($('launchStatus'), '', '')
        await loadStatus(false)
        poll()
      } else {
        status.hint = r.hint
        renderConn()
        setStatus($('launchStatus'), 'err', M(r.hint === 'hostMissing' ? 'launchFailedHost' : 'launchFailed'))
      }
    }

    $('openApp').onclick = async () => {
      await send({ type: 'show' })
      if (!side) window.close()
    }
    $('openOptions').onclick = () => {
      chrome.runtime.openOptionsPage()
      if (!side) window.close()
    }
    if (!side && chrome.sidePanel && chrome.sidePanel.open) {
      $('openPanel').hidden = false
      $('openPanel').onclick = () => {
        // called directly from the click (user gesture required)
        const p = chrome.sidePanel.open({ windowId: tab ? tab.windowId : windowId })
        Promise.resolve(p).then(() => window.close()).catch(() => {})
      }
    }

    // ------------------------------------------------------------ switches

    $('capture').onchange = (e) => send({ type: 'setCapture', value: e.target.checked })
    $('button').onchange = async (e) => {
      const r = await send({ type: 'setVideoButton', value: e.target.checked })
      if (r.error) {
        e.target.checked = !e.target.checked
        setStatus($('actionStatus'), 'err', r.error)
      }
    }

    // ------------------------------------------------------------ media cards

    function restricted() {
      return !tab || F.isRestrictedUrl(tab.url)
    }

    function renderMedia() {
      const list = $('list')
      list.textContent = ''
      const count = $('count')
      count.hidden = !items.length
      count.textContent = String(items.length)
      if (restricted() || !items.length) {
        const d = el('div', 'empty')
        d.insertAdjacentHTML('beforeend', F.icon(restricted() ? 'block' : 'video', 28))
        d.appendChild(el('p', '', M(restricted() ? 'restrictedPage' : 'noMedia')))
        list.appendChild(d)
        return
      }
      for (const m of items) list.appendChild(card(m))
    }

    function card(m) {
      const v = m.view || F.describe(m, m.info)
      const st = cardState.get(m.key) || {}
      const c = el('article', 'card media')
      const th = el('div', 'thumb', v.format)
      th.setAttribute('aria-hidden', 'true')
      if (m.info && m.info.thumbnail && /^https?:/i.test(m.info.thumbnail)) {
        th.style.backgroundImage = `url("${m.info.thumbnail.replace(/["\\]/g, '')}")`
        th.textContent = ''
      }
      c.appendChild(th)
      const body = el('div', 'body')
      const t = el('h3', 't', v.title)
      t.title = v.fileName || m.url
      body.appendChild(t)
      const sub = el('div', 'sub')
      sub.appendChild(el('span', 'badge', v.source))
      if (v.duration) sub.appendChild(el('span', '', v.duration))
      if (v.live) sub.appendChild(el('span', 'badge live', M('live')))
      if (v.fileName) sub.appendChild(el('span', 'fn', v.fileName))
      body.appendChild(sub)

      const chips = el('div', 'chips')
      chips.setAttribute('role', 'group')
      chips.setAttribute('aria-label', M('chooseQuality'))
      const qs = m.info && m.info.qualities
      const busy = st.kind === 'busy'
      if (qs && qs.length) {
        for (const q of qs.slice(0, 8)) {
          const b = el('button', 'chip' + (q.id === m.info.defaultQuality ? ' def' : ''))
          b.type = 'button'
          const name = q.label === 'Original' ? (q.ext || 'mp4').toUpperCase() : q.label
          const detail = [q.label !== 'Original' ? (q.ext || '').toUpperCase() : '', F.fmtSize(q.sizeEstimate, m.kind !== 'direct', LANG)].filter(Boolean).join(' · ')
          b.appendChild(el('b', '', name))
          b.appendChild(el('span', '', detail || '—'))
          b.setAttribute('aria-label', M('downloadQuality', [name + (detail ? ', ' + detail : '')]))
          b.disabled = busy
          b.onclick = () => go(m, q.id)
          chips.appendChild(b)
        }
      } else {
        if (!cardState.has(m.key + ':noinfo')) chips.appendChild(el('div', 'skel'))
        const b = el('button', 'chip')
        b.type = 'button'
        b.appendChild(el('b', '', v.format))
        b.appendChild(el('span', '', v.size || M('chooseEllipsis')))
        b.disabled = busy
        b.onclick = () => go(m, '')
        chips.appendChild(b)
        inspect(m)
      }
      body.appendChild(chips)
      const line = el('p', 'status-line')
      line.setAttribute('role', 'status')
      line.setAttribute('aria-live', 'polite')
      if (st.kind) setStatus(line, st.kind, st.text, st.kind === 'ok' ? openLink(st.id) : null)
      body.appendChild(line)
      c.appendChild(body)
      return c
    }

    function openLink(id) {
      const b = el('button', 'link', M('openApp'))
      b.type = 'button'
      b.onclick = () => send({ type: 'show', id })
      return b
    }

    async function inspect(m) {
      if (inspecting.has(m.key) || cardState.has(m.key + ':noinfo') || !tab) return
      inspecting.add(m.key)
      const r = await send({ type: 'inspectNow', tabId: tab.id, item: m })
      inspecting.delete(m.key)
      if (r && r.info) {
        m.info = r.info
        m.view = F.describe(m, r.info, LANG)
      } else {
        cardState.set(m.key + ':noinfo', true)
      }
      renderMedia()
    }

    async function go(m, quality) {
      const st = cardState.get(m.key)
      if (st && st.kind === 'busy') return // double click guard
      cardState.set(m.key, { kind: 'busy', text: M('sending') })
      renderMedia()
      const r = await send({ type: 'downloadMedia', tabId: tab.id, item: m, title: tab.title, quality, quick: !!quality })
      if (r.error) cardState.set(m.key, { kind: 'err', text: r.error })
      else cardState.set(m.key, { kind: 'ok', text: M('sent'), id: r.id })
      if (r.code === 'unreachable') { status.connected = false; status.hint = r.hint; renderConn() }
      renderMedia()
      poll()
    }

    async function loadMedia() {
      if (restricted()) { items = []; renderMedia(); return }
      const r = await send({ type: 'getMedia', tabId: tab.id, pageUrl: tab.url })
      items = r.items || []
      renderMedia()
    }

    chrome.runtime.onMessage.addListener((msg) => {
      if (msg && msg.type === 'fluxdm:popup' && tab && msg.tabId === tab.id) {
        // keep the inspection already fetched by this page
        const prev = new Map(items.map((x) => [x.key, x]))
        items = (msg.items || []).map((x) => (!x.info && prev.get(x.key) && prev.get(x.key).info ? prev.get(x.key) : x))
        renderMedia()
      }
    })

    // ------------------------------------------------------------ page actions

    function renderActions() {
      const dis = restricted()
      $('page').disabled = dis
      $('all').disabled = dis
      renderSkip()
    }

    function renderSkip() {
      const host = tab && !restricted() ? F.siteOf(tab.url) : ''
      const b = $('skipSite')
      b.disabled = !host
      const skipped = host && F.hostMatches(host, (status.settings && status.settings.skipSites) || [])
      $('skipLabel').textContent = M(skipped ? 'enableOnSite' : 'disableOnSite')
      $('skipHost').textContent = host
      b.dataset.on = skipped ? '1' : ''
    }

    async function action(btn, fn) {
      if (btn.getAttribute('aria-busy') === 'true') return
      btn.setAttribute('aria-busy', 'true')
      btn.disabled = true
      setStatus($('actionStatus'), 'busy', M('sending'))
      let r
      try { r = await fn() } finally {
        btn.removeAttribute('aria-busy')
        btn.disabled = false
        renderActions()
      }
      if (r.error) {
        setStatus($('actionStatus'), 'err', r.error)
        if (r.code === 'unreachable') { status.connected = false; status.hint = r.hint; renderConn() }
      }
      return r
    }

    $('page').onclick = async () => {
      const r = await action($('page'), () => send({ type: 'downloadPage', tabId: tab.id, pageUrl: tab.url, title: tab.title }))
      if (r && !r.error) setStatus($('actionStatus'), 'ok', M('sentPage'), openLink(r.id))
      poll()
    }
    $('all').onclick = async () => {
      const r = await action($('all'), () => send({ type: 'downloadAll', tabId: tab.id }))
      if (r && !r.error) setStatus($('actionStatus'), 'ok', M('linksSent', [String(r.count || 0)]), openLink())
    }
    $('skipSite').onclick = async () => {
      const on = $('skipSite').dataset.on !== '1'
      const r = await action($('skipSite'), () => send({ type: 'siteRule', list: 'skipSites', host: tab.url, on }))
      if (r && !r.error) {
        status.settings = r.settings || status.settings
        renderSkip()
        const txt = M(r.on ? 'siteDisabled' : 'siteEnabled', [r.host]) + (r.local ? ' ' + M('savedLocally') : '')
        setStatus($('actionStatus'), 'ok', txt)
      }
    }

    // ------------------------------------------------------------ live downloads

    const rows = new Map() // id -> { li, refs }

    function statusText(it) {
      switch (it.status) {
        case 'queued': return M('stQueued')
        case 'paused': return M('stPaused')
        case 'processing': return M('stProcessing')
        case 'error': return M('stError')
        case 'completed': return M('stCompleted')
        default: return ''
      }
    }

    function makeRow(it) {
      const li = el('li', 'dl')
      const head = el('div', 'dl-head')
      const name = el('span', 'dl-name')
      head.appendChild(name)
      const tools = el('div', 'dl-tools')
      const refs = { li, name, tools }
      if (side && controlSupported) {
        refs.pause = iconBtn('pause', M('pause'), 'small')
        refs.resume = iconBtn('play', M('resume'), 'small')
        refs.cancel = iconBtn('close', M('cancel'), 'small danger')
        refs.pause.onclick = () => control(it.id, 'pause', refs)
        refs.resume.onclick = () => control(it.id, 'resume', refs)
        refs.cancel.onclick = () => control(it.id, 'cancel', refs)
        tools.append(refs.pause, refs.resume, refs.cancel)
      }
      refs.open = iconBtn('open', M('openItem'), 'small')
      refs.open.onclick = () => send({ type: 'show', id: it.id })
      tools.appendChild(refs.open)
      head.appendChild(tools)
      li.appendChild(head)
      refs.bar = el('div', 'bar')
      refs.bar.setAttribute('role', 'progressbar')
      refs.bar.setAttribute('aria-valuemin', '0')
      refs.bar.setAttribute('aria-valuemax', '100')
      refs.fill = el('i')
      refs.bar.appendChild(refs.fill)
      li.appendChild(refs.bar)
      refs.meta = el('div', 'dl-meta')
      li.appendChild(refs.meta)
      return refs
    }

    function updateRow(refs, it) {
      const label = it.fileName || it.title || it.id
      refs.name.textContent = label
      refs.name.title = label
      const pct = F.percent(it.downloaded, it.size)
      const known = it.size > 0
      refs.bar.className = 'bar' + (it.status === 'paused' ? ' paused' : it.status === 'error' ? ' error' : it.status === 'completed' ? ' done' : '') +
        (!known && (it.status === 'downloading' || it.status === 'processing') ? ' indet' : '')
      refs.fill.style.width = (it.status === 'completed' ? 100 : pct).toFixed(1) + '%'
      refs.bar.setAttribute('aria-label', label)
      if (known || it.status === 'completed') refs.bar.setAttribute('aria-valuenow', String(Math.round(it.status === 'completed' ? 100 : pct)))
      else refs.bar.removeAttribute('aria-valuenow')
      const parts = []
      if (it.status === 'downloading') {
        if (known) parts.push(Math.floor(pct) + ' %')
        const sp = F.fmtSpeed(it.speed, LANG)
        if (sp) parts.push(sp)
        const eta = F.fmtEta(it.eta, it.etaConfidence, LANG)
        if (eta !== '—') parts.push(eta)
        const fin = F.fmtFinish(it.eta, LANG)
        refs.meta.title = fin
      } else {
        parts.push(statusText(it))
        if (it.status === 'completed' && it.size) parts.push(F.fmtSize(it.size, false, LANG))
        else if (known && it.status !== 'queued') parts.push(Math.floor(pct) + ' %')
        refs.meta.title = ''
      }
      refs.meta.textContent = parts.filter(Boolean).join(' · ')
      if (refs.pause) {
        refs.pause.hidden = !(it.status === 'downloading' || it.status === 'queued')
        refs.resume.hidden = !(it.status === 'paused' || it.status === 'error')
        refs.cancel.hidden = it.status === 'completed'
        if (!controlSupported) refs.pause.hidden = refs.resume.hidden = refs.cancel.hidden = true
      }
    }

    function syncList(ul, list) {
      const keep = new Set(list.map((x) => x.id))
      for (const [id, r] of rows) {
        if (r.li.parentNode === ul && !keep.has(id)) { r.li.remove(); rows.delete(id) }
      }
      let prev = null
      for (const it of list) {
        let r = rows.get(it.id)
        if (!r) { r = makeRow(it); rows.set(it.id, r) }
        updateRow(r, it)
        const want = prev ? prev.nextSibling : ul.firstChild
        if (r.li !== want || r.li.parentNode !== ul) ul.insertBefore(r.li, want)
        prev = r.li
      }
    }

    async function control(id, act, refs) {
      for (const b of [refs.pause, refs.resume, refs.cancel]) if (b) b.disabled = true
      const r = await send({ type: 'control', id, action: act })
      for (const b of [refs.pause, refs.resume, refs.cancel]) if (b) b.disabled = false
      if (r.error) {
        if (r.code === 'unsupported') { controlSupported = false; setStatus($('activeStatus'), 'err', M('errUnsupported')) }
        else setStatus($('activeStatus'), 'err', r.error)
      } else setStatus($('activeStatus'), '', '')
      poll()
    }

    let polling = false
    async function poll() {
      if (!activeSupported || polling) return
      polling = true
      const r = await send({ type: 'getActive' })
      polling = false
      if (r.code === 'unsupported') {
        activeSupported = false
        $('activeSec').hidden = true
        $('recentSec').hidden = true
        stopPolling()
        return
      }
      if (r.error) {
        $('activeSec').hidden = true
        $('recentSec').hidden = true
        if (r.code === 'unreachable' && status.connected !== false) { status.connected = false; renderConn() }
        return
      }
      if (status.connected === false) loadStatus(false)
      const all = Array.isArray(r.items) ? r.items : []
      const live = all.filter((x) => x.status !== 'completed')
      const done = all.filter((x) => x.status === 'completed')
      const shown = side ? live : live.filter((x) => x.status === 'downloading' || x.status === 'processing').slice(0, 3)
      $('activeSec').hidden = !shown.length
      syncList($('active'), shown)
      $('totalSpeed').textContent = r.speed > 0 ? F.fmtSpeed(r.speed, LANG) : ''
      if (side) {
        $('recentSec').hidden = !done.length
        syncList($('recent'), done.slice(0, 5))
      }
    }

    function startPolling() {
      if (pollTimer || !activeSupported) return
      poll()
      pollTimer = setInterval(poll, POLL_MS)
    }
    function stopPolling() {
      clearInterval(pollTimer)
      pollTimer = 0
    }

    // ------------------------------------------------------------ tab tracking

    async function setTab(t) {
      const changed = !tab || !t || tab.id !== t.id || tab.url !== t.url
      tab = t || null
      if (changed) { cardState.clear(); inspecting.clear(); setStatus($('actionStatus'), '', '') }
      renderActions()
      await loadMedia()
    }

    async function init() {
      try {
        const w = await chrome.windows.getCurrent()
        windowId = w.id
      } catch (e) { /* */ }
      let t = null
      try { [t] = await chrome.tabs.query({ active: true, currentWindow: true }) } catch (e) { /* */ }
      await setTab(t || null)
      await loadStatus(side)
      if (side) {
        chrome.tabs.onActivated.addListener(async (info) => {
          if (windowId != null && info.windowId !== windowId) return
          setTab(await chrome.tabs.get(info.tabId).catch(() => null))
        })
        chrome.tabs.onUpdated.addListener((id, change, t2) => {
          if (!tab || id !== tab.id) return
          if (change.url || change.status === 'complete' || change.title) setTab(t2)
        })
        document.addEventListener('visibilitychange', () => {
          if (document.visibilityState === 'visible') { startPolling(); loadStatus(true) } else stopPolling()
        })
        if (document.visibilityState === 'visible') startPolling()
      } else {
        startPolling()
      }
    }

    init()
  }

  root.FDMPanel = { mount, send }
})(typeof self !== 'undefined' ? self : globalThis)
