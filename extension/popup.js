'use strict'
const $ = (id) => document.getElementById(id)
const M = (k) => chrome.i18n.getMessage(k) || k

function send(msg) {
  return new Promise((resolve) => chrome.runtime.sendMessage(msg, (r) => { void chrome.runtime.lastError; resolve(r || {}) }))
}

function fmtSize(n) {
  if (!n || n < 0) return ''
  const u = ['o', 'Ko', 'Mo', 'Go']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return n.toFixed(i ? 1 : 0) + ' ' + u[i]
}

async function main() {
  $('videosTitle').textContent = M('videos')
  $('quickLabel').textContent = M('quick')
  $('page').textContent = '▶ ' + M('downloadPage')
  $('all').textContent = '⇣ ' + M('downloadAll')
  $('captureLabel').textContent = M('captureOn')
  $('open').textContent = M('openApp')

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  const tabId = tab ? tab.id : -1

  const st = await send({ type: 'status' })
  $('status').textContent = st.connected ? '● ' + M('connected') : '○ ' + M('notConnected')
  $('status').className = 'status ' + (st.connected ? 'ok' : 'ko')
  $('capture').checked = st.captureEnabled !== false
  $('capture').onchange = (e) => send({ type: 'setCapture', value: e.target.checked })
  $('quick').checked = localStorage.getItem('quick') === '1'
  $('quick').onchange = (e) => localStorage.setItem('quick', e.target.checked ? '1' : '0')

  const r = await send({ type: 'getMedia', tabId, pageUrl: tab ? tab.url : '' })
  const list = $('list')
  const items = r.items || []
  if (!items.length) {
    const d = document.createElement('div')
    d.className = 'empty'
    d.textContent = M('noMedia')
    list.appendChild(d)
  }
  for (const m of items) {
    const d = document.createElement('div')
    d.className = 'item'
    let name = m.url
    try { name = decodeURIComponent(new URL(m.url).pathname.split('/').pop()) || new URL(m.url).hostname } catch (e) { /* */ }
    const kind = { hls: 'HLS', dash: 'DASH' }[m.kind] || (m.mime || 'MP4').replace(/^(video|audio)\//, '').toUpperCase()
    d.innerHTML = '<span class="k"></span><span class="n"></span><span class="s"></span>'
    d.querySelector('.k').textContent = kind
    d.querySelector('.n').textContent = name
    d.querySelector('.n').title = m.url
    d.querySelector('.s').textContent = fmtSize(m.size)
    d.onclick = async () => {
      await send({ type: 'downloadMedia', tabId, item: m, title: tab ? tab.title : '', quick: $('quick').checked })
      window.close()
    }
    list.appendChild(d)
  }

  $('page').onclick = async () => {
    await send({ type: 'downloadPage', tabId, pageUrl: tab.url, title: tab.title })
    window.close()
  }
  $('all').onclick = async () => {
    await send({ type: 'downloadAll', tabId })
    window.close()
  }
  $('open').onclick = async () => {
    await send({ type: 'launch' })
    window.close()
  }
}

main()
